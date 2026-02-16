package model

import "sync"

// gameSlugRegistry is a thread-safe mapping of Twitch game IDs to their
// API-provided slugs. It is populated by the category watcher (which resolves
// slugs via GetTopStreamsByCategory) and consulted by GameSlug() when the
// stream's Game.Slug field is empty (e.g. because the VideoPlayerStreamInfo
// persisted query doesn't return it).
var gameSlugRegistry = struct {
	sync.RWMutex
	slugsByGameID map[string]string // gameID → slug
}{slugsByGameID: make(map[string]string)}

// RegisterGameSlug records a game ID → slug mapping in the global registry.
// Both gameID and slug must be non-empty; empty values are silently ignored.
func RegisterGameSlug(gameID, slug string) {
	if gameID == "" || slug == "" {
		return
	}
	gameSlugRegistry.Lock()
	gameSlugRegistry.slugsByGameID[gameID] = slug
	gameSlugRegistry.Unlock()
}

// LookupGameSlug returns the slug for a game ID, or "" if not found.
func LookupGameSlug(gameID string) string {
	gameSlugRegistry.RLock()
	defer gameSlugRegistry.RUnlock()
	return gameSlugRegistry.slugsByGameID[gameID]
}
