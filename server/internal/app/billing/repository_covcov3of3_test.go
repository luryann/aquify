package billing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov3xTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func TestCov3xRepositoryRoundTripsEveryRecordType(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	r := newRepository(pool)
	org, hh := uuid.NewString(), uuid.NewString()
	cov3xExec(t, pool, `INSERT INTO households (id, org_id, household_name) VALUES ('`+hh+`', '`+org+`', 'Repo HH')`)

	created := cov3xTime("2026-10-01T10:00:00Z")
	issued := cov3xTime("2026-10-02T10:00:00Z")
	inv := &Invoice{ID: uuid.NewString(), OrgID: org, HouseholdID: hh, InvoiceNumber: "REPO-1", Status: "issued",
		SubtotalCents: 9000, TaxCents: 1000, TotalCents: 10000, AmountDueCents: 10000, DueDate: "2026-11-01",
		IssuedAt: &issued, Notes: "first", ExternalRef: "ext-1", CreatedAt: created, UpdatedAt: created}
	if err := r.UpsertInvoice(ctx, inv); err != nil {
		t.Fatal(err)
	}
	invoices, err := r.LoadInvoices(ctx, org)
	if err != nil || len(invoices) != 1 {
		t.Fatalf("invoices: %v %v", invoices, err)
	}
	g := invoices[0]
	if g.ID != inv.ID || g.HouseholdID != hh || g.InvoiceNumber != "REPO-1" || g.TotalCents != 10000 || g.TaxCents != 1000 ||
		g.DueDate != "2026-11-01" || g.Notes != "first" || g.ExternalRef != "ext-1" || g.IssuedAt == nil || !g.IssuedAt.Equal(issued) {
		t.Fatalf("loaded invoice: %+v", g)
	}
	// Re-upserting updates in place instead of duplicating.
	voided := cov3xTime("2026-10-05T10:00:00Z")
	inv.Status, inv.Notes, inv.VoidedAt, inv.AmountDueCents = "voided", "second", &voided, 0
	if err := r.UpsertInvoice(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if invoices, _ = r.LoadInvoices(ctx, org); len(invoices) != 1 || invoices[0].Status != "voided" || invoices[0].Notes != "second" || invoices[0].VoidedAt == nil {
		t.Fatalf("invoice after update: %+v", invoices)
	}

	// Invoice items: total is derived, updates replace, delete removes.
	item := &InvoiceItem{ID: uuid.NewString(), OrgID: org, InvoiceID: inv.ID, Description: "Dues", Quantity: 3, UnitAmountCents: 1500, Category: "program_fee", CreatedAt: created, UpdatedAt: created}
	if err := r.UpsertInvoiceItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	items, err := r.LoadInvoiceItems(ctx, org)
	if err != nil || len(items) != 1 || items[0].TotalCents != 4500 || items[0].Category != "program_fee" || items[0].SwimmerID != "" {
		t.Fatalf("items: %+v %v", items, err)
	}
	item.Quantity, item.Description = 4, "Dues (4)"
	if err := r.UpsertInvoiceItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	if items, _ = r.LoadInvoiceItems(ctx, org); len(items) != 1 || items[0].TotalCents != 6000 || items[0].Description != "Dues (4)" {
		t.Fatalf("items after update: %+v", items)
	}
	if err := r.DeleteInvoiceItem(ctx, org, item.ID); err != nil {
		t.Fatal(err)
	}
	if items, _ = r.LoadInvoiceItems(ctx, org); len(items) != 0 {
		t.Fatalf("item must be deleted: %+v", items)
	}
	if err := r.DeleteInvoiceItem(ctx, org, uuid.NewString()); err != nil {
		t.Fatalf("deleting an unknown item is a no-op: %v", err)
	}

	// Payments.
	paidAt := cov3xTime("2026-10-03T10:00:00Z")
	pay := &Payment{ID: uuid.NewString(), OrgID: org, InvoiceID: inv.ID, AmountCents: 4000, Method: "stripe", Status: "pending",
		StripePaymentIntentID: "pi_cov3x", StripeAccountID: "acct_cov3x", ExternalRef: "ref-1", Notes: "n1", CreatedAt: created, UpdatedAt: created}
	if err := r.UpsertPayment(ctx, pay); err != nil {
		t.Fatal(err)
	}
	pay.Status, pay.PaidAt, pay.Notes = "succeeded", &paidAt, "n2"
	if err := r.UpsertPayment(ctx, pay); err != nil {
		t.Fatal(err)
	}
	payments, err := r.LoadPayments(ctx, org)
	if err != nil || len(payments) != 1 {
		t.Fatalf("payments: %+v %v", payments, err)
	}
	if pp := payments[0]; pp.Status != "succeeded" || pp.PaidAt == nil || !pp.PaidAt.Equal(paidAt) || pp.Notes != "n2" || pp.StripePaymentIntentID != "pi_cov3x" ||
		pp.StripeAccountID != "acct_cov3x" || pp.AmountCents != 4000 || pp.FailedAt != nil {
		t.Fatalf("loaded payment: %+v", pp)
	}

	// Refunds.
	ref := &Refund{ID: uuid.NewString(), OrgID: org, InvoiceID: inv.ID, PaymentID: pay.ID, AmountCents: 1000, Reason: "oops", Status: "pending", CreatedAt: created, UpdatedAt: created}
	if err := r.UpsertRefund(ctx, ref); err != nil {
		t.Fatal(err)
	}
	processed := cov3xTime("2026-10-04T10:00:00Z")
	ref.Status, ref.ProcessedAt, ref.StripeRefundID = "processed", &processed, "re_cov3x"
	if err := r.UpsertRefund(ctx, ref); err != nil {
		t.Fatal(err)
	}
	refunds, err := r.LoadRefunds(ctx, org)
	if err != nil || len(refunds) != 1 || refunds[0].Status != "processed" || refunds[0].StripeRefundID != "re_cov3x" ||
		refunds[0].ProcessedAt == nil || refunds[0].PaymentID != pay.ID || refunds[0].Reason != "oops" {
		t.Fatalf("refunds: %+v %v", refunds, err)
	}

	// Ledger events are append-only: a repeated id is ignored.
	ev := &LedgerEvent{ID: uuid.NewString(), OrgID: org, EventType: "invoice_issued", InvoiceID: inv.ID, HouseholdID: hh, AmountCents: 10000, Description: "issued", CreatedAt: created}
	if err := r.InsertLedgerEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	dup := *ev
	dup.AmountCents, dup.Description = 1, "tampered"
	if err := r.InsertLedgerEvent(ctx, &dup); err != nil {
		t.Fatal(err)
	}
	later := &LedgerEvent{ID: uuid.NewString(), OrgID: org, EventType: "payment_succeeded", InvoiceID: inv.ID, PaymentID: pay.ID, AmountCents: -4000, CreatedAt: created.Add(time.Hour)}
	if err := r.InsertLedgerEvent(ctx, later); err != nil {
		t.Fatal(err)
	}
	events, err := r.LoadLedgerEvents(ctx, org)
	if err != nil || len(events) != 2 {
		t.Fatalf("ledger: %+v %v", events, err)
	}
	if events[0].ID != ev.ID || events[0].AmountCents != 10000 || events[0].Description != "issued" || events[0].HouseholdID != hh || events[0].PaymentID != "" {
		t.Fatalf("ledger must keep the original and be ordered by time: %+v", events[0])
	}
	if events[1].PaymentID != pay.ID || events[1].AmountCents != -4000 || events[1].Description != "" {
		t.Fatalf("second ledger event: %+v", events[1])
	}

	// Autopay profile: nullable columns round-trip as empty values, upsert is per household.
	prof := &HouseholdAutopayProfile{ID: uuid.NewString(), OrgID: org, HouseholdID: hh, Enabled: true, PaymentMethodType: "card",
		PaymentMethodBrand: "visa", PaymentMethodLast4: "4242", PaymentMethodLabel: "Visa 4242", PaymentMethodExternalRef: "pm_cov3x",
		PaymentMethodExpMonth: 12, PaymentMethodExpYear: 2031, TriggerDaysBeforeDue: 3, CreatedAt: created, UpdatedAt: created, UpdatedBy: "admin-1"}
	if err := r.UpsertHouseholdAutopayProfile(ctx, prof); err != nil {
		t.Fatal(err)
	}
	prof2 := *prof
	prof2.ID = uuid.NewString() // same household: conflicts on (org, household) and updates
	prof2.Enabled, prof2.PaymentMethodBrand, prof2.PaymentMethodExternalRef, prof2.TriggerDaysBeforeDue, prof2.UpdatedBy = false, "", "", 7, ""
	if err := r.UpsertHouseholdAutopayProfile(ctx, &prof2); err != nil {
		t.Fatal(err)
	}
	profiles, err := r.LoadHouseholdAutopayProfiles(ctx, org)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles: %+v %v", profiles, err)
	}
	if pr := profiles[0]; pr.ID != prof.ID || pr.Enabled || pr.PaymentMethodBrand != "" || pr.PaymentMethodExternalRef != "" || pr.TriggerDaysBeforeDue != 7 ||
		pr.PaymentMethodLast4 != "4242" || pr.PaymentMethodExpYear != 2031 || pr.UpdatedBy != "" || pr.HouseholdID != hh {
		t.Fatalf("loaded profile: %+v", pr)
	}

	// LoadAll returns every table in one call.
	li, lit, lp, lr, ll, la, err := r.LoadAll(ctx, org)
	if err != nil || len(li) != 1 || len(lit) != 0 || len(lp) != 1 || len(lr) != 1 || len(ll) != 2 || len(la) != 1 {
		t.Fatalf("LoadAll: %d %d %d %d %d %d %v", len(li), len(lit), len(lp), len(lr), len(ll), len(la), err)
	}

	// The registry lists clubs.
	orgs, err := r.OrgIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range orgs {
		found = found || id == org
	}
	if !found {
		t.Fatalf("OrgIDs %v must include %s", orgs, org)
	}
}

func TestCov3xRepositoryTransactionBoundWritesRollBack(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	r := newRepository(pool)
	org, hh := uuid.NewString(), uuid.NewString()
	cov3xExec(t, pool, `INSERT INTO households (id, org_id, household_name) VALUES ('`+hh+`', '`+org+`', 'Tx HH')`)
	now := cov3xTime("2026-10-01T10:00:00Z")
	inv := &Invoice{ID: uuid.NewString(), OrgID: org, HouseholdID: hh, InvoiceNumber: "TX-1", Status: "draft", CreatedAt: now, UpdatedAt: now}

	tx, err := r.at(org).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bound := &Repository{pool: pool, tx: tx}
	if err := bound.UpsertInvoice(ctx, inv); err != nil {
		t.Fatal(err)
	}
	inside, err := bound.LoadInvoices(ctx, org)
	if err != nil || len(inside) != 1 {
		t.Fatalf("writes must be visible inside their transaction: %v %v", inside, err)
	}
	// inTx on a tx-bound repository joins the surrounding transaction.
	if err := bound.inTx(ctx, org, func(joined pgx.Tx) error {
		if joined != tx {
			t.Fatal("inTx must reuse the bound transaction")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if after, _ := r.LoadInvoices(ctx, org); len(after) != 0 {
		t.Fatalf("rolled back write persisted: %+v", after)
	}
}

func TestCov3xRepositoryLoadFailuresAreWrapped(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org := uuid.NewString()

	// Unreachable database: every loader reports what it was loading.
	dead := cov3xClosedPoolService(t, pool).repo
	loaders := []struct {
		prefix string
		call   func() error
	}{
		{"billing: load invoices", func() error { _, err := dead.LoadInvoices(ctx, org); return err }},
		{"billing: load invoice_items", func() error { _, err := dead.LoadInvoiceItems(ctx, org); return err }},
		{"billing: load payments", func() error { _, err := dead.LoadPayments(ctx, org); return err }},
		{"billing: load refunds", func() error { _, err := dead.LoadRefunds(ctx, org); return err }},
		{"billing: load ledger_events", func() error { _, err := dead.LoadLedgerEvents(ctx, org); return err }},
		{"billing: load household_autopay_profiles", func() error { _, err := dead.LoadHouseholdAutopayProfiles(ctx, org); return err }},
		{"billing: load invoices", func() error { _, _, _, _, _, _, err := dead.LoadAll(ctx, org); return err }},
	}
	for _, l := range loaders {
		if err := l.call(); err == nil || !strings.HasPrefix(err.Error(), l.prefix) {
			t.Fatalf("want error starting %q, got %v", l.prefix, err)
		}
	}

	// LoadAll stops at, and reports, the first table that cannot be read. Break
	// one column the loader selects, from the last table to the first, inside
	// this private schema (dropping a table would fall through to the shared
	// public one on the search path).
	stages := []struct{ table, column, what string }{
		{"household_autopay_profiles", "payment_method_label", "household_autopay_profiles"},
		{"ledger_events", "description", "ledger_events"},
		{"refunds", "reason", "refunds"},
		{"payments", "notes", "payments"},
		{"invoice_items", "category", "invoice_items"},
		{"invoices", "notes", "invoices"},
	}
	r := newRepository(pool)
	for _, st := range stages {
		cov3xExec(t, pool, "ALTER TABLE "+st.table+" RENAME COLUMN "+st.column+" TO "+st.column+"_broken")
		_, _, _, _, _, _, err := r.LoadAll(ctx, org)
		if err == nil || !strings.Contains(err.Error(), "billing: load "+st.what) {
			t.Fatalf("LoadAll with %s.%s missing: %v", st.table, st.column, err)
		}
	}
}

func TestCov3xRepositoryCorruptRowsFailTheLoad(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	r := newRepository(pool)
	org, hh := uuid.NewString(), uuid.NewString()
	cov3xExec(t, pool, `INSERT INTO households (id, org_id, household_name) VALUES ('`+hh+`', '`+org+`', 'Corrupt HH')`)
	now := cov3xTime("2026-10-01T10:00:00Z")
	inv := &Invoice{ID: uuid.NewString(), OrgID: org, HouseholdID: hh, InvoiceNumber: "CORRUPT-1", Status: "draft", CreatedAt: now, UpdatedAt: now}
	if err := r.UpsertInvoice(ctx, inv); err != nil {
		t.Fatal(err)
	}

	// A legacy row the current schema would reject: the load must fail loudly
	// instead of returning a truncated list.
	cov3xExec(t, pool, `ALTER TABLE invoice_items ALTER COLUMN description DROP NOT NULL`)
	cov3xExec(t, pool, `INSERT INTO invoice_items (id, org_id, invoice_id, description, quantity, unit_amount_cents, total_cents)
		VALUES (gen_random_uuid(), '`+org+`', '`+inv.ID+`', NULL, 1, 100, 100)`)
	if _, err := r.LoadInvoiceItems(ctx, org); err == nil || !strings.Contains(err.Error(), "billing: scan invoice_item") {
		t.Fatalf("corrupt item row: %v", err)
	}

	cov3xExec(t, pool, `ALTER TABLE household_autopay_profiles ALTER COLUMN payment_method_label DROP NOT NULL`)
	cov3xExec(t, pool, `INSERT INTO household_autopay_profiles (id, org_id, household_id, payment_method_label)
		VALUES (gen_random_uuid(), '`+org+`', '`+hh+`', NULL)`)
	if _, err := r.LoadHouseholdAutopayProfiles(ctx, org); err == nil || !strings.Contains(err.Error(), "billing: scan household_autopay_profile") {
		t.Fatalf("corrupt autopay row: %v", err)
	}
}
