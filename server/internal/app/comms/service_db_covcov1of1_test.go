package comms

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xEnv is a DB-backed comms service on a private schema with one seeded
// household and swimmer (messages FK to both).
type cov1xEnv struct {
	svc  *Service
	pool *pgxpool.Pool
	org  string
	hh   string
	sw   string
}

func cov1xNewEnv(t *testing.T) *cov1xEnv {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	e := &cov1xEnv{svc: NewServiceWithPool(pool), pool: pool, org: uuid.NewString(), hh: uuid.NewString(), sw: uuid.NewString()}
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1::uuid, $2::uuid, 'Cov1x Household')`, e.hh, e.org); err != nil {
		t.Fatalf("seed household: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Test', 'Swimmer', '2012-01-01', 'female')`, e.sw, e.org, e.hh); err != nil {
		t.Fatalf("seed swimmer: %v", err)
	}
	return e
}

// sibling returns a new club (and service instance) on the same schema.
func (e *cov1xEnv) sibling(t *testing.T) *cov1xEnv {
	t.Helper()
	org, hh, sw := cov1xSeedOrg(t, e.pool)
	return &cov1xEnv{svc: NewServiceWithPool(e.pool), pool: e.pool, org: org, hh: hh, sw: sw}
}

// other returns a second service instance on the same database (a second API replica).
func (e *cov1xEnv) other() *Service { return NewServiceWithPool(e.pool) }

func (e *cov1xEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (e *cov1xEnv) send(t *testing.T, sender, body string, atts ...AttachmentInput) *Message {
	t.Helper()
	m, err := e.svc.SendMessage(CreateMessageInput{OrgID: e.org, HouseholdID: e.hh, SwimmerID: e.sw, Body: body, SenderUserID: sender, SenderRole: "coach", Attachments: atts})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	return m
}

// cov1xFailOn makes every statement of the given kinds ("INSERT OR UPDATE", "DELETE")
// against table fail inside Postgres, so the repository's error paths run for real.
func cov1xFailOn(t *testing.T, pool *pgxpool.Pool, table, ops string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION cov1x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x injected failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	name := "cov1x_fail_" + strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(ops), " ", "_"), "_or_", "_")
	if _, err := pool.Exec(ctx, `CREATE TRIGGER `+name+` BEFORE `+ops+` ON `+table+` FOR EACH STATEMENT EXECUTE FUNCTION cov1x_fail()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+name+` ON `+table) })
}

func cov1xClearFail(t *testing.T, pool *pgxpool.Pool, table, ops string) {
	t.Helper()
	name := "cov1x_fail_" + strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(ops), " ", "_"), "_or_", "_")
	if _, err := pool.Exec(context.Background(), `DROP TRIGGER `+name+` ON `+table); err != nil {
		t.Fatal(err)
	}
}

func cov1xWantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) || !strings.Contains(err.Error(), "cov1x injected failure") {
		t.Fatalf("err = %v, want injected failure wrapped with %q", err, substr)
	}
}

type cov1xBrokenStore struct{ storage.ObjectStorage }

func (cov1xBrokenStore) PresignedGetURL(string, time.Duration) (string, error) {
	return "", errors.New("signing unavailable")
}

func TestCov1xDBMessageLifecyclePersistsAcrossInstances(t *testing.T) {
	e := cov1xNewEnv(t)
	coach, parent, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.svc.SetStorage(storage.NewStubStorage("http://stub.test/bucket"))

	m := e.send(t, coach, "  hello  ",
		AttachmentInput{ObjectKey: "msgs/b.png", Filename: "b.png", MimeType: "image/png", SizeBytes: 20},
		AttachmentInput{ObjectKey: "msgs/a.png", Filename: "a.png", MimeType: "image/png", SizeBytes: 10})
	if m.Body != "hello" {
		t.Fatalf("body not trimmed: %q", m.Body)
	}
	if err := e.svc.AddReaction(e.org, m.ID, parent, "👍"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AddReaction(e.org, m.ID, parent, "👍"); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := e.svc.AddReaction(e.org, m.ID, coach, "❤️"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveReaction(e.org, m.ID, coach, "❤️"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.EditMessage(e.org, m.ID, coach, " edited "); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.MarkThreadRead(e.org, e.hh, e.sw, parent); err != nil {
		t.Fatal(err)
	}
	firstReadID := ""
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM message_reads`).Scan(&firstReadID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.MarkThreadRead(e.org, e.hh, e.sw, parent); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM message_reads WHERE id = $1::uuid`, firstReadID); n != 1 {
		t.Fatalf("re-reading a thread must update the same watermark row, got %d", n)
	}

	b := e.other()
	b.SetStorage(storage.NewStubStorage("http://stub.test/bucket"))
	views := b.ListMessages(e.org, e.hh, e.sw, parent)
	if len(views) != 1 {
		t.Fatalf("replica sees %d messages", len(views))
	}
	v := views[0]
	if v.Body != "edited" || v.EditedAt == nil {
		t.Fatalf("edit not persisted: %+v", v.Message)
	}
	if len(v.Attachments) != 2 || v.Attachments[0].Filename != "a.png" || v.Attachments[1].Filename != "b.png" ||
		!strings.Contains(v.Attachments[0].DownloadURL, "msgs%2Fa.png") {
		t.Fatalf("attachments = %+v", v.Attachments)
	}
	if len(v.Reactions) != 1 || v.Reactions[0].Emoji != "👍" || v.Reactions[0].Count != 1 || !v.Reactions[0].ReactedByMe {
		t.Fatalf("reactions = %+v", v.Reactions)
	}
	if len(b.messageEdits) != 1 {
		t.Fatalf("edit history = %d", len(b.messageEdits))
	}
	for _, edit := range b.messageEdits {
		if edit.PreviousBody != "hello" || edit.EditedBy != coach {
			t.Fatalf("edit = %+v", edit)
		}
	}
	if hh, sw, ok := b.MessageThread(e.org, m.ID); !ok || hh != e.hh || sw != e.sw {
		t.Fatalf("MessageThread = %q %q %v", hh, sw, ok)
	}
	if _, _, ok := b.MessageThread(uuid.NewString(), m.ID); ok {
		t.Fatal("MessageThread must not resolve across orgs")
	}

	threads := b.ListMessageThreads(e.org, stranger)
	if len(threads) != 1 || threads[0].MessageCount != 1 || threads[0].UnreadCount != 1 || threads[0].LastBody != "edited" || threads[0].LastSenderRole != "coach" {
		t.Fatalf("stranger threads = %+v", threads)
	}
	if got := b.ListMessageThreads(e.org, parent); got[0].UnreadCount != 0 {
		t.Fatalf("parent already read the thread, unread = %d", got[0].UnreadCount)
	}
	if got := b.ListMessageThreads(e.org, coach); got[0].UnreadCount != 0 {
		t.Fatalf("sender's own message must not count as unread, got %d", got[0].UnreadCount)
	}

	// Right of access export mirrors what is stored.
	exp := b.ExportHouseholdData(e.org, e.hh)
	if len(exp.Messages) != 1 || len(exp.MessageEdits) != 1 || len(exp.MessageAttachments) != 2 || len(exp.MessageReads) != 1 {
		t.Fatalf("export = %+v", exp)
	}
	keys, err := b.HouseholdObjectKeys(context.Background(), e.org, e.hh)
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys = %v err=%v", keys, err)
	}

	erased, err := b.EraseHouseholdData(context.Background(), e.org, e.hh, nil)
	if err != nil || len(erased) != 2 {
		t.Fatalf("erase keys = %v err=%v", erased, err)
	}
	if n := e.count(t, `SELECT (SELECT count(*) FROM messages) + (SELECT count(*) FROM message_edits) + (SELECT count(*) FROM message_reactions) + (SELECT count(*) FROM message_attachments) + (SELECT count(*) FROM message_reads)`); n != 0 {
		t.Fatalf("%d comms rows remain after erase", n)
	}
	if len(b.messages)+len(b.messageEdits)+len(b.messageReactions)+len(b.messageAttachments)+len(b.messageReads) != 0 {
		t.Fatal("cache not evicted after erase")
	}
}

func TestCov1xAttachmentURLOmittedWhenSigningFails(t *testing.T) {
	s := NewService()
	s.SetStorage(cov1xBrokenStore{})
	m, err := s.SendMessage(CreateMessageInput{OrgID: "o", HouseholdID: "h", SwimmerID: "s", Body: "x", SenderUserID: "u", SenderRole: "coach",
		Attachments: []AttachmentInput{{ObjectKey: "k", Filename: "f.png", MimeType: "image/png", SizeBytes: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	views := s.ListMessages("o", "h", "s", "u")
	if len(views) != 1 || views[0].ID != m.ID || len(views[0].Attachments) != 1 || views[0].Attachments[0].DownloadURL != "" {
		t.Fatalf("views = %+v", views)
	}
}

func TestCov1xDBAnnouncementLifecycle(t *testing.T) {
	e := cov1xNewEnv(t)
	actor := uuid.NewString()
	a, err := e.svc.CreateAnnouncement(CreateAnnouncementInput{OrgID: e.org, Title: " Welcome ", Body: " body ", Audience: "all", Public: true, CreatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Welcome" || a.Body != "body" || a.Status != "draft" || !a.Public {
		t.Fatalf("announcement = %+v", a)
	}
	if got := e.svc.ListPublicAnnouncements(e.org); len(got) != 0 {
		t.Fatalf("draft announcement must not be public: %+v", got)
	}
	pub, err := e.other().PublishAnnouncement(e.org, a.ID, actor)
	if err != nil || pub.Status != "published" || pub.PublishedAt == nil {
		t.Fatalf("publish = %+v err=%v", pub, err)
	}
	if got := e.svc.ListPublicAnnouncements(e.org); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("public list = %+v", got)
	}
	off, err := e.svc.SetAnnouncementPublic(e.org, a.ID, actor, false)
	if err != nil || off.Public {
		t.Fatalf("unpublic = %+v err=%v", off, err)
	}
	if got := e.other().ListPublicAnnouncements(e.org); len(got) != 0 {
		t.Fatalf("replica still lists a private announcement: %+v", got)
	}
	if _, err := e.svc.SetAnnouncementPublic(uuid.NewString(), a.ID, actor, true); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatalf("cross-org set public err = %v", err)
	}
	if _, err := e.svc.SetAnnouncementPublic(e.org, a.ID, actor, true); err != nil {
		t.Fatal(err)
	}
	if got, err := e.svc.ListAnnouncements(e.org, "published"); err != nil || len(got) != 1 {
		t.Fatalf("published filter = %d err=%v", len(got), err)
	}
	if got, err := e.svc.ListAnnouncements(e.org, "archived"); err != nil || len(got) != 0 {
		t.Fatalf("archived filter = %d err=%v", len(got), err)
	}
	arch, err := e.svc.ArchiveAnnouncement(e.org, a.ID, actor)
	if err != nil || arch.Status != "archived" || arch.ArchivedAt == nil {
		t.Fatalf("archive = %+v err=%v", arch, err)
	}
	if _, err := e.svc.ArchiveAnnouncement(e.org, a.ID, actor); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("double archive err = %v", err)
	}
	if _, err := e.svc.PublishAnnouncement(e.org, a.ID, actor); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("publish archived err = %v", err)
	}
	if _, err := e.svc.PublishAnnouncement(e.org, uuid.NewString(), actor); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatalf("publish unknown err = %v", err)
	}
	if _, err := e.svc.GetAnnouncement(e.org, uuid.NewString()); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatalf("get unknown err = %v", err)
	}
	if got, err := e.other().GetAnnouncement(e.org, a.ID); err != nil || got.Status != "archived" {
		t.Fatalf("replica get = %+v err=%v", got, err)
	}
}

func TestCov1xAnnouncementValidationAndPublicOrdering(t *testing.T) {
	s := NewService()
	if _, err := s.CreateAnnouncement(CreateAnnouncementInput{OrgID: "o", Title: "t", Body: "b", Audience: "group", GroupID: "  "}); err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("group announcement without group err = %v", err)
	}
	g, err := s.CreateAnnouncement(CreateAnnouncementInput{OrgID: "o", Title: "g", Body: "b", Audience: "group", GroupID: "G1", Public: true})
	if err != nil {
		t.Fatal(err)
	}
	if g.Public {
		t.Fatal("group announcements can never be public")
	}
	if _, err := s.SetAnnouncementPublic("o", g.ID, "u", true); err == nil || !strings.Contains(err.Error(), "only all-club") {
		t.Fatalf("set public on group err = %v", err)
	}
	if _, err := s.SetAnnouncementPublic("o", "missing", "u", true); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatalf("missing err = %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	published := func(id string, createdAt time.Time, publishedAt *time.Time) {
		s.announcements[id] = &Announcement{ID: id, OrgID: "o", Status: "published", Audience: "all", Public: true, CreatedAt: createdAt, PublishedAt: publishedAt}
	}
	later := base.Add(48 * time.Hour)
	published("old-created", base, nil)                      // falls back to CreatedAt
	published("new-published", base.Add(-time.Hour), &later) // PublishedAt wins over an older CreatedAt
	published("mid", base.Add(24*time.Hour), nil)
	got := s.ListPublicAnnouncements("o")
	if len(got) != 3 || got[0].ID != "new-published" || got[1].ID != "mid" || got[2].ID != "old-created" {
		t.Fatalf("order = %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
	if got := s.ListPublicAnnouncements("other-org"); len(got) != 0 {
		t.Fatalf("other org list = %+v", got)
	}
}

func TestCov1xDBNotificationPreferences(t *testing.T) {
	e := cov1xNewEnv(t)
	user, actor := uuid.NewString(), uuid.NewString()
	views, err := e.svc.ReplaceNotificationPreferences(e.org, user, "  "+e.hh+"  ", actor, []NotificationPreferenceSetting{
		{EventID: "announcement", EmailEnabled: false, PushEnabled: true},
		{EventID: " invoice_due ", EmailEnabled: true, PushEnabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]NotificationPreferenceView{}
	for _, v := range views {
		byID[v.EventID] = v
	}
	if v := byID["announcement"]; v.EmailEnabled || !v.PushEnabled {
		t.Fatalf("announcement = %+v", v)
	}
	if v := byID["invoice_due"]; !v.EmailEnabled || v.PushEnabled {
		t.Fatalf("invoice_due = %+v", v)
	}
	if v := byID["meet_commitment"]; !v.EmailEnabled || !v.PushEnabled { // untouched default
		t.Fatalf("default changed: %+v", v)
	}
	// Update an existing row (copy path) and verify the household was trimmed.
	if _, err := e.svc.ReplaceNotificationPreferences(e.org, user, e.hh, actor, []NotificationPreferenceSetting{{EventID: "announcement", EmailEnabled: true, PushEnabled: true}}); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM notification_preferences WHERE user_id = $1 AND household_id = $2 AND updated_by = $3`, user, e.hh, actor); n != 2 {
		t.Fatalf("persisted rows = %d", n)
	}
	got := e.other().ListNotificationPreferences(e.org, user)
	for _, v := range got {
		if v.EventID == "announcement" && (!v.EmailEnabled || !v.PushEnabled) {
			t.Fatalf("replica sees stale preference: %+v", v)
		}
	}

	// Validation rejects the whole request without writing anything.
	if _, err := e.svc.ReplaceNotificationPreferences(e.org, user, "", actor, []NotificationPreferenceSetting{{EventID: "bogus"}}); err == nil || !strings.Contains(err.Error(), "unsupported notification event") {
		t.Fatalf("unsupported err = %v", err)
	}
	if _, err := e.svc.ReplaceNotificationPreferences(e.org, user, "", actor, []NotificationPreferenceSetting{{EventID: "announcement"}, {EventID: "announcement"}}); err == nil || !strings.Contains(err.Error(), "duplicate notification event") {
		t.Fatalf("duplicate err = %v", err)
	}

	// The single-preference repository entry point upserts too.
	pref := &NotificationPreference{ID: uuid.NewString(), OrgID: e.org, UserID: "solo", EventID: "doc_reminder", EmailEnabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := e.svc.repo.UpsertNotificationPreference(context.Background(), pref); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM notification_preferences WHERE user_id = 'solo' AND household_id IS NULL AND email_enabled`); n != 1 {
		t.Fatalf("solo preference rows = %d", n)
	}
	if err := e.svc.repo.UpsertNotificationPreferences(context.Background(), nil); err != nil {
		t.Fatalf("empty batch must be a no-op: %v", err)
	}

	// A failing write rolls back the whole batch and leaves the cache alone.
	cov1xFailOn(t, e.pool, "notification_preferences", "INSERT OR UPDATE")
	_, err = e.svc.ReplaceNotificationPreferences(e.org, user, "", actor, []NotificationPreferenceSetting{{EventID: "entry_update", EmailEnabled: true, PushEnabled: false}})
	cov1xWantErr(t, err, "persist notification preferences")
	for _, v := range e.svc.ListNotificationPreferences(e.org, user) {
		if v.EventID == "entry_update" && (v.EmailEnabled || !v.PushEnabled) {
			t.Fatalf("cache changed despite failed write: %+v", v)
		}
	}
}

func TestCov1xDBWriteFailuresSurfaceAndLeaveStateUntouched(t *testing.T) {
	base := cov1xNewEnv(t) // one schema shared by the subtests; each uses its own club
	actor := uuid.NewString()

	t.Run("announcement create", func(t *testing.T) {
		e := base.sibling(t)
		cov1xFailOn(t, e.pool, "announcements", "INSERT OR UPDATE")
		_, err := e.svc.CreateAnnouncement(CreateAnnouncementInput{OrgID: e.org, Title: "t", Body: "b", Audience: "all"})
		cov1xWantErr(t, err, "persist announcement")
		if len(e.svc.announcements) != 0 {
			t.Fatal("failed create left a cached announcement")
		}
	})

	t.Run("announcement update paths", func(t *testing.T) {
		e := base.sibling(t)
		a, err := e.svc.CreateAnnouncement(CreateAnnouncementInput{OrgID: e.org, Title: "t", Body: "b", Audience: "all"})
		if err != nil {
			t.Fatal(err)
		}
		cov1xFailOn(t, e.pool, "announcements", "INSERT OR UPDATE")
		_, err = e.svc.SetAnnouncementPublic(e.org, a.ID, actor, true)
		cov1xWantErr(t, err, "persist announcement")
		_, err = e.svc.PublishAnnouncement(e.org, a.ID, actor)
		cov1xWantErr(t, err, "persist published announcement")
		_, err = e.svc.ArchiveAnnouncement(e.org, a.ID, actor)
		cov1xWantErr(t, err, "persist archived announcement")
		got, err := e.svc.GetAnnouncement(e.org, a.ID)
		if err != nil || got.Status != "draft" || got.Public {
			t.Fatalf("announcement changed despite failed writes: %+v err=%v", got, err)
		}
	})

	t.Run("message insert", func(t *testing.T) {
		e := base.sibling(t)
		cov1xFailOn(t, e.pool, "messages", "INSERT OR UPDATE")
		_, err := e.svc.SendMessage(CreateMessageInput{OrgID: e.org, HouseholdID: e.hh, SwimmerID: e.sw, Body: "x", SenderUserID: actor, SenderRole: "coach"})
		cov1xWantErr(t, err, "persist message")
		if len(e.svc.messages) != 0 {
			t.Fatal("failed send left a cached message")
		}
	})

	t.Run("attachment insert rolls the message back", func(t *testing.T) {
		e := base.sibling(t)
		cov1xFailOn(t, e.pool, "message_attachments", "INSERT OR UPDATE")
		_, err := e.svc.SendMessage(CreateMessageInput{OrgID: e.org, HouseholdID: e.hh, SwimmerID: e.sw, Body: "x", SenderUserID: actor, SenderRole: "coach",
			Attachments: []AttachmentInput{{ObjectKey: "k", Filename: "f", MimeType: "image/png", SizeBytes: 1}}})
		cov1xWantErr(t, err, "persist message attachment")
		if n := e.count(t, `SELECT count(*) FROM messages WHERE org_id = $1::uuid`, e.org); n != 0 {
			t.Fatalf("message row not rolled back: %d", n)
		}
		if len(e.svc.messages) != 0 || len(e.svc.messageAttachments) != 0 {
			t.Fatal("cache not rolled back")
		}
	})

	t.Run("edit history insert", func(t *testing.T) {
		e := base.sibling(t)
		m := e.send(t, actor, "orig")
		cov1xFailOn(t, e.pool, "message_edits", "INSERT OR UPDATE")
		_, err := e.svc.EditMessage(e.org, m.ID, actor, "new")
		cov1xWantErr(t, err, "persist message edit")
		if e.svc.messages[m.ID].Body != "orig" {
			t.Fatal("cache body changed despite failure")
		}
	})

	t.Run("edit body update rolls the history back", func(t *testing.T) {
		e := base.sibling(t)
		m := e.send(t, actor, "orig")
		cov1xFailOn(t, e.pool, "messages", "UPDATE")
		_, err := e.svc.EditMessage(e.org, m.ID, actor, "new")
		cov1xWantErr(t, err, "persist message edit")
		if n := e.count(t, `SELECT count(*) FROM message_edits WHERE org_id = $1::uuid`, e.org); n != 0 {
			t.Fatalf("edit history not rolled back: %d", n)
		}
	})

	t.Run("reaction insert", func(t *testing.T) {
		e := base.sibling(t)
		m := e.send(t, actor, "x")
		cov1xFailOn(t, e.pool, "message_reactions", "INSERT OR UPDATE")
		cov1xWantErr(t, e.svc.AddReaction(e.org, m.ID, actor, "👍"), "persist reaction")
		if len(e.svc.messageReactions) != 0 {
			t.Fatal("failed reaction cached")
		}
	})

	t.Run("reaction delete", func(t *testing.T) {
		e := base.sibling(t)
		m := e.send(t, actor, "x")
		if err := e.svc.AddReaction(e.org, m.ID, actor, "👍"); err != nil {
			t.Fatal(err)
		}
		cov1xFailOn(t, e.pool, "message_reactions", "DELETE")
		cov1xWantErr(t, e.svc.RemoveReaction(e.org, m.ID, actor, "👍"), "delete reaction")
		if len(e.svc.messageReactions) != 1 {
			t.Fatal("reaction removed from cache despite failed delete")
		}
	})

	t.Run("read watermark", func(t *testing.T) {
		e := base.sibling(t)
		cov1xFailOn(t, e.pool, "message_reads", "INSERT OR UPDATE")
		cov1xWantErr(t, e.svc.MarkThreadRead(e.org, e.hh, e.sw, actor), "persist message read")
		if len(e.svc.messageReads) != 0 {
			t.Fatal("failed watermark cached")
		}
	})

	t.Run("campaign create", func(t *testing.T) {
		e := base.sibling(t)
		cov1xFailOn(t, e.pool, "email_campaigns", "INSERT OR UPDATE")
		_, err := e.svc.CreateCampaign(CreateCampaignInput{OrgID: e.org, Name: "n", Subject: "s", BodyHTML: "x", FromName: "f", FromEmail: "f@example.com", Audience: "all"})
		cov1xWantErr(t, err, "persist campaign")
		if len(e.svc.campaigns) != 0 {
			t.Fatal("failed campaign cached")
		}
	})

	t.Run("suppression add remove and upsert", func(t *testing.T) {
		e := base.sibling(t)
		cov1xFailOn(t, e.pool, "suppression_list", "INSERT OR UPDATE")
		_, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "a@example.com", Reason: "manual", CreatedBy: actor})
		cov1xWantErr(t, err, "persist suppression entry")
		cov1xClearFail(t, e.pool, "suppression_list", "INSERT OR UPDATE")

		entry, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "A@Example.com", Reason: "manual", CreatedBy: actor})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "a@example.com", Reason: "bounce"}); !errors.Is(err, ErrDuplicateSuppression) {
			t.Fatalf("duplicate err = %v", err)
		}

		cov1xFailOn(t, e.pool, "suppression_list", "INSERT OR UPDATE")
		err = e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: "a@example.com", RecordType: "SpamComplaint"})
		cov1xWantErr(t, err, "persist suppression entry")
		cov1xClearFail(t, e.pool, "suppression_list", "INSERT OR UPDATE")

		cov1xFailOn(t, e.pool, "suppression_list", "DELETE")
		cov1xWantErr(t, e.svc.RemoveSuppression(e.org, entry.ID), "delete suppression entry")
		err = e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: "a@example.com", RecordType: "SubscriptionChange"})
		cov1xWantErr(t, err, "delete suppression entry")
		cov1xClearFail(t, e.pool, "suppression_list", "DELETE")

		if err := e.svc.RemoveSuppression(e.org, entry.ID); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.RemoveSuppression(e.org, entry.ID); !errors.Is(err, ErrSuppressionNotFound) {
			t.Fatalf("second remove err = %v", err)
		}
	})
}
