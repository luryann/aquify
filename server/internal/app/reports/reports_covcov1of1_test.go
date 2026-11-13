package reports

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/storage"
)

const cov1xOrg = "org-cov1x"

var cov1xAdmin = &appmiddleware.Principal{UserID: "cov1x-admin", Roles: []string{"admin"}, OrgID: cov1xOrg}

type cov1xEnv struct {
	t       *testing.T
	members *members.Service
	groups  *groups.Service
	meets   *meets.Service
	billing *billing.Service
	svc     *Service
	now     time.Time
}

func cov1xNewEnv(t *testing.T) *cov1xEnv {
	t.Helper()
	e := &cov1xEnv{
		t:       t,
		members: members.NewService(),
		groups:  groups.NewService(),
		meets:   meets.NewService(),
		billing: billing.NewService(),
		now:     time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
	}
	e.svc = NewService(e.members, e.groups, e.meets, e.billing)
	e.svc.nowFunc = func() time.Time { return e.now }
	return e
}

func cov1xMust[T any](t *testing.T, v T, err error) T {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (e *cov1xEnv) household(name, email string) members.Household {
	h, err := e.members.CreateHousehold(cov1xAdmin, members.CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: name, PrimaryEmail: email, PrimaryPhoneE164: "+15550000000"})
	return cov1xMust(e.t, h, err)
}

func (e *cov1xEnv) contact(hhID, first, email string, primary bool) members.Contact {
	c, err := e.members.AddContact(cov1xAdmin, cov1xOrg, hhID, members.AddContactInput{Relationship: "parent", FirstName: first, LastName: "Contact", Email: email, PhoneE164: "+1555" + first, IsPrimary: primary})
	return cov1xMust(e.t, c, err)
}

func (e *cov1xEnv) swimmer(hhID, first, last string) members.Swimmer {
	s, _, err := e.members.CreateSwimmer(cov1xAdmin, cov1xOrg, members.CreateSwimmerInput{HouseholdID: hhID, FirstName: first, LastName: last, DateOfBirth: "2012-03-04", Gender: "female"})
	return cov1xMust(e.t, s, err)
}

func (e *cov1xEnv) group(code, name string) groups.Group {
	g, _, err := e.groups.CreateGroup(cov1xAdmin, cov1xOrg, groups.CreateGroupInput{Code: code, Name: name})
	return cov1xMust(e.t, g, err)
}

func (e *cov1xEnv) assign(swimmerID, code string, primary bool) {
	e.t.Helper()
	if _, err := e.members.AssignSwimmerToGroup(cov1xAdmin, cov1xOrg, swimmerID, members.AssignSwimmerGroupInput{GroupCode: code, IsPrimary: primary}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *cov1xEnv) register(hh, sw, grp, season string) groups.Registration {
	e.groups.SeedSwimmer(cov1xOrg, sw, hh, "2012-03-04", "female", "active")
	r, err := e.groups.CreateRegistration(cov1xAdmin, cov1xOrg, groups.CreateRegistrationInput{HouseholdID: hh, SwimmerID: sw, GroupID: grp, Season: season, RegistrationDate: "2026-01-02"})
	return cov1xMust(e.t, r, err)
}

func (e *cov1xEnv) meet(name string) meets.Meet {
	m, _, err := e.meets.CreateMeet(cov1xAdmin, cov1xOrg, meets.CreateMeetInput{Name: name})
	return cov1xMust(e.t, m, err)
}

func (e *cov1xEnv) invoice(number, hh, due string) *billing.Invoice {
	inv, err := e.billing.CreateInvoice(cov1xOrg, "u", cov1xAdmin, billing.CreateInvoiceInput{HouseholdID: hh, InvoiceNumber: number, DueDate: due})
	return cov1xMust(e.t, inv, err)
}

func (e *cov1xEnv) item(invID string, in billing.AddInvoiceItemInput) {
	e.t.Helper()
	if _, err := e.billing.AddInvoiceItem(cov1xOrg, "u", cov1xAdmin, invID, in); err != nil {
		e.t.Fatal(err)
	}
}

func (e *cov1xEnv) issue(invID string) {
	e.t.Helper()
	if _, _, err := e.billing.IssueInvoice(cov1xOrg, "u", cov1xAdmin, invID); err != nil {
		e.t.Fatal(err)
	}
}

// issuedInvoice creates an issued invoice with one manual item of the given total.
func (e *cov1xEnv) issuedInvoice(number, hh, due string, cents int) *billing.Invoice {
	inv := e.invoice(number, hh, due)
	e.item(inv.ID, billing.AddInvoiceItemInput{Description: "dues", Quantity: 1, UnitAmountCents: cents, Category: "manual"})
	e.issue(inv.ID)
	return inv
}

func (e *cov1xEnv) pay(invID string, cents int) *billing.Payment {
	p, _, err := e.billing.RecordPayment(cov1xOrg, "u", cov1xAdmin, invID, billing.RecordPaymentInput{AmountCents: cents, Method: "check"})
	return cov1xMust(e.t, p, err)
}

// ---- access control ----

func TestCov1xReportsRequireAdmin(t *testing.T) {
	e := cov1xNewEnv(t)
	coach := &appmiddleware.Principal{UserID: "c", Roles: []string{"coach"}}
	runs := map[string]func(p *appmiddleware.Principal) error{
		"membership": func(p *appmiddleware.Principal) error {
			_, err := e.svc.RunMembershipStatusReport(cov1xOrg, p)
			return err
		},
		"meet_fees": func(p *appmiddleware.Principal) error { _, err := e.svc.RunMeetFeesReport(cov1xOrg, p); return err },
		"aging": func(p *appmiddleware.Principal) error {
			_, err := e.svc.RunInvoiceAgingReport(cov1xOrg, p, time.Time{})
			return err
		},
		"detail": func(p *appmiddleware.Principal) error {
			_, err := e.svc.RunInvoiceDetailReport(cov1xOrg, p, "x")
			return err
		},
		"recon": func(p *appmiddleware.Principal) error {
			_, err := e.svc.RunPaymentReconciliationReport(cov1xOrg, p, nil)
			return err
		},
		"registration": func(p *appmiddleware.Principal) error {
			_, err := e.svc.RunRegistrationStatusReport(cov1xOrg, p)
			return err
		},
	}
	for name, run := range runs {
		for label, p := range map[string]*appmiddleware.Principal{"nil": nil, "coach": coach, "no roles": {UserID: "x"}} {
			if err := run(p); !errors.Is(err, ErrForbidden) {
				t.Errorf("%s with %s principal: err = %v, want ErrForbidden", name, label, err)
			}
		}
	}
	// Role matching is case-insensitive and any admin role in the list suffices.
	if err := e.svc.requireReportAccess(&appmiddleware.Principal{Roles: []string{"coach", "ADMIN"}}); err != nil {
		t.Fatalf("mixed-case admin rejected: %v", err)
	}
}

// ---- membership status ----

func TestCov1xMembershipStatusReport(t *testing.T) {
	e := cov1xNewEnv(t)
	hhZ := e.household("Zeta", "zeta@example.test")
	hhA := e.household("Alpha", "alpha@example.test")
	e.contact(hhA.ID, "P1", "primary1@example.test", true)
	e.contact(hhA.ID, "P2", "primary2@example.test", true)
	e.contact(hhA.ID, "N1", "other@example.test", false)

	g1 := e.group("ELITE", "Elite Squad")
	e.group("ORPH", "Orphan Group")
	_ = g1
	swAnn := e.swimmer(hhA.ID, "Ann", "Alpha")
	swBob := e.swimmer(hhA.ID, "Bob", "Alpha")
	swZed := e.swimmer(hhZ.ID, "Zed", "Zeta")
	swSolo := e.swimmer(hhZ.ID, "Aaron", "Zeta")

	// Ann: one non-primary link first-come, one primary -> primary wins.
	e.assign(swAnn.ID, "ELITE", false)
	e.assign(swAnn.ID, "ORPH", true)
	// Bob: two non-primary links -> the first visited sticks, group count 2.
	e.assign(swBob.ID, "ELITE", false)
	e.assign(swBob.ID, "ORPH", false)
	// Zed: link to a code with no group definition.
	e.assign(swZed.ID, "GHOST", true)
	_ = swSolo

	rep, err := e.svc.RunMembershipStatusReport(cov1xOrg, cov1xAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if rep.RowCount != 4 || len(rep.Rows) != 4 || !rep.GeneratedAt.Equal(e.now) {
		t.Fatalf("report header = %+v", rep)
	}
	// Sorted by household name then swimmer name.
	var order []string
	for _, r := range rep.Rows {
		order = append(order, r.HouseholdName+"/"+r.SwimmerName)
	}
	want := []string{"Alpha/Ann Alpha", "Alpha/Bob Alpha", "Zeta/Aaron Zeta", "Zeta/Zed Zeta"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}

	ann := rep.Rows[0]
	if ann.PrimaryGroupCode != "ORPH" || ann.PrimaryGroupName != "Orphan Group" || ann.GroupCount != 2 {
		t.Fatalf("Ann group = %+v", ann)
	}
	if ann.PrimaryEmail != "primary1@example.test" && ann.PrimaryEmail != "primary2@example.test" {
		t.Fatalf("Ann email = %q, want a primary contact email", ann.PrimaryEmail)
	}
	if ann.HouseholdID != hhA.ID || ann.RegistrationStatus != "active" || ann.SwimmerID != swAnn.ID {
		t.Fatalf("Ann row = %+v", ann)
	}
	bob := rep.Rows[1]
	if bob.GroupCount != 2 || (bob.PrimaryGroupCode != "ELITE" && bob.PrimaryGroupCode != "ORPH") || bob.PrimaryGroupName == "" {
		t.Fatalf("Bob group = %+v", bob)
	}
	solo := rep.Rows[2]
	if solo.GroupCount != 0 || solo.PrimaryGroupCode != "" || solo.PrimaryEmail != "zeta@example.test" || solo.PrimaryPhoneE164 != "+15550000000" {
		t.Fatalf("household fallback row = %+v", solo)
	}
	zed := rep.Rows[3]
	if zed.PrimaryGroupCode != "GHOST" || zed.PrimaryGroupName != "" {
		t.Fatalf("Zed unknown group row = %+v", zed)
	}
}

// ---- meet fees ----

func TestCov1xMeetFeesReport(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Fees", "fees@example.test")
	sw := e.swimmer(hh.ID, "Fay", "Fees")
	mA := e.meet("Alpha Invitational")
	mB := e.meet("Beta Classic")

	inv2 := e.invoice("INV-2", hh.ID, "2026-07-01")
	inv1 := e.invoice("INV-1", hh.ID, "2026-07-01")
	e.item(inv1.ID, billing.AddInvoiceItemInput{Description: "alpha fee", Quantity: 2, UnitAmountCents: 1500, Category: "meet_fee", SwimmerID: sw.ID, MeetID: mA.ID})
	e.item(inv1.ID, billing.AddInvoiceItemInput{Description: "alpha fee 2", Quantity: 1, UnitAmountCents: 700, Category: "meet_fee", SwimmerID: sw.ID, MeetID: mA.ID})
	e.item(inv2.ID, billing.AddInvoiceItemInput{Description: "alpha fee other invoice", Quantity: 1, UnitAmountCents: 400, Category: "meet_fee", MeetID: mA.ID})
	e.item(inv2.ID, billing.AddInvoiceItemInput{Description: "beta fee", Quantity: 1, UnitAmountCents: 900, Category: "meet_fee", MeetID: mB.ID})
	e.item(inv2.ID, billing.AddInvoiceItemInput{Description: "not a meet fee", Quantity: 1, UnitAmountCents: 5000, Category: "equipment"})
	e.issue(inv1.ID)

	rep, err := e.svc.RunMeetFeesReport(cov1xOrg, cov1xAdmin)
	if err != nil {
		t.Fatal(err)
	}
	// 2*1500 + 700 + 400 + 900; the equipment item is excluded.
	if rep.RowCount != 4 || rep.TotalFeesCents != 5000 {
		t.Fatalf("rows = %d total = %d, want 4 / 5000", rep.RowCount, rep.TotalFeesCents)
	}
	// Order: meet name, then invoice number, then line item id.
	var names []string
	for _, r := range rep.Rows {
		names = append(names, r.MeetName+"|"+r.InvoiceNumber)
	}
	got := strings.Join(names, ",")
	if !strings.HasPrefix(got, "Alpha Invitational|INV-1,Alpha Invitational|INV-1,Alpha Invitational|INV-2,Beta Classic|INV-2") {
		t.Fatalf("order = %s", got)
	}
	first := rep.Rows[0]
	second := rep.Rows[1]
	if first.LineItemID >= second.LineItemID {
		t.Fatalf("line items of the same invoice not ordered by id: %q %q", first.LineItemID, second.LineItemID)
	}
	if first.SwimmerName != "Fay Fees" || first.InvoiceStatus != "issued" || first.IssuedAt == nil || first.DueDate != "2026-07-01" || first.HouseholdID != hh.ID {
		t.Fatalf("first row = %+v", first)
	}
	last := rep.Rows[3]
	if last.InvoiceStatus != "draft" || last.IssuedAt != nil || last.SwimmerName != "" {
		t.Fatalf("draft row = %+v", last)
	}
}

// ---- aging ----

func TestCov1xAgingBucketsAndTotals(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Aging", "aging@example.test")
	asOf := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)

	byDue := map[string]*billing.Invoice{}
	for _, due := range []string{"2026-06-10", "2026-05-01", "2026-04-01", "2026-03-01", "2025-01-01", "2026-07-01"} {
		byDue[due] = e.issuedInvoice("INV-"+due, hh.ID, due, 1000)
	}
	noDue := e.issuedInvoice("INV-NODUE", hh.ID, "", 700)

	paid := e.issuedInvoice("INV-PAID", hh.ID, "2026-01-01", 400)
	e.pay(paid.ID, 400)

	voided := e.issuedInvoice("INV-VOID", hh.ID, "2026-01-01", 300)
	if _, _, err := e.billing.VoidInvoice(cov1xOrg, "u", cov1xAdmin, voided.ID); err != nil {
		t.Fatal(err)
	}
	off := e.issuedInvoice("INV-OFF", hh.ID, "2026-01-01", 200)
	if _, _, err := e.billing.WriteOffInvoice(cov1xOrg, "u", cov1xAdmin, off.ID); err != nil {
		t.Fatal(err)
	}
	draft := e.invoice("INV-DRAFT", hh.ID, "2026-01-01")

	rep, err := e.svc.RunInvoiceAgingReport(cov1xOrg, cov1xAdmin, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if rep.AsOfDate != "2026-06-15" || rep.RowCount != 11 {
		t.Fatalf("header = %+v", rep)
	}
	if rep.OutstandingInvoiceCount != 7 || rep.OutstandingCents != 6*1000+700 {
		t.Fatalf("outstanding = %d / %d", rep.OutstandingInvoiceCount, rep.OutstandingCents)
	}
	byNumber := map[string]InvoiceAgingRow{}
	for _, r := range rep.Rows {
		byNumber[r.InvoiceNumber] = r
	}
	cases := []struct {
		number string
		bucket string
		days   int
		out    bool
	}{
		{"INV-2026-06-10", "1_30", 5, true},
		{"INV-2026-05-01", "31_60", 45, true},
		{"INV-2026-04-01", "61_90", 75, true},
		{"INV-2026-03-01", "90_plus", 106, true},
		{"INV-2025-01-01", "90_plus", 530, true},
		{"INV-2026-07-01", "current", 0, true},
		{"INV-NODUE", "no_due_date", 0, true},
		{"INV-PAID", "settled", 165, false},
		{"INV-VOID", "voided", 0, false},
		{"INV-OFF", "written_off", 0, false},
		{"INV-DRAFT", "draft", 0, false},
	}
	for _, c := range cases {
		r, ok := byNumber[c.number]
		if !ok {
			t.Fatalf("missing row %s", c.number)
		}
		if r.AgingBucket != c.bucket || r.DaysPastDue != c.days || r.IsOutstanding != c.out {
			t.Errorf("%s = bucket %q days %d outstanding %v; want %q %d %v", c.number, r.AgingBucket, r.DaysPastDue, r.IsOutstanding, c.bucket, c.days, c.out)
		}
	}
	_ = noDue
	_ = draft
	// Most overdue first.
	if rep.Rows[0].InvoiceNumber != "INV-2025-01-01" {
		t.Fatalf("first row = %s, want the most overdue invoice", rep.Rows[0].InvoiceNumber)
	}
	paidRow := byNumber["INV-PAID"]
	if paidRow.AmountPaidCents != 400 || paidRow.AmountDueCents != 0 || paidRow.TotalCents != 400 || paidRow.HouseholdID != hh.ID {
		t.Fatalf("paid row = %+v", paidRow)
	}
}

func TestCov1xAgingTiesSortByInvoiceNumberAndDefaultsAsOf(t *testing.T) {
	e := cov1xNewEnv(t) // now = 2026-06-15 12:00 UTC
	hh := e.household("Ties", "t@example.test")
	e.issuedInvoice("INV-B", hh.ID, "2026-06-01", 100)
	e.issuedInvoice("INV-A", hh.ID, "2026-06-01", 100)
	rep, err := e.svc.RunInvoiceAgingReport(cov1xOrg, cov1xAdmin, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.AsOfDate != "2026-06-15" {
		t.Fatalf("as-of defaulted to %q, want the service clock date", rep.AsOfDate)
	}
	if len(rep.Rows) != 2 || rep.Rows[0].InvoiceNumber != "INV-A" || rep.Rows[1].InvoiceNumber != "INV-B" || rep.Rows[0].DaysPastDue != 14 {
		t.Fatalf("rows = %+v", rep.Rows)
	}
}

func TestCov1xAgingBucketAndParseDate(t *testing.T) {
	for _, c := range []struct {
		days, due int
		want      string
	}{{50, 0, "settled"}, {0, 5, "current"}, {-3, 5, "current"}, {1, 5, "1_30"}, {30, 5, "1_30"}, {31, 5, "31_60"}, {60, 5, "31_60"}, {61, 5, "61_90"}, {90, 5, "61_90"}, {91, 5, "90_plus"}, {10, -5, "settled"}} {
		if got := agingBucket(c.days, c.due); got != c.want {
			t.Errorf("agingBucket(%d,%d) = %q, want %q", c.days, c.due, got, c.want)
		}
	}
	for _, bad := range []string{"", "   ", "not-a-date", "2026/01/02", "2026-13-40"} {
		if _, ok := parseDate(bad); ok {
			t.Errorf("parseDate(%q) ok, want false", bad)
		}
	}
	got, ok := parseDate(" 2026-01-02 ")
	if !ok || !got.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseDate = %v %v", got, ok)
	}
}

// ---- invoice detail ----

func TestCov1xInvoiceDetailReport(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Detail", "d@example.test")
	sw := e.swimmer(hh.ID, "Dee", "Detail")
	inv := e.invoice("INV-D", hh.ID, "2026-07-01")
	other := e.issuedInvoice("INV-OTHER", hh.ID, "2026-07-01", 999)
	otherPay := e.pay(other.ID, 100)
	if _, _, err := e.billing.CreateRefund(cov1xOrg, "u", cov1xAdmin, other.ID, billing.CreateRefundInput{PaymentID: otherPay.ID, AmountCents: 50, Reason: "other invoice refund"}); err != nil {
		t.Fatal(err)
	}
	e.item(inv.ID, billing.AddInvoiceItemInput{Description: "with swimmer", Quantity: 2, UnitAmountCents: 1000, Category: "program_fee", SwimmerID: sw.ID})
	e.item(inv.ID, billing.AddInvoiceItemInput{Description: "unknown swimmer", Quantity: 1, UnitAmountCents: 500, Category: "manual", SwimmerID: "swm_missing"})
	e.item(inv.ID, billing.AddInvoiceItemInput{Description: "no swimmer", Quantity: 1, UnitAmountCents: 500, Category: "manual"})
	e.issue(inv.ID)
	pay := e.pay(inv.ID, 1500)
	if _, _, err := e.billing.CreateRefund(cov1xOrg, "u", cov1xAdmin, inv.ID, billing.CreateRefundInput{PaymentID: pay.ID, AmountCents: 250, Reason: "duplicate"}); err != nil {
		t.Fatal(err)
	}

	rep, err := e.svc.RunInvoiceDetailReport(cov1xOrg, cov1xAdmin, "  "+inv.ID+"  ")
	if err != nil {
		t.Fatal(err)
	}
	if rep.InvoiceID != inv.ID || rep.InvoiceNumber != "INV-D" || rep.HouseholdID != hh.ID || rep.RowCount != 5 || len(rep.Rows) != 5 {
		t.Fatalf("report = %+v", rep)
	}
	byDesc := map[string]InvoiceDetailRow{}
	for _, r := range rep.Rows {
		byDesc[r.Description] = r
	}
	if r := byDesc["with swimmer"]; r.RecordType != "item" || r.SwimmerName != "Dee Detail" || r.Quantity != 2 || r.AmountCents != 2000 || r.Status != "program_fee" || r.Date == "" {
		t.Fatalf("swimmer item = %+v", r)
	}
	if r := byDesc["unknown swimmer"]; r.RecordType != "item" || r.SwimmerName != "" {
		t.Fatalf("unknown swimmer item = %+v", r)
	}
	if r := byDesc["no swimmer"]; r.RecordType != "item" || r.SwimmerName != "" || r.AmountCents != 500 {
		t.Fatalf("no swimmer item = %+v", r)
	}
	if r := byDesc["check"]; r.RecordType != "payment" || r.AmountCents != 1500 || r.Status == "" {
		t.Fatalf("payment = %+v", r)
	}
	if r := byDesc["duplicate"]; r.RecordType != "refund" || r.AmountCents != 250 || r.Status == "" {
		t.Fatalf("refund = %+v", r)
	}
}

func TestCov1xInvoiceDetailErrors(t *testing.T) {
	e := cov1xNewEnv(t)
	if _, err := e.svc.RunInvoiceDetailReport(cov1xOrg, cov1xAdmin, "   "); !errors.Is(err, ErrInvoiceIDRequired) {
		t.Fatalf("blank id err = %v", err)
	}
	if _, err := e.svc.RunInvoiceDetailReport(cov1xOrg, cov1xAdmin, "inv_missing"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	// An invoice from another org is not visible.
	inv := e.invoice("INV-X", "", "")
	if _, err := e.svc.RunInvoiceDetailReport("org-other", &appmiddleware.Principal{UserID: "a", Roles: []string{"admin"}}, inv.ID); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("cross-org err = %v", err)
	}
}

// ---- payment reconciliation ----

func TestCov1xPaymentReconciliationReconciledAndStripe(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Recon", "r@example.test")
	inv := e.issuedInvoice("INV-R", hh.ID, "2026-07-01", 5000)
	p := e.pay(inv.ID, 3000)
	if _, _, err := e.billing.CreateRefund(cov1xOrg, "u", cov1xAdmin, inv.ID, billing.CreateRefundInput{PaymentID: p.ID, AmountCents: 1000, Reason: "partial"}); err != nil {
		t.Fatal(err)
	}

	rep, err := e.svc.RunPaymentReconciliationReport(cov1xOrg, cov1xAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.RowCount != 1 || rep.PaymentTotalCents != 3000 || rep.RefundTotalCents != 1000 || rep.NetCollectedCents != 2000 || rep.UnreconciledRowCount != 0 {
		t.Fatalf("report = %+v", rep)
	}
	row := rep.Rows[0]
	if !row.IsReconciled || row.ReconciliationNotes != "" || row.LedgerPaymentCents != 3000 || row.LedgerRefundCents != 1000 || row.InvoiceNumber != "INV-R" || row.Method != "check" {
		t.Fatalf("row = %+v", row)
	}
	if rep.StripeReportedCents != nil || rep.StripeReconciliationError != "" {
		t.Fatalf("stripe fields set without a request: %+v", rep)
	}

	reported := 3000
	rep, err = e.svc.RunPaymentReconciliationReport(cov1xOrg, cov1xAdmin, &reported)
	if err != nil {
		t.Fatal(err)
	}
	if rep.StripeReportedCents == nil || *rep.StripeReportedCents != 3000 || rep.StripeMismatchCents == nil || *rep.StripeMismatchCents != 3000 || rep.StripeAlertTriggered == nil || !*rep.StripeAlertTriggered {
		t.Fatalf("stripe comparison = %+v", rep)
	}
}

func TestCov1xPaymentReconciliationStripeError(t *testing.T) {
	e := cov1xNewEnv(t)
	e.billing.SetStripeAccountResolver(func(string) (string, error) { return "", errors.New("connect lookup failed") })
	reported := 10
	rep, err := e.svc.RunPaymentReconciliationReport(cov1xOrg, cov1xAdmin, &reported)
	if err != nil {
		t.Fatalf("a stripe failure must not fail the report: %v", err)
	}
	if rep.StripeReconciliationError != "stripe reconciliation unavailable: connect lookup failed" {
		t.Fatalf("error = %q", rep.StripeReconciliationError)
	}
	if rep.StripeReportedCents != nil || rep.StripeMismatchCents != nil || rep.StripeAlertTriggered != nil {
		t.Fatalf("stripe fields must be unset on error: %+v", rep)
	}
}

func TestCov1xPaymentReconciliationFlagsLedgerMismatches(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Mismatch", "m@example.test")
	good := e.issuedInvoice("INV-A-GOOD", hh.ID, "2026-07-01", 1000)
	e.pay(good.ID, 1000)
	bad := e.issuedInvoice("INV-B-BAD", hh.ID, "2026-07-01", 4000)
	badPay := e.pay(bad.ID, 4000)
	if _, _, err := e.billing.CreateRefund(cov1xOrg, "u", cov1xAdmin, bad.ID, billing.CreateRefundInput{PaymentID: badPay.ID, AmountCents: 500, Reason: "oops"}); err != nil {
		t.Fatal(err)
	}
	// A payment applied from account credit carries a credit_applied ledger
	// event instead of payment_succeeded; simulate the ledger of one payment
	// being lost and another being recorded as credit_applied.
	creditInv := e.issuedInvoice("INV-C-CREDIT", hh.ID, "2026-07-01", 800)
	creditPay := e.pay(creditInv.ID, 800)

	raw, err := e.billing.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	var ledger []map[string]any
	if err := json.Unmarshal(snap["ledger"], &ledger); err != nil {
		t.Fatal(err)
	}
	kept := ledger[:0]
	for _, ev := range ledger {
		if ev["payment_id"] == badPay.ID {
			continue // drop both the payment and the refund ledger rows for the bad payment
		}
		if ev["payment_id"] == creditPay.ID {
			ev["event_type"] = "credit_applied"
		}
		kept = append(kept, ev)
	}
	snap["ledger"], _ = json.Marshal(kept)
	raw, _ = json.Marshal(snap)
	if err := e.billing.LoadRuntimeSnapshot(raw); err != nil {
		t.Fatal(err)
	}

	rep, err := e.svc.RunPaymentReconciliationReport(cov1xOrg, cov1xAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.RowCount != 3 || rep.UnreconciledRowCount != 1 || rep.PaymentTotalCents != 5800 || rep.RefundTotalCents != 500 || rep.NetCollectedCents != 5300 {
		t.Fatalf("report = %+v", rep)
	}
	first := rep.Rows[0]
	if first.IsReconciled || first.InvoiceNumber != "INV-B-BAD" || first.LedgerPaymentCents != 0 || first.LedgerRefundCents != 0 || first.RefundedCents != 500 || first.NetCollectedCents != 3500 {
		t.Fatalf("unreconciled row must sort first: %+v", first)
	}
	if first.ReconciliationNotes != "payment ledger 0 != payment amount 4000; refund ledger 0 != refund total 500" {
		t.Fatalf("notes = %q", first.ReconciliationNotes)
	}
	if !rep.Rows[1].IsReconciled || rep.Rows[1].InvoiceNumber != "INV-A-GOOD" || rep.Rows[2].InvoiceNumber != "INV-C-CREDIT" || !rep.Rows[2].IsReconciled {
		t.Fatalf("reconciled rows = %+v", rep.Rows[1:])
	}
}

// ---- registration status ----

func TestCov1xRegistrationStatusReport(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Regs", "regs@example.test")
	swA := e.swimmer(hh.ID, "Amy", "Regs")
	swB := e.swimmer(hh.ID, "Ben", "Regs")
	gX := e.group("XG", "X Group")
	r1 := e.register(hh.ID, swA.ID, gX.ID, "2026")
	e.register(hh.ID, swB.ID, gX.ID, "2026")
	for i := 0; i < 2; i++ {
		if _, err := e.groups.AcceptWaiver(cov1xAdmin, cov1xOrg, r1.ID, groups.AcceptWaiverInput{WaiverType: []string{"liability", "photo"}[i]}); err != nil {
			t.Fatal(err)
		}
	}

	rep, err := e.svc.RunRegistrationStatusReport(cov1xOrg, cov1xAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if rep.RowCount != 2 || !rep.GeneratedAt.Equal(e.now) {
		t.Fatalf("report = %+v", rep)
	}
	amy, ben := rep.Rows[0], rep.Rows[1]
	if amy.SwimmerName != "Amy Regs" || ben.SwimmerName != "Ben Regs" {
		t.Fatalf("same season+group must sort by swimmer name: %q, %q", amy.SwimmerName, ben.SwimmerName)
	}
	if amy.RegistrationID != r1.ID || amy.WaiverCount != 2 || amy.LastWaiverAcceptedAt == nil || amy.GroupCode != "XG" || amy.GroupName != "X Group" || amy.HouseholdName != "Regs" || amy.Season != "2026" || amy.RegistrationDate != "2026-01-02" || amy.GroupID != gX.ID || amy.HouseholdID != hh.ID || amy.SwimmerID != swA.ID {
		t.Fatalf("Amy row = %+v", amy)
	}
	if ben.WaiverCount != 0 || ben.LastWaiverAcceptedAt != nil {
		t.Fatalf("Ben row = %+v", ben)
	}
}

func TestCov1xRegistrationStatusSortsBySeasonThenGroup(t *testing.T) {
	e := cov1xNewEnv(t)
	hh := e.household("Sorts", "s@example.test")
	sw := e.swimmer(hh.ID, "Sam", "Sorts")
	gB := e.group("BG", "B Group")
	gA := e.group("AG", "A Group")
	e.register(hh.ID, sw.ID, gB.ID, "2027")
	e.register(hh.ID, sw.ID, gB.ID, "2026")
	e.register(hh.ID, sw.ID, gA.ID, "2026")

	rep, err := e.svc.RunRegistrationStatusReport(cov1xOrg, cov1xAdmin)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rep.Rows {
		got = append(got, r.Season+"/"+r.GroupName)
	}
	if strings.Join(got, ",") != "2026/A Group,2026/B Group,2027/B Group" {
		t.Fatalf("order = %v", got)
	}
}

// ---- artifacts ----

type cov1xStore struct {
	storage.ObjectStorage
	putErr, delErr, presignErr error
}

func (c *cov1xStore) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if c.putErr != nil {
		return c.putErr
	}
	return c.ObjectStorage.PutObject(ctx, key, ct, data)
}

func (c *cov1xStore) Delete(key string) error {
	if c.delErr != nil {
		return c.delErr
	}
	return c.ObjectStorage.Delete(key)
}

func (c *cov1xStore) PresignedGetURL(key string, ttl time.Duration) (string, error) {
	if c.presignErr != nil {
		return "", c.presignErr
	}
	return c.ObjectStorage.PresignedGetURL(key, ttl)
}

func TestCov1xSetStorageAndTTLIgnoreInvalidValues(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	orig := svc.store
	svc.SetStorage(nil)
	if svc.store != orig {
		t.Fatal("SetStorage(nil) replaced the store")
	}
	replacement := storage.NewStubStorage("http://example.test/b")
	svc.SetStorage(replacement)
	if svc.store != storage.ObjectStorage(replacement) {
		t.Fatal("SetStorage did not apply")
	}

	svc.ConfigureArtifactTTL(0)
	svc.ConfigureArtifactTTL(-time.Hour)
	if svc.ttl != 7*24*time.Hour {
		t.Fatalf("ttl changed by invalid value: %v", svc.ttl)
	}
	svc.ConfigureArtifactTTL(time.Hour)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	art, err := svc.StoreCSVArtifact("org", "k", at, &CSVDocument{Filename: "a.csv", Data: []byte("a,b\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !art.ExpiresAt.Equal(at.Add(time.Hour)) {
		t.Fatalf("expires = %v, want generated+1h", art.ExpiresAt)
	}
}

func TestCov1xStoreCSVArtifact(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if _, err := svc.StoreCSVArtifact("org", "k", time.Now(), nil); err == nil || !strings.Contains(err.Error(), "csv document is required") {
		t.Fatalf("nil doc err = %v", err)
	}

	boom := errors.New("bucket unavailable")
	svc.SetStorage(&cov1xStore{ObjectStorage: storage.NewStubStorage(""), putErr: boom})
	if _, err := svc.StoreCSVArtifact("org", "k", time.Now(), &CSVDocument{Filename: "a.csv", Data: []byte("x")}); !errors.Is(err, boom) || !strings.Contains(err.Error(), "store report artifact") {
		t.Fatalf("put err = %v", err)
	}
	if len(svc.files) != 0 {
		t.Fatal("failed upload must not be indexed")
	}

	stub := storage.NewStubStorage("")
	svc.SetStorage(stub)
	svc.nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	art, err := svc.StoreCSVArtifact("org-1", "aging", at, &CSVDocument{Filename: "aging.csv", Data: []byte("h\nv\n")})
	if err != nil {
		t.Fatal(err)
	}
	if art.OrgID != "org-1" || art.Kind != "aging" || art.SizeBytes != 4 || art.ContentType != "text/csv; charset=utf-8" || art.DownloadURL == "" || !art.ExpiresAt.Equal(at.Add(7*24*time.Hour)) || !art.CreatedAt.Equal(at) {
		t.Fatalf("artifact = %+v", art)
	}
	if want := "reports/org-1/aging/" + art.ID + "/aging.csv"; art.StorageKey != want {
		t.Fatalf("key = %q, want %q", art.StorageKey, want)
	}
	data, ct, err := stub.GetObject(context.Background(), art.StorageKey)
	if err != nil || string(data) != "h\nv\n" || ct != "text/csv; charset=utf-8" {
		t.Fatalf("stored object = %q %q %v", data, ct, err)
	}
}

func TestCov1xCloneArtifactForResponse(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.nowFunc = func() time.Time { return now }

	if got := svc.cloneArtifactForResponse(StoredArtifact{ID: "a", ExpiresAt: now.Add(time.Hour)}); got.DownloadURL != "" {
		t.Fatalf("artifact without a key got a URL: %+v", got)
	}
	if got := svc.cloneArtifactForResponse(StoredArtifact{ID: "b", StorageKey: "k", ExpiresAt: now}); got.DownloadURL != "" {
		t.Fatalf("expired artifact got a URL: %+v", got)
	}
	if got := svc.cloneArtifactForResponse(StoredArtifact{ID: "c", StorageKey: "k", ExpiresAt: now.Add(time.Hour)}); !strings.Contains(got.DownloadURL, "X-Stub-Method=GET") {
		t.Fatalf("live artifact URL = %q", got.DownloadURL)
	}
	if got := svc.cloneArtifactForResponse(StoredArtifact{ID: "d", StorageKey: "k"}); got.DownloadURL == "" {
		t.Fatal("artifact with zero expiry should still be presigned")
	}
	svc.store = &cov1xStore{ObjectStorage: svc.store, presignErr: errors.New("sign failed")}
	if got := svc.cloneArtifactForResponse(StoredArtifact{ID: "e", StorageKey: "k", ExpiresAt: now.Add(time.Hour)}); got.DownloadURL != "" {
		t.Fatalf("presign failure should leave URL empty: %+v", got)
	}
}

func TestCov1xExpireArtifactsDeletesObjectsAndSkipsBlankKeys(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	stub := storage.NewStubStorage("")
	svc.SetStorage(stub)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	old, err := svc.StoreCSVArtifact("org", "k", base, &CSVDocument{Filename: "old.csv", Data: []byte("1")})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := svc.StoreCSVArtifact("org", "k", base.Add(30*24*time.Hour), &CSVDocument{Filename: "new.csv", Data: []byte("2")})
	if err != nil {
		t.Fatal(err)
	}
	blank := StoredArtifact{ID: "blank", ExpiresAt: base}
	svc.mu.Lock()
	svc.files[blank.ID] = blank
	svc.mu.Unlock()

	expired := svc.ExpireArtifacts(base.Add(8 * 24 * time.Hour))
	ids := map[string]bool{}
	for _, a := range expired {
		ids[a.ID] = true
	}
	if len(expired) != 2 || !ids[old.ID] || !ids["blank"] {
		t.Fatalf("expired = %+v", expired)
	}
	if _, _, err := stub.GetObject(context.Background(), old.StorageKey); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("expired object still stored: %v", err)
	}
	if _, _, err := stub.GetObject(context.Background(), fresh.StorageKey); err != nil {
		t.Fatalf("unexpired object removed: %v", err)
	}
	if _, ok := svc.files[fresh.ID]; !ok || len(svc.files) != 1 {
		t.Fatalf("index after expiry = %v", svc.files)
	}

	// With no store configured, expired entries are dropped from the index only.
	svc.store = nil
	svc.mu.Lock()
	svc.files["nostore"] = StoredArtifact{ID: "nostore", StorageKey: "reports/x", ExpiresAt: base}
	svc.mu.Unlock()
	if got := svc.ExpireArtifacts(base.Add(time.Hour)); len(got) != 1 || got[0].ID != "nostore" {
		t.Fatalf("expired without store = %+v", got)
	}
}

func TestCov1xDeleteArtifact(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	stub := storage.NewStubStorage("")
	boom := errors.New("delete denied")
	fs := &cov1xStore{ObjectStorage: stub}
	svc.SetStorage(fs)
	if err := stub.PutObject(context.Background(), "reports/k1", "text/csv", []byte("x")); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteArtifact("   "); err != nil {
		t.Fatalf("blank key should be a no-op: %v", err)
	}
	if ok, _ := stub.Exists(context.Background(), "reports/k1"); !ok {
		t.Fatal("blank key deleted something")
	}
	fs.delErr = boom
	if err := svc.DeleteArtifact("reports/k1"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the store error", err)
	}
	fs.delErr = nil
	if err := svc.DeleteArtifact("reports/k1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := stub.Exists(context.Background(), "reports/k1"); ok {
		t.Fatal("object not deleted")
	}
}

func TestCov1xRuntimeSnapshotRoundTrip(t *testing.T) {
	src := NewService(nil, nil, nil, nil)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	art, err := src.StoreCSVArtifact("org", "k", at, &CSVDocument{Filename: "a.csv", Data: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	data, err := src.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	dst := NewService(nil, nil, nil, nil)
	dst.files["stale"] = StoredArtifact{ID: "stale"}
	if err := dst.LoadRuntimeSnapshot(nil); err != nil || len(dst.files) != 1 {
		t.Fatalf("empty snapshot must be a no-op: %v %v", err, dst.files)
	}
	if err := dst.LoadRuntimeSnapshot([]byte("{not json")); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
	if len(dst.files) != 1 {
		t.Fatal("failed load modified state")
	}
	if err := dst.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if len(dst.files) != 1 || dst.files[art.ID].StorageKey != art.StorageKey || dst.files[art.ID].SizeBytes != 1 {
		t.Fatalf("loaded files = %+v", dst.files)
	}
}

// ---- helpers ----

func TestCov1xParseStripeReportedCents(t *testing.T) {
	if v, err := ParseStripeReportedCents("  "); v != nil || err != nil {
		t.Fatalf("blank = %v %v", v, err)
	}
	if v, err := ParseStripeReportedCents(" 250 "); err != nil || v == nil || *v != 250 {
		t.Fatalf("250 = %v %v", v, err)
	}
	if v, err := ParseStripeReportedCents("0"); err != nil || v == nil || *v != 0 {
		t.Fatalf("0 = %v %v", v, err)
	}
	for _, bad := range []string{"abc", "-1", "1.5"} {
		v, err := ParseStripeReportedCents(bad)
		if v != nil || err == nil || err.Error() != "stripe_reported_cents must be a non-negative integer" {
			t.Errorf("%q = %v %v", bad, v, err)
		}
	}
}

func TestCov1xFirstNonEmptyAndFullName(t *testing.T) {
	if got := firstNonEmpty("", "  ", " x ", "y"); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := fullName(" Ann", ""); got != "Ann" {
		t.Fatalf("got %q", got)
	}
	if v := intPtr(3); *v != 3 {
		t.Fatal("intPtr")
	}
	if v := boolPtr(true); !*v {
		t.Fatal("boolPtr")
	}
}

// ---- csv ----

func TestCov1xSafeFilename(t *testing.T) {
	for in, want := range map[string]string{
		"aging-20260101.csv": "aging-20260101.csv",
		"../etc/passwd":      "etc_passwd",
		"a b\r\nc.csv":       "a_b_c.csv",
		"///":                "report.csv",
		"":                   "report.csv",
		"..hidden.":          "hidden",
	} {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xNeutralizeCell(t *testing.T) {
	for in, want := range map[string]string{
		"":           "",
		"plain":      "plain",
		"1=1":        "1=1",
		"=1+1":       "'=1+1",
		"+cmd":       "'+cmd",
		"-cmd":       "'-cmd",
		"@sum":       "'@sum",
		"\tx":        "'\tx",
		"\rx":        "'\rx",
		"-5":         "-5",
		"-12.50":     "-12.50",
		"+3":         "+3",
		"-1e3":       "-1e3",
		"=HYPERLINK": "'=HYPERLINK",
	} {
		if got := neutralizeCell(in); got != want {
			t.Errorf("neutralizeCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xBuildCSV(t *testing.T) {
	doc, err := BuildCSV("bad/name.csv", []string{"name", "amount"}, [][]string{
		{"=SUM(A1)", "-5"},
		{"with,comma", "quote\"d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filename != "bad_name.csv" {
		t.Fatalf("filename = %q", doc.Filename)
	}
	want := "name,amount\n'=SUM(A1),-5\n\"with,comma\",\"quote\"\"d\"\n"
	if string(doc.Data) != want {
		t.Fatalf("data = %q, want %q", doc.Data, want)
	}
}
