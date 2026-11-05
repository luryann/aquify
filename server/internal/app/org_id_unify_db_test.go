package app

import (
	"context"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/testutil"
)

const demoOrgUUID = "00000000-0000-0000-0000-00000000a001"

// Tenancy tracker item 1.2: legacy TEXT org_id columns become UUID, 'org-demo'
// maps to the local-dev org UUID, and data plus constraints survive.
func TestOrgIDUnifyMigrationConvertsLegacyText(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolBefore(t, "20260801000200")
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO audit_events (id, org_id, actor_user_id, action, resource_type, resource_id)
		VALUES ('e1','org-demo','u','a','r','1'), ('e2','11111111-1111-1111-1111-111111111111','u','a','r','2')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var before string
	if err := pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name='audit_events' AND column_name='org_id'`).Scan(&before); err != nil || before != "text" {
		t.Fatalf("precondition: audit_events.org_id should be text, got %q err=%v", before, err)
	}

	testutil.ApplyMigration(t, pool, "20260801000200")

	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.columns c WHERE table_schema = current_schema() AND column_name='org_id' AND data_type <> 'uuid' AND table_name IN (SELECT table_name FROM information_schema.tables WHERE table_type='BASE TABLE' AND table_schema=current_schema())`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		t.Errorf("table %s still has a non-uuid org_id", n)
	}
	rows.Close()

	var got string
	if err := pool.QueryRow(ctx, `SELECT org_id::text FROM audit_events WHERE id='e1'`).Scan(&got); err != nil || got != demoOrgUUID {
		t.Fatalf("org-demo not mapped: %q err=%v", got, err)
	}
	if err := pool.QueryRow(ctx, `SELECT org_id::text FROM audit_events WHERE id='e2'`).Scan(&got); err != nil || got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("uuid not preserved: %q err=%v", got, err)
	}
	var idx int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'audit_events_org_time_idx'`).Scan(&idx); err != nil || idx != 1 {
		t.Fatalf("index lost: %d err=%v", idx, err)
	}
}

func TestOrgIDUnifyMigrationRejectsUnmappableValues(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolBefore(t, "20260801000200")
	if _, err := pool.Exec(context.Background(), `INSERT INTO audit_events (id, org_id, actor_user_id, action, resource_type, resource_id) VALUES ('e1','acme-club','u','a','r','1')`); err != nil {
		t.Fatal(err)
	}
	thisErr := testutil.TryApplyMigration(pool, "20260801000200")
	if thisErr == nil || !strings.Contains(thisErr.Error(), "non-UUID org_id") {
		t.Fatalf("migration must fail on a non-UUID, non-demo org_id, got %v", thisErr)
	}
}
