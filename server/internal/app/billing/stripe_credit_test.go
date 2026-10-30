package billing

import (
	"errors"
	"fmt"
	"testing"
)

func TestStripePaymentIntentSucceeded_ExcessAndUnpayableGoToCredit(t *testing.T) {
	t.Parallel()

	svc := NewService()
	p := billingAdminPrincipal()
	org := p.OrgID

	inv := issuedInvoice(t, svc, p, "hh-credit", "INV-STRIPE-CREDIT", 10000)

	// 12,000 captured against a 10,000 balance: invoice settles, 2,000 credited.
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_over", inv.ID, 12000, "stripe-webhook"); err != nil {
		t.Fatalf("overpayment: %v", err)
	}
	// Invoice is now paid; a second capture is credited in full, once.
	for i := 0; i < 2; i++ {
		if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_late", inv.ID, 3000, "stripe-webhook"); err != nil {
			t.Fatalf("late payment: %v", err)
		}
	}

	credit, err := svc.GetHouseholdCredit(org, p, "hh-credit")
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if credit.BalanceCents != 5000 {
		t.Fatalf("expected 5000 credit, got %d", credit.BalanceCents)
	}
}

func TestStripeCreditOnlyPayment_RedeliveryAndRefund(t *testing.T) {
	t.Parallel()

	svc := NewService()
	p := billingAdminPrincipal()
	org := p.OrgID

	inv := issuedInvoice(t, svc, p, "hh-refund", "INV-STRIPE-REFUND", 10000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_full", inv.ID, 10000, "stripe-webhook"); err != nil {
		t.Fatalf("settle: %v", err)
	}

	// Invoice is paid, so this capture is held wholly as credit (no Payment row).
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_credit", inv.ID, 3000, "stripe-webhook"); err != nil {
		t.Fatalf("credit-only payment: %v", err)
	}
	// Refunding the credit-only charge takes the credit back, once per refund.
	for i := 0; i < 2; i++ {
		if _, _, err := svc.HandleStripeChargeRefunded(org, "pi_credit", inv.ID, 3000, "re_1", "", "stripe-webhook"); err != nil {
			t.Fatalf("refund credit-only payment: %v", err)
		}
	}
	credit, err := svc.GetHouseholdCredit(org, p, "hh-refund")
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if credit.BalanceCents != 0 {
		t.Fatalf("expected credit reversed to 0, got %d", credit.BalanceCents)
	}
	// A refund for an unknown intent still reports ErrPaymentNotFound.
	if _, _, err := svc.HandleStripeChargeRefunded(org, "pi_none", inv.ID, 100, "re_2", "", "stripe-webhook"); err != ErrPaymentNotFound {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestRecordWebhookEvent_LeaseBlocksConcurrentDelivery(t *testing.T) {
	t.Parallel()

	svc := NewService()
	isNew, err := svc.RecordWebhookEvent("stripe", "org-1", "evt_1", "payment_intent.succeeded", nil)
	if err != nil || !isNew {
		t.Fatalf("first delivery: isNew=%v err=%v", isNew, err)
	}
	if isNew, err = svc.RecordWebhookEvent("stripe", "org-1", "evt_1", "payment_intent.succeeded", nil); isNew || !errors.Is(err, ErrWebhookInFlight) {
		t.Fatal("in-flight redelivery must not be processed concurrently")
	}
	svc.MarkWebhookProcessed("stripe", "evt_1", errors.New("db down"), true)
	if isNew, _ = svc.RecordWebhookEvent("stripe", "org-1", "evt_1", "payment_intent.succeeded", nil); !isNew {
		t.Fatal("transient failure must be reprocessed on redelivery")
	}
	svc.MarkWebhookProcessed("stripe", "evt_1", nil, false)
	if isNew, _ = svc.RecordWebhookEvent("stripe", "org-1", "evt_1", "payment_intent.succeeded", nil); isNew {
		t.Fatal("processed event must be a duplicate")
	}
}

func TestStripeCumulativeRefunds(t *testing.T) {
	for _, capture := range []int{10000, 12000} {
		t.Run(fmt.Sprint(capture), func(t *testing.T) {
			svc := NewService()
			inv := &Invoice{ID: "inv", OrgID: "org", HouseholdID: "hh", Status: "issued", TotalCents: 10000, AmountDueCents: 10000}
			svc.invoices[inv.ID] = inv
			if _, _, err := svc.HandleStripePaymentIntentSucceeded("org", "pi", inv.ID, capture, "actor"); err != nil {
				t.Fatal(err)
			}
			for _, total := range []int{2000, 5000, 5000, 2000, capture, capture} {
				if _, _, err := svc.HandleStripeChargeRefunded("org", "pi", inv.ID, total, fmt.Sprintf("re_%d", total), "", "actor"); err != nil {
					t.Fatal(err)
				}
				expectedPaid := max(0, 10000-total)
				if total == 2000 && len(svc.refunds[inv.ID]) > 1 {
					expectedPaid = 5000
				}
				if inv.AmountPaidCents != expectedPaid {
					t.Fatalf("cumulative refund %d: paid=%d want %d", total, inv.AmountPaidCents, expectedPaid)
				}
			}
			if balance := svc.finance.balances[balanceKey("org", "hh")]; balance != 0 {
				t.Fatalf("credit after full refund: %d", balance)
			}
			if len(svc.refunds[inv.ID]) != 3 {
				t.Fatalf("duplicate/out-of-order delivery created refunds: %d", len(svc.refunds[inv.ID]))
			}
		})
	}
}
