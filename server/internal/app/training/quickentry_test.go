package training

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSetLineVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want WorkoutSet
	}{
		{"8x100 free @1:30 EN2", WorkoutSet{Repeats: 8, Distance: 100, Stroke: "free", IntervalSeconds: 90, Zone: "EN2"}},
		{"8 x 100 Free on 1:30 en2", WorkoutSet{Repeats: 8, Distance: 100, Stroke: "free", IntervalSeconds: 90, Zone: "EN2"}},
		{"8×100fr @ 1:30", WorkoutSet{Repeats: 8, Distance: 100, Stroke: "free", IntervalSeconds: 90}},
		{"400 choice", WorkoutSet{Repeats: 1, Distance: 400, Stroke: "choice"}},
		{"400", WorkoutSet{Repeats: 1, Distance: 400}},
		{"200 easy", WorkoutSet{Repeats: 1, Distance: 200, Zone: "REC"}},
		{"4x50 fly @:50 SP2", WorkoutSet{Repeats: 4, Distance: 50, Stroke: "fly", IntervalSeconds: 50, Zone: "SP2"}},
		{"4x50 @45 sprint", WorkoutSet{Repeats: 4, Distance: 50, IntervalSeconds: 45, Zone: "SP3"}},
		{"10x100 IM r:15 threshold", WorkoutSet{Repeats: 10, Distance: 100, Stroke: "im", RestSeconds: 15, Zone: "EN2"}},
		{"6x200 back rest :20", WorkoutSet{Repeats: 6, Distance: 200, Stroke: "back", RestSeconds: 20}},
		{"6x200 breast :30 rest", WorkoutSet{Repeats: 6, Distance: 200, Stroke: "breast", RestSeconds: 30}},
		{"6x50 kick w/ fins @1:00", WorkoutSet{Repeats: 6, Distance: 50, Stroke: "kick", IntervalSeconds: 60, Equipment: []string{"fins"}}},
		{"4x100 pull paddles pull buoy @1:25 EN1", WorkoutSet{Repeats: 4, Distance: 100, Stroke: "pull", IntervalSeconds: 85, Zone: "EN1", Equipment: []string{"paddles", "pull_buoy"}}},
		{"8x50 fly kick on board @1:05", WorkoutSet{Repeats: 8, Distance: 50, Stroke: "kick", IntervalSeconds: 65, Equipment: []string{"kickboard"}, Notes: "fly"}},
		{"8x25 drill catch-up", WorkoutSet{Repeats: 8, Distance: 25, Stroke: "drill", Notes: "catch-up"}},
		{"5x100 free @1:20 Descend 1-5", WorkoutSet{Repeats: 5, Distance: 100, Stroke: "free", IntervalSeconds: 80, Notes: "Descend 1-5"}},
		{"100m back", WorkoutSet{Repeats: 1, Distance: 100, Stroke: "back"}},
		{"3 x 300 yds free race pace", WorkoutSet{Repeats: 3, Distance: 300, Stroke: "free", Zone: "SP1"}},
		{"1x800 free 10:30", WorkoutSet{Repeats: 1, Distance: 800, Stroke: "free", Notes: "10:30"}},
	}
	for _, tc := range cases {
		got, err := ParseSetLine(tc.in)
		if err != nil {
			t.Errorf("ParseSetLine(%q) error = %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseSetLine(%q)\n got  %+v\n want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseSetLineRejectsBadInput(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"free", "", "x100", "0x100", "200x100", "8x0", "8x20000", "100 200", "4x100 @99:00"} {
		if _, err := ParseSetLine(in); err == nil {
			t.Errorf("ParseSetLine(%q) error = nil, want error", in)
		}
	}
}

func TestParseQuickEntrySectionsAndRounds(t *testing.T) {
	t.Parallel()
	text := `Warmup:
400 choice
8x50 kick w/ fins @1:00

Main set
3 x {4x100 free @1:30 EN2, 100 easy} rest 1:00
2 rounds:
  4x50 fly @:50 SP2
  - 200 free r:20 EN1
Kick 6x100 @2:00
Cooldown: 200 easy
Total: 4,000`
	res := ParseQuickEntry(text)
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %+v", res.Errors)
	}
	kinds := []string{}
	for _, s := range res.Sections {
		kinds = append(kinds, s.Kind)
	}
	if want := []string{"warmup", "main", "kick", "cooldown"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("section kinds = %v, want %v", kinds, want)
	}
	main := res.Sections[1]
	if len(main.Sets) != 2 {
		t.Fatalf("main sets = %+v, want 2 rounds", main.Sets)
	}
	r1 := main.Sets[0]
	if r1.Repeats != 3 || r1.RestSeconds != 60 || len(r1.Sets) != 2 || r1.Sets[0].IntervalSeconds != 90 || r1.Sets[1].Zone != "REC" {
		t.Fatalf("round 1 = %+v", r1)
	}
	r2 := main.Sets[1]
	if r2.Repeats != 2 || len(r2.Sets) != 2 || r2.Sets[0].Stroke != "fly" || r2.Sets[1].RestSeconds != 20 {
		t.Fatalf("round 2 = %+v", r2)
	}

	totals := ComputeTotals(res.Sections, CourseSCY)
	// warmup 400+400, main 3*(400+100)=1500 + 2*(200+200)=800, kick 600, cd 200
	if totals.Distance != 3900 {
		t.Fatalf("total distance = %d, want 3900", totals.Distance)
	}
	if totals.ByStroke["kick"] != 1000 {
		t.Fatalf("kick = %d, want 1000 (400 warmup kick + 600 kick section)", totals.ByStroke["kick"])
	}
	if totals.ByZone["EN2"] != 1200 || totals.ByZone["SP2"] != 400 || totals.ByZone["EN1"] != 400 || totals.ByZone["REC"] != 500 {
		t.Fatalf("by zone = %+v", totals.ByZone)
	}
}

func TestParseQuickEntryMultilineBraceRound(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("4x {\n  100 free @1:20\n  2x {50 back @:45; 50 breast @:50}\n} EN1\n200 easy")
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %+v", res.Errors)
	}
	sets := res.Sections[0].Sets
	if len(sets) != 2 || sets[0].Repeats != 4 || sets[0].Zone != "EN1" || len(sets[0].Sets) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	inner := sets[0].Sets[1]
	if inner.Repeats != 2 || len(inner.Sets) != 2 || inner.Sets[1].Stroke != "breast" {
		t.Fatalf("inner round = %+v", inner)
	}
	totals := ComputeTotals(res.Sections, CourseSCY)
	if totals.Distance != 4*(100+2*100)+200 {
		t.Fatalf("distance = %d", totals.Distance)
	}
	// Inherited zone: everything inside the round counts as EN1.
	if totals.ByZone["EN1"] != 1200 || totals.ByZone["REC"] != 200 {
		t.Fatalf("zones = %+v", totals.ByZone)
	}
}

func TestParseQuickEntryReportsLineErrors(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("400 free\nswim fast please\n3x {100 free\n8x100 @1:30")
	if len(res.Errors) != 2 {
		t.Fatalf("errors = %+v, want bad line + unclosed round", res.Errors)
	}
	if res.Errors[0].Line != 2 || !strings.Contains(res.Errors[0].Message, "distance") {
		t.Fatalf("first error = %+v", res.Errors[0])
	}
	if !strings.Contains(res.Errors[1].Message, "never closed") {
		t.Fatalf("second error = %+v", res.Errors[1])
	}
	// Good lines still parse.
	if len(res.Sections) != 1 || len(res.Sections[0].Sets) != 2 {
		t.Fatalf("sections = %+v", res.Sections)
	}
}

func TestParseQuickEntryCustomSectionAndStrayCloser(t *testing.T) {
	t.Parallel()
	res := ParseQuickEntry("Aerobic block:\n10x100 @1:20 EN1\n}")
	if len(res.Sections) != 1 || res.Sections[0].Kind != "custom" || res.Sections[0].Title != "Aerobic block" {
		t.Fatalf("sections = %+v", res.Sections)
	}
	if len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

func TestParseQuickEntryEmptyAndNestingLimit(t *testing.T) {
	t.Parallel()
	if res := ParseQuickEntry("  \n# just a comment\n"); len(res.Sections) != 0 || len(res.Errors) != 0 {
		t.Fatalf("empty parse = %+v", res)
	}
	res := ParseQuickEntry("2x {2x {2x {100 free}}}")
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0].Message, "nested") {
		t.Fatalf("expected nesting error, got %+v", res.Errors)
	}
}

func TestFormatWorkoutTextRoundTrips(t *testing.T) {
	t.Parallel()
	text := `Warmup:
400 choice
8x50 kick @1:00 fins
Main set:
3x {4x100 free @1:30 EN2; 100 REC} rest 1:00
6x50 fly rest :15 SP3 Fast turns
Cooldown:
200 free REC`
	first := ParseQuickEntry(text)
	if len(first.Errors) != 0 {
		t.Fatalf("errors = %+v", first.Errors)
	}
	rendered := FormatWorkoutText(first.Sections)
	second := ParseQuickEntry(rendered)
	if len(second.Errors) != 0 {
		t.Fatalf("re-parse errors = %+v\n%s", second.Errors, rendered)
	}
	if !reflect.DeepEqual(first.Sections, second.Sections) {
		t.Fatalf("round trip mismatch\nfirst  %+v\nsecond %+v\ntext:\n%s", first.Sections, second.Sections, rendered)
	}
}
