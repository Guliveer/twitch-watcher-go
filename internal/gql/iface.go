package gql

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Guliveer/twitch-miner-go/internal/constants"
)

// Operations is the interface for all GQL query/mutation methods.
// *Client satisfies this interface.
type Operations interface {
	PostGQL(ctx context.Context, op constants.GQLOperation, vars map[string]interface{}) (json.RawMessage, error)
	PostGQLBatch(ctx context.Context, ops []constants.GQLOperation, varsList []map[string]interface{}) ([]json.RawMessage, error)
	HTTPClient() *http.Client
	SetStartupMode()
	SetNormalMode()

	GetUserID(ctx context.Context, login string) (string, error)
	GetStreamInfo(ctx context.Context, channelLogin string) (*StreamInfoResponse, error)
	GetChannelPointsContext(ctx context.Context, channelLogin string) (*ChannelPointsContext, error)
	ClaimCommunityPoints(ctx context.Context, claimID, channelID string) error
	GetFollowedStreamers(ctx context.Context, limit int, order string) ([]string, error)
	MakePrediction(ctx context.Context, eventID, outcomeID string, points int, transactionID string) error
	GetAvailableCampaigns(ctx context.Context, channelID string) ([]string, error)
	GetDropsDashboard(ctx context.Context, status string) ([]json.RawMessage, error)
	GetDropsInventory(ctx context.Context) (json.RawMessage, error)
	GetDropCampaignDetails(ctx context.Context, dropID, channelLogin string) (json.RawMessage, error)
	GetDropCampaignDetailsBatch(ctx context.Context, campaignIDs []string, userID string) ([]json.RawMessage, error)
	ClaimDropRewards(ctx context.Context, dropInstanceID string) (bool, error)
	JoinRaid(ctx context.Context, raidID string) error
	GetPlaybackAccessToken(ctx context.Context, login string) (*PlaybackAccessToken, error)
	ClaimCommunityMoment(ctx context.Context, momentID string) error
	GetTopStreamsByCategory(ctx context.Context, categorySlug string, limit int, dropsOnly bool) ([]TopStream, error)
	ContributeToCommunityGoal(ctx context.Context, goalID, channelID string, points int, transactionID string) error
	GetUserPointsContribution(ctx context.Context, channelLogin string) ([]GoalContribution, error)
	GetBroadcastID(ctx context.Context, channelID string) (string, error)
	CheckViewerIsMod(ctx context.Context, channelLogin string) (bool, error)
	GetGameSlug(ctx context.Context, gameID string) (string, error)
}
