package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1xDBOpenEmptyURLReturnsNilPool(t *testing.T) {
	pool, err := Open(context.Background(), "")
	if err != nil || pool != nil {
		t.Fatalf("want nil,nil got %v,%v", pool, err)
	}
}

func TestCov1xDBOpenRejectsMalformedURL(t *testing.T) {
	pool, err := Open(context.Background(), "postgres://user:pw@host:notaport/db")
	if pool != nil || err == nil || !strings.Contains(err.Error(), "db: parse config") {
		t.Fatalf("want parse config error, got %v, %v", pool, err)
	}
}

func TestCov1xDBOpenPingFailureClosesPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Port 1 on loopback is closed, so the connectivity ping must fail.
	pool, err := Open(ctx, "postgres://u:p@127.0.0.1:1/x?sslmode=disable&connect_timeout=2")
	if pool != nil || err == nil || !strings.Contains(err.Error(), "db: ping") {
		t.Fatalf("want ping error, got %v, %v", pool, err)
	}
}

func TestCov1xDBOpenConnectsAndConfiguresPool(t *testing.T) {
	url := testutil.IsolatedSchemaPool(t).Config().ConnString()
	pool, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != 20 {
		t.Fatalf("MaxConns = %d, want 20", got)
	}
	if got := pool.Config().MinConns; got != 2 {
		t.Fatalf("MinConns = %d, want 2", got)
	}
}

func TestCov1xDBValidateSchemaNilPoolIsNoop(t *testing.T) {
	if err := ValidateExpectedSchema(context.Background(), nil); err != nil {
		t.Fatalf("nil pool: %v", err)
	}
}

func TestCov1xDBValidateSchemaPassesOnMigratedSchema(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	if err := ValidateExpectedSchema(context.Background(), pool); err != nil {
		t.Fatalf("migrated schema should validate: %v", err)
	}
}

func TestCov1xDBValidateSchemaReportsDrift(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	var schema string
	if err := admin.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := admin.Exec(ctx, `ALTER TABLE announcements DROP COLUMN deleted_at`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	// Restrict the search path to the isolated schema so the shared public
	// schema's intact tables cannot mask the drift.
	url := admin.Config().ConnString() + "&search_path=" + schema
	if !strings.Contains(admin.Config().ConnString(), "?") {
		url = admin.Config().ConnString() + "?search_path=" + schema
	}
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer pool.Close()
	err = ValidateExpectedSchema(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "schema drift detected") ||
		!strings.Contains(err.Error(), "announcements.deleted_at") {
		t.Fatalf("want drift error naming announcements.deleted_at, got %v", err)
	}
}

func TestCov1xDBValidateSchemaQueryFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ValidateExpectedSchema(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "schema validation query failed") {
		t.Fatalf("want query failure, got %v", err)
	}
}
