package billing

import (
	"errors"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov2xCreditOrg = "org-cov2x-credit"

func cov2xAdmin(org string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "cov2x-admin", OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
}

func cov2xParent(org, household string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "cov2x-parent", OrgID: org, Roles: []string{"parent"}, HouseholdID: household}
}

func cov2xCoach(org string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "cov2x-coach", OrgID: org, Roles: []string{"coach"}}
}

// cov2xIssuedInvoice creates an issued single-line invoice with an explicit due date.
func cov2xIssuedInvoice(t *testing.T, svc *Service, p *appmiddleware.Principal, household, number, due string, cents int) *Invoice {
	t.Helper()
	inv, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: household, InvoiceNumber: number, DueDate: due})
	if err != nil {
		t.Fatalf("create invoice %s: %v", number, err)
	}
	if _, err := svc.AddInvoiceItem(p.OrgID, p.UserID, p, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee"}); err != nil {
		t.Fatalf("add item %s: %v", number, err)
	}
	issued, _, err := svc.IssueInvoice(p.OrgID, p.UserID, p, inv.ID)
	if err != nil {
		t.Fatalf("issue %s: %v", number, err)
	}
	return issued
}

func TestCov2xCreditDeltaSigns(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		CreditKindManualCredit:   500,
		CreditKindOverpayment:    500,
		CreditKindRefundToCredit: 500,
		CreditKindManualDebit:    -500,
		CreditKindApplied:        -500,
	}
	for kind, want := range cases {
		if got := creditDelta(kind, 500); got != want {
			t.Errorf("creditDelta(%s) = %d, want %d", kind, got, want)
		}
	}
}

func TestCov2xLedgerEventInheritsHouseholdFromInvoice(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov2xAdmin(cov2xCreditOrg)
	inv := cov2xIssuedInvoice(t, svc, p, "hh-led", "L-1", "", 1000)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	inherited := svc.newLedgerEventLocked(p.OrgID, inv.ID, "", "", "", "note", 5, "d", "actor")
	if inherited.HouseholdID != "hh-led" || inherited.InvoiceID != inv.ID || inherited.AmountCents != 5 || inherited.ActorID != "actor" {
		t.Fatalf("household must come from the invoice: %+v", inherited)
	}
	unknown := svc.newLedgerEventLocked(p.OrgID, "no-such-invoice", "", "", "", "note", 5, "d", "actor")
	if unknown.HouseholdID != "" {
		t.Fatalf("unknown invoice must leave household empty, got %q", unknown.HouseholdID)
	}
	explicit := svc.newLedgerEventLocked(p.OrgID, inv.ID, "", "", "hh-explicit", "note", 5, "d", "actor")
	if explicit.HouseholdID != "hh-explicit" {
		t.Fatalf("explicit household must win, got %q", explicit.HouseholdID)
	}
}

func TestCov2xAddCreditEntryValidationAndLedgerSigns(t *testing.T) {
	t.Parallel()
	svc := NewService()
	org := cov2xCreditOrg
	ctx := t.Context()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, err := svc.addCreditEntryLocked(ctx, creditEntryParams{OrgID: org, HouseholdID: "  ", AmountCents: 100, Kind: CreditKindManualCredit}); !errors.Is(err, ErrHouseholdRequired) {
		t.Fatalf("blank household: %v", err)
	}
	var verr *FinanceValidationError
	if _, err := svc.addCreditEntryLocked(ctx, creditEntryParams{OrgID: org, HouseholdID: "hh", AmountCents: 0, Kind: CreditKindManualCredit}); !errors.As(err, &verr) || verr.Field != "amount_cents" {
		t.Fatalf("non-positive amount must be an amount_cents validation error, got %v", err)
	}
	if len(svc.finance.credits) != 0 || len(svc.ledger) != 0 {
		t.Fatal("rejected entries must not leave records behind")
	}

	credit, err := svc.addCreditEntryLocked(ctx, creditEntryParams{OrgID: org, HouseholdID: "hh", AmountCents: 1000, Kind: CreditKindManualCredit, Reason: "  gift  ", ActorID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if credit.AmountCents != 1000 || credit.BalanceAfterCents != 1000 || credit.Reason != "gift" {
		t.Fatalf("credit entry: %+v", credit)
	}
	ev := svc.ledger[len(svc.ledger)-1]
	if ev.EventType != "credit_issued" || ev.AmountCents != -1000 {
		t.Fatalf("issued credit reduces what the household owes: %+v", ev)
	}

	debit, err := svc.addCreditEntryLocked(ctx, creditEntryParams{OrgID: org, HouseholdID: "hh", AmountCents: 400, Kind: CreditKindManualDebit, ActorID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if debit.AmountCents != -400 || debit.BalanceAfterCents != 600 {
		t.Fatalf("debit entry: %+v", debit)
	}
	ev = svc.ledger[len(svc.ledger)-1]
	if ev.EventType != "credit_debited" || ev.AmountCents != 400 {
		t.Fatalf("debited credit increases what the household owes: %+v", ev)
	}

	applied, err := svc.addCreditEntryLocked(ctx, creditEntryParams{OrgID: org, HouseholdID: "hh", AmountCents: 100, Kind: CreditKindApplied, ActorID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if applied.BalanceAfterCents != 500 {
		t.Fatalf("applied entry balance: %+v", applied)
	}
	ev = svc.ledger[len(svc.ledger)-1]
	if ev.EventType != "credit_applied" || ev.AmountCents != -100 {
		t.Fatalf("applied credit settles invoice balance: %+v", ev)
	}

	if _, err := svc.addCreditEntryLocked(ctx, creditEntryParams{OrgID: org, HouseholdID: "hh", AmountCents: 501, Kind: CreditKindManualDebit}); !errors.Is(err, ErrInsufficientCredit) {
		t.Fatalf("overdraw: %v", err)
	}
	if got := svc.finance.balances[balanceKey(org, "hh")]; got != 500 {
		t.Fatalf("failed overdraw must not change the balance, got %d", got)
	}
}

func TestCov2xApplyHouseholdCreditGuards(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov2xAdmin(cov2xCreditOrg)
	ctx := t.Context()

	issued := cov2xIssuedInvoice(t, svc, p, "hh-guard", "G-1", "", 10000)
	draft, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: "hh-guard", InvoiceNumber: "G-DRAFT"})
	if err != nil {
		t.Fatal(err)
	}
	noHousehold, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{InvoiceNumber: "G-NOHH"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-guard", ManualCreditInput{Direction: "credit", AmountCents: 3000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	// Re-read the invoice: AddManualCredit above did not apply to it.
	issuedInv := svc.invoices[issued.ID]
	for name, inv := range map[string]*Invoice{
		"nil invoice":  nil,
		"no household": svc.invoices[noHousehold.ID],
		"draft status": svc.invoices[draft.ID],
	} {
		got, err := svc.applyHouseholdCreditLocked(ctx, inv, "a")
		if got != 0 || err != nil {
			t.Fatalf("%s: applied %d err %v, want 0 nil", name, got, err)
		}
	}
	paid := *issuedInv
	paid.AmountDueCents = 0
	if got, err := svc.applyHouseholdCreditLocked(ctx, &paid, "a"); got != 0 || err != nil {
		t.Fatalf("nothing due: %d %v", got, err)
	}
	if bal := svc.finance.balances[balanceKey(p.OrgID, "hh-guard")]; bal != 3000 {
		t.Fatalf("guards must not spend credit, balance %d", bal)
	}

	// Partial application: the whole balance goes against the invoice.
	applied, err := svc.applyHouseholdCreditLocked(ctx, issuedInv, "a")
	if err != nil || applied != 3000 {
		t.Fatalf("partial apply: %d %v", applied, err)
	}
	if issuedInv.Status != "partially_paid" || issuedInv.AmountDueCents != 7000 || issuedInv.AmountPaidCents != 3000 {
		t.Fatalf("invoice after credit: %+v", issuedInv)
	}
	pays := svc.payments[issuedInv.ID]
	if len(pays) != 1 || pays[0].Method != PaymentMethodAccountCredit || pays[0].AmountCents != 3000 {
		t.Fatalf("account_credit payment expected: %+v", pays)
	}
	// Balance is now empty so a second pass is a no-op.
	if got, err := svc.applyHouseholdCreditLocked(ctx, issuedInv, "a"); got != 0 || err != nil {
		t.Fatalf("empty balance: %d %v", got, err)
	}
}

func TestCov2xApplyCreditAmountRefusesOverdrawInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov2xAdmin(cov2xCreditOrg)
	inv := cov2xIssuedInvoice(t, svc, p, "hh-over", "O-1", "", 5000)
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-over", ManualCreditInput{Direction: "credit", AmountCents: 1000, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	live := svc.invoices[inv.ID]
	if got, err := svc.applyCreditAmountLocked(t.Context(), live, 2000, "a"); !errors.Is(err, ErrInsufficientCredit) || got != 0 {
		t.Fatalf("overdraw: %d %v", got, err)
	}
	if live.AmountPaidCents != 0 || len(svc.payments[live.ID]) != 0 || svc.finance.balances[balanceKey(p.OrgID, "hh-over")] != 1000 {
		t.Fatal("a refused application must not mutate invoice, payments or balance")
	}
}

func TestCov2xRefreshLockedIsNoOpInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if err := svc.refreshLocked(t.Context(), "any-org"); err != nil {
		t.Fatalf("in-memory refresh must be a no-op: %v", err)
	}
}

func TestCov2xGetHouseholdCreditAccessAndRedaction(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := cov2xAdmin(cov2xCreditOrg)
	if _, _, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "hh-a", ManualCreditInput{Direction: "credit", AmountCents: 700, Reason: "staff only note"}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetHouseholdCredit(admin.OrgID, nil, "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal: %v", err)
	}
	if _, err := svc.GetHouseholdCredit(admin.OrgID, cov2xCoach(admin.OrgID), "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if _, err := svc.GetHouseholdCredit(admin.OrgID, cov2xParent(admin.OrgID, "hh-b"), "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign parent: %v", err)
	}

	staffView, err := svc.GetHouseholdCredit(admin.OrgID, admin, " hh-a ")
	if err != nil || staffView.BalanceCents != 700 || len(staffView.Entries) != 1 {
		t.Fatalf("staff view: %+v %v", staffView, err)
	}
	if staffView.Entries[0].Reason != "staff only note" || staffView.Entries[0].CreatedBy != admin.UserID {
		t.Fatalf("staff must see reason and author: %+v", staffView.Entries[0])
	}

	parentView, err := svc.GetHouseholdCredit(admin.OrgID, cov2xParent(admin.OrgID, "hh-a"), "hh-a")
	if err != nil || parentView.BalanceCents != 700 || len(parentView.Entries) != 1 {
		t.Fatalf("parent view: %+v %v", parentView, err)
	}
	if parentView.Entries[0].Reason != "" || parentView.Entries[0].CreatedBy != "" {
		t.Fatalf("parent must not see staff notes or author: %+v", parentView.Entries[0])
	}
	// Redaction works on copies; the stored trail is untouched.
	again, _ := svc.GetHouseholdCredit(admin.OrgID, admin, "hh-a")
	if again.Entries[0].Reason != "staff only note" {
		t.Fatal("redacting for a parent must not rewrite stored entries")
	}
}

func TestCov2xAddManualCreditValidation(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.SetHouseholdChecker(func(_, id string) bool { return id == "hh-real" })
	admin := cov2xAdmin(cov2xCreditOrg)
	ok := ManualCreditInput{Direction: "credit", AmountCents: 100, Reason: "x"}

	if _, _, err := svc.AddManualCredit(admin.OrgID, admin.UserID, cov2xCoach(admin.OrgID), "hh-real", ok); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if _, _, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "   ", ok); !errors.Is(err, ErrHouseholdRequired) {
		t.Fatalf("blank household: %v", err)
	}
	blank := ok
	blank.Reason = "   "
	var verr *FinanceValidationError
	if _, _, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "hh-real", blank); !errors.As(err, &verr) || verr.Field != "reason" {
		t.Fatalf("blank reason: %v", err)
	}
	if _, _, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "hh-other-org", ok); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("unknown household: %v", err)
	}
	if _, _, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "hh-real", ManualCreditInput{Direction: "debit", AmountCents: 1, Reason: "x"}); !errors.Is(err, ErrInsufficientCredit) {
		t.Fatalf("debit with empty balance: %v", err)
	}
	// A debit never applies credit to invoices even when asked to.
	entry, applied, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "hh-real", ManualCreditInput{Direction: "credit", AmountCents: 900, Reason: "x"})
	if err != nil || len(applied) != 0 || entry.BalanceAfterCents != 900 {
		t.Fatalf("plain credit: %+v %v %v", entry, applied, err)
	}
	debit, applied, err := svc.AddManualCredit(admin.OrgID, admin.UserID, admin, "hh-real", ManualCreditInput{Direction: "debit", AmountCents: 400, Reason: "fix", ApplyToOpenInvoices: true})
	if err != nil || len(applied) != 0 || debit.AmountCents != -400 || debit.Kind != CreditKindManualDebit || debit.BalanceAfterCents != 500 {
		t.Fatalf("debit: %+v %v %v", debit, applied, err)
	}
}

func TestCov2xAddManualCreditAppliesOldestDueFirstAndStopsWhenSpent(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov2xAdmin(cov2xCreditOrg)
	late := cov2xIssuedInvoice(t, svc, p, "hh-apply", "A-LATE", "2026-12-20", 10000)
	first := cov2xIssuedInvoice(t, svc, p, "hh-apply", "A-FIRST", "2026-12-10", 8000)
	second := cov2xIssuedInvoice(t, svc, p, "hh-apply", "A-SECOND", "2026-12-10", 4000)
	// None of these may be touched.
	_ = cov2xIssuedInvoice(t, svc, p, "hh-someone-else", "A-OTHER-HH", "2026-12-01", 9000)
	if _, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: "hh-apply", InvoiceNumber: "A-DRAFT", DueDate: "2026-12-01"}); err != nil {
		t.Fatal(err)
	}

	_, appliedTo, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-apply", ManualCreditInput{
		Direction: "credit", AmountCents: 11000, Reason: "scholarship", ApplyToOpenInvoices: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(appliedTo) != 2 || appliedTo[0] != first.ID || appliedTo[1] != second.ID {
		t.Fatalf("credit must go to the earliest due (ties: oldest created) first: %v want [%s %s]", appliedTo, first.ID, second.ID)
	}
	got := func(id string) *Invoice { inv, _ := svc.GetInvoice(p.OrgID, p, id); return inv }
	if inv := got(first.ID); inv.Status != "paid" || inv.AmountDueCents != 0 {
		t.Fatalf("first invoice: %+v", inv)
	}
	if inv := got(second.ID); inv.Status != "partially_paid" || inv.AmountDueCents != 1000 {
		t.Fatalf("second invoice: %+v", inv)
	}
	if inv := got(late.ID); inv.Status != "issued" || inv.AmountDueCents != 10000 {
		t.Fatalf("later invoice must be untouched once credit is spent: %+v", inv)
	}
	credit, _ := svc.GetHouseholdCredit(p.OrgID, p, "hh-apply")
	if credit.BalanceCents != 0 {
		t.Fatalf("credit fully spent, balance %d", credit.BalanceCents)
	}

	// Credit that exceeds every open invoice leaves the remainder on account.
	_, appliedTo, err = svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-apply", ManualCreditInput{
		Direction: "credit", AmountCents: 50000, Reason: "big", ApplyToOpenInvoices: true,
	})
	if err != nil || len(appliedTo) != 2 {
		t.Fatalf("remaining invoices: %v %v", appliedTo, err)
	}
	credit, _ = svc.GetHouseholdCredit(p.OrgID, p, "hh-apply")
	if credit.BalanceCents != 50000-1000-10000 {
		t.Fatalf("remainder stays as credit: %d", credit.BalanceCents)
	}
}

func TestCov2xRefundToCredit(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov2xAdmin(cov2xCreditOrg)
	coach := cov2xCoach(p.OrgID)

	if _, _, err := svc.RefundToCredit(p.OrgID, p.UserID, coach, "no-such-invoice", CreateRefundInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin on unknown invoice must be forbidden, got %v", err)
	}
	if _, _, err := svc.RefundToCredit(p.OrgID, p.UserID, p, "no-such-invoice", CreateRefundInput{}); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("admin on unknown invoice: %v", err)
	}

	noHH := cov2xIssuedInvoice(t, svc, p, "", "R-NOHH", "", 4000)
	if _, _, err := svc.RefundToCredit(p.OrgID, p.UserID, coach, noHH.ID, CreateRefundInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin on household-less invoice: %v", err)
	}
	if _, _, err := svc.RefundToCredit(p.OrgID, p.UserID, p, noHH.ID, CreateRefundInput{}); !errors.Is(err, ErrHouseholdRequired) {
		t.Fatalf("admin on household-less invoice: %v", err)
	}
	// An invoice from another club is invisible: no household is resolved.
	if _, _, err := svc.RefundToCredit("org-someone-else", p.UserID, p, noHH.ID, CreateRefundInput{}); !errors.Is(err, ErrHouseholdRequired) {
		t.Fatalf("cross-org invoice must not resolve a household: %v", err)
	}

	inv := cov2xIssuedInvoice(t, svc, p, "hh-refund", "R-1", "", 10000)
	pay, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 10000, Method: "check"})
	if err != nil {
		t.Fatal(err)
	}
	// Caller-supplied Stripe refund ids are ignored: no money leaves the club.
	refund, _, err := svc.RefundToCredit(p.OrgID, p.UserID, p, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 2500, Reason: "moved away", StripeRefundID: "re_ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if refund.StripeRefundID != "" || refund.AmountCents != 2500 {
		t.Fatalf("refund: %+v", refund)
	}
	credit, _ := svc.GetHouseholdCredit(p.OrgID, p, "hh-refund")
	if credit.BalanceCents != 2500 || len(credit.Entries) != 1 || credit.Entries[0].Kind != CreditKindRefundToCredit || credit.Entries[0].RefundID != refund.ID {
		t.Fatalf("refund must land as account credit: %+v", credit)
	}
}
