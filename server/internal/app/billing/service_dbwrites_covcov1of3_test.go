package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func (e *cov1xEnv) invoiceStatus(t *testing.T, invoiceID string) string {
	t.Helper()
	var status string
	if err := e.pool.QueryRow(context.Background(), `SELECT status FROM invoices WHERE id = $1::uuid`, invoiceID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (e *cov1xEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func cov1xDBInvoiceWritesRollBackOnFailure(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin

	t.Run("create invoice", func(t *testing.T) {
		release := e.inject(t, "invoices", "INSERT OR UPDATE", "")
		_, err := svc.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: "FAIL-CREATE"})
		release()
		cov1xWantErr(t, "create", err, "persist invoice")
		if n := e.count(t, `SELECT count(*) FROM invoices WHERE org_id = $1::uuid AND invoice_number = 'FAIL-CREATE'`, e.org); n != 0 {
			t.Fatalf("failed create left %d rows", n)
		}
		// The failed number is not burned: it can be used once the database recovers.
		if _, err := svc.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: "FAIL-CREATE"}); err != nil {
			t.Fatalf("number must remain usable: %v", err)
		}
	})

	t.Run("duplicate number decided by the database", func(t *testing.T) {
		stale := NewServiceWithPool(e.pool)
		fired := false
		stale.SetSyncHookForTest(func() {
			if fired {
				return
			}
			fired = true
			// Another instance takes the number after this one read its snapshot.
			if _, err := svc.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: "RACE-1"}); err != nil {
				t.Errorf("competing create: %v", err)
			}
		})
		_, err := stale.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: "RACE-1"})
		if !errors.Is(err, ErrDuplicateInvoiceNumber) {
			t.Fatalf("unique constraint must map to ErrDuplicateInvoiceNumber, got %v", err)
		}
		if n := e.count(t, `SELECT count(*) FROM invoices WHERE org_id = $1::uuid AND invoice_number = 'RACE-1'`, e.org); n != 1 {
			t.Fatalf("exactly one RACE-1 invoice expected, got %d", n)
		}
		stale.SetSyncHookForTest(nil)
		if _, err := stale.CreateInvoice(org, "a", admin, CreateInvoiceInput{HouseholdID: e.hh, InvoiceNumber: "RACE-1"}); !errors.Is(err, ErrDuplicateInvoiceNumber) {
			t.Fatalf("the losing instance must remember the number: %v", err)
		}
	})

	t.Run("add item", func(t *testing.T) {
		inv := e.draft(t, "ITEM-ADD", 0)
		item := AddInvoiceItemInput{Description: "Dues", Quantity: 2, UnitAmountCents: 500, Category: "manual"}

		release := e.inject(t, "invoice_items", "INSERT OR UPDATE", "")
		_, err := svc.AddInvoiceItem(org, "a", admin, inv.ID, item)
		release()
		cov1xWantErr(t, "item insert", err, "persist invoice item")

		release = e.inject(t, "invoices", "INSERT OR UPDATE", "")
		_, err = svc.AddInvoiceItem(org, "a", admin, inv.ID, item)
		release()
		cov1xWantErr(t, "invoice update after item", err, "persist invoice after item add")
		if n := e.count(t, `SELECT count(*) FROM invoice_items WHERE invoice_id = $1::uuid`, inv.ID); n != 0 {
			t.Fatalf("item must be rolled back with the invoice update: %d", n)
		}
		if items, _ := svc.ListInvoiceItems(org, admin, inv.ID); len(items) != 0 {
			t.Fatalf("cache must not keep the failed item: %+v", items)
		}
	})

	t.Run("remove item", func(t *testing.T) {
		inv := e.draft(t, "ITEM-RM", 900)
		items, err := svc.ListInvoiceItems(org, admin, inv.ID)
		if err != nil || len(items) != 1 {
			t.Fatalf("setup: %+v %v", items, err)
		}
		itemID := items[0].ID

		release := e.inject(t, "invoice_items", "DELETE", "")
		err = svc.RemoveInvoiceItem(org, "a", admin, inv.ID, itemID)
		release()
		cov1xWantErr(t, "delete", err, "delete invoice item")

		release = e.inject(t, "invoices", "INSERT OR UPDATE", "")
		err = svc.RemoveInvoiceItem(org, "a", admin, inv.ID, itemID)
		release()
		cov1xWantErr(t, "invoice update after removal", err, "persist invoice after item remove")
		if n := e.count(t, `SELECT count(*) FROM invoice_items WHERE invoice_id = $1::uuid`, inv.ID); n != 1 {
			t.Fatalf("a failed removal must keep the item: %d", n)
		}

		if err := svc.RemoveInvoiceItem(org, "a", admin, inv.ID, itemID); err != nil {
			t.Fatal(err)
		}
		if got, _ := svc.GetInvoice(org, admin, inv.ID); got.TotalCents != 0 {
			t.Fatalf("totals after removal: %+v", got)
		}
	})

	t.Run("issue", func(t *testing.T) {
		inv := e.draft(t, "ISSUE-FAIL", 1000)
		release := e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status = 'issued'")
		_, _, err := svc.IssueInvoice(org, "a", admin, inv.ID)
		release()
		cov1xWantErr(t, "issue persist", err, "persist issued invoice")

		release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'invoice_issued'")
		_, _, err = svc.IssueInvoice(org, "a", admin, inv.ID)
		release()
		cov1xWantErr(t, "issue ledger", err, "persist ledger event")
		if got := e.invoiceStatus(t, inv.ID); got != "draft" {
			t.Fatalf("failed issue must roll back, status %q", got)
		}
	})

	t.Run("issue with a failing credit application", func(t *testing.T) {
		if _, _, err := svc.AddManualCredit(org, "a", admin, e.hh, ManualCreditInput{AmountCents: 300, Reason: "goodwill"}); err != nil {
			t.Fatal(err)
		}
		inv := e.draft(t, "ISSUE-CREDIT", 1000)
		release := e.inject(t, "payments", "INSERT", "NEW.payment_method = 'account_credit'")
		_, _, err := svc.IssueInvoice(org, "a", admin, inv.ID)
		release()
		if err == nil {
			t.Fatal("the transaction is poisoned by the failed credit write and must not commit")
		}
		if got := e.invoiceStatus(t, inv.ID); got != "draft" {
			t.Fatalf("status %q", got)
		}
		// Without the fault the credit is applied automatically on issue.
		issued, _, err := svc.IssueInvoice(org, "a", admin, inv.ID)
		if err != nil || issued.AmountPaidCents != 300 || issued.AmountDueCents != 700 {
			t.Fatalf("credit auto-apply: %+v %v", issued, err)
		}
	})

	t.Run("void", func(t *testing.T) {
		inv := e.issued(t, "VOID-FAIL", 1000)
		release := e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status = 'voided'")
		_, _, err := svc.VoidInvoice(org, "a", admin, inv.ID)
		release()
		cov1xWantErr(t, "void persist", err, "persist voided invoice")

		release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'invoice_voided'")
		_, _, err = svc.VoidInvoice(org, "a", admin, inv.ID)
		release()
		cov1xWantErr(t, "void ledger", err, "persist ledger event")
		if got := e.invoiceStatus(t, inv.ID); got != "issued" {
			t.Fatalf("failed void must roll back, status %q", got)
		}
		voided, ev, err := svc.VoidInvoice(org, "a", admin, inv.ID)
		if err != nil || voided.Status != "voided" || ev == nil || ev.EventType != "invoice_voided" {
			t.Fatalf("void: %+v %+v %v", voided, ev, err)
		}
	})

	t.Run("write off", func(t *testing.T) {
		inv := e.issued(t, "WO-FAIL", 1000)
		release := e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status = 'written_off'")
		_, _, err := svc.WriteOffInvoice(org, "a", admin, inv.ID)
		release()
		cov1xWantErr(t, "write-off persist", err, "persist written-off invoice")

		release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'invoice_written_off'")
		_, _, err = svc.WriteOffInvoice(org, "a", admin, inv.ID)
		release()
		cov1xWantErr(t, "write-off ledger", err, "persist ledger event")
		if got := e.invoiceStatus(t, inv.ID); got != "issued" {
			t.Fatalf("failed write-off must roll back, status %q", got)
		}
		if got, _, err := svc.WriteOffInvoice(org, "a", admin, inv.ID); err != nil || got.Status != "written_off" {
			t.Fatalf("write-off: %+v %v", got, err)
		}
	})

	t.Run("record payment", func(t *testing.T) {
		inv := e.issued(t, "PAY-FAIL", 1000)
		in := RecordPaymentInput{AmountCents: 400, Method: "check"}

		release := e.inject(t, "payments", "INSERT OR UPDATE", "")
		_, _, err := svc.RecordPayment(org, "a", admin, inv.ID, in)
		release()
		cov1xWantErr(t, "payment insert", err, "persist payment")

		release = e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status = 'partially_paid'")
		_, _, err = svc.RecordPayment(org, "a", admin, inv.ID, in)
		release()
		cov1xWantErr(t, "invoice after payment", err, "persist invoice after payment")

		release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'payment_succeeded'")
		_, _, err = svc.RecordPayment(org, "a", admin, inv.ID, in)
		release()
		cov1xWantErr(t, "payment ledger", err, "persist ledger event")

		release = e.inject(t, "household_credit_entries", "INSERT", "NEW.kind = 'overpayment'")
		_, _, err = svc.RecordPayment(org, "a", admin, inv.ID, RecordPaymentInput{AmountCents: 1500, Method: "check", ExcessToCredit: true})
		release()
		cov1xWantErr(t, "overpayment credit", err, "")

		if n := e.count(t, `SELECT count(*) FROM payments WHERE invoice_id = $1::uuid`, inv.ID); n != 0 {
			t.Fatalf("every failed payment must roll back, found %d", n)
		}
		if got, _ := svc.GetInvoice(org, admin, inv.ID); got.AmountPaidCents != 0 || got.Status != "issued" {
			t.Fatalf("invoice must be untouched: %+v", got)
		}
	})
}

func cov1xDBAutopayWriteFailures(t *testing.T, e *cov1xEnv) {
	in := UpsertHouseholdAutopayInput{PaymentMethodType: "card", PaymentMethodLabel: "Visa 4242"}
	release := e.inject(t, "household_autopay_profiles", "INSERT OR UPDATE", "")
	_, err := e.svc.UpsertHouseholdAutopay(e.org, "a", e.admin, e.hh, in)
	cov1xWantErr(t, "upsert", err, "persist autopay profile")
	_, err = e.svc.OptOutHouseholdAutopay(e.org, "a", e.admin, e.hh)
	cov1xWantErr(t, "opt out", err, "persist autopay opt-out")
	release()
	if st, err := e.svc.GetHouseholdAutopayStatus(e.org, e.admin, e.hh); err != nil || st.Enabled || st.PaymentMethodLabel != "" {
		t.Fatalf("a failed write must not be cached: %+v %v", st, err)
	}
	if _, err := e.svc.UpsertHouseholdAutopay(e.org, "a", e.admin, e.hh, in); err != nil {
		t.Fatal(err)
	}
	release = e.inject(t, "household_autopay_profiles", "INSERT OR UPDATE", "")
	_, err = e.svc.OptOutHouseholdAutopay(e.org, "a", e.admin, e.hh)
	release()
	cov1xWantErr(t, "opt out existing", err, "persist autopay opt-out")
	if st, _ := e.svc.GetHouseholdAutopayStatus(e.org, e.admin, e.hh); !st.Enabled {
		t.Fatal("a failed opt-out must leave autopay enabled")
	}
}

func cov1xDBLedgerWriteFailureIsReported(t *testing.T, e *cov1xEnv) {
	release := e.inject(t, "ledger_events", "INSERT", "")
	e.svc.mu.Lock()
	_, err := e.svc.appendLedger(e.org, "", "", "", "manual_event", 1, "d", "actor")
	n := len(e.svc.ledger)
	e.svc.mu.Unlock()
	release()
	cov1xWantErr(t, "append", err, "persist ledger event")
	if n != 0 {
		t.Fatalf("failed ledger write must not be cached: %d", n)
	}
}

func cov1xDBRefundWriteFailures(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin
	setup := func(number string) (*Invoice, *Payment) {
		inv := e.issued(t, number, 4000)
		return inv, e.pay(t, inv.ID, 4000)
	}
	part := func(p *Payment) CreateRefundInput {
		return CreateRefundInput{PaymentID: p.ID, AmountCents: 1000, Reason: "r"}
	}

	inv, pay := setup("RF-1")
	release := e.inject(t, "refunds", "INSERT OR UPDATE", "")
	_, _, err := svc.CreateRefund(org, "a", admin, inv.ID, part(pay))
	release()
	cov1xWantErr(t, "refund insert", err, "persist refund")

	release = e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status LIKE 'refunded%'")
	_, _, err = svc.CreateRefund(org, "a", admin, inv.ID, part(pay))
	release()
	cov1xWantErr(t, "invoice after refund", err, "persist invoice after refund")

	release = e.inject(t, "payments", "INSERT OR UPDATE", "NEW.status = 'reversed'")
	_, _, err = svc.CreateRefund(org, "a", admin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 4000, Reason: "r"})
	release()
	cov1xWantErr(t, "payment after refund", err, "persist payment after refund")

	release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'refund_processed'")
	_, _, err = svc.CreateRefund(org, "a", admin, inv.ID, part(pay))
	release()
	cov1xWantErr(t, "refund ledger", err, "persist ledger event")

	release = e.inject(t, "household_credit_entries", "INSERT", "NEW.kind = 'refund_to_credit'")
	_, _, err = svc.CreateRefund(org, "a", admin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 1000, Reason: "r", ToAccountCredit: true})
	release()
	cov1xWantErr(t, "refund to credit", err, "record refund-to-credit")

	if n := e.count(t, `SELECT count(*) FROM refunds WHERE invoice_id = $1::uuid`, inv.ID); n != 0 {
		t.Fatalf("all failed refunds must roll back, found %d", n)
	}
	if got, _ := svc.GetInvoice(org, admin, inv.ID); got.Status != "paid" || got.AmountPaidCents != 4000 {
		t.Fatalf("invoice must be unchanged: %+v", got)
	}
	// And the refund works once the faults are gone.
	if r, _, err := svc.CreateRefund(org, "a", admin, inv.ID, part(pay)); err != nil || r.AmountCents != 1000 {
		t.Fatalf("refund: %+v %v", r, err)
	}
}

func cov1xDBCreateRefundRevalidatesAgainstLockedState(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin

	// The state the preflight validated can change before the refund transaction takes its row lock;
	// the transaction re-reads canonical state and must reject the refund.
	cases := []struct {
		name    string
		mutate  func(inv *Invoice, pay *Payment, other *Invoice)
		wantErr error
	}{
		{"invoice removed", func(inv *Invoice, _ *Payment, _ *Invoice) {
			e.mustExec(t, `UPDATE invoices SET deleted_at = now() WHERE id = $1::uuid`, inv.ID)
		}, ErrInvoiceNotFound},
		{"invoice no longer refundable", func(inv *Invoice, _ *Payment, _ *Invoice) {
			e.mustExec(t, `UPDATE invoices SET status = 'voided' WHERE id = $1::uuid`, inv.ID)
		}, ErrInvalidTransition},
		{"payment moved to another invoice", func(_ *Invoice, pay *Payment, other *Invoice) {
			e.mustExec(t, `UPDATE payments SET invoice_id = $2::uuid WHERE id = $1::uuid`, pay.ID, other.ID)
		}, ErrPaymentNotFound},
		{"payment no longer succeeded", func(_ *Invoice, pay *Payment, _ *Invoice) {
			e.mustExec(t, `UPDATE payments SET status = 'failed' WHERE id = $1::uuid`, pay.ID)
		}, ErrInvalidTransition},
		{"payment smaller than the refund", func(_ *Invoice, pay *Payment, _ *Invoice) {
			e.mustExec(t, `UPDATE payments SET amount_cents = 100 WHERE id = $1::uuid`, pay.ID)
		}, ErrRefundExceedsPayment},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv := e.issued(t, "REVAL-"+string(rune('A'+i)), 4000)
			other := e.issued(t, "REVAL-OTHER-"+string(rune('A'+i)), 100)
			pay := e.pay(t, inv.ID, 4000)
			// A direct write makes this instance's next read reload; the hook then changes the
			// database after that read and before the refund transaction starts.
			e.mustExec(t, `UPDATE payments SET notes = 'touched' WHERE id = $1::uuid`, pay.ID)
			fired := false
			svc.SetSyncHookForTest(func() {
				if !fired {
					fired = true
					tc.mutate(inv, pay, other)
				}
			})
			defer svc.SetSyncHookForTest(nil)
			_, _, err := svc.CreateRefund(org, "a", admin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 1000, Reason: "r"})
			if !fired {
				t.Fatal("the preflight read never reloaded, the race was not exercised")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
			if n := e.count(t, `SELECT count(*) FROM refunds WHERE invoice_id = $1::uuid`, inv.ID); n != 0 {
				t.Fatalf("rejected refund must not be stored: %d", n)
			}
		})
	}
}

func cov1xDBStripeRefundIDAdoption(t *testing.T, e *cov1xEnv) {
	svc, org, admin := e.svc, e.org, e.admin
	inv := e.issued(t, "ADOPT-1", 3000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_adopt", inv.ID, 3000, "wh"); err != nil {
		t.Fatal(err)
	}
	synth, _, err := svc.HandleStripeChargeRefunded(org, "pi_adopt", inv.ID, 1000, "", "", "wh")
	if err != nil || synth.StripeRefundID != "pi_adopt-cumulative-1000" {
		t.Fatalf("webhook refund: %+v %v", synth, err)
	}
	pay, err := svc.ListPayments(org, admin, inv.ID)
	if err != nil || len(pay) != 1 {
		t.Fatalf("payments: %+v %v", pay, err)
	}
	in := CreateRefundInput{PaymentID: pay[0].ID, AmountCents: 1000, Reason: "customer", StripeRefundID: "re_real"}

	release := e.inject(t, "refunds", "UPDATE", "NEW.stripe_refund_id = 're_real'")
	_, _, err = svc.CreateRefund(org, "a", admin, inv.ID, in)
	release()
	cov1xWantErr(t, "adopt", err, "persist adopted refund id")

	adopted, ev, err := svc.CreateRefund(org, "a", admin, inv.ID, in)
	if err != nil || ev != nil || adopted.ID != synth.ID || adopted.StripeRefundID != "re_real" {
		t.Fatalf("adoption: %+v %+v %v", adopted, ev, err)
	}
	if n := e.count(t, `SELECT count(*) FROM refunds WHERE invoice_id = $1::uuid`, inv.ID); n != 1 {
		t.Fatalf("adoption must not create a second refund: %d", n)
	}
	// Replaying the same provider refund returns the stored record.
	replay, ev, err := svc.CreateRefund(org, "a", admin, inv.ID, in)
	if err != nil || ev != nil || replay.ID != synth.ID {
		t.Fatalf("replay: %+v %v", replay, err)
	}
}

func cov1xDBStripeSuccessWriteFailures(t *testing.T, e *cov1xEnv) {
	svc, org := e.svc, e.org

	inv := e.issued(t, "ST-1", 1000)
	release := e.inject(t, "payments", "INSERT OR UPDATE", "")
	_, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_1", inv.ID, 1000, "wh")
	release()
	cov1xWantErr(t, "stripe payment", err, "persist stripe payment")

	release = e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status = 'paid'")
	_, _, err = svc.HandleStripePaymentIntentSucceeded(org, "pi_1", inv.ID, 1000, "wh")
	release()
	cov1xWantErr(t, "invoice after stripe payment", err, "persist invoice after stripe payment")

	release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'payment_succeeded'")
	_, _, err = svc.HandleStripePaymentIntentSucceeded(org, "pi_1", inv.ID, 1000, "wh")
	release()
	cov1xWantErr(t, "stripe ledger", err, "persist ledger event")

	release = e.inject(t, "household_credit_entries", "INSERT", "")
	_, _, err = svc.HandleStripePaymentIntentSucceeded(org, "pi_1", inv.ID, 1500, "wh")
	cov1xWantErr(t, "excess credit", err, "record stripe credit")
	paid := e.issued(t, "ST-PAID", 500)
	e.pay(t, paid.ID, 500)
	_, _, err = svc.HandleStripePaymentIntentSucceeded(org, "pi_2", paid.ID, 700, "wh")
	cov1xWantErr(t, "credit-only", err, "record stripe credit")
	release()

	if n := e.count(t, `SELECT count(*) FROM payments WHERE org_id = $1::uuid AND stripe_payment_intent_id IN ('pi_1','pi_2')`, e.org); n != 0 {
		t.Fatalf("failed stripe handling must leave no payment, found %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM household_credit_entries WHERE org_id = $1::uuid`, e.org); n != 0 {
		t.Fatalf("no credit may be recorded: %d", n)
	}
	// A clean redelivery now succeeds exactly once.
	pay, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_1", inv.ID, 1500, "wh")
	if err != nil || pay.AmountCents != 1000 {
		t.Fatalf("redelivery: %+v %v", pay, err)
	}
}

func cov1xDBStripeChargeRefundedWriteFailures(t *testing.T, e *cov1xEnv) {
	svc, org := e.svc, e.org

	inv := e.issued(t, "CR-1", 4000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_cr", inv.ID, 4000, "wh"); err != nil {
		t.Fatal(err)
	}
	refund := func(amount int) error {
		_, _, err := svc.HandleStripeChargeRefunded(org, "pi_cr", inv.ID, amount, "re_x", "", "wh")
		return err
	}
	release := e.inject(t, "refunds", "INSERT OR UPDATE", "")
	cov1xWantErr(t, "refund insert", refund(1000), "persist stripe refund")
	release()
	release = e.inject(t, "payments", "INSERT OR UPDATE", "NEW.status = 'reversed'")
	cov1xWantErr(t, "payment update", refund(4000), "persist payment after stripe refund")
	release()
	release = e.inject(t, "invoices", "INSERT OR UPDATE", "NEW.status LIKE 'refunded%'")
	cov1xWantErr(t, "invoice update", refund(1000), "persist invoice after stripe refund")
	release()
	release = e.inject(t, "ledger_events", "INSERT", "NEW.event_type = 'refund_processed'")
	cov1xWantErr(t, "ledger", refund(1000), "persist ledger event")
	release()
	if n := e.count(t, `SELECT count(*) FROM refunds WHERE invoice_id = $1::uuid`, inv.ID); n != 0 {
		t.Fatalf("failed webhook refunds must roll back: %d", n)
	}

	// Reversing credit created from a captured payment fails loudly too.
	paid := e.issued(t, "CR-PAID", 500)
	e.pay(t, paid.ID, 500)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_credit", paid.ID, 800, "wh"); err != nil {
		t.Fatal(err)
	}
	release = e.inject(t, "household_credit_entries", "INSERT", "NEW.kind = 'manual_debit'")
	_, _, err := svc.HandleStripeChargeRefunded(org, "pi_credit", paid.ID, 300, "re_c", "", "wh")
	release()
	cov1xWantErr(t, "credit reversal", err, "reverse stripe credit")
	if c, _ := svc.GetHouseholdCredit(org, e.admin, e.hh); c.BalanceCents != 800 {
		t.Fatalf("credit must be intact after the failed reversal: %+v", c)
	}
	if _, _, err := svc.HandleStripeChargeRefunded(org, "pi_credit", paid.ID, 300, "re_c", "", "wh"); err != nil {
		t.Fatal(err)
	}
	if c, _ := svc.GetHouseholdCredit(org, e.admin, e.hh); c.BalanceCents != 500 {
		t.Fatalf("credit after reversal: %+v", c)
	}
}

func cov1xDBFailedPaymentKeepsCacheAlignedWithDatabase(t *testing.T, e *cov1xEnv) {
	inv := e.issued(t, "FAILED-PI", 1000)
	pending := &Payment{
		ID: uuid.NewString(), OrgID: e.org, InvoiceID: inv.ID, AmountCents: 1000, Method: "stripe", Status: "pending",
		StripePaymentIntentID: "pi_pending",
	}
	if err := e.svc.repo.UpsertPayment(context.Background(), pending); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RefreshOrg(context.Background(), e.org); err != nil {
		t.Fatal(err)
	}

	release := e.inject(t, "payments", "INSERT OR UPDATE", "NEW.status = 'failed'")
	err := e.svc.HandleStripePaymentIntentFailedFor(e.org, "pi_pending")
	release()
	cov1xWantErr(t, "failed payment", err, "persist failed stripe payment")
	if got, err := e.svc.GetPayment(e.org, e.admin, inv.ID, pending.ID); err != nil || got.Status != "pending" || got.FailedAt != nil {
		t.Fatalf("memory must not run ahead of the database after a failed write: %+v %v", got, err)
	}
	if err := e.svc.HandleStripePaymentIntentFailedFor(e.org, "pi_pending"); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.GetPayment(e.org, e.admin, inv.ID, pending.ID); got.Status != "failed" || got.FailedAt == nil {
		t.Fatalf("payment must be failed: %+v", got)
	}
	if n := e.count(t, `SELECT count(*) FROM payments WHERE id = $1::uuid AND status = 'failed'`, pending.ID); n != 1 {
		t.Fatalf("failure must be persisted: %d", n)
	}
}

func cov1xDBWebhookEventListing(t *testing.T, e *cov1xEnv) {
	if got, err := e.svc.ListWebhookEvents("not-a-uuid", e.admin); err != nil || got != nil {
		t.Fatalf("a malformed org id matches nothing: %+v %v", got, err)
	}
	if isNew, err := e.svc.RecordWebhookEvent("stripe", e.org, "evt_db_1", "invoice.paid", []byte(`{"id":"evt_db_1"}`)); err != nil || !isNew {
		t.Fatalf("claim: %v %v", isNew, err)
	}
	if err := e.svc.MarkWebhookProcessed("stripe", "evt_db_1", nil, false); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.ListWebhookEvents(e.org, e.admin)
	if err != nil || len(got) != 1 || got[0].EventID != "evt_db_1" || !got[0].Processed {
		t.Fatalf("listing: %+v %v", got, err)
	}
	if isNew, err := e.svc.RecordWebhookEvent("stripe", e.org, "evt_db_1", "invoice.paid", nil); err != nil || isNew {
		t.Fatalf("processed events are not re-claimed: %v %v", isNew, err)
	}
}
