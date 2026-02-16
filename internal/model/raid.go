package model

// Raid represents an active raid event on a channel.
type Raid struct {
	RaidID string `json:"raid_id"`
	TargetLogin string `json:"target_login"`
}

// NewRaid creates a new Raid.
func NewRaid(raidID, targetLogin string) *Raid {
	return &Raid{
		RaidID:      raidID,
		TargetLogin: targetLogin,
	}
}

// Equal returns true if two raids have the same ID.
func (r *Raid) Equal(other *Raid) bool {
	if other == nil {
		return false
	}
	return r.RaidID == other.RaidID
}
