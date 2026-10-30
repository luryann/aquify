package app

// Integration tests for the DB-backed repository layer (Step 29A).
//
// These tests require DATABASE_URL to be set and the schema to be migrated.
// They run a write-through round-trip: create records via the service (which
// persists to DB), then instantiate a fresh service with NewServiceWithPool
// and verify that the records are reloaded from the DB on startup.
//
// Usage:
//   DATABASE_URL=postgres://... go test -run TestDBRepo ./internal/app/...

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openTestPool returns a pgxpool.Pool from DATABASE_URL. The test is skipped
// only when DATABASE_URL is unset; when it is set but unreachable the test
// FAILS, so a broken database can never turn DB coverage into silent passes.
func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping DB repo integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("DATABASE_URL is set but the pool cannot be opened: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("DATABASE_URL is set but the database is unreachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// deleteFixtures runs the DELETE statements for orgID now (so the test starts
// clean) and again via t.Cleanup (so it leaves nothing behind). A failed DELETE
// fails the test instead of leaving dirty state for the next one.
func deleteFixtures(t *testing.T, pool *pgxpool.Pool, orgID string, queries []string) {
	t.Helper()
	run := func(report func(format string, args ...any)) {
		for _, q := range queries {
			if _, err := pool.Exec(context.Background(), q, orgID); err != nil {
				report("fixture cleanup failed (%s): %v", q, err)
				return
			}
		}
	}
	run(t.Fatalf)
	t.Cleanup(func() { run(t.Errorf) })
}

// seedRow runs a fixture INSERT and fails the test if it errors, so a bad seed
// surfaces here instead of as a confusing FK violation later.
func seedRow(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("seed fixture (%s): %v", query, err)
	}
}

// testOrgID is a valid UUID for integration tests.
const testOrgID = "10000000-0000-0000-0000-000000000001"

// testAdminUserID backs created_by/updated_by columns, which are UUID FKs to
// users(id) — production actors are Keycloak UUIDs synced into users at login.
const testAdminUserID = "10000000-0000-0000-0000-0000000000a1"

// dbAdminPrincipal returns an admin principal whose user exists in the users
// table so audit columns satisfy their foreign keys.
func dbAdminPrincipal(t *testing.T, pool *pgxpool.Pool) *appmiddleware.Principal {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, org_id, email, oidc_subject)
		VALUES ($1::uuid, $2::uuid, 'db-test-admin@example.com', 'db-test-admin')
		ON CONFLICT DO NOTHING`, testAdminUserID, testOrgID); err != nil {
		t.Fatalf("seed test admin user: %v", err)
	}
	admin := testutil.AdminPrincipal()
	admin.UserID = testAdminUserID
	return admin
}

// ─── Members ────────────────────────────────────────────────────────────────

func TestDBRepoMembers_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)

	// Truncate to ensure clean slate for this test's org data.
	cleanupMembersFixtures(t, pool, testOrgID)

	// Write via DB-backed service.
	svc := members.NewServiceWithPool(pool)

	household, err := svc.CreateHousehold(admin, members.CreateHouseholdInput{
		OrgID:         testOrgID,
		HouseholdName: "DBRepo Family",
		PrimaryEmail:  "dbrepo@example.com",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	swimmer, _, err := svc.CreateSwimmer(admin, testOrgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "DB",
		LastName:           "Swimmer",
		DateOfBirth:        "2012-06-15",
		Gender:             "female",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}

	// Reload from DB via a fresh service instance.
	svc2 := members.NewServiceWithPool(pool)

	got, err := svc2.GetHousehold(admin, testOrgID, household.ID)
	if err != nil {
		t.Fatalf("GetHousehold after reload: %v", err)
	}
	if got.HouseholdName != "DBRepo Family" {
		t.Errorf("expected household_name 'DBRepo Family', got %q", got.HouseholdName)
	}

	swimmers, err := svc2.ListSwimmers(admin, testOrgID, members.ListSwimmersInput{HouseholdID: household.ID})
	if err != nil {
		t.Fatalf("ListSwimmers after reload: %v", err)
	}
	if len(swimmers) != 1 {
		t.Fatalf("expected 1 swimmer after reload, got %d", len(swimmers))
	}
	if swimmers[0].ID != swimmer.ID {
		t.Errorf("swimmer ID mismatch: want %q got %q", swimmer.ID, swimmers[0].ID)
	}
}

func TestDBRepoMembers_CrossInstanceVisibilityWithoutRestart(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)

	svc1 := members.NewServiceWithPool(pool)
	svc2 := members.NewServiceWithPool(pool)

	household, err := svc1.CreateHousehold(admin, members.CreateHouseholdInput{
		OrgID:         testOrgID,
		HouseholdName: "Cross Instance Family",
		PrimaryEmail:  "cross@example.com",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	swimmer, _, err := svc1.CreateSwimmer(admin, testOrgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Casey",
		LastName:           "Lane",
		DateOfBirth:        "2013-05-10",
		Gender:             "female",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}

	got, err := svc2.GetSwimmer(admin, testOrgID, swimmer.ID, true)
	if err != nil {
		t.Fatalf("GetSwimmer from second instance: %v", err)
	}
	if got.FirstName != "Casey" {
		t.Fatalf("expected first_name Casey, got %q", got.FirstName)
	}

	renamed := "Casey Updated"
	_, _, err = svc1.UpdateSwimmer(admin, testOrgID, swimmer.ID, members.UpdateSwimmerInput{
		FirstName: &renamed,
	})
	if err != nil {
		t.Fatalf("UpdateSwimmer: %v", err)
	}

	got, err = svc2.GetSwimmer(admin, testOrgID, swimmer.ID, true)
	if err != nil {
		t.Fatalf("GetSwimmer after update: %v", err)
	}
	if got.FirstName != renamed {
		t.Fatalf("expected updated first_name %q, got %q", renamed, got.FirstName)
	}

	if err := svc1.LinkHouseholdToUser(testOrgID, household.ID, "kc-user-cross"); err != nil {
		t.Fatalf("LinkHouseholdToUser: %v", err)
	}
	linked, found := svc2.GetHouseholdByKeycloakUserID("kc-user-cross")
	if !found || linked.ID != household.ID {
		t.Fatalf("expected linked household %q from second instance, got found=%t id=%q", household.ID, found, linked.ID)
	}
}

// A write must never be wiped from memory by a concurrent request that loaded
// its snapshot from Postgres just before the write committed.
func TestDBRepoMembers_ConcurrentSyncDoesNotLoseWrites(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)

	svc := members.NewServiceWithPool(pool)
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 12; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = svc.ListHouseholds(admin, testOrgID)
				}
			}
		}()
	}
	defer func() { close(stop); readers.Wait() }()

	for i := 0; i < 60; i++ {
		household, err := svc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: testOrgID, HouseholdName: "Race Family"})
		if err != nil {
			t.Fatalf("CreateHousehold %d: %v", i, err)
		}
		// Snapshot reads memory without syncing, so it exposes a write that a
		// concurrent stale reload wiped.
		found := false
		for _, h := range svc.SnapshotHouseholds(testOrgID) {
			found = found || h.ID == household.ID
		}
		if !found {
			t.Fatalf("household %s created in iteration %d is missing from memory right after the write", household.ID, i)
		}
	}
}

// writeInsideSyncWindow returns a sync hook that, the first time it fires while
// armed, runs write on another goroutine and waits up to 300ms for it. Against
// a reload that reads Postgres before taking the lock the write finishes inside
// that window and the stale snapshot then replaces it; against a reload that
// holds the lock while reading, the write is blocked until the reload is done.
// wait blocks until the write has finished either way.
func writeInsideSyncWindow(armed *atomic.Bool, write func()) (hook func(), wait func()) {
	done := make(chan struct{})
	started := make(chan struct{})
	hook = func() {
		if !armed.CompareAndSwap(true, false) {
			return
		}
		close(started)
		go func() { write(); close(done) }()
		select {
		case <-done:
		case <-time.After(300 * time.Millisecond):
		}
	}
	wait = func() {
		select {
		case <-started:
			<-done
		default:
		}
	}
	return hook, wait
}

func TestDBRepoMembers_WriteInsideReloadWindowSurvives(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)
	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)

	svc := members.NewServiceWithPool(pool)
	var armed atomic.Bool
	var created members.Household
	var createErr error
	hook, wait := writeInsideSyncWindow(&armed, func() {
		created, createErr = svc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: testOrgID, HouseholdName: "Window Family"})
	})
	svc.SetSyncHookForTest(hook)

	armed.Store(true)
	_, _ = svc.ListHouseholds(admin, testOrgID) // reload that the write lands inside
	wait()
	if createErr != nil {
		t.Fatalf("CreateHousehold: %v", createErr)
	}
	for _, h := range svc.SnapshotHouseholds(testOrgID) {
		if h.ID == created.ID {
			return
		}
	}
	t.Fatalf("household %s written during a reload is missing from memory", created.ID)
}

// Groups reads re-sync before looking, so a lost write is only visible through a
// read-modify-write: a rename that lands while another request is reloading must
// not be overwritten by that request's stale copy of the group.
func TestDBRepoGroups_UpdateInsideReloadWindowIsNotLost(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)
	admin := dbAdminPrincipal(t, pool)
	cleanupGroupsFixtures(t, pool, testOrgID)

	svc := groups.NewServiceWithPool(pool)
	group, _, err := svc.CreateGroup(admin, testOrgID, groups.CreateGroupInput{Code: "WINDOW", Name: "Original Name"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	var armed atomic.Bool
	var renameErr error
	hook, wait := writeInsideSyncWindow(&armed, func() {
		renamed := "Renamed"
		_, _, renameErr = svc.UpdateGroup(admin, testOrgID, group.ID, groups.UpdateGroupInput{Name: &renamed})
	})
	svc.SetSyncHookForTest(hook)

	armed.Store(true)
	description := "New description"
	if _, _, err := svc.UpdateGroup(admin, testOrgID, group.ID, groups.UpdateGroupInput{Description: &description}); err != nil {
		t.Fatalf("UpdateGroup (description): %v", err)
	}
	wait()
	if renameErr != nil {
		t.Fatalf("UpdateGroup (rename): %v", renameErr)
	}

	svc.SetSyncHookForTest(nil)
	fresh := groups.NewServiceWithPool(pool)
	for _, g := range fresh.SnapshotGroups(testOrgID) {
		if g.ID == group.ID {
			if g.Name != "Renamed" || g.Description != description {
				t.Fatalf("concurrent updates were lost: name=%q description=%q, want %q and %q", g.Name, g.Description, "Renamed", description)
			}
			return
		}
	}
	t.Fatal("group missing after updates")
}

func TestDBRepoMeets_WriteInsideReloadWindowSurvives(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)
	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)
	cleanupMeetsFixtures(t, pool, testOrgID)

	svc := meets.NewServiceWithPool(pool)
	var armed atomic.Bool
	var created meets.Meet
	var createErr error
	hook, wait := writeInsideSyncWindow(&armed, func() {
		created, _, createErr = svc.CreateMeet(admin, testOrgID, meets.CreateMeetInput{Name: "Window Meet", MeetType: "invitational", Course: "SCY"})
	})
	svc.SetSyncHookForTest(hook)

	armed.Store(true)
	_, _ = svc.ListMeets(admin, testOrgID)
	wait()
	if createErr != nil {
		t.Fatalf("CreateMeet: %v", createErr)
	}
	for _, m := range svc.SnapshotMeets(testOrgID) {
		if m.ID == created.ID {
			return
		}
	}
	t.Fatalf("meet %s written during a reload is missing from memory", created.ID)
}

func TestDBRepoMembers_DeleteSwimmerRejectsWhenDependenciesExist(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID

	cleanupPerformanceFixtures(t, pool, testOrgID)
	cleanupMeetsFixtures(t, pool, testOrgID)
	cleanupMembersFixtures(t, pool, testOrgID)

	svc := members.NewServiceWithPool(pool)

	household, err := svc.CreateHousehold(admin, members.CreateHouseholdInput{
		OrgID:         testOrgID,
		HouseholdName: "Delete Guard DB Family",
		PrimaryEmail:  "delete-guard@example.com",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	swimmer, _, err := svc.CreateSwimmer(admin, testOrgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Guarded",
		LastName:           "Swimmer",
		DateOfBirth:        "2012-06-15",
		Gender:             "female",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}

	meetID := "20000000-0000-0000-0000-000000000099"
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO meets (id, org_id, name, meet_type, course, status) VALUES ($1::uuid, $2::uuid, 'Delete Guard Meet', 'invitational', 'SCY', 'draft')`, meetID, testOrgID); err != nil {
		t.Fatalf("insert meet: %v", err)
	}

	finTime := 65432
	perfSvc := performance.NewServiceWithPool(pool)
	if _, err := perfSvc.UpsertResult(testOrgID, admin.UserID, admin, performance.UpsertResultInput{
		SwimmerID:  swimmer.ID,
		MeetID:     meetID,
		Distance:   100,
		Stroke:     "free",
		Course:     "SCY",
		Gender:     "F",
		FinTimeMs:  &finTime,
		IsOfficial: true,
	}); err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}

	_, dependencies, err := svc.DeleteSwimmer(admin, testOrgID, swimmer.ID)
	if !errors.Is(err, members.ErrSwimmerHasDependencies) {
		t.Fatalf("expected ErrSwimmerHasDependencies, got %v", err)
	}
	if len(dependencies) == 0 {
		t.Fatal("expected swimmer delete dependencies, got none")
	}

	foundResults := false
	for _, dependency := range dependencies {
		if dependency.Code == "results" && dependency.Count > 0 {
			foundResults = true
			break
		}
	}
	if !foundResults {
		t.Fatalf("expected results dependency in %#v", dependencies)
	}
}

// membersFixtureQueries lists the DELETEs (FK-safe order) that clear the fixtures.
var membersFixtureQueries = []string{
	"DELETE FROM results WHERE org_id = $1::uuid",
	"DELETE FROM swimmer_groups WHERE org_id = $1::uuid",
	"DELETE FROM swimmers WHERE org_id = $1::uuid",
	"DELETE FROM contacts WHERE org_id = $1::uuid",
	"DELETE FROM households WHERE org_id = $1::uuid",
}

func cleanupMembersFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, membersFixtureQueries)
}

// ─── Groups ─────────────────────────────────────────────────────────────────

func TestDBRepoGroups_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)

	cleanupGroupsFixtures(t, pool, testOrgID)

	svc := groups.NewServiceWithPool(pool)

	group, _, err := svc.CreateGroup(admin, testOrgID, groups.CreateGroupInput{
		Code: "dbrepo-gold",
		Name: "DB Repo Gold",
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// Fresh service loads from DB.
	svc2 := groups.NewServiceWithPool(pool)
	gs := svc2.SnapshotGroups(testOrgID)
	found := false
	for _, g := range gs {
		if g.ID == group.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("group %q not found after DB reload", group.ID)
	}
}

func TestDBRepoGroups_CrossInstanceConflictAndVisibility(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)
	cleanupGroupsFixtures(t, pool, testOrgID)

	membersSvc := members.NewServiceWithPool(pool)
	household, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{
		OrgID:         testOrgID,
		HouseholdName: "Groups Cross Instance",
		PrimaryEmail:  "groups-cross@example.com",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	swimmer, _, err := membersSvc.CreateSwimmer(admin, testOrgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Jordan",
		LastName:           "Reed",
		DateOfBirth:        "2012-03-03",
		Gender:             "male",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}

	svc1 := groups.NewServiceWithPool(pool)
	svc2 := groups.NewServiceWithPool(pool)

	group, _, err := svc1.CreateGroup(admin, testOrgID, groups.CreateGroupInput{
		Code: "step32e-gold",
		Name: "Step32E Gold",
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	groupsFromSvc2, err := svc2.ListGroups(admin, testOrgID, false)
	if err != nil {
		t.Fatalf("ListGroups from second instance: %v", err)
	}
	if len(groupsFromSvc2) != 1 || groupsFromSvc2[0].ID != group.ID {
		t.Fatalf("expected second instance to see created group %q, got %+v", group.ID, groupsFromSvc2)
	}

	var (
		wg      sync.WaitGroup
		results = make(chan error, 2)
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := svc1.CreateGroup(admin, testOrgID, groups.CreateGroupInput{Code: "step32e-dup", Name: "Dup A"})
		results <- err
	}()
	go func() {
		defer wg.Done()
		_, _, err := svc2.CreateGroup(admin, testOrgID, groups.CreateGroupInput{Code: "step32e-dup", Name: "Dup B"})
		results <- err
	}()
	wg.Wait()
	close(results)

	var successCount, conflictCount int
	for err := range results {
		if err == nil {
			successCount++
			continue
		}
		if err == groups.ErrGroupCodeConflict {
			conflictCount++
			continue
		}
		t.Fatalf("unexpected CreateGroup error: %v", err)
	}
	if successCount != 1 || conflictCount != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", successCount, conflictCount)
	}

	registration, err := svc1.CreateRegistration(admin, testOrgID, groups.CreateRegistrationInput{
		HouseholdID: household.ID,
		SwimmerID:   swimmer.ID,
		GroupID:     group.ID,
		Season:      "2026",
	})
	if err != nil {
		t.Fatalf("CreateRegistration: %v", err)
	}

	if _, err := svc2.CreateRegistration(admin, testOrgID, groups.CreateRegistrationInput{
		HouseholdID: household.ID,
		SwimmerID:   swimmer.ID,
		GroupID:     group.ID,
		Season:      "2026",
	}); err != groups.ErrDuplicateRegistration {
		t.Fatalf("expected ErrDuplicateRegistration from second instance, got %v", err)
	}

	registrations := svc2.SnapshotRegistrations(testOrgID)
	if len(registrations) != 1 {
		t.Fatalf("expected 1 registration after reload, got %d", len(registrations))
	}
	if registrations[0].ID != registration.ID {
		t.Fatalf("expected registration id %q after reload, got %q", registration.ID, registrations[0].ID)
	}
	if registrations[0].Notes != "" {
		t.Fatalf("expected empty notes to round-trip as empty string, got %q", registrations[0].Notes)
	}

	if _, err := svc1.AcceptWaiver(admin, testOrgID, registration.ID, groups.AcceptWaiverInput{WaiverType: "code-of-conduct"}); err != nil {
		t.Fatalf("AcceptWaiver: %v", err)
	}
	if _, err := svc2.AcceptWaiver(admin, testOrgID, registration.ID, groups.AcceptWaiverInput{WaiverType: "code-of-conduct"}); err != groups.ErrWaiverAlreadyAccepted {
		t.Fatalf("expected ErrWaiverAlreadyAccepted from second instance, got %v", err)
	}
}

func cleanupGroupsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{
		"DELETE FROM waiver_acceptances WHERE org_id = $1::uuid",
		"DELETE FROM registrations WHERE org_id = $1::uuid",
		"DELETE FROM eligibility_rules WHERE org_id = $1::uuid",
		"DELETE FROM groups WHERE org_id = $1::uuid",
	})
}

// ─── Training ───────────────────────────────────────────────────────────────

func TestDBRepoTraining_CrossInstanceVisibilityWithoutRestart(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupPracticesFixtures(t, pool, testOrgID)

	svc1 := training.NewServiceWithPool(pool)
	svc2 := training.NewServiceWithPool(pool)

	attendance := 18
	practice, err := svc1.Create(
		testOrgID,
		"2026-03-26",
		"group-training",
		"water",
		"Threshold set",
		4800,
		&attendance,
		training.PracticeStrokes{Free: 3000, Back: 600, Breast: 400, Fly: 400, IM: 400},
		"",
		"coach-step32e",
	)
	if err != nil {
		t.Fatalf("Create practice: %v", err)
	}

	practices, err := svc2.List(testOrgID)
	if err != nil {
		t.Fatalf("List practices from second instance: %v", err)
	}
	if len(practices) != 1 || practices[0].ID != practice.ID {
		t.Fatalf("expected second instance to see created practice %q, got %+v", practice.ID, practices)
	}

	description := "Threshold set updated"
	updated, err := svc1.Update(testOrgID, practice.ID, training.PracticeUpdates{
		Description: &description,
	}, "coach-step32e")
	if err != nil {
		t.Fatalf("Update practice: %v", err)
	}
	if updated.Description != description {
		t.Fatalf("expected updated description %q, got %q", description, updated.Description)
	}

	got, err := svc2.Get(testOrgID, practice.ID)
	if err != nil {
		t.Fatalf("Get practice after update: %v", err)
	}
	if got.Description != description {
		t.Fatalf("expected second instance to see description %q, got %q", description, got.Description)
	}

	if err := svc1.Delete(testOrgID, practice.ID); err != nil {
		t.Fatalf("Delete practice: %v", err)
	}
	if _, err := svc2.Get(testOrgID, practice.ID); err != training.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete from second instance, got %v", err)
	}
}

func cleanupPracticesFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{`DELETE FROM practices WHERE org_id = $1`})
}

// ─── Meets ──────────────────────────────────────────────────────────────────

// Same lost-write guard as the members test: concurrent syncing readers must
// never wipe a just-committed write from memory.
func TestDBRepoGroups_ConcurrentSyncDoesNotLoseWrites(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	cleanupGroupsFixtures(t, pool, testOrgID)

	svc := groups.NewServiceWithPool(pool)
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 12; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = svc.ListGroups(admin, testOrgID, true)
				}
			}
		}()
	}
	defer func() { close(stop); readers.Wait() }()

	for i := 0; i < 300; i++ {
		group, _, err := svc.CreateGroup(admin, testOrgID, groups.CreateGroupInput{Code: fmt.Sprintf("RACE%03d", i), Name: "Race Group"})
		if err != nil {
			t.Fatalf("CreateGroup %d: %v", i, err)
		}
		found := false
		for _, g := range svc.SnapshotGroups(testOrgID) {
			found = found || g.ID == group.ID
		}
		if !found {
			t.Fatalf("group %s created in iteration %d is missing from memory right after the write", group.ID, i)
		}
	}
}

// A meet written through one API instance must be usable on another without a
// restart (every meets entry point re-syncs from Postgres).
func TestDBRepoMeets_CrossInstanceVisibilityWithoutRestart(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)
	cleanupMeetsFixtures(t, pool, testOrgID)

	svc1 := meets.NewServiceWithPool(pool)
	svc2 := meets.NewServiceWithPool(pool)

	meet, _, err := svc1.CreateMeet(admin, testOrgID, meets.CreateMeetInput{Name: "Cross Instance Meet", MeetType: "invitational", Course: "SCY"})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	listed, err := svc2.ListMeets(admin, testOrgID)
	if err != nil || len(listed) != 1 || listed[0].ID != meet.ID {
		t.Fatalf("second instance ListMeets = %+v, err %v; want the meet created on the first", listed, err)
	}
	if _, _, err := svc2.TransitionMeetStatus(admin, testOrgID, meet.ID, "published"); err != nil {
		t.Fatalf("second instance cannot act on a meet the first created: %v", err)
	}
	got, err := svc1.GetMeet(admin, testOrgID, meet.ID)
	if err != nil || got.Status != "published" {
		t.Fatalf("first instance GetMeet = %+v, err %v; want status published from the second instance's write", got, err)
	}
}

func TestDBRepoMeets_ConcurrentSyncDoesNotLoseWrites(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)
	cleanupMeetsFixtures(t, pool, testOrgID)

	svc := meets.NewServiceWithPool(pool)
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = svc.ListCommitments(admin, testOrgID, "race-probe") // syncs from Postgres before it looks the meet up
				}
			}
		}()
	}
	defer func() { close(stop); readers.Wait() }()

	for i := 0; i < 300; i++ {
		meet, _, err := svc.CreateMeet(admin, testOrgID, meets.CreateMeetInput{Name: fmt.Sprintf("Race Meet %d", i), MeetType: "invitational", Course: "SCY"})
		if err != nil {
			t.Fatalf("CreateMeet %d: %v", i, err)
		}
		found := false
		for _, m := range svc.SnapshotMeets(testOrgID) {
			found = found || m.ID == meet.ID
		}
		if !found {
			t.Fatalf("meet %s created in iteration %d is missing from memory right after the write", meet.ID, i)
		}
	}
}

func TestDBRepoMeets_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	admin.OrgID = testOrgID
	cleanupMembersFixtures(t, pool, testOrgID)
	cleanupMeetsFixtures(t, pool, testOrgID)

	memberSvc := members.NewServiceWithPool(pool)
	household, err := memberSvc.CreateHousehold(admin, members.CreateHouseholdInput{
		OrgID:         testOrgID,
		HouseholdName: "DB Repo Meets Family",
		PrimaryEmail:  "dbrepo-meets@example.com",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	swimmer, _, err := memberSvc.CreateSwimmer(admin, testOrgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Meet",
		LastName:           "Swimmer",
		DateOfBirth:        "2012-06-15",
		Gender:             "female",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}

	svc := meets.NewServiceWithPool(pool)

	meet, _, err := svc.CreateMeet(admin, testOrgID, meets.CreateMeetInput{
		Name:     "DB Repo Spring Invite",
		MeetType: "invitational",
		Course:   "SCY",
		Sessions: []meets.CreateMeetSessionInput{
			{
				SessionNumber: 1,
				Name:          "Timed Finals",
				SessionDate:   "2026-06-15",
				WarmupTime:    "07:30:00",
				StartTime:     "08:30:00",
				Notes:         "Morning session",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, _, err := svc.TransitionMeetStatus(admin, testOrgID, meet.ID, "published"); err != nil {
		t.Fatalf("TransitionMeetStatus: %v", err)
	}

	svc2 := meets.NewServiceWithPool(pool)
	ms := svc2.SnapshotMeets(testOrgID)
	found := false
	for _, m := range ms {
		if m.ID == meet.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("meet %q not found after DB reload", meet.ID)
	}

	sessions, err := svc2.ListMeetSessions(admin, testOrgID, meet.ID)
	if err != nil {
		t.Fatalf("ListMeetSessions after reload: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after reload, got %d", len(sessions))
	}
	if sessions[0].WarmupTime != "07:30:00" {
		t.Fatalf("expected warmup_time %q, got %q", "07:30:00", sessions[0].WarmupTime)
	}

	_, commitments, _, err := svc2.PublishCommitmentRequest(admin, testOrgID, meet.ID)
	if err != nil {
		t.Fatalf("PublishCommitmentRequest after reload: %v", err)
	}
	if len(commitments) != 1 {
		t.Fatalf("expected 1 commitment after reload, got %d", len(commitments))
	}
	if commitments[0].SwimmerID != swimmer.ID {
		t.Fatalf("expected commitment for swimmer %q, got %q", swimmer.ID, commitments[0].SwimmerID)
	}
}

// meetsFixtureQueries lists the DELETEs (FK-safe order) that clear the fixtures.
var meetsFixtureQueries = []string{
	"DELETE FROM entries WHERE org_id = $1::uuid",
	"DELETE FROM commitments WHERE org_id = $1::uuid",
	"DELETE FROM meet_events WHERE org_id = $1::uuid",
	"DELETE FROM meet_sessions WHERE org_id = $1::uuid",
	"DELETE FROM meets WHERE org_id = $1::uuid",
}

func cleanupMeetsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, meetsFixtureQueries)
}

// ─── Performance ────────────────────────────────────────────────────────────

func TestDBRepoPerformance_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	cleanupPerformanceFixtures(t, pool, testOrgID)

	// Performance service needs a swimmer and meet to exist in DB.
	// Use a stable test swimmer UUID.
	swimmerID := "20000000-0000-0000-0000-000000000001"
	meetID := "20000000-0000-0000-0000-000000000002"

	// Seed household/swimmer and meet rows for FK satisfaction.
	seedPerformancePrereqs(t, pool, testOrgID, swimmerID, meetID)

	svc := performance.NewServiceWithPool(pool)

	finTime, seedTime := 34000, 35000
	_, err := svc.UpsertResult(testOrgID, admin.UserID, admin, performance.UpsertResultInput{
		SwimmerID:   swimmerID,
		MeetID:      meetID,
		Distance:    50,
		Stroke:      "free",
		Course:      "SCY",
		Gender:      "F",
		FinTimeMs:   &finTime,
		IsOfficial:  true,
		EventNumber: "12",
		SeedTimeMs:  &seedTime,
		Splits: []performance.SplitInput{
			{SplitNumber: 1, SplitDistance: 25, SplitTimeMs: 16000, CumulativeTimeMs: 16000},
			{SplitNumber: 2, SplitDistance: 25, SplitTimeMs: 18000, CumulativeTimeMs: 34000},
		},
	})
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}

	svc2 := performance.NewServiceWithPool(pool)
	progression, err := svc2.ListProgression(testOrgID, admin, swimmerID, performance.ListResultsFilters{})
	if err != nil || len(progression) != 1 {
		t.Fatalf("ListProgression after reload = %d rows, err %v", len(progression), err)
	}
	if progression[0].EventNumber != "12" || progression[0].SeedTimeMs == nil || *progression[0].SeedTimeMs != seedTime {
		t.Fatalf("event number / seed time lost on reload: %+v", progression[0])
	}
	var splitCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM splits WHERE result_id = $1::uuid`, progression[0].ID).Scan(&splitCount); err != nil || splitCount != 2 {
		t.Fatalf("persisted splits = %d, err %v; want 2", splitCount, err)
	}
	bestTimes, err := svc2.ListBestTimes(testOrgID, admin, swimmerID, performance.ListBestTimesFilters{})
	if err != nil {
		t.Fatalf("ListBestTimes after reload: %v", err)
	}
	if len(bestTimes) == 0 {
		t.Fatal("expected best times after DB reload, got none")
	}
}

func seedPerformancePrereqs(t *testing.T, pool *pgxpool.Pool, orgID, swimmerID, meetID string) {
	t.Helper()
	// Household
	householdID := "20000000-0000-0000-0000-000000000003"
	seedRow(t, pool, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Perf Test', 'active') ON CONFLICT DO NOTHING`, householdID, orgID)
	// Swimmer
	seedRow(t, pool, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Perf', 'Swimmer', '2010-01-01', 'female', 'active') ON CONFLICT DO NOTHING`, swimmerID, orgID, householdID)
	// Meet
	seedRow(t, pool, `INSERT INTO meets (id, org_id, name, meet_type, course, status) VALUES ($1::uuid, $2::uuid, 'Perf Meet', 'invitational', 'SCY', 'draft') ON CONFLICT DO NOTHING`, meetID, orgID)
}

func cleanupPerformanceFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{
		"DELETE FROM best_times WHERE org_id = $1::uuid",
		"DELETE FROM results WHERE org_id = $1::uuid",
		"DELETE FROM time_standards WHERE org_id = $1::uuid",
	})
}

// ─── Billing ────────────────────────────────────────────────────────────────

func TestDBRepoBilling_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	cleanupBillingFixtures(t, pool, testOrgID)

	// Need a household in DB for FK.
	householdID := "30000000-0000-0000-0000-000000000001"
	ctx := context.Background()
	seedRow(t, pool, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Billing Test', 'active') ON CONFLICT DO NOTHING`, householdID, testOrgID)

	svc := billing.NewServiceWithPool(pool)
	inv, err := svc.CreateInvoice(testOrgID, admin.UserID, admin, billing.CreateInvoiceInput{
		HouseholdID:   householdID,
		InvoiceNumber: "INV-DBREPO-1",
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := svc.AddInvoiceItem(testOrgID, admin.UserID, admin, inv.ID, billing.AddInvoiceItemInput{
		Description: "Season fee", Quantity: 2, UnitAmountCents: 7500, Category: "program_fee",
	}); err != nil {
		t.Fatalf("AddInvoiceItem: %v", err)
	}
	if _, _, err := svc.IssueInvoice(testOrgID, admin.UserID, admin, inv.ID); err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}

	svc2 := billing.NewServiceWithPool(pool)
	invs := svc2.SnapshotInvoices(testOrgID)
	found := false
	for _, i := range invs {
		if i.ID == inv.ID {
			found = true
			if i.TotalCents != 15000 || i.Status != "issued" {
				t.Fatalf("reloaded invoice: want issued total 15000, got %s %d", i.Status, i.TotalCents)
			}
			break
		}
	}
	if !found {
		t.Fatalf("invoice %q not found after DB reload", inv.ID)
	}
	var itemTotal int
	if err := pool.QueryRow(ctx, `SELECT total_cents FROM invoice_items WHERE invoice_id = $1::uuid`, inv.ID).Scan(&itemTotal); err != nil || itemTotal != 15000 {
		t.Fatalf("invoice item total_cents: want 15000, got %d (err %v)", itemTotal, err)
	}
}

func cleanupBillingFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{
		"DELETE FROM ledger_events WHERE org_id = $1::uuid",
		"DELETE FROM refunds WHERE org_id = $1::uuid",
		"DELETE FROM payments WHERE org_id = $1::uuid",
		"DELETE FROM invoice_items WHERE org_id = $1::uuid",
		"DELETE FROM invoices WHERE org_id = $1::uuid",
		"DELETE FROM results WHERE org_id = $1::uuid",
		"DELETE FROM best_times WHERE org_id = $1::uuid",
		"DELETE FROM swimmer_groups WHERE org_id = $1::uuid",
		"DELETE FROM swimmers WHERE org_id = $1::uuid",
		"DELETE FROM households WHERE org_id = $1::uuid",
	})
}

// ─── Comms ──────────────────────────────────────────────────────────────────

func TestDBRepoComms_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupCommsFixtures(t, pool, testOrgID)

	svc := comms.NewServiceWithPool(pool)
	ann, err := svc.CreateAnnouncement(comms.CreateAnnouncementInput{
		OrgID:     testOrgID,
		Title:     "DB Repo Announcement",
		Body:      "Testing DB-backed comms",
		Audience:  "all",
		CreatedBy: "admin-user",
	})
	if err != nil {
		t.Fatalf("CreateAnnouncement: %v", err)
	}

	svc2 := comms.NewServiceWithPool(pool)
	anns, err := svc2.ListAnnouncements(testOrgID, "")
	if err != nil {
		t.Fatalf("ListAnnouncements after reload: %v", err)
	}
	found := false
	for _, a := range anns {
		if a.ID == ann.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("announcement %q not found after DB reload", ann.ID)
	}
}

func cleanupCommsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{
		"DELETE FROM email_deliveries WHERE org_id = $1::uuid",
		"DELETE FROM email_campaigns WHERE org_id = $1::uuid",
		"DELETE FROM announcements WHERE org_id = $1::uuid",
	})
}

// ─── Compliance ─────────────────────────────────────────────────────────────

func TestDBRepoCompliance_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupComplianceFixtures(t, pool, testOrgID)

	// Need a household.
	householdID := "40000000-0000-0000-0000-000000000001"
	ctx := context.Background()
	seedRow(t, pool, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Compliance Test', 'active') ON CONFLICT DO NOTHING`, householdID, testOrgID)

	svc := compliance.NewServiceWithPool(pool, 30, 14)
	req, err := svc.CreateDataExportRequest(testOrgID, householdID, "admin-user")
	if err != nil {
		t.Fatalf("CreateDataExportRequest: %v", err)
	}

	svc2 := compliance.NewServiceWithPool(pool, 30, 14)
	reqs, err := svc2.ListDataExportRequests(testOrgID, householdID)
	if err != nil {
		t.Fatalf("ListDataExportRequests after reload: %v", err)
	}
	found := false
	for _, r := range reqs {
		if r.ID == req.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("data export request %q not found after DB reload", req.ID)
	}

	// Cleanup household
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM households WHERE id = $1::uuid", householdID)
	})
}

func cleanupComplianceFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{
		"DELETE FROM data_deletion_requests WHERE org_id = $1",
		"DELETE FROM data_export_requests WHERE org_id = $1",
	})
}

// ─── Exports ────────────────────────────────────────────────────────────────

func TestDBRepoExports_RoundTrip(t *testing.T) {
	// Not parallel: DB repo tests share testOrgID fixtures and clean them up.
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := dbAdminPrincipal(t, pool)
	cleanupExportsFixtures(t, pool, testOrgID)

	// Need a meet in DB for FK.
	meetID := "50000000-0000-0000-0000-000000000001"
	ctx := context.Background()
	seedRow(t, pool, `INSERT INTO meets (id, org_id, name, meet_type, course, status) VALUES ($1::uuid, $2::uuid, 'Export Test Meet', 'invitational', 'SCY', 'published') ON CONFLICT DO NOTHING`, meetID, testOrgID)

	store := storage.NewStubStorage("")
	svc := exports.NewServiceWithPool(pool, store)
	expJob, _, err := svc.GenerateExport(admin, testOrgID, meetID, exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet: exports.ReportMeet{
			Name:      "Export Test Meet",
			StartDate: "2026-03-22",
			EndDate:   "2026-03-22",
			Course:    "SCY",
		},
	})
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}

	svc2 := exports.NewServiceWithPool(pool, store)
	exps, err := svc2.ListExports(admin, testOrgID, meetID)
	if err != nil {
		t.Fatalf("ListExports after reload: %v", err)
	}
	found := false
	for _, e := range exps {
		if e.ID == expJob.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("export %q not found after DB reload", expJob.ID)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM meets WHERE id = $1::uuid", meetID)
	})
}

func cleanupExportsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	deleteFixtures(t, pool, orgID, []string{"DELETE FROM file_exports WHERE org_id = $1::uuid"})
}
