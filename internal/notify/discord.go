package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Discord sends notifications via a Discord webhook.
type Discord struct {
	baseNotifier
	webhookURL string
	httpClient *http.Client
}

// Send posts an embed message to the configured Discord webhook.
func (d *Discord) Send(ctx context.Context, _ model.Event, title, message string) error {
	payload := map[string]any{
		"username":   "Twitch Channel Points Miner",
		"avatar_url": "https://i.imgur.com/X9fEkhT.png",
		"embeds": []map[string]any{
			{
				"title":       title,
				"description": message,
				"color":       6570404, // Twitch purple
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord: unexpected status %d", resp.StatusCode)
	}

	return nil
}
