package comms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/postmark"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

func TestApplyPostmarkEventFallsBackToCampaignRecipient(t *testing.T) {
	s := NewService()
	// Batch call timed out after Postmark accepted it: delivery is "failed" with no message id.
	s.deliveries["d1"] = &EmailDelivery{
		ID: "d1", OrgID: "org-a", CampaignID: "c1", RecipientEmail: "x@example.com", Status: "failed",
	}
	err := s.ApplyPostmarkEvent(PostmarkEventInput{
		OrgID: "org-a", CampaignID: "c1", MessageID: "pm-9", RecipientEmail: "X@example.com", RecordType: "Delivery",
	})
	if err != nil {
		t.Fatalf("fallback match: %v", err)
	}
	d := s.deliveries["d1"]
	if d.Status != "delivered" || d.ProviderMessageID != "pm-9" {
		t.Fatalf("delivery = %+v", d)
	}
	// A different org's campaign id must never match.
	err = s.ApplyPostmarkEvent(PostmarkEventInput{
		OrgID: "org-b", CampaignID: "c1", MessageID: "pm-8", RecipientEmail: "x@example.com", RecordType: "Delivery",
	})
	if !errors.Is(err, ErrDeliveryNotFound) {
		t.Fatalf("cross-org fallback err = %v", err)
	}
}

func TestSuppressionAppliedWithoutMatchingDelivery(t *testing.T) {
	s := NewService()
	for _, rt := range []string{"Bounce", "SpamComplaint"} {
		email := rt + "@example.com"
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "org-a", MessageID: "unknown", RecipientEmail: email, RecordType: rt}); err != nil {
			t.Fatalf("%s: %v", rt, err)
		}
		if !s.isSuppressed("org-a", email) {
			t.Fatalf("%s did not suppress", rt)
		}
	}
	if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "org-a", RecipientEmail: "sub@example.com", RecordType: "SubscriptionChange", SuppressSending: true, SuppressionReason: "ManualSuppression"}); err != nil {
		t.Fatal(err)
	}
	if !s.isSuppressed("org-a", "sub@example.com") {
		t.Fatal("SubscriptionChange did not suppress")
	}
	if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "org-a", RecipientEmail: "sub@example.com", RecordType: "SubscriptionChange"}); err != nil {
		t.Fatal(err)
	}
	if s.isSuppressed("org-a", "sub@example.com") {
		t.Fatal("SubscriptionChange did not lift suppression")
	}
}

func TestClaimCampaignStaleSendingCanBeReclaimed(t *testing.T) {
	s := NewService()
	c, err := s.CreateCampaign(CreateCampaignInput{OrgID: "org-a", Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.ClaimCampaign(ctx, "org-a", c.ID, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimCampaign(ctx, "org-a", c.ID, "u"); !errors.Is(err, ErrCampaignNotDraft) {
		t.Fatalf("fresh sending claim err = %v", err)
	}
	s.campaigns[c.ID].UpdatedAt = time.Now().Add(-2 * campaignSendStaleAfter)
	if _, err := s.ClaimCampaign(ctx, "org-a", c.ID, "u"); err != nil {
		t.Fatalf("stale reclaim: %v", err)
	}
	// Prior non-failed deliveries are not re-mailed on the re-claimed run.
	s.deliveries["d1"] = &EmailDelivery{ID: "d1", OrgID: "org-a", CampaignID: c.ID, RecipientEmail: "a@example.com", Status: "sent"}
	got, err := s.RunCampaignSend(ctx, "org-a", c.ID, SendCampaignInput{Recipients: []Recipient{{Email: "a@example.com"}, {Email: "b@example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.SentCount != 2 || len(s.PostmarkClient().(*postmark.StubClient).Sent()) != 1 {
		t.Fatalf("campaign = %+v", got)
	}
}

func TestDBStuckSendingCampaignReclaimedAndDeduped(t *testing.T) {
	a, b, ca, cb, org := twoInstances(t)
	c := newDraft(t, a, org)
	ctx := context.Background()
	// Instance A claims and mails one recipient, then "dies" (never finishes).
	if _, err := a.ClaimCampaign(ctx, org, c.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ClaimCampaign(ctx, org, c.ID, ""); !errors.Is(err, ErrCampaignNotDraft) {
		t.Fatalf("live claim err = %v", err)
	}
	recips := []Recipient{{Email: "one@example.com"}, {Email: "two@example.com"}}
	// Simulate the crash after the first delivery was persisted.
	if err := a.repo.UpsertDelivery(ctx, &EmailDelivery{ID: uuid.NewString(), OrgID: org, CampaignID: c.ID, RecipientEmail: "one@example.com", Status: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.repo.pool.Exec(ctx, `UPDATE email_campaigns SET updated_at = now() - interval '1 hour' WHERE id = $1::uuid`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SendCampaign(ctx, org, c.ID, SendCampaignInput{Recipients: recips}); err != nil {
		t.Fatalf("stale reclaim send: %v", err)
	}
	if len(ca.Sent())+len(cb.Sent()) != 1 {
		t.Fatalf("emails sent = %d, want only the not-yet-mailed recipient", len(ca.Sent())+len(cb.Sent()))
	}
	got, err := a.GetCampaign(org, c.ID)
	if err != nil || got.Status != "sent" || got.SentCount != 2 {
		t.Fatalf("final campaign = %+v err=%v", got, err)
	}
}

func TestDBOrgScopedReloadLeavesOtherOrgsAlone(t *testing.T) {
	a, _, _, _, org := twoInstances(t)
	other := uuid.NewString()
	newDraft(t, a, org)
	oc := newDraft(t, a, other)
	// Remove other org's row behind A's back; an org-scoped read of `org` must not notice.
	if _, err := a.repo.pool.Exec(context.Background(), `DELETE FROM email_campaigns WHERE id = $1::uuid`, oc.ID); err != nil {
		t.Fatal(err)
	}
	if list, err := a.ListCampaigns(org); err != nil || len(list) != 1 {
		t.Fatalf("list = %d err=%v", len(list), err)
	}
	a.mu.RLock()
	_, stillCached := a.campaigns[oc.ID]
	a.mu.RUnlock()
	if !stillCached {
		t.Fatal("org-scoped reload touched another org's cache")
	}
	if _, err := a.GetCampaign(other, oc.ID); !errors.Is(err, ErrCampaignNotFound) {
		t.Fatalf("other org get after delete err = %v", err)
	}
}

func TestDBWebhookFallbackAcrossInstances(t *testing.T) {
	a, b, _, _, org := twoInstances(t)
	c := newDraft(t, a, org)
	if err := a.repo.UpsertDelivery(context.Background(), &EmailDelivery{ID: uuid.NewString(), OrgID: org, CampaignID: c.ID, RecipientEmail: "x@example.com", Status: "failed", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := b.ApplyPostmarkEvent(PostmarkEventInput{OrgID: org, CampaignID: c.ID, MessageID: "pm-1", RecipientEmail: "x@example.com", RecordType: "Delivery"}); err != nil {
		t.Fatal(err)
	}
	ds, err := a.ListDeliveries(org, c.ID)
	if err != nil || len(ds) != 1 || ds[0].Status != "delivered" || ds[0].ProviderMessageID != "pm-1" {
		t.Fatalf("deliveries = %+v err=%v", ds, err)
	}
	// Bounce with no matching delivery still suppresses and persists.
	if err := b.ApplyPostmarkEvent(PostmarkEventInput{OrgID: org, MessageID: "nope", RecipientEmail: "ghost@example.com", RecordType: "Bounce"}); err != nil {
		t.Fatal(err)
	}
	if list, err := a.ListSuppression(org); err != nil || len(list) != 1 {
		t.Fatalf("suppression = %d err=%v", len(list), err)
	}
}

var _ = testutil.IsolatedSchemaPool

func TestRunCampaignSendFinalStatusRule(t *testing.T) {
	cases := []struct {
		name       string
		failFirst  bool
		suppressed []string
		recipients []string
		want       string
		sent, fail int
		supp       int
	}{
		{"all deliverable failed with suppressed", true, []string{"s@example.com"}, []string{"a@example.com", "s@example.com"}, "failed", 0, 1, 1},
		{"at least one sent", false, []string{"s@example.com"}, []string{"a@example.com", "s@example.com"}, "sent", 1, 0, 1},
		{"everything suppressed", false, []string{"s@example.com"}, []string{"s@example.com"}, "sent", 0, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewService()
			c, err := s.CreateCampaign(CreateCampaignInput{OrgID: "org-a", Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range tc.suppressed {
				if _, err := s.AddSuppression(AddSuppressionInput{OrgID: "org-a", Email: e, Reason: "test"}); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			if _, err := s.ClaimCampaign(ctx, "org-a", c.ID, "u"); err != nil {
				t.Fatal(err)
			}
			s.PostmarkClient().(*postmark.StubClient).FailNext = tc.failFirst
			rs := make([]Recipient, 0, len(tc.recipients))
			for _, e := range tc.recipients {
				rs = append(rs, Recipient{Email: e})
			}
			got, err := s.RunCampaignSend(ctx, "org-a", c.ID, SendCampaignInput{Recipients: rs})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want || got.SentCount != tc.sent || got.FailedCount != tc.fail || got.SuppressedCount != tc.supp {
				t.Fatalf("campaign = status %s sent %d failed %d suppressed %d", got.Status, got.SentCount, got.FailedCount, got.SuppressedCount)
			}
		})
	}
}
