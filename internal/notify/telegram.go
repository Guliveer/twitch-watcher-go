package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Telegram sends notifications via the Telegram Bot API.
type Telegram struct {
	baseNotifier
	token               string
	chatID              string
	disableNotification bool
	httpClient          *http.Client
}

// Send posts a message to the configured Telegram chat.
func (t *Telegram) Send(ctx context.Context, _ model.Event, title, message string) error {
	text := message
	if title != "" {
		text = fmt.Sprintf("<b>%s</b>\n%s", title, message)
	}

	payload := map[string]any{
		"chat_id":                  t.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
		"disable_notification":     t.disableNotification,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
