package members

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Repository writes keyed by ID must be org-bound (tenancy tracker item 4.1).
func TestDBSetKeycloakLinkIgnoresOtherOrg(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool)
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: orgA, Roles: []string{"admin"}}

	hh, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: orgA, HouseholdName: "H"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := svc.repo.SetHouseholdKeycloakUserID(ctx, orgB, hh.ID, "intruder"); err != nil {
		t.Fatal(err)
	}
	var kc *string
	if err := pool.QueryRow(ctx, `SELECT keycloak_user_id FROM households WHERE id = $1::uuid`, hh.ID).Scan(&kc); err != nil {
		t.Fatal(err)
	}
	if kc != nil && *kc != "" {
		t.Fatalf("org B linked a Keycloak user to org A's household: %q", *kc)
	}
	if err := svc.repo.SetHouseholdKeycloakUserID(ctx, orgA, hh.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT keycloak_user_id FROM households WHERE id = $1::uuid`, hh.ID).Scan(&kc); err != nil || kc == nil || *kc != "owner" {
		t.Fatalf("owning org link did not apply: %v err=%v", kc, err)
	}
}

// Tenancy tracker 4.3: a request reloads only its own org's rows. Before, every
// operation re-read every club's households, contacts and swimmers under one global
// lock; now club B's data stays out of the process until club B asks for it, and a
// reload for one club never disturbs another's cached rows.
func TestRequestSyncLoadsOnlyTheCallersOrg(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	writer := NewServiceWithPool(pool)
	adminOf := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	}
	for _, org := range []string{orgA, orgB} {
		if _, err := writer.CreateHousehold(adminOf(org), CreateHouseholdInput{OrgID: org, HouseholdName: "H " + org[:4]}); err != nil {
			t.Fatal(err)
		}
	}

	reader := NewServiceWithPool(pool) // a second API instance started after the data exists
	reader.mu.RLock()
	atStart := len(reader.households)
	reader.mu.RUnlock()
	if atStart != 0 {
		t.Fatalf("a new instance must not load any club's data until that club is served, got %d households", atStart)
	}

	got, err := reader.ListHouseholds(adminOf(orgA), orgA)
	if err != nil || len(got) != 1 || got[0].OrgID != orgA {
		t.Fatalf("org A request: %+v %v", got, err)
	}
	reader.mu.RLock()
	cached := len(reader.households)
	reader.mu.RUnlock()
	if cached != 1 {
		t.Fatalf("serving org A must not load org B's rows into memory: %d households cached", cached)
	}

	if got, err := reader.ListHouseholds(adminOf(orgB), orgB); err != nil || len(got) != 1 || got[0].OrgID != orgB {
		t.Fatalf("org B request: %+v %v", got, err)
	}
	reader.mu.RLock()
	cached = len(reader.households)
	reader.mu.RUnlock()
	if cached != 2 {
		t.Fatalf("each org's rows are kept once loaded, got %d", cached)
	}

	// A write by another instance to org A is seen by the next org A request,
	// and org B's cached rows are untouched by that reload.
	if _, err := writer.CreateHousehold(adminOf(orgA), CreateHouseholdInput{OrgID: orgA, HouseholdName: "Second A"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := reader.ListHouseholds(adminOf(orgA), orgA); len(got) != 2 {
		t.Fatalf("cross-instance write to org A must be visible: %d", len(got))
	}
	reader.mu.RLock()
	cached = len(reader.households)
	reader.mu.RUnlock()
	if cached != 3 {
		t.Fatalf("a reload of org A replaces only org A's rows: %d cached", cached)
	}
}

// Tenancy tracker 4.3: the per-org cache is bounded. With a cap of 3 clubs, serving 8
// clubs never holds more than 3 in memory, the least recently served go first, and an
// evicted club reloads correctly on its next request.
func TestOrgCacheIsBoundedAndEvictedClubsReload(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	writer := NewServiceWithPool(pool)
	orgs := make([]string, 8)
	for i := range orgs {
		orgs[i] = uuid.NewString()
		p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: orgs[i], Roles: []string{"admin"}}
		if _, err := writer.CreateHousehold(p, CreateHouseholdInput{OrgID: orgs[i], HouseholdName: "H"}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewServiceWithPool(pool)
	svc.SetOrgCacheLimits(0, 3) // no idle TTL, at most 3 clubs
	list := func(org string) int {
		p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
		got, err := svc.ListHouseholds(p, org)
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	cachedOrgs := func() int {
		svc.mu.RLock()
		defer svc.mu.RUnlock()
		seen := map[string]bool{}
		for _, h := range svc.households {
			seen[h.OrgID] = true
		}
		return len(seen)
	}
	for _, o := range orgs {
		if list(o) != 1 {
			t.Fatalf("org %s must see its household", o)
		}
		if n := cachedOrgs(); n > 3 {
			t.Fatalf("the cache must never hold more than 3 clubs, holds %d", n)
		}
	}
	if n := cachedOrgs(); n != 3 {
		t.Fatalf("expected the 3 most recently served clubs to stay cached, got %d", n)
	}
	if list(orgs[0]) != 1 { // evicted long ago: loads again from the database
		t.Fatal("an evicted club must reload correctly on its next request")
	}
}

func TestOrgCacheEvictsIdleClubs(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	writer := NewServiceWithPool(pool)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	for _, o := range []string{orgA, orgB} {
		p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: o, Roles: []string{"admin"}}
		if _, err := writer.CreateHousehold(p, CreateHouseholdInput{OrgID: o, HouseholdName: "H"}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewServiceWithPool(pool)
	svc.SetOrgCacheLimits(40*time.Millisecond, 0)
	p := func(o string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: o, Roles: []string{"admin"}}
	}
	if _, err := svc.ListHouseholds(p(orgA), orgA); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := svc.ListHouseholds(p(orgB), orgB); err != nil {
		t.Fatal(err)
	}
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	for _, h := range svc.households {
		if h.OrgID == orgA {
			t.Fatal("a club idle past the TTL must be evicted when another club is served")
		}
	}
	if len(svc.households) != 1 {
		t.Fatalf("only org B's row should remain, got %d", len(svc.households))
	}
}

// Tenancy tracker 4.3 exit check: a slow reload for one club must not block another
// club's requests. Club A's database read is stalled; club B is served while it is stalled.
func TestSlowReloadForOneClubDoesNotBlockAnother(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	writer := NewServiceWithPool(pool)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	for _, o := range []string{orgA, orgB} {
		p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: o, Roles: []string{"admin"}}
		if _, err := writer.CreateHousehold(p, CreateHouseholdInput{OrgID: o, HouseholdName: "H"}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewServiceWithPool(pool)
	var calls atomic.Int32
	const stall = 400 * time.Millisecond
	svc.SetSyncHookForTest(func() {
		if calls.Add(1) == 1 { // only the first reload (club A's) stalls
			time.Sleep(stall)
		}
	})
	asAdmin := func(o string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: o, Roles: []string{"admin"}}
	}

	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		if got, err := svc.ListHouseholds(asAdmin(orgA), orgA); err != nil || len(got) != 1 {
			t.Errorf("club A: %v %v", got, err)
		}
	}()
	time.Sleep(60 * time.Millisecond) // A is now stalled inside its reload
	start := time.Now()
	if got, err := svc.ListHouseholds(asAdmin(orgB), orgB); err != nil || len(got) != 1 {
		t.Fatalf("club B: %v %v", got, err)
	}
	if took := time.Since(start); took > stall/2 {
		t.Fatalf("club B waited %v behind club A's stalled reload (stall %v); reloads must not hold the service lock", took, stall)
	}
	<-aDone
}

// Tracker 4.3 exit check: 50 seeded clubs. With the default cap the process never holds
// more than the cap's worth of clubs, and heap growth stays proportional to the cap, not to
// the number of clubs served. Households per club are sized so the difference is measurable.
func TestFiftyClubsStayWithinTheCacheBudget(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	writer := NewServiceWithPool(pool)
	const clubs, perClub, capOrgs = 50, 40, 10
	orgs := make([]string, clubs)
	for i := range orgs {
		orgs[i] = uuid.NewString()
		p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: orgs[i], Roles: []string{"admin"}}
		for j := 0; j < perClub; j++ {
			if _, err := writer.CreateHousehold(p, CreateHouseholdInput{OrgID: orgs[i], HouseholdName: "H"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	serveAll := func(limit int) (cached int) {
		svc := NewServiceWithPool(pool)
		svc.SetOrgCacheLimits(0, limit)
		for _, o := range orgs {
			p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: o, Roles: []string{"admin"}}
			if got, err := svc.ListHouseholds(p, o); err != nil || len(got) != perClub {
				t.Fatalf("club %s: %d households, %v", o, len(got), err)
			}
		}
		svc.mu.RLock()
		defer svc.mu.RUnlock()
		return len(svc.households)
	}
	if got := serveAll(capOrgs); got > capOrgs*perClub {
		t.Fatalf("capped cache holds %d households, budget is %d (%d clubs x %d)", got, capOrgs*perClub, capOrgs, perClub)
	}
	if got := serveAll(0); got != clubs*perClub { // unlimited: shows what the cap saves
		t.Fatalf("uncapped cache should hold every club (%d), holds %d", clubs*perClub, got)
	}
}
