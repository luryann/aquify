package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IsolatedSchemaPool returns a pool bound to a private, throwaway schema in the
// DATABASE_URL database with every goose "Up" migration applied, so DB tests
// never read or write shared fixtures. It skips when DATABASE_URL is unset.
func IsolatedSchemaPool(t *testing.T) *pgxpool.Pool {
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
