package billing

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov2xReportOrg = "org-cov2x-report"

func cov2xDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := parseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// cov2xSetIssuedAt moves an invoice's issue date so report windows do not depend on the wall clock.
func cov2xSetIssuedAt(t *testing.T, svc *Service, invoiceID, date string) {
	t.Helper()
	d := cov2xDate(t, date)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	inv, ok := svc.invoices[invoiceID]
	if !ok {
		t.Fatalf("invoice %s not found", invoiceID)
	}
	inv.IssuedAt = &d
}

func cov2xReportRoster() RosterSnapshot {
	r := finRoster()
	r.Households = append(r.Households, RosterHousehold{ID: "hh-eq", Name: "=EVIL()", Status: "active"})
	r.GroupLinks = append(r.GroupLinks,
		RosterGroupLink{SwimmerID: "sw-7", GroupCode: "BRONZE", IsPrimary: false}, // non-primary first ...
		RosterGroupLink{SwimmerID: "sw-7", GroupCode: "SILVER", IsPrimary: true},  // ... primary wins
		RosterGroupLink{SwimmerID: "sw-8", GroupCode: "GOLD", IsPrimary: true},    // group not in roster.Groups
		RosterGroupLink{SwimmerID: "sw-6", GroupCode: "ZETA", IsPrimary: false},   // only a non-primary link
	)
	return r
}

func cov2xReportService(t *testing.T) (*Service, *appmiddleware.Principal) {
	t.Helper()
	svc := NewService()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return cov2xReportRoster(), nil })
	return svc, cov2xAdmin(cov2xReportOrg)
}

func cov2xAddItem(t *testing.T, svc *Service, p *appmiddleware.Principal, invoiceID, category, swimmerID string, cents int) {
	t.Helper()
	if _, err := svc.AddInvoiceItem(p.OrgID, p.UserID, p, invoiceID, AddInvoiceItemInput{
		Description: category + " line", Quantity: 1, UnitAmountCents: cents, Category: category, SwimmerID: swimmerID,
	}); err != nil {
		t.Fatalf("add %s item: %v", category, err)
	}
}

type cov2xLine struct {
	category, swimmer string
	cents             int
}

// cov2xLinesInvoice creates and issues an invoice with explicit lines (items can only be added in draft).
func cov2xLinesInvoice(t *testing.T, svc *Service, p *appmiddleware.Principal, household, number, due string, lines ...cov2xLine) *Invoice {
	t.Helper()
	inv, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: household, InvoiceNumber: number, DueDate: due})
	if err != nil {
		t.Fatalf("create %s: %v", number, err)
	}
	for _, l := range lines {
		cov2xAddItem(t, svc, p, inv.ID, l.category, l.swimmer, l.cents)
	}
	issued, _, err := svc.IssueInvoice(p.OrgID, p.UserID, p, inv.ID)
	if err != nil {
		t.Fatalf("issue %s: %v", number, err)
	}
	return issued
}

func TestCov2xAgingBucketsBoundaries(t *testing.T) {
	t.Parallel()
	var b AgingBuckets
	for _, c := range []struct{ days, cents int }{{-5, 1}, {0, 2}, {1, 4}, {30, 8}, {31, 16}, {60, 32}, {61, 64}, {90, 128}, {91, 256}, {4000, 512}} {
		b.add(c.days, c.cents)
	}
	want := AgingBuckets{CurrentCents: 3, Days1To30Cents: 12, Days31To60Cents: 48, Days61To90Cents: 192, Days90PlusCents: 768, TotalCents: 1023}
	if b != want {
		t.Fatalf("buckets %+v, want %+v", b, want)
	}
}

func TestCov2xNameOr(t *testing.T) {
	t.Parallel()
	names := map[string]string{"hh-1": "Alpha"}
	for id, want := range map[string]string{"hh-1": "Alpha", "hh-unknown": "hh-unknown", "": "No household"} {
		if got := nameOr(names, id); got != want {
			t.Errorf("nameOr(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestCov2xCategoryLabel(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"program_fee": "Dues", "registration_fee": "Registration", "meet_fee": "Meet fees", "late_fee": "Late fees",
		"discount": "Discounts", "adjustment": "Adjustments", "equipment": "Equipment", "manual": "Manual",
		"credit": "Credits", "something_new": "something_new",
	}
	for in, label := range want {
		if got := categoryLabel(in); got != label {
			t.Errorf("categoryLabel(%q) = %q, want %q", in, got, label)
		}
	}
}

func TestCov2xCSVSafeNeutralisesFormulas(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":              "",
		"123.45":        "123.45",
		"-5":            "-5", // a plain negative number is data, not a formula
		"1e3":           "1e3",
		"=SUM(A1:A2)":   "'=SUM(A1:A2)",
		"+cmd|' /C'":    "'+cmd|' /C'",
		"-2+3":          "'-2+3",
		"@SUM(1)":       "'@SUM(1)",
		"\tleading tab": "'\tleading tab",
		"\rcarriage":    "'\rcarriage",
		"Alpha Family":  "Alpha Family",
	}
	for in, want := range cases {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov2xPeriodBucket(t *testing.T) {
	t.Parallel()
	sunday := cov2xDate(t, "2026-11-08")
	if sunday.Weekday() != time.Sunday {
		t.Fatalf("fixture assumption: %v", sunday.Weekday())
	}
	at := time.Date(2026, 11, 8, 17, 30, 0, 0, time.UTC)
	check := func(period string, wantStart, wantEnd string) {
		t.Helper()
		s, e := periodBucket(at, period)
		if fmtDate(s) != wantStart || fmtDate(e) != wantEnd {
			t.Errorf("%s bucket = %s..%s, want %s..%s", period, fmtDate(s), fmtDate(e), wantStart, wantEnd)
		}
	}
	check("day", "2026-11-08", "2026-11-08")
	check("week", "2026-11-02", "2026-11-08") // Monday-start weeks: Sunday belongs to the week before
	check("month", "2026-11-01", "2026-11-30")
	check("", "2026-11-01", "2026-11-30")
	s, e := periodBucket(cov2xDate(t, "2026-11-09"), "week")
	if fmtDate(s) != "2026-11-09" || fmtDate(e) != "2026-11-15" {
		t.Errorf("monday week bucket = %s..%s", fmtDate(s), fmtDate(e))
	}
}

func TestCov2xAgingOutstandingAt(t *testing.T) {
	t.Parallel()
	svc := NewService()
	asOf := cov2xDate(t, "2026-11-30")
	issued := cov2xDate(t, "2026-11-01")
	late := cov2xDate(t, "2026-12-01")
	before := cov2xDate(t, "2026-11-15")
	after := cov2xDate(t, "2026-12-15")
	svc.payments["inv-open"] = []*Payment{
		{Status: "succeeded", PaidAt: &after, AmountCents: 3000},  // paid after as_of: was still owed then
		{Status: "succeeded", PaidAt: &before, AmountCents: 7000}, // already reflected in AmountDue
		{Status: "failed", PaidAt: &after, AmountCents: 999},      // never settled
		{Status: "succeeded", PaidAt: nil, AmountCents: 888},      // no settlement date
	}
	svc.payments["inv-paid"] = []*Payment{
		{Status: "succeeded", PaidAt: &after, AmountCents: 6000},
		{Status: "succeeded", PaidAt: &after, AmountCents: 6000},
	}
	cases := []struct {
		name string
		inv  *Invoice
		want int
	}{
		{"never issued", &Invoice{ID: "x", Status: "issued", TotalCents: 100, AmountDueCents: 100}, 0},
		{"issued after as_of", &Invoice{ID: "x", IssuedAt: &late, Status: "issued", TotalCents: 100, AmountDueCents: 100}, 0},
		{"draft", &Invoice{ID: "x", IssuedAt: &issued, Status: "draft", TotalCents: 100, AmountDueCents: 100}, 0},
		{"voided", &Invoice{ID: "x", IssuedAt: &issued, Status: "voided", TotalCents: 100, AmountDueCents: 100}, 0},
		{"issued adds back later payments", &Invoice{ID: "inv-open", IssuedAt: &issued, Status: "partially_paid", TotalCents: 10000, AmountDueCents: 3000}, 6000},
		{"paid then owed at as_of", &Invoice{ID: "inv-paid", IssuedAt: &issued, Status: "paid", TotalCents: 10000, AmountDueCents: 0}, 10000}, // clamped to total
		{"paid with no later payments", &Invoice{ID: "other", IssuedAt: &issued, Status: "paid", TotalCents: 10000}, 0},
	}
	for _, c := range cases {
		if got := agingOutstandingAt(svc, c.inv, asOf); got != c.want {
			t.Errorf("%s: outstanding %d, want %d", c.name, got, c.want)
		}
	}
}

func TestCov2xAgingReport(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	asOf := cov2xDate(t, "2099-03-01")

	// hh-a: current + 1-30 + 31-60; hh-b: 61-90 + 90+; ghost ties hh-b on total; no-household sorts last.
	cov2xIssuedInvoice(t, svc, p, "hh-a", "AG-A1", "2099-03-05", 10000)
	cov2xIssuedInvoice(t, svc, p, "hh-a", "AG-A2", "2099-02-20", 5000)
	cov2xIssuedInvoice(t, svc, p, "hh-a", "AG-A3", "2099-01-20", 3000)
	cov2xIssuedInvoice(t, svc, p, "hh-b", "AG-B1", "2098-12-20", 2000)
	cov2xIssuedInvoice(t, svc, p, "hh-b", "AG-B2", "2098-06-01", 4000)
	cov2xIssuedInvoice(t, svc, p, "hh-ghost", "AG-G1", "2099-02-25", 6000)
	noDue := cov2xIssuedInvoice(t, svc, p, "", "AG-N1", "", 1000)
	cov2xSetIssuedAt(t, svc, noDue.ID, "2098-11-30") // no due date: age counts from the issue date
	paid := cov2xIssuedInvoice(t, svc, p, "hh-a", "AG-PAID", "2099-02-01", 7000)
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, paid.ID, RecordPaymentInput{AmountCents: 7000, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	// Another club's receivable never leaks into this club's report.
	other := cov2xAdmin("org-cov2x-other")
	cov2xIssuedInvoice(t, svc, other, "hh-a", "AG-OTHER", "2098-01-01", 99999)

	report, err := svc.AgingReport(p.OrgID, p, asOf.Add(15*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.AsOf != "2099-03-01" {
		t.Fatalf("as_of must be truncated to the date: %s", report.AsOf)
	}
	wantTotals := AgingBuckets{CurrentCents: 10000, Days1To30Cents: 5000 + 6000, Days31To60Cents: 3000, Days61To90Cents: 2000, Days90PlusCents: 4000 + 1000, TotalCents: 31000}
	if report.Totals != wantTotals {
		t.Fatalf("totals %+v, want %+v", report.Totals, wantTotals)
	}
	var order []string
	byID := map[string]AgingRow{}
	for _, r := range report.Rows {
		order = append(order, r.HouseholdName)
		byID[r.HouseholdID] = r
	}
	if !reflect.DeepEqual(order, []string{"Alpha", "Bravo", "hh-ghost", "No household"}) {
		t.Fatalf("row order (total desc, then name): %v", order)
	}
	alpha := byID["hh-a"]
	if alpha.TotalCents != 18000 || alpha.InvoiceCount != 3 || alpha.OldestDueDate != "2099-01-20" || alpha.CurrentCents != 10000 || alpha.Days1To30Cents != 5000 || alpha.Days31To60Cents != 3000 {
		t.Fatalf("alpha row: %+v", alpha)
	}
	if bravo := byID["hh-b"]; bravo.Days61To90Cents != 2000 || bravo.Days90PlusCents != 4000 || bravo.OldestDueDate != "2098-06-01" {
		t.Fatalf("bravo row: %+v", bravo)
	}
	if nh := byID[""]; nh.OldestDueDate != "2098-11-30" || nh.Days90PlusCents != 1000 {
		t.Fatalf("household-less row falls back to the issue date: %+v", nh)
	}

	// CSV: header + a row per household + totals, with formula-looking names neutralised.
	csv := report.CSV()
	if len(csv) != len(report.Rows)+2 || csv[0][0] != "Household" || csv[len(csv)-1][0] != "Total" || csv[len(csv)-1][7] != "310.00" {
		t.Fatalf("aging csv: %v", csv)
	}
	if !reflect.DeepEqual(csv[1], []string{"Alpha", "hh-a", "100.00", "50.00", "30.00", "0.00", "0.00", "180.00", "3", "2099-01-20"}) {
		t.Fatalf("aging csv row: %v", csv[1])
	}
}

func TestCov2xAgingReportHonoursPaymentsMadeAfterAsOf(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	inv := cov2xIssuedInvoice(t, svc, p, "hh-a", "AGP-1", "2026-11-10", 10000)
	cov2xSetIssuedAt(t, svc, inv.ID, "2026-10-01")
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 10000, Method: "check", PaidAt: "2026-12-15"}); err != nil {
		t.Fatal(err)
	}
	before, err := svc.AgingReport(p.OrgID, p, cov2xDate(t, "2026-11-30"))
	if err != nil || before.Totals.TotalCents != 10000 || before.Totals.Days1To30Cents != 10000 {
		t.Fatalf("a payment made after as_of leaves the invoice outstanding at as_of: %+v %v", before.Totals, err)
	}
	after, err := svc.AgingReport(p.OrgID, p, cov2xDate(t, "2026-12-15"))
	if err != nil || after.Totals.TotalCents != 0 || len(after.Rows) != 0 {
		t.Fatalf("once paid it drops out: %+v %v", after, err)
	}
}

func TestCov2xAgingReportMalformedDueDateCountsAsCurrent(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	inv := cov2xIssuedInvoice(t, svc, p, "hh-a", "AGM-1", "2026-11-10", 2500)
	svc.mu.Lock()
	svc.invoices[inv.ID].DueDate = "not-a-date"
	svc.mu.Unlock()
	report, err := svc.AgingReport(p.OrgID, p, cov2xDate(t, "2099-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.CurrentCents != 2500 || report.Totals.TotalCents != 2500 {
		t.Fatalf("an unparseable due date must not drop money from the report: %+v", report.Totals)
	}
}

func TestCov2xRevenueReport(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)

	sep := cov2xLinesInvoice(t, svc, p, "hh-a", "RV-SEP", "",
		cov2xLine{"program_fee", "", 10000}, // no swimmer: household-level
		cov2xLine{"meet_fee", "sw-9", 2000}, // swimmer without any group
		cov2xLine{"equipment", "", 1500})
	nov := cov2xLinesInvoice(t, svc, p, "hh-a", "RV-NOV", "",
		cov2xLine{"program_fee", "", 700},
		cov2xLine{"late_fee", "sw-8", 500},
		cov2xLine{"credit", "sw-6", 300},
		cov2xLine{"manual", "sw-7", 200},
		cov2xLine{"program_fee", "sw-1", 4000})
	far := cov2xIssuedInvoice(t, svc, p, "hh-a", "RV-FAR", "", 123456)
	voided := cov2xIssuedInvoice(t, svc, p, "hh-a", "RV-VOID", "", 88888)
	if _, _, err := svc.VoidInvoice(p.OrgID, p.UserID, p, voided.ID); err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: "hh-a", InvoiceNumber: "RV-DRAFT"})
	if err != nil {
		t.Fatal(err)
	}
	cov2xAddItem(t, svc, p, draft.ID, "program_fee", "sw-1", 77777)
	cov2xSetIssuedAt(t, svc, sep.ID, "2026-09-10")
	cov2xSetIssuedAt(t, svc, nov.ID, "2026-11-20")
	cov2xSetIssuedAt(t, svc, far.ID, "2027-03-05")
	cov2xSetIssuedAt(t, svc, voided.ID, "2026-10-10")
	other := cov2xAdmin("org-cov2x-other")
	cov2xIssuedInvoice(t, svc, other, "hh-a", "RV-OTHER", "", 55555)

	report, err := svc.RevenueReport(p.OrgID, p, cov2xDate(t, "2026-09-01"), cov2xDate(t, "2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	// sep: household-level program_fee 10000 (item without swimmer) + meet_fee 2000 + equipment 1500.
	// nov: program_fee 700 (household-level) + late_fee 500 + credit 300 + manual 200 + program_fee 4000 (SILVER).
	if report.TotalCents != 10000+2000+1500+700+500+300+200+4000 {
		t.Fatalf("total %d", report.TotalCents)
	}
	if report.From != "2026-09-01" || report.To != "2026-12-31" {
		t.Fatalf("range echo: %s..%s", report.From, report.To)
	}
	var months []string
	var monthCents []int
	for _, m := range report.ByMonth {
		months = append(months, m.Key+"/"+m.Label)
		monthCents = append(monthCents, m.AmountCents)
	}
	if !reflect.DeepEqual(months, []string{"2026-09/Sep 2026", "2026-10/Oct 2026", "2026-11/Nov 2026", "2026-12/Dec 2026"}) ||
		!reflect.DeepEqual(monthCents, []int{13500, 0, 5700, 0}) {
		t.Fatalf("months must form a continuous axis: %v %v", months, monthCents)
	}
	type kv struct {
		Key, Label string
		Cents      int
	}
	flat := func(ts []RevenueTotal) (out []kv) {
		for _, x := range ts {
			out = append(out, kv{x.Key, x.Label, x.AmountCents})
		}
		return out
	}
	wantCat := []kv{{"program_fee", "Dues", 14700}, {"meet_fee", "Meet fees", 2000}, {"equipment", "Equipment", 1500}, {"late_fee", "Late fees", 500}, {"credit", "Credits", 300}, {"manual", "Manual", 200}}
	if got := flat(report.ByCategory); !reflect.DeepEqual(got, wantCat) {
		t.Fatalf("by category %v, want %v", got, wantCat)
	}
	wantGroup := []kv{{"HOUSEHOLD", "Household-level", 14700 - 4000 + 1500}, {"SILVER", "Silver", 4200}, {"UNASSIGNED", "No group", 2000}, {"GOLD", "GOLD", 500}, {"ZETA", "ZETA", 300}}
	if got := flat(report.ByGroup); !reflect.DeepEqual(got, wantGroup) {
		t.Fatalf("by group %v, want %v", got, wantGroup)
	}
	if first := report.Rows[0]; first.Month != "2026-09" || first.Category != "equipment" || first.GroupCode != "HOUSEHOLD" || first.AmountCents != 1500 {
		t.Fatalf("rows sort by month, category, group: %+v", first)
	}
	if len(report.Rows) != 8 {
		t.Fatalf("rows: %+v", report.Rows)
	}
	for i := 1; i < len(report.Rows); i++ {
		a, b := report.Rows[i-1], report.Rows[i]
		if a.Month > b.Month || (a.Month == b.Month && (a.Category > b.Category || (a.Category == b.Category && a.GroupCode >= b.GroupCode))) {
			t.Fatalf("rows out of order at %d: %+v then %+v", i, a, b)
		}
	}

	csv := report.CSV()
	if len(csv) != len(report.Rows)+2 || !reflect.DeepEqual(csv[0], []string{"Month", "Category", "Group", "Amount"}) {
		t.Fatalf("revenue csv shape: %v", csv)
	}
	if !reflect.DeepEqual(csv[1], []string{"2026-09", "Equipment", "HOUSEHOLD", "15.00"}) || !reflect.DeepEqual(csv[len(csv)-1], []string{"Total", "", "", "192.00"}) {
		t.Fatalf("revenue csv rows: first=%v last=%v", csv[1], csv[len(csv)-1])
	}
}

func TestCov2xRevenueReportEmptyRangeHasNonNilSlices(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	// from after to: no month axis, nothing to total, but JSON must still carry [] not null.
	report, err := svc.RevenueReport(p.OrgID, p, cov2xDate(t, "2026-12-31"), cov2xDate(t, "2026-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if report.ByMonth == nil || report.ByCategory == nil || report.ByGroup == nil || len(report.ByMonth)+len(report.ByCategory)+len(report.ByGroup) != 0 {
		t.Fatalf("empty slices expected: %+v", report)
	}
	if report.TotalCents != 0 || len(report.Rows) != 0 {
		t.Fatalf("nothing to report: %+v", report)
	}
}

func TestCov2xReportsRequireAdmin(t *testing.T) {
	t.Parallel()
	svc, _ := cov2xReportService(t)
	coach := cov2xCoach(cov2xReportOrg)
	parent := cov2xParent(cov2xReportOrg, "hh-a")
	day := cov2xDate(t, "2026-11-01")
	for _, p := range []*appmiddleware.Principal{coach, parent} {
		if _, err := svc.AgingReport(cov2xReportOrg, p, day); !errors.Is(err, ErrForbidden) {
			t.Errorf("aging as %v: %v", p.Roles, err)
		}
		if _, err := svc.RevenueReport(cov2xReportOrg, p, day, day); !errors.Is(err, ErrForbidden) {
			t.Errorf("revenue as %v: %v", p.Roles, err)
		}
		if _, err := svc.OutstandingReport(cov2xReportOrg, p); !errors.Is(err, ErrForbidden) {
			t.Errorf("outstanding as %v: %v", p.Roles, err)
		}
		if _, err := svc.DepositsReport(cov2xReportOrg, p, day, day, "month"); !errors.Is(err, ErrForbidden) {
			t.Errorf("deposits as %v: %v", p.Roles, err)
		}
		if _, err := svc.QuickBooksCSV(cov2xReportOrg, p, "invoices", day, day); !errors.Is(err, ErrForbidden) {
			t.Errorf("quickbooks as %v: %v", p.Roles, err)
		}
	}
}

func TestCov2xReportsSurfaceRosterErrors(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	boom := errors.New("roster unavailable")
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, boom })
	day := cov2xDate(t, "2026-11-01")
	if _, err := svc.OutstandingReport(p.OrgID, p); !errors.Is(err, boom) {
		t.Errorf("outstanding: %v", err)
	}
	if _, err := svc.QuickBooksCSV(p.OrgID, p, "payments", day, day); !errors.Is(err, boom) {
		t.Errorf("quickbooks: %v", err)
	}
}

func TestCov2xOutstandingReport(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	cov2xIssuedInvoice(t, svc, p, "hh-b", "OS-B1", "2026-12-10", 4000)
	cov2xIssuedInvoice(t, svc, p, "hh-a", "OS-A1", "2026-12-20", 3000)
	cov2xIssuedInvoice(t, svc, p, "hh-a", "OS-A2", "2026-12-05", 1000)
	cov2xIssuedInvoice(t, svc, p, "hh-b", "OS-B2", "", 0+1) // no due date must not clear the oldest
	cov2xIssuedInvoice(t, svc, p, "hh-eq", "OS-EQ", "2026-12-01", 100)
	paid := cov2xIssuedInvoice(t, svc, p, "hh-c", "OS-PAID", "2026-12-01", 5000)
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, paid.ID, RecordPaymentInput{AmountCents: 5000, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	// Credit only: a household with credit and nothing owed still appears; zero balances do not.
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-credit-only", ManualCreditInput{Direction: "credit", AmountCents: 2500, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-zeroed", ManualCreditInput{Direction: "credit", AmountCents: 900, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-zeroed", ManualCreditInput{Direction: "debit", AmountCents: 900, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	// hh-b has both money owed and credit.
	if _, _, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-b", ManualCreditInput{Direction: "credit", AmountCents: 1000, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	// Other clubs' invoices and balances are ignored (their keys share no org prefix).
	other := cov2xAdmin("org-cov2x-other")
	cov2xIssuedInvoice(t, svc, other, "hh-a", "OS-OTHER", "2026-01-01", 77777)
	if _, _, err := svc.AddManualCredit(other.OrgID, other.UserID, other, "hh-a", ManualCreditInput{Direction: "credit", AmountCents: 66666, Reason: "x"}); err != nil {
		t.Fatal(err)
	}

	report, err := svc.OutstandingReport(p.OrgID, p)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalDueCents != 4000+1+3000+1000+100 || report.TotalCreditCents != 3500 || report.TotalNetCents != report.TotalDueCents-report.TotalCreditCents {
		t.Fatalf("totals: %+v", report)
	}
	var names []string
	rows := map[string]OutstandingRow{}
	for _, r := range report.Rows {
		names = append(names, r.HouseholdName)
		rows[r.HouseholdID] = r
	}
	// Net: Alpha 4000, Bravo 4001-1000=3001, "=EVIL()" 100, credit-only -2500.
	if !reflect.DeepEqual(names, []string{"Alpha", "Bravo", "=EVIL()", "hh-credit-only"}) {
		t.Fatalf("rows sorted by net balance desc: %v", names)
	}
	if a := rows["hh-a"]; a.OpenInvoiceCount != 2 || a.AmountDueCents != 4000 || a.OldestDueDate != "2026-12-05" || a.NetBalanceCents != 4000 {
		t.Fatalf("alpha: %+v", a)
	}
	if b := rows["hh-b"]; b.OpenInvoiceCount != 2 || b.CreditBalanceCents != 1000 || b.NetBalanceCents != 3001 || b.OldestDueDate != "2026-12-10" {
		t.Fatalf("bravo: %+v", b)
	}
	if c := rows["hh-credit-only"]; c.OpenInvoiceCount != 0 || c.CreditBalanceCents != 2500 || c.NetBalanceCents != -2500 {
		t.Fatalf("credit only: %+v", c)
	}
	if _, ok := rows["hh-zeroed"]; ok {
		t.Fatal("a zero credit balance must not create a row")
	}

	csv := report.CSV()
	if len(csv) != len(report.Rows)+2 {
		t.Fatalf("csv rows: %v", csv)
	}
	if !reflect.DeepEqual(csv[3], []string{"'=EVIL()", "hh-eq", "1", "1.00", "0.00", "1.00", "2026-12-01"}) {
		t.Fatalf("formula-like household name must be neutralised: %v", csv[3])
	}
	if !reflect.DeepEqual(csv[len(csv)-1], []string{"Total", "", "", "81.01", "35.00", "46.01", ""}) {
		t.Fatalf("csv totals: %v", csv[len(csv)-1])
	}
}

func TestCov2xOutstandingTiesSortByName(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	cov2xIssuedInvoice(t, svc, p, "hh-b", "OT-B", "", 500)
	cov2xIssuedInvoice(t, svc, p, "hh-a", "OT-A", "", 500)
	report, err := svc.OutstandingReport(p.OrgID, p)
	if err != nil || len(report.Rows) != 2 || report.Rows[0].HouseholdName != "Alpha" || report.Rows[1].HouseholdName != "Bravo" {
		t.Fatalf("equal balances sort by household name: %+v %v", report.Rows, err)
	}
}

// cov2xDepositsFixture builds a deterministic cash ledger in November 2026.
func cov2xDepositsFixture(t *testing.T) (*Service, *appmiddleware.Principal) {
	t.Helper()
	svc := NewService()
	p := cov2xAdmin("org-cov2x-deposits")
	inv1 := cov2xIssuedInvoice(t, svc, p, "hh-d", "DP-1", "", 30000)
	pay := func(inv *Invoice, method string, cents int, paidAt string, excess bool) *Payment {
		t.Helper()
		got, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: cents, Method: method, PaidAt: paidAt, ExcessToCredit: excess})
		if err != nil {
			t.Fatalf("pay %s %d: %v", method, cents, err)
		}
		return got
	}
	p1 := pay(inv1, "check", 5000, "2026-11-02", false)
	p2 := pay(inv1, "cash", 2000, "2026-11-04", false)
	p3 := pay(inv1, "stripe", 3000, "2026-11-10", false)
	pay(inv1, "ach", 1000, "2026-10-31", false)  // before the window
	pay(inv1, "other", 400, "2026-11-30", false) // last day of the window (inclusive)
	pay(inv1, "check", 600, "2026-12-01", false) // after the window

	inv2 := cov2xIssuedInvoice(t, svc, p, "hh-d", "DP-2", "", 4000)
	pay(inv2, "check", 5000, "2026-11-05", true) // 4000 settles the invoice, 1000 becomes credit

	// Applying account credit settles invoices but moves no cash.
	if _, applied, err := svc.AddManualCredit(p.OrgID, p.UserID, p, "hh-d", ManualCreditInput{Direction: "credit", AmountCents: 1500, Reason: "gift", ApplyToOpenInvoices: true}); err != nil || len(applied) != 1 {
		t.Fatalf("apply credit: %v %v", applied, err)
	}
	refund := func(pm *Payment, cents int) *Refund {
		t.Helper()
		r, _, err := svc.CreateRefund(p.OrgID, p.UserID, p, inv1.ID, CreateRefundInput{PaymentID: pm.ID, AmountCents: cents, Reason: "test"})
		if err != nil {
			t.Fatalf("refund %d: %v", cents, err)
		}
		return r
	}
	d := func(s string) *time.Time { v := cov2xDate(t, s); return &v }
	r1 := refund(p2, 2000) // reverses the cash payment entirely
	r2 := refund(p1, 500)
	r3 := refund(p1, 300)
	r4 := refund(p1, 200)
	toCredit, _, err := svc.RefundToCredit(p.OrgID, p.UserID, p, inv1.ID, CreateRefundInput{PaymentID: p3.ID, AmountCents: 1000, Reason: "to credit"})
	if err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.refundsByID[r1.ID].ProcessedAt = d("2026-11-12")
	svc.refundsByID[r2.ID].ProcessedAt = nil // falls back to the creation time
	svc.refundsByID[r2.ID].CreatedAt = *d("2026-11-13")
	svc.refundsByID[r3.ID].ProcessedAt = d("2026-12-20") // outside the window
	svc.refundsByID[r4.ID].Status = "pending"            // not yet money out
	svc.refundsByID[r4.ID].ProcessedAt = d("2026-11-14")
	svc.refundsByID[toCredit.ID].ProcessedAt = d("2026-11-15") // credit refund: no cash leaves
	for _, pm := range svc.paymentsByID {
		if pm.Method == PaymentMethodAccountCredit {
			pm.PaidAt = d("2026-11-06") // inside the window, still not a deposit
		}
	}
	// Payments that must be ignored for other reasons.
	svc.paymentsByID["pay-failed"] = &Payment{ID: "pay-failed", OrgID: p.OrgID, InvoiceID: inv1.ID, Method: "check", Status: "failed", AmountCents: 111, PaidAt: d("2026-11-03")}
	svc.paymentsByID["pay-nodate"] = &Payment{ID: "pay-nodate", OrgID: p.OrgID, InvoiceID: inv1.ID, Method: "check", Status: "succeeded", AmountCents: 222}
	svc.paymentsByID["pay-foreign"] = &Payment{ID: "pay-foreign", OrgID: "org-someone-else", InvoiceID: "x", Method: "check", Status: "succeeded", AmountCents: 333, PaidAt: d("2026-11-03")}
	// Overpayment credit entry date, and Stripe refunds of cash that had become credit.
	for _, e := range svc.finance.credits {
		if e.Kind == CreditKindOverpayment {
			e.CreatedAt = *d("2026-11-05")
		}
	}
	credit := func(org, kind, ref string, cents int, at string) {
		svc.finance.credits = append(svc.finance.credits, &CreditEntry{ID: "ce-" + ref + at, OrgID: org, HouseholdID: "hh-d", Kind: kind, StripeRef: ref, AmountCents: cents, CreatedAt: *d(at)})
	}
	credit(p.OrgID, CreditKindManualDebit, "refund|re_a", -700, "2026-11-06")
	credit(p.OrgID, CreditKindManualDebit, "refund|re_b", 300, "2026-11-07")  // positive magnitude is taken as is
	credit(p.OrgID, CreditKindManualDebit, "other|re_c", -9999, "2026-11-07") // not a Stripe refund reversal
	credit(p.OrgID, CreditKindManualDebit, "refund|re_d", -888, "2026-12-30") // outside the window
	credit("org-someone-else", CreditKindManualDebit, "refund|re_e", -777, "2026-11-07")
	credit("org-someone-else", CreditKindOverpayment, "", 4444, "2026-11-07")
	return svc, p
}

func TestCov2xDepositsReportMonth(t *testing.T) {
	t.Parallel()
	svc, p := cov2xDepositsFixture(t)
	report, err := svc.DepositsReport(p.OrgID, p, cov2xDate(t, "2026-11-01"), cov2xDate(t, "2026-11-30"), "quarter")
	if err != nil {
		t.Fatal(err)
	}
	if report.Period != "month" || len(report.Rows) != 1 {
		t.Fatalf("unknown periods fall back to month: %+v", report)
	}
	row := report.Rows[0]
	// Cash in: check 5000+4000, cash 2000 (reversed by its refund, still deposited), stripe 3000, other 400,
	// plus the 1000 overpayment that became credit.
	wantMethods := map[string]int{"check": 9000, "cash": 2000, "stripe": 3000, "other": 400}
	if row.PeriodStart != "2026-11-01" || row.PeriodEnd != "2026-11-30" || row.PaymentCount != 5 ||
		!reflect.DeepEqual(row.ByMethod, wantMethods) || row.OverpaymentCents != 1000 || row.GrossCents != 15400 {
		t.Fatalf("deposit row: %+v", row)
	}
	// Money out: r1 2000 + r2 500 (created-at fallback) + Stripe credit reversals 700 + 300.
	if row.RefundsCents != 3500 || row.NetCents != 11900 {
		t.Fatalf("refunds/net: %+v", row)
	}
	if report.GrossCents != 15400 || report.RefundCents != 3500 || report.NetCents != 11900 {
		t.Fatalf("report totals: %+v", report)
	}

	csv := report.CSV()
	if len(csv) != 2 {
		t.Fatalf("csv rows: %v", csv)
	}
	wantHeader := []string{"Period start", "Period end", "Payments", "Stripe", "Ach", "Check", "Cash", "Other", "Overpayments to credit", "Gross", "Refunds", "Net deposit"}
	wantRow := []string{"2026-11-01", "2026-11-30", "5", "30.00", "0.00", "90.00", "20.00", "4.00", "10.00", "154.00", "35.00", "119.00"}
	if !reflect.DeepEqual(csv[0], wantHeader) || !reflect.DeepEqual(csv[1], wantRow) {
		t.Fatalf("deposits csv:\n%v\n%v", csv[0], csv[1])
	}
}

func TestCov2xDepositsReportWeekAndDayBuckets(t *testing.T) {
	t.Parallel()
	svc, p := cov2xDepositsFixture(t)
	from, to := cov2xDate(t, "2026-11-01"), cov2xDate(t, "2026-11-30")
	for _, period := range []string{"week", "day"} {
		report, err := svc.DepositsReport(p.OrgID, p, from, to, period)
		if err != nil {
			t.Fatal(err)
		}
		if report.Period != period {
			t.Fatalf("period echo: %s", report.Period)
		}
		gross, refunds, last := 0, 0, ""
		for _, r := range report.Rows {
			gross += r.GrossCents
			refunds += r.RefundsCents
			if r.PeriodStart <= last {
				t.Fatalf("%s rows must ascend: %s after %s", period, r.PeriodStart, last)
			}
			last = r.PeriodStart
			start, end := cov2xDate(t, r.PeriodStart), cov2xDate(t, r.PeriodEnd)
			switch period {
			case "week":
				if start.Weekday() != time.Monday || end.Sub(start) != 6*24*time.Hour {
					t.Fatalf("week bucket %s..%s", r.PeriodStart, r.PeriodEnd)
				}
			case "day":
				if !start.Equal(end) {
					t.Fatalf("day bucket %s..%s", r.PeriodStart, r.PeriodEnd)
				}
			}
		}
		if gross != 15400 || refunds != 3500 || report.NetCents != 11900 {
			t.Fatalf("%s buckets must add up to the month: gross=%d refunds=%d net=%d", period, gross, refunds, report.NetCents)
		}
		if period == "day" && len(report.Rows) != 9 {
			t.Fatalf("nine distinct days carry activity, got %d", len(report.Rows))
		}
	}
}

func TestCov2xDepositsReportEmpty(t *testing.T) {
	t.Parallel()
	svc := NewService()
	p := cov2xAdmin("org-cov2x-empty")
	report, err := svc.DepositsReport(p.OrgID, p, cov2xDate(t, "2026-01-01"), cov2xDate(t, "2026-01-31"), "")
	if err != nil || report.Rows == nil || len(report.Rows) != 0 || report.NetCents != 0 {
		t.Fatalf("empty report: %+v %v", report, err)
	}
	if csv := report.CSV(); len(csv) != 1 {
		t.Fatalf("csv of an empty report is only the header: %v", csv)
	}
}

func TestCov2xQuickBooksCSV(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	inv1 := cov2xIssuedInvoice(t, svc, p, "hh-eq", "QB-2", "2026-12-15", 3000)
	inv2 := cov2xLinesInvoice(t, svc, p, "hh-a", "QB-1", "", cov2xLine{"program_fee", "", 4500}, cov2xLine{"meet_fee", "sw-1", 500})
	cov2xSetIssuedAt(t, svc, inv1.ID, "2026-11-05")
	cov2xSetIssuedAt(t, svc, inv2.ID, "2026-11-20")
	late := cov2xIssuedInvoice(t, svc, p, "hh-a", "QB-LATE", "", 100)
	cov2xSetIssuedAt(t, svc, late.ID, "2027-02-01")
	if _, err := svc.CreateInvoice(p.OrgID, p.UserID, p, CreateInvoiceInput{HouseholdID: "hh-a", InvoiceNumber: "QB-DRAFT"}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.invoices[inv2.ID].Notes = "=memo"
	svc.mu.Unlock()
	from, to := cov2xDate(t, "2026-11-01"), cov2xDate(t, "2026-11-30")

	rows, err := svc.QuickBooksCSV(p.OrgID, p, "invoices", from, to)
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"InvoiceNo", "Customer", "InvoiceDate", "DueDate", "Memo", "Item(Product/Service)", "ItemDescription", "ItemQuantity", "ItemRate", "ItemAmount"}
	if !reflect.DeepEqual(rows[0], wantHeader) || len(rows) != 1+1+2 {
		t.Fatalf("invoice export shape: %v", rows)
	}
	// Sorted by invoice number: QB-1 (two lines) before QB-2.
	if !reflect.DeepEqual(rows[1], []string{"QB-1", "Alpha", "11/20/2026", "", "'=memo", "Dues", "program_fee line", "1", "45.00", "45.00"}) {
		t.Fatalf("first line: %v", rows[1])
	}
	if rows[2][5] != "Meet fees" || rows[2][9] != "5.00" {
		t.Fatalf("second line: %v", rows[2])
	}
	if !reflect.DeepEqual(rows[3], []string{"QB-2", "'=EVIL()", "11/05/2026", "12/15/2026", "", "Dues", "Dues", "1", "30.00", "30.00"}) {
		t.Fatalf("customer must be neutralised and due date reformatted: %v", rows[3])
	}

	// Payments: sorted by date, account credit and unsettled rows excluded, ref falls back to the intent id.
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv2.ID, RecordPaymentInput{AmountCents: 1000, Method: "check", PaidAt: "2026-11-22", ExternalRef: "CHK-77", Notes: "-note"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv1.ID, RecordPaymentInput{AmountCents: 500, Method: "stripe", PaidAt: "2026-11-10", StripePaymentIntentID: "pi_cov2x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv1.ID, RecordPaymentInput{AmountCents: 100, Method: "cash", PaidAt: "2026-12-10"}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	t0 := cov2xDate(t, "2026-11-15")
	svc.paymentsByID["orphan"] = &Payment{ID: "orphan", OrgID: p.OrgID, InvoiceID: "gone", Method: "ach", Status: "succeeded", AmountCents: 250, PaidAt: &t0}
	svc.paymentsByID["credit"] = &Payment{ID: "credit", OrgID: p.OrgID, InvoiceID: inv1.ID, Method: PaymentMethodAccountCredit, Status: "succeeded", AmountCents: 999, PaidAt: &t0}
	svc.paymentsByID["failed"] = &Payment{ID: "failed", OrgID: p.OrgID, InvoiceID: inv1.ID, Method: "check", Status: "failed", AmountCents: 998, PaidAt: &t0}
	svc.paymentsByID["undated"] = &Payment{ID: "undated", OrgID: p.OrgID, InvoiceID: inv1.ID, Method: "check", Status: "succeeded", AmountCents: 997}
	svc.mu.Unlock()
	rows, err = svc.QuickBooksCSV(p.OrgID, p, "payments", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows[0], []string{"Date", "Customer", "Payment Method", "Reference No", "Amount", "Invoice No", "Memo"}) || len(rows) != 4 {
		t.Fatalf("payment export shape: %v", rows)
	}
	if !reflect.DeepEqual(rows[1], []string{"11/10/2026", "'=EVIL()", "Stripe", "pi_cov2x", "5.00", "QB-2", ""}) {
		t.Fatalf("stripe payment: %v", rows[1])
	}
	if !reflect.DeepEqual(rows[2], []string{"11/15/2026", "", "Ach", "", "2.50", "", ""}) {
		t.Fatalf("payment without an invoice keeps blank customer: %v", rows[2])
	}
	if !reflect.DeepEqual(rows[3], []string{"11/22/2026", "Alpha", "Check", "CHK-77", "10.00", "QB-1", "'-note"}) {
		t.Fatalf("check payment: %v", rows[3])
	}
}

func TestCov2xQuickBooksInvoiceWithoutDueDateLeavesBlank(t *testing.T) {
	t.Parallel()
	svc, p := cov2xReportService(t)
	inv := cov2xIssuedInvoice(t, svc, p, "hh-a", "QBD-1", "", 100)
	cov2xSetIssuedAt(t, svc, inv.ID, "2026-11-02")
	rows, err := svc.QuickBooksCSV(p.OrgID, p, "anything-else-is-invoices", cov2xDate(t, "2026-11-01"), cov2xDate(t, "2026-11-30"))
	if err != nil || len(rows) != 2 || rows[1][3] != "" || !strings.HasPrefix(rows[1][0], "QBD-1") {
		t.Fatalf("%v %v", rows, err)
	}
}

func TestCov2xRefreshOrgIsNoOpInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if err := svc.refreshOrg(t.Context(), "org"); err != nil {
		t.Fatal(err)
	}
	if err := svc.refreshForRead("org"); err != nil {
		t.Fatal(err)
	}
	called := false
	svc.SetSyncHookForTest(func() { called = true })
	if err := svc.refreshOrg(t.Context(), "org"); err != nil || called {
		t.Fatalf("in-memory refresh never reads the database: called=%v err=%v", called, err)
	}
}
