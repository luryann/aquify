package comms

import (
	"context"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

// A failure partway through a household erase must roll the whole DB erase back
// and leave the in-memory state intact; success removes both.
func TestEraseHouseholdDataIsAtomic(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org := uuid.NewString()
	hh := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO notification_preferences (org_id, user_id, household_id, event_id) VALUES ($1::uuid, 'u1', $2, 'evt')`, org, hh); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_deliveries (org_id, recipient_email) VALUES ($1::uuid, 'a@example.com')`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_delivery_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$;
		CREATE TRIGGER reject_delivery_delete BEFORE DELETE ON email_deliveries FOR EACH ROW EXECUTE FUNCTION reject_delivery_delete()`); err != nil {
		t.Fatal(err)
	}
	svc := NewServiceWithPool(pool)
	if len(svc.preferences) != 1 || len(svc.deliveries) != 1 {
		t.Fatalf("setup: prefs=%d deliveries=%d", len(svc.preferences), len(svc.deliveries))
	}

	if _, err := svc.EraseHouseholdData(ctx, org, hh, []string{"A@example.com"}); err == nil {
		t.Fatal("expected injected failure")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_preferences WHERE household_id = $1`, hh).Scan(&n); err != nil || n != 1 {
		t.Fatalf("db erase not rolled back: n=%d err=%v", n, err)
	}
	if len(svc.preferences) != 1 || len(svc.deliveries) != 1 {
		t.Fatal("memory was purged despite a failed erase")
	}

	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_delivery_delete ON email_deliveries`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EraseHouseholdData(ctx, org, hh, []string{"A@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM notification_preferences) + (SELECT count(*) FROM email_deliveries)`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows remain after erase: %d %v", n, err)
	}
	if len(svc.preferences) != 0 || len(svc.deliveries) != 0 {
		t.Fatal("memory not evicted after commit")
	}
}
