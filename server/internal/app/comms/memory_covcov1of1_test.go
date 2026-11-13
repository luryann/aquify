package comms

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func cov1xSend(t *testing.T, s *Service, org, hh, sw, sender, body string) *Message {
	t.Helper()
	m, err := s.SendMessage(CreateMessageInput{OrgID: org, HouseholdID: hh, SwimmerID: sw, Body: body, SenderUserID: sender, SenderRole: "coach"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCov1xNewServiceWithNilPoolIsInMemory(t *testing.T) {
	s := NewServiceWithPool(nil)
	if s.repo != nil {
		t.Fatal("nil pool must give an in-memory service")
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh without a repo must be a no-op: %v", err)
	}
	if err := s.RefreshOrg(context.Background(), "org"); err != nil {
		t.Fatal(err)
	}
	s.SetOrgCacheLimits(time.Minute, 2) // must not panic without a repo
	s.mu.Lock()
	err := s.reloadSuppressionLocked(context.Background(), "org")
	err2 := s.loadSuppressionLocked(context.Background(), "org", "a@example.com")
	s.mu.Unlock()
	if err != nil || err2 != nil {
		t.Fatalf("in-memory suppression reloads must be no-ops: %v %v", err, err2)
	}
	// IDs are hex in memory mode (uuid only when a database backs the service).
	if id := s.newID(); strings.Contains(id, "-") {
		t.Fatalf("id = %q", id)
	}
}

func TestCov1xRuntimeSnapshotEdgeCases(t *testing.T) {
	s := NewService()
	cov1xSend(t, s, "o", "h", "s", "u", "kept")
	if err := s.LoadRuntimeSnapshot(nil); err != nil {
		t.Fatalf("empty snapshot must be ignored: %v", err)
	}
	if len(s.messages) != 1 {
		t.Fatal("empty snapshot replaced state")
	}
	if err := s.LoadRuntimeSnapshot([]byte(`{not json`)); err == nil {
		t.Fatal("corrupt snapshot must be rejected")
	}
	if len(s.messages) != 1 {
		t.Fatal("corrupt snapshot partially applied")
	}
}

func TestCov1xListingsAreOrdered(t *testing.T) {
	s := NewService()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"a1", "a2", "a3"} {
		st := "draft"
		if i == 1 {
			st = "published"
		}
		s.announcements[id] = &Announcement{ID: id, OrgID: "o", Status: st, CreatedAt: base.Add(time.Duration(i) * time.Hour)}
	}
	s.announcements["other"] = &Announcement{ID: "other", OrgID: "x", CreatedAt: base}
	all, err := s.ListAnnouncements("o", "")
	if err != nil || len(all) != 3 || all[0].ID != "a3" || all[2].ID != "a1" {
		t.Fatalf("announcements newest first expected: %+v err=%v", all, err)
	}
	if only, _ := s.ListAnnouncements("o", "published"); len(only) != 1 || only[0].ID != "a2" {
		t.Fatalf("status filter = %+v", only)
	}

	for i, id := range []string{"c1", "c2"} {
		s.campaigns[id] = &EmailCampaign{ID: id, OrgID: "o", CreatedAt: base.Add(time.Duration(i) * time.Hour)}
		s.deliveries["d"+id] = &EmailDelivery{ID: "d" + id, OrgID: "o", CampaignID: "c1", CreatedAt: base.Add(time.Duration(2-i) * time.Hour)}
		s.suppression[suppressionKey("o", id+"@example.com")] = &SuppressionEntry{ID: id, OrgID: "o", Email: id + "@example.com", CreatedAt: base.Add(time.Duration(2-i) * time.Hour)}
	}
	cs, _ := s.ListCampaigns("o")
	if len(cs) != 2 || cs[0].ID != "c2" {
		t.Fatalf("campaigns newest first expected: %+v", cs)
	}
	ds, err := s.ListDeliveries("o", "c1")
	if err != nil || len(ds) != 2 || ds[0].ID != "dc2" {
		t.Fatalf("deliveries oldest first expected: %+v err=%v", ds, err)
	}
	sup, _ := s.ListSuppression("o")
	if len(sup) != 2 || sup[0].ID != "c2" {
		t.Fatalf("suppression oldest first expected: %+v", sup)
	}
}

func TestCov1xThreadViewsAreScopedPerThread(t *testing.T) {
	s := NewService()
	m1 := cov1xSend(t, s, "o", "h1", "s1", "coach", "first")
	m2 := cov1xSend(t, s, "o", "h1", "s1", "parent", "second")
	m3 := cov1xSend(t, s, "o", "h2", "s2", "coach", "other thread")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // explicit timestamps keep the ordering deterministic
	for i, m := range []*Message{m1, m2, m3} {
		m.CreatedAt = base.Add(time.Duration(i) * time.Minute)
	}
	cov1xSend(t, s, "other-org", "h1", "s1", "coach", "other club")

	for _, a := range []struct {
		m     *Message
		emoji string
		att   string
	}{{m1, "👍", "one.png"}, {m3, "🎉", "three.png"}} {
		if err := s.AddReaction("o", a.m.ID, "viewer", a.emoji); err != nil {
			t.Fatal(err)
		}
		id := s.newID()
		s.messageAttachments[id] = &MessageAttachment{ID: id, MessageID: a.m.ID, Filename: a.att, ObjectKey: "k/" + a.att}
	}

	views := s.ListMessages("o", "h1", "s1", "viewer")
	if len(views) != 2 || views[0].ID != m1.ID || views[1].ID != m2.ID {
		t.Fatalf("thread must be oldest first and exclude other threads/clubs: %+v", views)
	}
	if len(views[0].Reactions) != 1 || views[0].Reactions[0].Emoji != "👍" || !views[0].Reactions[0].ReactedByMe {
		t.Fatalf("m1 reactions = %+v", views[0].Reactions)
	}
	if len(views[0].Attachments) != 1 || views[0].Attachments[0].Filename != "one.png" || len(views[1].Reactions) != 0 || len(views[1].Attachments) != 0 {
		t.Fatalf("another message's reactions/attachments leaked: %+v", views)
	}

	threads := s.ListMessageThreads("o", "viewer")
	if len(threads) != 2 || threads[0].HouseholdID != "h2" || threads[1].HouseholdID != "h1" {
		t.Fatalf("threads must be most recent first and scoped to the club: %+v", threads)
	}
	if threads[1].MessageCount != 2 || threads[1].LastBody != "second" || threads[1].LastSenderRole != "coach" || threads[1].UnreadCount != 2 {
		t.Fatalf("h1 summary = %+v", threads[1])
	}
}

func TestCov1xReactionsAndEditsRejectUnknownMessages(t *testing.T) {
	s := NewService()
	if err := s.AddReaction("o", "missing", "u", "👍"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("AddReaction err = %v", err)
	}
	if err := s.RemoveReaction("o", "missing", "u", "👍"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("RemoveReaction err = %v", err)
	}
	m := cov1xSend(t, s, "o", "h", "s", "u", "x")
	if err := s.AddReaction("other-org", m.ID, "u", "👍"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("cross-club AddReaction err = %v", err)
	}
	if err := s.RemoveReaction("other-org", m.ID, "u", "👍"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("cross-club RemoveReaction err = %v", err)
	}
}

func TestCov1xCampaignValidationAndHelpers(t *testing.T) {
	s := NewService()
	_, err := s.CreateCampaign(CreateCampaignInput{OrgID: "o", Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "group", GroupID: " "})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("err = %v", err)
	}

	// updateCampaign re-inserts the campaign when a concurrent reload dropped it.
	base := EmailCampaign{ID: "c1", OrgID: "o", Status: "sending"}
	got := s.updateCampaign("o", "c1", base, func(c *EmailCampaign) { c.Status = "draft" })
	if got.Status != "draft" || s.campaigns["c1"] == nil || s.campaigns["c1"].Status != "draft" {
		t.Fatalf("got=%+v cached=%+v", got, s.campaigns["c1"])
	}
	// ...but never adopts another club's campaign under the same id.
	s.campaigns["c2"] = &EmailCampaign{ID: "c2", OrgID: "x", Status: "sent"}
	got = s.updateCampaign("o", "c2", EmailCampaign{ID: "c2", OrgID: "o"}, func(c *EmailCampaign) { c.Status = "draft" })
	if got.OrgID != "o" || s.campaigns["c2"].OrgID != "o" {
		t.Fatalf("got=%+v", got)
	}
}

func TestCov1xReleaseCampaignClaimInMemory(t *testing.T) {
	s := NewService()
	c, err := s.CreateCampaign(CreateCampaignInput{OrgID: "o", Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimCampaign(context.Background(), "o", c.ID, "u"); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("enqueue failed")
	if err := s.ReleaseCampaignClaim("o", c.ID, "releaser", cause); err != cause {
		t.Fatalf("err = %v", err)
	}
	got, err := s.GetCampaign("o", c.ID)
	if err != nil || got.Status != "draft" || got.UpdatedBy != "releaser" {
		t.Fatalf("campaign = %+v err=%v", got, err)
	}
	if err := s.ReleaseCampaignClaim("o", "nope", "u", cause); err != cause {
		t.Fatalf("unknown campaign err = %v", err)
	}
}

func TestCov1xRunCampaignSendRetriesFailedAndSkipsBlankRecipients(t *testing.T) {
	cl := newCov1xClient()
	s := NewServiceWithClient(cl)
	c, _ := s.CreateCampaign(CreateCampaignInput{OrgID: "o", Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
	s.deliveries["dfail"] = &EmailDelivery{ID: "dfail", OrgID: "o", CampaignID: c.ID, RecipientEmail: "failed@example.com", Status: "failed"}
	s.deliveries["dsupp"] = &EmailDelivery{ID: "dsupp", OrgID: "o", CampaignID: c.ID, RecipientEmail: "supp@example.com", Status: "suppressed"}
	s.deliveries["dother"] = &EmailDelivery{ID: "dother", OrgID: "x", CampaignID: c.ID, RecipientEmail: "elsewhere@example.com", Status: "sent"}
	got, err := s.SendCampaign(context.Background(), "o", c.ID, SendCampaignInput{Recipients: []Recipient{
		{Email: "failed@example.com"}, {Email: "supp@example.com"}, {Email: ""}, {Email: "elsewhere@example.com"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalRecipients != 3 || got.SentCount != 2 || got.SuppressedCount != 1 {
		t.Fatalf("campaign = %+v", got)
	}
	var to []string
	for _, m := range cl.inner.Sent() {
		to = append(to, m.Req.To)
	}
	if len(to) != 2 {
		t.Fatalf("mailed %v; another club's delivery row must not suppress this club's send", to)
	}
}

func TestCov1xApplyPostmarkEventInMemory(t *testing.T) {
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.FixedZone("x", 3600))
	newSvc := func() *Service {
		s := NewService()
		s.deliveries["d1"] = &EmailDelivery{ID: "d1", OrgID: "o", CampaignID: "c1", RecipientEmail: "a@example.com", ProviderMessageID: "pm-1", Status: "sent", CreatedAt: time.Now()}
		return s
	}

	t.Run("delivered keeps the provider timestamp in UTC", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", MessageID: "pm-1", RecordType: "Delivery", OccurredAt: at, Details: "250 ok"}); err != nil {
			t.Fatal(err)
		}
		d := s.deliveries["d1"]
		if d.Status != "delivered" || d.DeliveredAt == nil || !d.DeliveredAt.Equal(at) || d.DeliveredAt.Location() != time.UTC || d.ErrorMessage != "250 ok" {
			t.Fatalf("delivery = %+v", d)
		}
	})

	t.Run("bounce by message id alone adopts the delivery's club but suppresses nothing", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{MessageID: "pm-1", RecordType: "Bounce"}); err != nil {
			t.Fatal(err)
		}
		if s.deliveries["d1"].Status != "bounced" || s.deliveries["d1"].BouncedAt == nil {
			t.Fatalf("delivery = %+v", s.deliveries["d1"])
		}
		if len(s.suppression) != 0 {
			t.Fatalf("no recipient email was given, nothing to suppress: %+v", s.suppression)
		}
	})

	t.Run("bounce with nothing to attribute it to is rejected", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{MessageID: "unknown", RecordType: "Bounce"}); !errors.Is(err, ErrDeliveryNotFound) {
			t.Fatalf("err = %v", err)
		}
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", MessageID: "unknown", RecordType: "SpamComplaint"}); !errors.Is(err, ErrDeliveryNotFound) {
			t.Fatalf("no email err = %v", err)
		}
	})

	t.Run("delivery events need a delivery", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", MessageID: "unknown", RecordType: "Delivery"}); !errors.Is(err, ErrDeliveryNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a different recipient or campaign never matches", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", MessageID: "pm-1", RecipientEmail: "someone-else@example.com", RecordType: "Delivery"}); !errors.Is(err, ErrDeliveryNotFound) {
			t.Fatalf("recipient mismatch err = %v", err)
		}
		// Campaign fallback only adopts rows without a different provider id.
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", CampaignID: "c1", MessageID: "pm-other", RecipientEmail: "a@example.com", RecordType: "Delivery"}); !errors.Is(err, ErrDeliveryNotFound) {
			t.Fatalf("provider id mismatch err = %v", err)
		}
		if s.deliveries["d1"].Status != "sent" {
			t.Fatalf("delivery changed: %+v", s.deliveries["d1"])
		}
	})

	t.Run("subscription changes", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{RecordType: "SubscriptionChange", RecipientEmail: "a@example.com"}); err != nil {
			t.Fatalf("no org: %v", err)
		}
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", RecordType: "SubscriptionChange"}); err != nil {
			t.Fatalf("no email: %v", err)
		}
		if len(s.suppression) != 0 {
			t.Fatalf("suppression = %+v", s.suppression)
		}
		for reason, want := range map[string]string{"HardBounce": "bounce", " bounce ": "bounce", "SpamComplaint": "complaint", "complaint": "complaint", "": "manual", "ManualSuppression": "manual"} {
			email := strings.TrimSpace(strings.ToLower(reason)) + "-r@example.com"
			if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", RecipientEmail: email, RecordType: "SubscriptionChange", SuppressSending: true, SuppressionReason: reason}); err != nil {
				t.Fatal(err)
			}
			if got := s.suppression[suppressionKey("o", email)]; got == nil || got.Reason != want {
				t.Fatalf("reason %q stored as %+v, want %q", reason, got, want)
			}
		}
	})

	t.Run("unknown record types are ignored", func(t *testing.T) {
		s := newSvc()
		if err := s.ApplyPostmarkEvent(PostmarkEventInput{OrgID: "o", MessageID: "pm-1", RecordType: "Open"}); err != nil {
			t.Fatal(err)
		}
		if s.deliveries["d1"].Status != "sent" {
			t.Fatalf("delivery changed: %+v", s.deliveries["d1"])
		}
	})
}

func TestCov1xExportHouseholdDataIsScopedAndOrdered(t *testing.T) {
	s := NewService()
	m1 := cov1xSend(t, s, "o", "h1", "s1", "coach", "first")
	m2 := cov1xSend(t, s, "o", "h1", "s2", "coach", "second")
	m1.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m2.CreatedAt = m1.CreatedAt.Add(time.Minute)
	cov1xSend(t, s, "o", "h2", "s3", "coach", "someone else's")
	cov1xSend(t, s, "x", "h1", "s1", "coach", "another club's")
	if _, err := s.EditMessage("o", m1.ID, "coach", "first (edited)"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceNotificationPreferences("o", "parent", "h1", "parent", []NotificationPreferenceSetting{{EventID: "announcement", EmailEnabled: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceNotificationPreferences("o", "other-parent", "h2", "other-parent", []NotificationPreferenceSetting{{EventID: "announcement"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkThreadRead("o", "h1", "s1", "parent"); err != nil {
		t.Fatal(err)
	}

	exp := s.ExportHouseholdData("o", "h1")
	if len(exp.Messages) != 2 || exp.Messages[0].ID != m1.ID || exp.Messages[1].ID != m2.ID {
		t.Fatalf("messages = %+v", exp.Messages)
	}
	if len(exp.MessageEdits) != 1 || exp.MessageEdits[0].PreviousBody != "first" {
		t.Fatalf("edits = %+v", exp.MessageEdits)
	}
	if len(exp.MessageReads) != 1 || len(exp.Preferences) != 1 || exp.Preferences[0].UserID != "parent" {
		t.Fatalf("reads=%+v prefs=%+v", exp.MessageReads, exp.Preferences)
	}
	empty := s.ExportHouseholdData("o", "nobody")
	if empty.Messages == nil || empty.Preferences == nil || len(empty.Messages)+len(empty.Preferences) != 0 {
		t.Fatalf("empty export must use empty slices (JSON arrays): %+v", empty)
	}
}

func TestCov1xReplacePreferencesBackfillsMissingCreatedAt(t *testing.T) {
	s := NewService()
	key := notificationPreferenceKey("o", "u", "announcement")
	s.preferences[key] = &NotificationPreference{ID: "p1", OrgID: "o", UserID: "u", EventID: "announcement"} // legacy row, zero CreatedAt
	if _, err := s.ReplaceNotificationPreferences("o", "u", "", "actor", []NotificationPreferenceSetting{{EventID: "announcement", EmailEnabled: true}}); err != nil {
		t.Fatal(err)
	}
	p := s.preferences[key]
	if p.ID != "p1" || p.CreatedAt.IsZero() || !p.EmailEnabled || p.UpdatedBy != "actor" {
		t.Fatalf("pref = %+v", p)
	}
}

func TestCov1xFindDeliveryRespectsOrgBoundaryInMemory(t *testing.T) {
	s := NewService()
	s.deliveries["d1"] = &EmailDelivery{ID: "d1", OrgID: "a", CampaignID: "c", RecipientEmail: "x@example.com", ProviderMessageID: "pm", Status: "sent"}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.findDeliveryLocked(context.Background(), "b", "", "pm", "")
	if err != nil || d != nil {
		t.Fatalf("another club's delivery matched: %+v %v", d, err)
	}
	d, err = s.findDeliveryLocked(context.Background(), "", "", "pm", "X@EXAMPLE.com")
	if err != nil || d == nil || d.ID != "d1" {
		t.Fatalf("org-less lookup by message and email = %+v %v", d, err)
	}
	if d, _ := s.findDeliveryLocked(context.Background(), "a", "", "", ""); d != nil {
		t.Fatalf("a lookup with no keys must match nothing, got %+v", d)
	}
}
