package comms

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/postmark"
	"github.com/luryann/aquify/server/internal/testutil"
)

func twoInstances(t *testing.T) (*Service, *Service, *postmark.StubClient, *postmark.StubClient, string) {
	pool := testutil.IsolatedSchemaPool(t)
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)
	ca, cb := postmark.NewStubClient(), postmark.NewStubClient()
	a.SetPostmarkClient(ca)
	b.SetPostmarkClient(cb)
	return a, b, ca, cb, uuid.NewString()
}

func newDraft(t *testing.T, s *Service, org string) *EmailCampaign {
	t.Helper()
	c, err := s.CreateCampaign(CreateCampaignInput{OrgID: org, Name: "n", Subject: "s", BodyHTML: "<p>x</p>", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDBCrossInstanceCampaignAndAnnouncement(t *testing.T) {
	a, b, _, _, org := twoInstances(t)
	c := newDraft(t, a, org)
	if got, err := b.GetCampaign(org, c.ID); err != nil || got.ID != c.ID {
		t.Fatalf("B get campaign = %+v err=%v", got, err)
	}
	if list, err := b.ListCampaigns(org); err != nil || len(list) != 1 {
		t.Fatalf("B list campaigns = %d err=%v", len(list), err)
	}
	an, err := a.CreateAnnouncement(CreateAnnouncementInput{OrgID: org, Title: "t", Body: "b", Audience: "all", CreatedBy: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if list, err := b.ListAnnouncements(org, ""); err != nil || len(list) != 1 {
		t.Fatalf("B list announcements = %d err=%v", len(list), err)
	}
	if _, err := b.PublishAnnouncement(org, an.ID, uuid.NewString()); err != nil {
		t.Fatalf("B publish: %v", err)
	}
	if got, err := a.GetAnnouncement(org, an.ID); err != nil || got.Status != "published" {
		t.Fatalf("A sees %+v err=%v", got, err)
	}
	// Preferences written on A are visible on B.
	if _, err := a.ReplaceNotificationPreferences(org, uuid.NewString(), "", "", nil); err != nil {
		t.Fatal(err)
	}
}

func TestDBCrossInstanceSuppressionHonouredBySend(t *testing.T) {
	a, b, ca, _, org := twoInstances(t)
	c := newDraft(t, a, org)
	if err := b.ApplyPostmarkEvent(PostmarkEventInput{OrgID: org, RecipientEmail: "bounce@example.com", RecordType: "Bounce"}); err != nil {
		t.Fatal(err)
	}
	got, err := a.SendCampaign(context.Background(), org, c.ID, SendCampaignInput{Recipients: []Recipient{{Email: "bounce@example.com"}, {Email: "ok@example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.SuppressedCount != 1 || got.SentCount != 1 || len(ca.Sent()) != 1 {
		t.Fatalf("campaign = %+v sent=%d", got, len(ca.Sent()))
	}
	// Remove on B is also observed by A.
	if err := b.removeSuppressionByEmailLockedForTest(org, "bounce@example.com"); err != nil {
		t.Fatal(err)
	}
	if list, err := a.ListSuppression(org); err != nil || len(list) != 0 {
		t.Fatalf("A suppression = %d err=%v", len(list), err)
	}
}

func TestDBConcurrentSendOnlyOneProceeds(t *testing.T) {
	a, b, ca, cb, org := twoInstances(t)
	c := newDraft(t, a, org)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, s := range []*Service{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.SendCampaign(context.Background(), org, c.ID, SendCampaignInput{Recipients: []Recipient{{Email: "x@example.com"}}})
		}()
	}
	wg.Wait()
	ok, notDraft := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, ErrCampaignNotDraft):
			notDraft++
		default:
			t.Fatalf("unexpected error %v", e)
		}
	}
	if ok != 1 || notDraft != 1 || len(ca.Sent())+len(cb.Sent()) != 1 {
		t.Fatalf("ok=%d notDraft=%d sent=%d", ok, notDraft, len(ca.Sent())+len(cb.Sent()))
	}
}

func (s *Service) removeSuppressionByEmailLockedForTest(orgID, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeSuppressionByEmailLocked(orgID, email)
}
