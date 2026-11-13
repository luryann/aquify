package billing

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov2xRunOrg = "org-cov2x-runs"

func cov2xRunAdmin() *appmiddleware.Principal { return cov2xAdmin(cov2xRunOrg) }

func cov2xBoolPtr(b bool) *bool { return &b }
func cov2xIntPtr(i int) *int    { return &i }

// cov2xScenario is a roster with every kind of billable and unbillable swimmer.
type cov2xScenario struct {
	svc    *Service
	admin  *appmiddleware.Principal
	roster *RosterSnapshot
}

func cov2xScenarioRoster(t *testing.T) RosterSnapshot {
	oct := cov2xDate(t, "2026-10-01")
	mid := cov2xDate(t, "2026-11-16")
	future := cov2xDate(t, "2026-12-01")
	return RosterSnapshot{
		Households: []RosterHousehold{
			{ID: "hh-a", Name: "Alpha", Status: "active"},
			{ID: "hh-b", Name: "Bravo", Status: "active"},
			{ID: "hh-d", Name: "Dormant", Status: "inactive"},
			{ID: "hh-adj", Name: "Adjonly", Status: "active"},
			{ID: "hh-gone", Name: "Gone", Status: "active"},
		},
		Groups: []RosterGroup{{Code: "SILVER", Name: "Silver"}, {Code: "GOLD", Name: "Gold"}},
		Swimmers: []RosterSwimmer{
			{ID: "sw-1", HouseholdID: "hh-a", FirstName: "Ann", LastName: "Alpha", Status: "active"},
			{ID: "sw-2", HouseholdID: "hh-a", FirstName: "Ben", LastName: "Alpha", Status: "pending"},
			{ID: "sw-3", HouseholdID: "hh-d", FirstName: "Cal", LastName: "Dormant", Status: "active"},
			{ID: "sw-4", HouseholdID: "hh-missing", FirstName: "Dan", LastName: "Nowhere", Status: "active"},
			{ID: "sw-5", HouseholdID: "hh-b", FirstName: "Eve", LastName: "Bravo", Status: "withdrawn"},
			{ID: "sw-6", HouseholdID: "hh-b", FirstName: "Fay", LastName: "Bravo", Status: "active"},
			{ID: "sw-7", HouseholdID: "hh-b", FirstName: "Gus", LastName: "Bravo", Status: "active"},
		},
		GroupLinks: []RosterGroupLink{
			{SwimmerID: "sw-1", GroupCode: "SILVER", AssignedAt: oct},
			{SwimmerID: "sw-1", GroupCode: "SILVER", AssignedAt: oct}, // duplicate link is billed once
			{SwimmerID: "sw-ghost", GroupCode: "SILVER", AssignedAt: oct},
			{SwimmerID: "sw-2", GroupCode: "SILVER", AssignedAt: oct},
			{SwimmerID: "sw-3", GroupCode: "SILVER", AssignedAt: oct},
			{SwimmerID: "sw-4", GroupCode: "SILVER", AssignedAt: oct},
			{SwimmerID: "sw-5", GroupCode: "SILVER", AssignedAt: oct},
			{SwimmerID: "sw-6", GroupCode: "SILVER", AssignedAt: future},
			{SwimmerID: "sw-7", GroupCode: "GOLD", AssignedAt: mid},
		},
		Registrations: []RosterRegistration{
			{SwimmerID: "sw-1", GroupCode: "SILVER", Season: "2026-27", Status: "active"},
			{SwimmerID: "sw-1", GroupCode: "SILVER", Season: " 2026-27 ", Status: "active"}, // duplicate
			{SwimmerID: "sw-2", GroupCode: "SILVER", Season: "2026-27", Status: "withdrawn"},
			{SwimmerID: "sw-6", GroupCode: "SILVER", Season: "2025-26", Status: "active"},
			{SwimmerID: "sw-3", GroupCode: "SILVER", Season: "2026-27", Status: "active"},
			{SwimmerID: "sw-ghost", GroupCode: "SILVER", Season: "2026-27", Status: "active"},
			{SwimmerID: "sw-7", GroupCode: "GOLD", Season: "2026-27", Status: "pending"},
		},
	}
}

func cov2xNewScenario(t *testing.T) *cov2xScenario {
	t.Helper()
	sc := &cov2xScenario{svc: NewService(), admin: cov2xRunAdmin()}
	r := cov2xScenarioRoster(t)
	sc.roster = &r
	sc.svc.SetRosterSource(func(string) (RosterSnapshot, error) { return *sc.roster, nil })

	create := func(in FeeScheduleInput) *FeeSchedule {
		t.Helper()
		got, err := sc.svc.CreateFeeSchedule(cov2xRunOrg, sc.admin.UserID, sc.admin, in)
		if err != nil {
			t.Fatalf("schedule %q: %v", in.Name, err)
		}
		return got
	}
	create(FeeScheduleInput{Name: "Silver dues", Kind: "dues", GroupCode: "silver", Frequency: "monthly", AmountCents: 15000, StartDate: "2026-09-01", EndDate: "2026-11-25", Proration: "daily"})
	create(FeeScheduleInput{Name: "Gold dues", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 12000, StartDate: "2026-09-01", Proration: "waive"})
	archived := create(FeeScheduleInput{Name: "Old silver dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 99999, StartDate: "2026-09-01"})
	if _, err := sc.svc.ArchiveFeeSchedule(cov2xRunOrg, sc.admin.UserID, sc.admin, archived.ID); err != nil {
		t.Fatal(err)
	}
	create(FeeScheduleInput{Name: "Next year dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 88888, StartDate: "2027-01-01"})
	create(FeeScheduleInput{Name: "Registration", Kind: "registration", Season: "2026-27", Frequency: "one_time", AmountCents: 5000, StartDate: "2026-09-01"})
	create(FeeScheduleInput{Name: "Gold registration", Kind: "registration", GroupCode: "GOLD", Season: "2026-27", Frequency: "one_time", AmountCents: 3000, StartDate: "2026-09-01"})

	adj := func(household string, cents int, reason string) *Adjustment {
		t.Helper()
		a, err := sc.svc.CreateAdjustment(cov2xRunOrg, sc.admin.UserID, sc.admin, AdjustmentInput{HouseholdID: household, AmountCents: cents, Reason: reason})
		if err != nil {
			t.Fatalf("adjustment %q: %v", reason, err)
		}
		return a
	}
	adj("hh-adj", 2500, "Merchandise")
	adj("hh-d", 100, "Dormant fee one")
	adj("hh-d", 200, "Dormant fee two")
	adj("hh-gone", 700, "Household later removed")
	voided := adj("hh-a", 4000, "Entered by mistake")
	if _, err := sc.svc.VoidAdjustment(cov2xRunOrg, sc.admin.UserID, sc.admin, voided.ID); err != nil {
		t.Fatal(err)
	}
	// The household disappears from the roster after its adjustment was entered.
	kept := sc.roster.Households[:0:0]
	for _, h := range sc.roster.Households {
		if h.ID != "hh-gone" {
			kept = append(kept, h)
		}
	}
	sc.roster.Households = kept
	return sc
}

type cov2xSkipKey struct{ household, swimmer, reason string }

func cov2xSkips(p *RunPreview) map[cov2xSkipKey]bool {
	out := map[cov2xSkipKey]bool{}
	for _, s := range p.Skipped {
		out[cov2xSkipKey{s.HouseholdID, s.SwimmerID, s.Reason}] = true
	}
	return out
}

func TestCov2xPreviewExplainsEveryDecision(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	input := RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}
	preview, err := sc.svc.PreviewBillingRun(cov2xRunOrg, sc.admin, input)
	if err != nil {
		t.Fatal(err)
	}

	totals := map[string]int{}
	lines := map[string][]RunPreviewLine{}
	for _, h := range preview.Households {
		totals[h.HouseholdID] = h.TotalCents
		lines[h.HouseholdID] = h.Lines
	}
	// hh-a: Ann's dues are cut off by the schedule end date (25 of 30 days) plus the season fee.
	// hh-b: Gus's gold dues are waived for the partial month; both registration fees apply.
	// hh-adj: only a manual adjustment.
	want := map[string]int{"hh-a": 12500 + 5000, "hh-b": 5000 + 3000, "hh-adj": 2500}
	if !reflect.DeepEqual(totals, want) {
		t.Fatalf("household totals %v, want %v", totals, want)
	}
	if preview.HouseholdCount != 3 || preview.ChargeCount != 4 || preview.NetCents != 28000 || preview.AdjustmentCents != 2500 || preview.SwimmerCount != 2 {
		t.Fatalf("preview totals: %+v", preview)
	}
	if got := lines["hh-a"][0]; got.Kind != "dues" || got.AmountCents != 12500 || got.Note != "prorated 25/30 days" || got.SwimmerName != "Ann Alpha" || got.SiblingPosition != 1 {
		t.Fatalf("Ann's dues line: %+v", got)
	}
	if got := lines["hh-adj"]; len(got) != 1 || got[0].Kind != "adjustment" || got[0].Description != "Adjustment: Merchandise" {
		t.Fatalf("adjustment line: %+v", got)
	}

	skips := cov2xSkips(preview)
	for _, want := range []cov2xSkipKey{
		{"hh-a", "sw-2", "swimmer registration is pending"},
		{"hh-d", "sw-3", "household is inactive"},
		{"hh-missing", "sw-4", "household not found"},
		{"hh-b", "sw-5", "swimmer is withdrawn"},
		{"hh-b", "sw-6", "joined after the run date"},
		{"hh-b", "sw-7", "waived for partial period"},
		{"hh-d", "", "household is inactive (adjustments stay pending)"},
	} {
		if !skips[want] {
			t.Errorf("missing skip %+v in %+v", want, preview.Skipped)
		}
	}
	// Two pending adjustments on the inactive household produce a single explanation.
	n := 0
	for _, s := range preview.Skipped {
		if s.HouseholdID == "hh-d" && s.SwimmerID == "" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("inactive household adjustment skip must appear once, got %d", n)
	}
	if len(preview.Skipped) != 8 { // the 7 above plus the inactive household's registration
		t.Fatalf("skips: %+v", preview.Skipped)
	}
	if !skips[cov2xSkipKey{"hh-d", "sw-3", "household is inactive"}] {
		t.Fatal("registration for the inactive household must be skipped too")
	}

	again, _ := sc.svc.PreviewBillingRun(cov2xRunOrg, sc.admin, input)
	if again.Fingerprint != preview.Fingerprint || len(preview.Fingerprint) != 24 {
		t.Fatalf("preview must be deterministic: %q vs %q", preview.Fingerprint, again.Fingerprint)
	}
	noAdj, err := sc.svc.PreviewBillingRun(cov2xRunOrg, sc.admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued", IncludeAdjustments: cov2xBoolPtr(false)})
	if err != nil || noAdj.HouseholdCount != 2 || noAdj.AdjustmentCents != 0 || noAdj.Fingerprint == preview.Fingerprint {
		t.Fatalf("excluding adjustments: %+v %v", noAdj, err)
	}

	// Commit exactly what was reviewed.
	input.ExpectedFingerprint = preview.Fingerprint
	res, err := sc.svc.CommitBillingRun(cov2xRunOrg, sc.admin.UserID, sc.admin, input, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.InvoiceCount != 3 || res.Run.NetCents != 28000 || res.Run.ChargeCount != 4 || res.Run.SkippedCount != len(preview.Skipped) {
		t.Fatalf("run: %+v", res.Run)
	}
	// The next preview says why nothing is billed twice.
	after, err := sc.svc.PreviewBillingRun(cov2xRunOrg, sc.admin, RunInput{AsOf: "2026-11-21"})
	if err != nil {
		t.Fatal(err)
	}
	if after.HouseholdCount != 0 {
		t.Fatalf("nothing left to bill: %+v", after.Households)
	}
	skips = cov2xSkips(after)
	if !skips[cov2xSkipKey{"hh-a", "sw-1", "already billed for this period"}] || !skips[cov2xSkipKey{"hh-a", "sw-1", "already billed for this season"}] {
		t.Fatalf("already-billed skips missing: %+v", after.Skipped)
	}
}

func TestCov2xNormalizeRunInput(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	schedules, _ := sc.svc.ListFeeSchedules(cov2xRunOrg, sc.admin)
	if len(schedules) < 2 {
		t.Fatal("scenario schedules missing")
	}
	// A schedule that belongs to another club must be indistinguishable from an unknown id.
	other := NewService()
	other.SetRosterSource(func(string) (RosterSnapshot, error) { return cov2xScenarioRoster(t), nil })
	foreign, err := other.CreateFeeSchedule("org-cov2x-other", "a", cov2xAdmin("org-cov2x-other"), FeeScheduleInput{Name: "X", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 100, StartDate: "2026-09-01"})
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.mu.Lock()
	sc.svc.finance.schedules[foreign.ID] = foreign
	sc.svc.mu.Unlock()

	var verr *FinanceValidationError
	for name, c := range map[string]struct {
		in    RunInput
		field string
	}{
		"bad as_of":         {RunInput{AsOf: "11/20/2026"}, "as_of"},
		"bad due_date":      {RunInput{AsOf: "2026-11-20", DueDate: "soon"}, "due_date"},
		"due before as_of":  {RunInput{AsOf: "2026-11-20", DueDate: "2026-11-19"}, "due_date"},
		"unknown schedule":  {RunInput{AsOf: "2026-11-20", ScheduleIDs: []string{"nope"}}, "schedule_ids"},
		"foreign schedule":  {RunInput{AsOf: "2026-11-20", ScheduleIDs: []string{foreign.ID}}, "schedule_ids"},
		"unknown with real": {RunInput{AsOf: "2026-11-20", ScheduleIDs: []string{schedules[0].ID, "nope"}}, "schedule_ids"},
	} {
		if _, err := sc.svc.normalizeRunInput(cov2xRunOrg, c.in); !errors.As(err, &verr) || verr.Field != c.field {
			t.Errorf("%s: want %s validation error, got %v", name, c.field, err)
		}
		if _, err := sc.svc.PreviewBillingRun(cov2xRunOrg, sc.admin, c.in); !errors.Is(err, ErrInvalidFinanceInput) {
			t.Errorf("%s via preview: %v", name, err)
		}
		if _, err := sc.svc.CommitBillingRun(cov2xRunOrg, sc.admin.UserID, sc.admin, c.in, RunTriggerManual); !errors.Is(err, ErrInvalidFinanceInput) {
			t.Errorf("%s via commit: %v", name, err)
		}
	}

	ok, err := sc.svc.normalizeRunInput(cov2xRunOrg, RunInput{
		AsOf: "2026-11-20", DueDays: cov2xIntPtr(7), InvoiceStatus: "issued", ExpectedFingerprint: "  abc  ",
		ScheduleIDs: []string{" " + schedules[1].ID + " ", "", schedules[0].ID, schedules[1].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{schedules[0].ID, schedules[1].ID}
	if wantIDs[0] > wantIDs[1] {
		wantIDs[0], wantIDs[1] = wantIDs[1], wantIDs[0]
	}
	if ok.DueDate != "2026-11-27" || ok.InvoiceStatus != "issued" || ok.Expected != "abc" || !ok.IncludeAdjustments || !reflect.DeepEqual(ok.ScheduleIDs, wantIDs) {
		t.Fatalf("normalised input: %+v want ids %v", ok, wantIDs)
	}
	def, err := sc.svc.normalizeRunInput(cov2xRunOrg, RunInput{AsOf: "2026-11-20", DueDate: " 2026-12-31 ", IncludeAdjustments: cov2xBoolPtr(false)})
	if err != nil || def.DueDate != "2026-12-31" || def.InvoiceStatus != "draft" || def.IncludeAdjustments || len(def.ScheduleIDs) != 0 {
		t.Fatalf("defaults: %+v %v", def, err)
	}
	dflt, _ := sc.svc.normalizeRunInput(cov2xRunOrg, RunInput{AsOf: "2026-11-20"})
	if dflt.DueDate != "2026-12-05" {
		t.Fatalf("default due window is %d days: %s", defaultRunDueDays, dflt.DueDate)
	}
}

func TestCov2xPreviewAndCommitErrors(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	in := RunInput{AsOf: "2026-11-20"}
	coach := cov2xCoach(cov2xRunOrg)
	if _, err := sc.svc.PreviewBillingRun(cov2xRunOrg, coach, in); !errors.Is(err, ErrForbidden) {
		t.Errorf("preview as coach: %v", err)
	}
	if _, err := sc.svc.CommitBillingRun(cov2xRunOrg, "c", coach, in, RunTriggerManual); !errors.Is(err, ErrForbidden) {
		t.Errorf("commit as coach: %v", err)
	}
	if _, err := sc.svc.ListBillingRuns(cov2xRunOrg, coach); !errors.Is(err, ErrForbidden) {
		t.Errorf("list as coach: %v", err)
	}
	if _, err := sc.svc.GetBillingRun(cov2xRunOrg, coach, "x"); !errors.Is(err, ErrForbidden) {
		t.Errorf("get as coach: %v", err)
	}

	boom := errors.New("roster offline")
	sc.svc.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, boom })
	if _, err := sc.svc.PreviewBillingRun(cov2xRunOrg, sc.admin, in); !errors.Is(err, boom) {
		t.Errorf("preview roster error: %v", err)
	}
	if _, err := sc.svc.CommitBillingRun(cov2xRunOrg, "a", sc.admin, in, RunTriggerManual); !errors.Is(err, boom) {
		t.Errorf("commit roster error: %v", err)
	}
	bare := NewService()
	if _, err := bare.PreviewBillingRun(cov2xRunOrg, sc.admin, in); !errors.Is(err, ErrRosterUnavailable) {
		t.Errorf("no roster source: %v", err)
	}

	empty := NewService()
	empty.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, nil })
	if _, err := empty.CommitBillingRun(cov2xRunOrg, "a", sc.admin, in, RunTriggerManual); !errors.Is(err, ErrNothingToBill) {
		t.Errorf("nothing to bill: %v", err)
	}
}

func TestCov2xStalePreviewErrorCarriesFreshPreview(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	_, err := sc.svc.CommitBillingRun(cov2xRunOrg, "a", sc.admin, RunInput{AsOf: "2026-11-20", ExpectedFingerprint: "stale"}, RunTriggerManual)
	var stale *StalePreviewError
	if !errors.As(err, &stale) || !errors.Is(err, ErrRunPreviewStale) {
		t.Fatalf("expected a stale preview error, got %v", err)
	}
	if stale.Error() != ErrRunPreviewStale.Error() || stale.Preview == nil || stale.Preview.HouseholdCount != 3 || stale.Preview.Fingerprint == "stale" {
		t.Fatalf("stale error must carry the fresh preview: %q %+v", stale.Error(), stale.Preview)
	}
	if got := sc.svc.finance.runs; len(got) != 0 {
		t.Fatal("a rejected commit must not record a run")
	}
}

func TestCov2xCommitNormalisesTriggerAndPersistsRun(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	first, err := sc.svc.CommitBillingRun(cov2xRunOrg, "a", sc.admin, RunInput{AsOf: "2026-11-20", ScheduleIDs: nil, IncludeAdjustments: cov2xBoolPtr(false)}, "bogus-trigger")
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.Trigger != RunTriggerManual || first.Run.CreatedBy != "a" || first.Run.Fingerprint == "" {
		t.Fatalf("unknown triggers fall back to manual: %+v", first.Run)
	}
	if len(first.Invoices) != 2 || first.Invoices[0].Status != "draft" {
		t.Fatalf("invoices: %+v", first.Invoices)
	}
	second, err := sc.svc.CommitBillingRun(cov2xRunOrg, "a", sc.admin, RunInput{AsOf: "2026-11-20"}, RunTriggerScheduler)
	if err != nil {
		t.Fatal(err)
	}
	if second.Run.Trigger != RunTriggerScheduler || second.Run.InvoiceCount != 1 {
		t.Fatalf("second run bills only the adjustment household: %+v", second.Run)
	}

	runs, err := sc.svc.ListBillingRuns(cov2xRunOrg, sc.admin)
	if err != nil || len(runs) != 2 || runs[0].ID != second.Run.ID || runs[1].ID != first.Run.ID {
		t.Fatalf("runs newest first: %+v %v", runs, err)
	}
	// Other clubs' runs never show up.
	sc.svc.mu.Lock()
	sc.svc.finance.runs["foreign"] = &BillingRun{ID: "foreign", OrgID: "org-else", CreatedAt: time.Now().Add(time.Hour)}
	sc.svc.mu.Unlock()
	if runs, _ := sc.svc.ListBillingRuns(cov2xRunOrg, sc.admin); len(runs) != 2 {
		t.Fatalf("foreign run leaked into the list: %d", len(runs))
	}
	if _, err := sc.svc.GetBillingRun(cov2xRunOrg, sc.admin, "foreign"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("foreign run: %v", err)
	}
	if _, err := sc.svc.GetBillingRun(cov2xRunOrg, sc.admin, "missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("missing run: %v", err)
	}
	detail, err := sc.svc.GetBillingRun(cov2xRunOrg, sc.admin, first.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Charges) != 4 || len(detail.Invoices) != 2 || detail.Run.ID != first.Run.ID {
		t.Fatalf("detail: %+v", detail)
	}
	if detail.Invoices[0].InvoiceNumber > detail.Invoices[1].InvoiceNumber {
		t.Fatalf("invoices sorted by number: %+v", detail.Invoices)
	}
	// The adjustment-only run has an invoice (matched by external ref) but no charges.
	adjDetail, err := sc.svc.GetBillingRun(cov2xRunOrg, sc.admin, second.Run.ID)
	if err != nil || len(adjDetail.Charges) != 0 || len(adjDetail.Invoices) != 1 || adjDetail.Invoices[0].HouseholdID != "hh-adj" || adjDetail.Invoices[0].TotalCents != 2500 {
		t.Fatalf("adjustment-only run: %+v %v", adjDetail, err)
	}
}

func TestCov2xRunDetailReportsCreditAppliedToInvoices(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	if _, _, err := sc.svc.AddManualCredit(cov2xRunOrg, "a", sc.admin, "hh-a", ManualCreditInput{Direction: "credit", AmountCents: 2000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	res, err := sc.svc.CommitBillingRun(cov2xRunOrg, "a", sc.admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.CreditAppliedCents != 2000 {
		t.Fatalf("credit applied on the run: %+v", res.Run)
	}
	detail, err := sc.svc.GetBillingRun(cov2xRunOrg, sc.admin, res.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	credited := 0
	for _, inv := range detail.Invoices {
		credited += inv.CreditAppliedCents
		if inv.HouseholdID == "hh-a" && (inv.CreditAppliedCents != 2000 || inv.AmountDueCents != 15500) {
			t.Fatalf("Alpha invoice: %+v", inv)
		}
	}
	if credited != 2000 {
		t.Fatalf("only succeeded account_credit payments count as applied credit: %d", credited)
	}
}

// A preview that went stale because another writer billed the period meanwhile must never
// double-bill: the in-process guard skips the households with an explanation.
func TestCov2xStaleCommitSkipsConcurrentlyBilledHouseholds(t *testing.T) {
	t.Parallel()
	sc := cov2xNewScenario(t)
	in, err := sc.svc.normalizeRunInput(cov2xRunOrg, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"})
	if err != nil {
		t.Fatal(err)
	}
	roster, _ := sc.svc.loadRoster(cov2xRunOrg)
	sc.svc.mu.Lock()
	stale := sc.svc.buildRunPreviewLocked(cov2xRunOrg, roster, in)
	sc.svc.mu.Unlock()
	if stale.HouseholdCount != 3 {
		t.Fatalf("stale preview: %+v", stale)
	}
	if _, err := sc.svc.CommitBillingRun(cov2xRunOrg, "other-writer", sc.admin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual); err != nil {
		t.Fatal(err)
	}
	invoicesBefore := len(sc.svc.invoices)

	sc.svc.mu.Lock()
	res, err := sc.svc.commitPreviewLocked(t.Context(), cov2xRunOrg, "a", RunTriggerManual, in, stale)
	sc.svc.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.InvoiceCount != 0 || res.Run.NetCents != 0 || len(res.Invoices) != 0 || len(sc.svc.invoices) != invoicesBefore {
		t.Fatalf("nothing may be billed twice: run=%+v invoices=%d->%d", res.Run, invoicesBefore, len(sc.svc.invoices))
	}
	concurrent := 0
	for _, s := range res.Skipped {
		if s.Reason == "billed concurrently by another run; not double-billed" {
			concurrent++
		}
	}
	if concurrent != 3 || res.Run.SkippedCount != len(res.Skipped) {
		t.Fatalf("every stale household must be skipped as concurrent: %d / %+v", concurrent, res.Skipped)
	}
}

func TestCov2xAssignSiblingPositionsKeepsBilledPositionsAndOrdersNewcomers(t *testing.T) {
	t.Parallel()
	svc := NewService()
	org, hh := "org-sib", "hh-1"
	dues := &FeeSchedule{ID: "sch-dues", Kind: ScheduleKindDues, Name: "Dues"}
	reg := &FeeSchedule{ID: "sch-reg", Kind: ScheduleKindRegistration, Name: "Reg"}
	other := &FeeSchedule{ID: "sch-out-of-scope", Kind: ScheduleKindDues, Name: "Other"}
	released := time.Now()
	for _, c := range []*BillingCharge{
		{ID: "c1", OrgID: org, HouseholdID: hh, SwimmerID: "billed-a", ScheduleID: dues.ID, PeriodKey: "2026-11-01", SiblingPosition: 3},
		{ID: "c2", OrgID: org, HouseholdID: hh, SwimmerID: "billed-a", ScheduleID: dues.ID, PeriodKey: "2026-11-01", SiblingPosition: 2}, // lowest recorded position wins
		{ID: "c3", OrgID: org, HouseholdID: hh, SwimmerID: "billed-b", ScheduleID: dues.ID, PeriodKey: "2026-11-01", SiblingPosition: 1},
		{ID: "c4", OrgID: org, HouseholdID: hh, SwimmerID: "ignored-released", ScheduleID: dues.ID, PeriodKey: "2026-11-01", SiblingPosition: 9, ReleasedAt: &released},
		{ID: "c5", OrgID: "org-else", HouseholdID: hh, SwimmerID: "ignored-org", ScheduleID: dues.ID, PeriodKey: "2026-11-01", SiblingPosition: 9},
		{ID: "c6", OrgID: org, HouseholdID: "hh-2", SwimmerID: "ignored-household", ScheduleID: dues.ID, PeriodKey: "2026-11-01", SiblingPosition: 9},
		{ID: "c7", OrgID: org, HouseholdID: hh, SwimmerID: "ignored-scope", ScheduleID: other.ID, PeriodKey: "2026-11-01", SiblingPosition: 9},
		{ID: "c8", OrgID: org, HouseholdID: hh, SwimmerID: "ignored-period", ScheduleID: dues.ID, PeriodKey: "2026-10-01", SiblingPosition: 9},
	} {
		svc.finance.indexCharge(c)
	}
	considered := map[string]map[chargeScope]bool{ScheduleKindDues: {{dues.ID, "2026-11-01"}: true}}
	rules := []*DiscountRule{{ID: "r1", OrgID: org, Name: "Siblings", Active: true, AppliesTo: DiscountAppliesAll, MinPosition: 2, DiscountType: DiscountTypePercent, PercentBps: 1000}}
	mk := func(sch *FeeSchedule, swimmer, dob string, gross int) *pendingCharge {
		return &pendingCharge{schedule: sch, swimmerID: swimmer, swimmerDOB: dob, gross: gross}
	}
	charges := []*pendingCharge{
		mk(dues, "billed-a", "", 10000), // keeps its recorded position (2)
		mk(dues, "new-cheap", "2010-01-01", 5000),
		mk(dues, "new-old", "2008-01-01", 10000),
		mk(dues, "new-young", "2012-01-01", 10000),
		mk(dues, "new-nodob", "", 10000),
		mk(dues, "new-nodob-b", "", 10000),
		mk(dues, "new-twice", "2009-01-01", 6000), // two lines add up to 12000 gross
		mk(dues, "new-twice", "2009-01-01", 6000),
		mk(reg, "new-reg", "", 5000), // registration positions are numbered separately
	}
	svc.assignSiblingPositions(org, hh, charges, considered, rules)

	got := map[string]int{}
	for _, c := range charges {
		got[c.swimmerID] = c.position
	}
	// Existing max position is 3 (billed-a's older charge); newcomers follow in order:
	// gross desc (new-twice 12000), then dob asc with missing last (old, young, nodob-b < nodob by id), then cheap.
	want := map[string]int{
		"billed-a": 2, "new-twice": 4, "new-old": 5, "new-young": 6, "new-nodob": 7, "new-nodob-b": 8, "new-cheap": 9, "new-reg": 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions %v, want %v", got, want)
	}
	for _, c := range charges {
		switch {
		case c.position < 2:
			if c.rule != nil || c.discount != 0 {
				t.Errorf("%s: first sibling gets no discount: %+v", c.swimmerID, c)
			}
		case c.rule == nil || c.discount != c.gross/10:
			t.Errorf("%s at position %d: discount %d, want %d", c.swimmerID, c.position, c.discount, c.gross/10)
		}
	}
}

func TestCov2xBuildHouseholdPreviewLinesAndOrdering(t *testing.T) {
	t.Parallel()
	period := billingPeriod{Key: "2026-11-01", Start: cov2xDate(t, "2026-11-01"), End: cov2xDate(t, "2026-11-30")}
	dues := &FeeSchedule{ID: "s-dues", Kind: ScheduleKindDues, Name: "Dues"}
	duesB := &FeeSchedule{ID: "s-dues-b", Kind: ScheduleKindDues, Name: "Aqua dues"}
	reg := &FeeSchedule{ID: "s-reg", Kind: ScheduleKindRegistration, Name: "Registration", Season: "2026-27"}
	rule := &DiscountRule{ID: "r1", Name: "Sibling", DiscountType: DiscountTypePercent, PercentBps: 1500}
	charges := []*pendingCharge{
		{schedule: dues, period: period, swimmerID: "s2", swimmerName: "Zed Zee", gross: 10000, position: 2, rule: rule, discount: 1500, note: "prorated 15/30 days"},
		{schedule: reg, period: period, swimmerID: "s1", swimmerName: "Amy Ay", gross: 5000, position: 1},
		{schedule: dues, period: period, swimmerID: "s1", swimmerName: "Amy Ay", gross: 10000, position: 1},
		{schedule: duesB, period: period, swimmerID: "s1", swimmerName: "Amy Ay", gross: 2000, position: 1},
	}
	adjustments := []*Adjustment{
		{ID: "a2", SwimmerID: "s2", AmountCents: -300, Reason: "second", CreatedAt: time.Unix(200, 0)},
		{ID: "a1", AmountCents: 700, Reason: "first", CreatedAt: time.Unix(100, 0)},
		{ID: "a3", SwimmerID: "ghost", AmountCents: 50, Reason: "third", CreatedAt: time.Unix(300, 0)},
	}
	swimmers := map[string]RosterSwimmer{"s2": {ID: "s2", FirstName: "Zed", LastName: "Zee"}}
	hh := buildHouseholdPreview("hh-1", "Household One", charges, adjustments, swimmers)

	if hh.GrossCents != 27000 || hh.DiscountCents != 1500 || hh.AdjustmentCents != 450 || hh.TotalCents != 27000-1500+450 || hh.SwimmerCount != 2 {
		t.Fatalf("totals: %+v", hh)
	}
	var kinds []string
	for _, l := range hh.Lines {
		kinds = append(kinds, l.Kind+":"+l.SwimmerName+":"+l.ScheduleName)
	}
	// Charges by swimmer name, then kind (dues < registration), then schedule name; a discount follows its charge;
	// adjustments follow in creation order.
	wantKinds := []string{
		"dues:Amy Ay:Aqua dues", "dues:Amy Ay:Dues", "registration:Amy Ay:Registration",
		"dues:Zed Zee:Dues", "discount:Zed Zee:Dues",
		"adjustment::", "adjustment:Zed Zee:", "adjustment::",
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("line order:\n got %v\nwant %v", kinds, wantKinds)
	}
	byKind := map[string]RunPreviewLine{}
	for _, l := range hh.Lines {
		if _, ok := byKind[l.Kind+l.SwimmerID+l.ScheduleID]; !ok {
			byKind[l.Kind+l.SwimmerID+l.ScheduleID] = l
		}
	}
	if d := byKind["dues"+"s2"+"s-dues"].Description; d != "Dues — Zed Zee (2026-11-01 – 2026-11-30) · prorated 15/30 days" {
		t.Fatalf("dues description: %q", d)
	}
	if d := byKind["registration"+"s1"+"s-reg"]; d.Description != "Registration — Amy Ay (2026-27)" || d.Category != itemCategoryRegistrationFee {
		t.Fatalf("registration line: %+v", d)
	}
	disc := byKind["discount"+"s2"+"s-dues"]
	if disc.AmountCents != -1500 || disc.DiscountRuleID != "r1" || !strings.Contains(disc.Description, "Sibling — Zed Zee, 2nd swimmer") || disc.Category != itemCategoryDiscount {
		t.Fatalf("discount line: %+v", disc)
	}
	if a := byKind["adjustment"+"s2"]; a.Description != "Adjustment: second — Zed Zee" || a.AdjustmentID != "a2" || a.AmountCents != -300 {
		t.Fatalf("adjustment with swimmer: %+v", a)
	}
	if a := byKind["adjustment"+"ghost"]; a.Description != "Adjustment: third" || a.SwimmerName != "" {
		t.Fatalf("adjustment for a swimmer outside the roster has no name: %+v", a)
	}
}

func TestCov2xFinalizePreviewOrdersSkipsAndFingerprints(t *testing.T) {
	t.Parallel()
	mk := func() *RunPreview {
		return &RunPreview{
			AsOf: "2026-11-20", InvoiceStatus: "draft", DueDate: "2026-12-05", ScheduleIDs: []string{"s1", "s2"},
			Households: []*RunPreviewHousehold{
				{HouseholdID: "hh-3", HouseholdName: "Bravo", TotalCents: 300, GrossCents: 300, SwimmerCount: 1, Lines: []RunPreviewLine{{Kind: "dues", ScheduleID: "s1", SwimmerID: "x", PeriodKey: "p", AmountCents: 300}}},
				{HouseholdID: "hh-2", HouseholdName: "Alpha", TotalCents: 200, GrossCents: 400, DiscountCents: 200, AdjustmentCents: 0, CreditToApplyCents: 50, SwimmerCount: 2, charges: []*pendingCharge{{}, {}}, Lines: []RunPreviewLine{{Kind: "dues", AmountCents: 400}, {Kind: "discount", AmountCents: -200}}},
				{HouseholdID: "hh-1", HouseholdName: "Alpha", TotalCents: 100, GrossCents: 100, SwimmerCount: 1, Lines: []RunPreviewLine{{Kind: "adjustment", AdjustmentID: "a1", AmountCents: 100}}},
			},
			Skipped: []RunSkip{
				{HouseholdName: "Bravo", SwimmerName: "Zed", Reason: "r1"},
				{HouseholdName: "Alpha", SwimmerName: "Zoe", Reason: "r2"},
				{HouseholdName: "Alpha", SwimmerName: "Amy", Reason: "r3"},
				{HouseholdName: "Alpha", SwimmerName: "Amy", Reason: "r4"}, // equal keys keep their relative order
			},
		}
	}
	p := mk()
	finalizePreview(p)
	var ids []string
	for _, h := range p.Households {
		ids = append(ids, h.HouseholdID)
	}
	if !reflect.DeepEqual(ids, []string{"hh-1", "hh-2", "hh-3"}) {
		t.Fatalf("households by name then id: %v", ids)
	}
	var reasons []string
	for _, s := range p.Skipped {
		reasons = append(reasons, s.Reason)
	}
	if !reflect.DeepEqual(reasons, []string{"r3", "r4", "r2", "r1"}) {
		t.Fatalf("skips by household name, swimmer name (stable): %v", reasons)
	}
	if p.HouseholdCount != 3 || p.SwimmerCount != 4 || p.ChargeCount != 2 || p.GrossCents != 800 || p.DiscountCents != 200 || p.NetCents != 600 || p.CreditToApplyCents != 50 {
		t.Fatalf("totals: %+v", p)
	}
	q := mk()
	finalizePreview(q)
	if p.Fingerprint != q.Fingerprint || len(p.Fingerprint) != 24 {
		t.Fatalf("fingerprint must be stable: %q %q", p.Fingerprint, q.Fingerprint)
	}
	changed := mk()
	changed.Households[0].Lines[0].AmountCents = 301
	finalizePreview(changed)
	if changed.Fingerprint == p.Fingerprint {
		t.Fatal("a changed line amount must change the fingerprint")
	}
	changedDue := mk()
	changedDue.DueDate = "2026-12-06"
	finalizePreview(changedDue)
	if changedDue.Fingerprint == p.Fingerprint {
		t.Fatal("a changed due date must change the fingerprint")
	}
}

func TestCov2xPreviewNetZeroHouseholdsAreNotInvoiced(t *testing.T) {
	t.Parallel()
	svc := NewService()
	roster := RosterSnapshot{
		Households: []RosterHousehold{{ID: "hh-1", Name: "One", Status: "active"}, {ID: "hh-neg", Name: "Negative", Status: "active"}},
		Groups:     []RosterGroup{{Code: "SILVER", Name: "Silver"}},
		Swimmers:   []RosterSwimmer{{ID: "a", HouseholdID: "hh-1", FirstName: "A", LastName: "One", Status: "active"}},
		Registrations: []RosterRegistration{
			{SwimmerID: "a", GroupCode: "SILVER", Season: "2026-27", Status: "active"},
		},
	}
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return roster, nil })
	admin := cov2xRunAdmin()
	if _, err := svc.CreateFeeSchedule(cov2xRunOrg, "a", admin, FeeScheduleInput{Name: "Reg", Kind: "registration", Season: "2026-27", Frequency: "one_time", AmountCents: 5000, StartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDiscountRule(cov2xRunOrg, "a", admin, DiscountRuleInput{Name: "Free 2nd", AppliesTo: "registration", MinPosition: 2, DiscountType: "percent", PercentBps: 10000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAdjustment(cov2xRunOrg, "a", admin, AdjustmentInput{HouseholdID: "hh-neg", AmountCents: -300, Reason: "goodwill"}); err != nil {
		t.Fatal(err)
	}
	first, err := svc.CommitBillingRun(cov2xRunOrg, "a", admin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.InvoiceCount != 1 || first.Run.NetCents != 5000 {
		t.Fatalf("first sibling pays: %+v", first.Run)
	}
	skips := cov2xSkips(&RunPreview{Skipped: first.Skipped})
	if !skips[cov2xSkipKey{"hh-neg", "", "net total is zero or negative; nothing invoiced (adjustments stay pending)"}] {
		t.Fatalf("a credit-only household is not invoiced and its adjustment stays pending: %+v", first.Skipped)
	}
	adjs, _ := svc.ListAdjustments(cov2xRunOrg, admin, AdjustmentStatusPending, "hh-neg")
	if len(adjs) != 1 {
		t.Fatalf("adjustment must stay pending: %+v", adjs)
	}

	// A second sibling joins: position 2 is 100% off, so the household nets zero and is not invoiced.
	roster.Swimmers = append(roster.Swimmers, RosterSwimmer{ID: "b", HouseholdID: "hh-1", FirstName: "B", LastName: "One", Status: "active"})
	roster.Registrations = append(roster.Registrations, RosterRegistration{SwimmerID: "b", GroupCode: "SILVER", Season: "2026-27", Status: "active"})
	preview, err := svc.PreviewBillingRun(cov2xRunOrg, admin, RunInput{AsOf: "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.HouseholdCount != 0 {
		t.Fatalf("a fully discounted newcomer produces nothing to invoice: %+v", preview.Households)
	}
	if !cov2xSkips(preview)[cov2xSkipKey{"hh-1", "", "net total is zero or negative; nothing invoiced"}] {
		t.Fatalf("zero-total explanation missing: %+v", preview.Skipped)
	}
	if _, err := svc.CommitBillingRun(cov2xRunOrg, "a", admin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual); !errors.Is(err, ErrNothingToBill) {
		t.Fatalf("commit with nothing invoiceable: %v", err)
	}
}

func TestCov2xRunScheduledBillingGates(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := cov2xRunAdmin()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return finRoster(), nil })
	if res, err := svc.RunScheduledBilling(cov2xRunOrg, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)); res != nil || err != nil {
		t.Fatalf("automation off: %v %v", res, err)
	}
	if _, _, err := svc.UpdateBillingSettings(cov2xRunOrg, "a", admin, BillingSettingsInput{LateFeeType: LateFeeTypeFlat, AutoRunEnabled: true, AutoRunDay: 10, AutoRunInvoiceStatus: "issued", AutoRunDueDays: 14}); err != nil {
		t.Fatal(err)
	}
	if res, err := svc.RunScheduledBilling(cov2xRunOrg, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)); res != nil || err != nil {
		t.Fatalf("enabled but no auto-bill schedule: %v %v", res, err)
	}
	if _, err := svc.CreateFeeSchedule(cov2xRunOrg, "a", admin, FeeScheduleInput{Name: "Silver dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 15000, StartDate: "2026-09-01", Proration: "daily", AutoBill: true}); err != nil {
		t.Fatal(err)
	}
	if res, err := svc.RunScheduledBilling(cov2xRunOrg, time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC)); res != nil || err != nil {
		t.Fatalf("before the configured run day: %v %v", res, err)
	}
	res, err := svc.RunScheduledBilling(cov2xRunOrg, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
	if err != nil || res == nil {
		t.Fatalf("scheduled run: %v %v", res, err)
	}
	if res.Run.Trigger != RunTriggerScheduler || res.Run.CreatedBy != "system:billing-automation" || res.Run.InvoiceStatus != "issued" || res.Run.DueDate != "2026-12-04" {
		t.Fatalf("scheduled run settings: %+v", res.Run)
	}
	// Re-running later the same day finds nothing new and is not an error.
	if res, err := svc.RunScheduledBilling(cov2xRunOrg, time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)); res != nil || err != nil {
		t.Fatalf("idempotent rerun: %v %v", res, err)
	}
	p := systemFinancePrincipal("org-x")
	if p.OrgID != "org-x" || p.UserID != "system:billing-automation" || !reflect.DeepEqual(p.Roles, []string{"admin"}) {
		t.Fatalf("system principal: %+v", p)
	}
}

func TestCov2xNewInvoiceNumberIsUniquePerOrg(t *testing.T) {
	t.Parallel()
	svc := NewService()
	asOf := cov2xDate(t, "2026-11-20")
	seen := map[string]bool{}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for i := 0; i < 50; i++ {
		n := svc.newInvoiceNumberLocked(cov2xRunOrg, asOf)
		if !strings.HasPrefix(n, "BR-202611-") || len(n) != len("BR-202611-")+6 || seen[n] {
			t.Fatalf("invoice number %q (dup=%v)", n, seen[n])
		}
		seen[n] = true
		svc.invoiceNums[invoiceNumKey(cov2xRunOrg, n)] = true
	}
}
