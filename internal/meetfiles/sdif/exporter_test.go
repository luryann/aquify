package sdif_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/meetfiles/sdif"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func ptr(t time.Time) *time.Time { return &t }

func testMeetStart() *time.Time {
	t := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	return &t
}

func testMeetEnd() *time.Time {
	t := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	return &t
}

func testDOB() *time.Time {
	t := time.Date(1999, 2, 15, 0, 0, 0, 0, time.UTC)
	return &t
}

// validTime returns a meetfiles.Time with valid seconds.
func validTime(seconds float64) meetfiles.Time {
	return meetfiles.Time{Valid: true, Seconds: seconds}
}

func baseInput() sdif.ExportInput {
	return sdif.ExportInput{
		MeetName:      "Spring Invitational",
		MeetStartDate: testMeetStart(),
		MeetEndDate:   testMeetEnd(),
		Course:        meetfiles.CourseShortCourseYards,
		HostTeamCode:  "DARE",
		HostTeamName:  "DARE Aquatics",
		IndividualEntries: []sdif.ExportIndividualEntry{
			{
				Swimmer: sdif.ExportSwimmer{
					LastName:  "Smith",
					FirstName: "John",
					USSNumber: "123456789012",
					DOB:       testDOB(),
					Gender:    "M",
					TeamCode:  "DARE",
				},
				EventNumber: "1",
				Distance:    100,
				Stroke:      meetfiles.StrokeFreestyle,
				EventGender: "M",
				AgeGroup:    "1824",
				Course:      meetfiles.CourseShortCourseYards,
				SeedTime:    validTime(59.10),
			},
			{
				Swimmer: sdif.ExportSwimmer{
					LastName:  "Doe",
					FirstName: "Jane",
					DOB:       ptr(time.Date(2008, 5, 10, 0, 0, 0, 0, time.UTC)),
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "2",
				Distance:    200,
				Stroke:      meetfiles.StrokeBackstroke,
				EventGender: "F",
				AgeGroup:    "1517",
				Course:      meetfiles.CourseShortCourseYards,
				SeedTime:    validTime(135.00), // 2:15.00
			},
		},
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return normalizeGoldenLineEndings(string(raw))
}

func normalizeGoldenLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

// ─── Standard SD3 export ──────────────────────────────────────────────────────

func TestExportStandardSD3_Parseable(t *testing.T) {
	data, err := sdif.ExportStandardSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportStandardSD3: %v", err)
	}

	result, parseErr := sdif.Parse(data)
	if parseErr != nil {
		t.Fatalf("Parse re-exported standard SD3: %v", parseErr)
	}

	if len(result.ParseErrors) != 0 {
		t.Errorf("got %d parse errors; want 0: %v", len(result.ParseErrors), result.ParseErrors)
	}
	if len(result.IndividualResults) != 2 {
		t.Errorf("got %d individual results; want 2", len(result.IndividualResults))
	}
	if result.MeetName != "Spring Invitational" {
		t.Errorf("MeetName = %q; want %q", result.MeetName, "Spring Invitational")
	}
	if result.Course != meetfiles.CourseShortCourseYards {
		t.Errorf("Course = %v; want SCY", result.Course)
	}
}

func TestExportStandardSD3_GoldenFile(t *testing.T) {
	data, err := sdif.ExportStandardSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportStandardSD3: %v", err)
	}

	got := normalizeGoldenLineEndings(string(data))
	want := readGolden(t, "standard_sd3.golden")
	if got != want {
		t.Fatalf("standard SD3 output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestExportStandardSD3_NoSeedTimes(t *testing.T) {
	data, err := sdif.ExportStandardSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportStandardSD3: %v", err)
	}

	// Seed time field at 0-indexed [88:96] of each D0 line (col 89–96 in 1-indexed).
	// Standard export must have blank or NT there.
	for i, line := range strings.Split(string(data), "\r\n") {
		if len(line) < 2 || line[0:2] != "D0" {
			continue
		}
		if len(line) < 96 {
			t.Errorf("line %d: D0 too short (%d chars)", i+1, len(line))
			continue
		}
		seedField := strings.TrimSpace(line[88:96])
		if seedField != "" && seedField != "NT" {
			t.Errorf("standard export: D0 line %d has seed time %q; want blank or NT", i+1, seedField)
		}
	}
}

// ─── Extended SD3 export ──────────────────────────────────────────────────────

func TestExportExtendedSD3_Parseable(t *testing.T) {
	data, err := sdif.ExportExtendedSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}

	result, parseErr := sdif.Parse(data)
	if parseErr != nil {
		t.Fatalf("Parse re-exported extended SD3: %v", parseErr)
	}

	if len(result.ParseErrors) != 0 {
		t.Errorf("got %d parse errors; want 0: %v", len(result.ParseErrors), result.ParseErrors)
	}
	if len(result.IndividualResults) != 2 {
		t.Errorf("got %d individual results; want 2", len(result.IndividualResults))
	}
}

func TestExportExtendedSD3_GoldenFile(t *testing.T) {
	data, err := sdif.ExportExtendedSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}

	got := normalizeGoldenLineEndings(string(data))
	want := readGolden(t, "extended_sd3.golden")
	if got != want {
		t.Fatalf("extended SD3 output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestExportExtendedSD3_HasSeedTimes(t *testing.T) {
	data, err := sdif.ExportExtendedSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}

	result, parseErr := sdif.Parse(data)
	if parseErr != nil {
		t.Fatalf("Parse: %v", parseErr)
	}

	for _, r := range result.IndividualResults {
		if !r.SeedTime.Valid {
			t.Errorf("swimmer %s %s: expected valid seed time in extended export, got %q",
				r.Swimmer.FirstName, r.Swimmer.LastName, r.SeedTime.Raw)
		}
	}
}

// ─── Relay entries ────────────────────────────────────────────────────────────

func TestExportExtendedSD3_WithRelays(t *testing.T) {
	input := baseInput()
	input.RelayEntries = []sdif.ExportRelayEntry{
		{
			TeamCode:    "DARE",
			EventNumber: "10",
			Distance:    200,
			Stroke:      meetfiles.StrokeMedleyRelay,
			EventGender: "X",
			AgeGroup:    "OPEN",
			Course:      meetfiles.CourseShortCourseYards,
			Legs: []sdif.ExportRelayLeg{
				{
					Swimmer: sdif.ExportSwimmer{
						LastName: "Smith", FirstName: "John",
						DOB: testDOB(), Gender: "M", TeamCode: "DARE",
					},
					LegOrder: 1,
					LegTime:  validTime(58.12),
				},
				{
					Swimmer: sdif.ExportSwimmer{
						LastName: "Doe", FirstName: "Jane",
						DOB:    ptr(time.Date(2008, 5, 10, 0, 0, 0, 0, time.UTC)),
						Gender: "F", TeamCode: "DARE",
					},
					LegOrder: 2,
					LegTime:  validTime(32.01),
				},
			},
		},
	}

	data, err := sdif.ExportExtendedSD3(input)
	if err != nil {
		t.Fatalf("ExportExtendedSD3 with relay: %v", err)
	}

	result, parseErr := sdif.Parse(data)
	if parseErr != nil {
		t.Fatalf("Parse: %v", parseErr)
	}

	if len(result.RelayResults) != 1 {
		t.Errorf("got %d relay results; want 1", len(result.RelayResults))
	}
	if len(result.RelayResults) > 0 && len(result.RelayResults[0].Legs) != 2 {
		t.Errorf("got %d relay legs; want 2", len(result.RelayResults[0].Legs))
	}
}

// ─── SDIF roster round-trip ───────────────────────────────────────────────────

func TestSDIFRoster_RoundTrip(t *testing.T) {
	dob1 := time.Date(1999, 2, 15, 0, 0, 0, 0, time.UTC)
	dob2 := time.Date(2008, 5, 10, 0, 0, 0, 0, time.UTC)

	input := sdif.RosterExportInput{
		MeetName:  "Spring Invitational",
		StartDate: testMeetStart(),
		EndDate:   testMeetEnd(),
		Course:    meetfiles.CourseShortCourseYards,
		TeamCode:  "DARE",
		TeamName:  "DARE Aquatics",
		Swimmers: []sdif.RosterExportSwimmer{
			{
				LastName: "Smith", FirstName: "John",
				USSNumber: "123456789012",
				DOB:       &dob1, Gender: "M",
				TeamCode: "DARE", TeamName: "DARE Aquatics",
			},
			{
				LastName: "Doe", FirstName: "Jane",
				USSNumber: "987654321098",
				DOB:       &dob2, Gender: "F",
				TeamCode: "DARE", TeamName: "DARE Aquatics",
			},
		},
	}

	// Export to SDIF roster
	data, err := sdif.ExportSDIFRoster(input)
	if err != nil {
		t.Fatalf("ExportSDIFRoster: %v", err)
	}

	// Verify expected record types
	var recordTypes []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if len(line) >= 2 {
			recordTypes = append(recordTypes, line[0:2])
		}
	}
	wantTypes := []string{"A0", "B1", "C1", "D1", "D1", "Z0"}
	if len(recordTypes) != len(wantTypes) {
		t.Errorf("record count: got %d (%v); want %d (%v)",
			len(recordTypes), recordTypes, len(wantTypes), wantTypes)
	}

	// Re-import and verify round-trip fidelity
	result, err := sdif.ImportSDIFRoster(data)
	if err != nil {
		t.Fatalf("ImportSDIFRoster: %v", err)
	}
	if len(result.ParseErrors) != 0 {
		t.Errorf("ImportSDIFRoster: got %d parse errors: %v", len(result.ParseErrors), result.ParseErrors)
	}
	if len(result.Swimmers) != 2 {
		t.Fatalf("ImportSDIFRoster: got %d swimmers; want 2", len(result.Swimmers))
	}

	for i, sw := range result.Swimmers {
		orig := input.Swimmers[i]
		if sw.LastName != orig.LastName {
			t.Errorf("[%d] LastName = %q; want %q", i, sw.LastName, orig.LastName)
		}
		if sw.FirstName != orig.FirstName {
			t.Errorf("[%d] FirstName = %q; want %q", i, sw.FirstName, orig.FirstName)
		}
		if sw.USSNumber != orig.USSNumber {
			t.Errorf("[%d] USSNumber = %q; want %q", i, sw.USSNumber, orig.USSNumber)
		}
		if sw.Gender != orig.Gender {
			t.Errorf("[%d] Gender = %q; want %q", i, sw.Gender, orig.Gender)
		}
		if sw.DOB == nil {
			t.Errorf("[%d] DOB = nil; want non-nil", i)
		} else if !sw.DOB.Equal(*orig.DOB) {
			t.Errorf("[%d] DOB = %v; want %v", i, sw.DOB, orig.DOB)
		}
		if sw.TeamCode != orig.TeamCode {
			t.Errorf("[%d] TeamCode = %q; want %q", i, sw.TeamCode, orig.TeamCode)
		}
	}
}

// ─── Validation ───────────────────────────────────────────────────────────────

func TestExportStandardSD3_EmptyMeetName(t *testing.T) {
	input := baseInput()
	input.MeetName = ""

	_, err := sdif.ExportStandardSD3(input)
	if err == nil {
		t.Fatal("expected validation error for empty MeetName; got nil")
	}

	var ve sdif.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError; got %T: %v", err, err)
	}
}

func TestExportSDIFRoster_EmptyMeetName(t *testing.T) {
	_, err := sdif.ExportSDIFRoster(sdif.RosterExportInput{MeetName: ""})
	if err == nil {
		t.Fatal("expected validation error; got nil")
	}
}

// ─── Line ordering ────────────────────────────────────────────────────────────

func TestExportLineOrdering_EventThenName(t *testing.T) {
	input := baseInput()
	// Add a third entry with event 1 and last name "Adams" (sorts before Smith)
	input.IndividualEntries = append(input.IndividualEntries, sdif.ExportIndividualEntry{
		Swimmer: sdif.ExportSwimmer{
			LastName: "Adams", FirstName: "Alice",
			DOB:    ptr(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			Gender: "F", TeamCode: "DARE",
		},
		EventNumber: "1",
		Distance:    100,
		Stroke:      meetfiles.StrokeFreestyle,
		EventGender: "F",
		AgeGroup:    "1824",
		Course:      meetfiles.CourseShortCourseYards,
	})

	data, err := sdif.ExportStandardSD3(input)
	if err != nil {
		t.Fatalf("ExportStandardSD3: %v", err)
	}

	var d0Names []string
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) >= 2 && line[0:2] == "D0" && len(line) >= 39 {
			d0Names = append(d0Names, strings.TrimSpace(line[11:39]))
		}
	}

	if len(d0Names) < 3 {
		t.Fatalf("got %d D0 lines; want 3: %v", len(d0Names), d0Names)
	}
	// Adams (event 1) should come before Smith (event 1) due to name sort
	if !strings.HasPrefix(d0Names[0], "Adams") {
		t.Errorf("first D0 = %q; want Adams (sorted event 1, name ascending)", d0Names[0])
	}
}

// ─── Record length invariant ──────────────────────────────────────────────────

func TestExportAllLinesExactly160DataChars(t *testing.T) {
	data, err := sdif.ExportExtendedSD3(baseInput())
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}

	// Split on CRLF; each line should be exactly 160 data bytes.
	for i, line := range strings.Split(string(data), "\r\n") {
		if line == "" {
			continue // trailing after final CRLF
		}
		if len(line) != 160 {
			t.Errorf("line %d: length = %d; want 160 data chars", i+1, len(line))
		}
	}
}

// ─── Time encoding edge cases ─────────────────────────────────────────────────

func TestEncodeTime_SubMinute(t *testing.T) {
	input := baseInput()
	input.IndividualEntries = []sdif.ExportIndividualEntry{
		{
			Swimmer:     sdif.ExportSwimmer{LastName: "Smith", Gender: "M"},
			EventNumber: "1", Distance: 100,
			Stroke: meetfiles.StrokeFreestyle, EventGender: "M",
			Course:   meetfiles.CourseShortCourseYards,
			SeedTime: validTime(59.10),
		},
	}
	data, err := sdif.ExportExtendedSD3(input)
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) >= 97 && line[0:2] == "D0" {
			seed := line[88:96]
			if seed[0] != ' ' || seed[1] != ' ' {
				t.Errorf("sub-minute time: seed field = %q; want leading spaces", seed)
			}
		}
	}
}

func TestEncodeTime_MultiMinute(t *testing.T) {
	// 2:15.00 = 135 seconds
	input := baseInput()
	input.IndividualEntries = []sdif.ExportIndividualEntry{
		{
			Swimmer:     sdif.ExportSwimmer{LastName: "Doe", Gender: "F"},
			EventNumber: "2", Distance: 200,
			Stroke: meetfiles.StrokeBackstroke, EventGender: "F",
			Course:   meetfiles.CourseShortCourseYards,
			SeedTime: validTime(135.0),
		},
	}
	data, err := sdif.ExportExtendedSD3(input)
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) >= 97 && line[0:2] == "D0" {
			seed := strings.TrimRight(line[88:96], " ")
			if !strings.Contains(seed, ":") {
				t.Errorf("multi-minute time: seed field = %q; expected colon separator", seed)
			}
		}
	}
}

func TestEncodeTime_NT(t *testing.T) {
	input := baseInput()
	input.IndividualEntries = []sdif.ExportIndividualEntry{
		{
			Swimmer:     sdif.ExportSwimmer{LastName: "Smith", Gender: "M"},
			EventNumber: "1", Distance: 100,
			Stroke: meetfiles.StrokeFreestyle, EventGender: "M",
			Course:   meetfiles.CourseShortCourseYards,
			SeedTime: meetfiles.Time{Raw: "NT", Valid: false},
		},
	}
	data, err := sdif.ExportExtendedSD3(input)
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) >= 97 && line[0:2] == "D0" {
			seed := strings.TrimSpace(line[88:96])
			if seed != "NT" {
				t.Errorf("NT time: seed field trimmed = %q; want NT", seed)
			}
		}
	}
}
