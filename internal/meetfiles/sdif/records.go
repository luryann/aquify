// Package sdif implements parsing of SDIF v3 (SD3) files.
//
// Reference: SDIF Version 3 specification (US Masters Swimming, 28 April 1998).
// Every record is exactly 160 bytes of data followed by CR+LF (162 bytes).
// Column positions below are 1-indexed as in the spec; Go slice expressions
// subtract 1 from the start (e.g. col 12, len 28 → line[11:39]).
package sdif

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dareaquatics/aquify/meetfiles"
)

// ─── Record ID constants ──────────────────────────────────────────────────────

const (
	RecordA0 = "A0" // File Description
	RecordB1 = "B1" // Meet
	RecordB2 = "B2" // Meet Host
	RecordC1 = "C1" // Team ID
	RecordC2 = "C2" // Team Entry
	RecordD0 = "D0" // Individual Event (main result record)
	RecordD1 = "D1" // Individual Administrative
	RecordD2 = "D2" // Individual Contact
	RecordD3 = "D3" // Individual Information
	RecordE0 = "E0" // Relay Event
	RecordF0 = "F0" // Relay Name (one per swimmer)
	RecordG0 = "G0" // Splits
	RecordZ0 = "Z0" // File Terminator
)

// minLineLen is the minimum data length we require before trying to parse a
// record.  Short lines are treated as malformed.
const minLineLen = 40

// ─── Field extractors ─────────────────────────────────────────────────────────

// field returns a substring from a padded line using 1-indexed start and length
// as specified in the SDIF spec, trimmed of leading/trailing whitespace.
// If the line is too short the missing portion is treated as blank.
func field(line string, start, length int) string {
	begin := start - 1
	end := begin + length
	if begin >= len(line) {
		return ""
	}
	if end > len(line) {
		end = len(line)
	}
	// Columns are byte offsets; keep the slice on rune boundaries so a
	// multi-byte name that straddles a field edge isn't split into invalid
	// UTF-8.
	for begin < end && !utf8.RuneStart(line[begin]) {
		begin++
	}
	if end < len(line) {
		for end > begin && !utf8.RuneStart(line[end]) {
			end--
		}
	}
	// A rune cut by the record itself (e.g. a writer that truncated bytes)
	// is dropped rather than propagated as invalid UTF-8.
	return strings.TrimSpace(strings.ToValidUTF8(line[begin:end], ""))
}

// ─── Code table decoders ─────────────────────────────────────────────────────

// decodeCourse maps SDIF course codes to the canonical Course type.
func decodeCourse(code string) meetfiles.Course {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "1", "S":
		return meetfiles.CourseShortCourseMeters
	case "2", "Y":
		return meetfiles.CourseShortCourseYards
	case "3", "L":
		return meetfiles.CourseLongCourseMeters
	default:
		return meetfiles.CourseUnknown
	}
}

// decodeStroke maps SDIF stroke codes (1–7) to the canonical Stroke type.
func decodeStroke(code string) meetfiles.Stroke {
	switch strings.TrimSpace(code) {
	case "1":
		return meetfiles.StrokeFreestyle
	case "2":
		return meetfiles.StrokeBackstroke
	case "3":
		return meetfiles.StrokeBreaststroke
	case "4":
		return meetfiles.StrokeButterfly
	case "5":
		return meetfiles.StrokeIM
	case "6":
		return meetfiles.StrokeFreestyleRelay
	case "7":
		return meetfiles.StrokeMedleyRelay
	default:
		return meetfiles.StrokeUnknown
	}
}

// ─── Date parser ─────────────────────────────────────────────────────────────

// parseDate parses an SDIF MMDDYYYY date string.
// Returns nil when the input is blank or unparseable.
func parseDate(raw string) *time.Time {
	s := strings.TrimSpace(raw)
	if len(s) != 8 {
		return nil
	}
	t, err := time.Parse("01022006", s)
	if err != nil {
		return nil
	}
	t = t.UTC() // date-only values are UTC midnight, matching the Hytek parser
	return &t
}

// ─── Time parser ─────────────────────────────────────────────────────────────

// specialTimeCodes enumerates SDIF special time values that are not numeric.
var specialTimeCodes = map[string]bool{
	"NT":  true,
	"NS":  true,
	"DNF": true,
	"DQ":  true,
	"SCR": true,
}

// parseTime parses an 8-character SDIF TIME field (mm:ss.hh) or a special
// code (NT / NS / DNF / DQ / SCR).
func parseTime(raw string) meetfiles.Time {
	s := strings.TrimSpace(raw)
	if s == "" {
		return meetfiles.Time{Raw: ""}
	}
	if specialTimeCodes[s] {
		return meetfiles.Time{Raw: s, Valid: false}
	}

	// Expect mm:ss.hh  (total 8 chars, but may be shorter for sub-minute times).
	secs, err := parseTimeString(s)
	if err != nil {
		return meetfiles.Time{Raw: s, Valid: false}
	}
	return meetfiles.Time{Raw: s, Valid: true, Seconds: secs}
}

// parseTimeString converts "mm:ss.hh" or "ss.hh" to total seconds.  Only
// digits, one optional minutes separator and a decimal point are accepted;
// strconv.ParseFloat alone would also take "NaN", "Inf", "-5" and "1e3".
func parseTimeString(s string) (float64, error) {
	var minutes int
	if idx := strings.Index(s, ":"); idx >= 0 {
		m, err := strconv.Atoi(strings.TrimSpace(s[:idx]))
		if err != nil || m < 0 || strings.ContainsAny(s[:idx], "+-") {
			return 0, fmt.Errorf("invalid minutes in %q", s)
		}
		minutes = m
		s = s[idx+1:]
	}
	s = strings.TrimSpace(s)
	if s == "" || strings.Trim(s, "0123456789.") != "" {
		return 0, fmt.Errorf("invalid seconds in %q", s)
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in %q: %w", s, err)
	}
	return float64(minutes)*60 + sec, nil
}

// ─── Name parser ─────────────────────────────────────────────────────────────

// parseName splits an SDIF NAME field ("Last, First M") into last and first.
func parseName(raw string) (last, first string) {
	s := strings.TrimSpace(raw)
	idx := strings.Index(s, ",")
	if idx < 0 {
		return s, ""
	}
	last = strings.TrimSpace(s[:idx])
	// remainder is "First M"; the middle initial is kept as part of first name.
	first = strings.TrimSpace(s[idx+1:])
	return last, first
}

// ─── Integer field parser ────────────────────────────────────────────────────

// parseInt parses a right-justified integer field, returning 0 on blank/error.
func parseInt(raw string) int {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// ─── D0 individual result ────────────────────────────────────────────────────

// ParseD0 extracts a ParsedIndividualResult from a D0 data line.
// line should be the full 160-char data portion (no CRLF).
func ParseD0(line string, lineNumber int) (meetfiles.ParsedIndividualResult, error) {
	if len(line) < minLineLen {
		return meetfiles.ParsedIndividualResult{},
			fmt.Errorf("D0 line %d too short (%d chars)", lineNumber, len(line))
	}

	lastName, firstName := parseName(field(line, 12, 28))
	dob := parseDate(field(line, 56, 8))
	gender := field(line, 66, 1)
	eventGender := field(line, 67, 1)
	distance := parseInt(field(line, 68, 4))
	strokeCode := field(line, 72, 1)
	eventNumber := field(line, 73, 4)
	ageGroup := field(line, 77, 4)
	seedTimeRaw := field(line, 89, 8)
	seedCourseCode := field(line, 97, 1)
	finalsTimeRaw := field(line, 116, 8)
	finalsCourseCode := field(line, 124, 1)
	finalsHeat := parseInt(field(line, 129, 2))
	finalsLane := parseInt(field(line, 131, 2))
	finalsPlace := parseInt(field(line, 136, 3))
	ussNum := field(line, 40, 12)

	// Prefer finals course; fall back to seed course.
	courseCode := finalsCourseCode
	if courseCode == "" {
		courseCode = seedCourseCode
	}

	result := meetfiles.ParsedIndividualResult{
		LineNumber: lineNumber,
		Swimmer: meetfiles.ParsedSwimmer{
			LastName:  lastName,
			FirstName: firstName,
			DOB:       dob,
			Gender:    gender,
			USSNumber: ussNum,
		},
		Event: meetfiles.ParsedEvent{
			EventNumber: eventNumber,
			Distance:    distance,
			Stroke:      decodeStroke(strokeCode),
			EventGender: eventGender,
			AgeGroup:    ageGroup,
			Course:      decodeCourse(courseCode),
		},
		SeedTime:   parseTime(seedTimeRaw),
		FinalsTime: parseTime(finalsTimeRaw),
		Heat:       finalsHeat,
		Lane:       finalsLane,
		Place:      finalsPlace,
	}
	return result, nil
}

// ─── B1 meet record ───────────────────────────────────────────────────────────

// MeetInfo holds fields extracted from a B1 record.
type MeetInfo struct {
	Name      string
	StartDate *time.Time
	EndDate   *time.Time
	Course    meetfiles.Course
}

// ParseB1 extracts meet information from a B1 data line.
func ParseB1(line string) MeetInfo {
	return MeetInfo{
		Name:      field(line, 12, 30),
		StartDate: parseDate(field(line, 122, 8)),
		EndDate:   parseDate(field(line, 130, 8)),
		Course:    decodeCourse(field(line, 150, 1)),
	}
}

// ─── E0 relay event record ───────────────────────────────────────────────────

// ParseE0 extracts relay event header fields from an E0 data line.
// The returned ParsedRelayResult has no legs populated yet.
func ParseE0(line string, lineNumber int) meetfiles.ParsedRelayResult {
	eventGender := field(line, 21, 1)
	distance := parseInt(field(line, 22, 4))
	strokeCode := field(line, 26, 1)
	eventNumber := field(line, 27, 4)
	ageGroup := field(line, 31, 4)
	seedTimeRaw := field(line, 46, 8)
	seedCourseCode := field(line, 54, 1)
	finalsTimeRaw := field(line, 73, 8)
	finalsCourseCode := field(line, 81, 1)
	teamCode := field(line, 13, 6)
	courseCode := finalsCourseCode
	if courseCode == "" {
		courseCode = seedCourseCode
	}

	return meetfiles.ParsedRelayResult{
		LineNumber: lineNumber,
		TeamCode:   teamCode,
		Event: meetfiles.ParsedEvent{
			EventNumber: eventNumber,
			Distance:    distance,
			Stroke:      decodeStroke(strokeCode),
			EventGender: eventGender,
			AgeGroup:    ageGroup,
			Course:      decodeCourse(courseCode),
		},
		SeedTime:   parseTime(seedTimeRaw),
		FinalsTime: parseTime(finalsTimeRaw),
	}
}

// ─── F0 relay leg record ─────────────────────────────────────────────────────

// ParseF0 extracts a relay leg from an F0 data line.
func ParseF0(line string, lineNumber int) meetfiles.ParsedRelayLeg {
	lastName, firstName := parseName(field(line, 23, 28))
	ussNum := field(line, 51, 12)
	dob := parseDate(field(line, 66, 8))
	gender := field(line, 76, 1)
	legOrder := parseInt(field(line, 79, 1)) // finals leg position
	legTimeRaw := field(line, 80, 8)

	return meetfiles.ParsedRelayLeg{
		LineNumber: lineNumber,
		Swimmer: meetfiles.ParsedSwimmer{
			LastName:  lastName,
			FirstName: firstName,
			DOB:       dob,
			Gender:    gender,
			USSNumber: ussNum,
		},
		LegOrder: legOrder,
		LegTime:  parseTime(legTimeRaw),
	}
}
