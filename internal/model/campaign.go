package model

import (
	"fmt"
	"time"
)

// Campaign represents a Twitch drop campaign.
type Campaign struct {
	ID string `json:"id"`
	Game *GameInfo `json:"game,omitempty"`
	Name string `json:"name"`
	Status string `json:"status"`
	InInventory bool `json:"in_inventory"`
	EndAt time.Time `json:"end_at"`
	StartAt time.Time `json:"start_at"`
	DTMatch bool `json:"dt_match"`
	Drops []*Drop `json:"drops,omitempty"`
	Channels []string `json:"channels,omitempty"`
}

// NewCampaign creates a Campaign from raw API data.
func NewCampaign(id, name, status string, game *GameInfo, startAt, endAt time.Time, channels []string) *Campaign {
	now := time.Now()
	return &Campaign{
		ID:       id,
		Game:     game,
		Name:     name,
		Status:   status,
		StartAt:  startAt,
		EndAt:    endAt,
		DTMatch:  startAt.Before(now) && now.Before(endAt),
		Channels: channels,
		Drops:    make([]*Drop, 0),
	}
}

// ClearDrops removes drops that are outside the time window or already claimed.
func (c *Campaign) ClearDrops() {
	filtered := make([]*Drop, 0, len(c.Drops))
	for _, d := range c.Drops {
		if d.DTMatch && !d.IsClaimed {
			filtered = append(filtered, d)
		}
	}
	c.Drops = filtered
}

// Equal returns true if two campaigns have the same ID.
func (c *Campaign) Equal(other *Campaign) bool {
	if other == nil {
		return false
	}
	return c.ID == other.ID
}

// String returns a human-readable representation of the campaign.
func (c *Campaign) String() string {
	gameName := ""
	if c.Game != nil {
		gameName = c.Game.DisplayName
	}
	return fmt.Sprintf("Campaign(id=%s, name=%s, game=%s, in_inventory=%t)",
		c.ID, c.Name, gameName, c.InInventory)
}
