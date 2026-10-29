package exports

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
)

// Report types. Meet exports are human-readable PDF reports assembled
// server-side from the meet's approved (non-scratched) entries.
const (
	ReportTypeEligibility = "eligibility_report"
	ReportTypeEntry       = "entry_report"
	ReportTypeEvent       = "event_report"
)

// Entry-report grouping modes.
const (
	GroupingSession      = "session"
	GroupingGroup        = "group"
	GroupingAlphabetical = "alphabetical"
)

// ReportTeam is the org-level export identity printed in the report header.
type ReportTeam struct {
	TeamCode string `json:"team_code"`
	TeamName string `json:"team_name"`
	LSCCode  string `json:"lsc_code"`
}

// ReportMeet is the meet metadata printed in the report header.
type ReportMeet struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"` // YYYY-MM-DD
	EndDate   string `json:"end_date"`   // YYYY-MM-DD
	Course    string `json:"course"`
	Location  string `json:"location"`
	HostOrg   string `json:"host_org"`
}

// ReportEvent is a single meet event in the report.
type ReportEvent struct {
	MeetEventID string `json:"meet_event_id"`
	EventNumber string `json:"event_number"`
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	Stroke      string `json:"stroke"`
	Distance    int    `json:"distance"`
	Course      string `json:"course"`
	Gender      string `json:"gender"` // M|F|X|O
	MinAge      *int   `json:"min_age"`
	MaxAge      *int   `json:"max_age"`
	IsRelay     bool   `json:"is_relay"`
}

// ReportEntry is a single approved entry (swimmer in an event).
type ReportEntry struct {
	SwimmerFirst  string `json:"swimmer_first"`
	SwimmerLast   string `json:"swimmer_last"`
	USASwimmingID string `json:"usa_swimming_id"`
	Gender        string `json:"gender"` // female|male|non_binary|undisclosed
	AgeAtMeet     int    `json:"age_at_meet"`
	GroupCode     string `json:"group_code"`

	MeetEventID string `json:"meet_event_id"`
	EventNumber string `json:"event_number"`
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	Stroke      string `json:"stroke"`
	Distance    int    `json:"distance"`
	Course      string `json:"course"`
	SeedTimeMs  *int   `json:"seed_time_ms"`

	// Eligible reflects whether the swimmer meets the event's gender/age rules;
	// EligibilityNote explains a failure or override. Used by the eligibility report.
	Eligible        bool   `json:"eligible"`
	EligibilityNote string `json:"eligibility_note"`
}

// ReportInput is the fully-assembled data for one report, produced server-side.
type ReportInput struct {
	ReportType  string        `json:"report_type"`
	Grouping    string        `json:"grouping"`
	Team        ReportTeam    `json:"team"`
	Meet        ReportMeet    `json:"meet"`
	Events      []ReportEvent `json:"events"`
	Entries     []ReportEntry `json:"entries"`
	GeneratedAt time.Time     `json:"generated_at"`
}

func validateReportInput(in ReportInput) error {
	switch in.ReportType {
	case ReportTypeEligibility, ReportTypeEvent:
	case ReportTypeEntry:
		switch in.Grouping {
		case GroupingSession, GroupingGroup, GroupingAlphabetical, "":
		default:
			return fmt.Errorf("%w: grouping must be session, group, or alphabetical", ErrInvalidExportType)
		}
	default:
		return ErrInvalidExportType
	}
	if strings.TrimSpace(in.Meet.Name) == "" {
		return fmt.Errorf("meet name is required")
	}
	return nil
}

func reportFilename(in ReportInput) string {
	base := sanitizeFilename(in.Meet.Name)
	switch in.ReportType {
	case ReportTypeEligibility:
		return base + "_eligibility.pdf"
	case ReportTypeEntry:
		suffix := in.Grouping
		if suffix == "" {
			suffix = GroupingAlphabetical
		}
		return base + "_entries_by_" + suffix + ".pdf"
	case ReportTypeEvent:
		return base + "_event_report.pdf"
	default:
		return base + ".pdf"
	}
}

// ─── PDF rendering ──────────────────────────────────────────────────────────

const (
	pdfMarginL = 15.0
	pdfMarginR = 15.0
	pdfMarginT = 15.0
)

// pdfDoc wraps the PDF so every string drawn or measured goes through the core
// font's cp1252 translator; without it accented names (José, Núñez) and the
// bullets/dashes used in headings render as mojibake in Helvetica.
type pdfDoc struct {
	*fpdf.Fpdf
	tr func(string) string
}

func newPDFDoc(pdf *fpdf.Fpdf) pdfDoc {
	return pdfDoc{Fpdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}
}

func (d pdfDoc) CellFormat(w, h float64, txt, border string, ln int, align string, fill bool, link int, linkStr string) {
	d.Fpdf.CellFormat(w, h, d.tr(txt), border, ln, align, fill, link, linkStr)
}

func (d pdfDoc) GetStringWidth(s string) float64 { return d.Fpdf.GetStringWidth(d.tr(s)) }

func buildReportPDF(in ReportInput) ([]byte, string, error) {
	if err := validateReportInput(in); err != nil {
		return nil, "", err
	}
	if in.GeneratedAt.IsZero() {
		in.GeneratedAt = time.Now().UTC()
	}

	pdf := newPDFDoc(fpdf.New("P", "mm", "Letter", ""))
	pdf.SetMargins(pdfMarginL, pdfMarginT, pdfMarginR)
	pdf.SetAutoPageBreak(true, 15)
	title := reportTitle(in.ReportType)
	registerHeaderFooter(pdf, in, title)
	pdf.AddPage()

	switch in.ReportType {
	case ReportTypeEligibility:
		renderEligibilityReport(pdf, in)
	case ReportTypeEvent:
		renderEventReport(pdf, in)
	case ReportTypeEntry:
		renderEntryReport(pdf, in)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", fmt.Errorf("render report pdf: %w", err)
	}
	return buf.Bytes(), reportFilename(in), nil
}

func reportTitle(reportType string) string {
	switch reportType {
	case ReportTypeEligibility:
		return "Eligibility Report"
	case ReportTypeEvent:
		return "Event Report"
	case ReportTypeEntry:
		return "Entry Report"
	default:
		return "Meet Report"
	}
}

func registerHeaderFooter(pdf pdfDoc, in ReportInput, title string) {
	pageW, _ := pdf.GetPageSize()
	contentW := pageW - pdfMarginL - pdfMarginR

	pdf.SetHeaderFunc(func() {
		pdf.SetY(pdfMarginT)
		pdf.SetFont("Helvetica", "B", 16)
		pdf.SetTextColor(20, 20, 20)
		pdf.CellFormat(contentW, 8, in.Meet.Name, "", 1, "L", false, 0, "")

		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(90, 90, 90)
		meta := []string{title}
		if dr := dateRange(in.Meet.StartDate, in.Meet.EndDate); dr != "" {
			meta = append(meta, dr)
		}
		if c := courseLabel(in.Meet.Course); c != "" {
			meta = append(meta, c)
		}
		if in.Meet.Location != "" {
			meta = append(meta, in.Meet.Location)
		}
		pdf.CellFormat(contentW, 5, strings.Join(meta, "  •  "), "", 1, "L", false, 0, "")

		if team := teamLine(in.Team); team != "" {
			pdf.CellFormat(contentW, 5, team, "", 1, "L", false, 0, "")
		}

		pdf.SetDrawColor(235, 93, 30) // brand accent
		pdf.SetLineWidth(0.5)
		y := pdf.GetY() + 1
		pdf.Line(pdfMarginL, y, pageW-pdfMarginR, y)
		pdf.Ln(4)
		pdf.SetTextColor(20, 20, 20)
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(140, 140, 140)
		left := fmt.Sprintf("Generated %s", in.GeneratedAt.Format("Jan 2, 2006 15:04 MST"))
		pdf.CellFormat(contentW/2, 6, left, "", 0, "L", false, 0, "")
		pdf.CellFormat(contentW/2, 6, fmt.Sprintf("Page %d", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
}

// ─── Event report: swimmers grouped under each event, sorted by seed time ────

func renderEventReport(pdf pdfDoc, in ReportInput) {
	byEvent := entriesByEvent(in.Entries)
	events := sortedEvents(in.Events)

	if len(events) == 0 {
		renderEmpty(pdf, "No events with approved entries.")
		return
	}
	for _, ev := range events {
		entries := byEvent[ev.MeetEventID]
		sort.SliceStable(entries, func(i, j int) bool {
			return seedSortKey(entries[i].SeedTimeMs) < seedSortKey(entries[j].SeedTimeMs)
		})
		sectionHeading(pdf, eventHeading(ev), fmt.Sprintf("%d entered", len(entries)))
		if len(entries) == 0 {
			mutedLine(pdf, "No approved entries.")
			continue
		}
		cols := []tableCol{
			{"#", 10, "R"},
			{"Swimmer", 70, "L"},
			{"Age", 14, "R"},
			{"USA-S ID", 40, "L"},
			{"Seed", 30, "R"},
		}
		tableHeader(pdf, cols)
		for i, e := range entries {
			tableRow(pdf, cols, []string{
				fmt.Sprintf("%d", i+1),
				swimmerName(e.SwimmerLast, e.SwimmerFirst),
				fmt.Sprintf("%d", e.AgeAtMeet),
				e.USASwimmingID,
				formatSeedTime(e.SeedTimeMs),
			}, i)
		}
		pdf.Ln(4)
	}
}

// ─── Eligibility report: per event, each swimmer's eligibility status ─────────

func renderEligibilityReport(pdf pdfDoc, in ReportInput) {
	byEvent := entriesByEvent(in.Entries)
	events := sortedEvents(in.Events)

	if len(events) == 0 {
		renderEmpty(pdf, "No events with approved entries.")
		return
	}
	for _, ev := range events {
		entries := byEvent[ev.MeetEventID]
		sort.SliceStable(entries, func(i, j int) bool {
			return swimmerName(entries[i].SwimmerLast, entries[i].SwimmerFirst) <
				swimmerName(entries[j].SwimmerLast, entries[j].SwimmerFirst)
		})
		sectionHeading(pdf, eventHeading(ev), eventRuleLabel(ev))
		if len(entries) == 0 {
			mutedLine(pdf, "No approved entries.")
			continue
		}
		cols := []tableCol{
			{"Swimmer", 66, "L"},
			{"Sex", 14, "L"},
			{"Age", 14, "R"},
			{"USA-S ID", 36, "L"},
			{"Status", 34, "L"},
		}
		tableHeader(pdf, cols)
		for i, e := range entries {
			status := "Eligible"
			if !e.Eligible {
				status = "Override"
			}
			note := e.EligibilityNote
			pdf.SetFont("Helvetica", "", 9)
			if !e.Eligible {
				pdf.SetTextColor(190, 70, 20)
			}
			tableRowCells(pdf, cols, []string{
				swimmerName(e.SwimmerLast, e.SwimmerFirst),
				genderShort(e.Gender),
				fmt.Sprintf("%d", e.AgeAtMeet),
				e.USASwimmingID,
				status,
			}, i)
			pdf.SetTextColor(20, 20, 20)
			if note != "" {
				pdf.SetFont("Helvetica", "I", 8)
				pdf.SetTextColor(150, 150, 150)
				pdf.CellFormat(0, 4, "   "+note, "", 1, "L", false, 0, "")
				pdf.SetTextColor(20, 20, 20)
			}
		}
		pdf.Ln(4)
	}
}

// ─── Entry report: one row per (swimmer, event), grouped per toggle ───────────

func renderEntryReport(pdf pdfDoc, in ReportInput) {
	if len(in.Entries) == 0 {
		renderEmpty(pdf, "No approved entries.")
		return
	}
	grouping := in.Grouping
	if grouping == "" {
		grouping = GroupingAlphabetical
	}

	switch grouping {
	case GroupingSession:
		renderEntryReportBy(pdf, in, func(e ReportEntry) (string, string) {
			name := e.SessionName
			if name == "" {
				name = "Unassigned session"
			}
			return e.SessionID + "\x00" + name, name
		}, true)
	case GroupingGroup:
		renderEntryReportBy(pdf, in, func(e ReportEntry) (string, string) {
			g := strings.ToUpper(strings.TrimSpace(e.GroupCode))
			if g == "" {
				return "\xff", "No group"
			}
			return g, g
		}, false)
	default:
		renderEntryReportFlat(pdf, in.Entries)
	}
}

func renderEntryReportBy(pdf pdfDoc, in ReportInput, keyFn func(ReportEntry) (string, string), byEventOrder bool) {
	groups := map[string][]ReportEntry{}
	labels := map[string]string{}
	var order []string
	for _, e := range in.Entries {
		key, label := keyFn(e)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], e)
		labels[key] = label
	}
	sort.Strings(order)

	for _, key := range order {
		entries := groups[key]
		sortEntryRows(entries, byEventOrder)
		sectionHeading(pdf, labels[key], fmt.Sprintf("%d entries", len(entries)))
		entryRowsTable(pdf, entries)
		pdf.Ln(4)
	}
}

func renderEntryReportFlat(pdf pdfDoc, entries []ReportEntry) {
	rows := append([]ReportEntry(nil), entries...)
	sortEntryRows(rows, false)
	entryRowsTable(pdf, rows)
}

func entryRowsTable(pdf pdfDoc, entries []ReportEntry) {
	cols := []tableCol{
		{"Swimmer", 58, "L"},
		{"Age", 12, "R"},
		{"#", 10, "R"},
		{"Event", 56, "L"},
		{"Seed", 26, "R"},
	}
	tableHeader(pdf, cols)
	for i, e := range entries {
		tableRow(pdf, cols, []string{
			swimmerName(e.SwimmerLast, e.SwimmerFirst),
			fmt.Sprintf("%d", e.AgeAtMeet),
			e.EventNumber,
			eventDistanceStroke(e.Distance, e.Stroke, e.Course),
			formatSeedTime(e.SeedTimeMs),
		}, i)
	}
}

func sortEntryRows(entries []ReportEntry, byEventFirst bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if byEventFirst {
			if an, bn := eventNumKey(a.EventNumber), eventNumKey(b.EventNumber); an != bn {
				return an < bn
			}
			return swimmerName(a.SwimmerLast, a.SwimmerFirst) < swimmerName(b.SwimmerLast, b.SwimmerFirst)
		}
		na, nb := swimmerName(a.SwimmerLast, a.SwimmerFirst), swimmerName(b.SwimmerLast, b.SwimmerFirst)
		if na != nb {
			return na < nb
		}
		return eventNumKey(a.EventNumber) < eventNumKey(b.EventNumber)
	})
}

// ─── Low-level table + text helpers ───────────────────────────────────────────

type tableCol struct {
	title string
	width float64
	align string
}

func sectionHeading(pdf pdfDoc, title, right string) {
	pdf.Ln(1)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(20, 20, 20)
	pageW, _ := pdf.GetPageSize()
	contentW := pageW - pdfMarginL - pdfMarginR
	if right != "" {
		pdf.CellFormat(contentW*0.7, 6, title, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(contentW*0.3, 6, right, "", 1, "R", false, 0, "")
	} else {
		pdf.CellFormat(contentW, 6, title, "", 1, "L", false, 0, "")
	}
	pdf.SetTextColor(20, 20, 20)
}

func tableHeader(pdf pdfDoc, cols []tableCol) {
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFillColor(60, 60, 70)
	for _, c := range cols {
		pdf.CellFormat(c.width, 6, strings.ToUpper(c.title), "", 0, c.align, true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetTextColor(20, 20, 20)
}

func tableRow(pdf pdfDoc, cols []tableCol, values []string, rowIndex int) {
	pdf.SetFont("Helvetica", "", 9)
	tableRowCells(pdf, cols, values, rowIndex)
}

func tableRowCells(pdf pdfDoc, cols []tableCol, values []string, rowIndex int) {
	fill := rowIndex%2 == 1
	if fill {
		pdf.SetFillColor(244, 244, 246)
	} else {
		pdf.SetFillColor(255, 255, 255)
	}
	for i, c := range cols {
		v := ""
		if i < len(values) {
			v = truncateToWidth(pdf, values[i], c.width-2)
		}
		pdf.CellFormat(c.width, 5.5, v, "", 0, c.align, true, 0, "")
	}
	pdf.Ln(-1)
}

func mutedLine(pdf pdfDoc, text string) {
	pdf.SetFont("Helvetica", "I", 9)
	pdf.SetTextColor(150, 150, 150)
	pdf.CellFormat(0, 5, text, "", 1, "L", false, 0, "")
	pdf.SetTextColor(20, 20, 20)
}

func renderEmpty(pdf pdfDoc, text string) {
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "I", 11)
	pdf.SetTextColor(140, 140, 140)
	pdf.CellFormat(0, 8, text, "", 1, "C", false, 0, "")
	pdf.SetTextColor(20, 20, 20)
}

func truncateToWidth(pdf pdfDoc, s string, maxW float64) string {
	if pdf.GetStringWidth(s) <= maxW {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if candidate := string(runes) + "…"; pdf.GetStringWidth(candidate) <= maxW {
			return candidate
		}
	}
	return s
}

// ─── Domain formatting helpers ────────────────────────────────────────────────

func entriesByEvent(entries []ReportEntry) map[string][]ReportEntry {
	out := map[string][]ReportEntry{}
	for _, e := range entries {
		out[e.MeetEventID] = append(out[e.MeetEventID], e)
	}
	return out
}

func sortedEvents(events []ReportEvent) []ReportEvent {
	out := append([]ReportEvent(nil), events...)
	sort.SliceStable(out, func(i, j int) bool {
		return eventNumKey(out[i].EventNumber) < eventNumKey(out[j].EventNumber)
	})
	return out
}

// eventNumKey turns an event number like "12A" into a sortable numeric-major key.
func eventNumKey(num string) int64 {
	num = strings.TrimSpace(num)
	var digits strings.Builder
	var suffix int64
	for _, r := range num {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		} else if suffix == 0 {
			suffix = int64(r)
		}
	}
	var n int64
	fmt.Sscanf(digits.String(), "%d", &n)
	return n*1000 + suffix
}

func eventHeading(ev ReportEvent) string {
	parts := []string{}
	if ev.EventNumber != "" {
		parts = append(parts, "Event "+ev.EventNumber)
	}
	segment := strings.TrimSpace(eventRuleLabel(ev) + " " + eventDistanceStroke(ev.Distance, ev.Stroke, ev.Course))
	if segment != "" {
		parts = append(parts, segment)
	}
	return strings.Join(parts, " — ")
}

func eventRuleLabel(ev ReportEvent) string {
	return strings.TrimSpace(eventGenderLabel(ev.Gender) + " " + eventAgeLabel(ev.MinAge, ev.MaxAge))
}

func eventDistanceStroke(distance int, stroke, course string) string {
	s := strokeLabel(stroke)
	c := courseShort(course)
	if distance > 0 {
		if c != "" {
			return fmt.Sprintf("%d %s %s", distance, c, s)
		}
		return fmt.Sprintf("%d %s", distance, s)
	}
	return s
}

func strokeLabel(stroke string) string {
	switch strings.ToLower(strings.TrimSpace(stroke)) {
	case "freestyle", "free":
		return "Free"
	case "backstroke", "back":
		return "Back"
	case "breaststroke", "breast":
		return "Breast"
	case "butterfly", "fly":
		return "Fly"
	case "im", "medley", "individual_medley":
		return "IM"
	case "free_relay", "freestyle_relay":
		return "Free Relay"
	case "medley_relay":
		return "Medley Relay"
	default:
		if stroke == "" {
			return ""
		}
		return titleWords(strings.ToLower(stroke))
	}
}

// titleWords upper-cases the first letter of each whitespace-separated word.
func titleWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}

func courseShort(course string) string {
	switch strings.ToUpper(strings.TrimSpace(course)) {
	case "SCY":
		return "Y"
	case "SCM":
		return "S"
	case "LCM":
		return "L"
	default:
		return ""
	}
}

func courseLabel(course string) string {
	switch strings.ToUpper(strings.TrimSpace(course)) {
	case "SCY":
		return "Short Course Yards"
	case "SCM":
		return "Short Course Meters"
	case "LCM":
		return "Long Course Meters"
	default:
		return ""
	}
}

func eventGenderLabel(g string) string {
	switch strings.ToUpper(strings.TrimSpace(g)) {
	case "M":
		return "Boys"
	case "F":
		return "Girls"
	case "X", "O":
		return "Mixed"
	default:
		return ""
	}
}

func genderShort(g string) string {
	switch strings.ToLower(strings.TrimSpace(g)) {
	case "female":
		return "F"
	case "male":
		return "M"
	case "non_binary":
		return "X"
	default:
		return "-"
	}
}

func eventAgeLabel(minAge, maxAge *int) string {
	switch {
	case minAge == nil && maxAge == nil:
		return "Open"
	case minAge != nil && maxAge != nil:
		return fmt.Sprintf("%d-%d", *minAge, *maxAge)
	case maxAge != nil:
		return fmt.Sprintf("%d & Under", *maxAge)
	default:
		return fmt.Sprintf("%d & Over", *minAge)
	}
}

func swimmerName(last, first string) string {
	last = strings.TrimSpace(last)
	first = strings.TrimSpace(first)
	if last == "" {
		return first
	}
	if first == "" {
		return last
	}
	return last + ", " + first
}

func teamLine(t ReportTeam) string {
	parts := []string{}
	if t.TeamName != "" {
		parts = append(parts, t.TeamName)
	}
	code := t.TeamCode
	if t.LSCCode != "" {
		if code != "" {
			code += " / " + t.LSCCode
		} else {
			code = t.LSCCode
		}
	}
	if code != "" {
		parts = append(parts, "("+code+")")
	}
	return strings.Join(parts, " ")
}

func dateRange(start, end string) string {
	s := parseYMD(start)
	e := parseYMD(end)
	switch {
	case s == nil && e == nil:
		return ""
	case s != nil && e == nil:
		return s.Format("Jan 2, 2006")
	case s == nil && e != nil:
		return e.Format("Jan 2, 2006")
	case s.Equal(*e):
		return s.Format("Jan 2, 2006")
	case s.Year() == e.Year():
		return fmt.Sprintf("%s - %s", s.Format("Jan 2"), e.Format("Jan 2, 2006"))
	default:
		return fmt.Sprintf("%s - %s", s.Format("Jan 2, 2006"), e.Format("Jan 2, 2006"))
	}
}

func parseYMD(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// formatSeedTime renders milliseconds as a swim time (m:ss.hh or ss.hh); NT when unset.
func formatSeedTime(ms *int) string {
	if ms == nil || *ms <= 0 {
		return "NT"
	}
	total := *ms
	minutes := total / 60000
	seconds := (total % 60000) / 1000
	hundredths := (total % 1000) / 10
	if minutes > 0 {
		return fmt.Sprintf("%d:%02d.%02d", minutes, seconds, hundredths)
	}
	return fmt.Sprintf("%d.%02d", seconds, hundredths)
}

func seedSortKey(ms *int) int {
	if ms == nil || *ms <= 0 {
		return 1 << 30 // NT sorts last
	}
	return *ms
}

// AgeAtMeet computes a swimmer's age as of the meet start date (USA-Swimming
// "age on the first day" convention). DOB and meetStart are YYYY-MM-DD.
func AgeAtMeet(dob, meetStart string) int {
	d := parseYMD(dob)
	m := parseYMD(meetStart)
	if d == nil {
		return 0
	}
	ref := time.Now().UTC()
	if m != nil {
		ref = *m
	}
	age := ref.Year() - d.Year()
	if ref.Month() < d.Month() || (ref.Month() == d.Month() && ref.Day() < d.Day()) {
		age--
	}
	if age < 0 {
		return 0
	}
	return age
}
