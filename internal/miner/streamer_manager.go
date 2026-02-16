package miner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

func (m *Miner) getStreamers() []*model.Streamer {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()
	result := make([]*model.Streamer, len(m.streamers))
	copy(result, m.streamers)
	return result
}

func (m *Miner) getStreamerByChannelID(channelID string) *model.Streamer {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()
	for _, s := range m.streamers {
		s.Mu.RLock()
		streamerChannelID := s.ChannelID
		s.Mu.RUnlock()
		if streamerChannelID == channelID {
			return s
		}
	}
	return nil
}

// addStreamer adds a new streamer to the list and subscribes to its PubSub topics.
func (m *Miner) addStreamer(ctx context.Context, s *model.Streamer) {
	if s.AccountUsername == "" {
		s.AccountUsername = m.cfg.Username
	}
	m.streamersMu.Lock()
	m.streamers = append(m.streamers, s)
	m.streamersMu.Unlock()

	topics := m.streamerTopics(s)
	if err := m.pubsub.Subscribe(ctx, topics); err != nil {
		m.log.Warn("Failed to subscribe to topics for new streamer",
			"streamer", s.Username, "error", err)
	}

	if !s.IsCategoryWatched {
		m.log.Info("➕ Added",
			"streamer", s.Username,
			"channel_id", s.ChannelID,
		)
	}
}

func (m *Miner) removeStreamer(username string) {
	m.removeStreamerWithReason(username, "")
}

// removeStreamerWithReason removes a streamer by username with an optional reason,
// and unsubscribes from its PubSub topics.
func (m *Miner) removeStreamerWithReason(username string, reason string) {
	m.streamersMu.Lock()
	var removed *model.Streamer
	for i, s := range m.streamers {
		if strings.EqualFold(s.Username, username) {
			removed = s
			m.streamers = append(m.streamers[:i], m.streamers[i+1:]...)
			break
		}
	}
	m.streamersMu.Unlock()

	if removed == nil {
		m.log.Warn("Not found for removal", "streamer", username)
		return
	}

	if err := m.pubsub.UnsubscribeStreamer(removed); err != nil {
		m.log.Warn("Failed to unsubscribe streamer topics",
			"streamer", username, "error", err)
	}

	if m.chat.IsJoined(username) {
		if err := m.chat.Leave(username); err != nil {
			m.log.Debug("Failed to leave chat", "streamer", username, "error", err)
		}
	}

	logFields := []any{"streamer", username}
	if reason != "" {
		logFields = append(logFields, "reason", reason)
	}
	removed.Mu.RLock()
	if removed.CategorySlug != "" {
		logFields = append(logFields, "category", removed.CategorySlug)
	}
	removed.Mu.RUnlock()

	m.log.Info("➖ Removed", logFields...)
}

// resolveStreamers resolves channel IDs for all configured streamers,
// including followers if enabled. Uses a concurrent worker pool for
func (m *Miner) resolveStreamers(ctx context.Context) error {
	defaults := m.getStreamerDefaults()

	blacklist := make(map[string]bool, len(m.cfg.Blacklist))
	for _, blacklistedName := range m.cfg.Blacklist {
		blacklist[strings.ToLower(blacklistedName)] = true
	}

	var usernames []string
	settingsMap := make(map[string]*config.StreamerSettingsConfig)

	for _, sc := range m.cfg.Streamers {
		username := strings.ToLower(strings.TrimSpace(sc.Username))
		if blacklist[username] {
			continue
		}
		usernames = append(usernames, username)
		settingsMap[username] = sc.Settings
	}

	if m.cfg.Followers.Enabled {
		followers, err := m.twitch.GetFollowers(ctx, 100, m.cfg.Followers.Order)
		if err != nil {
			m.log.Warn("Failed to load followers", "error", err)
		} else {
			m.log.Info("📋 Loaded followers", "count", len(followers))
			existing := make(map[string]bool, len(usernames))
			for _, u := range usernames {
				existing[u] = true
			}
			for _, followerLogin := range followers {
					followerLower := strings.ToLower(followerLogin)
					if !existing[followerLower] && !blacklist[followerLower] {
						usernames = append(usernames, followerLower)
				}
			}
		}
	}

	m.log.Info("Resolving channel IDs", "count", len(usernames), "workers", constants.StartupWorkers)

	type resolveResult struct {
		streamer *model.Streamer
		index    int // preserve original order
	}

	results := make(chan resolveResult, len(usernames))
	sem := make(chan struct{}, constants.StartupWorkers)
	var wg sync.WaitGroup

	for i, username := range usernames {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(idx int, username string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			channelID, err := m.twitch.GetChannelID(ctx, username)
			if err != nil {
				m.log.Warn("Failed to resolve channel ID, skipping",
					"streamer", username, "error", err)
				return
			}
			if channelID == "" {
				m.log.Warn("Empty channel ID, skipping", "streamer", username)
				return
			}

			streamer := model.NewStreamer(username)
			streamer.ChannelID = channelID
			streamer.AccountUsername = m.cfg.Username

			streamerSettingsCfg := settingsMap[username]
			streamer.Settings = (&config.StreamerSettingsConfig{}).ToStreamerSettings(defaults)
			if streamerSettingsCfg != nil {
				streamer.Settings = streamerSettingsCfg.ToStreamerSettings(defaults)
			}

			m.log.Info("📋 Loaded",
				"streamer", username, "channel_id", channelID)

			results <- resolveResult{streamer: streamer, index: idx}
		}(i, username)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	collected := make([]resolveResult, 0, len(usernames))
	for r := range results {
		collected = append(collected, r)
	}

	resolved := make([]*model.Streamer, 0, len(collected))
	sortedResults := make(map[int]*model.Streamer, len(collected))
	for _, r := range collected {
		sortedResults[r.index] = r.streamer
	}
	for i := 0; i < len(usernames); i++ {
		if s, ok := sortedResults[i]; ok {
			resolved = append(resolved, s)
		}
	}

	if len(resolved) == 0 && !m.cfg.CategoryWatcher.Enabled {
		return fmt.Errorf("no streamers could be resolved for account %s", m.cfg.Username)
	}

	m.streamersMu.Lock()
	m.streamers = resolved
	m.streamersMu.Unlock()

	m.log.Info("📋 Streamers resolved", "count", len(resolved))
	return nil
}
