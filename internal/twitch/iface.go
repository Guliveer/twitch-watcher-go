package twitch

import (
	"context"

	"github.com/Guliveer/twitch-miner-go/internal/auth"
	"github.com/Guliveer/twitch-miner-go/internal/gql"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// API is the high-level Twitch API interface used by the miner.
// *Client satisfies this interface.
type API interface {
	Login(ctx context.Context) error
	CheckStreamerOnline(ctx context.Context, s *model.Streamer) error
	LoadChannelPointsContext(ctx context.Context, s *model.Streamer) error
	SendMinuteWatchedEvents(ctx context.Context, streamers []*model.Streamer) error
	MakePrediction(ctx context.Context, s *model.Streamer, ep *model.EventPrediction) error
	ClaimChannelPoints(ctx context.Context, s *model.Streamer, claimID string) error
	JoinRaid(ctx context.Context, raidID string) error
	ClaimMoment(ctx context.Context, momentID string) error
	SyncCampaigns(ctx context.Context, streamers []*model.Streamer) error
	ClaimAllDropsFromInventory(ctx context.Context) error
	GetChannelID(ctx context.Context, username string) (string, error)
	GetFollowers(ctx context.Context, limit int, order string) ([]string, error)
	CheckViewerIsMod(ctx context.Context, streamer *model.Streamer)
	RefreshSpadeURL(ctx context.Context, s *model.Streamer) error // re-fetch spade URL on demand
	GQLClient() *gql.Client      // expose GQL client for category watcher
	AuthProvider() auth.Provider  // expose auth provider for PubSub/chat
}
