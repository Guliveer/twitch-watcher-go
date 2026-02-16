package model

import (
	"fmt"
	"time"
)

// Stream represents the current state of a live broadcast.
type Stream struct {
	BroadcastID string `json:"broadcast_id,omitempty"`

	Title string `json:"title,omitempty"`
	Game *GameInfo `json:"game,omitempty"`
	Tags []Tag `json:"tags,omitempty"`

	DropsTags bool `json:"drops_tags"`
	Campaigns []Campaign `json:"campaigns,omitempty"`
	CampaignIDs []string `json:"campaign_ids,omitempty"`

	ViewersCount int `json:"viewers_count"`

	SpadeURL string `json:"spade_url,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`

	WatchStreakMissing bool `json:"watch_streak_missing"`
	MinuteWatched float64 `json:"minute_watched"`

	lastUpdate time.Time
	minuteWatchedTimestamp time.Time
}

// GameInfo holds game/category metadata from the Twitch API.
type GameInfo struct {
	ID string `json:"id"`
	Name string `json:"name"`
	DisplayName string `json:"displayName"`
	Slug string `json:"slug,omitempty"`
}

// Tag represents a stream tag.
type Tag struct {
	ID string `json:"id"`
	LocalizedName string `json:"localizedName"`
}

// NewStream creates a new Stream with default values.
func NewStream() *Stream {
	s := &Stream{
		Tags:        make([]Tag, 0),
		Campaigns:   make([]Campaign, 0),
		CampaignIDs: make([]string, 0),
	}
	s.InitWatchStreak()
	return s
}

// Update refreshes the stream information with new data.
func (s *Stream) Update(broadcastID, title string, game *GameInfo, tags []Tag, viewersCount int, dropID string) {
	s.BroadcastID = broadcastID
	s.Title = title
	s.Game = game
	if tags != nil {
		s.Tags = tags
	} else {
		s.Tags = make([]Tag, 0)
	}
	s.ViewersCount = viewersCount

	s.DropsTags = false
	if s.Game != nil {
		for _, tag := range s.Tags {
			if tag.ID == dropID {
				s.DropsTags = true
				break
			}
		}
	}

	s.lastUpdate = time.Now()
}

// GameName returns the game's internal name, or empty string if no game is set.
func (s *Stream) GameName() string {
	if s.Game == nil {
		return ""
	}
	return s.Game.Name
}

// GameDisplayName returns the game's display name, or empty string if no game is set.
func (s *Stream) GameDisplayName() string {
	if s.Game == nil {
		return ""
	}
	return s.Game.DisplayName
}

// GameID returns the game's ID, or empty string if no game is set.
func (s *Stream) GameID() string {
	if s.Game == nil {
		return ""
	}
	return s.Game.ID
}

// GameSlug returns the game's URL-friendly slug from the Twitch API.
// It checks the direct API field first, then falls back to the global
// game slug registry (populated by the category watcher). No string
// normalization is performed — slugs must come from the Twitch API.
func (s *Stream) GameSlug() string {
	if s == nil || s.Game == nil {
		return ""
	}
	if s.Game.Slug != "" {
		return s.Game.Slug
	}
	if s.Game.ID != "" {
		if slug := LookupGameSlug(s.Game.ID); slug != "" {
			return slug
		}
	}
	return ""
}

// UpdateRequired returns true if the stream info needs refreshing (>= 120s since last update).
func (s *Stream) UpdateRequired() bool {
	return s.lastUpdate.IsZero() || time.Since(s.lastUpdate) >= 120*time.Second
}

// MarkUpdated sets lastUpdate to the current time without changing other fields.
// This is useful when a stream is discovered via an external source (e.g. category
// watcher) and we want to prevent UpdateRequired() from immediately returning true.
func (s *Stream) MarkUpdated() {
	s.lastUpdate = time.Now()
}

// UpdateElapsed returns the duration since the last stream info update.
func (s *Stream) UpdateElapsed() time.Duration {
	if s.lastUpdate.IsZero() {
		return 0
	}
	return time.Since(s.lastUpdate)
}

// InitWatchStreak resets the watch streak tracking state.
func (s *Stream) InitWatchStreak() {
	s.WatchStreakMissing = true
	s.MinuteWatched = 0
	s.minuteWatchedTimestamp = time.Time{}
}

// UpdateMinuteWatched increments the minute-watched counter based on elapsed time.
func (s *Stream) UpdateMinuteWatched() {
	now := time.Now()
	if !s.minuteWatchedTimestamp.IsZero() {
		elapsed := now.Sub(s.minuteWatchedTimestamp).Minutes()
		s.MinuteWatched += elapsed
	}
	s.minuteWatchedTimestamp = now
}

// String returns a human-readable representation of the stream.
func (s *Stream) String() string {
	gameName := ""
	if s.Game != nil {
		gameName = s.Game.DisplayName
	}
	return fmt.Sprintf("Stream(title=%s, game=%s, viewers=%d)", s.Title, gameName, s.ViewersCount)
}
