package performance

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// cov1xEntry inserts a record entry straight into the cache with sensible defaults.
func cov1xEntry(s *Service, id string, mut func(*RecordEntry)) *RecordEntry {
	e := &RecordEntry{
		ID: id, OrgID: "org", Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free",
		TimeMs: 60000, SwimmerName: "Amy Ames", SwimDate: "2025-01-01", Source: RecordSourceManual,
		Status: RecordStatusAccepted, CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if mut != nil {
		mut(e)
	}
	s.recordEntries[e.ID] = e
	return e
}

func TestCov1xRecordKeyValidateLabelAndOrdering(t *testing.T) {
	ok := RecordKey{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := ok
	bad.Gender = "X"
	if err := bad.Validate(); !errors.Is(err, ErrRecordInvalid) || !strings.Contains(err.Error(), "gender must be M or F") {
		t.Fatalf("gender = %v", err)
	}
	bad = ok
	bad.Distance = 0
	if err := bad.Validate(); !errors.Is(err, ErrRecordInvalid) || !strings.Contains(err.Error(), "distance out of range") {
		t.Fatalf("distance = %v", err)
	}
	bad.Distance = 1651
	if err := bad.Validate(); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("distance = %v", err)
	}

	labels := map[string]RecordKey{
		"Girls 8 & Under 50 SCY Breaststroke": {Course: "SCY", Gender: "F", AgeGroup: "8U", Distance: 50, Stroke: "breast"},
		"Boys 9-10 200 SCM IM":                {Course: "SCM", Gender: "M", AgeGroup: "9-10", Distance: 200, Stroke: "im"},
		"Men Open 100 LCM Backstroke":         {Course: "LCM", Gender: "M", AgeGroup: AgeGroupOpen, Distance: 100, Stroke: "back"},
		"Women Open 100 SCY Butterfly":        {Course: "SCY", Gender: "F", AgeGroup: AgeGroupOpen, Distance: 100, Stroke: "fly"},
	}
	for want, k := range labels {
		if got := k.Label(); got != want {
			t.Errorf("Label() = %q, want %q", got, want)
		}
	}
	if got := strokeLabel("medley_relay"); got != "medley_relay" {
		t.Fatalf("unknown stroke label = %q", got)
	}

	if ageGroupIndex("nope") != len(AgeGroups) || ageGroupIndex("9-10") != 1 {
		t.Fatal("ageGroupIndex")
	}
	if courseIndex("LCM") != 2 || courseIndex("???") != 3 {
		t.Fatal("courseIndex")
	}
	base := RecordKey{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"}
	vary := func(f func(*RecordKey)) RecordKey { k := base; f(&k); return k }
	order := []struct {
		name string
		a, b RecordKey
	}{
		{"course", base, vary(func(k *RecordKey) { k.Course = "LCM" })},
		{"gender", base, vary(func(k *RecordKey) { k.Gender = "M" })},
		{"age group", base, vary(func(k *RecordKey) { k.AgeGroup = "13-14" })},
		{"stroke", base, vary(func(k *RecordKey) { k.Stroke = "fly" })},
		{"distance", base, vary(func(k *RecordKey) { k.Distance = 200 })},
	}
	for _, tc := range order {
		if !lessRecordKey(tc.a, tc.b) || lessRecordKey(tc.b, tc.a) {
			t.Errorf("%s: ordering is not strict", tc.name)
		}
	}
	if lessRecordKey(base, base) {
		t.Fatal("a key is not less than itself")
	}

	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	x := &RecordEntry{TimeMs: 5, SwimDate: "2025-01-01", CreatedAt: t0}
	y := &RecordEntry{TimeMs: 5, SwimDate: "2025-01-01", CreatedAt: t0.Add(time.Second)}
	if !fasterEntry(x, y) || fasterEntry(y, x) {
		t.Fatal("equal time and date resolves to the earlier creation")
	}
	if copyEntry(nil) != nil {
		t.Fatal("copyEntry(nil)")
	}
}

func TestCov1xEvaluateSlotLockedRecordBlocksSlowerBackDatedSwim(t *testing.T) {
	old := &RecordEntry{ID: "old", Status: RecordStatusAccepted, TimeMs: 70000, SwimDate: "2025-01-01"}
	locked := &RecordEntry{ID: "cur", Status: RecordStatusAccepted, TimeMs: 60000, SwimDate: "2026-01-01", Locked: true}
	// Faster than the entry dated before it, but slower than the locked standing record.
	if v := evaluateSlot([]*RecordEntry{old, locked}, "r", 65000, "2025-06-01"); v.Create {
		t.Fatalf("a swim that does not beat the locked record must not create an entry: %+v", v)
	}
	// Beating the locked record produces a pending entry that remembers the swim it beat.
	v := evaluateSlot([]*RecordEntry{old, locked}, "r", 59000, "2026-06-01")
	if !v.Create || v.Status != RecordStatusPending || v.Previous == nil || v.Previous.ID != "cur" {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestCov1xRecordsLoadFailuresPropagate(t *testing.T) {
	s := cov1xUnloaded(t)
	key := RecordKey{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"}
	calls := map[string]func() error{
		"ListRecords":      func() error { _, err := s.ListRecords("org", adminP, ListRecordsFilters{}); return err },
		"GetRecordHistory": func() error { _, err := s.GetRecordHistory("org", adminP, key); return err },
		"ListPending":      func() error { _, err := s.ListPendingRecords("org", adminP); return err },
		"GetRecordEntry":   func() error { _, err := s.GetRecordEntry("org", adminP, "x"); return err },
		"CreateManualRecord": func() error {
			_, err := s.CreateManualRecord("org", "a", adminP, CreateManualRecordInput{})
			return err
		},
		"UpdateRecordEntry": func() error {
			_, _, err := s.UpdateRecordEntry("org", "a", "x", adminP, UpdateRecordEntryInput{})
			return err
		},
		"DecideRecordEntry":  func() error { _, err := s.DecideRecordEntry("org", "a", "x", adminP, true, ""); return err },
		"DeleteManualRecord": func() error { _, err := s.DeleteManualRecord("org", "a", "x", adminP); return err },
		"RebuildRecords":     func() error { _, err := s.RebuildRecords("org", "a", adminP); return err },
		"Announcement":       func() error { _, _, err := s.RecordAnnouncementDraft("org", adminP, "x"); return err },
	}
	for name, call := range calls {
		err := call()
		if err == nil || errors.Is(err, ErrForbidden) || errors.Is(err, ErrRecordNotFound) {
			t.Errorf("%s: want the load error, got %v", name, err)
		}
	}
}

func TestCov1xRecordsRoleChecks(t *testing.T) {
	s := newRecordsFixture(t)
	e := cov1xEntry(s, "e1", nil)
	key := e.key()
	if _, err := s.ListRecords("org", cov1xGuest, ListRecordsFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListRecords(guest) = %v", err)
	}
	if _, err := s.GetRecordHistory("org", cov1xGuest, key); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetRecordHistory(guest) = %v", err)
	}
	if _, err := s.GetRecordHistory("org", adminP, RecordKey{}); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("GetRecordHistory(bad key) = %v", err)
	}
	if _, err := s.ListPendingRecords("org", parentP); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListPendingRecords(parent) = %v", err)
	}
	if _, err := s.GetRecordEntry("org", parentP, "e1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetRecordEntry(parent) = %v", err)
	}
	if _, err := s.DeleteManualRecord("org", "c", "e1", coachP); !errors.Is(err, ErrForbidden) {
		t.Fatalf("DeleteManualRecord(coach) = %v", err)
	}
	if _, err := s.DeleteManualRecord("org", "a", "nope", adminP); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("DeleteManualRecord(missing) = %v", err)
	}
	if _, err := s.DeleteManualRecord("other", "a", "e1", adminP); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("DeleteManualRecord(other org) = %v", err)
	}
	if _, err := s.GetRecordEntry("org", coachP, "nope"); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("GetRecordEntry(missing) = %v", err)
	}
	if _, err := s.GetRecordEntry("other", coachP, "e1"); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("GetRecordEntry(other org) = %v", err)
	}
	got, err := s.GetRecordEntry("org", coachP, "e1")
	if err != nil || got == e {
		t.Fatalf("GetRecordEntry must return a copy: %v same=%v", err, got == e)
	}
	if _, err := s.RebuildRecords("org", "c", coachP); !errors.Is(err, ErrForbidden) {
		t.Fatalf("RebuildRecords(coach) = %v", err)
	}
	if _, _, err := s.RecordAnnouncementDraft("org", parentP, "e1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("announcement(parent) = %v", err)
	}
}

func TestCov1xListRecordsFiltersScopeAndPendingOnlySlots(t *testing.T) {
	s := newRecordsFixture(t)
	cov1xEntry(s, "a", nil)
	cov1xEntry(s, "b", func(e *RecordEntry) {
		e.Course = "SCM"
		e.Gender = "M"
		e.AgeGroup = "13-14"
		e.Stroke = "fly"
		e.Distance = 50
	})
	cov1xEntry(s, "foreign", func(e *RecordEntry) { e.OrgID = "other" })
	cov1xEntry(s, "pending-only", func(e *RecordEntry) {
		e.Course = "LCM"
		e.Status = RecordStatusPending
		e.Notes = "staff note"
	})

	staff, err := s.ListRecords("org", coachP, ListRecordsFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(staff) != 3 || staff[0].Course != "SCY" || staff[2].Course != "LCM" {
		t.Fatalf("staff board = %+v", staff)
	}
	if staff[2].Record != nil || staff[2].PendingCount != 1 {
		t.Fatalf("a slot with only pending entries shows to staff without a record: %+v", staff[2])
	}
	fam, _ := s.ListRecords("org", parentP, ListRecordsFilters{})
	if len(fam) != 2 {
		t.Fatalf("families do not see pending-only slots, got %d", len(fam))
	}
	for name, f := range map[string]ListRecordsFilters{
		"course": {Course: "SCM"}, "gender": {Gender: "M"}, "age": {AgeGroup: "13-14"}, "stroke": {Stroke: "fly"}, "distance": {Distance: 50},
	} {
		got, _ := s.ListRecords("org", adminP, f)
		if len(got) != 1 || got[0].Course != "SCM" {
			t.Errorf("%s filter = %+v", name, got)
		}
	}
}

func TestCov1xRecordHistoryStaffAndFamilyViews(t *testing.T) {
	s := newRecordsFixture(t)
	cov1xEntry(s, "cur", func(e *RecordEntry) { e.TimeMs = 60000; e.Notes = "secret"; e.CreatedBy = "admin" })
	cov1xEntry(s, "p-slow", func(e *RecordEntry) { e.Status = RecordStatusPending; e.TimeMs = 59500; e.SwimDate = "2026-01-02" })
	cov1xEntry(s, "p-fast", func(e *RecordEntry) { e.Status = RecordStatusPending; e.TimeMs = 59000; e.SwimDate = "2026-01-03" })
	cov1xEntry(s, "r-old", func(e *RecordEntry) { e.Status = RecordStatusRejected; e.SwimDate = "2024-01-01" })
	cov1xEntry(s, "r-new", func(e *RecordEntry) { e.Status = RecordStatusRejected; e.SwimDate = "2026-05-01" })
	key := RecordKey{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"}

	h, err := s.GetRecordHistory("org", coachP, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Pending) != 2 || h.Pending[0].ID != "p-fast" || h.Pending[1].ID != "p-slow" {
		t.Fatalf("pending must be fastest first: %+v", h.Pending)
	}
	if len(h.Rejected) != 2 || h.Rejected[0].ID != "r-new" {
		t.Fatalf("rejected must be newest first: %+v", h.Rejected)
	}
	if h.Current == nil || h.Current.Notes != "secret" {
		t.Fatalf("staff keep notes: %+v", h.Current)
	}

	f, _ := s.GetRecordHistory("org", parentP, key)
	if len(f.Pending) != 0 || len(f.Rejected) != 0 {
		t.Fatalf("families never see pending/rejected: %+v", f)
	}
	if f.Current.Notes != "" || f.Current.CreatedBy != "" {
		t.Fatalf("family view must be redacted: %+v", f.Current)
	}
	if s.recordEntries["cur"].Notes != "secret" {
		t.Fatal("redaction must not alter stored entries")
	}

	pend, _ := s.ListPendingRecords("org", coachP)
	if len(pend) != 2 {
		t.Fatalf("pending list = %d", len(pend))
	}
	cov1xEntry(s, "p-other-org", func(e *RecordEntry) { e.OrgID = "other"; e.Status = RecordStatusPending })
	cov1xEntry(s, "p-scm", func(e *RecordEntry) { e.Status = RecordStatusPending; e.Course = "SCM" })
	pend, _ = s.ListPendingRecords("org", coachP)
	if len(pend) != 3 || pend[2].Course != "SCM" {
		t.Fatalf("pending list is org scoped and ordered by slot: %+v", pend)
	}
}

func TestCov1xCreateManualRecordValidation(t *testing.T) {
	s := newRecordsFixture(t)
	good := CreateManualRecordInput{
		Course: "scy", Gender: "f", AgeGroup: "11-12", Distance: 100, Stroke: "FREE", TimeMs: 60000,
		SwimmerName: "Amy Ames", SwimDate: "2024-03-01",
	}
	mut := func(f func(*CreateManualRecordInput)) CreateManualRecordInput { g := good; f(&g); return g }
	age2, age121 := 2, 121
	cases := map[string]struct {
		in   CreateManualRecordInput
		want string
	}{
		"bad key":         {mut(func(g *CreateManualRecordInput) { g.Course = "X" }), "course must be"},
		"zero time":       {mut(func(g *CreateManualRecordInput) { g.TimeMs = 0 }), "time_ms must be between"},
		"huge time":       {mut(func(g *CreateManualRecordInput) { g.TimeMs = 3_600_001 }), "time_ms must be between"},
		"unknown swimmer": {mut(func(g *CreateManualRecordInput) { g.SwimmerID = "ghost" }), "not a club swimmer"},
		"no name":         {mut(func(g *CreateManualRecordInput) { g.SwimmerName = " " }), "swimmer_name is required"},
		"long name":       {mut(func(g *CreateManualRecordInput) { g.SwimmerName = strings.Repeat("n", 121) }), "swimmer_name is required"},
		"bad date":        {mut(func(g *CreateManualRecordInput) { g.SwimDate = "yesterday" }), "swim_date must be"},
		"future date":     {mut(func(g *CreateManualRecordInput) { g.SwimDate = time.Now().AddDate(1, 0, 0).Format("2006-01-02") }), "cannot be in the future"},
		"ancient date":    {mut(func(g *CreateManualRecordInput) { g.SwimDate = "1850-01-01" }), "too early"},
		"long notes":      {mut(func(g *CreateManualRecordInput) { g.Notes = strings.Repeat("n", 1001) }), "too long"},
		"long meet":       {mut(func(g *CreateManualRecordInput) { g.MeetName = strings.Repeat("m", 201) }), "too long"},
		"age too low":     {mut(func(g *CreateManualRecordInput) { g.SwimmerAge = &age2 }), "swimmer_age out of range"},
		"age too high":    {mut(func(g *CreateManualRecordInput) { g.SwimmerAge = &age121 }), "swimmer_age out of range"},
	}
	for name, tc := range cases {
		_, err := s.CreateManualRecord("org", "admin", adminP, tc.in)
		if !errors.Is(err, ErrRecordInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want ErrRecordInvalid containing %q", name, err, tc.want)
		}
	}
	if _, err := s.CreateManualRecord("org", "c", coachP, good); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach = %v", err)
	}
	if len(s.recordEntries) != 0 {
		t.Fatalf("rejected input must store nothing, have %d", len(s.recordEntries))
	}

	// A known swimmer without a typed name borrows the directory name; normalization uppercases/lowercases the key.
	withID := good
	withID.SwimmerName, withID.SwimmerID, withID.Locked, withID.Verified = "", "amy", true, true
	e, err := s.CreateManualRecord("org", "admin", adminP, withID)
	if err != nil {
		t.Fatal(err)
	}
	if e.SwimmerName != "Amy Ames" || e.Course != "SCY" || e.Gender != "F" || e.Stroke != "free" || !e.Locked || !e.Verified || e.VerifiedBy != "admin" || e.VerifiedAt == nil {
		t.Fatalf("created entry = %+v", e)
	}

	// Without a directory an unknown swimmer id is accepted as typed.
	bare := NewService()
	e2, err := bare.CreateManualRecord("org", "admin", adminP, mut(func(g *CreateManualRecordInput) { g.SwimmerID = "legacy-id" }))
	if err != nil || e2.SwimmerID != "legacy-id" {
		t.Fatalf("no-directory create = %+v %v", e2, err)
	}

	cov1xBreakPersistence(t, bare, "org")
	if _, err := bare.CreateManualRecord("org", "admin", adminP, good); err == nil || !strings.Contains(err.Error(), "persist record entry") {
		t.Fatalf("persist failure = %v", err)
	}
	if len(bare.recordEntries) != 1 {
		t.Fatalf("failed create must not be cached, have %d", len(bare.recordEntries))
	}
}

func TestCov1xUpdateRecordEntryRulesAndFailures(t *testing.T) {
	s := newRecordsFixture(t)
	slow := cov1xEntry(s, "slow", func(e *RecordEntry) { e.TimeMs = 65000 })
	cov1xEntry(s, "fast", func(e *RecordEntry) {
		e.TimeMs = 60000
		e.Verified = true
		e.VerifiedBy = "x"
		now := time.Now()
		e.VerifiedAt = &now
	})
	cov1xEntry(s, "res", func(e *RecordEntry) { e.Source = RecordSourceResult; e.Course = "SCM" })
	pend := cov1xEntry(s, "pend", func(e *RecordEntry) { e.Status = RecordStatusPending; e.Course = "LCM" })

	tru, fls := true, false
	zero, huge := 0, 3_600_001
	blank, longName := "  ", strings.Repeat("n", 121)
	badDate, longMeet, longNotes := "nope", strings.Repeat("m", 201), strings.Repeat("n", 1001)
	invalid := map[string]struct {
		id   string
		in   UpdateRecordEntryInput
		want string
	}{
		"fact edit on result": {"res", UpdateRecordEntryInput{TimeMs: &huge}, "only manual entries"},
		"zero time":           {"slow", UpdateRecordEntryInput{TimeMs: &zero}, "time_ms out of range"},
		"huge time":           {"slow", UpdateRecordEntryInput{TimeMs: &huge}, "time_ms out of range"},
		"blank name":          {"slow", UpdateRecordEntryInput{SwimmerName: &blank}, "swimmer_name is required"},
		"long name":           {"slow", UpdateRecordEntryInput{SwimmerName: &longName}, "swimmer_name is required"},
		"bad date":            {"slow", UpdateRecordEntryInput{SwimDate: &badDate}, "swim_date must be"},
		"long meet":           {"slow", UpdateRecordEntryInput{MeetName: &longMeet}, "meet_name too long"},
		"long notes":          {"slow", UpdateRecordEntryInput{Notes: &longNotes}, "notes too long"},
		"lock pending":        {"pend", UpdateRecordEntryInput{Locked: &tru}, "only accepted entries can be locked"},
		"lock non-current":    {"slow", UpdateRecordEntryInput{Locked: &tru}, "only the current record can be locked"},
	}
	for name, tc := range invalid {
		_, _, err := s.UpdateRecordEntry("org", "admin", tc.id, adminP, tc.in)
		if !errors.Is(err, ErrRecordInvalid) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
	if _, _, err := s.UpdateRecordEntry("org", "c", "slow", coachP, UpdateRecordEntryInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach = %v", err)
	}
	if _, _, err := s.UpdateRecordEntry("org", "a", "ghost", adminP, UpdateRecordEntryInput{}); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("missing = %v", err)
	}
	if slow.TimeMs != 65000 || pend.Locked {
		t.Fatal("rejected updates must not change stored entries")
	}

	name, date, meet, notes, ms := "  Renamed  ", "2024-02-02", " Champs ", " note ", 64000
	before, after, err := s.UpdateRecordEntry("org", "admin", "slow", adminP, UpdateRecordEntryInput{
		SwimmerName: &name, SwimDate: &date, MeetName: &meet, Notes: &notes, TimeMs: &ms,
	})
	if err != nil {
		t.Fatal(err)
	}
	if before.TimeMs != 65000 || after.TimeMs != 64000 || after.SwimmerName != "Renamed" || after.SwimDate != "2024-02-02" || after.MeetName != "Champs" || after.Notes != "note" {
		t.Fatalf("before=%+v after=%+v", before, after)
	}

	// Lock the current record, then unlock it; unverify clears the verifier.
	if _, after, err = s.UpdateRecordEntry("org", "admin", "fast", adminP, UpdateRecordEntryInput{Locked: &tru}); err != nil || !after.Locked {
		t.Fatalf("lock current = %+v %v", after, err)
	}
	if _, after, err = s.UpdateRecordEntry("org", "admin", "fast", adminP, UpdateRecordEntryInput{Locked: &tru}); err != nil || !after.Locked {
		t.Fatalf("locking an already locked record is allowed: %+v %v", after, err)
	}
	if _, after, err = s.UpdateRecordEntry("org", "admin", "fast", adminP, UpdateRecordEntryInput{Locked: &fls, Verified: &fls}); err != nil {
		t.Fatal(err)
	}
	if after.Locked || after.Verified || after.VerifiedBy != "" || after.VerifiedAt != nil {
		t.Fatalf("unlock+unverify = %+v", after)
	}
	if _, after, _ = s.UpdateRecordEntry("org", "boss", "fast", adminP, UpdateRecordEntryInput{Verified: &tru}); !after.Verified || after.VerifiedBy != "boss" || after.VerifiedAt == nil {
		t.Fatalf("verify = %+v", after)
	}

	cov1xBreakPersistence(t, s, "org")
	if _, _, err := s.UpdateRecordEntry("org", "admin", "fast", adminP, UpdateRecordEntryInput{Notes: &notes}); err == nil || !strings.Contains(err.Error(), "persist record entry") {
		t.Fatalf("persist failure = %v", err)
	}
	if fast := s.recordEntries["fast"]; fast.Notes == "note" {
		t.Fatal("failed update must not change the cache")
	}
}

func TestCov1xDecideAndDeleteRecordEntryRules(t *testing.T) {
	s := newRecordsFixture(t)
	cov1xEntry(s, "pend", func(e *RecordEntry) { e.Source = RecordSourceResult; e.Status = RecordStatusPending })
	cov1xEntry(s, "rej", func(e *RecordEntry) { e.Source = RecordSourceResult; e.Status = RecordStatusRejected })
	cov1xEntry(s, "man", nil)
	cov1xEntry(s, "acc", func(e *RecordEntry) { e.Source = RecordSourceResult; e.Course = "SCM"; e.Locked = true })

	if _, err := s.DecideRecordEntry("org", "c", "pend", coachP, true, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach = %v", err)
	}
	if _, err := s.DecideRecordEntry("org", "a", "ghost", adminP, true, ""); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("missing = %v", err)
	}
	if _, err := s.DecideRecordEntry("org", "a", "man", adminP, true, ""); !errors.Is(err, ErrRecordNotPending) {
		t.Fatalf("approve accepted = %v", err)
	}
	if _, err := s.DecideRecordEntry("org", "a", "rej", adminP, false, ""); !errors.Is(err, ErrRecordNotPending) {
		t.Fatalf("reject rejected = %v", err)
	}
	if _, err := s.DecideRecordEntry("org", "a", "man", adminP, false, ""); !errors.Is(err, ErrRecordInvalid) || !strings.Contains(err.Error(), "delete manual entries") {
		t.Fatalf("reject manual = %v", err)
	}
	if _, err := s.DecideRecordEntry("org", "a", "pend", adminP, true, strings.Repeat("n", 1001)); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("long notes = %v", err)
	}
	if s.recordEntries["pend"].Status != RecordStatusPending {
		t.Fatal("failed decision must not change the entry")
	}

	cov1xBreakPersistence(t, s, "org")
	if _, err := s.DecideRecordEntry("org", "a", "pend", adminP, true, ""); err == nil || !strings.Contains(err.Error(), "persist record entry") {
		t.Fatalf("approve persist failure = %v", err)
	}
	if _, err := s.DeleteManualRecord("org", "a", "man", adminP); err == nil || !strings.Contains(err.Error(), "delete record entry") {
		t.Fatalf("delete persist failure = %v", err)
	}
	if s.recordEntries["pend"].Status != RecordStatusPending || s.recordEntries["man"] == nil {
		t.Fatal("persistence failures must leave the cache unchanged")
	}
	s.repo = nil

	got, err := s.DecideRecordEntry("org", "a", "acc", adminP, false, "  bad swim  ")
	if err != nil || got.Status != RecordStatusRejected || got.Locked || got.Notes != "bad swim" {
		t.Fatalf("reject = %+v %v", got, err)
	}
	if _, err := s.DeleteManualRecord("org", "a", "acc", adminP); !errors.Is(err, ErrRecordInvalid) {
		t.Fatalf("delete result entry = %v", err)
	}
	del, err := s.DeleteManualRecord("org", "a", "man", adminP)
	if err != nil || del.ID != "man" || s.recordEntries["man"] != nil {
		t.Fatalf("delete manual = %+v %v", del, err)
	}
}

func TestCov1xRebuildRecordsSkipsAndPersistenceFailures(t *testing.T) {
	s := newRecordsFixture(t)
	t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	mk := func(org, swimmer, gender string, ms *int, official bool, stroke string) *Result {
		t.Helper()
		r, err := s.UpsertResult(org, "coach", coachP, UpsertResultInput{
			SwimmerID: swimmer, Distance: 100, Stroke: stroke, Course: "SCY", Gender: gender, FinTimeMs: ms, IsOfficial: official,
		})
		if err != nil {
			t.Fatal(err)
		}
		r.CreatedAt = t0
		return r
	}
	a, b := 62000, 61000
	mk("org", "amy", "F", &a, true, "free")
	mk("org", "bea", "F", &b, true, "free")
	mk("other", "amy", "F", &b, true, "free") // another club
	mk("org", "amy", "F", &a, false, "free")  // unofficial
	mk("org", "ghost", "X", &a, true, "free") // gender cannot be determined
	mk("org", "amy", "F", &a, true, "fly")    // different slot
	dq := mk("org", "amy", "F", nil, true, "back")
	dq.DQ = true

	sum, err := s.RebuildRecords("org", "admin", adminP)
	if err != nil {
		t.Fatal(err)
	}
	// Live detection stored one entry per slot a swim broke (6); the replay order of
	// same-instant swims is by id, so the Open slot may need one entry or two.
	if sum.EntriesRemoved != 6 || (sum.EntriesCreated != 5 && sum.EntriesCreated != 6) {
		t.Fatalf("summary = %+v", sum)
	}
	openFree := RecordKey{Course: "SCY", Gender: "F", AgeGroup: AgeGroupOpen, Distance: 100, Stroke: "free"}
	h, _ := s.GetRecordHistory("org", adminP, openFree)
	if h.Current == nil || h.Current.TimeMs != 61000 || h.Current.SwimmerName != "Bea Bell" {
		t.Fatalf("open record after rebuild = %+v", h.Current)
	}
	otherClub := 0
	for _, e := range s.recordEntries {
		if e.OrgID == "other" {
			otherClub++
			continue
		}
		if e.TimeMs == 0 || e.SwimmerName == "Unknown swimmer" {
			t.Fatalf("unexpected entry %+v", e)
		}
	}
	if otherClub != 2 {
		t.Fatalf("another club's entries must be untouched by this club's rebuild, have %d", otherClub)
	}

	// A locked slot is left untouched by a rebuild.
	young := RecordKey{Course: "SCY", Gender: "F", AgeGroup: "11-12", Distance: 100, Stroke: "free"}
	hy, _ := s.GetRecordHistory("org", adminP, young)
	tru := true
	if _, _, err := s.UpdateRecordEntry("org", "admin", hy.Current.ID, adminP, UpdateRecordEntryInput{Locked: &tru}); err != nil {
		t.Fatal(err)
	}
	sum2, err := s.RebuildRecords("org", "admin", adminP)
	if err != nil || sum2.SlotsSkippedLocked != 1 {
		t.Fatalf("locked slot must be skipped: %+v %v", sum2, err)
	}
	if s.recordEntries[hy.Current.ID] == nil {
		t.Fatal("locked entry must survive a rebuild")
	}

	// Removing stale entries needs the database: a failed delete aborts the rebuild.
	cov1xBreakPersistence(t, s, "org")
	if _, err := s.RebuildRecords("org", "admin", adminP); err == nil || !strings.Contains(err.Error(), "delete record entry") {
		t.Fatalf("delete failure = %v", err)
	}

	// With no stale entries the failing step is writing the replayed ones.
	fresh := newRecordsFixture(t)
	// Inserted directly so no live-detection entry exists to be removed first.
	fresh.results["r1"] = &Result{
		ID: "r1", OrgID: "org", SwimmerID: "amy", Distance: 50, Stroke: "free", Course: "SCY", Gender: "F",
		FinTimeMs: &a, IsOfficial: true, Status: "active", CreatedAt: t0,
	}
	cov1xBreakPersistence(t, fresh, "org")
	if _, err := fresh.RebuildRecords("org", "admin", adminP); err == nil || !strings.Contains(err.Error(), "persist record entry") {
		t.Fatalf("persist failure = %v", err)
	}
}

func TestCov1xFormatSwimTimeAndAnnouncementDraft(t *testing.T) {
	for ms, want := range map[int]string{28004: "28.00", 62350: "1:02.35", 600000: "10:00.00"} {
		if got := FormatSwimTime(ms); got != want {
			t.Errorf("FormatSwimTime(%d) = %q, want %q", ms, got, want)
		}
	}
	s := newRecordsFixture(t)
	cov1xEntry(s, "old", func(e *RecordEntry) { e.TimeMs = 70000; e.SwimmerName = "Old Holder"; e.SwimDate = "2019-05-05" })
	cov1xEntry(s, "new", func(e *RecordEntry) {
		e.TimeMs = 62350
		e.SwimmerName = "Amy Ames"
		e.SwimDate = "2026-02-20"
		e.MeetName = "Winter Champs"
	})
	cov1xEntry(s, "pend", func(e *RecordEntry) { e.Status = RecordStatusPending })

	title, body, err := s.RecordAnnouncementDraft("org", coachP, "new")
	if err != nil {
		t.Fatal(err)
	}
	if title != "New team record: Amy Ames, Girls 11-12 100 SCY Freestyle" {
		t.Fatalf("title = %q", title)
	}
	for _, want := range []string{"1:02.35", "at Winter Champs", "on 2026-02-20", "previous record of 1:10.00 was held by Old Holder since 2019", "Congratulations!"} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q missing %q", body, want)
		}
	}
	if _, _, err := s.RecordAnnouncementDraft("org", coachP, "pend"); !errors.Is(err, ErrRecordInvalid) || !strings.Contains(err.Error(), "only accepted records") {
		t.Fatalf("pending announcement = %v", err)
	}
	if _, _, err := s.RecordAnnouncementDraft("org", coachP, "ghost"); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("missing announcement = %v", err)
	}
}

func TestCov1xDetectionSkipsUnattributableAndNonQualifyingSwims(t *testing.T) {
	s := newRecordsFixture(t)
	ms := 50000
	// Gender X for a swimmer the club does not know: no slot can be chosen.
	_, breaks, err := s.UpsertResultWithRecords("org", "coach", coachP, UpsertResultInput{
		SwimmerID: "ghost", Distance: 50, Stroke: "free", Course: "SCY", Gender: "X", FinTimeMs: &ms, IsOfficial: true,
	})
	if err != nil || len(breaks) != 0 || len(s.recordEntries) != 0 {
		t.Fatalf("unknown gender: breaks=%v err=%v entries=%d", breaks, err, len(s.recordEntries))
	}
	// A direct detection call on a non-qualifying result is a no-op.
	r := &Result{ID: "r", OrgID: "org", SwimmerID: "amy", Status: "active", IsOfficial: false, FinTimeMs: &ms, Stroke: "free"}
	dir := s.snapshotDirectory("org")
	s.mu.Lock()
	got, err := s.detectRecordBreaksLocked("org", "coach", r, dir)
	s.mu.Unlock()
	if err != nil || got != nil {
		t.Fatalf("non-qualifying = %v %v", got, err)
	}
}
