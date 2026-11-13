package groups

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xDBFixture is one isolated schema with a household + swimmer seeded the
// way the members service would have written them.
type cov1xDBFixture struct {
	pool      *pgxpool.Pool
	svc       *Service
	org       string
	household string
	swimmer   string
}

func cov1xSeedDB(t *testing.T) cov1xDBFixture {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	f := cov1xDBFixture{pool: pool, svc: NewServiceWithPool(pool), org: uuid.NewString(), household: uuid.NewString(), swimmer: uuid.NewString()}
	cov1xSeedMembers(t, pool, f.org, f.household, f.swimmer)
	return f
}

func cov1xSeedMembers(t *testing.T, pool *pgxpool.Pool, org, household, swimmer string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, keycloak_user_id) VALUES ($1::uuid, $2::uuid, 'Cov Family', $3::uuid)`, household, org, uuid.NewString()); err != nil {
		t.Fatalf("seed household: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Kai', 'Cov', '2014-06-01', 'male', 'active')`, swimmer, org, household); err != nil {
		t.Fatalf("seed swimmer: %v", err)
	}
}

func cov1xSeedUser(t *testing.T, pool *pgxpool.Pool, org string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1::uuid, $2::uuid, $3, $4)`,
		id, org, id+"@example.test", "sub-"+id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func cov1xPrincipals(org string) (admin, parent *appmiddleware.Principal) {
	return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}, nil
}

// cov1xBrokenService is DB-backed but cannot reach Postgres, so every request
// path's re-sync fails.
func cov1xBrokenService(t *testing.T) *Service {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://nobody:nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewServiceWithPool(pool)
}

func TestCov1xNewServiceWithPoolNil(t *testing.T) {
	t.Parallel()
	s := NewServiceWithPool(nil)
	if s.repo != nil {
		t.Fatal("nil pool must keep the service in-memory")
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("in-memory Refresh: %v", err)
	}
}

func TestCov1xRepositoryRoundTrip(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	repo := newRepository(pool)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	coach := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1::uuid, $2::uuid, 'coach@example.test', 'sub-coach')`, coach, orgA); err != nil {
		t.Fatal(err)
	}
	ga := Group{ID: uuid.NewString(), OrgID: orgA, CoachUserID: coach, Code: "SR1", Name: "Seniors", Description: "fast", Type: "competitive", MinAge: cov1xIntPtr(13), MaxAge: cov1xIntPtr(18), IsActive: true, CreatedAt: now, UpdatedAt: now}
	gb := Group{ID: uuid.NewString(), OrgID: orgB, Code: "SR1", Name: "Other club", Type: "recreational", IsActive: true, CreatedAt: now, UpdatedAt: now}
	for _, g := range []Group{ga, gb} {
		if err := repo.UpsertGroup(ctx, g); err != nil {
			t.Fatalf("UpsertGroup: %v", err)
		}
	}
	// Upsert again: ON CONFLICT updates in place.
	ga.Name = "Seniors Renamed"
	ga.UpdatedAt = now.Add(time.Minute)
	if err := repo.UpsertGroup(ctx, ga); err != nil {
		t.Fatalf("UpsertGroup update: %v", err)
	}
	// A coach id that is not a uuid is a persistence error, surfaced to the caller.
	bad := ga
	bad.ID = uuid.NewString()
	bad.Code = "BAD"
	bad.CoachUserID = "not-a-uuid"
	if err := repo.UpsertGroup(ctx, bad); err == nil {
		t.Fatal("expected error for non-uuid coach id")
	}

	rule := EligibilityRule{ID: uuid.NewString(), OrgID: orgA, GroupID: ga.ID, RuleType: "age_range", Parameters: map[string]any{"min_age": 13.0}, CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertEligibilityRule(ctx, rule); err != nil {
		t.Fatalf("UpsertEligibilityRule: %v", err)
	}
	rule.RuleType = "gender"
	rule.Parameters = map[string]any{"allowed": []string{"male"}}
	if err := repo.UpsertEligibilityRule(ctx, rule); err != nil {
		t.Fatalf("UpsertEligibilityRule update: %v", err)
	}

	hh, sw := uuid.NewString(), uuid.NewString()
	cov1xSeedMembers(t, pool, orgA, hh, sw)
	reg := Registration{ID: uuid.NewString(), OrgID: orgA, HouseholdID: hh, SwimmerID: sw, GroupID: ga.ID, Season: "2026", Status: "pending", Notes: "n1", RegistrationDate: "2026-02-03", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertRegistration(ctx, reg); err != nil {
		t.Fatalf("UpsertRegistration: %v", err)
	}
	reg.Status = "active"
	reg.Notes = "n2"
	if err := repo.UpsertRegistration(ctx, reg); err != nil {
		t.Fatalf("UpsertRegistration update: %v", err)
	}
	wv := WaiverAcceptance{ID: uuid.NewString(), OrgID: orgA, RegistrationID: reg.ID, WaiverType: "liability", AcceptedByUserID: coach, AcceptedAt: now, IPAddress: "203.0.113.9"}
	if err := repo.UpsertWaiverAcceptance(ctx, wv); err != nil {
		t.Fatalf("UpsertWaiverAcceptance: %v", err)
	}
	wv.IPAddress = "203.0.113.10"
	if err := repo.UpsertWaiverAcceptance(ctx, wv); err != nil {
		t.Fatalf("UpsertWaiverAcceptance update: %v", err)
	}

	grps, err := repo.LoadGroups(ctx, orgA)
	if err != nil || len(grps) != 1 || grps[0].Name != "Seniors Renamed" || grps[0].Type != "competitive" ||
		grps[0].MinAge == nil || *grps[0].MinAge != 13 || grps[0].CoachUserID != coach {
		t.Fatalf("LoadGroups = %+v, %v", grps, err)
	}
	grpsB, err := repo.LoadGroups(ctx, orgB)
	if err != nil || len(grpsB) != 1 || grpsB[0].Type != "recreational" {
		t.Fatalf("LoadGroups(orgB) = %+v, %v", grpsB, err)
	}
	// A group stored with a blank type loads as competitive.
	if _, err := pool.Exec(ctx, `UPDATE groups SET type = '' WHERE id = $1::uuid`, gb.ID); err != nil {
		t.Fatal(err)
	}
	grpsB, err = repo.LoadGroups(ctx, orgB)
	if err != nil || len(grpsB) != 1 || grpsB[0].Type != "competitive" {
		t.Fatalf("LoadGroups(orgB) = %+v, %v", grpsB, err)
	}
	rules, err := repo.LoadEligibilityRules(ctx, orgA)
	if err != nil || len(rules) != 1 || rules[0].RuleType != "gender" || !rules[0].IsActive || rules[0].Parameters["allowed"] == nil {
		t.Fatalf("LoadEligibilityRules = %+v, %v", rules, err)
	}
	regs, err := repo.LoadRegistrations(ctx, orgA)
	if err != nil || len(regs) != 1 || regs[0].Status != "active" || regs[0].Notes != "n2" || regs[0].RegistrationDate != "2026-02-03" {
		t.Fatalf("LoadRegistrations = %+v, %v", regs, err)
	}
	wvs, err := repo.LoadWaiverAcceptances(ctx, orgA)
	if err != nil || len(wvs) != 1 || wvs[0].IPAddress != "203.0.113.10" || wvs[0].AcceptedByUserID != wv.AcceptedByUserID {
		t.Fatalf("LoadWaiverAcceptances = %+v, %v", wvs, err)
	}
	sws, err := repo.LoadSwimmerRefs(ctx, orgA)
	if err != nil || len(sws) != 1 || sws[0].ID != sw || sws[0].DateOfBirth != "2014-06-01" || sws[0].Gender != "male" {
		t.Fatalf("LoadSwimmerRefs = %+v, %v", sws, err)
	}
	hhs, err := repo.LoadHouseholdRefs(ctx, orgA)
	if err != nil || len(hhs) != 1 || hhs[0].ID != hh || hhs[0].KeycloakUserID == "" {
		t.Fatalf("LoadHouseholdRefs = %+v, %v", hhs, err)
	}

	// Scoped LoadAll sees only its own club; the empty org walks every club.
	g1, r1, re1, w1, s1, h1, err := repo.LoadAll(ctx, orgA)
	if err != nil || len(g1) != 1 || len(r1) != 1 || len(re1) != 1 || len(w1) != 1 || len(s1) != 1 || len(h1) != 1 {
		t.Fatalf("LoadAll(orgA): %d %d %d %d %d %d %v", len(g1), len(r1), len(re1), len(w1), len(s1), len(h1), err)
	}
	gAll, _, _, _, _, _, err := repo.LoadAll(ctx, "")
	if err != nil || len(gAll) != 2 {
		t.Fatalf("LoadAll(all) groups = %d, %v", len(gAll), err)
	}
	ids, err := repo.OrgIDs(ctx)
	if err != nil || len(ids) < 2 {
		t.Fatalf("OrgIDs = %v, %v", ids, err)
	}
}

// Each loader failure must be reported with its own context, in load order.
func TestCov1xRepositoryLoadFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	repo := newRepository(pool)
	ctx := context.Background()
	org := uuid.NewString()
	g := Group{ID: uuid.NewString(), OrgID: org, Code: "X", Name: "X", Type: "competitive", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.UpsertGroup(ctx, g); err != nil {
		t.Fatal(err)
	}

	// Dropping a column the loader selects makes exactly that loader fail (dropping the
	// whole table would fall through to the shared public schema on the search path).
	steps := []struct{ table, column, want string }{
		{"households", "keycloak_user_id", "load household refs"},
		{"swimmers", "gender", "load swimmer refs"},
		{"waiver_acceptances", "ip_address", "load waiver_acceptances"},
		{"registrations", "notes", "load registrations"},
		{"eligibility_rules", "rule_value", "load eligibility_rules"},
		{"groups", "description", "load groups"},
	}
	for _, st := range steps {
		if _, err := pool.Exec(ctx, "ALTER TABLE "+st.table+" DROP COLUMN "+st.column+" CASCADE"); err != nil {
			t.Fatalf("drop %s.%s: %v", st.table, st.column, err)
		}
		_, _, _, _, _, _, err := repo.LoadAll(ctx, org)
		if err == nil || !strings.Contains(err.Error(), st.want) {
			t.Fatalf("after dropping %s: LoadAll err = %v, want %q", st.column, err, st.want)
		}
		// The full walk reports the same failure for the first club it reaches.
		if _, _, _, _, _, _, err := repo.LoadAll(ctx, ""); err == nil {
			t.Fatalf("after dropping %s: LoadAll(all) succeeded", st.column)
		}
	}
	if _, err := repo.LoadGroups(ctx, org); err == nil {
		t.Fatal("LoadGroups on dropped table must fail")
	}
}

func TestCov1xRepositoryAllClubWalkOrgListFailure(t *testing.T) {
	t.Parallel()
	svc := cov1xBrokenService(t)
	if _, _, _, _, _, _, err := svc.repo.LoadAll(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "list orgs") {
		t.Fatalf("LoadAll(all) on unreachable DB = %v", err)
	}
	if err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh must surface the load failure")
	}
}

func TestCov1xDBBackedLifecycleAcrossInstances(t *testing.T) {
	f := cov1xSeedDB(t)
	admin, _ := cov1xPrincipals(f.org)
	parent := &appmiddleware.Principal{UserID: cov1xSeedUser(t, f.pool, f.org), OrgID: f.org, HouseholdID: f.household, Roles: []string{"parent"}}
	ctx := context.Background()

	g, _, err := f.svc.CreateGroup(admin, f.org, CreateGroupInput{Code: "gold", Name: "Gold", MinAge: cov1xIntPtr(8), MaxAge: cov1xIntPtr(12)})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if _, err := uuid.Parse(g.ID); err != nil {
		t.Fatalf("DB mode must mint uuid ids, got %q", g.ID)
	}
	if _, _, err := f.svc.CreateGroup(admin, f.org, CreateGroupInput{Code: "GOLD", Name: "Dup"}); !errors.Is(err, ErrGroupCodeConflict) {
		t.Fatalf("duplicate code: %v", err)
	}
	if _, _, err := f.svc.UpdateGroup(admin, f.org, g.ID, UpdateGroupInput{Name: cov1xStrPtr("Gold Renamed")}); err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if _, _, err := f.svc.AttachEligibilityRule(admin, f.org, g.ID, AttachEligibilityRuleInput{RuleType: "age_range", Parameters: map[string]any{"min_age": 8, "max_age": 12}}); err != nil {
		t.Fatalf("AttachEligibilityRule: %v", err)
	}
	res, err := f.svc.EvaluateSwimmerEligibility(admin, f.org, g.ID, f.swimmer)
	if err != nil || len(res.RuleResults) != 1 {
		t.Fatalf("EvaluateSwimmerEligibility = %+v, %v", res, err)
	}

	reg, err := f.svc.CreateRegistration(parent, f.org, CreateRegistrationInput{HouseholdID: f.household, SwimmerID: f.swimmer, GroupID: g.ID, Season: "2026"})
	if err != nil {
		// The swimmer is born 2014; depending on today's date the age rule may legitimately reject a parent.
		if !errors.Is(err, ErrNotEligible) {
			t.Fatalf("CreateRegistration: %v", err)
		}
		reg, err = f.svc.CreateRegistration(admin, f.org, CreateRegistrationInput{HouseholdID: f.household, SwimmerID: f.swimmer, GroupID: g.ID, Season: "2026"})
		if err != nil {
			t.Fatalf("staff CreateRegistration: %v", err)
		}
	}
	if _, err := f.svc.CreateRegistration(admin, f.org, CreateRegistrationInput{HouseholdID: f.household, SwimmerID: f.swimmer, GroupID: g.ID, Season: "2026"}); !errors.Is(err, ErrDuplicateRegistration) {
		t.Fatalf("duplicate registration: %v", err)
	}
	if _, err := f.svc.AcceptWaiver(parent, f.org, reg.ID, AcceptWaiverInput{WaiverType: "Liability", IPAddress: "198.51.100.4"}); err != nil {
		t.Fatalf("AcceptWaiver: %v", err)
	}
	if _, err := f.svc.AcceptWaiver(parent, f.org, reg.ID, AcceptWaiverInput{WaiverType: "liability"}); !errors.Is(err, ErrWaiverAlreadyAccepted) {
		t.Fatalf("second waiver: %v", err)
	}
	if _, _, err := f.svc.TransitionRegistrationStatus(admin, f.org, reg.ID, "active"); err != nil {
		t.Fatalf("activate: %v", err)
	}

	// A second API instance observes everything without a restart.
	other := NewServiceWithPool(f.pool)
	if got, err := other.GetGroup(admin, f.org, g.ID); err != nil || got.Name != "Gold Renamed" {
		t.Fatalf("other.GetGroup = %+v, %v", got, err)
	}
	regs, err := other.ListRegistrations(admin, f.org)
	if err != nil || len(regs) != 1 || regs[0].Status != "active" || !regs[0].WaiverAccepted || regs[0].WaiverTypes[0] != "liability" {
		t.Fatalf("other.ListRegistrations = %+v, %v", regs, err)
	}
	if n := len(other.SnapshotGroups(f.org)); n != 1 {
		t.Fatalf("SnapshotGroups = %d", n)
	}
	if n := len(other.SnapshotRegistrations(f.org)); n != 1 {
		t.Fatalf("SnapshotRegistrations = %d", n)
	}
	if n := len(other.SnapshotWaivers(f.org)); n != 1 {
		t.Fatalf("SnapshotWaivers = %d", n)
	}
	if rules, err := other.ListEligibilityRules(admin, f.org, g.ID); err != nil || len(rules) != 1 {
		t.Fatalf("other.ListEligibilityRules = %v, %v", rules, err)
	}
	if list, err := other.ListGroups(admin, f.org, true); err != nil || len(list) != 1 {
		t.Fatalf("other.ListGroups = %v, %v", list, err)
	}

	// Withdraw then re-register reopens the same row (persisted through the DB).
	if _, _, err := f.svc.TransitionRegistrationStatus(admin, f.org, reg.ID, "withdrawn"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	re, err := f.svc.CreateRegistration(admin, f.org, CreateRegistrationInput{HouseholdID: f.household, SwimmerID: f.swimmer, GroupID: g.ID, Season: "2026", Notes: "back"})
	if err != nil || re.ID != reg.ID || re.Status != "pending" {
		t.Fatalf("re-register = %+v, %v", re, err)
	}

	// Full reload (Refresh) replaces everything from Postgres; scoped reload does too.
	if err := other.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if err := other.RefreshOrg(ctx, f.org); err != nil {
		t.Fatalf("RefreshOrg: %v", err)
	}
	other.SetOrgCacheLimits(time.Hour, 5)
	if got := other.SnapshotRegistrations(f.org); len(got) != 1 || got[0].Notes != "back" {
		t.Fatalf("after refresh registrations = %+v", got)
	}
}

// A writer that lands during every lock-free read forces the reload to fall back
// to loading under the write lock; a failure there must still reach the caller.
func TestCov1xSyncFallsBackToLockedReload(t *testing.T) {
	f := cov1xSeedDB(t)
	admin, _ := cov1xPrincipals(f.org)
	if _, _, err := f.svc.CreateGroup(admin, f.org, CreateGroupInput{Code: "LK", Name: "Lock"}); err != nil {
		t.Fatal(err)
	}

	other := NewServiceWithPool(f.pool)
	calls := 0
	lockedPhase := false
	other.SetSyncHookForTest(func() {
		calls++
		if calls <= 3 {
			// Simulate a concurrent writer: bump the write generation.
			other.mu.Lock()
			other.mu.Unlock()
			return
		}
		lockedPhase = true
	})
	if err := other.RefreshOrg(context.Background(), f.org); err != nil {
		t.Fatalf("RefreshOrg with contention: %v", err)
	}
	if !lockedPhase || calls != 4 {
		t.Fatalf("expected 3 lock-free conflicts then a locked load, calls=%d locked=%v", calls, lockedPhase)
	}
	if got := other.SnapshotGroups(f.org); len(got) != 1 || got[0].Code != "LK" {
		t.Fatalf("groups after locked reload = %+v", got)
	}

	// Same contention, but the locked load itself fails.
	failing := NewServiceWithPool(f.pool)
	n := 0
	failing.SetSyncHookForTest(func() {
		n++
		if n == 3 {
			if _, err := f.pool.Exec(context.Background(), "ALTER TABLE households DROP COLUMN keycloak_user_id CASCADE"); err != nil {
				t.Errorf("drop: %v", err)
			}

		}
		if n <= 3 {
			failing.mu.Lock()
			failing.mu.Unlock()
		}
	})
	if err := failing.RefreshOrg(context.Background(), f.org); err == nil || !strings.Contains(err.Error(), "load household refs") {
		t.Fatalf("locked reload failure = %v", err)
	}
}

func TestCov1xRequestPathsSurfaceSyncFailures(t *testing.T) {
	t.Parallel()
	svc := cov1xBrokenService(t)
	org := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	gid, sid, rid := uuid.NewString(), uuid.NewString(), uuid.NewString()

	calls := map[string]func() error{
		"CreateGroup": func() error {
			_, _, err := svc.CreateGroup(admin, org, CreateGroupInput{Code: "A", Name: "A"})
			return err
		},
		"UpdateGroup": func() error {
			_, _, err := svc.UpdateGroup(admin, org, gid, UpdateGroupInput{Name: cov1xStrPtr("B")})
			return err
		},
		"GetGroup":   func() error { _, err := svc.GetGroup(admin, org, gid); return err },
		"ListGroups": func() error { _, err := svc.ListGroups(admin, org, false); return err },
		"AttachEligibilityRule": func() error {
			_, _, err := svc.AttachEligibilityRule(admin, org, gid, AttachEligibilityRuleInput{RuleType: "gender"})
			return err
		},
		"EvaluateSwimmerEligibility": func() error {
			_, err := svc.EvaluateSwimmerEligibility(admin, org, gid, sid)
			return err
		},
		"ListEligibilityRules": func() error { _, err := svc.ListEligibilityRules(admin, org, gid); return err },
		"CreateRegistration": func() error {
			_, err := svc.CreateRegistration(admin, org, CreateRegistrationInput{HouseholdID: "h", SwimmerID: sid, GroupID: gid, Season: "s"})
			return err
		},
		"TransitionRegistrationStatus": func() error {
			_, _, err := svc.TransitionRegistrationStatus(admin, org, rid, "active")
			return err
		},
		"AcceptWaiver": func() error {
			_, err := svc.AcceptWaiver(admin, org, rid, AcceptWaiverInput{WaiverType: "liability"})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: expected sync failure, got nil", name)
		} else if errors.Is(err, ErrForbidden) {
			t.Errorf("%s: failed on authorization instead of sync: %v", name, err)
		}
	}
	if got := svc.SnapshotGroups(org); got == nil || len(got) != 0 {
		t.Errorf("SnapshotGroups on sync failure = %#v, want empty non-nil", got)
	}
	if got := svc.SnapshotRegistrations(org); got == nil || len(got) != 0 {
		t.Errorf("SnapshotRegistrations on sync failure = %#v", got)
	}
	if got := svc.SnapshotWaivers(org); got == nil || len(got) != 0 {
		t.Errorf("SnapshotWaivers on sync failure = %#v", got)
	}
}

// When Postgres rejects the write after a successful sync, the caller gets the
// error and memory is left untouched.
func TestCov1xPersistFailuresAreReturned(t *testing.T) {
	f := cov1xSeedDB(t)
	admin, _ := cov1xPrincipals(f.org)
	ctx := context.Background()
	deny := func(table string) {
		t.Helper()
		stmt := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT cov1x_deny CHECK (false) NOT VALID", table)
		if _, err := f.pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("deny writes on %s: %v", table, err)
		}
	}
	allow := func(table string) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT cov1x_deny", table)); err != nil {
			t.Fatalf("allow writes on %s: %v", table, err)
		}
	}

	// groups: create + update
	g, _, err := f.svc.CreateGroup(admin, f.org, CreateGroupInput{Code: "P1", Name: "Persist"})
	if err != nil {
		t.Fatal(err)
	}
	deny("groups")
	if _, _, err := f.svc.CreateGroup(admin, f.org, CreateGroupInput{Code: "P2", Name: "Nope"}); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("CreateGroup persist failure = %v", err)
	}
	if _, _, err := f.svc.UpdateGroup(admin, f.org, g.ID, UpdateGroupInput{Name: cov1xStrPtr("Changed")}); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("UpdateGroup persist failure = %v", err)
	}
	allow("groups")
	if got, err := f.svc.GetGroup(admin, f.org, g.ID); err != nil || got.Name != "Persist" {
		t.Fatalf("group must be unchanged after failed update: %+v, %v", got, err)
	}

	// eligibility_rules
	deny("eligibility_rules")
	if _, _, err := f.svc.AttachEligibilityRule(admin, f.org, g.ID, AttachEligibilityRuleInput{RuleType: "gender", Parameters: map[string]any{"allowed": []string{"male"}}}); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("Attach persist failure = %v", err)
	}
	allow("eligibility_rules")
	if rules, err := f.svc.ListEligibilityRules(admin, f.org, g.ID); err != nil || len(rules) != 0 {
		t.Fatalf("failed attach must not leave a rule: %v, %v", rules, err)
	}

	// registrations: brand-new, transition, and reopen-after-withdrawal
	in := CreateRegistrationInput{HouseholdID: f.household, SwimmerID: f.swimmer, GroupID: g.ID, Season: "2027"}
	deny("registrations")
	if _, err := f.svc.CreateRegistration(admin, f.org, in); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("CreateRegistration persist failure = %v", err)
	}
	allow("registrations")
	reg, err := f.svc.CreateRegistration(admin, f.org, in)
	if err != nil {
		t.Fatal(err)
	}
	deny("registrations")
	if _, _, err := f.svc.TransitionRegistrationStatus(admin, f.org, reg.ID, "active"); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("transition persist failure = %v", err)
	}
	allow("registrations")
	if _, _, err := f.svc.TransitionRegistrationStatus(admin, f.org, reg.ID, "withdrawn"); err != nil {
		t.Fatal(err)
	}
	deny("registrations")
	if _, err := f.svc.CreateRegistration(admin, f.org, in); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("reopen persist failure = %v", err)
	}
	allow("registrations")

	// waiver_acceptances
	deny("waiver_acceptances")
	if _, err := f.svc.AcceptWaiver(admin, f.org, reg.ID, AcceptWaiverInput{WaiverType: "liability"}); err == nil || !strings.Contains(err.Error(), "cov1x_deny") {
		t.Fatalf("AcceptWaiver persist failure = %v", err)
	}
	allow("waiver_acceptances")
	if n := len(f.svc.SnapshotWaivers(f.org)); n != 0 {
		t.Fatalf("failed waiver must not be recorded, got %d", n)
	}
}

func TestCov1xParentRegistrationSkipsOtherGroupsRules(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := cov1xAdmin()
	parent := cov1xParent("parent-x", "hh-x")
	svc.SeedSwimmer(cov1xOrg, "sw-x", "hh-x", "2014-01-01", "male", "active")
	svc.SeedParentHousehold("parent-x", "hh-x")
	strict := cov1xNewGroup(t, svc, "STRICT")
	open := cov1xNewGroup(t, svc, "OPEN2")
	// The strict group's rule would reject this swimmer, but it belongs to another group.
	if _, _, err := svc.AttachEligibilityRule(admin, cov1xOrg, strict.ID, AttachEligibilityRuleInput{RuleType: "gender", Parameters: map[string]any{"allowed": []string{"female"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRegistration(parent, cov1xOrg, CreateRegistrationInput{HouseholdID: "hh-x", SwimmerID: "sw-x", GroupID: open.ID, Season: "2026"}); err != nil {
		t.Fatalf("registration to the unrestricted group: %v", err)
	}
	if _, err := svc.CreateRegistration(parent, cov1xOrg, CreateRegistrationInput{HouseholdID: "hh-x", SwimmerID: "sw-x", GroupID: strict.ID, Season: "2026"}); !errors.Is(err, ErrNotEligible) {
		t.Fatalf("registration to the restricted group: %v", err)
	}
}

func TestCov1xListRegistrationsOrdersEqualDatesNewestFirst(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := cov1xAdmin()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	const n = 10 // below the insertion-sort cutoff, so every adjacent pair is compared
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("reg-%02d", i)
		svc.registrations[id] = Registration{ID: id, OrgID: cov1xOrg, RegistrationDate: "2026-03-01", CreatedAt: base.Add(time.Duration(i) * time.Minute)}
	}
	got, err := svc.ListRegistrations(admin, cov1xOrg)
	if err != nil || len(got) != n {
		t.Fatalf("ListRegistrations = %d, %v", len(got), err)
	}
	for i, r := range got {
		if want := fmt.Sprintf("reg-%02d", n-1-i); r.ID != want {
			t.Fatalf("position %d = %s, want %s (newest first)", i, r.ID, want)
		}
	}
}
