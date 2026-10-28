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

func TestIssueInvoiceRejectsEmptyInvoice(t *testing.T) {
	t.Parallel()

	svc := NewService()
	principal := billingAdminPrincipal()
	invoice, err := svc.CreateInvoice(principal.OrgID, principal.UserID, principal, CreateInvoiceInput{
		HouseholdID:   "hh-empty",
		InvoiceNumber: "INV-EMPTY-1",
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, _, err := svc.IssueInvoice(principal.OrgID, principal.UserID, principal, invoice.ID); err != ErrInvoiceEmpty {
		t.Fatalf("want ErrInvoiceEmpty, got %v", err)
	}
}

func TestParentCannotSeeDraftInvoices(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := billingAdminPrincipal()
	parent := &appmiddleware.Principal{UserID: "p", OrgID: admin.OrgID, Roles: []string{"parent"}, HouseholdID: "hh-draft"}
	draft, err := svc.CreateInvoice(admin.OrgID, admin.UserID, admin, CreateInvoiceInput{HouseholdID: "hh-draft", InvoiceNumber: "INV-DRAFT-1"})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := svc.GetInvoice(admin.OrgID, parent, draft.ID); err != ErrInvoiceNotFound {
		t.Fatalf("parent get draft: want ErrInvoiceNotFound, got %v", err)
	}
	if _, err := svc.ListInvoiceItems(admin.OrgID, parent, draft.ID); err != ErrInvoiceNotFound {
		t.Fatalf("parent list draft items: want ErrInvoiceNotFound, got %v", err)
	}
	if list, err := svc.ListInvoices(admin.OrgID, parent, ListInvoicesFilters{}); err != nil || len(list) != 0 {
		t.Fatalf("parent list: want empty, got %d err=%v", len(list), err)
	}
	// Staff still see the draft.
	if list, err := svc.ListInvoices(admin.OrgID, admin, ListInvoicesFilters{}); err != nil || len(list) != 1 {
		t.Fatalf("admin list: want 1, got %d err=%v", len(list), err)
	}
	if _, err := svc.AddInvoiceItem(admin.OrgID, admin.UserID, admin, draft.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: 1000, Category: "manual"}); err != nil {
		t.Fatalf("AddInvoiceItem: %v", err)
	}
	if _, _, err := svc.IssueInvoice(admin.OrgID, admin.UserID, admin, draft.ID); err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}
	if list, err := svc.ListInvoices(admin.OrgID, parent, ListInvoicesFilters{}); err != nil || len(list) != 1 {
		t.Fatalf("parent list after issue: want 1, got %d err=%v", len(list), err)
	}
}

func TestWriteOffRemainsZeroAfterDerivingPaymentAmounts(t *testing.T) {
	svc := NewService()
	svc.invoices["inv"] = &Invoice{ID: "inv", Status: "written_off", TotalCents: 5000, AmountDueCents: 0}
	svc.payments["inv"] = []*Payment{{AmountCents: 2000, Status: "succeeded"}}
	svc.deriveAmountsPaidLocked()
	inv := svc.invoices["inv"]
	if inv.AmountPaidCents != 2000 || inv.AmountDueCents != 0 {
		t.Fatalf("write-off changed on reload: paid=%d due=%d", inv.AmountPaidCents, inv.AmountDueCents)
	}
}

func TestStripeWebhookRedeliveryPreservesPaymentAndRefund(t *testing.T) {
	svc := NewService()
	inv := &Invoice{ID: "inv", OrgID: "org", Status: "issued", TotalCents: 1000, AmountDueCents: 1000}
	svc.invoices[inv.ID] = inv
	payment, _, err := svc.HandleStripePaymentIntentSucceeded("org", "pi", "inv", 1000, "actor")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleStripePaymentIntentFailed("pi"); err != nil {
		t.Fatal(err)
	}
	if payment.Status != "succeeded" {
		t.Fatalf("delayed failure downgraded payment: %s", payment.Status)
	}
	refund, _, err := svc.HandleStripeChargeRefunded("org", "pi", "inv", 1000, "re", "", "actor")
	if err != nil {
		t.Fatal(err)
	}
	count := len(svc.ledger)
	retry, event, err := svc.HandleStripeChargeRefunded("org", "pi", "inv", 1000, "re", "", "actor")
	if err != nil {
		t.Fatalf("full refund retry: %v", err)
	}
	if retry.ID != refund.ID || event != nil || len(svc.ledger) != count {
		t.Fatal("refund retry changed ledger")
	}
	if err := svc.HandleStripePaymentIntentFailed("pi"); err != nil {
		t.Fatal(err)
	}
	if payment.Status != "reversed" {
		t.Fatalf("delayed failure downgraded reversed payment: %s", payment.Status)
	}
}

func TestStripePendingPaymentCanFail(t *testing.T) {
	svc := NewService()
	payment := &Payment{ID: "payment", Status: "pending"}
	svc.stripeIntentIndex["pi"] = payment
	if err := svc.HandleStripePaymentIntentFailed("pi"); err != nil {
		t.Fatal(err)
	}
	if payment.Status != "failed" || payment.FailedAt == nil {
		t.Fatalf("pending failure not recorded: %+v", payment)
	}
}
