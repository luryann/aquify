package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// ManualCreditInput adds (credit) or removes (debit) household account credit.
type ManualCreditInput struct {
	Direction   string `json:"direction" validate:"required,oneof=credit debit"`
	AmountCents int    `json:"amount_cents" validate:"required,gt=0,lte=100000000"`
	Reason      string `json:"reason" validate:"required,max=500"`
	// ApplyToOpenInvoices immediately applies a new credit to the household's
	// open invoices (oldest due first) instead of waiting for the next invoice.
	ApplyToOpenInvoices bool `json:"apply_to_open_invoices"`
}

// HouseholdCreditSummary is the credit balance plus its full entry trail.
type HouseholdCreditSummary struct {
	HouseholdID  string         `json:"household_id"`
	BalanceCents int            `json:"balance_cents"`
	Entries      []*CreditEntry `json:"entries"`
}

type creditEntryParams struct {
	OrgID       string
	HouseholdID string
	AmountCents int // magnitude; the sign is derived from Kind
	Kind        string
	InvoiceID   string
	PaymentID   string
	RefundID    string
	Reason      string
	ActorID     string
}

func creditDelta(kind string, magnitude int) int {
	switch kind {
	case CreditKindManualDebit, CreditKindApplied:
		return -magnitude
	}
	return magnitude
}

// newLedgerEventLocked builds (but does not persist) a ledger event.
func (s *Service) newLedgerEventLocked(orgID, invoiceID, paymentID, refundID, householdID, eventType string, amountCents int, description, actorID string) *LedgerEvent {
	if householdID == "" {
		if inv, ok := s.invoices[invoiceID]; ok {
			householdID = inv.HouseholdID
		}
	}
	return &LedgerEvent{
		ID:          s.newID(),
		OrgID:       orgID,
		InvoiceID:   invoiceID,
		PaymentID:   paymentID,
		RefundID:    refundID,
		HouseholdID: householdID,
		EventType:   eventType,
		AmountCents: amountCents,
		Description: description,
		ActorID:     actorID,
		CreatedAt:   time.Now().UTC(),
	}
}

// addCreditEntryLocked records a credit movement (and its ledger event)
// atomically. Must be called with s.mu held (write).
func (s *Service) addCreditEntryLocked(ctx context.Context, p creditEntryParams) (*CreditEntry, error) {
	if strings.TrimSpace(p.HouseholdID) == "" {
		return nil, ErrHouseholdRequired
	}
	if p.AmountCents <= 0 {
		return nil, invalidField("amount_cents", "must be positive")
	}
	delta := creditDelta(p.Kind, p.AmountCents)
	key := balanceKey(p.OrgID, p.HouseholdID)
	now := time.Now().UTC()
	entry := &CreditEntry{
		ID:          s.newID(),
		OrgID:       p.OrgID,
		HouseholdID: p.HouseholdID,
		AmountCents: delta,
		Kind:        p.Kind,
		InvoiceID:   p.InvoiceID,
		PaymentID:   p.PaymentID,
		RefundID:    p.RefundID,
		Reason:      strings.TrimSpace(p.Reason),
		CreatedAt:   now,
		CreatedBy:   p.ActorID,
	}
	eventType := "credit_issued"
	description := fmt.Sprintf("Account credit %s (%s)", formatCents(p.AmountCents), strings.ReplaceAll(p.Kind, "_", " "))
	if delta < 0 {
		eventType = "credit_debited"
	}
	if p.Kind == CreditKindApplied {
		eventType = "credit_applied"
	}
	// Ledger sign convention: positive = owed by household, negative = credit.
	ledgerAmount := -delta
	if p.Kind == CreditKindApplied {
		// Applying credit settles part of the invoice: the invoice balance
		// falls by the applied amount.
		ledgerAmount = -p.AmountCents
		description = fmt.Sprintf("Account credit %s applied", formatCents(p.AmountCents))
	}
	ev := s.newLedgerEventLocked(p.OrgID, p.InvoiceID, p.PaymentID, p.RefundID, p.HouseholdID, eventType, ledgerAmount, description, p.ActorID)

	if s.repo != nil {
		err := s.repo.inTx(ctx, func(tx pgx.Tx) error {
			balance, err := adjustCreditBalance(ctx, tx, p.OrgID, p.HouseholdID, delta, now)
			if err != nil {
				return err
			}
			entry.BalanceAfterCents = balance
			if err := insertCreditEntry(ctx, tx, entry); err != nil {
				return err
			}
			return insertLedgerEvent(ctx, tx, ev)
		})
		if err != nil {
			return nil, err
		}
	} else {
		balance := s.finance.balances[key] + delta
		if balance < 0 {
			return nil, ErrInsufficientCredit
		}
		entry.BalanceAfterCents = balance
	}
	s.finance.balances[key] = entry.BalanceAfterCents
	s.finance.credits = append(s.finance.credits, entry)
	s.ledger = append(s.ledger, ev)
	return entry, nil
}

// applyHouseholdCreditLocked applies available account credit to an open
// invoice as an account_credit payment. It returns the applied cents.
// Must be called with s.mu held (write).
func (s *Service) applyHouseholdCreditLocked(ctx context.Context, inv *Invoice, actorID string) (int, error) {
	if inv == nil || strings.TrimSpace(inv.HouseholdID) == "" {
		return 0, nil
	}
	if (inv.Status != "issued" && inv.Status != "partially_paid") || inv.AmountDueCents <= 0 {
		return 0, nil
	}
	key := balanceKey(inv.OrgID, inv.HouseholdID)
	available := s.finance.balances[key]
	for attempt := 0; attempt < 2; attempt++ {
		if available <= 0 {
			return 0, nil
		}
		amount := available
		if amount > inv.AmountDueCents {
			amount = inv.AmountDueCents
		}
		applied, err := s.applyCreditAmountLocked(ctx, inv, amount, actorID)
		if errors.Is(err, ErrInsufficientCredit) && s.repo != nil && attempt == 0 {
			// Another instance spent credit; re-read the canonical balance once.
			if err := s.repo.pool.QueryRow(ctx,
				`SELECT COALESCE((SELECT balance_cents FROM household_credit_balances WHERE org_id = $1::uuid AND household_id = $2), 0)`,
				inv.OrgID, inv.HouseholdID).Scan(&available); err != nil {
				return 0, err
			}
			s.finance.balances[key] = available
			continue
		}
		return applied, err
	}
	return 0, nil
}

func (s *Service) applyCreditAmountLocked(ctx context.Context, inv *Invoice, amount int, actorID string) (int, error) {
	now := time.Now().UTC()
	key := balanceKey(inv.OrgID, inv.HouseholdID)
	payment := &Payment{
		ID:          s.newID(),
		OrgID:       inv.OrgID,
		InvoiceID:   inv.ID,
		AmountCents: amount,
		Method:      PaymentMethodAccountCredit,
		Status:      "succeeded",
		PaidAt:      &now,
		Notes:       "Applied from household account credit",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
	}
	updated := *inv
	updated.AmountPaidCents += amount
	s.updateInvoiceStatusAfterPayment(&updated)
	updated.UpdatedAt = now
	updated.UpdatedBy = actorID

	entry := &CreditEntry{
		ID:          s.newID(),
		OrgID:       inv.OrgID,
		HouseholdID: inv.HouseholdID,
		AmountCents: -amount,
		Kind:        CreditKindApplied,
		InvoiceID:   inv.ID,
		PaymentID:   payment.ID,
		Reason:      fmt.Sprintf("Applied to invoice %s", inv.InvoiceNumber),
		CreatedAt:   now,
		CreatedBy:   actorID,
	}
	ev := s.newLedgerEventLocked(inv.OrgID, inv.ID, payment.ID, "", inv.HouseholdID, "credit_applied", -amount,
		fmt.Sprintf("Account credit %s applied to invoice %s", formatCents(amount), inv.InvoiceNumber), actorID)

	if s.repo != nil {
		err := s.repo.inTx(ctx, func(tx pgx.Tx) error {
			balance, err := adjustCreditBalance(ctx, tx, inv.OrgID, inv.HouseholdID, -amount, now)
			if err != nil {
				return err
			}
			entry.BalanceAfterCents = balance
			if err := upsertPayment(ctx, tx, payment); err != nil {
				return err
			}
			if err := upsertInvoice(ctx, tx, &updated); err != nil {
				return err
			}
			if err := insertCreditEntry(ctx, tx, entry); err != nil {
				return err
			}
			return insertLedgerEvent(ctx, tx, ev)
		})
		if err != nil {
			return 0, err
		}
	} else {
		balance := s.finance.balances[key] - amount
		if balance < 0 {
			return 0, ErrInsufficientCredit
		}
		entry.BalanceAfterCents = balance
	}
	*inv = updated
	s.payments[inv.ID] = append(s.payments[inv.ID], payment)
	s.paymentsByID[payment.ID] = payment
	s.finance.balances[key] = entry.BalanceAfterCents
	s.finance.credits = append(s.finance.credits, entry)
	s.ledger = append(s.ledger, ev)
	return amount, nil
}

// refreshLocked re-reads canonical DB state for cross-instance finance reads.
// Must be called with s.mu held (write).
func (s *Service) refreshLocked(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	if err := s.reloadLocked(ctx); err != nil {
		return fmt.Errorf("billing: refresh from DB: %w", err)
	}
	return nil
}

func (s *Service) requireHouseholdFinanceRead(principal *appmiddleware.Principal, householdID string) error {
	if principal == nil {
		return ErrForbidden
	}
	if err := s.requireBillingReadAccess(principal); err != nil {
		return ErrForbidden
	}
	return s.ensureParentHouseholdAccess(principal, householdID)
}

// GetHouseholdCredit returns the household's account credit balance and trail.
func (s *Service) GetHouseholdCredit(orgID string, principal *appmiddleware.Principal, householdID string) (*HouseholdCreditSummary, error) {
	householdID = strings.TrimSpace(householdID)
	if err := s.requireHouseholdFinanceRead(principal, householdID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	return s.creditSummaryLocked(orgID, householdID), nil
}

func (s *Service) creditSummaryLocked(orgID, householdID string) *HouseholdCreditSummary {
	out := &HouseholdCreditSummary{
		HouseholdID:  householdID,
		BalanceCents: s.finance.balances[balanceKey(orgID, householdID)],
		Entries:      []*CreditEntry{},
	}
	for _, e := range s.finance.credits {
		if e.OrgID == orgID && e.HouseholdID == householdID {
			cp := *e
			out.Entries = append(out.Entries, &cp)
		}
	}
	sort.SliceStable(out.Entries, func(i, j int) bool { return out.Entries[i].CreatedAt.After(out.Entries[j].CreatedAt) })
	return out
}

// AddManualCredit adds or removes household account credit (admin only).
// It returns the entry plus the invoices credit was applied to.
func (s *Service) AddManualCredit(orgID, actorID string, principal *appmiddleware.Principal, householdID string, input ManualCreditInput) (*CreditEntry, []string, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, nil, err
	}
	householdID = strings.TrimSpace(householdID)
	if householdID == "" {
		return nil, nil, ErrHouseholdRequired
	}
	if strings.TrimSpace(input.Reason) == "" {
		return nil, nil, invalidField("reason", "is required")
	}
	kind := CreditKindManualCredit
	if input.Direction == "debit" {
		kind = CreditKindManualDebit
	}
	ctx := context.Background()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(ctx); err != nil {
		return nil, nil, err
	}
	entry, err := s.addCreditEntryLocked(ctx, creditEntryParams{
		OrgID: orgID, HouseholdID: householdID, AmountCents: input.AmountCents, Kind: kind,
		Reason: input.Reason, ActorID: actorID,
	})
	if err != nil {
		return nil, nil, err
	}
	var appliedTo []string
	if kind == CreditKindManualCredit && input.ApplyToOpenInvoices {
		open := make([]*Invoice, 0)
		for _, inv := range s.invoices {
			if inv.OrgID == orgID && inv.HouseholdID == householdID &&
				(inv.Status == "issued" || inv.Status == "partially_paid") && inv.AmountDueCents > 0 {
				open = append(open, inv)
			}
		}
		sort.Slice(open, func(i, j int) bool {
			if open[i].DueDate != open[j].DueDate {
				return open[i].DueDate < open[j].DueDate
			}
			return open[i].CreatedAt.Before(open[j].CreatedAt)
		})
		for _, inv := range open {
			applied, err := s.applyHouseholdCreditLocked(ctx, inv, actorID)
			if err != nil {
				slog.Error("billing: apply manual credit to open invoice failed", "invoice_id", inv.ID, "err", err)
				break
			}
			if applied > 0 {
				appliedTo = append(appliedTo, inv.ID)
			}
			if s.finance.balances[balanceKey(orgID, householdID)] <= 0 {
				break
			}
		}
	}
	cp := *entry
	return &cp, appliedTo, nil
}

// RefundToCredit refunds part of a payment into household account credit
// (no money leaves the club). Admin only.
func (s *Service) RefundToCredit(orgID, actorID string, principal *appmiddleware.Principal, invoiceID string, input CreateRefundInput) (*Refund, *LedgerEvent, error) {
	s.mu.RLock()
	inv, ok := s.invoices[invoiceID]
	householdID := ""
	if ok && inv.OrgID == orgID {
		householdID = inv.HouseholdID
	}
	s.mu.RUnlock()
	if !ok {
		if err := s.requireAdmin(principal); err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrInvoiceNotFound
	}
	if strings.TrimSpace(householdID) == "" {
		if err := s.requireAdmin(principal); err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrHouseholdRequired
	}
	input.ToAccountCredit = true
	input.StripeRefundID = ""
	return s.CreateRefund(orgID, actorID, principal, invoiceID, input)
}
