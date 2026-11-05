package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/luryann/aquify/server/internal/app/events"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/volunteer"
	"github.com/luryann/aquify/server/internal/app/wellness"
	"github.com/luryann/aquify/server/internal/tenantdb"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Tenancy tracker item 4.4 (pilot): with row-level security on, the database itself
// keeps one org's volunteer rows from another org, even if a query forgets its
// org_id filter, and an unbound session sees nothing. The application connects as
// a role that is subject to RLS (not a superuser), like production.
func TestVolunteerRowLevelSecurityBacksUpTheAppFilters(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)

	svc := volunteer.NewServiceWithPool(appPool) // the application's pool: RLS applies
	start := time.Now().UTC().Add(96 * time.Hour).Truncate(time.Second)
	create := func(org, title string) *volunteer.Job {
		p := &appmiddleware.Principal{UserID: "adm-" + org[:4], OrgID: org, Roles: []string{"admin"}}
		j, err := svc.CreateJob(ctx, p, org, volunteer.JobInput{EventName: "E", Role: "timer", Title: title, StartsAt: start, EndsAt: start.Add(time.Hour), Capacity: 2})
		if err != nil {
			t.Fatalf("create job for %s: %v", title, err)
		}
		return j
	}
	jobA, jobB := create(orgA, "A timer"), create(orgB, "B timer")

	// The service still works per org.
	pa := &appmiddleware.Principal{UserID: "adm-a", OrgID: orgA, Roles: []string{"admin"}}
	list, err := svc.ListJobs(ctx, pa, orgA, volunteer.JobFilter{})
	if err != nil || len(list) != 1 || list[0].ID != jobA.ID {
		t.Fatalf("org A list through the service: %+v %v", list, err)
	}
	if _, err := svc.GetJob(ctx, pa, orgA, jobB.ID); err == nil {
		t.Fatal("org A must not read org B's job through the service")
	}

	scan := func(bound string, sql string, args ...any) int {
		var n int
		var err error
		if bound == "" {
			err = appPool.QueryRow(ctx, sql, args...).Scan(&n) // unbound session
		} else {
			err = tenantdb.Pool{P: appPool}.For(bound).QueryRow(ctx, sql, args...).Scan(&n)
		}
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	// 1. An unbound session sees nothing (fail closed), as the RLS role.
	if n := scan("", `SELECT count(*) FROM volunteer_jobs`); n != 0 {
		t.Fatalf("an unbound session must see no rows, saw %d", n)
	}
	// 2. A bound session sees only its org, even with NO org_id predicate in the SQL.
	if n := scan(orgA, `SELECT count(*) FROM volunteer_jobs`); n != 1 {
		t.Fatalf("org A bound, no predicate: want exactly its 1 row, saw %d", n)
	}
	if n := scan(orgB, `SELECT count(*) FROM volunteer_jobs WHERE id = $1::uuid`, jobA.ID); n != 0 {
		t.Fatalf("org B bound must not see org A's job by id, saw %d", n)
	}
	// 3. The superuser pool (migrations, break-glass) still sees both: RLS is the app role's constraint.
	var all int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM volunteer_jobs`).Scan(&all); err != nil || all != 2 {
		t.Fatalf("the privileged pool bypasses RLS: %d %v", all, err)
	}
	// 4. Writes: a session bound to A cannot insert or change a row for B.
	_, err = tenantdb.Pool{P: appPool}.For(orgA).Exec(ctx, `INSERT INTO volunteer_jobs (id, org_id, event_name, role, title, starts_at, ends_at, capacity, hour_credit, created_at, updated_at, created_by, updated_by)
		VALUES (gen_random_uuid(), $1::uuid, 'E', 'timer', 'sneaky', now(), now() + interval '1 hour', 1, 1, now(), now(), 'x', 'x')`, orgB)
	if err == nil {
		t.Fatal("RLS WITH CHECK must reject a row for another org")
	}
	tag, err := tenantdb.Pool{P: appPool}.For(orgA).Exec(ctx, `UPDATE volunteer_jobs SET title = 'hijacked' WHERE id = $1::uuid`, jobB.ID)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("org A must not update org B's job: rows=%d err=%v", tag.RowsAffected(), err)
	}
	var title string
	_ = admin.QueryRow(ctx, `SELECT title FROM volunteer_jobs WHERE id = $1::uuid`, jobB.ID).Scan(&title)
	if title != "B timer" {
		t.Fatalf("org B's job was modified: %q", title)
	}
	// 5. The binding is transaction-local: it never leaks to the next statement on a pooled connection.
	_ = scan(orgA, `SELECT count(*) FROM volunteer_jobs`)
	if n := scan("", `SELECT count(*) FROM volunteer_jobs`); n != 0 {
		t.Fatalf("a previous request's org leaked into an unbound statement (saw %d rows)", n)
	}
}

// club events, RSVPs, feed tokens and wellness
// check-ins. The calendar feed has no session, so the token's hash is its only key.
func TestEventsAndWellnessRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	evSvc := events.NewServiceWithPool(appPool) // the application's pool: RLS applies
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)

	mk := func(org, title string) *events.Event {
		e, err := evSvc.Create(org, "actor", events.EventInput{Title: title, Category: "social", StartsAt: start, Audience: "all", Status: "scheduled"})
		if err != nil {
			t.Fatalf("create event %s: %v", title, err)
		}
		return e
	}
	evA, evB := mk(orgA, "A party"), mk(orgB, "B party")
	if got, err := evSvc.List(orgA, events.ListFilter{}); err != nil || len(got) != 1 || got[0].ID != evA.ID {
		t.Fatalf("org A lists only its event: %+v %v", got, err)
	}
	if _, err := evSvc.Get(orgA, evB.ID); err == nil {
		t.Fatal("org A must not read org B's event")
	}

	// Feed token: created per org, resolved with no org known (token hash only).
	hhA, hhB := uuid.NewString(), uuid.NewString()
	tokA, _, err := evSvc.RotateFeedToken(orgA, hhA, "actor")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := evSvc.RotateFeedToken(orgB, hhB, "actor"); err != nil {
		t.Fatal(err)
	}
	got, err := evSvc.ResolveFeedToken(tokA)
	if err != nil || got.OrgID != orgA || got.HouseholdID != hhA {
		t.Fatalf("the feed token resolves to its own org without a session: %+v %v", got, err)
	}
	if _, err := evSvc.ResolveFeedToken("not-a-token"); err == nil {
		t.Fatal("an unknown token resolves to nothing")
	}

	// Raw view as the app role.
	scan := func(sql string, bind func() interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}) int {
		var n int
		if err := bind().QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	unbound := func() interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	} {
		return appPool
	}
	forOrg := func(o string) func() interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	} {
		return func() interface {
			QueryRow(context.Context, string, ...any) pgx.Row
		} {
			return tenantdb.Pool{P: appPool}.For(o)
		}
	}
	for _, tbl := range []string{"club_events", "calendar_feed_tokens"} {
		if n := scan(`SELECT count(*) FROM `+tbl, unbound); n != 0 {
			t.Errorf("%s: an unbound session saw %d rows", tbl, n)
		}
		if n := scan(`SELECT count(*) FROM `+tbl, forOrg(orgA)); n != 1 {
			t.Errorf("%s: org A bound, no predicate: saw %d rows, want 1", tbl, n)
		}
	}
	// A hash binding exposes exactly that token row, read-only, and no other table.
	hashOf := events.HashFeedToken(tokA)
	hashBound := tenantdb.Pool{P: appPool}.ForLookup(tenantdb.LookupFeedTokenHash, hashOf)
	var visible, eventRows int
	_ = hashBound.QueryRow(ctx, `SELECT count(*) FROM calendar_feed_tokens`).Scan(&visible)
	_ = hashBound.QueryRow(ctx, `SELECT count(*) FROM club_events`).Scan(&eventRows)
	if visible != 1 || eventRows != 0 {
		t.Fatalf("a token hash must expose only its own token row: tokens=%d events=%d", visible, eventRows)
	}
	if _, err := hashBound.Exec(ctx, `UPDATE calendar_feed_tokens SET revoked_at = now()`); err != nil {
		t.Fatal(err)
	}
	var revoked int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM calendar_feed_tokens WHERE revoked_at IS NOT NULL`).Scan(&revoked)
	if revoked != 0 {
		t.Fatalf("the hash policy is read-only, but %d tokens were revoked through it", revoked)
	}

	// Wellness.
	swA, swB := uuid.NewString(), uuid.NewString()
	for _, s := range []struct{ org, hh, sw string }{{orgA, hhA, swA}, {orgB, hhB, swB}} {
		if _, err := admin.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'H', 'active')`, s.hh, s.org); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'W', 'S', '2011-01-01', 'female', 'active')`, s.sw, s.org, s.hh); err != nil {
			t.Fatal(err)
		}
	}
	wSvc := wellness.NewServiceWithPool(appPool)
	m := wellness.Metrics{SleepHours: 7, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 2, Mood: 4}
	for _, c := range []struct{ org, hh, sw string }{{orgA, hhA, swA}, {orgB, hhB, swB}} {
		if _, _, err := wSvc.Upsert(c.org, wellness.Input{SwimmerID: c.sw, HouseholdID: c.hh, Metrics: m}); err != nil {
			t.Fatalf("wellness upsert: %v", err)
		}
	}
	if n := scan(`SELECT count(*) FROM wellness_checkins`, forOrg(orgB)); n != 1 {
		t.Fatalf("org B bound sees %d wellness rows, want 1", n)
	}
	if n := scan(`SELECT count(*) FROM wellness_checkins`, unbound); n != 0 {
		t.Fatalf("unbound sees %d wellness rows", n)
	}
}
