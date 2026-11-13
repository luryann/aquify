package testutil

import (
	"context"
	"strings"
	"testing"
)

func TestCov1xIsolatedSchemaPoolAutoRegistersOrgs(t *testing.T) {
	pool := IsolatedSchemaPool(t)
	ctx := context.Background()
	org := "11111111-1111-4111-8111-111111111111"
	if _, err := pool.Exec(ctx, `INSERT INTO households (org_id, household_name) VALUES ($1::uuid, 'Cov1x')`, org); err != nil {
		t.Fatalf("insert with unregistered org should auto-register: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM orgs WHERE id = $1::uuid`, org).Scan(&n); err != nil || n != 1 {
		t.Fatalf("org auto-registered count=%d err=%v", n, err)
	}
}

func TestCov1xIsolatedSchemaPoolStrictKeepsForeignKeys(t *testing.T) {
	pool := IsolatedSchemaPoolStrict(t)
	_, err := pool.Exec(context.Background(), `INSERT INTO households (org_id, household_name) VALUES ('22222222-2222-4222-8222-222222222222', 'Cov1x')`)
	if err == nil || !strings.Contains(err.Error(), "foreign key") {
		t.Fatalf("expected foreign key violation, got %v", err)
	}
	SeedOrgs(t, pool, "22222222-2222-4222-8222-222222222222", "22222222-2222-4222-8222-222222222222")
	if _, err := pool.Exec(context.Background(), `INSERT INTO households (org_id, household_name) VALUES ('22222222-2222-4222-8222-222222222222', 'Cov1x')`); err != nil {
		t.Fatalf("insert after SeedOrgs: %v", err)
	}
}

func TestCov1xSeedOrgsFailsOnInvalidUUID(t *testing.T) {
	pool := IsolatedSchemaPoolStrict(t)
	fake := cov1xRunFake(func(ft *testing.T) { SeedOrgs(ft, pool, "not-a-uuid") })
	if !fake.Failed() {
		t.Fatal("expected SeedOrgs to fail for an invalid uuid")
	}
}

func TestCov1xIsolatedSchemaPoolBeforeAndApplyMigration(t *testing.T) {
	pool := IsolatedSchemaPoolBefore(t, "20260801000100")
	ctx := context.Background()
	var exists bool
	q := `SELECT to_regclass(current_schema() || '.orgs') IS NOT NULL`
	if err := pool.QueryRow(ctx, q).Scan(&exists); err != nil || exists {
		t.Fatalf("orgs must not exist before its migration: exists=%v err=%v", exists, err)
	}
	ApplyMigration(t, pool, "20260801000100")
	if err := pool.QueryRow(ctx, q).Scan(&exists); err != nil || !exists {
		t.Fatalf("orgs must exist after ApplyMigration: exists=%v err=%v", exists, err)
	}
}

func TestCov1xTryApplyMigrationErrors(t *testing.T) {
	pool := IsolatedSchemaPoolStrict(t)
	err := TryApplyMigration(pool, "99999999999999_nope")
	if err == nil || !strings.Contains(err.Error(), "matched 0 files") {
		t.Fatalf("expected no-match error, got %v", err)
	}
	err = TryApplyMigration(pool, "2026031500")
	if err == nil || !strings.Contains(err.Error(), "matched") {
		t.Fatalf("expected ambiguous-prefix error, got %v", err)
	}
	// households already exists in a fully migrated schema.
	err = TryApplyMigration(pool, "20260315000900")
	if err == nil || !strings.Contains(err.Error(), "migration ") {
		t.Fatalf("expected wrapped exec error, got %v", err)
	}
	fake := cov1xRunFake(func(ft *testing.T) { ApplyMigration(ft, pool, "99999999999999_nope") })
	if !fake.Failed() {
		t.Fatal("ApplyMigration must fail when the migration is missing")
	}
}

func TestCov1xIsolatedSchemaPoolAsAppRole(t *testing.T) {
	admin, app := IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	var user string
	var super, bypass bool
	if err := app.QueryRow(ctx, `SELECT current_user, r.rolsuper, r.rolbypassrls FROM pg_roles r WHERE r.rolname = current_user`).Scan(&user, &super, &bypass); err != nil {
		t.Fatal(err)
	}
	if user != RLSAppRole || super || bypass {
		t.Fatalf("app pool role=%q super=%v bypassrls=%v", user, super, bypass)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO households (org_id, household_name) VALUES ('33333333-3333-4333-8333-333333333333', 'Cov1x')`); err != nil {
		t.Fatalf("admin seed: %v", err)
	}
	var n int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM orgs`).Scan(&n); err != nil {
		t.Fatalf("app role must be granted the isolated schema: %v", err)
	}
}

func TestCov1xIsolatedSchemaPoolSkipsWithoutDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	fake := cov1xRunFake(func(ft *testing.T) { IsolatedSchemaPool(ft) })
	if !fake.Skipped() {
		t.Fatal("expected skip when DATABASE_URL is unset")
	}
}

func TestCov1xIsolatedSchemaPoolFailsOnBadDatabaseURL(t *testing.T) {
	for name, url := range map[string]string{
		"unparseable": "postgres://%zz",
		"unreachable": "postgres://postgres:postgres@127.0.0.1:1/none?sslmode=disable&connect_timeout=1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", url)
			fake := cov1xRunFake(func(ft *testing.T) { IsolatedSchemaPoolStrict(ft) })
			if !fake.Failed() || fake.Skipped() {
				t.Fatalf("expected failure, failed=%v skipped=%v", fake.Failed(), fake.Skipped())
			}
		})
	}
}
