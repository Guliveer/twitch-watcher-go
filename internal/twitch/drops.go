package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/Guliveer/twitch-miner-go/internal/model"
)

// SyncCampaigns synchronizes drop campaigns with the user's inventory
// and updates streamer campaign data. This is called periodically by the miner.
func (c *Client) SyncCampaigns(ctx context.Context, streamers []*model.Streamer) error {
	if err := c.ClaimAllDropsFromInventory(ctx); err != nil {
		c.Log.Warn("Failed to claim drops from inventory", "error", err)
	}

	dashboardCampaigns, err := c.GQL.GetDropsDashboard(ctx, "ACTIVE")
	if err != nil {
		return fmt.Errorf("getting drops dashboard: %w", err)
	}

	if len(dashboardCampaigns) == 0 {
		return nil
	}

	campaignIDs := make([]string, 0, len(dashboardCampaigns))
	for _, raw := range dashboardCampaigns {
		var campaignID struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &campaignID); err == nil && campaignID.ID != "" {
			campaignIDs = append(campaignIDs, campaignID.ID)
		}
	}

	campaignDetails, err := c.GQL.GetDropCampaignDetailsBatch(ctx, campaignIDs, c.Auth.UserID())
	if err != nil {
		return fmt.Errorf("getting campaign details: %w", err)
	}

	campaigns := make([]*model.Campaign, 0, len(campaignDetails))
	for _, raw := range campaignDetails {
		if raw == nil {
			continue
		}
		campaign, err := parseCampaign(raw)
		if err != nil {
			c.Log.Debug("Failed to parse campaign", "error", err)
			continue
		}
		if campaign.IsWithinTimeWindow {
			campaign.ClearDrops()
			if len(campaign.Drops) > 0 {
				campaigns = append(campaigns, campaign)
			}
		}
	}

	campaigns, err = c.syncCampaignsWithInventory(ctx, campaigns)
	if err != nil {
		c.Log.Warn("Failed to sync campaigns with inventory", "error", err)
	}

	for _, streamer := range streamers {
		streamer.Mu.Lock()
		if streamer.DropsCondition() {
			var matchingCampaigns []model.Campaign
			for _, campaign := range campaigns {
				if len(campaign.Drops) > 0 && campaignMatchesStreamer(campaign, streamer) {
					matchingCampaigns = append(matchingCampaigns, *campaign)
				}
			}
			streamer.Stream.Campaigns = matchingCampaigns
		}
		streamer.Mu.Unlock()
	}

	return nil
}

func (c *Client) syncCampaignsWithInventory(ctx context.Context, campaigns []*model.Campaign) ([]*model.Campaign, error) {
	inventoryData, err := c.GQL.GetDropsInventory(ctx)
	if err != nil {
		return campaigns, fmt.Errorf("getting inventory: %w", err)
	}

	if inventoryData == nil {
		return campaigns, nil
	}

	var inventory struct {
		DropCampaignsInProgress []struct {
			ID             string `json:"id"`
			TimeBasedDrops []struct {
				ID              string `json:"id"`
				Name            string `json:"name"`
				RequiredMinutes int    `json:"requiredMinutesWatched"`
				Self            *struct {
					HasPreconditionsMet   bool   `json:"hasPreconditionsMet"`
					CurrentMinutesWatched int    `json:"currentMinutesWatched"`
					DropInstanceID        string `json:"dropInstanceID"`
					IsClaimed             bool   `json:"isClaimed"`
				} `json:"self"`
			} `json:"timeBasedDrops"`
		} `json:"dropCampaignsInProgress"`
	}

	if err := json.Unmarshal(inventoryData, &inventory); err != nil {
		return campaigns, fmt.Errorf("parsing inventory: %w", err)
	}

	if inventory.DropCampaignsInProgress == nil {
		return campaigns, nil
	}

	for i, campaign := range campaigns {
		campaign.ClearDrops()
		for _, progress := range inventory.DropCampaignsInProgress {
			if progress.ID == campaign.ID {
				campaigns[i].InInventory = true
				for _, timeDrop := range progress.TimeBasedDrops {
					for _, drop := range campaigns[i].Drops {
						if drop.ID == timeDrop.ID && timeDrop.Self != nil {
							drop.Update(
								timeDrop.Self.HasPreconditionsMet,
								timeDrop.Self.CurrentMinutesWatched,
								timeDrop.Self.DropInstanceID,
								timeDrop.Self.IsClaimed,
							)
							if drop.IsClaimable {
								c.Log.Event(ctx, model.EventDropClaim, "Claiming drop",
									"drop", drop.String())
								claimed, err := c.GQL.ClaimDropRewards(ctx, drop.DropInstanceID)
								if err != nil {
									c.Log.Warn("Failed to claim drop",
										"drop", drop.Name, "error", err)
								} else {
									drop.IsClaimed = claimed
								}
							}
						}
					}
				}
				campaigns[i].ClearDrops()
				break
			}
		}
	}

	return campaigns, nil
}

// ClaimDrop claims a single drop reward.
func (c *Client) ClaimDrop(ctx context.Context, dropInstanceID string) error {
	c.Log.Info("Claiming drop", "drop_instance_id", dropInstanceID)
	claimed, err := c.GQL.ClaimDropRewards(ctx, dropInstanceID)
	if err != nil {
		return fmt.Errorf("claiming drop %s: %w", dropInstanceID, err)
	}
	if !claimed {
		return fmt.Errorf("drop %s was not claimed", dropInstanceID)
	}
	return nil
}

// ClaimAllDropsFromInventory claims all pending drops from the user's inventory.
func (c *Client) ClaimAllDropsFromInventory(ctx context.Context) error {
	inventoryData, err := c.GQL.GetDropsInventory(ctx)
	if err != nil {
		return fmt.Errorf("getting inventory: %w", err)
	}

	if inventoryData == nil {
		return nil
	}

	var inventory struct {
		DropCampaignsInProgress []struct {
			TimeBasedDrops []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Self *struct {
					DropInstanceID string `json:"dropInstanceID"`
					IsClaimed      bool   `json:"isClaimed"`
				} `json:"self"`
			} `json:"timeBasedDrops"`
		} `json:"dropCampaignsInProgress"`
	}

	if err := json.Unmarshal(inventoryData, &inventory); err != nil {
		return fmt.Errorf("parsing inventory: %w", err)
	}

	if inventory.DropCampaignsInProgress == nil {
		return nil
	}

	for _, campaign := range inventory.DropCampaignsInProgress {
		for _, drop := range campaign.TimeBasedDrops {
			if drop.Self == nil {
				continue
			}
			if !drop.Self.IsClaimed && drop.Self.DropInstanceID != "" {
					c.Log.Event(ctx, model.EventDropClaim, "Claiming drop from inventory",
						"drop", drop.Name)
				_, err := c.GQL.ClaimDropRewards(ctx, drop.Self.DropInstanceID)
				if err != nil {
					c.Log.Warn("Failed to claim drop from inventory",
						"drop", drop.Name, "error", err)
				}
				sleepDuration := time.Duration(5+rand.IntN(5)) * time.Second
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(sleepDuration):
				}
			}
		}
	}

	return nil
}

func parseCampaign(raw json.RawMessage) (*model.Campaign, error) {
	var data struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Status  string `json:"status"`
		StartAt string `json:"startAt"`
		EndAt   string `json:"endAt"`
		Game    *struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Slug        string `json:"slug"`
		} `json:"game"`
		Allow *struct {
			Channels []struct {
				ID string `json:"id"`
			} `json:"channels"`
		} `json:"allow"`
		TimeBasedDrops []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			RequiredMinutes int    `json:"requiredMinutesWatched"`
			StartAt         string `json:"startAt"`
			EndAt           string `json:"endAt"`
			BenefitEdges    []struct {
				Benefit struct {
					Name string `json:"name"`
				} `json:"benefit"`
			} `json:"benefitEdges"`
		} `json:"timeBasedDrops"`
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parsing campaign JSON: %w", err)
	}

	var gameInfo *model.GameInfo
	if data.Game != nil {
		gameInfo = &model.GameInfo{
			ID:          data.Game.ID,
			Name:        data.Game.Name,
			DisplayName: data.Game.DisplayName,
			Slug:        data.Game.Slug,
		}
	}

	startAt, _ := time.Parse(time.RFC3339, data.StartAt)
	endAt, _ := time.Parse(time.RFC3339, data.EndAt)

	var channels []string
	if data.Allow != nil {
		for _, channel := range data.Allow.Channels {
			channels = append(channels, channel.ID)
		}
	}

	campaign := model.NewCampaign(data.ID, data.Name, data.Status, gameInfo, startAt, endAt, channels)

	for _, timeDrop := range data.TimeBasedDrops {
		dropStart, _ := time.Parse(time.RFC3339, timeDrop.StartAt)
		dropEnd, _ := time.Parse(time.RFC3339, timeDrop.EndAt)

		var benefits []string
		for _, benefitEdge := range timeDrop.BenefitEdges {
			benefits = append(benefits, benefitEdge.Benefit.Name)
		}

		drop := model.NewDrop(timeDrop.ID, timeDrop.Name, benefits, timeDrop.RequiredMinutes, dropStart, dropEnd)
		campaign.Drops = append(campaign.Drops, drop)
	}

	return campaign, nil
}

func campaignMatchesStreamer(campaign *model.Campaign, streamer *model.Streamer) bool {
	if campaign.Game != nil && streamer.Stream.Game != nil {
		if campaign.Game.Name != streamer.Stream.Game.Name {
			return false
		}
	}

	for _, id := range streamer.Stream.CampaignIDs {
		if id == campaign.ID {
			return true
		}
	}

	return false
}
