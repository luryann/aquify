package billing

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ErrScheduleLocked is returned when a billed schedule's period-defining
// fields would change (that could re-key periods and double-bill).
var ErrScheduleLocked = errors.New("fee schedule has billed charges; period-defining fields cannot change")

// ─── Fee schedules ───────────────────────────────────────────────────────────

func (s *Service) normalizeScheduleInput(orgID string, input FeeScheduleInput) (FeeScheduleInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Kind = strings.TrimSpace(input.Kind)
	input.GroupCode = strings.ToUpper(strings.TrimSpace(input.GroupCode))
	input.Season = strings.TrimSpace(input.Season)
	input.Frequency = strings.TrimSpace(input.Frequency)
	input.Description = strings.TrimSpace(input.Description)
	input.StartDate = strings.TrimSpace(input.StartDate)
	input.EndDate = strings.TrimSpace(input.EndDate)
	if input.Proration == "" {
		input.Proration = ProrationNone
	}
	if input.Name == "" {
		return input, invalidField("name", "is required")
	}
	start, err := parseDate(input.StartDate)
	if err != nil {
		return input, invalidField("start_date", "must use YYYY-MM-DD")
	}
	if input.EndDate != "" {
		end, err := parseDate(input.EndDate)
		if err != nil {
			return input, invalidField("end_date", "must use YYYY-MM-DD")
		}
		if end.Before(start) {
			return input, invalidField("end_date", "must be on or after start_date")
		}
	}
	switch input.Kind {
	case ScheduleKindDues:
		if input.GroupCode == "" {
			return input, invalidField("group_code", "is required for dues schedules")
		}
		input.Season = ""
	case ScheduleKindRegistration:
		if input.Season == "" {
			return input, invalidField("season", "is required for registration fee schedules")
		}
		if input.Frequency != FrequencyOneTime {
			return input, invalidField("frequency", "registration fees are one_time per season")
		}
		input.Proration = ProrationNone
	}
	if input.Frequency == FrequencyOneTime && input.Proration != ProrationNone {
		return input, invalidField("proration", "one_time schedules are not prorated")
	}
	if input.GroupCode != "" {
		if roster, err := s.loadRoster(orgID); err == nil && len(roster.Groups) > 0 {
			found := false
			for _, g := range roster.Groups {
				if g.Code == input.GroupCode {
					found = true
					break
				}
			}
			if !found {
				return input, invalidField("group_code", "does not match a roster group")
			}
		}
	}
	return input, nil
}

// ListFeeSchedules returns the org's fee schedules (admin only).
func (s *Service) ListFeeSchedules(orgID string, principal *appmiddleware.Principal) ([]*FeeSchedule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	out := make([]*FeeSchedule, 0)
	for _, sc := range s.finance.schedules {
		if sc.OrgID == orgID {
			cp := *sc
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// GetFeeSchedule returns one fee schedule (admin only).
func (s *Service) GetFeeSchedule(orgID string, principal *appmiddleware.Principal, scheduleID string) (*FeeSchedule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	sc, ok := s.finance.schedules[scheduleID]
	if !ok || sc.OrgID != orgID {
		return nil, ErrFeeScheduleNotFound
	}
	cp := *sc
	return &cp, nil
}

// CreateFeeSchedule creates a fee schedule (admin only).
func (s *Service) CreateFeeSchedule(orgID, actorID string, principal *appmiddleware.Principal, input FeeScheduleInput) (*FeeSchedule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	input, err := s.normalizeScheduleInput(orgID, input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sc := &FeeSchedule{
		ID: s.newID(), OrgID: orgID, Name: input.Name, Kind: input.Kind, GroupCode: input.GroupCode,
		Season: input.Season, Frequency: input.Frequency, AmountCents: input.AmountCents,
		StartDate: input.StartDate, EndDate: input.EndDate, Proration: input.Proration, AutoBill: input.AutoBill,
		Active: input.Active == nil || *input.Active, Description: input.Description,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repo != nil {
		if err := s.repo.upsertFeeSchedule(context.Background(), sc); err != nil {
			return nil, err
		}
	}
	s.finance.schedules[sc.ID] = sc
	cp := *sc
	return &cp, nil
}

// UpdateFeeSchedule replaces a fee schedule (admin only). Once a schedule has
// billed charges, the fields that define its periods are frozen.
func (s *Service) UpdateFeeSchedule(orgID, actorID string, principal *appmiddleware.Principal, scheduleID string, input FeeScheduleInput) (*FeeSchedule, *FeeSchedule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, err
	}
	input, err := s.normalizeScheduleInput(orgID, input)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, nil, err
	}
	sc, ok := s.finance.schedules[scheduleID]
	if !ok || sc.OrgID != orgID {
		return nil, nil, ErrFeeScheduleNotFound
	}
	before := *sc
	if s.scheduleHasChargesLocked(scheduleID) {
		if input.Kind != sc.Kind || input.Frequency != sc.Frequency || input.StartDate != sc.StartDate ||
			input.GroupCode != sc.GroupCode || input.Season != sc.Season {
			return nil, nil, ErrScheduleLocked
		}
	}
	updated := *sc
	updated.Name, updated.Kind, updated.GroupCode, updated.Season = input.Name, input.Kind, input.GroupCode, input.Season
	updated.Frequency, updated.AmountCents = input.Frequency, input.AmountCents
	updated.StartDate, updated.EndDate, updated.Proration = input.StartDate, input.EndDate, input.Proration
	updated.AutoBill, updated.Description = input.AutoBill, input.Description
	if input.Active != nil {
		updated.Active = *input.Active
	}
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.upsertFeeSchedule(context.Background(), &updated); err != nil {
			return nil, nil, err
		}
	}
	*sc = updated
	cp := updated
	return &before, &cp, nil
}

// ArchiveFeeSchedule deactivates a fee schedule; billed history is retained.
func (s *Service) ArchiveFeeSchedule(orgID, actorID string, principal *appmiddleware.Principal, scheduleID string) (*FeeSchedule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	sc, ok := s.finance.schedules[scheduleID]
	if !ok || sc.OrgID != orgID {
		return nil, ErrFeeScheduleNotFound
	}
	updated := *sc
	updated.Active = false
	updated.AutoBill = false
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.upsertFeeSchedule(context.Background(), &updated); err != nil {
			return nil, err
		}
	}
	*sc = updated
	cp := updated
	return &cp, nil
}

func (s *Service) scheduleHasChargesLocked(scheduleID string) bool {
	for _, c := range s.finance.charges {
		if c.ScheduleID == scheduleID {
			return true
		}
	}
	return false
}

// ─── Discount rules ──────────────────────────────────────────────────────────

func normalizeDiscountInput(input DiscountRuleInput) (DiscountRuleInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return input, invalidField("name", "is required")
	}
	if input.MaxPosition != nil && *input.MaxPosition < input.MinPosition {
		return input, invalidField("max_position", "must be greater than or equal to min_position")
	}
	switch input.DiscountType {
	case DiscountTypePercent:
		if input.PercentBps <= 0 {
			return input, invalidField("percent_bps", "must be between 1 and 10000")
		}
		input.AmountCents = 0
	case DiscountTypeFixed:
		if input.AmountCents <= 0 {
			return input, invalidField("amount_cents", "must be positive")
		}
		input.PercentBps = 0
	}
	return input, nil
}

// ListDiscountRules returns the org's discount rules (admin only).
func (s *Service) ListDiscountRules(orgID string, principal *appmiddleware.Principal) ([]*DiscountRule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	out := make([]*DiscountRule, 0)
	for _, rule := range s.finance.rules {
		if rule.OrgID == orgID {
			cp := *rule
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MinPosition != out[j].MinPosition {
			return out[i].MinPosition < out[j].MinPosition
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// CreateDiscountRule creates a sibling discount tier (admin only).
func (s *Service) CreateDiscountRule(orgID, actorID string, principal *appmiddleware.Principal, input DiscountRuleInput) (*DiscountRule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	input, err := normalizeDiscountInput(input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rule := &DiscountRule{
		ID: s.newID(), OrgID: orgID, Name: input.Name, AppliesTo: input.AppliesTo, MinPosition: input.MinPosition,
		MaxPosition: input.MaxPosition, DiscountType: input.DiscountType, PercentBps: input.PercentBps,
		AmountCents: input.AmountCents, Active: input.Active == nil || *input.Active,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repo != nil {
		if err := s.repo.upsertDiscountRule(context.Background(), rule); err != nil {
			return nil, err
		}
	}
	s.finance.rules[rule.ID] = rule
	cp := *rule
	return &cp, nil
}

// UpdateDiscountRule replaces a discount rule (admin only).
func (s *Service) UpdateDiscountRule(orgID, actorID string, principal *appmiddleware.Principal, ruleID string, input DiscountRuleInput) (*DiscountRule, *DiscountRule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, err
	}
	input, err := normalizeDiscountInput(input)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, nil, err
	}
	rule, ok := s.finance.rules[ruleID]
	if !ok || rule.OrgID != orgID {
		return nil, nil, ErrDiscountRuleNotFound
	}
	before := *rule
	updated := *rule
	updated.Name, updated.AppliesTo, updated.MinPosition, updated.MaxPosition = input.Name, input.AppliesTo, input.MinPosition, input.MaxPosition
	updated.DiscountType, updated.PercentBps, updated.AmountCents = input.DiscountType, input.PercentBps, input.AmountCents
	if input.Active != nil {
		updated.Active = *input.Active
	}
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.upsertDiscountRule(context.Background(), &updated); err != nil {
			return nil, nil, err
		}
	}
	*rule = updated
	cp := updated
	return &before, &cp, nil
}

// ArchiveDiscountRule deactivates a discount rule (admin only).
func (s *Service) ArchiveDiscountRule(orgID, actorID string, principal *appmiddleware.Principal, ruleID string) (*DiscountRule, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	rule, ok := s.finance.rules[ruleID]
	if !ok || rule.OrgID != orgID {
		return nil, ErrDiscountRuleNotFound
	}
	updated := *rule
	updated.Active = false
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = actorID
	if s.repo != nil {
		if err := s.repo.upsertDiscountRule(context.Background(), &updated); err != nil {
			return nil, err
		}
	}
	*rule = updated
	cp := updated
	return &cp, nil
}

// ─── Manual adjustments ──────────────────────────────────────────────────────

// ListAdjustments returns adjustments filtered by optional status/household.
func (s *Service) ListAdjustments(orgID string, principal *appmiddleware.Principal, status, householdID string) ([]*Adjustment, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	out := make([]*Adjustment, 0)
	for _, a := range s.finance.adjustments {
		if a.OrgID != orgID || (status != "" && a.Status != status) || (householdID != "" && a.HouseholdID != householdID) {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// CreateAdjustment records a manual one-off charge/discount with a reason.
func (s *Service) CreateAdjustment(orgID, actorID string, principal *appmiddleware.Principal, input AdjustmentInput) (*Adjustment, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	input.HouseholdID = strings.TrimSpace(input.HouseholdID)
	input.SwimmerID = strings.TrimSpace(input.SwimmerID)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return nil, invalidField("reason", "is required")
	}
	if roster, err := s.loadRoster(orgID); err == nil {
		householdFound := false
		for _, h := range roster.Households {
			if h.ID == input.HouseholdID {
				householdFound = true
				break
			}
		}
		if !householdFound {
			return nil, invalidField("household_id", "does not match a household")
		}
		if input.SwimmerID != "" {
			swimmerOK := false
			for _, sw := range roster.Swimmers {
				if sw.ID == input.SwimmerID && sw.HouseholdID == input.HouseholdID {
					swimmerOK = true
					break
				}
			}
			if !swimmerOK {
				return nil, invalidField("swimmer_id", "is not in this household")
			}
		}
	}
	now := time.Now().UTC()
	a := &Adjustment{
		ID: s.newID(), OrgID: orgID, HouseholdID: input.HouseholdID, SwimmerID: input.SwimmerID,
		AmountCents: input.AmountCents, Reason: input.Reason, Status: AdjustmentStatusPending,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repo != nil {
		if err := upsertAdjustment(context.Background(), s.repo.pool, a); err != nil {
			return nil, err
		}
	}
	s.finance.adjustments[a.ID] = a
	cp := *a
	return &cp, nil
}

// VoidAdjustment cancels a pending adjustment.
func (s *Service) VoidAdjustment(orgID, actorID string, principal *appmiddleware.Principal, adjustmentID string) (*Adjustment, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	a, ok := s.finance.adjustments[adjustmentID]
	if !ok || a.OrgID != orgID {
		return nil, ErrAdjustmentNotFound
	}
	if a.Status != AdjustmentStatusPending {
		return nil, ErrAdjustmentNotPending
	}
	updated := *a
	updated.Status = AdjustmentStatusVoided
	updated.UpdatedAt = time.Now().UTC()
	updated.UpdatedBy = actorID
	if s.repo != nil {
		tag, err := s.repo.pool.Exec(context.Background(),
			`UPDATE billing_adjustments SET status = 'voided', updated_at = $2, updated_by = $3 WHERE id = $1::uuid AND status = 'pending'`,
			a.ID, updated.UpdatedAt, actorID)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() != 1 {
			return nil, ErrAdjustmentNotPending
		}
	}
	*a = updated
	cp := updated
	return &cp, nil
}

// ─── Settings ────────────────────────────────────────────────────────────────

func (s *Service) settingsLocked(orgID string) *BillingSettings {
	if st, ok := s.finance.settings[orgID]; ok {
		return st
	}
	return defaultBillingSettings(orgID)
}

// GetBillingSettings returns the org's late-fee and automation settings.
func (s *Service) GetBillingSettings(orgID string, principal *appmiddleware.Principal) (*BillingSettings, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	cp := *s.settingsLocked(orgID)
	return &cp, nil
}

// UpdateBillingSettings replaces the org's billing settings (admin only).
func (s *Service) UpdateBillingSettings(orgID, actorID string, principal *appmiddleware.Principal, input BillingSettingsInput) (*BillingSettings, *BillingSettings, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, err
	}
	if input.LateFeeEnabled {
		if input.LateFeeType == LateFeeTypeFlat && input.LateFeeFlatCents <= 0 {
			return nil, nil, invalidField("late_fee_flat_cents", "must be positive when a flat late fee is enabled")
		}
		if input.LateFeeType == LateFeeTypePercent && input.LateFeePercentBps <= 0 {
			return nil, nil, invalidField("late_fee_percent_bps", "must be positive when a percentage late fee is enabled")
		}
	}
	meetFeeStatus := input.MeetFeeInvoiceStatus
	if meetFeeStatus == "" {
		meetFeeStatus = "draft"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, nil, err
	}
	before := *s.settingsLocked(orgID)
	st := &BillingSettings{
		OrgID: orgID, LateFeeEnabled: input.LateFeeEnabled, LateFeeGraceDays: input.LateFeeGraceDays,
		LateFeeType: input.LateFeeType, LateFeeFlatCents: input.LateFeeFlatCents, LateFeePercentBps: input.LateFeePercentBps,
		LateFeeMaxCents: input.LateFeeMaxCents, AutoRunEnabled: input.AutoRunEnabled, AutoRunDay: input.AutoRunDay,
		AutoRunInvoiceStatus: input.AutoRunInvoiceStatus, AutoRunDueDays: input.AutoRunDueDays,
		MeetFeePerEventCents: input.MeetFeePerEventCents, MeetFeeFlatCents: input.MeetFeeFlatCents,
		MeetFeeInvoiceStatus: meetFeeStatus, MeetFeeDueDays: input.MeetFeeDueDays,
		UpdatedAt: time.Now().UTC(), UpdatedBy: actorID,
	}
	if s.repo != nil {
		if err := s.repo.upsertSettings(context.Background(), st); err != nil {
			return nil, nil, err
		}
	}
	s.finance.settings[orgID] = st
	cp := *st
	return &before, &cp, nil
}

// releaseInvoiceFinanceLocked releases a voided invoice's run charges and
// returns its adjustments to pending. Must be called with s.mu held (write).
func (s *Service) releaseInvoiceFinanceLocked(ctx context.Context, orgID, invoiceID, actorID string) error {
	now := time.Now().UTC()
	if s.repo != nil {
		if err := s.repo.inTx(ctx, func(tx pgx.Tx) error {
			if err := releaseCharges(ctx, tx, invoiceID, now); err != nil {
				return err
			}
			return reopenAdjustments(ctx, tx, invoiceID, now, actorID)
		}); err != nil {
			return err
		}
	}
	for _, c := range s.finance.charges {
		if c.InvoiceID == invoiceID && c.ReleasedAt == nil {
			released := now
			c.ReleasedAt = &released
			delete(s.finance.chargeKeys, chargeKey(c.OrgID, c.ScheduleID, c.PeriodKey, c.HouseholdID, c.SwimmerID))
		}
	}
	for _, a := range s.finance.adjustments {
		if a.OrgID == orgID && a.AppliedInvoiceID == invoiceID && a.Status == AdjustmentStatusApplied {
			a.Status = AdjustmentStatusPending
			a.AppliedInvoiceID = ""
			a.AppliedRunID = ""
			a.UpdatedAt = now
			a.UpdatedBy = actorID
		}
	}
	return nil
}
