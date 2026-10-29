package billing

import (
	"fmt"
	"sort"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// StatementLine is one row of a household statement. AmountCents is the
// signed effect on what the household owes (charges +, payments/credits −);
// informational rows (credit applied, refunds back to card) move money
// between buckets without changing the balance and carry DisplayAmountCents.
type StatementLine struct {
	Date               string `json:"date"`
	Type               string `json:"type"`
	Description        string `json:"description"`
	Reference          string `json:"reference,omitempty"`
	InvoiceID          string `json:"invoice_id,omitempty"`
	AmountCents        int    `json:"amount_cents"`
	DisplayAmountCents int    `json:"display_amount_cents"`
	BalanceCents       int    `json:"balance_cents"`
	Informational      bool   `json:"informational"`

	at  time.Time
	seq int
}

// Statement is a household account statement.
type Statement struct {
	HouseholdID          string          `json:"household_id"`
	From                 string          `json:"from,omitempty"`
	To                   string          `json:"to"`
	OpeningBalanceCents  int             `json:"opening_balance_cents"`
	ClosingBalanceCents  int             `json:"closing_balance_cents"`
	ChargesCents         int             `json:"charges_cents"`
	PaymentsCents        int             `json:"payments_cents"`
	CreditsCents         int             `json:"credits_cents"`
	CreditBalanceCents   int             `json:"credit_balance_cents"`
	OpenInvoicesDueCents int             `json:"open_invoices_due_cents"`
	Lines                []StatementLine `json:"lines"`
}

// HouseholdStatement builds the statement for a household (admin, or the
// household's own parent). from may be zero for "since the beginning".
func (s *Service) HouseholdStatement(orgID string, principal *appmiddleware.Principal, householdID string, from, to time.Time) (*Statement, error) {
	householdID = strings.TrimSpace(householdID)
	if err := s.requireHouseholdFinanceRead(principal, householdID); err != nil {
		return nil, err
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	lines, open := s.statementLinesLocked(orgID, householdID)

	st := &Statement{
		HouseholdID: householdID, To: fmtDate(to), Lines: []StatementLine{},
		CreditBalanceCents:   s.finance.balances[balanceKey(orgID, householdID)],
		OpenInvoicesDueCents: open,
	}
	if !from.IsZero() {
		st.From = fmtDate(from)
	}
	end := dateOnly(to).AddDate(0, 0, 1)
	balance := 0
	for _, line := range lines {
		if !line.at.Before(end) {
			break
		}
		balance += line.AmountCents
		line.BalanceCents = balance
		if !from.IsZero() && line.at.Before(dateOnly(from)) {
			st.OpeningBalanceCents = balance
			continue
		}
		switch {
		case line.Informational:
		case line.AmountCents > 0:
			st.ChargesCents += line.AmountCents
		case line.Type == "payment":
			st.PaymentsCents += -line.AmountCents
		default:
			st.CreditsCents += -line.AmountCents
		}
		st.Lines = append(st.Lines, line)
	}
	st.ClosingBalanceCents = balance
	return st, nil
}

// statementLinesLocked collects the household's statement lines in date order
// (callers hold s.mu) plus the amount due on its open invoices.
func (s *Service) statementLinesLocked(orgID, householdID string) (lines []StatementLine, open int) {
	lines = make([]StatementLine, 0)
	add := func(at time.Time, typ, desc, ref, invoiceID string, effect, display int, info bool) {
		lines = append(lines, StatementLine{
			Date: fmtDate(at), Type: typ, Description: desc, Reference: ref, InvoiceID: invoiceID,
			AmountCents: effect, DisplayAmountCents: display, Informational: info, at: at, seq: len(lines),
		})
	}
	issuedAmount := map[string]int{}
	for _, ev := range s.ledger {
		if ev.OrgID == orgID && ev.EventType == "invoice_issued" {
			issuedAmount[ev.InvoiceID] += ev.AmountCents
		}
	}
	creditRefunds := map[string]bool{}
	for _, e := range s.finance.credits {
		if e.OrgID == orgID && e.RefundID != "" {
			creditRefunds[e.RefundID] = true
		}
	}
	for _, inv := range s.invoices {
		if inv.OrgID != orgID || inv.HouseholdID != householdID || inv.IssuedAt == nil {
			continue
		}
		if isOpenInvoice(inv) {
			open += inv.AmountDueCents
		}
		amount, ok := issuedAmount[inv.ID]
		if !ok {
			amount = inv.TotalCents
			if app, has := s.finance.lateFees[orgID+"|"+inv.ID]; has {
				amount -= app.AmountCents
			}
		}
		add(*inv.IssuedAt, "invoice", "Invoice "+inv.InvoiceNumber, inv.InvoiceNumber, inv.ID, amount, amount, false)
		if app, has := s.finance.lateFees[orgID+"|"+inv.ID]; has {
			add(app.AppliedAt, "late_fee", "Late fee on invoice "+inv.InvoiceNumber, inv.InvoiceNumber, inv.ID, app.AmountCents, app.AmountCents, false)
		}
		for _, p := range s.payments[inv.ID] {
			if p.PaidAt == nil || (p.Status != "succeeded" && p.Status != "reversed") {
				continue
			}
			if p.Method == PaymentMethodAccountCredit {
				add(*p.PaidAt, "credit_applied", "Account credit applied to "+inv.InvoiceNumber, inv.InvoiceNumber, inv.ID, 0, -p.AmountCents, true)
				continue
			}
			add(*p.PaidAt, "payment", fmt.Sprintf("Payment (%s) — %s", p.Method, inv.InvoiceNumber), inv.InvoiceNumber, inv.ID, -p.AmountCents, -p.AmountCents, false)
		}
		for _, r := range s.refunds[inv.ID] {
			if r.Status != "processed" || creditRefunds[r.ID] {
				continue
			}
			at := r.CreatedAt
			if r.ProcessedAt != nil {
				at = *r.ProcessedAt
			}
			add(at, "refund", "Refund to original payment method — "+inv.InvoiceNumber, inv.InvoiceNumber, inv.ID, 0, r.AmountCents, true)
		}
	}
	for _, ev := range s.ledger {
		if ev.OrgID != orgID || (ev.EventType != "invoice_voided" && ev.EventType != "invoice_written_off") {
			continue
		}
		inv := s.invoices[ev.InvoiceID]
		if inv == nil || inv.HouseholdID != householdID {
			continue
		}
		label := "Invoice voided"
		typ := "void"
		if ev.EventType == "invoice_written_off" {
			label, typ = "Balance written off", "write_off"
		}
		add(ev.CreatedAt, typ, label+" — "+inv.InvoiceNumber, inv.InvoiceNumber, inv.ID, ev.AmountCents, ev.AmountCents, false)
	}
	for _, e := range s.finance.credits {
		if e.OrgID != orgID || e.HouseholdID != householdID || e.Kind == CreditKindApplied {
			continue
		}
		desc := map[string]string{
			CreditKindManualCredit:   "Account credit",
			CreditKindManualDebit:    "Account credit removed",
			CreditKindOverpayment:    "Overpayment held as account credit",
			CreditKindRefundToCredit: "Refund to account credit",
		}[e.Kind]
		if e.Reason != "" {
			desc += " — " + e.Reason
		}
		add(e.CreatedAt, "credit", desc, "", e.InvoiceID, -e.AmountCents, -e.AmountCents, false)
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if !lines[i].at.Equal(lines[j].at) {
			return lines[i].at.Before(lines[j].at)
		}
		return lines[i].seq < lines[j].seq
	})
	return lines, open
}

// UpcomingCharge is a projected recurring charge not yet billed.
type UpcomingCharge struct {
	ScheduleID    string `json:"schedule_id"`
	ScheduleName  string `json:"schedule_name"`
	Frequency     string `json:"frequency"`
	SwimmerID     string `json:"swimmer_id"`
	SwimmerName   string `json:"swimmer_name"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	GrossCents    int    `json:"gross_cents"`
	DiscountCents int    `json:"discount_cents"`
	NetCents      int    `json:"net_cents"`
}

// UpcomingCharges lists projected charges within the horizon.
type UpcomingCharges struct {
	HouseholdID        string           `json:"household_id"`
	Through            string           `json:"through"`
	Charges            []UpcomingCharge `json:"charges"`
	TotalCents         int              `json:"total_cents"`
	CreditBalanceCents int              `json:"credit_balance_cents"`
}

// HouseholdUpcomingCharges projects unbilled recurring dues for the
// household's swimmers over the next horizonDays (estimate: sibling tiers by
// current ranking; proration not applied to future full periods).
func (s *Service) HouseholdUpcomingCharges(orgID string, principal *appmiddleware.Principal, householdID string, today time.Time, horizonDays int) (*UpcomingCharges, error) {
	householdID = strings.TrimSpace(householdID)
	if err := s.requireHouseholdFinanceRead(principal, householdID); err != nil {
		return nil, err
	}
	if horizonDays <= 0 || horizonDays > 366 {
		horizonDays = 90
	}
	if err := s.refreshForRead(); err != nil {
		return nil, err
	}
	roster, err := s.loadRoster(orgID)
	if err != nil {
		return nil, err
	}
	today = dateOnly(today)
	through := today.AddDate(0, 0, horizonDays)
	swimmers := map[string]RosterSwimmer{}
	for _, sw := range roster.Swimmers {
		if sw.HouseholdID == householdID && sw.Status == "active" {
			swimmers[sw.ID] = sw
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := &UpcomingCharges{HouseholdID: householdID, Through: fmtDate(through), Charges: []UpcomingCharge{},
		CreditBalanceCents: s.finance.balances[balanceKey(orgID, householdID)]}
	rules := make([]*DiscountRule, 0)
	for _, rule := range s.finance.rules {
		if rule.OrgID == orgID {
			rules = append(rules, rule)
		}
	}
	type candidate struct {
		charge UpcomingCharge
		dob    string
	}
	cands := make([]candidate, 0)
	perSwimmer := map[string]int{}
	for _, sc := range s.finance.schedules {
		if sc.OrgID != orgID || !sc.Active || sc.Kind != ScheduleKindDues || sc.Frequency == FrequencyOneTime {
			continue
		}
		for _, link := range roster.GroupLinks {
			sw, ok := swimmers[link.SwimmerID]
			if !ok || link.GroupCode != sc.GroupCode {
				continue
			}
			period, ok := periodFor(sc, today)
			if !ok {
				if start, err := parseDate(sc.StartDate); err == nil && start.After(today) {
					period, ok = periodFor(sc, start)
				}
			}
			for ok && !period.Start.After(through) {
				if _, billed := s.finance.chargeKeys[chargeKey(orgID, sc.ID, period.Key, householdID, sw.ID)]; !billed {
					cands = append(cands, candidate{dob: sw.DateOfBirth, charge: UpcomingCharge{
						ScheduleID: sc.ID, ScheduleName: sc.Name, Frequency: sc.Frequency, SwimmerID: sw.ID,
						SwimmerName: swimmerDisplayName(sw), PeriodStart: fmtDate(period.Start), PeriodEnd: fmtDate(period.End),
						GrossCents: sc.AmountCents,
					}})
					perSwimmer[sw.ID] += sc.AmountCents
				}
				period, ok = nextPeriodAfter(sc, period)
			}
		}
	}
	// Rank swimmers like a billing run: most expensive, then oldest, then ID.
	ids := make([]string, 0, len(perSwimmer))
	for id := range perSwimmer {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if perSwimmer[ids[i]] != perSwimmer[ids[j]] {
			return perSwimmer[ids[i]] > perSwimmer[ids[j]]
		}
		if swimmers[ids[i]].DateOfBirth != swimmers[ids[j]].DateOfBirth {
			return swimmers[ids[i]].DateOfBirth < swimmers[ids[j]].DateOfBirth
		}
		return ids[i] < ids[j]
	})
	position := map[string]int{}
	for i, id := range ids {
		position[id] = i + 1
	}
	for _, c := range cands {
		ch := c.charge
		_, ch.DiscountCents = pickDiscount(rules, ScheduleKindDues, position[ch.SwimmerID], ch.GrossCents)
		ch.NetCents = ch.GrossCents - ch.DiscountCents
		out.TotalCents += ch.NetCents
		out.Charges = append(out.Charges, ch)
	}
	sort.Slice(out.Charges, func(i, j int) bool {
		if out.Charges[i].PeriodStart != out.Charges[j].PeriodStart {
			return out.Charges[i].PeriodStart < out.Charges[j].PeriodStart
		}
		return out.Charges[i].SwimmerName < out.Charges[j].SwimmerName
	})
	return out, nil
}
