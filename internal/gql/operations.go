package gql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// ChannelPointsContext holds the parsed response from the ChannelPointsContext GQL query.
type ChannelPointsContext struct {
	Balance int
	ActiveMultipliers []model.PointsMultiplier
	AvailableClaimID string
	CommunityGoals []*model.CommunityGoal
}

// PlaybackAccessToken holds the signature and token needed for HLS manifest access.
type PlaybackAccessToken struct {
	Signature string `json:"signature"`
	Value string `json:"value"`
}

// TopStream holds information about a stream returned by the DirectoryPage_Game query.
type TopStream struct {
	Username string
	ChannelID string
	DisplayName string
	ViewersCount int
	GameID string
	GameName string
	GameSlug string
}

// GetChannelPointsContext fetches channel points balance, multipliers, available claims,
// and community goals for a channel.
func (c *Client) GetChannelPointsContext(ctx context.Context, channelLogin string) (*ChannelPointsContext, error) {
	vars := map[string]any{"channelLogin": channelLogin}
	data, err := c.PostGQL(ctx, constants.GQLChannelPointsContext, vars)
	if err != nil {
		return nil, fmt.Errorf("ChannelPointsContext for %s: %w", channelLogin, err)
	}

	var resp struct {
		Community *struct {
			Channel struct {
				Self struct {
					CommunityPoints struct {
						Balance           int                `json:"balance"`
						ActiveMultipliers []json.RawMessage  `json:"activeMultipliers"`
						AvailableClaim    *struct {
							ID string `json:"id"`
						} `json:"availableClaim"`
					} `json:"communityPoints"`
				} `json:"self"`
				CommunityPointsSettings struct {
					Goals []json.RawMessage `json:"goals"`
				} `json:"communityPointsSettings"`
			} `json:"channel"`
		} `json:"community"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing ChannelPointsContext response: %w", err)
	}

	if resp.Community == nil {
		return nil, fmt.Errorf("channel %s not found (community is null)", channelLogin)
	}

	result := &ChannelPointsContext{
		Balance: resp.Community.Channel.Self.CommunityPoints.Balance,
	}

	for _, raw := range resp.Community.Channel.Self.CommunityPoints.ActiveMultipliers {
		var multiplier struct {
			Factor float64 `json:"factor"`
		}
		if err := json.Unmarshal(raw, &multiplier); err == nil {
			result.ActiveMultipliers = append(result.ActiveMultipliers, model.PointsMultiplier{Factor: multiplier.Factor})
		}
	}

	if resp.Community.Channel.Self.CommunityPoints.AvailableClaim != nil {
		result.AvailableClaimID = resp.Community.Channel.Self.CommunityPoints.AvailableClaim.ID
	}

	for _, raw := range resp.Community.Channel.CommunityPointsSettings.Goals {
		var goalMap map[string]any
		if err := json.Unmarshal(raw, &goalMap); err == nil {
			result.CommunityGoals = append(result.CommunityGoals, model.CommunityGoalFromGQL(goalMap))
		}
	}

	return result, nil
}

// ClaimCommunityPoints claims a channel points bonus.
func (c *Client) ClaimCommunityPoints(ctx context.Context, claimID, channelID string) error {
	vars := map[string]any{
		"input": map[string]any{
			"channelID": channelID,
			"claimID":   claimID,
		},
	}
	_, err := c.PostGQL(ctx, constants.GQLClaimCommunityPoints, vars)
	if err != nil {
		return fmt.Errorf("ClaimCommunityPoints: %w", err)
	}
	return nil
}

// GetStreamInfo fetches stream information for a channel.
// Returns nil if the streamer is offline.
func (c *Client) GetStreamInfo(ctx context.Context, channelLogin string) (*StreamInfoResponse, error) {
	vars := map[string]any{"channel": channelLogin}
	data, err := c.PostGQL(ctx, constants.GQLVideoPlayerStreamInfoOverlayChannel, vars)
	if err != nil {
		return nil, fmt.Errorf("GetStreamInfo for %s: %w", channelLogin, err)
	}

	var resp struct {
		User *struct {
			Stream *struct {
				ID           string `json:"id"`
				ViewersCount int    `json:"viewersCount"`
				Tags         []struct {
					ID            string `json:"id"`
					LocalizedName string `json:"localizedName"`
				} `json:"tags"`
			} `json:"stream"`
			BroadcastSettings struct {
				Title string     `json:"title"`
				Game  *GameResp  `json:"game"`
			} `json:"broadcastSettings"`
		} `json:"user"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetStreamInfo response: %w", err)
	}

	if resp.User == nil || resp.User.Stream == nil {
		return nil, nil // Streamer is offline
	}

	result := &StreamInfoResponse{
		BroadcastID:  resp.User.Stream.ID,
		Title:        resp.User.BroadcastSettings.Title,
		ViewersCount: resp.User.Stream.ViewersCount,
	}

	if resp.User.BroadcastSettings.Game != nil {
		result.Game = &model.GameInfo{
			ID:          resp.User.BroadcastSettings.Game.ID,
			Name:        resp.User.BroadcastSettings.Game.Name,
			DisplayName: resp.User.BroadcastSettings.Game.DisplayName,
			Slug:        resp.User.BroadcastSettings.Game.Slug,
		}
	}

	for _, tag := range resp.User.Stream.Tags {
		result.Tags = append(result.Tags, model.Tag{
			ID:            tag.ID,
			LocalizedName: tag.LocalizedName,
		})
	}

	return result, nil
}

// StreamInfoResponse holds parsed stream info from the GQL API.
type StreamInfoResponse struct {
	BroadcastID  string
	Title        string
	Game         *model.GameInfo
	Tags         []model.Tag
	ViewersCount int
}

// GameResp is the GQL game response shape.
type GameResp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Slug        string `json:"slug"`
}

// GetUserID fetches the Twitch user ID for a given login name.
func (c *Client) GetUserID(ctx context.Context, login string) (string, error) {
	vars := map[string]any{"login": login}
	data, err := c.PostGQL(ctx, constants.GQLGetIDFromLogin, vars)
	if err != nil {
		return "", fmt.Errorf("GetUserID for %s: %w", login, err)
	}

	var resp struct {
		User *struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing GetUserID response: %w", err)
	}

	if resp.User == nil || resp.User.ID == "" {
		return "", fmt.Errorf("user %s not found", login)
	}

	return resp.User.ID, nil
}

// GetFollowedStreamers fetches the list of followed channel logins for a user.
// It paginates through all results.
func (c *Client) GetFollowedStreamers(ctx context.Context, limit int, order string) ([]string, error) {
	var follows []string
	cursor := ""
	hasNext := true

	for hasNext {
		vars := map[string]any{
			"limit":  limit,
			"order":  order,
			"cursor": cursor,
		}

		data, err := c.PostGQL(ctx, constants.GQLChannelFollows, vars)
		if err != nil {
			return nil, fmt.Errorf("GetFollowedStreamers: %w", err)
		}

		var resp struct {
			User *struct {
				Follows struct {
					Edges []struct {
						Node struct {
							Login string `json:"login"`
						} `json:"node"`
						Cursor string `json:"cursor"`
					} `json:"edges"`
					PageInfo struct {
						HasNextPage bool `json:"hasNextPage"`
					} `json:"pageInfo"`
				} `json:"follows"`
			} `json:"user"`
		}

		if err := json.Unmarshal(data, &resp); err != nil {
			return follows, fmt.Errorf("parsing GetFollowedStreamers response: %w", err)
		}

		if resp.User == nil {
			return follows, nil
		}

		for _, edge := range resp.User.Follows.Edges {
			follows = append(follows, edge.Node.Login)
			cursor = edge.Cursor
		}

		hasNext = resp.User.Follows.PageInfo.HasNextPage
	}

	return follows, nil
}

// MakePrediction places a prediction bet on an event.
func (c *Client) MakePrediction(ctx context.Context, eventID, outcomeID string, points int, transactionID string) error {
	vars := map[string]any{
		"input": map[string]any{
			"eventID":       eventID,
			"outcomeID":     outcomeID,
			"points":        points,
			"transactionID": transactionID,
		},
	}

	data, err := c.PostGQL(ctx, constants.GQLMakePrediction, vars)
	if err != nil {
		return fmt.Errorf("MakePrediction: %w", err)
	}

	var resp struct {
		MakePrediction *struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"makePrediction"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parsing MakePrediction response: %w", err)
	}

	if resp.MakePrediction != nil && resp.MakePrediction.Error != nil {
		return fmt.Errorf("prediction error: %s", resp.MakePrediction.Error.Code)
	}

	return nil
}

// GetAvailableCampaigns fetches available drop campaigns for a channel.
func (c *Client) GetAvailableCampaigns(ctx context.Context, channelID string) ([]string, error) {
	vars := map[string]any{"channelID": channelID}
	data, err := c.PostGQL(ctx, constants.GQLDropsHighlightServiceAvailableDrops, vars)
	if err != nil {
		return nil, fmt.Errorf("GetAvailableCampaigns: %w", err)
	}

	var resp struct {
		Channel *struct {
			ViewerDropCampaigns []struct {
				ID string `json:"id"`
			} `json:"viewerDropCampaigns"`
		} `json:"channel"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetAvailableCampaigns response: %w", err)
	}

	if resp.Channel == nil || resp.Channel.ViewerDropCampaigns == nil {
		return nil, nil
	}

	ids := make([]string, 0, len(resp.Channel.ViewerDropCampaigns))
	for _, campaign := range resp.Channel.ViewerDropCampaigns {
		ids = append(ids, campaign.ID)
	}
	return ids, nil
}

// GetDropsDashboard fetches drop campaigns from the viewer dashboard.
// If status is non-empty, only campaigns with that status are returned.
func (c *Client) GetDropsDashboard(ctx context.Context, status string) ([]json.RawMessage, error) {
	data, err := c.PostGQL(ctx, constants.GQLViewerDropsDashboard, nil)
	if err != nil {
		return nil, fmt.Errorf("GetDropsDashboard: %w", err)
	}

	var resp struct {
		CurrentUser *struct {
			DropCampaigns []json.RawMessage `json:"dropCampaigns"`
		} `json:"currentUser"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetDropsDashboard response: %w", err)
	}

	if resp.CurrentUser == nil {
		return nil, nil
	}

	campaigns := resp.CurrentUser.DropCampaigns
	if status == "" {
		return campaigns, nil
	}

	var filtered []json.RawMessage
	for _, raw := range campaigns {
		var campaignStatus struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &campaignStatus); err == nil && campaignStatus.Status == status {
			filtered = append(filtered, raw)
		}
	}
	return filtered, nil
}

// GetDropsInventory fetches the user's drop inventory.
func (c *Client) GetDropsInventory(ctx context.Context) (json.RawMessage, error) {
	data, err := c.PostGQL(ctx, constants.GQLInventory, nil)
	if err != nil {
		return nil, fmt.Errorf("GetDropsInventory: %w", err)
	}

	var resp struct {
		CurrentUser *struct {
			Inventory json.RawMessage `json:"inventory"`
		} `json:"currentUser"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetDropsInventory response: %w", err)
	}

	if resp.CurrentUser == nil {
		return nil, nil
	}

	return resp.CurrentUser.Inventory, nil
}

// GetDropCampaignDetails fetches detailed information about a drop campaign.
func (c *Client) GetDropCampaignDetails(ctx context.Context, dropID, channelLogin string) (json.RawMessage, error) {
	vars := map[string]any{
		"dropID":       dropID,
		"channelLogin": channelLogin,
	}
	data, err := c.PostGQL(ctx, constants.GQLDropCampaignDetails, vars)
	if err != nil {
		return nil, fmt.Errorf("GetDropCampaignDetails: %w", err)
	}

	var resp struct {
		User *struct {
			DropCampaign json.RawMessage `json:"dropCampaign"`
		} `json:"user"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetDropCampaignDetails response: %w", err)
	}

	if resp.User == nil {
		return nil, nil
	}

	return resp.User.DropCampaign, nil
}

// ClaimDropRewards claims a drop reward by its instance ID.
func (c *Client) ClaimDropRewards(ctx context.Context, dropInstanceID string) (bool, error) {
	vars := map[string]any{
		"input": map[string]any{
			"dropInstanceID": dropInstanceID,
		},
	}

	data, err := c.PostGQL(ctx, constants.GQLDropsPageClaimDropRewards, vars)
	if err != nil {
		return false, fmt.Errorf("ClaimDropRewards: %w", err)
	}

	var resp struct {
		ClaimDropRewards *struct {
			Status string `json:"status"`
		} `json:"claimDropRewards"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return false, fmt.Errorf("parsing ClaimDropRewards response: %w", err)
	}

	if resp.ClaimDropRewards == nil {
		return false, nil
	}

	switch resp.ClaimDropRewards.Status {
	case "ELIGIBLE_FOR_ALL", "DROP_INSTANCE_ALREADY_CLAIMED":
		return true, nil
	default:
		return false, nil
	}
}

// JoinRaid joins a raid by its ID.
func (c *Client) JoinRaid(ctx context.Context, raidID string) error {
	vars := map[string]any{
		"input": map[string]any{
			"raidID": raidID,
		},
	}

	_, err := c.PostGQL(ctx, constants.GQLJoinRaid, vars)
	if err != nil {
		return fmt.Errorf("JoinRaid: %w", err)
	}
	return nil
}

// GetPlaybackAccessToken fetches the playback access token for a live stream.
func (c *Client) GetPlaybackAccessToken(ctx context.Context, login string) (*PlaybackAccessToken, error) {
	vars := map[string]any{
		"login":      login,
		"isLive":     true,
		"isVod":      false,
		"vodID":      "",
		"playerType": "site",
	}

	data, err := c.PostGQL(ctx, constants.GQLPlaybackAccessToken, vars)
	if err != nil {
		return nil, fmt.Errorf("GetPlaybackAccessToken for %s: %w", login, err)
	}

	var resp struct {
		StreamPlaybackAccessToken *PlaybackAccessToken `json:"streamPlaybackAccessToken"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetPlaybackAccessToken response: %w", err)
	}

	if resp.StreamPlaybackAccessToken == nil {
		return nil, fmt.Errorf("no playback access token for %s (stream may be offline)", login)
	}

	return resp.StreamPlaybackAccessToken, nil
}

// ClaimCommunityMoment claims a community moment by its ID.
func (c *Client) ClaimCommunityMoment(ctx context.Context, momentID string) error {
	vars := map[string]any{
		"input": map[string]any{
			"momentID": momentID,
		},
	}

	_, err := c.PostGQL(ctx, constants.GQLCommunityMomentCalloutClaim, vars)
	if err != nil {
		return fmt.Errorf("ClaimCommunityMoment: %w", err)
	}
	return nil
}

// GetTopStreamsByCategory fetches top streams for a game category.
func (c *Client) GetTopStreamsByCategory(ctx context.Context, categorySlug string, limit int, dropsOnly bool) ([]TopStream, error) {
	vars := map[string]any{
		"slug":  categorySlug,
		"first": limit,
	}

	if dropsOnly {
		vars["options"] = map[string]any{
			"tags": []string{constants.DropID},
		}
	}

	data, err := c.PostGQL(ctx, constants.GQLDirectoryPageGame, vars)
	if err != nil {
		return nil, fmt.Errorf("GetTopStreamsByCategory for %s: %w", categorySlug, err)
	}

	var resp struct {
		Game *struct {
			Streams struct {
				Edges []struct {
					Node struct {
						Broadcaster struct {
							ID          string `json:"id"`
							Login       string `json:"login"`
							DisplayName string `json:"displayName"`
						} `json:"broadcaster"`
						ViewersCount int `json:"viewersCount"`
						Game         *struct {
							ID          string `json:"id"`
							DisplayName string `json:"displayName"`
							Slug        string `json:"slug"`
						} `json:"game"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"streams"`
		} `json:"game"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetTopStreamsByCategory response: %w", err)
	}

	if resp.Game == nil {
		return nil, fmt.Errorf("category %s not found", categorySlug)
	}

	streams := make([]TopStream, 0, len(resp.Game.Streams.Edges))
	for _, edge := range resp.Game.Streams.Edges {
		node := edge.Node
		topStream := TopStream{
			Username:     node.Broadcaster.Login,
			ChannelID:    node.Broadcaster.ID,
			DisplayName:  node.Broadcaster.DisplayName,
			ViewersCount: node.ViewersCount,
		}
		if node.Game != nil {
			topStream.GameID = node.Game.ID
			topStream.GameName = node.Game.DisplayName
			topStream.GameSlug = node.Game.Slug
		}

		if topStream.ChannelID == "" {
			if topStream.Username != "" {
				if id, err := c.GetUserID(ctx, topStream.Username); err == nil {
					topStream.ChannelID = id
				}
			}
			if topStream.ChannelID == "" {
				continue
			}
		}

		streams = append(streams, topStream)
	}

	return streams, nil
}

// ContributeToCommunityGoal contributes channel points to a community goal.
func (c *Client) ContributeToCommunityGoal(ctx context.Context, goalID, channelID string, points int, transactionID string) error {
	vars := map[string]any{
		"input": map[string]any{
			"amount":        points,
			"channelID":     channelID,
			"goalID":        goalID,
			"transactionID": transactionID,
		},
	}

	data, err := c.PostGQL(ctx, constants.GQLContributeCommunityPointsCommunityGoal, vars)
	if err != nil {
		return fmt.Errorf("ContributeToCommunityGoal: %w", err)
	}

	var resp struct {
		ContributeCommunityPointsCommunityGoal struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"contributeCommunityPointsCommunityGoal"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parsing ContributeToCommunityGoal response: %w", err)
	}

	if resp.ContributeCommunityPointsCommunityGoal.Error != nil {
		return fmt.Errorf("community goal contribution error: %s",
			resp.ContributeCommunityPointsCommunityGoal.Error.Code)
	}

	return nil
}

// GetUserPointsContribution fetches the user's points contribution data for a channel.
func (c *Client) GetUserPointsContribution(ctx context.Context, channelLogin string) ([]GoalContribution, error) {
	vars := map[string]any{"channelLogin": channelLogin}
	data, err := c.PostGQL(ctx, constants.GQLUserPointsContribution, vars)
	if err != nil {
		return nil, fmt.Errorf("GetUserPointsContribution: %w", err)
	}

	var resp struct {
		User *struct {
			Channel struct {
				Self struct {
					CommunityPoints struct {
						GoalContributions []GoalContribution `json:"goalContributions"`
					} `json:"communityPoints"`
				} `json:"self"`
			} `json:"channel"`
		} `json:"user"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetUserPointsContribution response: %w", err)
	}

	if resp.User == nil {
		return nil, nil
	}

	return resp.User.Channel.Self.CommunityPoints.GoalContributions, nil
}

// GoalContribution holds user contribution data for a community goal.
type GoalContribution struct {
	Goal struct {
		ID string `json:"id"`
	} `json:"goal"`
	UserPointsContributedThisStream int `json:"userPointsContributedThisStream"`
}

// GetBroadcastID fetches the broadcast ID for a channel.
// Returns empty string if the streamer is offline.
func (c *Client) GetBroadcastID(ctx context.Context, channelID string) (string, error) {
	vars := map[string]any{"id": channelID}
	data, err := c.PostGQL(ctx, constants.GQLWithIsStreamLiveQuery, vars)
	if err != nil {
		return "", fmt.Errorf("GetBroadcastID: %w", err)
	}

	var resp struct {
		User *struct {
			Stream *struct {
				ID string `json:"id"`
			} `json:"stream"`
		} `json:"user"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing GetBroadcastID response: %w", err)
	}

	if resp.User == nil || resp.User.Stream == nil {
		return "", nil
	}

	return resp.User.Stream.ID, nil
}

// CheckViewerIsMod checks if the authenticated user is a moderator for a channel.
func (c *Client) CheckViewerIsMod(ctx context.Context, channelLogin string) (bool, error) {
	vars := map[string]any{"channelLogin": channelLogin}
	data, err := c.PostGQL(ctx, constants.GQLModViewChannelQuery, vars)
	if err != nil {
		return false, fmt.Errorf("CheckViewerIsMod: %w", err)
	}

	var resp struct {
		User *struct {
			Self struct {
				IsModerator bool `json:"isModerator"`
			} `json:"self"`
		} `json:"user"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return false, fmt.Errorf("parsing CheckViewerIsMod response: %w", err)
	}

	if resp.User == nil {
		return false, nil
	}

	return resp.User.Self.IsModerator, nil
}

// GetDropCampaignDetailsBatch fetches details for multiple drop campaigns in batches.
func (c *Client) GetDropCampaignDetailsBatch(ctx context.Context, campaignIDs []string, userID string) ([]json.RawMessage, error) {
	const batchSize = 20
	var results []json.RawMessage

	for i := 0; i < len(campaignIDs); i += batchSize {
		end := i + batchSize
		if end > len(campaignIDs) {
			end = len(campaignIDs)
		}
		chunk := campaignIDs[i:end]

		ops := make([]constants.GQLOperation, len(chunk))
		varsList := make([]map[string]any, len(chunk))
		for j, id := range chunk {
			ops[j] = constants.GQLDropCampaignDetails
			varsList[j] = map[string]any{
				"dropID":       id,
				"channelLogin": userID,
			}
		}

		batchResults, err := c.PostGQLBatch(ctx, ops, varsList)
		if err != nil {
			c.log.Warn("Failed to fetch campaign details batch", "error", err)
			continue
		}

		for _, data := range batchResults {
			if data == nil {
				continue
			}
			var resp struct {
				User *struct {
					DropCampaign json.RawMessage `json:"dropCampaign"`
				} `json:"user"`
			}
			if err := json.Unmarshal(data, &resp); err == nil && resp.User != nil {
				results = append(results, resp.User.DropCampaign)
			}
		}

		if end < len(campaignIDs) {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	return results, nil
}

// GetGameSlug fetches the slug for a game by its ID using a raw GQL query.
// Returns empty string if the game is not found.
func (c *Client) GetGameSlug(ctx context.Context, gameID string) (string, error) {
	vars := map[string]any{"id": gameID}
	data, err := c.PostGQL(ctx, constants.GQLGameByID, vars)
	if err != nil {
		return "", fmt.Errorf("GetGameSlug for game ID %s: %w", gameID, err)
	}

	var resp struct {
		Game *struct {
			Slug string `json:"slug"`
		} `json:"game"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing GetGameSlug response: %w", err)
	}

	if resp.Game == nil {
		return "", nil
	}

	return resp.Game.Slug, nil
}
