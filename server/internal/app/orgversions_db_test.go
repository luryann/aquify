package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Pruning folds stopped processes' counter rows into one retired row without ever lowering the
// sum another process would see, and leaves live writers alone.
func TestPruneOrgDataVersionsFoldsOldWritersAndKeepsTheSum(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	ctx := context.Background()
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	for w, ago := range map[string]string{"old-1": "30 days", "old-2": "20 days", "live": "1 minute"} {
		if _, err := pool.Exec(ctx, `INSERT INTO org_data_versions (org_id, domain, writer, version, touched_at)
			VALUES ($1::uuid, 'billing', $2, 5, now() - $3::interval)`, org, w, ago); err != nil {
			t.Fatal(err)
		}
	}
	foreignTo := func(self string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(version),0)::int FROM org_data_versions
			WHERE org_id=$1::uuid AND domain='billing' AND writer <> $2`, org, self).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := foreignTo("some-reader")
	pruneOrgDataVersionsNow(pool, slog.Default())
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM org_data_versions WHERE org_id=$1::uuid`, org).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("expected the live writer plus one retired row, got %d (%v)", rows, err)
	}
	if after := foreignTo("some-reader"); after != before {
		t.Fatalf("pruning must not change the sum another process sees: %d -> %d", before, after)
	}
	if foreignTo("live") != 10 {
		t.Fatal("the folded old writers (5+5) must count as foreign to the live one")
	}
}
