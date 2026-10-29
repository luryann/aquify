package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// stripeTransactionLocked serializes mutations for an invoice across instances,
// loads canonical state inside that transaction, and discards staged caches on
// failure. Payment/refund, invoice, credit and ledger writes commit together.
// The caller holds s.mu.
func (s *Service) stripeTransactionLocked(orgID, invoiceID string, mutate func(*Service) error) error {
	// Webhook metadata is caller-controlled text; a malformed ID can never
	// match a row and must not reach the ::uuid cast as a 500 (which Stripe
	// would redeliver).
	if _, err := uuid.Parse(invoiceID); err != nil {
		return ErrInvoiceNotFound
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return ErrInvoiceNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.repo.inTx(ctx, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `SELECT id::text FROM invoices WHERE id = $1::uuid AND org_id = $2::uuid FOR UPDATE`, invoiceID, orgID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvoiceNotFound
		}
		if err != nil {
			return err
		}
		staged := NewService()
		staged.repo = &Repository{pool: s.repo.pool, tx: tx}
		if err := staged.reloadLocked(ctx); err != nil {
			return err
		}
		return mutate(staged)
	})
	if err != nil {
		return err
	}
	return s.reloadLocked(ctx)
}

// stripeTransactionResult runs mutate inside stripeTransactionLocked and
// returns what it produced. Failures return nil results.
func stripeTransactionResult[R any](s *Service, orgID, invoiceID string, mutate func(*Service) (R, *LedgerEvent, error)) (R, *LedgerEvent, error) {
	var result R
	var event *LedgerEvent
	err := s.stripeTransactionLocked(orgID, invoiceID, func(staged *Service) error {
		var err error
		result, event, err = mutate(staged)
		return err
	})
	if err != nil {
		var zero R
		return zero, nil, err
	}
	return result, event, nil
}
