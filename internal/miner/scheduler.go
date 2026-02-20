package miner

import (
	"context"
	"math/rand/v2"
	"runtime"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/model"
	"github.com/Guliveer/twitch-miner-go/internal/twitch"
)

func (m *Miner) runMinuteWatcher(ctx context.Context) error {
	ticker := time.NewTicker(constants.DefaultMinuteWatchedInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			streamers := m.getStreamers()
			toWatch := twitch.SelectStreamersToWatch(streamers, m.priorities, constants.MaxWatchStreams)

			m.logWatchingChanges(toWatch)

			if len(toWatch) > 0 {
				if err := m.twitch.SendMinuteWatchedEvents(ctx, toWatch); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					m.log.Debug("Minute watched error", "error", err)
				}
			}
		}
	}
}

// logWatchingChanges compares the current set of watched streamers with the
func (m *Miner) logWatchingChanges(toWatch []*model.Streamer) {
	currentSet := make(map[string]bool, len(toWatch))
	for _, s := range toWatch {
		s.Mu.RLock()
		currentSet[s.Username] = true
		s.Mu.RUnlock()
	}

	m.lastWatchingMu.Lock()
	defer m.lastWatchingMu.Unlock()

	for username := range currentSet {
		if !m.lastWatching[username] {
			m.log.Info("👀 Watching", "streamer", username)
		}
	}

	for username := range m.lastWatching {
		if !currentSet[username] {
			m.log.Info("💤 Stopped watching", "streamer", username)
		}
	}

	m.lastWatching = currentSet
}

func (m *Miner) runCampaignSync(ctx context.Context) error {
	hasDrops := false
	for _, s := range m.getStreamers() {
		s.Mu.RLock()
		if s.Settings != nil && s.Settings.ClaimDrops {
			hasDrops = true
		}
		s.Mu.RUnlock()
		if hasDrops {
			break
		}
	}
	if !hasDrops {
		<-ctx.Done()
		return ctx.Err()
	}

	streamers := m.getStreamers()
	if err := m.twitch.SyncCampaigns(ctx, streamers); err != nil {
		m.log.Warn("Initial campaign sync failed", "error", err)
	}
	// Hint GC to reclaim transient campaign sync allocations
	runtime.GC()

	ticker := time.NewTicker(constants.DefaultCampaignSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			streamers := m.getStreamers()
			if err := m.twitch.SyncCampaigns(ctx, streamers); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				m.log.Warn("Campaign sync failed", "error", err)
			}
			// Hint GC to reclaim transient campaign sync allocations
			runtime.GC()
		}
	}
}

func (m *Miner) runContextRefresh(ctx context.Context) error {
	ticker := time.NewTicker(constants.DefaultCampaignSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			streamers := m.getStreamers()
			for _, s := range streamers {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				s.Mu.RLock()
				isOnline := s.IsOnline
				s.Mu.RUnlock()

				if isOnline {
					if err := m.twitch.LoadChannelPointsContext(ctx, s); err != nil {
						m.log.Debug("Context refresh failed",
							"streamer", s.Username, "error", err)
					}
				}
			}
		}
	}
}

func (m *Miner) runMonitorLoop(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(20+rand.IntN(40)) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ticker.Reset(time.Duration(20+rand.IntN(40)) * time.Second)

			streamers := m.getStreamers()
			for _, s := range streamers {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err := m.twitch.CheckStreamerOnline(ctx, s); err != nil {
					m.log.Debug("Online check failed",
						"streamer", s.Username, "error", err)
				}
			}
		}
	}
}
