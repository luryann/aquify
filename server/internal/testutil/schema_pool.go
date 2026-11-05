package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IsolatedSchemaPool returns a pool bound to a private, throwaway schema in the
// DATABASE_URL database with every goose "Up" migration applied, so DB tests
// never read or write shared fixtures. It skips when DATABASE_URL is unset.
//
// Tenant tables FK to orgs(id). Most tests invent throwaway org ids, so this
// pool installs a TEST-ONLY trigger on each such table that registers the org on
// insert. Tests of org registration/FK behaviour itself must use
// IsolatedSchemaPoolStrict, which keeps the real fail-closed constraints.
func IsolatedSchemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := isolatedSchemaPool(t, "")
	installOrgAutoRegister(t, pool)
	return pool
}

// IsolatedSchemaPoolStrict is IsolatedSchemaPool without org auto-registration.
func IsolatedSchemaPoolStrict(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return isolatedSchemaPool(t, "")
}

// RLSAppRole is the non-superuser, non-BYPASSRLS role IsolatedSchemaPoolAsAppRole
// connects as, standing in for the application's production database role.
const RLSAppRole = "aquify_rls_test_app"

// IsolatedSchemaPoolAsAppRole returns two pools on one fresh isolated schema: admin
// (superuser, bypasses row-level security, with orgs auto-registered like
// IsolatedSchemaPool) for seeding, and app, whose every connection runs as a role
// that is subject to row-level security, like the production application.
func IsolatedSchemaPoolAsAppRole(t *testing.T) (admin, app *pgxpool.Pool) {
	t.Helper()
	admin = IsolatedSchemaPool(t)
	ctx := context.Background()
	if _, err := admin.Exec(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '`+RLSAppRole+`') THEN
			CREATE ROLE `+RLSAppRole+` NOLOGIN NOSUPERUSER NOBYPASSRLS;
		END IF; END $$`); err != nil {
		t.Skipf("cannot create the RLS test role (needs CREATEROLE): %v", err)
	}
	var schema string
	if err := admin.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"GRANT USAGE ON SCHEMA " + schema + " TO " + RLSAppRole,
		"GRANT ALL ON ALL TABLES IN SCHEMA " + schema + " TO " + RLSAppRole,
		"GRANT ALL ON ALL SEQUENCES IN SCHEMA " + schema + " TO " + RLSAppRole,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	cfg := admin.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+RLSAppRole)
		return err
	}
	app, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return admin, app
}

func installOrgAutoRegister(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION test_autoregister_org() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.org_id IS NOT NULL THEN
    INSERT INTO orgs (id) VALUES (NEW.org_id) ON CONFLICT (id) DO NOTHING;
  END IF;
  RETURN NEW;
END $$`); err != nil {
		t.Fatalf("install org auto-register: %v", err)
	}
	rows, err := pool.Query(ctx, `
SELECT DISTINCT conrelid::regclass::text FROM pg_constraint
WHERE contype = 'f' AND confrelid = 'orgs'::regclass AND conrelid <> 'org_domains'::regclass`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var tbl string
		if err := rows.Scan(&tbl); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, tbl)
	}
	rows.Close()
	for _, tbl := range tables {
		if _, err := pool.Exec(ctx, `CREATE TRIGGER aaa_test_autoregister_org BEFORE INSERT ON `+tbl+` FOR EACH ROW EXECUTE FUNCTION test_autoregister_org()`); err != nil {
			t.Fatalf("trigger on %s: %v", tbl, err)
		}
	}
}

// IsolatedSchemaPoolBefore is IsolatedSchemaPool but stops before the migration
// whose file name starts with stopBefore, so a test can seed legacy-shaped data
// and then apply that migration with ApplyMigration.
func IsolatedSchemaPoolBefore(t *testing.T, stopBefore string) *pgxpool.Pool {
	t.Helper()
	return isolatedSchemaPool(t, stopBefore)
}

// ApplyMigration runs the Up section of the migration whose file name starts
// with prefix against pool.
func ApplyMigration(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	if err := TryApplyMigration(pool, prefix); err != nil {
		t.Fatal(err)
	}
}

// TryApplyMigration is ApplyMigration that returns the error.
func TryApplyMigration(pool *pgxpool.Pool, prefix string) error {
	_, thisFile, _, _ := runtime.Caller(0)
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations", prefix+"*.sql"))
	if len(paths) != 1 {
		return fmt.Errorf("migration %q matched %d files", prefix, len(paths))
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		return err
	}
	up := strings.SplitN(strings.SplitN(string(data), "-- +goose Up", 2)[1], "-- +goose Down", 2)[0]
	if _, err := pool.Exec(context.Background(), up); err != nil {
		return fmt.Errorf("migration %s: %w", paths[0], err)
	}
	return nil
}

// SeedOrgs registers tenant rows so FK-protected tables accept the org ids.
func SeedOrgs(t *testing.T, pool *pgxpool.Pool, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := pool.Exec(context.Background(), `INSERT INTO orgs (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING`, id); err != nil {
			t.Fatalf("seed org %s: %v", id, err)
		}
	}
}

func isolatedSchemaPool(t *testing.T, stopBefore string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "iso_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop isolated schema %s: %v", schema, err)
		}
		base.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	_, thisFile, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations", "*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	for _, path := range paths {
		if stopBefore != "" && filepath.Base(path) >= stopBefore {
			break
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := strings.SplitN(string(data), "-- +goose Up", 2)[1]
		sql = strings.SplitN(sql, "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	return pool
}
