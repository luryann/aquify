package training

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func cov1xValidSet() WorkoutSet { return WorkoutSet{Repeats: 1, Distance: 100, Stroke: "free"} }

func cov1xWith(sets ...WorkoutSet) WorkoutInput {
	return WorkoutInput{Title: "T", Sections: []WorkoutSection{{Kind: "main", Sets: sets}}}
}

func TestCov1xNormalizeWorkoutInputFieldErrors(t *testing.T) {
	t.Parallel()
	many := make([]string, MaxTags+1)
	for i := range many {
		many[i] = "tag" + string(rune('a'+i))
	}
	tooManySets := make([]WorkoutSet, MaxSetsPerSection+1)
	for i := range tooManySets {
		tooManySets[i] = cov1xValidSet()
	}
	tooManySections := make([]WorkoutSection, MaxSections+1)
	longNotes := strings.Repeat("n", MaxNotesLength+1)

	cases := []struct {
		name  string
		in    WorkoutInput
		field string
	}{
		{"title too long", WorkoutInput{Title: strings.Repeat("t", MaxTitleLength+1)}, "title"},
		{"description too long", WorkoutInput{Title: "T", Description: strings.Repeat("d", 4001)}, "description"},
		{"bad course", WorkoutInput{Title: "T", Course: "XYZ"}, "course"},
		{"tag too long", WorkoutInput{Title: "T", Tags: []string{strings.Repeat("x", MaxTagLength+1)}}, "tags"},
		{"too many tags", WorkoutInput{Title: "T", Tags: many}, "tags"},
		{"too many sections", WorkoutInput{Title: "T", Sections: tooManySections}, "sections"},
		{"section title too long", WorkoutInput{Title: "T", Sections: []WorkoutSection{{Title: strings.Repeat("s", MaxTitleLength+1)}}}, "sections[0].title"},
		{"section notes too long", WorkoutInput{Title: "T", Sections: []WorkoutSection{{Notes: longNotes}}}, "sections[0].notes"},
		{"too many sets in section", WorkoutInput{Title: "T", Sections: []WorkoutSection{{Sets: tooManySets}}}, "sections[0].sets"},
		{"bad section kind", WorkoutInput{Title: "T", Sections: []WorkoutSection{{Kind: "nope"}}}, "sections[0].kind"},
		{"negative rest", cov1xWith(WorkoutSet{Distance: 100, RestSeconds: -1}), "sections[0].sets[0].rest_seconds"},
		{"rest too big", cov1xWith(WorkoutSet{Distance: 100, RestSeconds: MaxIntervalSeconds + 1}), "sections[0].sets[0].rest_seconds"},
		{"set notes too long", cov1xWith(WorkoutSet{Distance: 100, Notes: longNotes}), "sections[0].sets[0].notes"},
		{"round with too many children", cov1xWith(WorkoutSet{Repeats: 2, Sets: tooManySets}), "sections[0].sets[0].sets"},
		{"total distance over limit", WorkoutInput{Title: "T", Sections: []WorkoutSection{
			{Kind: "main", Sets: []WorkoutSet{{Repeats: 99, Distance: 10000}}},
		}}, "sections"},
	}
	for _, tc := range cases {
		_, err := NormalizeWorkoutInput(tc.in)
		var ve *WorkoutValidationError
		if !errors.Is(err, ErrInvalidWorkout) || !errors.As(err, &ve) {
			t.Errorf("%s: err = %v, want a WorkoutValidationError", tc.name, err)
			continue
		}
		if ve.Field != tc.field {
			t.Errorf("%s: field = %q, want %q (%v)", tc.name, ve.Field, tc.field, err)
		}
	}
}

func TestCov1xNormalizeSectionDefaultsKindToCustom(t *testing.T) {
	t.Parallel()
	got, err := NormalizeSections([]WorkoutSection{{Kind: "  ", Sets: []WorkoutSet{cov1xValidSet()}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "custom" {
		t.Fatalf("sections = %+v, want kind custom", got)
	}
}

func TestCov1xComputeTotalsDefaultsAndFallbacks(t *testing.T) {
	t.Parallel()
	// Empty course behaves as SCY; unnormalised zero repeats count as one rep;
	// an unknown stroke uses the 95s/100 fallback pace.
	totals := ComputeTotals([]WorkoutSection{{Kind: "main", Sets: []WorkoutSet{
		{Repeats: 0, Distance: 100, Stroke: "weird"},
	}}}, "")
	if totals.Unit != "yd" || totals.Distance != 100 || totals.Lengths != 4 {
		t.Fatalf("totals = %+v", totals)
	}
	if totals.ByStroke["weird"] != 100 {
		t.Fatalf("by stroke = %v", totals.ByStroke)
	}
	if totals.EstimatedSeconds != 95 {
		t.Fatalf("estimated seconds = %d, want 95 (fallback pace)", totals.EstimatedSeconds)
	}
	if got := estimateSwimSeconds(200, "free"); got != 180 {
		t.Fatalf("free 200 = %d, want 180", got)
	}
}

func TestCov1xSectionKindForHeader(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"warm-up": "warmup", "wu": "warmup", "pre-set": "pre_set", "main set": "main",
		"kick": "kick", "pull set": "pull", "drills": "drill", "drill": "drill",
		"warm down": "cooldown", "cooldown": "cooldown", "swim-down": "cooldown", "cd": "cooldown",
	} {
		if got := sectionKindForHeader(in); got != want {
			t.Errorf("sectionKindForHeader(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xParseClock(t *testing.T) {
	t.Parallel()
	ok := map[string]int{"1:30": 90, ":45": 45, "90": 90, " 2:05 ": 125, "0:00": 0}
	for in, want := range ok {
		got, err := parseClock(in)
		if err != nil || got != want {
			t.Errorf("parseClock(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", "x:30", "1:75", "1:ab", "abc"} {
		if _, err := parseClock(in); err == nil {
			t.Errorf("parseClock(%q) error = nil", in)
		}
	}
}

func TestCov1xParseQuickEntryHeadersAndEmptySections(t *testing.T) {
	t.Parallel()
	text := "Warmup:\nPre-set: 4x50 free\nPull set\n4x100 pull\nDrills 8x25 drill\nMain:\n"
	res := ParseQuickEntry(text)
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %+v", res.Errors)
	}
	var kinds []string
	for _, sec := range res.Sections {
		kinds = append(kinds, sec.Kind)
	}
	// The empty "Warmup" and "Main" sections are dropped.
	if want := []string{"pre_set", "pull", "drill"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
}

func TestCov1xParseQuickEntryRoundHeaderWithInlineContent(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("3 rounds: 4x100 free @1:30\n  200 easy")
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %+v", res.Errors)
	}
	round := res.Sections[0].Sets[0]
	if round.Repeats != 3 || len(round.Sets) != 2 || round.Sets[0].Distance != 100 || round.Sets[1].Zone != "REC" {
		t.Fatalf("round = %+v", round)
	}
}

func TestCov1xParseQuickEntryNestingLimitBrace(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("2 x {\n2 x {\n2 x {\n100\n}\n}\n}")
	var nesting, closing bool
	for _, e := range res.Errors {
		if strings.Contains(e.Message, "nested at most 2 levels") && e.Line == 3 {
			nesting = true
		}
		if strings.Contains(e.Message, "closing bracket without an open round") {
			closing = true
		}
	}
	if !nesting || !closing {
		t.Fatalf("errors = %+v, want a line-3 nesting error and an unmatched close", res.Errors)
	}
}

func TestCov1xParseQuickEntryNestingLimitNewlineRounds(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("2 rounds\n  2 rounds\n    2 rounds\n      100")
	if len(res.Errors) != 1 || res.Errors[0].Line != 3 || !strings.Contains(res.Errors[0].Message, "nested at most 2 levels") {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

func TestCov1xParseQuickEntryReportsBadSetLine(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("400 free\n100 200")
	if len(res.Errors) != 1 || res.Errors[0].Line != 2 || !strings.Contains(res.Errors[0].Message, "unexpected number") {
		t.Fatalf("errors = %+v", res.Errors)
	}
	if len(res.Sections) != 1 || len(res.Sections[0].Sets) != 1 {
		t.Fatalf("sections = %+v, want the good line kept", res.Sections)
	}
}

func TestCov1xParseLineIgnoresBlankText(t *testing.T) {
	t.Parallel()
	p := &quickParser{}
	p.parseLine("   ", 0, 1)
	if len(p.sections) != 0 || len(p.errors) != 0 {
		t.Fatalf("blank line changed parser state: %+v", p)
	}
}

func TestCov1xClosingBraceAppliesRoundModifiers(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("2 x {\n100 free\n} fly @1:30 sprint steady")
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %+v", res.Errors)
	}
	round := res.Sections[0].Sets[0]
	// A send-off on a whole round is rest between rounds.
	if round.RestSeconds != 90 || round.Stroke != "fly" || round.Zone != "SP3" || round.Notes != "steady" {
		t.Fatalf("round = %+v", round)
	}

	// Notes accumulate onto existing notes and a nil round is ignored.
	r := &WorkoutSet{Notes: "first"}
	(&quickParser{}).applyRoundModifiers(r, "second")
	if r.Notes != "first second" {
		t.Fatalf("notes = %q", r.Notes)
	}
	(&quickParser{}).applyRoundModifiers(nil, "ignored")
}

func TestCov1xParseModifiersSkipsPunctuationTokens(t *testing.T) {
	t.Parallel()
	m := parseModifiers("free , ;")
	if m.stroke != "free" || m.notes != "" {
		t.Fatalf("modifiers = %+v", m)
	}
}

func TestCov1xFormatTextEquipmentAndCustomHeaders(t *testing.T) {
	t.Parallel()
	got := FormatSetText(WorkoutSet{Repeats: 4, Distance: 50, Stroke: "pull", Equipment: []string{"pull_buoy", "fins"}})
	if got != "4x50 pull buoy fins" {
		t.Fatalf("FormatSetText = %q", got)
	}
	text := FormatWorkoutText([]WorkoutSection{
		{Kind: "custom", Title: "Skills", Sets: []WorkoutSet{{Repeats: 1, Distance: 100}}},
		{Kind: "custom", Sets: []WorkoutSet{{Repeats: 1, Distance: 50}}},
		{Kind: "unknown-kind", Title: "Odd", Sets: []WorkoutSet{{Repeats: 1, Distance: 25}}},
		{Kind: "warmup", Sets: []WorkoutSet{{Repeats: 1, Distance: 200}}},
	})
	want := "Skills:\n100\nSet:\n50\nOdd:\n25\nWarmup:\n200"
	if text != want {
		t.Fatalf("FormatWorkoutText =\n%q\nwant\n%q", text, want)
	}
}

func TestCov1xNormalizePhaseErrors(t *testing.T) {
	t.Parallel()
	base := PhaseInput{GroupID: "g", Phase: "base", StartDate: "2026-01-05", EndDate: "2026-01-11"}
	with := func(f func(*PhaseInput)) PhaseInput { in := base; f(&in); return in }
	cases := []struct {
		name  string
		in    PhaseInput
		field string
	}{
		{"long name", with(func(p *PhaseInput) { p.Name = strings.Repeat("n", 81) }), "name"},
		{"long focus", with(func(p *PhaseInput) { p.Focus = strings.Repeat("f", MaxNotesLength+1) }), "focus"},
		{"bad start", with(func(p *PhaseInput) { p.StartDate = "01/05/2026" }), "start_date"},
		{"bad end", with(func(p *PhaseInput) { p.EndDate = "nope" }), "end_date"},
	}
	for _, tc := range cases {
		_, err := normalizePhase(tc.in, SeasonWindow{})
		var ve *WorkoutValidationError
		if !errors.As(err, &ve) || ve.Field != tc.field {
			t.Errorf("%s: err = %v, want field %s", tc.name, err, tc.field)
		}
	}
	if _, err := parseDate("d", "2026-13-40"); err == nil {
		t.Fatal("parseDate accepted an impossible date")
	}
}

func TestCov1xDateHelpersPassThroughInvalidInput(t *testing.T) {
	t.Parallel()
	if got := WeekStart("garbage"); got != "garbage" {
		t.Fatalf("WeekStart = %q", got)
	}
	if got := addDays("garbage", 3); got != "garbage" {
		t.Fatalf("addDays = %q", got)
	}
	if got := WeekStart("2026-11-08"); got != "2026-11-02" { // a Sunday belongs to the prior Monday's week
		t.Fatalf("WeekStart(Sunday) = %q", got)
	}
}

func TestCov1xNilWorkoutHelpers(t *testing.T) {
	t.Parallel()
	if cloneWorkout(nil) != nil {
		t.Fatal("cloneWorkout(nil) must be nil")
	}
	p := &Practice{YardageTarget: 5}
	derivePractice(p, nil)
	derivePractice(nil, &Workout{})
	if p.YardageTarget != 5 || p.WorkoutID != "" {
		t.Fatalf("derivePractice with nil workout changed the practice: %+v", p)
	}
	if got := zoneYards(nil); len(got) != 0 || got == nil {
		t.Fatalf("zoneYards(nil) = %v", got)
	}
	if got := strokeYards(nil); len(got) != 0 || got == nil {
		t.Fatalf("strokeYards(nil) = %v", got)
	}
}
