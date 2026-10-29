package exports

import (
	"bytes"
	"strings"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-pdf/fpdf"
)

func intPtr(v int) *int { return &v }

func sampleReportInput(reportType, grouping string) ReportInput {
	return ReportInput{
		ReportType: reportType,
		Grouping:   grouping,
		Team:       ReportTeam{TeamCode: "DARE", TeamName: "DARE Aquatics", LSCCode: "PC"},
		Meet: ReportMeet{
			Name:      "Spring Invitational",
			StartDate: "2026-03-15",
			EndDate:   "2026-03-16",
			Course:    "SCY",
			Location:  "Home Pool",
		},
		Events: []ReportEvent{
			{MeetEventID: "e2", EventNumber: "2", SessionID: "s1", SessionName: "Day 1 Prelims", Stroke: "freestyle", Distance: 200, Course: "SCY", Gender: "F", MinAge: nil, MaxAge: nil},
			{MeetEventID: "e10", EventNumber: "10", SessionID: "s2", SessionName: "Day 1 Finals", Stroke: "backstroke", Distance: 100, Course: "SCY", Gender: "F", MinAge: intPtr(15), MaxAge: intPtr(17)},
		},
		Entries: []ReportEntry{
			{SwimmerFirst: "Zoe", SwimmerLast: "Zimmer", USASwimmingID: "AB12CD34EF56", Gender: "female", AgeAtMeet: 17, GroupCode: "SENIOR", MeetEventID: "e10", EventNumber: "10", SessionID: "s2", SessionName: "Day 1 Finals", Stroke: "backstroke", Distance: 100, Course: "SCY", SeedTimeMs: intPtr(70250), Eligible: true},
			{SwimmerFirst: "Amy", SwimmerLast: "Adams", USASwimmingID: "GH78IJ90KL12", Gender: "female", AgeAtMeet: 27, GroupCode: "SENIOR", MeetEventID: "e2", EventNumber: "2", SessionID: "s1", SessionName: "Day 1 Prelims", Stroke: "freestyle", Distance: 200, Course: "SCY", SeedTimeMs: intPtr(125340), Eligible: true},
			{SwimmerFirst: "Amy", SwimmerLast: "Adams", USASwimmingID: "GH78IJ90KL12", Gender: "female", AgeAtMeet: 27, GroupCode: "SENIOR", MeetEventID: "e10", EventNumber: "10", SessionID: "s2", SessionName: "Day 1 Finals", Stroke: "backstroke", Distance: 100, Course: "SCY", SeedTimeMs: nil, Eligible: false, EligibilityNote: "age 27 above event maximum 17"},
		},
	}
}

func assertPDF(t *testing.T, content []byte, filename, wantFilename string) {
	t.Helper()
	if filename != wantFilename {
		t.Fatalf("filename = %q, want %q", filename, wantFilename)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatalf("content does not look like a PDF: prefix %q", string(content[:min(8, len(content))]))
	}
	if len(content) < 500 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(content))
	}
}

func TestBuildReportPDF_EventReport(t *testing.T) {
	t.Parallel()
	content, filename, err := buildReportPDF(sampleReportInput(ReportTypeEvent, ""))
	if err != nil {
		t.Fatalf("buildReportPDF: %v", err)
	}
	assertPDF(t, content, filename, "spring-invitational_event_report.pdf")
}

func TestBuildReportPDF_EligibilityReport(t *testing.T) {
	t.Parallel()
	content, filename, err := buildReportPDF(sampleReportInput(ReportTypeEligibility, ""))
	if err != nil {
		t.Fatalf("buildReportPDF: %v", err)
	}
	assertPDF(t, content, filename, "spring-invitational_eligibility.pdf")
}

func TestBuildReportPDF_EntryReportGroupings(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		GroupingSession:      "spring-invitational_entries_by_session.pdf",
		GroupingGroup:        "spring-invitational_entries_by_group.pdf",
		GroupingAlphabetical: "spring-invitational_entries_by_alphabetical.pdf",
	}
	for grouping, wantFile := range cases {
		content, filename, err := buildReportPDF(sampleReportInput(ReportTypeEntry, grouping))
		if err != nil {
			t.Fatalf("grouping %q: %v", grouping, err)
		}
		assertPDF(t, content, filename, wantFile)
	}
	// Default grouping (empty) should fall back to alphabetical.
	_, filename, err := buildReportPDF(sampleReportInput(ReportTypeEntry, ""))
	if err != nil {
		t.Fatalf("default grouping: %v", err)
	}
	if filename != "spring-invitational_entries_by_alphabetical.pdf" {
		t.Fatalf("default grouping filename = %q", filename)
	}
}

func TestBuildReportPDF_RejectsUnknownType(t *testing.T) {
	t.Parallel()
	if _, _, err := buildReportPDF(sampleReportInput("bogus", "")); err == nil {
		t.Fatal("expected error for unknown report type")
	}
}

func TestBuildReportPDF_RequiresMeetName(t *testing.T) {
	t.Parallel()
	in := sampleReportInput(ReportTypeEvent, "")
	in.Meet.Name = "  "
	if _, _, err := buildReportPDF(in); err == nil {
		t.Fatal("expected error for blank meet name")
	}
}

func TestFormatSeedTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ms   *int
		want string
	}{
		{nil, "NT"},
		{intPtr(0), "NT"},
		{intPtr(31450), "31.45"},
		{intPtr(70250), "1:10.25"},
		{intPtr(125340), "2:05.34"},
	}
	for _, c := range cases {
		if got := formatSeedTime(c.ms); got != c.want {
			t.Errorf("formatSeedTime(%v) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestAgeAtMeet(t *testing.T) {
	t.Parallel()
	// Born 2009-02-15, meet starts 2026-03-15 -> 17 (birthday already passed).
	if got := AgeAtMeet("2009-02-15", "2026-03-15"); got != 17 {
		t.Errorf("AgeAtMeet after birthday = %d, want 17", got)
	}
	// Meet before birthday -> 16.
	if got := AgeAtMeet("2009-02-15", "2026-01-01"); got != 16 {
		t.Errorf("AgeAtMeet before birthday = %d, want 16", got)
	}
}

func TestEventNumKeyOrdering(t *testing.T) {
	t.Parallel()
	if eventNumKey("2") >= eventNumKey("10") {
		t.Fatal("event 2 should sort before event 10")
	}
	if eventNumKey("10A") >= eventNumKey("10B") {
		t.Fatal("10A should sort before 10B")
	}
}

func TestReportTitles(t *testing.T) {
	t.Parallel()
	for reportType, want := range map[string]string{
		ReportTypeEligibility: "Eligibility Report",
		ReportTypeEntry:       "Entry Report",
		ReportTypeEvent:       "Event Report",
	} {
		if got := reportTitle(reportType); !strings.Contains(got, want) {
			t.Errorf("reportTitle(%q) = %q, want to contain %q", reportType, got, want)
		}
	}
}

func TestPDFDocTranslatesUnicodeForCoreFonts(t *testing.T) {
	t.Parallel()

	doc := newPDFDoc(fpdf.New("P", "mm", "Letter", ""))
	if got := doc.tr("José • Núñez"); got != "Jos\xe9 \x95 N\xfa\xf1ez" {
		t.Fatalf("translator output = %q, want cp1252 bytes", got)
	}

	content, _, err := buildReportPDF(ReportInput{
		ReportType: ReportTypeEntry,
		Meet:       ReportMeet{Name: "Año Meet", StartDate: "2026-03-01"},
		Entries: []ReportEntry{{
			SwimmerFirst: "José", SwimmerLast: "Núñez-Ñandú-Ångström-Ælfred", MeetEventID: "e1", EventNumber: "1",
			Stroke: "free", Distance: 50, Course: "SCY",
		}},
	})
	if err != nil || len(content) == 0 {
		t.Fatalf("buildReportPDF with accented names: err=%v len=%d", err, len(content))
	}
}

func TestListExportsForViewerAppliesPerTypeAudience(t *testing.T) {
	t.Parallel()
	svc := NewService()
	for id, typ := range map[string]string{"e1": "eligibility_report", "e2": "entry_report", "e3": "event_report"} {
		svc.exports[id] = &FileExport{ID: id, OrgID: "org-1", MeetID: "m1", ExportType: typ}
	}
	if !IsStaffOnlyExportType("eligibility_report") || IsStaffOnlyExportType("entry_report") {
		t.Fatal("staff-only attribute wrong")
	}
	parent := &appmiddleware.Principal{UserID: "p", Roles: []string{"parent"}}
	coach := &appmiddleware.Principal{UserID: "c", Roles: []string{"coach"}}
	got := svc.ListExportsForViewer(parent, "org-1", "m1")
	if len(got) != 2 {
		t.Fatalf("parent should see 2 non-staff exports, got %d", len(got))
	}
	for _, ex := range got {
		if ex.ExportType == "eligibility_report" {
			t.Fatal("staff-only report leaked to parent")
		}
	}
	if n := len(svc.ListExportsForViewer(coach, "org-1", "m1")); n != 3 {
		t.Fatalf("coach should see all 3, got %d", n)
	}
}
