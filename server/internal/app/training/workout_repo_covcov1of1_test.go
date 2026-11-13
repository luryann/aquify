package training

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1xUpsertWorkoutStoresNilTagsAsEmptyArray(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	now := time.Now().UTC()
	w := &Workout{ID: uuid.NewString(), OrgID: org, Title: "No tags", Course: CourseSCY, Tags: nil,
		Sections: []WorkoutSection{}, CreatedAt: now, UpdatedAt: now, CreatedBy: "c", UpdatedBy: "c"}
	if err := upsertWorkout(context.Background(), svc.repo.at(org), w); err != nil {
		t.Fatalf("upsert with nil tags: %v", err)
	}
	got, err := svc.GetWorkout(org, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tags == nil || len(got.Tags) != 0 || got.Title != "No tags" {
		t.Fatalf("stored workout = %+v, want an empty (non-null) tag list", got)
	}
}
