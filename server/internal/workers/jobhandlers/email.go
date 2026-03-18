package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

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
func SendCampaignBatchHandler(logger *slog.Logger) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p SendCampaignBatchPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("send campaign batch: decode payload: %w", err)
		}
		if p.CampaignID == "" || p.OrgID == "" {
			return fmt.Errorf("send campaign batch: missing required fields")
		}

		logger.Info("send campaign batch: starting",
			"campaign_id", p.CampaignID,
			"org_id", p.OrgID,
			"recipients", len(p.Recipients),
			"batch", fmt.Sprintf("%d/%d", p.BatchIndex+1, p.BatchTotal),
		)

		// Scaffold: simulate batch send.
		// Production: fetch campaign content from comms.Service →
		// filter suppressed addresses → send via postmark.Client.SendBatch →
		// record delivery statuses back to comms.Service.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		logger.Info("send campaign batch: completed",
			"campaign_id", p.CampaignID,
			"batch", fmt.Sprintf("%d/%d", p.BatchIndex+1, p.BatchTotal),
		)
		return nil
	}
}
