package billing

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const cov2yOrg = "org-cov2y-gaps"

func cov2yRoster(swimmers []RosterSwimmer) RosterSnapshot {
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	links := make([]RosterGroupLink, 0, len(swimmers))
	for _, sw := range swimmers {
		links = append(links, RosterGroupLink{SwimmerID: sw.ID, GroupCode: "SILVER", AssignedAt: oct})
	}
	return RosterSnapshot{
		Households: []RosterHousehold{{ID: "hh-y", Name: "Yankee", Status: "active"}},
		Groups:     []RosterGroup{{Code: "SILVER", Name: "Silver"}},
		Swimmers:   swimmers,
		GroupLinks: links,
	}
}

func cov2yService(t *testing.T, swimmers []RosterSwimmer) *Service {
	t.Helper()
	svc := NewService()
	roster := cov2yRoster(swimmers)
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return roster, nil })
	return svc
}

func cov2ySchedule(t *testing.T, svc *Service, in FeeScheduleInput) *FeeSchedule {
	t.Helper()
	admin := cov2xAdmin(cov2yOrg)
	sc, err := svc.CreateFeeSchedule(cov2yOrg, admin.UserID, admin, in)
	if err != nil {
		t.Fatalf("schedule %q: %v", in.Name, err)
	}
	return sc
}

func TestCov2yDuesProrateFromScheduleStartWhenItFallsInsideThePeriod(t *testing.T) {
	t.Parallel()
	svc := cov2yService(t, []RosterSwimmer{{ID: "sw-y1", HouseholdID: "hh-y", FirstName: "Ann", LastName: "Yankee", Status: "active"}})
	mid := cov2ySchedule(t, svc, FeeScheduleInput{Name: "Mid-month dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 15000, StartDate: "2026-11-10", Proration: "daily"})
	cov2ySchedule(t, svc, FeeScheduleInput{Name: "Full-month dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 9000, StartDate: "2026-09-01"})
	admin := cov2xAdmin(cov2yOrg)

	// Only the schedule named in the run is billed; the swimmer is active from the schedule's
	// start (10 Nov), not from the start of the month, so 21 of 30 days are charged.
	preview, err := svc.PreviewBillingRun(cov2yOrg, admin, RunInput{AsOf: "2026-11-20", ScheduleIDs: []string{mid.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ChargeCount != 1 || preview.NetCents != 10500 {
		t.Fatalf("only the selected schedule bills, prorated 21/30: %+v", preview)
	}
	line := preview.Households[0].Lines[0]
	if line.ScheduleID != mid.ID || line.AmountCents != 10500 || line.Note != "prorated 21/30 days" {
		t.Fatalf("line: %+v", line)
	}

	// Without a filter every active schedule applies.
	all, err := svc.PreviewBillingRun(cov2yOrg, admin, RunInput{AsOf: "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	if all.ChargeCount != 2 || all.NetCents != 10500+9000 {
		t.Fatalf("both schedules bill: %+v", all)
	}
}

func TestCov2ySiblingPositionsRankUndatedSwimmersLast(t *testing.T) {
	t.Parallel()
	swimmers := []RosterSwimmer{
		{ID: "sw-a", HouseholdID: "hh-y", FirstName: "Ann", LastName: "Y", Status: "active"},
		{ID: "sw-b", HouseholdID: "hh-y", FirstName: "Bea", LastName: "Y", Status: "active"},
		{ID: "sw-c", HouseholdID: "hh-y", FirstName: "Cal", LastName: "Y", DateOfBirth: "2012-01-01", Status: "active"},
		{ID: "sw-d", HouseholdID: "hh-y", FirstName: "Dee", LastName: "Y", DateOfBirth: "2010-05-05", Status: "active"},
	}
	svc := cov2yService(t, swimmers)
	cov2ySchedule(t, svc, FeeScheduleInput{Name: "Dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 10000, StartDate: "2026-09-01"})
	admin := cov2xAdmin(cov2yOrg)
	want := map[string]int{"sw-d": 1, "sw-c": 2, "sw-a": 3, "sw-b": 4}
	// Swimmers with equal charges are ordered by birth date (oldest first, undated last, then id).
	// The order candidates are gathered in is random, so repeat to exercise both comparison orders.
	for i := 0; i < 40; i++ {
		preview, err := svc.PreviewBillingRun(cov2yOrg, admin, RunInput{AsOf: "2026-11-20"})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, l := range preview.Households[0].Lines {
			if l.Kind == "dues" {
				got[l.SwimmerID] = l.SiblingPosition
			}
		}
		for id, pos := range want {
			if got[id] != pos {
				t.Fatalf("iteration %d: positions %v, want %v", i, got, want)
			}
		}
	}
}

func TestCov2yRevenueTotalsWithEqualAmountsSortByKey(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	inv := cov2xLinesInvoice(t, svc, p, "hh-a", "TIE-1", "",
		cov2xLine{"program_fee", "", 1000},
		cov2xLine{"meet_fee", "", 1000},
		cov2xLine{"equipment", "", 1000})
	cov2xSetIssuedAt(t, svc, inv.ID, "2026-11-02")
	report, err := svc.RevenueReport(p.OrgID, p, cov2xDate(t, "2026-11-01"), cov2xDate(t, "2026-11-30"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ByCategory) != 3 {
		t.Fatalf("categories: %+v", report.ByCategory)
	}
	for i := 1; i < len(report.ByCategory); i++ {
		a, b := report.ByCategory[i-1], report.ByCategory[i]
		if a.AmountCents != 1000 || b.AmountCents != 1000 || a.Key >= b.Key {
			t.Fatalf("equal totals must be ordered by key: %+v", report.ByCategory)
		}
	}
}

// ─── Database ────────────────────────────────────────────────────────────────

func TestCov2yDBLoadFinanceStateReportsQueryAndScanFailuresPerTable(t *testing.T) {
	env := cov2xNewDBEnv(t)
	inv := cov2xIssuedInvoiceDB(t, env.svc, env, env.hhA, "LD-1", 5000)
	schedule, run := uuid.NewString(), uuid.NewString()
	env.exec(`INSERT INTO billing_fee_schedules (id, org_id, name, kind, group_code, frequency, amount_cents, start_date)
		VALUES ($1, $2, 'Dues', 'dues', 'SILVER', 'monthly', 100, DATE '2026-09-01')`, schedule, env.org)
	env.exec(`INSERT INTO billing_discount_rules (id, org_id, name, applies_to, min_position, discount_type, percent_bps)
		VALUES ($1, $2, 'Sibling', 'dues', 2, 'percent', 1000)`, uuid.NewString(), env.org)
	env.exec(`INSERT INTO billing_adjustments (id, org_id, household_id, amount_cents, reason) VALUES ($1, $2, $3, 100, 'Merch')`,
		uuid.NewString(), env.org, env.hhA)
	env.exec(`INSERT INTO billing_runs (id, org_id, as_of, invoice_status, trigger) VALUES ($1, $2, DATE '2026-11-01', 'draft', 'manual')`, run, env.org)
	env.exec(`INSERT INTO billing_charges (id, org_id, run_id, invoice_id, household_id, schedule_id, period_key, period_start, period_end, gross_cents, net_cents)
		VALUES ($1, $2, $3, $4, $5, $6, '2026-11', DATE '2026-11-01', DATE '2026-11-30', 100, 100)`,
		uuid.NewString(), env.org, run, inv.ID, env.hhA, schedule)
	env.exec(`INSERT INTO household_credit_entries (id, org_id, household_id, amount_cents, kind, balance_after_cents)
		VALUES ($1, $2, $3, 100, 'manual_credit', 100)`, uuid.NewString(), env.org, env.hhA)
	env.exec(`INSERT INTO household_credit_balances (org_id, household_id, balance_cents) VALUES ($1, $2, 100)`, env.org, env.hhA)
	env.exec(`INSERT INTO billing_settings (org_id) VALUES ($1)`, env.org)
	env.exec(`INSERT INTO billing_late_fee_applications (id, org_id, invoice_id, item_id, amount_cents) VALUES ($1, $2, $3, $4, 100)`,
		uuid.NewString(), env.org, inv.ID, uuid.NewString())

	repo := env.svc.repo
	ctx := context.Background()
	st, err := repo.loadFinanceState(ctx, env.org)
	if err != nil {
		t.Fatalf("a healthy schema loads: %v", err)
	}
	if len(st.schedules) != 1 || len(st.rules) != 1 || len(st.adjustments) != 1 || len(st.runs) != 1 || len(st.charges) != 1 ||
		len(st.credits) != 1 || st.balances[balanceKey(env.org, env.hhA)] != 100 || st.settings[env.org] == nil || len(st.lateFees) != 1 {
		t.Fatalf("every table must be loaded: %+v", st)
	}

	tables := []struct {
		what, table, col, colType, restore string
	}{
		{"fee schedules", "billing_fee_schedules", "name", "TEXT", "'Dues'"},
		{"discount rules", "billing_discount_rules", "name", "TEXT", "'Sibling'"},
		{"adjustments", "billing_adjustments", "reason", "TEXT", "'Merch'"},
		{"runs", "billing_runs", "fingerprint", "TEXT", "''"},
		{"charges", "billing_charges", "period_key", "TEXT", "'2026-11'"},
		{"credit entries", "household_credit_entries", "reason", "TEXT", "''"},
		{"credit balances", "household_credit_balances", "balance_cents", "INT", "100"},
		{"settings", "billing_settings", "late_fee_type", "TEXT", "'flat'"},
		{"late fees", "billing_late_fee_applications", "amount_cents", "INT", "100"},
	}
	for _, tc := range tables {
		// A query that cannot run (column missing) is reported rather than yielding partial state.
		env.exec(`ALTER TABLE ` + tc.table + ` RENAME COLUMN ` + tc.col + ` TO ` + tc.col + `_hidden`)
		if got, err := repo.loadFinanceState(ctx, env.org); err == nil || got != nil || !strings.Contains(err.Error(), tc.col) {
			t.Fatalf("%s: query failure must be returned, got %v %v", tc.what, got, err)
		}
		env.exec(`ALTER TABLE ` + tc.table + ` RENAME COLUMN ` + tc.col + `_hidden TO ` + tc.col)

		// A NULL where the model needs a value fails the row scan and names the table being loaded.
		env.exec(`ALTER TABLE ` + tc.table + ` DROP COLUMN ` + tc.col)
		env.exec(`ALTER TABLE ` + tc.table + ` ADD COLUMN ` + tc.col + ` ` + tc.colType)
		if got, err := repo.loadFinanceState(ctx, env.org); err == nil || got != nil || !strings.Contains(err.Error(), "billing: scan "+tc.what) {
			t.Fatalf("%s: scan failure must be reported, got %v %v", tc.what, got, err)
		}
		env.exec(`UPDATE ` + tc.table + ` SET ` + tc.col + ` = ` + tc.restore)
		if _, err := repo.loadFinanceState(ctx, env.org); err != nil {
			t.Fatalf("%s: load must recover once the data is valid: %v", tc.what, err)
		}
	}
}

func TestCov2yDBAdjustCreditBalanceMapsConstraintAndPropagatesOtherErrors(t *testing.T) {
	env := cov2xNewDBEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// A constraint violation while crediting is reported as insufficient credit, not a raw SQL error.
	env.exec(`ALTER TABLE household_credit_balances ADD CONSTRAINT cov2y_cap CHECK (balance_cents < 1000)`)
	if _, err := adjustCreditBalance(ctx, env.pool, env.org, env.hhA, 5000, now); !errors.Is(err, ErrInsufficientCredit) {
		t.Fatalf("check violation: %v", err)
	}
	if n := env.count(`SELECT count(*) FROM household_credit_balances WHERE org_id = $1`, env.org); n != 0 {
		t.Fatalf("a rejected credit must not leave a balance row: %d", n)
	}
	env.exec(`ALTER TABLE household_credit_balances DROP CONSTRAINT cov2y_cap`)

	// Any other database failure is returned as-is and must not look like a short balance.
	env.failTrigger("household_credit_balances", "INSERT", "")
	_, err := adjustCreditBalance(ctx, env.pool, env.org, env.hhA, 500, now)
	if err == nil || errors.Is(err, ErrInsufficientCredit) || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("other failure: %v", err)
	}

	// The same failure surfaces from a manual credit and leaves no trail or cached balance.
	if _, _, err := env.svc.AddManualCredit(env.org, env.admin.UserID, env.admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 500, Reason: "gift"}); err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("manual credit: %v", err)
	}
	if n := env.count(`SELECT count(*) FROM household_credit_entries WHERE org_id = $1`, env.org); n != 0 {
		t.Fatalf("no entry may be recorded: %d", n)
	}
	env.svc.mu.Lock()
	cached := env.svc.finance.balances[balanceKey(env.org, env.hhA)]
	env.svc.mu.Unlock()
	if cached != 0 {
		t.Fatalf("cached balance must not move: %d", cached)
	}
}

func TestCov2yDBClaimAdjustmentReportsDatabaseErrors(t *testing.T) {
	env := cov2xNewDBEnv(t)
	ok, err := claimAdjustment(context.Background(), env.pool, &Adjustment{ID: uuid.NewString(), OrgID: env.org, AppliedInvoiceID: "not-a-uuid"})
	if err == nil || ok {
		t.Fatalf("an invalid reference must fail the claim, got %v %v", ok, err)
	}
	// A well-formed claim for an adjustment that is not pending reports false without an error.
	ok, err = claimAdjustment(context.Background(), env.pool, &Adjustment{ID: uuid.NewString(), OrgID: env.org, AppliedInvoiceID: uuid.NewString(), AppliedRunID: uuid.NewString()})
	if err != nil || ok {
		t.Fatalf("missing adjustment: %v %v", ok, err)
	}
}

func TestCov2yDBApplyCreditRollsBackWhenInvoiceChangedOrInvoiceWriteFails(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	inv := cov2xIssuedInvoiceDB(t, svc, env, env.hhA, "AC-1", 10000)
	if _, _, err := svc.AddManualCredit(env.org, admin.UserID, admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 4000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	assertUntouched := func(step string) {
		t.Helper()
		if n := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA); n != 4000 {
			t.Fatalf("%s: balance must roll back, got %d", step, n)
		}
		if n := env.count(`SELECT count(*) FROM payments WHERE org_id = $1 AND payment_method = 'account_credit'`, env.org); n != 0 {
			t.Fatalf("%s: no credit payment may remain: %d", step, n)
		}
		svc.mu.Lock()
		defer svc.mu.Unlock()
		if got := svc.finance.balances[balanceKey(env.org, env.hhA)]; got != 4000 {
			t.Fatalf("%s: cached balance moved to %d", step, got)
		}
		if got := svc.invoices[inv.ID]; got.AmountDueCents != 10000 || got.Status != "issued" {
			t.Fatalf("%s: cached invoice changed: %+v", step, got)
		}
	}

	// Another process changed the invoice after this one cached it: the stale write is refused.
	env.exec(`UPDATE invoices SET amount_paid_cents = 1, amount_due_cents = amount_due_cents - 1, status = 'partially_paid' WHERE id = $1`, inv.ID)
	svc.mu.Lock()
	applied, err := svc.applyCreditAmountLocked(context.Background(), svc.invoices[inv.ID], 1000, admin.UserID)
	svc.mu.Unlock()
	if !errors.Is(err, errInvoiceStale) || applied != 0 {
		t.Fatalf("stale invoice: applied %d err %v", applied, err)
	}
	assertUntouched("stale invoice")
	env.exec(`UPDATE invoices SET amount_paid_cents = 0, amount_due_cents = amount_due_cents + 1, status = 'issued' WHERE id = $1`, inv.ID)

	// The invoice update itself fails: everything written earlier in the transaction is undone.
	env.failTrigger("invoices", "INSERT OR UPDATE", "")
	svc.mu.Lock()
	applied, err = svc.applyCreditAmountLocked(context.Background(), svc.invoices[inv.ID], 1000, admin.UserID)
	svc.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "injected failure") || applied != 0 {
		t.Fatalf("invoice write failure: applied %d err %v", applied, err)
	}
	assertUntouched("invoice write")
	env.dropFailTrigger("invoices")

	// Once the fault clears the same application succeeds.
	svc.mu.Lock()
	applied, err = svc.applyCreditAmountLocked(context.Background(), svc.invoices[inv.ID], 1000, admin.UserID)
	svc.mu.Unlock()
	if err != nil || applied != 1000 {
		t.Fatalf("recovery: %d %v", applied, err)
	}
}

// cov2yHookedPool is a second pool on the test's isolated schema whose BeforeAcquire hook
// runs a callback with the 1-based number of the acquisition since it was armed.
type cov2yHookedPool struct {
	pool  *pgxpool.Pool
	armed atomic.Bool
	count atomic.Int32
	onNth func(n int32)
}

func cov2yNewHookedPool(t *testing.T, env *cov2xDBEnv, onNth func(n int32)) *cov2yHookedPool {
	t.Helper()
	h := &cov2yHookedPool{onNth: onNth}
	cfg := env.pool.Config().Copy()
	cfg.BeforeAcquire = func(context.Context, *pgx.Conn) bool {
		if h.armed.Load() {
			h.onNth(h.count.Add(1))
		}
		return true
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h.pool = pool
	return h
}

func TestCov2yDBStaleBalanceReReadFailureIsReturned(t *testing.T) {
	env := cov2xNewDBEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// The debit transaction is the first connection use; the balance re-read is the second.
	h := cov2yNewHookedPool(t, env, func(n int32) {
		if n == 2 {
			cancel()
		}
	})
	svc := NewServiceWithPool(h.pool)
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return env.roster, nil })
	inv := cov2xIssuedInvoiceDB(t, svc, env, env.hhA, "RR-1", 10000)
	if _, _, err := svc.AddManualCredit(env.org, env.admin.UserID, env.admin, env.hhA, ManualCreditInput{Direction: "credit", AmountCents: 5000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	// Another process spent most of the credit, so the cached 5000 is stale.
	env.exec(`UPDATE household_credit_balances SET balance_cents = 1200 WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA)

	h.armed.Store(true)
	svc.mu.Lock()
	applied, err := svc.applyHouseholdCreditLocked(ctx, svc.invoices[inv.ID], env.admin.UserID)
	cached := svc.finance.balances[balanceKey(env.org, env.hhA)]
	svc.mu.Unlock()
	h.armed.Store(false)
	if !errors.Is(err, context.Canceled) || applied != 0 {
		t.Fatalf("a failed re-read must be returned: applied %d err %v", applied, err)
	}
	if cached != 5000 {
		t.Fatalf("the cache must be left alone when the re-read fails: %d", cached)
	}
	if n := env.count(`SELECT balance_cents FROM household_credit_balances WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA); n != 1200 {
		t.Fatalf("db balance: %d", n)
	}
}

func TestCov2yDBOrgLockReportsLockStatementFailure(t *testing.T) {
	env := cov2xNewDBEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// The connection is handed out, then the caller's context ends before the lock is taken.
	h := cov2yNewHookedPool(t, env, func(int32) { cancel() })
	h.armed.Store(true)
	repo := newRepository(h.pool)
	ran := false
	err := repo.withOrgLock(ctx, "run", env.org, func() error { ran = true; return nil })
	if err == nil || ran {
		t.Fatalf("the guarded function must not run without the lock: ran=%v err=%v", ran, err)
	}
	if !strings.Contains(err.Error(), "billing: advisory lock") || !errors.Is(err, context.Canceled) {
		t.Fatalf("lock failure must be wrapped: %v", err)
	}
}

func TestCov2yDBRunCommitStepFailuresAreReportedPerHousehold(t *testing.T) {
	env := cov2xNewDBEnv(t)
	svc, admin := env.svc, env.admin
	env.seedSchedulesAndRules(svc)
	adj, err := svc.CreateAdjustment(env.org, admin.UserID, admin, AdjustmentInput{HouseholdID: env.hhA, AmountCents: 500, Reason: "Merchandise"})
	if err != nil {
		t.Fatal(err)
	}
	input := RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}
	const failedReason = "commit failed; see server logs"

	failedHouseholds := func(res *RunResult) int {
		n := 0
		for _, s := range res.Skipped {
			if s.Reason == failedReason {
				n++
			}
		}
		return n
	}
	steps := []struct {
		name, table, event string
		wantFailed         int
	}{
		{"invoice header", "invoices", "INSERT", 2},
		{"invoice item", "invoice_items", "INSERT", 2},
		{"adjustment claim", "billing_adjustments", "UPDATE", 1},
	}
	for _, step := range steps {
		env.failTrigger(step.table, step.event, "")
		res, err := svc.CommitBillingRun(env.org, admin.UserID, admin, input, RunTriggerManual)
		env.dropFailTrigger(step.table)
		if err != nil {
			t.Fatalf("%s: a household failure must not abort the run: %v", step.name, err)
		}
		if got := failedHouseholds(res); got != step.wantFailed {
			t.Fatalf("%s: %d households reported failed, want %d: %+v", step.name, got, step.wantFailed, res.Skipped)
		}
		if res.Run.InvoiceCount != 2-step.wantFailed {
			t.Fatalf("%s: invoice count %d", step.name, res.Run.InvoiceCount)
		}
		// Whatever failed was rolled back in full: the charges of a failed household are not recorded.
		if n := env.count(`SELECT count(*) FROM billing_charges WHERE org_id = $1`, env.org); n != 0 && step.wantFailed == 2 {
			t.Fatalf("%s: charges must roll back with the invoice: %d", step.name, n)
		}
		if n := env.count(`SELECT count(*) FROM billing_adjustments WHERE org_id = $1 AND status = 'pending' AND id = $2`, env.org, adj.ID); n != 1 {
			t.Fatalf("%s: the adjustment must still be pending after a failed commit: %d", step.name, n)
		}
		if step.wantFailed == 1 {
			// Household B billed normally; A is the one that failed and can be retried.
			if n := env.count(`SELECT count(*) FROM invoices WHERE org_id = $1 AND household_id = $2`, env.org, env.hhA); n != 0 {
				t.Fatalf("household A must have no invoice: %d", n)
			}
			if n := env.count(`SELECT count(*) FROM invoices WHERE org_id = $1 AND household_id = $2`, env.org, env.hhB); n != 1 {
				t.Fatalf("household B must be billed: %d", n)
			}
		}
	}

	// With no fault the pending adjustment is applied to household A's retried invoice.
	res, err := svc.CommitBillingRun(env.org, admin.UserID, admin, input, RunTriggerManual)
	if err != nil || res.Run.InvoiceCount != 1 {
		t.Fatalf("retry bills the household that failed: %+v %v", res, err)
	}
	if n := env.count(`SELECT count(*) FROM billing_adjustments WHERE org_id = $1 AND status = 'applied' AND id = $2`, env.org, adj.ID); n != 1 {
		t.Fatalf("adjustment must be applied on retry: %d", n)
	}
}
