package attendance

import (
	"errors"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/roster"
)

type fakeSource struct {
	practices []Practice
	records   map[string][]Record
}

func (f fakeSource) Practices(string) ([]Practice, error) { return f.practices, nil }
func (f fakeSource) Attendance(_ string, id string) ([]Record, error) {
	return f.records[id], nil
}

func TestSummarize(t *testing.T) {
	src := fakeSource{
		practices: []Practice{
			{ID: "p1", Date: "2026-10-01", GroupID: "grp-senior", Type: "water"},
			{ID: "p2", Date: "2026-10-02", GroupID: "SENIOR", Type: "water"},
			{ID: "p3", Date: "2026-10-03", GroupID: "", Type: "dryland"},
			{ID: "p4", Date: "2026-10-04", GroupID: "AGE", Type: "water"},    // other group
			{ID: "p5", Date: "2026-10-05", GroupID: "SENIOR", Type: "rest"},  // rest days excluded
			{ID: "p6", Date: "2026-09-01", GroupID: "SENIOR", Type: "water"}, // outside window
			{ID: "p7", Date: "2026-10-06", GroupID: "SENIOR", Type: "water"}, // unmarked
			{ID: "p8", Date: "2026-10-07", GroupID: "SENIOR", Type: "water"},
		},
		records: map[string][]Record{
			"p1": {{SwimmerID: "s1", Status: "present"}},
			"p2": {{SwimmerID: "s1", Status: "absent"}, {SwimmerID: "s2", Status: "present"}},
			"p3": {{SwimmerID: "s1", Status: "present"}},
			"p4": {{SwimmerID: "s1", Status: "absent"}},
			"p8": {{SwimmerID: "s1", Status: "excused"}},
		},
	}
	dir := &roster.StaticDirectory{GroupIDs: map[string]string{"grp-senior": "senior"}}
	sw := roster.Swimmer{ID: "s1", GroupCodes: []string{"SENIOR"}}
	sum, err := Summarize(src, dir, "org", sw, "2026-09-15", "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	if sum.EligiblePractices != 5 || sum.Present != 2 || sum.Absent != 1 || sum.Excused != 1 || sum.Unmarked != 1 {
		t.Fatalf("unexpected counts %+v", sum)
	}
	if sum.AttendancePct == nil || *sum.AttendancePct != 66.7 {
		t.Fatalf("unexpected pct %v", sum.AttendancePct)
	}
	if len(sum.Recent) != 5 || sum.Recent[0].PracticeID != "p8" {
		t.Fatalf("recent should be newest first: %+v", sum.Recent)
	}
	empty, _ := Summarize(fakeSource{}, dir, "org", sw, "2026-09-15", "2026-10-15")
	if empty.AttendancePct != nil || len(empty.Recent) != 0 {
		t.Fatalf("expected nil pct with no practices: %+v", empty)
	}
}

func TestSummarizeRejectsBadRange(t *testing.T) {
	for _, c := range [][2]string{{"", ""}, {"2026-01-01", ""}, {"nope", "2026-01-02"}, {"2026-02-01", "2026-01-01"}} {
		if _, err := Summarize(nil, nil, "org", roster.Swimmer{}, c[0], c[1]); !errors.Is(err, ErrInvalidRange) {
			t.Fatalf("range %v: err = %v", c, err)
		}
	}
}
