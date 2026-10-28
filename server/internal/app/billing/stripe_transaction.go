package billing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// stripeTransactionLocked serializes mutations for an invoice across instances,
// loads canonical state inside that transaction, and discards staged caches on
// failure. Payment/refund, invoice, credit and ledger writes commit together.
// The caller holds s.mu.
func (s *Service) stripeTransactionLocked(orgID, invoiceID string, mutate func(*Service) error) error {
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
