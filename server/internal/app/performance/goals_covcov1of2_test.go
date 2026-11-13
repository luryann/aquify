package performance

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestCov1xGoalsLoadFailureIsReturned(t *testing.T) {
	s := cov1xUnloaded(t)
	st := "active"
	tm := 30000
	calls := map[string]func() error{
		"ListGoals":    func() error { _, err := s.ListGoals("org", adminP, "amy", false); return err },
		"ListOrgGoals": func() error { _, err := s.ListOrgGoals("org", adminP, ""); return err },
		"CreateGoal": func() error {
			_, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{Distance: 50, Stroke: "free", Course: "SCY", TargetTimeMs: 30000})
			return err
		},
		"UpdateGoal": func() error {
			_, err := s.UpdateGoal("org", "admin", adminP, "amy", "g", UpdateGoalInput{Status: &st, TargetTimeMs: &tm})
			return err
		},
		"DeleteGoal": func() error { return s.DeleteGoal("org", "admin", adminP, "amy", "g") },
	}
	for name, call := range calls {
		err := call()
		if err == nil {
			t.Errorf("%s: expected the club load failure to be returned", name)
			continue
		}
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrGoalNotFound) {
			t.Errorf("%s: want a load error, got %v", name, err)
		}
	}
}

func TestCov1xGoalsDeniedWithoutReadRole(t *testing.T) {
	s := newRecordsFixture(t)
	nobody := &appmiddleware.Principal{UserID: "x", OrgID: "org", Roles: []string{"guest"}}
	if _, err := s.ListOrgGoals("org", nobody, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListOrgGoals(guest) = %v", err)
	}
	if _, err := s.ListOrgGoals("org", nil, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListOrgGoals(nil) = %v", err)
	}
	if _, err := s.ListGoals("org", nobody, "amy", false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListGoals(guest) = %v", err)
	}
	if _, err := s.CreateGoal("org", "x", nobody, "amy", GoalInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateGoal(guest) = %v", err)
	}
	if _, err := s.UpdateGoal("org", "x", nobody, "amy", "g", UpdateGoalInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpdateGoal(guest) = %v", err)
	}
	if err := s.DeleteGoal("org", "x", nobody, "amy", "g"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DeleteGoal(guest) = %v", err)
	}
}

func TestCov1xCreateGoalValidation(t *testing.T) {
	s := newRecordsFixture(t)
	good := GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000}
	mut := func(f func(*GoalInput)) GoalInput { g := good; f(&g); return g }
	cases := []struct {
		name string
		in   GoalInput
		want string
	}{
		{"relay stroke", mut(func(g *GoalInput) { g.Stroke = "free_relay" }), "stroke must be"},
		{"bad course", mut(func(g *GoalInput) { g.Course = "XYZ" }), "course must be"},
		{"zero distance", mut(func(g *GoalInput) { g.Distance = 0 }), "distance out of range"},
		{"huge distance", mut(func(g *GoalInput) { g.Distance = 1651 }), "distance out of range"},
		{"zero target", mut(func(g *GoalInput) { g.TargetTimeMs = 0 }), "target_time_ms out of range"},
		{"huge target", mut(func(g *GoalInput) { g.TargetTimeMs = 3_600_001 }), "target_time_ms out of range"},
		{"long notes", mut(func(g *GoalInput) { g.Notes = strings.Repeat("a", 501) }), "notes max 500"},
		{"bad deadline", mut(func(g *GoalInput) { g.Deadline = "tomorrow" }), "deadline must be"},
	}
	for _, tc := range cases {
		_, err := s.CreateGoal("org", "admin", adminP, "amy", tc.in)
		if !errors.Is(err, ErrGoalInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want ErrGoalInvalid containing %q", tc.name, err, tc.want)
		}
	}
	if _, err := s.CreateGoal("org", "admin", adminP, "ghost", good); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("unknown swimmer = %v, want ErrGoalNotFound", err)
	}
	if list, _ := s.ListGoals("org", adminP, "amy", true); len(list) != 0 {
		t.Fatalf("rejected goals must not be stored: %+v", list)
	}
}

func TestCov1xUpdateGoalValidationAndNotFound(t *testing.T) {
	s := newRecordsFixture(t)
	gp, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000, Notes: "orig"})
	if err != nil {
		t.Fatal(err)
	}
	zero, huge := 0, 3_600_001
	badDeadline := "soon"
	longNotes := strings.Repeat("n", 501)
	badStatus := "paused"
	for name, in := range map[string]UpdateGoalInput{
		"zero target": {TargetTimeMs: &zero},
		"huge target": {TargetTimeMs: &huge},
		"deadline":    {Deadline: &badDeadline},
		"notes":       {Notes: &longNotes},
		"status":      {Status: &badStatus},
	} {
		if _, err := s.UpdateGoal("org", "admin", adminP, "amy", gp.ID, in); !errors.Is(err, ErrGoalInvalid) {
			t.Errorf("%s: err = %v, want ErrGoalInvalid", name, err)
		}
	}
	if _, err := s.UpdateGoal("org", "admin", adminP, "amy", "missing", UpdateGoalInput{}); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("missing goal = %v", err)
	}
	if err := s.DeleteGoal("org", "admin", adminP, "amy", "missing"); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("delete missing goal = %v", err)
	}

	target, deadline, notes := 59000, "2030-01-31", "  new notes  "
	got, err := s.UpdateGoal("org", "admin", adminP, "amy", gp.ID, UpdateGoalInput{TargetTimeMs: &target, Deadline: &deadline, Notes: &notes})
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetTimeMs != 59000 || got.Deadline != "2030-01-31" || got.Notes != "new notes" || got.Status != "active" {
		t.Fatalf("update result = %+v", got.Goal)
	}
	if got.DaysRemaining == nil || got.Overdue {
		t.Fatalf("future deadline must report days remaining, not overdue: %+v", got)
	}
}

func TestCov1xGoalListingFiltersAndOrdering(t *testing.T) {
	s := newRecordsFixture(t)
	mk := func(org, swimmer string, dist int, stroke, course string) string {
		t.Helper()
		gp, err := s.CreateGoal(org, "admin", adminP, swimmer, GoalInput{Distance: dist, Stroke: stroke, Course: course, TargetTimeMs: 30000 + dist})
		if err != nil {
			t.Fatal(err)
		}
		return gp.ID
	}
	// Created out of order on purpose.
	mk("org", "bea", 50, "free", "SCY")
	mk("org", "amy", 100, "free", "LCM")
	mk("org", "amy", 100, "fly", "SCY")
	mk("org", "amy", 200, "free", "SCY")
	mk("org", "amy", 100, "free", "SCY")
	mk("org", "amy", 100, "free", "SCM")
	dup := mk("org", "amy", 100, "free", "SCY") // same event as above, created later
	mk("other-org", "amy", 50, "free", "SCY")
	archived := mk("org", "amy", 400, "im", "SCY")
	st := "archived"
	if _, err := s.UpdateGoal("org", "admin", adminP, "amy", archived, UpdateGoalInput{Status: &st}); err != nil {
		t.Fatal(err)
	}

	label := func(g GoalProgress) string {
		return g.SwimmerID + "/" + g.Course + "/" + g.Stroke + "/" + strconv.Itoa(g.Distance)
	}
	all, err := s.ListOrgGoals("org", adminP, "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range all {
		got = append(got, label(g))
	}
	want := []string{
		"amy/SCY/free/100", "amy/SCY/free/100", "amy/SCY/free/200", "amy/SCY/fly/100",
		"amy/SCM/free/100", "amy/LCM/free/100", "bea/SCY/free/50",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("org goals order\n got %v\nwant %v", got, want)
	}
	if all[1].ID != dup {
		t.Fatalf("equal events must sort by creation time; later goal %s should be second, got %s", dup, all[1].ID)
	}

	onlyBea, _ := s.ListOrgGoals("org", adminP, "bea")
	if len(onlyBea) != 1 || onlyBea[0].SwimmerID != "bea" {
		t.Fatalf("swimmer filter = %+v", onlyBea)
	}
	withArchived, _ := s.ListGoals("org", adminP, "amy", true)
	without, _ := s.ListGoals("org", adminP, "amy", false)
	if len(withArchived) != len(without)+1 {
		t.Fatalf("includeArchived: %d vs %d", len(withArchived), len(without))
	}
	// A parent only sees their own household's goals.
	fam, _ := s.ListOrgGoals("org", parentP, "")
	for _, g := range fam {
		if g.SwimmerID != "amy" {
			t.Fatalf("parent leaked goal for %s", g.SwimmerID)
		}
	}
	if len(fam) != 6 {
		t.Fatalf("parent should see amy's 6 active goals, got %d", len(fam))
	}
}

func TestCov1xGoalPersistenceFailuresLeaveMemoryUnchanged(t *testing.T) {
	s := newRecordsFixture(t)
	gp, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000})
	if err != nil {
		t.Fatal(err)
	}
	cov1xBreakPersistence(t, s, "org")

	if _, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{Distance: 200, Stroke: "free", Course: "SCY", TargetTimeMs: 120000}); err == nil || !strings.Contains(err.Error(), "persist goal") {
		t.Fatalf("CreateGoal = %v, want persist goal error", err)
	}
	target := 58000
	if _, err := s.UpdateGoal("org", "admin", adminP, "amy", gp.ID, UpdateGoalInput{TargetTimeMs: &target}); err == nil || !strings.Contains(err.Error(), "persist goal") {
		t.Fatalf("UpdateGoal = %v, want persist goal error", err)
	}
	if err := s.DeleteGoal("org", "admin", adminP, "amy", gp.ID); err == nil || !strings.Contains(err.Error(), "delete goal") {
		t.Fatalf("DeleteGoal = %v, want delete goal error", err)
	}
	if g := s.goals[gp.ID]; g == nil || g.TargetTimeMs != 60000 || len(s.goals) != 1 {
		t.Fatalf("failed writes must not change memory: %+v (n=%d)", g, len(s.goals))
	}
}
