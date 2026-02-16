// Package config handles loading, parsing, and validating YAML configuration
// files for the Twitch miner. It supports per-account configuration with
// environment variable overrides for secrets.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfigDir is the default directory for account configuration files.
const DefaultConfigDir = "configs"

// LoadAccountConfig loads a single account configuration from a YAML file,
// then overlays environment variables for secrets.
func LoadAccountConfig(path string) (*AccountConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg AccountConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	filename := filepath.Base(path)
	ext := filepath.Ext(filename)
	cfg.Username = strings.TrimSuffix(filename, ext)

	applyDefaults(&cfg)
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// LoadAllAccountConfigs loads all .yaml/.yml files from the given directory.
// Each file is expected to contain a single AccountConfig.
// Only files ending in .yaml or .yml are loaded; everything else (including
// .yaml.example) is ignored by the extension check.
// The username for each account is derived from the config filename.
func LoadAllAccountConfigs(dir string) ([]*AccountConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config directory %s: %w", dir, err)
	}

	var configs []*AccountConfig
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		cfg, err := LoadAccountConfig(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", name, err)
		}

		configs = append(configs, cfg)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no account config files found in %s", dir)
	}

	return configs, nil
}

func applyDefaults(cfg *AccountConfig) {
	if len(cfg.Priority) == 0 {
		cfg.Priority = []string{"STREAK", "DROPS", "ORDER"}
	}

	if cfg.CategoryWatcher.PollInterval == 0 {
		cfg.CategoryWatcher.PollInterval = 120 * time.Second
	}

	if cfg.Followers.Order == "" {
		cfg.Followers.Order = "ASC"
	}
}

// getEnv looks up an environment variable with a per-account suffix.
func getEnv(key, username string) string {
	return os.Getenv(key + "_" + strings.ToUpper(username))
}

// applyEnvOverrides overlays environment variables for secrets.
// Every variable requires the username suffix: KEY_<UPPERCASE_USERNAME>
func applyEnvOverrides(cfg *AccountConfig) {
	u := cfg.Username

	if cfg.Notifications.Telegram != nil {
		if v := getEnv("TELEGRAM_TOKEN", u); v != "" {
			cfg.Notifications.Telegram.Token = v
		}
		if v := getEnv("TELEGRAM_CHAT_ID", u); v != "" {
			cfg.Notifications.Telegram.ChatID = v
		}
	}

	if cfg.Notifications.Discord != nil {
		if v := getEnv("DISCORD_WEBHOOK", u); v != "" {
			cfg.Notifications.Discord.WebhookURL = v
		}
	}

	if cfg.Notifications.Webhook != nil {
		if v := getEnv("WEBHOOK_URL", u); v != "" {
			cfg.Notifications.Webhook.Endpoint = v
		}
	}

	if cfg.Notifications.Matrix != nil {
		if v := getEnv("MATRIX_HOMESERVER", u); v != "" {
			cfg.Notifications.Matrix.Homeserver = v
		}
		if v := getEnv("MATRIX_ROOM_ID", u); v != "" {
			cfg.Notifications.Matrix.RoomID = v
		}
		if v := getEnv("MATRIX_ACCESS_TOKEN", u); v != "" {
			cfg.Notifications.Matrix.AccessToken = v
		}
	}

	if cfg.Notifications.Pushover != nil {
		if v := getEnv("PUSHOVER_TOKEN", u); v != "" {
			cfg.Notifications.Pushover.APIToken = v
		}
		if v := getEnv("PUSHOVER_USER_KEY", u); v != "" {
			cfg.Notifications.Pushover.UserKey = v
		}
	}

	if cfg.Notifications.Gotify != nil {
		if v := getEnv("GOTIFY_URL", u); v != "" {
			cfg.Notifications.Gotify.URL = v
		}
		if v := getEnv("GOTIFY_TOKEN", u); v != "" {
			cfg.Notifications.Gotify.Token = v
		}
	}
}

// Validate checks the configuration for common errors.
func Validate(cfg *AccountConfig) error {
	if cfg.Username == "" {
		return fmt.Errorf("username is required")
	}

	if len(cfg.Streamers) == 0 && !cfg.Followers.Enabled && !cfg.CategoryWatcher.Enabled {
		return fmt.Errorf("account %s: at least one of streamers, followers, or category_watcher must be configured", cfg.Username)
	}

	for i, s := range cfg.Streamers {
		if s.Username == "" {
			return fmt.Errorf("account %s: streamer at index %d has empty username", cfg.Username, i)
		}
	}

	if cfg.Notifications.Telegram != nil && cfg.Notifications.Telegram.Enabled {
		if cfg.Notifications.Telegram.Token == "" || cfg.Notifications.Telegram.ChatID == "" {
			u := strings.ToUpper(cfg.Username)
			return fmt.Errorf("account %s: telegram enabled but token or chat_id not set (use env vars TELEGRAM_TOKEN_%s and TELEGRAM_CHAT_ID_%s)", cfg.Username, u, u)
		}
	}

	if cfg.Notifications.Discord != nil && cfg.Notifications.Discord.Enabled {
		if cfg.Notifications.Discord.WebhookURL == "" {
			return fmt.Errorf("account %s: discord enabled but webhook_url not set (use env var DISCORD_WEBHOOK_%s)", cfg.Username, strings.ToUpper(cfg.Username))
		}
	}

	return nil
}
