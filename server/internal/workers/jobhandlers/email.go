package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

// SendCampaignBatchPayload is the JSON payload for the TypeSendCampaignBatch task.
type SendCampaignBatchPayload struct {
	CampaignID string   `json:"campaign_id"`
	OrgID      string   `json:"org_id"`
	Recipients []string `json:"recipients"`
	// BatchIndex is the zero-based segment index for large sends.
	BatchIndex int `json:"batch_index"`
	BatchTotal int `json:"batch_total"`
}

// SendCampaignBatchHandler returns a Handler that sends a batch of campaign emails.
func SendCampaignBatchHandler(logger *slog.Logger, commsService *comms.Service) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p SendCampaignBatchPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("send campaign batch: decode payload: %w", err)
		}
		if p.CampaignID == "" || p.OrgID == "" {
			return fmt.Errorf("send campaign batch: missing required fields")
		}
		if commsService == nil {
			return fmt.Errorf("send campaign batch: communications service is not configured")
		}

		logger.Info("send campaign batch: starting",
			"campaign_id", p.CampaignID,
			"org_id", p.OrgID,
			"recipients", len(p.Recipients),
			"batch", fmt.Sprintf("%d/%d", p.BatchIndex+1, p.BatchTotal),
		)

		recipients := make([]comms.Recipient, 0, len(p.Recipients))
		for _, raw := range p.Recipients {
			email := strings.TrimSpace(raw)
			if email == "" {
				continue
			}
			recipients = append(recipients, comms.Recipient{Email: email})
		}
		if len(recipients) == 0 {
			return fmt.Errorf("send campaign batch: recipients are required")
		}

		if _, err := commsService.SendCampaign(ctx, p.OrgID, p.CampaignID, comms.SendCampaignInput{
			Recipients: recipients,
			ActorID:    "worker:" + workers.TypeSendCampaignBatch,
		}); err != nil {
			return fmt.Errorf("send campaign batch: send campaign %s: %w", p.CampaignID, err)
		}

		logger.Info("send campaign batch: completed",
			"campaign_id", p.CampaignID,
			"batch", fmt.Sprintf("%d/%d", p.BatchIndex+1, p.BatchTotal),
		)
		return nil
	}
}
