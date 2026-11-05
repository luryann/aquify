package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/performance"
	"github.com/luryann/aquify/server/internal/testutil"
)

// results, best times, standards and goals are written under each club's
// binding, a restarted instance reloads every club, and the database refuses cross-club
// access and unbound reads.
func TestPerformanceRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := performance.NewServiceWithPool(appPool)
	mem := members.NewServiceWithPool(appPool)
	staff := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	}
	for _, org := range []string{orgA, orgB} {
		p := staff(org)
		hh, err := mem.CreateHousehold(p, members.CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := mem.CreateSwimmer(p, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "S", LastName: "W", DateOfBirth: "2012-01-01", Gender: "female"})
		if err != nil {
			t.Fatal(err)
		}
		ft := 60000
		if _, err := svc.UpsertResult(org, p.UserID, p, performance.UpsertResultInput{SwimmerID: sw.ID, Distance: 50, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ft, IsOfficial: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateTimeStandard(org, p.UserID, p, performance.CreateTimeStandardInput{Name: "AA", SeasonYear: 2026, StandardCode: "AA", EventDistance: 50, Stroke: "free", Course: "SCY", Gender: "F", TimeMs: 30000, IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}

	// A restarted instance loads every club under each club's own binding.
	restarted := performance.NewServiceWithPool(appPool)
	for _, org := range []string{orgA, orgB} {
		got, err := restarted.ListTimeStandards(org, performance.ListTimeStandardsFilters{})
		if err != nil || len(got) != 1 || got[0].OrgID != org {
			t.Fatalf("club %s sees only its standard after a restart: %+v %v", org, got, err)
		}
		bts, err := restarted.ListBestTimesForOrg(org, staff(org), performance.ListBestTimesFilters{})
		if err != nil || len(bts) != 1 || bts[0].OrgID != org {
			t.Fatalf("club %s sees only its best time after a restart: %+v %v", org, bts, err)
		}
	}
	// Clubs load on first use and are evicted beyond the cap: with room for one club, serving
	// A, then B, then A again must still return each club its own data.
	capped := performance.NewServiceWithPool(appPool)
	capped.SetOrgCacheLimits(0, 1)
	for _, org := range []string{orgA, orgB, orgA} {
		got, err := capped.ListBestTimesForOrg(org, staff(org), performance.ListBestTimesFilters{})
		if err != nil || len(got) != 1 || got[0].OrgID != org {
			t.Fatalf("capped cache serves club %s its own best time: %+v %v", org, got, err)
		}
	}
	// Another instance's write becomes visible once the cached club is older than the refresh
	// interval (no restart needed).
	x := performance.NewServiceWithPool(appPool)
	x.SetRefreshInterval(time.Nanosecond) // every read of a cached club is due a check
	var loads int
	x.SetSyncHookForTest(func() { loads++ })
	read := func() int {
		got, err := x.ListBestTimesForOrg(orgA, staff(orgA), performance.ListBestTimesFilters{})
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	if n := read(); n != 1 || loads != 1 {
		t.Fatalf("instance X loads club A once: best times=%d loads=%d", n, loads)
	}
	if read(); loads != 1 {
		t.Fatalf("nothing changed, so the counters must let X skip the re-read: loads=%d", loads)
	}
	pA := staff(orgA)
	hh, err := mem.CreateHousehold(pA, members.CreateHouseholdInput{OrgID: orgA, HouseholdName: "H2"})
	if err != nil {
		t.Fatal(err)
	}
	sw2, _, err := mem.CreateSwimmer(pA, orgA, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "T", LastName: "U", DateOfBirth: "2011-01-01", Gender: "male"})
	if err != nil {
		t.Fatal(err)
	}
	ft := 55000
	if _, err := svc.UpsertResult(orgA, pA.UserID, pA, performance.UpsertResultInput{SwimmerID: sw2.ID, Distance: 50, Stroke: "free", Course: "SCY", Gender: "M", FinTimeMs: &ft, IsOfficial: true}); err != nil {
		t.Fatal(err)
	}
	if n := read(); n != 2 || loads != 2 {
		t.Fatalf("instance X sees instance Y's write after one re-read: best times=%d loads=%d", n, loads)
	}

	for _, table := range []string{"results", "best_times", "time_standards"} {
		var n int
		if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
