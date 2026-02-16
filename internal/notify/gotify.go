package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Gotify sends notifications via a Gotify server.
type Gotify struct {
	baseNotifier
	url        string
	token      string
	httpClient *http.Client
}

// Send posts a notification to the Gotify server.
func (g *Gotify) Send(ctx context.Context, _ model.Event, title, message string) error {
	payload := map[string]any{
		"title":    title,
		"message":  message,
		"priority": 5,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("gotify: marshal payload: %w", err)
	}

	endpoint := strings.TrimRight(g.url, "/") + "/message"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gotify: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", g.token)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gotify: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("gotify: unexpected status %d", resp.StatusCode)
	}

	return nil
}
