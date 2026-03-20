package billing

import (
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func billingAdminPrincipal() *appmiddleware.Principal {
	return &appmiddleware.Principal{
		UserID: "billing-admin",
		OrgID:  "org-billing-invariants",
		Roles:  []string{"admin"},
	}
}

func TestLedgerInvariants_PaymentAndRefundRemainBalanced(t *testing.T) {
	t.Parallel()

	svc := NewService()
	principal := billingAdminPrincipal()
	orgID := principal.OrgID

	invoice, err := svc.CreateInvoice(orgID, principal.UserID, principal, CreateInvoiceInput{
		HouseholdID:   "hh-100",
		InvoiceNumber: "INV-INVARIANT-1",
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := svc.AddInvoiceItem(orgID, principal.UserID, principal, invoice.ID, AddInvoiceItemInput{
		Description:     "Program fee",
		Quantity:        1,
		UnitAmountCents: 12000,
		Category:        "program_fee",
	}); err != nil {
		t.Fatalf("add invoice item: %v", err)
	}
	issued, _, err := svc.IssueInvoice(orgID, principal.UserID, principal, invoice.ID)
	if err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	payment, _, err := svc.RecordPayment(orgID, principal.UserID, principal, issued.ID, RecordPaymentInput{
		AmountCents: 12000,
		Method:      "check",
	})
	if err != nil {
		t.Fatalf("record payment: %v", err)
	}
	if _, _, err := svc.CreateRefund(orgID, principal.UserID, principal, issued.ID, CreateRefundInput{
		PaymentID:   payment.ID,
		AmountCents: 2500,
		Reason:      "lane adjustment",
	}); err != nil {
		t.Fatalf("create refund: %v", err)
	}

	updated, err := svc.GetInvoice(orgID, principal, issued.ID)
	if err != nil {
		t.Fatalf("get invoice: %v", err)
	}
	if updated.Status != "refunded_partial" {
		t.Fatalf("expected invoice status refunded_partial, got %q", updated.Status)
	}
	if updated.AmountDueCents != 2500 {
		t.Fatalf("expected invoice amount due to match refunded amount 2500, got %d", updated.AmountDueCents)
	}

	events := svc.SnapshotLedgerEvents(orgID)
	total := 0
	for _, event := range events {
		if event.InvoiceID == issued.ID {
			total += event.AmountCents
		}
	}
	// invoice_issued +12000, payment_succeeded -12000, refund_processed +2500
	if total != 2500 {
		t.Fatalf("expected ledger net total 2500 for refunded amount, got %d", total)
	}
}

func TestLedgerInvariants_WriteOffProducesZeroOutstandingBalance(t *testing.T) {
	t.Parallel()

	svc := NewService()
	principal := billingAdminPrincipal()
	orgID := principal.OrgID

	invoice, err := svc.CreateInvoice(orgID, principal.UserID, principal, CreateInvoiceInput{
		HouseholdID:   "hh-101",
		InvoiceNumber: "INV-INVARIANT-2",
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := svc.AddInvoiceItem(orgID, principal.UserID, principal, invoice.ID, AddInvoiceItemInput{
		Description:     "Meet fee",
		Quantity:        1,
		UnitAmountCents: 5000,
		Category:        "meet_fee",
	}); err != nil {
		t.Fatalf("add invoice item: %v", err)
	}
	issued, _, err := svc.IssueInvoice(orgID, principal.UserID, principal, invoice.ID)
	if err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	if _, _, err := svc.RecordPayment(orgID, principal.UserID, principal, issued.ID, RecordPaymentInput{
		AmountCents: 2000,
		Method:      "cash",
	}); err != nil {
		t.Fatalf("record partial payment: %v", err)
	}
	writtenOff, ledgerEvent, err := svc.WriteOffInvoice(orgID, principal.UserID, principal, issued.ID)
	if err != nil {
		t.Fatalf("write off invoice: %v", err)
	}
	if ledgerEvent.EventType != "invoice_written_off" {
		t.Fatalf("expected write-off ledger event, got %q", ledgerEvent.EventType)
	}
	if writtenOff.Status != "written_off" {
		t.Fatalf("expected invoice status written_off, got %q", writtenOff.Status)
	}
	if writtenOff.AmountDueCents != 0 {
		t.Fatalf("expected write-off to zero amount due, got %d", writtenOff.AmountDueCents)
	}
}
