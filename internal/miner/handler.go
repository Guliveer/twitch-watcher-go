package miner

import (
	"context"
	"fmt"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/jsonutil"
	"github.com/Guliveer/twitch-miner-go/internal/model"
	"github.com/Guliveer/twitch-miner-go/internal/utils"
)

// handleMessage routes a PubSub message to the appropriate handler based on topic.
func (m *Miner) handleMessage(ctx context.Context, msg *model.Message) {
	if msg == nil {
		return
	}

	streamer := m.getStreamerByChannelID(msg.ChannelID)

	switch msg.Topic {
	case "community-points-user-v1":
		m.handleCommunityPoints(ctx, msg, streamer)
	case "video-playback-by-id":
		m.handleVideoPlayback(ctx, msg, streamer)
	case "predictions-channel-v1":
		m.handlePredictionsChannel(ctx, msg, streamer)
	case "predictions-user-v1":
		m.handlePredictionsUser(ctx, msg, streamer)
	case "raid":
		m.handleRaid(ctx, msg, streamer)
	case "community-moments-channel-v1":
		m.handleCommunityMoments(ctx, msg, streamer)
	case "community-points-channel-v1":
		m.handleCommunityGoals(ctx, msg, streamer)
	default:
		m.log.Debug("Unhandled PubSub topic", "topic", msg.Topic, "type", string(msg.Type))
	}

	// Allow GC to collect the raw JSON map now that all data has been extracted.
	msg.RawMessage = nil
}


func (m *Miner) handleCommunityPoints(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if msg.Data == nil {
		return
	}

	switch msg.Type {
	case model.MsgTypePointsEarned, model.MsgTypePointsSpent:
		m.handlePointsEarnedOrSpent(ctx, msg, streamer)
	case model.MsgTypeClaimAvailable:
		m.handleClaimAvailable(ctx, msg, streamer)
	default:
		m.log.Debug("Unhandled community-points message type", "type", string(msg.Type))
	}
}

func (m *Miner) handlePointsEarnedOrSpent(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	balance := extractNestedInt(msg.Data, "balance", "balance")
	if streamer != nil && balance > 0 {
		streamer.Mu.Lock()
		streamer.ChannelPoints = balance
		streamer.Mu.Unlock()
	}

	if msg.Type == model.MsgTypePointsEarned {
		pointGain, _ := msg.Data["point_gain"].(map[string]any)
		if pointGain == nil {
			return
		}

		earned := jsonutil.IntFromAny(pointGain["total_points"])
		reasonCode, _ := pointGain["reason_code"].(string)

		if streamer != nil {
			streamer.Mu.Lock()
			streamer.UpdateHistory(reasonCode, earned, 1)
			streamer.Mu.Unlock()

			streamer.Mu.RLock()
			username := streamer.Username
			currentBalance := streamer.ChannelPoints
			streamer.Mu.RUnlock()

			event := mapReasonToEvent(reasonCode)
			m.log.Event(ctx, event,
				fmt.Sprintf("+%s points", utils.Millify(earned, 2)),
				"streamer", username,
				"reason", reasonCode,
				"balance", currentBalance)
		}
	}
}

func (m *Miner) handleClaimAvailable(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if streamer == nil {
		return
	}

	claim, _ := msg.Data["claim"].(map[string]any)
	if claim == nil {
		return
	}

	claimID, _ := claim["id"].(string)
	if claimID == "" {
		return
	}

	streamer.Mu.RLock()
	username := streamer.Username
	streamer.Mu.RUnlock()

	m.log.Event(ctx, model.EventBonusClaim,
		"Claiming bonus",
		"streamer", username,
		"claim_id", claimID)

	if err := m.twitch.ClaimChannelPoints(ctx, streamer, claimID); err != nil {
		m.log.Warn("Failed to claim bonus",
			"streamer", username, "error", err)
	}
}


func (m *Miner) handleVideoPlayback(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if streamer == nil {
		return
	}

	switch msg.Type {
	case model.MsgTypeStreamUp:
		m.handleStreamUp(ctx, streamer)
	case model.MsgTypeStreamDown:
		m.handleStreamDown(ctx, streamer)
	case model.MsgTypeViewCount:
		m.handleViewCount(ctx, msg, streamer)
	}
}

func (m *Miner) handleStreamUp(ctx context.Context, streamer *model.Streamer) {
	streamer.Mu.Lock()
	streamer.StreamUpAt = time.Now()
	username := streamer.Username
	category := streamer.ResolveCategory()
	streamer.Mu.Unlock()

	m.log.Event(ctx, model.EventStreamerOnline,
		"Stream online",
		"streamer", username,
		"category", category)

	m.updateChatPresence(streamer, true)
}

func (m *Miner) handleStreamDown(ctx context.Context, streamer *model.Streamer) {
	streamer.Mu.Lock()
	wasOnline := streamer.IsOnline
	streamer.SetOffline()
	username := streamer.Username
	streamer.Mu.Unlock()

	if wasOnline {
		m.log.Event(ctx, model.EventStreamerOffline,
			"Stream went offline",
			"streamer", username)
	}

	m.updateChatPresence(streamer, false)
}

func (m *Miner) handleViewCount(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if viewers, ok := msg.RawMessage["viewers"].(float64); ok {
		streamer.Mu.Lock()
		streamer.Stream.ViewersCount = int(viewers)
		streamer.Mu.Unlock()
	}

	streamer.Mu.RLock()
	elapsed := streamer.StreamUpElapsed()
	streamer.Mu.RUnlock()

	if elapsed {
		if err := m.twitch.CheckStreamerOnline(ctx, streamer); err != nil {
			m.log.Debug("Failed to check online status on viewcount",
				"streamer", streamer.Username, "error", err)
		}
	}
}


func (m *Miner) handleRaid(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if streamer == nil {
		return
	}

	if msg.Type != model.MsgTypeRaidUpdate {
		return
	}

	streamer.Mu.RLock()
	followRaid := streamer.Settings != nil && streamer.Settings.FollowRaid
	username := streamer.Username
	streamer.Mu.RUnlock()

	if !followRaid {
		return
	}

	raidData, _ := msg.RawMessage["raid"].(map[string]any)
	if raidData == nil {
		return
	}

	raidID, _ := raidData["id"].(string)
	targetLogin, _ := raidData["target_login"].(string)

	if raidID == "" {
		return
	}

	m.log.Event(ctx, model.EventJoinRaid,
		"Joining raid",
		"streamer", username,
		"target", targetLogin)

	streamer.Mu.Lock()
	streamer.Raid = &model.Raid{
		RaidID:      raidID,
		TargetLogin: targetLogin,
	}
	streamer.Mu.Unlock()

	if err := m.twitch.JoinRaid(ctx, raidID); err != nil {
		m.log.Warn("Failed to join raid",
			"streamer", username, "raid_id", raidID, "error", err)
	}
}


func (m *Miner) handleCommunityMoments(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if streamer == nil || msg.Data == nil {
		return
	}

	if msg.Type != model.MsgTypeMomentAvailable {
		return
	}

	streamer.Mu.RLock()
	claimMoments := streamer.Settings != nil && streamer.Settings.ClaimMoments
	username := streamer.Username
	streamer.Mu.RUnlock()

	if !claimMoments {
		return
	}

	momentID, _ := msg.Data["moment_id"].(string)
	if momentID == "" {
		return
	}

	m.log.Event(ctx, model.EventMomentClaim,
		"Claiming moment",
		"streamer", username,
		"moment_id", momentID)

	if err := m.twitch.ClaimMoment(ctx, momentID); err != nil {
		m.log.Warn("Failed to claim moment",
			"streamer", username, "moment_id", momentID, "error", err)
	}
}


func (m *Miner) handleCommunityGoals(_ context.Context, msg *model.Message, streamer *model.Streamer) {
	if streamer == nil || msg.Data == nil {
		return
	}

	streamer.Mu.RLock()
	goalsEnabled := streamer.Settings != nil && streamer.Settings.CommunityGoalsEnabled
	streamer.Mu.RUnlock()

	if !goalsEnabled {
		return
	}

	switch msg.Type {
	case "community-goal-created", model.MsgTypeGoalUpdated:
		goalData, _ := msg.Data["community_goal"].(map[string]any)
		if goalData == nil {
			return
		}

		goal := parseCommunityGoal(goalData)
		if goal != nil {
			streamer.Mu.Lock()
			streamer.UpdateCommunityGoal(goal)
			streamer.Mu.Unlock()
		}

	case "community-goal-deleted":
		goalData, _ := msg.Data["community_goal"].(map[string]any)
		if goalData == nil {
			return
		}
		goalID, _ := goalData["id"].(string)
		if goalID != "" {
			streamer.Mu.Lock()
			streamer.DeleteCommunityGoal(goalID)
			streamer.Mu.Unlock()
		}
	}
}

func (m *Miner) updateChatPresence(streamer *model.Streamer, isOnline bool) {
	streamer.Mu.RLock()
	chatPresence := model.ChatNever
	if streamer.Settings != nil {
		chatPresence = streamer.Settings.Chat
	}
	username := streamer.Username
	streamer.Mu.RUnlock()

	if model.ShouldJoinChat(chatPresence, isOnline) {
		if err := m.chat.Join(username); err != nil {
			m.log.Debug("Failed to join chat", "streamer", username, "error", err)
		}
	} else {
		if m.chat.IsJoined(username) {
			if err := m.chat.Leave(username); err != nil {
				m.log.Debug("Failed to leave chat", "streamer", username, "error", err)
			}
		}
	}
}

func mapReasonToEvent(reasonCode string) model.Event {
	switch reasonCode {
	case "WATCH", "WATCH_CONSECUTIVE_GAMES":
		return model.EventGainForWatch
	case "CLAIM":
		return model.EventGainForClaim
	case "RAID":
		return model.EventGainForRaid
	case "WATCH_STREAK":
		return model.EventGainForWatchStreak
	default:
		return model.EventGainForWatch
	}
}

func parseOutcomes(raw any) []model.Outcome {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}

	outcomes := make([]model.Outcome, 0, len(arr))
	for _, item := range arr {
		outcomeMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		outcome := model.Outcome{
			ID:          jsonutil.StringFromAny(outcomeMap["id"]),
			Title:       jsonutil.StringFromAny(outcomeMap["title"]),
			Color:       jsonutil.StringFromAny(outcomeMap["color"]),
			TotalUsers:  jsonutil.IntFromAny(outcomeMap["total_users"]),
			TotalPoints: jsonutil.IntFromAny(outcomeMap["total_points"]),
			TopPoints:   jsonutil.IntFromAny(outcomeMap["top_points"]),
		}

		if outcome.TopPoints == 0 {
			if predictors, ok := outcomeMap["top_predictors"].([]any); ok && len(predictors) > 0 {
				if topPredictor, ok := predictors[0].(map[string]any); ok {
					outcome.TopPoints = jsonutil.IntFromAny(topPredictor["points"])
				}
			}
		}

		outcomes = append(outcomes, outcome)
	}

	return outcomes
}

// parseCommunityGoal parses a community goal from raw PubSub data.
func parseCommunityGoal(data map[string]any) *model.CommunityGoal {
	goalID, _ := data["id"].(string)
	if goalID == "" {
		return nil
	}
	return model.CommunityGoalFromPubSub(data)
}

func extractNestedInt(data map[string]any, keys ...string) int {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			return jsonutil.IntFromAny(current[key])
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			return 0
		}
		current = next
	}
	return 0
}
