// Package watcher provides the CategoryWatcher that automatically discovers
// and tracks streamers based on configured game categories.
package watcher

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/gql"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

type categoryEntry struct {
	Slug      string
	GameID    string
	DropsOnly *bool
}

// CategoryWatcher polls Twitch GQL for top streams in configured categories
// and adds/removes streamers dynamically.
type CategoryWatcher struct {
	mu sync.Mutex

	gqlClient        *gql.Client
	log              *logger.Logger
	categories       []categoryEntry
	globalDropsOnly  bool
	pollInterval     time.Duration
	blacklist        map[string]bool
	streamerDefaults *model.StreamerSettings

	categoryStreamers map[string]string
}

// NewCategoryWatcher creates a new CategoryWatcher from configuration.
func NewCategoryWatcher(
	cfg config.CategoryWatcherConfig,
	gqlClient *gql.Client,
	log *logger.Logger,
	blacklist []string,
	streamerDefaults *model.StreamerSettings,
) *CategoryWatcher {
	cats := make([]categoryEntry, 0, len(cfg.Categories))
	for _, c := range cfg.Categories {
		cats = append(cats, categoryEntry{
			Slug:      c.Slug,
			DropsOnly: c.DropsOnly,
		})
	}

	bl := make(map[string]bool, len(blacklist))
	for _, b := range blacklist {
		bl[strings.ToLower(b)] = true
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = constants.DefaultCategoryWatcherInterval
	}

	catStreamers := make(map[string]string, len(cats))
	for _, c := range cats {
		catStreamers[c.Slug] = ""
	}

	return &CategoryWatcher{
		gqlClient:         gqlClient,
		log:               log,
		categories:        cats,
		globalDropsOnly:   cfg.DropsOnly,
		pollInterval:      interval,
		blacklist:         bl,
		streamerDefaults:  streamerDefaults,
		categoryStreamers: catStreamers,
	}
}

// Run starts the category watcher loop. It calls addStreamer when a new streamer
// should be tracked and removeStreamer when a streamer should be removed.
// The function blocks until the context is cancelled.
func (cw *CategoryWatcher) Run(
	ctx context.Context,
	addStreamer func(context.Context, *model.Streamer),
	removeStreamer func(string, string),
	getTrackedStreamers func() []*model.Streamer,
) error {
	cw.log.Info("👁️ CategoryWatcher started",
		"categories", len(cw.categories),
		"poll_interval", cw.pollInterval,
	)

	cw.evaluate(ctx, addStreamer, removeStreamer, getTrackedStreamers)

	ticker := time.NewTicker(cw.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cw.log.Info("👁️ CategoryWatcher stopping")
			cw.mu.Lock()
			for slug, username := range cw.categoryStreamers {
				if username != "" {
					removeStreamer(username, "category_watcher_shutdown")
					cw.categoryStreamers[slug] = ""
				}
			}
			cw.mu.Unlock()
			return ctx.Err()
		case <-ticker.C:
			cw.evaluate(ctx, addStreamer, removeStreamer, getTrackedStreamers)
		}
	}
}

// evaluate checks all configured categories and adds/removes streamers as needed.
// Streamers are only removed when they go offline or change category — NOT when
// another streamer has more viewers. New streamers are only added when the slot
func (cw *CategoryWatcher) evaluate(
	ctx context.Context,
	addStreamer func(context.Context, *model.Streamer),
	removeStreamer func(string, string),
	getTrackedStreamers func() []*model.Streamer,
) {
	for i := range cw.categories {
		cat := &cw.categories[i]

		if ctx.Err() != nil {
			return
		}

		dropsOnly := cw.globalDropsOnly
		if cat.DropsOnly != nil {
			dropsOnly = *cat.DropsOnly
		}

		trackedStreamers := getTrackedStreamers()

		if cw.isCategoryCovered(trackedStreamers, cat) {
			cw.mu.Lock()
			current := cw.categoryStreamers[cat.Slug]
			if current != "" {
				cw.log.Debug("Category now covered by regular streamer, removing category watcher",
					"category", cat.Slug)
				removeStreamer(current, "category_covered_by_regular")
				cw.categoryStreamers[cat.Slug] = ""
			}
			cw.mu.Unlock()
			continue
		}

		cw.mu.Lock()
		currentUsername := cw.categoryStreamers[cat.Slug]
		cw.mu.Unlock()

		if currentUsername != "" {
			valid, reason := cw.checkStreamerValidity(trackedStreamers, currentUsername, cat)
			if valid {
				continue // Still online and in the right category — keep them.
			}
			removeStreamer(currentUsername, reason)
			cw.mu.Lock()
			cw.categoryStreamers[cat.Slug] = ""
			cw.mu.Unlock()
		}

		streams, err := cw.gqlClient.GetTopStreamsByCategory(ctx, cat.Slug, 10, dropsOnly)
		if err != nil {
			cw.log.Warn("Failed to fetch top streams for category",
				"category", cat.Slug,
				"error", err,
			)
			continue
		}

		if len(streams) == 0 {
			filterNote := ""
			if dropsOnly {
				filterNote = " (drops-only filter active)"
			}
			cw.log.Info("No live streams for category"+filterNote,
				"category", cat.Slug,
			)
			continue
		}

		if cat.GameID == "" && streams[0].GameID != "" {
			cat.GameID = streams[0].GameID
			cw.log.Info("Resolved category to game ID",
				"category", cat.Slug,
				"game_id", cat.GameID,
			)
		}

		// Register game ID → slug mappings from the API response so that
		// GameSlug() can resolve slugs for streamers outside category watch.
		if cat.GameID != "" && cat.Slug != "" {
			model.RegisterGameSlug(cat.GameID, cat.Slug)
		}
		for _, s := range streams {
			if s.GameID != "" && s.GameSlug != "" {
				model.RegisterGameSlug(s.GameID, s.GameSlug)
			}
		}

		existingIDs := make(map[string]bool, len(trackedStreamers))
		for _, s := range trackedStreamers {
			s.Mu.RLock()
			existingIDs[s.ChannelID] = true
			s.Mu.RUnlock()
		}

		var candidate *gql.TopStream
		for i := range streams {
			s := &streams[i]
			if existingIDs[s.ChannelID] {
				continue
			}
			if cw.blacklist[strings.ToLower(s.Username)] {
				continue
			}
			candidate = s
			break
		}

		if candidate == nil {
			cw.log.Info("All top streams for category are already tracked",
				"category", cat.Slug,
			)
			continue
		}

		streamer := model.NewStreamer(candidate.Username)
		streamer.ChannelID = candidate.ChannelID
		streamer.DisplayName = candidate.DisplayName
		streamer.IsCategoryWatched = true
		streamer.CategorySlug = cat.Slug

		streamer.IsOnline = true
		streamer.OnlineAt = time.Now()
		stream := model.NewStream()
		stream.Game = &model.GameInfo{
			ID:   cat.GameID,
			Slug: cat.Slug,
			Name: candidate.GameName,
		}
		stream.ViewersCount = candidate.ViewersCount
		stream.MarkUpdated()
		streamer.Stream = stream

		defaults := *cw.streamerDefaults
		if defaults.Bet != nil {
			betCopy := *defaults.Bet
			if betCopy.FilterCondition != nil {
				fcCopy := *betCopy.FilterCondition
				betCopy.FilterCondition = &fcCopy
			}
			defaults.Bet = &betCopy
			}
			defaults.FollowRaid = false
			streamer.Settings = &defaults

		cw.mu.Lock()
		cw.categoryStreamers[cat.Slug] = candidate.Username
		cw.mu.Unlock()

		addStreamer(ctx, streamer)

		cw.log.Info("🔍 Discovered via category",
			"streamer", candidate.Username,
			"category", cat.Slug,
			"viewers", candidate.ViewersCount,
		)
	}
}

func (cw *CategoryWatcher) isCategoryCovered(streamers []*model.Streamer, cat *categoryEntry) bool {
	for _, s := range streamers {
		s.Mu.RLock()
		isCatWatched := s.IsCategoryWatched
		isOnline := s.IsOnline
		matches := cw.streamerMatchesCategory(s, cat)
		s.Mu.RUnlock()

		if !isCatWatched && isOnline && matches {
			return true
		}
	}
	return false
}

// checkStreamerValidity checks if a category-watched streamer is still valid.
// Returns (true, "") if the streamer is still online and streaming the right category.
// Returns (false, reason) with a descriptive reason if the streamer should be removed:
//   - "streamer_went_offline" if the streamer is no longer online
//   - "streamer_changed_category" if the streamer switched to a different game
func (cw *CategoryWatcher) checkStreamerValidity(streamers []*model.Streamer, username string, cat *categoryEntry) (bool, string) {
	for _, s := range streamers {
		s.Mu.RLock()
		if s.Username != username {
			s.Mu.RUnlock()
			continue
		}
		isOnline := s.IsOnline
		matches := cw.streamerMatchesCategory(s, cat)
		s.Mu.RUnlock()

		if !isOnline {
			return false, "streamer_went_offline"
		}
		if !matches {
			return false, "streamer_changed_category"
		}
		return true, ""
	}
	return false, "streamer_not_found"
}

// streamerMatchesCategory checks if a streamer's current game matches the category.
// Uses immutable Game ID for comparison when available, falling back to slug comparison
// before the Game ID has been resolved (first poll).
func (cw *CategoryWatcher) streamerMatchesCategory(s *model.Streamer, cat *categoryEntry) bool {
	if s.Stream == nil || s.Stream.Game == nil {
		return false
	}

	if cat.GameID != "" && s.Stream.Game.ID != "" {
		return cat.GameID == s.Stream.Game.ID
	}

	game := s.Stream.Game
	if game.Slug != "" {
		return strings.EqualFold(game.Slug, cat.Slug)
	}

	return false
}
