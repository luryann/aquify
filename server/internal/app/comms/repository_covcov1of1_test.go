package comms

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cov1xSeedOrg registers a fresh club with one household and swimmer on pool.
func cov1xSeedOrg(t *testing.T, pool *pgxpool.Pool) (org, hh, sw string) {
	t.Helper()
	org, hh, sw = uuid.NewString(), uuid.NewString(), uuid.NewString()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1::uuid, $2::uuid, 'Cov1x Household')`, hh, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Test', 'Swimmer', '2012-01-01', 'male')`, sw, org, hh); err != nil {
		t.Fatal(err)
	}
	return org, hh, sw
}

// cov1xLoadBreakers renames a column each loader selects so that exactly one loader's
// query fails to prepare. (A dropped table could silently resolve to a same-named
// table in the shared public schema, so the schema-private table is altered instead.)
var cov1xLoadBreakers = []struct{ table, column, wantErr string }{
	{"announcements", "title", "comms: load announcements"},
	{"email_campaigns", "name", "comms: load campaigns"},
	{"email_deliveries", "status", "comms: load email_deliveries"},
	{"suppression_list", "reason", "comms: load suppression_list"},
	{"notification_preferences", "event_id", "comms: load notification_preferences"},
	{"messages", "body", "comms: load messages"},
	{"message_edits", "previous_body", "comms: load message_edits"},
	{"message_reactions", "emoji", "comms: load message_reactions"},
	{"message_attachments", "filename", "comms: load message_attachments"},
	{"message_reads", "last_read_at", "comms: load message_reads"},
}

func cov1xBreak(t *testing.T, pool *pgxpool.Pool, table, column string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `ALTER TABLE `+table+` RENAME COLUMN `+column+` TO `+column+`_broken`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `ALTER TABLE `+table+` RENAME COLUMN `+column+`_broken TO `+column)
	})
}

func TestCov1xLoadAllReportsWhichLoaderFailed(t *testing.T) {
	base := cov1xNewEnv(t) // one schema shared by the subtests; each uses its own club
	ctx := context.Background()
	for _, tc := range cov1xLoadBreakers {
		t.Run(tc.table, func(t *testing.T) {
			e := base.sibling(t)
			cov1xBreak(t, e.pool, tc.table, tc.column)

			if _, _, _, _, _, _, _, _, _, _, err := e.svc.repo.LoadAll(ctx, e.org); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("single-club LoadAll err = %v, want %q", err, tc.wantErr)
			}
			if _, _, _, _, _, _, _, _, _, _, err := e.svc.repo.LoadAll(ctx, ""); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("all-club LoadAll err = %v, want %q", err, tc.wantErr)
			}
			if err := e.svc.RefreshOrg(ctx, e.org); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("RefreshOrg err = %v", err)
			}
			if err := e.svc.Refresh(ctx); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Refresh err = %v", err)
			}
			// Reads and writes that reload first fail instead of acting on a stale cache.
			if _, err := e.svc.GetAnnouncement(e.org, "x"); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("GetAnnouncement err = %v", err)
			}
			if _, err := e.svc.ReplaceNotificationPreferences(e.org, "u", "", "a", nil); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ReplaceNotificationPreferences err = %v", err)
			}
		})
	}
}

func TestCov1xLoadersFailWhenTheirTableIsUnreadable(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	cov1xBreak(t, e.pool, "suppression_list", "reason")
	if _, err := e.svc.repo.LoadSuppressionEntry(ctx, e.org, "a@example.com"); err == nil || !strings.Contains(err.Error(), "comms: load suppression entry") {
		t.Fatalf("LoadSuppressionEntry err = %v", err)
	}
	if _, err := e.svc.repo.LoadSuppressionEntries(ctx, ""); err == nil {
		t.Fatal("unscoped LoadSuppressionEntries should fail too")
	}
	// reloadSuppressionLocked reports the same failure.
	e.svc.mu.Lock()
	err := e.svc.reloadSuppressionLocked(ctx, e.org)
	e.svc.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "comms: load suppression_list") {
		t.Fatalf("reloadSuppressionLocked err = %v", err)
	}
	// ...and loadSuppressionLocked, used by webhooks.
	e.svc.mu.Lock()
	err = e.svc.loadSuppressionLocked(ctx, e.org, "a@example.com")
	e.svc.mu.Unlock()
	if err == nil {
		t.Fatal("loadSuppressionLocked should fail")
	}
	for _, rt := range []string{"Bounce", "SpamComplaint"} {
		if err := e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: "a@example.com", RecordType: rt}); err == nil {
			t.Fatalf("%s with unreadable suppression list should fail so the webhook retries", rt)
		}
	}
	if err := e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: "a@example.com", RecordType: "SubscriptionChange", SuppressSending: true}); err == nil {
		t.Fatal("SubscriptionChange suppress should fail")
	}
}

func TestCov1xListOrgsFailureStopsAllClubLoad(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	if _, err := e.pool.Exec(ctx, `ALTER TABLE orgs RENAME COLUMN id TO id_broken`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.repo.OrgIDs(ctx); err == nil || !strings.Contains(err.Error(), "list orgs") {
		t.Fatalf("OrgIDs err = %v", err)
	}
	if err := e.svc.Refresh(ctx); err == nil || !strings.Contains(err.Error(), "list orgs") {
		t.Fatalf("Refresh err = %v", err)
	}
}

func TestCov1xRefreshLoadsEveryClub(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	org2, hh2, sw2 := cov1xSeedOrg(t, e.pool)
	e.send(t, uuid.NewString(), "club one")
	if _, err := e.svc.SendMessage(CreateMessageInput{OrgID: org2, HouseholdID: hh2, SwimmerID: sw2, Body: "club two", SenderUserID: uuid.NewString(), SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}
	ids, err := e.svc.repo.OrgIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[e.org] || !seen[org2] {
		t.Fatalf("OrgIDs = %v", ids)
	}

	b := e.other()
	if err := b.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if len(b.messages) != 2 {
		t.Fatalf("full reload cached %d messages, want 2", len(b.messages))
	}
	if got := b.ListMessages(org2, hh2, sw2, "x"); len(got) != 1 || got[0].Body != "club two" {
		t.Fatalf("club two thread = %+v", got)
	}
	// A club's rows never leak into another club's view.
	if got := b.ListMessages(org2, e.hh, e.sw, "x"); len(got) != 0 {
		t.Fatalf("cross-club read = %+v", got)
	}
}

func TestCov1xRepositoryDirectWrites(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	m := e.send(t, uuid.NewString(), "before")
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := e.svc.repo.UpdateMessageBody(ctx, e.org, m.ID, "after", at); err != nil {
		t.Fatal(err)
	}
	var body string
	var editedAt time.Time
	if err := e.pool.QueryRow(ctx, `SELECT body, edited_at FROM messages WHERE id = $1::uuid`, m.ID).Scan(&body, &editedAt); err != nil {
		t.Fatal(err)
	}
	if body != "after" || !editedAt.Equal(at) {
		t.Fatalf("body=%q editedAt=%v", body, editedAt)
	}
	// Another club cannot edit the message through its own binding.
	if err := e.svc.repo.UpdateMessageBody(ctx, uuid.NewString(), m.ID, "hijack", at); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `SELECT body FROM messages WHERE id = $1::uuid`, m.ID).Scan(&body); err != nil || body != "after" {
		t.Fatalf("cross-club update changed the row: %q %v", body, err)
	}

	// Claiming: first caller wins, a fresh claim cannot be stolen, a stale one can.
	c := e.draftCampaign(t)
	if won, err := e.svc.repo.ClaimCampaignForSend(ctx, e.org, c.ID, time.Hour); err != nil || !won {
		t.Fatalf("first claim = %v %v", won, err)
	}
	if won, err := e.svc.repo.ClaimCampaignForSend(ctx, e.org, c.ID, time.Hour); err != nil || won {
		t.Fatalf("second claim = %v %v", won, err)
	}
	if won, err := e.svc.repo.ClaimCampaignForSend(ctx, e.org, c.ID, 0); err != nil || !won {
		t.Fatalf("stale reclaim = %v %v", won, err)
	}

	if rows, err := e.svc.repo.FindDeliveryByProviderMessageID(ctx, "", "anything"); err != nil || rows != nil {
		t.Fatalf("org-less lookup = %v %v", rows, err)
	}
}

func TestCov1xClosedDatabaseFailsEveryOperation(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	m := e.send(t, uuid.NewString(), "x")
	c := e.draftCampaign(t)
	a, err := e.svc.CreateAnnouncement(CreateAnnouncementInput{OrgID: e.org, Title: "t", Body: "b", Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	e.pool.Close()

	ops := map[string]func() error{
		"Refresh":           func() error { return e.svc.Refresh(ctx) },
		"RefreshOrg":        func() error { return e.svc.RefreshOrg(ctx, e.org) },
		"GetAnnouncement":   func() error { _, err := e.svc.GetAnnouncement(e.org, a.ID); return err },
		"ListAnnouncements": func() error { _, err := e.svc.ListAnnouncements(e.org, ""); return err },
		"SetAnnouncementPublic": func() error {
			_, err := e.svc.SetAnnouncementPublic(e.org, a.ID, "u", true)
			return err
		},
		"PublishAnnouncement": func() error { _, err := e.svc.PublishAnnouncement(e.org, a.ID, "u"); return err },
		"SendMessage": func() error {
			_, err := e.svc.SendMessage(CreateMessageInput{OrgID: e.org, HouseholdID: e.hh, SwimmerID: e.sw, Body: "x", SenderUserID: "u", SenderRole: "coach"})
			return err
		},
		"EditMessage":    func() error { _, err := e.svc.EditMessage(e.org, m.ID, "u", "y"); return err },
		"AddReaction":    func() error { return e.svc.AddReaction(e.org, m.ID, "u", "👍") },
		"RemoveReaction": func() error { return e.svc.RemoveReaction(e.org, m.ID, "u", "👍") },
		"MarkThreadRead": func() error { return e.svc.MarkThreadRead(e.org, e.hh, e.sw, "u") },
		"ReplaceNotificationPreferences": func() error {
			_, err := e.svc.ReplaceNotificationPreferences(e.org, "u", "", "a", nil)
			return err
		},
		"GetCampaign":   func() error { _, err := e.svc.GetCampaign(e.org, c.ID); return err },
		"ListCampaigns": func() error { _, err := e.svc.ListCampaigns(e.org); return err },
		"CreateCampaign": func() error {
			_, err := e.svc.CreateCampaign(CreateCampaignInput{OrgID: e.org, Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
			return err
		},
		"ClaimCampaign":   func() error { _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "u"); return err },
		"RunCampaignSend": func() error { _, err := e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{}); return err },
		"ListDeliveries":  func() error { _, err := e.svc.ListDeliveries(e.org, c.ID); return err },
		"AddSuppression": func() error {
			_, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "a@example.com", Reason: "manual"})
			return err
		},
		"RemoveSuppression": func() error { return e.svc.RemoveSuppression(e.org, "id") },
		"ListSuppression":   func() error { _, err := e.svc.ListSuppression(e.org); return err },
		"HouseholdObjectKeys": func() error {
			_, err := e.svc.HouseholdObjectKeys(ctx, e.org, e.hh)
			return err
		},
		"EraseHouseholdData": func() error {
			_, err := e.svc.EraseHouseholdData(ctx, e.org, e.hh, nil)
			return err
		},
		"ApplyPostmarkEvent by message id": func() error {
			return e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: "pm-1", RecordType: "Delivery"})
		},
		"ApplyPostmarkEvent by campaign recipient": func() error {
			return e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, CampaignID: c.ID, RecipientEmail: "a@example.com", RecordType: "Delivery"})
		},
		"repo.EraseHousehold": func() error { return e.svc.repo.EraseHousehold(ctx, e.org, e.hh, nil) },
		"repo.UpsertNotificationPreferences": func() error {
			return e.svc.repo.UpsertNotificationPreferences(ctx, []*NotificationPreference{{ID: "1", OrgID: e.org, UserID: "u", EventID: "announcement"}})
		},
		"repo.EditMessageTx": func() error {
			return e.svc.repo.EditMessageTx(ctx, e.org, &MessageEdit{ID: uuid.NewString(), MessageID: m.ID}, "b", time.Now())
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			err := op()
			if err == nil || !strings.Contains(err.Error(), "closed pool") {
				t.Fatalf("err = %v, want closed pool", err)
			}
		})
	}

	// Read paths that cannot return an error serve the last known state and log why.
	logs := cov1xCaptureLogs(t)
	if got := e.svc.ListMessages(e.org, e.hh, e.sw, "u"); len(got) != 1 || got[0].ID != m.ID {
		t.Fatalf("cached ListMessages = %+v", got)
	}
	if got := e.svc.ListMessageThreads(e.org, "u"); len(got) != 1 {
		t.Fatalf("cached threads = %+v", got)
	}
	if _, _, ok := e.svc.MessageThread(e.org, m.ID); !ok {
		t.Fatal("cached MessageThread lost")
	}
	if got := e.svc.ListNotificationPreferences(e.org, "u"); len(got) != len(notificationPreferenceDefinitions) {
		t.Fatalf("cached prefs = %+v", got)
	}
	if got := e.svc.ListPublicAnnouncements(e.org); len(got) != 0 {
		t.Fatalf("draft must not be public: %+v", got)
	}
	if !strings.Contains(logs.String(), "refresh from DB failed") {
		t.Fatalf("expected a refresh warning, logs = %s", logs.String())
	}
}

func TestCov1xReloadFallsBackToLockedLoadUnderWriterContention(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	e.send(t, uuid.NewString(), "visible after contended reload")
	b := e.other()

	// A writer that keeps finishing invalidates every lock-free snapshot, so the
	// reload must give up and load under the write lock.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				b.mu.Lock()
				b.mu.Unlock()
			}
		}
	}()
	err := b.RefreshOrg(ctx, e.org)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if got := b.ListMessages(e.org, e.hh, e.sw, "x"); len(got) != 1 {
		t.Fatalf("messages after contended reload = %d", len(got))
	}
}

func TestCov1xOrgCacheLimitEvictsIdleClubsAndReloadsOnDemand(t *testing.T) {
	e := cov1xNewEnv(t)
	org2, hh2, sw2 := cov1xSeedOrg(t, e.pool)
	e.send(t, uuid.NewString(), "one")
	if _, err := e.svc.SendMessage(CreateMessageInput{OrgID: org2, HouseholdID: hh2, SwimmerID: sw2, Body: "two", SenderUserID: uuid.NewString(), SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}
	b := e.other()
	b.SetOrgCacheLimits(time.Hour, 1)
	if got := b.ListMessages(e.org, e.hh, e.sw, "x"); len(got) != 1 {
		t.Fatalf("club one = %+v", got)
	}
	if got := b.ListMessages(org2, hh2, sw2, "x"); len(got) != 1 {
		t.Fatalf("club two = %+v", got)
	}
	b.mu.RLock()
	cached := len(b.messages)
	b.mu.RUnlock()
	if cached != 1 {
		t.Fatalf("cache holds %d clubs' messages, want only the most recent club", cached)
	}
	// The evicted club is transparently reloaded.
	if got := b.ListMessages(e.org, e.hh, e.sw, "x"); len(got) != 1 || got[0].Body != "one" {
		t.Fatalf("evicted club reload = %+v", got)
	}
}
