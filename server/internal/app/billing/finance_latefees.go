package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

// LateFeeAssessment is one late fee added to an overdue invoice.
type LateFeeAssessment struct {
	InvoiceID     string `json:"invoice_id"`
	InvoiceNumber string `json:"invoice_number"`
	HouseholdID   string `json:"household_id,omitempty"`
	DueDate       string `json:"due_date"`
	DaysOverdue   int    `json:"days_overdue"`
	FeeCents      int    `json:"fee_cents"`
}

// LateFeeSweepResult summarises one late-fee sweep.
type LateFeeSweepResult struct {
	AsOf         string              `json:"as_of"`
	Enabled      bool                `json:"enabled"`
	AppliedCount int                 `json:"applied_count"`
	TotalCents   int                 `json:"total_cents"`
	Assessments  []LateFeeAssessment `json:"assessments"`
}

var errLateFeeExists = errors.New("late fee already applied")

// errInvoiceStale means the invoice changed in the database after it was loaded.
var errInvoiceStale = errors.New("invoice changed concurrently")

// lockInvoiceFresh row-locks the invoice and verifies its money fields still
// match the in-memory copy, so a full-row upsert cannot overwrite a payment
// committed by another instance.
func lockInvoiceFresh(ctx context.Context, tx pgx.Tx, inv *Invoice) error {
	var status string
	var paid, total, due int
	err := tx.QueryRow(ctx, `SELECT status, amount_paid_cents, total_cents, amount_due_cents FROM invoices WHERE id = $1::uuid AND org_id = $2::uuid FOR UPDATE`, inv.ID, inv.OrgID).Scan(&status, &paid, &total, &due)
	if err != nil {
		return err
	}
	if status != inv.Status || paid != inv.AmountPaidCents || total != inv.TotalCents || due != inv.AmountDueCents {
		return errInvoiceStale
	}
	return nil
}

func lateFeeFor(settings *BillingSettings, amountDue int) int {
	fee := 0
	switch settings.LateFeeType {
	case LateFeeTypeFlat:
		fee = settings.LateFeeFlatCents
	case LateFeeTypePercent:
		fee = roundDiv(amountDue*settings.LateFeePercentBps, 10000)
	}
	if settings.LateFeeMaxCents > 0 && fee > settings.LateFeeMaxCents {
		fee = settings.LateFeeMaxCents
	}
	if fee < 0 {
		fee = 0
	}
	return fee
}

// ApplyLateFees assesses the org's late-fee rule on invoices whose due date
// plus grace days has passed. At most one late fee is ever applied per
// invoice (unique per invoice in the DB), so the sweep is safe to re-run.
func (s *Service) ApplyLateFees(orgID, actorID string, principal *appmiddleware.Principal, asOf time.Time) (*LateFeeSweepResult, error) {
	if err := s.requireAdmin(principal); err != nil {
		return nil, err
	}
	asOf = dateOnly(asOf)
	result := &LateFeeSweepResult{AsOf: fmtDate(asOf), Assessments: []LateFeeAssessment{}}
	ctx := context.Background()
	s.runMu.Lock()
	defer s.runMu.Unlock()
	sweep := func() error {
		if err := s.refreshOrg(ctx, orgID); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		settings := s.settingsLocked(orgID)
		result.Enabled = settings.LateFeeEnabled
		if !settings.LateFeeEnabled {
			return nil
		}
		candidates := make([]*Invoice, 0)
		for _, inv := range s.invoices {
			if inv.OrgID != orgID || (inv.Status != "issued" && inv.Status != "partially_paid") || inv.AmountDueCents <= 0 {
				continue
			}
			if _, applied := s.finance.lateFees[orgID+"|"+inv.ID]; applied {
				continue
			}
			due, err := parseDate(inv.DueDate)
			if err != nil {
				continue
			}
			if !asOf.After(due.AddDate(0, 0, settings.LateFeeGraceDays)) {
				continue
			}
			candidates = append(candidates, inv)
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].InvoiceNumber < candidates[j].InvoiceNumber })
		for _, inv := range candidates {
			fee := lateFeeFor(settings, inv.AmountDueCents)
			if fee <= 0 {
				continue
			}
			due, _ := parseDate(inv.DueDate)
			if err := s.assessLateFeeLocked(ctx, inv, fee, actorID); err != nil {
				if errors.Is(err, errLateFeeExists) {
					continue
				}
				slog.Error("billing late fees: assess failed", "invoice_id", inv.ID, "err", err)
				continue
			}
			result.AppliedCount++
			result.TotalCents += fee
			result.Assessments = append(result.Assessments, LateFeeAssessment{
				InvoiceID: inv.ID, InvoiceNumber: inv.InvoiceNumber, HouseholdID: inv.HouseholdID,
				DueDate: inv.DueDate, DaysOverdue: daysInclusive(due, asOf) - 1, FeeCents: fee,
			})
		}
		return nil
	}
	var err error
	if s.repo != nil {
		err = s.repo.withOrgLock(ctx, "late_fees", orgID, sweep)
	} else {
		err = sweep()
	}
	if err != nil {
		return nil, err
	}
	if result.AppliedCount > 0 {
		slog.Info("billing late fees assessed", "org_id", orgID, "as_of", result.AsOf, "count", result.AppliedCount, "total_cents", result.TotalCents)
	}
	return result, nil
}

func (s *Service) assessLateFeeLocked(ctx context.Context, inv *Invoice, fee int, actorID string) error {
	now := time.Now().UTC()
	item := &InvoiceItem{
		ID: s.newID(), OrgID: inv.OrgID, InvoiceID: inv.ID,
		Description: fmt.Sprintf("Late fee (invoice due %s)", inv.DueDate),
		Quantity:    1, UnitAmountCents: fee, TotalCents: fee, Category: itemCategoryLateFee,
		CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID,
	}
	app := &LateFeeApplication{ID: s.newID(), OrgID: inv.OrgID, InvoiceID: inv.ID, ItemID: item.ID, AmountCents: fee, AppliedAt: now}
	updated := *inv
	updated.SubtotalCents += fee
	updated.TotalCents = updated.SubtotalCents + updated.TaxCents
	updated.AmountDueCents = updated.TotalCents - updated.AmountPaidCents
	updated.UpdatedAt = now
	updated.UpdatedBy = actorID
	ev := s.newLedgerEventLocked(inv.OrgID, inv.ID, "", "", inv.HouseholdID, "late_fee_assessed", fee,
		fmt.Sprintf("Late fee %s assessed on invoice %s", formatCents(fee), inv.InvoiceNumber), actorID)
	if s.repo != nil {
		err := s.repo.inTx(ctx, inv.OrgID, func(tx pgx.Tx) error {
			inserted, err := insertLateFee(ctx, tx, app)
			if err != nil {
				return err
			}
			if !inserted {
				return errLateFeeExists
			}
			if err := upsertInvoiceItem(ctx, tx, item); err != nil {
				return err
			}
			if err := lockInvoiceFresh(ctx, tx, inv); err != nil {
				return err
			}
			if err := upsertInvoice(ctx, tx, &updated); err != nil {
				return err
			}
			return insertLedgerEvent(ctx, tx, ev)
		})
		if err != nil {
			return err
		}
	} else if _, exists := s.finance.lateFees[inv.OrgID+"|"+inv.ID]; exists {
		return errLateFeeExists
	}
	*inv = updated
	s.items[inv.ID] = append(s.items[inv.ID], item)
	s.itemsByID[item.ID] = item
	s.finance.lateFees[inv.OrgID+"|"+inv.ID] = app
	s.ledger = append(s.ledger, ev)
	return nil
}

// RunScheduledLateFees is the worker entry point for the late-fee sweep.
func (s *Service) RunScheduledLateFees(orgID string, today time.Time) (*LateFeeSweepResult, error) {
	principal := systemFinancePrincipal(orgID)
	return s.ApplyLateFees(orgID, principal.UserID, principal, today)
}
