package performance

import (
	"errors"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestCov2xTopTimesQueryNormalize(t *testing.T) {
	q := TopTimesQuery{Course: " scm ", Gender: " f ", AgeGroup: "open", Stroke: " Free ", From: "2025-01-05", To: "2025-02-01", Limit: 500}
	if err := q.normalize(); err != nil {
		t.Fatal(err)
	}
	if q.Course != "SCM" || q.Gender != "F" || q.AgeGroup != AgeGroupOpen || q.Stroke != "free" ||
		q.From != "2025-01-05" || q.To != "2025-02-01" || q.Limit != 100 {
		t.Fatalf("normalized query: %+v", q)
	}
	d := TopTimesQuery{}
	if err := d.normalize(); err != nil || d.Course != "SCY" || d.AgeGroup != AgeGroupOpen || d.Limit != 10 {
		t.Fatalf("defaults: %v %+v", err, d)
	}
	for name, bad := range map[string]TopTimesQuery{
		"course":   {Course: "XYZ"},
		"gender":   {Gender: "Q"},
		"age":      {AgeGroup: "bogus"},
		"stroke":   {Stroke: "dive"},
		"date":     {From: "not-a-date"},
		"to date":  {To: "2025-13-45"},
		"reversed": {From: "2025-06-01", To: "2025-05-01"},
	} {
		bad := bad
		if err := bad.normalize(); !errors.Is(err, ErrRecordInvalid) {
			t.Errorf("%s: want ErrRecordInvalid, got %v", name, err)
		}
	}
}

func TestCov2xTopTimesAccessControl(t *testing.T) {
	s := cov2xFixture(t)
	if _, err := s.TopTimes("org", nil, TopTimesQuery{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal: %v", err)
	}
	if _, err := s.TopTimes("org", &appmiddleware.Principal{UserID: "x", Roles: []string{"guest"}}, TopTimesQuery{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unknown role: %v", err)
	}
}

func TestCov2xTopTimesFiltersOrderingAndTies(t *testing.T) {
	s := cov2xFixture(t)
	otherOrg := &appmiddleware.Principal{UserID: "coach2", OrgID: "other", Roles: []string{"coach"}}
	for _, sw := range []cov2xSwim{
		{"amy", "mA", "SCY", "free", 100, 60000},
		{"bea", "mA", "SCY", "free", 100, 60000}, // same time and day as Amy: tie
		{"dan", "mA", "SCY", "free", 100, 55000},
		{"eve", "mA", "SCY", "free", 100, 65000},
		{"amy", "mA", "SCY", "back", 100, 70000},
		{"amy", "mA", "SCY", "free", 50, 28000},
		{"amy", "mA", "SCM", "free", 100, 1000}, // other course
	} {
		cov2xAdd(t, s, coachP, "org", sw)
	}
	cov2xAdd(t, s, otherOrg, "other", cov2xSwim{"amy", "mA", "SCY", "free", 100, 100}) // other club

	lists, err := s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY"})
	if err != nil {
		t.Fatal(err)
	}
	// Lists are per gender/event, sorted by gender then event.
	var labels []string
	for _, l := range lists {
		labels = append(labels, l.Label)
		if l.Entries == nil {
			t.Fatalf("entries must be non-nil: %+v", l)
		}
	}
	if len(lists) != 4 {
		t.Fatalf("want 4 lists (F free50, F free100, F back100, M free100), got %v", labels)
	}
	for i := 1; i < len(lists); i++ {
		if !lessRecordKey(lists[i-1].RecordKey, lists[i].RecordKey) {
			t.Fatalf("lists out of order: %v", labels)
		}
	}

	f100 := cov2xTopList(t, lists, "F", 100, "free")
	if len(f100.Entries) != 3 {
		t.Fatalf("F 100 free entries: %+v", f100.Entries)
	}
	e := f100.Entries
	if e[0].Rank != 1 || e[1].Rank != 1 || e[2].Rank != 3 || e[2].SwimmerID != "eve" || e[2].TimeMs != 65000 {
		t.Fatalf("ties share a rank and the next rank skips: %+v", e)
	}
	// Equal time and date: ordered by result id.
	if e[0].ResultID >= e[1].ResultID {
		t.Fatalf("tie must break by result id: %+v", e[:2])
	}
	if got := map[string]bool{e[0].SwimmerID: true, e[1].SwimmerID: true}; !got["amy"] || !got["bea"] {
		t.Fatalf("tied swimmers: %+v", e[:2])
	}
	if m := cov2xTopList(t, lists, "M", 100, "free"); len(m.Entries) != 1 || m.Entries[0].SwimmerID != "dan" || m.Entries[0].MeetName != "Fall Classic" {
		t.Fatalf("male list: %+v", m.Entries)
	}

	// Filters.
	got, err := s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY", Gender: "M"})
	if err != nil || len(got) != 1 || got[0].Gender != "M" {
		t.Fatalf("gender filter: %v %+v", err, got)
	}
	got, _ = s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY", Distance: 50})
	if len(got) != 1 || got[0].Distance != 50 {
		t.Fatalf("distance filter: %+v", got)
	}
	got, _ = s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY", Stroke: "back"})
	if len(got) != 1 || got[0].Stroke != "back" || got[0].Entries[0].TimeMs != 70000 {
		t.Fatalf("stroke filter: %+v", got)
	}
	got, _ = s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY", Gender: "F", Distance: 100, Stroke: "free", Limit: 1})
	if len(got) != 1 || len(got[0].Entries) != 1 {
		t.Fatalf("limit: %+v", got)
	}
	got, _ = s.TopTimes("org", coachP, TopTimesQuery{Course: "LCM"})
	if len(got) != 0 {
		t.Fatalf("no LCM swims expected: %+v", got)
	}
}

func cov2xTopList(t *testing.T, lists []TopTimesList, gender string, dist int, stroke string) TopTimesList {
	t.Helper()
	for _, l := range lists {
		if l.Gender == gender && l.Distance == dist && l.Stroke == stroke {
			return l
		}
	}
	t.Fatalf("no list %s %d %s in %+v", gender, dist, stroke, lists)
	return TopTimesList{}
}
