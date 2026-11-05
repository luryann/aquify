package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Tenancy tracker items 1.3, 1.4 and 1.5: schema-level isolation guarantees,
// checked against a real migrated database with the strict (fail-closed) pool.

func TestEveryTenantTableReferencesOrgs(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	rows, err := pool.Query(context.Background(), `
SELECT c.table_name FROM information_schema.columns c
JOIN information_schema.tables x ON x.table_schema = c.table_schema AND x.table_name = c.table_name AND x.table_type = 'BASE TABLE'
WHERE c.table_schema = current_schema() AND c.column_name = 'org_id' AND c.table_name <> 'orgs'
  AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.contype = 'f' AND k.conrelid = c.table_name::regclass AND k.confrelid = 'orgs'::regclass)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		t.Errorf("tenant table %s has org_id but no FK to orgs(id)", name)
	}
}

// Every global-by-design table (no org_id) must be listed here with a reason, so
// a new tenant table cannot silently ship without org_id.
var globalTables = map[string]string{
	"goose_db_version":  "migration bookkeeping",
	"runtime_snapshots": "legacy single-process snapshot bridge; unused when a DB pool backs services",
	"orgs":              "the tenant registry itself",
}

func TestEveryTableHasOrgIDOrIsAllowlisted(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	rows, err := pool.Query(context.Background(), `
SELECT t.table_name FROM information_schema.tables t
WHERE t.table_schema = current_schema() AND t.table_type = 'BASE TABLE'
  AND NOT EXISTS (SELECT 1 FROM information_schema.columns c WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name AND c.column_name = 'org_id')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		if _, ok := globalTables[name]; !ok {
			t.Errorf("table %s has no org_id and is not in globalTables; add org_id or justify it", name)
		}
	}
}

func TestCompositeOrgForeignKeyTwinsExist(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	rows, err := pool.Query(context.Background(), `
SELECT con.conrelid::regclass::text, a.attname, con.confrelid::regclass::text
FROM pg_constraint con
JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = con.conkey[1]
JOIN pg_attribute p ON p.attrelid = con.confrelid AND p.attnum = con.confkey[1]
WHERE con.contype = 'f' AND array_length(con.conkey, 1) = 1 AND con.connamespace = current_schema()::regnamespace
  AND p.attname = 'id' AND a.attname <> 'org_id'
  AND con.confrelid::regclass::text NOT IN ('users', 'roles', 'orgs')
  AND EXISTS (SELECT 1 FROM pg_attribute x WHERE x.attrelid = con.conrelid AND x.attname = 'org_id' AND x.attnotnull AND NOT x.attisdropped)
  AND EXISTS (SELECT 1 FROM pg_attribute x WHERE x.attrelid = con.confrelid AND x.attname = 'org_id' AND x.attnotnull AND NOT x.attisdropped)
  AND NOT EXISTS (
    SELECT 1 FROM pg_constraint k WHERE k.contype = 'f' AND k.conrelid = con.conrelid AND k.confrelid = con.confrelid
      AND array_length(k.conkey, 1) = 2
      AND (SELECT attname FROM pg_attribute WHERE attrelid = k.conrelid AND attnum = k.conkey[1]) = 'org_id'
      AND (SELECT attname FROM pg_attribute WHERE attrelid = k.conrelid AND attnum = k.conkey[2]) = a.attname)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var child, col, parent string
		_ = rows.Scan(&child, &col, &parent)
		t.Errorf("FK %s.%s -> %s(id) has no composite (org_id, %s) twin", child, col, parent, col)
	}
}

func TestUnregisteredOrgIsRejectedAndCrossOrgChildIsRejected(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()

	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES (gen_random_uuid(), $1::uuid, 'H', 'active')`, orgA); err == nil {
		t.Fatal("insert for an unregistered org must fail")
	}
	testutil.SeedOrgs(t, pool, orgA, orgB)
	hhA := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'H', 'active')`, hhA, orgA); err != nil {
		t.Fatalf("registered org insert: %v", err)
	}

	// A message in org B whose household belongs to org A must be refused by the database.
	swB := uuid.NewString()
	hhB := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'HB', 'active')`, hhB, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'S', 'B', '2012-01-01', 'female', 'active')`, swB, orgB, hhB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (id, org_id, household_id, swimmer_id, sender_user_id, sender_role, body) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3::uuid, gen_random_uuid(), 'parent', 'x')`, orgB, hhA, swB); err == nil {
		t.Fatal("a message in org B pointing at org A's household must be rejected by the composite FK")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (id, org_id, household_id, swimmer_id, sender_user_id, sender_role, body) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3::uuid, gen_random_uuid(), 'parent', 'x')`, orgB, hhB, swB); err != nil {
		t.Fatalf("same-org message must be accepted: %v", err)
	}
}

func TestMessageChildTablesBackfillOrgFromMessage(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolBefore(t, "20260801000300")
	ctx := context.Background()
	org, hh, sw, msg := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'H', 'active')`, []any{hh, org}},
		{`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'S', 'S', '2012-01-01', 'female', 'active')`, []any{sw, org, hh}},
		{`INSERT INTO messages (id, org_id, household_id, swimmer_id, sender_user_id, sender_role, body) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, gen_random_uuid(), 'parent', 'hi')`, []any{msg, org, hh, sw}},
		{`INSERT INTO message_edits (message_id, previous_body, edited_by) VALUES ($1::uuid, 'old', gen_random_uuid())`, []any{msg}},
		{`INSERT INTO message_reactions (message_id, user_id, emoji) VALUES ($1::uuid, gen_random_uuid(), 'x')`, []any{msg}},
		{`INSERT INTO message_attachments (message_id, object_key, filename, mime_type, size_bytes) VALUES ($1::uuid, 'k', 'f', 'image/png', 1)`, []any{msg}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, q.sql)
		}
	}
	testutil.ApplyMigration(t, pool, "20260801000300")
	for _, tbl := range []string{"message_edits", "message_reactions", "message_attachments"} {
		var got string
		if err := pool.QueryRow(ctx, `SELECT org_id::text FROM `+tbl+` WHERE message_id = $1::uuid`, msg).Scan(&got); err != nil || got != org {
			t.Errorf("%s.org_id = %q (err %v), want %s", tbl, got, err, org)
		}
	}
}

// rlsEnforcedTables is the explicit list of tables protected by row-level security.
// Every other tenant table relies on application filters, composite FKs and tests
// (tracker item 4.4). Adding a table here is only valid once its repository binds
// the org on every statement (internal/tenantdb); a new entry that is not covered
// would hide every row from the application role.
var rlsEnforcedTables = map[string]bool{
	"volunteer_settings": true, "volunteer_jobs": true, "volunteer_signups": true,
	"volunteer_requirements": true, "volunteer_credits": true, "volunteer_buyouts": true,
	"club_events": true, "club_event_rsvps": true, "calendar_feed_tokens": true, "wellness_checkins": true,
	"document_types": true, "document_submissions": true, "seasons": true, "data_export_requests": true, "audit_events": true, "file_exports": true,
	"results": true, "splits": true, "best_times": true, "time_standards": true,
	"team_record_entries": true, "swimmer_goals": true, "relay_lineups": true,
	"announcements": true, "email_campaigns": true, "email_deliveries": true, "suppression_list": true,
	"notification_preferences": true, "messages": true, "message_edits": true, "message_reactions": true,
	"message_attachments": true, "message_reads": true,
	"usa_swimming_integrations": true, "org_settings": true, "org_data_versions": true, "billing_fee_schedules": true, "billing_discount_rules": true, "billing_settings": true,
	"billing_adjustments": true, "billing_charges": true, "billing_runs": true, "billing_late_fee_applications": true,
	"household_autopay_profiles": true, "household_credit_balances": true, "household_credit_entries": true,
	"invoices": true, "invoice_items": true, "ledger_events": true, "payments": true, "refunds": true, "webhook_events": true, "households": true, "contacts": true, "swimmers": true, "swimmer_groups": true, "swimmer_link_requests": true, "meets": true, "meet_sessions": true, "meet_events": true, "commitments": true,
	"commitment_event_selections": true, "entries": true, "entry_legs": true, "groups": true, "eligibility_rules": true, "registrations": true, "waiver_acceptances": true, "users": true, "roles": true, "user_roles": true, "sessions": true, "mfa_factors": true, "org_domains": true, "org_invites": true, "org_payment_accounts": true,
	"practices": true, "practice_attendance": true, "training_workouts": true, "training_plan_phases": true, "data_deletion_requests": true,
}

func TestRowLevelSecurityIsEnabledOnExactlyTheListedTables(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	rows, err := pool.Query(context.Background(), `
SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity FROM pg_class c
WHERE c.relnamespace = current_schema()::regnamespace AND c.relkind = 'r'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var on, forced bool
		if err := rows.Scan(&name, &on, &forced); err != nil {
			t.Fatal(err)
		}
		seen[name] = on
		if rlsEnforcedTables[name] && !(on && forced) {
			t.Errorf("%s must have FORCE ROW LEVEL SECURITY enabled", name)
		}
		if !rlsEnforcedTables[name] && on {
			t.Errorf("%s has row-level security but is not in rlsEnforcedTables: its repository must bind the org (tenantdb) first", name)
		}
	}
	for name := range rlsEnforcedTables {
		if _, ok := seen[name]; !ok {
			t.Errorf("listed table %s does not exist", name)
		}
	}
}

// Every table with an org_id column must be protected by FORCE row-level security. A new
// tenant table fails this test until its repository binds the org (internal/tenantdb) and a
// migration enables RLS on it (tracker 4.4). There are no exemptions.
func TestEveryTenantTableHasRowLevelSecurity(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	rows, err := pool.Query(context.Background(), `
SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'org_id' AND NOT a.attisdropped
WHERE c.relnamespace = current_schema()::regnamespace AND c.relkind = 'r' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var name string
		var on, forced bool
		if err := rows.Scan(&name, &on, &forced); err != nil {
			t.Fatal(err)
		}
		n++
		if !on || !forced {
			t.Errorf("tenant table %s has no FORCE row-level security: bind the org in its repository and enable RLS in a migration", name)
		}
		if !rlsEnforcedTables[name] {
			t.Errorf("tenant table %s is missing from rlsEnforcedTables", name)
		}
	}
	if n != len(rlsEnforcedTables) {
		t.Fatalf("schema has %d tenant tables but rlsEnforcedTables lists %d", n, len(rlsEnforcedTables))
	}
}

// The change-counter triggers (org_data_versions) must sit on exactly the tables the
// cache-resident billing and performance services load. A table missing here would let a
// cache stay stale after another process writes it (until the 5-minute full reload).
// `splits` is deliberately absent: performance writes it through and never reads it back.
func TestChangeCounterTriggersCoverTheCachedTables(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	want := map[string]string{
		"billing_fee_schedules": "billing", "billing_discount_rules": "billing", "billing_settings": "billing",
		"billing_adjustments": "billing", "billing_charges": "billing", "billing_runs": "billing",
		"billing_late_fee_applications": "billing", "household_autopay_profiles": "billing",
		"household_credit_balances": "billing", "household_credit_entries": "billing", "invoices": "billing",
		"invoice_items": "billing", "ledger_events": "billing", "payments": "billing", "refunds": "billing",
		"results": "performance", "best_times": "performance", "time_standards": "performance",
		"team_record_entries": "performance", "swimmer_goals": "performance", "relay_lineups": "performance",
	}
	rows, err := pool.Query(context.Background(), `
SELECT c.relname, substring(encode(t.tgargs, 'escape') from '^[a-z]+')
FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
WHERE t.tgname = 'org_data_version' AND c.relnamespace = current_schema()::regnamespace`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var table, args string
		if err := rows.Scan(&table, &args); err != nil {
			t.Fatal(err)
		}
		got[table] = args
	}
	for table, domain := range want {
		if got[table] != domain {
			t.Errorf("table %s must carry the %q change-counter trigger, has %q", table, domain, got[table])
		}
	}
	for table := range got {
		if _, ok := want[table]; !ok {
			t.Errorf("unexpected change-counter trigger on %s: add it to the guard list and to the cache that loads it", table)
		}
	}
}
