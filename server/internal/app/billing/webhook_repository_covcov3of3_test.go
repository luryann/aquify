package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov3xIsUUIDString(t *testing.T) {
	valid := []string{
		"123e4567-e89b-12d3-a456-426614174000",
		"123E4567-E89B-12D3-A456-426614174000",
		"00000000-0000-0000-0000-000000000000",
		uuid.NewString(),
	}
	for _, s := range valid {
		if !isUUIDString(s) {
			t.Fatalf("%q must be accepted", s)
		}
	}
	invalid := []string{
		"",
		"org-demo",
		"123e4567e89b12d3a456426614174000",      // no dashes, wrong length
		"123e4567-e89b-12d3-a456-42661417400",   // one short
		"123e4567-e89b-12d3-a456-4266141740000", // one long
		"123e4567_e89b-12d3-a456-426614174000",  // wrong separator at 8
		"123e4567-e89b-12d3-a456_426614174000",  // wrong separator at 23
		"123e4567-e89b-12d3-a456-42661417400g",  // non-hex digit
		"123e4567-e89b-12d3-a456-42661417400 ",  // trailing space
		"123e4567-e89b-12d3-a456-4266141740é",   // 36 bytes, non-ASCII final rune
	}
	for _, s := range invalid {
		if isUUIDString(s) {
			t.Fatalf("%q must be rejected", s)
		}
	}
}

func cov3xWebhookRec(org, eventID string, at time.Time) *WebhookEventRecord {
	return &WebhookEventRecord{
		ID: uuid.NewString(), OrgID: org, Provider: "stripe", EventID: eventID, EventType: "payment_intent.succeeded",
		Payload: json.RawMessage(`{"id":"` + eventID + `"}`), CreatedAt: at,
	}
}

func TestCov3xWebhookClaimFinishAndLoadPostgres(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	r := newRepository(pool)
	org := uuid.NewString()
	base := cov3xTime("2026-10-01T10:00:00Z")

	// Fresh delivery: claimed; a concurrent redelivery sees the active lease.
	first := cov3xWebhookRec(org, "evt_cov3x_1", base)
	if ok, err := r.claimWebhookEvent(ctx, first); err != nil || !ok {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	if ok, err := r.claimWebhookEvent(ctx, cov3xWebhookRec(org, "evt_cov3x_1", base)); ok || !errors.Is(err, ErrWebhookInFlight) {
		t.Fatalf("claim during lease: %v %v", ok, err)
	}

	// A transient failure releases the lease and allows a retry.
	failedAt := base.Add(time.Minute)
	first.ErrorMessage, first.Retryable, first.ProcessedAt = "stripe timeout", true, &failedAt
	if err := r.finishWebhookEvent(ctx, first); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.claimWebhookEvent(ctx, cov3xWebhookRec(org, "evt_cov3x_1", base)); err != nil || !ok {
		t.Fatalf("retryable failure must be re-claimable: %v %v", ok, err)
	}
	// Success is terminal.
	doneAt := base.Add(2 * time.Minute)
	first.ErrorMessage, first.Retryable, first.Processed, first.ProcessedAt = "", false, true, &doneAt
	if err := r.finishWebhookEvent(ctx, first); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.claimWebhookEvent(ctx, cov3xWebhookRec(org, "evt_cov3x_1", base)); ok || err != nil {
		t.Fatalf("processed event: %v %v", ok, err)
	}

	// A permanent failure is terminal too; the stored message is kept.
	second := cov3xWebhookRec(org, "evt_cov3x_2", base.Add(time.Hour))
	if ok, err := r.claimWebhookEvent(ctx, second); err != nil || !ok {
		t.Fatalf("second claim: %v %v", ok, err)
	}
	second.ErrorMessage, second.ProcessedAt = "invoice not found", &doneAt
	if err := r.finishWebhookEvent(ctx, second); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.claimWebhookEvent(ctx, cov3xWebhookRec(org, "evt_cov3x_2", base)); ok || err != nil {
		t.Fatalf("permanently failed event: %v %v", ok, err)
	}

	// Deliveries that arrive before the club is known are stored without an org and with an empty payload object.
	orphan := cov3xWebhookRec("org-demo", "evt_cov3x_orphan", base.Add(2*time.Hour))
	orphan.Payload = nil
	if ok, err := r.claimWebhookEvent(ctx, orphan); err != nil || !ok {
		t.Fatalf("orphan claim: %v %v", ok, err)
	}
	var payload string
	var orgIsNull bool
	if err := pool.QueryRow(ctx, `SELECT payload::text, org_id IS NULL FROM webhook_events WHERE event_id = 'evt_cov3x_orphan'`).Scan(&payload, &orgIsNull); err != nil {
		t.Fatal(err)
	}
	if payload != "{}" || !orgIsNull {
		t.Fatalf("orphan row: payload=%q org null=%v", payload, orgIsNull)
	}

	// Loading: newest first, limited, only this club's events, full fidelity.
	got, err := r.loadWebhookEvents(ctx, org, 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("events: %+v %v", got, err)
	}
	if got[0].EventID != "evt_cov3x_2" || got[1].EventID != "evt_cov3x_1" {
		t.Fatalf("order: %s, %s", got[0].EventID, got[1].EventID)
	}
	if !got[1].Processed || got[1].ProcessedAt == nil || !got[1].ProcessedAt.Equal(doneAt) || got[1].ErrorMessage != "" || got[1].OrgID != org ||
		got[1].Provider != "stripe" || got[1].EventType != "payment_intent.succeeded" || string(got[1].Payload) != `{"id": "evt_cov3x_1"}` {
		t.Fatalf("event 1: %+v payload=%s", got[1], got[1].Payload)
	}
	if got[0].Processed || got[0].ErrorMessage != "invoice not found" || got[0].Retryable {
		t.Fatalf("event 2: %+v", got[0])
	}
	if limited, err := r.loadWebhookEvents(ctx, org, 1); err != nil || len(limited) != 1 || limited[0].EventID != "evt_cov3x_2" {
		t.Fatalf("limit: %+v %v", limited, err)
	}
	if other, err := r.loadWebhookEvents(ctx, uuid.NewString(), 10); err != nil || len(other) != 0 {
		t.Fatalf("another club must see nothing: %+v %v", other, err)
	}

	// Unreachable database.
	dead := cov3xClosedPoolService(t, pool).repo
	if ok, err := dead.claimWebhookEvent(ctx, cov3xWebhookRec(org, "evt_dead", base)); ok || err == nil || !strings.Contains(err.Error(), "billing: claim webhook event") {
		t.Fatalf("claim on dead pool: %v %v", ok, err)
	}
	if err := dead.finishWebhookEvent(ctx, first); err == nil || !strings.Contains(err.Error(), "billing: finish webhook event") {
		t.Fatalf("finish on dead pool: %v", err)
	}
	if _, err := dead.loadWebhookEvents(ctx, org, 5); err == nil || !strings.Contains(err.Error(), "billing: load webhook events") {
		t.Fatalf("load on dead pool: %v", err)
	}

	// Rows the schema normally forbids (legacy data) surface as errors, not as
	// a silently wrong "in flight"/"terminal" answer.
	cov3xExec(t, pool, `ALTER TABLE webhook_events ALTER COLUMN retryable DROP NOT NULL`)
	cov3xExec(t, pool, `INSERT INTO webhook_events (id, org_id, provider, event_id, event_type, payload, processed, error_message, retryable, processing_until)
		VALUES (gen_random_uuid(), '`+org+`', 'stripe', 'evt_cov3x_legacy', 'x', '{}'::jsonb, FALSE, 'legacy failure', NULL, now() + interval '1 hour')`)
	ok, err := r.claimWebhookEvent(ctx, cov3xWebhookRec(org, "evt_cov3x_legacy", base))
	if ok || err == nil || errors.Is(err, ErrWebhookInFlight) {
		t.Fatalf("undecidable terminal state must be an error: %v %v", ok, err)
	}
	cov3xExec(t, pool, `ALTER TABLE webhook_events ALTER COLUMN payload DROP NOT NULL`)
	cov3xExec(t, pool, `INSERT INTO webhook_events (id, org_id, provider, event_id, event_type, payload, created_at)
		VALUES (gen_random_uuid(), '`+org+`', 'stripe', 'evt_cov3x_nopayload', 'x', NULL, now() + interval '1 day')`)
	if _, err := r.loadWebhookEvents(ctx, org, 10); err == nil || !strings.Contains(err.Error(), "billing: scan webhook event") {
		t.Fatalf("unreadable row must fail the load: %v", err)
	}
}
