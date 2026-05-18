package sdif

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
)

// ─── Export variant ───────────────────────────────────────────────────────────

// ExportVariant selects which SD3 file variant to generate.
type ExportVariant string

const (
	// ExportVariantStandard generates Standard SD3 — required fields only;
	// seed times are omitted (D0 seed field set to "NT      ").
	ExportVariantStandard ExportVariant = "standard"
	// ExportVariantExtended generates Extended SD3 — includes seed times and
	// seed course in D0 records.
	ExportVariantExtended ExportVariant = "extended"
)

// ─── Export input types ───────────────────────────────────────────────────────

// ExportSwimmer holds swimmer identity fields required for SD3 export.
type ExportSwimmer struct {
	LastName  string
	FirstName string
	USSNumber string     // raw USAS registration number; may be blank
	DOB       *time.Time // nil = blank in output
	Gender    string     // "M" or "F"
	TeamCode  string
}

// ExportIndividualEntry represents one swimmer's entry in one individual event.
type ExportIndividualEntry struct {
	Swimmer     ExportSwimmer
	EventNumber string           // e.g. "1", "12", "12A"
	Distance    int              // in yards or meters
	Stroke      meetfiles.Stroke
	EventGender string           // "M", "F", or "X"
	AgeGroup    string           // e.g. "1517" or "" (blank is valid)
	Course      meetfiles.Course
	SeedTime    meetfiles.Time // zero value = NT
	Heat        int            // 0 = unassigned
	Lane        int            // 0 = unassigned
}

// ExportRelayEntry represents one team relay entry.
type ExportRelayEntry struct {
	TeamCode    string
	EventNumber string
	Distance    int
	Stroke      meetfiles.Stroke
	EventGender string           // "M", "F", or "X"
	AgeGroup    string
	Course      meetfiles.Course
	SeedTime    meetfiles.Time
	Legs        []ExportRelayLeg
}

// ExportRelayLeg is one swimmer's contribution to a relay entry.
type ExportRelayLeg struct {
	Swimmer  ExportSwimmer
	LegOrder int // 1-4; 0 = unassigned
	LegTime  meetfiles.Time
}

// ExportInput holds all data needed to write an SD3 entries file.
type ExportInput struct {
	MeetName          string
	MeetStartDate     *time.Time
	MeetEndDate       *time.Time
	Course            meetfiles.Course
	HostTeamCode      string
	HostTeamName      string
	IndividualEntries []ExportIndividualEntry
	RelayEntries      []ExportRelayEntry
}

// ValidationError is returned when required export fields are missing or invalid.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("export validation: %s: %s", e.Field, e.Message)
}

// ─── Public export functions ──────────────────────────────────────────────────

// ExportStandardSD3 generates a Standard SD3 entries file.
// Seed times are omitted from D0 records (set to "NT      ").
func ExportStandardSD3(input ExportInput) ([]byte, error) {
	return exportEntries(input, ExportVariantStandard)
}

// ExportExtendedSD3 generates an Extended SD3 entries file.
// Seed times and seed course are included in D0 records when available.
func ExportExtendedSD3(input ExportInput) ([]byte, error) {
	return exportEntries(input, ExportVariantExtended)
}

// ─── Roster export / import ───────────────────────────────────────────────────

// RosterExportSwimmer holds swimmer identity data for roster export.
type RosterExportSwimmer struct {
	LastName  string
	FirstName string
	USSNumber string
	DOB       *time.Time
	Gender    string // "M" or "F"
	TeamCode  string
	TeamName  string
}

// RosterExportInput holds all data needed to generate an SDIF roster file.
type RosterExportInput struct {
	MeetName  string
	StartDate *time.Time
	EndDate   *time.Time
	Course    meetfiles.Course
	TeamCode  string
	TeamName  string
	Swimmers  []RosterExportSwimmer
}

// RosterImportResult holds the output of parsing an SDIF roster file.
type RosterImportResult struct {
	Swimmers   []RosterExportSwimmer
	ParseErrors []meetfiles.ParseError
}

// ExportSDIFRoster generates an SDIF roster file (A0 file-type "5").
// The file contains A0, B1, C1, and D1 records (one D1 per swimmer).
func ExportSDIFRoster(input RosterExportInput) ([]byte, error) {
	if strings.TrimSpace(input.MeetName) == "" {
		return nil, ValidationError{Field: "MeetName", Message: "meet name is required"}
	}

	var sb strings.Builder

	// A0: file description (file type 5 = roster / SDIF record)
	sb.WriteString(buildA0("5"))

	// B1: meet info
	sb.WriteString(buildB1(ExportInput{
		MeetName:      input.MeetName,
		MeetStartDate: input.StartDate,
		MeetEndDate:   input.EndDate,
		Course:        input.Course,
	}))

	// C1: team
	sb.WriteString(buildC1(input.TeamCode, input.TeamName))

	// D1: one per swimmer
	for _, sw := range input.Swimmers {
		sb.WriteString(buildD1(sw))
	}

	// Z0: file terminator
	sb.WriteString(buildZ0())

	return []byte(sb.String()), nil
}

// ImportSDIFRoster parses an SDIF roster file (file type "5") and extracts
// swimmer identity from D1 records.  C1 records are used to populate team
// fields on each swimmer.
func ImportSDIFRoster(data []byte) (*RosterImportResult, error) {
	result := &RosterImportResult{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0

	var currentTeamCode, currentTeamName string

	for scanner.Scan() {
		lineNumber++
		raw := scanner.Text()
		line := strings.TrimRight(raw, "\r")

		if len(line) < 2 {
			continue
		}

		switch line[0:2] {
		case RecordC1:
			currentTeamCode = field(line, 12, 6)
			currentTeamName = field(line, 18, 30)

		case RecordD1:
			if len(line) < minLineLen {
				result.ParseErrors = append(result.ParseErrors, meetfiles.ParseError{
					LineNumber: lineNumber,
					Raw:        truncate(raw, 120),
					Code:       meetfiles.ErrCodeMalformedRecord,
					Message:    fmt.Sprintf("D1 line %d too short (%d chars)", lineNumber, len(line)),
				})
				continue
			}
			lastName, firstName := parseName(field(line, 12, 28))
			ussNumber := field(line, 40, 12)
			dob := parseDate(field(line, 56, 8))
			gender := field(line, 66, 1)

			// Team from the line itself (if populated) or fall back to last C1.
			tc := field(line, 3, 6) // some roster files embed team at col 3+
			if tc == "" {
				tc = currentTeamCode
			}

			result.Swimmers = append(result.Swimmers, RosterExportSwimmer{
				LastName:  lastName,
				FirstName: firstName,
				USSNumber: ussNumber,
				DOB:       dob,
				Gender:    gender,
				TeamCode:  currentTeamCode,
				TeamName:  currentTeamName,
			})
			_ = tc // suppress unused-variable warning; kept for clarity
		}
	}

	return result, scanner.Err()
}

// ─── Internal entry-file generator ───────────────────────────────────────────

func exportEntries(input ExportInput, variant ExportVariant) ([]byte, error) {
	if err := validateExportInput(input); err != nil {
		return nil, err
	}

	// Sort individual entries by event number then swimmer last name.
	sorted := make([]ExportIndividualEntry, len(input.IndividualEntries))
	copy(sorted, input.IndividualEntries)
	sort.Slice(sorted, func(i, j int) bool {
		ei := sorted[i].EventNumber
		ej := sorted[j].EventNumber
		if ei != ej {
			return eventNumberLess(ei, ej)
		}
		return sorted[i].Swimmer.LastName < sorted[j].Swimmer.LastName
	})

	// Sort relay entries by event number.
	sortedRelays := make([]ExportRelayEntry, len(input.RelayEntries))
	copy(sortedRelays, input.RelayEntries)
	sort.Slice(sortedRelays, func(i, j int) bool {
		return eventNumberLess(sortedRelays[i].EventNumber, sortedRelays[j].EventNumber)
	})

	var sb strings.Builder

	// A0: file description (file type 1 = entries)
	sb.WriteString(buildA0("1"))

	// B1: meet info
	sb.WriteString(buildB1(input))

	// C1: team (host team; in full implementation each distinct team gets a C1)
	if input.HostTeamCode != "" || input.HostTeamName != "" {
		sb.WriteString(buildC1(input.HostTeamCode, input.HostTeamName))
	}

	// D0: individual entries
	for _, entry := range sorted {
		sb.WriteString(buildD0(entry, variant))
	}

	// E0 + F0: relay entries
	for _, relay := range sortedRelays {
		sb.WriteString(buildE0(relay))
		// Sort legs by leg order
		legs := make([]ExportRelayLeg, len(relay.Legs))
		copy(legs, relay.Legs)
		sort.Slice(legs, func(i, j int) bool {
			return legs[i].LegOrder < legs[j].LegOrder
		})
		for _, leg := range legs {
			sb.WriteString(buildF0(relay, leg))
		}
	}

	// Z0: file terminator
	sb.WriteString(buildZ0())

	return []byte(sb.String()), nil
}

func validateExportInput(input ExportInput) error {
	if strings.TrimSpace(input.MeetName) == "" {
		return ValidationError{Field: "MeetName", Message: "meet name is required"}
	}
	return nil
}

// eventNumberLess provides ordering for event number strings.
// Pure numeric strings are compared numerically; otherwise lexicographic.
func eventNumberLess(a, b string) bool {
	var ai, bi int
	_, errA := fmt.Sscanf(strings.TrimSpace(a), "%d", &ai)
	_, errB := fmt.Sscanf(strings.TrimSpace(b), "%d", &bi)
	if errA == nil && errB == nil {
		return ai < bi
	}
	return a < b
}

// ─── Record buffer helpers ────────────────────────────────────────────────────

// newBuf returns a 160-byte buffer pre-filled with spaces with the given
// 2-char record type placed at cols 1-2 and org code "1" at col 3.
func newBuf(recordType string) []byte {
	buf := make([]byte, 160)
	for i := range buf {
		buf[i] = ' '
	}
	buf[0] = recordType[0]
	buf[1] = recordType[1]
	buf[2] = '1' // org code
	return buf
}

// place writes a left-justified, space-padded string at 1-indexed start for
// the given width.  Values longer than width are truncated silently.
func place(buf []byte, startOneBased, width int, value string) {
	begin := startOneBased - 1
	for i := 0; i < width; i++ {
		if begin+i >= len(buf) {
			break
		}
		if i < len(value) {
			buf[begin+i] = value[i]
		}
		// positions beyond len(value) already hold the space from newBuf
	}
}

// placeRight writes a right-justified, space-padded string at 1-indexed start.
// Values longer than width are truncated to the rightmost characters.
func placeRight(buf []byte, startOneBased, width int, value string) {
	begin := startOneBased - 1
	if len(value) > width {
		value = value[len(value)-width:]
	}
	pad := width - len(value)
	for i := 0; i < width; i++ {
		if begin+i >= len(buf) {
			break
		}
		if i < pad {
			// leave the space from newBuf
			_ = i
		} else {
			buf[begin+i] = value[i-pad]
		}
	}
}

// finalizeLine appends CRLF to the 160-byte buffer and returns the 162-byte
// line as a string.
func finalizeLine(buf []byte) string {
	return string(buf) + "\r\n"
}

// ─── Field encoders ───────────────────────────────────────────────────────────

// encodeDate formats a *time.Time as MMDDYYYY for SDIF date fields.
// Returns 8 spaces when t is nil.
func encodeDate(t *time.Time) string {
	if t == nil {
		return "        "
	}
	return t.Format("01022006")
}

// encodeCourse maps the canonical Course to its single-char SDIF code.
func encodeCourse(c meetfiles.Course) string {
	switch c {
	case meetfiles.CourseShortCourseYards:
		return "Y"
	case meetfiles.CourseShortCourseMeters:
		return "S"
	case meetfiles.CourseLongCourseMeters:
		return "L"
	default:
		return " "
	}
}

// encodeStroke maps the canonical Stroke to its single-char SDIF code.
func encodeStroke(s meetfiles.Stroke) string {
	switch s {
	case meetfiles.StrokeFreestyle:
		return "1"
	case meetfiles.StrokeBackstroke:
		return "2"
	case meetfiles.StrokeBreaststroke:
		return "3"
	case meetfiles.StrokeButterfly:
		return "4"
	case meetfiles.StrokeIM:
		return "5"
	case meetfiles.StrokeFreestyleRelay:
		return "6"
	case meetfiles.StrokeMedleyRelay:
		return "7"
	default:
		return " "
	}
}

// encodeTime formats a meetfiles.Time as an 8-char SDIF time field.
//
// Format rules (matching SDIF v3 convention observed in test fixtures):
//   - Sub-minute times:     "  ss.hh " (2 leading spaces + 5-char time + 1 trailing space)
//   - 1–9 minute times:     "m:ss.hh " (7-char time + 1 trailing space)
//   - 10+ minute times:     "mm:ss.hh" (exactly 8 chars, no trailing space)
//   - Special/blank codes:  left-justified, right-padded to 8 chars ("NT      ")
func encodeTime(t meetfiles.Time) string {
	if !t.Valid {
		raw := strings.TrimSpace(t.Raw)
		if raw == "" {
			raw = "NT"
		}
		return fmt.Sprintf("%-8s", raw)
	}

	totalHundredths := int(math.Round(t.Seconds * 100)) // convert decimal seconds to integer hundredths
	minutes := totalHundredths / 6000
	remaining := totalHundredths % 6000
	secs := remaining / 100
	hundredths := remaining % 100

	switch {
	case minutes >= 10:
		return fmt.Sprintf("%02d:%02d.%02d", minutes, secs, hundredths) // 8 chars exactly
	case minutes >= 1:
		return fmt.Sprintf("%d:%02d.%02d ", minutes, secs, hundredths) // 7 + trailing space
	default:
		return fmt.Sprintf("  %02d.%02d ", secs, hundredths) // 2 + 2 + 1 + 2 + 1
	}
}

// formatName returns an SDIF name string "Last, First" (max 28 chars).
func formatName(last, first string) string {
	if first == "" {
		return last
	}
	return last + ", " + first
}

// ─── Record builders ──────────────────────────────────────────────────────────

func buildA0(fileTypeCode string) string {
	buf := newBuf("A0")
	place(buf, 4, 1, fileTypeCode) // file type at col 4 (after org code at 3)
	// Software name, version, contact — left blank for interoperability
	return finalizeLine(buf)
}

func buildB1(input ExportInput) string {
	buf := newBuf("B1")
	place(buf, 12, 30, input.MeetName)
	place(buf, 122, 8, encodeDate(input.MeetStartDate))
	place(buf, 130, 8, encodeDate(input.MeetEndDate))
	place(buf, 150, 1, encodeCourse(input.Course))
	return finalizeLine(buf)
}

func buildC1(teamCode, teamName string) string {
	buf := newBuf("C1")
	place(buf, 12, 6, teamCode)
	place(buf, 18, 30, teamName)
	return finalizeLine(buf)
}

func buildD0(entry ExportIndividualEntry, variant ExportVariant) string {
	buf := newBuf("D0")

	// Swimmer identity
	place(buf, 12, 28, formatName(entry.Swimmer.LastName, entry.Swimmer.FirstName))
	place(buf, 40, 12, entry.Swimmer.USSNumber)
	place(buf, 52, 1, "A") // attach code (standard value)
	place(buf, 56, 8, encodeDate(entry.Swimmer.DOB))
	place(buf, 66, 1, entry.Swimmer.Gender)

	// Event fields
	place(buf, 67, 1, entry.EventGender)
	placeRight(buf, 68, 4, fmt.Sprintf("%d", entry.Distance))
	place(buf, 72, 1, encodeStroke(entry.Stroke))
	place(buf, 73, 4, entry.EventNumber)
	place(buf, 77, 4, entry.AgeGroup)

	// Seed time: in extended variant, always write the seed time field
	// (valid numeric times or special codes like "NT"); standard omits it.
	if variant == ExportVariantExtended {
		place(buf, 89, 8, encodeTime(entry.SeedTime))
		if entry.SeedTime.Valid {
			place(buf, 97, 1, encodeCourse(entry.Course))
		}
	}

	// Heat and lane (when assigned)
	if entry.Heat > 0 {
		placeRight(buf, 129, 2, fmt.Sprintf("%d", entry.Heat))
	}
	if entry.Lane > 0 {
		placeRight(buf, 131, 2, fmt.Sprintf("%d", entry.Lane))
	}

	return finalizeLine(buf)
}

func buildD1(sw RosterExportSwimmer) string {
	buf := newBuf("D1")
	place(buf, 12, 28, formatName(sw.LastName, sw.FirstName))
	place(buf, 40, 12, sw.USSNumber)
	place(buf, 52, 1, "A") // attach code
	place(buf, 56, 8, encodeDate(sw.DOB))
	place(buf, 66, 1, sw.Gender)
	return finalizeLine(buf)
}

func buildE0(relay ExportRelayEntry) string {
	buf := newBuf("E0")
	place(buf, 12, 1, "A")  // relay team char
	place(buf, 13, 6, relay.TeamCode)
	place(buf, 21, 1, relay.EventGender)
	placeRight(buf, 22, 4, fmt.Sprintf("%d", relay.Distance))
	place(buf, 26, 1, encodeStroke(relay.Stroke))
	place(buf, 27, 4, relay.EventNumber)
	place(buf, 31, 4, relay.AgeGroup)
	if relay.SeedTime.Valid {
		place(buf, 46, 8, encodeTime(relay.SeedTime))
	}
	return finalizeLine(buf)
}

func buildF0(relay ExportRelayEntry, leg ExportRelayLeg) string {
	buf := newBuf("F0")
	place(buf, 16, 6, relay.TeamCode)
	place(buf, 22, 1, "A") // relay team char
	place(buf, 23, 28, formatName(leg.Swimmer.LastName, leg.Swimmer.FirstName))
	place(buf, 51, 12, leg.Swimmer.USSNumber)
	place(buf, 66, 8, encodeDate(leg.Swimmer.DOB))
	place(buf, 76, 1, leg.Swimmer.Gender)
	if leg.LegOrder > 0 {
		place(buf, 79, 1, fmt.Sprintf("%d", leg.LegOrder)) // finals order
	}
	if leg.LegTime.Valid {
		place(buf, 80, 8, encodeTime(leg.LegTime))
	}
	return finalizeLine(buf)
}

func buildZ0() string {
	buf := newBuf("Z0")
	return finalizeLine(buf)
}
