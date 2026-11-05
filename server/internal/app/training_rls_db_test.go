package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/app/training"
	"github.com/luryann/aquify/server/internal/testutil"
)

// practices, attendance and workouts are isolated by the database.
func TestTrainingRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := training.NewServiceWithPool(appPool)
	mem := members.NewServiceWithPool(appPool)
	staff := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	}
	seasonsSvc := seasons.NewServiceWithPool(appPool)
	type club struct{ practice, workout, swimmer, season, phase string }
	mk := func(org string) club {
		p := staff(org)
		hh, err := mem.CreateHousehold(p, members.CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := mem.CreateSwimmer(p, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "S", LastName: "W", DateOfBirth: "2012-01-01", Gender: "female"})
		if err != nil {
			t.Fatal(err)
		}
		pr, err := svc.Create(org, "2026-11-03", "", "water", "d", 0, nil, training.PracticeStrokes{}, "", p.UserID)
		if err != nil {
			t.Fatal(err)
		}
		w, err := svc.CreateWorkout(org, training.WorkoutInput{Title: "W", Course: "SCY", Sections: []training.WorkoutSection{
			{Kind: "main", Sets: []training.WorkoutSet{{Repeats: 1, Distance: 200, Stroke: "free", Zone: "EN1"}}}}}, p.UserID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetAttendance(org, pr.ID, sw.ID, "present", p.UserID); err != nil {
			t.Fatal(err)
		}
		se, err := seasonsSvc.Create(org, "2026-27", "2026-09-01", "2027-08-31", "active", "", "")
		if err != nil {
			t.Fatal(err)
		}
		ph, err := svc.CreatePhase(org, training.SeasonWindow{ID: se.ID, StartDate: se.StartDate, EndDate: se.EndDate},
			training.PhaseInput{GroupID: "GOLD", Phase: "base", Name: "Base", StartDate: "2026-09-01", EndDate: "2026-10-31", WeeklyYardageTarget: 20000}, "")
		if err != nil {
			t.Fatal(err)
		}
		return club{pr.ID, w.ID, sw.ID, se.ID, ph.ID}
	}
	a, b := mk(orgA), mk(orgB)

	if got, err := svc.List(orgA); err != nil || len(got) != 1 || got[0].ID != a.practice {
		t.Fatalf("org A lists only its practice: %+v %v", got, err)
	}
	if _, err := svc.Get(orgA, b.practice); !errors.Is(err, training.ErrNotFound) {
		t.Fatalf("org A must not read org B's practice: %v", err)
	}
	if _, err := svc.GetWorkout(orgA, b.workout); err == nil {
		t.Fatal("org A must not read org B's workout")
	}
	if err := svc.Delete(orgA, b.practice); !errors.Is(err, training.ErrNotFound) {
		t.Fatalf("org A must not delete org B's practice: %v", err)
	}
	if got, err := svc.ListAttendance(orgA, b.practice); err == nil && len(got) != 0 {
		t.Fatalf("org A must not see org B's attendance: %+v", got)
	}
	if got, err := svc.ListPhases(orgA, a.season); err != nil || len(got) != 1 || got[0].ID != a.phase {
		t.Fatalf("org A lists only its plan phase: %+v %v", got, err)
	}
	if got, err := svc.ListPhases(orgA, b.season); err != nil || len(got) != 0 {
		t.Fatalf("org A must not see org B's plan phases: %+v %v", got, err)
	}
	if _, err := svc.GetPhase(orgA, b.phase); err == nil {
		t.Fatal("org A must not read org B's plan phase")
	}
	for _, table := range []string{"practices", "practice_attendance", "training_workouts", "training_plan_phases"} {
		var n int
		if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
