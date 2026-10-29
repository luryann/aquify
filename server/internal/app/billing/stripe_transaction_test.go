package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests use a private schema and the real migrations. They never reset
// existing tables and require an explicitly configured integration database.
func stripeTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("BILLING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("BILLING_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "stripe_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); base.Close() })
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	paths, err := filepath.Glob("../../../../db/migrations/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := strings.SplitN(string(data), "-- +goose Up", 2)[1]
		sql = strings.SplitN(sql, "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	return pool
}

func TestStripeTransactionsRollbackAndRefreshAcrossInstances(t *testing.T) {
	pool := stripeTestPool(t)
	ctx := context.Background()
	org, household, invoice := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id,org_id,household_name) VALUES ($1,$2,'Stripe test')`, household, org); err != nil {
		t.Fatal(err)
	}
	// Both instances start before the invoice exists.
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)
	inv := &Invoice{ID: invoice, OrgID: org, HouseholdID: household, InvoiceNumber: "STRIPE", Status: "issued", TotalCents: 10000, SubtotalCents: 10000, AmountDueCents: 10000}
	if err := pool.QueryRow(ctx, `INSERT INTO invoices (id,org_id,household_id,invoice_number,status,total_cents,subtotal_cents,amount_due_cents) VALUES ($1,$2,$3,'STRIPE','issued',10000,10000,10000) RETURNING created_at,updated_at`, invoice, org, household).Scan(&inv.CreatedAt, &inv.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	// A failing credit write occurs after payment, invoice and ledger writes.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_stripe_credit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected credit failure'; END $$; CREATE TRIGGER reject_stripe_credit BEFORE INSERT ON household_credit_entries FOR EACH ROW EXECUTE FUNCTION reject_stripe_credit()`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.HandleStripePaymentIntentSucceeded(org, "pi_atomic", invoice, 12000, "stripe-webhook"); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, table := range []string{"payments", "ledger_events", "household_credit_entries", "household_credit_balances"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback left %d rows in %s", count, table)
		}
	}
	var paid int
	if err := pool.QueryRow(ctx, "SELECT amount_paid_cents FROM invoices WHERE id=$1", invoice).Scan(&paid); err != nil || paid != 0 {
		t.Fatalf("invoice changed: %d %v", paid, err)
	}
	if len(a.stripeIntentIndex) != 0 {
		t.Fatal("uncommitted payment leaked into cache")
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER reject_stripe_credit ON household_credit_entries"); err != nil {
		t.Fatal(err)
	}
	// Different event IDs for one PaymentIntent must still record it once.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, svc := range []*Service{a, b} {
		wg.Add(1)
		go func(s *Service) {
			defer wg.Done()
			_, _, err := s.HandleStripePaymentIntentSucceeded(org, "pi_atomic", invoice, 12000, "stripe-webhook")
			errs <- err
		}(svc)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count, balance int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM payments").Scan(&count); err != nil || count != 1 {
		t.Fatalf("payments=%d err=%v", count, err)
	}
	if err := pool.QueryRow(ctx, "SELECT balance_cents FROM household_credit_balances").Scan(&balance); err != nil || balance != 2000 {
		t.Fatalf("credit=%d err=%v", balance, err)
	}
	// Fail a refund ledger write after its other records have been updated.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_stripe_refund() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type = 'refund_processed' THEN RAISE EXCEPTION 'injected refund failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_stripe_refund BEFORE INSERT ON ledger_events FOR EACH ROW EXECUTE FUNCTION reject_stripe_refund()`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.HandleStripeChargeRefunded(org, "pi_atomic", invoice, 12000, "re_atomic", "", "stripe-webhook"); err == nil {
		t.Fatal("expected refund failure")
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM refunds").Scan(&count); err != nil || count != 0 {
		t.Fatalf("refund not rolled back: %d %v", count, err)
	}
	if err := pool.QueryRow(ctx, "SELECT balance_cents FROM household_credit_balances").Scan(&balance); err != nil || balance != 2000 {
		t.Fatalf("credit reversal not rolled back: %d %v", balance, err)
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER reject_stripe_refund ON ledger_events"); err != nil {
		t.Fatal(err)
	}
	for _, total := range []int{2000, 5000, 12000, 12000} {
		if _, _, err := b.HandleStripeChargeRefunded(org, "pi_atomic", invoice, total, fmt.Sprintf("re_%d", total), "", "stripe-webhook"); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT sum(amount_cents) FROM refunds").Scan(&paid); err != nil || paid != 10000 {
		t.Fatalf("refunded=%d err=%v", paid, err)
	}
	if err := pool.QueryRow(ctx, "SELECT balance_cents FROM household_credit_balances").Scan(&balance); err != nil || balance != 0 {
		t.Fatalf("credit=%d err=%v", balance, err)
	}
}

func TestStripeWebhookDatabaseLeaseRequiresRetry(t *testing.T) {
	pool := stripeTestPool(t)
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)
	if claimed, err := a.RecordWebhookEvent("stripe", "", "evt_lease", "test", nil); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if _, err := b.RecordWebhookEvent("stripe", "", "evt_lease", "test", nil); err != ErrWebhookInFlight {
		t.Fatalf("expected active lease, got %v", err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE webhook_events SET processing_until=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if claimed, err := b.RecordWebhookEvent("stripe", "", "evt_lease", "test", nil); err != nil || !claimed {
		t.Fatalf("expired claim: %v %v", claimed, err)
	}
	if err := b.MarkWebhookProcessed("stripe", "evt_lease", nil, false); err != nil {
		t.Fatal(err)
	}
	if claimed, err := a.RecordWebhookEvent("stripe", "", "evt_lease", "test", nil); err != nil || claimed {
		t.Fatalf("completed event: %v %v", claimed, err)
	}
}

func TestStripeWebhookMalformedIDsAreNotFoundNotDatabaseErrors(t *testing.T) {
	pool := stripeTestPool(t)
	svc := NewServiceWithPool(pool)
	for _, tc := range []struct{ org, invoice string }{
		{uuid.NewString(), "inv-load-1"},
		{"org-demo", uuid.NewString()},
	} {
		_, _, err := svc.HandleStripePaymentIntentSucceeded(tc.org, "pi_malformed", tc.invoice, 100, "stripe-webhook")
		if !errors.Is(err, ErrInvoiceNotFound) {
			t.Errorf("org %q invoice %q: err = %v, want ErrInvoiceNotFound", tc.org, tc.invoice, err)
		}
	}
}

// A second API instance must see fee schedules and discount rules created on
// the first without a restart, and be able to archive them.
func TestFinanceConfigVisibleAcrossInstances(t *testing.T) {
	pool := stripeTestPool(t)
	org := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: "admin-x", OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)

	sc, err := a.CreateFeeSchedule(org, admin.UserID, admin, FeeScheduleInput{
		Name: "Dues", Kind: ScheduleKindDues, GroupCode: "SENIOR", Frequency: FrequencyMonthly, AmountCents: 5000, StartDate: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	rule, err := a.CreateDiscountRule(org, admin.UserID, admin, DiscountRuleInput{
		Name: "Sibling", AppliesTo: "dues", MinPosition: 2, DiscountType: DiscountTypePercent, PercentBps: 1000,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	if _, err := b.GetFeeSchedule(org, admin, sc.ID); err != nil {
		t.Fatalf("second instance get schedule: %v", err)
	}
	if _, err := b.ArchiveFeeSchedule(org, admin.UserID, admin, sc.ID); err != nil {
		t.Fatalf("second instance archive schedule: %v", err)
	}
	if _, err := b.ArchiveDiscountRule(org, admin.UserID, admin, rule.ID); err != nil {
		t.Fatalf("second instance archive rule: %v", err)
	}
	got, err := a.GetFeeSchedule(org, admin, sc.ID)
	if err != nil || got.Active {
		t.Fatalf("first instance should see the archived schedule, got %+v err %v", got, err)
	}
}

// Invoice numbers are arbitrated by the DB unique constraint, so an instance
// whose in-memory set is stale must still get ErrDuplicateInvoiceNumber.
func TestCreateInvoiceDuplicateNumberAcrossInstances(t *testing.T) {
	pool := stripeTestPool(t)
	org := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)
	in := CreateInvoiceInput{InvoiceNumber: "DUP-1"}
	if _, err := a.CreateInvoice(org, admin.UserID, admin, in); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateInvoice(org, admin.UserID, admin, in); !errors.Is(err, ErrDuplicateInvoiceNumber) {
		t.Fatalf("second instance: want ErrDuplicateInvoiceNumber, got %v", err)
	}
}
