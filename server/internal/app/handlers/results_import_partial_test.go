package handlers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

// fakeRecorder records results in memory and can be told to fail the Nth write.
type fakeRecorder struct {
	recorded    map[string]bool
	writes      int
	failOnWrite int   // 1-based; 0 = never
	failWith    error // error returned on that write (result is stored when it wraps ErrRecordDetection)
}

func (f *fakeRecorder) key(swimmerID string, distance int, stroke string) string {
	return fmt.Sprintf("%s|%d|%s", swimmerID, distance, stroke)
}

func (f *fakeRecorder) HasMeetResult(_, swimmerID, _ string, distance int, stroke, _ string) bool {
	return f.recorded[f.key(swimmerID, distance, stroke)]
}

func (f *fakeRecorder) UpsertResultWithRecords(_, _ string, _ *appmiddleware.Principal, in performance.UpsertResultInput) (*performance.Result, []performance.RecordBreak, error) {
	f.writes++
	if f.failOnWrite == f.writes {
		if errors.Is(f.failWith, performance.ErrRecordDetection) {
			f.recorded[f.key(in.SwimmerID, in.Distance, in.Stroke)] = true
			return &performance.Result{}, nil, f.failWith
		}
		return nil, nil, f.failWith
	}
	f.recorded[f.key(in.SwimmerID, in.Distance, in.Stroke)] = true
	return &performance.Result{}, nil, nil
}

func threeSwimFile() (*meetfiles.ParseResult, *resultSwimmerMatcher) {
	row := func(distance int, seconds float64) meetfiles.ParsedIndividualResult {
		return meetfiles.ParsedIndividualResult{
			Swimmer:    meetfiles.ParsedSwimmer{FirstName: "Amy", LastName: "Ames"},
			Event:      meetfiles.ParsedEvent{Distance: distance, Stroke: meetfiles.StrokeFreestyle, EventGender: "F", Course: meetfiles.Course("SCY")},
			FinalsTime: meetfiles.Time{Raw: "x", Valid: true, Seconds: seconds},
		}
	}
	parsed := &meetfiles.ParseResult{IndividualResults: []meetfiles.ParsedIndividualResult{row(50, 25), row(100, 55), row(200, 120)}}
	matcher := newResultSwimmerMatcher([]members.Swimmer{{ID: "amy", FirstName: "Amy", LastName: "Ames", Gender: "female"}})
	return parsed, matcher
}

var importPrincipal = &appmiddleware.Principal{UserID: "coach", OrgID: "org", Roles: []string{"coach"}}

// Idempotency is per swim: a failure mid-file reports exact partial progress,
// and re-running the same file finishes the rest without duplicating anything.
func TestResultsImportReportsPartialProgressAndResumesToCompletion(t *testing.T) {
	parsed, matcher := threeSwimFile()
	rec := &fakeRecorder{recorded: map[string]bool{}, failOnWrite: 2, failWith: errors.New("db down")}

	first, err := importParsedResults(rec, "org", importPrincipal, "meet", "SCY", parsed, matcher)
	if err == nil {
		t.Fatal("expected the mid-file failure to be returned")
	}
	if first.Imported != 1 || first.Processed != 1 || first.Total != 3 {
		t.Fatalf("partial progress = %+v, want imported=1 processed=1 total=3", first)
	}
	if len(rec.recorded) != 1 {
		t.Fatalf("exactly the first swim should be recorded, got %v", rec.recorded)
	}

	rec.failOnWrite = 0
	second, err := importParsedResults(rec, "org", importPrincipal, "meet", "SCY", parsed, matcher)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if second.Duplicates != 1 || second.Imported != 2 || second.Processed != 3 {
		t.Fatalf("re-run progress = %+v, want duplicates=1 imported=2", second)
	}
	if len(rec.recorded) != 3 || rec.writes != 4 {
		t.Fatalf("all three swims recorded exactly once: recorded=%d writes=%d", len(rec.recorded), rec.writes)
	}

	third, err := importParsedResults(rec, "org", importPrincipal, "meet", "SCY", parsed, matcher)
	if err != nil || third.Imported != 0 || third.Duplicates != 3 || rec.writes != 4 {
		t.Fatalf("a completed import is a no-op: %+v writes=%d err=%v", third, rec.writes, err)
	}
}

func TestResultsImportKeepsGoingWhenOnlyRecordDetectionFails(t *testing.T) {
	parsed, matcher := threeSwimFile()
	rec := &fakeRecorder{recorded: map[string]bool{}, failOnWrite: 1, failWith: fmt.Errorf("%w: boom", performance.ErrRecordDetection)}
	got, err := importParsedResults(rec, "org", importPrincipal, "meet", "SCY", parsed, matcher)
	if err != nil {
		t.Fatalf("detection failures must not abort the import: %v", err)
	}
	if got.Imported != 3 || got.RecordDetectionFailures != 1 {
		t.Fatalf("progress = %+v, want imported=3 record_detection_failures=1", got)
	}
}
