package meetfiles_test

import (
	"testing"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
)

func dob(month, day, year int) *time.Time {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return &t
}

// ─── Test roster ─────────────────────────────────────────────────────────────

var testRoster = []meetfiles.RosterSwimmer{
	{
		ID:        "swimmer_001",
		USSNumber: "123456789ABC",
		LastName:  "Smith",
		FirstName: "John",
		DOB:       dob(2, 15, 1999),
		Gender:    "M",
	},
	{
		ID:        "swimmer_002",
		USSNumber: "",
		LastName:  "Doe",
		FirstName: "Jane",
		DOB:       dob(5, 10, 2008),
		Gender:    "F",
	},
	{
		ID:        "swimmer_003",
		USSNumber: "",
		LastName:  "Smith",
		FirstName: "Michael",
		DOB:       dob(7, 22, 2005),
		Gender:    "M",
	},
	// Duplicate USS# (data quality scenario).
	{
		ID:        "swimmer_004",
		USSNumber: "DUPUSS111111",
		LastName:  "Brown",
		FirstName: "Alice",
		DOB:       dob(9, 1, 2000),
		Gender:    "F",
	},
	{
		ID:        "swimmer_005",
		USSNumber: "DUPUSS111111",
		LastName:  "Brown",
		FirstName: "Alicia",
		DOB:       dob(9, 1, 2000),
		Gender:    "F",
	},
}

// ─── USS# match ───────────────────────────────────────────────────────────────

func TestMatchSwimmer_USSExactMatch(t *testing.T) {
	parsed := meetfiles.ParsedSwimmer{
		LastName:  "Smith",
		FirstName: "John",
		DOB:       dob(2, 15, 1999),
		Gender:    "M",
		USSNumber: "123456789ABC",
	}
	result := meetfiles.MatchSwimmer(parsed, testRoster)
	if result.Outcome != meetfiles.MatchExact {
		t.Fatalf("Outcome = %v, want MatchExact", result.Outcome)
	}
	if result.SwimmerID != "swimmer_001" {
		t.Errorf("SwimmerID = %q, want swimmer_001", result.SwimmerID)
	}
	if result.Strategy != "uss_number" {
		t.Errorf("Strategy = %q, want uss_number", result.Strategy)
	}
}

func TestMatchSwimmer_USSAmbiguous(t *testing.T) {
	// Two roster entries share the same USS number — treat as ambiguous.
	parsed := meetfiles.ParsedSwimmer{
		USSNumber: "DUPUSS111111",
		LastName:  "Brown",
		FirstName: "Alice",
	}
	result := meetfiles.MatchSwimmer(parsed, testRoster)
	if result.Outcome != meetfiles.MatchAmbiguous {
		t.Fatalf("Outcome = %v, want MatchAmbiguous", result.Outcome)
	}
	if len(result.Candidates) != 2 {
		t.Errorf("len(Candidates) = %d, want 2", len(result.Candidates))
	}
}

// ─── Name + DOB + gender fallback ────────────────────────────────────────────

func TestMatchSwimmer_NameDOBFallback(t *testing.T) {
	parsed := meetfiles.ParsedSwimmer{
		LastName:  "Doe",
		FirstName: "Jane",
		DOB:       dob(5, 10, 2008),
		Gender:    "F",
	}
	result := meetfiles.MatchSwimmer(parsed, testRoster)
	if result.Outcome != meetfiles.MatchExact {
		t.Fatalf("Outcome = %v, want MatchExact", result.Outcome)
	}
	if result.SwimmerID != "swimmer_002" {
		t.Errorf("SwimmerID = %q, want swimmer_002", result.SwimmerID)
	}
	if result.Strategy != "name_dob_gender" {
		t.Errorf("Strategy = %q, want name_dob_gender", result.Strategy)
	}
}

func TestMatchSwimmer_NoMatch(t *testing.T) {
	parsed := meetfiles.ParsedSwimmer{
		LastName:  "Unknown",
		FirstName: "Person",
		DOB:       dob(1, 1, 2010),
		Gender:    "F",
	}
	result := meetfiles.MatchSwimmer(parsed, testRoster)
	if result.Outcome != meetfiles.MatchNone {
		t.Fatalf("Outcome = %v, want MatchNone", result.Outcome)
	}
}

func TestMatchSwimmer_AmbiguousNameMatch(t *testing.T) {
	// Two "Smith, M*" swimmers — DOB and gender differ, but if parsed record
	// has no DOB/gender we can still get ambiguity on last name alone.
	parsed := meetfiles.ParsedSwimmer{
		LastName: "Smith",
		// No first name, DOB, or gender — intentionally incomplete.
	}
	result := meetfiles.MatchSwimmer(parsed, testRoster)
	// Expect ambiguous: both Smith, John and Smith, Michael match last name.
	if result.Outcome != meetfiles.MatchAmbiguous {
		t.Fatalf("Outcome = %v, want MatchAmbiguous", result.Outcome)
	}
	if len(result.Candidates) < 2 {
		t.Errorf("len(Candidates) = %d, want >= 2", len(result.Candidates))
	}
}

func TestMatchSwimmer_EmptyRoster(t *testing.T) {
	parsed := meetfiles.ParsedSwimmer{
		LastName: "Anyone",
		USSNumber: "X",
	}
	result := meetfiles.MatchSwimmer(parsed, nil)
	if result.Outcome != meetfiles.MatchNone {
		t.Fatalf("Outcome = %v, want MatchNone", result.Outcome)
	}
}

// ─── USS# normalisation ───────────────────────────────────────────────────────

func TestMatchSwimmer_USSNumberTrimmed(t *testing.T) {
	// USS# with trailing asterisk padding (old format) should still match.
	parsed := meetfiles.ParsedSwimmer{
		USSNumber: "123456789ABC**", // asterisk-padded
		LastName:  "Smith",
	}
	result := meetfiles.MatchSwimmer(parsed, testRoster)
	if result.Outcome != meetfiles.MatchExact {
		t.Fatalf("Outcome = %v, want MatchExact after asterisk trim", result.Outcome)
	}
}
