package billing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov3xLateFeeFor(t *testing.T) {
	cases := []struct {
		name     string
		settings BillingSettings
		due      int
		want     int
	}{
		{"flat", BillingSettings{LateFeeType: LateFeeTypeFlat, LateFeeFlatCents: 2500}, 99999, 2500},
		{"percent", BillingSettings{LateFeeType: LateFeeTypePercent, LateFeePercentBps: 500}, 20000, 1000},
		{"percent rounds half up", BillingSettings{LateFeeType: LateFeeTypePercent, LateFeePercentBps: 500}, 30, 2},
		{"percent below a cent", BillingSettings{LateFeeType: LateFeeTypePercent, LateFeePercentBps: 500}, 5, 0},
		{"cap applies to flat", BillingSettings{LateFeeType: LateFeeTypeFlat, LateFeeFlatCents: 2500, LateFeeMaxCents: 1000}, 1, 1000},
		{"cap applies to percent", BillingSettings{LateFeeType: LateFeeTypePercent, LateFeePercentBps: 500, LateFeeMaxCents: 800}, 20000, 800},
		{"cap above fee leaves fee", BillingSettings{LateFeeType: LateFeeTypePercent, LateFeePercentBps: 500, LateFeeMaxCents: 5000}, 20000, 1000},
		{"negative flat fee is clamped", BillingSettings{LateFeeType: LateFeeTypeFlat, LateFeeFlatCents: -50}, 1000, 0},
		{"unknown type charges nothing", BillingSettings{LateFeeType: "bogus", LateFeeFlatCents: 700}, 1000, 0},
	}
	for _, c := range cases {
		s := c.settings
		if got := lateFeeFor(&s, c.due); got != c.want {
			t.Fatalf("%s: lateFeeFor = %d want %d", c.name, got, c.want)
		}
	}
}

func TestCov3xApplyLateFeesInMemory(t *testing.T) {
	svc := NewService()
	org := "org-cov3x-lf"
	p := cov3xAdmin(org)

	coach := *p
	coach.Roles = []string{"coach"}
	if _, err := svc.ApplyLateFees(org, "c", &coach, time.Now()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin must be forbidden: %v", err)
	}

	invA := cov3xInvoice(t, svc, org, "hh-1", "LF-A", "2026-10-01", 20000) // 5% = 1000, capped to 800
	invB := cov3xInvoice(t, svc, org, "hh-2", "LF-B", "2026-10-01", 4000)  // 5% = 200
	// Ineligible invoices, each for a different reason.
	draft, err := svc.CreateInvoice(org, p.UserID, p, CreateInvoiceInput{HouseholdID: "hh-3", InvoiceNumber: "LF-C-DRAFT", DueDate: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	paid := cov3xInvoice(t, svc, org, "hh-4", "LF-D-PAID", "2026-10-01", 3000)
	if _, _, err := svc.RecordPayment(org, p.UserID, p, paid.ID, RecordPaymentInput{AmountCents: 3000, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	badDue := cov3xInvoice(t, svc, org, "hh-5", "LF-E-BADDUE", "2026-10-01", 3000)
	svc.mu.Lock()
	svc.invoices[badDue.ID].DueDate = "not-a-date"
	svc.mu.Unlock()
	cov3xInvoice(t, svc, org, "hh-6", "LF-F-GRACE", "2026-12-01", 3000)
	tiny := cov3xInvoice(t, svc, org, "hh-7", "LF-G-TINY", "2026-10-01", 5) // 5% of 5 cents rounds to 0
	already := cov3xInvoice(t, svc, org, "hh-8", "LF-H-ALREADY", "2026-10-01", 3000)
	svc.mu.Lock()
	svc.finance.lateFees[org+"|"+already.ID] = &LateFeeApplication{ID: "lf-existing", OrgID: org, InvoiceID: already.ID, AmountCents: 100}
	svc.mu.Unlock()

	asOf := cov3xDay(t, "2026-12-05")

	// Disabled by default: reports Enabled=false and changes nothing.
	res, err := svc.ApplyLateFees(org, p.UserID, p, asOf)
	if err != nil || res.Enabled || res.AppliedCount != 0 || len(res.Assessments) != 0 || res.AsOf != "2026-12-05" {
		t.Fatalf("disabled sweep: %+v %v", res, err)
	}
	if got, _ := svc.GetInvoice(org, p, invA.ID); got.TotalCents != 20000 {
		t.Fatalf("disabled sweep must not touch invoices: %+v", got)
	}

	if _, _, err := svc.UpdateBillingSettings(org, p.UserID, p, BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 5, LateFeeType: "percent", LateFeePercentBps: 500, LateFeeMaxCents: 800, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	res, err = svc.ApplyLateFees(org, p.UserID, p, asOf.Add(13*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Enabled || res.AppliedCount != 2 || res.TotalCents != 1000 || len(res.Assessments) != 2 {
		t.Fatalf("sweep: %+v", res)
	}
	a0, a1 := res.Assessments[0], res.Assessments[1]
	if a0.InvoiceNumber != "LF-A" || a0.FeeCents != 800 || a0.DaysOverdue != 65 || a0.DueDate != "2026-10-01" || a0.HouseholdID != "hh-1" || a0.InvoiceID != invA.ID {
		t.Fatalf("assessment A: %+v", a0)
	}
	if a1.InvoiceNumber != "LF-B" || a1.FeeCents != 200 {
		t.Fatalf("assessment B (sorted by number): %+v", a1)
	}

	gotA, _ := svc.GetInvoice(org, p, invA.ID)
	if gotA.SubtotalCents != 20800 || gotA.TotalCents != 20800 || gotA.AmountDueCents != 20800 {
		t.Fatalf("invoice A after fee: %+v", gotA)
	}
	items, err := svc.ListInvoiceItems(org, p, invA.ID)
	if err != nil || len(items) != 2 || items[1].Category != itemCategoryLateFee || items[1].TotalCents != 800 || items[1].Description != "Late fee (invoice due 2026-10-01)" {
		t.Fatalf("late fee line item: %+v %v", items, err)
	}
	var feeEvents int
	for _, ev := range svc.SnapshotLedgerEvents(org) {
		if ev.EventType == "late_fee_assessed" {
			feeEvents++
			if ev.AmountCents != 800 && ev.AmountCents != 200 {
				t.Fatalf("unexpected late-fee ledger amount: %+v", ev)
			}
		}
	}
	if feeEvents != 2 {
		t.Fatalf("want 2 late_fee_assessed events, got %d", feeEvents)
	}
	for _, untouched := range []string{draft.ID, paid.ID, badDue.ID, tiny.ID, already.ID} {
		inv, _ := svc.GetInvoice(org, p, untouched)
		if inv.TotalCents != map[string]int{draft.ID: 0, paid.ID: 3000, badDue.ID: 3000, tiny.ID: 5, already.ID: 3000}[untouched] {
			t.Fatalf("ineligible invoice %s was charged a fee: %+v", inv.InvoiceNumber, inv)
		}
	}

	// Re-running never double-charges: a later sweep only picks up the invoice
	// whose grace period has since expired (5% of 3000 = 150), once.
	res, err = svc.ApplyLateFees(org, p.UserID, p, cov3xDay(t, "2027-03-01"))
	if err != nil || res.AppliedCount != 1 || res.Assessments[0].InvoiceNumber != "LF-F-GRACE" || res.Assessments[0].FeeCents != 150 {
		t.Fatalf("second sweep: %+v %v", res, err)
	}
	res, err = svc.ApplyLateFees(org, p.UserID, p, cov3xDay(t, "2027-04-01"))
	if err != nil || res.AppliedCount != 0 {
		t.Fatalf("third sweep: %+v %v", res, err)
	}
	if got, _ := svc.GetInvoice(org, p, invB.ID); got.TotalCents != 4200 {
		t.Fatalf("invoice B total: %+v", got)
	}
}

func TestCov3xAssessLateFeeLockedRejectsDuplicateInMemory(t *testing.T) {
	svc := NewService()
	org := "org-cov3x-lf-dup"
	inv := cov3xInvoice(t, svc, org, "hh-1", "LF-DUP", "2026-10-01", 1000)
	ctx := context.Background()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	live := svc.invoices[inv.ID]
	if err := svc.assessLateFeeLocked(ctx, live, 250, "tester"); err != nil {
		t.Fatal(err)
	}
	if live.TotalCents != 1250 || live.AmountDueCents != 1250 {
		t.Fatalf("first assessment: %+v", live)
	}
	itemsBefore := len(svc.items[live.ID])
	if err := svc.assessLateFeeLocked(ctx, live, 250, "tester"); !errors.Is(err, errLateFeeExists) {
		t.Fatalf("duplicate assessment: %v", err)
	}
	if live.TotalCents != 1250 || len(svc.items[live.ID]) != itemsBefore {
		t.Fatalf("a rejected duplicate must not alter the invoice or add items: %+v", live)
	}
}

func TestCov3xRunScheduledLateFeesUsesSystemPrincipal(t *testing.T) {
	svc := NewService()
	org := "org-cov3x-lf-sched"
	p := cov3xAdmin(org)
	inv := cov3xInvoice(t, svc, org, "hh-1", "LF-S", "2026-10-01", 10000)
	if _, _, err := svc.UpdateBillingSettings(org, p.UserID, p, BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 7, LateFeeType: "flat", LateFeeFlatCents: 1500, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	// Inside the 7 day grace window: nothing happens.
	res, err := svc.RunScheduledLateFees(org, cov3xDay(t, "2026-10-08"))
	if err != nil || res.AppliedCount != 0 {
		t.Fatalf("within grace: %+v %v", res, err)
	}
	res, err = svc.RunScheduledLateFees(org, cov3xDay(t, "2026-10-09"))
	if err != nil || res.AppliedCount != 1 || res.TotalCents != 1500 {
		t.Fatalf("after grace: %+v %v", res, err)
	}
	items, _ := svc.ListInvoiceItems(org, p, inv.ID)
	if len(items) != 2 || items[1].CreatedBy != "system:billing-automation" {
		t.Fatalf("scheduled fee must be attributed to the automation principal: %+v", items)
	}
}

// ─── Postgres-backed behaviour ───────────────────────────────────────────────

// cov3xClosedPoolService returns a pool-backed Service whose pool has already
// been closed, so every database call fails deterministically.
func cov3xClosedPoolService(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	dead, err := pgxpool.NewWithConfig(context.Background(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewServiceWithPool(dead)
	dead.Close()
	return svc
}

func cov3xScalar[T any](t *testing.T, pool *pgxpool.Pool, query string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

func cov3xExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestCov3xApplyLateFeesPostgres(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org, hh := uuid.NewString(), uuid.NewString()
	cov3xExec(t, pool, `INSERT INTO households (id, org_id, household_name) VALUES ('`+hh+`', '`+org+`', 'Late Fee HH')`)
	p := cov3xAdmin(org)
	svc := NewServiceWithPool(pool)
	other := NewServiceWithPool(pool) // a second API instance

	inv := cov3xInvoice(t, svc, org, hh, "PG-LF-1", "2026-10-01", 10000)
	if _, _, err := svc.UpdateBillingSettings(org, p.UserID, p, BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 0, LateFeeType: "flat", LateFeeFlatCents: 500, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	asOf := cov3xDay(t, "2026-11-01")

	invoiceTotal := func() int {
		return cov3xScalar[int](t, pool, `SELECT total_cents FROM invoices WHERE id = $1`, inv.ID)
	}
	feeRows := func() int {
		return cov3xScalar[int](t, pool, `SELECT count(*) FROM billing_late_fee_applications WHERE invoice_id = $1`, inv.ID)
	}
	itemRows := func() int {
		return cov3xScalar[int](t, pool, `SELECT count(*) FROM invoice_items WHERE invoice_id = $1`, inv.ID)
	}
	ledgerRows := func() int {
		return cov3xScalar[int](t, pool, `SELECT count(*) FROM ledger_events WHERE invoice_id = $1 AND event_type = 'late_fee_assessed'`, inv.ID)
	}
	assertUntouched := func(label string) {
		t.Helper()
		res, err := svc.ApplyLateFees(org, p.UserID, p, asOf)
		if err != nil || res.AppliedCount != 0 {
			t.Fatalf("%s: sweep result %+v err %v (failures are logged per invoice, never fatal)", label, res, err)
		}
		if invoiceTotal() != 10000 || feeRows() != 0 || itemRows() != 1 || ledgerRows() != 0 {
			t.Fatalf("%s: failed assessment must roll back completely: total=%d fees=%d items=%d ledger=%d", label, invoiceTotal(), feeRows(), itemRows(), ledgerRows())
		}
		svc.mu.RLock()
		cached := *svc.invoices[inv.ID]
		svc.mu.RUnlock()
		if cached.TotalCents != 10000 || len(svc.finance.lateFees) != 0 {
			t.Fatalf("%s: cache must not run ahead of the database: %+v", label, cached)
		}
	}

	// A failure at each write step aborts the whole assessment.
	injections := []struct{ name, setup, teardown string }{
		{"late fee row insert", `CREATE FUNCTION cov3x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$;
			CREATE TRIGGER cov3x_t BEFORE INSERT ON billing_late_fee_applications FOR EACH ROW EXECUTE FUNCTION cov3x_fail()`,
			`DROP TRIGGER cov3x_t ON billing_late_fee_applications; DROP FUNCTION cov3x_fail()`},
		{"item upsert", `CREATE FUNCTION cov3x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.category = 'late_fee' THEN RAISE EXCEPTION 'injected'; END IF; RETURN NEW; END $$;
			CREATE TRIGGER cov3x_t BEFORE INSERT ON invoice_items FOR EACH ROW EXECUTE FUNCTION cov3x_fail()`,
			`DROP TRIGGER cov3x_t ON invoice_items; DROP FUNCTION cov3x_fail()`},
		{"invoice upsert", `CREATE FUNCTION cov3x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$;
			CREATE TRIGGER cov3x_t BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION cov3x_fail()`,
			`DROP TRIGGER cov3x_t ON invoices; DROP FUNCTION cov3x_fail()`},
		{"ledger insert", `CREATE FUNCTION cov3x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type = 'late_fee_assessed' THEN RAISE EXCEPTION 'injected'; END IF; RETURN NEW; END $$;
			CREATE TRIGGER cov3x_t BEFORE INSERT ON ledger_events FOR EACH ROW EXECUTE FUNCTION cov3x_fail()`,
			`DROP TRIGGER cov3x_t ON ledger_events; DROP FUNCTION cov3x_fail()`},
	}
	for _, in := range injections {
		cov3xExec(t, pool, in.setup)
		assertUntouched(in.name)
		cov3xExec(t, pool, in.teardown)
	}

	// Cached money fields that disagree with the locked row (another instance
	// paid meanwhile) must refuse the full-row overwrite.
	svc.mu.Lock()
	svc.invoices[inv.ID].AmountDueCents += 1
	svc.mu.Unlock()
	res, err := svc.ApplyLateFees(org, p.UserID, p, asOf)
	if err != nil || res.AppliedCount != 0 || invoiceTotal() != 10000 || feeRows() != 0 {
		t.Fatalf("stale cache: %+v %v total=%d fees=%d", res, err, invoiceTotal(), feeRows())
	}
	svc.mu.Lock()
	svc.invoices[inv.ID].AmountDueCents -= 1
	svc.mu.Unlock()

	// The healthy sweep persists everything atomically.
	// Warm the other instance's view of the club before the fee exists.
	if _, err := other.GetInvoice(org, p, inv.ID); err != nil {
		t.Fatal(err)
	}
	res, err = svc.ApplyLateFees(org, p.UserID, p, asOf)
	if err != nil || res.AppliedCount != 1 || res.TotalCents != 500 || !res.Enabled {
		t.Fatalf("sweep: %+v %v", res, err)
	}
	if invoiceTotal() != 10500 || feeRows() != 1 || itemRows() != 2 || ledgerRows() != 1 {
		t.Fatalf("persisted: total=%d fees=%d items=%d ledger=%d", invoiceTotal(), feeRows(), itemRows(), ledgerRows())
	}
	if got := cov3xScalar[int](t, pool, `SELECT amount_due_cents FROM invoices WHERE id = $1`, inv.ID); got != 10500 {
		t.Fatalf("amount due = %d", got)
	}

	// Another instance sees the fee and does not assess it again.
	res, err = other.ApplyLateFees(org, p.UserID, p, asOf)
	if err != nil || res.AppliedCount != 0 || feeRows() != 1 {
		t.Fatalf("second instance: %+v %v fees=%d", res, err, feeRows())
	}
	// If that instance's cache were stale, the unique row still stops a double fee.
	other.mu.Lock()
	delete(other.finance.lateFees, org+"|"+inv.ID)
	other.mu.Unlock()
	res, err = other.ApplyLateFees(org, p.UserID, p, asOf)
	if err != nil || res.AppliedCount != 0 || feeRows() != 1 || invoiceTotal() != 10500 {
		t.Fatalf("stale second instance: %+v %v fees=%d total=%d", res, err, feeRows(), invoiceTotal())
	}

	// lockInvoiceFresh compares the locked row with the supplied copy.
	check := func(mutate func(*Invoice)) error {
		cp := Invoice{ID: inv.ID, OrgID: org, Status: "issued", AmountPaidCents: 0, TotalCents: 10500, AmountDueCents: 10500}
		mutate(&cp)
		return svc.repo.inTx(ctx, org, func(tx pgx.Tx) error { return lockInvoiceFresh(ctx, tx, &cp) })
	}
	if err := check(func(*Invoice) {}); err != nil {
		t.Fatalf("matching copy: %v", err)
	}
	for name, mut := range map[string]func(*Invoice){
		"status": func(i *Invoice) { i.Status = "paid" },
		"paid":   func(i *Invoice) { i.AmountPaidCents = 1 },
		"total":  func(i *Invoice) { i.TotalCents = 1 },
		"due":    func(i *Invoice) { i.AmountDueCents = 1 },
	} {
		if err := check(mut); !errors.Is(err, errInvoiceStale) {
			t.Fatalf("%s mismatch: %v", name, err)
		}
	}
	if err := check(func(i *Invoice) { i.ID = uuid.NewString() }); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing row: %v", err)
	}

	// Unreachable database: the sweep and the finance reads surface the failure.
	dead := cov3xClosedPoolService(t, pool)
	if _, err := dead.ApplyLateFees(org, p.UserID, p, asOf); err == nil {
		t.Fatal("sweep against a closed pool must fail")
	}
	if _, err := dead.HouseholdStatement(org, p, hh, time.Time{}, cov3xFarFuture(t)); err == nil {
		t.Fatal("statement against a closed pool must fail")
	}
	// A club whose data cannot be read is never swept (and no lock is left held).
	cov3xExec(t, pool, `ALTER TABLE invoices RENAME COLUMN notes TO notes_broken`)
	fresh := NewServiceWithPool(pool)
	if _, err := fresh.ApplyLateFees(org, p.UserID, p, asOf); err == nil || !strings.Contains(err.Error(), `column "notes"`) {
		t.Fatalf("unreadable club: %v", err)
	}
	dead.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, nil })
	if _, err := dead.HouseholdUpcomingCharges(org, p, hh, asOf, 30); err == nil {
		t.Fatal("upcoming charges against a closed pool must fail")
	}
}
