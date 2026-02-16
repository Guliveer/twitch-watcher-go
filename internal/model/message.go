package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType represents the type of a PubSub message for notification routing.
type MessageType string

// Message types for PubSub events.
const (
	// Points-related messages
	MsgTypePointsEarned    MessageType = "points-earned"
	MsgTypePointsSpent     MessageType = "points-spent"
	MsgTypeClaimAvailable  MessageType = "claim-available"
	MsgTypeClaimClaimed    MessageType = "claim-claimed"

	// Prediction messages
	MsgTypePredictionEvent  MessageType = "event-created"
	MsgTypePredictionUpdate MessageType = "event-updated"
	MsgTypePredictionLocked MessageType = "event-locked"
	MsgTypePredictionResult MessageType = "event-end"

	// Stream messages
	MsgTypeStreamUp       MessageType = "stream-up"
	MsgTypeStreamDown     MessageType = "stream-down"
	MsgTypeViewCount      MessageType = "viewcount"

	// Raid messages
	MsgTypeRaidUpdate     MessageType = "raid_update_v2"
	MsgTypeRaidGo         MessageType = "raid_go_v2"
	MsgTypeRaidCancel     MessageType = "raid_cancel_v2"

	// Moment messages
	MsgTypeMomentAvailable MessageType = "active"

	// Community goal messages
	MsgTypeGoalContribution MessageType = "community-goal-contribution"
	MsgTypeGoalUpdated      MessageType = "community-goal-updated"
)

// Message represents a parsed PubSub message.
type Message struct {
	Topic string `json:"topic"`
	TopicUser string `json:"topic_user"`
	RawMessage map[string]any `json:"message"`
	Type MessageType `json:"type"`
	Data map[string]any `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	ChannelID string `json:"channel_id"`
	Identifier string `json:"identifier"`
}

// ParseMessage creates a Message from raw PubSub data.
func ParseMessage(topicFull string, rawMessageJSON []byte) (*Message, error) {
	topic, topicUser := splitTopic(topicFull)

	var msgBody map[string]any
	if err := json.Unmarshal(rawMessageJSON, &msgBody); err != nil {
		return nil, fmt.Errorf("failed to parse message body: %w", err)
	}

	msgType := ""
	if t, ok := msgBody["type"].(string); ok {
		msgType = t
	}

	var data map[string]any
	if d, ok := msgBody["data"].(map[string]any); ok {
		data = d
	}

	msg := &Message{
		Topic:      topic,
		TopicUser:  topicUser,
		RawMessage: msgBody,
		Type:       MessageType(msgType),
		Data:       data,
	}

	msg.Timestamp = msg.resolveTimestamp()
	msg.ChannelID = msg.resolveChannelID()
	msg.Identifier = fmt.Sprintf("%s.%s.%s", msg.Type, msg.Topic, msg.ChannelID)

	return msg, nil
}

func (m *Message) resolveTimestamp() time.Time {
	if m.Data == nil {
		return serverTime(m.RawMessage)
	}
	if ts, ok := m.Data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return t
		}
	}
	return serverTime(m.Data)
}

func (m *Message) resolveChannelID() string {
	if m.Data == nil {
		return m.TopicUser
	}

	if pred, ok := m.Data["prediction"].(map[string]any); ok {
		if cid, ok := pred["channel_id"].(string); ok {
			return cid
		}
	}
	if claim, ok := m.Data["claim"].(map[string]any); ok {
		if cid, ok := claim["channel_id"].(string); ok {
			return cid
		}
	}
	if cid, ok := m.Data["channel_id"].(string); ok {
		return cid
	}
	if balance, ok := m.Data["balance"].(map[string]any); ok {
		if cid, ok := balance["channel_id"].(string); ok {
			return cid
		}
	}

	return m.TopicUser
}

// String returns a string representation of the message.
func (m *Message) String() string {
	return fmt.Sprintf("Message(type=%s, topic=%s, channel_id=%s)", m.Type, m.Topic, m.ChannelID)
}

func splitTopic(topicFull string) (string, string) {
	for i := len(topicFull) - 1; i >= 0; i-- {
		if topicFull[i] == '.' {
			return topicFull[:i], topicFull[i+1:]
		}
	}
	return topicFull, ""
}

func serverTime(data map[string]any) time.Time {
	if data != nil {
		if st, ok := data["server_time"].(float64); ok {
			return time.Unix(int64(st), 0).UTC()
		}
	}
	return time.Now().UTC()
}
