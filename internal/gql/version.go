package gql

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
)

// clientVersionTTL is how long the cached client version remains valid.
// This fixes the critical Python bug where it fetches the Twitch homepage
// on every single GQL request.
const clientVersionTTL = 30 * time.Minute

var twilightBuildIDPattern = regexp.MustCompile(
	`window\.__twilightBuildID\s*=\s*"([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})"`,
)

type versionCache struct {
	mu        sync.RWMutex
	version   string
	updatedAt time.Time
}

func newVersionCache() *versionCache {
	return &versionCache{
		version: constants.ClientVersion,
	}
}

func (vc *versionCache) get() (string, bool) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	if vc.version != "" && time.Since(vc.updatedAt) < clientVersionTTL {
		return vc.version, true
	}
	return vc.version, false
}

func (vc *versionCache) set(version string) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.version = version
	vc.updatedAt = time.Now()
}

// updateClientVersion fetches the Twitch homepage to extract the current
// client build ID. The result is cached for clientVersionTTL (30 minutes).
func (c *Client) updateClientVersion(ctx context.Context) string {
	if version, valid := c.versionCache.get(); valid {
		return version
	}

	version, err := fetchClientVersion(ctx, c.httpClient)
	if err != nil {
		c.log.Debug("Failed to update client version, using cached",
			"error", err)
		v, _ := c.versionCache.get()
		return v
	}

	c.versionCache.set(version)
	c.log.Debug("Updated client version", "version", version)
	return version
}

// fetchClientVersion makes an HTTP GET to the Twitch homepage and extracts
func fetchClientVersion(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, constants.TwitchURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", constants.DefaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching Twitch homepage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Twitch homepage returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading Twitch homepage: %w", err)
	}

	matches := twilightBuildIDPattern.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("twilight build ID not found in homepage")
	}

	return string(matches[1]), nil
}
