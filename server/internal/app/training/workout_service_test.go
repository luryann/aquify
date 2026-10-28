package training

import (
	"errors"
	"strings"
	"testing"
)

func mainSetInput(title string, course string) WorkoutInput {
	return WorkoutInput{
		Title:      title,
		Course:     course,
		IsTemplate: true,
		Tags:       []string{"Threshold", "threshold", " Senior  "},
		Sections: []WorkoutSection{
			{Kind: "warmup", Sets: []WorkoutSet{{Repeats: 1, Distance: 400, Stroke: "choice", Zone: "rec"}}},
			{Kind: "main", Sets: []WorkoutSet{{
				Repeats: 3, RestSeconds: 60, Zone: "EN2",
				Sets: []WorkoutSet{
					{Repeats: 4, Distance: 100, Stroke: "free", IntervalSeconds: 90},
					{Repeats: 1, Distance: 100, Stroke: "back", Zone: "REC"},
				},
			}}},
			{Kind: "kick", Sets: []WorkoutSet{{Repeats: 6, Distance: 50, IntervalSeconds: 60, Equipment: []string{"fins", "FINS"}}}},
			{Kind: "cooldown", Sets: []WorkoutSet{{Distance: 200}}},
		},
	}
}

func TestComputeTotalsNestedCourseAware(t *testing.T) {
	t.Parallel()
	in, err := NormalizeWorkoutInput(mainSetInput("Threshold Tuesday", "scm"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if in.Course != CourseSCM || strings.Join(in.Tags, ",") != "senior,threshold" {
		t.Fatalf("normalized course/tags = %s %v", in.Course, in.Tags)
	}
	if got := in.Sections[2].Sets[0].Equipment; len(got) != 1 || got[0] != "fins" {
		t.Fatalf("equipment dedupe = %v", got)
	}
	totals := ComputeTotals(in.Sections, in.Course)
	// 400 + 3*(400+100) + 300 + 200
	if totals.Distance != 2400 || totals.Unit != "m" || totals.Lengths != 96 {
		t.Fatalf("totals = %+v", totals)
	}
	if totals.DistanceYards != 2625 {
		t.Fatalf("distance yards = %d, want 2625", totals.DistanceYards)
	}
	if totals.ByStroke["free"] != 1400 || totals.ByStroke["back"] != 300 || totals.ByStroke["kick"] != 300 || totals.ByStroke["choice"] != 400 {
		t.Fatalf("by stroke = %+v (cooldown defaults to free, kick section defaults to kick)", totals.ByStroke)
	}
	if totals.ByZone["EN2"] != 1200 || totals.ByZone["REC"] != 700 || totals.ByZone[ZoneUnassigned] != 500 {
		t.Fatalf("by zone = %+v", totals.ByZone)
	}
	// warmup 400 choice @95/100 = 380; main 3*(4*90 + 1*100 + 60) = 1560;
	// kick 6*60 = 360; cooldown 200 free @90/100 = 180.
	if totals.EstimatedSeconds != 380+1560+360+180 {
		t.Fatalf("estimated seconds = %d", totals.EstimatedSeconds)
	}
	if len(totals.BySection) != 4 || totals.BySection[1].Distance != 1500 {
		t.Fatalf("by section = %+v", totals.BySection)
	}
	if len(totals.Warnings) != 0 {
		t.Fatalf("warnings = %v", totals.Warnings)
	}

	lcm := ComputeTotals([]WorkoutSection{{Kind: "main", Sets: []WorkoutSet{{Repeats: 4, Distance: 25, IntervalSeconds: 5}}}}, CourseLCM)
	if lcm.Lengths != 2 || len(lcm.Warnings) != 2 {
		t.Fatalf("LCM 25s should warn about half laps and a too-fast send-off: %+v", lcm)
	}
}

func TestNormalizeWorkoutRejectsBadInput(t *testing.T) {
	t.Parallel()
	base := func() WorkoutInput { return mainSetInput("ok", "SCY") }
	cases := map[string]func(*WorkoutInput){
		"title":         func(in *WorkoutInput) { in.Title = "  " },
		"course":        func(in *WorkoutInput) { in.Course = "SCX" },
		"section kind":  func(in *WorkoutInput) { in.Sections[0].Kind = "spin" },
		"stroke":        func(in *WorkoutInput) { in.Sections[0].Sets[0].Stroke = "doggy" },
		"zone":          func(in *WorkoutInput) { in.Sections[0].Sets[0].Zone = "EN9" },
		"equipment":     func(in *WorkoutInput) { in.Sections[0].Sets[0].Equipment = []string{"jetpack"} },
		"repeats":       func(in *WorkoutInput) { in.Sections[0].Sets[0].Repeats = 500 },
		"distance":      func(in *WorkoutInput) { in.Sections[0].Sets[0].Distance = 0 },
		"round dist":    func(in *WorkoutInput) { in.Sections[1].Sets[0].Distance = 100 },
		"round sendoff": func(in *WorkoutInput) { in.Sections[1].Sets[0].IntervalSeconds = 60 },
		"interval":      func(in *WorkoutInput) { in.Sections[0].Sets[0].IntervalSeconds = -1 },
		"too long":      func(in *WorkoutInput) { in.Sections[0].Sets[0] = WorkoutSet{Repeats: 50, Distance: 1000} },
		"nesting": func(in *WorkoutInput) {
			in.Sections[0].Sets = []WorkoutSet{{Repeats: 2, Sets: []WorkoutSet{{Repeats: 2, Sets: []WorkoutSet{{Repeats: 2, Sets: []WorkoutSet{{Distance: 50}}}}}}}}
		},
	}
	for name, mutate := range cases {
		in := base()
		mutate(&in)
		_, err := NormalizeWorkoutInput(in)
		var verr *WorkoutValidationError
		if !errors.Is(err, ErrInvalidWorkout) || !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want validation error", name, err)
		}
	}
}

func TestWorkoutLibrarySearchDuplicateAndAssign(t *testing.T) {
	t.Parallel()
	svc := NewService()
	tmpl, err := svc.CreateWorkout("org-a", mainSetInput("Threshold Tuesday", "SCY"), "coach")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	other := mainSetInput("Sprint Friday", "LCM")
	other.Tags = []string{"sprint"}
	if _, err := svc.CreateWorkout("org-a", other, "coach"); err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := svc.CreateWorkout("org-b", mainSetInput("Threshold elsewhere", "SCY"), "coach"); err != nil {
		t.Fatalf("create org-b: %v", err)
	}

	got, _ := svc.ListWorkouts("org-a", WorkoutFilter{TemplatesOnly: true, Query: "THRESH"})
	if len(got) != 1 || got[0].ID != tmpl.ID {
		t.Fatalf("search = %+v", got)
	}
	got, _ = svc.ListWorkouts("org-a", WorkoutFilter{Tag: "sprint"})
	if len(got) != 1 || got[0].Title != "Sprint Friday" {
		t.Fatalf("tag filter = %+v", got)
	}
	if _, err := svc.GetWorkout("org-b", tmpl.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}

	dup, err := svc.DuplicateWorkout("org-a", tmpl.ID, DuplicateInput{}, "coach-2")
	if err != nil || dup.Title != "Threshold Tuesday (copy)" || !dup.IsTemplate || dup.SourceWorkoutID != tmpl.ID || dup.Totals.Distance != tmpl.Totals.Distance {
		t.Fatalf("duplicate = %+v err=%v", dup, err)
	}

	// Assign to a new practice: instance copy, derived yardage.
	practice, inst, err := svc.AssignWorkout("org-a", tmpl.ID, AssignInput{Date: "2026-11-03", GroupID: "grp-senior"}, "coach")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if inst.IsTemplate || inst.ID == tmpl.ID || inst.SourceWorkoutID != tmpl.ID {
		t.Fatalf("instance = %+v", inst)
	}
	if practice.WorkoutID != inst.ID || practice.YardageTarget != 2400 || practice.Strokes.Free != 1400 || practice.Type != "water" || practice.Description != "Threshold Tuesday" {
		t.Fatalf("practice = %+v", practice)
	}

	// Editing the instance re-derives the practice; the template is untouched.
	edit := mainSetInput("Threshold Tuesday v2", "SCY")
	edit.Sections = edit.Sections[:1]
	edit.IsTemplate = true // ignored: is_template is fixed at creation
	updated, err := svc.UpdateWorkout("org-a", inst.ID, edit, "coach")
	if err != nil || updated.IsTemplate || updated.Totals.Distance != 400 {
		t.Fatalf("update instance = %+v err=%v", updated, err)
	}
	p2, _ := svc.Get("org-a", practice.ID)
	if p2.YardageTarget != 400 {
		t.Fatalf("practice yardage after workout edit = %d, want 400", p2.YardageTarget)
	}
	// A manual yardage patch cannot drift from the linked workout.
	manual := 9999
	p3, err := svc.Update("org-a", practice.ID, PracticeUpdates{YardageTarget: &manual}, "coach")
	if err != nil || p3.YardageTarget != 400 {
		t.Fatalf("manual yardage override = %+v err=%v", p3, err)
	}
	if t2, _ := svc.GetWorkout("org-a", tmpl.ID); t2.Totals.Distance != 2400 {
		t.Fatalf("template changed: %d", t2.Totals.Distance)
	}

	// Re-assigning to the same practice replaces (and deletes) the old instance.
	_, inst2, err := svc.AssignWorkout("org-a", dup.ID, AssignInput{PracticeID: practice.ID}, "coach")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if _, err := svc.GetWorkout("org-a", inst.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("old instance should be deleted, err = %v", err)
	}
	if _, w, err := svc.GetPracticeWorkout("org-a", practice.ID); err != nil || w.ID != inst2.ID {
		t.Fatalf("practice workout = %+v err=%v", w, err)
	}

	// Deleting the practice removes its instance but never the template.
	if err := svc.Delete("org-a", practice.ID); err != nil {
		t.Fatalf("delete practice: %v", err)
	}
	if _, err := svc.GetWorkout("org-a", inst2.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("instance should go with practice, err = %v", err)
	}
	if _, err := svc.GetWorkout("org-a", dup.ID); err != nil {
		t.Fatalf("template must survive: %v", err)
	}

	if _, _, err := svc.AssignWorkout("org-a", tmpl.ID, AssignInput{}, "coach"); !errors.Is(err, ErrInvalidAssignment) {
		t.Fatalf("assign without target err = %v", err)
	}
	if _, _, err := svc.AssignWorkout("org-a", tmpl.ID, AssignInput{Date: "11/03/2026"}, "coach"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("assign bad date err = %v", err)
	}
}

func TestSetAndClearPracticeWorkout(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p, _ := svc.Create("org-a", "2026-11-04", "grp", "water", "Aerobic", 3000, nil, PracticeStrokes{}, "", "coach")
	if _, _, err := svc.GetPracticeWorkout("org-a", p.ID); !errors.Is(err, ErrPracticeNoWorkout) {
		t.Fatalf("no workout err = %v", err)
	}
	res := ParseQuickEntry("10x100 free @1:20 EN1\n200 easy")
	p1, w1, err := svc.SetPracticeWorkout("org-a", p.ID, WorkoutInput{Sections: res.Sections}, "coach")
	if err != nil || w1.Title != "Aerobic" || p1.YardageTarget != 1200 {
		t.Fatalf("set = %+v %+v err=%v", p1, w1, err)
	}
	p2, w2, err := svc.SetPracticeWorkout("org-a", p.ID, WorkoutInput{Title: "Aerobic", Sections: []WorkoutSection{{Kind: "main", Sets: res.Sections[0].Sets[:1]}}}, "coach")
	if err != nil || w2.ID != w1.ID || p2.YardageTarget != 1000 {
		t.Fatalf("replace keeps the same instance: %+v %+v err=%v", p2, w2, err)
	}
	cleared, err := svc.ClearPracticeWorkout("org-a", p.ID, "coach")
	if err != nil || cleared.WorkoutID != "" || cleared.YardageTarget != 1000 {
		t.Fatalf("clear = %+v err=%v", cleared, err)
	}
	if _, err := svc.ClearPracticeWorkout("org-a", p.ID, "coach"); !errors.Is(err, ErrPracticeNoWorkout) {
		t.Fatalf("second clear err = %v", err)
	}
	if _, _, err := svc.SetPracticeWorkout("org-b", p.ID, WorkoutInput{Sections: res.Sections}, "coach"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org set err = %v", err)
	}
}

func TestPlanPhasesValidationOverlapAndWeeklyPlan(t *testing.T) {
	t.Parallel()
	svc := NewService()
	season := SeasonWindow{ID: "season-1", StartDate: "2026-09-07", EndDate: "2026-12-20"}
	base, err := svc.CreatePhase("org-a", season, PhaseInput{GroupID: "grp", Phase: "Base", StartDate: "2026-09-07", EndDate: "2026-10-04", WeeklyYardageTarget: 20000}, "coach")
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if base.Phase != "base" {
		t.Fatalf("phase not normalised: %s", base.Phase)
	}
	if _, err := svc.CreatePhase("org-a", season, PhaseInput{GroupID: "grp", Phase: "build", StartDate: "2026-10-01", EndDate: "2026-10-31"}, "coach"); !errors.Is(err, ErrPhaseOverlap) {
		t.Fatalf("overlap err = %v", err)
	}
	// Same dates for a different group are fine.
	if _, err := svc.CreatePhase("org-a", season, PhaseInput{GroupID: "grp-2", Phase: "build", StartDate: "2026-10-01", EndDate: "2026-10-31"}, "coach"); err != nil {
		t.Fatalf("other group: %v", err)
	}
	for name, in := range map[string]PhaseInput{
		"kind":          {GroupID: "grp", Phase: "sprint", StartDate: "2026-10-05", EndDate: "2026-10-10"},
		"group":         {Phase: "build", StartDate: "2026-10-05", EndDate: "2026-10-10"},
		"before season": {GroupID: "grp", Phase: "build", StartDate: "2026-09-01", EndDate: "2026-09-05"},
		"after season":  {GroupID: "grp", Phase: "build", StartDate: "2026-12-15", EndDate: "2026-12-31"},
		"reversed":      {GroupID: "grp", Phase: "build", StartDate: "2026-10-10", EndDate: "2026-10-05"},
		"target":        {GroupID: "grp", Phase: "build", StartDate: "2026-10-05", EndDate: "2026-10-10", WeeklyYardageTarget: -5},
	} {
		if _, err := svc.CreatePhase("org-a", season, in, "coach"); !errors.Is(err, ErrInvalidWorkout) {
			t.Errorf("%s: err = %v, want validation error", name, err)
		}
	}
	build, err := svc.CreatePhase("org-a", season, PhaseInput{GroupID: "grp", Phase: "build", StartDate: "2026-10-05", EndDate: "2026-11-01", WeeklyYardageTarget: 25000}, "coach")
	if err != nil {
		t.Fatalf("create build: %v", err)
	}
	// Updating a phase onto itself is not an overlap.
	if _, err := svc.UpdatePhase("org-a", build.ID, season, PhaseInput{GroupID: "grp", Phase: "build", StartDate: "2026-10-05", EndDate: "2026-11-08", WeeklyYardageTarget: 26000}, "coach"); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Practices: group referenced by code on one, by ID on another.
	_, _ = svc.Create("org-a", "2026-10-06", "GRP-CODE", "water", "", 5000, nil, PracticeStrokes{}, "", "coach")
	_, _ = svc.Create("org-a", "2026-10-08", "grp", "water", "", 6000, nil, PracticeStrokes{}, "", "coach")
	canon := func(v string) string {
		if v == "GRP-CODE" {
			return "grp"
		}
		return v
	}
	plan, err := svc.SeasonPlan("org-a", season, "grp", canon)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Phases) != 2 || len(plan.Weeks) != 15 {
		t.Fatalf("plan phases=%d weeks=%d", len(plan.Phases), len(plan.Weeks))
	}
	var oct5 *PlanWeek
	for i := range plan.Weeks {
		if plan.Weeks[i].WeekStart == "2026-10-05" {
			oct5 = &plan.Weeks[i]
		}
	}
	if oct5 == nil || oct5.Phase != "build" || oct5.TargetYards != 26000 || oct5.PlannedYards != 11000 || oct5.Practices != 2 {
		t.Fatalf("week of Oct 5 = %+v", oct5)
	}
	if plan.Weeks[0].Phase != "base" || plan.Weeks[0].TargetYards != 20000 || plan.Weeks[0].PlannedYards != 0 {
		t.Fatalf("first week = %+v", plan.Weeks[0])
	}
	if err := svc.DeletePhase("org-b", build.ID); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("cross-org delete err = %v", err)
	}
	if err := svc.DeletePhase("org-a", build.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestGroupAndSwimmerAnalytics(t *testing.T) {
	t.Parallel()
	svc := NewService()
	tmpl, _ := svc.CreateWorkout("org-a", mainSetInput("Threshold", "SCY"), "coach")
	p1, _, _ := svc.AssignWorkout("org-a", tmpl.ID, AssignInput{Date: "2026-10-06", GroupID: "grp"}, "coach") // 2400 yd
	p2, _ := svc.Create("org-a", "2026-10-08", "grp", "water", "", 3000, nil, PracticeStrokes{}, "", "coach")
	p3, _ := svc.Create("org-a", "2026-10-14", "other", "water", "", 4000, nil, PracticeStrokes{}, "", "coach")
	_, _ = svc.Create("org-a", "2026-12-01", "grp", "water", "", 9999, nil, PracticeStrokes{}, "", "coach") // out of range
	mark := func(p *Practice, swimmer, status string) {
		if _, err := svc.SetAttendance("org-a", p.ID, swimmer, status, "coach"); err != nil {
			t.Fatalf("attendance: %v", err)
		}
	}
	mark(p1, "s1", "present")
	mark(p1, "s2", "present")
	mark(p1, "s3", "absent")
	mark(p2, "s1", "excused")
	mark(p2, "s2", "present")
	mark(p3, "s1", "present") // guest swim outside s1's group

	a, err := svc.GroupAnalytics("org-a", "2026-10-05", "2026-10-18", "", nil)
	if err != nil {
		t.Fatalf("group analytics: %v", err)
	}
	if a.TotalPractices != 3 || a.StructuredPractices != 1 || len(a.Groups) != 2 || len(a.Weeks) != 4 {
		t.Fatalf("analytics = %+v", a)
	}
	g := a.Groups[0]
	if g.GroupID != "grp" || g.PlannedYards != 5400 || g.SwimmerYards != 2400*2+3000 || g.Present != 3 || g.Absent != 1 || g.Excused != 1 {
		t.Fatalf("grp summary = %+v", g)
	}
	if g.AttendancePct == nil || *g.AttendancePct != 75 {
		t.Fatalf("grp attendance pct = %v", g.AttendancePct)
	}
	if a.ZoneYards["EN2"] != 1200 || a.StrokeYards["free"] != 1400 {
		t.Fatalf("zones = %+v strokes = %+v", a.ZoneYards, a.StrokeYards)
	}
	if a.Weeks[1].WeekStart != "2026-10-12" || a.Weeks[1].Practices != 0 || a.Weeks[1].AttendancePct != nil {
		t.Fatalf("dense empty week = %+v", a.Weeks[1])
	}

	only, _ := svc.GroupAnalytics("org-a", "2026-10-05", "2026-10-18", "other", nil)
	if len(only.Groups) != 1 || only.Groups[0].PlannedYards != 4000 {
		t.Fatalf("filtered = %+v", only.Groups)
	}

	sa, err := svc.SwimmerAnalytics("org-a", "s1", map[string]bool{"grp": true}, "2026-10-05", "2026-10-18", nil)
	if err != nil {
		t.Fatalf("swimmer analytics: %v", err)
	}
	if sa.Scheduled != 3 || sa.Present != 2 || sa.Excused != 1 || sa.Yards != 2400+4000 {
		t.Fatalf("swimmer = %+v", sa)
	}
	if sa.AttendancePct == nil || *sa.AttendancePct != 100 || sa.ZoneYards["EN2"] != 1200 {
		t.Fatalf("swimmer pct/zones = %v %+v", sa.AttendancePct, sa.ZoneYards)
	}
	if sa.Weeks[0].Yards != 2400 || sa.Weeks[1].Yards != 4000 {
		t.Fatalf("swimmer weeks = %+v", sa.Weeks)
	}

	for _, r := range [][2]string{{"2026-10-18", "2026-10-05"}, {"bad", "2026-10-05"}, {"2025-01-01", "2026-10-05"}} {
		if _, err := svc.GroupAnalytics("org-a", r[0], r[1], "", nil); !errors.Is(err, ErrInvalidRange) {
			t.Errorf("range %v err = %v", r, err)
		}
	}
}
