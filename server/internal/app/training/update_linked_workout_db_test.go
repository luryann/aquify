package training

import (
	"context"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

func TestUpdateLinkedPracticeSurfacesWorkoutLookupFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()

	p, err := svc.Create(org, "2026-03-26", "GRP", "water", "Set", 1000, nil, PracticeStrokes{Free: 1000}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	res := ParseQuickEntry("10x100 free @1:20 EN1")
	linked, _, err := svc.SetPracticeWorkout(org, p.ID, WorkoutInput{Sections: res.Sections}, "coach")
	if err != nil {
		t.Fatal(err)
	}
	if linked.WorkoutID == "" {
		t.Fatal("practice should be linked to a workout")
	}

	// A transient failure of the workout lookup (simulated by renaming a column the query reads).
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE training_workouts RENAME COLUMN org_id TO org_id_offline`); err != nil {
		t.Fatal(err)
	}
	notes := "changed while the workout lookup is failing"
	if _, err := svc.Update(org, p.ID, PracticeUpdates{Notes: &notes}, "coach"); err == nil {
		t.Fatal("Update must return the workout lookup error instead of saving stale totals")
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE training_workouts RENAME COLUMN org_id_offline TO org_id`); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(org, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes == notes {
		t.Fatal("the failed update must not have been persisted")
	}
	if _, err := svc.Update(org, p.ID, PracticeUpdates{Notes: &notes}, "coach"); err != nil {
		t.Fatalf("update should succeed once the lookup works: %v", err)
	}
}
