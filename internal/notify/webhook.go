package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Webhook sends notifications via a generic HTTP webhook.
type Webhook struct {
	baseNotifier
	url        string
	method     string
	httpClient *http.Client
}

// Send delivers a notification via the configured webhook endpoint.
// For POST requests, the payload is sent as JSON in the body.
// For GET requests, event and message are appended as query parameters.
func (w *Webhook) Send(ctx context.Context, event model.Event, title, message string) error {
	method := strings.ToUpper(w.method)

	var req *http.Request
	var err error

	switch method {
	case http.MethodGet:
		u, parseErr := url.Parse(w.url)
		if parseErr != nil {
			return fmt.Errorf("webhook: parse url: %w", parseErr)
		}
		q := u.Query()
		q.Set("event_name", string(event))
		q.Set("title", title)
		q.Set("message", message)
		u.RawQuery = q.Encode()
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)

	case http.MethodPost:
		payload := map[string]string{
			"event":   string(event),
			"title":   title,
			"message": message,
		}
		body, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return fmt.Errorf("webhook: marshal payload: %w", marshalErr)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}

	default:
		return fmt.Errorf("webhook: unsupported method %q (use GET or POST)", method)
	}

	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}

	return nil
}
