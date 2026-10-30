//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

package sdif

import (
	"fmt"

	"github.com/dareaquatics/aquify/meetfiles"
)

// Parse parses an SD3/SDIF v3 file and returns a normalized ParseResult.
//
// The function is lenient about line endings (CRLF or LF both work) and line
// lengths (lines shorter than 160 chars are treated as right-padded with
// spaces).  Lines that cannot be identified by their record ID are recorded as
// ParseErrors with ErrCodeMalformedRecord, but parsing continues.
//
// The returned ParseResult.ParseErrors contains only structural parsing
// problems.  Swimmer-matching errors (unknown_athlete, ambiguous_match, etc.)
// are produced by the matcher package after parsing.
func Parse(data []byte) (*meetfiles.ParseResult, error) {
	result := &meetfiles.ParseResult{
		FileType: meetfiles.FileTypeSD3,
	}

	lineNumber := 0

	// pendingRelay holds the current E0 relay being built until the next
	// non-F0 record signals its end.
	var pendingRelay *meetfiles.ParsedRelayResult

	flush := func() {
		if pendingRelay != nil {
			result.RelayResults = append(result.RelayResults, *pendingRelay)
			pendingRelay = nil
		}
	}

	for _, raw := range splitRecordLines(data) {
		lineNumber++
		line := raw

		if len(line) < 2 {
			// Blank or too-short line — skip silently.
			continue
		}

		recordID := line[0:2]

		// Only F0 records belong to the current relay; any other record
		// closes the pending relay.
		if recordID != RecordF0 {
			flush()
		}

		switch recordID {
		case RecordA0:
			// File type is recorded; no further extraction needed.

		case RecordB1:
			info := ParseB1(line)
			result.MeetName = info.Name
			result.MeetStartDate = info.StartDate
			result.MeetEndDate = info.EndDate
			result.Course = info.Course

		case RecordB2, RecordC1, RecordC2, RecordD1, RecordD2, RecordD3, RecordG0, RecordZ0:
			// Recognised but not processed at this stage.

		case RecordD0:
			res, err := ParseD0(line, lineNumber)
			if err != nil {
				result.ParseErrors = append(result.ParseErrors, meetfiles.ParseError{
					LineNumber: lineNumber,
					Raw:        truncate(raw, 120),
					Code:       meetfiles.ErrCodeMalformedRecord,
					Message:    err.Error(),
				})
				continue
			}
			result.IndividualResults = append(result.IndividualResults, res)

		case RecordE0:
			relay := ParseE0(line, lineNumber)
			pendingRelay = &relay

		case RecordF0:
			if pendingRelay == nil {
				result.ParseErrors = append(result.ParseErrors, meetfiles.ParseError{
					LineNumber: lineNumber,
					Raw:        truncate(raw, 120),
					Code:       meetfiles.ErrCodeMalformedRecord,
					Message:    "F0 record found without preceding E0",
				})
				continue
			}
			leg := ParseF0(line, lineNumber)
			pendingRelay.Legs = append(pendingRelay.Legs, leg)

		default:
			result.ParseErrors = append(result.ParseErrors, meetfiles.ParseError{
				LineNumber: lineNumber,
				Raw:        truncate(raw, 120),
				Code:       meetfiles.ErrCodeMalformedRecord,
				Message:    fmt.Sprintf("unrecognised record ID %q", recordID),
			})
		}
	}

	// Flush any trailing relay.
	flush()

	return result, nil
}

// truncate returns the first n bytes of s, appending "…" when truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// splitRecordLines splits data into lines on CRLF, LF or a bare CR, with no
// per-line length limit (a bufio.Scanner would fail on >64KB lines, and a
// CR-only file is a single "line" to it).
func splitRecordLines(data []byte) []string {
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\n':
			lines = append(lines, string(data[start:i]))
			start = i + 1
		case '\r':
			lines = append(lines, string(data[start:i]))
			if i+1 < len(data) && data[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
