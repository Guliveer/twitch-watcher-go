package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

func (s *AnalyticsServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML) //nolint:errcheck
}

func (s *AnalyticsServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(logsHTML) //nolint:errcheck
}

func (s *AnalyticsServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// filterStreamers applies query-parameter filters to a streamer list.
// When a filter parameter is empty the corresponding check is skipped,
// so callers with no filters get the full list back unchanged.
func filterStreamers(streamers []*model.Streamer, r *http.Request) []*model.Streamer {
	accountFilter := strings.ToLower(r.URL.Query().Get("account"))
	channelFilter := strings.ToLower(r.URL.Query().Get("channel"))
	categoryFilter := strings.ToLower(r.URL.Query().Get("category"))
	onlineFilter := r.URL.Query().Get("online")

	// Fast path: no filters provided — return original slice.
	if accountFilter == "" && channelFilter == "" && categoryFilter == "" && onlineFilter == "" {
		return streamers
	}

	filtered := make([]*model.Streamer, 0, len(streamers))
	for _, st := range streamers {
		st.Mu.RLock()

		// Account filter (exact, case-insensitive)
		if accountFilter != "" && !strings.EqualFold(st.AccountUsername, accountFilter) {
			st.Mu.RUnlock()
			continue
		}
		// Channel filter (substring, case-insensitive)
		if channelFilter != "" && !strings.Contains(strings.ToLower(st.Username), channelFilter) {
			st.Mu.RUnlock()
			continue
		}
		// Category filter (substring match on game name)
		if categoryFilter != "" {
			gameName := ""
			if st.Stream != nil && st.Stream.Game != nil {
				gameName = strings.ToLower(st.Stream.Game.DisplayName)
			}
			if !strings.Contains(gameName, categoryFilter) {
				st.Mu.RUnlock()
				continue
			}
		}
		// Online filter
		if onlineFilter != "" {
			wantOnline := onlineFilter == "true"
			if st.IsOnline != wantOnline {
				st.Mu.RUnlock()
				continue
			}
		}

		st.Mu.RUnlock()
		filtered = append(filtered, st)
	}
	return filtered
}

func (s *AnalyticsServer) handleStreamers(w http.ResponseWriter, r *http.Request) {
	streamers := filterStreamers(s.getStreamers(), r)
	result := make([]streamerSummary, 0, len(streamers))

	for _, streamer := range streamers {
		streamer.Mu.RLock()
		summary := streamerSummary{
			Account:           streamer.AccountUsername,
			Username:          streamer.Username,
			DisplayName:       streamer.DisplayName,
			ChannelID:         streamer.ChannelID,
			IsOnline:          streamer.IsOnline,
			IsCategoryWatched: streamer.IsCategoryWatched,
			ChannelPoints:     streamer.ChannelPoints,
			StreamerURL:       streamer.StreamerURL,
		}
		if streamer.Stream != nil && streamer.Stream.Game != nil {
			summary.Game = streamer.Stream.Game.DisplayName
		}
		if streamer.Stream != nil {
			summary.ViewersCount = streamer.Stream.ViewersCount
			summary.Title = streamer.Stream.Title
		}
		streamer.Mu.RUnlock()
		result = append(result, summary)
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *AnalyticsServer) handleStreamer(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(r.PathValue("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing streamer name"})
		return
	}

	streamers := s.getStreamers()
	for _, streamer := range streamers {
		streamer.Mu.RLock()
		if strings.ToLower(streamer.Username) == name {
			detail := streamerDetail{
				Account:           streamer.AccountUsername,
				Username:          streamer.Username,
				DisplayName:       streamer.DisplayName,
				ChannelID:         streamer.ChannelID,
				IsOnline:          streamer.IsOnline,
				IsCategoryWatched: streamer.IsCategoryWatched,
				CategorySlug:      streamer.CategorySlug,
				ChannelPoints:     streamer.ChannelPoints,
				StreamerURL:       streamer.StreamerURL,
				ViewerIsMod:       streamer.ViewerIsMod,
				History:           streamer.History,
			}
			if streamer.Stream != nil {
				detail.Stream = &streamInfo{
					BroadcastID:  streamer.Stream.BroadcastID,
					Title:        streamer.Stream.Title,
					ViewersCount: streamer.Stream.ViewersCount,
					DropsTags:    streamer.Stream.DropsTags,
				}
				if streamer.Stream.Game != nil {
					detail.Stream.Game = streamer.Stream.Game.DisplayName
				}
			}
			if len(streamer.ActiveMultipliers) > 0 {
				detail.Multipliers = make([]float64, 0, len(streamer.ActiveMultipliers))
				for _, m := range streamer.ActiveMultipliers {
					detail.Multipliers = append(detail.Multipliers, m.Factor)
				}
			}
			streamer.Mu.RUnlock()
			writeJSON(w, http.StatusOK, detail)
			return
		}
		streamer.Mu.RUnlock()
	}

	writeJSON(w, http.StatusNotFound, errorResponse{Error: "streamer not found"})
}

func (s *AnalyticsServer) handleStats(w http.ResponseWriter, r *http.Request) {
	streamers := filterStreamers(s.getStreamers(), r)

	// Parse optional event filter (comma-separated reason codes).
	eventFilter := r.URL.Query().Get("event")
	var allowedEvents map[string]bool
	if eventFilter != "" {
		allowedEvents = make(map[string]bool)
		for _, e := range strings.Split(eventFilter, ",") {
			allowedEvents[strings.TrimSpace(strings.ToUpper(e))] = true
		}
	}

	stats := overallStats{
		TotalStreamers: len(streamers),
		History:        make(map[string]historyAggregate),
	}

	for _, streamer := range streamers {
		streamer.Mu.RLock()
		stats.TotalPoints += streamer.ChannelPoints
		if streamer.IsOnline {
			stats.OnlineStreamers++
		}
		for reason, entry := range streamer.History {
			if allowedEvents != nil && !allowedEvents[reason] {
				continue
			}
			agg := stats.History[reason]
			agg.Counter += entry.Counter
			agg.Amount += entry.Amount
			stats.History[reason] = agg
		}
		streamer.Mu.RUnlock()
	}

	writeJSON(w, http.StatusOK, stats)
}

func (s *AnalyticsServer) handleFilters(w http.ResponseWriter, _ *http.Request) {
	streamers := s.getStreamers()

	accountSet := make(map[string]bool)
	channelSet := make(map[string]bool)
	categorySet := make(map[string]bool)
	eventSet := make(map[string]bool)

	for _, st := range streamers {
		st.Mu.RLock()
		if st.AccountUsername != "" {
			accountSet[st.AccountUsername] = true
		}
		channelSet[st.Username] = true
		if st.Stream != nil && st.Stream.Game != nil && st.Stream.Game.DisplayName != "" {
			categorySet[st.Stream.Game.DisplayName] = true
		}
		for reason := range st.History {
			eventSet[reason] = true
		}
		st.Mu.RUnlock()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":   sortedKeys(accountSet),
		"channels":   sortedKeys(channelSet),
		"categories": sortedKeys(categorySet),
		"events":     sortedKeys(eventSet),
	})
}

// Event category groups for filtering on the logs page.
var eventCategories = map[string][]string{
	"drops":   {"DROP_CLAIM", "DROP_STATUS"},
	"points":  {"GAIN_FOR_WATCH", "GAIN_FOR_WATCH_STREAK", "GAIN_FOR_CLAIM", "GAIN_FOR_RAID", "BONUS_CLAIM"},
	"bets":    {"BET_START", "BET_WIN", "BET_LOSE", "BET_REFUND", "BET_FILTERS", "BET_GENERAL", "BET_FAILED"},
	"raids":   {"JOIN_RAID"},
	"streams": {"STREAMER_ONLINE", "STREAMER_OFFLINE"},
	"other":   {"MOMENT_CLAIM", "CHAT_MENTION"},
}

type eventLogEntry struct {
	Account  string `json:"account"`
	Streamer string `json:"streamer"`
	Event    string `json:"event"`
	Count    int    `json:"count"`
	Amount   int    `json:"amount"`
}

func (s *AnalyticsServer) handleEventLogs(w http.ResponseWriter, r *http.Request) {
	accountFilter := strings.ToLower(r.URL.Query().Get("account"))
	channelFilter := strings.ToLower(r.URL.Query().Get("channel"))
	categoryFilter := strings.ToLower(r.URL.Query().Get("category"))
	eventFilter := strings.ToUpper(r.URL.Query().Get("event"))

	// Build allowed events set from category filter.
	var allowedEvents map[string]bool
	if categoryFilter != "" {
		if events, ok := eventCategories[categoryFilter]; ok {
			allowedEvents = make(map[string]bool, len(events))
			for _, e := range events {
				allowedEvents[e] = true
			}
		}
	}

	var entries []eventLogEntry
	for _, st := range s.getStreamers() {
		if accountFilter != "" && strings.ToLower(st.AccountUsername) != accountFilter {
			continue
		}
		if channelFilter != "" && !strings.Contains(strings.ToLower(st.Username), channelFilter) {
			continue
		}

		st.Mu.RLock()
		for event, hist := range st.History {
			// Apply event filter.
			if eventFilter != "" && event != eventFilter {
				continue
			}
			// Apply category filter.
			if allowedEvents != nil && !allowedEvents[event] {
				continue
			}
			entries = append(entries, eventLogEntry{
				Account:  st.AccountUsername,
				Streamer: st.DisplayName,
				Event:    event,
				Count:    hist.Counter,
				Amount:   hist.Amount,
			})
		}
		st.Mu.RUnlock()
	}

	if entries == nil {
		entries = []eventLogEntry{}
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *AnalyticsServer) handleEventFilters(w http.ResponseWriter, _ *http.Request) {
	accountSet := make(map[string]bool)
	channelSet := make(map[string]bool)
	eventSet := make(map[string]bool)

	for _, st := range s.getStreamers() {
		if st.AccountUsername != "" {
			accountSet[st.AccountUsername] = true
		}
		channelSet[st.DisplayName] = true
		st.Mu.RLock()
		for event := range st.History {
			eventSet[event] = true
		}
		st.Mu.RUnlock()
	}

	categories := make([]string, 0, len(eventCategories))
	for cat := range eventCategories {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":   sortedKeys(accountSet),
		"channels":   sortedKeys(channelSet),
		"events":     sortedKeys(eventSet),
		"categories": categories,
	})
}

// sortedKeys returns the keys of a map[string]bool as a sorted slice.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type streamerSummary struct {
	Account           string `json:"account"`
	Username          string `json:"username"`
	DisplayName       string `json:"display_name,omitempty"`
	ChannelID         string `json:"channel_id"`
	IsOnline          bool   `json:"is_online"`
	IsCategoryWatched bool   `json:"is_category_watched"`
	ChannelPoints     int    `json:"channel_points"`
	StreamerURL       string `json:"streamer_url"`
	Game              string `json:"game,omitempty"`
	ViewersCount      int    `json:"viewers_count"`
	Title             string `json:"title,omitempty"`
}

type streamerDetail struct {
	Account           string                         `json:"account"`
	Username          string                         `json:"username"`
	DisplayName       string                         `json:"display_name,omitempty"`
	ChannelID         string                         `json:"channel_id"`
	IsOnline          bool                           `json:"is_online"`
	IsCategoryWatched bool                           `json:"is_category_watched"`
	CategorySlug      string                         `json:"category_slug,omitempty"`
	ChannelPoints     int                            `json:"channel_points"`
	StreamerURL       string                         `json:"streamer_url"`
	ViewerIsMod       bool                           `json:"viewer_is_mod"`
	Stream            *streamInfo                    `json:"stream,omitempty"`
	Multipliers       []float64                      `json:"multipliers,omitempty"`
	History           map[string]*model.HistoryEntry `json:"history,omitempty"`
}

type streamInfo struct {
	BroadcastID  string `json:"broadcast_id,omitempty"`
	Title        string `json:"title,omitempty"`
	Game         string `json:"game,omitempty"`
	ViewersCount int    `json:"viewers_count"`
	DropsTags    bool   `json:"drops_tags"`
}

type overallStats struct {
	TotalStreamers  int                         `json:"total_streamers"`
	OnlineStreamers int                         `json:"online_streamers"`
	TotalPoints     int                         `json:"total_points"`
	History         map[string]historyAggregate `json:"history"`
}

type historyAggregate struct {
	Counter int `json:"counter"`
	Amount  int `json:"amount"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *AnalyticsServer) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	fn := s.notifyTestFunc
	s.mu.RUnlock()

	if fn == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "no notification dispatchers configured"})
		return
	}

	errs := fn(r.Context())
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, err := range errs {
			errMsgs[i] = err.Error()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "partial",
			"errors": errMsgs,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Test notification sent to all enabled notifiers",
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data) //nolint:errcheck
}
