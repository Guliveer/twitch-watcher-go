// Package constants defines all Twitch API endpoints, client identifiers,
// GQL operation hashes, user-agent strings, PubSub topic formats, and
// default timeout/interval values used throughout the miner.
package constants

import "time"

const (
	// TwitchURL is the base Twitch web URL.
	TwitchURL = "https://www.twitch.tv"
	// IRCURL is the Twitch IRC chat server hostname.
	IRCURL = "irc.chat.twitch.tv"
	// IRCPort is the plaintext IRC port.
	IRCPort = 6667
	// IRCPortTLS is the TLS-encrypted IRC port.
	IRCPortTLS = 6697
	// PubSubURL is the Twitch PubSub WebSocket endpoint.
	PubSubURL = "wss://pubsub-edge.twitch.tv/v1"
	// GQLURL is the Twitch GraphQL API endpoint.
	GQLURL = "https://gql.twitch.tv/gql"
	// IntegrityURL is the Twitch GQL integrity token endpoint.
	IntegrityURL = "https://gql.twitch.tv/integrity"
	// LoginURL is the Twitch passport login endpoint.
	LoginURL = "https://passport.twitch.tv/protected_login"
	// DeviceCodeURL is the Twitch OAuth2 device code endpoint.
	DeviceCodeURL = "https://id.twitch.tv/oauth2/device"
	// TokenURL is the Twitch OAuth2 token endpoint.
	TokenURL = "https://id.twitch.tv/oauth2/token"
)

// DeviceCodeScopes are the OAuth scopes requested during device code authorization.
const DeviceCodeScopes = "channel_read chat:read chat:edit user_read user:read:email"

const (
	// ClientID is the Twitch client ID (TV client).
	ClientID = "ue6666qo983tsx6so1t0vnawi233wa"
	// ClientIDBrowser is the Twitch client ID for browser clients.
	ClientIDBrowser = "kimne78kx3ncx6brgo4mv6wki5h1ko"
	// ClientIDMobile is the Twitch client ID for mobile browser clients.
	ClientIDMobile = "r8s4dac0uhzifbpu9sjdiwzctle17ff"
	// ClientIDAndroid is the Twitch client ID for the Android app.
	ClientIDAndroid = "kd1unb4b3q4t58fwlpcbzcbnm76a8fp"
	// ClientIDiOS is the Twitch client ID for the iOS app.
	ClientIDiOS = "851cqzxpb9bqu9z6galo155du"

	// ClientVersion is the Twitch client version string (browser).
	ClientVersion = "ef928475-9403-42f2-8a34-55784bd08e16"

	// DropID is the tag ID used to identify streams with Drops enabled.
	DropID = "c2542d6d-cd10-4532-919b-3d19f30a768b"
)

// UserAgents maps platform and browser/app to user-agent strings.
var UserAgents = map[string]map[string]string{
	"Windows": {
		"CHROME":  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36",
		"FIREFOX": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:84.0) Gecko/20100101 Firefox/84.0",
	},
	"Linux": {
		"CHROME":  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.96 Safari/537.36",
		"FIREFOX": "Mozilla/5.0 (X11; Linux x86_64; rv:85.0) Gecko/20100101 Firefox/85.0",
	},
	"Android": {
		"App": "Dalvik/2.1.0 (Linux; U; Android 7.1.2; SM-G977N Build/LMY48Z) tv.twitch.android.app/14.3.2/1403020",
		"TV":  "Mozilla/5.0 (Linux; Android 7.1; Smart Box C1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36",
	},
}

// DefaultUserAgent is the user-agent string used for API requests.
const DefaultUserAgent = "Mozilla/5.0 (Linux; Android 7.1; Smart Box C1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

const (
	// MaxTopicsPerConn is the maximum number of topics per PubSub WebSocket connection.
	MaxTopicsPerConn = 50
	// MaxPubSubConns is the maximum number of PubSub WebSocket connections.
	MaxPubSubConns = 10
	// MaxWatchStreams is the maximum number of streams to send minute-watched events for.
	MaxWatchStreams = 2
)

const (
	// TopicVideoPlayback is the PubSub topic for stream playback events.
	TopicVideoPlayback = "video-playback-by-id"
	// TopicCommunityPoints is the PubSub topic for channel points events.
	TopicCommunityPoints = "community-points-user-v1"
	// TopicPredictions is the PubSub topic for prediction events.
	TopicPredictions = "predictions-channel-v1"
	// TopicPredictionsUser is the PubSub topic for user prediction events.
	TopicPredictionsUser = "predictions-user-v1"
	// TopicRaid is the PubSub topic for raid events.
	TopicRaid = "raid"
	// TopicCommunityMoments is the PubSub topic for community moment events.
	TopicCommunityMoments = "community-moments-channel-v1"
	// TopicCommunityGoals is the PubSub topic for community goal events.
	TopicCommunityGoals = "community-points-channel-v1"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests.
	// Reduced from 30s — Twitch usually responds within 2-5s; retrying
	// sooner is more effective than waiting the full 30s.
	DefaultHTTPTimeout = 15 * time.Second
	// StartupHTTPTimeout is a reduced timeout for startup operations where
	// speed matters more than reliability (context will be refreshed later).
	StartupHTTPTimeout = 10 * time.Second
	// DefaultMaxRetries is the default number of retries for GQL requests.
	DefaultMaxRetries = 3
	// StartupMaxRetries is a reduced retry count for startup operations.
	StartupMaxRetries = 1
	// StartupWorkers is the number of concurrent workers for parallel startup operations.
	StartupWorkers = 5
	// DefaultPubSubPingInterval is the interval between PubSub PING messages.
	DefaultPubSubPingInterval = 4 * time.Minute
	// DefaultPubSubPongTimeout is the timeout waiting for a PONG response.
	DefaultPubSubPongTimeout = 10 * time.Second
	// DefaultMinuteWatchedInterval is the interval between minute-watched event sends.
	DefaultMinuteWatchedInterval = 20 * time.Second
	// DefaultCampaignSyncInterval is the interval between drop campaign syncs.
	DefaultCampaignSyncInterval = 30 * time.Minute
	// DefaultCategoryWatcherInterval is the default interval for category watcher polling.
	DefaultCategoryWatcherInterval = 120 * time.Second
	// DefaultStreamUpdateInterval is the interval for refreshing stream info.
	DefaultStreamUpdateInterval = 120 * time.Second
	// DefaultStreamUpDebounce is the debounce duration after a stream-up event.
	DefaultStreamUpDebounce = 120 * time.Second
	// DefaultGracefulShutdownTimeout is the timeout for graceful HTTP server shutdown.
	DefaultGracefulShutdownTimeout = 5 * time.Second
)

// GQLOperation represents a persisted GQL query with its operation name and SHA256 hash.
type GQLOperation struct {
	OperationName string
	SHA256Hash string
	Query string
}

// Persisted GQL operations migrated from Python constants.py.
var (
	GQLWithIsStreamLiveQuery = GQLOperation{
		OperationName: "WithIsStreamLiveQuery",
		SHA256Hash:    "04e46329a6786ff3a81c01c50bfa5d725902507a0deb83b0edbf7abe7a3716ea",
	}
	GQLPlaybackAccessToken = GQLOperation{
		OperationName: "PlaybackAccessToken",
		SHA256Hash:    "3093517e37e4f4cb48906155bcd894150aef92617939236d2508f3375ab732ce",
	}
	GQLVideoPlayerStreamInfoOverlayChannel = GQLOperation{
		OperationName: "VideoPlayerStreamInfoOverlayChannel",
		SHA256Hash:    "a5f2e34d626a9f4f5c0204f910bab2194948a9502089be558bb6e779a9e1b3d2",
	}
	GQLClaimCommunityPoints = GQLOperation{
		OperationName: "ClaimCommunityPoints",
		SHA256Hash:    "46aaeebe02c99afdf4fc97c7c0cba964124bf6b0af229395f1f6d1feed05b3d0",
	}
	GQLCommunityMomentCalloutClaim = GQLOperation{
		OperationName: "CommunityMomentCallout_Claim",
		SHA256Hash:    "e2d67415aead910f7f9ceb45a77b750a1e1d9622c936d832328a0689e054db62",
	}
	GQLDropsPageClaimDropRewards = GQLOperation{
		OperationName: "DropsPage_ClaimDropRewards",
		SHA256Hash:    "a455deea71bdc9015b78eb49f4acfbce8baa7ccbedd28e549bb025bd0f751930",
	}
	GQLChannelPointsContext = GQLOperation{
		OperationName: "ChannelPointsContext",
		SHA256Hash:    "1530a003a7d374b0380b79db0be0534f30ff46e61cffa2bc0e2468a909fbc024",
	}
	GQLJoinRaid = GQLOperation{
		OperationName: "JoinRaid",
		SHA256Hash:    "c6a332a86d1087fbbb1a8623aa01bd1313d2386e7c63be60fdb2d1901f01a4ae",
	}
	GQLModViewChannelQuery = GQLOperation{
		OperationName: "ModViewChannelQuery",
		SHA256Hash:    "df5d55b6401389afb12d3017c9b2cf1237164220c8ef4ed754eae8188068a807",
	}
	GQLInventory = GQLOperation{
		OperationName: "Inventory",
		SHA256Hash:    "d86775d0ef16a63a33ad52e80eaff963b2d5b72fada7c991504a57496e1d8e4b",
	}
	GQLMakePrediction = GQLOperation{
		OperationName: "MakePrediction",
		SHA256Hash:    "b44682ecc88358817009f20e69d75081b1e58825bb40aa53d5dbadcc17c881d8",
	}
	GQLViewerDropsDashboard = GQLOperation{
		OperationName: "ViewerDropsDashboard",
		SHA256Hash:    "5a4da2ab3d5b47c9f9ce864e727b2cb346af1e3ea8b897fe8f704a97ff017619",
	}
	GQLDropCampaignDetails = GQLOperation{
		OperationName: "DropCampaignDetails",
		SHA256Hash:    "f6396f5ffdde867a8f6f6da18286e4baf02e5b98d14689a69b5af320a4c7b7b8",
	}
	GQLDropsHighlightServiceAvailableDrops = GQLOperation{
		OperationName: "DropsHighlightService_AvailableDrops",
		SHA256Hash:    "9a62a09bce5b53e26e64a671e530bc599cb6aab1e5ba3cbd5d85966d3940716f",
	}
	GQLGetIDFromLogin = GQLOperation{
		OperationName: "GetIDFromLogin",
		SHA256Hash:    "94e82a7b1e3c21e186daa73ee2afc4b8f23bade1fbbff6fe8ac133f50a2f58ca",
	}
	GQLPersonalSections = GQLOperation{
		OperationName: "PersonalSections",
		SHA256Hash:    "9fbdfb00156f754c26bde81eb47436dee146655c92682328457037da1a48ed39",
	}
	GQLChannelFollows = GQLOperation{
		OperationName: "ChannelFollows",
		SHA256Hash:    "eecf815273d3d949e5cf0085cc5084cd8a1b5b7b6f7990cf43cb0beadf546907",
	}
	GQLUserPointsContribution = GQLOperation{
		OperationName: "UserPointsContribution",
		SHA256Hash:    "23ff2c2d60708379131178742327ead913b93b1bd6f665517a6d9085b73f661f",
	}
	GQLContributeCommunityPointsCommunityGoal = GQLOperation{
		OperationName: "ContributeCommunityPointsCommunityGoal",
		SHA256Hash:    "5774f0ea5d89587d73021a2e03c3c44777d903840c608754a1be519f51e37bb6",
	}
	GQLDirectoryPageGame = GQLOperation{
		OperationName: "DirectoryPage_Game",
		Query:         `query DirectoryPage_Game($slug: String!, $first: Int!, $after: Cursor, $options: GameStreamOptions) { game(slug: $slug) { displayName name streams(first: $first, after: $after, options: $options) { edges { node { broadcaster { id login displayName } viewersCount title game { id name displayName slug } } cursor } pageInfo { hasNextPage } } } }`,
	}
	GQLGameByID = GQLOperation{
		OperationName: "GameByID",
		Query:         `query GameByID($id: ID!) { game(id: $id) { slug } }`,
	}
)

// AllGQLOperations returns a slice of all defined GQL operations for iteration.
func AllGQLOperations() []GQLOperation {
	return []GQLOperation{
		GQLWithIsStreamLiveQuery,
		GQLPlaybackAccessToken,
		GQLVideoPlayerStreamInfoOverlayChannel,
		GQLClaimCommunityPoints,
		GQLCommunityMomentCalloutClaim,
		GQLDropsPageClaimDropRewards,
		GQLChannelPointsContext,
		GQLJoinRaid,
		GQLModViewChannelQuery,
		GQLInventory,
		GQLMakePrediction,
		GQLViewerDropsDashboard,
		GQLDropCampaignDetails,
		GQLDropsHighlightServiceAvailableDrops,
		GQLGetIDFromLogin,
		GQLPersonalSections,
		GQLChannelFollows,
		GQLUserPointsContribution,
		GQLContributeCommunityPointsCommunityGoal,
		GQLDirectoryPageGame,
		GQLGameByID,
	}
}
