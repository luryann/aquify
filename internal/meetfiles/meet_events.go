package meetfiles

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SourceFile is an uploaded file: its name and raw bytes.
type SourceFile struct {
	Name    string
	Content []byte
}

// ErrNoMeetEvents means none of the supplied files was a meet event file.
var ErrNoMeetEvents = errors.New("meetfiles: no .ev3 or .hyv meet event file found")

// maxEventFileWarnings caps the disagreement list so a pair of unrelated
// files cannot produce thousands of lines.
const maxEventFileWarnings = 20

// ParseMeetEvents reads the meet event files a host publishes. Hosts ship
// the same event list twice, as .hyv and as .ev3, usually zipped together;
// callers may pass either file, both, or the ZIP.
//
// The .ev3 is the richer of the two (it carries sessions and start times),
// so it is preferred when present. When both are supplied they are compared
// and every disagreement is returned as a warning, because the two files are
// meant to describe exactly the same meet.
func ParseMeetEvents(files []SourceFile) (*ParsedMeetDefinition, []string, error) {
	var ev3s, hyvs []SourceFile
	var warnings []string
	for _, f := range files {
		switch DetectFileType(f.Name, f.Content) {
		case FileTypeEV3:
			ev3s = append(ev3s, f)
		case FileTypeHYV:
			hyvs = append(hyvs, f)
		case FileTypeZIP:
			entries, err := ReadEventsZip(f.Content)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			for _, e := range entries {
				src := SourceFile(e)
				if DetectFileType(e.Name, e.Content) == FileTypeEV3 {
					ev3s = append(ev3s, src)
				} else {
					hyvs = append(hyvs, src)
				}
			}
		default:
			warnings = append(warnings, fmt.Sprintf("%s is not a meet event file (.ev3, .hyv or a .zip of them) and was ignored", f.Name))
		}
	}
	if len(ev3s) == 0 && len(hyvs) == 0 {
		return nil, nil, ErrNoMeetEvents
	}
	pick := func(kind string, list []SourceFile) *SourceFile {
		if len(list) == 0 {
			return nil
		}
		sort.SliceStable(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
		if len(list) > 1 {
			warnings = append(warnings, fmt.Sprintf("%d %s files were supplied; used %s", len(list), kind, list[0].Name))
		}
		return &list[0]
	}

	var ev3, hyv *ParsedMeetDefinition
	if src := pick(".ev3", ev3s); src != nil {
		parsed, err := ParseEV3(src.Name, src.Content)
		if err != nil {
			return nil, nil, err
		}
		ev3 = parsed
	}
	if src := pick(".hyv", hyvs); src != nil {
		parsed, err := ParseHYV(src.Name, src.Content)
		if err != nil {
			return nil, nil, err
		}
		hyv = parsed
	}

	result := ev3
	if result == nil {
		result = hyv
	}
	for _, def := range []*ParsedMeetDefinition{ev3, hyv} {
		if def == nil {
			continue
		}
		for _, pe := range def.ParseErrors {
			warnings = append(warnings, fmt.Sprintf("%s line %d: %s", def.SourceFile, pe.LineNumber, pe.Message))
		}
	}
	if ev3 != nil && hyv != nil {
		diffs := CompareMeetDefinitions(ev3, hyv)
		if len(diffs) > maxEventFileWarnings {
			extra := len(diffs) - maxEventFileWarnings
			diffs = append(diffs[:maxEventFileWarnings], fmt.Sprintf("...and %d more differences between the .ev3 and .hyv files", extra))
		}
		warnings = append(warnings, diffs...)
	}
	return result, warnings, nil
}

// CompareMeetDefinitions lists every way two parses of the same meet differ
// in the facts both formats carry: meet header and each event's number,
// distance, stroke, gender, age range, fee and time standards. Sessions are
// not compared because only the .ev3 has them.
func CompareMeetDefinitions(a, b *ParsedMeetDefinition) []string {
	var diffs []string
	add := func(format string, args ...any) { diffs = append(diffs, fmt.Sprintf(format, args...)) }
	la, lb := "."+string(a.FileType), "."+string(b.FileType)

	if a.MeetName != b.MeetName {
		add("meet name differs: %q (%s) vs %q (%s)", a.MeetName, la, b.MeetName, lb)
	}
	if a.Location != b.Location {
		add("location differs: %q (%s) vs %q (%s)", a.Location, la, b.Location, lb)
	}
	if a.Course != b.Course {
		add("course differs: %s (%s) vs %s (%s)", a.Course, la, b.Course, lb)
	}
	if d1, d2 := dateLabel(a.MeetStartDate), dateLabel(b.MeetStartDate); d1 != d2 {
		add("start date differs: %s (%s) vs %s (%s)", d1, la, d2, lb)
	}
	if d1, d2 := dateLabel(a.MeetEndDate), dateLabel(b.MeetEndDate); d1 != d2 {
		add("end date differs: %s (%s) vs %s (%s)", d1, la, d2, lb)
	}
	if d1, d2 := dateLabel(a.AgeUpDate), dateLabel(b.AgeUpDate); d1 != d2 {
		add("age-up date differs: %s (%s) vs %s (%s)", d1, la, d2, lb)
	}

	other := make(map[string]ParsedMeetEvent, len(b.Events))
	for _, ev := range b.Events {
		other[ev.EventNumber] = ev
	}
	seen := make(map[string]bool, len(a.Events))
	for _, ev := range a.Events {
		seen[ev.EventNumber] = true
		match, ok := other[ev.EventNumber]
		if !ok {
			add("event %s is in the %s file but not the %s file", ev.EventNumber, la, lb)
			continue
		}
		if x, y := describeMeetEvent(ev), describeMeetEvent(match); x != y {
			add("event %s differs: %s (%s) vs %s (%s)", ev.EventNumber, x, la, y, lb)
		}
	}
	for _, ev := range b.Events {
		if !seen[ev.EventNumber] {
			add("event %s is in the %s file but not the %s file", ev.EventNumber, lb, la)
		}
	}
	return diffs
}

func dateLabel(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "none"
	}
	return t.Format("2006-01-02")
}

func describeMeetEvent(ev ParsedMeetEvent) string {
	age := func(v *int) string {
		if v == nil {
			return "-"
		}
		return fmt.Sprintf("%d", *v)
	}
	kind := "individual"
	if ev.IsRelay {
		kind = "relay"
	}
	return fmt.Sprintf("%s %d %s, gender %s, ages %s to %s, fee %d cents, standards SCY %d ms / LCM %d ms",
		kind, ev.Distance, ev.Stroke, ev.Gender, age(ev.MinAge), age(ev.MaxAge), ev.EntryFeeCents,
		ev.TimeStandards[CourseShortCourseYards], ev.TimeStandards[CourseLongCourseMeters])
}
