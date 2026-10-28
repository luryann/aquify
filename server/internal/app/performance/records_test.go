package performance

import (
	"errors"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

type fakeDirectory struct {
	swimmers []SwimmerProfile
	meets    []MeetInfo
}

func (f *fakeDirectory) SwimmerProfiles(string) []SwimmerProfile { return f.swimmers }
func (f *fakeDirectory) Meets(string) []MeetInfo                 { return f.meets }

type fakeHouseholds map[string]string

func (f fakeHouseholds) SwimmerHouseholdID(_, swimmerID string) (string, bool) {
	h, ok := f[swimmerID]
	return h, ok
}

var (
	coachP  = &appmiddleware.Principal{UserID: "coach", OrgID: "org", Roles: []string{"coach"}}
	adminP  = &appmiddleware.Principal{UserID: "admin", OrgID: "org", Roles: []string{"admin"}}
	parentP = &appmiddleware.Principal{UserID: "parent", OrgID: "org", Roles: []string{"parent"}, HouseholdID: "hh-1"}
)

func newRecordsFixture(t *testing.T) *Service {
	t.Helper()
	s := NewService()
	s.SetDirectory(&fakeDirectory{
		swimmers: []SwimmerProfile{
			{ID: "amy", HouseholdID: "hh-1", FirstName: "Amy", LastName: "Ames", DateOfBirth: "2014-06-01", Gender: "female", GroupCodes: []string{"AGE"}},
			{ID: "bea", HouseholdID: "hh-2", FirstName: "Bea", LastName: "Bell", DateOfBirth: "2013-01-15", Gender: "female", GroupCodes: []string{"SENIOR"}},
			{ID: "old", HouseholdID: "hh-3", FirstName: "Olga", LastName: "Old", DateOfBirth: "2000-01-01", Gender: "female"},
		},
		meets: []MeetInfo{
			{ID: "m-2025", Name: "Fall Classic", StartDate: "2025-10-04", Course: "SCY"},
			{ID: "m-2026", Name: "Winter Champs", StartDate: "2026-02-20", Course: "SCY"},
		},
	})
	s.SetHouseholdChecker(fakeHouseholds{"amy": "hh-1", "bea": "hh-2", "old": "hh-3"})
	return s
}

func swim(t *testing.T, s *Service, swimmer, meet string, ms int) []RecordBreak {
	t.Helper()
	_, breaks, err := s.UpsertResultWithRecords("org", "coach", coachP, UpsertResultInput{
		SwimmerID: swimmer, MeetID: meet, Distance: 100, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return breaks
}

func TestAgeGroupsAndAgeOnSwimDate(t *testing.T) {
	cases := map[int]string{5: "8U", 8: "8U", 9: "9-10", 12: "11-12", 13: "13-14", 16: "15-16", 18: "17-18", 19: ""}
	for age, want := range cases {
		if got := AgeGroupForAge(age); got != want {
			t.Fatalf("age %d: got %q want %q", age, got, want)
		}
	}
	if a := AgeOn("2014-06-01", "2026-05-31"); a == nil || *a != 11 {
		t.Fatalf("day before birthday should be 11, got %v", a)
	}
	if a := AgeOn("2014-06-01", "2026-06-01"); a == nil || *a != 12 {
		t.Fatalf("on birthday should be 12, got %v", a)
	}
	if AgeOn("", "2026-01-01") != nil {
		t.Fatal("missing DOB must yield nil age")
	}
}

func TestRecordBreaksAgeGroupAndOpenWithHistory(t *testing.T) {
	s := newRecordsFixture(t)
	// Amy is 11 at the 2025 meet → 11-12 and Open records.
	breaks := swim(t, s, "amy", "m-2025", 62000)
	if len(breaks) != 2 || breaks[0].AgeGroup != "11-12" || breaks[1].AgeGroup != AgeGroupOpen {
		t.Fatalf("expected 11-12 + Open breaks, got %+v", breaks)
	}
	if breaks[0].PreviousTimeMs != nil || breaks[0].Status != RecordStatusAccepted {
		t.Fatalf("first record should have no previous: %+v", breaks[0])
	}
	// Bea is 13 at the 2026 meet; 60.00 breaks only Open (she's 13-14 there → new 13-14 record too).
	breaks = swim(t, s, "bea", "m-2026", 60000)
	if len(breaks) != 2 {
		t.Fatalf("expected 13-14 + Open breaks, got %+v", breaks)
	}
	open := breaks[1]
	if open.AgeGroup != AgeGroupOpen || open.PreviousTimeMs == nil || *open.PreviousTimeMs != 62000 || open.PreviousHolder != "Amy Ames" {
		t.Fatalf("open break should cite Amy's 62.00: %+v", open)
	}
	// A slower swim breaks nothing; an equal swim does not break either.
	if b := swim(t, s, "amy", "m-2026", 62000); len(b) != 0 {
		t.Fatalf("tie must not break a record: %+v", b)
	}
	hist, err := s.GetRecordHistory("org", parentP, RecordKey{Course: "SCY", Gender: "F", AgeGroup: AgeGroupOpen, Distance: 100, Stroke: "free"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Progression) != 2 || hist.Progression[0].SwimmerName != "Bea Bell" || hist.Progression[1].SwimmerName != "Amy Ames" {
		t.Fatalf("progression should be Bea then Amy: %+v", hist.Progression)
	}
	if hist.Current.SwimDate != "2026-02-20" || hist.Current.MeetName != "Winter Champs" {
		t.Fatalf("swim date should come from the meet: %+v", hist.Current)
	}
	recs, _ := s.ListRecords("org", parentP, ListRecordsFilters{AgeGroup: AgeGroupOpen})
	if len(recs) != 1 || recs[0].Previous == nil || recs[0].Previous.SwimmerName != "Amy Ames" {
		t.Fatalf("record board should list previous holder: %+v", recs)
	}
}

func TestLockedRecordProducesPendingUntilApproved(t *testing.T) {
	s := newRecordsFixture(t)
	manual, err := s.CreateManualRecord("org", "admin", adminP, CreateManualRecordInput{
		Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free", TimeMs: 55000,
		SwimmerName: "Historic Hannah", SwimDate: "1998-03-01", Locked: true, Verified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Coach cannot add manual records.
	if _, err := s.CreateManualRecord("org", "coach", coachP, CreateManualRecordInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach manual create: %v", err)
	}
	breaks := swim(t, s, "old", "m-2026", 54000) // Olga is 26 → Open only
	if len(breaks) != 1 || breaks[0].Status != RecordStatusPending || *breaks[0].PreviousTimeMs != 55000 {
		t.Fatalf("expected one pending break against locked record: %+v", breaks)
	}
	recs, _ := s.ListRecords("org", coachP, ListRecordsFilters{})
	if len(recs) != 1 || recs[0].Record.ID != manual.ID || recs[0].PendingCount != 1 {
		t.Fatalf("locked record should stand with 1 pending: %+v", recs)
	}
	if _, err := s.DecideRecordEntry("org", "coach", breaks[0].EntryID, coachP, true, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach approve should be forbidden: %v", err)
	}
	if _, err := s.DecideRecordEntry("org", "admin", breaks[0].EntryID, adminP, true, "Verified with meet results"); err != nil {
		t.Fatal(err)
	}
	recs, _ = s.ListRecords("org", coachP, ListRecordsFilters{})
	if recs[0].Record.SwimmerName != "Olga Old" || recs[0].Previous.SwimmerName != "Historic Hannah" || recs[0].PendingCount != 0 {
		t.Fatalf("approved swim should be the record: %+v", recs[0])
	}
	if _, err := s.DecideRecordEntry("org", "admin", breaks[0].EntryID, adminP, true, ""); !errors.Is(err, ErrRecordNotPending) {
		t.Fatalf("double approve: %v", err)
	}
	title, body, err := s.RecordAnnouncementDraft("org", coachP, breaks[0].EntryID)
	if err != nil || title == "" || body == "" {
		t.Fatalf("announcement draft: %v %q", err, body)
	}
	if _, _, err := s.RecordAnnouncementDraft("org", parentP, breaks[0].EntryID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent announcement draft should be forbidden: %v", err)
	}
}

func TestRebuildRecordsReplaysResultsAndKeepsManualAndRejected(t *testing.T) {
	s := newRecordsFixture(t)
	// Results recorded before any record existed and then wiped out of the
	// record table (simulating pre-feature history).
	swim(t, s, "amy", "m-2025", 63000)
	swim(t, s, "amy", "m-2026", 61000)
	b := swim(t, s, "bea", "m-2026", 60500)
	for id := range s.recordEntries {
		delete(s.recordEntries, id)
	}
	if _, err := s.CreateManualRecord("org", "admin", adminP, CreateManualRecordInput{
		Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free", TimeMs: 62500,
		SwimmerName: "Pat Past", SwimDate: "2010-01-01",
	}); err != nil {
		t.Fatal(err)
	}
	summary, err := s.RebuildRecords("org", "admin", adminP)
	if err != nil {
		t.Fatal(err)
	}
	if summary.EntriesCreated == 0 {
		t.Fatalf("expected entries created: %+v", summary)
	}
	h, _ := s.GetRecordHistory("org", coachP, RecordKey{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"})
	// 11-12: Pat 62.50 (2010) → Amy's 63.00 is not a record → Amy 61.00 (2026, still 11).
	if len(h.Progression) != 2 || h.Progression[0].TimeMs != 61000 || h.Progression[1].SwimmerName != "Pat Past" {
		t.Fatalf("unexpected 11-12 progression: %+v", h.Progression)
	}
	open, _ := s.GetRecordHistory("org", coachP, RecordKey{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free"})
	if open.Current == nil || open.Current.TimeMs != 60500 {
		t.Fatalf("open record should be Bea 60.50: %+v", open.Current)
	}
	// Reject Bea's Open swim; a rebuild must not resurrect it.
	if _, err := s.DecideRecordEntry("org", "admin", open.Current.ID, adminP, false, "wrong course"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RebuildRecords("org", "admin", adminP); err != nil {
		t.Fatal(err)
	}
	open, _ = s.GetRecordHistory("org", coachP, RecordKey{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free"})
	if open.Current == nil || open.Current.TimeMs != 61000 || len(open.Rejected) != 1 {
		t.Fatalf("rejected swim must stay excluded after rebuild: %+v", open)
	}
	_ = b
	if _, err := s.RebuildRecords("org", "coach", coachP); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach rebuild: %v", err)
	}
}

func TestManualRecordValidationAndEdits(t *testing.T) {
	s := newRecordsFixture(t)
	bad := []CreateManualRecordInput{
		{Course: "XXX", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free", TimeMs: 1, SwimmerName: "x", SwimDate: "2000-01-01"},
		{Course: "SCY", Gender: "F", AgeGroup: "10U", Distance: 100, Stroke: "free", TimeMs: 1, SwimmerName: "x", SwimDate: "2000-01-01"},
		{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free_relay", TimeMs: 1, SwimmerName: "x", SwimDate: "2000-01-01"},
		{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free", TimeMs: 1, SwimmerName: "", SwimDate: "2000-01-01"},
		{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free", TimeMs: 1, SwimmerName: "x", SwimDate: "2999-01-01"},
		{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free", TimeMs: 1, SwimmerName: "x", SwimDate: "2000-01-01", SwimmerID: "ghost"},
	}
	for i, in := range bad {
		if _, err := s.CreateManualRecord("org", "admin", adminP, in); !errors.Is(err, ErrRecordInvalid) {
			t.Fatalf("case %d: expected ErrRecordInvalid, got %v", i, err)
		}
	}
	e, err := s.CreateManualRecord("org", "admin", adminP, CreateManualRecordInput{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free", TimeMs: 59000, SwimmerID: "amy", SwimDate: "2020-01-01"})
	if err != nil || e.SwimmerName != "Amy Ames" {
		t.Fatalf("manual with swimmer id should fill name: %v %+v", err, e)
	}
	verified := true
	tm := 58000
	_, after, err := s.UpdateRecordEntry("org", "admin", e.ID, adminP, UpdateRecordEntryInput{Verified: &verified, TimeMs: &tm})
	if err != nil || !after.Verified || after.VerifiedBy != "admin" || after.TimeMs != 58000 {
		t.Fatalf("update: %v %+v", err, after)
	}
	b := swim(t, s, "bea", "m-2026", 57000)
	var resultEntry string
	for _, x := range b {
		if x.AgeGroup == AgeGroupOpen {
			resultEntry = x.EntryID
		}
	}
	if _, _, err := s.UpdateRecordEntry("org", "admin", resultEntry, adminP, UpdateRecordEntryInput{TimeMs: &tm}); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("editing time of a result entry must fail: %v", err)
	}
	if _, err := s.DeleteManualRecord("org", "admin", resultEntry, adminP); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("deleting a result entry must fail: %v", err)
	}
	if _, err := s.DeleteManualRecord("org", "admin", e.ID, adminP); err != nil {
		t.Fatal(err)
	}
	// Families see records but not staff bookkeeping.
	recs, _ := s.ListRecords("org", parentP, ListRecordsFilters{})
	for _, r := range recs {
		if r.PendingCount != 0 || r.Record.CreatedBy != "" {
			t.Fatalf("parent view must be redacted: %+v", r)
		}
	}
}

func TestTopTimesOneSwimPerSwimmerAndFilters(t *testing.T) {
	s := newRecordsFixture(t)
	swim(t, s, "amy", "m-2025", 63000)
	swim(t, s, "amy", "m-2026", 61000)
	swim(t, s, "bea", "m-2025", 61000)
	swim(t, s, "old", "m-2026", 58000)
	lists, err := s.TopTimes("org", parentP, TopTimesQuery{Course: "SCY", Gender: "F", Distance: 100, Stroke: "free"})
	if err != nil || len(lists) != 1 {
		t.Fatalf("top times: %v %+v", err, lists)
	}
	e := lists[0].Entries
	if len(e) != 3 || e[0].SwimmerID != "old" || e[1].Rank != 2 || e[2].Rank != 2 {
		t.Fatalf("expected Olga then tie at rank 2: %+v", e)
	}
	// Ties break by earlier swim: Bea (2025) before Amy (2026).
	if e[1].SwimmerID != "bea" {
		t.Fatalf("tie should list earlier swim first: %+v", e)
	}
	// 11-12 list only counts swims done at 11-12: Amy (11 both meets) and Bea (12 at 2025 meet).
	lists, _ = s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"})
	if len(lists) != 1 || len(lists[0].Entries) != 2 {
		t.Fatalf("11-12 list: %+v", lists)
	}
	// Date window: only the 2025 meet.
	lists, _ = s.TopTimes("org", coachP, TopTimesQuery{Course: "SCY", Gender: "F", From: "2025-09-01", To: "2025-12-31", Limit: 1})
	if len(lists) != 1 || len(lists[0].Entries) != 1 || lists[0].Entries[0].SwimmerID != "bea" {
		t.Fatalf("windowed list: %+v", lists)
	}
	if _, err := s.TopTimes("org", coachP, TopTimesQuery{AgeGroup: "bogus"}); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("bad age group: %v", err)
	}
}

func TestGoalsProgressAndHouseholdScope(t *testing.T) {
	s := newRecordsFixture(t)
	swim(t, s, "amy", "m-2025", 64000)
	g, err := s.CreateGoal("org", "parent", parentP, "amy", GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000, Deadline: "2027-03-01"})
	if err != nil {
		t.Fatal(err)
	}
	if g.BaselineTimeMs == nil || *g.BaselineTimeMs != 64000 || g.Achieved || *g.ProgressPercent != 0 {
		t.Fatalf("new goal: %+v", g)
	}
	if _, err := s.CreateGoal("org", "parent", parentP, "bea", GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent goal for foreign swimmer: %v", err)
	}
	swim(t, s, "amy", "m-2026", 62000)
	goals, _ := s.ListGoals("org", parentP, "amy", false)
	if len(goals) != 1 || *goals[0].ProgressPercent != 50 || *goals[0].GapMs != 2000 {
		t.Fatalf("expected halfway progress: %+v", goals)
	}
	swim(t, s, "amy", "m-2026", 59900)
	goals, _ = s.ListGoals("org", coachP, "amy", false)
	if !goals[0].Achieved || *goals[0].ProgressPercent != 100 {
		t.Fatalf("expected achieved: %+v", goals[0])
	}
	archived := "archived"
	if _, err := s.UpdateGoal("org", "parent", parentP, "amy", g.ID, UpdateGoalInput{Status: &archived}); err != nil {
		t.Fatal(err)
	}
	if goals, _ := s.ListGoals("org", parentP, "amy", false); len(goals) != 0 {
		t.Fatal("archived goals are hidden by default")
	}
	if err := s.DeleteGoal("org", "x", &appmiddleware.Principal{Roles: []string{"parent"}, HouseholdID: "hh-2"}, "amy", g.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign parent delete: %v", err)
	}
	if err := s.DeleteGoal("org", "coach", coachP, "amy", g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGoal("org", "coach", coachP, "amy", GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000, Deadline: "soon"}); !errors.Is(err, ErrGoalInvalid) {
		t.Fatalf("bad deadline: %v", err)
	}
	if _, err := s.CreateGoal("org", "coach", coachP, "ghost", GoalInput{Distance: 100, Stroke: "free", Course: "SCY", TargetTimeMs: 60000}); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("unknown swimmer: %v", err)
	}
}

func TestProgressReportDropsAndMostImproved(t *testing.T) {
	s := newRecordsFixture(t)
	swim(t, s, "amy", "m-2025", 64000)
	swim(t, s, "amy", "m-2026", 60800) // 5% drop
	swim(t, s, "bea", "m-2025", 60000)
	swim(t, s, "bea", "m-2026", 59400) // 1% drop
	swim(t, s, "old", "m-2026", 58000) // new event only
	in := ProgressReportInput{FromA: "2025-09-01", ToA: "2025-12-31", FromB: "2026-01-01", ToB: "2026-08-31"}
	rep, err := s.ProgressReport("org", coachP, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.MostImproved) != 2 || rep.MostImproved[0].SwimmerID != "amy" || rep.MostImproved[0].AvgPercent != 5 || rep.MostImproved[1].AvgPercent != 1 {
		t.Fatalf("most improved: %+v", rep.MostImproved)
	}
	for _, sp := range rep.Swimmers {
		if sp.SwimmerID == "old" && (sp.NewEvents != 1 || sp.EventsCompared != 0) {
			t.Fatalf("olga should have one new event: %+v", sp)
		}
	}
	in.GroupCode = "senior"
	rep, _ = s.ProgressReport("org", coachP, in)
	if len(rep.Swimmers) != 1 || rep.Swimmers[0].SwimmerID != "bea" {
		t.Fatalf("group filter: %+v", rep.Swimmers)
	}
	in.GroupCode = ""
	rep, _ = s.ProgressReport("org", parentP, in)
	if len(rep.Swimmers) != 1 || rep.Swimmers[0].SwimmerID != "amy" {
		t.Fatalf("parent should only see own household: %+v", rep.Swimmers)
	}
	if _, err := s.ProgressReport("org", coachP, ProgressReportInput{FromA: "2026-01-01", ToA: "2025-01-01", FromB: "2026-01-01", ToB: "2026-02-01"}); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("inverted range: %v", err)
	}
}

func TestOptimizeRelayFromServiceUsesAgeAndGender(t *testing.T) {
	s := newRecordsFixture(t)
	for _, sw := range []string{"amy", "bea"} {
		ms := 30000
		if _, err := s.UpsertResult("org", "coach", coachP, UpsertResultInput{SwimmerID: sw, Distance: 50, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.OptimizeRelay("org", coachP, RelayOptimizeInput{MeetID: "m-2026", Gender: "F", AgeGroup: "11-12", RelayType: "free", Distance: 200})
	if err != nil {
		t.Fatal(err)
	}
	// Only Amy is 11-12 on 2026-02-20 (Bea is 13); not enough for a relay.
	if res.Course != "SCY" || len(res.Pool) != 1 || res.Pool[0].SwimmerID != "amy" || len(res.Teams) != 0 || len(res.Warnings) == 0 {
		t.Fatalf("unexpected relay result: %+v", res)
	}
	if _, err := s.OptimizeRelay("org", parentP, RelayOptimizeInput{Gender: "F", RelayType: "free", Distance: 200, Course: "SCY"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent relay builder: %v", err)
	}
}
