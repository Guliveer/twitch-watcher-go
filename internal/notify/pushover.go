package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Pushover sends notifications via the Pushover API.
type Pushover struct {
	baseNotifier
	token      string
	userKey    string
	httpClient *http.Client
}

// Send posts a notification to the Pushover API.
func (p *Pushover) Send(ctx context.Context, _ model.Event, title, message string) error {
	form := url.Values{
		"token":   {p.token},
		"user":    {p.userKey},
		"title":   {title},
		"message": {message},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.pushover.net/1/messages.json",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("pushover: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pushover: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("pushover: unexpected status %d", resp.StatusCode)
	}

	return nil
}
