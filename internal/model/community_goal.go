package model

import (
	"fmt"

	"github.com/Guliveer/twitch-miner-go/internal/jsonutil"
)

// CommunityGoal represents a channel community goal.
type CommunityGoal struct {
	GoalID string `json:"goal_id"`
	Title string `json:"title"`
	IsInStock bool `json:"is_in_stock"`
	PointsContributed int `json:"points_contributed"`
	AmountNeeded int `json:"amount_needed"`
	PerStreamUserMaxContribution int `json:"per_stream_user_maximum_contribution"`
	Status string `json:"status"`
}

// NewCommunityGoal creates a new CommunityGoal.
func NewCommunityGoal(goalID, title string, isInStock bool, pointsContributed, amountNeeded, perStreamMax int, status string) *CommunityGoal {
	return &CommunityGoal{
		GoalID:                       goalID,
		Title:                        title,
		IsInStock:                    isInStock,
		PointsContributed:            pointsContributed,
		AmountNeeded:                 amountNeeded,
		PerStreamUserMaxContribution: perStreamMax,
		Status:                       status,
	}
}

// CommunityGoalFromGQL creates a CommunityGoal from a GQL response map.
func CommunityGoalFromGQL(data map[string]any) *CommunityGoal {
	return &CommunityGoal{
		GoalID:                       jsonutil.StringFromMap(data, "id"),
		Title:                        jsonutil.StringFromMap(data, "title"),
		IsInStock:                    jsonutil.BoolFromMap(data, "isInStock"),
		PointsContributed:            jsonutil.IntFromMap(data, "pointsContributed"),
		AmountNeeded:                 jsonutil.IntFromMap(data, "amountNeeded"),
		PerStreamUserMaxContribution: jsonutil.IntFromMap(data, "perStreamUserMaximumContribution"),
		Status:                       jsonutil.StringFromMap(data, "status"),
	}
}

// CommunityGoalFromPubSub creates a CommunityGoal from a PubSub message map.
func CommunityGoalFromPubSub(data map[string]any) *CommunityGoal {
	return &CommunityGoal{
		GoalID:                       jsonutil.StringFromMap(data, "id"),
		Title:                        jsonutil.StringFromMap(data, "title"),
		IsInStock:                    jsonutil.BoolFromMap(data, "is_in_stock"),
		PointsContributed:            jsonutil.IntFromMap(data, "points_contributed"),
		AmountNeeded:                 jsonutil.IntFromMap(data, "goal_amount"),
		PerStreamUserMaxContribution: jsonutil.IntFromMap(data, "per_stream_maximum_user_contribution"),
		Status:                       jsonutil.StringFromMap(data, "status"),
	}
}

// AmountLeft returns the remaining points needed to complete the goal.
func (cg *CommunityGoal) AmountLeft() int {
	return cg.AmountNeeded - cg.PointsContributed
}

// Equal returns true if two community goals have the same ID.
func (cg *CommunityGoal) Equal(other *CommunityGoal) bool {
	if other == nil {
		return false
	}
	return cg.GoalID == other.GoalID
}

// String returns a human-readable representation of the community goal.
func (cg *CommunityGoal) String() string {
	return fmt.Sprintf("CommunityGoal(goal_id=%s, title=%s, is_in_stock=%t, points_contributed=%d, amount_needed=%d, status=%s)",
		cg.GoalID, cg.Title, cg.IsInStock, cg.PointsContributed, cg.AmountNeeded, cg.Status)
}

