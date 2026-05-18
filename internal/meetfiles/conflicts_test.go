package meetfiles_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
)

// ─── BuildConflictQueue ───────────────────────────────────────────────────────

func TestBuildConflictQueue_NoConflicts(t *testing.T) {
	results := []meetfiles.ResultWithMatch{
		{
			Result: ind(1, "Smith", "John", "0001"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
		{
			Result: ind(2, "Doe", "Jane", "0002"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_002"},
		},
	}
	conflicts := meetfiles.BuildConflictQueue(results)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d: %+v", len(conflicts), conflicts)
	}
}

func TestBuildConflictQueue_NoneAndAmbiguous(t *testing.T) {
	results := []meetfiles.ResultWithMatch{
		{
			Result: ind(5, "Ghost", "Person", "0001"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchNone, Strategy: "name_dob_gender"},
		},
		{
			Result: ind(10, "Common", "Name", "0002"),
			Match: meetfiles.MatchResult{
				Outcome:    meetfiles.MatchAmbiguous,
				Candidates: []string{"swimmer_003", "swimmer_004"},
				Strategy:   "name_dob_gender",
			},
		},
		{
			Result: ind(15, "Smith", "John", "0001"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
	}

	conflicts := meetfiles.BuildConflictQueue(results)
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d: %+v", len(conflicts), conflicts)
	}

	// First conflict: MatchNone on line 5.
	c0 := conflicts[0]
	if c0.LineNumber != 5 {
		t.Errorf("conflict[0].LineNumber = %d, want 5", c0.LineNumber)
	}
	if c0.Code != meetfiles.ErrCodeUnknownAthlete {
		t.Errorf("conflict[0].Code = %q, want unknown_athlete", c0.Code)
	}

	// Second conflict: MatchAmbiguous on line 10.
	c1 := conflicts[1]
	if c1.LineNumber != 10 {
		t.Errorf("conflict[1].LineNumber = %d, want 10", c1.LineNumber)
	}
	if c1.Code != meetfiles.ErrCodeAmbiguousMatch {
		t.Errorf("conflict[1].Code = %q, want ambiguous_match", c1.Code)
	}
	if len(c1.CandidateSwimmerIDs) != 2 {
		t.Errorf("len(CandidateSwimmerIDs) = %d, want 2", len(c1.CandidateSwimmerIDs))
	}
}

// ─── ClassifyEventErrors ─────────────────────────────────────────────────────

func TestClassifyEventErrors_AllKnown(t *testing.T) {
	known := []meetfiles.KnownEvent{
		{EventNumber: "0001", Distance: 100, Stroke: meetfiles.StrokeFreestyle, EventGender: "M"},
		{EventNumber: "0002", Distance: 200, Stroke: meetfiles.StrokeBackstroke, EventGender: "F"},
	}
	results := []meetfiles.ParsedIndividualResult{
		ind(1, "Smith", "John", "0001"),
		ind(2, "Doe", "Jane", "0002"),
	}
	errs := meetfiles.ClassifyEventErrors(results, known)
	if len(errs) != 0 {
		t.Errorf("expected 0 event errors, got %d: %+v", len(errs), errs)
	}
}

func TestClassifyEventErrors_UnknownEvent(t *testing.T) {
	known := []meetfiles.KnownEvent{
		{EventNumber: "0001"},
	}
	results := []meetfiles.ParsedIndividualResult{
		ind(1, "Smith", "John", "0001"), // known
		ind(2, "Doe", "Jane", "9999"),   // unknown
	}
	errs := meetfiles.ClassifyEventErrors(results, known)
	if len(errs) != 1 {
		t.Fatalf("expected 1 event error, got %d: %+v", len(errs), errs)
	}
	if errs[0].Code != meetfiles.ErrCodeBadEventCode {
		t.Errorf("Code = %q, want bad_event_code", errs[0].Code)
	}
	if errs[0].LineNumber != 2 {
		t.Errorf("LineNumber = %d, want 2", errs[0].LineNumber)
	}
}

func TestClassifyEventErrors_EmptyKnownList(t *testing.T) {
	// When no known events are provided, no errors should be returned.
	results := []meetfiles.ParsedIndividualResult{
		ind(1, "Smith", "John", "9999"),
	}
	errs := meetfiles.ClassifyEventErrors(results, nil)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors with empty knownEvents, got %d", len(errs))
	}
}

// ─── ClassifyDuplicates ───────────────────────────────────────────────────────

func TestClassifyDuplicates_NoDuplicates(t *testing.T) {
	results := []meetfiles.ResultWithMatch{
		{
			Result: ind(1, "Smith", "John", "0001"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
		{
			Result: ind(2, "Doe", "Jane", "0001"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_002"},
		},
	}
	errs := meetfiles.ClassifyDuplicates(results)
	if len(errs) != 0 {
		t.Errorf("expected 0 duplicate errors, got %d: %+v", len(errs), errs)
	}
}

func TestClassifyDuplicates_WithDuplicate(t *testing.T) {
	results := []meetfiles.ResultWithMatch{
		{
			Result: ind(3, "Smith", "John", "0001"),
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
		{
			Result: ind(7, "Smith", "John", "0001"), // same swimmer + event
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
	}
	errs := meetfiles.ClassifyDuplicates(results)
	if len(errs) != 1 {
		t.Fatalf("expected 1 duplicate error, got %d: %+v", len(errs), errs)
	}
	if errs[0].Code != meetfiles.ErrCodeDuplicateRow {
		t.Errorf("Code = %q, want duplicate_row", errs[0].Code)
	}
	if errs[0].LineNumber != 7 {
		t.Errorf("LineNumber = %d, want 7 (second occurrence)", errs[0].LineNumber)
	}
}

// ─── BuildReport ─────────────────────────────────────────────────────────────

func TestBuildReport_Deterministic(t *testing.T) {
	parsed := &meetfiles.ParseResult{
		FileType: meetfiles.FileTypeSD3,
		ParseErrors: []meetfiles.ParseError{
			{LineNumber: 10, Code: meetfiles.ErrCodeMalformedRecord, Message: "bad line"},
			{LineNumber: 3, Code: meetfiles.ErrCodeMalformedRecord, Message: "another bad line"},
		},
		IndividualResults: []meetfiles.ParsedIndividualResult{
			ind(5, "Smith", "John", "0001"),
			ind(8, "Doe", "Jane", "0002"),
		},
	}

	withMatches := []meetfiles.ResultWithMatch{
		{
			Result: parsed.IndividualResults[0],
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
		{
			Result: parsed.IndividualResults[1],
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchNone, Strategy: "name_dob_gender"},
		},
	}

	report := meetfiles.BuildReport(parsed, withMatches, nil)

	if report.TotalRows != 2 {
		t.Errorf("TotalRows = %d, want 2", report.TotalRows)
	}
	if report.MatchedRows != 1 {
		t.Errorf("MatchedRows = %d, want 1", report.MatchedRows)
	}
	if report.ErrorRows != 2 {
		t.Errorf("ErrorRows = %d, want 2 (parse errors)", report.ErrorRows)
	}
	if report.ConflictRows != 1 {
		t.Errorf("ConflictRows = %d, want 1 (no_match)", report.ConflictRows)
	}

	// Errors should be sorted by line number: 3 before 10.
	if len(report.Errors) < 2 {
		t.Fatalf("len(Errors) = %d, want 2", len(report.Errors))
	}
	if report.Errors[0].LineNumber != 3 {
		t.Errorf("Errors[0].LineNumber = %d, want 3", report.Errors[0].LineNumber)
	}
	if report.Errors[1].LineNumber != 10 {
		t.Errorf("Errors[1].LineNumber = %d, want 10", report.Errors[1].LineNumber)
	}
}

func TestBuildReport_GoldenFile(t *testing.T) {
	parsed := &meetfiles.ParseResult{
		FileType: meetfiles.FileTypeSD3,
		ParseErrors: []meetfiles.ParseError{
			{LineNumber: 10, Code: meetfiles.ErrCodeMalformedRecord, Message: "bad line"},
			{LineNumber: 3, Code: meetfiles.ErrCodeMalformedRecord, Message: "another bad line"},
		},
		IndividualResults: []meetfiles.ParsedIndividualResult{
			ind(5, "Smith", "John", "0001"),
			ind(8, "Doe", "Jane", "0002"),
		},
	}

	withMatches := []meetfiles.ResultWithMatch{
		{
			Result: parsed.IndividualResults[0],
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchExact, SwimmerID: "swimmer_001"},
		},
		{
			Result: parsed.IndividualResults[1],
			Match:  meetfiles.MatchResult{Outcome: meetfiles.MatchNone, Strategy: "name_dob_gender"},
		},
	}

	report := meetfiles.BuildReport(parsed, withMatches, nil)
	got, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	want, err := os.ReadFile(filepath.Join("testdata", "import_report.golden.json"))
	if err != nil {
		t.Fatalf("read golden report: %v", err)
	}

	if string(got)+"\n" != string(want) {
		t.Fatalf("import report golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// ind creates a minimal ParsedIndividualResult for testing.
func ind(line int, last, first, eventNum string) meetfiles.ParsedIndividualResult {
	return meetfiles.ParsedIndividualResult{
		LineNumber: line,
		Swimmer: meetfiles.ParsedSwimmer{
			LastName:  last,
			FirstName: first,
			DOB:       timePtr(2005, 1, 1),
			Gender:    "M",
		},
		Event: meetfiles.ParsedEvent{
			EventNumber: eventNum,
			Distance:    100,
			Stroke:      meetfiles.StrokeFreestyle,
		},
	}
}

func timePtr(year, month, day int) *time.Time {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return &t
}
