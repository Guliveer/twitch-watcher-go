// Package twitch provides a high-level Twitch API client that combines
// authentication, GQL operations, and business logic for the miner.
package twitch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/auth"
	"github.com/Guliveer/twitch-miner-go/internal/config"
	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/gql"
	"github.com/Guliveer/twitch-miner-go/internal/logger"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// Pre-compiled regexes for updateSpadeURL (Fix #4: avoid compiling per-call).
var (
	settingsURLRegex = regexp.MustCompile(`(https://static\.twitchcdn\.net/config/settings.*?js|https://assets\.twitch\.tv/config/settings.*?\.js)`)
	spadeURLRegex    = regexp.MustCompile(`"spade_url":"(.*?)"`)
)

// spadeCacheTTL is how long a cached spade URL remains valid before re-fetching.
// Increased to 6 hours: the spade URL rarely changes during a stream session.
// The previous 30-minute TTL caused silent failures when the cache expired
// but the streamer stayed online (updateSpadeURL was only called on offline→online).
const spadeCacheTTL = 6 * time.Hour

type spadeCache struct {
	mu      sync.RWMutex
	entries map[string]spadeCacheEntry
}

type spadeCacheEntry struct {
	url       string
	fetchedAt time.Time
}

func (sc *spadeCache) get(login string) (string, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	entry, ok := sc.entries[login]
	if !ok {
		return "", false
	}
	if time.Since(entry.fetchedAt) > spadeCacheTTL {
		delete(sc.entries, login)
		return "", false
	}
	return entry.url, true
}

func (sc *spadeCache) set(login, url string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.entries[login] = spadeCacheEntry{url: url, fetchedAt: time.Now()}
}

// prune removes all expired entries from the cache. This is called after each
// successful spade URL update to clean up stale entries from streamers that
// were removed by the category watcher.
func (sc *spadeCache) prune() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for login, entry := range sc.entries {
		if time.Since(entry.fetchedAt) > spadeCacheTTL {
			delete(sc.entries, login)
		}
	}
}

// Client is the high-level Twitch API facade combining auth and GQL client.
// It provides business-logic methods for the miner.
type Client struct {
	Auth *auth.Authenticator
	GQL *gql.Client
	Log *logger.Logger
	cfg *config.AccountConfig
	spadeURLs *spadeCache
}

// NewClient creates a new high-level Twitch Client from account configuration.
func NewClient(cfg *config.AccountConfig, log *logger.Logger) (*Client, error) {
	authenticator := auth.NewAuthenticator(cfg, log)
	gqlClient := gql.NewClient(authenticator, log)

	return &Client{
		Auth:      authenticator,
		GQL:       gqlClient,
		Log:       log,
		cfg:       cfg,
		spadeURLs: &spadeCache{entries: make(map[string]spadeCacheEntry)},
	}, nil
}

// Login performs the authentication flow.
func (c *Client) Login(ctx context.Context) error {
	return c.Auth.Login(ctx)
}

// CheckStreamerOnline checks if a streamer is online and updates their state.
// If the streamer was recently marked offline (< 60s), it skips the check.
func (c *Client) CheckStreamerOnline(ctx context.Context, streamer *model.Streamer) error {
	streamer.Mu.Lock()
	if !streamer.OfflineAt.IsZero() && time.Since(streamer.OfflineAt) < 60*time.Second {
		streamer.Mu.Unlock()
		return nil
	}
	// Fix #2: Don't skip the check entirely when the streamer was recently marked
	// online — only skip if the spade URL is already populated. Category-watched
	// streamers are marked online before updateSpadeURL runs, so the early return
	// previously prevented them from ever getting a spade URL.
	if streamer.IsOnline && !streamer.OnlineAt.IsZero() && time.Since(streamer.OnlineAt) < 2*time.Minute {
		hasSpadeURL := streamer.Stream != nil && streamer.Stream.SpadeURL != ""
		if hasSpadeURL {
			streamer.Mu.Unlock()
			return nil
		}
		// Fall through to fetch spade URL even though recently marked online.
	}
	wasOnline := streamer.IsOnline
	streamer.Mu.Unlock()

	if !wasOnline {
		if err := c.updateSpadeURL(ctx, streamer); err != nil {
			c.Log.Debug("Failed to get spade URL", "streamer", streamer.Username, "error", err)
		}

		if err := c.updateStream(ctx, streamer); err != nil {
			streamer.Mu.Lock()
			streamer.SetOffline()
			streamer.Mu.Unlock()
			return nil
		}

		streamer.Mu.Lock()
		streamer.SetOnline()
		streamer.Mu.Unlock()
	} else {
		// Streamer is already online — refresh spade URL if missing.
		streamer.Mu.RLock()
		needsSpade := streamer.Stream == nil || streamer.Stream.SpadeURL == ""
		streamer.Mu.RUnlock()
		if needsSpade {
			if err := c.updateSpadeURL(ctx, streamer); err != nil {
				c.Log.Debug("Failed to refresh spade URL", "streamer", streamer.Username, "error", err)
			}
		}

		if err := c.updateStream(ctx, streamer); err != nil {
			streamer.Mu.Lock()
			streamer.SetOffline()
			streamer.Mu.Unlock()
		}
	}

	return nil
}

func (c *Client) updateStream(ctx context.Context, streamer *model.Streamer) error {
	streamer.Mu.RLock()
	needsUpdate := streamer.Stream.UpdateRequired()
	username := streamer.Username
	streamer.Mu.RUnlock()

	if !needsUpdate {
		return nil
	}

	info, err := c.GQL.GetStreamInfo(ctx, username)
	if err != nil {
		return fmt.Errorf("getting stream info for %s: %w", username, err)
	}

	if info == nil {
		return fmt.Errorf("streamer %s is offline", username)
	}

	streamer.Mu.Lock()
	defer streamer.Mu.Unlock()

	streamer.Stream.Update(
		info.BroadcastID,
		info.Title,
		info.Game,
		info.Tags,
		info.ViewersCount,
		constants.DropID,
	)

	// Resolve game slug if the API didn't return one (e.g. VideoPlayerStreamInfo
	// persisted query omits slug). Check the registry first, then fetch via GQL.
	if streamer.Stream.Game != nil && streamer.Stream.Game.Slug == "" && streamer.Stream.Game.ID != "" {
		if slug := model.LookupGameSlug(streamer.Stream.Game.ID); slug != "" {
			streamer.Stream.Game.Slug = slug
		} else {
			gameID := streamer.Stream.Game.ID
			// Release lock while making the network call to avoid blocking other goroutines.
			streamer.Mu.Unlock()
			slug, err := c.GQL.GetGameSlug(ctx, gameID)
			streamer.Mu.Lock()
			if err == nil && slug != "" {
				streamer.Stream.Game.Slug = slug
				model.RegisterGameSlug(gameID, slug)
			} else if err != nil {
				c.Log.Debug("Failed to fetch game slug",
					"streamer", username,
					"game_id", gameID,
					"error", err)
			}
		}
	}

	payload := map[string]any{
		"channel_id":   streamer.ChannelID,
		"broadcast_id": streamer.Stream.BroadcastID,
		"player":       "site",
		"user_id":      c.Auth.UserID(),
		"live":         true,
		"channel":      streamer.Username,
	}

	if streamer.Stream.GameName() != "" && streamer.Stream.GameID() != "" &&
		streamer.Settings != nil && streamer.Settings.ClaimDrops {
		payload["game"] = streamer.Stream.GameName()
		payload["game_id"] = streamer.Stream.GameID()

		campaignIDs, err := c.GQL.GetAvailableCampaigns(ctx, streamer.ChannelID)
		if err == nil {
			streamer.Stream.CampaignIDs = campaignIDs
		}
	}

	streamer.Stream.Payload = map[string]any{
		"event":      "minute-watched",
		"properties": payload,
	}

	return nil
}

// updateSpadeURL fetches the spade URL for a streamer by scraping their channel page.
func (c *Client) updateSpadeURL(ctx context.Context, streamer *model.Streamer) error {
	streamer.Mu.RLock()
	streamerURL := streamer.StreamerURL
	username := streamer.Username
	streamer.Mu.RUnlock()

	if cached, ok := c.spadeURLs.get(username); ok {
		streamer.Mu.Lock()
		streamer.Stream.SpadeURL = cached
		streamer.Mu.Unlock()
		c.Log.Debug("Using cached spade URL", "streamer", username)
		return nil
	}

	httpClient := c.GQL.HTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamerURL, nil)
	if err != nil {
		return fmt.Errorf("creating request for %s: %w", streamerURL, err)
	}
	req.Header.Set("User-Agent", constants.UserAgents["Linux"]["FIREFOX"])

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", streamerURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return fmt.Errorf("reading response from %s: %w", streamerURL, err)
	}

	settingsMatch := settingsURLRegex.FindString(string(body))
	if settingsMatch == "" {
		return fmt.Errorf("settings URL not found in %s", streamerURL)
	}

	settingsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, settingsMatch, nil)
	if err != nil {
		return fmt.Errorf("creating settings request: %w", err)
	}
	settingsReq.Header.Set("User-Agent", constants.UserAgents["Linux"]["FIREFOX"])

	settingsResp, err := httpClient.Do(settingsReq)
	if err != nil {
		return fmt.Errorf("fetching settings: %w", err)
	}
	defer settingsResp.Body.Close()

	settingsBody, err := io.ReadAll(io.LimitReader(settingsResp.Body, 512<<10))
	if err != nil {
		return fmt.Errorf("reading settings response: %w", err)
	}

	spadeMatch := spadeURLRegex.FindSubmatch(settingsBody)
	if len(spadeMatch) < 2 {
		return fmt.Errorf("spade_url not found in settings")
	}

	spadeURL := string(spadeMatch[1])

	c.spadeURLs.set(username, spadeURL)
	c.spadeURLs.prune()

	streamer.Mu.Lock()
	streamer.Stream.SpadeURL = spadeURL
	streamer.Mu.Unlock()

	c.Log.Debug("Updated spade URL", "streamer", username, "spade_url", spadeURL)
	return nil
}

// LoadChannelPointsContext loads channel points balance, multipliers,
// available claims, and community goals for a streamer.
func (c *Client) LoadChannelPointsContext(ctx context.Context, streamer *model.Streamer) error {
	streamer.Mu.RLock()
	username := streamer.Username
	channelID := streamer.ChannelID
	streamer.Mu.RUnlock()

	cpc, err := c.GQL.GetChannelPointsContext(ctx, username)
	if err != nil {
		return fmt.Errorf("loading channel points context for %s: %w", username, err)
	}

	streamer.Mu.Lock()
	streamer.ChannelPoints = cpc.Balance
	streamer.ActiveMultipliers = cpc.ActiveMultipliers

	goalsEnabled := streamer.Settings != nil && streamer.Settings.CommunityGoalsEnabled
	if goalsEnabled {
		for _, goal := range cpc.CommunityGoals {
			streamer.UpdateCommunityGoal(goal)
		}
	}

	if cpc.AvailableClaimID != "" {
		c.Log.Info("Claiming channel points bonus",
			"streamer", username,
			"claim_id", cpc.AvailableClaimID)
		streamer.Mu.Unlock()
		if err := c.GQL.ClaimCommunityPoints(ctx, cpc.AvailableClaimID, channelID); err != nil {
			c.Log.Warn("Failed to claim bonus",
				"streamer", username,
				"error", err)
		}
	} else {
		streamer.Mu.Unlock()
	}

	if goalsEnabled {
		c.contributeToCommunityGoals(ctx, streamer)
	}

	return nil
}

// contributeToCommunityGoals contributes channel points to active community goals.
func (c *Client) contributeToCommunityGoals(ctx context.Context, streamer *model.Streamer) {
	streamer.Mu.RLock()
	hasActiveGoals := false
	type goalSnapshot struct {
		goalID                       string
		title                        string
		amountLeft                   int
		perStreamUserMaxContribution int
	}
	var activeGoals []goalSnapshot
	for _, goal := range streamer.CommunityGoals {
		if goal.Status == "STARTED" && goal.IsInStock {
			hasActiveGoals = true
			activeGoals = append(activeGoals, goalSnapshot{
				goalID:                       goal.GoalID,
				title:                        goal.Title,
				amountLeft:                   goal.AmountLeft(),
				perStreamUserMaxContribution: goal.PerStreamUserMaxContribution,
			})
		}
	}
	username := streamer.Username
	channelID := streamer.ChannelID
	streamer.Mu.RUnlock()

	if !hasActiveGoals {
		return
	}

	contributions, err := c.GQL.GetUserPointsContribution(ctx, username)
	if err != nil {
		c.Log.Debug("Failed to get user points contribution",
			"streamer", username, "error", err)
		return
	}

	goalMap := make(map[string]goalSnapshot, len(activeGoals))
	for _, g := range activeGoals {
		goalMap[g.goalID] = g
	}

	for _, contrib := range contributions {
		goalID := contrib.Goal.ID
		gs, ok := goalMap[goalID]
		if !ok {
			continue
		}

		streamer.Mu.RLock()
		balance := streamer.ChannelPoints
		streamer.Mu.RUnlock()

		userLeftToContribute := gs.perStreamUserMaxContribution - contrib.UserPointsContributedThisStream
		amount := min(gs.amountLeft, userLeftToContribute, balance)

		if amount > 0 {
			transactionID := auth.GenerateHex(16)

			err := c.GQL.ContributeToCommunityGoal(ctx, goalID, channelID, amount, transactionID)
			if err != nil {
				c.Log.Warn("Failed to contribute to community goal",
					"streamer", username,
					"goal", gs.title,
					"error", err)
			} else {
				c.Log.Info("Contributed to community goal",
					"streamer", username,
					"goal", gs.title,
					"amount", amount)
				streamer.Mu.Lock()
				streamer.ChannelPoints -= amount
				streamer.Mu.Unlock()
			}
		}
	}
}

// ClaimChannelPoints claims a channel points bonus for a streamer.
func (c *Client) ClaimChannelPoints(ctx context.Context, streamer *model.Streamer, claimID string) error {
	streamer.Mu.RLock()
	channelID := streamer.ChannelID
	username := streamer.Username
	streamer.Mu.RUnlock()

	c.Log.Info("Claiming channel points bonus",
		"streamer", username,
		"claim_id", claimID)

	return c.GQL.ClaimCommunityPoints(ctx, claimID, channelID)
}

// JoinRaid joins a raid by its ID.
func (c *Client) JoinRaid(ctx context.Context, raidID string) error {
	c.Log.Info("Joining raid", "raid_id", raidID)
	return c.GQL.JoinRaid(ctx, raidID)
}

// ClaimMoment claims a community moment.
func (c *Client) ClaimMoment(ctx context.Context, momentID string) error {
	c.Log.Info("Claiming moment", "moment_id", momentID)
	return c.GQL.ClaimCommunityMoment(ctx, momentID)
}

// GetChannelID fetches the channel ID for a streamer username.
func (c *Client) GetChannelID(ctx context.Context, username string) (string, error) {
	return c.GQL.GetUserID(ctx, username)
}

// GetFollowers fetches the list of followed channel logins.
func (c *Client) GetFollowers(ctx context.Context, limit int, order string) ([]string, error) {
	return c.GQL.GetFollowedStreamers(ctx, limit, order)
}

// CheckViewerIsMod checks if the authenticated user is a moderator for a channel.
func (c *Client) CheckViewerIsMod(ctx context.Context, streamer *model.Streamer) {
	isMod, err := c.GQL.CheckViewerIsMod(ctx, streamer.Username)
	if err != nil {
		c.Log.Debug("Failed to check mod status",
			"streamer", streamer.Username,
			"error", err)
		return
	}
	streamer.Mu.Lock()
	streamer.ViewerIsMod = isMod
	streamer.Mu.Unlock()
}

// RefreshSpadeURL re-fetches the spade URL for a streamer if it is missing or expired.
// This is called from sendMinuteWatchedForStreamer when the spade URL is empty,
// ensuring that minute-watched events don't silently fail after cache expiry.
func (c *Client) RefreshSpadeURL(ctx context.Context, streamer *model.Streamer) error {
	return c.updateSpadeURL(ctx, streamer)
}

// GQLClient returns the underlying GQL client for use by other packages
// (e.g., category watcher). This satisfies the twitch.API interface.
func (c *Client) GQLClient() *gql.Client {
	return c.GQL
}

// AuthProvider returns the authenticator as an auth.Provider interface.
// This satisfies the twitch.API interface.
func (c *Client) AuthProvider() auth.Provider {
	return c.Auth
}

