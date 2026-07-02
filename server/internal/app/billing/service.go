//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

// Package billing provides the in-memory service for invoices, payments,
// refunds, and the append-only ledger.
//
// Production note: all data is held in-memory maps protected by a RWMutex.
// Replace with sqlc/pgx queries when the DB layer is wired.
package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrForbidden                    = errors.New("forbidden")
	ErrInvoiceNotFound              = errors.New("invoice not found")
	ErrInvoiceItemNotFound          = errors.New("invoice item not found")
	ErrPaymentNotFound              = errors.New("payment not found")
	ErrRefundNotFound               = errors.New("refund not found")
	ErrInvalidTransition            = errors.New("invalid invoice state transition")
	ErrInvoiceNotDraft              = errors.New("invoice must be in draft status to modify items")
	ErrInvoiceNotIssuable           = errors.New("invoice must be in draft status to issue")
	ErrInvoiceNotVoidable           = errors.New("invoice must be in issued status to void")
	ErrInvoiceNotWriteOffable       = errors.New("invoice must be in issued or partially_paid status to write off")
	ErrPaymentExceedsBalance        = errors.New("payment amount exceeds outstanding balance")
	ErrRefundExceedsPayment         = errors.New("refund amount exceeds original payment amount")
	ErrRefundRequiresReason         = errors.New("a reason is required for refund requests")
	ErrDuplicateInvoiceNumber       = errors.New("invoice number already exists for this organisation")
	ErrWebhookNotFound              = errors.New("webhook event not found")
	ErrAutopayPaymentMethodRequired = errors.New("autopay payment method metadata is required")
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// Invoice represents a billing invoice for a household.
type Invoice struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	HouseholdID     string     `json:"household_id,omitempty"`
	InvoiceNumber   string     `json:"invoice_number"`
	Status          string     `json:"status"`
	SubtotalCents   int        `json:"subtotal_cents"`
	TaxCents        int        `json:"tax_cents"`
	TotalCents      int        `json:"total_cents"`
	AmountPaidCents int        `json:"amount_paid_cents"`
	AmountDueCents  int        `json:"amount_due_cents"`
	DueDate         string     `json:"due_date,omitempty"` // YYYY-MM-DD
	IssuedAt        *time.Time `json:"issued_at,omitempty"`
	VoidedAt        *time.Time `json:"voided_at,omitempty"`
	WrittenOffAt    *time.Time `json:"written_off_at,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	ExternalRef     string     `json:"external_ref,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CreatedBy       string     `json:"created_by"`
	UpdatedBy       string     `json:"updated_by"`
}

// InvoiceItem represents a line item on an invoice.
type InvoiceItem struct {
	ID              string    `json:"id"`
	OrgID           string    `json:"org_id"`
	InvoiceID       string    `json:"invoice_id"`
	Description     string    `json:"description"`
	Quantity        int       `json:"quantity"`
	UnitAmountCents int       `json:"unit_amount_cents"`
	TotalCents      int       `json:"total_cents"`
	Category        string    `json:"category"`
	SwimmerID       string    `json:"swimmer_id,omitempty"`
	MeetID          string    `json:"meet_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedBy       string    `json:"created_by"`
	UpdatedBy       string    `json:"updated_by"`
}

// Payment represents a payment record against an invoice.
type Payment struct {
	ID                    string     `json:"id"`
	OrgID                 string     `json:"org_id"`
	InvoiceID             string     `json:"invoice_id"`
	AmountCents           int        `json:"amount_cents"`
	Method                string     `json:"method"`
	Status                string     `json:"status"`
	StripePaymentIntentID string     `json:"stripe_payment_intent_id,omitempty"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	FailedAt              *time.Time `json:"failed_at,omitempty"`
	ExternalRef           string     `json:"external_ref,omitempty"`
	Notes                 string     `json:"notes,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	CreatedBy             string     `json:"created_by"`
	UpdatedBy             string     `json:"updated_by"`
}

// Refund represents a refund request against a payment.
type Refund struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"org_id"`
	InvoiceID      string     `json:"invoice_id"`
	PaymentID      string     `json:"payment_id"`
	AmountCents    int        `json:"amount_cents"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	StripeRefundID string     `json:"stripe_refund_id,omitempty"`
	ProcessedAt    *time.Time `json:"processed_at,omitempty"`
	RejectedAt     *time.Time `json:"rejected_at,omitempty"`
	RequestedBy    string     `json:"requested_by,omitempty"`
	ApprovedBy     string     `json:"approved_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CreatedBy      string     `json:"created_by"`
	UpdatedBy      string     `json:"updated_by"`
}

// LedgerEvent represents an immutable entry in the billing ledger.
type LedgerEvent struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	InvoiceID   string    `json:"invoice_id,omitempty"`
	PaymentID   string    `json:"payment_id,omitempty"`
	RefundID    string    `json:"refund_id,omitempty"`
	EventType   string    `json:"event_type"`
	AmountCents int       `json:"amount_cents"`
	Description string    `json:"description,omitempty"`
	ActorID     string    `json:"actor_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// WebhookEventRecord represents a persisted Stripe webhook delivery.
// The table is effectively append-only; processed is updated in-place
// (in the DB layer this would be a single UPDATE).
type WebhookEventRecord struct {
	ID           string          `json:"id"`
	OrgID        string          `json:"org_id"`
	Provider     string          `json:"provider"`   // e.g. "stripe"
	EventID      string          `json:"event_id"`   // Stripe event ID
	EventType    string          `json:"event_type"` // e.g. "payment_intent.succeeded"
	Payload      json.RawMessage `json:"payload"`    // raw event JSON
	Processed    bool            `json:"processed"`
	ProcessedAt  *time.Time      `json:"processed_at,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ReconciliationReport compares the internal billing ledger (Stripe payments)
// against the amount reported by Stripe. A mismatch above the threshold
// triggers an alert.
type ReconciliationReport struct {
	LedgerStripeTotalCents int       `json:"ledger_stripe_total_cents"`
	StripeReportedCents    int       `json:"stripe_reported_cents"`
	MismatchCents          int       `json:"mismatch_cents"`
	HasMismatch            bool      `json:"has_mismatch"`
	AlertTriggered         bool      `json:"alert_triggered"`
	AlertThresholdCents    int       `json:"alert_threshold_cents"`
	GeneratedAt            time.Time `json:"generated_at"`
}

type HouseholdAutopayProfile struct {
	ID                       string    `json:"id"`
	OrgID                    string    `json:"org_id"`
	HouseholdID              string    `json:"household_id"`
	Enabled                  bool      `json:"enabled"`
	PaymentMethodType        string    `json:"payment_method_type"`
	PaymentMethodBrand       string    `json:"payment_method_brand,omitempty"`
	PaymentMethodLast4       string    `json:"payment_method_last4,omitempty"`
	PaymentMethodLabel       string    `json:"payment_method_label"`
	PaymentMethodExternalRef string    `json:"payment_method_external_ref,omitempty"`
	PaymentMethodExpMonth    int       `json:"payment_method_exp_month,omitempty"`
	PaymentMethodExpYear     int       `json:"payment_method_exp_year,omitempty"`
	TriggerDaysBeforeDue     int       `json:"trigger_days_before_due"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	UpdatedBy                string    `json:"updated_by,omitempty"`
}

type HouseholdAutopayStatus struct {
	HouseholdID              string `json:"household_id"`
	Enabled                  bool   `json:"enabled"`
	PaymentMethodType        string `json:"payment_method_type,omitempty"`
	PaymentMethodBrand       string `json:"payment_method_brand,omitempty"`
	PaymentMethodLast4       string `json:"payment_method_last4,omitempty"`
	PaymentMethodLabel       string `json:"payment_method_label,omitempty"`
	PaymentMethodExternalRef string `json:"payment_method_external_ref,omitempty"`
	PaymentMethodExpMonth    int    `json:"payment_method_exp_month,omitempty"`
	PaymentMethodExpYear     int    `json:"payment_method_exp_year,omitempty"`
	TriggerDaysBeforeDue     int    `json:"trigger_days_before_due"`
	NextChargeDate           string `json:"next_charge_date,omitempty"`
	NextChargeAmountCents    int    `json:"next_charge_amount_cents,omitempty"`
	NextInvoiceID            string `json:"next_invoice_id,omitempty"`
}

// ─── Input types ─────────────────────────────────────────────────────────────

// CreateInvoiceInput holds fields for creating an invoice.
type CreateInvoiceInput struct {
	HouseholdID   string `json:"household_id"`
	InvoiceNumber string `json:"invoice_number" validate:"required"`
	DueDate       string `json:"due_date"` // YYYY-MM-DD optional
	Notes         string `json:"notes"`
	ExternalRef   string `json:"external_ref"`
}

// AddInvoiceItemInput holds fields for adding a line item to an invoice.
type AddInvoiceItemInput struct {
	Description     string `json:"description" validate:"required"`
	Quantity        int    `json:"quantity" validate:"required,gt=0"`
	UnitAmountCents int    `json:"unit_amount_cents" validate:"required,gt=0"`
	Category        string `json:"category" validate:"required,oneof=program_fee meet_fee equipment manual late_fee credit"`
	SwimmerID       string `json:"swimmer_id"`
	MeetID          string `json:"meet_id"`
}

// RecordPaymentInput holds fields for recording an external payment.
type RecordPaymentInput struct {
	AmountCents           int    `json:"amount_cents" validate:"required,gt=0"`
	Method                string `json:"method" validate:"required,oneof=stripe check cash ach other"`
	StripePaymentIntentID string `json:"stripe_payment_intent_id"`
	ExternalRef           string `json:"external_ref"`
	Notes                 string `json:"notes"`
	PaidAt                string `json:"paid_at"`
}

// CreateRefundInput holds fields for creating a refund request.
type CreateRefundInput struct {
	PaymentID      string `json:"payment_id" validate:"required"`
	AmountCents    int    `json:"amount_cents" validate:"required,gt=0"`
	Reason         string `json:"reason" validate:"required"`
	StripeRefundID string `json:"-"`
}

// RefundPreflight captures the local state validated before an external refund
// is attempted.
type RefundPreflight struct {
	Invoice Invoice
	Payment Payment
}

// ListInvoicesFilters supports optional filtering for invoice queries.
type ListInvoicesFilters struct {
	HouseholdID string
	Status      string
}

// ListLedgerFilters supports optional filtering for ledger event queries.
type ListLedgerFilters struct {
	InvoiceID string
	EventType string
}

// ListPaymentsFilters supports optional filtering for org-wide payment queries.
type ListPaymentsFilters struct {
	InvoiceID string
	Status    string
}

type UpsertHouseholdAutopayInput struct {
	PaymentMethodType        string `json:"payment_method_type" validate:"required,oneof=card ach"`
	PaymentMethodBrand       string `json:"payment_method_brand"`
	PaymentMethodLast4       string `json:"payment_method_last4"`
	PaymentMethodLabel       string `json:"payment_method_label" validate:"required,max=120"`
	PaymentMethodExternalRef string `json:"payment_method_external_ref"`
	PaymentMethodExpMonth    int    `json:"payment_method_exp_month"`
	PaymentMethodExpYear     int    `json:"payment_method_exp_year"`
	TriggerDaysBeforeDue     int    `json:"trigger_days_before_due" validate:"oneof=0 3 7"`
}

// ─── Service ─────────────────────────────────────────────────────────────────

// Service is the billing domain service.
type Service struct {
	mu                sync.RWMutex
	repo              *Repository
	invoices          map[string]*Invoice
	items             map[string][]*InvoiceItem // invoiceID → []InvoiceItem
	itemsByID         map[string]*InvoiceItem   // itemID → InvoiceItem
	payments          map[string][]*Payment     // invoiceID → []Payment
	paymentsByID      map[string]*Payment       // paymentID → Payment
	stripeIntentIndex map[string]*Payment       // stripePaymentIntentID → Payment
	refunds           map[string][]*Refund      // invoiceID → []Refund
	refundsByID       map[string]*Refund        // refundID → Refund
	ledger            []*LedgerEvent
	invoiceNums       map[string]bool                // "orgID|invoiceNumber" → true
	webhookEvents     map[string]*WebhookEventRecord // "provider|eventID" → record
	autopayProfiles   map[string]*HouseholdAutopayProfile
}

// NewService creates a new in-memory billing Service.
func NewService() *Service {
	return &Service{
		invoices:          make(map[string]*Invoice),
		items:             make(map[string][]*InvoiceItem),
		itemsByID:         make(map[string]*InvoiceItem),
		payments:          make(map[string][]*Payment),
		paymentsByID:      make(map[string]*Payment),
		stripeIntentIndex: make(map[string]*Payment),
		refunds:           make(map[string][]*Refund),
		refundsByID:       make(map[string]*Refund),
		invoiceNums:       make(map[string]bool),
		webhookEvents:     make(map[string]*WebhookEventRecord),
		autopayProfiles:   make(map[string]*HouseholdAutopayProfile),
	}
}

// NewServiceWithPool creates a DB-backed billing Service and loads existing
// data from Postgres into memory on startup.
func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	if pool == nil {
		return s
	}
	s.repo = newRepository(pool)
	if err := s.loadFromDB(context.Background()); err != nil {
		slog.Error("billing: failed to load from DB", "err", err)
	}
	return s
}

func (s *Service) loadFromDB(ctx context.Context) error {
	invoices, items, payments, refunds, ledger, autopayProfiles, err := s.repo.LoadAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inv := range invoices {
		s.invoices[inv.ID] = inv
		s.invoiceNums[invoiceNumKey(inv.OrgID, inv.InvoiceNumber)] = true
	}
	for _, item := range items {
		s.items[item.InvoiceID] = append(s.items[item.InvoiceID], item)
		s.itemsByID[item.ID] = item
	}
	for _, p := range payments {
		s.payments[p.InvoiceID] = append(s.payments[p.InvoiceID], p)
		s.paymentsByID[p.ID] = p
		if p.StripePaymentIntentID != "" {
			s.stripeIntentIndex[p.StripePaymentIntentID] = p
		}
	}
	for _, r := range refunds {
		s.refunds[r.InvoiceID] = append(s.refunds[r.InvoiceID], r)
		s.refundsByID[r.ID] = r
	}
	s.ledger = ledger
	for _, profile := range autopayProfiles {
		s.autopayProfiles[autopayProfileKey(profile.OrgID, profile.HouseholdID)] = profile
	}
	return nil
}

func (s *Service) newID() string {
	if s.repo != nil {
		return uuid.New().String()
	}
	return newID()
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("secure random unavailable for billing ID generation: %w", err))
	}
	return hex.EncodeToString(b)
}

func (s *Service) requireBillingRole(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin", "owner", "billing":
			return nil
		}
	}
	return ErrForbidden
}

// RequireBillingWriteAccess is the exported counterpart of requireBillingRole.
// It allows handler packages to delegate role enforcement to the service layer
// without duplicating the role list locally.
func (s *Service) RequireBillingWriteAccess(principal *appmiddleware.Principal) error {
	return s.requireBillingRole(principal)
}

func (s *Service) requireAdmin(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin", "owner":
			return nil
		}
	}
	return ErrForbidden
}

func (s *Service) requireBillingReadAccess(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin", "owner", "billing", "parent":
			return nil
		}
	}
	return ErrForbidden
}

func (s *Service) parentHouseholdScope(principal *appmiddleware.Principal) (string, bool) {
	if principal == nil {
		return "", false
	}
	parentOnly := false
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "parent":
			parentOnly = true
		case "admin", "owner", "billing":
			return "", false
		}
	}
	if !parentOnly {
		return "", false
	}
	return strings.TrimSpace(principal.HouseholdID), true
}

func (s *Service) ensureParentHouseholdAccess(principal *appmiddleware.Principal, householdID string) error {
	scopedHouseholdID, parentScoped := s.parentHouseholdScope(principal)
	if !parentScoped {
		return nil
	}
	if scopedHouseholdID == "" || strings.TrimSpace(householdID) != scopedHouseholdID {
		return ErrForbidden
	}
	return nil
}

func (s *Service) requireLedgerReadAccess(principal *appmiddleware.Principal) error {
	for _, r := range principal.Roles {
		switch strings.ToLower(r) {
		case "admin", "owner", "billing":
			return nil
		}
	}
	return ErrForbidden
}

func invoiceNumKey(orgID, number string) string {
	return orgID + "|" + number
}

func autopayProfileKey(orgID, householdID string) string {
	return orgID + "|" + householdID
}

// recomputeTotals updates subtotal, total, amount_due on an invoice from its items.
// Must be called with s.mu held (write).
func (s *Service) recomputeTotals(inv *Invoice) {
	subtotal := 0
	for _, item := range s.items[inv.ID] {
		subtotal += item.TotalCents
	}
	inv.SubtotalCents = subtotal
	inv.TotalCents = subtotal + inv.TaxCents
	inv.AmountDueCents = inv.TotalCents - inv.AmountPaidCents
	if inv.AmountDueCents < 0 {
		inv.AmountDueCents = 0
	}
}

// appendLedger appends an immutable ledger event. Must be called with s.mu held (write).
func (s *Service) appendLedger(orgID, invoiceID, paymentID, refundID, eventType string, amountCents int, description, actorID string) (*LedgerEvent, error) {
	ev := &LedgerEvent{
		ID:          s.newID(),
		OrgID:       orgID,
		InvoiceID:   invoiceID,
		PaymentID:   paymentID,
		RefundID:    refundID,
		EventType:   eventType,
		AmountCents: amountCents,
		Description: description,
		ActorID:     actorID,
		CreatedAt:   time.Now().UTC(),
	}
	if s.repo != nil {
		if err := s.repo.InsertLedgerEvent(context.Background(), ev); err != nil {
			return nil, fmt.Errorf("persist ledger event: %w", err)
		}
	}
	s.ledger = append(s.ledger, ev)
	return ev, nil
}

func normalizeAutopayInput(input UpsertHouseholdAutopayInput) UpsertHouseholdAutopayInput {
	input.PaymentMethodType = strings.ToLower(strings.TrimSpace(input.PaymentMethodType))
	input.PaymentMethodBrand = strings.TrimSpace(input.PaymentMethodBrand)
	input.PaymentMethodLast4 = strings.TrimSpace(input.PaymentMethodLast4)
	input.PaymentMethodLabel = strings.TrimSpace(input.PaymentMethodLabel)
	input.PaymentMethodExternalRef = strings.TrimSpace(input.PaymentMethodExternalRef)
	return input
}

func (s *Service) requireAutopayReadAccess(principal *appmiddleware.Principal, householdID string) error {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return ErrForbidden
	}
	return s.ensureParentHouseholdAccess(principal, householdID)
}

func (s *Service) requireAutopayWriteAccess(principal *appmiddleware.Principal, householdID string) error {
	if principal == nil {
		return ErrForbidden
	}
	for _, role := range principal.Roles {
		switch strings.ToLower(role) {
		case "admin", "owner", "billing":
			return nil
		}
	}
	if err := s.ensureParentHouseholdAccess(principal, householdID); err != nil {
		return err
	}
	for _, role := range principal.Roles {
		if strings.EqualFold(role, "parent") {
			return nil
		}
	}
	return ErrForbidden
}

func (s *Service) buildAutopayStatusLocked(orgID, householdID string, profile *HouseholdAutopayProfile) *HouseholdAutopayStatus {
	status := &HouseholdAutopayStatus{
		HouseholdID: householdID,
	}
	if profile != nil {
		status.Enabled = profile.Enabled
		status.PaymentMethodType = profile.PaymentMethodType
		status.PaymentMethodBrand = profile.PaymentMethodBrand
		status.PaymentMethodLast4 = profile.PaymentMethodLast4
		status.PaymentMethodLabel = profile.PaymentMethodLabel
		status.PaymentMethodExternalRef = profile.PaymentMethodExternalRef
		status.PaymentMethodExpMonth = profile.PaymentMethodExpMonth
		status.PaymentMethodExpYear = profile.PaymentMethodExpYear
		status.TriggerDaysBeforeDue = profile.TriggerDaysBeforeDue
	}

	var nextInvoice *Invoice
	for _, invoice := range s.invoices {
		if invoice.OrgID != orgID || invoice.HouseholdID != householdID {
			continue
		}
		if !slices.Contains([]string{"issued", "partially_paid"}, invoice.Status) || invoice.AmountDueCents <= 0 || strings.TrimSpace(invoice.DueDate) == "" {
			continue
		}
		if nextInvoice == nil || invoice.DueDate < nextInvoice.DueDate {
			nextInvoice = invoice
		}
	}
	if nextInvoice != nil {
		status.NextInvoiceID = nextInvoice.ID
		status.NextChargeAmountCents = nextInvoice.AmountDueCents
		if dueDate, err := time.Parse("2006-01-02", nextInvoice.DueDate); err == nil {
			status.NextChargeDate = dueDate.AddDate(0, 0, -status.TriggerDaysBeforeDue).Format("2006-01-02")
		} else {
			status.NextChargeDate = nextInvoice.DueDate
		}
	}
	return status
}

// updateInvoiceStatusAfterPayment recalculates invoice status based on paid vs total.
// Must be called with s.mu held (write).
func (s *Service) updateInvoiceStatusAfterPayment(inv *Invoice) {
	if inv.AmountPaidCents <= 0 {
		inv.Status = "issued"
		return
	}
	if inv.AmountPaidCents >= inv.TotalCents {
		inv.Status = "paid"
	} else {
		inv.Status = "partially_paid"
	}
	inv.AmountDueCents = inv.TotalCents - inv.AmountPaidCents
	if inv.AmountDueCents < 0 {
		inv.AmountDueCents = 0
	}
}

// ─── Invoices ─────────────────────────────────────────────────────────────────

// CreateInvoice creates a draft invoice.
func (s *Service) CreateInvoice(orgID, actorID string, principal *appmiddleware.Principal, input CreateInvoiceInput) (*Invoice, error) {
	if err := s.requireBillingRole(principal); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := invoiceNumKey(orgID, input.InvoiceNumber)
	if s.invoiceNums[key] {
		return nil, ErrDuplicateInvoiceNumber
	}

	now := time.Now().UTC()
	inv := &Invoice{
		ID:            s.newID(),
		OrgID:         orgID,
		HouseholdID:   input.HouseholdID,
		InvoiceNumber: input.InvoiceNumber,
		Status:        "draft",
		DueDate:       input.DueDate,
		Notes:         input.Notes,
		ExternalRef:   input.ExternalRef,
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
	}

	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, fmt.Errorf("persist invoice: %w", err)
		}
	}
	s.invoices[inv.ID] = inv
	s.invoiceNums[key] = true
	return inv, nil
}

// GetInvoice retrieves an invoice by ID with read access check.
// Returns a value copy to prevent data races with concurrent writes.
func (s *Service) GetInvoice(orgID string, principal *appmiddleware.Principal, invoiceID string) (*Invoice, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}
	if err := s.ensureParentHouseholdAccess(principal, inv.HouseholdID); err != nil {
		return nil, err
	}
	cp := *inv
	return &cp, nil
}

// ListInvoices returns invoices filtered by optional household/status.
func (s *Service) ListInvoices(orgID string, principal *appmiddleware.Principal, filters ListInvoicesFilters) ([]*Invoice, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}
	if scopedHouseholdID, parentScoped := s.parentHouseholdScope(principal); parentScoped {
		if scopedHouseholdID == "" {
			return nil, ErrForbidden
		}
		if filters.HouseholdID != "" && filters.HouseholdID != scopedHouseholdID {
			return nil, ErrForbidden
		}
		filters.HouseholdID = scopedHouseholdID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Invoice
	for _, inv := range s.invoices {
		if inv.OrgID != orgID {
			continue
		}
		if filters.HouseholdID != "" && inv.HouseholdID != filters.HouseholdID {
			continue
		}
		if filters.Status != "" && inv.Status != filters.Status {
			continue
		}
		cp := *inv
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ─── Invoice items ────────────────────────────────────────────────────────────

// AddInvoiceItem adds a line item to a draft invoice.
func (s *Service) AddInvoiceItem(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string, input AddInvoiceItemInput) (*InvoiceItem, error) {
	if err := s.requireBillingRole(principal); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}
	if inv.Status != "draft" {
		return nil, ErrInvoiceNotDraft
	}

	now := time.Now().UTC()
	item := &InvoiceItem{
		ID:              s.newID(),
		OrgID:           orgID,
		InvoiceID:       invoiceID,
		Description:     input.Description,
		Quantity:        input.Quantity,
		UnitAmountCents: input.UnitAmountCents,
		TotalCents:      input.Quantity * input.UnitAmountCents,
		Category:        input.Category,
		SwimmerID:       input.SwimmerID,
		MeetID:          input.MeetID,
		CreatedAt:       now,
		UpdatedAt:       now,
		CreatedBy:       actorID,
		UpdatedBy:       actorID,
	}

	if s.repo != nil {
		if err := s.repo.UpsertInvoiceItem(context.Background(), item); err != nil {
			return nil, fmt.Errorf("persist invoice item: %w", err)
		}
	}
	s.items[invoiceID] = append(s.items[invoiceID], item)
	s.itemsByID[item.ID] = item

	inv.UpdatedAt = now
	inv.UpdatedBy = actorID
	s.recomputeTotals(inv)
	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, fmt.Errorf("persist invoice after item add: %w", err)
		}
	}

	return item, nil
}

// RemoveInvoiceItem removes a line item from a draft invoice.
func (s *Service) RemoveInvoiceItem(orgID, actorID string, principal *appmiddleware.Principal, invoiceID, itemID string) error {
	if err := s.requireBillingRole(principal); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return ErrInvoiceNotFound
	}
	if inv.Status != "draft" {
		return ErrInvoiceNotDraft
	}

	item, ok := s.itemsByID[itemID]
	if !ok || item.InvoiceID != invoiceID {
		return ErrInvoiceItemNotFound
	}

	// Remove from slice.
	s.items[invoiceID] = slices.DeleteFunc(s.items[invoiceID], func(i *InvoiceItem) bool {
		return i.ID == itemID
	})
	delete(s.itemsByID, itemID)

	now := time.Now().UTC()
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID
	s.recomputeTotals(inv)

	return nil
}

// ListInvoiceItems returns all items for an invoice.
func (s *Service) ListInvoiceItems(orgID string, principal *appmiddleware.Principal, invoiceID string) ([]*InvoiceItem, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}
	if err := s.ensureParentHouseholdAccess(principal, inv.HouseholdID); err != nil {
		return nil, err
	}

	items := s.items[invoiceID]
	out := make([]*InvoiceItem, len(items))
	copy(out, items)
	return out, nil
}

// ─── Invoice lifecycle ────────────────────────────────────────────────────────

// IssueInvoice transitions a draft invoice to issued and writes a ledger event.
func (s *Service) IssueInvoice(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string) (*Invoice, *LedgerEvent, error) {
	if err := s.requireBillingRole(principal); err != nil {
		return nil, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}
	if inv.Status != "draft" {
		return nil, nil, ErrInvoiceNotIssuable
	}

	now := time.Now().UTC()
	inv.Status = "issued"
	inv.IssuedAt = &now
	inv.AmountDueCents = inv.TotalCents - inv.AmountPaidCents
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, nil, fmt.Errorf("persist issued invoice: %w", err)
		}
	}
	ev, err := s.appendLedger(orgID, invoiceID, "", "", "invoice_issued", inv.TotalCents,
		fmt.Sprintf("Invoice %s issued", inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return inv, ev, nil
}

// VoidInvoice voids an issued invoice (no payments must exist).
func (s *Service) VoidInvoice(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string) (*Invoice, *LedgerEvent, error) {
	if err := s.requireBillingRole(principal); err != nil {
		return nil, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}
	if inv.Status != "issued" {
		return nil, nil, ErrInvoiceNotVoidable
	}
	if inv.AmountPaidCents > 0 {
		return nil, nil, errors.New("cannot void invoice with existing payments")
	}

	now := time.Now().UTC()
	inv.Status = "voided"
	inv.VoidedAt = &now
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, nil, fmt.Errorf("persist voided invoice: %w", err)
		}
	}
	ev, err := s.appendLedger(orgID, invoiceID, "", "", "invoice_voided", -inv.AmountDueCents,
		fmt.Sprintf("Invoice %s voided", inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return inv, ev, nil
}

// WriteOffInvoice writes off the outstanding balance on an issued/partially_paid invoice.
func (s *Service) WriteOffInvoice(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string) (*Invoice, *LedgerEvent, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}
	if inv.Status != "issued" && inv.Status != "partially_paid" {
		return nil, nil, ErrInvoiceNotWriteOffable
	}

	now := time.Now().UTC()
	remaining := inv.AmountDueCents
	inv.Status = "written_off"
	inv.WrittenOffAt = &now
	inv.AmountDueCents = 0
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, nil, fmt.Errorf("persist written-off invoice: %w", err)
		}
	}
	ev, err := s.appendLedger(orgID, invoiceID, "", "", "invoice_written_off", -remaining,
		fmt.Sprintf("Invoice %s written off", inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return inv, ev, nil
}

// ─── Payments ────────────────────────────────────────────────────────────────

// RecordPayment records an external payment against an issued invoice.
func (s *Service) RecordPayment(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string, input RecordPaymentInput) (*Payment, *LedgerEvent, error) {
	if err := s.requireBillingRole(principal); err != nil {
		return nil, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}
	if inv.Status != "issued" && inv.Status != "partially_paid" {
		return nil, nil, ErrInvalidTransition
	}
	if input.AmountCents > inv.AmountDueCents {
		return nil, nil, ErrPaymentExceedsBalance
	}

	now := time.Now().UTC()
	paidAt := now
	if input.PaidAt != "" {
		parsedPaidAt, err := time.Parse("2006-01-02", strings.TrimSpace(input.PaidAt))
		if err != nil {
			return nil, nil, fmt.Errorf("paid_at must use YYYY-MM-DD format: %w", err)
		}
		paidAt = parsedPaidAt.UTC()
	}
	payment := &Payment{
		ID:                    s.newID(),
		OrgID:                 orgID,
		InvoiceID:             invoiceID,
		AmountCents:           input.AmountCents,
		Method:                input.Method,
		Status:                "succeeded",
		StripePaymentIntentID: input.StripePaymentIntentID,
		PaidAt:                &paidAt,
		ExternalRef:           input.ExternalRef,
		Notes:                 input.Notes,
		CreatedAt:             now,
		UpdatedAt:             now,
		CreatedBy:             actorID,
		UpdatedBy:             actorID,
	}

	if s.repo != nil {
		if err := s.repo.UpsertPayment(context.Background(), payment); err != nil {
			return nil, nil, fmt.Errorf("persist payment: %w", err)
		}
	}
	s.payments[invoiceID] = append(s.payments[invoiceID], payment)
	s.paymentsByID[payment.ID] = payment
	if input.StripePaymentIntentID != "" {
		s.stripeIntentIndex[input.StripePaymentIntentID] = payment
	}

	inv.AmountPaidCents += input.AmountCents
	s.updateInvoiceStatusAfterPayment(inv)
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, nil, fmt.Errorf("persist invoice after payment: %w", err)
		}
	}
	ev, err := s.appendLedger(orgID, invoiceID, payment.ID, "", "payment_succeeded", -input.AmountCents,
		fmt.Sprintf("Payment of %d cents recorded for invoice %s", input.AmountCents, inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return payment, ev, nil
}

// ListPayments returns all payments for an invoice.
func (s *Service) ListPayments(orgID string, principal *appmiddleware.Principal, invoiceID string) ([]*Payment, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}
	if err := s.ensureParentHouseholdAccess(principal, inv.HouseholdID); err != nil {
		return nil, err
	}

	pyts := s.payments[invoiceID]
	out := make([]*Payment, len(pyts))
	for i, p := range pyts {
		cp := *p
		out[i] = &cp
	}
	return out, nil
}

// ListPaymentsForOrg returns payments across the org using optional filters.
func (s *Service) ListPaymentsForOrg(orgID string, principal *appmiddleware.Principal, filters ListPaymentsFilters) ([]*Payment, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	scopedHouseholdID, parentScoped := s.parentHouseholdScope(principal)
	if parentScoped && scopedHouseholdID == "" {
		return nil, ErrForbidden
	}

	var out []*Payment
	for _, payment := range s.paymentsByID {
		if payment.OrgID != orgID {
			continue
		}
		if parentScoped {
			invoice, ok := s.invoices[payment.InvoiceID]
			if !ok || invoice.OrgID != orgID || invoice.HouseholdID != scopedHouseholdID {
				continue
			}
		}
		if filters.InvoiceID != "" && payment.InvoiceID != filters.InvoiceID {
			continue
		}
		if filters.Status != "" && payment.Status != filters.Status {
			continue
		}
		cp := *payment
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		leftPaidAt, rightPaidAt := out[i].PaidAt, out[j].PaidAt
		switch {
		case leftPaidAt != nil && rightPaidAt != nil && !leftPaidAt.Equal(*rightPaidAt):
			return leftPaidAt.After(*rightPaidAt)
		case leftPaidAt != nil && rightPaidAt == nil:
			return true
		case leftPaidAt == nil && rightPaidAt != nil:
			return false
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// GetPayment returns a single payment for an invoice.
func (s *Service) GetPayment(orgID string, principal *appmiddleware.Principal, invoiceID, paymentID string) (*Payment, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}
	if err := s.ensureParentHouseholdAccess(principal, inv.HouseholdID); err != nil {
		return nil, err
	}
	payment, ok := s.paymentsByID[paymentID]
	if !ok || payment.InvoiceID != invoiceID || payment.OrgID != orgID {
		return nil, ErrPaymentNotFound
	}

	cp := *payment
	return &cp, nil
}

// ─── Refunds ─────────────────────────────────────────────────────────────────

// GetHouseholdAutopayStatus returns the current autopay configuration and next-charge preview for a household.
func (s *Service) GetHouseholdAutopayStatus(orgID string, principal *appmiddleware.Principal, householdID string) (*HouseholdAutopayStatus, error) {
	if err := s.requireAutopayReadAccess(principal, householdID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.buildAutopayStatusLocked(orgID, householdID, s.autopayProfiles[autopayProfileKey(orgID, householdID)]), nil
}

// ListHouseholdAutopayStatuses returns autopay statuses for the provided households.
func (s *Service) ListHouseholdAutopayStatuses(orgID string, principal *appmiddleware.Principal, householdIDs []string) ([]*HouseholdAutopayStatus, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	scopedHouseholdID, parentScoped := s.parentHouseholdScope(principal)
	if parentScoped {
		if scopedHouseholdID == "" {
			return nil, ErrForbidden
		}
		householdIDs = []string{scopedHouseholdID}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]struct{}, len(householdIDs))
	normalized := make([]string, 0, len(householdIDs))
	for _, householdID := range householdIDs {
		householdID = strings.TrimSpace(householdID)
		if householdID == "" {
			continue
		}
		if _, ok := seen[householdID]; ok {
			continue
		}
		seen[householdID] = struct{}{}
		normalized = append(normalized, householdID)
	}
	if len(normalized) == 0 {
		for _, profile := range s.autopayProfiles {
			if profile == nil || profile.OrgID != orgID {
				continue
			}
			normalized = append(normalized, profile.HouseholdID)
		}
		sort.Strings(normalized)
	}

	out := make([]*HouseholdAutopayStatus, 0, len(normalized))
	for _, householdID := range normalized {
		out = append(out, s.buildAutopayStatusLocked(orgID, householdID, s.autopayProfiles[autopayProfileKey(orgID, householdID)]))
	}
	return out, nil
}

// UpsertHouseholdAutopay enables autopay and persists payment-method metadata for a household.
func (s *Service) UpsertHouseholdAutopay(orgID, actorID string, principal *appmiddleware.Principal, householdID string, input UpsertHouseholdAutopayInput) (*HouseholdAutopayStatus, error) {
	if err := s.requireAutopayWriteAccess(principal, householdID); err != nil {
		return nil, err
	}

	input = normalizeAutopayInput(input)
	if input.PaymentMethodLabel == "" || input.PaymentMethodType == "" {
		return nil, ErrAutopayPaymentMethodRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	key := autopayProfileKey(orgID, householdID)
	profile := s.autopayProfiles[key]
	if profile == nil {
		profile = &HouseholdAutopayProfile{
			ID:          s.newID(),
			OrgID:       orgID,
			HouseholdID: householdID,
			CreatedAt:   now,
		}
	}
	profile.Enabled = true
	profile.PaymentMethodType = input.PaymentMethodType
	profile.PaymentMethodBrand = input.PaymentMethodBrand
	profile.PaymentMethodLast4 = input.PaymentMethodLast4
	profile.PaymentMethodLabel = input.PaymentMethodLabel
	profile.PaymentMethodExternalRef = input.PaymentMethodExternalRef
	profile.PaymentMethodExpMonth = input.PaymentMethodExpMonth
	profile.PaymentMethodExpYear = input.PaymentMethodExpYear
	profile.TriggerDaysBeforeDue = input.TriggerDaysBeforeDue
	profile.UpdatedAt = now
	profile.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertHouseholdAutopayProfile(context.Background(), profile); err != nil {
			return nil, fmt.Errorf("persist autopay profile: %w", err)
		}
	}
	s.autopayProfiles[key] = profile

	return s.buildAutopayStatusLocked(orgID, householdID, profile), nil
}

// OptOutHouseholdAutopay disables autopay while keeping saved payment metadata.
func (s *Service) OptOutHouseholdAutopay(orgID, actorID string, principal *appmiddleware.Principal, householdID string) (*HouseholdAutopayStatus, error) {
	if err := s.requireAutopayWriteAccess(principal, householdID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	key := autopayProfileKey(orgID, householdID)
	profile := s.autopayProfiles[key]
	if profile == nil {
		profile = &HouseholdAutopayProfile{
			ID:          s.newID(),
			OrgID:       orgID,
			HouseholdID: householdID,
			CreatedAt:   now,
		}
	}
	profile.Enabled = false
	profile.UpdatedAt = now
	profile.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertHouseholdAutopayProfile(context.Background(), profile); err != nil {
			return nil, fmt.Errorf("persist autopay opt-out: %w", err)
		}
	}
	s.autopayProfiles[key] = profile

	return s.buildAutopayStatusLocked(orgID, householdID, profile), nil
}

// ValidateRefundPreflight verifies whether a refund is locally eligible before
// any external provider call is made.
func (s *Service) ValidateRefundPreflight(orgID string, principal *appmiddleware.Principal, invoiceID string, input CreateRefundInput) (*RefundPreflight, error) {
	if err := s.requireBillingRole(principal); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return nil, ErrRefundRequiresReason
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}

	validRefundStatuses := map[string]bool{
		"paid":             true,
		"partially_paid":   true,
		"refunded_partial": true,
	}
	if !validRefundStatuses[inv.Status] {
		return nil, ErrInvalidTransition
	}

	payment, ok := s.paymentsByID[input.PaymentID]
	if !ok || payment.InvoiceID != invoiceID || payment.OrgID != orgID {
		return nil, ErrPaymentNotFound
	}
	if payment.Status != "succeeded" {
		return nil, ErrInvalidTransition
	}

	alreadyRefunded := 0
	for _, r := range s.refunds[invoiceID] {
		if r.PaymentID == input.PaymentID && (r.Status == "pending" || r.Status == "approved" || r.Status == "processed") {
			alreadyRefunded += r.AmountCents
		}
	}
	if alreadyRefunded+input.AmountCents > payment.AmountCents {
		return nil, ErrRefundExceedsPayment
	}

	return &RefundPreflight{
		Invoice: *inv,
		Payment: *payment,
	}, nil
}

// CreateRefund creates a refund request for a payment.
func (s *Service) CreateRefund(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string, input CreateRefundInput) (*Refund, *LedgerEvent, error) {
	if _, err := s.ValidateRefundPreflight(orgID, principal, invoiceID, input); err != nil {
		return nil, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}

	validRefundStatuses := map[string]bool{
		"paid":             true,
		"partially_paid":   true,
		"refunded_partial": true,
	}
	if !validRefundStatuses[inv.Status] {
		return nil, nil, ErrInvalidTransition
	}

	payment, ok := s.paymentsByID[input.PaymentID]
	if !ok || payment.InvoiceID != invoiceID || payment.OrgID != orgID {
		return nil, nil, ErrPaymentNotFound
	}
	if payment.Status != "succeeded" {
		return nil, nil, ErrInvalidTransition
	}

	alreadyRefunded := 0
	for _, r := range s.refunds[invoiceID] {
		if r.PaymentID == input.PaymentID && (r.Status == "pending" || r.Status == "approved" || r.Status == "processed") {
			alreadyRefunded += r.AmountCents
		}
	}
	if alreadyRefunded+input.AmountCents > payment.AmountCents {
		return nil, nil, ErrRefundExceedsPayment
	}

	now := time.Now().UTC()
	refund := &Refund{
		ID:             s.newID(),
		OrgID:          orgID,
		InvoiceID:      invoiceID,
		PaymentID:      input.PaymentID,
		AmountCents:    input.AmountCents,
		Reason:         input.Reason,
		Status:         "processed", // external refunds are immediately marked processed
		StripeRefundID: input.StripeRefundID,
		ProcessedAt:    &now,
		RequestedBy:    actorID,
		ApprovedBy:     actorID,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      actorID,
		UpdatedBy:      actorID,
	}

	if s.repo != nil {
		if err := s.repo.UpsertRefund(context.Background(), refund); err != nil {
			return nil, nil, fmt.Errorf("persist refund: %w", err)
		}
	}
	s.refunds[invoiceID] = append(s.refunds[invoiceID], refund)
	s.refundsByID[refund.ID] = refund

	// Update payment status if fully refunded.
	totalRefundedForPayment := alreadyRefunded + input.AmountCents
	if totalRefundedForPayment >= payment.AmountCents {
		payment.Status = "reversed"
		payment.UpdatedAt = now
	}

	// Update invoice amount paid and status.
	inv.AmountPaidCents -= input.AmountCents
	if inv.AmountPaidCents < 0 {
		inv.AmountPaidCents = 0
	}
	inv.AmountDueCents = inv.TotalCents - inv.AmountPaidCents
	if inv.AmountDueCents < 0 {
		inv.AmountDueCents = 0
	}

	// Update invoice refund status.
	if inv.AmountPaidCents == 0 {
		inv.Status = "refunded_full"
	} else {
		inv.Status = "refunded_partial"
	}
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	if s.repo != nil {
		if err := s.repo.UpsertInvoice(context.Background(), inv); err != nil {
			return nil, nil, fmt.Errorf("persist invoice after refund: %w", err)
		}
		if err := s.repo.UpsertPayment(context.Background(), payment); err != nil {
			return nil, nil, fmt.Errorf("persist payment after refund: %w", err)
		}
	}
	ev, err := s.appendLedger(orgID, invoiceID, input.PaymentID, refund.ID, "refund_processed", input.AmountCents,
		fmt.Sprintf("Refund of %d cents processed for invoice %s", input.AmountCents, inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return refund, ev, nil
}

// ListRefunds returns all refunds for an invoice.
func (s *Service) ListRefunds(orgID string, principal *appmiddleware.Principal, invoiceID string) ([]*Refund, error) {
	if err := s.requireBillingReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, ErrInvoiceNotFound
	}
	if err := s.ensureParentHouseholdAccess(principal, inv.HouseholdID); err != nil {
		return nil, err
	}

	rfs := s.refunds[invoiceID]
	out := make([]*Refund, len(rfs))
	for i, r := range rfs {
		cp := *r
		out[i] = &cp
	}
	return out, nil
}

// ─── Ledger ───────────────────────────────────────────────────────────────────

// ListLedgerEvents returns ledger events filtered by optional invoice/event type.
func (s *Service) ListLedgerEvents(orgID string, principal *appmiddleware.Principal, filters ListLedgerFilters) ([]*LedgerEvent, error) {
	if err := s.requireLedgerReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*LedgerEvent
	for _, ev := range s.ledger {
		if ev.OrgID != orgID {
			continue
		}
		if filters.InvoiceID != "" && ev.InvoiceID != filters.InvoiceID {
			continue
		}
		if filters.EventType != "" && ev.EventType != filters.EventType {
			continue
		}
		out = append(out, ev)
	}
	// Newest first.
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// LedgerEvents returns a snapshot of all ledger events (for tests).
func (s *Service) LedgerEvents() []*LedgerEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*LedgerEvent, len(s.ledger))
	copy(out, s.ledger)
	return out
}

// SnapshotInvoices returns a copy of all invoices for the given org.
func (s *Service) SnapshotInvoices(orgID string) []Invoice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Invoice, 0, len(s.invoices))
	for _, invoice := range s.invoices {
		if invoice.OrgID != orgID {
			continue
		}
		out = append(out, *invoice)
	}
	return out
}

// SnapshotInvoiceItems returns a copy of all invoice items for the given org.
func (s *Service) SnapshotInvoiceItems(orgID string) []InvoiceItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]InvoiceItem, 0, len(s.itemsByID))
	for _, item := range s.itemsByID {
		if item.OrgID != orgID {
			continue
		}
		out = append(out, *item)
	}
	return out
}

// SnapshotPayments returns a copy of all payments for the given org.
func (s *Service) SnapshotPayments(orgID string) []Payment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Payment, 0, len(s.paymentsByID))
	for _, payment := range s.paymentsByID {
		if payment.OrgID != orgID {
			continue
		}
		out = append(out, *payment)
	}
	return out
}

// SnapshotRefunds returns a copy of all refunds for the given org.
func (s *Service) SnapshotRefunds(orgID string) []Refund {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Refund, 0, len(s.refundsByID))
	for _, refund := range s.refundsByID {
		if refund.OrgID != orgID {
			continue
		}
		out = append(out, *refund)
	}
	return out
}

// SnapshotLedgerEvents returns a copy of all ledger events for the given org.
func (s *Service) SnapshotLedgerEvents(orgID string) []LedgerEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]LedgerEvent, 0, len(s.ledger))
	for _, event := range s.ledger {
		if event.OrgID != orgID {
			continue
		}
		out = append(out, *event)
	}
	return out
}

// ─── Stripe webhook events ────────────────────────────────────────────────────

// webhookKey returns the deduplication key for a webhook event.
func webhookKey(provider, eventID string) string { return provider + "|" + eventID }

// RecordWebhookEvent persists a new webhook event and returns true when the
// event is new (not a duplicate). If the event ID has already been seen for
// this provider, it returns (false, nil) — the caller should skip processing
// and respond 200 OK to Stripe to acknowledge safe receipt.
func (s *Service) RecordWebhookEvent(provider, orgID, eventID, eventType string, payload json.RawMessage) (isNew bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := webhookKey(provider, eventID)
	if _, exists := s.webhookEvents[key]; exists {
		return false, nil
	}

	cp := make(json.RawMessage, len(payload))
	copy(cp, payload)

	s.webhookEvents[key] = &WebhookEventRecord{
		ID:        newID(),
		OrgID:     orgID,
		Provider:  provider,
		EventID:   eventID,
		EventType: eventType,
		Payload:   cp,
		Processed: false,
		CreatedAt: time.Now().UTC(),
	}
	return true, nil
}

// MarkWebhookProcessed updates the processed state of a webhook event.
// If processErr is non-nil, the error message is stored and processed=false.
func (s *Service) MarkWebhookProcessed(provider, eventID string, processErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := webhookKey(provider, eventID)
	rec, ok := s.webhookEvents[key]
	if !ok {
		return
	}
	now := time.Now().UTC()
	if processErr != nil {
		rec.Processed = false
		rec.ErrorMessage = processErr.Error()
	} else {
		rec.Processed = true
		rec.ProcessedAt = &now
		rec.ErrorMessage = ""
	}
}

// ListWebhookEvents returns all webhook event records visible to admin/billing roles.
func (s *Service) ListWebhookEvents(orgID string, principal *appmiddleware.Principal) ([]*WebhookEventRecord, error) {
	if err := s.requireLedgerReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*WebhookEventRecord
	for _, rec := range s.webhookEvents {
		if orgID != "" && rec.OrgID != orgID {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ─── Stripe payment handling ──────────────────────────────────────────────────

// HandleStripePaymentIntentSucceeded records a succeeded Stripe payment against
// the invoice identified by invoiceID. It is idempotent: if a payment with the
// same stripePaymentIntentID already exists, it returns that existing payment.
func (s *Service) HandleStripePaymentIntentSucceeded(
	orgID, paymentIntentID, invoiceID string,
	amountCents int,
	actorID string,
) (*Payment, *LedgerEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency: skip if a payment with this intent ID already exists.
	if existing, ok := s.stripeIntentIndex[paymentIntentID]; ok {
		return existing, nil, nil
	}

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}
	if inv.Status != "issued" && inv.Status != "partially_paid" {
		return nil, nil, ErrInvalidTransition
	}
	if amountCents > inv.AmountDueCents {
		return nil, nil, ErrPaymentExceedsBalance
	}

	now := time.Now().UTC()
	payment := &Payment{
		ID:                    newID(),
		OrgID:                 orgID,
		InvoiceID:             invoiceID,
		AmountCents:           amountCents,
		Method:                "stripe",
		Status:                "succeeded",
		StripePaymentIntentID: paymentIntentID,
		PaidAt:                &now,
		CreatedAt:             now,
		UpdatedAt:             now,
		CreatedBy:             actorID,
		UpdatedBy:             actorID,
	}

	s.payments[invoiceID] = append(s.payments[invoiceID], payment)
	s.paymentsByID[payment.ID] = payment
	s.stripeIntentIndex[paymentIntentID] = payment

	inv.AmountPaidCents += amountCents
	s.updateInvoiceStatusAfterPayment(inv)
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	ev, err := s.appendLedger(orgID, invoiceID, payment.ID, "", "payment_succeeded", -amountCents,
		fmt.Sprintf("Stripe payment %s of %d cents for invoice %s", paymentIntentID, amountCents, inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return payment, ev, nil
}

// HandleStripePaymentIntentFailed marks a previously-recorded pending payment
// as failed. If no payment exists for the given paymentIntentID, this is a
// no-op (the PaymentIntent may have never touched our system).
func (s *Service) HandleStripePaymentIntentFailed(paymentIntentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.stripeIntentIndex[paymentIntentID]
	if !ok {
		// No matching payment — treat as no-op.
		return nil
	}

	now := time.Now().UTC()
	payment.Status = "failed"
	payment.FailedAt = &now
	payment.UpdatedAt = now
	return nil
}

// HandleStripeChargeRefunded processes a charge.refunded webhook: it creates a
// refund on the billing invoice associated with the given paymentIntentID.
// NOTE: This method acquires s.mu.Lock() for the entire operation and therefore
// CANNOT delegate to CreateRefund (which also acquires s.mu.Lock()). The refund
// creation logic is intentionally duplicated here. When migrating to a real DB
// layer, use a shared internal helper that does NOT acquire the lock (lock at
// the service boundary only).
func (s *Service) HandleStripeChargeRefunded(
	orgID, paymentIntentID, invoiceID string,
	amountRefundedCents int,
	stripeRefundID, reason, actorID string,
) (*Refund, *LedgerEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invoices[invoiceID]
	if !ok || inv.OrgID != orgID {
		return nil, nil, ErrInvoiceNotFound
	}

	payment, ok := s.stripeIntentIndex[paymentIntentID]
	if !ok {
		return nil, nil, ErrPaymentNotFound
	}

	validRefundStatuses := map[string]bool{
		"paid": true, "partially_paid": true, "refunded_partial": true,
	}
	if !validRefundStatuses[inv.Status] {
		return nil, nil, ErrInvalidTransition
	}

	// Dedup: if a refund with this stripeRefundID already exists, return it.
	for _, r := range s.refunds[invoiceID] {
		if r.StripeRefundID == stripeRefundID {
			return r, nil, nil
		}
	}

	// Sum existing refunds for this payment.
	alreadyRefunded := 0
	for _, r := range s.refunds[invoiceID] {
		if r.PaymentID == payment.ID && (r.Status == "pending" || r.Status == "approved" || r.Status == "processed") {
			alreadyRefunded += r.AmountCents
		}
	}
	if alreadyRefunded+amountRefundedCents > payment.AmountCents {
		return nil, nil, ErrRefundExceedsPayment
	}

	if reason == "" {
		reason = "Stripe charge.refunded webhook"
	}

	now := time.Now().UTC()
	refund := &Refund{
		ID:             newID(),
		OrgID:          orgID,
		InvoiceID:      invoiceID,
		PaymentID:      payment.ID,
		AmountCents:    amountRefundedCents,
		Reason:         reason,
		Status:         "processed",
		StripeRefundID: stripeRefundID,
		ProcessedAt:    &now,
		RequestedBy:    actorID,
		ApprovedBy:     actorID,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      actorID,
		UpdatedBy:      actorID,
	}

	s.refunds[invoiceID] = append(s.refunds[invoiceID], refund)
	s.refundsByID[refund.ID] = refund

	totalRefundedForPayment := alreadyRefunded + amountRefundedCents
	if totalRefundedForPayment >= payment.AmountCents {
		payment.Status = "reversed"
		payment.UpdatedAt = now
	}

	inv.AmountPaidCents -= amountRefundedCents
	if inv.AmountPaidCents < 0 {
		inv.AmountPaidCents = 0
	}
	inv.AmountDueCents = inv.TotalCents - inv.AmountPaidCents
	if inv.AmountDueCents < 0 {
		inv.AmountDueCents = 0
	}
	if inv.AmountPaidCents == 0 {
		inv.Status = "refunded_full"
	} else {
		inv.Status = "refunded_partial"
	}
	inv.UpdatedAt = now
	inv.UpdatedBy = actorID

	ev, err := s.appendLedger(orgID, invoiceID, payment.ID, refund.ID, "refund_processed", amountRefundedCents,
		fmt.Sprintf("Stripe refund %s of %d cents for invoice %s", stripeRefundID, amountRefundedCents, inv.InvoiceNumber), actorID)
	if err != nil {
		return nil, nil, err
	}

	return refund, ev, nil
}

// ─── Reconciliation ────────────────────────────────────────────────────────────

// GetReconciliationReport computes the difference between the sum of
// payment_succeeded ledger events for Stripe payments and the amount reported
// by the Stripe client. A mismatch above alertThresholdCents sets AlertTriggered.
func (s *Service) GetReconciliationReport(
	orgID string,
	principal *appmiddleware.Principal,
	stripeTotalCents int,
	alertThresholdCents int,
) (*ReconciliationReport, error) {
	if err := s.requireLedgerReadAccess(principal); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Sum ledger events: payment_succeeded events for Stripe method payments.
	ledgerTotal := 0
	for _, ev := range s.ledger {
		if ev.OrgID != orgID || ev.EventType != "payment_succeeded" {
			continue
		}
		if ev.PaymentID == "" {
			continue
		}
		// Check if the payment was made via Stripe.
		if p, ok := s.paymentsByID[ev.PaymentID]; ok && p.Method == "stripe" {
			// Ledger stores payment_succeeded as a negative amount (credit).
			// Use the absolute value for the total.
			ledgerTotal += -ev.AmountCents
		}
	}

	mismatch := ledgerTotal - stripeTotalCents
	if mismatch < 0 {
		mismatch = -mismatch
	}
	if alertThresholdCents <= 0 {
		alertThresholdCents = 100 // default: $1.00
	}

	return &ReconciliationReport{
		LedgerStripeTotalCents: ledgerTotal,
		StripeReportedCents:    stripeTotalCents,
		MismatchCents:          mismatch,
		HasMismatch:            mismatch > 0,
		AlertTriggered:         mismatch > alertThresholdCents,
		AlertThresholdCents:    alertThresholdCents,
		GeneratedAt:            time.Now().UTC(),
	}, nil
}
