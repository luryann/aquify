package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Legacy sessions and MFA factors with no club are given their user's club by migration
// 20260801002700, so row-level security does not hide them; rows whose user has no club stay NULL.
func TestBackfillGivesLegacyRowsTheirUsersOrg(t *testing.T) {
	// Build the schema as it was just before the backfill, seed legacy-shaped rows, then apply it.
	pool := testutil.IsolatedSchemaPoolBefore(t, "20260801002700")
	ctx := context.Background()
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	withOrg, noOrg := uuid.NewString(), uuid.NewString()
	for id, o := range map[string]any{withOrg: org, noOrg: nil} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $1, 'U', 'active', now(), now())`, id, o, id+"@example.test"); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO mfa_factors (id, user_id, org_id, factor_type, label, is_verified, backup_codes)
			VALUES ($1, $2, NULL, 'totp', 'phone', true, '[]'::jsonb)`, uuid.NewString(), id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO sessions (org_id, user_id, refresh_token_hash, access_token_hash, principal, expires_at, last_seen_at)
			VALUES (NULL, $1::uuid, $2, $3, '{}'::jsonb, now() + interval '1 day', now())`, id, uuid.NewString(), uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	testutil.ApplyMigration(t, pool, "20260801002700")
	count := func(table, user string, wantOrg bool) int {
		cond := "org_id IS NULL"
		if wantOrg {
			cond = "org_id = '" + org + "'::uuid"
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE user_id::text = $1 AND `+cond, user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"mfa_factors", "sessions"} {
		if count(table, withOrg, true) != 1 {
			t.Errorf("%s row of a user with a club must be backfilled", table)
		}
		if count(table, noOrg, false) != 1 {
			t.Errorf("%s row of a user with no club must stay NULL", table)
		}
	}
	var forced bool
	if err := pool.QueryRow(ctx, `SELECT relforcerowsecurity FROM pg_class WHERE relname = 'mfa_factors' AND relnamespace = current_schema()::regnamespace`).Scan(&forced); err != nil || !forced {
		t.Fatalf("FORCE row-level security must be restored after the backfill: %v %v", forced, err)
	}
}
