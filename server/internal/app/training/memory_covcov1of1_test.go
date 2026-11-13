package training

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func cov1xMkPractice(t *testing.T, svc *Service, org, date, group, typ string, yards int) *Practice {
	t.Helper()
	p, err := svc.Create(org, date, group, typ, "desc "+date, yards, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatalf("create practice: %v", err)
	}
	return p
}

func cov1xSimpleWorkout(title string) WorkoutInput {
	return WorkoutInput{Title: title, Sections: []WorkoutSection{{Kind: "main", Sets: []WorkoutSet{{Repeats: 4, Distance: 100, Stroke: "free"}}}}}
}

func TestCov1xNewServiceWithNilPoolIsInMemory(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil)
	if svc.repo != nil {
		t.Fatal("nil pool must yield an in-memory service")
	}
	if _, err := svc.Create("org-a", "2026-11-02", "g", "water", "", 100, nil, PracticeStrokes{}, "", "c"); err != nil {
		t.Fatal(err)
	}
}

func TestCov1xListScopesToOrgOrdersTiesAndNeverReturnsNil(t *testing.T) {
	t.Parallel()
	svc := NewService()
	empty, err := svc.List("org-empty")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v err=%v, want non-nil empty", empty, err)
	}
	first := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 100)
	second := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 200)
	cov1xMkPractice(t, svc, "org-b", "2026-11-02", "g", "water", 300)
	// Make the creation order unambiguous regardless of clock resolution.
	svc.practices[first.ID].CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.practices[second.ID].CreatedAt = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	got, err := svc.List("org-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("list = %+v, want only org-a practices, newest-created first on a date tie", got)
	}
}

func TestCov1xAttendanceForSwimmerInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p1 := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 100)
	p2 := cov1xMkPractice(t, svc, "org-a", "2026-11-03", "g", "water", 100)
	other := cov1xMkPractice(t, svc, "org-b", "2026-11-02", "g", "water", 100)
	for _, call := range []struct{ org, practice, swimmer, status string }{
		{"org-a", p1.ID, "sw1", "present"},
		{"org-a", p1.ID, "sw2", "absent"},
		{"org-b", other.ID, "sw1", "excused"},
	} {
		if _, err := svc.SetAttendance(call.org, call.practice, call.swimmer, call.status, "coach"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.AttendanceForSwimmer("org-a", "sw1", []string{p1.ID, p2.ID, other.ID, "missing"})
	if err != nil {
		t.Fatal(err)
	}
	// p2 has no mark, the foreign-org practice and the unknown id are ignored.
	if want := map[string]string{p1.ID: "present"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("attendance = %v, want %v", got, want)
	}
}

func TestCov1xDeleteWorkoutInMemoryUnlinksPractices(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 0)
	linked, w, err := svc.SetPracticeWorkout("org-a", p.ID, cov1xSimpleWorkout("Set"), "coach")
	if err != nil {
		t.Fatal(err)
	}
	if linked.YardageTarget != 400 {
		t.Fatalf("yardage = %d", linked.YardageTarget)
	}
	if err := svc.DeleteWorkout("org-b", w.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("cross-org delete err = %v", err)
	}
	if err := svc.DeleteWorkout("org-a", w.ID); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get("org-a", p.ID)
	if err != nil || got.WorkoutID != "" || got.YardageTarget != 400 {
		t.Fatalf("practice after workout delete = %+v err=%v, want unlinked but yardage kept", got, err)
	}
	if err := svc.DeleteWorkout("org-a", w.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestCov1xListWorkoutsOrderingAndTagSearch(t *testing.T) {
	t.Parallel()
	svc := NewService()
	in := cov1xSimpleWorkout("Alpha")
	in.Tags = []string{"endurance"}
	a, _ := svc.CreateWorkout("org-a", in, "c")
	b, _ := svc.CreateWorkout("org-a", cov1xSimpleWorkout("Bravo"), "c")
	c, _ := svc.CreateWorkout("org-a", cov1xSimpleWorkout("Charlie"), "c")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.workouts[a.ID].UpdatedAt = base.Add(time.Hour) // newest
	svc.workouts[b.ID].UpdatedAt = base
	svc.workouts[c.ID].UpdatedAt = base // ties with b: ordered by ID

	got, _ := svc.ListWorkouts("org-a", WorkoutFilter{})
	if len(got) != 3 || got[0].ID != a.ID {
		t.Fatalf("order = %v", cov1xIDs(got))
	}
	lo, hi := b.ID, c.ID
	if lo > hi {
		lo, hi = hi, lo
	}
	if got[1].ID != lo || got[2].ID != hi {
		t.Fatalf("tie order = %v, want %s then %s", cov1xIDs(got), lo, hi)
	}
	// "endur" matches neither title nor description; only the tag.
	got, _ = svc.ListWorkouts("org-a", WorkoutFilter{Query: "endur"})
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("tag-substring search = %v", cov1xIDs(got))
	}
}

func cov1xIDs(ws []*Workout) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.ID
	}
	return out
}

func TestCov1xDuplicateWorkoutVariantsAndErrors(t *testing.T) {
	t.Parallel()
	svc := NewService()
	src, err := svc.CreateWorkout("org-a", cov1xSimpleWorkout("Base"), "c")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DuplicateWorkout("org-a", "missing", DuplicateInput{}, "c"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("missing source err = %v", err)
	}
	name := "Custom name"
	yes := true
	dup, err := svc.DuplicateWorkout("org-a", src.ID, DuplicateInput{Title: &name, IsTemplate: &yes}, "c2")
	if err != nil || dup.Title != name || !dup.IsTemplate || src.IsTemplate {
		t.Fatalf("dup = %+v err=%v (source template=%v)", dup, err, src.IsTemplate)
	}
	// A blank override falls back to the "(copy)" title.
	blank := "   "
	dup, err = svc.DuplicateWorkout("org-a", src.ID, DuplicateInput{Title: &blank}, "c2")
	if err != nil || dup.Title != "Base (copy)" {
		t.Fatalf("blank-title dup = %+v err=%v", dup, err)
	}
	// An over-long multi-byte title is cut on a rune boundary to the limit.
	long := strings.Repeat("é", 100)
	dup, err = svc.DuplicateWorkout("org-a", src.ID, DuplicateInput{Title: &long}, "c2")
	if err != nil || len(dup.Title) > MaxTitleLength || !utf8.ValidString(dup.Title) || !strings.HasPrefix(long, dup.Title) {
		t.Fatalf("long-title dup = %q err=%v", dup.Title, err)
	}
	// A stored workout that no longer validates cannot be duplicated.
	svc.workouts[src.ID].Sections[0].Sets[0].Distance = 0
	if _, err := svc.DuplicateWorkout("org-a", src.ID, DuplicateInput{}, "c"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("corrupt source err = %v", err)
	}
}

func TestCov1xUpdateWorkoutErrors(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, err := svc.UpdateWorkout("org-a", "missing", cov1xSimpleWorkout("x"), "c"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	w, _ := svc.CreateWorkout("org-a", cov1xSimpleWorkout("x"), "c")
	if _, err := svc.UpdateWorkout("org-a", w.ID, WorkoutInput{Title: " "}, "c"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("invalid err = %v", err)
	}
}

func TestCov1xAssignWorkoutErrors(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w, _ := svc.CreateWorkout("org-a", cov1xSimpleWorkout("x"), "c")
	if _, _, err := svc.AssignWorkout("org-a", "missing", AssignInput{Date: "2026-11-02"}, "c"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("missing workout err = %v", err)
	}
	if _, _, err := svc.AssignWorkout("org-a", w.ID, AssignInput{Date: "2026-11-02", Type: "swim"}, "c"); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("bad type err = %v", err)
	}
	if _, _, err := svc.AssignWorkout("org-a", w.ID, AssignInput{PracticeID: "no-such-practice"}, "c"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown practice err = %v", err)
	}
	other := cov1xMkPractice(t, svc, "org-b", "2026-11-02", "g", "water", 0)
	if _, _, err := svc.AssignWorkout("org-a", w.ID, AssignInput{PracticeID: other.ID}, "c"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-org practice err = %v", err)
	}
	// A dryland assignment keeps the requested type.
	p, _, err := svc.AssignWorkout("org-a", w.ID, AssignInput{Date: "2026-11-02", Type: "dryland", GroupID: " G1 "}, "c")
	if err != nil || p.Type != "dryland" || p.GroupID != "G1" {
		t.Fatalf("dryland assign = %+v err=%v", p, err)
	}
}

func TestCov1xSetPracticeWorkoutVariants(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, _, err := svc.SetPracticeWorkout("org-a", "missing", cov1xSimpleWorkout("x"), "c"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing practice err = %v", err)
	}
	p, err := svc.Create("org-a", "2026-11-02", "g", "water", "", 0, nil, PracticeStrokes{}, "", "c")
	if err != nil {
		t.Fatal(err)
	}
	in := cov1xSimpleWorkout("")
	// No title and no practice description: titled after the date.
	_, w, err := svc.SetPracticeWorkout("org-a", p.ID, in, "c")
	if err != nil || w.Title != "Practice 2026-11-02" {
		t.Fatalf("fallback title = %+v err=%v", w, err)
	}
	// Replacing an existing instance with invalid content surfaces the error.
	bad := cov1xSimpleWorkout("ok")
	bad.Sections[0].Sets[0].Distance = 0
	if _, _, err := svc.SetPracticeWorkout("org-a", p.ID, bad, "c"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("invalid replace err = %v", err)
	}
	// A fresh practice rejects invalid content before linking anything.
	p2 := cov1xMkPractice(t, svc, "org-a", "2026-11-03", "g", "water", 0)
	if _, _, err := svc.SetPracticeWorkout("org-a", p2.ID, bad, "c"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("invalid create err = %v", err)
	}
	if got, _ := svc.Get("org-a", p2.ID); got.WorkoutID != "" {
		t.Fatalf("practice linked despite invalid workout: %+v", got)
	}
}

func TestCov1xPracticeWorkoutLookupsInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, err := svc.ClearPracticeWorkout("org-a", "missing", "c"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("clear missing err = %v", err)
	}
	if _, _, err := svc.GetPracticeWorkout("org-a", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing err = %v", err)
	}
	p := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 0)
	_, w, err := svc.SetPracticeWorkout("org-a", p.ID, cov1xSimpleWorkout("x"), "c")
	if err != nil {
		t.Fatal(err)
	}
	// The workout row vanished behind the practice's back: reported as "no workout".
	delete(svc.workouts, w.ID)
	got, gw, err := svc.GetPracticeWorkout("org-a", p.ID)
	if !errors.Is(err, ErrPracticeNoWorkout) || gw != nil || got == nil || got.ID != p.ID {
		t.Fatalf("dangling workout = %+v %+v err=%v", got, gw, err)
	}
}

func TestCov1xWorkoutsForPracticesAndAttendanceInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 0)
	plain := cov1xMkPractice(t, svc, "org-a", "2026-11-03", "g", "water", 0)
	linked, w, _ := svc.SetPracticeWorkout("org-a", p.ID, cov1xSimpleWorkout("x"), "c")
	got, err := svc.WorkoutsForPractices("org-a", []*Practice{linked, plain})
	if err != nil || len(got) != 1 || got[w.ID] == nil || got[w.ID].Title != "x" {
		t.Fatalf("workouts = %+v err=%v", got, err)
	}
	if got, _ := svc.WorkoutsForPractices("org-b", []*Practice{linked}); len(got) != 0 {
		t.Fatalf("cross-org workouts leaked: %+v", got)
	}
	if _, err := svc.SetAttendance("org-a", p.ID, "sw", "present", "c"); err != nil {
		t.Fatal(err)
	}
	recs, err := svc.attendanceForPractices("org-a", []string{p.ID, plain.ID})
	if err != nil || len(recs) != 1 || recs[0].SwimmerID != "sw" {
		t.Fatalf("attendance = %+v err=%v", recs, err)
	}
	if recs, _ := svc.attendanceForPractices("org-b", []string{p.ID}); len(recs) != 0 {
		t.Fatalf("cross-org attendance leaked: %+v", recs)
	}
}

func TestCov1xPhaseLifecycleErrorsInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	season := SeasonWindow{ID: "s1", StartDate: "2026-01-01", EndDate: "2026-06-30"}
	mk := func(group, start, end string) *PlanPhase {
		t.Helper()
		p, err := svc.CreatePhase("org-a", season, PhaseInput{GroupID: group, Phase: "base", StartDate: start, EndDate: end}, "c")
		if err != nil {
			t.Fatalf("create phase: %v", err)
		}
		return p
	}
	a := mk("g", "2026-01-05", "2026-01-11")
	b := mk("g", "2026-01-12", "2026-01-18")

	if _, err := svc.GetPhase("org-b", a.ID); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	if _, err := svc.UpdatePhase("org-a", "missing", season, PhaseInput{}, "c"); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("update missing err = %v", err)
	}
	if _, err := svc.UpdatePhase("org-a", b.ID, season, PhaseInput{GroupID: "g", Phase: "bogus", StartDate: "2026-01-12", EndDate: "2026-01-18"}, "c"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("update invalid err = %v", err)
	}
	if _, err := svc.UpdatePhase("org-a", b.ID, season, PhaseInput{GroupID: "g", Phase: "build", StartDate: "2026-01-08", EndDate: "2026-01-18"}, "c"); !errors.Is(err, ErrPhaseOverlap) {
		t.Fatalf("update overlap err = %v", err)
	}
	// The failed update left the phase untouched.
	if got, _ := svc.GetPhase("org-a", b.ID); got.StartDate != "2026-01-12" || got.Phase != "base" {
		t.Fatalf("phase mutated by failed update: %+v", got)
	}
	if err := svc.DeletePhase("org-a", "missing"); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("delete missing err = %v", err)
	}
	if err := svc.DeletePhase("org-b", a.ID); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("cross-org delete err = %v", err)
	}
}

func TestCov1xListPhasesTieBreaksOnID(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.phases["bbb"] = &PlanPhase{ID: "bbb", OrgID: "org-a", SeasonID: "s1", GroupID: "g", StartDate: "2026-01-05", EndDate: "2026-01-06"}
	svc.phases["aaa"] = &PlanPhase{ID: "aaa", OrgID: "org-a", SeasonID: "s1", GroupID: "g", StartDate: "2026-01-05", EndDate: "2026-01-07"}
	svc.phases["ccc"] = &PlanPhase{ID: "ccc", OrgID: "org-a", SeasonID: "s1", GroupID: "a", StartDate: "2026-02-01", EndDate: "2026-02-02"}
	got, err := svc.ListPhases("org-a", "s1")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	if want := []string{"ccc", "aaa", "bbb"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
}

func TestCov1xSeasonPlanSpansPhasesAcrossGroups(t *testing.T) {
	t.Parallel()
	svc := NewService()
	open := SeasonWindow{ID: "s1"} // no bounds, so phases may sit anywhere
	for _, in := range []PhaseInput{
		{GroupID: "g1", Phase: "base", StartDate: "2026-01-05", EndDate: "2026-01-18", WeeklyYardageTarget: 10000},
		{GroupID: "g2", Phase: "build", StartDate: "2026-01-12", EndDate: "2026-01-25", WeeklyYardageTarget: 5000},
	} {
		if _, err := svc.CreatePhase("org-a", open, in, "c"); err != nil {
			t.Fatal(err)
		}
	}
	cov1xMkPractice(t, svc, "org-a", "2026-01-13", "g1", "water", 6000)

	// The narrower season window is widened to cover both phases; nil canon is identity.
	plan, err := svc.SeasonPlan("org-a", SeasonWindow{ID: "s1", StartDate: "2026-01-12", EndDate: "2026-01-18"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.StartDate != "2026-01-05" || plan.EndDate != "2026-01-25" || len(plan.Weeks) != 6 {
		t.Fatalf("plan range %s..%s weeks=%d", plan.StartDate, plan.EndDate, len(plan.Weeks))
	}
	byKey := map[string]PlanWeek{}
	for _, w := range plan.Weeks {
		byKey[w.GroupID+"@"+w.WeekStart] = w
	}
	if w := byKey["g1@2026-01-12"]; w.TargetYards != 10000 || w.PlannedYards != 6000 || w.Practices != 1 || w.Phase != "base" {
		t.Fatalf("g1 week 2 = %+v", w)
	}
	if w := byKey["g1@2026-01-19"]; w.PhaseID != "" || w.TargetYards != 0 {
		t.Fatalf("g1 has no phase in week 3: %+v", w)
	}
	if w := byKey["g2@2026-01-19"]; w.Phase != "build" || w.TargetYards != 5000 {
		t.Fatalf("g2 week 3 = %+v", w)
	}

	// Filtering to one group keeps only its phases and rows.
	only, err := svc.SeasonPlan("org-a", open, "g2", func(v string) string { return v })
	if err != nil {
		t.Fatal(err)
	}
	if len(only.Phases) != 1 || only.Phases[0].GroupID != "g2" || only.StartDate != "2026-01-12" {
		t.Fatalf("filtered plan = %+v", only)
	}
	for _, w := range only.Weeks {
		if w.GroupID != "g2" {
			t.Fatalf("row for %s in g2-only plan", w.GroupID)
		}
	}
}

func TestCov1xSeasonPlanWithoutUsableRange(t *testing.T) {
	t.Parallel()
	svc := NewService()
	for name, season := range map[string]SeasonWindow{
		"no dates":     {ID: "s1"},
		"inverted":     {ID: "s1", StartDate: "2026-02-01", EndDate: "2026-01-01"},
		"only a start": {ID: "s1", StartDate: "2026-02-01"},
	} {
		plan, err := svc.SeasonPlan("org-a", season, "", nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(plan.Weeks) != 0 || plan.Weeks == nil || len(plan.Phases) != 0 {
			t.Fatalf("%s: plan = %+v, want empty non-nil weeks", name, plan)
		}
	}
}

func TestCov1xPracticesInRangeOrdersSameDateByID(t *testing.T) {
	t.Parallel()
	svc := NewService()
	a := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 1)
	b := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "g", "water", 2)
	cov1xMkPractice(t, svc, "org-a", "2026-12-25", "g", "water", 3) // outside the range
	got, _, err := svc.practicesInRange("org-a", "2026-11-01", "2026-11-30")
	if err != nil || len(got) != 2 {
		t.Fatalf("range = %+v err=%v", got, err)
	}
	lo, hi := a.ID, b.ID
	if lo > hi {
		lo, hi = hi, lo
	}
	if got[0].ID != lo || got[1].ID != hi {
		t.Fatalf("ids = %s,%s want %s,%s", got[0].ID, got[1].ID, lo, hi)
	}
}

func cov1xAnalyticsFixture(t *testing.T) *Service {
	t.Helper()
	svc := NewService()
	g1 := cov1xMkPractice(t, svc, "org-a", "2026-11-02", "G1", "water", 3000)
	cov1xMkPractice(t, svc, "org-a", "2026-11-02", "G1", "rest", 9000) // rest days never count
	g2 := cov1xMkPractice(t, svc, "org-a", "2026-11-03", "G2", "water", 2000)
	cov1xMkPractice(t, svc, "org-a", "2026-11-03", "G2", "water", 500) // unmarked, other group
	team := cov1xMkPractice(t, svc, "org-a", "2026-11-04", "", "water", 1000)
	for _, m := range []struct{ practice, swimmer, status string }{
		{g1.ID, "sw1", "present"}, {g1.ID, "sw2", "present"}, {g1.ID, "sw3", "absent"},
		{g2.ID, "sw1", "absent"}, {team.ID, "sw1", "excused"},
	} {
		if _, err := svc.SetAttendance("org-a", m.practice, m.swimmer, m.status, "coach"); err != nil {
			t.Fatal(err)
		}
	}
	return svc
}

func TestCov1xGroupAnalyticsSkipsRestAndRejectsBadRange(t *testing.T) {
	t.Parallel()
	svc := cov1xAnalyticsFixture(t)
	if _, err := svc.GroupAnalytics("org-a", "2026-11-22", "2026-11-02", "", nil); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("bad range err = %v", err)
	}
	out, err := svc.GroupAnalytics("org-a", "2026-11-02", "2026-11-22", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.TotalPractices != 4 || out.StructuredPractices != 0 {
		t.Fatalf("totals = %d/%d, want 4 non-rest practices", out.TotalPractices, out.StructuredPractices)
	}
	var ids []string
	sum := map[string]GroupSummary{}
	for _, g := range out.Groups {
		ids = append(ids, g.GroupID)
		sum[g.GroupID] = g
	}
	if want := []string{"", "G1", "G2"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("groups = %v, want %v", ids, want)
	}
	g1 := sum["G1"]
	if g1.Practices != 1 || g1.PlannedYards != 3000 || g1.SwimmerYards != 6000 || g1.Present != 2 || g1.Absent != 1 {
		t.Fatalf("G1 = %+v", g1)
	}
	if g1.AttendancePct == nil || *g1.AttendancePct != 66.7 {
		t.Fatalf("G1 pct = %v", g1.AttendancePct)
	}
	if sum[""].AttendancePct != nil {
		t.Fatalf("team-wide only has an excused mark, pct = %v", *sum[""].AttendancePct)
	}
	// Dense weeks: three ISO weeks for each of the three groups.
	if len(out.Weeks) != 9 {
		t.Fatalf("weeks = %d, want 9", len(out.Weeks))
	}
}

func TestCov1xSwimmerAnalyticsScheduledScopeAndEmptyWeeks(t *testing.T) {
	t.Parallel()
	svc := cov1xAnalyticsFixture(t)
	if _, err := svc.SwimmerAnalytics("org-a", "sw1", nil, "bad", "2026-11-22", nil); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("bad range err = %v", err)
	}
	out, err := svc.SwimmerAnalytics("org-a", "sw1", map[string]bool{"G1": true}, "2026-11-02", "2026-11-22", func(v string) string { return v })
	if err != nil {
		t.Fatal(err)
	}
	// Scheduled: the G1 practice, the G2 practice sw1 was marked at, and the
	// team-wide practice. The rest day and the unmarked G2 practice are excluded.
	if out.Scheduled != 3 || out.Present != 1 || out.Absent != 1 || out.Excused != 1 || out.Yards != 3000 {
		t.Fatalf("analytics = %+v", out)
	}
	if out.AttendancePct == nil || *out.AttendancePct != 50 {
		t.Fatalf("pct = %v", out.AttendancePct)
	}
	if len(out.Weeks) != 3 || out.Weeks[0].Scheduled != 3 || out.Weeks[1].Scheduled != 0 || out.Weeks[2].AttendancePct != nil {
		t.Fatalf("weeks = %+v", out.Weeks)
	}
}
