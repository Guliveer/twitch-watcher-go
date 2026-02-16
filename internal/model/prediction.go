package model

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/utils"
)

// Strategy defines the prediction betting strategy.
type Strategy int

const (
	// StrategyMostVoted bets on the outcome with the most voters.
	StrategyMostVoted Strategy = iota
	// StrategyHighOdds bets on the outcome with the highest odds.
	StrategyHighOdds
	// StrategyPercentage bets on the outcome with the highest odds percentage.
	StrategyPercentage
	// StrategySmartMoney bets on the outcome with the highest top predictor points.
	StrategySmartMoney
	// StrategySmart uses a hybrid approach: high odds if close, most voted otherwise.
	StrategySmart
	// StrategyNumber1 always bets on outcome index 0.
	StrategyNumber1
	// StrategyNumber2 always bets on outcome index 1.
	StrategyNumber2
	// StrategyNumber3 always bets on outcome index 2.
	StrategyNumber3
	// StrategyNumber4 always bets on outcome index 3.
	StrategyNumber4
	// StrategyNumber5 always bets on outcome index 4.
	StrategyNumber5
	// StrategyNumber6 always bets on outcome index 5.
	StrategyNumber6
	// StrategyNumber7 always bets on outcome index 6.
	StrategyNumber7
	// StrategyNumber8 always bets on outcome index 7.
	StrategyNumber8
)

// String returns the string representation of a Strategy.
func (s Strategy) String() string {
	names := [...]string{
		"MOST_VOTED", "HIGH_ODDS", "PERCENTAGE", "SMART_MONEY", "SMART",
		"NUMBER_1", "NUMBER_2", "NUMBER_3", "NUMBER_4",
		"NUMBER_5", "NUMBER_6", "NUMBER_7", "NUMBER_8",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return "SMART"
}

// ParseStrategy converts a string to a Strategy value.
func ParseStrategy(s string) Strategy {
	switch s {
	case "MOST_VOTED":
		return StrategyMostVoted
	case "HIGH_ODDS":
		return StrategyHighOdds
	case "PERCENTAGE":
		return StrategyPercentage
	case "SMART_MONEY":
		return StrategySmartMoney
	case "SMART":
		return StrategySmart
	case "NUMBER_1":
		return StrategyNumber1
	case "NUMBER_2":
		return StrategyNumber2
	case "NUMBER_3":
		return StrategyNumber3
	case "NUMBER_4":
		return StrategyNumber4
	case "NUMBER_5":
		return StrategyNumber5
	case "NUMBER_6":
		return StrategyNumber6
	case "NUMBER_7":
		return StrategyNumber7
	case "NUMBER_8":
		return StrategyNumber8
	default:
		return StrategySmart
	}
}

// Condition defines a comparison operator for filter conditions.
type Condition int

const (
	// ConditionGT is the greater-than operator.
	ConditionGT Condition = iota
	// ConditionLT is the less-than operator.
	ConditionLT
	// ConditionGTE is the greater-than-or-equal operator.
	ConditionGTE
	// ConditionLTE is the less-than-or-equal operator.
	ConditionLTE
)

// String returns the string representation of a Condition.
func (c Condition) String() string {
	switch c {
	case ConditionGT:
		return "GT"
	case ConditionLT:
		return "LT"
	case ConditionGTE:
		return "GTE"
	case ConditionLTE:
		return "LTE"
	default:
		return "GT"
	}
}

// ParseCondition converts a string to a Condition value.
func ParseCondition(s string) Condition {
	switch s {
	case "GT":
		return ConditionGT
	case "LT":
		return ConditionLT
	case "GTE":
		return ConditionGTE
	case "LTE":
		return ConditionLTE
	default:
		return ConditionGT
	}
}

// OutcomeKey defines the keys used to access outcome statistics.
type OutcomeKey string

const (
	// OutcomeKeyPercentageUsers is the percentage of users who voted for this outcome.
	OutcomeKeyPercentageUsers OutcomeKey = "percentage_users"
	// OutcomeKeyOddsPercentage is the odds expressed as a percentage.
	OutcomeKeyOddsPercentage OutcomeKey = "odds_percentage"
	// OutcomeKeyOdds is the raw odds multiplier.
	OutcomeKeyOdds OutcomeKey = "odds"
	// OutcomeKeyTopPoints is the highest individual bet on this outcome.
	OutcomeKeyTopPoints OutcomeKey = "top_points"
	// OutcomeKeyTotalUsers is the total number of users who bet on this outcome.
	OutcomeKeyTotalUsers OutcomeKey = "total_users"
	// OutcomeKeyTotalPoints is the total points bet on this outcome.
	OutcomeKeyTotalPoints OutcomeKey = "total_points"
	// OutcomeKeyDecisionUsers is a virtual key for filter conditions.
	OutcomeKeyDecisionUsers OutcomeKey = "decision_users"
	// OutcomeKeyDecisionPoints is a virtual key for filter conditions.
	OutcomeKeyDecisionPoints OutcomeKey = "decision_points"
)

// DelayMode defines how the prediction delay is calculated.
type DelayMode int

const (
	// DelayModeFromStart delays from the start of the prediction window.
	DelayModeFromStart DelayMode = iota
	// DelayModeFromEnd delays from the end of the prediction window.
	DelayModeFromEnd
	// DelayModePercentage delays by a percentage of the prediction window.
	DelayModePercentage
)

// String returns the string representation of a DelayMode.
func (d DelayMode) String() string {
	switch d {
	case DelayModeFromStart:
		return "FROM_START"
	case DelayModeFromEnd:
		return "FROM_END"
	case DelayModePercentage:
		return "PERCENTAGE"
	default:
		return "FROM_END"
	}
}

// ParseDelayMode converts a string to a DelayMode value.
func ParseDelayMode(s string) DelayMode {
	switch s {
	case "FROM_START":
		return DelayModeFromStart
	case "FROM_END":
		return DelayModeFromEnd
	case "PERCENTAGE":
		return DelayModePercentage
	default:
		return DelayModeFromEnd
	}
}

// FilterCondition defines a condition for filtering predictions before betting.
type FilterCondition struct {
	By OutcomeKey `json:"by" yaml:"by"`
	Where Condition `json:"where" yaml:"where"`
	Value float64 `json:"value" yaml:"value"`
}

// String returns a human-readable representation of the filter condition.
func (fc *FilterCondition) String() string {
	return fmt.Sprintf("FilterCondition(by=%s, where=%s, value=%.2f)", fc.By, fc.Where, fc.Value)
}

// BetSettings holds configuration for automatic prediction betting.
type BetSettings struct {
	Strategy Strategy `json:"strategy" yaml:"strategy"`
	Percentage int `json:"percentage" yaml:"percentage"`
	PercentageGap int `json:"percentage_gap" yaml:"percentage_gap"`
	MaxPoints int `json:"max_points" yaml:"max_points"`
	MinimumPoints int `json:"minimum_points" yaml:"minimum_points"`
	StealthMode bool `json:"stealth_mode" yaml:"stealth_mode"`
	FilterCondition *FilterCondition `json:"filter_condition,omitempty" yaml:"filter_condition"`
	Delay float64 `json:"delay" yaml:"delay"`
	DelayMode DelayMode `json:"delay_mode" yaml:"delay_mode"`
}

// DefaultBetSettings returns BetSettings with default values.
func DefaultBetSettings() *BetSettings {
	return &BetSettings{
		Strategy:      StrategySmart,
		Percentage:    5,
		PercentageGap: 20,
		MaxPoints:     50000,
		MinimumPoints: 0,
		StealthMode:   false,
		Delay:         6,
		DelayMode:     DelayModeFromEnd,
	}
}

// String returns a human-readable representation of the bet settings.
func (bs *BetSettings) String() string {
	return fmt.Sprintf("BetSettings(strategy=%s, percentage=%d, percentage_gap=%d, max_points=%d, minimum_points=%d, stealth_mode=%t)",
		bs.Strategy, bs.Percentage, bs.PercentageGap, bs.MaxPoints, bs.MinimumPoints, bs.StealthMode)
}

// Outcome represents a single prediction outcome with computed statistics.
type Outcome struct {
	ID string `json:"id"`
	Title string `json:"title"`
	Color string `json:"color"`
	TotalUsers int `json:"total_users"`
	TotalPoints int `json:"total_points"`
	TopPoints int `json:"top_points"`
	PercentageUsers float64 `json:"percentage_users"`
	Odds float64 `json:"odds"`
	OddsPercentage float64 `json:"odds_percentage"`
}

// BetDecision holds the result of a bet calculation.
type BetDecision struct {
	Choice int `json:"choice"`
	Amount int `json:"amount"`
	OutcomeID string `json:"id"`
}

// Bet holds the state of a prediction bet calculation.
type Bet struct {
	Outcomes []Outcome `json:"outcomes"`
	Decision BetDecision `json:"decision"`
	TotalUsers int `json:"total_users"`
	TotalPoints int `json:"total_points"`
	Settings *BetSettings `json:"-"`
}

// NewBet creates a new Bet from a list of outcomes and settings.
func NewBet(outcomes []Outcome, settings *BetSettings) *Bet {
	return &Bet{
		Outcomes: outcomes,
		Decision: BetDecision{Choice: -1},
		Settings: settings,
	}
}

// UpdateOutcomes refreshes outcome statistics from new data.
func (b *Bet) UpdateOutcomes(updates []Outcome) {
	for i := range b.Outcomes {
		if i >= len(updates) {
			break
		}
		b.Outcomes[i].TotalUsers = updates[i].TotalUsers
		b.Outcomes[i].TotalPoints = updates[i].TotalPoints
		if updates[i].TopPoints > 0 {
			b.Outcomes[i].TopPoints = updates[i].TopPoints
		}
	}

	b.TotalPoints = 0
	b.TotalUsers = 0
	for _, o := range b.Outcomes {
		b.TotalUsers += o.TotalUsers
		b.TotalPoints += o.TotalPoints
	}

	if b.TotalUsers > 0 && b.TotalPoints > 0 {
		for i := range b.Outcomes {
			b.Outcomes[i].PercentageUsers = utils.FloatRound(
				(100.0*float64(b.Outcomes[i].TotalUsers))/float64(b.TotalUsers), 2,
			)
			if b.Outcomes[i].TotalPoints == 0 {
				b.Outcomes[i].Odds = 0
			} else {
				b.Outcomes[i].Odds = utils.FloatRound(
					float64(b.TotalPoints)/float64(b.Outcomes[i].TotalPoints), 2,
				)
			}
			if b.Outcomes[i].Odds == 0 {
				b.Outcomes[i].OddsPercentage = 0
			} else {
				b.Outcomes[i].OddsPercentage = utils.FloatRound(100.0/b.Outcomes[i].Odds, 2)
			}
		}
	}
}

func (b *Bet) outcomeValue(index int, key OutcomeKey) float64 {
	outcome := b.Outcomes[index]
	switch key {
	case OutcomeKeyTotalUsers:
		return float64(outcome.TotalUsers)
	case OutcomeKeyTotalPoints:
		return float64(outcome.TotalPoints)
	case OutcomeKeyPercentageUsers:
		return outcome.PercentageUsers
	case OutcomeKeyOdds:
		return outcome.Odds
	case OutcomeKeyOddsPercentage:
		return outcome.OddsPercentage
	case OutcomeKeyTopPoints:
		return float64(outcome.TopPoints)
	default:
		return 0
	}
}

func (b *Bet) returnChoice(key OutcomeKey) int {
	largest := 0
	for i := range b.Outcomes {
		if b.outcomeValue(i, key) > b.outcomeValue(largest, key) {
			largest = i
		}
	}
	return largest
}

func (b *Bet) returnNumberChoice(number int) int {
	if len(b.Outcomes) > number {
		return number
	}
	return 0
}

// Skip checks the filter condition and returns whether the bet should be skipped
// and the compared value.
func (b *Bet) Skip() (bool, float64) {
	if b.Settings.FilterCondition == nil {
		return false, 0
	}

	fc := b.Settings.FilterCondition
	key := fc.By
	condition := fc.Where
	value := fc.Value

	resolvedKey := key
	if key == OutcomeKeyDecisionUsers || key == OutcomeKeyDecisionPoints {
		if key == OutcomeKeyDecisionUsers {
			resolvedKey = OutcomeKeyTotalUsers
		} else {
			resolvedKey = OutcomeKeyTotalPoints
		}
	}

	var comparedValue float64
	if key == OutcomeKeyTotalUsers || key == OutcomeKeyTotalPoints {
		for i := range b.Outcomes {
			comparedValue += b.outcomeValue(i, resolvedKey)
		}
	} else {
		comparedValue = b.outcomeValue(b.Decision.Choice, resolvedKey)
	}

	switch condition {
	case ConditionGT:
		if comparedValue > value {
			return false, comparedValue
		}
	case ConditionLT:
		if comparedValue < value {
			return false, comparedValue
		}
	case ConditionGTE:
		if comparedValue >= value {
			return false, comparedValue
		}
	case ConditionLTE:
		if comparedValue <= value {
			return false, comparedValue
		}
	}

	return true, comparedValue
}

// Calculate determines which outcome to bet on and how much to bet.
func (b *Bet) Calculate(balance int) BetDecision {
	b.Decision = BetDecision{Choice: -1, Amount: 0, OutcomeID: ""}

	switch b.Settings.Strategy {
	case StrategyMostVoted:
		b.Decision.Choice = b.returnChoice(OutcomeKeyTotalUsers)
	case StrategyHighOdds:
		b.Decision.Choice = b.returnChoice(OutcomeKeyOdds)
	case StrategyPercentage:
		b.Decision.Choice = b.returnChoice(OutcomeKeyOddsPercentage)
	case StrategySmartMoney:
		b.Decision.Choice = b.returnChoice(OutcomeKeyTopPoints)
	case StrategyNumber1:
		b.Decision.Choice = b.returnNumberChoice(0)
	case StrategyNumber2:
		b.Decision.Choice = b.returnNumberChoice(1)
	case StrategyNumber3:
		b.Decision.Choice = b.returnNumberChoice(2)
	case StrategyNumber4:
		b.Decision.Choice = b.returnNumberChoice(3)
	case StrategyNumber5:
		b.Decision.Choice = b.returnNumberChoice(4)
	case StrategyNumber6:
		b.Decision.Choice = b.returnNumberChoice(5)
	case StrategyNumber7:
		b.Decision.Choice = b.returnNumberChoice(6)
	case StrategyNumber8:
		b.Decision.Choice = b.returnNumberChoice(7)
	case StrategySmart:
		if len(b.Outcomes) >= 2 {
			difference := math.Abs(b.Outcomes[0].PercentageUsers - b.Outcomes[1].PercentageUsers)
			if difference < float64(b.Settings.PercentageGap) {
				b.Decision.Choice = b.returnChoice(OutcomeKeyOdds)
			} else {
				b.Decision.Choice = b.returnChoice(OutcomeKeyTotalUsers)
			}
		}
	}

	if b.Decision.Choice >= 0 && b.Decision.Choice < len(b.Outcomes) {
		chosen := b.Outcomes[b.Decision.Choice]
		b.Decision.OutcomeID = chosen.ID

		amount := int(float64(balance) * float64(b.Settings.Percentage) / 100.0)
		if amount > b.Settings.MaxPoints {
			amount = b.Settings.MaxPoints
		}

		if b.Settings.StealthMode && amount >= chosen.TopPoints && chosen.TopPoints > 0 {
			stealthReduction := 1.0 + rand.Float64()*4.0
			amount = chosen.TopPoints - int(stealthReduction)
		}

		b.Decision.Amount = amount
	}

	return b.Decision
}

// String returns a human-readable representation of the bet.
func (b *Bet) String() string {
	return fmt.Sprintf("Bet(total_users=%d, total_points=%d, decision=%+v)",
		b.TotalUsers, b.TotalPoints, b.Decision)
}

// PredictionResult holds the result of a resolved prediction.
type PredictionResult struct {
	ResultString string `json:"string"`
	Type string `json:"type"`
	Gained int `json:"gained"`
}

// EventPrediction represents an active prediction event on a channel.
type EventPrediction struct {
	Mu sync.Mutex `json:"-"`

	Streamer *Streamer `json:"-"`
	EventID string `json:"event_id"`
	Title string `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	PredictionWindowSeconds float64 `json:"prediction_window_seconds"`
	Status string `json:"status"`
	Result PredictionResult `json:"result"`
	BoxFillable bool `json:"box_fillable"`
	BetConfirmed bool `json:"bet_confirmed"`
	BetPlaced bool `json:"bet_placed"`
	Bet *Bet `json:"bet"`
}

// NewEventPrediction creates a new EventPrediction.
func NewEventPrediction(
	streamer *Streamer,
	eventID, title string,
	createdAt time.Time,
	predictionWindowSeconds float64,
	status string,
	outcomes []Outcome,
) *EventPrediction {
	return &EventPrediction{
		Streamer:                streamer,
		EventID:                 eventID,
		Title:                   title,
		CreatedAt:               createdAt,
		PredictionWindowSeconds: predictionWindowSeconds,
		Status:                  status,
		Result:                  PredictionResult{},
		Bet:                     NewBet(outcomes, streamer.Settings.Bet),
	}
}

// Elapsed returns the seconds elapsed since the prediction was created.
func (ep *EventPrediction) Elapsed(timestamp time.Time) float64 {
	return utils.FloatRound(timestamp.Sub(ep.CreatedAt).Seconds(), 2)
}

// ClosingBetAfter returns the seconds remaining until the prediction window closes.
func (ep *EventPrediction) ClosingBetAfter(timestamp time.Time) float64 {
	return utils.FloatRound(ep.PredictionWindowSeconds-ep.Elapsed(timestamp), 2)
}

// ParseResult processes a prediction result and returns the points breakdown.
func (ep *EventPrediction) ParseResult(resultType string, pointsWon int) map[string]int {
	points := make(map[string]int)

	if resultType != "REFUND" {
		points["placed"] = ep.Bet.Decision.Amount
	}
	if pointsWon > 0 || resultType == "REFUND" {
		points["won"] = pointsWon
	}
	if resultType != "REFUND" {
		points["gained"] = points["won"] - points["placed"]
	}

	action := "Gained"
	if resultType == "LOSE" {
		action = "Lost"
	} else if resultType == "REFUND" {
		action = "Refunded"
	}

	prefix := ""
	if points["gained"] >= 0 {
		prefix = "+"
	}

	ep.Result = PredictionResult{
		ResultString: fmt.Sprintf("%s, %s: %s%d", resultType, action, prefix, points["gained"]),
		Type:         resultType,
		Gained:       points["gained"],
	}

	return points
}

// String returns a human-readable representation of the event prediction.
func (ep *EventPrediction) String() string {
	return fmt.Sprintf("EventPrediction(event_id=%s, title=%s, status=%s)",
		ep.EventID, ep.Title, ep.Status)
}

// GetPredictionWindow calculates the actual delay before placing a bet based on settings.
func GetPredictionWindow(settings *BetSettings, predictionWindowSeconds float64) float64 {
	switch settings.DelayMode {
	case DelayModeFromStart:
		return math.Min(settings.Delay, predictionWindowSeconds)
	case DelayModeFromEnd:
		return math.Max(predictionWindowSeconds-settings.Delay, 0)
	case DelayModePercentage:
		return predictionWindowSeconds * settings.Delay
	default:
		return predictionWindowSeconds
	}
}

