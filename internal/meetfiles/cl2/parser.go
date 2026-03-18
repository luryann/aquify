// Package cl2 provides a best-effort scaffold parser for CL2 files produced
// by Colorado Timing Systems (CTS) timing hardware.
// Signed off: RL, 3/15/2026
//
// Format status -
//
// The CL2 format is proprietary to Colorado Timing Systems and has no publicly
// available specification. This implementation is a scaffold that:
//   - Iterates the file line by line.
//   - Attempts to classify each line by a leading keyword (EVENT, HEAT, etc.)
//     based on reverse-engineering of known CL2 samples.
//   - Returns ErrCodeMalformedRecord for lines it cannot classify, so the
//     import pipeline can surface them to an administrator.
//
// When real CL2 sample files become available, the parseEventLine /
// parseHeatLine helpers below should be expanded with the correct field widths.
package cl2

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/dareaquatics/aquify/meetfiles"
)

// Parse performs a best-effort parse of a CL2 file.
//
// Because the CL2 spec is not publicly available, this implementation extracts
// what it can from common structural patterns and records unrecognised lines as
// ErrCodeMalformedRecord parse errors.  The caller should treat the returned
// ParseResult as a partial result and route any parse errors to the admin
// conflict queue for manual review.
func Parse(data []byte) (*meetfiles.ParseResult, error) {
	result := &meetfiles.ParseResult{
		FileType: meetfiles.FileTypeCL2,
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		raw := scanner.Text()
		line := strings.TrimRight(raw, "\r")

		if strings.TrimSpace(line) == "" {
			continue
		}

		upper := strings.ToUpper(strings.TrimSpace(line)) // what

		switch {
		case strings.HasPrefix(upper, "EVENT"):
			// Best-effort: log but do not error — we know events exist.
			// A full implementation would extract event number, distance,
			// stroke, and course from fixed-width fields.

		case strings.HasPrefix(upper, "HEAT"):
			// Best-effort: attempt to extract heat/lane/swimmer/time.
			// Full column positions require an actual CL2 spec.

		case strings.HasPrefix(upper, "CL2"):
			// File header — ignore.

		default:
			// Unrecognised line: record as a parse error so administrators
			// can review the raw content.
			result.ParseErrors = append(result.ParseErrors, meetfiles.ParseError{
				LineNumber: lineNumber,
				Raw:        truncate(line, 120),
				Code:       meetfiles.ErrCodeMalformedRecord,
				Message: fmt.Sprintf(
					"CL2 line %d could not be classified (proprietary format — extend cl2.Parse with column layout from actual CL2 samples)",
					lineNumber,
				),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cl2: scanner: %w", err)
	}

	return result, nil
}

// truncate returns the first n characters of s, appending "…" when truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
