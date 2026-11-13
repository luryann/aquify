package billing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xEnv is a Postgres-backed billing service on a private throwaway schema
// (with the real migrations). The DB tests inject failures with triggers, which
// is the only way to exercise "the write failed" paths against the real SQL.
type cov1xEnv struct {
	pool  *pgxpool.Pool
	svc   *Service
	org   string
	hh    string
	admin *appmiddleware.Principal
}

// cov1xEnvOn builds an environment for one club on a shared isolated schema. Every
// scenario uses its own club, so scenarios do not see each other's rows.
func cov1xEnvOn(t *testing.T, pool *pgxpool.Pool) *cov1xEnv {
	t.Helper()
	e := &cov1xEnv{pool: pool, org: uuid.NewString(), hh: uuid.NewString()}
	e.admin = &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: e.org, Roles: []string{"admin"}, MFAAuthenticated: true}
	e.mustExec(t, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Cov1x household')`, e.hh, e.org)
	e.svc = NewServiceWithPool(pool)
	return e
}

// TestCov1xDBScenarios runs every Postgres scenario on ONE private schema: creating and
// dropping a schema with the full migration set is expensive and takes many catalog locks,
// so the scenarios share it (each with its own club).
func TestCov1xDBScenarios(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	if _, err := pool.Exec(context.Background(), `CREATE FUNCTION cov1x_inject_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x injected failure'; END $$;
		CREATE FUNCTION cov1x_inject_skip() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`); err != nil {
		t.Fatal(err)
	}
	scenarios := []struct {
		name string
		run  func(*testing.T, *cov1xEnv)
	}{
		{"DBReadsFailClosedWhenRefreshFails", cov1xDBReadsFailClosedWhenRefreshFails},
		{"DBMutationsOnUnknownInvoicesAreNotFound", cov1xDBMutationsOnUnknownInvoicesAreNotFound},
		{"DBFullReloadAndScopedReloadErrors", cov1xDBFullReloadAndScopedReloadErrors},
		{"DBScopedReloadReplacesOnlyThatOrg", cov1xDBScopedReloadReplacesOnlyThatOrg},
		{"DBFinanceStateRoundTripsAcrossInstances", cov1xDBFinanceStateRoundTripsAcrossInstances},
		{"DBFinanceConfigWriteFailuresAreReported", cov1xDBFinanceConfigWriteFailuresAreReported},
		{"DBReleaseInvoiceFinanceReportsTransactionFailures", cov1xDBReleaseInvoiceFinanceReportsTransactionFailures},
		{"DBInvoiceWritesRollBackOnFailure", cov1xDBInvoiceWritesRollBackOnFailure},
		{"DBAutopayWriteFailures", cov1xDBAutopayWriteFailures},
		{"DBLedgerWriteFailureIsReported", cov1xDBLedgerWriteFailureIsReported},
		{"DBRefundWriteFailures", cov1xDBRefundWriteFailures},
		{"DBCreateRefundRevalidatesAgainstLockedState", cov1xDBCreateRefundRevalidatesAgainstLockedState},
		{"DBStripeRefundIDAdoption", cov1xDBStripeRefundIDAdoption},
		{"DBStripeSuccessWriteFailures", cov1xDBStripeSuccessWriteFailures},
		{"DBStripeChargeRefundedWriteFailures", cov1xDBStripeChargeRefundedWriteFailures},
		{"DBFailedPaymentKeepsCacheAlignedWithDatabase", cov1xDBFailedPaymentKeepsCacheAlignedWithDatabase},
		{"DBWebhookEventListing", cov1xDBWebhookEventListing},
		{"DBOrgCacheEviction", cov1xDBOrgCacheEviction},
		{"DBBillingRunsSurviveReload", cov1xDBBillingRunsSurviveReload},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { sc.run(t, cov1xEnvOn(t, pool)) })
	}
}

// inject installs a BEFORE trigger that makes matching statements fail
// ("INSERT OR UPDATE", an optional WHEN condition on NEW/OLD) and returns the
// function that removes it. An empty when makes every such statement fail.
func (e *cov1xEnv) inject(t *testing.T, table, events, when string) (release func()) {
	t.Helper()
	return e.injectWith(t, "cov1x_inject_failure", table, events, when, "ROW")
}

// injectStatement fails every matching statement even when it touches no rows.
func (e *cov1xEnv) injectStatement(t *testing.T, table, events string) (release func()) {
	t.Helper()
	return e.injectWith(t, "cov1x_inject_failure", table, events, "", "STATEMENT")
}

// injectSkip silently drops matching row writes (the statement reports 0 rows).
func (e *cov1xEnv) injectSkip(t *testing.T, table, events string) (release func()) {
	t.Helper()
	return e.injectWith(t, "cov1x_inject_skip", table, events, "", "ROW")
}

func (e *cov1xEnv) injectWith(t *testing.T, fn, table, events, when, level string) func() {
	t.Helper()
	name := "cov1x_trg_" + table
	sql := "CREATE TRIGGER " + name + " BEFORE " + events + " ON " + table + " FOR EACH " + level
	if when != "" {
		sql += " WHEN (" + when + ")"
	}
	sql += " EXECUTE FUNCTION " + fn + "()"
	if _, err := e.pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("inject on %s: %v", table, err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		if _, err := e.pool.Exec(context.Background(), "DROP TRIGGER IF EXISTS "+name+" ON "+table); err != nil {
			t.Fatalf("release trigger on %s: %v", table, err)
		}
	}
	t.Cleanup(release)
	return release
}

func (e *cov1xEnv) mustExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// draft creates a draft invoice for the household with one line item.
func (e *cov1xEnv) draft(t *testing.T, number string, cents int) *Invoice {
	t.Helper()
	inv, err := e.svc.CreateInvoice(e.org, e.admin.UserID, e.admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: number, DueDate: "2026-12-31"})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if cents > 0 {
		if _, err := e.svc.AddInvoiceItem(e.org, e.admin.UserID, e.admin, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "manual"}); err != nil {
			t.Fatalf("add item: %v", err)
		}
	}
	return inv
}

func (e *cov1xEnv) issued(t *testing.T, number string, cents int) *Invoice {
	t.Helper()
	inv := e.draft(t, number, cents)
	issued, _, err := e.svc.IssueInvoice(e.org, e.admin.UserID, e.admin, inv.ID)
	if err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	return issued
}

func (e *cov1xEnv) pay(t *testing.T, invoiceID string, cents int) *Payment {
	t.Helper()
	p, _, err := e.svc.RecordPayment(e.org, e.admin.UserID, e.admin, invoiceID, RecordPaymentInput{AmountCents: cents, Method: "check"})
	if err != nil {
		t.Fatalf("record payment: %v", err)
	}
	return p
}

// brokenService is a DB-backed service whose pool is already closed, so every
// database read fails.
func (e *cov1xEnv) brokenService(t *testing.T) *Service {
	t.Helper()
	closed, err := pgxpool.NewWithConfig(context.Background(), e.pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	return NewServiceWithPool(closed)
}

func cov1xWantErr(t *testing.T, name string, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error containing %q", name, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: error %q does not contain %q", name, err.Error(), substr)
	}
}

func cov1xDBReadsFailClosedWhenRefreshFails(t *testing.T, e *cov1xEnv) {
	b := e.brokenService(t)
	org, admin := e.org, e.admin
	const want = "refresh from DB"
	sched := FeeScheduleInput{Name: "n", Kind: ScheduleKindDues, GroupCode: "A", Frequency: FrequencyMonthly, AmountCents: 1, StartDate: "2026-09-01"}
	rule := DiscountRuleInput{Name: "n", AppliesTo: "all", MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 1}

	calls := map[string]func() error{
		"ListFeeSchedules":   func() error { _, err := b.ListFeeSchedules(org, admin); return err },
		"GetFeeSchedule":     func() error { _, err := b.GetFeeSchedule(org, admin, "x"); return err },
		"UpdateFeeSchedule":  func() error { _, _, err := b.UpdateFeeSchedule(org, "a", admin, "x", sched); return err },
		"ArchiveFeeSchedule": func() error { _, err := b.ArchiveFeeSchedule(org, "a", admin, "x"); return err },
		"ListDiscountRules":  func() error { _, err := b.ListDiscountRules(org, admin); return err },
		"UpdateDiscountRule": func() error { _, _, err := b.UpdateDiscountRule(org, "a", admin, "x", rule); return err },
		"ArchiveDiscount":    func() error { _, err := b.ArchiveDiscountRule(org, "a", admin, "x"); return err },
		"ListAdjustments":    func() error { _, err := b.ListAdjustments(org, admin, "", ""); return err },
		"VoidAdjustment":     func() error { _, err := b.VoidAdjustment(org, "a", admin, "x"); return err },
		"GetBillingSettings": func() error { _, err := b.GetBillingSettings(org, admin); return err },
		"UpdateBillingSettings": func() error {
			_, _, err := b.UpdateBillingSettings(org, "a", admin, BillingSettingsInput{LateFeeType: LateFeeTypeFlat, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"})
			return err
		},
		"CreateInvoice": func() error {
			_, err := b.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: "X"})
			return err
		},
		"InvoicesByExternalRef": func() error { _, err := b.InvoicesByExternalRef(org, admin, "ref"); return err },
		"GetInvoice":            func() error { _, err := b.GetInvoice(org, admin, "x"); return err },
		"ListInvoices":          func() error { _, err := b.ListInvoices(org, admin, ListInvoicesFilters{}); return err },
		"ListInvoiceItems":      func() error { _, err := b.ListInvoiceItems(org, admin, "x"); return err },
		"ListPayments":          func() error { _, err := b.ListPayments(org, admin, "x"); return err },
		"ListPaymentsForOrg":    func() error { _, err := b.ListPaymentsForOrg(org, admin, ListPaymentsFilters{}); return err },
		"GetPayment":            func() error { _, err := b.GetPayment(org, admin, "x", "y"); return err },
		"GetAutopayStatus":      func() error { _, err := b.GetHouseholdAutopayStatus(org, admin, e.hh); return err },
		"ListAutopayStatuses":   func() error { _, err := b.ListHouseholdAutopayStatuses(org, admin, nil); return err },
		"UpsertAutopay": func() error {
			_, err := b.UpsertHouseholdAutopay(org, "a", admin, e.hh, UpsertHouseholdAutopayInput{PaymentMethodType: "card", PaymentMethodLabel: "Visa"})
			return err
		},
		"OptOutAutopay": func() error { _, err := b.OptOutHouseholdAutopay(org, "a", admin, e.hh); return err },
		"RefundPreflight": func() error {
			_, err := b.ValidateRefundPreflight(org, admin, "x", CreateRefundInput{PaymentID: "p", AmountCents: 1, Reason: "r"})
			return err
		},
		"ListRefunds":      func() error { _, err := b.ListRefunds(org, admin, "x"); return err },
		"ListLedgerEvents": func() error { _, err := b.ListLedgerEvents(org, admin, ListLedgerFilters{}); return err },
		"IntegritySnapshot": func() error {
			_, _, _, _, _, err := b.IntegritySnapshot(context.Background(), org)
			return err
		},
		"StripePaymentFailed": func() error { return b.HandleStripePaymentIntentFailedFor(org, "pi") },
		"Reconciliation":      func() error { _, err := b.GetReconciliationReport(org, admin, 0, 0); return err },
		"RefreshAllClubs":     func() error { return b.Refresh(context.Background()) },
	}
	for name, call := range calls {
		cov1xWantErr(t, name, call(), want)
	}
	// Webhook bookkeeping talks to the database directly and reports the failure too.
	_, err := b.RecordWebhookEvent("stripe", org, "evt_broken", "x", nil)
	cov1xWantErr(t, "RecordWebhookEvent", err, "claim webhook event")
	cov1xWantErr(t, "MarkWebhookProcessed", b.MarkWebhookProcessed("stripe", "evt_broken", nil, false), "finish webhook event")
	_, err = b.ListWebhookEvents(org, admin)
	cov1xWantErr(t, "ListWebhookEvents", err, "load webhook events")
}

func cov1xDBMutationsOnUnknownInvoicesAreNotFound(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin
	missing := uuid.NewString()
	for _, id := range []string{"not-a-uuid", missing} {
		if _, err := svc.AddInvoiceItem(org, "a", admin, id, AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: 1, Category: "manual"}); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("add item %q: %v", id, err)
		}
		if err := svc.RemoveInvoiceItem(org, "a", admin, id, "item"); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("remove item %q: %v", id, err)
		}
		if _, _, err := svc.IssueInvoice(org, "a", admin, id); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("issue %q: %v", id, err)
		}
		if _, _, err := svc.VoidInvoice(org, "a", admin, id); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("void %q: %v", id, err)
		}
		if _, _, err := svc.WriteOffInvoice(org, "a", admin, id); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("write off %q: %v", id, err)
		}
		if _, _, err := svc.RecordPayment(org, "a", admin, id, RecordPaymentInput{AmountCents: 1, Method: "cash"}); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("payment %q: %v", id, err)
		}
		if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi", id, 1, "wh"); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("stripe success %q: %v", id, err)
		}
		if _, _, err := svc.HandleStripeChargeRefunded(org, "pi", id, 1, "re", "", "wh"); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("stripe refund %q: %v", id, err)
		}
	}
	// CreateInvoice insists on a UUID household when database backed.
	if _, err := svc.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: "hh-not-a-uuid", InvoiceNumber: "BAD"}); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("household must be a uuid: %v", err)
	}
}

func cov1xDBFullReloadAndScopedReloadErrors(t *testing.T, e *cov1xEnv) {
	ctx := context.Background()
	inv := e.issued(t, "RELOAD-1", 1000)
	if err := e.svc.Refresh(ctx); err != nil {
		t.Fatalf("all-club refresh: %v", err)
	}
	if got, err := e.svc.GetInvoice(e.org, e.admin, inv.ID); err != nil || got.ID != inv.ID {
		t.Fatalf("invoice must survive a full reload: %+v %v", got, err)
	}

	// A broken invoices schema fails the club load, in both the all-club and scoped paths.
	e.mustExec(t, `ALTER TABLE invoices RENAME COLUMN deleted_at TO deleted_at_cov1x`)
	cov1xWantErr(t, "all clubs", e.svc.Refresh(ctx), "invoices")
	cov1xWantErr(t, "scoped", e.svc.RefreshOrg(ctx, e.org), "invoices")
	e.mustExec(t, `ALTER TABLE invoices RENAME COLUMN deleted_at_cov1x TO deleted_at`)

	// A broken finance schema is reported as a finance load failure.
	e.mustExec(t, `ALTER TABLE billing_fee_schedules RENAME COLUMN description TO description_cov1x`)
	cov1xWantErr(t, "finance", e.svc.RefreshOrg(ctx, e.org), "load finance state")
	e.mustExec(t, `ALTER TABLE billing_fee_schedules RENAME COLUMN description_cov1x TO description`)
	if err := e.svc.RefreshOrg(ctx, e.org); err != nil {
		t.Fatalf("recovered refresh: %v", err)
	}
}

func TestCov1xInMemoryServiceNeverReloads(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if err := svc.reloadLocked(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func cov1xDBScopedReloadReplacesOnlyThatOrg(t *testing.T, e *cov1xEnv) {
	ctx := context.Background()
	const other = "00000000-0000-4000-8000-0000000000b2"
	// Cached state for this club (must be replaced) and for another club (must survive).
	e.svc.mu.Lock()
	f := e.svc.finance
	for _, o := range []string{e.org, other} {
		e.svc.ledger = append(e.svc.ledger, &LedgerEvent{ID: "led-" + o, OrgID: o})
		e.svc.autopayProfiles["ap-"+o] = &HouseholdAutopayProfile{OrgID: o, HouseholdID: "h"}
		f.adjustments["adj-"+o] = &Adjustment{ID: "adj-" + o, OrgID: o}
		f.lateFees["lf-"+o] = &LateFeeApplication{ID: "lf-" + o, OrgID: o}
		f.runs["run-"+o] = &BillingRun{ID: "run-" + o, OrgID: o}
		f.credits = append(f.credits, &CreditEntry{ID: "cr-" + o, OrgID: o})
		f.charges = append(f.charges, &BillingCharge{ID: "ch-" + o, OrgID: o})
		f.chargeKeys["ck-"+o] = &BillingCharge{ID: "ck-" + o, OrgID: o}
		f.balances[balanceKey(o, "h")] = 5
		f.settings[o] = &BillingSettings{OrgID: o}
	}
	e.svc.mu.Unlock()

	if err := e.svc.RefreshOrg(ctx, e.org); err != nil {
		t.Fatal(err)
	}
	e.svc.mu.Lock()
	defer e.svc.mu.Unlock()
	f = e.svc.finance
	if len(e.svc.ledger) != 1 || e.svc.ledger[0].OrgID != other {
		t.Fatalf("ledger after scoped reload: %+v", e.svc.ledger)
	}
	if len(e.svc.autopayProfiles) != 1 || e.svc.autopayProfiles["ap-"+other] == nil {
		t.Fatalf("autopay profiles: %+v", e.svc.autopayProfiles)
	}
	if len(f.adjustments) != 1 || len(f.lateFees) != 1 || len(f.runs) != 1 || len(f.chargeKeys) != 1 || len(f.settings) != 1 {
		t.Fatalf("finance maps must keep only the other club: %d %d %d %d %d", len(f.adjustments), len(f.lateFees), len(f.runs), len(f.chargeKeys), len(f.settings))
	}
	if len(f.credits) != 1 || f.credits[0].OrgID != other || len(f.charges) != 1 || f.charges[0].OrgID != other {
		t.Fatalf("credits/charges: %+v %+v", f.credits, f.charges)
	}
	if len(f.balances) != 1 || f.balances[balanceKey(other, "h")] != 5 {
		t.Fatalf("balances: %+v", f.balances)
	}
}

func cov1xDBFinanceStateRoundTripsAcrossInstances(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin
	max3 := 3
	if _, err := svc.CreateFeeSchedule(org, "a", admin, FeeScheduleInput{Name: "Dues", Kind: ScheduleKindDues, GroupCode: "GOLD", Frequency: FrequencyMonthly, AmountCents: 9000, StartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	rule, err := svc.CreateDiscountRule(org, "a", admin, DiscountRuleInput{Name: "Sibling", AppliesTo: "dues", MinPosition: 2, MaxPosition: &max3, DiscountType: DiscountTypePercent, PercentBps: 1000})
	if err != nil {
		t.Fatal(err)
	}
	adj, err := svc.CreateAdjustment(org, "a", admin, AdjustmentInput{HouseholdID: e.hh, AmountCents: -250, Reason: "scholarship"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UpdateBillingSettings(org, "a", admin, BillingSettingsInput{LateFeeEnabled: true, LateFeeType: LateFeeTypeFlat, LateFeeFlatCents: 500, AutoRunDay: 2, AutoRunInvoiceStatus: "issued"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertHouseholdAutopay(org, "a", admin, e.hh, UpsertHouseholdAutopayInput{PaymentMethodType: "card", PaymentMethodLabel: "Visa"}); err != nil {
		t.Fatal(err)
	}
	inv := e.issued(t, "RT-1", 4000)
	pay := e.pay(t, inv.ID, 3000)
	if _, _, err := svc.CreateRefund(org, "a", admin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 500, Reason: "adjustment"}); err != nil {
		t.Fatal(err)
	}

	// A second instance (worker/replica) sees everything, including the rules, refunds and autopay profile.
	other := NewServiceWithPool(e.pool)
	rules, err := other.ListDiscountRules(org, admin)
	if err != nil || len(rules) != 1 || rules[0].ID != rule.ID || rules[0].MaxPosition == nil || *rules[0].MaxPosition != 3 {
		t.Fatalf("rules: %+v %v", rules, err)
	}
	if adjs, _ := other.ListAdjustments(org, admin, "", e.hh); len(adjs) != 1 || adjs[0].ID != adj.ID {
		t.Fatalf("adjustments: %+v", adjs)
	}
	if st, _ := other.GetBillingSettings(org, admin); !st.LateFeeEnabled || st.LateFeeFlatCents != 500 {
		t.Fatalf("settings: %+v", st)
	}
	if refunds, err := other.ListRefunds(org, admin, inv.ID); err != nil || len(refunds) != 1 || refunds[0].AmountCents != 500 {
		t.Fatalf("refunds: %+v %v", refunds, err)
	}
	if st, err := other.GetHouseholdAutopayStatus(org, admin, e.hh); err != nil || !st.Enabled {
		t.Fatalf("autopay: %+v %v", st, err)
	}
	if got, err := other.GetInvoice(org, admin, inv.ID); err != nil || got.AmountPaidCents != 2500 {
		t.Fatalf("invoice after refund: %+v %v", got, err)
	}
	refs := other.SnapshotRefunds(org)
	if len(refs) != 1 {
		t.Fatalf("refund snapshot: %+v", refs)
	}
}

func cov1xDBFinanceConfigWriteFailuresAreReported(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin
	sched := FeeScheduleInput{Name: "Dues", Kind: ScheduleKindDues, GroupCode: "GOLD", Frequency: FrequencyMonthly, AmountCents: 9000, StartDate: "2026-09-01"}
	rule := DiscountRuleInput{Name: "Sib", AppliesTo: "all", MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 100}
	settings := BillingSettingsInput{LateFeeType: LateFeeTypeFlat, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}

	existingSched, err := svc.CreateFeeSchedule(org, "a", admin, sched)
	if err != nil {
		t.Fatal(err)
	}
	existingRule, err := svc.CreateDiscountRule(org, "a", admin, rule)
	if err != nil {
		t.Fatal(err)
	}
	adj, err := svc.CreateAdjustment(org, "a", admin, AdjustmentInput{HouseholdID: e.hh, AmountCents: 100, Reason: "fee"})
	if err != nil {
		t.Fatal(err)
	}

	release := e.inject(t, "billing_fee_schedules", "INSERT OR UPDATE", "")
	_, err = svc.CreateFeeSchedule(org, "a", admin, sched)
	cov1xWantErr(t, "create schedule", err, "injected failure")
	_, _, err = svc.UpdateFeeSchedule(org, "a", admin, existingSched.ID, sched)
	cov1xWantErr(t, "update schedule", err, "injected failure")
	_, err = svc.ArchiveFeeSchedule(org, "a", admin, existingSched.ID)
	cov1xWantErr(t, "archive schedule", err, "injected failure")
	release()
	if got, _ := svc.GetFeeSchedule(org, admin, existingSched.ID); !got.Active {
		t.Fatal("a failed archive must leave the schedule active")
	}
	if list, _ := svc.ListFeeSchedules(org, admin); len(list) != 1 {
		t.Fatalf("a failed create must not be cached: %d", len(list))
	}

	release = e.inject(t, "billing_discount_rules", "INSERT OR UPDATE", "")
	_, err = svc.CreateDiscountRule(org, "a", admin, rule)
	cov1xWantErr(t, "create rule", err, "injected failure")
	_, _, err = svc.UpdateDiscountRule(org, "a", admin, existingRule.ID, rule)
	cov1xWantErr(t, "update rule", err, "injected failure")
	_, err = svc.ArchiveDiscountRule(org, "a", admin, existingRule.ID)
	cov1xWantErr(t, "archive rule", err, "injected failure")
	release()
	if got, _ := svc.ListDiscountRules(org, admin); len(got) != 1 || !got[0].Active {
		t.Fatalf("rules after failed writes: %+v", got)
	}

	release = e.inject(t, "billing_adjustments", "INSERT OR UPDATE", "")
	_, err = svc.CreateAdjustment(org, "a", admin, AdjustmentInput{HouseholdID: e.hh, AmountCents: 1, Reason: "x"})
	cov1xWantErr(t, "create adjustment", err, "injected failure")
	_, err = svc.VoidAdjustment(org, "a", admin, adj.ID)
	cov1xWantErr(t, "void adjustment", err, "injected failure")
	release()

	// A write that silently touches no row means another process already changed it.
	release = e.injectSkip(t, "billing_adjustments", "UPDATE")
	if _, err := svc.VoidAdjustment(org, "a", admin, adj.ID); !errors.Is(err, ErrAdjustmentNotPending) {
		t.Fatalf("lost update must report not pending: %v", err)
	}
	release()
	if got, _ := svc.ListAdjustments(org, admin, AdjustmentStatusPending, ""); len(got) != 1 {
		t.Fatalf("adjustment must stay pending: %+v", got)
	}

	release = e.inject(t, "billing_settings", "INSERT OR UPDATE", "")
	_, _, err = svc.UpdateBillingSettings(org, "a", admin, settings)
	cov1xWantErr(t, "settings", err, "injected failure")
	release()
	if st, _ := svc.GetBillingSettings(org, admin); st.UpdatedBy != "" {
		t.Fatalf("failed settings write must not be cached: %+v", st)
	}
}

func cov1xDBReleaseInvoiceFinanceReportsTransactionFailures(t *testing.T, e *cov1xEnv) {
	ctx := context.Background()
	call := func() error {
		e.svc.mu.Lock()
		defer e.svc.mu.Unlock()
		return e.svc.releaseInvoiceFinanceLocked(ctx, e.org, uuid.NewString(), "actor")
	}
	if err := call(); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	release := e.injectStatement(t, "billing_charges", "UPDATE")
	cov1xWantErr(t, "release charges", call(), "injected failure")
	release()
	release = e.injectStatement(t, "billing_adjustments", "UPDATE")
	cov1xWantErr(t, "reopen adjustments", call(), "injected failure")
	release()
	cov1xWantErr(t, "closed pool", func() error {
		b := e.brokenService(t)
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.releaseInvoiceFinanceLocked(ctx, e.org, uuid.NewString(), "actor")
	}(), "")

	// Voiding still succeeds when releasing the run charges fails: the failure is logged, not fatal.
	inv := e.draft(t, "VOID-RELEASE", 500)
	release = e.injectStatement(t, "billing_charges", "UPDATE")
	voided, _, err := e.svc.VoidInvoice(e.org, "a", e.admin, inv.ID)
	release()
	if err == nil {
		// The staged transaction is the one that failed to release; the void must roll back with it.
		t.Fatalf("void must not commit when the release statement fails, got %+v", voided)
	}
	if got, err := e.svc.GetInvoice(e.org, e.admin, inv.ID); err != nil || got.Status != "draft" {
		t.Fatalf("a failed void must leave the invoice a draft: %+v %v", got, err)
	}
}
