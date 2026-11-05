package training

import (
	"errors"
	"testing"
)

func TestMetricWorkoutAssignStoresYards(t *testing.T) {
	svc := NewService()
	p, err := svc.Create("org-a", "2026-10-06", "grp", "water", "", 0, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	in := WorkoutInput{Title: "LCM", Course: CourseLCM, Sections: []WorkoutSection{{Kind: "main", Sets: []WorkoutSet{{Repeats: 10, Distance: 100, Stroke: "free"}}}}}
	linked, w, err := svc.SetPracticeWorkout("org-a", p.ID, in, "coach")
	if err != nil {
		t.Fatal(err)
	}
	if want := ToYards(1000, CourseLCM); linked.YardageTarget != want || want == 1000 || w.Totals.DistanceYards != want {
		t.Fatalf("yardage_target = %d, want %d (not raw metres)", linked.YardageTarget, want)
	}
	if linked.Strokes.Free != ToYards(1000, CourseLCM) {
		t.Fatalf("free = %d, want yards", linked.Strokes.Free)
	}
}

func TestPracticeNumbersAreBounded(t *testing.T) {
	svc := NewService()
	if _, err := svc.Create("org-a", "2026-10-06", "grp", "water", "", -5000, nil, PracticeStrokes{}, "", "coach"); !errors.Is(err, ErrInvalidNumeric) {
		t.Fatalf("negative yardage err = %v", err)
	}
	if _, err := svc.Create("org-a", "2026-10-06", "grp", "water", "", 1000, nil, PracticeStrokes{Free: -1}, "", "coach"); !errors.Is(err, ErrInvalidNumeric) {
		t.Fatalf("negative stroke err = %v", err)
	}
	p, err := svc.Create("org-a", "2026-10-06", "grp", "water", "", 1000, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	big := 3_000_000_000
	if _, err := svc.Update("org-a", p.ID, PracticeUpdates{YardageTarget: &big}, "coach"); !errors.Is(err, ErrInvalidNumeric) {
		t.Fatalf("huge yardage err = %v", err)
	}
	neg := -1
	if _, err := svc.Update("org-a", p.ID, PracticeUpdates{Attendance: &neg}, "coach"); !errors.Is(err, ErrInvalidNumeric) {
		t.Fatalf("negative attendance err = %v", err)
	}
}

func TestSeasonPlanSkipsRestPractices(t *testing.T) {
	svc := NewService()
	season := SeasonWindow{ID: "s1", StartDate: "2026-10-05", EndDate: "2026-10-11"}
	_, _ = svc.Create("org-a", "2026-10-06", "grp", "water", "", 5000, nil, PracticeStrokes{}, "", "coach")
	_, _ = svc.Create("org-a", "2026-10-07", "grp", "rest", "", 3000, nil, PracticeStrokes{}, "", "coach")
	plan, err := svc.SeasonPlan("org-a", season, "grp", func(v string) string { return v })
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Weeks) != 1 || plan.Weeks[0].Practices != 1 || plan.Weeks[0].PlannedYards != 5000 {
		t.Fatalf("weeks = %+v", plan.Weeks)
	}
}
