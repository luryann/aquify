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
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openTestPool returns a pgxpool.Pool from DATABASE_URL, or skips the test.
func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping DB repo integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Skipf("cannot open test pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("cannot ping test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// testOrgID is a valid UUID for integration tests.
const testOrgID = "10000000-0000-0000-0000-000000000001"

// ─── Members ────────────────────────────────────────────────────────────────

func TestDBRepoMembers_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
	parent := testutil.ParentPrincipal("db-repo-parent", "")

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

	swimmers, err := svc2.ListSwimmers(parent, testOrgID, members.ListSwimmersInput{HouseholdID: household.ID})
	_ = err
	// parent doesn't have household association, use admin
	swimmers, err = svc2.ListSwimmers(admin, testOrgID, members.ListSwimmersInput{HouseholdID: household.ID})
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
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
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

func TestDBRepoMembers_DeleteSwimmerRejectsWhenDependenciesExist(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
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

func cleanupMembersFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	// Delete in FK-safe order.
	for _, q := range []string{
		"DELETE FROM swimmer_groups WHERE org_id = $1::uuid",
		"DELETE FROM swimmers WHERE org_id = $1::uuid",
		"DELETE FROM contacts WHERE org_id = $1::uuid",
		"DELETE FROM households WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Groups ─────────────────────────────────────────────────────────────────

func TestDBRepoGroups_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()

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
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
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
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM waiver_acceptances WHERE org_id = $1::uuid",
		"DELETE FROM registrations WHERE org_id = $1::uuid",
		"DELETE FROM eligibility_rules WHERE org_id = $1::uuid",
		"DELETE FROM groups WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Training ───────────────────────────────────────────────────────────────

func TestDBRepoTraining_CrossInstanceVisibilityWithoutRestart(t *testing.T) {
	t.Parallel()
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
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM practices WHERE org_id = $1::uuid`, orgID); err != nil {
		t.Logf("cleanup warning (DELETE FROM practices): %v", err)
	}
}

// ─── Meets ──────────────────────────────────────────────────────────────────

func TestDBRepoMeets_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
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

func cleanupMeetsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM entries WHERE org_id = $1::uuid",
		"DELETE FROM commitments WHERE org_id = $1::uuid",
		"DELETE FROM meet_events WHERE org_id = $1::uuid",
		"DELETE FROM meet_sessions WHERE org_id = $1::uuid",
		"DELETE FROM meets WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Performance ────────────────────────────────────────────────────────────

func TestDBRepoPerformance_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
	cleanupPerformanceFixtures(t, pool, testOrgID)

	// Performance service needs a swimmer and meet to exist in DB.
	// Use a stable test swimmer UUID.
	swimmerID := "20000000-0000-0000-0000-000000000001"
	meetID := "20000000-0000-0000-0000-000000000002"

	// Seed household/swimmer and meet rows for FK satisfaction.
	seedPerformancePrereqs(t, pool, testOrgID, swimmerID, meetID)

	svc := performance.NewServiceWithPool(pool)

	finTime := 34000
	_, err := svc.UpsertResult(testOrgID, admin.UserID, admin, performance.UpsertResultInput{
		SwimmerID:  swimmerID,
		MeetID:     meetID,
		Distance:   50,
		Stroke:     "free",
		Course:     "SCY",
		Gender:     "F",
		FinTimeMs:  &finTime,
		IsOfficial: true,
	})
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}

	svc2 := performance.NewServiceWithPool(pool)
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
	ctx := context.Background()
	// Household
	householdID := "20000000-0000-0000-0000-000000000003"
	pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Perf Test', 'active') ON CONFLICT DO NOTHING`, householdID, orgID)
	// Swimmer
	pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Perf', 'Swimmer', '2010-01-01', 'female', 'active') ON CONFLICT DO NOTHING`, swimmerID, orgID, householdID)
	// Meet
	pool.Exec(ctx, `INSERT INTO meets (id, org_id, name, meet_type, course, status) VALUES ($1::uuid, $2::uuid, 'Perf Meet', 'invitational', 'SCY', 'draft') ON CONFLICT DO NOTHING`, meetID, orgID)
}

func cleanupPerformanceFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM best_times WHERE org_id = $1::uuid",
		"DELETE FROM results WHERE org_id = $1::uuid",
		"DELETE FROM time_standards WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Billing ────────────────────────────────────────────────────────────────

func TestDBRepoBilling_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
	cleanupBillingFixtures(t, pool, testOrgID)

	// Need a household in DB for FK.
	householdID := "30000000-0000-0000-0000-000000000001"
	ctx := context.Background()
	pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Billing Test', 'active') ON CONFLICT DO NOTHING`, householdID, testOrgID)

	svc := billing.NewServiceWithPool(pool)
	inv, err := svc.CreateInvoice(testOrgID, admin.UserID, admin, billing.CreateInvoiceInput{
		HouseholdID:   householdID,
		InvoiceNumber: "INV-DBREPO-1",
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	svc2 := billing.NewServiceWithPool(pool)
	invs := svc2.SnapshotInvoices(testOrgID)
	found := false
	for _, i := range invs {
		if i.ID == inv.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("invoice %q not found after DB reload", inv.ID)
	}
}

func cleanupBillingFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM ledger_events WHERE org_id = $1::uuid",
		"DELETE FROM refunds WHERE org_id = $1::uuid",
		"DELETE FROM payments WHERE org_id = $1::uuid",
		"DELETE FROM invoice_items WHERE org_id = $1::uuid",
		"DELETE FROM invoices WHERE org_id = $1::uuid",
		"DELETE FROM households WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Comms ──────────────────────────────────────────────────────────────────

func TestDBRepoComms_RoundTrip(t *testing.T) {
	t.Parallel()
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
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM email_deliveries WHERE org_id = $1::uuid",
		"DELETE FROM email_campaigns WHERE org_id = $1::uuid",
		"DELETE FROM announcements WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Compliance ─────────────────────────────────────────────────────────────

func TestDBRepoCompliance_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupComplianceFixtures(t, pool, testOrgID)

	// Need a household.
	householdID := "40000000-0000-0000-0000-000000000001"
	ctx := context.Background()
	pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Compliance Test', 'active') ON CONFLICT DO NOTHING`, householdID, testOrgID)

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
		pool.Exec(ctx, "DELETE FROM households WHERE id = $1::uuid", householdID)
	})
}

func cleanupComplianceFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM data_deletion_requests WHERE org_id = $1::uuid",
		"DELETE FROM data_export_requests WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Imports ────────────────────────────────────────────────────────────────

func TestDBRepoImports_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
	cleanupImportsFixtures(t, pool, testOrgID)

	store := storage.NewStubStorage("")
	svc := imports.NewServiceWithPool(pool, store)

	imp, _, err := svc.RequestUploadURL(admin, testOrgID, imports.RequestUploadURLInput{
		Filename:  "test.sd3",
		FileType:  "sd3",
		SizeBytes: 1024,
		MIMEType:  "application/octet-stream",
	})
	if err != nil {
		t.Fatalf("RequestUploadURL: %v", err)
	}

	svc2 := imports.NewServiceWithPool(pool, store)
	imps, err := svc2.ListImports(admin, testOrgID)
	if err != nil {
		t.Fatalf("ListImports after reload: %v", err)
	}
	found := false
	for _, i := range imps {
		if i.ID == imp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("import %q not found after DB reload", imp.ID)
	}
}

func cleanupImportsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM match_conflicts WHERE org_id = $1::uuid",
		"DELETE FROM file_import_errors WHERE org_id = $1::uuid",
		"DELETE FROM file_import_rows WHERE org_id = $1::uuid",
		"DELETE FROM file_imports WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

// ─── Exports ────────────────────────────────────────────────────────────────

func TestDBRepoExports_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	admin := testutil.AdminPrincipal()
	cleanupExportsFixtures(t, pool, testOrgID)

	// Need a meet in DB for FK.
	meetID := "50000000-0000-0000-0000-000000000001"
	ctx := context.Background()
	pool.Exec(ctx, `INSERT INTO meets (id, org_id, name, meet_type, course, status) VALUES ($1::uuid, $2::uuid, 'Export Test Meet', 'invitational', 'SCY', 'published') ON CONFLICT DO NOTHING`, meetID, testOrgID)

	store := storage.NewStubStorage("")
	svc := exports.NewServiceWithPool(pool, store)
	expJob, _, err := svc.GenerateExport(admin, testOrgID, meetID, exports.GenerateExportInput{
		ExportType:    exports.ExportTypeStandardSD3,
		MeetName:      "Export Test Meet",
		MeetStartDate: "03222026",
		MeetEndDate:   "03222026",
		Course:        "SCY",
		HostTeamCode:  "DARE",
		HostTeamName:  "DARE Aquatics",
		Entries:       []exports.ExportEntry{},
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
		pool.Exec(ctx, "DELETE FROM meets WHERE id = $1::uuid", meetID)
	})
}

func cleanupExportsFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "DELETE FROM file_exports WHERE org_id = $1::uuid", orgID)
}

// Placeholder to avoid import errors when DATABASE_URL is absent.
var _ = time.Now
