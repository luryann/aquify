package meetfiles

import "fmt"

// ─── Conflict entry ───────────────────────────────────────────────────────────

// ConflictEntry represents a single import conflict that must be resolved
// before the import can be fully applied.
type ConflictEntry struct {
	LineNumber          int
	RawData             string   // short representation of the parsed row
	Code                ErrorCode
	Message             string
	CandidateSwimmerIDs []string // populated for MatchAmbiguous outcomes
}

// ─── ResultWithMatch ─────────────────────────────────────────────────────────

// ResultWithMatch pairs a parsed individual result with its match outcome.
type ResultWithMatch struct {
	Result  ParsedIndividualResult
	Match   MatchResult
}

// ─── Conflict queue generation ───────────────────────────────────────────────

// BuildConflictQueue inspects a slice of match results and returns a
// deterministic list of ConflictEntries (sorted by line number ascending) for
// any result that could not be uniquely matched.
//
// Caller should also pass parse-level errors (from ParseResult.ParseErrors)
// separately if they should be included in the conflict queue.
func BuildConflictQueue(results []ResultWithMatch) []ConflictEntry {
	var conflicts []ConflictEntry

	for _, rm := range results {
		r := rm.Result
		m := rm.Match

		switch m.Outcome {
		case MatchExact:
			// Successfully matched — nothing to queue.

		case MatchNone:
			conflicts = append(conflicts, ConflictEntry{
				LineNumber: r.LineNumber,
				RawData:    formatRawData(r),
				Code:       ErrCodeUnknownAthlete,
				Message: fmt.Sprintf(
					"no roster swimmer matched %q (DOB: %s, gender: %s) via %s",
					r.Swimmer.LastName+", "+r.Swimmer.FirstName,
					formatDOB(r.Swimmer),
					r.Swimmer.Gender,
					m.Strategy,
				),
				CandidateSwimmerIDs: nil,
			})

		case MatchAmbiguous:
			conflicts = append(conflicts, ConflictEntry{
				LineNumber: r.LineNumber,
				RawData:    formatRawData(r),
				Code:       ErrCodeAmbiguousMatch,
				Message: fmt.Sprintf(
					"multiple roster swimmers matched %q (DOB: %s, gender: %s) via %s",
					r.Swimmer.LastName+", "+r.Swimmer.FirstName,
					formatDOB(r.Swimmer),
					r.Swimmer.Gender,
					m.Strategy,
				),
				CandidateSwimmerIDs: m.Candidates,
			})
		}
	}

	return conflicts
}

// ─── Event validation ─────────────────────────────────────────────────────────

// KnownEvent is a simplified descriptor used to validate whether a parsed
// event exists in the meet program.
type KnownEvent struct {
	// EventNumber is the alphanumeric event number from the meet program.
	EventNumber string
	Distance    int
	Stroke      Stroke
	EventGender string
}

// ClassifyEventErrors checks each individual result against the set of known
// meet events and returns ErrorCode entries for any results whose event is not
// in the set.
//
// If knownEvents is empty (e.g. the meet program is not available), all events
// are treated as valid and an empty slice is returned.
func ClassifyEventErrors(results []ParsedIndividualResult, knownEvents []KnownEvent) []ParseError {
	if len(knownEvents) == 0 {
		return nil
	}

	// Build a lookup set keyed by (eventNumber).  Event number is the
	// authoritative identifier in SD3.
	known := make(map[string]bool, len(knownEvents))
	for _, e := range knownEvents {
		known[e.EventNumber] = true
	}

	var errs []ParseError
	for _, r := range results {
		if r.Event.EventNumber == "" {
			continue
		}
		if !known[r.Event.EventNumber] {
			errs = append(errs, ParseError{
				LineNumber: r.LineNumber,
				Raw:        formatRawData(r),
				Code:       ErrCodeBadEventCode,
				Message: fmt.Sprintf(
					"event %q not found in meet program (dist=%d, stroke=%s)",
					r.Event.EventNumber, r.Event.Distance, r.Event.Stroke,
				),
			})
		}
	}
	return errs
}

// ─── Duplicate detection ─────────────────────────────────────────────────────

// ClassifyDuplicates detects rows where the same swimmer appears in the same
// event more than once.  The first occurrence is kept; subsequent occurrences
// are returned as ParseErrors with ErrCodeDuplicateRow.
//
// "Same swimmer + event" is determined by (swimmerID, eventNumber).  When the
// swimmerID is blank (not yet matched), the raw name+event combination is used.
func ClassifyDuplicates(results []ResultWithMatch) []ParseError {
	seen := make(map[string]int) // key → first line number
	var errs []ParseError

	for _, rm := range results {
		r := rm.Result
		swimmerKey := rm.Match.SwimmerID
		if swimmerKey == "" {
			swimmerKey = normKey(r.Swimmer.LastName + "|" + r.Swimmer.FirstName)
		}
		key := swimmerKey + ":" + r.Event.EventNumber
		if first, exists := seen[key]; exists {
			errs = append(errs, ParseError{
				LineNumber: r.LineNumber,
				Raw:        formatRawData(r),
				Code:       ErrCodeDuplicateRow,
				Message: fmt.Sprintf(
					"duplicate result for swimmer %q in event %q (first occurrence at line %d)",
					r.Swimmer.LastName+", "+r.Swimmer.FirstName,
					r.Event.EventNumber,
					first,
				),
			})
		} else {
			seen[key] = r.LineNumber
		}
	}
	return errs
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func formatRawData(r ParsedIndividualResult) string {
	return fmt.Sprintf("%s, %s | event %s | %dm %s",
		r.Swimmer.LastName, r.Swimmer.FirstName,
		r.Event.EventNumber, r.Event.Distance, r.Event.Stroke)
}

func formatDOB(s ParsedSwimmer) string {
	if s.DOB == nil {
		return "unknown"
	}
	return s.DOB.Format("2006-01-02")
}

func normKey(s string) string {
	var out []byte
	for _, b := range []byte(s) {
		if b >= 'A' && b <= 'Z' {
			out = append(out, b+32)
		} else {
			out = append(out, b)
		}
	}
	return string(out)
}
