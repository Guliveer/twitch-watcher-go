package miner

import (
	"context"
	"fmt"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/jsonutil"
	"github.com/Guliveer/twitch-miner-go/internal/model"
)


func (m *Miner) handlePredictionsChannel(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if streamer == nil || msg.Data == nil {
		return
	}

	eventDict, _ := msg.Data["event"].(map[string]any)
	if eventDict == nil {
		return
	}

	eventID, _ := eventDict["id"].(string)
	eventStatus, _ := eventDict["status"].(string)

	switch msg.Type {
	case model.MsgTypePredictionEvent:
		m.handlePredictionCreated(ctx, streamer, eventDict, eventID, eventStatus, msg)
	case model.MsgTypePredictionUpdate:
		m.handlePredictionUpdated(eventDict, eventID, eventStatus)
	case model.MsgTypePredictionLocked:
		m.handlePredictionLocked(eventID, eventStatus)
	}
}

func (m *Miner) handlePredictionCreated(
	ctx context.Context,
	streamer *model.Streamer,
	eventDict map[string]any,
	eventID, eventStatus string,
	msg *model.Message,
) {
	m.eventsPredictionsMu.RLock()
	_, exists := m.eventsPredictions[eventID]
	m.eventsPredictionsMu.RUnlock()
	if exists {
		return
	}

	if eventStatus != "ACTIVE" {
		return
	}

	streamer.Mu.RLock()
	isOnline := streamer.IsOnline
	makePredictions := streamer.Settings != nil && streamer.Settings.MakePredictions
	balance := streamer.ChannelPoints
	betSettings := streamer.Settings.Bet
	username := streamer.Username
	streamer.Mu.RUnlock()

	if !makePredictions || !isOnline {
		return
	}

	predictionWindowSeconds := jsonutil.FloatFromAny(eventDict["prediction_window_seconds"])

	actualWindow := model.GetPredictionWindow(betSettings, predictionWindowSeconds)

	outcomes := parseOutcomes(eventDict["outcomes"])

	createdAtStr, _ := eventDict["created_at"].(string)
	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		createdAt = time.Now()
	}

	event := model.NewEventPrediction(
		streamer,
		eventID,
		jsonutil.StringFromAny(eventDict["title"]),
		createdAt,
		actualWindow,
		eventStatus,
		outcomes,
	)

	secondsUntilClose := event.ClosingBetAfter(msg.Timestamp)
	if secondsUntilClose <= 0 {
		m.log.Debug("Prediction window already closed",
			"streamer", username, "event_id", eventID)
		return
	}

	if betSettings.MinimumPoints > 0 && balance < betSettings.MinimumPoints {
		m.log.Event(ctx, model.EventBetFilters,
			"Insufficient points for bet",
			"streamer", username,
			"balance", balance,
			"minimum", betSettings.MinimumPoints)
		return
	}

	m.eventsPredictionsMu.Lock()
	m.eventsPredictions[eventID] = event
	m.eventsPredictionsMu.Unlock()

	m.log.Event(ctx, model.EventBetStart,
		fmt.Sprintf("Placing bet in %.0fs", secondsUntilClose),
		"streamer", username,
		"title", event.Title)

	betTimer := time.AfterFunc(time.Duration(secondsUntilClose)*time.Second, func() {
		m.pendingTimersMu.Lock()
		delete(m.pendingTimers, eventID)
		m.pendingTimersMu.Unlock()

		m.eventsPredictionsMu.RLock()
		prediction, ok := m.eventsPredictions[eventID]
		m.eventsPredictionsMu.RUnlock()
		if !ok {
			return
		}

		if err := m.twitch.MakePrediction(ctx, streamer, prediction); err != nil {
			m.log.Warn("Failed to place prediction",
				"streamer", username, "event_id", eventID, "error", err)
		}
	})

	m.pendingTimersMu.Lock()
	m.pendingTimers[eventID] = betTimer
	m.pendingTimersMu.Unlock()
}

func (m *Miner) handlePredictionUpdated(eventDict map[string]any, eventID, eventStatus string) {
	m.eventsPredictionsMu.RLock()
	event, ok := m.eventsPredictions[eventID]
	m.eventsPredictionsMu.RUnlock()
	if !ok {
		return
	}

	event.Mu.Lock()
	defer event.Mu.Unlock()

	event.Status = eventStatus

	if !event.BetPlaced && event.Bet.Decision.Choice == -1 {
		outcomes := parseOutcomes(eventDict["outcomes"])
		event.Bet.UpdateOutcomes(outcomes)
	}
}

func (m *Miner) handlePredictionLocked(eventID, eventStatus string) {
	m.eventsPredictionsMu.RLock()
	event, ok := m.eventsPredictions[eventID]
	m.eventsPredictionsMu.RUnlock()
	if !ok {
		return
	}

	event.Mu.Lock()
	event.Status = eventStatus
	event.Mu.Unlock()

	m.log.Debug("Prediction locked", "event_id", eventID)
}


func (m *Miner) handlePredictionsUser(ctx context.Context, msg *model.Message, streamer *model.Streamer) {
	if msg.Data == nil {
		return
	}

	prediction, _ := msg.Data["prediction"].(map[string]any)
	if prediction == nil {
		return
	}

	eventID, _ := prediction["event_id"].(string)

	m.eventsPredictionsMu.RLock()
	event, ok := m.eventsPredictions[eventID]
	m.eventsPredictionsMu.RUnlock()
	if !ok {
		return
	}

	switch msg.Type {
	case "prediction-result":
		m.handlePredictionResult(ctx, event, prediction, streamer)
	case "prediction-made":
		m.handlePredictionMade(event)
	}
}

func (m *Miner) handlePredictionResult(ctx context.Context, event *model.EventPrediction, prediction map[string]any, streamer *model.Streamer) {
	event.Mu.Lock()
	betConfirmed := event.BetConfirmed
	event.Mu.Unlock()

	if !betConfirmed {
		return
	}

	result, _ := prediction["result"].(map[string]any)
	if result == nil {
		return
	}

	resultType, _ := result["type"].(string)
	pointsWon := jsonutil.IntFromAny(result["points_won"])

	event.Mu.Lock()
	points := event.ParseResult(resultType, pointsWon)

	var notifyEvent model.Event
	switch resultType {
	case "WIN":
		notifyEvent = model.EventBetWin
	case "LOSE":
		notifyEvent = model.EventBetLose
	case "REFUND":
		notifyEvent = model.EventBetRefund
	default:
		notifyEvent = model.EventBetGeneral
	}

	choiceStr := "unknown"
	if event.Bet.Decision.Choice >= 0 && event.Bet.Decision.Choice < len(event.Bet.Outcomes) {
		chosen := event.Bet.Outcomes[event.Bet.Decision.Choice]
		choiceStr = fmt.Sprintf("%s (%s)", chosen.Title, chosen.Color)
	}
	eventTitle := event.Title
	resultString := event.Result.ResultString
	event.Mu.Unlock()

	m.pendingTimersMu.Lock()
	if t, ok := m.pendingTimers[event.EventID]; ok {
		t.Stop()
		delete(m.pendingTimers, event.EventID)
	}
	m.pendingTimersMu.Unlock()

	streamerName := ""
	if streamer != nil {
		streamer.Mu.RLock()
		streamerName = streamer.Username
		streamer.Mu.RUnlock()
	}

	m.log.Event(ctx, notifyEvent,
		"Prediction result",
		"streamer", streamerName,
		"title", eventTitle,
		"choice", choiceStr,
		"result", resultString)

	if streamer != nil {
		streamer.Mu.Lock()
		streamer.UpdateHistory("PREDICTION", points["gained"], 1)

		if resultType == "REFUND" {
			streamer.UpdateHistory("REFUND", -points["placed"], -1)
		} else if resultType == "WIN" {
			streamer.UpdateHistory("PREDICTION", -points["won"], -1)
		}
		streamer.Mu.Unlock()
	}

	// Clean up resolved prediction to prevent unbounded map growth (OOM fix).
	m.eventsPredictionsMu.Lock()
	delete(m.eventsPredictions, event.EventID)
	m.eventsPredictionsMu.Unlock()
}

func (m *Miner) handlePredictionMade(event *model.EventPrediction) {
	event.Mu.Lock()
	event.BetConfirmed = true
	event.Mu.Unlock()
	m.log.Debug("Prediction confirmed", "event_id", event.EventID)
}
