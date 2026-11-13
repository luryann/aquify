package performance

import (
	"errors"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

type cov2xSwim struct {
	swimmer, meet, course, stroke string
	distance, ms                  int
}

func cov2xFixture(t *testing.T) *Service {
	t.Helper()
	s := NewService()
	s.SetDirectory(&fakeDirectory{
		swimmers: []SwimmerProfile{
			{ID: "amy", HouseholdID: "hh-1", FirstName: "Amy", LastName: "Ames", DateOfBirth: "2014-06-01", Gender: "female", GroupCodes: []string{"AGE"}},
			{ID: "bea", HouseholdID: "hh-2", FirstName: "Bea", LastName: "Bell", DateOfBirth: "2013-01-15", Gender: "female", GroupCodes: []string{"SENIOR"}},
			{ID: "dan", HouseholdID: "hh-3", FirstName: "Dan", LastName: "Dole", DateOfBirth: "2012-01-15", Gender: "male", GroupCodes: []string{"SENIOR"}},
			{ID: "eve", HouseholdID: "hh-4", FirstName: "Eve", LastName: "Eld", PreferredName: "Evie", DateOfBirth: "2012-03-15", Gender: "female", GroupCodes: []string{"SENIOR"}},
			{ID: "fay", HouseholdID: "hh-5", FirstName: "Fay", LastName: "Fox", DateOfBirth: "2012-03-15", Gender: "female"},
		},
		meets: []MeetInfo{
			{ID: "mA", Name: "Fall Classic", StartDate: "2025-01-10", Course: "SCY"},
			{ID: "mB", Name: "Winter Champs", StartDate: "2026-01-10", Course: "SCY"},
			{ID: "mA2", Name: "Fall Classic II", StartDate: "2025-01-10", Course: "SCY"},
		},
	})
	s.SetHouseholdChecker(fakeHouseholds{"amy": "hh-1", "bea": "hh-2", "dan": "hh-3", "eve": "hh-4", "ghost": "hh-9"})
	return s
}

func cov2xAdd(t *testing.T, s *Service, principal *appmiddleware.Principal, org string, sw cov2xSwim) {
	t.Helper()
	ms := sw.ms
	gender := "F"
	if sw.swimmer == "dan" {
		gender = "M"
	}
	if _, err := s.UpsertResult(org, principal.UserID, principal, UpsertResultInput{
		SwimmerID: sw.swimmer, MeetID: sw.meet, Distance: sw.distance, Stroke: sw.stroke, Course: sw.course,
		Gender: gender, FinTimeMs: &ms, IsOfficial: true,
	}); err != nil {
		t.Fatalf("upsert %+v: %v", sw, err)
	}
}

func cov2xSeedProgress(t *testing.T, s *Service) {
	t.Helper()
	otherOrg := &appmiddleware.Principal{UserID: "coach2", OrgID: "other", Roles: []string{"coach"}}
	for _, sw := range []cov2xSwim{
		// Amy: several swims per period (the fastest wins), a new event, and a second course.
		{"amy", "mA", "SCY", "free", 100, 64000},
		{"amy", "mA2", "SCY", "free", 100, 62000},
		{"amy", "mA", "SCY", "free", 100, 63000}, // slower than 62000: ignored
		{"amy", "mB", "SCY", "free", 100, 61000},
		{"amy", "mB", "SCY", "free", 100, 60000},
		{"amy", "mB", "SCY", "free", 100, 60500}, // slower than 60000: ignored
		{"amy", "mA", "SCY", "free", 50, 30000},
		{"amy", "mB", "SCY", "free", 50, 29000},
		{"amy", "mA", "SCY", "back", 100, 70000},
		{"amy", "mB", "SCY", "back", 100, 69000},
		{"amy", "mA", "SCM", "free", 100, 70000},
		{"amy", "mB", "SCM", "free", 100, 69000},
		{"amy", "mB", "SCY", "fly", 100, 80000}, // only in B: counted as a new event
		// Bea got slower.
		{"bea", "mA", "SCY", "free", 100, 50000},
		{"bea", "mB", "SCY", "free", 100, 51000},
		// Dan: one event, 10% faster.
		{"dan", "mA", "SCY", "free", 100, 50000},
		{"dan", "mB", "SCY", "free", 100, 45000},
		// Evie: two events, both 10% faster (ties Dan on average, beats him on improved count).
		{"eve", "mA", "SCY", "free", 100, 100000},
		{"eve", "mB", "SCY", "free", 100, 90000},
		{"eve", "mA", "SCY", "free", 50, 50000},
		{"eve", "mB", "SCY", "free", 50, 45000},
		// Fay swam only in B: new event, nothing compared.
		{"fay", "mB", "SCY", "free", 100, 55000},
		// Not in the directory at all.
		{"ghost", "mA", "SCY", "free", 100, 40000},
		{"ghost", "mB", "SCY", "free", 100, 39000},
	} {
		cov2xAdd(t, s, coachP, "org", sw)
	}
	// Another club's swim must never leak into this club's report.
	cov2xAdd(t, s, otherOrg, "other", cov2xSwim{"amy", "mA", "SCY", "free", 100, 99000})
	cov2xAdd(t, s, otherOrg, "other", cov2xSwim{"amy", "mB", "SCY", "free", 100, 1000})
}

func cov2xIn() ProgressReportInput {
	return ProgressReportInput{FromA: "2025-01-01", ToA: "2025-12-31", FromB: "2026-01-01", ToB: "2026-12-31"}
}

func cov2xByID(r *ProgressReport, id string) *SwimmerProgress {
	for i := range r.Swimmers {
		if r.Swimmers[i].SwimmerID == id {
			return &r.Swimmers[i]
		}
	}
	return nil
}

func TestCov2xProgressReportStaffView(t *testing.T) {
	s := cov2xFixture(t)
	cov2xSeedProgress(t, s)
	rep, err := s.ProgressReport("org", coachP, cov2xIn())
	if err != nil {
		t.Fatal(err)
	}
	if rep.PeriodA.From != "2025-01-01" || rep.PeriodB.To != "2026-12-31" {
		t.Fatalf("periods: %+v %+v", rep.PeriodA, rep.PeriodB)
	}
	if len(rep.Swimmers) != 6 {
		t.Fatalf("expected 6 swimmers, got %d: %+v", len(rep.Swimmers), rep.Swimmers)
	}
	// Sorted by display name: Amy, Bea, Dan, Evie, Fay, then "Unknown swimmer".
	var names []string
	for _, sp := range rep.Swimmers {
		names = append(names, sp.SwimmerName)
	}
	want := []string{"Amy Ames", "Bea Bell", "Dan Dole", "Evie Eld", "Fay Fox", "Unknown swimmer"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("name order: got %v want %v", names, want)
		}
	}

	amy := cov2xByID(rep, "amy")
	if amy.EventsCompared != 4 || amy.EventsImproved != 4 || amy.NewEvents != 1 {
		t.Fatalf("amy counts: %+v", amy)
	}
	// Events are ordered course, stroke, distance: SCY free 50, SCY free 100, SCY back 100, SCM free 100.
	type ek struct {
		course, stroke string
		dist           int
	}
	wantOrder := []ek{{"SCY", "free", 50}, {"SCY", "free", 100}, {"SCY", "back", 100}, {"SCM", "free", 100}}
	for i, w := range wantOrder {
		e := amy.Events[i]
		if e.Course != w.course || e.Stroke != w.stroke || e.Distance != w.dist {
			t.Fatalf("amy event %d: %+v want %+v", i, e, w)
		}
	}
	free100 := amy.Events[1]
	if free100.BeforeMs != 62000 || free100.AfterMs != 60000 || free100.DropMs != 2000 || free100.Percent != 3.23 {
		t.Fatalf("amy free 100 should compare best 62000 -> 60000: %+v", free100)
	}
	if amy.TotalDropMs != 1000+2000+1000+1000 {
		t.Fatalf("amy total drop: %d", amy.TotalDropMs)
	}
	if len(amy.GroupCodes) != 1 || amy.GroupCodes[0] != "AGE" {
		t.Fatalf("amy groups: %v", amy.GroupCodes)
	}

	bea := cov2xByID(rep, "bea")
	if bea.EventsCompared != 1 || bea.EventsImproved != 0 || bea.TotalDropMs != -1000 || bea.AvgPercent != -2 {
		t.Fatalf("bea got slower: %+v", bea)
	}
	fay := cov2xByID(rep, "fay")
	if fay.EventsCompared != 0 || fay.NewEvents != 1 || fay.AvgPercent != 0 || len(fay.Events) != 0 {
		t.Fatalf("fay: %+v", fay)
	}
	ghost := cov2xByID(rep, "ghost")
	if ghost.SwimmerName != "Unknown swimmer" || len(ghost.GroupCodes) != 0 || ghost.EventsCompared != 1 {
		t.Fatalf("ghost: %+v", ghost)
	}

	// Most improved: Evie and Dan tie on 10% average, Evie improved more events;
	// Amy (avg 3.4%) next, then the unknown swimmer. Bea (slower) and Fay (nothing compared) are excluded.
	var ranked []string
	for _, sp := range rep.MostImproved {
		ranked = append(ranked, sp.SwimmerID)
	}
	wantRank := []string{"eve", "dan", "ghost", "amy"}
	if len(ranked) != len(wantRank) {
		t.Fatalf("most improved: got %v want %v", ranked, wantRank)
	}
	for i := range wantRank {
		if ranked[i] != wantRank[i] {
			t.Fatalf("most improved: got %v want %v", ranked, wantRank)
		}
	}
}

func TestCov2xProgressReportFilters(t *testing.T) {
	s := cov2xFixture(t)
	cov2xSeedProgress(t, s)

	in := cov2xIn()
	in.Course = " scm "
	rep, err := s.ProgressReport("org", adminP, in)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Course != "SCM" || len(rep.Swimmers) != 1 || rep.Swimmers[0].SwimmerID != "amy" || rep.Swimmers[0].EventsCompared != 1 {
		t.Fatalf("course filter: %+v", rep)
	}

	in = cov2xIn()
	in.SwimmerID = "dan"
	rep, _ = s.ProgressReport("org", coachP, in)
	if len(rep.Swimmers) != 1 || rep.Swimmers[0].SwimmerID != "dan" {
		t.Fatalf("swimmer filter: %+v", rep.Swimmers)
	}

	in = cov2xIn()
	in.GroupCode = " senior "
	rep, _ = s.ProgressReport("org", coachP, in)
	if rep.GroupCode != "SENIOR" || len(rep.Swimmers) != 3 {
		t.Fatalf("group filter should keep bea, dan, eve: %+v", rep.Swimmers)
	}
	for _, sp := range rep.Swimmers {
		if sp.SwimmerID == "amy" || sp.SwimmerID == "ghost" || sp.SwimmerID == "fay" {
			t.Fatalf("swimmer outside group leaked: %s", sp.SwimmerID)
		}
	}

	in = cov2xIn()
	in.Limit = 1
	rep, _ = s.ProgressReport("org", coachP, in)
	if len(rep.MostImproved) != 1 || rep.MostImproved[0].SwimmerID != "eve" {
		t.Fatalf("limit 1: %+v", rep.MostImproved)
	}
	in.Limit = 500 // clamped to 50, still returns everyone who improved
	rep, _ = s.ProgressReport("org", coachP, in)
	if len(rep.MostImproved) != 4 {
		t.Fatalf("clamped limit: %d", len(rep.MostImproved))
	}
}

func TestCov2xProgressReportParentSeesOnlyOwnHousehold(t *testing.T) {
	s := cov2xFixture(t)
	cov2xSeedProgress(t, s)
	rep, err := s.ProgressReport("org", parentP, cov2xIn())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Swimmers) != 1 || rep.Swimmers[0].SwimmerID != "amy" {
		t.Fatalf("parent in hh-1 must see only amy: %+v", rep.Swimmers)
	}
	// A parent without a household sees nothing.
	lost := &appmiddleware.Principal{UserID: "p2", OrgID: "org", Roles: []string{"parent"}}
	rep, err = s.ProgressReport("org", lost, cov2xIn())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Swimmers) != 0 || len(rep.MostImproved) != 0 {
		t.Fatalf("household-less parent must see nothing: %+v", rep.Swimmers)
	}
}

func TestCov2xProgressReportValidation(t *testing.T) {
	s := cov2xFixture(t)
	if _, err := s.ProgressReport("org", nil, cov2xIn()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal: %v", err)
	}
	if _, err := s.ProgressReport("org", &appmiddleware.Principal{UserID: "x", Roles: []string{"guest"}}, cov2xIn()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unknown role: %v", err)
	}
	cases := map[string]func(*ProgressReportInput){
		"baseline range needs":       func(in *ProgressReportInput) { in.FromA = "" },
		"baseline range starts":      func(in *ProgressReportInput) { in.FromA, in.ToA = "2025-12-31", "2025-01-01" },
		"comparison range needs":     func(in *ProgressReportInput) { in.ToB = "garbage" },
		"comparison range starts":    func(in *ProgressReportInput) { in.FromB, in.ToB = "2026-12-31", "2026-01-01" },
		"course must be SCY, SCM or": func(in *ProgressReportInput) { in.Course = "YARDS" },
	}
	for wantMsg, mutate := range cases {
		in := cov2xIn()
		mutate(&in)
		_, err := s.ProgressReport("org", coachP, in)
		if !errors.Is(err, ErrRecordInvalid) {
			t.Errorf("%s: want ErrRecordInvalid, got %v", wantMsg, err)
			continue
		}
		if !cov2xContainsSubstr(err.Error(), wantMsg) {
			t.Errorf("error %q should mention %q", err, wantMsg)
		}
	}
}

func cov2xContainsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
