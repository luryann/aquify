package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/performance"
)

func TestMatcherStripsMiddleInitial(t *testing.T) {
	dob := time.Date(2012, 5, 1, 0, 0, 0, 0, time.UTC)
	m := newResultSwimmerMatcher([]members.Swimmer{{ID: "j", FirstName: "John", LastName: "Smith", DateOfBirth: "2012-05-01"}})
	got, reason := m.match(meetfiles.ParsedSwimmer{FirstName: "John A", LastName: "Smith", DOB: &dob})
	if got == nil {
		t.Fatalf("middle initial not stripped: %s", reason)
	}
	if got, _ := m.match(meetfiles.ParsedSwimmer{FirstName: "John Alan", LastName: "Smith", DOB: &dob}); got != nil {
		t.Fatal("multi-letter second token must not be stripped")
	}
}

func TestMatcherRejectsUSSConflictAndDOBlessForeignSwimmer(t *testing.T) {
	dob := time.Date(2012, 5, 1, 0, 0, 0, 0, time.UTC)
	m := newResultSwimmerMatcher([]members.Swimmer{{ID: "a", FirstName: "Alex", LastName: "Kim", USASwimmingID: "AAA111", DateOfBirth: "2012-05-01"}})
	if got, reason := m.match(meetfiles.ParsedSwimmer{FirstName: "Alex", LastName: "Kim", USSNumber: "XYZ999", DOB: &dob}); got != nil || reason == "" {
		t.Fatalf("USS conflict matched: %v %q", got, reason)
	}
	m2 := newResultSwimmerMatcher([]members.Swimmer{{ID: "a", FirstName: "Alex", LastName: "Kim"}})
	if got, _ := m2.match(meetfiles.ParsedSwimmer{FirstName: "Alex", LastName: "Kim", USSNumber: "XYZ999"}); got != nil {
		t.Fatal("name-only match with unknown file USS and no DOB must be rejected")
	}
}

func TestImportUsesPrelimTimeAndSkipsImplausible(t *testing.T) {
	dob := time.Date(2012, 5, 1, 0, 0, 0, 0, time.UTC)
	matcher := newResultSwimmerMatcher([]members.Swimmer{{ID: "amy", FirstName: "Amy", LastName: "Ames", Gender: "female", DateOfBirth: "2012-05-01"}})
	ev := func(d int) meetfiles.ParsedEvent {
		return meetfiles.ParsedEvent{Distance: d, Stroke: meetfiles.StrokeFreestyle, EventGender: "F", Course: meetfiles.Course("SCY")}
	}
	sw := meetfiles.ParsedSwimmer{FirstName: "Amy", LastName: "Ames", DOB: &dob}
	parsed := &meetfiles.ParseResult{IndividualResults: []meetfiles.ParsedIndividualResult{
		{Swimmer: sw, Event: ev(50), PrelimTime: meetfiles.Time{Raw: "x", Valid: true, Seconds: 26.5}},
		{Swimmer: sw, Event: ev(100), FinalsTime: meetfiles.Time{Raw: "x", Valid: true, Seconds: 0}},
	}}
	rec := &fakeRecorder{recorded: map[string]bool{}}
	prog, err := importParsedResults(rec, "org", importPrincipal, "meet", "SCY", parsed, matcher)
	if err != nil {
		t.Fatal(err)
	}
	if prog.Imported != 1 || prog.Skipped != 1 {
		t.Fatalf("progress = %+v", prog)
	}
}

func TestImportCountsAtomicDuplicateAsDuplicate(t *testing.T) {
	parsed, matcher := threeSwimFile()
	prog, err := importParsedResults(&dupRecorder{fakeRecorder{recorded: map[string]bool{}}}, "org", importPrincipal, "meet", "SCY", parsed, matcher)
	if err != nil || prog.Duplicates != 3 || prog.Imported != 0 {
		t.Fatalf("progress = %+v err = %v", prog, err)
	}
}

type dupRecorder struct{ fakeRecorder }

func (d *dupRecorder) UpsertResultWithRecords(_, _ string, _ *appmiddleware.Principal, _ performance.UpsertResultInput) (*performance.Result, []performance.RecordBreak, error) {
	return nil, nil, performance.ErrDuplicateMeetResult
}

var _ = errors.New
