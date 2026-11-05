package main

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMigrationDSNRules(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr string
	}{
		{"production uses the migration role", map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": "postgres://owner@h/db", "DATABASE_URL": "postgres://app@h/db"}, "postgres://owner@h/db", ""},
		{"production never falls back to the app role", map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://app@h/db"}, "", "MIGRATE_DATABASE_URL"},
		{"staging never falls back either", map[string]string{"APP_ENV": "staging", "DATABASE_URL": "postgres://app@h/db"}, "", "MIGRATE_DATABASE_URL"},
		{"development may fall back", map[string]string{"APP_ENV": "development", "DATABASE_URL": "postgres://dev@h/db"}, "postgres://dev@h/db", ""},
		{"missing APP_ENV is refused", map[string]string{"DATABASE_URL": "postgres://dev@h/db"}, "", "APP_ENV"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := migrationDSN(envOf(c.env))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want mention of %q", err, c.wantErr)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %q err=%v, want %q", got, err, c.want)
			}
		})
	}
}

func TestRunMigrateUsageAndMissingConfigFailWithoutTouchingTheDatabase(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runMigrate(context.Background(), []string{"down"}, envOf(nil), &out, &errb); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
	errb.Reset()
	if code := runMigrate(context.Background(), []string{"up"}, envOf(map[string]string{"APP_ENV": "production"}), &out, &errb); code != 1 || !strings.Contains(errb.String(), "MIGRATE_DATABASE_URL") {
		t.Fatalf("exit=%d stderr=%q", code, errb.String())
	}
}

// End to end against a real database: an empty schema is initialised by `up`
// from the real migration files, `status` is clean afterwards, and a repeat run
// is a no-op.
func TestRunMigrateUpThenStatusOnAnEmptyDatabase(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if base == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	schema := "cli_mig_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); _ = conn.Close(ctx) })
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()

	old := migrationSource
	migrationSource = func() fs.FS { return os.DirFS(filepath.Join("..", "..", "..", "db", "migrations")) }
	t.Cleanup(func() { migrationSource = old })
	env := envOf(map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": u.String()})

	var out, errb bytes.Buffer
	if code := runMigrate(ctx, []string{"status"}, env, &out, &errb); code != 1 {
		t.Fatalf("status on an empty database exit = %d, want 1 (pending)", code)
	}
	out.Reset()
	errb.Reset()
	if code := runMigrate(ctx, []string{"up"}, env, &out, &errb); code != 0 {
		t.Fatalf("up exit = %d stderr=%s", code, errb.String())
	}
	out.Reset()
	if code := runMigrate(ctx, []string{"up"}, env, &out, &errb); code != 0 || !strings.Contains(out.String(), "0 applied") {
		t.Fatalf("second up exit=%d out=%q", code, out.String())
	}
	out.Reset()
	errb.Reset()
	if code := runMigrate(ctx, []string{"status"}, env, &out, &errb); code != 0 {
		t.Fatalf("status after up exit = %d stderr=%s", code, errb.String())
	}
}

func TestRunMigrateWithNothingEmbeddedFailsClosed(t *testing.T) {
	old := migrationSource
	migrationSource = func() fs.FS { return fstest.MapFS{} }
	t.Cleanup(func() { migrationSource = old })
	var out, errb bytes.Buffer
	env := envOf(map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": "postgres://o@127.0.0.1:1/db"})
	if code := runMigrate(context.Background(), []string{"up"}, env, &out, &errb); code != 1 || !strings.Contains(errb.String(), "no migrations embedded") {
		t.Fatalf("exit=%d stderr=%q", code, errb.String())
	}
}
