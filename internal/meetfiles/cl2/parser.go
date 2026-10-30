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
	"github.com/dareaquatics/aquify/meetfiles/sdif"
)

var utf8BOM = []byte("\xef\xbb\xbf")

// Parse performs a best-effort parse of a CL2 file.
//
// Because the CL2 spec is not publicly available, this implementation extracts
// what it can from common structural patterns and records unrecognised lines as
// ErrCodeMalformedRecord parse errors.  The caller should treat the returned
// ParseResult as a partial result and route any parse errors to the admin
// conflict queue for manual review.
func Parse(data []byte) (*meetfiles.ParseResult, error) {
	// Real-world CL2 files (USA Swimming / Hy-Tek results exports) use the SDIF
	// v3 record layout (A0/B1/C1/D0/...), the same one SD3 uses. Hand those to
	// the SDIF parser and only fall back to the scaffold for anything else.
	if looksLikeSDIF(data) {
		parsed, err := sdif.Parse(data)
		if err != nil {
			return nil, err
		}
		parsed.FileType = meetfiles.FileTypeCL2
		return parsed, nil
	}

	result := &meetfiles.ParseResult{
		FileType: meetfiles.FileTypeCL2,
	}

	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		upper := strings.ToUpper(strings.TrimSpace(line))
		if upper == "" {
			continue
		}

		switch {
		case strings.HasPrefix(upper, "EVENT"), strings.HasPrefix(upper, "HEAT"), strings.HasPrefix(upper, "CL2"):
			// Recognised but not yet decoded: a full implementation would
			// extract event/heat/lane/swimmer/time from fixed-width fields,
			// which needs an actual CL2 spec.

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

// truncate returns the first n characters (runes) of s, appending "…" when truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// looksLikeSDIF reports whether the first non-blank line starts with the SDIF
// file-description record ID "A0" (optionally after a UTF-8 BOM).
func looksLikeSDIF(data []byte) bool {
	data = bytes.TrimLeft(bytes.TrimPrefix(data, utf8BOM), " \t\r\n")
	return len(data) >= 2 && data[0] == 'A' && data[1] == '0'
}
