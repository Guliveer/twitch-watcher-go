package model

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Streamer represents a Twitch channel being watched by the miner.
// Fields that may be accessed concurrently are protected by Mu.
type Streamer struct {
	Mu sync.RWMutex `json:"-"`

	Username        string `json:"username"`
	ChannelID       string `json:"channel_id"`
	DisplayName     string `json:"display_name,omitempty"`
	AccountUsername string `json:"-"` // The miner account that owns this streamer

	Settings *StreamerSettings `json:"settings,omitempty"`

	IsOnline bool `json:"is_online"`
	IsCategoryWatched bool `json:"is_category_watched"`
	CategorySlug string `json:"category_slug,omitempty"`

	StreamUpAt time.Time `json:"stream_up_at"`
	OnlineAt time.Time `json:"online_at"`
	OfflineAt time.Time `json:"offline_at"`

	ChannelPoints int `json:"channel_points"`

	CommunityGoals map[string]*CommunityGoal `json:"community_goals,omitempty"`

	ViewerIsMod bool `json:"viewer_is_mod"`
	ActiveMultipliers []PointsMultiplier `json:"active_multipliers,omitempty"`

	Stream *Stream `json:"stream"`

	Raid *Raid `json:"raid,omitempty"`

	History map[string]*HistoryEntry `json:"history,omitempty"`

	StreamerURL string `json:"streamer_url"`
}

// PointsMultiplier represents an active channel points multiplier.
type PointsMultiplier struct {
	Factor float64 `json:"factor"`
}

// HistoryEntry tracks cumulative points earned for a specific reason code.
type HistoryEntry struct {
	Counter int `json:"counter"`
	Amount int `json:"amount"`
}

// NewStreamer creates a new Streamer with sensible defaults.
func NewStreamer(username string) *Streamer {
	return &Streamer{
		Username:       username,
		Stream:         NewStream(),
		CommunityGoals: make(map[string]*CommunityGoal),
		History:        make(map[string]*HistoryEntry),
		StreamerURL:    fmt.Sprintf("https://www.twitch.tv/%s", username),
	}
}

// SetOffline marks the streamer as offline. Must be called with Mu held.
func (s *Streamer) SetOffline() {
	if s.IsOnline {
		s.OfflineAt = time.Now()
		s.IsOnline = false
	}
}

// SetOnline marks the streamer as online. Must be called with Mu held.
func (s *Streamer) SetOnline() {
	if !s.IsOnline {
		s.OnlineAt = time.Now()
		s.IsOnline = true
		s.Stream.InitWatchStreak()
	}
}

// UpdateHistory adds earned points for a given reason code.
func (s *Streamer) UpdateHistory(reasonCode string, earned int, counter int) {
	if _, ok := s.History[reasonCode]; !ok {
		s.History[reasonCode] = &HistoryEntry{}
	}
	s.History[reasonCode].Counter += counter
	s.History[reasonCode].Amount += earned

	if reasonCode == "WATCH_STREAK" {
		s.Stream.WatchStreakMissing = false
	}
}

// ResolveCategory returns the best available category identifier for this streamer.
// It checks CategorySlug first, then the API-provided game slug (including the
// global registry lookup), then falls back to the game's display name. Returns
// "unknown" only if no category information is available at all.
// Must be called with Mu held (at least RLock).
func (s *Streamer) ResolveCategory() string {
	if s.CategorySlug != "" {
		return s.CategorySlug
	}
	if s.Stream != nil {
		if slug := s.Stream.GameSlug(); slug != "" {
			return slug
		}
		// Slug unavailable — fall back to display name for logging purposes.
		if dn := s.Stream.GameDisplayName(); dn != "" {
			return dn
		}
	}
	return "unknown"
}

// StreamUpElapsed returns true if enough time has passed since the last stream-up event.
func (s *Streamer) StreamUpElapsed() bool {
	return s.StreamUpAt.IsZero() || time.Since(s.StreamUpAt) > 120*time.Second
}

// DropsCondition returns true if the streamer qualifies for drops collection.
func (s *Streamer) DropsCondition() bool {
	return s.Settings != nil &&
		s.Settings.ClaimDrops &&
		s.IsOnline &&
		len(s.Stream.CampaignIDs) > 0
}

// HasPointsMultiplier returns true if the viewer has active points multipliers.
func (s *Streamer) HasPointsMultiplier() bool {
	return len(s.ActiveMultipliers) > 0
}

// TotalPointsMultiplier returns the sum of all active multiplier factors.
func (s *Streamer) TotalPointsMultiplier() float64 {
	var total float64
	for _, multiplier := range s.ActiveMultipliers {
		total += multiplier.Factor
	}
	return total
}

// UpdateCommunityGoal adds or updates a community goal for this streamer.
func (s *Streamer) UpdateCommunityGoal(goal *CommunityGoal) {
	s.CommunityGoals[goal.GoalID] = goal
}

// DeleteCommunityGoal removes a community goal by ID.
func (s *Streamer) DeleteCommunityGoal(goalID string) {
	delete(s.CommunityGoals, goalID)
}

// String returns a human-readable representation of the streamer.
func (s *Streamer) String() string {
	return fmt.Sprintf("Streamer(username=%s, channel_id=%s, channel_points=%d)",
		s.Username, s.ChannelID, s.ChannelPoints)
}

// MarshalJSON implements custom JSON marshaling to handle the mutex.
func (s *Streamer) MarshalJSON() ([]byte, error) {
	type Alias Streamer
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return json.Marshal((*Alias)(s))
}

// ChatPresence controls when the miner joins a streamer's IRC chat.
type ChatPresence int

const (
	// ChatAlways means always stay in chat.
	ChatAlways ChatPresence = iota
	// ChatNever means never join chat.
	ChatNever
	// ChatOnline means join chat only when the streamer is online.
	ChatOnline
	// ChatOffline means join chat only when the streamer is offline.
	ChatOffline
)

// String returns the string representation of a ChatPresence value.
func (c ChatPresence) String() string {
	switch c {
	case ChatAlways:
		return "ALWAYS"
	case ChatNever:
		return "NEVER"
	case ChatOnline:
		return "ONLINE"
	case ChatOffline:
		return "OFFLINE"
	default:
		return "ONLINE"
	}
}

// ShouldJoinChat returns whether the miner should join chat for the given
// presence setting and online state.
func ShouldJoinChat(presence ChatPresence, isOnline bool) bool {
	switch presence {
	case ChatAlways:
		return true
	case ChatOnline:
		return isOnline
	case ChatOffline:
		return !isOnline
	default:
		return false
	}
}

// ParseChatPresence converts a string to a ChatPresence value.
func ParseChatPresence(s string) ChatPresence {
	switch s {
	case "ALWAYS":
		return ChatAlways
	case "NEVER":
		return ChatNever
	case "ONLINE":
		return ChatOnline
	case "OFFLINE":
		return ChatOffline
	default:
		return ChatOnline
	}
}

// StreamerSettings holds per-streamer feature toggles and bet configuration.
type StreamerSettings struct {
	MakePredictions bool `json:"make_predictions" yaml:"make_predictions"`
	FollowRaid bool `json:"follow_raid" yaml:"follow_raid"`
	ClaimDrops bool `json:"claim_drops" yaml:"claim_drops"`
	ClaimMoments bool `json:"claim_moments" yaml:"claim_moments"`
	WatchStreak bool `json:"watch_streak" yaml:"watch_streak"`
	CommunityGoalsEnabled bool `json:"community_goals" yaml:"community_goals"`
	Bet *BetSettings `json:"bet,omitempty" yaml:"bet"`
	Chat ChatPresence `json:"chat" yaml:"chat"`
}

// DefaultStreamerSettings returns StreamerSettings with default values.
func DefaultStreamerSettings() *StreamerSettings {
	return &StreamerSettings{
		MakePredictions:       true,
		FollowRaid:            true,
		ClaimDrops:            true,
		ClaimMoments:          true,
		WatchStreak:           true,
		CommunityGoalsEnabled: false,
		Bet:                   DefaultBetSettings(),
		Chat:                  ChatOnline,
	}
}
