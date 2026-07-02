package jobhandlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/postmark"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

func TestSendCampaignBatchHandler_SendsThroughCommsService(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	postmarkClient := postmark.NewStubClient()
	commsService := comms.NewServiceWithClient(postmarkClient)
	campaign, err := commsService.CreateCampaign(comms.CreateCampaignInput{
		OrgID:     "org-email",
		Name:      "Practice Update",
		Subject:   "Practice update",
		BodyHTML:  "<p>Pool opens at 4.</p>",
		FromName:  "DARE Aquatics",
		FromEmail: "noreply@example.com",
		Audience:  "all",
		CreatedBy: "admin-user",
	})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	task, err := workers.NewTask(workers.TypeSendCampaignBatch, SendCampaignBatchPayload{
		CampaignID: campaign.ID,
		OrgID:      "org-email",
		Recipients: []string{"parent@example.com", " second@example.com "},
		BatchIndex: 0,
		BatchTotal: 1,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := SendCampaignBatchHandler(logger, commsService)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	after, err := commsService.GetCampaign("org-email", campaign.ID)
	if err != nil {
		t.Fatalf("GetCampaign: %v", err)
	}
	if after.Status != "sent" {
		t.Fatalf("status = %q, want sent", after.Status)
	}
	if after.TotalRecipients != 2 || after.SentCount != 2 {
		t.Fatalf("campaign counts = total %d sent %d", after.TotalRecipients, after.SentCount)
	}

	deliveries, err := commsService.ListDeliveries("org-email", campaign.ID)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("deliveries = %d, want 2", len(deliveries))
	}
	if got := len(postmarkClient.Sent()); got != 2 {
		t.Fatalf("postmark sends = %d, want 2", got)
	}
}
