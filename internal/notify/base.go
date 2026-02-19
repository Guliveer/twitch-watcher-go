package notify

import "github.com/Guliveer/twitch-miner-go/internal/model"

// baseNotifier provides shared boilerplate for all notification providers.
// Embed it in concrete notifier structs to eliminate duplicated Name(),
type baseNotifier struct {
	name    string
	enabled bool
	events  []model.Event
}

// Name returns the human-readable name of the notifier.
func (b *baseNotifier) Name() string { return b.name }

// IsEnabled reports whether this notifier is active.
func (b *baseNotifier) IsEnabled() bool { return b.enabled }

// ShouldNotify reports whether this notifier should fire for the given event.
// If the events list is empty, all events are allowed (treat as "subscribe to all").
func (b *baseNotifier) ShouldNotify(event model.Event) bool {
	if len(b.events) == 0 {
		return true
	}
	return containsEvent(b.events, event)
}
