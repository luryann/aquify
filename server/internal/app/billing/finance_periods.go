package billing

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// billingPeriod is an inclusive [Start, End] date range at UTC midnight.
type billingPeriod struct {
	Key   string
	Start time.Time
	End   time.Time
}

func parseDate(value string) (time.Time, error) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func fmtDate(t time.Time) string { return t.Format(dateLayout) }

func dateOnly(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// daysInclusive returns the number of calendar days in [a, b] (0 when b < a).
func daysInclusive(a, b time.Time) int {
	if b.Before(a) {
		return 0
	}
	return int(b.Sub(a).Hours()/24) + 1
}

// roundDiv divides with round-half-up semantics for non-negative operands.
func roundDiv(numerator, denominator int) int {
	if denominator <= 0 {
		return 0
	}
	return (numerator*2 + denominator) / (2 * denominator)
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func frequencyMonths(frequency string) int {
	switch frequency {
	case FrequencyMonthly:
		return 1
	case FrequencyQuarterly:
		return 3
	case FrequencyAnnual:
		return 12
	}
	return 0
}

// periodFor returns the schedule period containing asOf. Monthly periods are
// calendar months; quarterly/annual periods are anchored on the month of the
// schedule's start date; one-time schedules have a single period spanning
// [start_date, end_date|start_date].
func periodFor(schedule *FeeSchedule, asOf time.Time) (billingPeriod, bool) {
	start, err := parseDate(schedule.StartDate)
	if err != nil {
		return billingPeriod{}, false
	}
	var end time.Time
	hasEnd := false
	if strings.TrimSpace(schedule.EndDate) != "" {
		if end, err = parseDate(schedule.EndDate); err == nil {
			hasEnd = true
		}
	}
	asOf = dateOnly(asOf)
	if asOf.Before(start) {
		return billingPeriod{}, false
	}
	if schedule.Frequency == FrequencyOneTime {
		periodEnd := start
		if hasEnd {
			periodEnd = end
			if asOf.After(end) {
				return billingPeriod{}, false
			}
		}
		return billingPeriod{Key: "once:" + fmtDate(start), Start: start, End: periodEnd}, true
	}
	n := frequencyMonths(schedule.Frequency)
	if n == 0 {
		return billingPeriod{}, false
	}
	anchor := monthStart(start)
	if n == 1 {
		anchor = monthStart(asOf)
	}
	months := (asOf.Year()-anchor.Year())*12 + int(asOf.Month()-anchor.Month())
	ps := anchor.AddDate(0, (months/n)*n, 0)
	pe := ps.AddDate(0, n, -1)
	if hasEnd && ps.After(end) {
		return billingPeriod{}, false
	}
	return billingPeriod{Key: fmtDate(ps), Start: ps, End: pe}, true
}

// nextPeriodAfter returns the first schedule period that starts after the
// given period (used for upcoming-charge projections).
func nextPeriodAfter(schedule *FeeSchedule, current billingPeriod) (billingPeriod, bool) {
	if schedule.Frequency == FrequencyOneTime {
		return billingPeriod{}, false
	}
	return periodFor(schedule, current.End.AddDate(0, 0, 1))
}

// prorate applies the schedule's proration rule to a member active for
// [activeStart, activeEnd] within the period. It returns the charge and a
// human-readable note (empty when the full amount applies).
func prorate(amount int, rule string, period billingPeriod, activeStart, activeEnd time.Time) (int, string) {
	if activeStart.Before(period.Start) {
		activeStart = period.Start
	}
	if activeEnd.After(period.End) {
		activeEnd = period.End
	}
	periodDays := daysInclusive(period.Start, period.End)
	activeDays := daysInclusive(activeStart, activeEnd)
	if activeDays <= 0 || periodDays <= 0 {
		return 0, "not active in period"
	}
	if activeDays >= periodDays {
		return amount, ""
	}
	switch rule {
	case ProrationDaily:
		return roundDiv(amount*activeDays, periodDays), fmt.Sprintf("prorated %d/%d days", activeDays, periodDays)
	case ProrationHalf:
		if activeDays*2 < periodDays {
			return roundDiv(amount, 2), "half period"
		}
		return amount, ""
	case ProrationWaive:
		return 0, "waived for partial period"
	}
	return amount, ""
}

// pickDiscount selects the single best sibling tier for a line: active rules
// that match the line kind and position; the largest discount wins (rules do
// not stack). Ties break on rule name then ID for determinism.
func pickDiscount(rules []*DiscountRule, kind string, position, gross int) (*DiscountRule, int) {
	if position < 2 || gross <= 0 {
		return nil, 0
	}
	candidates := make([]*DiscountRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if rule.AppliesTo != DiscountAppliesAll && rule.AppliesTo != kind {
			continue
		}
		if position < rule.MinPosition {
			continue
		}
		if rule.MaxPosition != nil && position > *rule.MaxPosition {
			continue
		}
		candidates = append(candidates, rule)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].ID < candidates[j].ID
	})
	var best *DiscountRule
	bestAmount := 0
	for _, rule := range candidates {
		amount := discountAmount(rule, gross)
		if amount > bestAmount {
			best, bestAmount = rule, amount
		}
	}
	return best, bestAmount
}

func discountAmount(rule *DiscountRule, gross int) int {
	amount := 0
	switch rule.DiscountType {
	case DiscountTypePercent:
		amount = roundDiv(gross*rule.PercentBps, 10000)
	case DiscountTypeFixed:
		amount = rule.AmountCents
	}
	if amount > gross {
		amount = gross
	}
	if amount < 0 {
		amount = 0
	}
	return amount
}

func describeDiscount(rule *DiscountRule) string {
	if rule.DiscountType == DiscountTypePercent {
		whole := rule.PercentBps / 100
		frac := rule.PercentBps % 100
		if frac == 0 {
			return fmt.Sprintf("%d%%", whole)
		}
		return fmt.Sprintf("%d.%02d%%", whole, frac)
	}
	return formatCents(rule.AmountCents)
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// formatCents renders cents as a dollar string ("$1,234.50", "-$5.00").
func formatCents(cents int) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := cents / 100
	digits := fmt.Sprintf("%d", whole)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s$%s.%02d", sign, b.String(), cents%100)
}

// centsDecimal renders cents as a plain decimal ("1234.50") for CSV exports.
func centsDecimal(cents int) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}
