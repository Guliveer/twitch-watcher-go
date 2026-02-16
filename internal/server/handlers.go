package server

import (
	"encoding/json"
	"net/http"
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

func (s *AnalyticsServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *AnalyticsServer) handleStreamers(w http.ResponseWriter, _ *http.Request) {
	streamers := s.getStreamers()
	result := make([]streamerSummary, 0, len(streamers))

	for _, streamer := range streamers {
		streamer.Mu.RLock()
		summary := streamerSummary{
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

func (s *AnalyticsServer) handleStats(w http.ResponseWriter, _ *http.Request) {
	streamers := s.getStreamers()

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
			agg := stats.History[reason]
			agg.Counter += entry.Counter
			agg.Amount += entry.Amount
			stats.History[reason] = agg
		}
		streamer.Mu.RUnlock()
	}

	writeJSON(w, http.StatusOK, stats)
}


type streamerSummary struct {
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
	TotalStreamers  int                          `json:"total_streamers"`
	OnlineStreamers int                          `json:"online_streamers"`
	TotalPoints     int                          `json:"total_points"`
	History         map[string]historyAggregate  `json:"history"`
}

type historyAggregate struct {
	Counter int `json:"counter"`
	Amount  int `json:"amount"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data) //nolint:errcheck
}
