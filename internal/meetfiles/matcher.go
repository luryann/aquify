package meetfiles

import (
	"strings"
	"time"
)

// ─── Roster types ─────────────────────────────────────────────────────────────

// RosterSwimmer is a swimmer record from the platform database, used for
// identity matching against parsed records.
type RosterSwimmer struct {
	// ID is the platform swimmer ID (primary key).
	ID string
	// USSNumber is the swimmer's USAS registration number.  May be blank.
	USSNumber string
	// LastName and FirstName are stored as normalised strings.
	LastName  string
	FirstName string
	// DOB is the swimmer's date of birth.  Nil means unknown.
	DOB *time.Time
	// Gender is "M" or "F".
	Gender string
}

// ─── Match result ─────────────────────────────────────────────────────────────

// MatchOutcome classifies the result of a swimmer match attempt.
type MatchOutcome string

const (
	// MatchExact indicates a unique, confident match.
	MatchExact MatchOutcome = "exact"
	// MatchAmbiguous means multiple roster swimmers are equally plausible.
	MatchAmbiguous MatchOutcome = "ambiguous"
	// MatchNone means no roster swimmer could be matched.
	MatchNone MatchOutcome = "none"
)

// MatchResult holds the outcome of matching one ParsedSwimmer against a roster.
type MatchResult struct {
	Outcome    MatchOutcome
	SwimmerID  string   // populated only when Outcome == MatchExact
	Candidates []string // roster IDs of plausible swimmers (for MatchAmbiguous)
	Strategy   string   // "uss_number" | "name_dob_gender" — how the match was made
}

// ─── Matching algorithm ───────────────────────────────────────────────────────

// MatchSwimmer attempts to identify a ParsedSwimmer in the provided roster.
//
// Strategy:
//  1. If the parsed record contains a non-blank USS number, attempt an exact
//     match on USSNumber.  If exactly one roster swimmer matches, return
//     MatchExact with Strategy="uss_number".
//  2. Otherwise, normalise (last name, first name, DOB, gender) and collect all
//     roster swimmers that match all non-blank identity fields.
//     - Exactly one match → MatchExact with Strategy="name_dob_gender".
//     - More than one match → MatchAmbiguous.
//     - Zero matches → MatchNone.
func MatchSwimmer(parsed ParsedSwimmer, roster []RosterSwimmer) MatchResult {
	// ── Strategy 1: USS number ────────────────────────────────────────────
	if ussNum := normaliseUSS(parsed.USSNumber); ussNum != "" {
		var hits []RosterSwimmer
		for _, r := range roster {
			if normaliseUSS(r.USSNumber) == ussNum {
				hits = append(hits, r)
			}
		}
		switch len(hits) {
		case 1:
			return MatchResult{
				Outcome:   MatchExact,
				SwimmerID: hits[0].ID,
				Strategy:  "uss_number",
			}
		case 0:
			// USS# not found — fall through to name matching.
		default:
			// Multiple roster entries with same USS# (data quality issue);
			// treat as ambiguous and let admin resolve.
			ids := make([]string, len(hits))
			for i, h := range hits {
				ids[i] = h.ID
			}
			return MatchResult{
				Outcome:    MatchAmbiguous,
				Candidates: ids,
				Strategy:   "uss_number",
			}
		}
	}

	// ── Strategy 2: Name + DOB + Gender ──────────────────────────────────
	parsedLast := normaliseName(parsed.LastName)
	parsedFirst := normaliseName(parsed.FirstName)
	parsedGender := strings.ToUpper(strings.TrimSpace(parsed.Gender))

	var candidates []RosterSwimmer
	for _, r := range roster {
		if normaliseName(r.LastName) != parsedLast {
			continue
		}
		if parsedFirst != "" && normaliseName(r.FirstName) != parsedFirst {
			continue
		}
		if parsedGender != "" && strings.ToUpper(r.Gender) != parsedGender {
			continue
		}
		if parsed.DOB != nil && r.DOB != nil {
			if !sameDay(*parsed.DOB, *r.DOB) {
				continue
			}
		}
		candidates = append(candidates, r)
	}

	switch len(candidates) {
	case 1:
		return MatchResult{
			Outcome:   MatchExact,
			SwimmerID: candidates[0].ID,
			Strategy:  "name_dob_gender",
		}
	case 0:
		return MatchResult{Outcome: MatchNone, Strategy: "name_dob_gender"}
	default:
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		return MatchResult{
			Outcome:    MatchAmbiguous,
			Candidates: ids,
			Strategy:   "name_dob_gender",
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// normaliseUSS strips whitespace and asterisk padding from a USS number string.
func normaliseUSS(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "*") // asterisk is the padding char in old USS#
	return s
}

// normaliseName lower-cases and trims a name component.
func normaliseName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// sameDay returns true when two times fall on the same calendar day (UTC).
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}
