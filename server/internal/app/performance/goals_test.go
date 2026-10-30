package performance

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestUpdateGoalReactivationRespectsActiveGoalCap(t *testing.T) {
	s := newRecordsFixture(t)
	var archivedID string
	for i := 0; i < maxGoalsPerSwimmer+1; i++ {
		gp, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{
			Distance: 50 + i, Stroke: "free", Course: "SCY", TargetTimeMs: 30000 + i,
		})
		if i == maxGoalsPerSwimmer {
			if !errors.Is(err, ErrGoalInvalid) {
				t.Fatalf("goal %d: want cap error, got %v", i, err)
			}
			break
		}
		if err != nil {
			t.Fatal(fmt.Errorf("goal %d: %w", i, err))
		}
		if i == 0 {
			archivedID = gp.ID
		}
	}
	archived := "archived"
	if _, err := s.UpdateGoal("org", "admin", adminP, "amy", archivedID, UpdateGoalInput{Status: &archived}); err != nil {
		t.Fatal(err)
	}
	// Archiving freed a slot; a new goal fills it, so reactivation must now fail.
	if _, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{Distance: 500, Stroke: "free", Course: "SCY", TargetTimeMs: 300000}); err != nil {
		t.Fatal(err)
	}
	active := "active"
	_, err := s.UpdateGoal("org", "admin", adminP, "amy", archivedID, UpdateGoalInput{Status: &active})
	if !errors.Is(err, ErrGoalInvalid) {
		t.Fatalf("reactivating past the cap = %v, want ErrGoalInvalid", err)
	}
}

func TestGoalNotesCountRunesNotBytes(t *testing.T) {
	s := newRecordsFixture(t)
	// 300 three-byte runes = 900 bytes but only 300 characters.
	notes := strings.Repeat("泳", 300)
	if _, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{
		Distance: 50, Stroke: "free", Course: "SCY", TargetTimeMs: 30000, Notes: notes,
	}); err != nil {
		t.Fatalf("300-character note rejected: %v", err)
	}
}

func TestNilPrincipalDenied(t *testing.T) {
	s := newRecordsFixture(t)
	if err := s.requireReadAccess(nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("requireReadAccess(nil) = %v", err)
	}
	if _, err := s.UpsertResult("org", "x", nil, UpsertResultInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpsertResult(nil principal) = %v", err)
	}
}

func TestCopyLineupIsDeep(t *testing.T) {
	orig := &RelayLineup{Teams: []RelayTeam{{Label: "A", Legs: []RelayLeg{{SwimmerID: "s1"}}}}}
	cp := copyLineup(orig)
	cp.Teams[0].Label = "X"
	cp.Teams[0].Legs[0].SwimmerID = "zzz"
	if orig.Teams[0].Label != "A" || orig.Teams[0].Legs[0].SwimmerID != "s1" {
		t.Fatal("copy aliases stored lineup")
	}
}
