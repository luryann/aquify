package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/workers"
)

// TaskTypeCampaignSend is the background job that delivers a claimed email
// campaign. It runs on the email queue.
const TaskTypeCampaignSend = "email:campaign_send"

// CampaignSendPayload identifies a claimed campaign. Recipients are NOT carried
// in the payload; the job resolves them server-side from org membership data.
type CampaignSendPayload struct {
	OrgID         string `json:"org_id"`
	CampaignID    string `json:"campaign_id"`
	ActorID       string `json:"actor_id"`
	ClaimedAtUnix int64  `json:"claimed_at_unix_nano"`
}

// SetJobQueue wires the worker queue used to deliver campaigns. Without a
// queue (nil), sends fall back to a background goroutine.
func (h *CommsHandlers) SetJobQueue(queue workers.Queue) {
	h.queue = queue
}

// dispatchCampaignSend hands a claimed campaign to the durable job queue, or to
// a background goroutine when no queue is configured.
func (h *CommsHandlers) dispatchCampaignSend(ctx context.Context, p CampaignSendPayload) error {
	if h.queue == nil {
		go func() {
			if err := h.runCampaignSend(context.Background(), p); err != nil {
				slog.Error("comms: campaign send failed", "campaign_id", p.CampaignID, "org_id", p.OrgID, "error", err)
			}
		}()
		return nil
	}
	task, err := workers.NewTask(TaskTypeCampaignSend, p)
	if err != nil {
		return fmt.Errorf("build campaign send task: %w", err)
	}
	// Unique per claim so a stale re-claim can enqueue again.
	key := "campaign-send:" + p.CampaignID + ":" + strconv.FormatInt(p.ClaimedAtUnix, 10)
	if _, err := h.queue.Enqueue(ctx, task, workers.WithQueue(workers.QueueEmail), workers.WithUniqueKey(key)); err != nil {
		return fmt.Errorf("enqueue campaign send: %w", err)
	}
	return nil
}

// runCampaignSend resolves recipients and delivers the claimed campaign.
func (h *CommsHandlers) runCampaignSend(ctx context.Context, p CampaignSendPayload) error {
	if p.OrgID == "" || p.CampaignID == "" {
		return errors.New("campaign send: missing org_id or campaign_id")
	}
	campaign, err := h.service.GetCampaign(p.OrgID, p.CampaignID)
	if err != nil {
		return fmt.Errorf("campaign send: load campaign: %w", err)
	}
	if campaign.Status != "sending" {
		slog.Info("comms: campaign send skipped; not in sending status", "campaign_id", p.CampaignID, "status", campaign.Status)
		return nil
	}
	recipients, err := h.resolveCampaignRecipients(ctx, p.OrgID, campaign)
	if err != nil {
		_ = h.service.ReleaseCampaignClaim(p.OrgID, p.CampaignID, p.ActorID, err)
		return fmt.Errorf("campaign send: resolve recipients: %w", err)
	}
	result, err := h.service.RunCampaignSend(ctx, p.OrgID, p.CampaignID, comms.SendCampaignInput{Recipients: recipients, ActorID: p.ActorID})
	if err != nil {
		if errors.Is(err, comms.ErrCampaignNotSending) {
			return nil
		}
		return fmt.Errorf("campaign send: %w", err)
	}
	slog.Info("comms: campaign send finished", "campaign_id", p.CampaignID, "status", result.Status,
		"sent", result.SentCount, "failed", result.FailedCount, "suppressed", result.SuppressedCount)
	return nil
}

// CampaignSendJobHandler processes TaskTypeCampaignSend jobs.
func (h *CommsHandlers) CampaignSendJobHandler() workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p CampaignSendPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("campaign send: decode payload: %w", err)
		}
		if err := h.runCampaignSend(ctx, p); err != nil {
			slog.Error("comms: campaign send job failed", "campaign_id", p.CampaignID, "org_id", p.OrgID, "error", err)
			return err
		}
		return nil
	}
}
