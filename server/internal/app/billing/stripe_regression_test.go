package billing

import (
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// Staff-recorded Stripe payments must not be credited a second time when the
// payment_intent.succeeded webhook for the same intent is delivered.
func TestStaffRecordedStripePayment_WebhookDoesNotDoubleCredit(t *testing.T) {
	t.Parallel()

	svc := NewService()
	p := billingAdminPrincipal()
	org := p.OrgID

	inv := issuedInvoice(t, svc, p, "hh-double", "INV-DOUBLE", 10000)

	if _, _, err := svc.RecordPayment(org, p.UserID, p, inv.ID, RecordPaymentInput{
		AmountCents: 15000, Method: "stripe", StripePaymentIntentID: "pi_staff",
		ExcessToCredit: true,
	}); err != nil {
		t.Fatalf("record payment: %v", err)
	}
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_staff", inv.ID, 15000, "stripe-webhook"); err != nil {
		t.Fatalf("webhook: %v", err)
	}

	credit, err := svc.GetHouseholdCredit(org, p, "hh-double")
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if credit.BalanceCents != 5000 {
		t.Fatalf("expected a single 5000 credit, got %d", credit.BalanceCents)
	}
}

// A partially recorded Stripe intent leaves the rest of the balance owed; the
// webhook must not turn the unapplied part into household credit.
func TestPartiallyRecordedStripePayment_WebhookDoesNotCreditBalance(t *testing.T) {
	t.Parallel()

	svc := NewService()
	p := billingAdminPrincipal()
	org := p.OrgID

	inv := issuedInvoice(t, svc, p, "hh-partial", "INV-PARTIAL", 10000)

	if _, _, err := svc.RecordPayment(org, p.UserID, p, inv.ID, RecordPaymentInput{
		AmountCents: 4000, Method: "stripe", StripePaymentIntentID: "pi_partial",
	}); err != nil {
		t.Fatalf("record payment: %v", err)
	}
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(org, "pi_partial", inv.ID, 4000, "stripe-webhook"); err != nil {
		t.Fatalf("webhook: %v", err)
	}

	credit, err := svc.GetHouseholdCredit(org, p, "hh-partial")
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if credit.BalanceCents != 0 {
		t.Fatalf("expected no credit for an unapplied balance, got %d", credit.BalanceCents)
	}
}

func issuedInvoice(t *testing.T, svc *Service, p *appmiddleware.Principal, household, number string, cents int) *Invoice {
	t.Helper()
	inv, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: household, InvoiceNumber: number})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := svc.AddInvoiceItem(p.OrgID, p.UserID, p, inv.ID, AddInvoiceItemInput{
		Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee",
	}); err != nil {
		t.Fatalf("add item: %v", err)
	}
	issued, _, err := svc.IssueInvoice(p.OrgID, p.UserID, p, inv.ID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return issued
}
