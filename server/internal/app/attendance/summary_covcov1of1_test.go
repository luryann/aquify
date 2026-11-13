package attendance

import (
	"errors"
	"fmt"
	"testing"

	"github.com/luryann/aquify/server/internal/app/roster"
)

type cov1xSource struct {
	practices    []Practice
	practicesErr error
	records      map[string][]Record
	attErr       error
}

func (s cov1xSource) Practices(string) ([]Practice, error) { return s.practices, s.practicesErr }
func (s cov1xSource) Attendance(_, id string) ([]Record, error) {
	return s.records[id], s.attErr
}

type cov1xBulkSource struct {
	cov1xSource
	bulk     map[string]string
	bulkErr  error
	gotIDs   []string
	gotSwimr string
}

func (s *cov1xBulkSource) AttendanceForSwimmer(_, swimmerID string, ids []string) (map[string]string, error) {
	s.gotSwimr = swimmerID
	s.gotIDs = ids
	return s.bulk, s.bulkErr
}

func cov1xFixtures() (*roster.StaticDirectory, roster.Swimmer) {
	return &roster.StaticDirectory{}, roster.Swimmer{ID: "s1", GroupCodes: []string{"SENIOR"}}
}

func TestCov1xSummarizeInvalidRange(t *testing.T) {
	dir, sw := cov1xFixtures()
	cases := map[string][2]string{
		"bad from":      {"nope", "2026-10-01"},
		"bad to":        {"2026-10-01", "10/02/2026"},
		"from after to": {"2026-10-05", "2026-10-01"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			sum, err := Summarize(cov1xSource{}, dir, "org", sw, c[0], c[1])
			if !errors.Is(err, ErrInvalidRange) {
				t.Fatalf("err = %v, want ErrInvalidRange", err)
			}
			if sum.SwimmerID != "s1" || sum.Recent == nil || len(sum.Recent) != 0 {
				t.Fatalf("unexpected partial summary %+v", sum)
			}
		})
	}
}

func TestCov1xSummarizePropagatesSourceErrors(t *testing.T) {
	dir, sw := cov1xFixtures()
	boom := errors.New("boom")
	p := []Practice{{ID: "p1", Date: "2026-10-01", Type: "water"}}

	if _, err := Summarize(cov1xSource{practicesErr: boom}, dir, "org", sw, "2026-10-01", "2026-10-02"); !errors.Is(err, boom) {
		t.Fatalf("practices err = %v", err)
	}
	if _, err := Summarize(cov1xSource{practices: p, attErr: boom}, dir, "org", sw, "2026-10-01", "2026-10-02"); !errors.Is(err, boom) {
		t.Fatalf("attendance err = %v", err)
	}
	bulk := &cov1xBulkSource{cov1xSource: cov1xSource{practices: p}, bulkErr: boom}
	if _, err := Summarize(bulk, dir, "org", sw, "2026-10-01", "2026-10-02"); !errors.Is(err, boom) {
		t.Fatalf("bulk err = %v", err)
	}
}

func TestCov1xSummarizeBulkSource(t *testing.T) {
	dir, sw := cov1xFixtures()
	src := &cov1xBulkSource{
		cov1xSource: cov1xSource{
			practices: []Practice{
				{ID: "p1", Date: "2026-10-01", Type: "water"},
				{ID: "p2", Date: "2026-10-02", Type: "water"},
				{ID: "p3", Date: "2026-10-03", Type: "water"},
				{ID: "p4", Date: "2026-10-04", Type: "water"},
				{ID: "p5", Date: "2026-10-05", Type: "rest"},
			},
			// must not be consulted when the bulk read is available
			attErr: errors.New("per-practice read must not be used"),
		},
		bulk: map[string]string{"p1": "present", "p2": "absent", "p3": "excused"},
	}
	sum, err := Summarize(src, dir, "org", sw, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if src.gotSwimr != "s1" || fmt.Sprint(src.gotIDs) != "[p1 p2 p3 p4]" {
		t.Fatalf("bulk called with %q %v", src.gotSwimr, src.gotIDs)
	}
	if sum.EligiblePractices != 4 || sum.Present != 1 || sum.Absent != 1 || sum.Excused != 1 || sum.Unmarked != 1 {
		t.Fatalf("counts %+v", sum)
	}
	if sum.AttendancePct == nil || *sum.AttendancePct != 50 {
		t.Fatalf("pct %v", sum.AttendancePct)
	}
	if sum.Recent[0].PracticeID != "p4" || sum.Recent[0].Status != "unmarked" {
		t.Fatalf("recent %+v", sum.Recent)
	}
}

func TestCov1xSummarizeBulkSkippedWhenNoEligible(t *testing.T) {
	dir, sw := cov1xFixtures()
	src := &cov1xBulkSource{bulkErr: errors.New("must not be called")}
	sum, err := Summarize(src, dir, "org", sw, "2026-10-01", "2026-10-02")
	if err != nil || sum.EligiblePractices != 0 || sum.AttendancePct != nil {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
}

func TestCov1xSummarizeCapsRecent(t *testing.T) {
	dir, sw := cov1xFixtures()
	var ps []Practice
	for d := 1; d <= RecentLimit+3; d++ {
		ps = append(ps, Practice{ID: fmt.Sprintf("p%02d", d), Date: fmt.Sprintf("2026-10-%02d", d), Type: "water"})
	}
	sum, err := Summarize(cov1xSource{practices: ps}, dir, "org", sw, "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if sum.EligiblePractices != RecentLimit+3 || sum.Unmarked != RecentLimit+3 {
		t.Fatalf("counts %+v", sum)
	}
	if len(sum.Recent) != RecentLimit || sum.Recent[0].Date != "2026-10-15" || sum.Recent[RecentLimit-1].Date != "2026-10-04" {
		t.Fatalf("recent %+v", sum.Recent)
	}
}
