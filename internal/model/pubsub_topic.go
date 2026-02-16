package model

import (
	"fmt"
	"log/slog"
)

// PubSubTopicType identifies the category of a PubSub topic.
type PubSubTopicType int

const (
	// PubSubTopicVideoPlayback tracks stream up/down and viewer count.
	PubSubTopicVideoPlayback PubSubTopicType = iota
	// PubSubTopicCommunityPoints tracks channel points events.
	PubSubTopicCommunityPoints
	// PubSubTopicPredictions tracks prediction events on a channel.
	PubSubTopicPredictions
	// PubSubTopicPredictionsUser tracks the user's own prediction events.
	PubSubTopicPredictionsUser
	// PubSubTopicRaid tracks raid events.
	PubSubTopicRaid
	// PubSubTopicCommunityMoments tracks community moment events.
	PubSubTopicCommunityMoments
	// PubSubTopicCommunityGoals tracks community goal events.
	PubSubTopicCommunityGoals
)

var topicNames = map[PubSubTopicType]string{
	PubSubTopicVideoPlayback:    "video-playback-by-id",
	PubSubTopicCommunityPoints:  "community-points-user-v1",
	PubSubTopicPredictions:      "predictions-channel-v1",
	PubSubTopicPredictionsUser:  "predictions-user-v1",
	PubSubTopicRaid:             "raid",
	PubSubTopicCommunityMoments: "community-moments-channel-v1",
	PubSubTopicCommunityGoals:   "community-points-channel-v1",
}

// String returns the Twitch topic string prefix for this topic type.
func (t PubSubTopicType) String() string {
	if name, ok := topicNames[t]; ok {
		return name
	}
	return "unknown"
}

// PubSubTopic represents a PubSub subscription topic.
type PubSubTopic struct {
	TopicType PubSubTopicType `json:"topic_type"`
	UserID string `json:"user_id,omitempty"`
	Streamer *Streamer `json:"-"`
}

// NewUserTopic creates a PubSubTopic scoped to the authenticated user.
func NewUserTopic(topicType PubSubTopicType, userID string) *PubSubTopic {
	return &PubSubTopic{
		TopicType: topicType,
		UserID:    userID,
	}
}

// NewStreamerTopic creates a PubSubTopic scoped to a specific streamer's channel.
func NewStreamerTopic(topicType PubSubTopicType, streamer *Streamer) *PubSubTopic {
	return &PubSubTopic{
		TopicType: topicType,
		Streamer:  streamer,
	}
}

// IsUserTopic returns true if this topic is scoped to the user (not a streamer).
func (pt *PubSubTopic) IsUserTopic() bool {
	return pt.Streamer == nil
}

// String returns the full topic string in the format "topic_name.id".
func (pt *PubSubTopic) String() string {
	if pt.IsUserTopic() {
		return fmt.Sprintf("%s.%s", pt.TopicType, pt.UserID)
	}
	if pt.Streamer.ChannelID == "" {
		slog.Warn("PubSubTopic constructed with empty channel_id",
			"streamer", pt.Streamer.Username,
			"topic", pt.TopicType.String(),
		)
	}
	return fmt.Sprintf("%s.%s", pt.TopicType, pt.Streamer.ChannelID)
}
