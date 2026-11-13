package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov2xDBEnv is a Postgres-backed billing service (real schema, isolated per test) with two
// households and a small roster, plus a second "instance" helper for cross-process behaviour.
type cov2xDBEnv struct {
	t      *testing.T
	pool   *pgxpool.Pool
	svc    *Service
	org    string
	admin  *appmiddleware.Principal
	hhA    string
	hhB    string
	sw     [4]string // swimmer ids: Ann, Ben, Cal (household A) and Dee (household B)
	roster RosterSnapshot
}

func cov2xDBRoster(hhA, hhB string, sw [4]string) RosterSnapshot {
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return RosterSnapshot{
		Households: []RosterHousehold{{ID: hhA, Name: "Alpha", Status: "active"}, {ID: hhB, Name: "Bravo", Status: "active"}},
		Swimmers: []RosterSwimmer{
			{ID: sw[0], HouseholdID: hhA, FirstName: "Ann", LastName: "Alpha", DateOfBirth: "2012-01-01", Status: "active"},
			{ID: sw[1], HouseholdID: hhA, FirstName: "Ben", LastName: "Alpha", DateOfBirth: "2014-01-01", Status: "active"},
			{ID: sw[2], HouseholdID: hhA, FirstName: "Cal", LastName: "Alpha", DateOfBirth: "2016-01-01", Status: "active"},
			{ID: sw[3], HouseholdID: hhB, FirstName: "Dee", LastName: "Bravo", DateOfBirth: "2013-01-01", Status: "active"},
		},
		GroupLinks: []RosterGroupLink{
			{SwimmerID: sw[0], GroupCode: "SILVER", IsPrimary: true, AssignedAt: oct},
			{SwimmerID: sw[1], GroupCode: "SILVER", IsPrimary: true, AssignedAt: oct},
			{SwimmerID: sw[2], GroupCode: "SILVER", IsPrimary: true, AssignedAt: oct},
			{SwimmerID: sw[3], GroupCode: "SILVER", IsPrimary: true, AssignedAt: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)},
		},
		Registrations: []RosterRegistration{
			{ID: "r1", HouseholdID: hhA, SwimmerID: sw[0], GroupCode: "SILVER", Season: "2026-27", Status: "active"},
			{ID: "r4", HouseholdID: hhB, SwimmerID: sw[3], GroupCode: "SILVER", Season: "2026-27", Status: "pending"},
		},
		Groups: []RosterGroup{{Code: "SILVER", Name: "Silver"}},
	}
}

func cov2xNewDBEnv(t *testing.T) *cov2xDBEnv {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	env := &cov2xDBEnv{t: t, pool: pool, org: uuid.NewString(), hhA: uuid.NewString(), hhB: uuid.NewString()}
	for i := range env.sw {
		env.sw[i] = uuid.NewString()
	}
	testutil.SeedOrgs(t, pool, env.org)
	env.admin = &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: env.org, Roles: []string{"admin"}, MFAAuthenticated: true}
	for id, name := range map[string]string{env.hhA: "Alpha", env.hhB: "Bravo"} {
		env.exec(`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, $3)`, id, env.org, name)
	}
	swimmers := []struct{ id, household, first string }{
		{env.sw[0], env.hhA, "Ann"}, {env.sw[1], env.hhA, "Ben"}, {env.sw[2], env.hhA, "Cal"}, {env.sw[3], env.hhB, "Dee"},
	}
	for _, sw := range swimmers {
		env.exec(`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender)
			VALUES ($1, $2, $3, $4, 'Swimmer', DATE '2012-01-01', 'undisclosed')`, sw.id, env.org, sw.household, sw.first)
	}
	env.roster = cov2xDBRoster(env.hhA, env.hhB, env.sw)
	env.svc = env.newInstance()
	return env
}

// newInstance simulates another API/worker process sharing the database.
func (e *cov2xDBEnv) newInstance() *Service {
	svc := NewServiceWithPool(e.pool)
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return e.roster, nil })
	return svc
}

func (e *cov2xDBEnv) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e *cov2xDBEnv) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (e *cov2xDBEnv) seedSchedulesAndRules(svc *Service) {
	e.t.Helper()
	a := e.admin
	if _, err := svc.CreateFeeSchedule(e.org, a.UserID, a, FeeScheduleInput{Name: "Silver dues", Kind: "dues", GroupCode: "silver", Frequency: "monthly", AmountCents: 15000, StartDate: "2026-09-01", Proration: "daily", AutoBill: true}); err != nil {
		e.t.Fatal(err)
	}
	if _, err := svc.CreateFeeSchedule(e.org, a.UserID, a, FeeScheduleInput{Name: "Registration", Kind: "registration", Season: "2026-27", Frequency: "one_time", AmountCents: 5000, StartDate: "2026-09-01", EndDate: "2027-06-30"}); err != nil {
		e.t.Fatal(err)
	}
	max2 := 2
	if _, err := svc.CreateDiscountRule(e.org, a.UserID, a, DiscountRuleInput{Name: "2nd sibling", AppliesTo: "dues", MinPosition: 2, MaxPosition: &max2, DiscountType: "percent", PercentBps: 1000}); err != nil {
		e.t.Fatal(err)
	}
	if _, err := svc.CreateDiscountRule(e.org, a.UserID, a, DiscountRuleInput{Name: "3rd+ sibling", AppliesTo: "dues", MinPosition: 3, DiscountType: "percent", PercentBps: 1500}); err != nil {
		e.t.Fatal(err)
	}
}

// failTrigger makes every matching write to table raise an error until dropped.
func (e *cov2xDBEnv) failTrigger(table, event, when string) {
	e.t.Helper()
	e.exec(`CREATE OR REPLACE FUNCTION cov2x_inject_failure() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'cov2x injected failure on %', TG_TABLE_NAME; END $$`)
	cond := ""
	if when != "" {
		cond = " WHEN (" + when + ")"
	}
	e.exec(fmt.Sprintf(`CREATE TRIGGER cov2x_fail BEFORE %s ON %s FOR EACH ROW%s EXECUTE FUNCTION cov2x_inject_failure()`, event, table, cond))
}

func (e *cov2xDBEnv) dropFailTrigger(table string) {
	e.t.Helper()
	e.exec(`DROP TRIGGER cov2x_fail ON ` + table)
}

func TestCov2xDBFinanceLifecycle(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	env.seedSchedulesAndRules(svc)

	// Two credits for Bravo exercise both the insert and the ON CONFLICT branch of the balance upsert.
	for _, cents := range []int{1500, 500} {
		if _, _, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhB, ManualCreditInput{Direction: "credit", AmountCents: cents, Reason: "fundraiser"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB); got != 2000 {
		t.Fatalf("balance row: %d", got)
	}
	adjA, err := svc.CreateAdjustment(env.org, admin.UserID, admin, AdjustmentInput{HouseholdID: env.hhA, SwimmerID: env.sw[0], AmountCents: -1000, Reason: "volunteer hours"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAdjustment(env.org, admin.UserID, admin, AdjustmentInput{HouseholdID: env.hhB, AmountCents: 500, Reason: "late form"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UpdateBillingSettings(env.org, admin.UserID, admin, BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 5, LateFeeType: "flat", LateFeeFlatCents: 2500, AutoRunDay: 1, AutoRunInvoiceStatus: "draft", AutoRunDueDays: 15}); err != nil {
		t.Fatal(err)
	}

	input := RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}
	preview, err := svc.PreviewBillingRun(env.org, admin, input)
	if err != nil {
		t.Fatal(err)
	}
	// Alpha: 3 x 150.00 dues, -10% and -15% sibling tiers, 50.00 registration, -10.00 adjustment.
	// Bravo: 75.00 prorated dues + 50.00 registration + 5.00 adjustment.
	if preview.HouseholdCount != 2 || preview.ChargeCount != 6 || preview.NetCents != 45250+13000 {
		t.Fatalf("preview: %+v", preview)
	}
	input.ExpectedFingerprint = preview.Fingerprint
	res, err := svc.CommitBillingRun(env.org, admin.UserID, admin, input, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.InvoiceCount != 2 || res.Run.NetCents != 58250 || res.Run.CreditAppliedCents != 2000 || res.Run.ChargeCount != 6 {
		t.Fatalf("run: %+v", res.Run)
	}
	var bravoInv, alphaInv RunInvoiceResult
	for _, inv := range res.Invoices {
		if inv.HouseholdID == env.hhB {
			bravoInv = inv
		} else {
			alphaInv = inv
		}
	}
	if bravoInv.CreditAppliedCents != 2000 || bravoInv.AmountDueCents != 11000 || alphaInv.TotalCents != 45250 {
		t.Fatalf("invoices: bravo=%+v alpha=%+v", bravoInv, alphaInv)
	}

	// The database holds exactly what the run did.
	if n := env.count(`SELECT count(*) FROM billing_charges WHERE org_id = $1 AND released_at IS NULL`, env.org); n != 6 {
		t.Fatalf("active charges: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM billing_adjustments WHERE org_id = $1 AND status = 'applied' AND applied_run_id = $2`, env.org, res.Run.ID); n != 2 {
		t.Fatalf("applied adjustments: %d", n)
	}
	if n := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB); n != 0 {
		t.Fatalf("credit must be spent: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM household_credit_entries WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB); n != 3 {
		t.Fatalf("credit trail: %d", n)
	}
	if n := env.count(`SELECT net_cents FROM billing_runs WHERE id = $1`, res.Run.ID); n != 58250 {
		t.Fatalf("persisted run totals: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM payments WHERE org_id = $1 AND payment_method = 'account_credit' AND amount_cents = 2000`, env.org); n != 1 {
		t.Fatalf("account_credit payment: %d", n)
	}

	// A second process reads everything back from Postgres and refuses to bill the period again.
	other := env.newInstance()
	runs, err := other.ListBillingRuns(env.org, admin)
	if err != nil || len(runs) != 1 || runs[0].ID != res.Run.ID || runs[0].NetCents != 58250 || runs[0].Trigger != RunTriggerManual {
		t.Fatalf("runs from the database: %+v %v", runs, err)
	}
	detail, err := other.GetBillingRun(env.org, admin, res.Run.ID)
	if err != nil || len(detail.Charges) != 6 || len(detail.Invoices) != 2 {
		t.Fatalf("run detail: %+v %v", detail, err)
	}
	if _, err := other.GetBillingRun(env.org, admin, uuid.NewString()); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("unknown run: %v", err)
	}
	credit, err := other.GetHouseholdCredit(env.org, admin, env.hhB)
	if err != nil || credit.BalanceCents != 0 || len(credit.Entries) != 3 {
		t.Fatalf("credit from the database: %+v %v", credit, err)
	}
	if schedules, _ := other.ListFeeSchedules(env.org, admin); len(schedules) != 2 {
		t.Fatalf("schedules: %d", len(schedules))
	}
	if rules, _ := other.ListDiscountRules(env.org, admin); len(rules) != 2 {
		t.Fatalf("rules: %d", len(rules))
	}
	if settings, _ := other.GetBillingSettings(env.org, admin); !settings.LateFeeEnabled || settings.LateFeeFlatCents != 2500 {
		t.Fatalf("settings: %+v", settings)
	}
	if applied, _ := other.ListAdjustments(env.org, admin, AdjustmentStatusApplied, ""); len(applied) != 2 {
		t.Fatalf("adjustments: %+v", applied)
	}
	if _, err := other.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-25"}, RunTriggerManual); !errors.Is(err, ErrNothingToBill) {
		t.Fatalf("re-running the period from another process must bill nothing: %v", err)
	}

	// Voiding Alpha's invoice (on the other process) releases its charges and re-opens its adjustment ...
	if _, _, err := other.VoidInvoice(env.org, admin.UserID, admin, alphaInv.InvoiceID); err != nil {
		t.Fatal(err)
	}
	if n := env.count(`SELECT count(*) FROM billing_charges WHERE invoice_id = $1 AND released_at IS NOT NULL`, alphaInv.InvoiceID); n != 4 {
		t.Fatalf("released charges: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM billing_adjustments WHERE id = $1 AND status = 'pending' AND applied_invoice_id IS NULL`, adjA.ID); n != 1 {
		t.Fatalf("adjustment must be pending again: %d", n)
	}
	// ... which the first process notices (change counters) and can bill again, exactly once.
	again := RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}
	rebill, err := svc.CommitBillingRun(env.org, admin.UserID, admin, again, RunTriggerManual)
	if err != nil || rebill.Run.InvoiceCount != 1 || rebill.Run.NetCents != 45250 {
		t.Fatalf("rebill: %+v %v", rebill, err)
	}
	if n := env.count(`SELECT count(*) FROM billing_charges WHERE org_id = $1 AND released_at IS NULL`, env.org); n != 6 {
		t.Fatalf("active charges after rebill: %d", n)
	}

	// Late fees: one per overdue invoice, recorded in Postgres, never twice.
	sweep, err := svc.ApplyLateFees(env.org, admin.UserID, admin, mustDate(t, "2026-12-11"))
	if err != nil || sweep.AppliedCount != 2 || sweep.TotalCents != 5000 {
		t.Fatalf("late fee sweep: %+v %v", sweep, err)
	}
	if n := env.count(`SELECT count(*) FROM billing_late_fee_applications WHERE org_id = $1`, env.org); n != 2 {
		t.Fatalf("persisted late fees: %d", n)
	}
	if sweep, err := other.ApplyLateFees(env.org, admin.UserID, admin, mustDate(t, "2027-02-01")); err != nil || sweep.AppliedCount != 0 {
		t.Fatalf("late fees are once per invoice, even from another process: %+v %v", sweep, err)
	}

	// Money movements against the persisted invoices.
	var bravoDB string
	if err := env.pool.QueryRow(t.Context(), `SELECT id::text FROM invoices WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB).Scan(&bravoDB); err != nil {
		t.Fatal(err)
	}
	pay, _, err := svc.RecordPayment(env.org, admin.UserID, admin, bravoDB, RecordPaymentInput{AmountCents: 20000, Method: "check", ExcessToCredit: true, PaidAt: "2026-12-12"})
	if err != nil || pay.AmountCents != 13500 {
		t.Fatalf("payment: %+v %v", pay, err)
	}
	if credit, _ := svc.GetHouseholdCredit(env.org, admin, env.hhB); credit.BalanceCents != 6500 {
		t.Fatalf("overpayment credit: %+v", credit)
	}
	refund, _, err := svc.RefundToCredit(env.org, admin.UserID, admin, bravoDB, CreateRefundInput{PaymentID: pay.ID, AmountCents: 1000, Reason: "swimmer withdrew"})
	if err != nil || refund.AmountCents != 1000 {
		t.Fatalf("refund to credit: %+v %v", refund, err)
	}
	if credit, _ := svc.GetHouseholdCredit(env.org, admin, env.hhB); credit.BalanceCents != 7500 {
		t.Fatalf("credit after refund-to-credit: %+v", credit)
	}
	// Alpha's open invoice absorbs a manual credit straight away.
	_, appliedTo, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 1000, Reason: "gift", ApplyToOpenInvoices: true})
	if err != nil || len(appliedTo) != 1 {
		t.Fatalf("apply to open invoices: %v %v", appliedTo, err)
	}
	if n := env.count(`SELECT count(*) FROM payments WHERE org_id = $1 AND payment_method = 'account_credit' AND amount_cents = 1000`, env.org); n != 1 {
		t.Fatalf("credit payment row: %d", n)
	}

	// Reports read the same rows back.
	asOf := mustDate(t, "2099-01-01")
	aging, err := svc.AgingReport(env.org, admin, asOf)
	if err != nil {
		t.Fatal(err)
	}
	outstanding, err := svc.OutstandingReport(env.org, admin)
	if err != nil {
		t.Fatal(err)
	}
	if aging.Totals.TotalCents != outstanding.TotalDueCents || outstanding.TotalDueCents == 0 {
		t.Fatalf("aging %d vs outstanding %d", aging.Totals.TotalCents, outstanding.TotalDueCents)
	}
	var dbDue int
	if err := env.pool.QueryRow(t.Context(), `SELECT COALESCE(SUM(amount_due_cents), 0) FROM invoices WHERE org_id = $1 AND status IN ('issued','partially_paid')`, env.org).Scan(&dbDue); err != nil {
		t.Fatal(err)
	}
	if dbDue != outstanding.TotalDueCents {
		t.Fatalf("outstanding report %d disagrees with the invoices table %d", outstanding.TotalDueCents, dbDue)
	}
	revenue, err := svc.RevenueReport(env.org, admin, mustDate(t, "2020-01-01"), asOf)
	if err != nil || revenue.TotalCents == 0 {
		t.Fatalf("revenue: %+v %v", revenue, err)
	}
	deposits, err := svc.DepositsReport(env.org, admin, mustDate(t, "2020-01-01"), asOf, "month")
	if err != nil || deposits.GrossCents != 20000 || cov2xOverpayments(deposits) != 6500 {
		t.Fatalf("deposits: %+v %v", deposits, err)
	}
	for _, kind := range []string{"invoices", "payments"} {
		rows, err := svc.QuickBooksCSV(env.org, admin, kind, mustDate(t, "2020-01-01"), asOf)
		if err != nil || len(rows) < 2 {
			t.Fatalf("quickbooks %s: %v %v", kind, rows, err)
		}
	}
}

// cov2xOverpayments totals the overpayment column across a deposits report.
func cov2xOverpayments(r *DepositsReport) int {
	total := 0
	for _, row := range r.Rows {
		total += row.OverpaymentCents
	}
	return total
}

// Another process billing the period after this one built its preview must win exactly once:
// the unique charge index and the adjustment claim stop the stale writer.
func TestCov2xDBStaleWriterCannotDoubleBill(t *testing.T) {
	env := cov2xNewDBEnv(t)
	hhC := uuid.NewString()
	env.exec(`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Charlie')`, hhC, env.org)
	env.roster.Households = append(env.roster.Households, RosterHousehold{ID: hhC, Name: "Charlie", Status: "active"})
	stale, winner, admin := env.svc, env.newInstance(), env.admin
	env.seedSchedulesAndRules(stale)
	if _, err := stale.CreateAdjustment(env.org, admin.UserID, admin, AdjustmentInput{HouseholdID: hhC, AmountCents: 2500, Reason: "merch"}); err != nil {
		t.Fatal(err)
	}

	in, err := stale.normalizeRunInput(env.org, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.refreshOrg(t.Context(), env.org); err != nil {
		t.Fatal(err)
	}
	stale.mu.Lock()
	preview := stale.buildRunPreviewLocked(env.org, env.roster, in)
	stale.mu.Unlock()
	if preview.HouseholdCount != 3 {
		t.Fatalf("stale preview: %+v", preview)
	}

	won, err := winner.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual)
	if err != nil || won.Run.InvoiceCount != 3 {
		t.Fatalf("winner: %+v %v", won, err)
	}

	stale.mu.Lock()
	res, err := stale.commitPreviewLocked(t.Context(), env.org, admin.UserID, RunTriggerManual, in, preview)
	stale.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.InvoiceCount != 0 {
		t.Fatalf("the stale writer billed something: %+v", res.Run)
	}
	concurrent := 0
	for _, s := range res.Skipped {
		if s.Reason == "billed concurrently by another run; not double-billed" {
			concurrent++
		}
	}
	if concurrent != 3 {
		t.Fatalf("each household must be skipped as billed concurrently, got %d: %+v", concurrent, res.Skipped)
	}
	if n := env.count(`SELECT count(*) FROM invoices WHERE org_id = $1`, env.org); n != 3 {
		t.Fatalf("exactly one invoice per household must exist, got %d", n)
	}
	if n := env.count(`SELECT count(*) FROM billing_charges WHERE org_id = $1`, env.org); n != 6 {
		t.Fatalf("charges: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM billing_runs WHERE org_id = $1`, env.org); n != 2 {
		t.Fatalf("the stale writer's (empty) run is still recorded: %d", n)
	}
}

func TestCov2xDBHouseholdCommitFailureRollsBackAndIsReported(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	env.seedSchedulesAndRules(svc)
	env.failTrigger("billing_charges", "INSERT", "")

	res, err := svc.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual)
	if err != nil {
		t.Fatalf("a household failure must not abort the run: %v", err)
	}
	if res.Run.InvoiceCount != 0 || res.Run.NetCents != 0 {
		t.Fatalf("nothing was billed: %+v", res.Run)
	}
	failed := 0
	for _, s := range res.Skipped {
		if s.Reason == "commit failed; see server logs" {
			failed++
		}
	}
	if failed != 2 || res.Run.SkippedCount != len(res.Skipped) {
		t.Fatalf("both households must be reported as failed: %+v", res.Skipped)
	}
	// The household transactions rolled back completely: no half-written invoices.
	if n := env.count(`SELECT count(*) FROM invoices WHERE org_id = $1`, env.org); n != 0 {
		t.Fatalf("invoices must roll back with their charges: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM invoice_items WHERE org_id = $1`, env.org); n != 0 {
		t.Fatalf("items must roll back: %d", n)
	}

	// Once the fault clears, the same period bills normally (nothing was recorded as billed).
	env.dropFailTrigger("billing_charges")
	res, err = svc.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual)
	if err != nil || res.Run.InvoiceCount != 2 {
		t.Fatalf("retry: %+v %v", res, err)
	}
}

func TestCov2xDBRunPersistenceFailures(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	env.seedSchedulesAndRules(svc)

	// The run header cannot be written: the commit fails before any household is touched.
	env.failTrigger("billing_runs", "INSERT", "")
	if _, err := svc.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual); err == nil || !strings.Contains(err.Error(), "persist billing run") {
		t.Fatalf("expected a persist error, got %v", err)
	}
	if n := env.count(`SELECT count(*) FROM invoices WHERE org_id = $1`, env.org); n != 0 {
		t.Fatalf("no invoices without a run: %d", n)
	}
	env.dropFailTrigger("billing_runs")

	// Only the final totals update fails: the invoices exist and the caller still gets the run.
	env.failTrigger("billing_runs", "UPDATE", "")
	res, err := svc.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual)
	if err != nil {
		t.Fatalf("totals persistence failure is logged, not fatal: %v", err)
	}
	if res.Run.InvoiceCount != 2 || res.Run.NetCents == 0 {
		t.Fatalf("in-memory run keeps its totals: %+v", res.Run)
	}
	if n := env.count(`SELECT net_cents FROM billing_runs WHERE id = $1`, res.Run.ID); n != 0 {
		t.Fatalf("the failed update left the header's totals at zero in the database, got %d", n)
	}
	if n := env.count(`SELECT count(*) FROM invoices WHERE org_id = $1`, env.org); n != 2 {
		t.Fatalf("invoices: %d", n)
	}
}

func TestCov2xDBCreditApplicationFailureKeepsInvoiceAndBalance(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	env.seedSchedulesAndRules(svc)
	if _, _, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhB, ManualCreditInput{Direction: "credit", AmountCents: 3000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	// Applying credit writes an 'applied' trail entry; make exactly those fail.
	env.failTrigger("household_credit_entries", "INSERT", "NEW.kind = 'applied'")

	res, err := svc.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.InvoiceCount != 2 || res.Run.CreditAppliedCents != 0 {
		t.Fatalf("invoices are issued even if credit could not be applied: %+v", res.Run)
	}
	for _, inv := range res.Invoices {
		if inv.HouseholdID == env.hhB && (inv.Status != "issued" || inv.CreditAppliedCents != 0 || inv.AmountDueCents != inv.TotalCents) {
			t.Fatalf("Bravo invoice must stay fully due: %+v", inv)
		}
	}
	if n := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB); n != 3000 {
		t.Fatalf("the failed application must roll the balance back: %d", n)
	}
	if n := env.count(`SELECT count(*) FROM payments WHERE org_id = $1 AND payment_method = 'account_credit'`, env.org); n != 0 {
		t.Fatalf("no half-written credit payment: %d", n)
	}

	// The manual "apply to open invoices" path logs the same failure and stops, keeping the credit.
	_, appliedTo, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhB, ManualCreditInput{Direction: "credit", AmountCents: 100, Reason: "more", ApplyToOpenInvoices: true})
	if err != nil || len(appliedTo) != 0 {
		t.Fatalf("manual apply: %v %v", appliedTo, err)
	}
	if n := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB); n != 3100 {
		t.Fatalf("credit stays on account: %d", n)
	}
}

func TestCov2xDBStaleCreditBalanceIsReReadOnce(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	inv := cov2xIssuedInvoiceDB(t, svc, env, env.hhA, "SC-1", 10000)
	if _, _, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 5000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	// Another process spent most of the credit: the cache still says 5000, the database says 1200.
	env.exec(`UPDATE household_credit_balances SET balance_cents = 1200 WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA)

	svc.mu.Lock()
	live := svc.invoices[inv.ID]
	applied, err := svc.applyHouseholdCreditLocked(t.Context(), live, admin.UserID)
	cached := svc.finance.balances[balanceKey(env.org, env.hhA)]
	svc.mu.Unlock()
	if err != nil || applied != 1200 || cached != 0 {
		t.Fatalf("applied %d (want the real 1200), cached balance %d, err %v", applied, cached, err)
	}
	if n := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA); n != 0 {
		t.Fatalf("db balance: %d", n)
	}

	// If the credit is gone entirely the retry finds nothing to apply and says so quietly.
	if _, _, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 700, Reason: "again"}); err != nil {
		t.Fatal(err)
	}
	env.exec(`UPDATE household_credit_balances SET balance_cents = 0 WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA)
	svc.mu.Lock()
	applied, err = svc.applyHouseholdCreditLocked(t.Context(), svc.invoices[inv.ID], admin.UserID)
	cached = svc.finance.balances[balanceKey(env.org, env.hhA)]
	svc.mu.Unlock()
	if err != nil || applied != 0 || cached != 0 {
		t.Fatalf("spent elsewhere: applied %d cached %d err %v", applied, cached, err)
	}
}

func cov2xIssuedInvoiceDB(t *testing.T, svc *Service, env *cov2xDBEnv, household, number string, cents int) *Invoice {
	t.Helper()
	inv, err := svc.CreateInvoice(env.org, env.admin.UserID, env.admin, CreateInvoiceInput{HouseholdID: household, InvoiceNumber: number, DueDate: "2026-12-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddInvoiceItem(env.org, env.admin.UserID, env.admin, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee"}); err != nil {
		t.Fatal(err)
	}
	issued, _, err := svc.IssueInvoice(env.org, env.admin.UserID, env.admin, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

func TestCov2xDBCreditMovementsFailCleanlyOnCanceledContext(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	inv := cov2xIssuedInvoiceDB(t, svc, env, env.hhA, "CC-1", 10000)
	if _, _, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 4000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, err := svc.addCreditEntryLocked(canceled, creditEntryParams{OrgID: env.org, HouseholdID: env.hhA, AmountCents: 100, Kind: CreditKindManualCredit}); !errors.Is(err, context.Canceled) {
		t.Fatalf("add entry: %v", err)
	}
	if _, err := svc.applyCreditAmountLocked(canceled, svc.invoices[inv.ID], 100, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("apply amount: %v", err)
	}
	if err := svc.refreshLocked(canceled, env.org); err == nil || !strings.Contains(err.Error(), "refresh from DB") {
		t.Fatalf("refresh: %v", err)
	}
	if got := svc.finance.balances[balanceKey(env.org, env.hhA)]; got != 4000 {
		t.Fatalf("failed operations must not change the cached balance: %d", got)
	}
	if n := env.count(`SELECT count(*) FROM household_credit_entries WHERE org_id = $1`, env.org); n != 1 {
		t.Fatalf("no entry may be written: %d", n)
	}
	if err := svc.refreshLocked(t.Context(), env.org); err != nil {
		t.Fatalf("a live refresh succeeds: %v", err)
	}
}

// breakFinanceLoad makes every full club load fail by hiding one finance table.
func (e *cov2xDBEnv) breakFinanceLoad() {
	e.t.Helper()
	// (A table rename would fall through to the shared public schema; a column rename cannot.)
	e.exec(`ALTER TABLE billing_late_fee_applications RENAME COLUMN applied_at TO applied_at_hidden`)
}

func TestCov2xDBRefreshFailuresSurfaceToCallers(t *testing.T) {
	env := cov2xNewDBEnv(t)
	admin := env.admin
	inv := cov2xIssuedInvoiceDB(t, env.svc, env, env.hhA, "RF-1", 10000)
	env.breakFinanceLoad()
	broken := env.newInstance() // never loaded this club, so every call must read the database
	day := mustDate(t, "2026-11-20")
	calls := map[string]func() error{
		"credit": func() error { _, err := broken.GetHouseholdCredit(env.org, admin, env.hhA); return err },
		"manual": func() error {
			_, _, err := broken.AddManualCredit(env.org, admin.UserID, admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 1, Reason: "x"})
			return err
		},
		"refund": func() error {
			_, _, err := broken.RefundToCredit(env.org, admin.UserID, admin, inv.ID, CreateRefundInput{})
			return err
		},
		"aging":       func() error { _, err := broken.AgingReport(env.org, admin, day); return err },
		"revenue":     func() error { _, err := broken.RevenueReport(env.org, admin, day, day); return err },
		"outstanding": func() error { _, err := broken.OutstandingReport(env.org, admin); return err },
		"deposits":    func() error { _, err := broken.DepositsReport(env.org, admin, day, day, "month"); return err },
		"quickbooks":  func() error { _, err := broken.QuickBooksCSV(env.org, admin, "invoices", day, day); return err },
		"list runs":   func() error { _, err := broken.ListBillingRuns(env.org, admin); return err },
		"get run":     func() error { _, err := broken.GetBillingRun(env.org, admin, uuid.NewString()); return err },
		"preview": func() error {
			_, err := broken.PreviewBillingRun(env.org, admin, RunInput{AsOf: "2026-11-20"})
			return err
		},
		"commit": func() error {
			_, err := broken.CommitBillingRun(env.org, admin.UserID, admin, RunInput{AsOf: "2026-11-20"}, RunTriggerManual)
			return err
		},
		"scheduled": func() error { _, err := broken.RunScheduledBilling(env.org, day); return err },
		"schedule ids": func() error {
			_, err := broken.normalizeRunInput(env.org, RunInput{AsOf: "2026-11-20", ScheduleIDs: []string{uuid.NewString()}})
			return err
		},
	}
	for name, call := range calls {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "refresh from DB") {
			t.Errorf("%s: a failed database read must be reported, got %v", name, err)
		}
	}
}

func TestCov2xDBReloadFallsBackToLockedReadUnderWriteContention(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc := env.svc
	if _, err := svc.CreateFeeSchedule(env.org, env.admin.UserID, env.admin, FeeScheduleInput{Name: "Silver dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 100, StartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	other := env.newInstance()
	reads := 0
	other.SetSyncHookForTest(func() {
		reads++
		other.mu.Lock() // a writer finishing mid-read invalidates the lock-free snapshot
		other.mu.Unlock()
	})
	schedules, err := other.ListFeeSchedules(env.org, env.admin)
	if err != nil || len(schedules) != 1 {
		t.Fatalf("schedules: %+v %v", schedules, err)
	}
	if reads != 3 {
		t.Fatalf("three lock-free attempts must be made before reading under the lock, got %d", reads)
	}
	if n := other.finance.schedules; len(n) != 1 {
		t.Fatalf("the locked fallback must populate the cache: %d", len(n))
	}
}
