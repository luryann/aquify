package billing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func cov1xIssued(t *testing.T, svc *Service, household, number string, cents int) *Invoice {
	t.Helper()
	return issuedInvoice(t, svc, cov1xAdmin, household, number, cents)
}

func cov1xDraft(t *testing.T, svc *Service, household, number string, cents int) *Invoice {
	t.Helper()
	inv, err := svc.CreateInvoice(cov1xOrg, "a", cov1xAdmin, CreateInvoiceInput{HouseholdID: household, InvoiceNumber: number})
	if err != nil {
		t.Fatal(err)
	}
	if cents > 0 {
		if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.GetInvoice(cov1xOrg, cov1xAdmin, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func cov1xPay(t *testing.T, svc *Service, invoiceID string, cents int) *Payment {
	t.Helper()
	p, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, invoiceID, RecordPaymentInput{AmountCents: cents, Method: "check"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func cov1xParentOf(household string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "parent-" + household, OrgID: cov1xOrg, Roles: []string{"parent"}, HouseholdID: household}
}

func TestCov1xRebuildIndexesSkipsNilEntriesAndDerivesAmounts(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.mu.Lock()
	defer svc.mu.Unlock()

	svc.invoices["inv-a"] = &Invoice{ID: "inv-a", OrgID: cov1xOrg, InvoiceNumber: "A", Status: "partially_paid", TotalCents: 10000}
	svc.invoices["inv-wo"] = &Invoice{ID: "inv-wo", OrgID: cov1xOrg, InvoiceNumber: "WO", Status: "written_off", TotalCents: 500, AmountDueCents: 500}
	svc.invoices["inv-over"] = &Invoice{ID: "inv-over", OrgID: cov1xOrg, InvoiceNumber: "OVER", Status: "paid", TotalCents: 100}
	svc.invoices["inv-refunded"] = &Invoice{ID: "inv-refunded", OrgID: cov1xOrg, InvoiceNumber: "REF", Status: "issued", TotalCents: 1000}
	svc.items["inv-a"] = []*InvoiceItem{nil, {ID: "item-1", InvoiceID: "inv-a"}}
	svc.payments["inv-a"] = []*Payment{
		{ID: "p-ok", InvoiceID: "inv-a", AmountCents: 6000, Status: "succeeded", StripePaymentIntentID: "pi_ok"},
		{ID: "p-rev", InvoiceID: "inv-a", AmountCents: 2000, Status: "reversed"},
		{ID: "p-fail", InvoiceID: "inv-a", AmountCents: 900, Status: "failed"},
	}
	svc.payments["inv-wo"] = []*Payment{{ID: "p-wo", InvoiceID: "inv-wo", AmountCents: 100, Status: "succeeded"}}
	svc.payments["inv-over"] = []*Payment{{ID: "p-over", InvoiceID: "inv-over", AmountCents: 500, Status: "succeeded"}}
	svc.refunds["inv-a"] = []*Refund{
		{ID: "r-done", InvoiceID: "inv-a", AmountCents: 1000, Status: "processed"},
		{ID: "r-pending", InvoiceID: "inv-a", AmountCents: 300, Status: "pending"},
	}
	svc.refunds["inv-refunded"] = []*Refund{{ID: "r-big", InvoiceID: "inv-refunded", AmountCents: 5000, Status: "processed"}}

	svc.rebuildIndexesLocked()

	if len(svc.itemsByID) != 1 || svc.itemsByID["item-1"] == nil {
		t.Fatalf("itemsByID: %+v", svc.itemsByID)
	}
	if len(svc.paymentsByID) != 5 || svc.stripeIntentIndex["pi_ok"] == nil || len(svc.stripeIntentIndex) != 1 {
		t.Fatalf("payment indexes: %d %v", len(svc.paymentsByID), svc.stripeIntentIndex)
	}
	if len(svc.refundsByID) != 3 || svc.refundsByID["r-done"] == nil {
		t.Fatalf("refundsByID: %+v", svc.refundsByID)
	}
	if !svc.invoiceNums[invoiceNumKey(cov1xOrg, "A")] || len(svc.invoiceNums) != 4 {
		t.Fatalf("invoiceNums: %v", svc.invoiceNums)
	}
	// 6000 + 2000 (reversed still counted as paid) - 1000 processed refund; pending refunds and failed payments ignored.
	if a := svc.invoices["inv-a"]; a.AmountPaidCents != 7000 || a.AmountDueCents != 3000 {
		t.Fatalf("derived amounts: paid=%d due=%d", a.AmountPaidCents, a.AmountDueCents)
	}
	if wo := svc.invoices["inv-wo"]; wo.AmountDueCents != 0 || wo.AmountPaidCents != 100 {
		t.Fatalf("written-off invoice must owe nothing: %+v", wo)
	}
	if o := svc.invoices["inv-over"]; o.AmountDueCents != 0 || o.AmountPaidCents != 500 {
		t.Fatalf("overpaid invoice must not owe a negative amount: %+v", o)
	}
	if r := svc.invoices["inv-refunded"]; r.AmountPaidCents != 0 || r.AmountDueCents != 1000 {
		t.Fatalf("paid must clamp at zero: %+v", r)
	}
}

func TestCov1xRoleScopeHelpers(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if id, scoped := svc.parentHouseholdScope(nil); id != "" || scoped {
		t.Fatal("nil principal is never parent scoped")
	}
	if _, scoped := svc.parentHouseholdScope(cov1xCoach); scoped {
		t.Fatal("coach is not parent scoped")
	}
	both := &appmiddleware.Principal{Roles: []string{"parent", "ADMIN"}, HouseholdID: "hh"}
	if _, scoped := svc.parentHouseholdScope(both); scoped {
		t.Fatal("admin role wins over parent")
	}
	if id, scoped := svc.parentHouseholdScope(&appmiddleware.Principal{Roles: []string{"Parent"}, HouseholdID: "  hh-1 "}); !scoped || id != "hh-1" {
		t.Fatalf("parent scope: %q %v", id, scoped)
	}

	if err := svc.requireAutopayReadAccess(cov1xCoach, "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach autopay read: %v", err)
	}
	if err := svc.requireAutopayReadAccess(cov1xParentOf("hh-b"), "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign household autopay read: %v", err)
	}
	if err := svc.requireAutopayReadAccess(cov1xAdmin, "hh-a"); err != nil {
		t.Fatalf("admin autopay read: %v", err)
	}
	if err := svc.requireAutopayWriteAccess(nil, "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil autopay write: %v", err)
	}
	if err := svc.requireAutopayWriteAccess(cov1xCoach, "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach autopay write: %v", err)
	}
	if err := svc.requireAutopayWriteAccess(cov1xParentOf("hh-b"), "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign household autopay write: %v", err)
	}
	if err := svc.requireAutopayWriteAccess(cov1xParentOf("hh-a"), "hh-a"); err != nil {
		t.Fatalf("own household autopay write: %v", err)
	}
	if err := svc.requireAutopayWriteAccess(cov1xAdmin, "any"); err != nil {
		t.Fatalf("admin autopay write: %v", err)
	}
	if err := svc.RequireBillingWriteAccess(cov1xParentOf("hh-a")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent billing write: %v", err)
	}
}

func TestCov1xValidateAutopayExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name         string
		month, year  int
		wantContains string
	}{
		{"unset", 0, 0, ""},
		{"month missing", 0, 2030, "payment_method_exp_month"},
		{"month too large", 13, 2030, "payment_method_exp_month"},
		{"year too small", 6, 1999, "payment_method_exp_year"},
		{"year too large", 6, 2201, "payment_method_exp_year"},
		{"expired year", 12, 2025, "expired"},
		{"expired month", 5, 2026, "expired"},
		{"current month ok", 6, 2026, ""},
		{"future ok", 1, 2030, ""},
	}
	for _, tc := range cases {
		err := validateAutopayExpiry(UpsertHouseholdAutopayInput{PaymentMethodExpMonth: tc.month, PaymentMethodExpYear: tc.year}, now)
		if tc.wantContains == "" {
			if err != nil {
				t.Fatalf("%s: unexpected %v", tc.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidFinanceInput) || !strings.Contains(err.Error(), tc.wantContains) {
			t.Fatalf("%s: got %v", tc.name, err)
		}
	}
}

func TestCov1xAutopayLifecycleAndNextChargePreview(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()
	input := UpsertHouseholdAutopayInput{PaymentMethodType: " CARD ", PaymentMethodLabel: " Visa 4242 ", PaymentMethodLast4: "4242", TriggerDaysBeforeDue: 3}

	if _, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xCoach, "hh-a", input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach upsert: %v", err)
	}
	if _, err := svc.OptOutHouseholdAutopay(cov1xOrg, "a", cov1xCoach, "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach opt-out: %v", err)
	}
	if _, err := svc.GetHouseholdAutopayStatus(cov1xOrg, cov1xCoach, "hh-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach get: %v", err)
	}
	if _, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, "hh-a", UpsertHouseholdAutopayInput{PaymentMethodType: "card"}); !errors.Is(err, ErrAutopayPaymentMethodRequired) {
		t.Fatalf("missing label: %v", err)
	}
	if _, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, "hh-a", UpsertHouseholdAutopayInput{PaymentMethodType: "card", PaymentMethodLabel: "x", PaymentMethodExpMonth: 13, PaymentMethodExpYear: 2030}); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("bad expiry: %v", err)
	}
	if _, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, "hh-zzz", input); !errors.Is(err, ErrAutopayHouseholdNotFound) {
		t.Fatalf("household missing from roster: %v", err)
	}

	// The household checker is consulted before anything else.
	svc.SetHouseholdChecker(func(_, id string) bool { return id != "hh-gone" })
	if _, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, "hh-gone", input); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("upsert checker: %v", err)
	}
	if _, err := svc.OptOutHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, "hh-gone"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("opt-out checker: %v", err)
	}
	if _, err := svc.GetHouseholdAutopayStatus(cov1xOrg, cov1xAdmin, "hh-gone"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("get checker: %v", err)
	}

	// An invoice with an unparseable due date is the only candidate: the raw value is reported.
	odd := cov1xIssued(t, svc, "hh-a", "ODD", 1000)
	svc.mu.Lock()
	svc.invoices[odd.ID].DueDate = "next-friday"
	svc.mu.Unlock()
	status, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xParentOf("hh-a"), "hh-a", input)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.PaymentMethodType != "card" || status.PaymentMethodLabel != "Visa 4242" || status.NextInvoiceID != odd.ID || status.NextChargeDate != "next-friday" {
		t.Fatalf("status with unparseable due date: %+v", status)
	}

	// Candidate selection: earliest due date among issued/partially paid invoices of this household and org.
	mk := func(num, due string, cents int) *Invoice {
		inv, err := svc.CreateInvoice(cov1xOrg, "a", cov1xAdmin, CreateInvoiceInput{HouseholdID: "hh-a", InvoiceNumber: num, DueDate: due})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, inv.ID, AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: cents, Category: "manual"}); err != nil {
			t.Fatal(err)
		}
		return inv
	}
	late := mk("LATE", "2026-12-20", 5000)
	early := mk("EARLY", "2026-12-10", 7000)
	noDue := mk("NODUE", "", 100)
	draft := mk("DRAFT", "2026-12-01", 100)
	for _, inv := range []*Invoice{late, early, noDue} {
		if _, _, err := svc.IssueInvoice(cov1xOrg, "a", cov1xAdmin, inv.ID); err != nil {
			t.Fatal(err)
		}
	}
	_ = draft
	// Other household / other org / settled invoices never count.
	otherHH := cov1xIssued(t, svc, "hh-b", "OTHERHH", 100)
	svc.mu.Lock()
	svc.invoices[otherHH.ID].DueDate = "2026-01-01"
	svc.invoices["foreign-org"] = &Invoice{ID: "foreign-org", OrgID: "other-org", HouseholdID: "hh-a", Status: "issued", AmountDueCents: 5, DueDate: "2026-01-01"}
	svc.mu.Unlock()
	svc.mu.Lock()
	svc.invoices[odd.ID].Status, svc.invoices[odd.ID].AmountDueCents = "paid", 0
	svc.mu.Unlock()

	got, err := svc.GetHouseholdAutopayStatus(cov1xOrg, cov1xParentOf("hh-a"), "hh-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.NextInvoiceID != early.ID || got.NextChargeAmountCents != 7000 || got.NextChargeDate != "2026-12-07" {
		t.Fatalf("next charge preview: %+v", got)
	}

	// Opting out keeps the saved method but stops the preview.
	off, err := svc.OptOutHouseholdAutopay(cov1xOrg, "a", cov1xParentOf("hh-a"), "hh-a")
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled || off.PaymentMethodLabel != "Visa 4242" || off.NextInvoiceID != "" {
		t.Fatalf("opt-out: %+v", off)
	}
	// Opting out for a household with no profile creates a disabled one.
	if st, err := svc.OptOutHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, "hh-b"); err != nil || st.Enabled {
		t.Fatalf("opt-out without profile: %+v %v", st, err)
	}
}

func TestCov1xListHouseholdAutopayStatuses(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()
	in := UpsertHouseholdAutopayInput{PaymentMethodType: "ach", PaymentMethodLabel: "Checking"}
	for _, hh := range []string{"hh-b", "hh-a"} {
		if _, err := svc.UpsertHouseholdAutopay(cov1xOrg, "a", cov1xAdmin, hh, in); err != nil {
			t.Fatal(err)
		}
	}
	svc.mu.Lock()
	svc.autopayProfiles["nil-profile"] = nil
	svc.autopayProfiles["foreign"] = &HouseholdAutopayProfile{OrgID: "other-org", HouseholdID: "hh-x", Enabled: true}
	svc.mu.Unlock()

	if _, err := svc.ListHouseholdAutopayStatuses(cov1xOrg, cov1xCoach, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	noHH := &appmiddleware.Principal{OrgID: cov1xOrg, Roles: []string{"parent"}}
	if _, err := svc.ListHouseholdAutopayStatuses(cov1xOrg, noHH, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent without household: %v", err)
	}
	// A parent only ever sees their own household, whatever they ask for.
	own, err := svc.ListHouseholdAutopayStatuses(cov1xOrg, cov1xParentOf("hh-a"), []string{"hh-b"})
	if err != nil || len(own) != 1 || own[0].HouseholdID != "hh-a" || !own[0].Enabled {
		t.Fatalf("parent list: %+v %v", own, err)
	}
	// Admin with no ids: every profile of the org, sorted; foreign orgs and nil entries skipped.
	all, err := svc.ListHouseholdAutopayStatuses(cov1xOrg, cov1xAdmin, nil)
	if err != nil || len(all) != 2 || all[0].HouseholdID != "hh-a" || all[1].HouseholdID != "hh-b" {
		t.Fatalf("admin list all: %+v %v", all, err)
	}
	// Explicit ids are trimmed and de-duplicated; unknown households report disabled.
	some, err := svc.ListHouseholdAutopayStatuses(cov1xOrg, cov1xAdmin, []string{" hh-b ", "", "hh-b", "hh-new"})
	if err != nil || len(some) != 2 || some[0].HouseholdID != "hh-b" || !some[0].Enabled || some[1].HouseholdID != "hh-new" || some[1].Enabled {
		t.Fatalf("admin list ids: %+v %v", some, err)
	}
}

func TestCov1xInvoiceReadsEnforceScopeAndFilters(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, err := svc.CreateInvoice(cov1xOrg, "a", cov1xAdmin, CreateInvoiceInput{InvoiceNumber: strings.Repeat("N", 201)}); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("long number: %v", err)
	}
	issuedA := cov1xIssued(t, svc, "hh-a", "A-1", 1000)
	draftA := cov1xDraft(t, svc, "hh-a", "A-2", 500)
	issuedB := cov1xIssued(t, svc, "hh-b", "B-1", 2000)
	svc.mu.Lock()
	svc.invoices["foreign"] = &Invoice{ID: "foreign", OrgID: "other-org", HouseholdID: "hh-a", Status: "issued"}
	svc.mu.Unlock()

	if _, err := svc.GetInvoice(cov1xOrg, cov1xCoach, issuedA.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach get: %v", err)
	}
	if _, err := svc.GetInvoice(cov1xOrg, cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.GetInvoice(cov1xOrg, cov1xAdmin, "foreign"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
	if _, err := svc.GetInvoice(cov1xOrg, cov1xParentOf("hh-a"), issuedB.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign household: %v", err)
	}
	if _, err := svc.GetInvoice(cov1xOrg, cov1xParentOf("hh-a"), draftA.ID); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("parents must not see drafts: %v", err)
	}
	if got, err := svc.GetInvoice(cov1xOrg, cov1xParentOf("hh-a"), issuedA.ID); err != nil || got.ID != issuedA.ID {
		t.Fatalf("parent own issued: %+v %v", got, err)
	}

	// ListInvoices scoping.
	noHH := &appmiddleware.Principal{OrgID: cov1xOrg, Roles: []string{"parent"}}
	if _, err := svc.ListInvoices(cov1xOrg, noHH, ListInvoicesFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent without household: %v", err)
	}
	if _, err := svc.ListInvoices(cov1xOrg, cov1xParentOf("hh-a"), ListInvoicesFilters{HouseholdID: "hh-b"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent foreign filter: %v", err)
	}
	if got, err := svc.ListInvoices(cov1xOrg, cov1xParentOf("hh-a"), ListInvoicesFilters{}); err != nil || len(got) != 1 || got[0].ID != issuedA.ID {
		t.Fatalf("parent list hides drafts and other households: %+v %v", got, err)
	}
	if got, _ := svc.ListInvoices(cov1xOrg, cov1xAdmin, ListInvoicesFilters{Status: "draft"}); len(got) != 1 || got[0].ID != draftA.ID {
		t.Fatalf("status filter: %+v", got)
	}
	if got, _ := svc.ListInvoices(cov1xOrg, cov1xAdmin, ListInvoicesFilters{HouseholdID: "hh-a"}); len(got) != 2 {
		t.Fatalf("household filter: %+v", got)
	}
	if got, _ := svc.ListInvoices(cov1xOrg, cov1xAdmin, ListInvoicesFilters{}); len(got) != 3 {
		t.Fatalf("admin sees own org only: %d", len(got))
	}

	// ListInvoiceItems.
	if _, err := svc.ListInvoiceItems(cov1xOrg, cov1xCoach, issuedA.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach items: %v", err)
	}
	if _, err := svc.ListInvoiceItems(cov1xOrg, cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("items missing: %v", err)
	}
	if _, err := svc.ListInvoiceItems(cov1xOrg, cov1xParentOf("hh-a"), issuedB.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("items foreign household: %v", err)
	}
	if _, err := svc.ListInvoiceItems(cov1xOrg, cov1xParentOf("hh-a"), draftA.ID); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("items of a draft are hidden from parents: %v", err)
	}
	if items, err := svc.ListInvoiceItems(cov1xOrg, cov1xParentOf("hh-a"), issuedA.ID); err != nil || len(items) != 1 || items[0].TotalCents != 1000 {
		t.Fatalf("parent items: %+v %v", items, err)
	}
}

func TestCov1xInvoicesByExternalRef(t *testing.T) {
	t.Parallel()
	svc := NewService()
	mk := func(num, ref string, age time.Duration) *Invoice {
		inv, err := svc.CreateInvoice(cov1xOrg, "a", cov1xAdmin, CreateInvoiceInput{HouseholdID: "hh-a", InvoiceNumber: num, ExternalRef: ref})
		if err != nil {
			t.Fatal(err)
		}
		svc.mu.Lock()
		svc.invoices[inv.ID].CreatedAt = time.Now().UTC().Add(-age)
		svc.mu.Unlock()
		return inv
	}
	newer := mk("N", "ext-1", time.Hour)
	older := mk("O", "ext-1", 3*time.Hour)
	mk("X", "ext-2", 0)
	svc.mu.Lock()
	svc.invoices["foreign"] = &Invoice{ID: "foreign", OrgID: "other-org", ExternalRef: "ext-1"}
	svc.mu.Unlock()

	if _, err := svc.InvoicesByExternalRef(cov1xOrg, cov1xCoach, "ext-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if got, err := svc.InvoicesByExternalRef(cov1xOrg, cov1xAdmin, ""); err != nil || got != nil {
		t.Fatalf("empty ref: %+v %v", got, err)
	}
	got, err := svc.InvoicesByExternalRef(cov1xOrg, cov1xAdmin, "ext-1")
	if err != nil || len(got) != 2 || got[0].ID != older.ID || got[1].ID != newer.ID {
		t.Fatalf("oldest first, own org only: %+v %v", got, err)
	}
}

func TestCov1xInvoiceItemGuards(t *testing.T) {
	t.Parallel()
	svc := NewService()
	draft := cov1xDraft(t, svc, "hh-a", "D", 0)
	issued := cov1xIssued(t, svc, "hh-a", "I", 1000)
	item := AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: 100, Category: "manual"}

	if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xCoach, draft.ID, item); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach add: %v", err)
	}
	for name, in := range map[string]AddInvoiceItemInput{
		"quantity":   {Description: "d", Quantity: 10001, UnitAmountCents: 1, Category: "manual"},
		"unit":       {Description: "d", Quantity: 1, UnitAmountCents: 100000001, Category: "manual"},
		"line total": {Description: "d", Quantity: 10000, UnitAmountCents: 99999999, Category: "manual"},
	} {
		if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, draft.ID, in); !errors.Is(err, ErrInvalidFinanceInput) {
			t.Fatalf("%s: expected too-large error, got %v", name, err)
		}
	}
	if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, "missing", item); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("add missing: %v", err)
	}
	if _, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, issued.ID, item); !errors.Is(err, ErrInvoiceNotDraft) {
		t.Fatalf("add to issued: %v", err)
	}
	added, err := svc.AddInvoiceItem(cov1xOrg, "a", cov1xAdmin, draft.ID, AddInvoiceItemInput{Description: "d", Quantity: 3, UnitAmountCents: 250, Category: "meet_fee", SwimmerID: "sw-1", MeetID: "m-1"})
	if err != nil || added.TotalCents != 750 || added.SwimmerID != "sw-1" || added.MeetID != "m-1" {
		t.Fatalf("add: %+v %v", added, err)
	}

	if err := svc.RemoveInvoiceItem(cov1xOrg, "a", cov1xCoach, draft.ID, added.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach remove: %v", err)
	}
	if err := svc.RemoveInvoiceItem(cov1xOrg, "a", cov1xAdmin, "missing", added.ID); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("remove from missing invoice: %v", err)
	}
	if err := svc.RemoveInvoiceItem(cov1xOrg, "a", cov1xAdmin, issued.ID, added.ID); !errors.Is(err, ErrInvoiceNotDraft) {
		t.Fatalf("remove from issued: %v", err)
	}
	if err := svc.RemoveInvoiceItem(cov1xOrg, "a", cov1xAdmin, draft.ID, "missing"); !errors.Is(err, ErrInvoiceItemNotFound) {
		t.Fatalf("remove missing item: %v", err)
	}
	other := cov1xDraft(t, svc, "hh-a", "D2", 400)
	if err := svc.RemoveInvoiceItem(cov1xOrg, "a", cov1xAdmin, other.ID, added.ID); !errors.Is(err, ErrInvoiceItemNotFound) {
		t.Fatalf("an item cannot be removed through another invoice: %v", err)
	}
	if err := svc.RemoveInvoiceItem(cov1xOrg, "a", cov1xAdmin, draft.ID, added.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, draft.ID); got.TotalCents != 0 {
		t.Fatalf("totals must be recomputed after removal: %+v", got)
	}
}

func TestCov1xInvoiceLifecycleGuards(t *testing.T) {
	t.Parallel()
	svc := NewService()
	empty := cov1xDraft(t, svc, "hh-a", "EMPTY", 0)
	draft := cov1xDraft(t, svc, "hh-a", "DRAFT", 1000)
	issued := cov1xIssued(t, svc, "hh-a", "ISSUED", 1000)

	for name, fn := range map[string]func() error{
		"issue":    func() error { _, _, err := svc.IssueInvoice(cov1xOrg, "a", cov1xCoach, draft.ID); return err },
		"void":     func() error { _, _, err := svc.VoidInvoice(cov1xOrg, "a", cov1xCoach, draft.ID); return err },
		"writeoff": func() error { _, _, err := svc.WriteOffInvoice(cov1xOrg, "a", cov1xCoach, draft.ID); return err },
		"payment": func() error {
			_, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xCoach, draft.ID, RecordPaymentInput{AmountCents: 1})
			return err
		},
	} {
		if err := fn(); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s as coach: %v", name, err)
		}
	}
	if _, _, err := svc.IssueInvoice(cov1xOrg, "a", cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("issue missing: %v", err)
	}
	if _, _, err := svc.IssueInvoice(cov1xOrg, "a", cov1xAdmin, issued.ID); !errors.Is(err, ErrInvoiceNotIssuable) {
		t.Fatalf("issue twice: %v", err)
	}
	if _, _, err := svc.IssueInvoice(cov1xOrg, "a", cov1xAdmin, empty.ID); !errors.Is(err, ErrInvoiceEmpty) {
		t.Fatalf("issue empty: %v", err)
	}

	if _, _, err := svc.VoidInvoice(cov1xOrg, "a", cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("void missing: %v", err)
	}
	paid := cov1xIssued(t, svc, "hh-a", "PAID", 500)
	cov1xPay(t, svc, paid.ID, 500)
	if _, _, err := svc.VoidInvoice(cov1xOrg, "a", cov1xAdmin, paid.ID); !errors.Is(err, ErrInvoiceNotVoidable) {
		t.Fatalf("void paid: %v", err)
	}
	withPayment := cov1xIssued(t, svc, "hh-a", "WITHPAY", 500)
	svc.mu.Lock()
	svc.invoices[withPayment.ID].AmountPaidCents = 100
	svc.mu.Unlock()
	if _, _, err := svc.VoidInvoice(cov1xOrg, "a", cov1xAdmin, withPayment.ID); err == nil || !strings.Contains(err.Error(), "existing payments") {
		t.Fatalf("void with payments: %v", err)
	}
	// Voiding a draft writes no ledger event; voiding an issued invoice writes one for its balance.
	if inv, ev, err := svc.VoidInvoice(cov1xOrg, "a", cov1xAdmin, draft.ID); err != nil || ev != nil || inv.Status != "voided" || inv.VoidedAt == nil {
		t.Fatalf("void draft: %+v %v %v", inv, ev, err)
	}
	if inv, ev, err := svc.VoidInvoice(cov1xOrg, "a", cov1xAdmin, issued.ID); err != nil || ev == nil || ev.AmountCents != -1000 || inv.Status != "voided" {
		t.Fatalf("void issued: %+v %+v %v", inv, ev, err)
	}
	zeroDue := cov1xIssued(t, svc, "hh-a", "ZERODUE", 700)
	svc.mu.Lock()
	svc.invoices[zeroDue.ID].AmountDueCents = 0
	svc.mu.Unlock()
	if _, ev, err := svc.VoidInvoice(cov1xOrg, "a", cov1xAdmin, zeroDue.ID); err != nil || ev != nil {
		t.Fatalf("void with nothing due writes no ledger event: %+v %v", ev, err)
	}

	if _, _, err := svc.WriteOffInvoice(cov1xOrg, "a", cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("write off missing: %v", err)
	}
	draft2 := cov1xDraft(t, svc, "hh-a", "DRAFT2", 100)
	if _, _, err := svc.WriteOffInvoice(cov1xOrg, "a", cov1xAdmin, draft2.ID); !errors.Is(err, ErrInvoiceNotWriteOffable) {
		t.Fatalf("write off draft: %v", err)
	}
	owing := cov1xIssued(t, svc, "hh-a", "OWING", 900)
	cov1xPay(t, svc, owing.ID, 400)
	inv, ev, err := svc.WriteOffInvoice(cov1xOrg, "a", cov1xAdmin, owing.ID)
	if err != nil || inv.Status != "written_off" || inv.AmountDueCents != 0 || ev.AmountCents != -500 {
		t.Fatalf("write off: %+v %+v %v", inv, ev, err)
	}
}

func TestCov1xRecordPaymentRules(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, "missing", RecordPaymentInput{AmountCents: 1}); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing: %v", err)
	}
	draft := cov1xDraft(t, svc, "hh-a", "D", 1000)
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, draft.ID, RecordPaymentInput{AmountCents: 100, Method: "cash"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pay a draft: %v", err)
	}

	inv := cov1xIssued(t, svc, "hh-a", "I", 1000)
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, inv.ID, RecordPaymentInput{AmountCents: 1500, Method: "cash"}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("overpay without opt-in: %v", err)
	}
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, inv.ID, RecordPaymentInput{AmountCents: 100, Method: "cash", PaidAt: "yesterday"}); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("bad paid_at: %v", err)
	}
	p, ev, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, inv.ID, RecordPaymentInput{
		AmountCents: 250, Method: "stripe", PaidAt: " 2026-10-31 ", StripePaymentIntentID: "pi_manual", ExternalRef: "ref-1", Notes: "n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.PaidAt == nil || p.PaidAt.Format("2006-01-02") != "2026-10-31" || ev.AmountCents != -250 {
		t.Fatalf("payment: %+v %+v", p, ev)
	}
	if got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, inv.ID); got.Status != "partially_paid" || got.AmountDueCents != 750 {
		t.Fatalf("invoice after partial payment: %+v", got)
	}
	svc.mu.RLock()
	indexed := svc.stripeIntentIndex["pi_manual"]
	svc.mu.RUnlock()
	if indexed == nil || indexed.ID != p.ID {
		t.Fatal("manual stripe payments must be indexed by intent")
	}

	// Excess is only allowed when the household can receive credit.
	noHH := cov1xIssued(t, svc, "", "NOHH", 500)
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, noHH.ID, RecordPaymentInput{AmountCents: 900, Method: "cash", ExcessToCredit: true}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("excess without household: %v", err)
	}
	svc.mu.Lock()
	svc.invoices[inv.ID].AmountDueCents = 0 // inconsistent cache: nothing left to apply the payment to
	svc.mu.Unlock()
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, inv.ID, RecordPaymentInput{AmountCents: 10, Method: "cash", ExcessToCredit: true}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("excess with nothing due: %v", err)
	}
	over := cov1xIssued(t, svc, "hh-a", "OVER", 500)
	if _, _, err := svc.RecordPayment(cov1xOrg, "a", cov1xAdmin, over.ID, RecordPaymentInput{AmountCents: 800, Method: "cash", ExcessToCredit: true, StripePaymentIntentID: "pi_over"}); err != nil {
		t.Fatal(err)
	}
	credit, err := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a")
	if err != nil || credit.BalanceCents != 300 {
		t.Fatalf("excess must land in household credit: %+v %v", credit, err)
	}
	if got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, over.ID); got.Status != "paid" || got.AmountDueCents != 0 {
		t.Fatalf("invoice after excess payment: %+v", got)
	}
}

func TestCov1xPaymentReadsScopeAndRedaction(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "A", 5000)
	foreign := cov1xIssued(t, svc, "hh-b", "B", 5000)
	pay, _, err := svc.RecordPayment(cov1xOrg, "staff-1", cov1xAdmin, inv.ID, RecordPaymentInput{AmountCents: 1000, Method: "check", ExternalRef: "chk-9", Notes: "staff note"})
	if err != nil {
		t.Fatal(err)
	}
	cov1xPay(t, svc, foreign.ID, 100)
	parent := cov1xParentOf("hh-a")

	// ListPayments.
	if _, err := svc.ListPayments(cov1xOrg, cov1xCoach, inv.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if _, err := svc.ListPayments(cov1xOrg, cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.ListPayments(cov1xOrg, parent, foreign.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign household: %v", err)
	}
	staff, _ := svc.ListPayments(cov1xOrg, cov1xAdmin, inv.ID)
	if len(staff) != 1 || staff[0].Notes != "staff note" || staff[0].ExternalRef != "chk-9" || staff[0].CreatedBy != "staff-1" {
		t.Fatalf("staff view: %+v", staff)
	}
	par, err := svc.ListPayments(cov1xOrg, parent, inv.ID)
	if err != nil || len(par) != 1 || par[0].Notes != "" || par[0].ExternalRef != "" || par[0].CreatedBy != "" || par[0].AmountCents != 1000 {
		t.Fatalf("parent view must be redacted: %+v %v", par, err)
	}

	// GetPayment.
	if _, err := svc.GetPayment(cov1xOrg, cov1xCoach, inv.ID, pay.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach get: %v", err)
	}
	if _, err := svc.GetPayment(cov1xOrg, cov1xAdmin, "missing", pay.ID); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("get missing invoice: %v", err)
	}
	if _, err := svc.GetPayment(cov1xOrg, parent, foreign.ID, pay.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("get foreign household: %v", err)
	}
	if _, err := svc.GetPayment(cov1xOrg, cov1xAdmin, inv.ID, "missing"); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("get missing payment: %v", err)
	}
	if _, err := svc.GetPayment(cov1xOrg, cov1xAdmin, foreign.ID, pay.ID); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("a payment cannot be read through another invoice: %v", err)
	}
	svc.mu.Lock()
	svc.paymentsByID["other-org-pay"] = &Payment{ID: "other-org-pay", OrgID: "other-org", InvoiceID: inv.ID}
	svc.mu.Unlock()
	if _, err := svc.GetPayment(cov1xOrg, cov1xAdmin, inv.ID, "other-org-pay"); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("cross-org payment: %v", err)
	}
	if got, err := svc.GetPayment(cov1xOrg, cov1xAdmin, inv.ID, pay.ID); err != nil || got.Notes != "staff note" {
		t.Fatalf("admin get: %+v %v", got, err)
	}
	if got, err := svc.GetPayment(cov1xOrg, parent, inv.ID, pay.ID); err != nil || got.Notes != "" || got.UpdatedBy != "" || got.ID != pay.ID {
		t.Fatalf("parent get must be redacted: %+v %v", got, err)
	}
}

func TestCov1xListPaymentsForOrgFiltersAndOrdering(t *testing.T) {
	t.Parallel()
	svc := NewService()
	invA := cov1xIssued(t, svc, "hh-a", "A", 9000)
	invB := cov1xIssued(t, svc, "hh-b", "B", 9000)
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := t0.Add(d); return &v }
	svc.mu.Lock()
	add := func(p *Payment) {
		p.OrgID = cov1xOrg
		svc.paymentsByID[p.ID] = p
	}
	add(&Payment{ID: "p1", InvoiceID: invA.ID, PaidAt: at(2 * time.Hour), CreatedAt: t0.Add(1 * time.Minute), Status: "succeeded", Notes: "secret"})
	add(&Payment{ID: "p2", InvoiceID: invA.ID, PaidAt: at(time.Hour), CreatedAt: t0.Add(2 * time.Minute), Status: "succeeded"})
	add(&Payment{ID: "p3", InvoiceID: invA.ID, PaidAt: at(time.Hour), CreatedAt: t0.Add(3 * time.Minute), Status: "failed"})
	add(&Payment{ID: "p4", InvoiceID: invA.ID, CreatedAt: t0.Add(4 * time.Minute), Status: "pending"})
	add(&Payment{ID: "p5", InvoiceID: invA.ID, CreatedAt: t0.Add(5 * time.Minute), Status: "pending"})
	add(&Payment{ID: "pb", InvoiceID: invB.ID, PaidAt: at(10 * time.Hour), CreatedAt: t0, Status: "succeeded"})
	add(&Payment{ID: "orphan", InvoiceID: "no-such-invoice", PaidAt: at(11 * time.Hour), CreatedAt: t0, Status: "succeeded"})
	svc.paymentsByID["foreign"] = &Payment{ID: "foreign", OrgID: "other-org", InvoiceID: invA.ID}
	svc.mu.Unlock()

	ids := func(ps []*Payment) string {
		out := make([]string, len(ps))
		for i, p := range ps {
			out[i] = p.ID
		}
		return strings.Join(out, ",")
	}
	if _, err := svc.ListPaymentsForOrg(cov1xOrg, cov1xCoach, ListPaymentsFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	noHH := &appmiddleware.Principal{OrgID: cov1xOrg, Roles: []string{"parent"}}
	if _, err := svc.ListPaymentsForOrg(cov1xOrg, noHH, ListPaymentsFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent without household: %v", err)
	}
	all, err := svc.ListPaymentsForOrg(cov1xOrg, cov1xAdmin, ListPaymentsFilters{})
	if err != nil {
		t.Fatal(err)
	}
	// Paid payments newest first (ties broken by creation time), unpaid ones last, newest first.
	if got := ids(all); got != "orphan,pb,p1,p3,p2,p5,p4" {
		t.Fatalf("order = %s", got)
	}
	if got, _ := svc.ListPaymentsForOrg(cov1xOrg, cov1xAdmin, ListPaymentsFilters{InvoiceID: invB.ID}); ids(got) != "pb" {
		t.Fatalf("invoice filter: %s", ids(got))
	}
	if got, _ := svc.ListPaymentsForOrg(cov1xOrg, cov1xAdmin, ListPaymentsFilters{Status: "failed"}); ids(got) != "p3" {
		t.Fatalf("status filter: %s", ids(got))
	}
	parent, err := svc.ListPaymentsForOrg(cov1xOrg, cov1xParentOf("hh-a"), ListPaymentsFilters{})
	if err != nil || ids(parent) != "p1,p3,p2,p5,p4" {
		t.Fatalf("parent scope must exclude other households and orphans: %s %v", ids(parent), err)
	}
	if parent[0].Notes != "" {
		t.Fatalf("parent rows must be redacted: %+v", parent[0])
	}
}

func TestCov1xRefundPreflightAndReads(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "A", 4000)
	pay := cov1xPay(t, svc, inv.ID, 3000)
	other := cov1xIssued(t, svc, "hh-a", "B", 1000)
	otherPay := cov1xPay(t, svc, other.ID, 1000)
	in := CreateRefundInput{PaymentID: pay.ID, AmountCents: 1000, Reason: "overcharged"}

	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xCoach, inv.ID, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 1, Reason: "  "}); !errors.Is(err, ErrRefundRequiresReason) {
		t.Fatalf("no reason: %v", err)
	}
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, "missing", in); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing invoice: %v", err)
	}
	unpaid := cov1xIssued(t, svc, "hh-a", "UNPAID", 100)
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, unpaid.ID, in); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("an issued invoice cannot be refunded: %v", err)
	}
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: "missing", AmountCents: 1, Reason: "r"}); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("missing payment: %v", err)
	}
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: otherPay.ID, AmountCents: 1, Reason: "r"}); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("payment of another invoice: %v", err)
	}
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 3001, Reason: "r"}); !errors.Is(err, ErrRefundExceedsPayment) {
		t.Fatalf("over-refund: %v", err)
	}
	svc.mu.Lock()
	svc.paymentsByID[pay.ID].Status = "failed"
	svc.mu.Unlock()
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, in); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("non-succeeded payment: %v", err)
	}
	svc.mu.Lock()
	svc.paymentsByID[pay.ID].Status = "succeeded"
	svc.paymentsByID[pay.ID].Method = PaymentMethodAccountCredit
	svc.mu.Unlock()
	if _, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, in); !errors.Is(err, ErrCreditPaymentRefundToCredit) {
		t.Fatalf("credit payments refund to credit only: %v", err)
	}
	in.ToAccountCredit = true
	pre, err := svc.ValidateRefundPreflight(cov1xOrg, cov1xAdmin, inv.ID, in)
	if err != nil || pre.Invoice.ID != inv.ID || pre.Payment.ID != pay.ID {
		t.Fatalf("preflight: %+v %v", pre, err)
	}

	// CreateRefund surfaces preflight failures and stores a refund readable by parents without staff fields.
	in.ToAccountCredit = false
	svc.mu.Lock()
	svc.paymentsByID[pay.ID].Method = "check"
	svc.mu.Unlock()
	if _, _, err := svc.CreateRefund(cov1xOrg, "a", cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 5000, Reason: "r"}); !errors.Is(err, ErrRefundExceedsPayment) {
		t.Fatalf("create over-refund: %v", err)
	}
	refund, _, err := svc.CreateRefund(cov1xOrg, "staff-2", cov1xAdmin, inv.ID, in)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ListRefunds(cov1xOrg, cov1xCoach, inv.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach list: %v", err)
	}
	if _, err := svc.ListRefunds(cov1xOrg, cov1xAdmin, "missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("list missing: %v", err)
	}
	if _, err := svc.ListRefunds(cov1xOrg, cov1xParentOf("hh-b"), inv.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list foreign: %v", err)
	}
	staff, _ := svc.ListRefunds(cov1xOrg, cov1xAdmin, inv.ID)
	if len(staff) != 1 || staff[0].Reason != "overcharged" || staff[0].RequestedBy != "staff-2" {
		t.Fatalf("staff refunds: %+v", staff)
	}
	par, err := svc.ListRefunds(cov1xOrg, cov1xParentOf("hh-a"), inv.ID)
	if err != nil || len(par) != 1 || par[0].ID != refund.ID || par[0].AmountCents != 1000 {
		t.Fatalf("parent refunds: %+v %v", par, err)
	}
	if r := par[0]; r.Reason != "" || r.RequestedBy != "" || r.ApprovedBy != "" || r.CreatedBy != "" || r.UpdatedBy != "" {
		t.Fatalf("parent refund must be redacted: %+v", r)
	}
}

func TestCov1xLedgerAndSnapshotsAreOrgScoped(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "A", 2000)
	pay := cov1xPay(t, svc, inv.ID, 500)
	if _, _, err := svc.CreateRefund(cov1xOrg, "a", cov1xAdmin, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 100, Reason: "r"}); err != nil {
		t.Fatal(err)
	}
	// Another club's rows exist in the same process and must never be returned.
	foreignAdmin := &appmiddleware.Principal{UserID: "x", OrgID: "other-org", Roles: []string{"admin"}}
	other := issuedInvoice(t, svc, foreignAdmin, "hh-z", "Z", 900)
	if _, _, err := svc.RecordPayment("other-org", "x", foreignAdmin, other.ID, RecordPaymentInput{AmountCents: 100, Method: "check"}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ListLedgerEvents(cov1xOrg, cov1xCoach, ListLedgerFilters{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach ledger: %v", err)
	}
	all, err := svc.ListLedgerEvents(cov1xOrg, cov1xAdmin, ListLedgerFilters{})
	if err != nil || len(all) != 3 {
		t.Fatalf("ledger: %d %v", len(all), err)
	}
	for _, ev := range all {
		if ev.OrgID != cov1xOrg {
			t.Fatalf("foreign ledger event leaked: %+v", ev)
		}
	}
	if got, _ := svc.ListLedgerEvents(cov1xOrg, cov1xAdmin, ListLedgerFilters{EventType: "refund_processed"}); len(got) != 1 {
		t.Fatalf("event type filter: %d", len(got))
	}
	if got, _ := svc.ListLedgerEvents(cov1xOrg, cov1xAdmin, ListLedgerFilters{InvoiceID: "nope"}); len(got) != 0 {
		t.Fatalf("invoice filter: %d", len(got))
	}

	if n := len(svc.SnapshotInvoices(cov1xOrg)); n != 1 {
		t.Fatalf("invoice snapshot: %d", n)
	}
	if n := len(svc.SnapshotInvoiceItems(cov1xOrg)); n != 1 {
		t.Fatalf("item snapshot: %d", n)
	}
	if n := len(svc.SnapshotPayments(cov1xOrg)); n != 1 {
		t.Fatalf("payment snapshot: %d", n)
	}
	if n := len(svc.SnapshotRefunds(cov1xOrg)); n != 1 {
		t.Fatalf("refund snapshot: %d", n)
	}
	if n := len(svc.SnapshotLedgerEvents(cov1xOrg)); n != 3 {
		t.Fatalf("ledger snapshot: %d", n)
	}
	if n := len(svc.LedgerEvents()); n != 5 {
		t.Fatalf("all ledger events: %d", n)
	}

	invs, items, pays, refs, led, err := svc.IntegritySnapshot(t.Context(), cov1xOrg)
	if err != nil {
		t.Fatal(err)
	}
	if len(invs) != 1 || len(items) != 1 || len(pays) != 1 || len(refs) != 1 || len(led) != 3 {
		t.Fatalf("integrity snapshot sizes: %d %d %d %d %d", len(invs), len(items), len(pays), len(refs), len(led))
	}
	if invs[0].OrgID != cov1xOrg || pays[0].OrgID != cov1xOrg || refs[0].OrgID != cov1xOrg || items[0].OrgID != cov1xOrg {
		t.Fatal("integrity snapshot must only contain the requested org")
	}
	invs[0].TotalCents = -1
	if got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, inv.ID); got.TotalCents != 2000 {
		t.Fatal("integrity snapshot must return copies")
	}
}

func TestCov1xWebhookEventsInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	payload := json.RawMessage(`{"id":"evt_1"}`)
	isNew, err := svc.RecordWebhookEvent("stripe", cov1xOrg, "evt_1", "payment_intent.succeeded", payload)
	if err != nil || !isNew {
		t.Fatalf("first claim: %v %v", isNew, err)
	}
	payload[2] = 'X' // the stored copy must not alias the caller's buffer
	if _, err := svc.RecordWebhookEvent("stripe", cov1xOrg, "evt_1", "payment_intent.succeeded", payload); !errors.Is(err, ErrWebhookInFlight) {
		t.Fatalf("active lease: %v", err)
	}
	// Unknown event: marking it creates a record, in memory.
	if err := svc.MarkWebhookProcessed("stripe", "evt_unknown", errors.New("boom"), true); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkWebhookProcessed("stripe", "evt_1", nil, false); err != nil {
		t.Fatal(err)
	}
	if isNew, err := svc.RecordWebhookEvent("stripe", cov1xOrg, "evt_1", "x", nil); err != nil || isNew {
		t.Fatalf("processed events are not re-claimed: %v %v", isNew, err)
	}
	if isNew, err := svc.RecordWebhookEvent("stripe", cov1xOrg, "evt_unknown", "x", nil); err != nil || !isNew {
		t.Fatalf("retryable failure is re-claimed: %v %v", isNew, err)
	}
	// Second org's event, then listing/filtering.
	if _, err := svc.RecordWebhookEvent("stripe", "other-org", "evt_other", "x", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListWebhookEvents(cov1xOrg, cov1xCoach); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	mine, err := svc.ListWebhookEvents(cov1xOrg, cov1xAdmin)
	if err != nil || len(mine) != 1 || mine[0].EventID != "evt_1" {
		t.Fatalf("org filter: %+v %v", mine, err)
	}
	all, err := svc.ListWebhookEvents("", cov1xAdmin)
	if err != nil || len(all) != 3 {
		t.Fatalf("empty org lists everything: %d %v", len(all), err)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].CreatedAt.Before(all[i].CreatedAt) {
			t.Fatal("events must be newest first")
		}
	}
}

func TestCov1xReconciliationReport(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, err := svc.GetReconciliationReport(cov1xOrg, cov1xCoach, 0, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	inv := cov1xIssued(t, svc, "hh-a", "A", 5000)
	if _, _, err := svc.HandleStripePaymentIntentSucceededOn(cov1xOrg, "acct_1", "pi_1", inv.ID, 3000, "stripe-webhook"); err != nil {
		t.Fatal(err)
	}
	other := cov1xIssued(t, svc, "hh-a", "B", 1000)
	if _, _, err := svc.HandleStripePaymentIntentSucceededOn(cov1xOrg, "acct_2", "pi_2", other.ID, 700, "stripe-webhook"); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	// A payment_succeeded event without a payment id, and credit rows that must not count.
	svc.ledger = append(svc.ledger, &LedgerEvent{ID: "e-nopay", OrgID: cov1xOrg, EventType: "payment_succeeded", AmountCents: -9999})
	svc.finance.credits = append(svc.finance.credits,
		&CreditEntry{OrgID: cov1xOrg, Kind: CreditKindManualCredit, AmountCents: 111, StripeRef: "pi_x"},
		&CreditEntry{OrgID: cov1xOrg, Kind: CreditKindOverpayment, AmountCents: -50, StripeRef: "pi_neg"},
		&CreditEntry{OrgID: cov1xOrg, Kind: CreditKindOverpayment, AmountCents: 60, StripeRef: ""},
		&CreditEntry{OrgID: cov1xOrg, Kind: CreditKindOverpayment, AmountCents: 70, StripeRef: "refund|pi_1|re_1"},
		&CreditEntry{OrgID: "other-org", Kind: CreditKindOverpayment, AmountCents: 80, StripeRef: "pi_other"},
		&CreditEntry{OrgID: cov1xOrg, Kind: CreditKindOverpayment, AmountCents: 400, StripeRef: "pi_credit"},
	)
	svc.mu.Unlock()

	svc.SetStripeAccountResolver(func(string) (string, error) { return "", errors.New("lookup down") })
	if _, err := svc.GetReconciliationReport(cov1xOrg, cov1xAdmin, 0, 0); err == nil || !strings.Contains(err.Error(), "lookup down") {
		t.Fatalf("resolver failure must surface: %v", err)
	}
	svc.SetStripeAccountResolver(func(string) (string, error) { return "acct_1", nil })
	rep, err := svc.GetReconciliationReport(cov1xOrg, cov1xAdmin, 3000, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 3000 (acct_1 payment) + 400 (credited overpayment) = 3400 vs 3000 reported.
	if rep.LedgerStripeTotalCents != 3400 || rep.MismatchCents != 400 || !rep.HasMismatch || !rep.AlertTriggered || rep.AlertThresholdCents != 100 {
		t.Fatalf("report: %+v", rep)
	}
	rep, err = svc.GetReconciliationReport(cov1xOrg, cov1xAdmin, 3500, 500)
	if err != nil {
		t.Fatal(err)
	}
	if rep.MismatchCents != 100 || !rep.HasMismatch || rep.AlertTriggered || rep.AlertThresholdCents != 500 {
		t.Fatalf("below threshold: %+v", rep)
	}
}

func TestCov1xStripeSuccessCreditRouting(t *testing.T) {
	t.Parallel()
	svc := NewService()
	noHH := cov1xIssued(t, svc, "", "NOHH", 1000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_a", noHH.ID, 1500, "wh"); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("excess without household on a payable invoice: %v", err)
	}
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_a", "missing", 100, "wh"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing invoice: %v", err)
	}
	cov1xPay(t, svc, noHH.ID, 1000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_b", noHH.ID, 100, "wh"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("paid invoice without household cannot take credit: %v", err)
	}

	// A paid invoice cannot take the money, so it is credited to the household, once per intent.
	paid := cov1xIssued(t, svc, "hh-a", "PAID", 1000)
	cov1xPay(t, svc, paid.ID, 1000)
	for i := 0; i < 2; i++ {
		p, ev, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_credit", paid.ID, 2500, "wh")
		if err != nil || p != nil || ev != nil {
			t.Fatalf("delivery %d: %+v %+v %v", i, p, ev, err)
		}
	}
	if c, _ := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a"); c.BalanceCents != 2500 || len(c.Entries) != 1 {
		t.Fatalf("credit must be recorded exactly once: %+v", c)
	}
	if err := svc.ensureStripeCreditLocked(&Invoice{OrgID: cov1xOrg, ID: paid.ID, HouseholdID: "hh-a"}, "pi_credit", "", 10, "wh", "dup"); err != nil {
		t.Fatalf("already credited intents are a no-op: %v", err)
	}
	if c, _ := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a"); c.BalanceCents != 2500 {
		t.Fatalf("no-op must not move the balance: %+v", c)
	}
	// Missing household: the credit entry itself is refused and the failure is wrapped.
	if err := svc.ensureStripeCreditLocked(&Invoice{OrgID: cov1xOrg, ID: paid.ID}, "pi_nohh", "", 10, "wh", "r"); err == nil || !strings.Contains(err.Error(), "record stripe credit") || !errors.Is(err, ErrHouseholdRequired) {
		t.Fatalf("credit failure: %v", err)
	}
	if _, _, err := svc.creditStripeExcessLocked(&Invoice{OrgID: cov1xOrg, ID: paid.ID, Status: "paid"}, "pi_nohh2", 10, "wh"); !errors.Is(err, ErrHouseholdRequired) {
		t.Fatalf("creditStripeExcessLocked failure: %v", err)
	}

	// Partially payable: the balance is applied, the excess goes to credit.
	due := cov1xIssued(t, svc, "hh-b", "DUE", 4000)
	pay, ev, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_mixed", due.ID, 5000, "wh")
	if err != nil || pay == nil || pay.AmountCents != 4000 || ev == nil {
		t.Fatalf("mixed payment: %+v %+v %v", pay, ev, err)
	}
	if c, _ := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-b"); c.BalanceCents != 1000 {
		t.Fatalf("excess credit: %+v", c)
	}
	// Redelivery returns the stored payment.
	if again, evAgain, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_mixed", due.ID, 5000, "wh"); err != nil || again.ID != pay.ID || evAgain != nil {
		t.Fatalf("redelivery: %+v %v", again, err)
	}
}

func TestCov1xStripeChargeRefundedPaths(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "A", 4000)
	if _, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_x", "missing", 100, "re_1", "", "wh"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing invoice: %v", err)
	}
	if _, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_x", inv.ID, 100, "re_1", "", "wh"); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("unknown intent: %v", err)
	}

	// Issued before any credit exists, so it cannot absorb credit later.
	otherInv := cov1xIssued(t, svc, "hh-a", "B", 100)
	// Intent paid 4000 plus 1000 credited (the invoice had 4000 due and 5000 arrived).
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_mix", inv.ID, 5000, "wh"); err != nil {
		t.Fatal(err)
	}
	// The intent belongs to a different invoice.
	if _, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_mix", otherInv.ID, 100, "re_1", "", "wh"); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("intent on another invoice: %v", err)
	}
	for _, amount := range []int{0, -5, 5001} {
		if _, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_mix", inv.ID, amount, "re_1", "", "wh"); !errors.Is(err, ErrRefundExceedsPayment) {
			t.Fatalf("amount %d: %v", amount, err)
		}
	}

	// Cumulative 4500 refunds the payment (4000) and reverses 500 of the credit.
	refund, ev, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_mix", inv.ID, 4500, "", "", "wh")
	if err != nil || refund == nil || ev == nil {
		t.Fatalf("refund: %+v %+v %v", refund, ev, err)
	}
	if refund.AmountCents != 4000 || refund.Reason != "Stripe charge.refunded webhook" || refund.StripeRefundID != "pi_mix-cumulative-4000" {
		t.Fatalf("refund fields: %+v", refund)
	}
	if c, _ := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a"); c.BalanceCents != 500 {
		t.Fatalf("credit after reversing 500 of 1000: %+v", c)
	}
	// Redelivery of the same cumulative amount is idempotent and returns the existing refund.
	again, evAgain, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_mix", inv.ID, 4500, "", "", "wh")
	if err != nil || evAgain != nil || again == nil || again.ID != refund.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if c, _ := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a"); c.BalanceCents != 500 {
		t.Fatalf("replay must not reverse credit twice: %+v", c)
	}
	// Refunding the rest reverses only what is left of the credit.
	if _, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_mix", inv.ID, 5000, "re_last", "", "wh"); err != nil {
		t.Fatal(err)
	}
	if c, _ := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a"); c.BalanceCents != 0 {
		t.Fatalf("credit fully reversed: %+v", c)
	}
}

func TestCov1xStripeChargeRefundedOfCreditOnlyPayment(t *testing.T) {
	t.Parallel()
	svc := NewService()
	paid := cov1xIssued(t, svc, "hh-a", "PAID", 1000)
	cov1xPay(t, svc, paid.ID, 1000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_credit", paid.ID, 3000, "wh"); err != nil {
		t.Fatal(err)
	}
	balance := func() int {
		c, err := svc.GetHouseholdCredit(cov1xOrg, cov1xAdmin, "hh-a")
		if err != nil {
			t.Fatal(err)
		}
		return c.BalanceCents
	}
	if balance() != 3000 {
		t.Fatalf("setup balance %d", balance())
	}
	refundCredit := func(cumulative int) {
		t.Helper()
		r, ev, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_credit", paid.ID, cumulative, "", "", "wh")
		if err != nil || r != nil || ev != nil {
			t.Fatalf("credit-only refund %d: %+v %+v %v", cumulative, r, ev, err)
		}
	}
	refundCredit(1000)
	if balance() != 2000 {
		t.Fatalf("after first reversal: %d", balance())
	}
	refundCredit(1000) // same cumulative id: already reversed
	if balance() != 2000 {
		t.Fatalf("replay must be a no-op: %d", balance())
	}
	refundCredit(2500) // only the 1500 delta is reversed
	if balance() != 500 {
		t.Fatalf("after second reversal: %d", balance())
	}

	// A household that already spent the credit can only give back what is left.
	invoice2 := cov1xIssued(t, svc, "hh-a", "NEXT", 5000)
	svc.mu.Lock()
	svc.finance.balances[balanceKey(cov1xOrg, "hh-a")] = 100
	svc.mu.Unlock()
	_ = invoice2
	refundCredit(3000)
	if balance() != 0 {
		t.Fatalf("reversal is clamped to the remaining balance: %d", balance())
	}
	// Nothing left to reverse is a quiet success.
	refundCredit(3000 - 0)
	if balance() != 0 {
		t.Fatalf("nothing left: %d", balance())
	}

	// reverseStripeCreditLocked refuses an intent that was never credited.
	svc.mu.Lock()
	err := svc.reverseStripeCreditLocked(&Invoice{OrgID: cov1xOrg, HouseholdID: "hh-a"}, "pi_never", "re_x", 100, "wh")
	svc.mu.Unlock()
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("never credited: %v", err)
	}
	// A foreign org's credit rows and rows without a ref are ignored.
	svc.mu.Lock()
	svc.finance.credits = append(svc.finance.credits,
		&CreditEntry{OrgID: "other-org", StripeRef: "pi_never", AmountCents: 50, Kind: CreditKindOverpayment},
		&CreditEntry{OrgID: cov1xOrg, StripeRef: "", AmountCents: 50, Kind: CreditKindManualCredit})
	err = svc.reverseStripeCreditLocked(&Invoice{OrgID: cov1xOrg, HouseholdID: "hh-a"}, "pi_never", "re_x", 100, "wh")
	svc.mu.Unlock()
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("foreign credit rows must not count: %v", err)
	}
}

func TestCov1xStripeChargeRefundedClampsDerivedAmounts(t *testing.T) {
	t.Parallel()
	svc := NewService()
	inv := cov1xIssued(t, svc, "hh-a", "A", 2000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_1", inv.ID, 2000, "wh"); err != nil {
		t.Fatal(err)
	}
	// A drifted cache: the invoice recorded less paid than the refund, and a total below the paid amount.
	svc.mu.Lock()
	svc.invoices[inv.ID].AmountPaidCents = 500
	svc.invoices[inv.ID].TotalCents = 100
	svc.mu.Unlock()
	refund, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_1", inv.ID, 1000, "re_1", "customer request", "wh")
	if err != nil || refund.Reason != "customer request" {
		t.Fatalf("refund: %+v %v", refund, err)
	}
	got, _ := svc.GetInvoice(cov1xOrg, cov1xAdmin, inv.ID)
	if got.AmountPaidCents != 0 || got.AmountDueCents != 100 || got.Status != "refunded_full" {
		t.Fatalf("paid must clamp at zero: %+v", got)
	}

	// Partial refund leaves the invoice partially refunded, with a negative due clamped to zero.
	inv2 := cov1xIssued(t, svc, "hh-a", "B", 2000)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_2", inv2.ID, 2000, "wh"); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.invoices[inv2.ID].TotalCents = 100
	svc.mu.Unlock()
	if _, _, err := svc.HandleStripeChargeRefunded(cov1xOrg, "pi_2", inv2.ID, 500, "re_2", "", "wh"); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetInvoice(cov1xOrg, cov1xAdmin, inv2.ID)
	if got.AmountPaidCents != 1500 || got.AmountDueCents != 0 || got.Status != "refunded_partial" {
		t.Fatalf("partial refund: %+v", got)
	}
}

func TestCov1xRecordedCreditIndexAndFailedPaymentNoop(t *testing.T) {
	t.Parallel()
	svc := NewService()
	paid := cov1xIssued(t, svc, "hh-a", "PAID", 500)
	cov1xPay(t, svc, paid.ID, 500)
	if _, _, err := svc.HandleStripePaymentIntentSucceeded(cov1xOrg, "pi_c", paid.ID, 200, "wh"); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.stripeCreditIndex = nil
	svc.rebuildStripeCreditIndexLocked()
	svc.finance.credits = append(svc.finance.credits, &CreditEntry{OrgID: cov1xOrg, Kind: CreditKindManualCredit, StripeRef: "pi_manual"})
	svc.rebuildStripeCreditIndexLocked()
	indexed := svc.stripeCreditIndex[cov1xOrg+"|pi_c"]
	manual := svc.stripeCreditIndex[cov1xOrg+"|pi_manual"]
	svc.mu.Unlock()
	if !indexed || manual {
		t.Fatalf("index must contain overpayment credits only: indexed=%v manual=%v", indexed, manual)
	}

	// Failed-payment events never downgrade a settled payment and ignore unknown intents.
	if err := svc.HandleStripePaymentIntentFailed("pi_unknown"); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleStripePaymentIntentFailedFor(cov1xOrg, "pi_unknown"); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.stripeIntentIndex["pi_pending"] = &Payment{ID: "p-pend", OrgID: cov1xOrg, Status: "pending", StripePaymentIntentID: "pi_pending"}
	svc.mu.Unlock()
	if err := svc.HandleStripePaymentIntentFailed("pi_pending"); err != nil {
		t.Fatal(err)
	}
	svc.mu.RLock()
	p := svc.stripeIntentIndex["pi_pending"]
	svc.mu.RUnlock()
	if p.Status != "failed" || p.FailedAt == nil {
		t.Fatalf("pending payment must be marked failed: %+v", p)
	}
}
