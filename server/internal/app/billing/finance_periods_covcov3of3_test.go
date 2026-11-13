package billing

import (
	"testing"
	"time"
)

func cov3xDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := parseDate(s)
	if err != nil {
		t.Fatalf("parseDate(%q): %v", s, err)
	}
	return d
}

func TestCov3xParseDate(t *testing.T) {
	got, err := parseDate("  2026-02-03 ")
	if err != nil {
		t.Fatalf("whitespace must be trimmed: %v", err)
	}
	if want := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, bad := range []string{"", "2026/02/03", "2026-13-01", "not-a-date"} {
		if v, err := parseDate(bad); err == nil || !v.IsZero() {
			t.Fatalf("parseDate(%q) = %v, %v; want zero time and error", bad, v, err)
		}
	}
}

func TestCov3xDaysInclusiveAndRoundDiv(t *testing.T) {
	a, b := cov3xDay(t, "2026-11-01"), cov3xDay(t, "2026-11-30")
	if got := daysInclusive(a, b); got != 30 {
		t.Fatalf("November = 30 days, got %d", got)
	}
	if got := daysInclusive(a, a); got != 1 {
		t.Fatalf("single day = 1, got %d", got)
	}
	if got := daysInclusive(b, a); got != 0 {
		t.Fatalf("reversed range = 0, got %d", got)
	}

	cases := []struct{ n, d, want int }{
		{1, 2, 1}, // half rounds up
		{1, 3, 0},
		{2, 3, 1},
		{5, 0, 0},  // non-positive denominators are guarded
		{5, -2, 0}, // not a division by zero or a sign flip
		{0, 7, 0},
	}
	for _, c := range cases {
		if got := roundDiv(c.n, c.d); got != c.want {
			t.Fatalf("roundDiv(%d,%d) = %d want %d", c.n, c.d, got, c.want)
		}
	}
}

func TestCov3xFrequencyMonths(t *testing.T) {
	cases := map[string]int{
		FrequencyMonthly:   1,
		FrequencyQuarterly: 3,
		FrequencyAnnual:    12,
		FrequencyOneTime:   0,
		"weekly":           0,
		"":                 0,
	}
	for freq, want := range cases {
		if got := frequencyMonths(freq); got != want {
			t.Fatalf("frequencyMonths(%q) = %d want %d", freq, got, want)
		}
	}
}

func TestCov3xPeriodFor(t *testing.T) {
	type want struct {
		ok         bool
		key        string
		start, end string
	}
	cases := []struct {
		name  string
		sched FeeSchedule
		asOf  string
		want  want
	}{
		{"unparseable start", FeeSchedule{Frequency: FrequencyMonthly, StartDate: "garbage"}, "2026-11-20", want{}},
		{"monthly is calendar month", FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-15"}, "2026-11-20", want{true, "2026-11-01", "2026-11-01", "2026-11-30"}},
		{"before start", FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-01"}, "2026-08-31", want{}},
		{"quarterly anchored on start month", FeeSchedule{Frequency: FrequencyQuarterly, StartDate: "2026-09-15"}, "2027-01-10", want{true, "2026-12-01", "2026-12-01", "2027-02-28"}},
		{"quarterly first period", FeeSchedule{Frequency: FrequencyQuarterly, StartDate: "2026-09-15"}, "2026-09-20", want{true, "2026-09-01", "2026-09-01", "2026-11-30"}},
		{"annual within first year", FeeSchedule{Frequency: FrequencyAnnual, StartDate: "2026-09-01"}, "2027-08-31", want{true, "2026-09-01", "2026-09-01", "2027-08-31"}},
		{"annual rolls over", FeeSchedule{Frequency: FrequencyAnnual, StartDate: "2026-09-01"}, "2027-09-01", want{true, "2027-09-01", "2027-09-01", "2028-08-31"}},
		{"period begins after end date", FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-01", EndDate: "2026-12-15"}, "2027-01-05", want{}},
		{"period straddling end date still billed", FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-01", EndDate: "2026-12-15"}, "2026-12-20", want{true, "2026-12-01", "2026-12-01", "2026-12-31"}},
		{"invalid end date is ignored", FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-01", EndDate: "bogus"}, "2030-01-10", want{true, "2030-01-01", "2030-01-01", "2030-01-31"}},
		{"one-time without end spans start day", FeeSchedule{Frequency: FrequencyOneTime, StartDate: "2026-09-01"}, "2027-05-05", want{true, "once:2026-09-01", "2026-09-01", "2026-09-01"}},
		{"one-time with end spans window", FeeSchedule{Frequency: FrequencyOneTime, StartDate: "2026-09-01", EndDate: "2026-10-31"}, "2026-10-15", want{true, "once:2026-09-01", "2026-09-01", "2026-10-31"}},
		{"one-time after end", FeeSchedule{Frequency: FrequencyOneTime, StartDate: "2026-09-01", EndDate: "2026-10-31"}, "2026-11-01", want{}},
		{"unknown frequency", FeeSchedule{Frequency: "weekly", StartDate: "2026-09-01"}, "2026-09-10", want{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sched := c.sched
			p, ok := periodFor(&sched, cov3xDay(t, c.asOf))
			if ok != c.want.ok {
				t.Fatalf("ok = %v want %v (period %+v)", ok, c.want.ok, p)
			}
			if !ok {
				if p != (billingPeriod{}) {
					t.Fatalf("failed lookups must return the zero period, got %+v", p)
				}
				return
			}
			if p.Key != c.want.key || fmtDate(p.Start) != c.want.start || fmtDate(p.End) != c.want.end {
				t.Fatalf("period = %s [%s..%s], want %s [%s..%s]", p.Key, fmtDate(p.Start), fmtDate(p.End), c.want.key, c.want.start, c.want.end)
			}
		})
	}
}

func TestCov3xNextPeriodAfter(t *testing.T) {
	monthly := &FeeSchedule{Frequency: FrequencyMonthly, StartDate: "2026-09-01", EndDate: "2026-12-20"}
	nov, ok := periodFor(monthly, cov3xDay(t, "2026-11-10"))
	if !ok {
		t.Fatal("november period missing")
	}
	dec, ok := nextPeriodAfter(monthly, nov)
	if !ok || dec.Key != "2026-12-01" {
		t.Fatalf("next after november = %+v %v", dec, ok)
	}
	if jan, ok := nextPeriodAfter(monthly, dec); ok {
		t.Fatalf("no period may start after the schedule end date, got %+v", jan)
	}

	once := &FeeSchedule{Frequency: FrequencyOneTime, StartDate: "2026-09-01"}
	cur, _ := periodFor(once, cov3xDay(t, "2026-09-01"))
	if next, ok := nextPeriodAfter(once, cur); ok {
		t.Fatalf("one-time schedules have no next period, got %+v", next)
	}
}

func TestCov3xProrate(t *testing.T) {
	period := billingPeriod{Key: "2026-11-01", Start: cov3xDay(t, "2026-11-01"), End: cov3xDay(t, "2026-11-30")}
	d := func(s string) time.Time { return cov3xDay(t, s) }
	cases := []struct {
		name       string
		rule       string
		from, to   string
		wantAmount int
		wantNote   string
	}{
		{"full period", ProrationDaily, "2026-11-01", "2026-11-30", 15000, ""},
		{"active range wider than the period is clamped", ProrationDaily, "2026-10-01", "2026-12-31", 15000, ""},
		{"active only after the period", ProrationDaily, "2026-12-01", "2026-12-31", 0, "not active in period"},
		{"active ended before the period", ProrationDaily, "2026-09-01", "2026-10-31", 0, "not active in period"},
		{"daily proration", ProrationDaily, "2026-11-16", "2026-11-30", 7500, "prorated 15/30 days"},
		{"daily proration rounds half up", ProrationDaily, "2026-11-30", "2026-11-30", 500, "prorated 1/30 days"},
		{"half rule under half the period", ProrationHalf, "2026-11-21", "2026-11-30", 7500, "half period"},
		{"half rule at half the period is full", ProrationHalf, "2026-11-16", "2026-11-30", 15000, ""},
		{"waive rule", ProrationWaive, "2026-11-10", "2026-11-30", 0, "waived for partial period"},
		{"none rule charges in full", ProrationNone, "2026-11-10", "2026-11-30", 15000, ""},
		{"unknown rule charges in full", "mystery", "2026-11-10", "2026-11-30", 15000, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, note := prorate(15000, c.rule, period, d(c.from), d(c.to))
			if got != c.wantAmount || note != c.wantNote {
				t.Fatalf("prorate = (%d, %q) want (%d, %q)", got, note, c.wantAmount, c.wantNote)
			}
		})
	}

	empty := billingPeriod{Key: "x", Start: d("2026-11-30"), End: d("2026-11-01")}
	if got, note := prorate(100, ProrationDaily, empty, d("2026-11-01"), d("2026-11-30")); got != 0 || note != "not active in period" {
		t.Fatalf("inverted period = (%d, %q)", got, note)
	}
}

func TestCov3xPickDiscount(t *testing.T) {
	two := 2
	rules := []*DiscountRule{
		{ID: "r-inactive", Name: "Inactive", Active: false, AppliesTo: DiscountAppliesAll, MinPosition: 2, DiscountType: DiscountTypePercent, PercentBps: 9000},
		{ID: "r-reg", Name: "Registration only", Active: true, AppliesTo: DiscountAppliesRegistration, MinPosition: 2, DiscountType: DiscountTypePercent, PercentBps: 5000},
		{ID: "r-third", Name: "Third plus", Active: true, AppliesTo: DiscountAppliesDues, MinPosition: 3, DiscountType: DiscountTypePercent, PercentBps: 2000},
		{ID: "r-second", Name: "Second only", Active: true, AppliesTo: DiscountAppliesDues, MinPosition: 2, MaxPosition: &two, DiscountType: DiscountTypePercent, PercentBps: 1000},
		{ID: "r-fixed", Name: "Fixed five", Active: true, AppliesTo: DiscountAppliesAll, MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 500},
	}

	if r, amt := pickDiscount(rules, ScheduleKindDues, 1, 10000); r != nil || amt != 0 {
		t.Fatalf("first swimmer never discounted: %v %d", r, amt)
	}
	if r, amt := pickDiscount(rules, ScheduleKindDues, 2, 0); r != nil || amt != 0 {
		t.Fatalf("zero gross never discounted: %v %d", r, amt)
	}
	// Position 2, dues: "Second only" 10% = 1000 beats fixed 500; inactive/registration rules ignored.
	r, amt := pickDiscount(rules, ScheduleKindDues, 2, 10000)
	if r == nil || r.ID != "r-second" || amt != 1000 {
		t.Fatalf("position 2: %+v %d", r, amt)
	}
	// Position 3, dues: max_position excludes "Second only"; "Third plus" 20% = 2000 wins.
	r, amt = pickDiscount(rules, ScheduleKindDues, 3, 10000)
	if r == nil || r.ID != "r-third" || amt != 2000 {
		t.Fatalf("position 3: %+v %d", r, amt)
	}
	// Registration lines: only the registration-only 50% and the all-kinds fixed apply.
	r, amt = pickDiscount(rules, ScheduleKindRegistration, 2, 4000)
	if r == nil || r.ID != "r-reg" || amt != 2000 {
		t.Fatalf("registration: %+v %d", r, amt)
	}
	// Nothing applies -> nil result.
	only := []*DiscountRule{{ID: "x", Name: "x", Active: true, AppliesTo: DiscountAppliesDues, MinPosition: 5, DiscountType: DiscountTypeFixed, AmountCents: 100}}
	if r, amt := pickDiscount(only, ScheduleKindDues, 2, 1000); r != nil || amt != 0 {
		t.Fatalf("no candidates: %v %d", r, amt)
	}
}

func TestCov3xPickDiscountTieBreaksAreDeterministic(t *testing.T) {
	mk := func(id, name string) *DiscountRule {
		return &DiscountRule{ID: id, Name: name, Active: true, AppliesTo: DiscountAppliesAll, MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 700}
	}
	// Same amount: the alphabetically first name wins, then the lowest ID, regardless of input order.
	orders := [][]*DiscountRule{
		{mk("b", "Beta"), mk("a2", "Alpha"), mk("a1", "Alpha")},
		{mk("a1", "Alpha"), mk("a2", "Alpha"), mk("b", "Beta")},
		{mk("a2", "Alpha"), mk("b", "Beta"), mk("a1", "Alpha")},
	}
	for i, rules := range orders {
		r, amt := pickDiscount(rules, ScheduleKindDues, 2, 5000)
		if r == nil || r.ID != "a1" || amt != 700 {
			t.Fatalf("order %d: got %+v %d, want rule a1", i, r, amt)
		}
	}
}

func TestCov3xDiscountAmountClamping(t *testing.T) {
	cases := []struct {
		name  string
		rule  DiscountRule
		gross int
		want  int
	}{
		{"percent", DiscountRule{DiscountType: DiscountTypePercent, PercentBps: 1500}, 10000, 1500},
		{"percent rounds half up", DiscountRule{DiscountType: DiscountTypePercent, PercentBps: 1}, 5000, 1},
		{"fixed", DiscountRule{DiscountType: DiscountTypeFixed, AmountCents: 2500}, 10000, 2500},
		{"fixed capped at gross", DiscountRule{DiscountType: DiscountTypeFixed, AmountCents: 99999}, 10000, 10000},
		{"percent over 100 capped at gross", DiscountRule{DiscountType: DiscountTypePercent, PercentBps: 20000}, 10000, 10000},
		{"negative amount floored at zero", DiscountRule{DiscountType: DiscountTypeFixed, AmountCents: -300}, 10000, 0},
		{"unknown type yields nothing", DiscountRule{DiscountType: "bogus", AmountCents: 300, PercentBps: 500}, 10000, 0},
	}
	for _, c := range cases {
		rule := c.rule
		if got := discountAmount(&rule, c.gross); got != c.want {
			t.Fatalf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestCov3xDescribeDiscount(t *testing.T) {
	cases := []struct {
		rule DiscountRule
		want string
	}{
		{DiscountRule{DiscountType: DiscountTypePercent, PercentBps: 1000}, "10%"},
		{DiscountRule{DiscountType: DiscountTypePercent, PercentBps: 1250}, "12.50%"},
		{DiscountRule{DiscountType: DiscountTypePercent, PercentBps: 705}, "7.05%"},
		{DiscountRule{DiscountType: DiscountTypeFixed, AmountCents: 2500}, "$25.00"},
		{DiscountRule{DiscountType: DiscountTypeFixed, AmountCents: 123456}, "$1,234.56"},
	}
	for _, c := range cases {
		rule := c.rule
		if got := describeDiscount(&rule); got != c.want {
			t.Fatalf("describeDiscount(%+v) = %q want %q", c.rule, got, c.want)
		}
	}
}

func TestCov3xOrdinal(t *testing.T) {
	cases := map[int]string{
		1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 10: "10th",
		11: "11th", 12: "12th", 13: "13th", 14: "14th",
		21: "21st", 22: "22nd", 23: "23rd", 101: "101st", 111: "111th", 112: "112th", 113: "113th",
	}
	for n, want := range cases {
		if got := ordinal(n); got != want {
			t.Fatalf("ordinal(%d) = %q want %q", n, got, want)
		}
	}
}

func TestCov3xMoneyFormatting(t *testing.T) {
	cents := map[int]string{
		0:          "$0.00",
		5:          "$0.05",
		99:         "$0.99",
		100:        "$1.00",
		123456:     "$1,234.56",
		123456789:  "$1,234,567.89",
		-500:       "-$5.00",
		-123456789: "-$1,234,567.89",
	}
	for c, want := range cents {
		if got := formatCents(c); got != want {
			t.Fatalf("formatCents(%d) = %q want %q", c, got, want)
		}
	}
	dec := map[int]string{0: "0.00", 7: "0.07", 123450: "1234.50", -1234: "-12.34"}
	for c, want := range dec {
		if got := centsDecimal(c); got != want {
			t.Fatalf("centsDecimal(%d) = %q want %q", c, got, want)
		}
	}
}

func TestCov3xDateHelpers(t *testing.T) {
	loc := time.FixedZone("UTC+9", 9*3600)
	got := dateOnly(time.Date(2026, 11, 8, 3, 30, 0, 0, loc)) // 2026-11-07 18:30 UTC
	if fmtDate(got) != "2026-11-07" || got.Location() != time.UTC {
		t.Fatalf("dateOnly must normalise to the UTC calendar day, got %v", got)
	}
	if ms := monthStart(time.Date(2026, 2, 28, 23, 0, 0, 0, time.UTC)); fmtDate(ms) != "2026-02-01" {
		t.Fatalf("monthStart = %v", ms)
	}
}
