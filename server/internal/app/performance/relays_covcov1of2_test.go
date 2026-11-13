package performance

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCov1xOptimizeRelaysSpecAndLockErrors(t *testing.T) {
	free := map[string]int{"free": 50000}
	pool := []RelayCandidate{cand("a", "F", free), cand("b", "F", free)}
	spec := RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 1}
	cases := []struct {
		name  string
		spec  RelaySpec
		locks []RelayLegLock
		excl  map[string]bool
		want  string
	}{
		{"relay type", RelaySpec{RelayType: "x", Distance: 200, Gender: "F", Teams: 1}, nil, nil, "relay_type must be free or medley"},
		{"gender", RelaySpec{RelayType: "free", Distance: 200, Gender: "Z", Teams: 1}, nil, nil, "gender must be M, F or X"},
		{"locked and excluded", spec, []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "a"}}, map[string]bool{"a": true}, "locked swimmer a is also excluded"},
		{"two swimmers one slot", spec, []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "a"}, {Team: 0, Leg: 1, SwimmerID: "b"}}, nil, "two swimmers locked to relay A leg 1"},
		{"one swimmer two legs", spec, []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "a"}, {Team: 0, Leg: 2, SwimmerID: "a"}}, nil, "locked to more than one leg"},
	}
	for _, tc := range cases {
		_, err := OptimizeRelays(tc.spec, pool, tc.locks, tc.excl)
		if !errors.Is(err, ErrRelayInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want ErrRelayInvalid containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCov1xOptimizeRelaysTieBreaksAndMixedPoolFiltering(t *testing.T) {
	free := map[string]int{"free": 50000}
	pool := []RelayCandidate{cand("e", "F", free), cand("c", "F", free), cand("a", "F", free), cand("d", "F", free), cand("b", "F", free)}
	got, err := OptimizeRelays(RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 1}, pool, nil, nil)
	if err != nil || len(got.Teams) != 1 {
		t.Fatalf("proposal = %+v %v", got, err)
	}
	// Equal times resolve by swimmer id; the anchor is the lowest id, then lead-off, then 2 and 3.
	if ids := teamIDs(got.Teams[0]); strings.Join(ids, "") != "bcda" {
		t.Fatalf("leg order = %v, want [b c d a]", ids)
	}
	if got.Teams[0].TotalMs != 200000 {
		t.Fatalf("total = %d", got.Teams[0].TotalMs)
	}

	mixed := []RelayCandidate{
		cand("u", "U", map[string]int{"free": 1}), // gender outside M/F is never eligible for a mixed relay
		cand("m1", "M", free), cand("m2", "M", free), cand("m3", "M", map[string]int{"free": 60000}),
		cand("f1", "F", free), cand("f2", "F", free),
	}
	got, err = OptimizeRelays(RelaySpec{RelayType: "free", Distance: 200, Gender: "X", Teams: 1}, mixed, nil, nil)
	if err != nil || len(got.Teams) != 1 {
		t.Fatalf("mixed proposal = %+v %v", got, err)
	}
	men := 0
	for _, l := range got.Teams[0].Legs {
		if l.SwimmerID == "u" || l.SwimmerID == "m3" {
			t.Fatalf("ineligible or slower swimmer %s was picked", l.SwimmerID)
		}
		if l.Gender == "M" {
			men++
		}
	}
	if men != 2 {
		t.Fatalf("mixed relay needs exactly two men, got %d", men)
	}
}

func TestCov1xOptimizeRelayPoolFiltersAndErrors(t *testing.T) {
	s := NewService()
	s.SetDirectory(&fakeDirectory{
		swimmers: []SwimmerProfile{
			{ID: "g1", FirstName: "Gia", LastName: "One", DateOfBirth: "2012-01-01", Gender: "female", GroupCodes: []string{"AGE"}},
			{ID: "g2", FirstName: "Gwen", LastName: "Two", DateOfBirth: "2012-02-01", Gender: "female", GroupCodes: []string{"AGE"}},
			{ID: "g3", FirstName: "Gus", LastName: "Three", DateOfBirth: "2012-03-01", Gender: "female", GroupCodes: []string{"SENIOR"}},
			{ID: "g4", FirstName: "Gail", LastName: "Four", DateOfBirth: "2012-04-01", Gender: "female", GroupCodes: []string{"AGE"}},
			{ID: "g5", FirstName: "Gabi", LastName: "Five", DateOfBirth: "2000-04-01", Gender: "female"},
			{ID: "b1", FirstName: "Bo", LastName: "One", DateOfBirth: "2012-01-01", Gender: "male"},
			{ID: "nb", FirstName: "Nic", LastName: "Bee", DateOfBirth: "2012-01-01", Gender: "non_binary"},
		},
		meets: []MeetInfo{{ID: "m", Name: "Meet", StartDate: "2026-02-20", Course: "scy"}},
	})
	add := func(swimmer string, dist int, stroke, course string, ms int) {
		t.Helper()
		if _, err := s.UpsertResult("org", "coach", coachP, UpsertResultInput{
			SwimmerID: swimmer, Distance: dist, Stroke: stroke, Course: course, Gender: "F", FinTimeMs: &ms, IsOfficial: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{"g1", "g2", "g3", "g4", "g5", "b1", "nb"} {
		add(id, 50, "free", "SCY", 25000+i*100)
	}
	add("g1", 100, "free", "SCY", 1) // wrong leg distance
	add("g1", 50, "free", "SCM", 1)  // wrong course
	add("g1", 50, "fly", "SCY", 1)   // stroke not swum in a free relay
	add("ghost", 50, "free", "SCY", 1)
	s.bestTimes["nil-bt"] = &BestTime{ID: "nil-bt", OrgID: "org", SwimmerID: "g2", EventDistance: 50, Stroke: "free", Course: "SCY"}

	base := RelayOptimizeInput{Course: "scy", Gender: "f", RelayType: "FREE", Distance: 200, Teams: 1, AsOf: "2026-02-20", AgeGroup: "13-14"}
	res, err := s.OptimizeRelay("org", coachP, base)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, p := range res.Pool {
		names[p.SwimmerID] = true
		if p.Times["free"] > 25400 {
			t.Fatalf("pool times must come from best times: %+v", p)
		}
	}
	if len(res.Pool) != 4 || names["g5"] || names["b1"] || names["nb"] || names["ghost"] {
		t.Fatalf("pool = %v (adult, boy, unknown-gender and unknown swimmers must be excluded)", names)
	}
	if len(res.Teams) != 1 || res.LegLength != 50 || res.Course != "SCY" || res.AgeGroup != "13-14" {
		t.Fatalf("result = %+v", res)
	}

	in := base
	in.GroupCode = "age"
	if r, _ := s.OptimizeRelay("org", coachP, in); len(r.Pool) != 3 || len(r.Teams) != 0 {
		t.Fatalf("group filter pool = %d", len(r.Pool))
	}
	in = base
	in.IncludeSwimmerIDs = []string{" g1 ", "g2"}
	if r, _ := s.OptimizeRelay("org", coachP, in); len(r.Pool) != 2 || len(r.Teams) != 0 || len(r.Warnings) != 1 {
		t.Fatalf("include filter: pool=%d teams=%d warnings=%v", len(r.Pool), len(r.Teams), r.Warnings)
	}
	in = base
	in.ExcludedSwimmerIDs = []string{" g1"}
	if r, _ := s.OptimizeRelay("org", coachP, in); len(r.Pool) != 4 || len(r.Teams) != 0 || !strings.Contains(r.Warnings[0], "Not enough eligible swimmers") {
		t.Fatalf("excluding one of four leaves no relay: teams=%d warnings=%v", len(r.Teams), r.Warnings)
	}
	in = base
	in.Gender, in.Teams = "X", 0
	if r, err := s.OptimizeRelay("org", coachP, in); err != nil || len(r.Pool) != 5 {
		t.Fatalf("mixed pool should hold the 4 girls and the boy: %v %+v", err, r)
	}

	// Defaults from the meet: course and as-of date.
	in = RelayOptimizeInput{MeetID: "m", Gender: "F", RelayType: "free", Distance: 200, Teams: 1}
	r, err := s.OptimizeRelay("org", coachP, in)
	if err != nil || r.Course != "SCY" || r.AsOf != "2026-02-20" || r.AgeGroup != AgeGroupOpen || r.MeetID != "m" {
		t.Fatalf("meet defaults = %+v %v", r, err)
	}
	in.AsOf, in.MeetID, in.Course = "", "", "SCY"
	if r, err = s.OptimizeRelay("org", coachP, in); err != nil || r.AsOf != time.Now().UTC().Format("2006-01-02") {
		t.Fatalf("as_of defaults to today: %+v %v", r, err)
	}

	fail := map[string]struct {
		mut  func(*RelayOptimizeInput)
		want string
	}{
		"age group": {func(i *RelayOptimizeInput) { i.AgeGroup = "99" }, "unknown age group"},
		"meet":      {func(i *RelayOptimizeInput) { i.MeetID = "nope" }, "meet not found"},
		"as of":     {func(i *RelayOptimizeInput) { i.AsOf = "tomorrow" }, "as_of must be"},
		"course":    {func(i *RelayOptimizeInput) { i.Course = "XXX" }, "course must be"},
		"spec":      {func(i *RelayOptimizeInput) { i.Distance = 300 }, "distance must be"},
		"bad lock":  {func(i *RelayOptimizeInput) { i.Locks = []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "b1"}} }, "not eligible"},
	}
	for name, tc := range fail {
		in := base
		tc.mut(&in)
		if _, err := s.OptimizeRelay("org", coachP, in); !errors.Is(err, ErrRelayInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
	if _, err := s.OptimizeRelay("org", parentP, base); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent = %v", err)
	}
	if _, err := cov1xUnloaded(t).OptimizeRelay("org", coachP, base); err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("load failure = %v", err)
	}
}

func cov1xLineupInput() RelayLineupInput {
	leg := func(n int, id string) RelayLeg { return RelayLeg{Leg: n, SwimmerID: id, TimeMs: 25000} }
	return RelayLineupInput{
		MeetID: " m-2026 ", Course: "scy", Gender: "f", AgeGroup: "11-12", RelayType: "FREE", Distance: 200,
		Teams: []RelayTeam{{Legs: []RelayLeg{leg(4, "amy"), leg(2, "bea"), leg(1, "old"), leg(3, "amy2")}}},
	}
}

func TestCov1xRelayLineupLifecycleAndValidation(t *testing.T) {
	s := newRecordsFixture(t)
	d := s.directory.(*fakeDirectory)
	d.swimmers = append(d.swimmers, SwimmerProfile{ID: "amy2", FirstName: "Amy", LastName: "Two"})

	saved, err := s.SaveRelayLineup("org", "coach", coachP, "", cov1xLineupInput())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Women 11-12 200 Free Relay" || saved.MeetID != "m-2026" || saved.Course != "SCY" || saved.RelayType != "free" {
		t.Fatalf("saved = %+v", saved)
	}
	legs := saved.Teams[0].Legs
	if legs[0].SwimmerID != "old" || legs[3].SwimmerID != "amy" || legs[0].Stroke != "free" || legs[1].SwimmerName != "Bea Bell" || saved.Teams[0].TotalMs != 100000 || saved.Teams[0].Label != "A" {
		t.Fatalf("legs must be sorted, named and totalled: %+v", saved.Teams[0])
	}

	in := cov1xLineupInput()
	in.Name, in.Notes, in.Teams[0].Legs[0].TimeMs = "Finals", "notes", -5
	updated, err := s.SaveRelayLineup("org", "admin", adminP, saved.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != saved.ID || updated.Name != "Finals" || updated.CreatedBy != "coach" || updated.UpdatedBy != "admin" || !updated.CreatedAt.Equal(saved.CreatedAt) {
		t.Fatalf("replace keeps identity and creation: %+v", updated)
	}
	if updated.Teams[0].TotalMs != 75000 {
		t.Fatalf("negative leg time is clamped to zero, total = %d", updated.Teams[0].TotalMs)
	}
	updated.Teams[0].Legs[0].SwimmerID = "mutated"
	if s.lineups[saved.ID].Teams[0].Legs[0].SwimmerID == "mutated" {
		t.Fatal("returned lineup must not alias the stored one")
	}

	if _, err := s.SaveRelayLineup("org", "coach", coachP, "ghost", cov1xLineupInput()); !errors.Is(err, ErrRelayLineupNotFound) {
		t.Fatalf("replace missing = %v", err)
	}
	if _, err := s.SaveRelayLineup("other", "coach", coachP, saved.ID, cov1xLineupInput()); !errors.Is(err, ErrRelayLineupNotFound) {
		t.Fatalf("replace other club's lineup = %v", err)
	}
	if _, err := s.SaveRelayLineup("org", "p", parentP, "", cov1xLineupInput()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent save = %v", err)
	}

	// A second lineup for another meet, plus one for another club.
	other := cov1xLineupInput()
	other.MeetID = "m-2025"
	if _, err := s.SaveRelayLineup("org", "coach", coachP, "", other); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.ListRelayLineups("org", coachP, ""); len(all) != 2 || all[0].CreatedAt.After(all[1].CreatedAt) {
		t.Fatalf("list all must be in creation order: %+v", all)
	}
	if one, _ := s.ListRelayLineups("org", coachP, "m-2025"); len(one) != 1 || one[0].MeetID != "m-2025" {
		t.Fatalf("meet filter = %+v", one)
	}
	if none, _ := s.ListRelayLineups("other", coachP, ""); len(none) != 0 {
		t.Fatalf("lineups are club scoped: %+v", none)
	}
	if _, err := s.ListRelayLineups("org", parentP, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent list = %v", err)
	}

	if err := s.DeleteRelayLineup("org", "coach", parentP, saved.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent delete = %v", err)
	}
	if err := s.DeleteRelayLineup("org", "coach", coachP, "ghost"); !errors.Is(err, ErrRelayLineupNotFound) {
		t.Fatalf("delete missing = %v", err)
	}
	if err := s.DeleteRelayLineup("other", "coach", coachP, saved.ID); !errors.Is(err, ErrRelayLineupNotFound) {
		t.Fatalf("delete other club's lineup = %v", err)
	}
	cov1xBreakPersistence(t, s, "org")
	if err := s.DeleteRelayLineup("org", "coach", coachP, saved.ID); err == nil || !strings.Contains(err.Error(), "delete relay lineup") {
		t.Fatalf("delete persist failure = %v", err)
	}
	if _, err := s.SaveRelayLineup("org", "coach", coachP, saved.ID, cov1xLineupInput()); err == nil || !strings.Contains(err.Error(), "persist relay lineup") {
		t.Fatalf("save persist failure = %v", err)
	}
	if s.lineups[saved.ID] == nil || s.lineups[saved.ID].Name != "Finals" {
		t.Fatal("failed writes must leave the stored lineup alone")
	}
	s.repo = nil
	if err := s.DeleteRelayLineup("org", "coach", coachP, saved.ID); err != nil || s.lineups[saved.ID] != nil {
		t.Fatalf("delete = %v", err)
	}
}

func TestCov1xRelayLineupNormalizeErrors(t *testing.T) {
	s := newRecordsFixture(t)
	d := s.directory.(*fakeDirectory)
	d.swimmers = append(d.swimmers, SwimmerProfile{ID: "amy2", FirstName: "Amy", LastName: "Two"})
	mut := func(f func(*RelayLineupInput)) RelayLineupInput {
		in := cov1xLineupInput()
		f(&in)
		return in
	}
	cases := map[string]struct {
		in   RelayLineupInput
		want string
	}{
		"no meet":         {mut(func(i *RelayLineupInput) { i.MeetID = " " }), "meet_id is required"},
		"unknown meet":    {mut(func(i *RelayLineupInput) { i.MeetID = "zzz" }), "meet not found"},
		"course":          {mut(func(i *RelayLineupInput) { i.Course = "XX" }), "course must be"},
		"age group":       {mut(func(i *RelayLineupInput) { i.AgeGroup = "XX" }), "unknown age group"},
		"relay spec":      {mut(func(i *RelayLineupInput) { i.Distance = 300 }), "distance must be"},
		"long name":       {mut(func(i *RelayLineupInput) { i.Name = strings.Repeat("n", 121) }), "name/notes too long"},
		"long notes":      {mut(func(i *RelayLineupInput) { i.Notes = strings.Repeat("n", 1001) }), "name/notes too long"},
		"no teams":        {mut(func(i *RelayLineupInput) { i.Teams = nil }), "needs 1"},
		"too many":        {mut(func(i *RelayLineupInput) { i.Teams = make([]RelayTeam, maxRelayTeams+1) }), "needs 1"},
		"three legs":      {mut(func(i *RelayLineupInput) { i.Teams[0].Legs = i.Teams[0].Legs[:3] }), "exactly 4 legs"},
		"leg numbers":     {mut(func(i *RelayLineupInput) { i.Teams[0].Legs[0].Leg = 9 }), "numbered 1"},
		"empty swimmer":   {mut(func(i *RelayLineupInput) { i.Teams[0].Legs[0].SwimmerID = "  " }), "has no swimmer"},
		"duplicate":       {mut(func(i *RelayLineupInput) { i.Teams[0].Legs[1].SwimmerID = i.Teams[0].Legs[0].SwimmerID }), "more than one leg"},
		"unknown swimmer": {mut(func(i *RelayLineupInput) { i.Teams[0].Legs[2].SwimmerID = "ghost" }), "unknown swimmer"},
	}
	for name, tc := range cases {
		if _, err := s.SaveRelayLineup("org", "coach", coachP, "", tc.in); !errors.Is(err, ErrRelayInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want ErrRelayInvalid containing %q", name, err, tc.want)
		}
	}
	if len(s.lineups) != 0 {
		t.Fatalf("invalid lineups must not be stored: %d", len(s.lineups))
	}

	// Without a directory the lineup is accepted as typed (no meet or swimmer lookups).
	bare := NewService()
	in := cov1xLineupInput()
	in.Teams[0].Legs[2].SwimmerID = "whoever"
	got, err := bare.SaveRelayLineup("org", "coach", coachP, "", in)
	if err != nil || got.Teams[0].Legs[0].SwimmerName != "" {
		t.Fatalf("no directory = %+v %v", got, err)
	}
}

func TestCov1xRelayLineupLoadFailures(t *testing.T) {
	s := cov1xUnloaded(t)
	if _, err := s.ListRelayLineups("org", coachP, ""); err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("list = %v", err)
	}
	if _, err := s.SaveRelayLineup("org", "c", coachP, "", cov1xLineupInput()); err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("save = %v", err)
	}
	if err := s.DeleteRelayLineup("org", "c", coachP, "x"); err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("delete = %v", err)
	}
}
