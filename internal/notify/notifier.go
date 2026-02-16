// Package notify provides notification dispatching to multiple providers
// (Telegram, Discord, Webhook, Matrix, Pushover, Gotify) based on event filtering.
package notify

import (
	"context"
	"net/http"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// defaultHTTPTimeout is the timeout for notification HTTP requests.
const defaultHTTPTimeout = 5 * time.Second

// Notifier is the interface that all notification providers must implement.
type Notifier interface {
	Send(ctx context.Context, event model.Event, title, message string) error
	Name() string
	IsEnabled() bool
	ShouldNotify(event model.Event) bool
}

// Dispatcher manages multiple notifiers and dispatches notifications to all
// enabled notifiers that match the event.
type Dispatcher struct {
	notifiers []Notifier
	log       *logger.Logger
}

// NewDispatcher creates a Dispatcher from the notification configuration.
// It initialises all configured and enabled notification providers.
func NewDispatcher(cfg config.NotificationsConfig, log *logger.Logger) *Dispatcher {
	d := &Dispatcher{log: log}

	httpClient := &http.Client{
		Timeout: defaultHTTPTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	if cfg.Telegram != nil && cfg.Telegram.Enabled {
		d.notifiers = append(d.notifiers, &Telegram{
			baseNotifier:        baseNotifier{name: "Telegram", enabled: true, events: parseEvents(cfg.Telegram.Events)},
			token:               cfg.Telegram.Token,
			chatID:              cfg.Telegram.ChatID,
			disableNotification: cfg.Telegram.DisableNotification,
			httpClient:          httpClient,
		})
	}

	if cfg.Discord != nil && cfg.Discord.Enabled {
		d.notifiers = append(d.notifiers, &Discord{
			baseNotifier: baseNotifier{name: "Discord", enabled: true, events: parseEvents(cfg.Discord.Events)},
			webhookURL:   cfg.Discord.WebhookURL,
			httpClient:   httpClient,
		})
	}

	if cfg.Webhook != nil && cfg.Webhook.Enabled {
		method := cfg.Webhook.Method
		if method == "" {
			method = http.MethodPost
		}
		d.notifiers = append(d.notifiers, &Webhook{
			baseNotifier: baseNotifier{name: "Webhook", enabled: true, events: parseEvents(cfg.Webhook.Events)},
			url:          cfg.Webhook.Endpoint,
			method:       method,
			httpClient:   httpClient,
		})
	}

	if cfg.Matrix != nil && cfg.Matrix.Enabled {
		d.notifiers = append(d.notifiers, &Matrix{
			baseNotifier: baseNotifier{name: "Matrix", enabled: true, events: parseEvents(cfg.Matrix.Events)},
			homeserver:   cfg.Matrix.Homeserver,
			accessToken:  cfg.Matrix.AccessToken,
			roomID:       cfg.Matrix.RoomID,
			httpClient:   httpClient,
		})
	}

	if cfg.Pushover != nil && cfg.Pushover.Enabled {
		d.notifiers = append(d.notifiers, &Pushover{
			baseNotifier: baseNotifier{name: "Pushover", enabled: true, events: parseEvents(cfg.Pushover.Events)},
			token:        cfg.Pushover.APIToken,
			userKey:      cfg.Pushover.UserKey,
			httpClient:   httpClient,
		})
	}

	if cfg.Gotify != nil && cfg.Gotify.Enabled {
		d.notifiers = append(d.notifiers, &Gotify{
			baseNotifier: baseNotifier{name: "Gotify", enabled: true, events: parseEvents(cfg.Gotify.Events)},
			url:          cfg.Gotify.URL,
			token:        cfg.Gotify.Token,
			httpClient:   httpClient,
		})
	}

	return d
}

// Dispatch sends a notification to all enabled notifiers that match the event.
// Sends are non-blocking — each notifier runs in its own goroutine.
func (d *Dispatcher) Dispatch(ctx context.Context, event model.Event, title, message string) {
	for _, n := range d.notifiers {
		if !n.IsEnabled() || !n.ShouldNotify(event) {
			continue
		}
		go func(notifier Notifier) {
			sendCtx, cancel := context.WithTimeout(ctx, defaultHTTPTimeout)
			defer cancel()
			if err := notifier.Send(sendCtx, event, title, message); err != nil {
				d.log.Warn("notification send failed",
					"provider", notifier.Name(),
					"event", string(event),
					"error", err,
				)
			}
		}(n)
	}
}

// NotifyFunc returns a logger.NotifyFunc that dispatches notifications via this Dispatcher.
func (d *Dispatcher) NotifyFunc() logger.NotifyFunc {
	return func(ctx context.Context, message string, event model.Event) {
		d.Dispatch(ctx, event, "Twitch Miner", message)
	}
}

// HasNotifiers reports whether any notifiers are configured.
func (d *Dispatcher) HasNotifiers() bool {
	return len(d.notifiers) > 0
}

// parseEvents converts a slice of event name strings to model.Event values,
func parseEvents(names []string) []model.Event {
	events := make([]model.Event, 0, len(names))
	for _, name := range names {
		e := model.ParseEvent(name)
		if e != "" {
			events = append(events, e)
		}
	}
	return events
}

func containsEvent(events []model.Event, event model.Event) bool {
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}
