package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/utils"
)

// Drop represents a single time-based drop within a campaign.
type Drop struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Benefit string `json:"benefit"`
	MinutesRequired int `json:"minutes_required"`

	HasPreconditionsMet *bool `json:"has_preconditions_met,omitempty"`
	CurrentMinutesWatched int `json:"current_minutes_watched"`
	DropInstanceID string `json:"drop_instance_id,omitempty"`
	IsClaimed bool `json:"is_claimed"`
	IsClaimable bool `json:"is_claimable"`
	IsPrintable bool `json:"is_printable"`
	PercentageProgress int `json:"percentage_progress"`

	EndAt time.Time `json:"end_at"`
	StartAt time.Time `json:"start_at"`
	IsWithinTimeWindow bool `json:"dt_match"`
}

// NewDrop creates a Drop from raw API data.
func NewDrop(id, name string, benefits []string, minutesRequired int, startAt, endAt time.Time) *Drop {
	now := time.Now()
	return &Drop{
		ID:              id,
		Name:            name,
		Benefit:         strings.Join(benefits, ", "),
		MinutesRequired: minutesRequired,
		StartAt:         startAt,
		EndAt:           endAt,
		IsWithinTimeWindow:         startAt.Before(now) && now.Before(endAt),
	}
}

// Update refreshes the drop's progress from inventory data.
func (d *Drop) Update(hasPreconditionsMet bool, currentMinutesWatched int, dropInstanceID string, isClaimed bool) {
	d.HasPreconditionsMet = &hasPreconditionsMet

	updatedPercentage := Percentage(currentMinutesWatched, d.MinutesRequired)
	quarter := (updatedPercentage/25)*25 == updatedPercentage

	d.IsPrintable = currentMinutesWatched > d.CurrentMinutesWatched &&
		((updatedPercentage > d.PercentageProgress && quarter && d.CurrentMinutesWatched != 0) ||
			(currentMinutesWatched == 1 && d.CurrentMinutesWatched == 0))

	d.CurrentMinutesWatched = currentMinutesWatched
	d.DropInstanceID = dropInstanceID
	d.IsClaimed = isClaimed
	d.IsClaimable = !d.IsClaimed && d.DropInstanceID != ""
	d.PercentageProgress = updatedPercentage
}

// ProgressBar returns a text-based progress bar for the drop.
func (d *Drop) ProgressBar() string {
	progress := d.PercentageProgress / 2
	remaining := (100 - d.PercentageProgress) / 2
	if remaining+progress < 50 {
		remaining += 50 - (remaining + progress)
	}
	bar := strings.Repeat("█", progress) + strings.Repeat(" ", remaining)
	return fmt.Sprintf("|%s|\t%d%% [%d/%d]", bar, d.PercentageProgress, d.CurrentMinutesWatched, d.MinutesRequired)
}

// Equal returns true if two drops have the same ID.
func (d *Drop) Equal(other *Drop) bool {
	if other == nil {
		return false
	}
	return d.ID == other.ID
}

// String returns a human-readable representation of the drop.
func (d *Drop) String() string {
	return fmt.Sprintf("Drop(id=%s, name=%s, benefit=%s, minutes_required=%d, progress=%d%%)",
		d.ID, d.Name, d.Benefit, d.MinutesRequired, d.PercentageProgress)
}

// Percentage calculates the integer percentage of a/b.
// Delegates to utils.Percentage for the canonical implementation.
func Percentage(a, b int) int {
	return utils.Percentage(a, b)
}
