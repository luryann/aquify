package comms

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/postmark"
)

// cov1xClient is a Postmark boundary fake layered over the stub client: it can
// reject chosen recipients, fail or short-change whole batches, and run a hook
// while a batch is "in flight" (to change database state mid-send).
type cov1xClient struct {
	inner        *postmark.StubClient
	onBatch      func(call int)
	calls        int
	batchErr     error
	short        bool
	rejectPrefix string
}

func newCov1xClient() *cov1xClient { return &cov1xClient{inner: postmark.NewStubClient()} }

func (c *cov1xClient) Ready(ctx context.Context) error { return c.inner.Ready(ctx) }
func (c *cov1xClient) SendEmail(ctx context.Context, r postmark.SendEmailRequest) (postmark.SendEmailResponse, error) {
	return c.inner.SendEmail(ctx, r)
}

func (c *cov1xClient) SendBatch(ctx context.Context, reqs []postmark.SendEmailRequest) ([]postmark.BatchResult, error) {
	c.calls++
	if c.onBatch != nil {
		c.onBatch(c.calls)
	}
	if c.batchErr != nil {
		return nil, c.batchErr
	}
	res, err := c.inner.SendBatch(ctx, reqs)
	if err != nil {
		return nil, err
	}
	for i, r := range reqs {
		if c.rejectPrefix != "" && strings.HasPrefix(r.To, c.rejectPrefix) {
			res[i].Err = errors.New("mailbox rejected")
		}
	}
	if c.short {
		res = res[:len(res)-1]
	}
	return res, nil
}

// cov1xFailWhen makes row-level writes matching cond fail inside Postgres. cond may
// only reference NEW for INSERT triggers.
func cov1xFailWhen(t *testing.T, pool *pgxpool.Pool, name, table, ops, cond string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION cov1x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x injected failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER cov1x_when_`+name+` BEFORE `+ops+` ON `+table+` FOR EACH ROW WHEN (`+cond+`) EXECUTE FUNCTION cov1x_fail()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS cov1x_when_`+name+` ON `+table) })
}

// cov1xSkipUpdatesWhen silently turns matching UPDATEs into no-ops, as if another
// replica had changed the row first.
func cov1xSkipUpdatesWhen(t *testing.T, pool *pgxpool.Pool, name, table, cond string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE OR REPLACE FUNCTION cov1x_skip() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER cov1x_skip_`+name+` BEFORE UPDATE ON `+table+` FOR EACH ROW WHEN (`+cond+`) EXECUTE FUNCTION cov1x_skip()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS cov1x_skip_`+name+` ON `+table) })
}

func cov1xRecipients(n int) []Recipient {
	out := make([]Recipient, n)
	for i := range out {
		out[i] = Recipient{Email: fmt.Sprintf("r%03d@example.com", i)}
	}
	return out
}

// cov1xCaptureLogs routes slog's default logger into a buffer for the test.
func cov1xCaptureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&cov1xLockedWriter{w: &buf}, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

type cov1xLockedWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (l *cov1xLockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (e *cov1xEnv) draftCampaign(t *testing.T) *EmailCampaign {
	t.Helper()
	c, err := e.svc.CreateCampaign(CreateCampaignInput{OrgID: e.org, Name: "n", Subject: "s", BodyHTML: "<p>x</p>", BodyText: "x", FromName: "Club", FromEmail: "club@example.com", Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *cov1xEnv) campaignStatus(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(context.Background(), `SELECT status FROM email_campaigns WHERE id = $1::uuid`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCov1xDBCampaignSendOutcomes(t *testing.T) {
	e := cov1xNewEnv(t)
	cl := newCov1xClient()
	cl.rejectPrefix = "fail"
	e.svc.SetPostmarkClient(cl)
	if _, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "sup@example.com", Reason: "manual"}); err != nil {
		t.Fatal(err)
	}
	c := e.draftCampaign(t)
	actor := uuid.NewString()
	got, err := e.svc.SendCampaign(context.Background(), e.org, c.ID, SendCampaignInput{ActorID: actor, Recipients: []Recipient{
		{Email: "ok1@example.com"}, {Email: " OK2@example.com "}, {Email: "fail1@example.com"}, {Email: "sup@example.com"}, {Email: "   "},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "sent" || got.SentCount != 2 || got.FailedCount != 1 || got.SuppressedCount != 1 || got.TotalRecipients != 4 || got.SentAt == nil || got.UpdatedBy != actor {
		t.Fatalf("campaign = %+v", got)
	}
	if len(cl.inner.Sent()) != 3 { // ok1, ok2 and the recipient whose mailbox the provider rejects
		t.Fatalf("provider received %d messages, want 3", len(cl.inner.Sent()))
	}
	for _, m := range cl.inner.Sent() {
		if m.Req.Metadata["campaign_id"] != c.ID || m.Req.Metadata["org_id"] != e.org || m.Req.FromEmail != "club@example.com" {
			t.Fatalf("request = %+v", m.Req)
		}
	}
	ds, err := e.other().ListDeliveries(e.org, c.ID)
	if err != nil || len(ds) != 4 {
		t.Fatalf("deliveries = %d err=%v", len(ds), err)
	}
	by := map[string]*EmailDelivery{}
	for _, d := range ds {
		by[d.RecipientEmail] = d
	}
	if d := by["fail1@example.com"]; d.Status != "failed" || d.ErrorMessage != "mailbox rejected" {
		t.Fatalf("failed delivery = %+v", d)
	}
	if d := by["sup@example.com"]; d.Status != "suppressed" {
		t.Fatalf("suppressed delivery = %+v", d)
	}
	if d := by["ok2@example.com"]; d.Status != "sent" || d.ProviderMessageID == "" || d.SentAt == nil {
		t.Fatalf("sent delivery = %+v", d)
	}
	if _, err := e.svc.ListDeliveries(e.org, uuid.NewString()); !errors.Is(err, ErrCampaignNotFound) {
		t.Fatalf("unknown campaign err = %v", err)
	}
}

func TestCov1xDBCampaignAllSuppressedAndBatchFailures(t *testing.T) {
	base := cov1xNewEnv(t) // one schema shared by the subtests; each uses its own club
	t.Run("everything suppressed is a completed send", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		e.svc.SetPostmarkClient(cl)
		if _, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "s@example.com", Reason: "bounce"}); err != nil {
			t.Fatal(err)
		}
		got, err := e.svc.SendCampaign(context.Background(), e.org, e.draftCampaign(t).ID, SendCampaignInput{Recipients: []Recipient{{Email: "s@example.com"}}})
		if err != nil || got.Status != "sent" || got.SentCount != 0 || got.FailedCount != 0 || got.SuppressedCount != 1 || cl.calls != 0 {
			t.Fatalf("got=%+v err=%v batches=%d", got, err, cl.calls)
		}
	})
	t.Run("transport failure fails every recipient", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		cl.batchErr = errors.New("postmark down")
		e.svc.SetPostmarkClient(cl)
		got, err := e.svc.SendCampaign(context.Background(), e.org, e.draftCampaign(t).ID, SendCampaignInput{Recipients: cov1xRecipients(2)})
		if err != nil || got.Status != "failed" || got.FailedCount != 2 || got.SentCount != 0 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		ds, _ := e.svc.ListDeliveries(e.org, got.ID)
		for _, d := range ds {
			if d.Status != "failed" || d.ErrorMessage != "postmark down" {
				t.Fatalf("delivery = %+v", d)
			}
		}
	})
	t.Run("provider returning too few results fails the batch", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		cl.short = true
		e.svc.SetPostmarkClient(cl)
		got, err := e.svc.SendCampaign(context.Background(), e.org, e.draftCampaign(t).ID, SendCampaignInput{Recipients: cov1xRecipients(2)})
		if err != nil || got.Status != "failed" || got.FailedCount != 2 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		ds, _ := e.svc.ListDeliveries(e.org, got.ID)
		if len(ds) != 2 || !strings.Contains(ds[0].ErrorMessage, "batch returned 1 results for 2 messages") {
			t.Fatalf("deliveries = %+v", ds)
		}
	})
}

func TestCov1xDBCampaignRetryReusesDeliveryRows(t *testing.T) {
	e := cov1xNewEnv(t)
	cl := newCov1xClient()
	e.svc.SetPostmarkClient(cl)
	c := e.draftCampaign(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seed := func(email, status string) *EmailDelivery {
		d := &EmailDelivery{ID: uuid.NewString(), OrgID: e.org, CampaignID: c.ID, RecipientEmail: email, Status: status, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
		if err := e.svc.repo.UpsertDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	seed("sent@example.com", "sent")
	seed("supp@example.com", "suppressed")
	seed("failed@example.com", "failed")
	pend := seed("pending@example.com", "pending")
	// A crashed sender left the campaign "sending" with no heartbeat.
	if _, err := e.pool.Exec(ctx, `UPDATE email_campaigns SET status = 'sending', updated_at = now() - interval '1 hour' WHERE id = $1::uuid`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
		t.Fatalf("stale reclaim: %v", err)
	}
	got, err := e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{Recipients: []Recipient{
		{Email: "sent@example.com"}, {Email: "supp@example.com"}, {Email: "failed@example.com"}, {Email: "pending@example.com"}, {Email: "new@example.com"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "sent" || got.SentCount != 4 || got.SuppressedCount != 1 || got.FailedCount != 0 || got.TotalRecipients != 5 {
		t.Fatalf("campaign = %+v", got)
	}
	var mailed []string
	for _, m := range cl.inner.Sent() {
		mailed = append(mailed, m.Req.To)
	}
	if len(mailed) != 3 {
		t.Fatalf("mailed %v; terminal rows must not be re-sent", mailed)
	}
	// The pending row is reused; a failed attempt is kept as history next to its retry.
	if n := e.count(t, `SELECT count(*) FROM email_deliveries WHERE campaign_id = $1::uuid`, c.ID); n != 6 {
		t.Fatalf("delivery rows = %d, want 6", n)
	}
	if n := e.count(t, `SELECT count(*) FROM email_deliveries WHERE campaign_id = $1::uuid AND recipient_email = 'failed@example.com' AND status IN ('failed','sent')`, c.ID); n != 2 {
		t.Fatalf("failed recipient should have its failed row plus a sent retry, got %d", n)
	}
	var status string
	if err := e.pool.QueryRow(ctx, `SELECT status FROM email_deliveries WHERE id = $1::uuid`, pend.ID).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("pending row not reused: %q %v", status, err)
	}
}

func TestCov1xDBCampaignQuotaRevertsToDraft(t *testing.T) {
	e := cov1xNewEnv(t)
	cl := newCov1xClient()
	e.svc.SetPostmarkClient(cl)
	e.svc.SetDailyEmailCap(1)
	c := e.draftCampaign(t)
	_, err := e.svc.SendCampaign(context.Background(), e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(2)})
	if !errors.Is(err, ErrEmailQuotaExceeded) {
		t.Fatalf("err = %v", err)
	}
	if got := e.campaignStatus(t, c.ID); got != "draft" || cl.calls != 0 {
		t.Fatalf("status=%s batches=%d", got, cl.calls)
	}
	e.svc.SetDailyEmailCap(0)
	if got, err := e.svc.SendCampaign(context.Background(), e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(2)}); err != nil || got.SentCount != 2 {
		t.Fatalf("send after lifting cap: %+v %v", got, err)
	}
}

func TestCov1xDBCampaignSendPersistenceFailures(t *testing.T) {
	base := cov1xNewEnv(t) // one schema shared by the subtests; each uses its own club
	ctx := context.Background()
	run := func(t *testing.T, e *cov1xEnv, c *EmailCampaign, rcpts []Recipient) (*EmailCampaign, error) {
		t.Helper()
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
			t.Fatal(err)
		}
		return e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{ActorID: "actor", Recipients: rcpts})
	}

	t.Run("suppressed delivery cannot be saved", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		e.svc.SetPostmarkClient(cl)
		if _, err := e.svc.AddSuppression(AddSuppressionInput{OrgID: e.org, Email: "sup@example.com", Reason: "manual"}); err != nil {
			t.Fatal(err)
		}
		c := e.draftCampaign(t)
		cov1xFailWhen(t, e.pool, "supp", "email_deliveries", "INSERT OR UPDATE", `NEW.status = 'suppressed'`)
		_, err := run(t, e, c, []Recipient{{Email: "sup@example.com"}})
		cov1xWantErr(t, err, "persist email delivery")
		if got := e.campaignStatus(t, c.ID); got != "draft" {
			t.Fatalf("campaign must return to draft, is %s", got)
		}
	})

	t.Run("pending marker cannot be saved so nothing is mailed", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		e.svc.SetPostmarkClient(cl)
		c := e.draftCampaign(t)
		cov1xFailWhen(t, e.pool, "pend", "email_deliveries", "INSERT OR UPDATE", `NEW.status = 'pending'`)
		_, err := run(t, e, c, cov1xRecipients(2))
		cov1xWantErr(t, err, "persist email delivery")
		if len(cl.inner.Sent()) != 0 || cl.calls != 0 {
			t.Fatalf("mailed %d despite the marker failing", len(cl.inner.Sent()))
		}
		if got := e.campaignStatus(t, c.ID); got != "draft" {
			t.Fatalf("status = %s", got)
		}
	})

	t.Run("sent outcome cannot be saved", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		e.svc.SetPostmarkClient(cl)
		c := e.draftCampaign(t)
		cov1xFailWhen(t, e.pool, "sent", "email_deliveries", "INSERT OR UPDATE", `NEW.status = 'sent'`)
		_, err := run(t, e, c, cov1xRecipients(1))
		cov1xWantErr(t, err, "persist email delivery")
		if len(cl.inner.Sent()) != 1 { // at-least-once: the mail left, the retry may duplicate it
			t.Fatalf("sent = %d", len(cl.inner.Sent()))
		}
		if got := e.campaignStatus(t, c.ID); got != "draft" {
			t.Fatalf("status = %s", got)
		}
	})

	t.Run("revert failure is reported alongside the send error", func(t *testing.T) {
		logs := cov1xCaptureLogs(t)
		e := base.sibling(t)
		e.svc.SetPostmarkClient(newCov1xClient())
		c := e.draftCampaign(t)
		cov1xFailWhen(t, e.pool, "pend", "email_deliveries", "INSERT OR UPDATE", `NEW.status = 'pending'`)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
			t.Fatal(err)
		}
		cov1xFailWhen(t, e.pool, "draft", "email_campaigns", "INSERT OR UPDATE", `NEW.status = 'draft'`)
		_, err := e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(1)})
		if err == nil || !strings.Contains(err.Error(), "persist email delivery") || !strings.Contains(err.Error(), "reverting campaign status also failed") {
			t.Fatalf("err = %v", err)
		}
		if got := e.campaignStatus(t, c.ID); got != "sending" {
			t.Fatalf("status = %s; it must stay sending until the stale-claim window elapses", got)
		}
		if !strings.Contains(logs.String(), "failed to revert campaign to draft") {
			t.Fatalf("logs = %s", logs.String())
		}
	})

	t.Run("heartbeat failure is logged and the send still completes", func(t *testing.T) {
		logs := cov1xCaptureLogs(t)
		e := base.sibling(t)
		e.svc.SetPostmarkClient(newCov1xClient())
		c := e.draftCampaign(t)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
			t.Fatal(err)
		}
		cov1xFailWhen(t, e.pool, "hb", "email_campaigns", "UPDATE", `OLD.status = 'sending' AND NEW.status = 'sending'`)
		got, err := e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(1)})
		if err != nil || got.Status != "sent" || got.SentCount != 1 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if !strings.Contains(logs.String(), "campaign send heartbeat failed") {
			t.Fatalf("logs = %s", logs.String())
		}
	})

	t.Run("final campaign status cannot be saved", func(t *testing.T) {
		e := base.sibling(t)
		e.svc.SetPostmarkClient(newCov1xClient())
		c := e.draftCampaign(t)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
			t.Fatal(err)
		}
		cov1xFailWhen(t, e.pool, "final", "email_campaigns", "INSERT OR UPDATE", `NEW.status IN ('sent', 'failed')`)
		_, err := e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(1)})
		cov1xWantErr(t, err, "persist sent campaign")
	})
}

func TestCov1xDBCampaignSecondBatchRechecksSuppression(t *testing.T) {
	base := cov1xNewEnv(t) // one schema shared by the subtests; each uses its own club
	ctx := context.Background()
	t.Run("suppression list unreadable before the second batch", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		cl.onBatch = func(call int) {
			if call == 1 {
				cov1xBreak(t, e.pool, "suppression_list", "reason")
			}
		}
		e.svc.SetPostmarkClient(cl)
		c := e.draftCampaign(t)
		_, err := e.svc.SendCampaign(ctx, e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(postmark.MaxBatchSize + 1)})
		if err == nil || !strings.Contains(err.Error(), "suppression_list") {
			t.Fatalf("err = %v", err)
		}
		if cl.calls != 1 {
			t.Fatalf("second batch must not be mailed, batches=%d", cl.calls)
		}
		if got := e.campaignStatus(t, c.ID); got != "draft" {
			t.Fatalf("status = %s", got)
		}
	})

	t.Run("bounce handled elsewhere stops the second batch", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		last := fmt.Sprintf("r%03d@example.com", postmark.MaxBatchSize)
		cl.onBatch = func(call int) {
			if call == 1 { // another instance processes a bounce for the straggler
				if err := e.other().ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: last, RecordType: "Bounce"}); err != nil {
					t.Error(err)
				}
			}
		}
		e.svc.SetPostmarkClient(cl)
		c := e.draftCampaign(t)
		got, err := e.svc.SendCampaign(ctx, e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(postmark.MaxBatchSize + 1)})
		if err != nil {
			t.Fatal(err)
		}
		if got.SentCount != postmark.MaxBatchSize || got.SuppressedCount != 1 || cl.calls != 1 || len(cl.inner.Sent()) != postmark.MaxBatchSize {
			t.Fatalf("campaign=%+v batches=%d sent=%d", got, cl.calls, len(cl.inner.Sent()))
		}
	})

	t.Run("suppressed straggler cannot be saved", func(t *testing.T) {
		e := base.sibling(t)
		cl := newCov1xClient()
		last := fmt.Sprintf("r%03d@example.com", postmark.MaxBatchSize)
		cl.onBatch = func(call int) {
			if call == 1 {
				if err := e.other().ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: last, RecordType: "Bounce"}); err != nil {
					t.Error(err)
				}
			}
		}
		e.svc.SetPostmarkClient(cl)
		c := e.draftCampaign(t)
		cov1xFailWhen(t, e.pool, "supp", "email_deliveries", "INSERT OR UPDATE", `NEW.status = 'suppressed'`)
		_, err := e.svc.SendCampaign(ctx, e.org, c.ID, SendCampaignInput{Recipients: cov1xRecipients(postmark.MaxBatchSize + 1)})
		cov1xWantErr(t, err, "persist email delivery")
	})
}

func TestCov1xDBClaimAndReleaseCampaign(t *testing.T) {
	base := cov1xNewEnv(t) // one schema shared by the subtests; each uses its own club
	ctx := context.Background()
	t.Run("claim write failure", func(t *testing.T) {
		e := base.sibling(t)
		c := e.draftCampaign(t)
		cov1xFailOn(t, e.pool, "email_campaigns", "UPDATE")
		_, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor")
		cov1xWantErr(t, err, "claim campaign for send")
		if got := e.campaignStatus(t, c.ID); got != "draft" {
			t.Fatalf("status = %s", got)
		}
	})

	t.Run("losing the database claim race", func(t *testing.T) {
		e := base.sibling(t)
		c := e.draftCampaign(t)
		// The row is claimed by another replica between our read and our UPDATE.
		cov1xSkipUpdatesWhen(t, e.pool, "claim", "email_campaigns", `NEW.status = 'sending'`)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); !errors.Is(err, ErrCampaignNotDraft) {
			t.Fatalf("err = %v", err)
		}
		if got := e.svc.campaigns[c.ID].Status; got != "draft" {
			t.Fatalf("cache status = %s", got)
		}
	})

	t.Run("unknown campaign", func(t *testing.T) {
		e := base.sibling(t)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, uuid.NewString(), "actor"); !errors.Is(err, ErrCampaignNotFound) {
			t.Fatalf("err = %v", err)
		}
		if _, err := e.svc.RunCampaignSend(ctx, e.org, uuid.NewString(), SendCampaignInput{}); !errors.Is(err, ErrCampaignNotFound) {
			t.Fatalf("run err = %v", err)
		}
	})

	t.Run("run requires a claim", func(t *testing.T) {
		e := base.sibling(t)
		c := e.draftCampaign(t)
		if _, err := e.svc.RunCampaignSend(ctx, e.org, c.ID, SendCampaignInput{}); !errors.Is(err, ErrCampaignNotSending) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("release returns the campaign to draft", func(t *testing.T) {
		e := base.sibling(t)
		c := e.draftCampaign(t)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
			t.Fatal(err)
		}
		cause := errors.New("queue unavailable")
		if err := e.svc.ReleaseCampaignClaim(e.org, c.ID, "actor", cause); err != cause {
			t.Fatalf("err = %v, want the original cause", err)
		}
		if got := e.campaignStatus(t, c.ID); got != "draft" {
			t.Fatalf("db status = %s", got)
		}
		if got := e.svc.campaigns[c.ID].Status; got != "draft" {
			t.Fatalf("cache status = %s", got)
		}
		if err := e.svc.ReleaseCampaignClaim(e.org, "unknown", "actor", cause); err != cause {
			t.Fatalf("unknown campaign err = %v", err)
		}
	})

	t.Run("release that cannot persist says so", func(t *testing.T) {
		cov1xCaptureLogs(t)
		e := base.sibling(t)
		c := e.draftCampaign(t)
		if _, err := e.svc.ClaimCampaign(ctx, e.org, c.ID, "actor"); err != nil {
			t.Fatal(err)
		}
		cov1xFailWhen(t, e.pool, "draft", "email_campaigns", "INSERT OR UPDATE", `NEW.status = 'draft'`)
		err := e.svc.ReleaseCampaignClaim(e.org, c.ID, "actor", errors.New("queue unavailable"))
		if err == nil || !strings.Contains(err.Error(), "queue unavailable") || !strings.Contains(err.Error(), "reverting campaign status also failed") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCov1xDBPostmarkEvents(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	e.svc.SetPostmarkClient(newCov1xClient())
	c := e.draftCampaign(t)
	if _, err := e.svc.SendCampaign(ctx, e.org, c.ID, SendCampaignInput{Recipients: []Recipient{{Email: "a@example.com"}, {Email: "b@example.com"}, {Email: "c@example.com"}}}); err != nil {
		t.Fatal(err)
	}
	ds, err := e.svc.ListDeliveries(e.org, c.ID)
	if err != nil || len(ds) != 3 {
		t.Fatalf("deliveries = %d err=%v", len(ds), err)
	}
	msgID := map[string]string{}
	for _, d := range ds {
		msgID[d.RecipientEmail] = d.ProviderMessageID
	}
	status := func(email string) (string, string) {
		var st, em string
		if err := e.pool.QueryRow(ctx, `SELECT status, COALESCE(error_message,'') FROM email_deliveries WHERE recipient_email = $1`, email).Scan(&st, &em); err != nil {
			t.Fatal(err)
		}
		return st, em
	}
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	// A fresh replica has nothing cached; the delivery is found by provider message id.
	b := e.other()
	if err := b.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["a@example.com"], RecordType: "Delivery", OccurredAt: at, Details: "ok"}); err != nil {
		t.Fatal(err)
	}
	if st, em := status("a@example.com"); st != "delivered" || em != "ok" {
		t.Fatalf("a = %s %q", st, em)
	}
	// An event naming no club cannot be attributed through the database.
	if err := e.other().ApplyPostmarkEvent(PostmarkEventInput{MessageID: msgID["a@example.com"], RecordType: "Delivery"}); !errors.Is(err, ErrDeliveryNotFound) {
		t.Fatalf("org-less err = %v", err)
	}

	// Bounce after delivery marks the delivery and suppresses the recipient.
	if err := b.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["a@example.com"], RecipientEmail: "A@example.com", RecordType: "Bounce", OccurredAt: at, Details: "mailbox full"}); err != nil {
		t.Fatal(err)
	}
	if st, em := status("a@example.com"); st != "bounced" || em != "mailbox full" {
		t.Fatalf("a after bounce = %s %q", st, em)
	}
	if err := b.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["a@example.com"], RecordType: "Delivery"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := status("a@example.com"); st != "bounced" {
		t.Fatalf("a delivery event must not erase a bounce, status = %s", st)
	}
	if err := e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["b@example.com"], RecipientEmail: "b@example.com", RecordType: "SpamComplaint"}); err != nil {
		t.Fatal(err)
	}
	sup, err := e.other().ListSuppression(e.org)
	if err != nil || len(sup) != 2 {
		t.Fatalf("suppression = %d err=%v", len(sup), err)
	}
	reasons := map[string]string{}
	for _, s := range sup {
		reasons[s.Email] = s.Reason
	}
	if reasons["a@example.com"] != "bounce" || reasons["b@example.com"] != "complaint" {
		t.Fatalf("reasons = %v", reasons)
	}
	// Re-bounce of an already suppressed address updates the entry in place.
	if err := e.svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, RecipientEmail: "b@example.com", RecordType: "Bounce"}); err != nil {
		t.Fatal(err)
	}
	if sup, _ := e.svc.ListSuppression(e.org); len(sup) != 2 {
		t.Fatalf("suppression grew to %d", len(sup))
	}
	var reason string
	if err := e.pool.QueryRow(ctx, `SELECT reason FROM suppression_list WHERE email = 'b@example.com'`).Scan(&reason); err != nil || reason != "bounce" {
		t.Fatalf("reason = %q err=%v", reason, err)
	}

	// Persistence failures surface so the webhook is retried.
	cov1xFailOn(t, e.pool, "email_deliveries", "INSERT OR UPDATE")
	fresh := e.other()
	cov1xWantErr(t, fresh.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["c@example.com"], RecordType: "Delivery"}), "persist delivered email delivery")
	cov1xWantErr(t, fresh.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["c@example.com"], RecordType: "Bounce"}), "persist bounced email delivery")
	cov1xWantErr(t, fresh.ApplyPostmarkEvent(PostmarkEventInput{OrgID: e.org, MessageID: msgID["c@example.com"], RecordType: "SpamComplaint"}), "persist complained email delivery")
}
