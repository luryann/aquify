package exports

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"
)

func TestCov1xValidateReportInput(t *testing.T) {
	t.Parallel()
	in := sampleReportInput(ReportTypeEntry, "bogus")
	err := validateReportInput(in)
	if !errors.Is(err, ErrInvalidExportType) || !strings.Contains(err.Error(), "grouping must be") {
		t.Fatalf("bad grouping err = %v", err)
	}
	for _, g := range []string{"", GroupingSession, GroupingGroup, GroupingAlphabetical} {
		if err := validateReportInput(sampleReportInput(ReportTypeEntry, g)); err != nil {
			t.Errorf("grouping %q rejected: %v", g, err)
		}
	}
}

func TestCov1xReportFilenameAndTitleDefaults(t *testing.T) {
	t.Parallel()
	in := sampleReportInput("mystery", "")
	if got := reportFilename(in); got != "spring-invitational.pdf" {
		t.Fatalf("default filename = %q", got)
	}
	in.ReportType = ReportTypeEntry
	if got := reportFilename(in); got != "spring-invitational_entries_by_alphabetical.pdf" {
		t.Fatalf("entry default grouping filename = %q", got)
	}
	if got := reportTitle("mystery"); got != "Meet Report" {
		t.Fatalf("default title = %q", got)
	}
}

func cov1xMustPDF(t *testing.T, in ReportInput, wantName string) []byte {
	t.Helper()
	content, name, err := buildReportPDF(in)
	if err != nil {
		t.Fatalf("buildReportPDF(%s): %v", wantName, err)
	}
	if name != wantName {
		t.Fatalf("filename = %q, want %q", name, wantName)
	}
	if !strings.HasPrefix(string(content), "%PDF-") || !strings.Contains(string(content), "%%EOF") {
		t.Fatalf("%s is not a well-formed PDF", wantName)
	}
	return content
}

func TestCov1xBuildReportPDFEmptyAndEdgeShapes(t *testing.T) {
	t.Parallel()
	for rt, base := range map[string]string{ReportTypeEvent: "event_report", ReportTypeEligibility: "eligibility"} {
		wantName := "spring-invitational_" + base + ".pdf"
		full := cov1xMustPDF(t, sampleReportInput(rt, ""), wantName)
		in := sampleReportInput(rt, "")
		in.Events, in.Entries = nil, nil
		empty := cov1xMustPDF(t, in, wantName)
		if len(empty) >= len(full) {
			t.Errorf("%s: empty report (%d bytes) should be smaller than full (%d)", rt, len(empty), len(full))
		}

		// Events present but nobody entered: "No approved entries." rows.
		in = sampleReportInput(rt, "")
		in.Entries = nil
		cov1xMustPDF(t, in, wantName)
	}
	in := sampleReportInput(ReportTypeEntry, GroupingSession)
	in.Entries = nil
	cov1xMustPDF(t, in, "spring-invitational_entries_by_session.pdf")
	// Entry grouping with unassigned sessions/groups and a long name that must be truncated.
	in = sampleReportInput(ReportTypeEntry, GroupingSession)
	in.Entries[0].SessionName = ""
	in.Entries[0].SwimmerLast = strings.Repeat("Verylongsurname", 10)
	cov1xMustPDF(t, in, "spring-invitational_entries_by_session.pdf")
	in.Grouping = GroupingGroup
	in.Entries[0].GroupCode = "  "
	in.Entries[1].GroupCode = "senior"
	cov1xMustPDF(t, in, "spring-invitational_entries_by_group.pdf")
	in.Grouping = ""
	cov1xMustPDF(t, in, "spring-invitational_entries_by_alphabetical.pdf")
	// Eligibility rows with notes and an event with zero distance + custom stroke.
	in = sampleReportInput(ReportTypeEligibility, "")
	in.Events = append(in.Events, ReportEvent{MeetEventID: "e3", EventNumber: "3", Stroke: "open water", Gender: "X"})
	cov1xMustPDF(t, in, "spring-invitational_eligibility.pdf")
}

func TestCov1xBuildReportPDFManyRowsPaginates(t *testing.T) {
	t.Parallel()
	in := sampleReportInput(ReportTypeEvent, "")
	for i := 0; i < 120; i++ {
		e := in.Entries[1]
		e.SwimmerFirst = "Swimmer"
		e.SwimmerLast = strings.Repeat("N", i%7+1)
		in.Entries = append(in.Entries, e)
	}
	content, _, err := buildReportPDF(in)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(content), "/Type /Page\n"); n < 2 {
		t.Fatalf("expected multiple pages, found %d", n)
	}
}

func TestCov1xTruncateToWidth(t *testing.T) {
	t.Parallel()
	pdf, _ := cov1xDoc()
	if got := truncateToWidth(pdf, "Adams", 100); got != "Adams" {
		t.Fatalf("short string changed: %q", got)
	}
	got := truncateToWidth(pdf, "Alexandria Montgomery-Featherstonehaugh", 20)
	if !strings.HasSuffix(got, "…") || pdf.GetStringWidth(got) > 20 {
		t.Fatalf("truncated = %q (w=%v)", got, pdf.GetStringWidth(got))
	}
	// Width too small for even one rune plus ellipsis: original is returned.
	if got := truncateToWidth(pdf, "Wide", 0.1); got != "Wide" {
		t.Fatalf("impossible width returned %q", got)
	}
}

func TestCov1xLabelHelpers(t *testing.T) {
	t.Parallel()
	strokes := map[string]string{
		"freestyle": "Free", " Free ": "Free", "backstroke": "Back", "back": "Back",
		"breaststroke": "Breast", "breast": "Breast", "butterfly": "Fly", "fly": "Fly",
		"im": "IM", "medley": "IM", "individual_medley": "IM",
		"free_relay": "Free Relay", "freestyle_relay": "Free Relay", "medley_relay": "Medley Relay",
		"": "", "open water swim": "Open Water Swim",
	}
	for in, want := range strokes {
		if got := strokeLabel(in); got != want {
			t.Errorf("strokeLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if got := titleWords("  alpha   beta "); got != "Alpha Beta" {
		t.Errorf("titleWords = %q", got)
	}
	for in, want := range map[string]string{"SCY": "Y", "scm": "S", " LCM ": "L", "xyz": ""} {
		if got := courseShort(in); got != want {
			t.Errorf("courseShort(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{"SCY": "Short Course Yards", "scm": "Short Course Meters", "LCM": "Long Course Meters", "": ""} {
		if got := courseLabel(in); got != want {
			t.Errorf("courseLabel(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{"M": "Boys", "f": "Girls", "X": "Mixed", "O": "Mixed", "?": ""} {
		if got := eventGenderLabel(in); got != want {
			t.Errorf("eventGenderLabel(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{"female": "F", "Male": "M", "non_binary": "X", "undisclosed": "-", "": "-"} {
		if got := genderShort(in); got != want {
			t.Errorf("genderShort(%q) = %q", in, got)
		}
	}
	lo, hi := intPtr(9), intPtr(12)
	ageCases := []struct {
		min, max *int
		want     string
	}{{nil, nil, "Open"}, {lo, hi, "9-12"}, {nil, hi, "12 & Under"}, {lo, nil, "9 & Over"}}
	for _, c := range ageCases {
		if got := eventAgeLabel(c.min, c.max); got != c.want {
			t.Errorf("eventAgeLabel = %q, want %q", got, c.want)
		}
	}
}

func TestCov1xEventDistanceStrokeAndHeading(t *testing.T) {
	t.Parallel()
	if got := eventDistanceStroke(100, "free", "SCY"); got != "100 Y Free" {
		t.Errorf("got %q", got)
	}
	if got := eventDistanceStroke(100, "free", ""); got != "100 Free" {
		t.Errorf("got %q", got)
	}
	if got := eventDistanceStroke(0, "fly", "SCY"); got != "Fly" {
		t.Errorf("got %q", got)
	}
	ev := ReportEvent{EventNumber: "4", Stroke: "back", Distance: 50, Course: "LCM", Gender: "M", MaxAge: intPtr(10)}
	if got := eventHeading(ev); got != "Event 4 — Boys 10 & Under 50 L Back" {
		t.Errorf("heading = %q", got)
	}
	if got := eventHeading(ReportEvent{Gender: "?", MinAge: intPtr(1), MaxAge: intPtr(2)}); got != "1-2" {
		t.Errorf("no-number heading = %q", got)
	}
	if got := eventRuleLabel(ReportEvent{Gender: "F"}); got != "Girls Open" {
		t.Errorf("rule label = %q", got)
	}
}

func TestCov1xSwimmerNameTeamLineDateRange(t *testing.T) {
	t.Parallel()
	for _, c := range [][3]string{{"Adams", "Amy", "Adams, Amy"}, {" ", "Amy", "Amy"}, {"Adams", "", "Adams"}} {
		if got := swimmerName(c[0], c[1]); got != c[2] {
			t.Errorf("swimmerName(%q,%q) = %q", c[0], c[1], got)
		}
	}
	teams := []struct {
		in   ReportTeam
		want string
	}{
		{ReportTeam{}, ""},
		{ReportTeam{TeamName: "Sharks", TeamCode: "SHK", LSCCode: "PC"}, "Sharks (SHK / PC)"},
		{ReportTeam{TeamName: "Sharks", LSCCode: "PC"}, "Sharks (PC)"},
		{ReportTeam{TeamCode: "SHK"}, "(SHK)"},
	}
	for _, c := range teams {
		if got := teamLine(c.in); got != c.want {
			t.Errorf("teamLine(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
	dates := [][3]string{
		{"", "", ""}, {"2026-03-15", "", "Mar 15, 2026"}, {"", "2026-03-16", "Mar 16, 2026"},
		{"2026-03-15", "2026-03-15", "Mar 15, 2026"}, {"2026-03-15", "2026-03-16", "Mar 15 - Mar 16, 2026"},
		{"2025-12-30", "2026-01-02", "Dec 30, 2025 - Jan 2, 2026"}, {"garbage", "2026-03-16", "Mar 16, 2026"},
	}
	for _, c := range dates {
		if got := dateRange(c[0], c[1]); got != c[2] {
			t.Errorf("dateRange(%q,%q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
	if parseYMD("  ") != nil || parseYMD("2026-13-45") != nil {
		t.Error("invalid dates must parse to nil")
	}
}

func TestCov1xAgeAtMeetEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dob, meet string
		want      int
	}{
		{"", "2026-03-15", 0},
		{"2010-03-15", "2026-03-15", 16},
		{"2010-03-16", "2026-03-15", 15},
		{"2010-04-01", "2026-03-15", 15},
		{"2030-01-01", "2026-03-15", 0},
	}
	for _, c := range cases {
		if got := AgeAtMeet(c.dob, c.meet); got != c.want {
			t.Errorf("AgeAtMeet(%q,%q) = %d, want %d", c.dob, c.meet, got, c.want)
		}
	}
	// No meet date: measured against today.
	dob := time.Now().UTC().AddDate(-10, 0, -1).Format("2006-01-02")
	if got := AgeAtMeet(dob, ""); got != 10 {
		t.Errorf("age vs today = %d, want 10", got)
	}
}

func TestCov1xSortEntryRowsOrders(t *testing.T) {
	t.Parallel()
	rows := []ReportEntry{
		{SwimmerLast: "B", SwimmerFirst: "x", EventNumber: "2"},
		{SwimmerLast: "A", SwimmerFirst: "x", EventNumber: "10"},
		{SwimmerLast: "A", SwimmerFirst: "x", EventNumber: "3"},
	}
	sortEntryRows(rows, false)
	if rows[0].EventNumber != "3" || rows[1].EventNumber != "10" || rows[2].SwimmerLast != "B" {
		t.Fatalf("name-first order wrong: %+v", rows)
	}
	sortEntryRows(rows, true)
	if rows[0].EventNumber != "2" || rows[1].EventNumber != "3" || rows[2].EventNumber != "10" {
		t.Fatalf("event-first order wrong: %+v", rows)
	}
}

func cov1xDoc() (pdfDoc, error) {
	f := fpdf.New("P", "mm", "Letter", "")
	f.AddPage()
	f.SetFont("Helvetica", "", 9)
	return newPDFDoc(f), nil
}
