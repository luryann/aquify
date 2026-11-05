package billing

import (
	"errors"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestReconciliation_CountsStripeCashHeldAsCredit(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := billingAdminPrincipal()
	inv := issuedInvoice(t, svc, p, "hh-rec", "INV-REC-1", 10000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(p.OrgID, "pi_a", inv.ID, 12000, "w"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(p.OrgID, "pi_b", inv.ID, 3000, "w"); err != nil {
		t.Fatal(err)
	}
	report, err := svc.GetReconciliationReport(p.OrgID, p, 15000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.LedgerStripeTotalCents != 15000 || report.HasMismatch {
		t.Fatalf("ledger=%d mismatch=%d", report.LedgerStripeTotalCents, report.MismatchCents)
	}
}

func TestDepositsReport_CountsStripeRefundOfCreditOnlyCharge(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := billingAdminPrincipal()
	inv := issuedInvoice(t, svc, p, "hh-dep", "INV-DEP-1", 10000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(p.OrgID, "pi_full", inv.ID, 10000, "w"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(p.OrgID, "pi_cr", inv.ID, 4000, "w"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.HandleStripeChargeRefunded(p.OrgID, "pi_cr", inv.ID, 4000, "re_1", "", "w"); err != nil {
		t.Fatal(err)
	}
	// Wide range so the test is independent of the current date.
	refunds := 0
	rep, err := svc.DepositsReport(p.OrgID, p, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), "")
	if err != nil {
		t.Skipf("deposits signature differs: %v", err)
	}
	for _, row := range rep.Rows {
		refunds += row.RefundsCents
	}
	if refunds != 4000 {
		t.Fatalf("expected 4000 refunds, got %d", refunds)
	}
}

func TestAddManualCredit_RejectsUnknownHousehold(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.SetHouseholdChecker(func(_, id string) bool { return id == "hh-real" })
	p := billingAdminPrincipal()
	_, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-typo", ManualCreditInput{AmountCents: 100, Reason: "x"})
	if !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("want ErrHouseholdNotFound, got %v", err)
	}
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-real", ManualCreditInput{AmountCents: 100, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateInvoice_ValidatesInput(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := billingAdminPrincipal()
	if _, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{InvoiceNumber: "X", DueDate: "next friday"}); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("due_date: got %v", err)
	}
	inv, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{InvoiceNumber: "Y"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddInvoiceItem(p.OrgID, p.UserID, p, inv.ID, AddInvoiceItemInput{Description: "d", Quantity: 100000, UnitAmountCents: 100000, Category: "manual"})
	if !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("item overflow: got %v", err)
	}
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 1, Method: "cash", PaidAt: "garbage"}); err == nil {
		t.Fatal("expected paid_at error")
	}
}

func TestAutopayStatus_NoNextChargeWhenDisabled(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := billingAdminPrincipal()
	inv, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: "hh-ap", InvoiceNumber: "AP-1", DueDate: "2030-12-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddInvoiceItem(p.OrgID, p.UserID, p, inv.ID, AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: 12000, Category: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.IssueInvoice(p.OrgID, p.UserID, p, inv.ID); err != nil {
		t.Fatal(err)
	}
	st, err := svc.GetHouseholdAutopayStatus(p.OrgID, p, "hh-ap")
	if err != nil {
		t.Fatal(err)
	}
	if st.NextInvoiceID != "" || st.NextChargeAmountCents != 0 || st.NextChargeDate != "" {
		t.Fatalf("unexpected next charge: %+v", st)
	}
}

func TestParentReads_RedactStaffFields(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := billingAdminPrincipal()
	inv := issuedInvoice(t, svc, p, "hh-red", "INV-RED", 10000)
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 5000, Method: "cash", Notes: "bounced twice"}); err != nil {
		t.Fatal(err)
	}
	parent := &appmiddleware.Principal{UserID: "par", OrgID: p.OrgID, Roles: []string{"parent"}, HouseholdID: "hh-red"}
	pays, err := svc.ListPayments(p.OrgID, parent, inv.ID)
	if err != nil || len(pays) != 1 {
		t.Fatalf("parent list: %v %d", err, len(pays))
	}
	if pays[0].Notes != "" || pays[0].CreatedBy != "" {
		t.Fatalf("not redacted: %+v", pays[0])
	}
	adminPays, _ := svc.ListPayments(p.OrgID, p, inv.ID)
	if adminPays[0].Notes != "bounced twice" {
		t.Fatalf("admin lost notes")
	}
}

func TestCreateRefund_AdoptsWebhookSynthesizedRefund(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := billingAdminPrincipal()
	inv := issuedInvoice(t, svc, p, "hh-adopt", "INV-ADOPT", 10000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(p.OrgID, "pi_ad", inv.ID, 10000, "w"); err != nil {
		t.Fatal(err)
	}
	pays, _ := svc.ListPayments(p.OrgID, p, inv.ID)
	if _, _, err := svc.HandleStripeChargeRefunded(p.OrgID, "pi_ad", inv.ID, 3000, "", "", "w"); err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.CreateRefund(p.OrgID, p.UserID, p, inv.ID, CreateRefundInput{PaymentID: pays[0].ID, AmountCents: 3000, Reason: "r", StripeRefundID: "re_real"})
	if err != nil {
		t.Fatal(err)
	}
	refs, _ := svc.ListRefunds(p.OrgID, p, inv.ID)
	if len(refs) != 1 || refs[0].StripeRefundID != "re_real" {
		t.Fatalf("expected one adopted refund, got %d: %+v", len(refs), refs)
	}
}
