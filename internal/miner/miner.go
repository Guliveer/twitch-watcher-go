// Package miner implements the core mining orchestrator for a single Twitch
// account. It wires together authentication, PubSub, chat, notifications,
// minute-watched events, drop campaign syncing, and the category watcher.
package miner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Guliveer/twitch-miner-go/internal/chat"
	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
	"github.com/Guliveer/twitch-miner-go/internal/notify"
	"github.com/Guliveer/twitch-miner-go/internal/pubsub"
	"github.com/Guliveer/twitch-miner-go/internal/twitch"
	"github.com/Guliveer/twitch-miner-go/internal/watcher"
)

// Miner orchestrates all mining activities for a single Twitch account.
// It implements the [pubsub.MessageHandler] interface so the PubSub pool
// can route messages directly to it.
type Miner struct {
	cfg    *config.AccountConfig
	log    *logger.Logger
	twitch twitch.API
	pubsub *pubsub.Pool
	chat   *chat.Manager
	notify *notify.Dispatcher

	running atomic.Bool

	catWatcher *watcher.CategoryWatcher

	streamers   []*model.Streamer
	streamersMu sync.RWMutex

	eventsPredictions   map[string]*model.EventPrediction
	eventsPredictionsMu sync.RWMutex

	pendingTimers   map[string]*time.Timer
	pendingTimersMu sync.Mutex

	priorities []model.Priority

	lastWatching   map[string]bool
	lastWatchingMu sync.Mutex
}

// NewMiner creates a new Miner from account configuration.
func NewMiner(cfg *config.AccountConfig, log *logger.Logger) *Miner {
	return &Miner{
		cfg:               cfg,
		log:               log,
		eventsPredictions: make(map[string]*model.EventPrediction),
		pendingTimers:     make(map[string]*time.Timer),
		priorities:        cfg.ParsedPriorities(),
		lastWatching:      make(map[string]bool),
	}
}

// Streamers returns a snapshot of the current streamer list.
// Exported for use by the analytics server.
func (m *Miner) Streamers() []*model.Streamer {
	return m.getStreamers()
}

// NotifyDispatcher returns the notification dispatcher for this miner.
// May return nil if the miner hasn't been started yet.
func (m *Miner) NotifyDispatcher() *notify.Dispatcher {
	return m.notify
}

// IsRunning reports whether the miner is currently running its main loop.
func (m *Miner) IsRunning() bool {
	return m.running.Load()
}

// Username returns the account username for this miner.
func (m *Miner) Username() string {
	return m.cfg.Username
}

// Run is the main entry point for the miner. It performs the full lifecycle
// with optimized parallel startup:
//  1. Login via Twitch client
//  2. Claim drops on startup (if enabled)
//  3. Resolve streamer channel IDs — concurrent (worker pool)
//  4. Create notification dispatcher
//  5. Create PubSub pool and subscribe to topics — immediately after IDs resolved
//  6. Create chat manager and join channels — immediately
//  7. Load channel points context — concurrent in background (worker pool)
//  8. Check online status — concurrent in background (worker pool)
//  9. Start background goroutines (minute watcher, campaign sync, context refresh)
//  10. Monitor loop + graceful shutdown
func (m *Miner) Run(ctx context.Context) error {
	defer m.running.Store(false)

	startTime := time.Now()
	m.log.Info("🚀 Starting miner", "account", m.cfg.Username)

	tc, err := twitch.NewClient(m.cfg, m.log)
	if err != nil {
		return fmt.Errorf("creating twitch client: %w", err)
	}
	m.twitch = tc

	if err := m.twitch.Login(ctx); err != nil {
		return fmt.Errorf("login failed for %s: %w", m.cfg.Username, err)
	}
	m.log.Info("🔑 Logged in successfully", "account", m.cfg.Username)

	if m.cfg.Features.ClaimDropsStartup {
		m.log.Info("🎯 Claiming pending drops from inventory on startup")
		if err := m.twitch.ClaimAllDropsFromInventory(ctx); err != nil {
			m.log.Warn("Failed to claim drops on startup", "error", err)
		}
	}

	m.twitch.GQLClient().SetStartupMode()
	if err := m.resolveStreamers(ctx); err != nil {
		m.twitch.GQLClient().SetNormalMode()
		return fmt.Errorf("resolving streamers: %w", err)
	}

	m.notify = notify.NewDispatcher(m.cfg.Notifications, m.log)
	m.log.SetNotifyFunc(m.notify.NotifyFunc())

	m.pubsub = pubsub.NewPool(m.twitch.AuthProvider(), m.log, m)

	if err := m.subscribeAllTopics(ctx); err != nil {
		m.twitch.GQLClient().SetNormalMode()
		return fmt.Errorf("subscribing to PubSub topics: %w", err)
	}

	m.chat = chat.NewManager(m.cfg.Username, m.twitch.AuthProvider().AuthToken(), m.log)
	m.joinInitialChats()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return m.pubsub.Run(ctx)
	})

	g.Go(func() error {
		return m.chat.Run(ctx)
	})

	g.Go(func() error {
		m.loadAllChannelPointsContext(ctx)
		m.twitch.GQLClient().SetNormalMode()
		return nil
	})

	g.Go(func() error {
		m.checkAllStreamersOnline(ctx)
		return nil
	})

	g.Go(func() error {
		return m.runMinuteWatcher(ctx)
	})

	g.Go(func() error {
		return m.runCampaignSync(ctx)
	})

	g.Go(func() error {
		return m.runContextRefresh(ctx)
	})

	if m.cfg.CategoryWatcher.Enabled && len(m.cfg.CategoryWatcher.Categories) > 0 {
		defaults := m.getStreamerDefaults()
		m.catWatcher = watcher.NewCategoryWatcher(
			m.cfg.CategoryWatcher,
			m.twitch.GQLClient(),
			m.log,
			m.cfg.Blacklist,
			defaults,
		)
		g.Go(func() error {
			return m.catWatcher.Run(ctx, m.addStreamer, m.removeStreamerWithReason, m.getStreamers)
		})
	}

	g.Go(func() error {
		return m.runMonitorLoop(ctx)
	})

	m.running.Store(true)

	m.log.Info("✅ Miner fully started",
		"account", m.cfg.Username,
		"streamers", len(m.getStreamers()),
		"pubsub_topics", m.pubsub.TotalTopicCount(),
		"startup_duration", time.Since(startTime).Round(time.Millisecond),
	)

	err = g.Wait()

	m.pendingTimersMu.Lock()
	for id, t := range m.pendingTimers {
		t.Stop()
		delete(m.pendingTimers, id)
	}
	m.pendingTimersMu.Unlock()

	return err
}

func (m *Miner) getStreamerDefaults() *model.StreamerSettings {
	return m.cfg.StreamerDefaults.ToStreamerSettings(model.DefaultStreamerSettings())
}

// loadAllChannelPointsContext loads channel points context for all streamers
func (m *Miner) loadAllChannelPointsContext(ctx context.Context) {
	streamers := m.getStreamers()
	if len(streamers) == 0 {
		return
	}

	m.log.Info("Loading channel points context", "count", len(streamers), "workers", constants.StartupWorkers)

	sem := make(chan struct{}, constants.StartupWorkers)
	var wg sync.WaitGroup

	for _, s := range streamers {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(streamer *model.Streamer) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			if err := m.twitch.LoadChannelPointsContext(ctx, streamer); err != nil {
				m.log.Warn("Failed to load channel points context",
					"streamer", streamer.Username, "error", err)
			} else {
				streamer.Mu.RLock()
				balance := streamer.ChannelPoints
				online := streamer.IsOnline
				streamer.Mu.RUnlock()
				if online {
					m.log.Info("💎 Channel points loaded",
						"streamer", streamer.Username,
						"balance", balance)
				} else {
					m.log.Info("⚫ Offline",
						"streamer", streamer.Username,
						"balance", balance)
				}
			}
		}(s)
	}

	wg.Wait()
	m.log.Info("Channel points context loaded", "count", len(streamers))
}

// checkAllStreamersOnline checks online status for all streamers
func (m *Miner) checkAllStreamersOnline(ctx context.Context) {
	streamers := m.getStreamers()
	if len(streamers) == 0 {
		return
	}

	m.log.Info("Checking initial online status", "count", len(streamers), "workers", constants.StartupWorkers)

	sem := make(chan struct{}, constants.StartupWorkers)
	var wg sync.WaitGroup
	var onlineCount, offlineCount int
	var mu sync.Mutex

	for _, s := range streamers {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(streamer *model.Streamer) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			if err := m.twitch.CheckStreamerOnline(ctx, streamer); err != nil {
				m.log.Debug("Failed to check online status",
					"streamer", streamer.Username, "error", err)
				return
			}

				streamer.Mu.RLock()
				isOnline := streamer.IsOnline
				category := streamer.ResolveCategory()
				viewers := 0
				if streamer.Stream != nil {
					viewers = streamer.Stream.ViewersCount
				}
				streamer.Mu.RUnlock()
	
				mu.Lock()
				if isOnline {
					onlineCount++
					mu.Unlock()
					m.log.Info("🟢 Online",
						"streamer", streamer.Username,
						"category", category,
						"viewers", viewers)
				} else {
					offlineCount++
					mu.Unlock()
				}
		}(s)
	}

	wg.Wait()
	m.log.Info("Initial online status check complete",
		"online", onlineCount,
		"offline", offlineCount,
		"total", len(streamers))
}

func (m *Miner) subscribeAllTopics(ctx context.Context) error {
	userID := m.twitch.AuthProvider().UserID()
	streamers := m.getStreamers()

	var topics []*model.PubSubTopic

	topics = append(topics, model.NewUserTopic(model.PubSubTopicCommunityPoints, userID))

	for _, s := range streamers {
		s.Mu.RLock()
		makePred := s.Settings != nil && s.Settings.MakePredictions
		s.Mu.RUnlock()
		if makePred {
			topics = append(topics, model.NewUserTopic(model.PubSubTopicPredictionsUser, userID))
			break
		}
	}

	for _, s := range streamers {
		topics = append(topics, m.streamerTopics(s)...)
	}

	return m.pubsub.Subscribe(ctx, topics)
}

func (m *Miner) streamerTopics(s *model.Streamer) []*model.PubSubTopic {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	var topics []*model.PubSubTopic

	topics = append(topics, model.NewStreamerTopic(model.PubSubTopicVideoPlayback, s))

	if s.Settings == nil {
		return topics
	}

	if s.Settings.FollowRaid {
		topics = append(topics, model.NewStreamerTopic(model.PubSubTopicRaid, s))
	}
	if s.Settings.MakePredictions {
		topics = append(topics, model.NewStreamerTopic(model.PubSubTopicPredictions, s))
	}
	if s.Settings.ClaimMoments {
		topics = append(topics, model.NewStreamerTopic(model.PubSubTopicCommunityMoments, s))
	}
	if s.Settings.CommunityGoalsEnabled {
		topics = append(topics, model.NewStreamerTopic(model.PubSubTopicCommunityGoals, s))
	}

	return topics
}

func (m *Miner) joinInitialChats() {
	streamers := m.getStreamers()
	for _, s := range streamers {
		s.Mu.RLock()
		chatPresence := model.ChatNever
		if s.Settings != nil {
			chatPresence = s.Settings.Chat
		}
		isOnline := s.IsOnline
		username := s.Username
		s.Mu.RUnlock()

		if model.ShouldJoinChat(chatPresence, isOnline) {
			if err := m.chat.Join(username); err != nil {
				m.log.Warn("Failed to join chat", "streamer", username, "error", err)
			}
		}
	}
}

// HandlePubSubMessage implements the [pubsub.MessageHandler] interface.
// It delegates to the handler logic in handler.go.
func (m *Miner) HandlePubSubMessage(ctx context.Context, msg *model.Message) {
	m.handleMessage(ctx, msg)
}
