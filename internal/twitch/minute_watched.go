package twitch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// SendMinuteWatchedEvents sends minute-watched events for the given streamers.
// This is the core "watching" simulation that:
// 1. Gets PlaybackAccessToken for the streamer
// 2. Fetches HLS manifest URL
// 3. Parses the manifest to get the lowest quality stream URL
// 4. Makes a HEAD request to the stream URL
// 5. Sends the spade_url tracking event
func (c *Client) SendMinuteWatchedEvents(ctx context.Context, streamers []*model.Streamer) error {
	httpClient := c.GQL.HTTPClient()

	for _, streamer := range streamers {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := c.sendMinuteWatchedForStreamer(ctx, httpClient, streamer); err != nil {
			c.Log.Debug("Failed to send minute watched",
				"streamer", streamer.Username,
				"error", err)
			continue
		}
	}

	return nil
}

func (c *Client) sendMinuteWatchedForStreamer(ctx context.Context, httpClient *http.Client, streamer *model.Streamer) error {
	streamer.Mu.RLock()
	username := streamer.Username
	spadeURL := streamer.Stream.SpadeURL
	payload := streamer.Stream.Payload
	streamer.Mu.RUnlock()

	if spadeURL == "" {
		return fmt.Errorf("no spade URL for %s", username)
	}
	if payload == nil {
		return fmt.Errorf("no payload for %s", username)
	}

	token, err := c.GQL.GetPlaybackAccessToken(ctx, username)
	if err != nil {
		return fmt.Errorf("getting playback access token for %s: %w", username, err)
	}

	c.Log.Debug("Got playback access token", "streamer", username)

	manifestURL := fmt.Sprintf(
		"https://usher.ttvnw.net/api/channel/hls/%s.m3u8?sig=%s&token=%s",
		username, token.Signature, token.Value,
	)

	manifestReq, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return fmt.Errorf("creating manifest request: %w", err)
	}
	manifestReq.Header.Set("User-Agent", constants.DefaultUserAgent)

	manifestResp, err := httpClient.Do(manifestReq)
	if err != nil {
		return fmt.Errorf("fetching manifest for %s: %w", username, err)
	}
	defer manifestResp.Body.Close()

	if manifestResp.StatusCode != http.StatusOK {
		return fmt.Errorf("manifest for %s returned status %d", username, manifestResp.StatusCode)
	}

	manifestBody, err := io.ReadAll(io.LimitReader(manifestResp.Body, 256<<10))
	if err != nil {
		return fmt.Errorf("reading manifest for %s: %w", username, err)
	}

	c.Log.Debug("Got HLS manifest", "streamer", username)

	lowestQualityURL := getLastURL(string(manifestBody))
	if lowestQualityURL == "" {
		return fmt.Errorf("no stream URL found in manifest for %s", username)
	}

	streamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, lowestQualityURL, nil)
	if err != nil {
		return fmt.Errorf("creating stream request: %w", err)
	}
	streamReq.Header.Set("User-Agent", constants.DefaultUserAgent)

	streamResp, err := httpClient.Do(streamReq)
	if err != nil {
		return fmt.Errorf("fetching stream URL list for %s: %w", username, err)
	}
	defer streamResp.Body.Close()

	if streamResp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream URL list for %s returned status %d", username, streamResp.StatusCode)
	}

	streamBody, err := io.ReadAll(io.LimitReader(streamResp.Body, 256<<10))
	if err != nil {
		return fmt.Errorf("reading stream URL list for %s: %w", username, err)
	}

	segmentURL := getSecondLastURL(string(streamBody))
	if segmentURL == "" {
		return fmt.Errorf("no segment URL found for %s", username)
	}

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, segmentURL, nil)
	if err != nil {
		return fmt.Errorf("creating HEAD request: %w", err)
	}
	headReq.Header.Set("User-Agent", constants.DefaultUserAgent)

	headResp, err := httpClient.Do(headReq)
	if err != nil {
		return fmt.Errorf("HEAD request for %s: %w", username, err)
	}
	headResp.Body.Close()

	if headResp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD request for %s returned status %d", username, headResp.StatusCode)
	}

	c.Log.Debug("Simulated stream watching", "streamer", username)

	encodedPayload, err := encodePayload(payload)
	if err != nil {
		return fmt.Errorf("encoding payload for %s: %w", username, err)
	}

	spadeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, spadeURL,
		strings.NewReader(encodedPayload))
	if err != nil {
		return fmt.Errorf("creating spade request: %w", err)
	}
	spadeReq.Header.Set("User-Agent", constants.DefaultUserAgent)

	spadeResp, err := httpClient.Do(spadeReq)
	if err != nil {
		return fmt.Errorf("sending spade event for %s: %w", username, err)
	}
	spadeResp.Body.Close()

	if spadeResp.StatusCode == http.StatusNoContent || spadeResp.StatusCode == http.StatusOK {
		streamer.Mu.Lock()
		streamer.Stream.UpdateMinuteWatched()
		streamer.Mu.Unlock()

		c.Log.Debug("Sent minute watched event",
			"streamer", username,
			"status", spadeResp.StatusCode)

		c.logDropProgress(streamer)
		return nil
	}

	return fmt.Errorf("spade event for %s returned status %d", username, spadeResp.StatusCode)
}

func (c *Client) logDropProgress(streamer *model.Streamer) {
	streamer.Mu.RLock()
	defer streamer.Mu.RUnlock()

	for _, campaign := range streamer.Stream.Campaigns {
		for _, drop := range campaign.Drops {
			if drop.HasPreconditionsMet != nil && !*drop.HasPreconditionsMet {
				continue
			}
			if drop.IsPrintable {
				c.Log.Info("Drop progress",
					"streamer", streamer.Username,
					"stream", streamer.Stream.String(),
					"campaign", campaign.String(),
					"drop", drop.String(),
					"progress", drop.ProgressBar(),
					"event", string(model.EventDropStatus))
			}
		}
	}
}

// encodePayload encodes the minute-watched payload as base64 JSON,
func encodePayload(payload map[string]any) (string, error) {
	wrapped := []map[string]any{payload}
	jsonData, err := json.Marshal(wrapped)
	if err != nil {
		return "", fmt.Errorf("marshaling payload: %w", err)
	}
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

func getLastURL(manifest string) string {
	lines := strings.Split(strings.TrimSpace(manifest), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			return line
		}
	}
	return ""
}

// getSecondLastURL returns the second-to-last URL from an m3u8 segment playlist.
func getSecondLastURL(playlist string) string {
	lines := strings.Split(strings.TrimSpace(playlist), "\n")
	count := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			count++
			if count == 1 {
				continue
			}
			return line
		}
	}
	if count == 1 {
		return getLastURL(playlist)
	}
	return ""
}

// SelectStreamersToWatch selects up to maxWatch streamers to send minute-watched
// events for, based on the configured priority order.
// This implements the priority selection logic from the Python version.
func SelectStreamersToWatch(streamers []*model.Streamer, priorities []model.Priority, maxWatch int) []*model.Streamer {
	if maxWatch <= 0 {
		maxWatch = constants.MaxWatchStreams
	}

	now := time.Now()

	var onlineIndices []int
	for i, s := range streamers {
		s.Mu.RLock()
		isOnline := s.IsOnline
		onlineAt := s.OnlineAt
		s.Mu.RUnlock()

		if isOnline && (onlineAt.IsZero() || now.Sub(onlineAt) > 30*time.Second) {
			onlineIndices = append(onlineIndices, i)
		}
	}

	if len(onlineIndices) == 0 {
		return nil
	}

	watching := make(map[int]struct{})

	for _, priority := range priorities {
		if len(watching) >= maxWatch {
			break
		}

		remaining := maxWatch - len(watching)

		switch priority {
		case model.PriorityOrder:
			for _, idx := range onlineIndices {
				if _, ok := watching[idx]; !ok {
					watching[idx] = struct{}{}
					remaining--
					if remaining <= 0 {
						break
					}
				}
			}

		case model.PriorityStreak:
			for _, idx := range onlineIndices {
				if _, ok := watching[idx]; ok {
					continue
				}
				s := streamers[idx]
				s.Mu.RLock()
				watchStreak := s.Settings != nil && s.Settings.WatchStreak
				missing := s.Stream.WatchStreakMissing
				offlineAt := s.OfflineAt
				minuteWatched := s.Stream.MinuteWatched
				s.Mu.RUnlock()

				if watchStreak && missing &&
					(offlineAt.IsZero() || now.Sub(offlineAt).Minutes() > 30) &&
					minuteWatched < 7 {
					watching[idx] = struct{}{}
					remaining--
					if remaining <= 0 {
						break
					}
				}
			}

		case model.PriorityDrops:
			for _, idx := range onlineIndices {
				if _, ok := watching[idx]; ok {
					continue
				}
				s := streamers[idx]
				s.Mu.RLock()
				dropsCondition := s.DropsCondition()
				s.Mu.RUnlock()

				if dropsCondition {
					watching[idx] = struct{}{}
					remaining--
					if remaining <= 0 {
						break
					}
				}
			}

		case model.PrioritySubscribed:
			type indexMultiplier struct {
				index      int
				multiplier float64
			}
			var withMultiplier []indexMultiplier
			for _, idx := range onlineIndices {
				if _, ok := watching[idx]; ok {
					continue
				}
				s := streamers[idx]
				s.Mu.RLock()
				hasMultiplier := s.HasPointsMultiplier()
				total := s.TotalPointsMultiplier()
				s.Mu.RUnlock()

				if hasMultiplier {
					withMultiplier = append(withMultiplier, indexMultiplier{idx, total})
				}
			}
			sort.Slice(withMultiplier, func(i, j int) bool {
				return withMultiplier[i].multiplier > withMultiplier[j].multiplier
			})
			for _, im := range withMultiplier {
				watching[im.index] = struct{}{}
				remaining--
				if remaining <= 0 {
					break
				}
			}

		case model.PriorityPointsAscending, model.PriorityPointsDescending:
			type indexPoints struct {
				index  int
				points int
			}
			var items []indexPoints
			for _, idx := range onlineIndices {
				s := streamers[idx]
				s.Mu.RLock()
				points := s.ChannelPoints
				s.Mu.RUnlock()
				items = append(items, indexPoints{idx, points})
			}
			if priority == model.PriorityPointsAscending {
				sort.Slice(items, func(i, j int) bool {
					return items[i].points < items[j].points
				})
			} else {
				sort.Slice(items, func(i, j int) bool {
					return items[i].points > items[j].points
				})
			}
			for _, item := range items {
				if _, ok := watching[item.index]; !ok {
					watching[item.index] = struct{}{}
					remaining--
					if remaining <= 0 {
						break
					}
				}
			}
		}
	}

	result := make([]*model.Streamer, 0, len(watching))
	for idx := range watching {
		result = append(result, streamers[idx])
	}

	if len(result) > maxWatch {
		result = result[:maxWatch]
	}

	return result
}
