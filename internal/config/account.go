package config

import (
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// AccountConfig represents the full configuration for a single Twitch account.
// It is loaded from a YAML file and optionally overlaid with environment variables.
type AccountConfig struct {
	Username string `yaml:"-"`

	Enabled *bool `yaml:"enabled,omitempty"`

	Auth AuthConfig `yaml:"-"`

	Features FeaturesConfig `yaml:"features"`

	Priority []string `yaml:"priority"`

	CategoryWatcher CategoryWatcherConfig `yaml:"category_watcher"`

	StreamerDefaults StreamerSettingsConfig `yaml:"streamer_defaults"`

	Streamers []StreamerConfig `yaml:"streamers"`

	Blacklist []string `yaml:"blacklist"`

	Followers FollowersConfig `yaml:"followers"`

	Notifications NotificationsConfig `yaml:"notifications"`
}

// AuthConfig holds authentication-related settings.
type AuthConfig struct {
	AuthToken string `yaml:"auth_token"`
	Password string `yaml:"password"`
}

// FeaturesConfig holds global feature toggles for an account.
type FeaturesConfig struct {
	ClaimDropsStartup bool `yaml:"claim_drops_startup"`
	EnableAnalytics bool `yaml:"enable_analytics"`
}

// CategoryWatcherConfig holds settings for the category watcher.
type CategoryWatcherConfig struct {
	Enabled bool `yaml:"enabled"`
	PollInterval time.Duration `yaml:"poll_interval"`
	DropsOnly bool `yaml:"drops_only"`
	Categories []CategoryConfig `yaml:"categories"`
}

// CategoryConfig holds settings for a single game category.
type CategoryConfig struct {
	Slug string `yaml:"slug"`
	DropsOnly *bool `yaml:"drops_only,omitempty"`
}

// StreamerSettingsConfig is the YAML representation of per-streamer settings.
type StreamerSettingsConfig struct {
	MakePredictions *bool `yaml:"make_predictions,omitempty"`
	FollowRaid *bool `yaml:"follow_raid,omitempty"`
	ClaimDrops *bool `yaml:"claim_drops,omitempty"`
	ClaimMoments *bool `yaml:"claim_moments,omitempty"`
	WatchStreak *bool `yaml:"watch_streak,omitempty"`
	CommunityGoals *bool `yaml:"community_goals,omitempty"`
	Chat string `yaml:"chat,omitempty"`
	Bet *BetSettingsConfig `yaml:"bet,omitempty"`
}

// BetSettingsConfig is the YAML representation of bet settings.
type BetSettingsConfig struct {
	Strategy string `yaml:"strategy,omitempty"`
	Percentage *int `yaml:"percentage,omitempty"`
	PercentageGap *int `yaml:"percentage_gap,omitempty"`
	MaxPoints *int `yaml:"max_points,omitempty"`
	MinimumPoints *int `yaml:"minimum_points,omitempty"`
	StealthMode *bool `yaml:"stealth_mode,omitempty"`
	Delay *float64 `yaml:"delay,omitempty"`
	DelayMode string `yaml:"delay_mode,omitempty"`
	FilterCondition *FilterConditionConfig `yaml:"filter_condition,omitempty"`
}

// FilterConditionConfig is the YAML representation of a filter condition.
type FilterConditionConfig struct {
	By string `yaml:"by"`
	Where string `yaml:"where"`
	Value float64 `yaml:"value"`
}

// StreamerConfig holds per-streamer configuration from YAML.
type StreamerConfig struct {
	Username string `yaml:"username"`
	Settings *StreamerSettingsConfig `yaml:"settings,omitempty"`
}

// FollowersConfig holds settings for watching followed channels.
type FollowersConfig struct {
	Enabled bool `yaml:"enabled"`
	Order string `yaml:"order"`
}

// NotificationsConfig holds all notification provider configurations.
type NotificationsConfig struct {
	Telegram *TelegramConfig `yaml:"telegram,omitempty"`
	Discord *DiscordConfig `yaml:"discord,omitempty"`
	Webhook *WebhookConfig `yaml:"webhook,omitempty"`
	Matrix *MatrixConfig `yaml:"matrix,omitempty"`
	Pushover *PushoverConfig `yaml:"pushover,omitempty"`
	Gotify *GotifyConfig `yaml:"gotify,omitempty"`
}

// TelegramConfig holds Telegram notification settings.
type TelegramConfig struct {
	Enabled bool `yaml:"enabled"`
	Token string `yaml:"token,omitempty"`
	ChatID string `yaml:"chat_id,omitempty"`
	Events []string `yaml:"events"`
	DisableNotification bool `yaml:"disable_notification"`
}

// DiscordConfig holds Discord notification settings.
type DiscordConfig struct {
	Enabled bool `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url,omitempty"`
	Events []string `yaml:"events"`
}

// WebhookConfig holds generic webhook notification settings.
type WebhookConfig struct {
	Enabled bool `yaml:"enabled"`
	Endpoint string `yaml:"endpoint,omitempty"`
	Method string `yaml:"method"`
	Events []string `yaml:"events"`
}

// MatrixConfig holds Matrix notification settings.
type MatrixConfig struct {
	Enabled bool `yaml:"enabled"`
	Homeserver string `yaml:"homeserver,omitempty"`
	RoomID string `yaml:"room_id,omitempty"`
	AccessToken string `yaml:"access_token,omitempty"`
	Events []string `yaml:"events"`
}

// PushoverConfig holds Pushover notification settings.
type PushoverConfig struct {
	Enabled bool `yaml:"enabled"`
	UserKey string `yaml:"user_key,omitempty"`
	APIToken string `yaml:"api_token,omitempty"`
	Events []string `yaml:"events"`
}

// GotifyConfig holds Gotify notification settings.
type GotifyConfig struct {
	Enabled bool `yaml:"enabled"`
	URL string `yaml:"url,omitempty"`
	Token string `yaml:"token,omitempty"`
	Events []string `yaml:"events"`
}

// ToStreamerSettings converts a StreamerSettingsConfig to a model.StreamerSettings,
// using defaults for any unset fields.
func (ssc *StreamerSettingsConfig) ToStreamerSettings(defaults *model.StreamerSettings) *model.StreamerSettings {
	settings := *defaults // copy defaults

	if ssc == nil {
		return &settings
	}

	if ssc.MakePredictions != nil {
		settings.MakePredictions = *ssc.MakePredictions
	}
	if ssc.FollowRaid != nil {
		settings.FollowRaid = *ssc.FollowRaid
	}
	if ssc.ClaimDrops != nil {
		settings.ClaimDrops = *ssc.ClaimDrops
	}
	if ssc.ClaimMoments != nil {
		settings.ClaimMoments = *ssc.ClaimMoments
	}
	if ssc.WatchStreak != nil {
		settings.WatchStreak = *ssc.WatchStreak
	}
	if ssc.CommunityGoals != nil {
		settings.CommunityGoalsEnabled = *ssc.CommunityGoals
	}
	if ssc.Chat != "" {
		settings.Chat = model.ParseChatPresence(ssc.Chat)
	}
	if ssc.Bet != nil {
		settings.Bet = ssc.Bet.ToBetSettings(defaults.Bet)
	}

	return &settings
}

// ToBetSettings converts a BetSettingsConfig to a model.BetSettings,
// using defaults for any unset fields.
func (bsc *BetSettingsConfig) ToBetSettings(defaults *model.BetSettings) *model.BetSettings {
	betSettings := *defaults // copy defaults

	if bsc == nil {
		return &betSettings
	}

	if bsc.Strategy != "" {
		betSettings.Strategy = model.ParseStrategy(bsc.Strategy)
	}
	if bsc.Percentage != nil {
		betSettings.Percentage = *bsc.Percentage
	}
	if bsc.PercentageGap != nil {
		betSettings.PercentageGap = *bsc.PercentageGap
	}
	if bsc.MaxPoints != nil {
		betSettings.MaxPoints = *bsc.MaxPoints
	}
	if bsc.MinimumPoints != nil {
		betSettings.MinimumPoints = *bsc.MinimumPoints
	}
	if bsc.StealthMode != nil {
		betSettings.StealthMode = *bsc.StealthMode
	}
	if bsc.Delay != nil {
		betSettings.Delay = *bsc.Delay
	}
	if bsc.DelayMode != "" {
		betSettings.DelayMode = model.ParseDelayMode(bsc.DelayMode)
	}
	if bsc.FilterCondition != nil {
		betSettings.FilterCondition = &model.FilterCondition{
			By:    model.OutcomeKey(bsc.FilterCondition.By),
			Where: model.ParseCondition(bsc.FilterCondition.Where),
			Value: bsc.FilterCondition.Value,
		}
	}

	return &betSettings
}

// IsEnabled returns whether this account is enabled.
// If the Enabled field is not set (nil), it defaults to true.
func (ac *AccountConfig) IsEnabled() bool {
	if ac.Enabled == nil {
		return true // default to true when not specified
	}
	return *ac.Enabled
}

// ParsedPriorities converts the string priority list to model.Priority values.
func (ac *AccountConfig) ParsedPriorities() []model.Priority {
	priorities := make([]model.Priority, 0, len(ac.Priority))
	for _, priorityStr := range ac.Priority {
		priorities = append(priorities, model.ParsePriority(priorityStr))
	}
	return priorities
}
