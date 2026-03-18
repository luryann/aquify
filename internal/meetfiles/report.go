package meetfiles

import "sort"

// Report types 

// ImportReport is the final, deterministic summary produced after a completed
// import parse + match run.  It is persisted alongside the import job record.
type ImportReport struct {
	TotalRows    int          `json:"total_rows"`
	MatchedRows  int          `json:"matched_rows"`
	ErrorRows    int          `json:"error_rows"`
	ConflictRows int          `json:"conflict_rows"`
	Errors       []ReportLine `json:"errors,omitempty"`
	Conflicts    []ReportLine `json:"conflicts,omitempty"`
}

// ReportLine is a single entry in the error or conflict section of a report.
type ReportLine struct {
	LineNumber int       `json:"line_number,omitempty"`
	ErrorCode  ErrorCode `json:"error_code"`
	Message    string    `json:"message"`
	RawData    string    `json:"raw_data,omitempty"`
}

// Report builder 

// BuildReport constructs a deterministic ImportReport from a ParseResult and
// the per-result match outcomes.
//
// Determinism guarantees:
//   - All report lines are sorted by line number ascending.
//   - Lines with the same line number are sorted by error code then message.
//
// The caller is expected to have already run ClassifyEventErrors and
// ClassifyDuplicates and pass those errors as additionalErrors.
func BuildReport(
	parsed *ParseResult,
	withMatches []ResultWithMatch,
	additionalErrors []ParseError,
) ImportReport {
	var errors []ReportLine
	var conflicts []ReportLine

	// Structural parse errors 
	for _, pe := range parsed.ParseErrors {
		errors = append(errors, ReportLine{
			LineNumber: pe.LineNumber,
			ErrorCode:  pe.Code,
			Message:    pe.Message,
			RawData:    pe.Raw,
		})
	}

	// Additional classified errors (bad event code, duplicate) 
	for _, pe := range additionalErrors {
		errors = append(errors, ReportLine{
			LineNumber: pe.LineNumber,
			ErrorCode:  pe.Code,
			Message:    pe.Message,
			RawData:    pe.Raw,
		})
	}

	// Match conflicts 
	conflictEntries := BuildConflictQueue(withMatches)
	for _, ce := range conflictEntries {
		conflicts = append(conflicts, ReportLine{
			LineNumber: ce.LineNumber,
			ErrorCode:  ce.Code,
			Message:    ce.Message,
			RawData:    ce.RawData,
		})
	}

	// Counts 
	totalRows := len(parsed.IndividualResults)
	matchedRows := 0
	for _, rm := range withMatches {
		if rm.Match.Outcome == MatchExact {
			matchedRows++
		}
	}

	// Sort for determinism 
	sortLines(errors)
	sortLines(conflicts)

	return ImportReport{
		TotalRows:    totalRows,
		MatchedRows:  matchedRows,
		ErrorRows:    len(errors),
		ConflictRows: len(conflicts),
		Errors:       nilIfEmpty(errors),
		Conflicts:    nilIfEmpty(conflicts),
	}
}

// Helpers 

// sortLines sorts report lines by (line_number, error_code, message) for
// deterministic output regardless of map iteration order.

// i don't know what the fuck I just wrote but it compiles so
func sortLines(lines []ReportLine) {
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.LineNumber != b.LineNumber {
			return a.LineNumber < b.LineNumber
		}
		if a.ErrorCode != b.ErrorCode {
			return a.ErrorCode < b.ErrorCode
		}
		return a.Message < b.Message
	})
}

// nilIfEmpty returns nil when the slice has no elements, which keeps JSON
// output clean ("errors": null vs "errors": []).
func nilIfEmpty(lines []ReportLine) []ReportLine {
	if len(lines) == 0 {
		return nil
	}
	return lines
}
