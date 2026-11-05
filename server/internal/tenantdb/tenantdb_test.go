package tenantdb

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

func rlsTable(t *testing.T) (admin, app *pgxpool.Pool, orgA, orgB string) {
	t.Helper()
	admin, app = testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB = uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	// volunteer_settings has RLS (migration 20260801000700) and a simple shape.
	for _, o := range []string{orgA, orgB} {
		if _, err := admin.Exec(ctx, `INSERT INTO volunteer_settings (org_id) VALUES ($1::uuid)`, o); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return
}

func TestScopedStatementsSeeOnlyTheBoundOrg(t *testing.T) {
	_, app, orgA, orgB := rlsTable(t)
	ctx := context.Background()
	count := func(bound string) int {
		var n int
		var err error
		if bound == "" {
			err = app.QueryRow(ctx, `SELECT count(*) FROM volunteer_settings`).Scan(&n)
		} else {
			err = Pool{P: app}.For(bound).QueryRow(ctx, `SELECT count(*) FROM volunteer_settings`).Scan(&n)
		}
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(orgA) != 1 || count(orgB) != 1 {
		t.Fatal("each org sees exactly its own row")
	}
	if count("") != 0 {
		t.Fatal("an unbound statement sees nothing")
	}
	// Exec honours the binding too: org A cannot delete org B's row.
	tag, err := Pool{P: app}.For(orgA).Exec(ctx, `DELETE FROM volunteer_settings WHERE org_id = $1::uuid`, orgB)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("cross-org delete: rows=%d err=%v", tag.RowsAffected(), err)
	}
	if count(orgB) != 1 {
		t.Fatal("org B's row must survive")
	}
}

func TestScopedErrorsAndNoRowsAreReportedAndConnectionsAreReleased(t *testing.T) {
	_, app, orgA, _ := rlsTable(t)
	ctx := context.Background()
	scoped := Pool{P: app}.For(orgA)
	var x int
	if err := scoped.QueryRow(ctx, `SELECT 1 FROM volunteer_settings WHERE org_id = $1::uuid`, uuid.NewString()).Scan(&x); err != pgx.ErrNoRows {
		t.Fatalf("no rows must be pgx.ErrNoRows, got %v", err)
	}
	if _, err := scoped.Exec(ctx, `SELEC nonsense`); err == nil {
		t.Fatal("a bad statement must return its error")
	}
	if err := scoped.QueryRow(ctx, `SELECT nope FROM nowhere`).Scan(&x); err == nil {
		t.Fatal("a bad query must return its error")
	}
	// Far more statements than pool connections: leaked connections would deadlock here.
	for i := 0; i < 3*int(app.Config().MaxConns)+10; i++ {
		rows, err := scoped.Query(ctx, `SELECT org_id FROM volunteer_settings`)
		if err != nil {
			t.Fatal(err)
		}
		rows.Close()
		rows.Close() // closing twice is safe
		var n int
		if err := scoped.QueryRow(ctx, `SELECT count(*) FROM volunteer_settings`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("iteration %d: n=%d err=%v", i, n, err)
		}
	}
	if app.Stat().AcquiredConns() != 0 {
		t.Fatalf("connections still held: %d", app.Stat().AcquiredConns())
	}
}

func TestWithOrgTxRequiresAnOrgAndBindsTheTransaction(t *testing.T) {
	_, app, orgA, orgB := rlsTable(t)
	ctx := context.Background()
	if err := WithOrgTx(ctx, app, "", func(pgx.Tx) error { return nil }); err == nil {
		t.Fatal("an empty org is refused")
	}
	var seen int
	err := WithOrgTx(ctx, app, orgA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM volunteer_settings WHERE org_id = $1::uuid`, orgB).Scan(&seen)
	})
	if err != nil || seen != 0 {
		t.Fatalf("the transaction is bound to org A: saw %d of org B (err %v)", seen, err)
	}
}
