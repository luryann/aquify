package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/testutil"
)

// Verifies the orgs registry migration (tracker item 1.1): constraints hold and
// the backfill registers every UUID-shaped org_id found in tenant tables while
// skipping legacy non-UUID values (none remain after 1.2).
func TestOrgsRegistryMigrationBackfill(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolBefore(t, "20260801000100")
	ctx := context.Background()

	const orgA = "00000000-0000-0000-0000-00000000a001"
	const orgB = "00000000-0000-0000-0000-00000000b002"
	if _, err := pool.Exec(ctx, `INSERT INTO org_settings (org_id, team_name, public_slug) VALUES ($1, 'Aqua Club', 'aqua-club'), ($2, 'Other Club', 'other-club')`, orgA, orgB); err != nil {
		t.Fatalf("seed org_settings: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events (id, org_id, actor_user_id, action, resource_type, resource_id) VALUES ('e1','org-demo','u','a','r','1'), ('e2',$1,'u','a','r','1')`, "00000000-0000-0000-0000-00000000c003"); err != nil {
		t.Fatalf("seed audit_events: %v", err)
	}

	// Re-run the migration's Up section now that data exists (idempotent by design).
	_, thisFile, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations", "20260801000100_create_orgs.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(strings.SplitN(string(raw), "-- +goose Up", 2)[1], "-- +goose Down", 2)[0]
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("re-run orgs migration: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM orgs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("want 3 registered orgs (a, b, c; legacy 'org-demo' left unregistered), got %d", n)
	}
	var slugA, slugB *string
	var nameA string
	if err := pool.QueryRow(ctx, `SELECT slug, name FROM orgs WHERE id = $1`, orgA).Scan(&slugA, &nameA); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT slug FROM orgs WHERE id = $1`, orgB).Scan(&slugB); err != nil {
		t.Fatal(err)
	}
	if nameA != "Aqua Club" {
		t.Fatalf("name not backfilled: %q", nameA)
	}
	if slugA == nil || *slugA != "aqua-club" || slugB == nil || *slugB != "other-club" {
		t.Fatalf("slugs not backfilled: a=%v b=%v", slugA, slugB)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, slug) VALUES (gen_random_uuid(), 'Bad Slug!')`); err == nil {
		t.Fatal("invalid slug must be rejected")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, status) VALUES (gen_random_uuid(), 'bogus')`); err == nil {
		t.Fatal("invalid status must be rejected")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_domains (domain, org_id) VALUES ('Club.Example', $1)`, orgA); err == nil {
		t.Fatal("non-lowercase domain must be rejected")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_domains (domain, org_id) VALUES ('club.example', gen_random_uuid())`); err == nil {
		t.Fatal("domain for an unknown org must be rejected")
	}
}
