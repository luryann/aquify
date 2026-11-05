package comms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/postmark"
)

func TestCampaignFromAddressIsPinnedToTheOrgSender(t *testing.T) {
	svc := NewService()
	cfg := map[string][2]string{"org-a": {"Club A", "swim@club-a.example"}}
	svc.SetSenderPolicy(func(orgID string) (string, string, bool) {
		v, ok := cfg[orgID]
		return v[0], v[1], ok
	})
	in := CreateCampaignInput{OrgID: "org-a", Name: "n", Subject: "s", BodyHTML: "b", FromEmail: "ceo@club-b.example", Audience: "all"}
	c, err := svc.CreateCampaign(in)
	if err != nil || c.FromEmail != "swim@club-a.example" || c.FromName != "Club A" {
		t.Fatalf("From must be forced to the org sender: %+v %v", c, err)
	}
	in.OrgID = "org-without-sender"
	if _, err := svc.CreateCampaign(in); !errors.Is(err, ErrSenderNotConfigured) {
		t.Fatalf("an org with no sender must not create campaigns: %v", err)
	}
	// With no policy (dev/test) From stays caller-supplied.
	open := NewService()
	in.OrgID = "org-a"
	if c, err := open.CreateCampaign(in); err != nil || c.FromEmail != "ceo@club-b.example" {
		t.Fatalf("no policy: %+v %v", c, err)
	}
}

func TestDailyEmailCapIsPerOrgAndRollsOver(t *testing.T) {
	svc := NewService()
	svc.SetPostmarkClient(postmark.NewStubClient())
	svc.SetDailyEmailCap(3)
	mk := func(org string) *EmailCampaign {
		c, err := svc.CreateCampaign(CreateCampaignInput{OrgID: org, Name: "n", Subject: "s", BodyHTML: "b", FromEmail: "a@x.example", FromName: "A", Audience: "all"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ClaimCampaign(context.Background(), org, c.ID, "u"); err != nil {
			t.Fatal(err)
		}
		return c
	}
	recips := func(emails ...string) SendCampaignInput {
		in := SendCampaignInput{ActorID: "u"}
		for _, e := range emails {
			in.Recipients = append(in.Recipients, Recipient{Email: e})
		}
		return in
	}
	c1 := mk("org-a")
	if _, err := svc.RunCampaignSend(context.Background(), "org-a", c1.ID, recips("1@x.example", "2@x.example")); err != nil {
		t.Fatalf("2 of 3 emails: %v", err)
	}
	c2 := mk("org-a")
	if _, err := svc.RunCampaignSend(context.Background(), "org-a", c2.ID, recips("3@x.example", "4@x.example")); !errors.Is(err, ErrEmailQuotaExceeded) {
		t.Fatalf("a campaign that would pass the cap must be refused: %v", err)
	}
	if got, _ := svc.GetCampaign("org-a", c2.ID); got.Status == "sent" || got.Status == "failed" {
		t.Fatalf("a refused campaign must return to a sendable state, got %q", got.Status)
	}
	// Another org has its own allowance.
	cb := mk("org-b")
	if _, err := svc.RunCampaignSend(context.Background(), "org-b", cb.ID, recips("1@x.example", "2@x.example", "3@x.example")); err != nil {
		t.Fatalf("org B is unaffected by org A's usage: %v", err)
	}
	// Deliveries older than 24h no longer count.
	svc.mu.Lock()
	for _, d := range svc.deliveries {
		if d.OrgID == "org-a" {
			d.CreatedAt = time.Now().UTC().Add(-25 * time.Hour)
		}
	}
	svc.mu.Unlock()
	c3 := mk("org-a")
	if _, err := svc.RunCampaignSend(context.Background(), "org-a", c3.ID, recips("5@x.example", "6@x.example", "7@x.example")); err != nil {
		t.Fatalf("the allowance rolls over after 24h: %v", err)
	}
}
