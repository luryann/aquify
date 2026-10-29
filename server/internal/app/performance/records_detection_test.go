package performance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type slotSnapshot []string

// recordSet is the comparable shape of every result-sourced record entry.
func recordSet(s *Service) slotSnapshot {
	var out slotSnapshot
	for _, e := range s.recordEntries {
		if e.Source != RecordSourceResult {
			continue
		}
		out = append(out, fmt.Sprintf("%s|%s|%s|%d|%s", e.key().Label(), e.ResultID, e.SwimDate, e.TimeMs, e.Status))
	}
	sort.Strings(out)
	return out
}

// Live detection and rebuild share evaluateSlot, so replaying the same
// back-dated imports through both must leave identical record sets.
func TestLiveAndRebuildRecordDetectionAgreeOnBackDatedImports(t *testing.T) {
	type swimStep struct {
		swimmer, meet string
		ms            int
	}
	scenarios := map[string][]swimStep{
		"back-dated faster than later record": {
			{"amy", "m-2026", 62000}, {"amy", "m-2025", 61000},
		},
		"back-dated slower than the current record": {
			{"amy", "m-2026", 60000}, {"amy", "m-2025", 61000},
		},
		"back-dated between two records": {
			{"amy", "m-2025", 63000}, {"bea", "m-2026", 60000}, {"amy", "m-2024", 62000},
		},
		"same meet slower swim after faster": {
			{"amy", "m-2026", 60000}, {"bea", "m-2026", 61000}, {"amy", "m-2025", 59000},
		},
		"ties and out-of-order": {
			{"bea", "m-2026", 60000}, {"amy", "m-2026", 60000}, {"amy", "m-2024", 60000}, {"bea", "m-2025", 59500},
		},
	}
	for name, steps := range scenarios {
		t.Run(name, func(t *testing.T) {
			s := newRecordsFixture(t)
			s.directory.(*fakeDirectory).meets = append(s.directory.(*fakeDirectory).meets,
				MeetInfo{ID: "m-2024", Name: "Old Meet", StartDate: "2024-10-04", Course: "SCY"})
			for _, st := range steps {
				swim(t, s, st.swimmer, st.meet, st.ms)
			}
			live := recordSet(s)
			if len(live) == 0 {
				t.Fatal("scenario produced no records")
			}
			if _, err := s.RebuildRecords("org", "admin", adminP); err != nil {
				t.Fatal(err)
			}
			rebuilt := recordSet(s)
			if fmt.Sprint(live) != fmt.Sprint(rebuilt) {
				t.Fatalf("live and rebuild disagree\nlive:    %v\nrebuilt: %v", live, rebuilt)
			}
		})
	}
}

func TestBackDatedSlowerSwimStillRecordedAgainstItsOwnDate(t *testing.T) {
	s := newRecordsFixture(t)
	swim(t, s, "amy", "m-2026", 60000)
	// 61.00 in 2025 is slower than today's record but beat everything swum by then.
	breaks := swim(t, s, "amy", "m-2025", 61000)
	if len(breaks) != 2 {
		t.Fatalf("back-dated swim should be a historical break in both slots, got %+v", breaks)
	}
}

func TestDetectRecordBreaksReturnsPersistenceError(t *testing.T) {
	s := newRecordsFixture(t)
	swim(t, s, "amy", "m-2026", 62000)

	var base *Result
	for _, r := range s.results {
		base = r
	}
	faster := *base
	faster.ID = "result-faster"
	ms := 50000
	faster.FinTimeMs = &ms

	// A closed pool makes every repository write fail.
	pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	s.repo = newRepository(pool)

	dir := s.snapshotDirectory("org")
	s.mu.Lock()
	breaks, err := s.detectRecordBreaksLocked("org", "coach", &faster, dir)
	s.mu.Unlock()
	if !errors.Is(err, ErrRecordDetection) {
		t.Fatalf("want ErrRecordDetection, got %v (breaks %+v)", err, breaks)
	}
	if len(breaks) != 0 {
		t.Fatalf("no break may be reported for an entry that failed to persist: %+v", breaks)
	}
}
