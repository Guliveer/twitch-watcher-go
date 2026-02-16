package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Matrix sends notifications via the Matrix client-server API.
type Matrix struct {
	baseNotifier
	homeserver  string
	accessToken string
	roomID      string
	httpClient  *http.Client
	txnCounter  atomic.Int64
}

// Send puts a message into the configured Matrix room.
func (m *Matrix) Send(ctx context.Context, _ model.Event, _, message string) error {
	encodedRoomID := url.PathEscape(m.roomID)
	txnID := fmt.Sprintf("m%d.%d", time.Now().UnixNano(), m.txnCounter.Add(1))

	apiURL := fmt.Sprintf("https://%s/_matrix/client/r0/rooms/%s/send/m.room.message/%s",
		m.homeserver, encodedRoomID, txnID)

	payload := map[string]string{
		"msgtype": "m.text",
		"body":    message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("matrix: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("matrix: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("matrix: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("matrix: unexpected status %d", resp.StatusCode)
	}

	return nil
}
