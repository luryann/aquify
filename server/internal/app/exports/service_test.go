package exports

import (
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/meetfiles"
	sdifexport "github.com/dareaquatics/aquify/meetfiles/sdif"
)

func TestBuildFileContent_MatchesCanonicalStandardSD3(t *testing.T) {
	t.Parallel()

	input := testGenerateExportInput()
	got, filename, err := buildFileContent(input)
	if err != nil {
		t.Fatalf("buildFileContent: %v", err)
	}
	if filename != "spring-invitational_standard.sd3" {
		t.Fatalf("filename = %q, want %q", filename, "spring-invitational_standard.sd3")
	}

	want, err := sdifexport.ExportStandardSD3(testCanonicalExportInput())
	if err != nil {
		t.Fatalf("ExportStandardSD3: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("standard export mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	lines := strings.Split(string(got), "\r\n")
	var eventLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "D0") {
			eventLines = append(eventLines, line)
		}
	}
	if len(eventLines) != 3 {
		t.Fatalf("got %d D0 lines, want 3", len(eventLines))
	}
	if eventNumberField(eventLines[0]) != "2" || eventNumberField(eventLines[1]) != "10" {
		t.Fatalf("event ordering mismatch: got %q then %q", eventNumberField(eventLines[0]), eventNumberField(eventLines[1]))
	}
}

func TestBuildFileContent_MatchesCanonicalExtendedSD3(t *testing.T) {
	t.Parallel()

	input := testGenerateExportInput()
	input.ExportType = ExportTypeExtendedSD3

	got, filename, err := buildFileContent(input)
	if err != nil {
		t.Fatalf("buildFileContent: %v", err)
	}
	if filename != "spring-invitational_extended.sd3" {
		t.Fatalf("filename = %q, want %q", filename, "spring-invitational_extended.sd3")
	}

	want, err := sdifexport.ExportExtendedSD3(testCanonicalExportInput())
	if err != nil {
		t.Fatalf("ExportExtendedSD3: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("extended export mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildFileContent_MatchesCanonicalRoster(t *testing.T) {
	t.Parallel()

	input := testGenerateExportInput()
	input.ExportType = ExportTypeSDIFRoster

	got, filename, err := buildFileContent(input)
	if err != nil {
		t.Fatalf("buildFileContent: %v", err)
	}
	if filename != "spring-invitational_roster.sd3" {
		t.Fatalf("filename = %q, want %q", filename, "spring-invitational_roster.sd3")
	}

	want, err := sdifexport.ExportSDIFRoster(testCanonicalRosterInput())
	if err != nil {
		t.Fatalf("ExportSDIFRoster: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("roster export mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if !strings.Contains(string(got), "D1") {
		t.Fatalf("expected canonical roster D1 records, got %q", string(got))
	}
}

func eventNumberField(line string) string {
	if len(line) < 76 {
		return ""
	}
	return strings.TrimSpace(line[72:76])
}

func testGenerateExportInput() GenerateExportInput {
	return GenerateExportInput{
		ExportType:    ExportTypeStandardSD3,
		MeetName:      "Spring Invitational",
		MeetStartDate: "03152026",
		MeetEndDate:   "03162026",
		Course:        "SCY",
		HostTeamCode:  "DARE",
		HostTeamName:  "DARE Aquatics",
		Entries: []ExportEntry{
			{
				Swimmer: ExportSwimmer{
					LastName:  "Zimmer",
					FirstName: "Zoe",
					USSNumber: "555555555555",
					DOBRaw:    "02152009",
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "10",
				Distance:    100,
				Stroke:      "back",
				EventGender: "F",
				AgeGroup:    "1517",
				Course:      "SCY",
				SeedTimeMs:  70250,
				Heat:        2,
				Lane:        5,
			},
			{
				Swimmer: ExportSwimmer{
					LastName:  "Adams",
					FirstName: "Amy",
					USSNumber: "111111111111",
					DOBRaw:    "02151999",
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "2",
				Distance:    200,
				Stroke:      "free",
				EventGender: "F",
				AgeGroup:    "Open",
				Course:      "SCY",
				SeedTimeMs:  125340,
				Heat:        1,
				Lane:        3,
			},
			{
				Swimmer: ExportSwimmer{
					LastName:  "Adams",
					FirstName: "Amy",
					USSNumber: "111111111111",
					DOBRaw:    "02151999",
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "10",
				Distance:    100,
				Stroke:      "fly",
				EventGender: "F",
				AgeGroup:    "1517",
				Course:      "SCY",
				SeedTimeMs:  68890,
				Heat:        3,
				Lane:        4,
			},
		},
	}
}

func testCanonicalExportInput() sdifexport.ExportInput {
	start := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC)
	return sdifexport.ExportInput{
		MeetName:      "Spring Invitational",
		MeetStartDate: &start,
		MeetEndDate:   &end,
		Course:        meetfiles.CourseShortCourseYards,
		HostTeamCode:  "DARE",
		HostTeamName:  "DARE Aquatics",
		IndividualEntries: []sdifexport.ExportIndividualEntry{
			{
				Swimmer: sdifexport.ExportSwimmer{
					LastName:  "Zimmer",
					FirstName: "Zoe",
					USSNumber: "555555555555",
					DOB:       datePtr(2009, time.February, 15),
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "10",
				Distance:    100,
				Stroke:      meetfiles.StrokeBackstroke,
				EventGender: "F",
				AgeGroup:    "1517",
				Course:      meetfiles.CourseShortCourseYards,
				SeedTime:    meetfiles.Time{Valid: true, Seconds: 70.25},
				Heat:        2,
				Lane:        5,
			},
			{
				Swimmer: sdifexport.ExportSwimmer{
					LastName:  "Adams",
					FirstName: "Amy",
					USSNumber: "111111111111",
					DOB:       datePtr(1999, time.February, 15),
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "2",
				Distance:    200,
				Stroke:      meetfiles.StrokeFreestyle,
				EventGender: "F",
				AgeGroup:    "Open",
				Course:      meetfiles.CourseShortCourseYards,
				SeedTime:    meetfiles.Time{Valid: true, Seconds: 125.34},
				Heat:        1,
				Lane:        3,
			},
			{
				Swimmer: sdifexport.ExportSwimmer{
					LastName:  "Adams",
					FirstName: "Amy",
					USSNumber: "111111111111",
					DOB:       datePtr(1999, time.February, 15),
					Gender:    "F",
					TeamCode:  "DARE",
				},
				EventNumber: "10",
				Distance:    100,
				Stroke:      meetfiles.StrokeButterfly,
				EventGender: "F",
				AgeGroup:    "1517",
				Course:      meetfiles.CourseShortCourseYards,
				SeedTime:    meetfiles.Time{Valid: true, Seconds: 68.89},
				Heat:        3,
				Lane:        4,
			},
		},
	}
}

func testCanonicalRosterInput() sdifexport.RosterExportInput {
	start := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC)
	return sdifexport.RosterExportInput{
		MeetName:  "Spring Invitational",
		StartDate: &start,
		EndDate:   &end,
		Course:    meetfiles.CourseShortCourseYards,
		TeamCode:  "DARE",
		TeamName:  "DARE Aquatics",
		Swimmers: []sdifexport.RosterExportSwimmer{
			{
				LastName:  "Zimmer",
				FirstName: "Zoe",
				USSNumber: "555555555555",
				DOB:       datePtr(2009, time.February, 15),
				Gender:    "F",
				TeamCode:  "DARE",
				TeamName:  "DARE Aquatics",
			},
			{
				LastName:  "Adams",
				FirstName: "Amy",
				USSNumber: "111111111111",
				DOB:       datePtr(1999, time.February, 15),
				Gender:    "F",
				TeamCode:  "DARE",
				TeamName:  "DARE Aquatics",
			},
		},
	}
}

func datePtr(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}
