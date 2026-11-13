package performance

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

var cov1xGuest = &appmiddleware.Principal{UserID: "guest", OrgID: "org", Roles: []string{"guest"}}

func cov1xSwim(t *testing.T, s *Service, org, swimmer string, dist int, stroke, course string, ms int, official bool) *Result {
	t.Helper()
	r, err := s.UpsertResult(org, "coach", coachP, UpsertResultInput{
		SwimmerID: swimmer, Distance: dist, Stroke: stroke, Course: course, Gender: "F", FinTimeMs: &ms, IsOfficial: official,
	})
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}
	return r
}

func TestCov1xServiceLoadFailuresPropagate(t *testing.T) {
	s := cov1xUnloaded(t)
	ms := 1000
	calls := map[string]func() error{
		"UpsertResult": func() error {
			_, err := s.UpsertResult("org", "a", coachP, UpsertResultInput{})
			return err
		},
		"UpsertResultWithRecords": func() error {
			_, _, err := s.UpsertResultWithRecords("org", "a", coachP, UpsertResultInput{})
			return err
		},
		"RecomputeBestTime": func() error {
			_, err := s.RecomputeBestTime("org", "a", coachP, "amy", 50, "free", "SCY")
			return err
		},
		"ListBestTimes": func() error { _, err := s.ListBestTimes("org", adminP, "amy", ListBestTimesFilters{}); return err },
		"ListBestTimesForOrg": func() error {
			_, err := s.ListBestTimesForOrg("org", adminP, ListBestTimesFilters{})
			return err
		},
		"OverrideBestTime": func() error {
			_, _, err := s.OverrideBestTime("org", "a", adminP, OverrideBestTimeInput{BestTimeMs: &ms})
			return err
		},
		"ListProgression": func() error {
			_, err := s.ListProgression("org", adminP, "amy", ListResultsFilters{})
			return err
		},
		"ListProgressionForOrg": func() error {
			_, err := s.ListProgressionForOrg("org", adminP, ListResultsFilters{})
			return err
		},
		"CreateTimeStandard": func() error {
			_, err := s.CreateTimeStandard("org", "a", adminP, CreateTimeStandardInput{})
			return err
		},
		"ListTimeStandards": func() error { _, err := s.ListTimeStandards("org", ListTimeStandardsFilters{}); return err },
		"GetTimeStandard":   func() error { _, err := s.GetTimeStandard("org", "x"); return err },
		"UpdateTimeStandard": func() error {
			_, err := s.UpdateTimeStandard("org", "a", "x", adminP, UpdateTimeStandardInput{})
			return err
		},
		"DeleteTimeStandard": func() error { return s.DeleteTimeStandard("org", "a", "x", adminP) },
		"EvaluateCutQualification": func() error {
			_, err := s.EvaluateCutQualification("org", adminP, "amy", "x")
			return err
		},
	}
	for name, call := range calls {
		err := call()
		if err == nil {
			t.Errorf("%s: a failed club load must be returned", name)
			continue
		}
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrStandardNotFound) {
			t.Errorf("%s: want the load error, got %v", name, err)
		}
	}
	// Read paths that cannot return an error serve the (empty) cache instead.
	if s.HasMeetResult("org", "amy", "m", 50, "free", "SCY") || s.HasResultTime("org", "amy", 50, "free", "SCY", 1000) {
		t.Fatal("an unloadable club has no results")
	}
}

func TestCov1xUpsertResultSyncFailureIsReturned(t *testing.T) {
	s := NewService()
	cov1xBreakPersistence(t, s, "org")
	ms := 30000
	_, _, err := s.UpsertResultWithRecords("org", "coach", coachP, UpsertResultInput{
		SwimmerID: "amy", Distance: 50, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true,
	})
	if err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("want the database sync error, got %v", err)
	}
	if len(s.results) != 0 {
		t.Fatalf("nothing may be stored when the club cannot be synced: %d", len(s.results))
	}
}

func TestCov1xServiceRoleChecks(t *testing.T) {
	s := newRecordsFixture(t)
	if err := s.requireAdmin(nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("requireAdmin(nil) = %v", err)
	}
	if err := s.requireAdmin(coachP); !errors.Is(err, ErrForbidden) {
		t.Fatalf("requireAdmin(coach) = %v", err)
	}
	if _, err := s.RecomputeBestTime("org", "g", cov1xGuest, "amy", 50, "free", "SCY"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("RecomputeBestTime(guest) = %v", err)
	}
	if _, err := s.ListBestTimes("org", nil, "amy", ListBestTimesFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListBestTimes(nil) = %v", err)
	}
	if _, err := s.ListBestTimesForOrg("org", cov1xGuest, ListBestTimesFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListBestTimesForOrg(guest) = %v", err)
	}
	if _, err := s.ListProgression("org", cov1xGuest, "amy", ListResultsFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListProgression(guest) = %v", err)
	}
	if _, err := s.ListProgressionForOrg("org", cov1xGuest, ListResultsFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListProgressionForOrg(guest) = %v", err)
	}
	if _, err := s.EvaluateCutQualification("org", cov1xGuest, "amy", "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("EvaluateCutQualification(guest) = %v", err)
	}
	// Coaches may read but never mutate standards.
	if _, err := s.CreateTimeStandard("org", "c", coachP, CreateTimeStandardInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateTimeStandard(coach) = %v", err)
	}
	if _, err := s.UpdateTimeStandard("org", "c", "x", coachP, UpdateTimeStandardInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpdateTimeStandard(coach) = %v", err)
	}
	if err := s.DeleteTimeStandard("org", "c", "x", coachP); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DeleteTimeStandard(coach) = %v", err)
	}
}

func TestCov1xHouseholdScopeChecks(t *testing.T) {
	s := newRecordsFixture(t)
	if err := s.checkParentHouseholdForSwimmer(nil, "org", "amy"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal = %v", err)
	}
	if err := s.checkParentHouseholdForSwimmer(parentP, "org", "amy"); err != nil {
		t.Fatalf("own household = %v", err)
	}
	if err := s.checkParentHouseholdForSwimmer(parentP, "org", "bea"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign household = %v", err)
	}
	if err := s.checkParentHouseholdForSwimmer(parentP, "org", "ghost"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unknown swimmer = %v", err)
	}
	if err := s.checkParentHouseholdForSwimmer(adminP, "org", "bea"); err != nil {
		t.Fatalf("staff are not household scoped: %v", err)
	}
	noChecker := NewService()
	if err := noChecker.checkParentHouseholdForSwimmer(parentP, "org", "amy"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent without a wired checker must fail closed, got %v", err)
	}
	if principalRequiresHouseholdScope(cov1xGuest) {
		t.Fatal("a role outside parent/swimmer is not household scoped")
	}
	if s.householdScopedSwimmerAllowed(parentP, "org", "ghost") {
		t.Fatal("unknown swimmer must not be allowed")
	}
	if s.householdScopedSwimmerAllowed(&appmiddleware.Principal{Roles: []string{"parent"}}, "org", "amy") {
		t.Fatal("principal without household must not be allowed")
	}
	if _, err := s.ListBestTimes("org", parentP, "bea", ListBestTimesFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListBestTimes foreign swimmer = %v", err)
	}
	if _, err := s.ListProgression("org", parentP, "bea", ListResultsFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListProgression foreign swimmer = %v", err)
	}
	if _, err := s.EvaluateCutQualification("org", parentP, "bea", "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("EvaluateCutQualification foreign swimmer = %v", err)
	}
}

func TestCov1xBestTimeListsFilterScopeAndOrder(t *testing.T) {
	s := newRecordsFixture(t)
	cov1xSwim(t, s, "org", "amy", 100, "free", "SCY", 60000, true)
	cov1xSwim(t, s, "org", "amy", 100, "free", "SCM", 61000, true)
	cov1xSwim(t, s, "org", "amy", 50, "free", "SCY", 28000, true)
	cov1xSwim(t, s, "org", "amy", 100, "fly", "SCY", 70000, true)
	cov1xSwim(t, s, "org", "bea", 100, "free", "SCY", 59000, true)
	cov1xSwim(t, s, "other", "amy", 100, "free", "SCY", 1, true)

	label := func(list []*BestTime) string {
		var parts []string
		for _, b := range list {
			parts = append(parts, b.SwimmerID+"/"+b.Stroke+"/"+b.Course+"/"+string(rune('0'+b.EventDistance/50)))
		}
		return strings.Join(parts, " ")
	}
	all, err := s.ListBestTimesForOrg("org", adminP, ListBestTimesFilters{})
	if err != nil {
		t.Fatal(err)
	}
	// swimmer, then stroke, then distance, then course
	if got, want := label(all), "amy/fly/SCY/2 amy/free/SCY/1 amy/free/SCM/2 amy/free/SCY/2 bea/free/SCY/2"; got != want {
		t.Fatalf("order\n got %s\nwant %s", got, want)
	}
	if got, _ := s.ListBestTimesForOrg("org", adminP, ListBestTimesFilters{SwimmerID: "bea"}); len(got) != 1 || got[0].SwimmerID != "bea" {
		t.Fatalf("swimmer filter = %+v", got)
	}
	if got, _ := s.ListBestTimesForOrg("org", adminP, ListBestTimesFilters{Course: "SCM"}); len(got) != 1 || got[0].Course != "SCM" {
		t.Fatalf("course filter = %+v", got)
	}
	if got, _ := s.ListBestTimesForOrg("org", adminP, ListBestTimesFilters{Stroke: "fly"}); len(got) != 1 || got[0].Stroke != "fly" {
		t.Fatalf("stroke filter = %+v", got)
	}
	fam, _ := s.ListBestTimesForOrg("org", parentP, ListBestTimesFilters{})
	if len(fam) != 4 {
		t.Fatalf("parent sees only their household's 4 best times, got %d: %s", len(fam), label(fam))
	}
	for _, b := range fam {
		if b.SwimmerID != "amy" {
			t.Fatalf("parent leaked %s", b.SwimmerID)
		}
	}
	one, err := s.ListBestTimes("org", adminP, "amy", ListBestTimesFilters{SwimmerID: "bea"})
	if err != nil || len(one) != 4 {
		t.Fatalf("path swimmer overrides the filter swimmer: %v %d", err, len(one))
	}
}

func TestCov1xProgressionFiltersAndScope(t *testing.T) {
	s := newRecordsFixture(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	add := func(org, swimmer string, dist int, stroke, course string, ms int, at time.Time, status string) *Result {
		t.Helper()
		r := cov1xSwim(t, s, org, swimmer, dist, stroke, course, ms, true)
		r.CreatedAt = at
		r.Status = status
		return r
	}
	add("org", "amy", 100, "free", "SCY", 61000, base.Add(48*time.Hour), "active")
	add("org", "amy", 100, "free", "SCY", 60000, base, "active")
	add("org", "amy", 50, "free", "SCY", 28000, base.Add(24*time.Hour), "active")
	add("org", "amy", 100, "fly", "SCY", 70000, base.Add(24*time.Hour), "active")
	add("org", "amy", 100, "free", "SCM", 62000, base.Add(24*time.Hour), "active")
	add("org", "amy", 200, "free", "SCY", 130000, base, "deleted")
	add("org", "bea", 100, "free", "SCY", 59000, base, "active")
	add("other", "amy", 100, "free", "SCY", 1, base, "active")

	got, err := s.ListProgression("org", adminP, "amy", ListResultsFilters{EventDistance: 100, Stroke: "free", Course: "SCY"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || *got[0].FinTimeMs != 60000 || *got[1].FinTimeMs != 61000 {
		t.Fatalf("progression must be chronological and filtered: %+v", got)
	}
	if all, _ := s.ListProgression("org", adminP, "amy", ListResultsFilters{}); len(all) != 5 {
		t.Fatalf("unfiltered progression excludes other swimmers/orgs/inactive results, got %d", len(all))
	}
	if got, _ := s.ListProgression("org", adminP, "amy", ListResultsFilters{EventDistance: 50}); len(got) != 1 {
		t.Fatalf("distance filter = %d", len(got))
	}
	if got, _ := s.ListProgression("org", adminP, "amy", ListResultsFilters{Stroke: "fly"}); len(got) != 1 {
		t.Fatalf("stroke filter = %d", len(got))
	}
	if got, _ := s.ListProgression("org", adminP, "amy", ListResultsFilters{Course: "SCM"}); len(got) != 1 {
		t.Fatalf("course filter = %d", len(got))
	}

	org, err := s.ListProgressionForOrg("org", adminP, ListResultsFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(org) != 6 || org[0].SwimmerID != "amy" || org[len(org)-1].SwimmerID != "bea" {
		t.Fatalf("org progression order/size: %d first=%s last=%s", len(org), org[0].SwimmerID, org[len(org)-1].SwimmerID)
	}
	for i := 1; i < 5; i++ {
		if org[i].CreatedAt.Before(org[i-1].CreatedAt) {
			t.Fatalf("same swimmer must be chronological at %d", i)
		}
	}
	if got, _ := s.ListProgressionForOrg("org", adminP, ListResultsFilters{SwimmerID: "bea"}); len(got) != 1 {
		t.Fatalf("swimmer filter = %d", len(got))
	}
	if got, _ := s.ListProgressionForOrg("org", adminP, ListResultsFilters{EventDistance: 50}); len(got) != 1 {
		t.Fatalf("distance filter = %d", len(got))
	}
	if got, _ := s.ListProgressionForOrg("org", adminP, ListResultsFilters{Stroke: "fly"}); len(got) != 1 {
		t.Fatalf("stroke filter = %d", len(got))
	}
	if got, _ := s.ListProgressionForOrg("org", adminP, ListResultsFilters{Course: "SCM"}); len(got) != 1 {
		t.Fatalf("course filter = %d", len(got))
	}
	fam, _ := s.ListProgressionForOrg("org", parentP, ListResultsFilters{})
	if len(fam) != 5 {
		t.Fatalf("parent sees amy's 5 active results, got %d", len(fam))
	}
	for _, r := range fam {
		if r.SwimmerID != "amy" {
			t.Fatalf("parent leaked %s", r.SwimmerID)
		}
	}
}

func TestCov1xDuplicateMeetResultAndLookups(t *testing.T) {
	s := newRecordsFixture(t)
	ms := 30000
	in := UpsertResultInput{
		SwimmerID: "amy", MeetID: "m-2026", Distance: 50, Stroke: "free", Course: "SCY", Gender: "F",
		FinTimeMs: &ms, IsOfficial: true, SkipIfMeetResultExists: true,
	}
	if _, _, err := s.UpsertResultWithRecords("org", "coach", coachP, in); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertResultWithRecords("org", "coach", coachP, in); !errors.Is(err, ErrDuplicateMeetResult) {
		t.Fatalf("second import = %v, want ErrDuplicateMeetResult", err)
	}
	if len(s.results) != 1 {
		t.Fatalf("duplicate must not be stored, have %d", len(s.results))
	}
	if !s.HasMeetResult("org", "amy", "m-2026", 50, "free", "SCY") || s.HasMeetResult("org", "amy", "m-2025", 50, "free", "SCY") {
		t.Fatal("HasMeetResult must match the meet exactly")
	}
	if !s.HasResultTime("org", "amy", 50, "free", "SCY", 30000) || s.HasResultTime("org", "amy", 50, "free", "SCY", 30001) {
		t.Fatal("HasResultTime must match the exact finish time")
	}
}

func TestCov1xBetterSwimTieBreaks(t *testing.T) {
	fast, slow := 1000, 2000
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := &Result{ID: "a", FinTimeMs: &fast, CreatedAt: t0}
	b := &Result{ID: "b", FinTimeMs: &slow, CreatedAt: t0}
	if !betterSwim(a, b) || betterSwim(b, a) {
		t.Fatal("faster time wins")
	}
	same := 1500
	early := &Result{ID: "z", FinTimeMs: &same, CreatedAt: t0}
	late := &Result{ID: "a", FinTimeMs: &same, CreatedAt: t0.Add(time.Hour)}
	if !betterSwim(early, late) || betterSwim(late, early) {
		t.Fatal("equal times resolve to the earliest swim")
	}
	x := &Result{ID: "a", FinTimeMs: &same, CreatedAt: t0}
	y := &Result{ID: "b", FinTimeMs: &same, CreatedAt: t0}
	if !betterSwim(x, y) || betterSwim(y, x) {
		t.Fatal("identical time and date resolve to the lower id")
	}
}

func TestCov1xRecomputeBestTimeEdgeCases(t *testing.T) {
	s := newRecordsFixture(t)
	bt, err := s.RecomputeBestTime("org", "coach", coachP, "amy", 50, "free", "SCY")
	if err != nil || bt != nil {
		t.Fatalf("no results and no best time => nothing to create, got %+v %v", bt, err)
	}
	r := cov1xSwim(t, s, "org", "amy", 50, "free", "SCY", 28000, true)
	if cur := s.btByKey[btKey("org", "amy", 50, "free", "SCY")]; cur == nil || *cur.BestTimeMs != 28000 {
		t.Fatalf("best time after swim = %+v", cur)
	}
	r.Status = "deleted"
	bt, err = s.RecomputeBestTime("org", "coach", coachP, "amy", 50, "free", "SCY")
	if err != nil {
		t.Fatal(err)
	}
	if bt == nil || bt.BestTimeMs != nil || bt.ResultID != "" || bt.RecordedAt != nil {
		t.Fatalf("with no qualifying swim the best time is cleared to NT: %+v", bt)
	}
}

func TestCov1xOverrideRecordedAtAndPersistFailure(t *testing.T) {
	s := NewService()
	best := 27000
	in := OverrideBestTimeInput{SwimmerID: "amy", EventDistance: 50, Stroke: "free", Course: "SCY", BestTimeMs: &best, Reason: "verified", RecordedAt: "2025-03-04"}
	bt, _, err := s.OverrideBestTime("org", "admin", adminP, in)
	if err != nil {
		t.Fatal(err)
	}
	if bt.RecordedAt == nil || bt.RecordedAt.Format("2006-01-02") != "2025-03-04" {
		t.Fatalf("recorded_at = %v", bt.RecordedAt)
	}
	in.SwimmerID, in.RecordedAt = "bea", "not-a-date"
	bt2, _, err := s.OverrideBestTime("org", "admin", adminP, in)
	if err != nil || bt2.RecordedAt != nil {
		t.Fatalf("an unparseable recorded_at is ignored, got %v %v", bt2.RecordedAt, err)
	}

	cov1xBreakPersistence(t, s, "org")
	in.SwimmerID = "cat"
	if _, _, err := s.OverrideBestTime("org", "admin", adminP, in); err == nil || !strings.Contains(err.Error(), "persist best time override") {
		t.Fatalf("want persist error, got %v", err)
	}
	if s.btByKey[btKey("org", "cat", 50, "free", "SCY")] != nil {
		t.Fatal("failed override must not be cached")
	}
}

func TestCov1xTimeStandardValidationFilteringAndPersistFailure(t *testing.T) {
	s := NewService()
	mk := func(name, code string, season, dist int, stroke, course, gender string) *TimeStandard {
		t.Helper()
		ts, err := s.CreateTimeStandard("org", "admin", adminP, CreateTimeStandardInput{
			Name: name, SeasonYear: season, StandardCode: code, EventDistance: dist, Stroke: stroke, Course: course, Gender: gender, TimeMs: 60000, IsActive: code != "OFF",
		})
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	neg, big, lo, hi := -1, 121, 14, 10
	for name, in := range map[string]CreateTimeStandardInput{
		"negative min":  {AgeMin: &neg},
		"huge max":      {AgeMax: &big},
		"min above max": {AgeMin: &lo, AgeMax: &hi},
	} {
		if _, err := s.CreateTimeStandard("org", "admin", adminP, in); !errors.Is(err, ErrInvalidAgeRange) {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	a := mk("A", "AAA", 2025, 100, "free", "SCY", "F")
	b := mk("B", "BBB", 2026, 100, "free", "SCY", "F")
	c := mk("C", "AAA", 2026, 200, "back", "SCM", "M")
	off := mk("D", "OFF", 2026, 50, "fly", "LCM", "F")

	list, _ := s.ListTimeStandards("org", ListTimeStandardsFilters{})
	var ids []string
	for _, ts := range list {
		ids = append(ids, ts.ID)
	}
	// newest season first, then code ascending; ties are not asserted beyond the two groups
	if len(list) != 4 || list[0].SeasonYear != 2026 || list[3].ID != a.ID {
		t.Fatalf("order = %v", ids)
	}
	if list[0].StandardCode > list[1].StandardCode {
		t.Fatalf("same season orders by code: %s then %s", list[0].StandardCode, list[1].StandardCode)
	}
	cases := []struct {
		name string
		f    ListTimeStandardsFilters
		want string
	}{
		{"active", ListTimeStandardsFilters{ActiveOnly: true, SeasonYear: 2026, EventDistance: 50}, ""},
		{"season", ListTimeStandardsFilters{SeasonYear: 2025}, a.ID},
		{"code", ListTimeStandardsFilters{StandardCode: "bbb"}, b.ID},
		{"distance", ListTimeStandardsFilters{EventDistance: 200}, c.ID},
		{"stroke", ListTimeStandardsFilters{Stroke: "fly"}, off.ID},
		{"course", ListTimeStandardsFilters{Course: "SCM"}, c.ID},
		{"gender", ListTimeStandardsFilters{Gender: "M"}, c.ID},
	}
	for _, tc := range cases {
		got, _ := s.ListTimeStandards("org", tc.f)
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("%s: want none, got %d", tc.name, len(got))
			}
			continue
		}
		if len(got) != 1 || got[0].ID != tc.want {
			t.Errorf("%s: got %+v", tc.name, got)
		}
	}
	if other, _ := s.ListTimeStandards("other", ListTimeStandardsFilters{}); len(other) != 0 {
		t.Fatal("standards must be org scoped")
	}
	if err := s.DeleteTimeStandard("org", "admin", c.ID, adminP); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListTimeStandards("org", ListTimeStandardsFilters{Gender: "M"}); len(got) != 0 {
		t.Fatal("soft-deleted standards are hidden")
	}
	if _, err := s.GetTimeStandard("org", c.ID); !errors.Is(err, ErrStandardNotFound) {
		t.Fatalf("deleted get = %v", err)
	}
	if _, err := s.GetTimeStandard("other", a.ID); !errors.Is(err, ErrStandardNotFound) {
		t.Fatalf("cross-org get = %v", err)
	}

	desc := "new description"
	upd, err := s.UpdateTimeStandard("org", "admin", a.ID, adminP, UpdateTimeStandardInput{Description: &desc})
	if err != nil || upd.Description != desc {
		t.Fatalf("description update = %+v %v", upd, err)
	}

	cov1xBreakPersistence(t, s, "org")
	if _, err := s.CreateTimeStandard("org", "admin", adminP, CreateTimeStandardInput{Name: "E", StandardCode: "E"}); err == nil || !strings.Contains(err.Error(), "persist time standard") {
		t.Fatalf("create = %v", err)
	}
	other := "other"
	if _, err := s.UpdateTimeStandard("org", "admin", a.ID, adminP, UpdateTimeStandardInput{Name: &other}); err == nil || !strings.Contains(err.Error(), "persist time standard") {
		t.Fatalf("update = %v", err)
	}
	if err := s.DeleteTimeStandard("org", "admin", a.ID, adminP); err == nil || !strings.Contains(err.Error(), "persist time standard delete") {
		t.Fatalf("delete = %v", err)
	}
	if got, _ := s.GetTimeStandard("org", a.ID); got.Name != "A" || got.DeletedAt != nil {
		t.Fatalf("failed writes must not change memory: %+v", got)
	}
}

func TestCov1xCutQualificationEligibility(t *testing.T) {
	s := newRecordsFixture(t)
	mk := func(gender string, ageMin, ageMax *int) *TimeStandard {
		t.Helper()
		ts, err := s.CreateTimeStandard("org", "admin", adminP, CreateTimeStandardInput{
			Name: "Cut", SeasonYear: 2026, StandardCode: "AA", EventDistance: 100, Stroke: "free", Course: "SCY",
			Gender: gender, AgeMin: ageMin, AgeMax: ageMax, TimeMs: 62000, IsActive: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	cov1xSwim(t, s, "org", "amy", 100, "free", "SCY", 60000, true)

	male := mk("M", nil, nil)
	q, err := s.EvaluateCutQualification("org", adminP, "amy", male.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q.IneligibleReason != "swimmer gender does not match the standard" || q.Qualified || q.BestTimeMs == nil || *q.BestTimeMs != 60000 {
		t.Fatalf("gender mismatch = %+v", q)
	}

	tooOld := 9
	young := mk("F", nil, &tooOld)
	q, _ = s.EvaluateCutQualification("org", adminP, "amy", young.ID)
	if q.IneligibleReason != "swimmer age is outside the standard's age range" || q.Qualified {
		t.Fatalf("age mismatch = %+v", q)
	}
	tooYoung := 80
	senior := mk("F", &tooYoung, nil)
	if q, _ = s.EvaluateCutQualification("org", adminP, "amy", senior.ID); q.IneligibleReason == "" {
		t.Fatalf("below the minimum age must be ineligible: %+v", q)
	}
	// ineligible with no best time on record leaves the time empty
	q, _ = s.EvaluateCutQualification("org", adminP, "bea", male.ID)
	if q.IneligibleReason == "" || q.BestTimeMs != nil {
		t.Fatalf("no time on record = %+v", q)
	}

	ok := mk("F", nil, nil)
	q, _ = s.EvaluateCutQualification("org", adminP, "amy", ok.ID)
	if !q.Qualified || q.MarginMs == nil || *q.MarginMs != 2000 || q.IneligibleReason != "" {
		t.Fatalf("eligible and under the cut = %+v", q)
	}
	if err := s.DeleteTimeStandard("org", "admin", ok.ID, adminP); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EvaluateCutQualification("org", adminP, "amy", ok.ID); !errors.Is(err, ErrStandardNotFound) {
		t.Fatalf("deleted standard = %v", err)
	}
}

func TestCov1xEnsureOrgSkipsWhenAnotherRequestIsRefreshing(t *testing.T) {
	s := NewService()
	s.repo = cov1xClosedRepo(t)
	stale := new(atomic.Int64)
	stale.Store(time.Now().Add(-time.Hour).UnixNano())
	s.loadedAt.Store("org", stale)
	busy := new(atomic.Bool)
	busy.Store(true)
	s.refreshing.Store("org", busy)
	// A stale club is being re-read by someone else, so this request serves the cache
	// instead of hitting the (unreachable) database.
	if err := s.ensureOrg("org"); err != nil {
		t.Fatalf("ensureOrg = %v", err)
	}
	if !busy.Load() {
		t.Fatal("the in-flight refresh flag belongs to the other request")
	}
}
