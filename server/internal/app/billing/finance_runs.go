package billing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ErrNothingToBill is returned when a run commit finds no unbilled charges.
var ErrNothingToBill = errors.New("nothing to bill for this period")

const defaultRunDueDays = 15

// RunInput describes a billing run for preview or commit.
type RunInput struct {
	AsOf               string   `json:"as_of" validate:"required"`
	ScheduleIDs        []string `json:"schedule_ids" validate:"max=200"`
	InvoiceStatus      string   `json:"invoice_status" validate:"omitempty,oneof=draft issued"`
	DueDate            string   `json:"due_date"`
	DueDays            *int     `json:"due_days" validate:"omitempty,gte=0,lte=120"`
	IncludeAdjustments *bool    `json:"include_adjustments"`
	// ExpectedFingerprint (commit only) must match the reviewed preview; a
	// mismatch means the roster or schedules changed since the preview.
	ExpectedFingerprint string `json:"expected_fingerprint"`
}

type normalizedRunInput struct {
	AsOf               time.Time
	ScheduleIDs        []string
	InvoiceStatus      string
	DueDate            string
	IncludeAdjustments bool
	Expected           string
}

// RunPreviewLine is one invoice line a run would create.
type RunPreviewLine struct {
	Kind            string `json:"kind"` // dues | registration | discount | adjustment
	Category        string `json:"category"`
	Description     string `json:"description"`
	ScheduleID      string `json:"schedule_id,omitempty"`
	ScheduleName    string `json:"schedule_name,omitempty"`
	SwimmerID       string `json:"swimmer_id,omitempty"`
	SwimmerName     string `json:"swimmer_name,omitempty"`
	PeriodKey       string `json:"period_key,omitempty"`
	PeriodStart     string `json:"period_start,omitempty"`
	PeriodEnd       string `json:"period_end,omitempty"`
	AmountCents     int    `json:"amount_cents"`
	SiblingPosition int    `json:"sibling_position,omitempty"`
	Note            string `json:"note,omitempty"`
	DiscountRuleID  string `json:"discount_rule_id,omitempty"`
	AdjustmentID    string `json:"adjustment_id,omitempty"`
}

// RunSkip explains why a candidate charge or household is not billed.
type RunSkip struct {
	HouseholdID   string `json:"household_id"`
	HouseholdName string `json:"household_name,omitempty"`
	SwimmerID     string `json:"swimmer_id,omitempty"`
	SwimmerName   string `json:"swimmer_name,omitempty"`
	ScheduleID    string `json:"schedule_id,omitempty"`
	ScheduleName  string `json:"schedule_name,omitempty"`
	PeriodKey     string `json:"period_key,omitempty"`
	Reason        string `json:"reason"`
}

// RunPreviewHousehold is one household's would-be invoice.
type RunPreviewHousehold struct {
	HouseholdID          string           `json:"household_id"`
	HouseholdName        string           `json:"household_name"`
	SwimmerCount         int              `json:"swimmer_count"`
	Lines                []RunPreviewLine `json:"lines"`
	GrossCents           int              `json:"gross_cents"`
	DiscountCents        int              `json:"discount_cents"`
	AdjustmentCents      int              `json:"adjustment_cents"`
	TotalCents           int              `json:"total_cents"`
	CreditAvailableCents int              `json:"credit_available_cents"`
	CreditToApplyCents   int              `json:"credit_to_apply_cents"`
	AmountDueCents       int              `json:"amount_due_cents"`

	charges     []*pendingCharge
	adjustments []*Adjustment
}

// RunPreview is the full dry-run result for a period.
type RunPreview struct {
	OrgID              string                 `json:"org_id"`
	AsOf               string                 `json:"as_of"`
	InvoiceStatus      string                 `json:"invoice_status"`
	DueDate            string                 `json:"due_date"`
	ScheduleIDs        []string               `json:"schedule_ids"`
	Households         []*RunPreviewHousehold `json:"households"`
	Skipped            []RunSkip              `json:"skipped"`
	HouseholdCount     int                    `json:"household_count"`
	SwimmerCount       int                    `json:"swimmer_count"`
	ChargeCount        int                    `json:"charge_count"`
	GrossCents         int                    `json:"gross_cents"`
	DiscountCents      int                    `json:"discount_cents"`
	AdjustmentCents    int                    `json:"adjustment_cents"`
	NetCents           int                    `json:"net_cents"`
	CreditToApplyCents int                    `json:"credit_to_apply_cents"`
	Fingerprint        string                 `json:"fingerprint"`
	GeneratedAt        time.Time              `json:"generated_at"`
}

// RunInvoiceResult summarises one invoice a committed run created.
type RunInvoiceResult struct {
	HouseholdID        string `json:"household_id"`
	HouseholdName      string `json:"household_name"`
	InvoiceID          string `json:"invoice_id"`
	InvoiceNumber      string `json:"invoice_number"`
	Status             string `json:"status"`
	TotalCents         int    `json:"total_cents"`
	CreditAppliedCents int    `json:"credit_applied_cents"`
	AmountDueCents     int    `json:"amount_due_cents"`
}

// RunResult is the outcome of a commit.
type RunResult struct {
	Run      *BillingRun        `json:"run"`
	Invoices []RunInvoiceResult `json:"invoices"`
	Skipped  []RunSkip          `json:"skipped"`
}

// RunDetail is a committed run plus the charges it billed.
type RunDetail struct {
	Run      *BillingRun        `json:"run"`
	Invoices []RunInvoiceResult `json:"invoices"`
	Charges  []*BillingCharge   `json:"charges"`
}

type pendingCharge struct {
	schedule    *FeeSchedule
	period      billingPeriod
	householdID string
	swimmerID   string
	swimmerName string
	swimmerDOB  string
	gross       int
	note        string
	position    int
	rule        *DiscountRule
	discount    int
}

func (s *Service) normalizeRunInput(orgID string, input RunInput) (normalizedRunInput, error) {
	out := normalizedRunInput{InvoiceStatus: "draft", IncludeAdjustments: true, Expected: strings.TrimSpace(input.ExpectedFingerprint)}
	asOf, err := parseDate(input.AsOf)
	if err != nil {
		return out, invalidField("as_of", "must use YYYY-MM-DD")
	}
	out.AsOf = asOf
	if input.InvoiceStatus != "" {
		out.InvoiceStatus = input.InvoiceStatus
	}
	if input.IncludeAdjustments != nil {
		out.IncludeAdjustments = *input.IncludeAdjustments
	}
	if strings.TrimSpace(input.DueDate) != "" {
		due, err := parseDate(input.DueDate)
		if err != nil {
			return out, invalidField("due_date", "must use YYYY-MM-DD")
		}
		if due.Before(asOf) {
			return out, invalidField("due_date", "must be on or after as_of")
		}
		out.DueDate = fmtDate(due)
	} else {
		days := defaultRunDueDays
		if input.DueDays != nil {
			days = *input.DueDays
		}
		out.DueDate = fmtDate(asOf.AddDate(0, 0, days))
	}
	seen := map[string]bool{}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range input.ScheduleIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		sc, ok := s.finance.schedules[id]
		if !ok || sc.OrgID != orgID {
			return out, invalidField("schedule_ids", "contains an unknown schedule")
		}
		seen[id] = true
		out.ScheduleIDs = append(out.ScheduleIDs, id)
	}
	sort.Strings(out.ScheduleIDs)
	return out, nil
}

func swimmerDisplayName(sw RosterSwimmer) string {
	return strings.TrimSpace(sw.FirstName + " " + sw.LastName)
}

// buildRunPreviewLocked computes what a run would bill. Pure with respect to
// state: it never mutates. Must be called with s.mu held.
func (s *Service) buildRunPreviewLocked(orgID string, roster RosterSnapshot, in normalizedRunInput) *RunPreview {
	preview := &RunPreview{
		OrgID: orgID, AsOf: fmtDate(in.AsOf), InvoiceStatus: in.InvoiceStatus, DueDate: in.DueDate,
		ScheduleIDs: in.ScheduleIDs, Households: []*RunPreviewHousehold{}, Skipped: []RunSkip{},
		GeneratedAt: time.Now().UTC(),
	}
	if preview.ScheduleIDs == nil {
		preview.ScheduleIDs = []string{}
	}
	households := map[string]RosterHousehold{}
	for _, h := range roster.Households {
		households[h.ID] = h
	}
	swimmers := map[string]RosterSwimmer{}
	for _, sw := range roster.Swimmers {
		swimmers[sw.ID] = sw
	}
	wanted := map[string]bool{}
	for _, id := range in.ScheduleIDs {
		wanted[id] = true
	}
	schedules := make([]*FeeSchedule, 0)
	for _, sc := range s.finance.schedules {
		if sc.OrgID != orgID || !sc.Active {
			continue
		}
		if len(wanted) > 0 && !wanted[sc.ID] {
			continue
		}
		schedules = append(schedules, sc)
	}
	sort.Slice(schedules, func(i, j int) bool { return schedules[i].ID < schedules[j].ID })

	skip := func(householdID, swimmerID string, sc *FeeSchedule, periodKey, reason string) {
		entry := RunSkip{HouseholdID: householdID, SwimmerID: swimmerID, PeriodKey: periodKey, Reason: reason}
		entry.HouseholdName = households[householdID].Name
		if sw, ok := swimmers[swimmerID]; ok {
			entry.SwimmerName = swimmerDisplayName(sw)
		}
		if sc != nil {
			entry.ScheduleID, entry.ScheduleName = sc.ID, sc.Name
		}
		preview.Skipped = append(preview.Skipped, entry)
	}

	billable := func(sw RosterSwimmer) (string, bool) {
		h, ok := households[sw.HouseholdID]
		if !ok {
			return "household not found", false
		}
		if h.Status != "" && h.Status != "active" {
			return "household is " + h.Status, false
		}
		if sw.Status != "" && sw.Status != "active" && sw.Status != "pending" {
			return "swimmer is " + sw.Status, false
		}
		return "", true
	}

	pending := map[string][]*pendingCharge{}        // household → charges
	considered := map[string]map[chargeScope]bool{} // kind → (schedule, period) pairs in scope
	for _, sc := range schedules {
		period, ok := periodFor(sc, in.AsOf)
		if !ok {
			continue
		}
		if considered[sc.Kind] == nil {
			considered[sc.Kind] = map[chargeScope]bool{}
		}
		considered[sc.Kind][chargeScope{sc.ID, period.Key}] = true
		schedStart, _ := parseDate(sc.StartDate)
		schedEnd := period.End
		if sc.EndDate != "" {
			if end, err := parseDate(sc.EndDate); err == nil && end.Before(schedEnd) {
				schedEnd = end
			}
		}
		seenSwimmers := map[string]bool{}
		switch sc.Kind {
		case ScheduleKindDues:
			links := make([]RosterGroupLink, 0)
			for _, link := range roster.GroupLinks {
				if link.GroupCode == sc.GroupCode {
					links = append(links, link)
				}
			}
			sort.Slice(links, func(i, j int) bool { return links[i].SwimmerID < links[j].SwimmerID })
			for _, link := range links {
				if seenSwimmers[link.SwimmerID] {
					continue
				}
				seenSwimmers[link.SwimmerID] = true
				sw, ok := swimmers[link.SwimmerID]
				if !ok {
					continue
				}
				if reason, ok := billable(sw); !ok {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, reason)
					continue
				}
				if sw.Status == "pending" {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, "swimmer registration is pending")
					continue
				}
				activeStart := period.Start
				if schedStart.After(activeStart) {
					activeStart = schedStart
				}
				if !link.AssignedAt.IsZero() {
					if join := dateOnly(link.AssignedAt); join.After(activeStart) {
						activeStart = join
					}
				}
				if activeStart.After(period.End) || activeStart.After(in.AsOf) {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, "joined after the run date")
					continue
				}
				if _, billed := s.finance.chargeKeys[chargeKey(orgID, sc.ID, period.Key, sw.HouseholdID, sw.ID)]; billed {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, "already billed for this period")
					continue
				}
				amount, note := prorate(sc.AmountCents, sc.Proration, period, activeStart, schedEnd)
				if amount <= 0 {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, note)
					continue
				}
				pending[sw.HouseholdID] = append(pending[sw.HouseholdID], &pendingCharge{
					schedule: sc, period: period, householdID: sw.HouseholdID, swimmerID: sw.ID,
					swimmerName: swimmerDisplayName(sw), swimmerDOB: sw.DateOfBirth, gross: amount, note: note,
				})
			}
		case ScheduleKindRegistration:
			regs := make([]RosterRegistration, 0)
			for _, reg := range roster.Registrations {
				if !strings.EqualFold(strings.TrimSpace(reg.Season), sc.Season) {
					continue
				}
				if reg.Status != "active" && reg.Status != "pending" {
					continue
				}
				if sc.GroupCode != "" && reg.GroupCode != sc.GroupCode {
					continue
				}
				regs = append(regs, reg)
			}
			sort.Slice(regs, func(i, j int) bool { return regs[i].SwimmerID < regs[j].SwimmerID })
			for _, reg := range regs {
				if seenSwimmers[reg.SwimmerID] {
					continue
				}
				seenSwimmers[reg.SwimmerID] = true
				sw, ok := swimmers[reg.SwimmerID]
				if !ok {
					continue
				}
				if reason, ok := billable(sw); !ok {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, reason)
					continue
				}
				if _, billed := s.finance.chargeKeys[chargeKey(orgID, sc.ID, period.Key, sw.HouseholdID, sw.ID)]; billed {
					skip(sw.HouseholdID, sw.ID, sc, period.Key, "already billed for this season")
					continue
				}
				pending[sw.HouseholdID] = append(pending[sw.HouseholdID], &pendingCharge{
					schedule: sc, period: period, householdID: sw.HouseholdID, swimmerID: sw.ID,
					swimmerName: swimmerDisplayName(sw), swimmerDOB: sw.DateOfBirth, gross: sc.AmountCents,
				})
			}
		}
	}

	// Pending manual adjustments.
	adjustmentsByHousehold := map[string][]*Adjustment{}
	if in.IncludeAdjustments {
		for _, a := range s.finance.adjustments {
			if a.OrgID != orgID || a.Status != AdjustmentStatusPending {
				continue
			}
			if _, ok := households[a.HouseholdID]; !ok {
				continue
			}
			adjustmentsByHousehold[a.HouseholdID] = append(adjustmentsByHousehold[a.HouseholdID], a)
		}
	}

	rules := make([]*DiscountRule, 0)
	for _, rule := range s.finance.rules {
		if rule.OrgID == orgID {
			rules = append(rules, rule)
		}
	}

	householdIDs := map[string]bool{}
	for id := range pending {
		householdIDs[id] = true
	}
	for id := range adjustmentsByHousehold {
		householdIDs[id] = true
	}

	for householdID := range householdIDs {
		charges := pending[householdID]
		s.assignSiblingPositions(orgID, householdID, charges, considered, rules)

		hh := buildHouseholdPreview(householdID, households[householdID].Name, charges, adjustmentsByHousehold[householdID], swimmers)
		adjustments := hh.adjustments
		if hh.TotalCents <= 0 {
			reason := "net total is zero or negative; nothing invoiced"
			if len(adjustments) > 0 {
				reason += " (adjustments stay pending)"
			}
			skip(householdID, "", nil, "", reason)
			continue
		}
		hh.CreditAvailableCents = s.finance.balances[balanceKey(orgID, householdID)]
		if in.InvoiceStatus == "issued" {
			hh.CreditToApplyCents = min(hh.CreditAvailableCents, hh.TotalCents)
		}
		hh.AmountDueCents = hh.TotalCents - hh.CreditToApplyCents
		preview.Households = append(preview.Households, hh)
	}

	finalizePreview(preview)
	return preview
}

// chargeScope identifies a (schedule, billing period) pair a run is billing.
type chargeScope struct{ schedule, period string }

// assignSiblingPositions sets each pending charge's sibling position and the
// discount that position earns. Swimmers already billed in the same
// (schedule, period) scope keep their recorded position; new swimmers follow,
// most expensive first, then oldest, then ID.
func (s *Service) assignSiblingPositions(orgID, householdID string, charges []*pendingCharge, considered map[string]map[chargeScope]bool, rules []*DiscountRule) {
	for _, kind := range []string{ScheduleKindDues, ScheduleKindRegistration} {
		positions := map[string]int{}
		maxPos := 0
		for _, c := range s.finance.charges {
			if c.ReleasedAt != nil || c.OrgID != orgID || c.HouseholdID != householdID {
				continue
			}
			if !considered[kind][chargeScope{c.ScheduleID, c.PeriodKey}] {
				continue
			}
			if existing, ok := positions[c.SwimmerID]; !ok || c.SiblingPosition < existing {
				positions[c.SwimmerID] = c.SiblingPosition
			}
			if c.SiblingPosition > maxPos {
				maxPos = c.SiblingPosition
			}
		}
		type newSwimmer struct {
			id, dob string
			gross   int
		}
		fresh := map[string]*newSwimmer{}
		for _, pc := range charges {
			if pc.schedule.Kind != kind {
				continue
			}
			if _, ok := positions[pc.swimmerID]; ok {
				continue
			}
			ns := fresh[pc.swimmerID]
			if ns == nil {
				ns = &newSwimmer{id: pc.swimmerID, dob: pc.swimmerDOB}
				fresh[pc.swimmerID] = ns
			}
			ns.gross += pc.gross
		}
		ordered := make([]*newSwimmer, 0, len(fresh))
		for _, ns := range fresh {
			ordered = append(ordered, ns)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].gross != ordered[j].gross {
				return ordered[i].gross > ordered[j].gross
			}
			if ordered[i].dob != ordered[j].dob {
				if ordered[i].dob == "" {
					return false
				}
				if ordered[j].dob == "" {
					return true
				}
				return ordered[i].dob < ordered[j].dob
			}
			return ordered[i].id < ordered[j].id
		})
		for _, ns := range ordered {
			maxPos++
			positions[ns.id] = maxPos
		}
		for _, pc := range charges {
			if pc.schedule.Kind != kind {
				continue
			}
			pc.position = positions[pc.swimmerID]
			pc.rule, pc.discount = pickDiscount(rules, kind, pc.position, pc.gross)
		}
	}
}

// buildHouseholdPreview turns a household's pending charges and adjustments
// into ordered invoice lines with gross, discount, adjustment and net totals.
func buildHouseholdPreview(householdID, householdName string, charges []*pendingCharge, adjustments []*Adjustment, swimmers map[string]RosterSwimmer) *RunPreviewHousehold {
	sort.SliceStable(charges, func(i, j int) bool {
		if charges[i].swimmerName != charges[j].swimmerName {
			return charges[i].swimmerName < charges[j].swimmerName
		}
		if charges[i].schedule.Kind != charges[j].schedule.Kind {
			return charges[i].schedule.Kind < charges[j].schedule.Kind
		}
		return charges[i].schedule.Name < charges[j].schedule.Name
	})

	hh := &RunPreviewHousehold{
		HouseholdID: householdID, HouseholdName: householdName,
		Lines: []RunPreviewLine{}, charges: charges,
	}
	swimmerSet := map[string]bool{}
	for _, pc := range charges {
		swimmerSet[pc.swimmerID] = true
		category := itemCategoryProgramFee
		if pc.schedule.Kind == ScheduleKindRegistration {
			category = itemCategoryRegistrationFee
		}
		desc := fmt.Sprintf("%s — %s (%s – %s)", pc.schedule.Name, pc.swimmerName, fmtDate(pc.period.Start), fmtDate(pc.period.End))
		if pc.schedule.Kind == ScheduleKindRegistration {
			desc = fmt.Sprintf("%s — %s (%s)", pc.schedule.Name, pc.swimmerName, pc.schedule.Season)
		}
		if pc.note != "" {
			desc += " · " + pc.note
		}
		hh.Lines = append(hh.Lines, RunPreviewLine{
			Kind: pc.schedule.Kind, Category: category, Description: desc,
			ScheduleID: pc.schedule.ID, ScheduleName: pc.schedule.Name,
			SwimmerID: pc.swimmerID, SwimmerName: pc.swimmerName,
			PeriodKey: pc.period.Key, PeriodStart: fmtDate(pc.period.Start), PeriodEnd: fmtDate(pc.period.End),
			AmountCents: pc.gross, SiblingPosition: pc.position, Note: pc.note,
		})
		hh.GrossCents += pc.gross
		if pc.discount > 0 {
			hh.Lines = append(hh.Lines, RunPreviewLine{
				Kind: "discount", Category: itemCategoryDiscount,
				Description: fmt.Sprintf("%s — %s, %s swimmer (−%s)", pc.rule.Name, pc.swimmerName, ordinal(pc.position), describeDiscount(pc.rule)),
				ScheduleID:  pc.schedule.ID, ScheduleName: pc.schedule.Name,
				SwimmerID: pc.swimmerID, SwimmerName: pc.swimmerName, PeriodKey: pc.period.Key,
				AmountCents: -pc.discount, SiblingPosition: pc.position, DiscountRuleID: pc.rule.ID,
			})
			hh.DiscountCents += pc.discount
		}
	}
	sort.Slice(adjustments, func(i, j int) bool { return adjustments[i].CreatedAt.Before(adjustments[j].CreatedAt) })
	for _, a := range adjustments {
		name := ""
		if sw, ok := swimmers[a.SwimmerID]; ok {
			name = swimmerDisplayName(sw)
		}
		desc := "Adjustment: " + a.Reason
		if name != "" {
			desc += " — " + name
		}
		hh.Lines = append(hh.Lines, RunPreviewLine{
			Kind: "adjustment", Category: itemCategoryAdjustment, Description: desc,
			SwimmerID: a.SwimmerID, SwimmerName: name, AmountCents: a.AmountCents, AdjustmentID: a.ID,
		})
		hh.AdjustmentCents += a.AmountCents
	}
	hh.adjustments = adjustments
	hh.SwimmerCount = len(swimmerSet)
	hh.TotalCents = hh.GrossCents - hh.DiscountCents + hh.AdjustmentCents
	return hh
}

// finalizePreview orders the preview's households and skips, totals them, and
// fingerprints the result so a commit can detect that the preview went stale.
func finalizePreview(preview *RunPreview) {
	sort.Slice(preview.Households, func(i, j int) bool {
		if preview.Households[i].HouseholdName != preview.Households[j].HouseholdName {
			return preview.Households[i].HouseholdName < preview.Households[j].HouseholdName
		}
		return preview.Households[i].HouseholdID < preview.Households[j].HouseholdID
	})
	sort.SliceStable(preview.Skipped, func(i, j int) bool {
		if preview.Skipped[i].HouseholdName != preview.Skipped[j].HouseholdName {
			return preview.Skipped[i].HouseholdName < preview.Skipped[j].HouseholdName
		}
		return preview.Skipped[i].SwimmerName < preview.Skipped[j].SwimmerName
	})

	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%s|%s|%s\n", preview.AsOf, preview.InvoiceStatus, preview.DueDate, strings.Join(preview.ScheduleIDs, ","))
	for _, hh := range preview.Households {
		preview.HouseholdCount++
		preview.SwimmerCount += hh.SwimmerCount
		preview.ChargeCount += len(hh.charges)
		preview.GrossCents += hh.GrossCents
		preview.DiscountCents += hh.DiscountCents
		preview.AdjustmentCents += hh.AdjustmentCents
		preview.NetCents += hh.TotalCents
		preview.CreditToApplyCents += hh.CreditToApplyCents
		for _, line := range hh.Lines {
			fmt.Fprintf(hash, "%s|%s|%s|%s|%s|%d|%s\n", hh.HouseholdID, line.Kind, line.ScheduleID, line.SwimmerID, line.PeriodKey, line.AmountCents, line.AdjustmentID)
		}
	}
	preview.Fingerprint = hex.EncodeToString(hash.Sum(nil))[:24]
}

// PreviewBillingRun computes a dry run (admin only). Nothing is written.
func (s *Service) PreviewBillingRun(orgID string, principal *appmiddleware.Principal, input RunInput) (*RunPreview, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	ctx := context.Background()
	s.mu.Lock()
	err := s.refreshLocked(ctx)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	in, err := s.normalizeRunInput(orgID, input)
	if err != nil {
		return nil, err
	}
	roster, err := s.loadRoster(orgID)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buildRunPreviewLocked(orgID, roster, in), nil
}

// StalePreviewError carries the fresh preview when a commit's expected
// fingerprint no longer matches.
type StalePreviewError struct {
	Preview *RunPreview
}

func (e *StalePreviewError) Error() string { return ErrRunPreviewStale.Error() }

func (e *StalePreviewError) Unwrap() error { return ErrRunPreviewStale }

// CommitBillingRun bills the period: one invoice per household. Re-running
// the same period is a no-op for already-billed (schedule, period, household,
// swimmer) keys, including under concurrent commits across instances.
func (s *Service) CommitBillingRun(orgID, actorID string, principal *appmiddleware.Principal, input RunInput, trigger string) (*RunResult, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	if trigger != RunTriggerScheduler {
		trigger = RunTriggerManual
	}
	in, err := s.normalizeRunInput(orgID, input)
	if err != nil {
		return nil, err
	}
	roster, err := s.loadRoster(orgID)
	if err != nil {
		return nil, err
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()
	ctx := context.Background()
	var result *RunResult
	commit := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.refreshLocked(ctx); err != nil {
			return err
		}
		preview := s.buildRunPreviewLocked(orgID, roster, in)
		if in.Expected != "" && in.Expected != preview.Fingerprint {
			return &StalePreviewError{Preview: preview}
		}
		if len(preview.Households) == 0 {
			return ErrNothingToBill
		}
		var cerr error
		result, cerr = s.commitPreviewLocked(ctx, orgID, actorID, trigger, in, preview)
		return cerr
	}
	if s.repo != nil {
		err = s.repo.withOrgLock(ctx, "run", orgID, commit)
	} else {
		err = commit()
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) newInvoiceNumberLocked(orgID string, asOf time.Time) string {
	for {
		b := make([]byte, 3)
		_, _ = rand.Read(b)
		number := fmt.Sprintf("BR-%s-%s", asOf.Format("200601"), strings.ToUpper(hex.EncodeToString(b)))
		if !s.invoiceNums[invoiceNumKey(orgID, number)] {
			return number
		}
	}
}

func (s *Service) commitPreviewLocked(ctx context.Context, orgID, actorID, trigger string, in normalizedRunInput, preview *RunPreview) (*RunResult, error) {
	now := time.Now().UTC()
	run := &BillingRun{
		ID: s.newID(), OrgID: orgID, AsOf: preview.AsOf, InvoiceStatus: in.InvoiceStatus, DueDate: in.DueDate,
		ScheduleIDs: preview.ScheduleIDs, Trigger: trigger, Fingerprint: preview.Fingerprint,
		CreatedAt: now, CreatedBy: actorID,
	}
	if s.repo != nil {
		if err := upsertRun(ctx, s.repo.pool, run); err != nil {
			return nil, fmt.Errorf("persist billing run: %w", err)
		}
	}
	result := &RunResult{Run: run, Invoices: []RunInvoiceResult{}, Skipped: append([]RunSkip{}, preview.Skipped...)}

	scheduleNames := map[string]bool{}
	for _, hh := range preview.Households {
		for _, pc := range hh.charges {
			scheduleNames[pc.schedule.Name] = true
		}
	}
	names := make([]string, 0, len(scheduleNames))
	for name := range scheduleNames {
		names = append(names, name)
	}
	sort.Strings(names)
	notes := fmt.Sprintf("Billing run %s", preview.AsOf)
	if len(names) > 0 {
		notes += ": " + strings.Join(names, ", ")
	}

	for _, hh := range preview.Households {
		inv, err := s.commitHouseholdLocked(ctx, orgID, actorID, run, in, hh, notes, now)
		if errors.Is(err, errChargeConflict) {
			result.Skipped = append(result.Skipped, RunSkip{HouseholdID: hh.HouseholdID, HouseholdName: hh.HouseholdName, Reason: "billed concurrently by another run; not double-billed"})
			continue
		}
		if err != nil {
			slog.Error("billing run: household commit failed", "run_id", run.ID, "household_id", hh.HouseholdID, "err", err)
			result.Skipped = append(result.Skipped, RunSkip{HouseholdID: hh.HouseholdID, HouseholdName: hh.HouseholdName, Reason: "commit failed: " + err.Error()})
			continue
		}
		applied := 0
		if inv.Status == "issued" {
			if applied, err = s.applyHouseholdCreditLocked(ctx, inv, actorID); err != nil {
				slog.Error("billing run: credit auto-apply failed", "invoice_id", inv.ID, "err", err)
			}
		}
		run.HouseholdCount++
		run.InvoiceCount++
		run.ChargeCount += len(hh.charges)
		run.GrossCents += hh.GrossCents
		run.DiscountCents += hh.DiscountCents
		run.AdjustmentCents += hh.AdjustmentCents
		run.CreditAppliedCents += applied
		run.NetCents += hh.TotalCents
		result.Invoices = append(result.Invoices, RunInvoiceResult{
			HouseholdID: hh.HouseholdID, HouseholdName: hh.HouseholdName, InvoiceID: inv.ID,
			InvoiceNumber: inv.InvoiceNumber, Status: inv.Status, TotalCents: inv.TotalCents,
			CreditAppliedCents: applied, AmountDueCents: inv.AmountDueCents,
		})
	}
	run.SkippedCount = len(result.Skipped)
	if s.repo != nil {
		if err := upsertRun(ctx, s.repo.pool, run); err != nil {
			slog.Error("billing run: persist totals failed", "run_id", run.ID, "err", err)
		}
	}
	s.finance.runs[run.ID] = run
	slog.Info("billing run committed", "org_id", orgID, "run_id", run.ID, "trigger", trigger,
		"as_of", run.AsOf, "invoices", run.InvoiceCount, "charges", run.ChargeCount,
		"net_cents", run.NetCents, "skipped", run.SkippedCount)
	cp := *run
	result.Run = &cp
	return result, nil
}

var errChargeConflict = errors.New("billing charge already exists")

func (s *Service) commitHouseholdLocked(ctx context.Context, orgID, actorID string, run *BillingRun, in normalizedRunInput, hh *RunPreviewHousehold, notes string, now time.Time) (*Invoice, error) {
	asOf, _ := parseDate(run.AsOf)
	inv := &Invoice{
		ID: s.newID(), OrgID: orgID, HouseholdID: hh.HouseholdID,
		InvoiceNumber: s.newInvoiceNumberLocked(orgID, asOf), Status: "draft", DueDate: in.DueDate,
		Notes: notes, ExternalRef: "billing_run:" + run.ID,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	items := make([]*InvoiceItem, 0, len(hh.Lines))
	for _, line := range hh.Lines {
		items = append(items, &InvoiceItem{
			ID: s.newID(), OrgID: orgID, InvoiceID: inv.ID, Description: line.Description, Quantity: 1,
			UnitAmountCents: line.AmountCents, TotalCents: line.AmountCents, Category: line.Category,
			SwimmerID: line.SwimmerID, CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
		})
	}
	subtotal := 0
	for _, item := range items {
		subtotal += item.TotalCents
	}
	inv.SubtotalCents = subtotal
	inv.TotalCents = subtotal
	inv.AmountDueCents = subtotal
	var issuedEvent *LedgerEvent
	if in.InvoiceStatus == "issued" {
		issued := now
		inv.Status = "issued"
		inv.IssuedAt = &issued
		issuedEvent = &LedgerEvent{
			ID: s.newID(), OrgID: orgID, InvoiceID: inv.ID, HouseholdID: hh.HouseholdID,
			EventType: "invoice_issued", AmountCents: inv.TotalCents,
			Description: fmt.Sprintf("Invoice %s issued (billing run %s)", inv.InvoiceNumber, run.AsOf),
			ActorID:     actorID, CreatedAt: now,
		}
	}
	charges := make([]*BillingCharge, 0, len(hh.charges))
	for _, pc := range hh.charges {
		charges = append(charges, &BillingCharge{
			ID: s.newID(), OrgID: orgID, RunID: run.ID, InvoiceID: inv.ID, HouseholdID: hh.HouseholdID,
			SwimmerID: pc.swimmerID, ScheduleID: pc.schedule.ID, PeriodKey: pc.period.Key,
			PeriodStart: fmtDate(pc.period.Start), PeriodEnd: fmtDate(pc.period.End),
			GrossCents: pc.gross, DiscountCents: pc.discount, NetCents: pc.gross - pc.discount,
			SiblingPosition: max(pc.position, 1), CreatedAt: now,
		})
	}
	claimed := make([]*Adjustment, 0, len(hh.adjustments))
	for _, a := range hh.adjustments {
		cp := *a
		cp.Status = AdjustmentStatusApplied
		cp.AppliedInvoiceID = inv.ID
		cp.AppliedRunID = run.ID
		cp.UpdatedAt = now
		cp.UpdatedBy = actorID
		claimed = append(claimed, &cp)
	}

	// In-process idempotency guard (the DB unique index is the cross-instance one).
	for _, c := range charges {
		if _, exists := s.finance.chargeKeys[chargeKey(orgID, c.ScheduleID, c.PeriodKey, c.HouseholdID, c.SwimmerID)]; exists {
			return nil, errChargeConflict
		}
	}
	for _, a := range hh.adjustments {
		if current, ok := s.finance.adjustments[a.ID]; !ok || current.Status != AdjustmentStatusPending {
			return nil, errChargeConflict
		}
	}

	if s.repo != nil {
		err := s.repo.inTx(ctx, func(tx pgx.Tx) error {
			if err := upsertInvoice(ctx, tx, inv); err != nil {
				return err
			}
			for _, item := range items {
				if err := upsertInvoiceItem(ctx, tx, item); err != nil {
					return err
				}
			}
			for _, c := range charges {
				inserted, err := insertCharge(ctx, tx, c)
				if err != nil {
					return err
				}
				if !inserted {
					return errChargeConflict
				}
			}
			for _, a := range claimed {
				ok, err := claimAdjustment(ctx, tx, a)
				if err != nil {
					return err
				}
				if !ok {
					return errChargeConflict
				}
			}
			if issuedEvent != nil {
				return insertLedgerEvent(ctx, tx, issuedEvent)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	s.invoices[inv.ID] = inv
	s.invoiceNums[invoiceNumKey(orgID, inv.InvoiceNumber)] = true
	for _, item := range items {
		s.items[inv.ID] = append(s.items[inv.ID], item)
		s.itemsByID[item.ID] = item
	}
	for _, c := range charges {
		s.finance.indexCharge(c)
	}
	for _, a := range claimed {
		s.finance.adjustments[a.ID] = a
	}
	if issuedEvent != nil {
		s.ledger = append(s.ledger, issuedEvent)
	}
	return inv, nil
}

// ListBillingRuns returns committed runs, newest first (admin only).
func (s *Service) ListBillingRuns(orgID string, principal *appmiddleware.Principal) ([]*BillingRun, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	out := make([]*BillingRun, 0)
	for _, run := range s.finance.runs {
		if run.OrgID == orgID {
			cp := *run
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// GetBillingRun returns one run with its invoices and charges (admin only).
func (s *Service) GetBillingRun(orgID string, principal *appmiddleware.Principal, runID string) (*RunDetail, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	run, ok := s.finance.runs[runID]
	if !ok || run.OrgID != orgID {
		return nil, ErrRunNotFound
	}
	detail := &RunDetail{Invoices: []RunInvoiceResult{}, Charges: []*BillingCharge{}}
	cp := *run
	detail.Run = &cp
	invoiceIDs := map[string]bool{}
	for _, c := range s.finance.charges {
		if c.RunID == runID {
			cc := *c
			detail.Charges = append(detail.Charges, &cc)
			invoiceIDs[c.InvoiceID] = true
		}
	}
	for _, inv := range s.invoices {
		if inv.OrgID == orgID && (invoiceIDs[inv.ID] || inv.ExternalRef == "billing_run:"+runID) {
			credit := 0
			for _, p := range s.payments[inv.ID] {
				if p.Method == PaymentMethodAccountCredit && p.Status == "succeeded" {
					credit += p.AmountCents
				}
			}
			detail.Invoices = append(detail.Invoices, RunInvoiceResult{
				HouseholdID: inv.HouseholdID, InvoiceID: inv.ID, InvoiceNumber: inv.InvoiceNumber,
				Status: inv.Status, TotalCents: inv.TotalCents, CreditAppliedCents: credit, AmountDueCents: inv.AmountDueCents,
			})
		}
	}
	sort.Slice(detail.Invoices, func(i, j int) bool { return detail.Invoices[i].InvoiceNumber < detail.Invoices[j].InvoiceNumber })
	return detail, nil
}

// RunScheduledBilling is the worker entry point: when automation is enabled
// and today is on/after the configured run day, it commits a run for all
// auto-bill schedules. Re-running daily is safe (idempotent charge keys) and
// picks up mid-period joiners with proration.
func (s *Service) RunScheduledBilling(orgID string, today time.Time) (*RunResult, error) {
	principal := systemFinancePrincipal(orgID)
	s.mu.Lock()
	if err := s.refreshLocked(context.Background()); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	settings := *s.settingsLocked(orgID)
	scheduleIDs := make([]string, 0)
	for _, sc := range s.finance.schedules {
		if sc.OrgID == orgID && sc.Active && sc.AutoBill {
			scheduleIDs = append(scheduleIDs, sc.ID)
		}
	}
	s.mu.Unlock()
	if !settings.AutoRunEnabled || today.UTC().Day() < settings.AutoRunDay || len(scheduleIDs) == 0 {
		return nil, nil
	}
	dueDays := settings.AutoRunDueDays
	include := true
	result, err := s.CommitBillingRun(orgID, principal.UserID, principal, RunInput{
		AsOf: fmtDate(today), ScheduleIDs: scheduleIDs, InvoiceStatus: settings.AutoRunInvoiceStatus,
		DueDays: &dueDays, IncludeAdjustments: &include,
	}, RunTriggerScheduler)
	if errors.Is(err, ErrNothingToBill) {
		return nil, nil
	}
	return result, err
}

func systemFinancePrincipal(orgID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "system:billing-automation", OrgID: orgID, Roles: []string{"admin"}}
}
