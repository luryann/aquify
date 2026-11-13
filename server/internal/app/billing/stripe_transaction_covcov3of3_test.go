package billing

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov3xStripeTransactionLockedAndResultPostgres(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	cov3xExec(t, pool, `INSERT INTO households (id, org_id, household_name) VALUES ('`+hh+`', '`+org+`', 'Tx HH')`)
	p := cov3xAdmin(org)
	svc := NewServiceWithPool(pool)
	inv := cov3xInvoice(t, svc, org, hh, "STX-1", "", 10000)

	paymentRows := func() int {
		return cov3xScalar[int](t, pool, `SELECT count(*) FROM payments WHERE invoice_id = $1`, inv.ID)
	}
	run := func(orgID, invoiceID string, mutate func(*Service) error) error {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return svc.stripeTransactionLocked(orgID, invoiceID, mutate)
	}

	// Unknown invoices (well-formed ids) and another club's invoices are "not found"
	// and never reach the mutation.
	called := false
	for name, ids := range map[string][2]string{
		"unknown invoice": {org, uuid.NewString()},
		"foreign club":    {uuid.NewString(), inv.ID},
	} {
		if err := run(ids[0], ids[1], func(*Service) error { called = true; return nil }); !errors.Is(err, ErrInvoiceNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if called {
		t.Fatal("mutate must not run for an invoice that cannot be locked")
	}

	// Success: the mutation commits and the live cache is refreshed from the database.
	svc.mu.Lock()
	pay, ev, err := stripeTransactionResult(svc, org, inv.ID, func(staged *Service) (*Payment, *LedgerEvent, error) {
		return staged.RecordPayment(org, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 4000, Method: "check"})
	})
	cached := len(svc.payments[inv.ID])
	svc.mu.Unlock()
	if err != nil || pay == nil || ev == nil || pay.AmountCents != 4000 || ev.EventType != "payment_succeeded" {
		t.Fatalf("result: %+v %+v %v", pay, ev, err)
	}
	if paymentRows() != 1 || cached != 1 {
		t.Fatalf("committed rows=%d cached=%d, want 1 and 1", paymentRows(), cached)
	}

	// A failing mutation rolls everything back and returns zero values.
	boom := errors.New("boom after write")
	svc.mu.Lock()
	gotPay, gotEv, err := stripeTransactionResult(svc, org, inv.ID, func(staged *Service) (*Payment, *LedgerEvent, error) {
		if _, _, err := staged.RecordPayment(org, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 1000, Method: "cash"}); err != nil {
			t.Errorf("staged write: %v", err)
		}
		return &Payment{ID: "must-not-escape"}, &LedgerEvent{ID: "must-not-escape"}, boom
	})
	svc.mu.Unlock()
	if !errors.Is(err, boom) || gotPay != nil || gotEv != nil {
		t.Fatalf("failure result: %+v %+v %v", gotPay, gotEv, err)
	}
	if paymentRows() != 1 || cov3xScalar[int](t, pool, `SELECT amount_paid_cents FROM invoices WHERE id = $1`, inv.ID) != 4000 {
		t.Fatal("the failed mutation's writes must be rolled back")
	}

	// The staged reload failing (a table the loader needs is unreadable) aborts
	// the transaction before the mutation runs.
	cov3xExec(t, pool, `ALTER TABLE refunds RENAME COLUMN reason TO reason_broken`)
	called = false
	err = run(org, inv.ID, func(*Service) error { called = true; return nil })
	if err == nil || called || !strings.Contains(err.Error(), `column "reason"`) {
		t.Fatalf("reload failure: called=%v err=%v", called, err)
	}

	// The invoice lock query failing is a database error, not "not found".
	cov3xExec(t, pool, `ALTER TABLE invoices RENAME COLUMN org_id TO org_id_broken`)
	err = run(org, inv.ID, func(*Service) error { called = true; return nil })
	if err == nil || errors.Is(err, ErrInvoiceNotFound) || called {
		t.Fatalf("lock query failure must surface as a database error: called=%v err=%v", called, err)
	}
}

// Webhook metadata is attacker-controlled text: malformed identifiers must be
// reported as "not found" before any database access (so Stripe is not told to
// redeliver forever).
func TestCov3xStripeTransactionRejectsMalformedIDsBeforeTouchingTheDatabase(t *testing.T) {
	svc := NewService() // no repository: reaching the database would panic
	valid := uuid.NewString()
	cases := map[string][2]string{
		"bad invoice id": {valid, "not-a-uuid"},
		"bad org id":     {"org-demo", valid},
		"empty ids":      {"", ""},
	}
	for name, ids := range cases {
		called := false
		svc.mu.Lock()
		err := svc.stripeTransactionLocked(ids[0], ids[1], func(*Service) error { called = true; return nil })
		svc.mu.Unlock()
		if !errors.Is(err, ErrInvoiceNotFound) || called {
			t.Fatalf("%s: err=%v called=%v", name, err, called)
		}
	}
}
