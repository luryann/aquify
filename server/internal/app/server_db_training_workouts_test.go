package app

import (
	"context"
	"errors"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/seasons"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/google/uuid"
)

// TestDBRepoTrainingWorkouts_RoundTrip exercises the workout library,
// practice links and periodization phases against Postgres, across two
// service instances sharing one database.
func TestDBRepoTrainingWorkouts_RoundTrip(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	orgID := "org-dbtrain-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM practices WHERE org_id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM training_workouts WHERE org_id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM training_plan_phases WHERE org_id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM seasons WHERE org_id = $1`, orgID)
	})

	a := training.NewServiceWithPool(pool)
	b := training.NewServiceWithPool(pool)

	parsed := training.ParseQuickEntry("Warmup: 400 choice\nMain\n3 x {4x100 free @1:30 EN2, 100 easy} rest 1:00")
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse errors: %+v", parsed.Errors)
	}
	tmpl, err := a.CreateWorkout(orgID, training.WorkoutInput{Title: "Threshold %_", Course: "SCM", IsTemplate: true, Tags: []string{"EN2", "senior"}, Sections: parsed.Sections}, "coach")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := b.GetWorkout(orgID, tmpl.ID)
	if err != nil || got.Totals.Distance != 1900 || got.Course != "SCM" || len(got.Sections[1].Sets[0].Sets) != 2 {
		t.Fatalf("cross-instance get = %+v err=%v", got, err)
	}
	if _, err := b.GetWorkout("other-org", tmpl.ID); !errors.Is(err, training.ErrWorkoutNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	for _, f := range []training.WorkoutFilter{{TemplatesOnly: true, Query: "%_"}, {Tag: "en2"}, {Query: "SENIOR"}} {
		list, err := b.ListWorkouts(orgID, f)
		if err != nil || len(list) != 1 {
			t.Fatalf("list %+v = %d err=%v", f, len(list), err)
		}
	}
	if list, _ := b.ListWorkouts(orgID, training.WorkoutFilter{Query: "nomatch"}); len(list) != 0 {
		t.Fatalf("nomatch list = %d", len(list))
	}

	practice, inst, err := a.AssignWorkout(orgID, tmpl.ID, training.AssignInput{Date: "2026-11-02", GroupID: "grp"}, "coach")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	p, err := b.Get(orgID, practice.ID)
	if err != nil || p.WorkoutID != inst.ID || p.YardageTarget != 1900 || p.Strokes.Free != 1500 {
		t.Fatalf("practice via b = %+v err=%v", p, err)
	}
	if _, err := b.UpdateWorkout(orgID, inst.ID, training.WorkoutInput{Title: "Short", Course: "SCM", Sections: parsed.Sections[:1]}, "coach"); err != nil {
		t.Fatalf("update instance: %v", err)
	}
	if p, _ := a.Get(orgID, practice.ID); p.YardageTarget != 400 {
		t.Fatalf("re-derived yardage = %d", p.YardageTarget)
	}
	schedule, err := a.WorkoutsForPractices(orgID, []*training.Practice{p})
	if err != nil || schedule[inst.ID] == nil {
		t.Fatalf("workouts for practices = %+v err=%v", schedule, err)
	}

	an, err := b.GroupAnalytics(orgID, "2026-11-02", "2026-11-08", "", nil)
	if err != nil || an.TotalPractices != 1 || an.Groups[0].PlannedYards != 437 {
		t.Fatalf("analytics = %+v err=%v", an, err)
	}

	if _, err := a.ClearPracticeWorkout(orgID, practice.ID, "coach"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := b.GetWorkout(orgID, inst.ID); !errors.Is(err, training.ErrWorkoutNotFound) {
		t.Fatalf("instance should be deleted with clear, err = %v", err)
	}
	_, inst2, err := a.AssignWorkout(orgID, tmpl.ID, training.AssignInput{PracticeID: practice.ID}, "coach")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if err := b.Delete(orgID, practice.ID); err != nil {
		t.Fatalf("delete practice: %v", err)
	}
	if _, err := a.GetWorkout(orgID, inst2.ID); !errors.Is(err, training.ErrWorkoutNotFound) {
		t.Fatalf("instance should be deleted with practice, err = %v", err)
	}
	if _, err := a.GetWorkout(orgID, tmpl.ID); err != nil {
		t.Fatalf("template must survive: %v", err)
	}

	// Periodization phases.
	seasonSvc := seasons.NewServiceWithPool(pool)
	season, err := seasonSvc.Create(orgID, "Fall", "2026-09-07", "2026-12-20", "active", "", "admin")
	if err != nil {
		t.Fatalf("season: %v", err)
	}
	win := training.SeasonWindow{ID: season.ID, StartDate: season.StartDate, EndDate: season.EndDate}
	phase, err := a.CreatePhase(orgID, win, training.PhaseInput{GroupID: "grp", Phase: "base", StartDate: "2026-09-07", EndDate: "2026-10-04", WeeklyYardageTarget: 20000}, "coach")
	if err != nil {
		t.Fatalf("create phase: %v", err)
	}
	if _, err := b.CreatePhase(orgID, win, training.PhaseInput{GroupID: "grp", Phase: "build", StartDate: "2026-10-01", EndDate: "2026-10-31"}, "coach"); !errors.Is(err, training.ErrPhaseOverlap) {
		t.Fatalf("overlap via second instance err = %v", err)
	}
	if _, err := b.UpdatePhase(orgID, phase.ID, win, training.PhaseInput{GroupID: "grp", Phase: "base", StartDate: "2026-09-07", EndDate: "2026-10-11", WeeklyYardageTarget: 21000}, "coach"); err != nil {
		t.Fatalf("update phase: %v", err)
	}
	plan, err := a.SeasonPlan(orgID, win, "grp", nil)
	if err != nil || len(plan.Phases) != 1 || plan.Weeks[0].TargetYards != 21000 {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
	if err := a.DeletePhase(orgID, phase.ID); err != nil {
		t.Fatalf("delete phase: %v", err)
	}
	if err := b.DeletePhase(orgID, phase.ID); !errors.Is(err, training.ErrPhaseNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}
