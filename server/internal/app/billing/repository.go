package billing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the billing domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ─── Invoices ─────────────────────────────────────────────────────────────────

func (r *Repository) UpsertInvoice(ctx context.Context, inv *Invoice) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO invoices (id, org_id, household_id, invoice_number, status,
		                      subtotal_cents, total_cents, issued_at, due_date, notes, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  status       = EXCLUDED.status,
		  subtotal_cents = EXCLUDED.subtotal_cents,
		  total_cents  = EXCLUDED.total_cents,
		  issued_at    = EXCLUDED.issued_at,
		  due_date     = EXCLUDED.due_date,
		  notes        = EXCLUDED.notes,
		  updated_at   = EXCLUDED.updated_at`,
		inv.ID, inv.OrgID,
		nullableString(inv.HouseholdID),
		inv.InvoiceNumber, inv.Status,
		inv.SubtotalCents, inv.TotalCents,
		inv.IssuedAt, nullableString(inv.DueDate), nullableString(inv.Notes),
		inv.CreatedAt, inv.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadInvoices(ctx context.Context) ([]*Invoice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, COALESCE(household_id::text, ''),
		       invoice_number, status, subtotal_cents, total_cents,
		       issued_at, COALESCE(due_date, ''), COALESCE(notes, ''),
		       created_at, updated_at
		FROM invoices WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("billing: load invoices: %w", err)
	}
	defer rows.Close()
	var out []*Invoice
	for rows.Next() {
		inv := &Invoice{}
		if err := rows.Scan(
			&inv.ID, &inv.OrgID, &inv.HouseholdID,
			&inv.InvoiceNumber, &inv.Status, &inv.SubtotalCents, &inv.TotalCents,
			&inv.IssuedAt, &inv.DueDate, &inv.Notes,
			&inv.CreatedAt, &inv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("billing: scan invoice: %w", err)
		}
		inv.AmountDueCents = inv.TotalCents
		out = append(out, inv)
	}
	return out, rows.Err()
}

// ─── Invoice Items ────────────────────────────────────────────────────────────

func (r *Repository) UpsertInvoiceItem(ctx context.Context, item *InvoiceItem) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO invoice_items (id, org_id, invoice_id, description, quantity, unit_amount_cents, category, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
		  description      = EXCLUDED.description,
		  quantity         = EXCLUDED.quantity,
		  unit_amount_cents = EXCLUDED.unit_amount_cents,
		  category         = EXCLUDED.category,
		  updated_at       = EXCLUDED.updated_at`,
		item.ID, item.OrgID, item.InvoiceID,
		item.Description, item.Quantity, item.UnitAmountCents, item.Category,
		item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadInvoiceItems(ctx context.Context) ([]*InvoiceItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, invoice_id::text,
		       description, quantity, unit_amount_cents, COALESCE(category, ''),
		       created_at, updated_at
		FROM invoice_items`)
	if err != nil {
		return nil, fmt.Errorf("billing: load invoice_items: %w", err)
	}
	defer rows.Close()
	var out []*InvoiceItem
	for rows.Next() {
		item := &InvoiceItem{}
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.InvoiceID,
			&item.Description, &item.Quantity, &item.UnitAmountCents, &item.Category,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("billing: scan invoice_item: %w", err)
		}
		item.TotalCents = item.Quantity * item.UnitAmountCents
		out = append(out, item)
	}
	return out, rows.Err()
}

// ─── Payments ────────────────────────────────────────────────────────────────

func (r *Repository) UpsertPayment(ctx context.Context, p *Payment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments (id, org_id, invoice_id, household_id, amount_cents, payment_method,
		                      status, stripe_payment_intent_id, notes, paid_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  status                    = EXCLUDED.status,
		  stripe_payment_intent_id  = EXCLUDED.stripe_payment_intent_id,
		  notes                     = EXCLUDED.notes,
		  paid_at                   = EXCLUDED.paid_at,
		  updated_at                = EXCLUDED.updated_at`,
		p.ID, p.OrgID, p.InvoiceID,
		nil, // household_id — not stored on Payment struct
		p.AmountCents, p.Method, p.Status,
		nullableString(p.StripePaymentIntentID), nullableString(p.Notes),
		p.PaidAt, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadPayments(ctx context.Context) ([]*Payment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, invoice_id::text,
		       amount_cents, payment_method, status,
		       COALESCE(stripe_payment_intent_id, ''), COALESCE(notes, ''),
		       paid_at, created_at, updated_at
		FROM payments`)
	if err != nil {
		return nil, fmt.Errorf("billing: load payments: %w", err)
	}
	defer rows.Close()
	var out []*Payment
	for rows.Next() {
		p := &Payment{}
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.InvoiceID,
			&p.AmountCents, &p.Method, &p.Status,
			&p.StripePaymentIntentID, &p.Notes,
			&p.PaidAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("billing: scan payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ─── Refunds ──────────────────────────────────────────────────────────────────

func (r *Repository) UpsertRefund(ctx context.Context, ref *Refund) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refunds (id, org_id, payment_id, amount_cents, reason, status, stripe_refund_id, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
		  status          = EXCLUDED.status,
		  stripe_refund_id = EXCLUDED.stripe_refund_id,
		  updated_at      = EXCLUDED.updated_at`,
		ref.ID, ref.OrgID, ref.PaymentID,
		ref.AmountCents, ref.Reason, ref.Status,
		nullableString(ref.StripeRefundID),
		ref.CreatedAt, ref.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadRefunds(ctx context.Context) ([]*Refund, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, payment_id::text,
		       amount_cents, reason, status, COALESCE(stripe_refund_id, ''),
		       created_at, updated_at
		FROM refunds`)
	if err != nil {
		return nil, fmt.Errorf("billing: load refunds: %w", err)
	}
	defer rows.Close()
	var out []*Refund
	for rows.Next() {
		ref := &Refund{}
		if err := rows.Scan(
			&ref.ID, &ref.OrgID, &ref.PaymentID,
			&ref.AmountCents, &ref.Reason, &ref.Status, &ref.StripeRefundID,
			&ref.CreatedAt, &ref.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("billing: scan refund: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// ─── Ledger Events (append-only INSERT) ──────────────────────────────────────

func (r *Repository) InsertLedgerEvent(ctx context.Context, ev *LedgerEvent) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ledger_events (id, org_id, event_type, reference_id, amount_cents, description, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING`,
		ev.ID, ev.OrgID, ev.EventType,
		nullableString(ev.InvoiceID), // use invoice_id as reference_id
		ev.AmountCents, nullableString(ev.Description),
		ev.CreatedAt,
	)
	return err
}

func (r *Repository) LoadLedgerEvents(ctx context.Context) ([]*LedgerEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, event_type,
		       COALESCE(reference_id::text, ''), amount_cents, COALESCE(description, ''),
		       created_at
		FROM ledger_events ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("billing: load ledger_events: %w", err)
	}
	defer rows.Close()
	var out []*LedgerEvent
	for rows.Next() {
		ev := &LedgerEvent{}
		if err := rows.Scan(
			&ev.ID, &ev.OrgID, &ev.EventType,
			&ev.InvoiceID, &ev.AmountCents, &ev.Description,
			&ev.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("billing: scan ledger_event: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	invoices []*Invoice,
	items []*InvoiceItem,
	payments []*Payment,
	refunds []*Refund,
	ledger []*LedgerEvent,
	err error,
) {
	if invoices, err = r.LoadInvoices(ctx); err != nil {
		return
	}
	if items, err = r.LoadInvoiceItems(ctx); err != nil {
		return
	}
	if payments, err = r.LoadPayments(ctx); err != nil {
		return
	}
	if refunds, err = r.LoadRefunds(ctx); err != nil {
		return
	}
	ledger, err = r.LoadLedgerEvents(ctx)
	return
}
