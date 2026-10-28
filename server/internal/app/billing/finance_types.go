package billing

import (
	"errors"
	"time"
)

// ─── Club finance: shared types ──────────────────────────────────────────────
//
// Fee schedules describe recurring dues (per roster group) and per-season
// registration fees. Billing runs turn schedules + roster into invoices,
// guarded by a unique charge key per (schedule, period, household, swimmer) so
// re-running the same period never double-bills. Household account credit is
// a non-negative balance with an append-only entry trail that is auto-applied
// to newly issued invoices. All money is integer cents.

const (
	ScheduleKindDues         = "dues"
	ScheduleKindRegistration = "registration"

	FrequencyMonthly   = "monthly"
	FrequencyQuarterly = "quarterly"
	FrequencyAnnual    = "annual"
	FrequencyOneTime   = "one_time"

	ProrationNone  = "none"  // full amount regardless of join date
	ProrationDaily = "daily" // amount × active days / period days
	ProrationHalf  = "half"  // 50% when active for less than half the period
	ProrationWaive = "waive" // no charge for a partial join period

	DiscountAppliesDues         = "dues"
	DiscountAppliesRegistration = "registration"
	DiscountAppliesAll          = "all"

	DiscountTypePercent = "percent"
	DiscountTypeFixed   = "fixed"

	AdjustmentStatusPending = "pending"
	AdjustmentStatusApplied = "applied"
	AdjustmentStatusVoided  = "voided"

	CreditKindManualCredit   = "manual_credit"
	CreditKindManualDebit    = "manual_debit"
	CreditKindOverpayment    = "overpayment"
	CreditKindRefundToCredit = "refund_to_credit"
	CreditKindApplied        = "applied"

	PaymentMethodAccountCredit = "account_credit"

	RunTriggerManual    = "manual"
	RunTriggerScheduler = "scheduler"

	LateFeeTypeFlat    = "flat"
	LateFeeTypePercent = "percent"

	itemCategoryProgramFee      = "program_fee"
	itemCategoryRegistrationFee = "registration_fee"
	itemCategoryDiscount        = "discount"
	itemCategoryAdjustment      = "adjustment"
	itemCategoryLateFee         = "late_fee"
)

var (
	ErrFeeScheduleNotFound         = errors.New("fee schedule not found")
	ErrDiscountRuleNotFound        = errors.New("discount rule not found")
	ErrAdjustmentNotFound          = errors.New("adjustment not found")
	ErrAdjustmentNotPending        = errors.New("adjustment is not pending")
	ErrRunNotFound                 = errors.New("billing run not found")
	ErrRunPreviewStale             = errors.New("billing run preview is stale; review the new preview before committing")
	ErrInvalidFinanceInput         = errors.New("invalid finance input")
	ErrInsufficientCredit          = errors.New("insufficient household account credit")
	ErrHouseholdRequired           = errors.New("invoice has no household")
	ErrCreditPaymentRefundToCredit = errors.New("account-credit payments can only be refunded back to account credit")
	ErrRosterUnavailable           = errors.New("roster source is not configured")
)

// FinanceValidationError carries a field-level validation message.
type FinanceValidationError struct {
	Field   string
	Message string
}

func (e *FinanceValidationError) Error() string { return e.Field + ": " + e.Message }

func (e *FinanceValidationError) Unwrap() error { return ErrInvalidFinanceInput }

func invalidField(field, message string) error {
	return &FinanceValidationError{Field: field, Message: message}
}

// FeeSchedule is a recurring dues line for a roster group, or a one-time
// registration fee for a season.
type FeeSchedule struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	GroupCode   string    `json:"group_code,omitempty"`
	Season      string    `json:"season,omitempty"`
	Frequency   string    `json:"frequency"`
	AmountCents int       `json:"amount_cents"`
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date,omitempty"`
	Proration   string    `json:"proration"`
	AutoBill    bool      `json:"auto_bill"`
	Active      bool      `json:"active"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by,omitempty"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
}

// FeeScheduleInput creates or replaces a fee schedule.
type FeeScheduleInput struct {
	Name        string `json:"name" validate:"required,max=120"`
	Kind        string `json:"kind" validate:"required,oneof=dues registration"`
	GroupCode   string `json:"group_code" validate:"max=40"`
	Season      string `json:"season" validate:"max=80"`
	Frequency   string `json:"frequency" validate:"required,oneof=monthly quarterly annual one_time"`
	AmountCents int    `json:"amount_cents" validate:"required,gt=0,lte=100000000"`
	StartDate   string `json:"start_date" validate:"required"`
	EndDate     string `json:"end_date"`
	Proration   string `json:"proration" validate:"omitempty,oneof=none daily half waive"`
	AutoBill    bool   `json:"auto_bill"`
	Active      *bool  `json:"active"`
	Description string `json:"description" validate:"max=500"`
}

// DiscountRule is one sibling/family tier: swimmers at positions
// [MinPosition, MaxPosition] within a household get the discount.
type DiscountRule struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	Name         string    `json:"name"`
	AppliesTo    string    `json:"applies_to"`
	MinPosition  int       `json:"min_position"`
	MaxPosition  *int      `json:"max_position,omitempty"`
	DiscountType string    `json:"discount_type"`
	PercentBps   int       `json:"percent_bps"`
	AmountCents  int       `json:"amount_cents"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    string    `json:"created_by,omitempty"`
	UpdatedBy    string    `json:"updated_by,omitempty"`
}

// DiscountRuleInput creates or replaces a discount rule.
type DiscountRuleInput struct {
	Name         string `json:"name" validate:"required,max=120"`
	AppliesTo    string `json:"applies_to" validate:"required,oneof=dues registration all"`
	MinPosition  int    `json:"min_position" validate:"required,gte=2,lte=20"`
	MaxPosition  *int   `json:"max_position" validate:"omitempty,gte=2,lte=20"`
	DiscountType string `json:"discount_type" validate:"required,oneof=percent fixed"`
	PercentBps   int    `json:"percent_bps" validate:"gte=0,lte=10000"`
	AmountCents  int    `json:"amount_cents" validate:"gte=0,lte=100000000"`
	Active       *bool  `json:"active"`
}

// Adjustment is a manual one-off charge (positive) or discount (negative)
// that the next billing run for the household picks up exactly once.
type Adjustment struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	HouseholdID      string    `json:"household_id"`
	SwimmerID        string    `json:"swimmer_id,omitempty"`
	AmountCents      int       `json:"amount_cents"`
	Reason           string    `json:"reason"`
	Status           string    `json:"status"`
	AppliedInvoiceID string    `json:"applied_invoice_id,omitempty"`
	AppliedRunID     string    `json:"applied_run_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedBy        string    `json:"created_by,omitempty"`
	UpdatedBy        string    `json:"updated_by,omitempty"`
}

// AdjustmentInput creates a manual adjustment.
type AdjustmentInput struct {
	HouseholdID string `json:"household_id" validate:"required"`
	SwimmerID   string `json:"swimmer_id"`
	AmountCents int    `json:"amount_cents" validate:"required,ne=0,gte=-100000000,lte=100000000"`
	Reason      string `json:"reason" validate:"required,max=500"`
}

// BillingSettings holds the org's late-fee rule and billing automation.
type BillingSettings struct {
	OrgID                string    `json:"org_id"`
	LateFeeEnabled       bool      `json:"late_fee_enabled"`
	LateFeeGraceDays     int       `json:"late_fee_grace_days"`
	LateFeeType          string    `json:"late_fee_type"`
	LateFeeFlatCents     int       `json:"late_fee_flat_cents"`
	LateFeePercentBps    int       `json:"late_fee_percent_bps"`
	LateFeeMaxCents      int       `json:"late_fee_max_cents"`
	AutoRunEnabled       bool      `json:"auto_run_enabled"`
	AutoRunDay           int       `json:"auto_run_day"`
	AutoRunInvoiceStatus string    `json:"auto_run_invoice_status"`
	AutoRunDueDays       int       `json:"auto_run_due_days"`
	UpdatedAt            time.Time `json:"updated_at"`
	UpdatedBy            string    `json:"updated_by,omitempty"`
}

// BillingSettingsInput replaces the org's billing settings.
type BillingSettingsInput struct {
	LateFeeEnabled       bool   `json:"late_fee_enabled"`
	LateFeeGraceDays     int    `json:"late_fee_grace_days" validate:"gte=0,lte=365"`
	LateFeeType          string `json:"late_fee_type" validate:"required,oneof=flat percent"`
	LateFeeFlatCents     int    `json:"late_fee_flat_cents" validate:"gte=0,lte=10000000"`
	LateFeePercentBps    int    `json:"late_fee_percent_bps" validate:"gte=0,lte=10000"`
	LateFeeMaxCents      int    `json:"late_fee_max_cents" validate:"gte=0,lte=10000000"`
	AutoRunEnabled       bool   `json:"auto_run_enabled"`
	AutoRunDay           int    `json:"auto_run_day" validate:"gte=1,lte=28"`
	AutoRunInvoiceStatus string `json:"auto_run_invoice_status" validate:"required,oneof=draft issued"`
	AutoRunDueDays       int    `json:"auto_run_due_days" validate:"gte=0,lte=120"`
}

func defaultBillingSettings(orgID string) *BillingSettings {
	return &BillingSettings{
		OrgID:                orgID,
		LateFeeGraceDays:     7,
		LateFeeType:          LateFeeTypeFlat,
		AutoRunDay:           1,
		AutoRunInvoiceStatus: "draft",
		AutoRunDueDays:       15,
	}
}

// BillingRun is the committed record of one billing run.
type BillingRun struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	AsOf               string    `json:"as_of"`
	InvoiceStatus      string    `json:"invoice_status"`
	DueDate            string    `json:"due_date,omitempty"`
	ScheduleIDs        []string  `json:"schedule_ids"`
	Trigger            string    `json:"trigger"`
	Fingerprint        string    `json:"fingerprint"`
	HouseholdCount     int       `json:"household_count"`
	InvoiceCount       int       `json:"invoice_count"`
	ChargeCount        int       `json:"charge_count"`
	SkippedCount       int       `json:"skipped_count"`
	GrossCents         int       `json:"gross_cents"`
	DiscountCents      int       `json:"discount_cents"`
	AdjustmentCents    int       `json:"adjustment_cents"`
	CreditAppliedCents int       `json:"credit_applied_cents"`
	NetCents           int       `json:"net_cents"`
	CreatedAt          time.Time `json:"created_at"`
	CreatedBy          string    `json:"created_by,omitempty"`
}

// BillingCharge is the idempotency record for one billed schedule period.
type BillingCharge struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	RunID           string     `json:"run_id"`
	InvoiceID       string     `json:"invoice_id"`
	HouseholdID     string     `json:"household_id"`
	SwimmerID       string     `json:"swimmer_id,omitempty"`
	ScheduleID      string     `json:"schedule_id"`
	PeriodKey       string     `json:"period_key"`
	PeriodStart     string     `json:"period_start"`
	PeriodEnd       string     `json:"period_end"`
	GrossCents      int        `json:"gross_cents"`
	DiscountCents   int        `json:"discount_cents"`
	NetCents        int        `json:"net_cents"`
	SiblingPosition int        `json:"sibling_position"`
	ReleasedAt      *time.Time `json:"released_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func chargeKey(orgID, scheduleID, periodKey, householdID, swimmerID string) string {
	return orgID + "|" + scheduleID + "|" + periodKey + "|" + householdID + "|" + swimmerID
}

// CreditEntry is one movement in a household's account credit.
type CreditEntry struct {
	ID                string    `json:"id"`
	OrgID             string    `json:"org_id"`
	HouseholdID       string    `json:"household_id"`
	AmountCents       int       `json:"amount_cents"`
	Kind              string    `json:"kind"`
	InvoiceID         string    `json:"invoice_id,omitempty"`
	PaymentID         string    `json:"payment_id,omitempty"`
	RefundID          string    `json:"refund_id,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	BalanceAfterCents int       `json:"balance_after_cents"`
	CreatedAt         time.Time `json:"created_at"`
	CreatedBy         string    `json:"created_by,omitempty"`
}

// LateFeeApplication records the single late fee assessed on an invoice.
type LateFeeApplication struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	InvoiceID   string    `json:"invoice_id"`
	ItemID      string    `json:"item_id"`
	AmountCents int       `json:"amount_cents"`
	AppliedAt   time.Time `json:"applied_at"`
}

// financeState is the in-memory cache of finance tables. Guarded by Service.mu.
type financeState struct {
	schedules   map[string]*FeeSchedule
	rules       map[string]*DiscountRule
	adjustments map[string]*Adjustment
	runs        map[string]*BillingRun
	charges     []*BillingCharge
	chargeKeys  map[string]*BillingCharge // active (unreleased) charges by key
	credits     []*CreditEntry
	balances    map[string]int // orgID|householdID → balance cents
	settings    map[string]*BillingSettings
	lateFees    map[string]*LateFeeApplication // orgID|invoiceID
}

func newFinanceState() *financeState {
	return &financeState{
		schedules:   make(map[string]*FeeSchedule),
		rules:       make(map[string]*DiscountRule),
		adjustments: make(map[string]*Adjustment),
		runs:        make(map[string]*BillingRun),
		chargeKeys:  make(map[string]*BillingCharge),
		balances:    make(map[string]int),
		settings:    make(map[string]*BillingSettings),
		lateFees:    make(map[string]*LateFeeApplication),
	}
}

func (f *financeState) indexCharge(c *BillingCharge) {
	f.charges = append(f.charges, c)
	if c.ReleasedAt == nil {
		f.chargeKeys[chargeKey(c.OrgID, c.ScheduleID, c.PeriodKey, c.HouseholdID, c.SwimmerID)] = c
	}
}

func balanceKey(orgID, householdID string) string { return orgID + "|" + householdID }

// ─── Roster source ───────────────────────────────────────────────────────────

// RosterHousehold, RosterSwimmer, … are the billing-relevant projections of
// the members/groups domains, supplied by a RosterSource wired in server.go so
// billing never imports those packages.
type RosterHousehold struct {
	ID     string
	Name   string
	Status string
}

type RosterSwimmer struct {
	ID          string
	HouseholdID string
	FirstName   string
	LastName    string
	DateOfBirth string
	Status      string
}

type RosterGroupLink struct {
	SwimmerID  string
	GroupCode  string
	IsPrimary  bool
	AssignedAt time.Time
}

type RosterRegistration struct {
	ID               string
	HouseholdID      string
	SwimmerID        string
	GroupCode        string
	Season           string
	Status           string
	RegistrationDate string
}

type RosterGroup struct {
	Code string
	Name string
}

// RosterSnapshot is a point-in-time view of the roster for one org.
type RosterSnapshot struct {
	Households    []RosterHousehold
	Swimmers      []RosterSwimmer
	GroupLinks    []RosterGroupLink
	Registrations []RosterRegistration
	Groups        []RosterGroup
}

// RosterSource returns the current roster for an org.
type RosterSource func(orgID string) (RosterSnapshot, error)

// SetRosterSource wires the roster provider used by billing runs, upcoming
// charges, and group-level revenue reporting.
func (s *Service) SetRosterSource(src RosterSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roster = src
}

func (s *Service) loadRoster(orgID string) (RosterSnapshot, error) {
	s.mu.RLock()
	src := s.roster
	s.mu.RUnlock()
	if src == nil {
		return RosterSnapshot{}, ErrRosterUnavailable
	}
	return src(orgID)
}
