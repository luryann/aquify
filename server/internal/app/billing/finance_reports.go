package billing

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ─── Report types ────────────────────────────────────────────────────────────

// AgingBuckets splits open balances by days past due.
type AgingBuckets struct {
	CurrentCents    int `json:"current_cents"`
	Days1To30Cents  int `json:"days_1_30_cents"`
	Days31To60Cents int `json:"days_31_60_cents"`
	Days61To90Cents int `json:"days_61_90_cents"`
	Days90PlusCents int `json:"days_90_plus_cents"`
	TotalCents      int `json:"total_cents"`
}

func (b *AgingBuckets) add(daysPastDue, cents int) {
	switch {
	case daysPastDue <= 0:
		b.CurrentCents += cents
	case daysPastDue <= 30:
		b.Days1To30Cents += cents
	case daysPastDue <= 60:
		b.Days31To60Cents += cents
	case daysPastDue <= 90:
		b.Days61To90Cents += cents
	default:
		b.Days90PlusCents += cents
	}
	b.TotalCents += cents
}

// AgingRow is one household's aged receivable.
type AgingRow struct {
	HouseholdID   string `json:"household_id"`
	HouseholdName string `json:"household_name"`
	AgingBuckets
	InvoiceCount  int    `json:"invoice_count"`
	OldestDueDate string `json:"oldest_due_date,omitempty"`
}

// AgingReport is accounts receivable aged as of a date.
type AgingReport struct {
	AsOf        string       `json:"as_of"`
	Rows        []AgingRow   `json:"rows"`
	Totals      AgingBuckets `json:"totals"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// RevenueRow is revenue for one (month, category, group) cell.
type RevenueRow struct {
	Month       string `json:"month"`
	Category    string `json:"category"`
	GroupCode   string `json:"group_code"`
	AmountCents int    `json:"amount_cents"`
}

// RevenueTotal is an aggregate along one dimension.
type RevenueTotal struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	AmountCents int    `json:"amount_cents"`
}

// RevenueReport is invoiced revenue (issued, not voided) by issue month.
type RevenueReport struct {
	From        string         `json:"from"`
	To          string         `json:"to"`
	Rows        []RevenueRow   `json:"rows"`
	ByMonth     []RevenueTotal `json:"by_month"`
	ByCategory  []RevenueTotal `json:"by_category"`
	ByGroup     []RevenueTotal `json:"by_group"`
	TotalCents  int            `json:"total_cents"`
	GeneratedAt time.Time      `json:"generated_at"`
}

// OutstandingRow is one household's open balance net of credit.
type OutstandingRow struct {
	HouseholdID        string `json:"household_id"`
	HouseholdName      string `json:"household_name"`
	OpenInvoiceCount   int    `json:"open_invoice_count"`
	AmountDueCents     int    `json:"amount_due_cents"`
	CreditBalanceCents int    `json:"credit_balance_cents"`
	NetBalanceCents    int    `json:"net_balance_cents"`
	OldestDueDate      string `json:"oldest_due_date,omitempty"`
}

// OutstandingReport lists household balances.
type OutstandingReport struct {
	Rows             []OutstandingRow `json:"rows"`
	TotalDueCents    int              `json:"total_due_cents"`
	TotalCreditCents int              `json:"total_credit_cents"`
	TotalNetCents    int              `json:"total_net_cents"`
	GeneratedAt      time.Time        `json:"generated_at"`
}

// DepositRow is cash received in one payout period.
type DepositRow struct {
	PeriodStart      string         `json:"period_start"`
	PeriodEnd        string         `json:"period_end"`
	PaymentCount     int            `json:"payment_count"`
	ByMethod         map[string]int `json:"by_method_cents"`
	GrossCents       int            `json:"gross_cents"`
	RefundsCents     int            `json:"refunds_cents"`
	OverpaymentCents int            `json:"overpayment_cents"`
	NetCents         int            `json:"net_cents"`
}

// DepositsReport is cash in/out grouped by payout period.
type DepositsReport struct {
	From        string       `json:"from"`
	To          string       `json:"to"`
	Period      string       `json:"period"`
	Rows        []DepositRow `json:"rows"`
	GrossCents  int          `json:"gross_cents"`
	RefundCents int          `json:"refunds_cents"`
	NetCents    int          `json:"net_cents"`
	GeneratedAt time.Time    `json:"generated_at"`
}

func (s *Service) householdNames(orgID string) map[string]string {
	names := map[string]string{}
	if roster, err := s.loadRoster(orgID); err == nil {
		for _, h := range roster.Households {
			names[h.ID] = h.Name
		}
	}
	return names
}

func nameOr(names map[string]string, id string) string {
	if n := names[id]; n != "" {
		return n
	}
	if id == "" {
		return "No household"
	}
	return id
}

func (s *Service) refreshForRead() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshLocked(context.Background())
}

func isOpenInvoice(inv *Invoice) bool {
	return (inv.Status == "issued" || inv.Status == "partially_paid") && inv.AmountDueCents > 0
}

// AgingReport ages open receivables as of asOf (admin only).
func (s *Service) AgingReport(orgID string, principal *appmiddleware.Principal, asOf time.Time) (*AgingReport, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	names := s.householdNames(orgID)
	asOf = dateOnly(asOf)
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := map[string]*AgingRow{}
	report := &AgingReport{AsOf: fmtDate(asOf), Rows: []AgingRow{}, GeneratedAt: time.Now().UTC()}
	for _, inv := range s.invoices {
		if inv.OrgID != orgID || !isOpenInvoice(inv) {
			continue
		}
		ref := inv.DueDate
		if ref == "" && inv.IssuedAt != nil {
			ref = fmtDate(*inv.IssuedAt)
		}
		days := 0
		if due, err := parseDate(ref); err == nil {
			days = daysInclusive(due, asOf) - 1
		}
		row := rows[inv.HouseholdID]
		if row == nil {
			row = &AgingRow{HouseholdID: inv.HouseholdID, HouseholdName: nameOr(names, inv.HouseholdID)}
			rows[inv.HouseholdID] = row
		}
		row.add(days, inv.AmountDueCents)
		report.Totals.add(days, inv.AmountDueCents)
		row.InvoiceCount++
		if ref != "" && (row.OldestDueDate == "" || ref < row.OldestDueDate) {
			row.OldestDueDate = ref
		}
	}
	for _, row := range rows {
		report.Rows = append(report.Rows, *row)
	}
	sort.Slice(report.Rows, func(i, j int) bool {
		if report.Rows[i].TotalCents != report.Rows[j].TotalCents {
			return report.Rows[i].TotalCents > report.Rows[j].TotalCents
		}
		return report.Rows[i].HouseholdName < report.Rows[j].HouseholdName
	})
	return report, nil
}

// RevenueReport summarises invoiced revenue between from and to (inclusive)
// by issue month, line category, and roster group (admin only).
func (s *Service) RevenueReport(orgID string, principal *appmiddleware.Principal, from, to time.Time) (*RevenueReport, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	groupBySwimmer := map[string]string{}
	groupNames := map[string]string{}
	if roster, err := s.loadRoster(orgID); err == nil {
		for _, g := range roster.Groups {
			groupNames[g.Code] = g.Name
		}
		for _, link := range roster.GroupLinks {
			if link.IsPrimary || groupBySwimmer[link.SwimmerID] == "" {
				groupBySwimmer[link.SwimmerID] = link.GroupCode
			}
		}
	}
	from, to = dateOnly(from), dateOnly(to)
	s.mu.RLock()
	defer s.mu.RUnlock()
	report := &RevenueReport{From: fmtDate(from), To: fmtDate(to), Rows: []RevenueRow{}, GeneratedAt: time.Now().UTC()}
	cells := map[[3]string]int{}
	for _, inv := range s.invoices {
		if inv.OrgID != orgID || inv.IssuedAt == nil || inv.Status == "draft" || inv.Status == "voided" {
			continue
		}
		issued := dateOnly(*inv.IssuedAt)
		if issued.Before(from) || issued.After(to) {
			continue
		}
		month := issued.Format("2006-01")
		for _, item := range s.items[inv.ID] {
			group := "UNASSIGNED"
			if item.SwimmerID == "" {
				group = "HOUSEHOLD"
			} else if g := groupBySwimmer[item.SwimmerID]; g != "" {
				group = g
			}
			cells[[3]string{month, item.Category, group}] += item.TotalCents
		}
	}
	byMonth, byCategory, byGroup := map[string]int{}, map[string]int{}, map[string]int{}
	for key, cents := range cells {
		report.Rows = append(report.Rows, RevenueRow{Month: key[0], Category: key[1], GroupCode: key[2], AmountCents: cents})
		byMonth[key[0]] += cents
		byCategory[key[1]] += cents
		byGroup[key[2]] += cents
		report.TotalCents += cents
	}
	sort.Slice(report.Rows, func(i, j int) bool {
		a, b := report.Rows[i], report.Rows[j]
		if a.Month != b.Month {
			return a.Month < b.Month
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.GroupCode < b.GroupCode
	})
	// Every month in range appears so charts have a continuous axis.
	for m := monthStart(from); !m.After(to); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01")
		report.ByMonth = append(report.ByMonth, RevenueTotal{Key: key, Label: m.Format("Jan 2006"), AmountCents: byMonth[key]})
	}
	for key, cents := range byCategory {
		report.ByCategory = append(report.ByCategory, RevenueTotal{Key: key, Label: categoryLabel(key), AmountCents: cents})
	}
	for key, cents := range byGroup {
		label := groupNames[key]
		switch {
		case key == "UNASSIGNED":
			label = "No group"
		case key == "HOUSEHOLD":
			label = "Household-level"
		case label == "":
			label = key
		}
		report.ByGroup = append(report.ByGroup, RevenueTotal{Key: key, Label: label, AmountCents: cents})
	}
	sortTotals := func(t []RevenueTotal) {
		sort.Slice(t, func(i, j int) bool {
			if t[i].AmountCents != t[j].AmountCents {
				return t[i].AmountCents > t[j].AmountCents
			}
			return t[i].Key < t[j].Key
		})
	}
	sortTotals(report.ByCategory)
	sortTotals(report.ByGroup)
	if report.ByMonth == nil {
		report.ByMonth = []RevenueTotal{}
	}
	if report.ByCategory == nil {
		report.ByCategory = []RevenueTotal{}
	}
	if report.ByGroup == nil {
		report.ByGroup = []RevenueTotal{}
	}
	return report, nil
}

func categoryLabel(category string) string {
	switch category {
	case "program_fee":
		return "Dues"
	case "registration_fee":
		return "Registration"
	case "meet_fee":
		return "Meet fees"
	case "late_fee":
		return "Late fees"
	case "discount":
		return "Discounts"
	case "adjustment":
		return "Adjustments"
	case "equipment":
		return "Equipment"
	case "manual":
		return "Manual"
	case "credit":
		return "Credits"
	}
	return category
}

// OutstandingReport lists every household with an open balance or credit.
func (s *Service) OutstandingReport(orgID string, principal *appmiddleware.Principal) (*OutstandingReport, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	names := s.householdNames(orgID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := map[string]*OutstandingRow{}
	get := func(id string) *OutstandingRow {
		row := rows[id]
		if row == nil {
			row = &OutstandingRow{HouseholdID: id, HouseholdName: nameOr(names, id)}
			rows[id] = row
		}
		return row
	}
	for _, inv := range s.invoices {
		if inv.OrgID != orgID || !isOpenInvoice(inv) {
			continue
		}
		row := get(inv.HouseholdID)
		row.OpenInvoiceCount++
		row.AmountDueCents += inv.AmountDueCents
		if inv.DueDate != "" && (row.OldestDueDate == "" || inv.DueDate < row.OldestDueDate) {
			row.OldestDueDate = inv.DueDate
		}
	}
	prefix := orgID + "|"
	for key, balance := range s.finance.balances {
		if balance <= 0 || !strings.HasPrefix(key, prefix) {
			continue
		}
		get(strings.TrimPrefix(key, prefix)).CreditBalanceCents = balance
	}
	report := &OutstandingReport{Rows: []OutstandingRow{}, GeneratedAt: time.Now().UTC()}
	for _, row := range rows {
		row.NetBalanceCents = row.AmountDueCents - row.CreditBalanceCents
		report.TotalDueCents += row.AmountDueCents
		report.TotalCreditCents += row.CreditBalanceCents
		report.TotalNetCents += row.NetBalanceCents
		report.Rows = append(report.Rows, *row)
	}
	sort.Slice(report.Rows, func(i, j int) bool {
		if report.Rows[i].NetBalanceCents != report.Rows[j].NetBalanceCents {
			return report.Rows[i].NetBalanceCents > report.Rows[j].NetBalanceCents
		}
		return report.Rows[i].HouseholdName < report.Rows[j].HouseholdName
	})
	return report, nil
}

func periodBucket(t time.Time, period string) (time.Time, time.Time) {
	t = dateOnly(t)
	switch period {
	case "day":
		return t, t
	case "week":
		offset := (int(t.Weekday()) + 6) % 7 // Monday-start weeks
		start := t.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 6)
	}
	start := monthStart(t)
	return start, start.AddDate(0, 1, -1)
}

// DepositsReport groups cash received (and refunded) by payout period.
// Account-credit applications are not cash and are excluded; overpayments
// that landed in account credit are cash and are included.
func (s *Service) DepositsReport(orgID string, principal *appmiddleware.Principal, from, to time.Time, period string) (*DepositsReport, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	if period != "day" && period != "week" {
		period = "month"
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	from, to = dateOnly(from), dateOnly(to)
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := map[string]*DepositRow{}
	get := func(t time.Time) *DepositRow {
		start, end := periodBucket(t, period)
		key := fmtDate(start)
		row := rows[key]
		if row == nil {
			row = &DepositRow{PeriodStart: key, PeriodEnd: fmtDate(end), ByMethod: map[string]int{}}
			rows[key] = row
		}
		return row
	}
	inRange := func(t time.Time) bool {
		d := dateOnly(t)
		return !d.Before(from) && !d.After(to)
	}
	for _, p := range s.paymentsByID {
		if p.OrgID != orgID || p.Method == PaymentMethodAccountCredit || p.PaidAt == nil {
			continue
		}
		if p.Status != "succeeded" && p.Status != "reversed" {
			continue
		}
		if !inRange(*p.PaidAt) {
			continue
		}
		row := get(*p.PaidAt)
		row.PaymentCount++
		row.ByMethod[p.Method] += p.AmountCents
		row.GrossCents += p.AmountCents
	}
	creditRefunds := map[string]bool{}
	for _, e := range s.finance.credits {
		if e.OrgID != orgID {
			continue
		}
		if e.RefundID != "" {
			creditRefunds[e.RefundID] = true
		}
		if e.Kind == CreditKindOverpayment && inRange(e.CreatedAt) {
			row := get(e.CreatedAt)
			row.OverpaymentCents += e.AmountCents
			row.GrossCents += e.AmountCents
		}
	}
	for _, r := range s.refundsByID {
		if r.OrgID != orgID || r.Status != "processed" || creditRefunds[r.ID] {
			continue
		}
		at := r.CreatedAt
		if r.ProcessedAt != nil {
			at = *r.ProcessedAt
		}
		if !inRange(at) {
			continue
		}
		get(at).RefundsCents += r.AmountCents
	}
	report := &DepositsReport{From: fmtDate(from), To: fmtDate(to), Period: period, Rows: []DepositRow{}, GeneratedAt: time.Now().UTC()}
	for _, row := range rows {
		row.NetCents = row.GrossCents - row.RefundsCents
		report.GrossCents += row.GrossCents
		report.RefundCents += row.RefundsCents
		report.NetCents += row.NetCents
		report.Rows = append(report.Rows, *row)
	}
	sort.Slice(report.Rows, func(i, j int) bool { return report.Rows[i].PeriodStart < report.Rows[j].PeriodStart })
	return report, nil
}

// ─── CSV exports ─────────────────────────────────────────────────────────────

// csvSafe neutralises spreadsheet formula injection in free-text cells.
func csvSafe(value string) string {
	if value == "" {
		return value
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// CSV renders the aging report.
func (r *AgingReport) CSV() [][]string {
	out := [][]string{{"Household", "Household ID", "Current", "1-30", "31-60", "61-90", "90+", "Total", "Open invoices", "Oldest due date"}}
	for _, row := range r.Rows {
		out = append(out, []string{csvSafe(row.HouseholdName), row.HouseholdID, centsDecimal(row.CurrentCents),
			centsDecimal(row.Days1To30Cents), centsDecimal(row.Days31To60Cents), centsDecimal(row.Days61To90Cents),
			centsDecimal(row.Days90PlusCents), centsDecimal(row.TotalCents), strconv.Itoa(row.InvoiceCount), row.OldestDueDate})
	}
	t := r.Totals
	out = append(out, []string{"Total", "", centsDecimal(t.CurrentCents), centsDecimal(t.Days1To30Cents),
		centsDecimal(t.Days31To60Cents), centsDecimal(t.Days61To90Cents), centsDecimal(t.Days90PlusCents),
		centsDecimal(t.TotalCents), "", ""})
	return out
}

// CSV renders the revenue report (one row per month/category/group cell).
func (r *RevenueReport) CSV() [][]string {
	out := [][]string{{"Month", "Category", "Group", "Amount"}}
	for _, row := range r.Rows {
		out = append(out, []string{row.Month, categoryLabel(row.Category), csvSafe(row.GroupCode), centsDecimal(row.AmountCents)})
	}
	out = append(out, []string{"Total", "", "", centsDecimal(r.TotalCents)})
	return out
}

// CSV renders the outstanding balances report.
func (r *OutstandingReport) CSV() [][]string {
	out := [][]string{{"Household", "Household ID", "Open invoices", "Amount due", "Account credit", "Net balance", "Oldest due date"}}
	for _, row := range r.Rows {
		out = append(out, []string{csvSafe(row.HouseholdName), row.HouseholdID, strconv.Itoa(row.OpenInvoiceCount),
			centsDecimal(row.AmountDueCents), centsDecimal(row.CreditBalanceCents), centsDecimal(row.NetBalanceCents), row.OldestDueDate})
	}
	out = append(out, []string{"Total", "", "", centsDecimal(r.TotalDueCents), centsDecimal(r.TotalCreditCents), centsDecimal(r.TotalNetCents), ""})
	return out
}

// CSV renders the deposits report.
func (r *DepositsReport) CSV() [][]string {
	methods := []string{"stripe", "ach", "check", "cash", "other"}
	header := []string{"Period start", "Period end", "Payments"}
	for _, m := range methods {
		header = append(header, strings.ToUpper(m[:1])+m[1:])
	}
	header = append(header, "Overpayments to credit", "Gross", "Refunds", "Net deposit")
	out := [][]string{header}
	for _, row := range r.Rows {
		line := []string{row.PeriodStart, row.PeriodEnd, strconv.Itoa(row.PaymentCount)}
		for _, m := range methods {
			line = append(line, centsDecimal(row.ByMethod[m]))
		}
		line = append(line, centsDecimal(row.OverpaymentCents), centsDecimal(row.GrossCents), centsDecimal(row.RefundsCents), centsDecimal(row.NetCents))
		out = append(out, line)
	}
	return out
}

// QuickBooksCSV exports invoices or payments in the column layout QuickBooks
// Online's CSV importers accept (one row per invoice line / per payment).
func (s *Service) QuickBooksCSV(orgID string, principal *appmiddleware.Principal, kind string, from, to time.Time) ([][]string, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	names := s.householdNames(orgID)
	from, to = dateOnly(from), dateOnly(to)
	inRange := func(t time.Time) bool {
		d := dateOnly(t)
		return !d.Before(from) && !d.After(to)
	}
	qbDate := func(t time.Time) string { return t.Format("01/02/2006") }
	s.mu.RLock()
	defer s.mu.RUnlock()
	if kind == "payments" {
		out := [][]string{{"Date", "Customer", "Payment Method", "Reference No", "Amount", "Invoice No", "Memo"}}
		payments := make([]*Payment, 0)
		for _, p := range s.paymentsByID {
			if p.OrgID != orgID || p.PaidAt == nil || p.Method == PaymentMethodAccountCredit || (p.Status != "succeeded" && p.Status != "reversed") {
				continue
			}
			if inRange(*p.PaidAt) {
				payments = append(payments, p)
			}
		}
		sort.Slice(payments, func(i, j int) bool { return payments[i].PaidAt.Before(*payments[j].PaidAt) })
		for _, p := range payments {
			inv := s.invoices[p.InvoiceID]
			customer, number := "", ""
			if inv != nil {
				customer, number = nameOr(names, inv.HouseholdID), inv.InvoiceNumber
			}
			ref := p.ExternalRef
			if ref == "" {
				ref = p.StripePaymentIntentID
			}
			out = append(out, []string{qbDate(*p.PaidAt), csvSafe(customer), strings.ToUpper(p.Method[:1]) + p.Method[1:],
				csvSafe(ref), centsDecimal(p.AmountCents), csvSafe(number), csvSafe(p.Notes)})
		}
		return out, nil
	}
	out := [][]string{{"InvoiceNo", "Customer", "InvoiceDate", "DueDate", "Memo", "Item(Product/Service)", "ItemDescription", "ItemQuantity", "ItemRate", "ItemAmount"}}
	invoices := make([]*Invoice, 0)
	for _, inv := range s.invoices {
		if inv.OrgID != orgID || inv.IssuedAt == nil || inv.Status == "draft" || inv.Status == "voided" {
			continue
		}
		if inRange(*inv.IssuedAt) {
			invoices = append(invoices, inv)
		}
	}
	sort.Slice(invoices, func(i, j int) bool { return invoices[i].InvoiceNumber < invoices[j].InvoiceNumber })
	for _, inv := range invoices {
		due := ""
		if d, err := parseDate(inv.DueDate); err == nil {
			due = qbDate(d)
		}
		for _, item := range s.items[inv.ID] {
			out = append(out, []string{csvSafe(inv.InvoiceNumber), csvSafe(nameOr(names, inv.HouseholdID)), qbDate(*inv.IssuedAt), due,
				csvSafe(inv.Notes), categoryLabel(item.Category), csvSafe(item.Description), strconv.Itoa(item.Quantity),
				centsDecimal(item.UnitAmountCents), centsDecimal(item.TotalCents)})
		}
	}
	return out, nil
}
