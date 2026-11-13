package tenantdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCov1xTenantdbForLookupBindsTheNamedSetting(t *testing.T) {
	_, app, _, _ := rlsTable(t)
	ctx := context.Background()
	p := Pool{P: app, Writer: "cov1x-writer"}
	var got, writer string
	err := p.ForLookup(LookupPublicSlug, "my-club").QueryRow(ctx,
		`SELECT current_setting('app.public_slug', true), current_setting('app.writer', true)`).Scan(&got, &writer)
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-club" || writer != "cov1x-writer" {
		t.Fatalf("lookup=%q writer=%q", got, writer)
	}
	// The org setting is not bound by a lookup handle.
	var org string
	if err := p.ForLookup(LookupDomain, "x.example").QueryRow(ctx, `SELECT COALESCE(current_setting('app.org_id', true), '')`).Scan(&org); err != nil || org != "" {
		t.Fatalf("org=%q err=%v", org, err)
	}
}

func TestCov1xTenantdbForeignWritesExcludesOwnWriter(t *testing.T) {
	admin, app, orgA, orgB := rlsTable(t)
	ctx := context.Background()
	ins := func(org, domain, writer string, v int) {
		if _, err := admin.Exec(ctx, `INSERT INTO org_data_versions (org_id, domain, writer, version) VALUES ($1::uuid,$2,$3,$4)`, org, domain, writer, v); err != nil {
			t.Fatal(err)
		}
	}
	ins(orgA, "billing", "me", 5)
	ins(orgA, "billing", "other1", 2)
	ins(orgA, "billing", "other2", 3)
	ins(orgA, "performance", "other1", 100)
	ins(orgB, "billing", "other1", 50)

	p := Pool{P: app, Writer: "me"}
	n, err := p.ForeignWrites(ctx, orgA, "billing")
	if err != nil || n != 5 {
		t.Fatalf("foreign writes = %d err=%v, want 5", n, err)
	}
	n, err = p.ForeignWrites(ctx, orgA, "unknown")
	if err != nil || n != 0 {
		t.Fatalf("unknown domain = %d err=%v, want 0", n, err)
	}
	n, err = Pool{P: app, Writer: "other1"}.ForeignWrites(ctx, orgA, "billing")
	if err != nil || n != 8 {
		t.Fatalf("other writer view = %d err=%v, want 8", n, err)
	}
	if _, err := p.ForeignWrites(ctx, "not-a-uuid", "billing"); err == nil {
		t.Fatal("an invalid org id must return an error")
	}
}

func TestCov1xTenantdbBindFailureIsReturned(t *testing.T) {
	_, app, _, _ := rlsTable(t)
	ctx := context.Background()
	// An empty setting name is rejected by set_config, so the bind statement fails.
	bad := Pool{P: app}.ForLookup(Lookup(""), "v")
	if _, err := bad.Begin(ctx); err == nil || !strings.Contains(err.Error(), "configuration parameter") {
		t.Fatalf("Begin err = %v", err)
	}
	if _, err := bad.Exec(ctx, `SELECT 1`); err == nil {
		t.Fatal("Exec must surface the bind failure")
	}
	if _, err := bad.Query(ctx, `SELECT 1`); err == nil {
		t.Fatal("Query must surface the bind failure")
	}
	var x int
	if err := bad.QueryRow(ctx, `SELECT 1`).Scan(&x); err == nil {
		t.Fatal("QueryRow must surface the bind failure")
	}
	if app.Stat().AcquiredConns() != 0 {
		t.Fatalf("connections leaked: %d", app.Stat().AcquiredConns())
	}
}

func TestCov1xTenantdbClosedPoolErrors(t *testing.T) {
	_, app, orgA, _ := rlsTable(t)
	ctx := context.Background()
	scoped := Pool{P: app}.For(orgA)
	app.Close()
	if _, err := scoped.Begin(ctx); err == nil {
		t.Fatal("Begin on a closed pool must fail")
	}
	if _, err := scoped.Exec(ctx, `SELECT 1`); err == nil {
		t.Fatal("Exec on a closed pool must fail")
	}
	if _, err := scoped.Query(ctx, `SELECT 1`); err == nil {
		t.Fatal("Query on a closed pool must fail")
	}
	if err := WithOrgTx(ctx, app, orgA, func(pgx.Tx) error { t.Fatal("fn must not run"); return nil }); err == nil {
		t.Fatal("WithOrgTx on a closed pool must fail")
	}
}

func TestCov1xTenantdbQueryErrorsAndScanRowsErr(t *testing.T) {
	_, app, orgA, _ := rlsTable(t)
	ctx := context.Background()
	scoped := Pool{P: app}.For(orgA)
	if rows, err := scoped.Query(ctx, `SELEC broken`); err == nil {
		rows.Close()
		t.Fatal("a syntactically invalid query must fail")
	}
	// A runtime error raised while streaming rows is reported by Scan, not masked as ErrNoRows.
	var x int
	err := scoped.QueryRow(ctx, `SELECT 1/(count(*)-count(*)) FROM volunteer_settings`).Scan(&x)
	if err == nil || errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("err = %v", err)
	}
	if app.Stat().AcquiredConns() != 0 {
		t.Fatalf("connections leaked: %d", app.Stat().AcquiredConns())
	}
}

func TestCov1xTenantdbWithOrgTxCommitsAndRollsBack(t *testing.T) {
	admin, app, orgA, _ := rlsTable(t)
	ctx := context.Background()
	boom := errors.New("boom")
	set := func(v int) func(pgx.Tx) error {
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE volunteer_settings SET cancellation_cutoff_hours = $1 WHERE org_id = $2::uuid`, v, orgA)
			return err
		}
	}
	read := func() int {
		var n int
		if err := admin.QueryRow(ctx, `SELECT cancellation_cutoff_hours FROM volunteer_settings WHERE org_id = $1::uuid`, orgA).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := WithOrgTx(ctx, app, orgA, set(77)); err != nil || read() != 77 {
		t.Fatalf("commit: err=%v value=%d", err, read())
	}
	err := WithOrgTx(ctx, app, orgA, func(tx pgx.Tx) error {
		if err := set(11)(tx); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || read() != 77 {
		t.Fatalf("rollback: err=%v value=%d", err, read())
	}
}
