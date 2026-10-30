package billing

import (
	"errors"
	"sync"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

const finOrg = "org-fin"

var finAdmin = &appmiddleware.Principal{UserID: "admin-1", OrgID: finOrg, Roles: []string{"admin"}, MFAAuthenticated: true}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := parseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// finRoster: household A has 3 swimmers in SILVER (one joined mid-November),
// household B has 1 swimmer; both registered for the 2026-27 season.
func finRoster() RosterSnapshot {
	nov := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return RosterSnapshot{
		Households: []RosterHousehold{{ID: "hh-a", Name: "Alpha", Status: "active"}, {ID: "hh-b", Name: "Bravo", Status: "active"}},
		Swimmers: []RosterSwimmer{
			{ID: "sw-1", HouseholdID: "hh-a", FirstName: "Ann", LastName: "Alpha", DateOfBirth: "2012-01-01", Status: "active"},
			{ID: "sw-2", HouseholdID: "hh-a", FirstName: "Ben", LastName: "Alpha", DateOfBirth: "2014-01-01", Status: "active"},
			{ID: "sw-3", HouseholdID: "hh-a", FirstName: "Cal", LastName: "Alpha", DateOfBirth: "2016-01-01", Status: "active"},
			{ID: "sw-4", HouseholdID: "hh-b", FirstName: "Dee", LastName: "Bravo", DateOfBirth: "2013-01-01", Status: "active"},
		},
		GroupLinks: []RosterGroupLink{
			{SwimmerID: "sw-1", GroupCode: "SILVER", IsPrimary: true, AssignedAt: nov},
			{SwimmerID: "sw-2", GroupCode: "SILVER", IsPrimary: true, AssignedAt: nov},
			{SwimmerID: "sw-3", GroupCode: "SILVER", IsPrimary: true, AssignedAt: nov},
			{SwimmerID: "sw-4", GroupCode: "SILVER", IsPrimary: true, AssignedAt: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)},
		},
		Registrations: []RosterRegistration{
			{ID: "r1", HouseholdID: "hh-a", SwimmerID: "sw-1", GroupCode: "SILVER", Season: "2026-27", Status: "active"},
			{ID: "r4", HouseholdID: "hh-b", SwimmerID: "sw-4", GroupCode: "SILVER", Season: "2026-27", Status: "pending"},
		},
		Groups: []RosterGroup{{Code: "SILVER", Name: "Silver"}},
	}
}

func newFinService(t *testing.T) (*Service, *FeeSchedule) {
	t.Helper()
	svc := NewService()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return finRoster(), nil })
	sched, err := svc.CreateFeeSchedule(finOrg, "admin-1", finAdmin, FeeScheduleInput{
		Name: "Silver dues", Kind: "dues", GroupCode: "silver", Frequency: "monthly", AmountCents: 15000,
		StartDate: "2026-09-01", Proration: "daily",
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if sched.GroupCode != "SILVER" {
		t.Fatalf("group code must be canonical uppercase, got %q", sched.GroupCode)
	}
	max2 := 2
	if _, err := svc.CreateDiscountRule(finOrg, "admin-1", finAdmin, DiscountRuleInput{Name: "2nd sibling", AppliesTo: "dues", MinPosition: 2, MaxPosition: &max2, DiscountType: "percent", PercentBps: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDiscountRule(finOrg, "admin-1", finAdmin, DiscountRuleInput{Name: "3rd+ sibling", AppliesTo: "dues", MinPosition: 3, DiscountType: "percent", PercentBps: 1500}); err != nil {
		t.Fatal(err)
	}
	return svc, sched
}

func TestFinancePeriodsAndProration(t *testing.T) {
	monthly := &FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-01"}
	p, ok := periodFor(monthly, mustDate(t, "2026-11-20"))
	if !ok || p.Key != "2026-11-01" || fmtDate(p.End) != "2026-11-30" {
		t.Fatalf("monthly period: %+v %v", p, ok)
	}
	quarterly := &FeeSchedule{Frequency: FrequencyQuarterly, StartDate: "2026-09-15"}
	p, _ = periodFor(quarterly, mustDate(t, "2026-12-01"))
	if p.Key != "2026-12-01" || fmtDate(p.End) != "2027-02-28" {
		t.Fatalf("quarterly anchored on Sep: %+v", p)
	}
	if _, ok := periodFor(monthly, mustDate(t, "2026-08-31")); ok {
		t.Fatal("before start must not bill")
	}
	if amt, _ := prorate(15000, ProrationDaily, billingPeriod{Start: mustDate(t, "2026-11-01"), End: mustDate(t, "2026-11-30")}, mustDate(t, "2026-11-16"), mustDate(t, "2026-11-30")); amt != 7500 {
		t.Fatalf("daily proration 15/30 days = 7500, got %d", amt)
	}
	if amt, _ := prorate(15000, ProrationWaive, billingPeriod{Start: mustDate(t, "2026-11-01"), End: mustDate(t, "2026-11-30")}, mustDate(t, "2026-11-02"), mustDate(t, "2026-11-30")); amt != 0 {
		t.Fatalf("waive: %d", amt)
	}
	if amt, _ := prorate(15000, ProrationHalf, billingPeriod{Start: mustDate(t, "2026-11-01"), End: mustDate(t, "2026-11-30")}, mustDate(t, "2026-11-20"), mustDate(t, "2026-11-30")); amt != 7500 {
		t.Fatalf("half: %d", amt)
	}
}

func TestFinanceRunPreviewCommitIdempotent(t *testing.T) {
	svc, _ := newFinService(t)
	input := RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}
	preview, err := svc.PreviewBillingRun(finOrg, finAdmin, input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.HouseholdCount != 2 || preview.ChargeCount != 4 {
		t.Fatalf("expected 2 households / 4 charges, got %d / %d", preview.HouseholdCount, preview.ChargeCount)
	}
	// Alpha: 3 × 150.00 gross; 2nd −10% (15.00), 3rd −15% (22.50).
	alpha := preview.Households[0]
	if alpha.HouseholdID != "hh-a" || alpha.GrossCents != 45000 || alpha.DiscountCents != 3750 || alpha.TotalCents != 41250 {
		t.Fatalf("alpha totals wrong: %+v", alpha)
	}
	// Bravo joined Nov 16: daily proration 15/30.
	if bravo := preview.Households[1]; bravo.TotalCents != 7500 {
		t.Fatalf("bravo prorated total = 7500, got %d", bravo.TotalCents)
	}

	input.ExpectedFingerprint = preview.Fingerprint
	result, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, input, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.InvoiceCount != 2 || result.Run.NetCents != 48750 {
		t.Fatalf("run totals: %+v", result.Run)
	}
	inv, err := svc.GetInvoice(finOrg, finAdmin, result.Invoices[0].InvoiceID)
	if err != nil || inv.Status != "issued" || inv.TotalCents != 41250 {
		t.Fatalf("issued invoice: %+v %v", inv, err)
	}

	// Re-running the same period never double-bills.
	if _, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-11-25"}, RunTriggerManual); !errors.Is(err, ErrNothingToBill) {
		t.Fatalf("rerun must bill nothing, got %v", err)
	}
	// A stale fingerprint is rejected.
	if _, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-12-01", ExpectedFingerprint: "nope"}, RunTriggerManual); !errors.Is(err, ErrRunPreviewStale) {
		t.Fatalf("expected stale preview, got %v", err)
	}
}

func TestFinanceRunConcurrentCommitsBillOnce(t *testing.T) {
	svc, _ := newFinService(t)
	var wg sync.WaitGroup
	results := make(chan *RunResult, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual); err == nil {
				results <- res
			}
		}()
	}
	wg.Wait()
	close(results)
	invoices := 0
	for res := range results {
		invoices += res.Run.InvoiceCount
	}
	if invoices != 2 {
		t.Fatalf("concurrent commits must create exactly 2 invoices, got %d", invoices)
	}
	all, _ := svc.ListInvoices(finOrg, finAdmin, ListInvoicesFilters{})
	if len(all) != 2 {
		t.Fatalf("expected 2 invoices total, got %d", len(all))
	}
}

func TestFinanceAdjustmentsVoidReleaseAndRegistration(t *testing.T) {
	svc, _ := newFinService(t)
	if _, err := svc.CreateFeeSchedule(finOrg, "admin-1", finAdmin, FeeScheduleInput{Name: "Registration", Kind: "registration", Season: "2026-27", Frequency: "one_time", AmountCents: 5000, StartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	adj, err := svc.CreateAdjustment(finOrg, "admin-1", finAdmin, AdjustmentInput{HouseholdID: "hh-b", AmountCents: -1000, Reason: "Volunteer hours"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	var bravoInvoice RunInvoiceResult
	for _, inv := range res.Invoices {
		if inv.HouseholdID == "hh-b" {
			bravoInvoice = inv
		}
	}
	// 75.00 prorated dues + 50.00 registration − 10.00 adjustment.
	if bravoInvoice.TotalCents != 11500 || bravoInvoice.Status != "draft" {
		t.Fatalf("bravo invoice: %+v", bravoInvoice)
	}
	adjs, _ := svc.ListAdjustments(finOrg, finAdmin, "", "hh-b")
	if adjs[0].ID != adj.ID || adjs[0].Status != AdjustmentStatusApplied {
		t.Fatalf("adjustment must be applied: %+v", adjs[0])
	}
	// Voiding the draft releases its charges + adjustment so the period can be re-billed.
	if _, _, err := svc.VoidInvoice(finOrg, "admin-1", finAdmin, bravoInvoice.InvoiceID); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewBillingRun(finOrg, finAdmin, RunInput{AsOf: "2026-11-20"})
	if err != nil || preview.HouseholdCount != 1 || preview.Households[0].TotalCents != 11500 {
		t.Fatalf("re-bill after void: %+v %v", preview, err)
	}
}

func TestFinanceCreditsLateFeesStatementAndReports(t *testing.T) {
	svc, _ := newFinService(t)
	if _, _, err := svc.AddManualCredit(finOrg, "admin-1", finAdmin, "hh-b", ManualCreditInput{Direction: "credit", AmountCents: 2000, Reason: "Fundraiser"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AddManualCredit(finOrg, "admin-1", finAdmin, "hh-b", ManualCreditInput{Direction: "debit", AmountCents: 999999, Reason: "too much"}); !errors.Is(err, ErrInsufficientCredit) {
		t.Fatalf("debit beyond balance must fail, got %v", err)
	}
	res, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued", DueDate: "2026-12-01"}, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	var bravo RunInvoiceResult
	for _, inv := range res.Invoices {
		if inv.HouseholdID == "hh-b" {
			bravo = inv
		}
	}
	if bravo.CreditAppliedCents != 2000 || bravo.AmountDueCents != 5500 {
		t.Fatalf("credit auto-apply: %+v", bravo)
	}
	credit, _ := svc.GetHouseholdCredit(finOrg, finAdmin, "hh-b")
	if credit.BalanceCents != 0 || len(credit.Entries) != 2 {
		t.Fatalf("credit trail: %+v", credit)
	}

	// Overpayment lands in account credit.
	if _, _, err := svc.RecordPayment(finOrg, "admin-1", finAdmin, bravo.InvoiceID, RecordPaymentInput{AmountCents: 6000, Method: "check", ExcessToCredit: true}); err != nil {
		t.Fatal(err)
	}
	credit, _ = svc.GetHouseholdCredit(finOrg, finAdmin, "hh-b")
	if credit.BalanceCents != 500 {
		t.Fatalf("overpayment credit = 500, got %d", credit.BalanceCents)
	}

	// Late fee: flat 25.00 after 5 grace days, once per invoice.
	if _, _, err := svc.UpdateBillingSettings(finOrg, "admin-1", finAdmin, BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 5, LateFeeType: "flat", LateFeeFlatCents: 2500, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	if sweep, _ := svc.ApplyLateFees(finOrg, "admin-1", finAdmin, mustDate(t, "2026-12-06")); sweep.AppliedCount != 0 {
		t.Fatal("within grace: no fee")
	}
	sweep, err := svc.ApplyLateFees(finOrg, "admin-1", finAdmin, mustDate(t, "2026-12-07"))
	if err != nil || sweep.AppliedCount != 1 || sweep.TotalCents != 2500 {
		t.Fatalf("late fee sweep: %+v %v", sweep, err)
	}
	if again, _ := svc.ApplyLateFees(finOrg, "admin-1", finAdmin, mustDate(t, "2027-01-30")); again.AppliedCount != 0 {
		t.Fatal("late fee must be idempotent per invoice")
	}

	aging, err := svc.AgingReport(finOrg, finAdmin, mustDate(t, "2027-01-15"))
	if err != nil || aging.Totals.TotalCents != 41250+2500 || aging.Totals.Days31To60Cents != 43750 {
		t.Fatalf("aging: %+v %v", aging.Totals, err)
	}
	st, err := svc.HouseholdStatement(finOrg, finAdmin, "hh-b", time.Time{}, mustDate(t, "2099-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if st.ClosingBalanceCents != st.OpenInvoicesDueCents-st.CreditBalanceCents {
		t.Fatalf("statement closing %d != open %d − credit %d", st.ClosingBalanceCents, st.OpenInvoicesDueCents, st.CreditBalanceCents)
	}
	outstanding, _ := svc.OutstandingReport(finOrg, finAdmin)
	if outstanding.TotalNetCents != 43750-500 {
		t.Fatalf("outstanding net: %d", outstanding.TotalNetCents)
	}
	deposits, _ := svc.DepositsReport(finOrg, finAdmin, mustDate(t, "2020-01-01"), mustDate(t, "2099-01-01"), "month")
	if deposits.GrossCents != 6000 {
		t.Fatalf("deposits include overpayment cash: %d", deposits.GrossCents)
	}
	rows, _ := svc.QuickBooksCSV(finOrg, finAdmin, "invoices", mustDate(t, "2020-01-01"), mustDate(t, "2099-01-01"))
	if len(rows) < 2 || rows[0][0] != "InvoiceNo" {
		t.Fatalf("quickbooks csv: %v", rows)
	}
}

func TestFinanceRBAC(t *testing.T) {
	svc, _ := newFinService(t)
	coach := &appmiddleware.Principal{UserID: "c", OrgID: finOrg, Roles: []string{"coach"}}
	parent := &appmiddleware.Principal{UserID: "p", OrgID: finOrg, Roles: []string{"parent"}, HouseholdID: "hh-a"}
	if _, err := svc.ListFeeSchedules(finOrg, coach); !errors.Is(err, ErrForbidden) {
		t.Fatal("coach must not read schedules")
	}
	if _, err := svc.PreviewBillingRun(finOrg, parent, RunInput{AsOf: "2026-11-20"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("parent must not run billing")
	}
	if _, err := svc.HouseholdStatement(finOrg, parent, "hh-a", time.Time{}, time.Now()); err != nil {
		t.Fatalf("parent reads own statement: %v", err)
	}
	if _, err := svc.HouseholdStatement(finOrg, parent, "hh-b", time.Time{}, time.Now()); !errors.Is(err, ErrForbidden) {
		t.Fatal("parent must not read another household")
	}
	if _, err := svc.HouseholdStatement(finOrg, coach, "hh-a", time.Time{}, time.Now()); !errors.Is(err, ErrForbidden) {
		t.Fatal("coach must not read statements")
	}
	up, err := svc.HouseholdUpcomingCharges(finOrg, parent, "hh-a", mustDate(t, "2026-11-20"), 60)
	if err != nil || len(up.Charges) == 0 {
		t.Fatalf("upcoming: %+v %v", up, err)
	}
}

// The commit must load the roster inside runMu so its fingerprint cannot come
// from a snapshot older than a concurrent commit.
func TestCommitBillingRunLoadsRosterInsideRunLock(t *testing.T) {
	svc, _ := newFinService(t)
	var lockedDuringLoad bool
	svc.SetRosterSource(func(string) (RosterSnapshot, error) {
		if svc.runMu.TryLock() {
			svc.runMu.Unlock()
		} else {
			lockedDuringLoad = true
		}
		return finRoster(), nil
	})
	if _, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual); err != nil {
		t.Fatal(err)
	}
	if !lockedDuringLoad {
		t.Fatal("roster was loaded outside runMu")
	}
}

func TestReportsSurfaceRosterErrors(t *testing.T) {
	svc, _ := newFinService(t)
	boom := errors.New("roster down")
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, boom })
	if _, err := svc.AgingReport(finOrg, finAdmin, mustDate(t, "2027-01-15")); !errors.Is(err, boom) {
		t.Fatalf("aging: want roster error, got %v", err)
	}
	if _, err := svc.RevenueReport(finOrg, finAdmin, mustDate(t, "2026-01-01"), mustDate(t, "2027-01-01")); !errors.Is(err, boom) {
		t.Fatalf("revenue: want roster error, got %v", err)
	}
}

func TestRunScheduledBillingUsesUTCDay(t *testing.T) {
	svc := NewService()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return finRoster(), nil })
	if _, err := svc.CreateFeeSchedule(finOrg, "admin-1", finAdmin, FeeScheduleInput{
		Name: "Silver dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 15000,
		StartDate: "2026-09-01", Proration: "daily", AutoBill: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UpdateBillingSettings(finOrg, "admin-1", finAdmin, BillingSettingsInput{
		LateFeeType: LateFeeTypeFlat, AutoRunEnabled: true, AutoRunDay: 1, AutoRunInvoiceStatus: "draft", AutoRunDueDays: 10,
	}); err != nil {
		t.Fatal(err)
	}
	// 2026-11-01 01:00 at UTC+10 is still 2026-10-31 in UTC.
	local := time.Date(2026, 11, 1, 1, 0, 0, 0, time.FixedZone("AEST", 10*3600))
	res, err := svc.RunScheduledBilling(finOrg, local)
	if err != nil || res == nil {
		t.Fatalf("run: %v %v", res, err)
	}
	if res.Run.AsOf != "2026-10-31" {
		t.Fatalf("as_of must be the UTC date, got %s", res.Run.AsOf)
	}
}

func TestUpsertAutopayValidatesExpiryAndHousehold(t *testing.T) {
	svc := NewService()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return finRoster(), nil })
	base := UpsertHouseholdAutopayInput{PaymentMethodType: "card", PaymentMethodLabel: "Visa 4242", TriggerDaysBeforeDue: 3}

	if _, err := svc.UpsertHouseholdAutopay(finOrg, "admin-1", finAdmin, "hh-missing", base); !errors.Is(err, ErrAutopayHouseholdNotFound) {
		t.Fatalf("unknown household: %v", err)
	}
	bad := base
	bad.PaymentMethodExpMonth, bad.PaymentMethodExpYear = 13, 2030
	if _, err := svc.UpsertHouseholdAutopay(finOrg, "admin-1", finAdmin, "hh-a", bad); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("bad month: %v", err)
	}
	bad.PaymentMethodExpMonth, bad.PaymentMethodExpYear = 1, 2020
	if _, err := svc.UpsertHouseholdAutopay(finOrg, "admin-1", finAdmin, "hh-a", bad); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("expired: %v", err)
	}
	ok := base
	ok.PaymentMethodExpMonth, ok.PaymentMethodExpYear = 12, time.Now().Year()+2
	if _, err := svc.UpsertHouseholdAutopay(finOrg, "admin-1", finAdmin, "hh-a", ok); err != nil {
		t.Fatalf("valid: %v", err)
	}
}
