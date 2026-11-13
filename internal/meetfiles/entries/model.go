// Package entries builds the entry bundle a club sends to a meet host once
// sign-ups are final: an SDIF entry file (.sd3), a Hy-Tek entry file (.hy3),
// two PDF entry reports (by event and by swimmer) and a plain-text log.
//
// Every layout in this package was derived from a reference bundle and is
// covered by byte-level tests, so treat column positions, padding and
// ordering as part of the contract rather than as style.
package entries

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/luryann/aquify/meetfiles"
)

// Default software identity written into the SD3 A0 and HY3 A1 records.
const (
	DefaultSoftwareName    = "aquify"
	DefaultSoftwareVersion = "1.0"
)

// Club is the entering team.
type Club struct {
	Name string
	// Code is the club's team code (e.g. "DARE"); LSC its two-letter LSC.
	Code       string
	LSC        string
	Address1   string
	Address2   string
	City       string
	State      string
	PostalCode string
	Country    string
}

// Meet identifies the meet the entries are for.
type Meet struct {
	Name     string
	Facility string
	Start    time.Time
	End      time.Time
	// AgeUpDate is the date swimmer ages are computed on; zero means Start.
	AgeUpDate time.Time
	Course    meetfiles.Course
	// AthleteFeeCents is the per-swimmer surcharge used in the cost summary.
	AthleteFeeCents int
	// RelayFeeCents is the per-relay fee used in the cost summary.
	RelayFeeCents int
}

// Swimmer is one entered athlete. ID must be unique within an Input.
type Swimmer struct {
	ID            string
	LastName      string
	FirstName     string
	MiddleInitial string
	PreferredName string
	// USSID is the USA Swimming member ID (14 characters for current IDs).
	USSID string
	DOB   time.Time
	// Gender is "M" or "F".
	Gender string
}

// DisplayName is the name shown on reports: preferred (or first) then last.
func (s Swimmer) DisplayName() string {
	first := strings.TrimSpace(s.PreferredName)
	if first == "" {
		first = strings.TrimSpace(s.FirstName)
	}
	return strings.TrimSpace(first + " " + strings.TrimSpace(s.LastName))
}

// Event is one individual event of the meet.
type Event struct {
	// Number is the host's event number, optionally with a letter suffix ("22A").
	Number   string
	Distance int
	Stroke   meetfiles.Stroke
	// Gender is "M", "F" or "X" (mixed).
	Gender   string
	MinAge   *int
	MaxAge   *int
	FeeCents int
}

// Entry enters one swimmer in one event.
type Entry struct {
	Swimmer Swimmer
	Event   Event
	// SeedTimeMs is the entry time in milliseconds; zero means no time (NT).
	SeedTimeMs int
	// SeedCourse is the course the seed time was swum in; blank means the
	// meet course.
	SeedCourse meetfiles.Course
	// SeedMeet and SeedDate say where the seed time came from. A blank
	// SeedMeet is reported as a manually added time.
	SeedMeet string
	SeedDate time.Time
}

// Input is everything needed to build an entry bundle.
type Input struct {
	Club Club
	Meet Meet
	// Entries are individual entries. Their order is preserved within each
	// event on the by-event report.
	Entries []Entry
	// RelayTeams is the number of relay teams entered (summary only).
	RelayTeams int
	// GeneratedAt is the export time in the club's local time zone.
	GeneratedAt time.Time
	// GeneratedBy is the name of the person who ran the export.
	GeneratedBy string
	// SoftwareName / SoftwareVersion default to the aquify identity.
	SoftwareName    string
	SoftwareVersion string
}

// ErrInvalidInput wraps every validation failure.
var ErrInvalidInput = errors.New("entries: invalid input")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// swimmerEntries is one swimmer with their entries in event order.
type swimmerEntries struct {
	swimmer Swimmer
	entries []Entry
}

// eventEntries is one event with its entries in input order.
type eventEntries struct {
	event   Event
	entries []Entry
}

// prepared is a validated Input with the orderings every writer needs.
type prepared struct {
	in Input
	// byEvent lists events in event-number order.
	byEvent []eventEntries
	// byName lists swimmers by last name then first name (byte order).
	byName []swimmerEntries
	// byReportName lists swimmers the way the by-swimmer report does:
	// case-insensitively by "Last, First".
	byReportName []swimmerEntries
	// byAppearance lists swimmers in the order they first appear when
	// walking byEvent; this is the HY3 athlete numbering.
	byAppearance []swimmerEntries
}

func prepare(in Input) (*prepared, error) {
	if strings.TrimSpace(in.Club.Code) == "" {
		return nil, invalid("club code is required")
	}
	if strings.TrimSpace(in.Meet.Name) == "" {
		return nil, invalid("meet name is required")
	}
	if in.Meet.Start.IsZero() {
		return nil, invalid("meet start date is required")
	}
	if in.GeneratedAt.IsZero() {
		return nil, invalid("generated-at time is required")
	}
	if len(in.Entries) == 0 {
		return nil, invalid("at least one entry is required")
	}
	if in.Meet.End.IsZero() {
		in.Meet.End = in.Meet.Start
	}
	if in.Meet.AgeUpDate.IsZero() {
		in.Meet.AgeUpDate = in.Meet.Start
	}
	if in.Meet.Course == meetfiles.CourseUnknown {
		in.Meet.Course = meetfiles.CourseShortCourseYards
	}
	if strings.TrimSpace(in.SoftwareName) == "" {
		in.SoftwareName = DefaultSoftwareName
	}
	if strings.TrimSpace(in.SoftwareVersion) == "" {
		in.SoftwareVersion = DefaultSoftwareVersion
	}

	p := &prepared{in: in}
	eventIndex := map[string]int{}
	swimmers := map[string]*swimmerEntries{}
	seen := map[string]bool{}
	for i, e := range in.Entries {
		if strings.TrimSpace(e.Swimmer.ID) == "" {
			return nil, invalid("entry %d: swimmer id is required", i+1)
		}
		if strings.TrimSpace(e.Swimmer.LastName) == "" || strings.TrimSpace(e.Swimmer.FirstName) == "" {
			return nil, invalid("entry %d: swimmer name is required", i+1)
		}
		if g := e.Swimmer.Gender; g != "M" && g != "F" {
			return nil, invalid("entry %d: swimmer gender must be M or F", i+1)
		}
		if e.Swimmer.DOB.IsZero() {
			return nil, invalid("entry %d: swimmer date of birth is required", i+1)
		}
		if _, _, ok := splitEventNumber(e.Event.Number); !ok {
			return nil, invalid("entry %d: event number %q must start with a number", i+1, e.Event.Number)
		}
		if e.Event.Distance <= 0 || e.Event.Distance > 9999 {
			return nil, invalid("entry %d: event distance %d is out of range", i+1, e.Event.Distance)
		}
		if strokeDigit(e.Event.Stroke) == 0 {
			return nil, invalid("entry %d: %q is not an individual stroke", i+1, e.Event.Stroke)
		}
		if g := e.Event.Gender; g != "M" && g != "F" && g != "X" {
			return nil, invalid("entry %d: event gender must be M, F or X", i+1)
		}
		if e.SeedTimeMs < 0 {
			return nil, invalid("entry %d: seed time must not be negative", i+1)
		}
		number := strings.ToUpper(strings.TrimSpace(e.Event.Number))
		e.Event.Number = number
		key := e.Swimmer.ID + "\x00" + number
		if seen[key] {
			return nil, invalid("swimmer %s is entered in event %s more than once", e.Swimmer.DisplayName(), number)
		}
		seen[key] = true

		idx, ok := eventIndex[number]
		if !ok {
			idx = len(p.byEvent)
			eventIndex[number] = idx
			p.byEvent = append(p.byEvent, eventEntries{event: e.Event})
		}
		p.byEvent[idx].entries = append(p.byEvent[idx].entries, e)

		sw := swimmers[e.Swimmer.ID]
		if sw == nil {
			sw = &swimmerEntries{swimmer: e.Swimmer}
			swimmers[e.Swimmer.ID] = sw
		}
		sw.entries = append(sw.entries, e)
	}

	sort.SliceStable(p.byEvent, func(i, j int) bool {
		return eventNumberLess(p.byEvent[i].event.Number, p.byEvent[j].event.Number)
	})
	for _, sw := range swimmers {
		sort.SliceStable(sw.entries, func(i, j int) bool {
			return eventNumberLess(sw.entries[i].Event.Number, sw.entries[j].Event.Number)
		})
	}

	placed := map[string]bool{}
	for _, ev := range p.byEvent {
		for _, e := range ev.entries {
			if !placed[e.Swimmer.ID] {
				placed[e.Swimmer.ID] = true
				p.byAppearance = append(p.byAppearance, *swimmers[e.Swimmer.ID])
			}
		}
	}
	p.byName = append([]swimmerEntries(nil), p.byAppearance...)
	sort.SliceStable(p.byName, func(i, j int) bool {
		a, b := p.byName[i].swimmer, p.byName[j].swimmer
		if a.LastName != b.LastName {
			return a.LastName < b.LastName
		}
		return a.FirstName < b.FirstName
	})
	p.byReportName = append([]swimmerEntries(nil), p.byAppearance...)
	reportKey := func(s Swimmer) string { return strings.ToLower(s.LastName + ", " + s.FirstName) }
	sort.SliceStable(p.byReportName, func(i, j int) bool {
		return reportKey(p.byReportName[i].swimmer) < reportKey(p.byReportName[j].swimmer)
	})
	return p, nil
}

// splitEventNumber splits "22A" into 22 and "A".
func splitEventNumber(v string) (int, string, bool) {
	v = strings.TrimSpace(v)
	end := 0
	for end < len(v) && v[end] >= '0' && v[end] <= '9' {
		end++
	}
	if end == 0 || end > 4 {
		return 0, v, false
	}
	n, err := strconv.Atoi(v[:end])
	if err != nil {
		return 0, v, false
	}
	return n, v[end:], true
}

func eventNumberLess(a, b string) bool {
	na, sa, _ := splitEventNumber(a)
	nb, sb, _ := splitEventNumber(b)
	if na != nb {
		return na < nb
	}
	return sa < sb
}

// ageOn returns a swimmer's age in whole years on the given date.
func ageOn(dob, on time.Time) int {
	age := on.Year() - dob.Year()
	if on.Month() < dob.Month() || (on.Month() == dob.Month() && on.Day() < dob.Day()) {
		age--
	}
	if age < 0 {
		return 0
	}
	return age
}

// strokeDigit is the SDIF stroke code; strokeLetter the Hy-Tek one.
func strokeDigit(s meetfiles.Stroke) int {
	switch s {
	case meetfiles.StrokeFreestyle:
		return 1
	case meetfiles.StrokeBackstroke:
		return 2
	case meetfiles.StrokeBreaststroke:
		return 3
	case meetfiles.StrokeButterfly:
		return 4
	case meetfiles.StrokeIM:
		return 5
	}
	return 0
}

func strokeLetter(s meetfiles.Stroke) string {
	if d := strokeDigit(s); d > 0 {
		return string(rune('A' + d - 1))
	}
	return " "
}

func strokeLabel(s meetfiles.Stroke) string {
	switch s {
	case meetfiles.StrokeFreestyle:
		return "Free"
	case meetfiles.StrokeBackstroke:
		return "Back"
	case meetfiles.StrokeBreaststroke:
		return "Breast"
	case meetfiles.StrokeButterfly:
		return "Fly"
	case meetfiles.StrokeIM:
		return "IM"
	}
	return ""
}

// courseLetter is the one-letter course code shared by SD3 and HY3.
func courseLetter(c meetfiles.Course) string {
	switch c {
	case meetfiles.CourseShortCourseMeters:
		return "S"
	case meetfiles.CourseLongCourseMeters:
		return "L"
	}
	return "Y"
}

func (p *prepared) seedCourse(e Entry) meetfiles.Course {
	if e.SeedCourse != meetfiles.CourseUnknown {
		return e.SeedCourse
	}
	return p.in.Meet.Course
}

// clockTime formats milliseconds as [m:]ss.hh (hundredths truncated).
func clockTime(ms int) string {
	hundredths := ms / 10
	minutes := hundredths / 6000
	rest := hundredths % 6000
	if minutes > 0 {
		return fmt.Sprintf("%d:%02d.%02d", minutes, rest/100, rest%100)
	}
	return fmt.Sprintf("%d.%02d", rest/100, rest%100)
}

// ascii replaces every non-printable or non-ASCII character with '?' after
// folding the common Latin letters, so fixed-width records stay one byte per
// column.
func ascii(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		default:
			if folded, ok := latinFold[r]; ok {
				b.WriteString(folded)
			} else {
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

var latinFold = map[rune]string{
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A", 'Æ': "AE", 'Ç': "C",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E", 'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I",
	'Ñ': "N", 'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O", 'Ù': "U",
	'Ú': "U", 'Û': "U", 'Ü': "U", 'Ý': "Y", 'ß': "ss",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae", 'ç': "c",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
	'ñ': "n", 'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ù': "u",
	'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y",
	'‘': "'", '’': "'", '“': "\"", '”': "\"", '–': "-", '—': "-",
}

// padRight left-justifies s in width columns, truncating if needed.
func padRight(s string, width int) string {
	s = ascii(s)
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// padLeft right-justifies s in width columns, keeping the rightmost
// characters if it is too long.
func padLeft(s string, width int) string {
	s = ascii(s)
	if len(s) >= width {
		return s[len(s)-width:]
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func mmddyyyy(t time.Time) string {
	if t.IsZero() {
		return strings.Repeat(" ", 8)
	}
	return t.Format("01022006")
}
