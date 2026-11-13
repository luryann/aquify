package meets

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/meetfiles"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov1xFixtureDir = "../../../../internal/meetfiles/testdata/fixtures"

func cov1xFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cov1xFixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// cov1xZip builds an in-memory zip archive from name -> content pairs, in the
// order given.
func cov1xZip(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// cov1xJunk is more unrecognised SDIF records than the parsers tolerate, so
// both the SD3 and CL2 parsers reject it outright.
func cov1xJunk() string {
	return strings.Repeat("ZZ junk record that no parser understands\n", 10001)
}

var cov1xAdmin = &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}

func TestCov1xPersistenceErrorWrapsAndMatchesSentinel(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	err := error(&persistenceError{op: "meet bundle", err: cause})
	if got, want := err.Error(), "persist meet bundle: boom"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, ErrPersistence) {
		t.Fatal("persistenceError must match ErrPersistence")
	}
	if !errors.Is(err, cause) {
		t.Fatal("persistenceError must unwrap to its cause")
	}
	if errors.Is(err, ErrValidation) {
		t.Fatal("persistenceError must not match unrelated sentinels")
	}
}

func TestCov1xValidateEventBounds(t *testing.T) {
	t.Parallel()
	one, ten, neg := 1, 10, -1
	cases := []struct {
		name     string
		min, max *int
		fee      int
		wantMsg  string
	}{
		{"ok no bounds", nil, nil, 0, ""},
		{"ok bounds", &one, &ten, 500, ""},
		{"negative fee", nil, nil, -1, "entry_fee_cents must not be negative"},
		{"negative min", &neg, nil, 0, "min_age and max_age must not be negative"},
		{"negative max", nil, &neg, 0, "min_age and max_age must not be negative"},
		{"min above max", &ten, &one, 0, "min_age must not be greater than max_age"},
	}
	for _, tc := range cases {
		err := validateEventBounds(tc.min, tc.max, tc.fee)
		if tc.wantMsg == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), tc.wantMsg) {
			t.Errorf("%s: err = %v, want ErrValidation containing %q", tc.name, err, tc.wantMsg)
		}
	}
}

func TestCov1xValidateSeedTimeMs(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0, -5, maxSeedTimeMs + 1} {
		v := v
		if err := validateSeedTimeMs(&v); !errors.Is(err, ErrValidation) {
			t.Errorf("seed %d: err = %v, want ErrValidation", v, err)
		}
	}
	ok := 1
	max := maxSeedTimeMs
	for _, v := range []*int{nil, &ok, &max} {
		if err := validateSeedTimeMs(v); err != nil {
			t.Errorf("seed %v: unexpected error %v", v, err)
		}
	}
}

func TestCov1xValidateOptionalClock(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"", "   ", "08:30", "08:30:15", " 23:59 "} {
		if err := validateOptionalClock("start_time", ok); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
	for _, bad := range []string{"8am", "25:00", "08:30:99", "08-30"} {
		err := validateOptionalClock("start_time", bad)
		if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "start_time must be an HH:MM time") {
			t.Errorf("%q: err = %v, want ErrValidation naming the field", bad, err)
		}
	}
}

func TestCov1xParseOptionalDate(t *testing.T) {
	t.Parallel()
	if d, err := parseOptionalDate("start_date", "  "); err != nil || !d.IsZero() {
		t.Fatalf("blank date = %v, %v; want zero, nil", d, err)
	}
	d, err := parseOptionalDate("start_date", " 2026-07-04 ")
	if err != nil || !d.Equal(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date = %v, %v", d, err)
	}
	if _, err := parseOptionalDate("start_date", "07/04/2026"); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad date err = %v, want ErrValidation", err)
	}
}

func TestCov1xSwimmerAgeAsOf(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, dob, ref string
		wantAge        int
		wantOK         bool
	}{
		{"before birthday month", "2010-08-15", "2026-07-10", 15, true},
		{"after birthday month", "2010-06-15", "2026-07-10", 16, true},
		{"same month earlier day", "2010-07-20", "2026-07-10", 15, true},
		{"on birthday", "2010-07-10", "2026-07-10", 16, true},
		{"leap day birthday before leap-year ref", "2012-02-29", "2026-02-28", 13, true},
		{"unparseable dob", "not-a-date", "2026-07-10", 0, false},
		{"empty dob", "", "2026-07-10", 0, false},
	}
	for _, tc := range cases {
		age, ok := swimmerAgeAsOf(tc.dob, tc.ref)
		if age != tc.wantAge || ok != tc.wantOK {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", tc.name, age, ok, tc.wantAge, tc.wantOK)
		}
	}
	// An empty or malformed reference date falls back to today (UTC).
	today := time.Now().UTC()
	dob := today.AddDate(-12, 0, 0).Format("2006-01-02")
	for _, ref := range []string{"", "garbage"} {
		age, ok := swimmerAgeAsOf(dob, ref)
		if !ok || age < 11 || age > 12 {
			t.Errorf("ref %q: age = %d ok=%v, want about 12", ref, age, ok)
		}
	}
}

func TestCov1xEventGenderForSwimmer(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"female": "F", " Male ": "M", "FEMALE": "F",
		"non_binary": "", "undisclosed": "", "": "", "other": "",
	} {
		if got := eventGenderForSwimmer(in); got != want {
			t.Errorf("eventGenderForSwimmer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xValidateSwimmerEventEligibility(t *testing.T) {
	t.Parallel()
	min12, max14 := 12, 14
	girl := SwimmerEligibilityView{Gender: "female", DateOfBirth: "2013-01-01"} // 13 on 2026-07-10
	cases := []struct {
		name    string
		sw      SwimmerEligibilityView
		ev      MeetEvent
		start   string
		wantErr string
	}{
		{"open event accepts anyone", SwimmerEligibilityView{Gender: "non_binary"}, MeetEvent{Gender: "O"}, "2026-07-10", ""},
		{"mixed event accepts anyone", SwimmerEligibilityView{Gender: "undisclosed"}, MeetEvent{Gender: "X"}, "2026-07-10", ""},
		{"matching gender", girl, MeetEvent{Gender: " f "}, "2026-07-10", ""},
		{"wrong gender", girl, MeetEvent{Gender: "M"}, "2026-07-10", "not eligible for a M event"},
		{"unmapped gender in gendered event", SwimmerEligibilityView{Gender: "non_binary"}, MeetEvent{Gender: "F"}, "2026-07-10", "not eligible for a F event"},
		{"age within bounds", girl, MeetEvent{Gender: "F", MinAge: &min12, MaxAge: &max14}, "2026-07-10", ""},
		{"missing dob", SwimmerEligibilityView{Gender: "female"}, MeetEvent{Gender: "F", MinAge: &min12}, "2026-07-10", "date of birth is required"},
		{"below minimum", girl, MeetEvent{Gender: "F", MinAge: &min12}, "2024-07-10", "below the event minimum 12"},
		{"above maximum", girl, MeetEvent{Gender: "F", MaxAge: &min12}, "2026-07-10", "above the event maximum 12"},
	}
	for _, tc := range cases {
		err := validateSwimmerEventEligibility(tc.sw, tc.ev, tc.start)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrIneligibleEntry) || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want ErrIneligibleEntry containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestCov1xValidatorsRejectUnknownValues(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"invitational", "dual_meet", "time_trial", "championship", "other"} {
		if err := validateMeetType(v); err != nil {
			t.Errorf("meet type %q rejected: %v", v, err)
		}
	}
	if err := validateMeetType("party"); err == nil || !strings.Contains(err.Error(), "meet_type must be one of") {
		t.Errorf("meet type party: %v", err)
	}
	if err := validateCourse("SCM"); err != nil {
		t.Errorf("SCM rejected: %v", err)
	}
	if err := validateCourse("scy"); err == nil || !strings.Contains(err.Error(), "course must be one of") {
		t.Errorf("lowercase course: %v", err)
	}
	if err := validateStroke("butterfly"); err != nil {
		t.Errorf("butterfly rejected: %v", err)
	}
	if err := validateStroke("dog_paddle"); err == nil || !strings.Contains(err.Error(), "stroke must be one of") {
		t.Errorf("dog_paddle: %v", err)
	}
	for _, g := range []string{"M", "F", "X", "O"} {
		if err := validateGender(g); err != nil {
			t.Errorf("gender %q rejected: %v", g, err)
		}
	}
	if err := validateGender("Z"); err == nil || !strings.Contains(err.Error(), "gender must be one of") {
		t.Errorf("gender Z: %v", err)
	}
	if got := normalizeChoice("  LCM ", "SCY"); got != "LCM" {
		t.Errorf("normalizeChoice trimmed = %q", got)
	}
	if got := normalizeChoice("   ", "SCY"); got != "SCY" {
		t.Errorf("normalizeChoice default = %q", got)
	}
}

func TestCov1xValidateMeetTransition(t *testing.T) {
	t.Parallel()
	if err := validateMeetTransition("draft", "published"); err != nil {
		t.Fatalf("draft->published: %v", err)
	}
	if err := validateMeetTransition("published", "draft"); err != nil {
		t.Fatalf("published->draft: %v", err)
	}
	if err := validateMeetTransition("draft", "completed"); !errors.Is(err, ErrInvalidTransition) || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("draft->completed: %v", err)
	}
	if err := validateMeetTransition("archived", "draft"); !errors.Is(err, ErrInvalidTransition) || !strings.Contains(err.Error(), "unknown current status") {
		t.Fatalf("archived->draft: %v", err)
	}
}

func TestCov1xNormalizePerfStroke(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"free": "freestyle", "back": "backstroke", "breast": "breaststroke", "fly": "butterfly",
		"im": "individual_medley", "free_relay": "freestyle_relay", "medley_relay": "medley_relay",
		"mystery": "mystery",
	} {
		if got := normalizePerfStroke(in); got != want {
			t.Errorf("normalizePerfStroke(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xMeetEventStrokeFromParsed(t *testing.T) {
	t.Parallel()
	for in, want := range map[meetfiles.Stroke]string{
		meetfiles.StrokeFreestyle:      "freestyle",
		meetfiles.StrokeBackstroke:     "backstroke",
		meetfiles.StrokeBreaststroke:   "breaststroke",
		meetfiles.StrokeButterfly:      "butterfly",
		meetfiles.StrokeIM:             "individual_medley",
		meetfiles.StrokeFreestyleRelay: "freestyle_relay",
		meetfiles.StrokeMedleyRelay:    "medley_relay",
		meetfiles.Stroke("weird"):      "other",
	} {
		if got := meetEventStrokeFromParsed(in); got != want {
			t.Errorf("stroke %q -> %q, want %q", in, got, want)
		}
	}
}

func TestCov1xSplitEventNumberOverflowIsNumberless(t *testing.T) {
	t.Parallel()
	n, rest, ok := splitEventNumber("99999999999999999999999X")
	if ok || n != 0 || rest != "99999999999999999999999X" {
		t.Fatalf("overflow = (%d,%q,%v), want numberless", n, rest, ok)
	}
	n, rest, ok = splitEventNumber(" 12B ")
	if !ok || n != 12 || rest != "B" {
		t.Fatalf("12B = (%d,%q,%v)", n, rest, ok)
	}
}

func TestCov1xDeriveSessionDrafts(t *testing.T) {
	t.Parallel()
	d := func(y int, m time.Month, day int) *time.Time {
		v := time.Date(y, m, day, 15, 0, 0, 0, time.UTC)
		return &v
	}
	if got := deriveSessionDrafts(nil); len(got) != 1 || got[0].Name != "Session 1" || got[0].Section != "Day 1" {
		t.Fatalf("nil definition: %+v", got)
	}
	if got := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{}); len(got) != 1 || got[0].SessionNumber != 1 {
		t.Fatalf("no dates: %+v", got)
	}
	// Declared sessions are passed through, trimmed.
	declared := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{Sessions: []meetfiles.ParsedMeetSession{
		{SessionNumber: 2, Section: " Day 2 ", Name: " Finals ", SessionDate: d(2026, 7, 11), WarmupTime: " 07:00 ", StartTime: " 08:00 "},
		{SessionNumber: 3, Name: "No date"},
	}})
	if len(declared) != 2 || declared[0].Name != "Finals" || declared[0].Section != "Day 2" ||
		declared[0].SessionDate != "2026-07-11" || declared[0].WarmupTime != "07:00" || declared[0].StartTime != "08:00" {
		t.Fatalf("declared sessions: %+v", declared)
	}
	if declared[1].SessionDate != "" {
		t.Fatalf("session without a date must stay blank: %+v", declared[1])
	}
	// One session per meet day; a missing end date reuses the start and vice versa.
	span := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{MeetStartDate: d(2026, 7, 10), MeetEndDate: d(2026, 7, 12)})
	if len(span) != 3 || span[2].SessionDate != "2026-07-12" || span[2].Name != "Session 3" {
		t.Fatalf("3-day span: %+v", span)
	}
	if got := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{MeetStartDate: d(2026, 7, 10)}); len(got) != 1 || got[0].SessionDate != "2026-07-10" {
		t.Fatalf("start only: %+v", got)
	}
	if got := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{MeetEndDate: d(2026, 7, 10)}); len(got) != 1 || got[0].SessionDate != "2026-07-10" {
		t.Fatalf("end only: %+v", got)
	}
	// An end before the start collapses to the start day.
	if got := deriveSessionDrafts(&meetfiles.ParsedMeetDefinition{MeetStartDate: d(2026, 7, 10), MeetEndDate: d(2026, 7, 1)}); len(got) != 1 || got[0].SessionDate != "2026-07-10" {
		t.Fatalf("reversed range: %+v", got)
	}
}

func TestCov1xDeriveEventDrafts(t *testing.T) {
	t.Parallel()
	if got := deriveEventDrafts(nil); got != nil {
		t.Fatalf("nil definition = %v", got)
	}
	if got := deriveEventDrafts(&meetfiles.ParsedMeetDefinition{}); got != nil {
		t.Fatalf("no events = %v", got)
	}
	min := 9
	got := deriveEventDrafts(&meetfiles.ParsedMeetDefinition{Events: []meetfiles.ParsedMeetEvent{{
		SessionNumber: 2, EventNumber: " 5 ", Stroke: meetfiles.StrokeBackstroke, Distance: 100,
		Course: meetfiles.CourseShortCourseYards, Gender: " F ", MinAge: &min, IsRelay: true,
	}}})
	if len(got) != 1 {
		t.Fatalf("events: %+v", got)
	}
	e := got[0]
	if e.SessionNumber != 2 || e.EventNumber != "5" || e.Stroke != "backstroke" || e.Distance != 100 ||
		e.Course != string(meetfiles.CourseShortCourseYards) || e.Gender != "F" || e.MinAge != &min || !e.IsRelay {
		t.Fatalf("draft = %+v", e)
	}
}

func TestCov1xParseWarningsCapsAndSkipsBlank(t *testing.T) {
	t.Parallel()
	if got := parseWarnings(nil); got != nil {
		t.Fatalf("no errors = %v", got)
	}
	var errs []meetfiles.ParseError
	errs = append(errs, meetfiles.ParseError{Message: "  "})
	for _, m := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		errs = append(errs, meetfiles.ParseError{Message: m})
	}
	got := parseWarnings(errs)
	// Only the first five errors are considered; the blank one among them is dropped.
	if strings.Join(got, ",") != "a,b,c,d" {
		t.Fatalf("warnings = %v", got)
	}
}

func TestCov1xRandomTokenIsHexAndUnique(t *testing.T) {
	t.Parallel()
	a, b := randomToken(12), randomToken(12)
	if len(a) != 24 || a == b {
		t.Fatalf("tokens %q %q", a, b)
	}
	for _, r := range a {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("non-hex rune %q in %q", r, a)
		}
	}
}

func TestCov1xNewIDInMemoryUsesPrefix(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if id := svc.newID("meet_"); !strings.HasPrefix(id, "meet_") || len(id) != len("meet_")+24 {
		t.Fatalf("id = %q", id)
	}
}
