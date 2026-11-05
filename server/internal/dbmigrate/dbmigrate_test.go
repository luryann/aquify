package dbmigrate

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func sourceDir() string { return filepath.Join("..", "..", "..", "db", "migrations") }

func countSQL(t *testing.T) int {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(sourceDir(), "*.sql"))
	if err != nil || len(m) == 0 {
		t.Fatalf("no source migrations: %v", err)
	}
	return len(m)
}

// freshSchemaDSN creates an empty schema in the test database and returns a DSN
// whose search_path is that schema, so goose's bookkeeping table and every
// migrated object land there and are dropped afterwards.
func freshSchemaDSN(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if base == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	schema := "mig_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = conn.Close(ctx)
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

func TestUpAppliesEverythingOnceAndIsIdempotent(t *testing.T) {
	dsn := freshSchemaDSN(t)
	fsys := os.DirFS(sourceDir())
	want := countSQL(t)

	applied, err := Up(context.Background(), dsn, fsys)
	if err != nil {
		t.Fatalf("first up: %v", err)
	}
	if len(applied) != want {
		t.Fatalf("applied %d migrations, want %d", len(applied), want)
	}
	again, err := Up(context.Background(), dsn, fsys)
	if err != nil || len(again) != 0 {
		t.Fatalf("second up = %d applied, err=%v; want a no-op", len(again), err)
	}
	st, err := Status(context.Background(), dsn, fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if !s.Applied {
			t.Errorf("%s not applied after up", s.Source)
		}
	}
}

func TestConcurrentRunnersApplyEachMigrationExactlyOnce(t *testing.T) {
	dsn := freshSchemaDSN(t)
	fsys := os.DirFS(sourceDir())
	want := countSQL(t)

	var wg sync.WaitGroup
	counts := make([]int, 3)
	errs := make([]error, 3)
	for i := range counts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, err := Up(context.Background(), dsn, fsys)
			counts[i], errs[i] = len(a), err
		}(i)
	}
	wg.Wait()
	total := 0
	for i := range counts {
		if errs[i] != nil {
			t.Fatalf("runner %d: %v", i, errs[i])
		}
		total += counts[i]
	}
	if total != want {
		t.Fatalf("runners applied %d migrations in total, want exactly %d", total, want)
	}
}

func TestFailingMigrationStopsTheRunAndIsNotRecorded(t *testing.T) {
	dsn := freshSchemaDSN(t)
	fsys := fstest.MapFS{
		"00001_ok.sql":   {Data: []byte("-- +goose Up\nCREATE TABLE ok_one (id int);\n-- +goose Down\nDROP TABLE ok_one;\n")},
		"00002_bad.sql":  {Data: []byte("-- +goose Up\nCREATE TABLE bad_one (id int);\nSELECT 1/0;\n-- +goose Down\nDROP TABLE bad_one;\n")},
		"00003_late.sql": {Data: []byte("-- +goose Up\nCREATE TABLE late_one (id int);\n-- +goose Down\nDROP TABLE late_one;\n")},
	}
	applied, err := Up(context.Background(), dsn, fsys)
	if err == nil {
		t.Fatal("expected the failing migration to return an error")
	}
	if len(applied) != 1 || applied[0].Version != 1 {
		t.Fatalf("applied = %+v, want only version 1", applied)
	}
	st, serr := Status(context.Background(), dsn, fsys)
	if serr != nil {
		t.Fatal(serr)
	}
	for _, s := range st {
		if s.Version != 1 && s.Applied {
			t.Errorf("version %d recorded as applied after failure", s.Version)
		}
	}
	// The failed migration ran in a transaction: its table must not exist.
	ctx := context.Background()
	conn, cerr := pgx.Connect(ctx, dsn)
	if cerr != nil {
		t.Fatal(cerr)
	}
	defer conn.Close(ctx)
	var exists bool
	if qerr := conn.QueryRow(ctx, "SELECT to_regclass('bad_one') IS NOT NULL").Scan(&exists); qerr != nil {
		t.Fatal(qerr)
	}
	if exists {
		t.Fatal("bad_one exists: the failing migration was not rolled back")
	}
}

func TestEmptyMigrationSetIsAHardError(t *testing.T) {
	_, err := Up(context.Background(), "postgres://u:p@127.0.0.1:1/db", fstest.MapFS{})
	if !errors.Is(err, ErrNoMigrations) {
		t.Fatalf("err = %v, want ErrNoMigrations", err)
	}
}

func TestErrorsNeverEchoThePassword(t *testing.T) {
	const pw = "s3cretPassw0rd"
	dsn := "postgres://aquify:" + pw + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"
	_, err := Up(context.Background(), dsn, fstest.MapFS{"1_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}})
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), pw) {
		t.Fatalf("error leaks the password: %v", err)
	}
}
