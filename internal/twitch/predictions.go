package twitch

import (
	"context"
	"fmt"

	"github.com/Guliveer/twitch-miner-go/internal/auth"
	"github.com/Guliveer/twitch-miner-go/internal/model"
	"github.com/Guliveer/twitch-miner-go/internal/utils"
)

// MakePrediction calculates and places a prediction bet for an event.
// It delegates the bet calculation to model.EventPrediction and handles
// the GQL request to place the bet. Uses event.Mu for thread-safe access.
func (c *Client) MakePrediction(ctx context.Context, streamer *model.Streamer, event *model.EventPrediction) error {
	streamer.Mu.RLock()
	balance := streamer.ChannelPoints
	username := streamer.Username
	streamer.Mu.RUnlock()

	event.Mu.Lock()

	decision := event.Bet.Calculate(balance)

	title := event.Title
	status := event.Status
	eventID := event.EventID

	c.Log.Info("Completing bet",
		"streamer", username,
		"title", title,
		"event", string(model.EventBetGeneral))

	if status != "ACTIVE" {
		event.Mu.Unlock()
		c.Log.Info("Event is not active anymore",
			"streamer", username,
			"status", status,
			"event", string(model.EventBetFailed))
		return fmt.Errorf("event %s is not active (status: %s)", eventID, status)
	}

	skip, comparedValue := event.Bet.Skip()
	var filterCondStr string
	if event.Bet.Settings.FilterCondition != nil {
		filterCondStr = event.Bet.Settings.FilterCondition.String()
	}

	if skip {
		event.Mu.Unlock()
		c.Log.Info("Skip betting for event",
			"streamer", username,
			"title", title,
			"event", string(model.EventBetFilters))
		if filterCondStr != "" {
			c.Log.Info("Skip settings applied",
				"streamer", username,
				"filter", filterCondStr,
				"current_value", fmt.Sprintf("%.2f", comparedValue),
				"event", string(model.EventBetFilters))
		}
		return nil
	}

	if decision.Amount < 10 {
		event.Mu.Unlock()
		c.Log.Info("Bet amount below minimum",
			"streamer", username,
			"amount", utils.Millify(decision.Amount, 2),
			"minimum", 10,
			"event", string(model.EventBetGeneral))
		return nil
	}

	chosenOutcome := "unknown"
	if decision.Choice >= 0 && decision.Choice < len(event.Bet.Outcomes) {
		chosenOutcome = event.Bet.Outcomes[decision.Choice].Title
	}

	event.Mu.Unlock()

	c.Log.Info("Placing bet",
		"streamer", username,
		"amount", utils.Millify(decision.Amount, 2),
		"outcome", chosenOutcome,
		"event", string(model.EventBetGeneral))

	transactionID := auth.GenerateHex(16)

	err := c.GQL.MakePrediction(ctx, eventID, decision.OutcomeID, decision.Amount, transactionID)
	if err != nil {
		c.Log.Error("Failed to place bet",
			"streamer", username,
			"error", err,
			"event", string(model.EventBetFailed))
		return fmt.Errorf("placing prediction: %w", err)
	}

	event.Mu.Lock()
	event.BetPlaced = true
	event.Mu.Unlock()

	c.Log.Info("Prediction placed successfully",
		"streamer", username,
		"event_id", eventID,
		"outcome", chosenOutcome,
		"amount", decision.Amount)

	return nil
}
