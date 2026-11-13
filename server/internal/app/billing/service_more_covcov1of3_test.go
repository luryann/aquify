package billing

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCov1xHouseholdCheckerAndStripeAccountResolver(t *testing.T) {
	t.Parallel()
	svc := NewService()
	rejectAll := func(string, string) bool { return false }
	acceptAll := func(string, string) bool { return true }

	// With no checker every id passes; the default only applies when nothing is set.
	if err := svc.requireHouseholdInOrg(cov1xOrg, "hh"); err != nil {
		t.Fatalf("no checker: %v", err)
	}
	svc.SetHouseholdCheckerDefault(rejectAll)
	svc.SetHouseholdCheckerDefault(acceptAll)
	if err := svc.requireHouseholdInOrg(cov1xOrg, " hh "); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("the first default must stay in force: %v", err)
	}
	svc.SetHouseholdChecker(acceptAll)
	if err := svc.requireHouseholdInOrg(cov1xOrg, "hh"); err != nil {
		t.Fatalf("explicit checker replaces the default: %v", err)
	}

	if acct, err := svc.StripeAccountFor(cov1xOrg); err != nil || acct != "" {
		t.Fatalf("no resolver means the platform account: %q %v", acct, err)
	}
	svc.SetStripeAccountResolver(func(org string) (string, error) { return "acct_" + org, nil })
	if acct, err := svc.StripeAccountFor("o1"); err != nil || acct != "acct_o1" {
		t.Fatalf("resolver: %q %v", acct, err)
	}
	svc.SetStripeAccountResolver(func(string) (string, error) { return "", errors.New("no connected account") })
	if _, err := svc.StripeAccountFor("o1"); err == nil || err.Error() != "no connected account" {
		t.Fatalf("resolver failures must be returned, never replaced by the platform account: %v", err)
	}
}

func TestCov1xServiceWithoutPoolIsInMemory(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil)
	if svc.repo != nil {
		t.Fatal("a nil pool must yield an in-memory service")
	}
	if _, err := svc.CreateInvoice(cov1xOrg, "a", cov1xAdmin, CreateInvoiceInput{HouseholdID: "hh", InvoiceNumber: "MEM-1"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh is a no-op without a database: %v", err)
	}
	if id := svc.newID(); len(id) == 0 {
		t.Fatal("ids must be generated")
	}
}

func TestCov1xInvoiceArithmeticHelpersClampAtZero(t *testing.T) {
	t.Parallel()
	svc := NewService()

	// Adding a line to a draft that already carries more paid than total never yields a negative balance.
	draft := cov1xDraft(t, svc, "hh-a", "OVERPAID-DRAFT", 0)
	svc.mu.Lock()
	svc.invoices[draft.ID].AmountPaidCents = 5000
	svc.mu.Unlock()
	if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, draft.ID, AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: 100, Category: "manual"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, draft.ID); got.TotalCents != 100 || got.AmountDueCents != 0 {
		t.Fatalf("due must clamp at zero: %+v", got)
	}

	// Status follows what has been paid.
	unpaid := &Invoice{TotalCents: 1000, AmountPaidCents: 0, Status: "partially_paid", AmountDueCents: 500}
	svc.updateInvoiceStatusAfterPayment(unpaid)
	if unpaid.Status != "issued" {
		t.Fatalf("nothing paid means issued, got %q", unpaid.Status)
	}
	over := &Invoice{TotalCents: 1000, AmountPaidCents: 1500, Status: "issued", AmountDueCents: 1000}
	svc.updateInvoiceStatusAfterPayment(over)
	if over.Status != "paid" || over.AmountDueCents != 0 {
		t.Fatalf("overpaid: %+v", over)
	}
	part := &Invoice{TotalCents: 1000, AmountPaidCents: 400, Status: "issued"}
	svc.updateInvoiceStatusAfterPayment(part)
	if part.Status != "partially_paid" || part.AmountDueCents != 600 {
		t.Fatalf("partial: %+v", part)
	}
}

func TestCov1xCoachCannotCreateOrListInvoices(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, err := svc.CreateInvoice(cov1xOrg, "c", cov1xCoach, CreateInvoiceInput{InvoiceNumber: "X"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ListInvoices(cov1xOrg, cov1xCoach, ListInvoicesFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list: %v", err)
	}
}

func TestCov1xListPaymentsForOrgPutsPaidBeforeUnpaidWhateverTheMapOrder(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "A", 5000)
	paidAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	svc.mu.Lock()
	svc.paymentsByID["unpaid"] = &Payment{ID: "unpaid", OrgID: cov1xOrg, InvoiceID: inv.ID, CreatedAt: paidAt.Add(time.Hour)}
	svc.paymentsByID["paid"] = &Payment{ID: "paid", OrgID: cov1xOrg, InvoiceID: inv.ID, PaidAt: &paidAt, CreatedAt: paidAt}
	svc.mu.Unlock()
	// Map iteration order is random, so repeat to see both input orders.
	for i := 0; i < 40; i++ {
		got, err := svc.ListPaymentsForOrg(cov1xOrg, cov1xAdmin, ListPaymentsFilters{})
		if err != nil || len(got) != 2 || got[0].ID != "paid" || got[1].ID != "unpaid" {
			t.Fatalf("iteration %d: %+v %v", i, got, err)
		}
	}
}

func TestCov1xFullRefundReversesPaymentAndClosesInvoice(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "FULL", 2500)
	pay := cov1xPay(t, svc, inv.ID, 2500)
	// A refund belonging to another club sits in the same process.
	svc.mu.Lock()
	svc.refundsByID["foreign"] = &Refund{ID: "foreign", OrgID: "other-org"}
	svc.mu.Unlock()

	if _, _, err := svc.CreateRefund(cov1xOrg, "a", cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 2500, Reason: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, inv.ID)
	if got.Status != "refunded_full" || got.AmountPaidCents != 0 || got.AmountDueCents != 2500 {
		t.Fatalf("invoice after a full refund: %+v", got)
	}
	if p, _ := svc.GetPayment(cov1xOrg, cov1xAdmin, inv.ID, pay.ID); p.Status != "reversed" {
		t.Fatalf("payment after a full refund: %+v", p)
	}
	if refunds := svc.SnapshotRefunds(cov1xOrg); len(refunds) != 1 || refunds[0].AmountCents != 2500 {
		t.Fatalf("refund snapshot must exclude other clubs: %+v", refunds)
	}
}

func TestCov1xReverseStripeCreditClampsToWhatWasCredited(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := &Invoice{ID: "inv-1", OrgID: cov1xOrg, HouseholdID: "hh-a"}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.finance.credits = append(svc.finance.credits, &CreditEntry{OrgID: cov1xOrg, HouseholdID: "hh-a", Kind: CreditKindOverpayment, AmountCents: 300, StripeRef: "pi_z"})
	svc.finance.balances[balanceKey(cov1xOrg, "hh-a")] = 1000

	// Asking to reverse more than was credited only takes back the credited amount.
	if err := svc.reverseStripeCreditLocked(inv, "pi_z", "re_a", 500, "wh"); err != nil {
		t.Fatal(err)
	}
	if bal := svc.finance.balances[balanceKey(cov1xOrg, "hh-a")]; bal != 700 {
		t.Fatalf("balance after clamped reversal: %d", bal)
	}
	// A second refund id for the same intent finds everything already reversed: nothing more to take.
	if err := svc.reverseStripeCreditLocked(inv, "pi_z", "re_b", 300, "wh"); err != nil {
		t.Fatal(err)
	}
	if bal := svc.finance.balances[balanceKey(cov1xOrg, "hh-a")]; bal != 700 {
		t.Fatalf("nothing left to reverse, balance must stay: %d", bal)
	}
	// Replaying the first refund id is a no-op.
	if err := svc.reverseStripeCreditLocked(inv, "pi_z", "re_a", 500, "wh"); err != nil {
		t.Fatal(err)
	}
	if bal := svc.finance.balances[balanceKey(cov1xOrg, "hh-a")]; bal != 700 {
		t.Fatalf("replay must not reverse twice: %d", bal)
	}
}
