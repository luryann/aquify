package app

import (
	"context"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

// TestDBRepoPerformanceRecordsGoalsLineups round-trips team record entries,
// swimmer goals and relay lineups through Postgres and a fresh service.
func TestDBRepoPerformanceRecordsGoalsLineups(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)
	admin := dbAdminPrincipal(t, pool)
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM team_record_entries WHERE org_id = $1",
		"DELETE FROM swimmer_goals WHERE org_id = $1",
		"DELETE FROM relay_lineups WHERE org_id = $1",
	} {
		if _, err := pool.Exec(ctx, q, testOrgID); err != nil {
			t.Fatalf("cleanup %s: %v", q, err)
		}
	}
	cleanupMembersFixtures(t, pool, testOrgID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM team_record_entries WHERE org_id = $1", testOrgID)
		_, _ = pool.Exec(ctx, "DELETE FROM relay_lineups WHERE org_id = $1", testOrgID)
		cleanupMembersFixtures(t, pool, testOrgID)
	})

	membersSvc := members.NewServiceWithPool(pool)
	household, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: testOrgID, HouseholdName: "Perf DB", PrimaryEmail: "perfdb@example.com"})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	var swimmerIDs []string
	for _, name := range []string{"Ann", "Bri", "Cat", "Dee"} {
		sw, _, err := membersSvc.CreateSwimmer(admin, testOrgID, members.CreateSwimmerInput{
			HouseholdID: household.ID, FirstName: name, LastName: "Db", DateOfBirth: "2013-05-05", Gender: "female", RegistrationStatus: "active",
		})
		if err != nil {
			t.Fatalf("CreateSwimmer: %v", err)
		}
		swimmerIDs = append(swimmerIDs, sw.ID)
	}
	meetsSvc := meets.NewServiceWithPool(pool)
	meet, _, err := meetsSvc.CreateMeet(admin, testOrgID, meets.CreateMeetInput{Name: "Perf DB Meet", MeetType: "invitational", Course: "SCY", StartDate: "2026-03-01"})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}

	perf := performance.NewServiceWithPool(pool)
	perf.SetDirectory(newPerformanceDirectory(membersSvc, meetsSvc))
	perf.SetHouseholdChecker(membersSvc)
	ms := 61000
	_, breaks, err := perf.UpsertResultWithRecords(testOrgID, admin.UserID, admin, performance.UpsertResultInput{
		SwimmerID: swimmerIDs[0], MeetID: meet.ID, Distance: 100, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true,
	})
	if err != nil || len(breaks) != 2 {
		t.Fatalf("upsert with records: %v %+v", err, breaks)
	}
	manual, err := perf.CreateManualRecord(testOrgID, admin.UserID, admin, performance.CreateManualRecordInput{
		Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 200, Stroke: "back", TimeMs: 121000, SwimmerName: "Legacy Lou", SwimDate: "1999-02-02", Locked: true, Verified: true,
	})
	if err != nil {
		t.Fatalf("manual record: %v", err)
	}
	goal, err := perf.CreateGoal(testOrgID, admin.UserID, admin, swimmerIDs[0], performance.GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 59000, Deadline: "2027-01-31", Notes: "Sub 59"})
	if err != nil {
		t.Fatalf("goal: %v", err)
	}
	var legs []performance.RelayLeg
	for i, id := range swimmerIDs {
		legs = append(legs, performance.RelayLeg{Leg: i + 1, SwimmerID: id, TimeMs: 28000 + i})
	}
	lineup, err := perf.SaveRelayLineup(testOrgID, admin.UserID, admin, "", performance.RelayLineupInput{
		MeetID: meet.ID, Course: "SCY", Gender: "F", AgeGroup: "13-14", RelayType: "free", Distance: 200, Teams: []performance.RelayTeam{{Legs: legs}},
	})
	if err != nil {
		t.Fatalf("lineup: %v", err)
	}

	// Fresh instance sees everything.
	perf2 := performance.NewServiceWithPool(pool)
	perf2.SetDirectory(newPerformanceDirectory(membersSvc, meetsSvc))
	perf2.SetHouseholdChecker(membersSvc)
	records, err := perf2.ListRecords(testOrgID, admin, performance.ListRecordsFilters{})
	if err != nil || len(records) != 3 {
		t.Fatalf("records after reload: %v %+v", err, records)
	}
	for _, r := range records {
		if r.Record.ID == manual.ID && (!r.Record.Locked || !r.Record.Verified || r.Record.SwimDate != "1999-02-02") {
			t.Fatalf("manual record did not round-trip: %+v", r.Record)
		}
		if r.Record.ResultID != "" && (r.Record.MeetName != "Perf DB Meet" || r.Record.SwimDate != "2026-03-01" || r.Record.SwimmerAge == nil) {
			t.Fatalf("result record did not round-trip: %+v", r.Record)
		}
	}
	goals, err := perf2.ListGoals(testOrgID, admin, swimmerIDs[0], false)
	if err != nil || len(goals) != 1 || goals[0].ID != goal.ID || goals[0].Deadline != "2027-01-31" || goals[0].BaselineTimeMs == nil {
		t.Fatalf("goals after reload: %v %+v", err, goals)
	}
	lineups, err := perf2.ListRelayLineups(testOrgID, admin, meet.ID)
	if err != nil || len(lineups) != 1 || lineups[0].ID != lineup.ID || len(lineups[0].Teams) != 1 || lineups[0].Teams[0].TotalMs != 28000*4+6 {
		t.Fatalf("lineups after reload: %v %+v", err, lineups)
	}
	if err := perf2.DeleteRelayLineup(testOrgID, admin.UserID, admin, lineup.ID); err != nil {
		t.Fatalf("delete lineup: %v", err)
	}
	if err := perf2.DeleteGoal(testOrgID, admin.UserID, admin, swimmerIDs[0], goal.ID); err != nil {
		t.Fatalf("delete goal: %v", err)
	}
	if _, err := perf2.RebuildRecords(testOrgID, admin.UserID, admin); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
}
