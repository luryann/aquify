package billing

import (
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const (
	cov3xStOrg = "org-cov3x-st"
	cov3xStHH  = "hh-cov3x-st"
)

func cov3xAdmin(org string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "admin-cov3x", OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
}

func cov3xFarFuture(t *testing.T) time.Time { return cov3xDay(t, "2099-01-01") }

// cov3xInvoice creates an invoice with one line item and issues it.
func cov3xInvoice(t *testing.T, svc *Service, org, hh, number, due string, cents int) *Invoice {
	t.Helper()
	p := cov3xAdmin(org)
	inv, err := svc.CreateInvoice(org, p.UserID, p, CreateInvoiceInput{HouseholdID: hh, InvoiceNumber: number, DueDate: due})
	if err != nil {
		t.Fatalf("create %s: %v", number, err)
	}
	if _, err := svc.AddInvoiceItem(org, p.UserID, p, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee"}); err != nil {
		t.Fatalf("item %s: %v", number, err)
	}
	issued, _, err := svc.IssueInvoice(org, p.UserID, p, inv.ID)
	if err != nil {
		t.Fatalf("issue %s: %v", number, err)
	}
	return issued
}

func cov3xLinesOfType(st *Statement, typ string) []StatementLine {
	var out []StatementLine
	for _, l := range st.Lines {
		if l.Type == typ {
			out = append(out, l)
		}
	}
	return out
}

func TestCov3xHouseholdStatementCoversEveryLineKind(t *testing.T) {
	svc := NewService()
	org, hh := cov3xStOrg, cov3xStHH
	p := cov3xAdmin(org)

	// (a) voided invoice and (b) written-off invoice, issued while no credit exists.
	inv3 := cov3xInvoice(t, svc, org, hh, "INV-3", "", 4000)
	if _, _, err := svc.VoidInvoice(org, p.UserID, p, inv3.ID); err != nil {
		t.Fatal(err)
	}
	inv4 := cov3xInvoice(t, svc, org, hh, "INV-4", "", 700)
	if _, _, err := svc.WriteOffInvoice(org, p.UserID, p, inv4.ID); err != nil {
		t.Fatal(err)
	}

	// Another household's void and a ledger event pointing at a missing invoice
	// must never leak onto this household's statement.
	foreignInv := cov3xInvoice(t, svc, org, "hh-cov3x-other", "INV-OTHER", "", 1234)
	if _, _, err := svc.VoidInvoice(org, p.UserID, p, foreignInv.ID); err != nil {
		t.Fatal(err)
	}
	svc.ledger = append(svc.ledger, &LedgerEvent{ID: "ev-ghost", OrgID: org, InvoiceID: "inv-ghost", EventType: "invoice_voided", AmountCents: -999, CreatedAt: time.Now().UTC()})

	// (c) an overdue invoice that picks up a late fee; strip its issue event to
	// model a legacy invoice whose original charge must be derived from the total.
	inv5 := cov3xInvoice(t, svc, org, hh, "INV-5", "2026-09-01", 2000)
	if _, _, err := svc.UpdateBillingSettings(org, p.UserID, p, BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 0, LateFeeType: "flat", LateFeeFlatCents: 500, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	sweep, err := svc.ApplyLateFees(org, p.UserID, p, cov3xDay(t, "2026-12-01"))
	if err != nil || sweep.AppliedCount != 1 {
		t.Fatalf("late fee sweep: %+v %v", sweep, err)
	}
	kept := svc.ledger[:0]
	for _, ev := range svc.ledger {
		if !(ev.EventType == "invoice_issued" && ev.InvoiceID == inv5.ID) {
			kept = append(kept, ev)
		}
	}
	svc.ledger = kept

	// (d) overpayment held as credit.
	inv6 := cov3xInvoice(t, svc, org, hh, "INV-6", "", 1000)
	if _, _, err := svc.RecordPayment(org, p.UserID, p, inv6.ID, RecordPaymentInput{AmountCents: 1500, Method: "check", ExcessToCredit: true}); err != nil {
		t.Fatal(err)
	}
	// (e) manual credit; then INV-1 auto-applies the whole 2000 balance.
	if _, _, err := svc.AddManualCredit(org, p.UserID, p, hh, ManualCreditInput{Direction: "credit", AmountCents: 1500, Reason: "Fundraiser"}); err != nil {
		t.Fatal(err)
	}
	inv1 := cov3xInvoice(t, svc, org, hh, "INV-1", "", 10000)
	if inv1.AmountDueCents != 8000 {
		t.Fatalf("auto-applied credit should leave 8000 due, got %d", inv1.AmountDueCents)
	}
	pay1, _, err := svc.RecordPayment(org, p.UserID, p, inv1.ID, RecordPaymentInput{AmountCents: 8000, Method: "check"})
	if err != nil {
		t.Fatal(err)
	}
	// (f) refund to the original payment method (informational line).
	if _, _, err := svc.CreateRefund(org, p.UserID, p, inv1.ID, CreateRefundInput{PaymentID: pay1.ID, AmountCents: 2000, Reason: "overcharge"}); err != nil {
		t.Fatal(err)
	}
	// (g) refund to account credit (must not also show as a card refund).
	inv2 := cov3xInvoice(t, svc, org, hh, "INV-2", "", 5000)
	pay2, _, err := svc.RecordPayment(org, p.UserID, p, inv2.ID, RecordPaymentInput{AmountCents: 5000, Method: "check"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RefundToCredit(org, p.UserID, p, inv2.ID, CreateRefundInput{PaymentID: pay2.ID, AmountCents: 3000, Reason: "goodwill"}); err != nil {
		t.Fatal(err)
	}
	// (h) manual debit.
	if _, _, err := svc.AddManualCredit(org, p.UserID, p, hh, ManualCreditInput{Direction: "debit", AmountCents: 300, Reason: "Correction"}); err != nil {
		t.Fatal(err)
	}
	// A failed payment must never appear on the statement.
	svc.mu.Lock()
	paidAt := time.Now().UTC()
	svc.payments[inv2.ID] = append(svc.payments[inv2.ID], &Payment{ID: "p-failed", OrgID: org, InvoiceID: inv2.ID, AmountCents: 999, Method: "stripe", Status: "failed", PaidAt: &paidAt})
	svc.payments[inv2.ID] = append(svc.payments[inv2.ID], &Payment{ID: "p-nopaidat", OrgID: org, InvoiceID: inv2.ID, AmountCents: 998, Method: "check", Status: "succeeded"})
	svc.mu.Unlock()

	st, err := svc.HouseholdStatement(org, p, " "+hh+" ", time.Time{}, cov3xFarFuture(t))
	if err != nil {
		t.Fatal(err)
	}
	if st.HouseholdID != hh || st.From != "" || st.To != "2099-01-01" {
		t.Fatalf("header: %+v", st)
	}

	// Per-kind expectations.
	if got := cov3xLinesOfType(st, "void"); len(got) != 1 || got[0].AmountCents != -4000 || got[0].Description != "Invoice voided — INV-3" || got[0].InvoiceID != inv3.ID {
		t.Fatalf("void lines: %+v", got)
	}
	if got := cov3xLinesOfType(st, "write_off"); len(got) != 1 || got[0].AmountCents != -700 || got[0].Description != "Balance written off — INV-4" {
		t.Fatalf("write-off lines: %+v", got)
	}
	late := cov3xLinesOfType(st, "late_fee")
	if len(late) != 1 || late[0].AmountCents != 500 || late[0].Description != "Late fee on invoice INV-5" {
		t.Fatalf("late fee lines: %+v", late)
	}
	var inv5Line *StatementLine
	for i, l := range st.Lines {
		if l.Type == "invoice" && l.Reference == "INV-5" {
			inv5Line = &st.Lines[i]
		}
	}
	if inv5Line == nil || inv5Line.AmountCents != 2000 {
		t.Fatalf("legacy invoice charge must exclude the late fee (2500 total - 500): %+v", inv5Line)
	}
	applied := cov3xLinesOfType(st, "credit_applied")
	if len(applied) != 1 || !applied[0].Informational || applied[0].AmountCents != 0 || applied[0].DisplayAmountCents != -2000 || applied[0].Description != "Account credit applied to INV-1" {
		t.Fatalf("credit-applied lines: %+v", applied)
	}
	refunds := cov3xLinesOfType(st, "refund")
	if len(refunds) != 1 || !refunds[0].Informational || refunds[0].AmountCents != 0 || refunds[0].DisplayAmountCents != 2000 || refunds[0].Reference != "INV-1" {
		t.Fatalf("only the card refund may appear as a refund line: %+v", refunds)
	}
	descs := map[string]StatementLine{}
	for _, l := range cov3xLinesOfType(st, "credit") {
		descs[l.Description] = l
	}
	wantCredit := map[string]int{
		"Account credit — Fundraiser":         -1500,
		"Account credit removed — Correction": 300,
		"Refund to account credit — goodwill": -3000,
	}
	for desc, amt := range wantCredit {
		l, ok := descs[desc]
		if !ok || l.AmountCents != amt || l.Informational {
			t.Fatalf("credit line %q = %+v (present=%v), want amount %d; all: %v", desc, l, ok, amt, descs)
		}
	}
	overpay := 0
	for desc, l := range descs {
		if strings.HasPrefix(desc, "Overpayment held as account credit") {
			overpay++
			if l.AmountCents != -500 {
				t.Fatalf("overpayment line: %+v", l)
			}
		}
	}
	if overpay != 1 {
		t.Fatalf("want exactly one overpayment credit line, descriptions: %v", descs)
	}
	for _, l := range st.Lines {
		if l.Type == "payment" && (l.AmountCents == -999 || l.AmountCents == -998) {
			t.Fatalf("failed / unpaid payments must not be listed: %+v", l)
		}
	}

	// Running balance integrity and the account identity.
	run := 0
	for i, l := range st.Lines {
		run += l.AmountCents
		if l.BalanceCents != run {
			t.Fatalf("line %d (%s) balance %d != running %d", i, l.Type, l.BalanceCents, run)
		}
	}
	if st.ClosingBalanceCents != run || st.OpeningBalanceCents != 0 {
		t.Fatalf("closing %d opening %d, running %d", st.ClosingBalanceCents, st.OpeningBalanceCents, run)
	}
	if st.CreditBalanceCents != 2700 {
		t.Fatalf("credit balance = 2000 applied away + 3000 refund-to-credit - 300 debit = 2700, got %d", st.CreditBalanceCents)
	}
	if st.OpenInvoicesDueCents != 2500 {
		t.Fatalf("only INV-5 (2000 + 500 late fee) is open, got %d", st.OpenInvoicesDueCents)
	}
	if st.ClosingBalanceCents != st.OpenInvoicesDueCents-st.CreditBalanceCents {
		t.Fatalf("closing %d != open %d - credit %d", st.ClosingBalanceCents, st.OpenInvoicesDueCents, st.CreditBalanceCents)
	}
	if st.ChargesCents <= 0 || st.PaymentsCents <= 0 || st.CreditsCents <= 0 {
		t.Fatalf("bucket totals must all be populated: %+v", st)
	}
}

func TestCov3xHouseholdStatementAccessControl(t *testing.T) {
	svc := NewService()
	org, hh := cov3xStOrg, cov3xStHH
	cov3xInvoice(t, svc, org, hh, "INV-AC", "", 1000)

	if _, err := svc.HouseholdStatement(org, nil, hh, time.Time{}, cov3xFarFuture(t)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal: %v", err)
	}
	coach := &appmiddleware.Principal{UserID: "c", OrgID: org, Roles: []string{"coach"}}
	if _, err := svc.HouseholdStatement(org, coach, hh, time.Time{}, cov3xFarFuture(t)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	other := &appmiddleware.Principal{UserID: "par", OrgID: org, Roles: []string{"parent"}, HouseholdID: "hh-other"}
	if _, err := svc.HouseholdStatement(org, other, hh, time.Time{}, cov3xFarFuture(t)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign household parent: %v", err)
	}
	own := &appmiddleware.Principal{UserID: "par", OrgID: org, Roles: []string{"parent"}, HouseholdID: hh}
	st, err := svc.HouseholdStatement(org, own, hh, time.Time{}, cov3xFarFuture(t))
	if err != nil || st.OpenInvoicesDueCents != 1000 {
		t.Fatalf("own household parent: %+v %v", st, err)
	}
}

func TestCov3xHouseholdStatementWindowAndOrdering(t *testing.T) {
	svc := NewService()
	org, hh := cov3xStOrg, cov3xStHH
	p := cov3xAdmin(org)
	inv := cov3xInvoice(t, svc, org, hh, "INV-W", "", 10000)
	pay, _, err := svc.RecordPayment(org, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 4000, Method: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	// Pin timestamps so the window arithmetic is deterministic.
	issued := cov3xDay(t, "2026-10-01").Add(9 * time.Hour)
	paid := cov3xDay(t, "2026-10-20").Add(15 * time.Hour)
	svc.mu.Lock()
	svc.invoices[inv.ID].IssuedAt = &issued
	for _, pm := range svc.payments[inv.ID] {
		if pm.ID == pay.ID {
			pm.PaidAt = &paid
		}
	}
	svc.mu.Unlock()

	// Window starting after the invoice but containing the payment.
	st, err := svc.HouseholdStatement(org, p, hh, cov3xDay(t, "2026-10-10"), cov3xDay(t, "2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	if st.From != "2026-10-10" || st.To != "2026-10-31" {
		t.Fatalf("window labels: %+v", st)
	}
	if st.OpeningBalanceCents != 10000 || st.ChargesCents != 0 || st.PaymentsCents != 4000 || st.ClosingBalanceCents != 6000 {
		t.Fatalf("window totals: %+v", st)
	}
	if len(st.Lines) != 1 || st.Lines[0].Type != "payment" || st.Lines[0].BalanceCents != 6000 {
		t.Fatalf("window lines: %+v", st.Lines)
	}

	// The end date is inclusive of the whole day but excludes later activity.
	st, err = svc.HouseholdStatement(org, p, hh, time.Time{}, cov3xDay(t, "2026-10-19"))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Lines) != 1 || st.Lines[0].Type != "invoice" || st.ClosingBalanceCents != 10000 || st.PaymentsCents != 0 {
		t.Fatalf("early window: %+v", st)
	}
	st, err = svc.HouseholdStatement(org, p, hh, time.Time{}, cov3xDay(t, "2026-10-20"))
	if err != nil || len(st.Lines) != 2 || st.ClosingBalanceCents != 6000 {
		t.Fatalf("end-day inclusive: %+v %v", st, err)
	}

	// Same-instant events keep insertion order (invoice before its payment).
	svc.mu.Lock()
	for _, pm := range svc.payments[inv.ID] {
		pm.PaidAt = &issued
	}
	svc.mu.Unlock()
	st, err = svc.HouseholdStatement(org, p, hh, time.Time{}, cov3xFarFuture(t))
	if err != nil || len(st.Lines) != 2 || st.Lines[0].Type != "invoice" || st.Lines[1].Type != "payment" {
		t.Fatalf("tie must keep insertion order: %+v %v", st.Lines, err)
	}
}

// ─── Upcoming charges ────────────────────────────────────────────────────────

func cov3xUpcomingRoster() RosterSnapshot {
	return RosterSnapshot{
		Swimmers: []RosterSwimmer{
			{ID: "sw-a", HouseholdID: "hh-u", FirstName: "Ann", LastName: "Up", DateOfBirth: "2010-05-01", Status: "active"},
			{ID: "sw-b", HouseholdID: "hh-u", FirstName: "Ben", LastName: "Up", DateOfBirth: "2010-05-01", Status: "active"},
			{ID: "sw-d", HouseholdID: "hh-u", FirstName: "Dee", LastName: "Up", DateOfBirth: "2008-01-01", Status: "active"},
			{ID: "sw-c", HouseholdID: "hh-u", FirstName: "Cal", LastName: "Up", DateOfBirth: "2012-01-01", Status: "inactive"},
			{ID: "sw-x", HouseholdID: "hh-other", FirstName: "Xia", LastName: "Else", DateOfBirth: "2011-01-01", Status: "active"},
		},
		GroupLinks: []RosterGroupLink{
			{SwimmerID: "sw-a", GroupCode: "GOLD"}, {SwimmerID: "sw-b", GroupCode: "GOLD"}, {SwimmerID: "sw-d", GroupCode: "GOLD"},
			{SwimmerID: "sw-c", GroupCode: "GOLD"}, {SwimmerID: "sw-x", GroupCode: "GOLD"},
			{SwimmerID: "sw-ghost", GroupCode: "GOLD"}, // not on the roster at all
			{SwimmerID: "sw-a", GroupCode: "BRONZE"},   // a group with no schedule
		},
		Groups: []RosterGroup{{Code: "GOLD", Name: "Gold"}, {Code: "BRONZE", Name: "Bronze"}},
	}
}

func cov3xUpcomingService(t *testing.T) (*Service, *FeeSchedule) {
	t.Helper()
	svc := NewService()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return cov3xUpcomingRoster(), nil })
	org := "org-cov3x-up"
	p := cov3xAdmin(org)
	mk := func(in FeeScheduleInput) *FeeSchedule {
		t.Helper()
		sc, err := svc.CreateFeeSchedule(org, p.UserID, p, in)
		if err != nil {
			t.Fatalf("schedule %q: %v", in.Name, err)
		}
		return sc
	}
	monthly := mk(FeeScheduleInput{Name: "Gold dues", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 10000, StartDate: "2026-09-01"})
	mk(FeeScheduleInput{Name: "Gold future add-on", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 2000, StartDate: "2026-12-05"})
	archived := mk(FeeScheduleInput{Name: "Old dues", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 77701, StartDate: "2026-09-01"})
	if _, err := svc.ArchiveFeeSchedule(org, p.UserID, p, archived.ID); err != nil {
		t.Fatal(err)
	}
	mk(FeeScheduleInput{Name: "Gold one-time dues", Kind: "dues", GroupCode: "GOLD", Frequency: "one_time", AmountCents: 77702, StartDate: "2026-11-15"})
	mk(FeeScheduleInput{Name: "Season fee", Kind: "registration", Season: "2026-27", Frequency: "one_time", AmountCents: 77703, StartDate: "2026-09-01"})
	if _, err := svc.CreateFeeSchedule("org-cov3x-other", p.UserID, p, FeeScheduleInput{Name: "Other club", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 77704, StartDate: "2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	max2 := 2
	if _, err := svc.CreateDiscountRule(org, p.UserID, p, DiscountRuleInput{Name: "2nd", AppliesTo: "dues", MinPosition: 2, MaxPosition: &max2, DiscountType: "percent", PercentBps: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDiscountRule(org, p.UserID, p, DiscountRuleInput{Name: "3rd+", AppliesTo: "dues", MinPosition: 3, DiscountType: "percent", PercentBps: 2000}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDiscountRule("org-cov3x-other", p.UserID, p, DiscountRuleInput{Name: "foreign", AppliesTo: "dues", MinPosition: 2, DiscountType: "percent", PercentBps: 10000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AddManualCredit(org, p.UserID, p, "hh-u", ManualCreditInput{Direction: "credit", AmountCents: 3000, Reason: "Gift"}); err != nil {
		t.Fatal(err)
	}
	return svc, monthly
}

func TestCov3xHouseholdUpcomingChargesProjectionAndRanking(t *testing.T) {
	svc, _ := cov3xUpcomingService(t)
	org := "org-cov3x-up"
	p := cov3xAdmin(org)
	today := cov3xDay(t, "2026-11-10")

	up, err := svc.HouseholdUpcomingCharges(org, p, " hh-u ", today, 40)
	if err != nil {
		t.Fatal(err)
	}
	if up.HouseholdID != "hh-u" || up.Through != "2026-12-20" || up.CreditBalanceCents != 3000 {
		t.Fatalf("header: %+v", up)
	}
	// Totals tie at 22000 per swimmer, so rank by age (Dee, 2008) then ID (Ann, Ben):
	// Dee pays full, Ann gets 10% (position 2), Ben gets 20% (position 3).
	net := map[string]int{}
	for _, c := range up.Charges {
		net[c.PeriodStart+"|"+c.SwimmerName] += c.NetCents
		if c.NetCents != c.GrossCents-c.DiscountCents {
			t.Fatalf("net arithmetic: %+v", c)
		}
		if c.ScheduleName == "Gold future add-on" && c.PeriodStart != "2026-12-01" {
			t.Fatalf("a schedule starting 2026-12-05 projects its first (December) period: %+v", c)
		}
	}
	want := map[string]int{
		"2026-11-01|Ann Up": 9000, "2026-11-01|Ben Up": 8000, "2026-11-01|Dee Up": 10000,
		"2026-12-01|Ann Up": 9000 + 1800, "2026-12-01|Ben Up": 8000 + 1600, "2026-12-01|Dee Up": 10000 + 2000,
	}
	if len(net) != len(want) {
		t.Fatalf("charges by period/swimmer = %v, want %v", net, want)
	}
	total := 0
	for k, v := range want {
		if net[k] != v {
			t.Fatalf("%s net = %d want %d (all %v)", k, net[k], v, net)
		}
		total += v
	}
	if up.TotalCents != total || len(up.Charges) != 9 {
		t.Fatalf("total %d (want %d) over %d charges (want 9)", up.TotalCents, total, len(up.Charges))
	}
	// Sorted by period start, then swimmer name.
	for i := 1; i < len(up.Charges); i++ {
		a, b := up.Charges[i-1], up.Charges[i]
		if a.PeriodStart > b.PeriodStart || (a.PeriodStart == b.PeriodStart && a.SwimmerName > b.SwimmerName) {
			t.Fatalf("charges out of order at %d: %+v then %+v", i, a, b)
		}
	}
}

func TestCov3xHouseholdUpcomingChargesSkipsAlreadyBilledPeriods(t *testing.T) {
	svc, monthly := cov3xUpcomingService(t)
	org := "org-cov3x-up"
	p := cov3xAdmin(org)
	svc.mu.Lock()
	svc.finance.chargeKeys[chargeKey(org, monthly.ID, "2026-11-01", "hh-u", "sw-d")] = &BillingCharge{ID: "billed"}
	svc.mu.Unlock()

	up, err := svc.HouseholdUpcomingCharges(org, p, "hh-u", cov3xDay(t, "2026-11-10"), 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range up.Charges {
		if c.SwimmerID == "sw-d" && c.PeriodStart == "2026-11-01" && c.ScheduleID == monthly.ID {
			t.Fatalf("an already billed period must not be projected again: %+v", c)
		}
	}
	if len(up.Charges) != 8 {
		t.Fatalf("expected 8 remaining charges, got %d", len(up.Charges))
	}
}

func TestCov3xHouseholdUpcomingChargesHorizonDefaultsAndTieBreakByID(t *testing.T) {
	svc := NewService()
	org := "org-cov3x-up2"
	p := cov3xAdmin(org)
	svc.SetRosterSource(func(string) (RosterSnapshot, error) {
		return RosterSnapshot{
			Swimmers: []RosterSwimmer{
				{ID: "sw-2", HouseholdID: "hh-t", FirstName: "Bo", LastName: "T", DateOfBirth: "2010-01-01", Status: "active"},
				{ID: "sw-1", HouseholdID: "hh-t", FirstName: "Al", LastName: "T", DateOfBirth: "2010-01-01", Status: "active"},
			},
			GroupLinks: []RosterGroupLink{{SwimmerID: "sw-2", GroupCode: "TEAL"}, {SwimmerID: "sw-1", GroupCode: "TEAL"}},
			Groups:     []RosterGroup{{Code: "TEAL", Name: "Teal"}},
		}, nil
	})
	if _, err := svc.CreateFeeSchedule(org, p.UserID, p, FeeScheduleInput{Name: "Teal", Kind: "dues", GroupCode: "TEAL", Frequency: "monthly", AmountCents: 1000, StartDate: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDiscountRule(org, p.UserID, p, DiscountRuleInput{Name: "Sibling", AppliesTo: "dues", MinPosition: 2, DiscountType: "fixed", AmountCents: 250}); err != nil {
		t.Fatal(err)
	}
	for _, horizon := range []int{0, -5, 367} {
		up, err := svc.HouseholdUpcomingCharges(org, p, "hh-t", cov3xDay(t, "2026-11-10"), horizon)
		if err != nil {
			t.Fatal(err)
		}
		if up.Through != "2027-02-08" {
			t.Fatalf("horizon %d must default to 90 days, through=%s", horizon, up.Through)
		}
	}
	up, err := svc.HouseholdUpcomingCharges(org, p, "hh-t", cov3xDay(t, "2026-11-10"), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Identical totals and birthdays: the lower swimmer ID ranks first and pays full price.
	discount := map[string]int{}
	for _, c := range up.Charges {
		discount[c.SwimmerID] += c.DiscountCents
	}
	if discount["sw-1"] != 0 || discount["sw-2"] == 0 {
		t.Fatalf("sw-1 must rank first (no discount), sw-2 second: %v", discount)
	}
}

func TestCov3xHouseholdUpcomingChargesErrors(t *testing.T) {
	svc := NewService()
	org := "org-cov3x-up3"
	p := cov3xAdmin(org)
	today := cov3xDay(t, "2026-11-10")

	if _, err := svc.HouseholdUpcomingCharges(org, nil, "hh", today, 30); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal: %v", err)
	}
	foreign := &appmiddleware.Principal{UserID: "par", OrgID: org, Roles: []string{"parent"}, HouseholdID: "hh-mine"}
	if _, err := svc.HouseholdUpcomingCharges(org, foreign, "hh-theirs", today, 30); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign parent: %v", err)
	}
	if _, err := svc.HouseholdUpcomingCharges(org, p, "hh", today, 30); !errors.Is(err, ErrRosterUnavailable) {
		t.Fatalf("no roster source: %v", err)
	}
	boom := errors.New("roster backend down")
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, boom })
	if _, err := svc.HouseholdUpcomingCharges(org, p, "hh", today, 30); !errors.Is(err, boom) {
		t.Fatalf("roster error must be returned unchanged: %v", err)
	}
}
