package billing

import (
	"context"
	"fmt"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbExec is satisfied by both *pgxpool.Pool and pgx.Tx so the same write
// helpers can run standalone or inside a multi-statement transaction.
type dbExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository handles DB persistence for the billing domain.
type Repository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// db routes all reads and writes through the active transaction, when present.
func (r *Repository) db() dbExec {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}

// ─── Invoices ─────────────────────────────────────────────────────────────────

func (r *Repository) UpsertInvoice(ctx context.Context, inv *Invoice) error {
	return upsertInvoice(ctx, r.db(), inv)
}

func upsertInvoice(ctx context.Context, q dbExec, inv *Invoice) error {
	_, err := q.Exec(ctx, `
		INSERT INTO invoices (id, org_id, household_id, invoice_number, status,
		                      subtotal_cents, tax_cents, total_cents, amount_paid_cents, amount_due_cents,
		                      issued_at, voided_at, written_off_at, due_date, notes, external_ref,
		                      created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (id) DO UPDATE SET
		  status            = EXCLUDED.status,
		  subtotal_cents    = EXCLUDED.subtotal_cents,
		  tax_cents         = EXCLUDED.tax_cents,
		  total_cents       = EXCLUDED.total_cents,
		  amount_paid_cents = EXCLUDED.amount_paid_cents,
		  amount_due_cents  = EXCLUDED.amount_due_cents,
		  issued_at         = EXCLUDED.issued_at,
		  voided_at         = EXCLUDED.voided_at,
		  written_off_at    = EXCLUDED.written_off_at,
		  due_date          = EXCLUDED.due_date,
		  notes             = EXCLUDED.notes,
		  external_ref      = EXCLUDED.external_ref,
		  updated_at        = EXCLUDED.updated_at`,
		inv.ID, inv.OrgID,
		dbutil.NullableString(inv.HouseholdID),
		inv.InvoiceNumber, inv.Status,
		inv.SubtotalCents, inv.TaxCents, inv.TotalCents, inv.AmountPaidCents, inv.AmountDueCents,
		inv.IssuedAt, inv.VoidedAt, inv.WrittenOffAt,
		dbutil.NullableString(inv.DueDate), dbutil.NullableString(inv.Notes), dbutil.NullableString(inv.ExternalRef),
		inv.CreatedAt, inv.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadInvoices(ctx context.Context) ([]*Invoice, error) {
	rows, err := r.db().Query(ctx, `
		SELECT id::text, org_id::text, COALESCE(household_id::text, ''),
		       invoice_number, status, subtotal_cents, tax_cents, total_cents,
		       amount_paid_cents, amount_due_cents,
		       issued_at, voided_at, written_off_at,
		       COALESCE(due_date::text, ''), COALESCE(notes, ''), COALESCE(external_ref, ''),
		       created_at, updated_at
		FROM invoices WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("billing: load invoices: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan invoice", func(rows pgx.Rows) (*Invoice, error) {
		inv := &Invoice{}
		err := rows.Scan(
			&inv.ID, &inv.OrgID, &inv.HouseholdID,
			&inv.InvoiceNumber, &inv.Status, &inv.SubtotalCents, &inv.TaxCents, &inv.TotalCents,
			&inv.AmountPaidCents, &inv.AmountDueCents,
			&inv.IssuedAt, &inv.VoidedAt, &inv.WrittenOffAt,
			&inv.DueDate, &inv.Notes, &inv.ExternalRef,
			&inv.CreatedAt, &inv.UpdatedAt,
		)
		return inv, err
	})
}

// ─── Invoice Items ────────────────────────────────────────────────────────────

func (r *Repository) UpsertInvoiceItem(ctx context.Context, item *InvoiceItem) error {
	return upsertInvoiceItem(ctx, r.db(), item)
}

func upsertInvoiceItem(ctx context.Context, q dbExec, item *InvoiceItem) error {
	_, err := q.Exec(ctx, `
		INSERT INTO invoice_items (id, org_id, invoice_id, description, quantity, unit_amount_cents, total_cents, category, swimmer_id, meet_id, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::int, $6::int, $5::int * $6::int, $7, $8::uuid, $9::uuid, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
		  description      = EXCLUDED.description,
		  quantity         = EXCLUDED.quantity,
		  unit_amount_cents = EXCLUDED.unit_amount_cents,
		  total_cents      = EXCLUDED.total_cents,
		  category         = EXCLUDED.category,
		  swimmer_id       = EXCLUDED.swimmer_id,
		  meet_id          = EXCLUDED.meet_id,
		  updated_at       = EXCLUDED.updated_at`,
		item.ID, item.OrgID, item.InvoiceID,
		item.Description, item.Quantity, item.UnitAmountCents, item.Category,
		dbutil.NullableString(item.SwimmerID), dbutil.NullableString(item.MeetID),
		item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (r *Repository) DeleteInvoiceItem(ctx context.Context, itemID string) error {
	_, err := r.db().Exec(ctx, `DELETE FROM invoice_items WHERE id = $1::uuid`, itemID)
	return err
}

func (r *Repository) LoadInvoiceItems(ctx context.Context) ([]*InvoiceItem, error) {
	rows, err := r.db().Query(ctx, `
		SELECT id::text, org_id::text, invoice_id::text,
		       description, quantity, unit_amount_cents, COALESCE(category, ''),
		       COALESCE(swimmer_id::text, ''), COALESCE(meet_id::text, ''),
		       created_at, updated_at
		FROM invoice_items`)
	if err != nil {
		return nil, fmt.Errorf("billing: load invoice_items: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan invoice_item", func(rows pgx.Rows) (*InvoiceItem, error) {
		item := &InvoiceItem{}
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.InvoiceID,
			&item.Description, &item.Quantity, &item.UnitAmountCents, &item.Category,
			&item.SwimmerID, &item.MeetID,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return item, err
		}
		item.TotalCents = item.Quantity * item.UnitAmountCents
		return item, nil
	})
}

// ─── Payments ────────────────────────────────────────────────────────────────

func (r *Repository) UpsertPayment(ctx context.Context, p *Payment) error {
	return upsertPayment(ctx, r.db(), p)
}

func upsertPayment(ctx context.Context, q dbExec, p *Payment) error {
	_, err := q.Exec(ctx, `
		INSERT INTO payments (id, org_id, invoice_id, household_id, amount_cents, payment_method,
		                      status, stripe_payment_intent_id, external_ref, notes, paid_at, failed_at,
		                      created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
		  status                    = EXCLUDED.status,
		  stripe_payment_intent_id  = EXCLUDED.stripe_payment_intent_id,
		  external_ref              = EXCLUDED.external_ref,
		  notes                     = EXCLUDED.notes,
		  paid_at                   = EXCLUDED.paid_at,
		  failed_at                 = EXCLUDED.failed_at,
		  updated_at                = EXCLUDED.updated_at`,
		p.ID, p.OrgID, p.InvoiceID,
		nil, // household_id — derived from the invoice; not stored on Payment
		p.AmountCents, p.Method, p.Status,
		dbutil.NullableString(p.StripePaymentIntentID), dbutil.NullableString(p.ExternalRef), dbutil.NullableString(p.Notes),
		p.PaidAt, p.FailedAt, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadPayments(ctx context.Context) ([]*Payment, error) {
	rows, err := r.db().Query(ctx, `
		SELECT id::text, org_id::text, invoice_id::text,
		       amount_cents, payment_method, status,
		       COALESCE(stripe_payment_intent_id, ''), COALESCE(external_ref, ''), COALESCE(notes, ''),
		       paid_at, failed_at, created_at, updated_at
		FROM payments`)
	if err != nil {
		return nil, fmt.Errorf("billing: load payments: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan payment", func(rows pgx.Rows) (*Payment, error) {
		p := &Payment{}
		err := rows.Scan(
			&p.ID, &p.OrgID, &p.InvoiceID,
			&p.AmountCents, &p.Method, &p.Status,
			&p.StripePaymentIntentID, &p.ExternalRef, &p.Notes,
			&p.PaidAt, &p.FailedAt, &p.CreatedAt, &p.UpdatedAt,
		)
		return p, err
	})
}

// ─── Refunds ──────────────────────────────────────────────────────────────────

func (r *Repository) UpsertRefund(ctx context.Context, ref *Refund) error {
	return upsertRefund(ctx, r.db(), ref)
}

func upsertRefund(ctx context.Context, q dbExec, ref *Refund) error {
	_, err := q.Exec(ctx, `
		INSERT INTO refunds (id, org_id, invoice_id, payment_id, amount_cents, reason, status,
		                     stripe_refund_id, processed_at, rejected_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  status           = EXCLUDED.status,
		  stripe_refund_id = EXCLUDED.stripe_refund_id,
		  processed_at     = EXCLUDED.processed_at,
		  rejected_at      = EXCLUDED.rejected_at,
		  updated_at       = EXCLUDED.updated_at`,
		ref.ID, ref.OrgID, ref.InvoiceID, ref.PaymentID,
		ref.AmountCents, ref.Reason, ref.Status,
		dbutil.NullableString(ref.StripeRefundID), ref.ProcessedAt, ref.RejectedAt,
		ref.CreatedAt, ref.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadRefunds(ctx context.Context) ([]*Refund, error) {
	rows, err := r.db().Query(ctx, `
		SELECT id::text, org_id::text, invoice_id::text, payment_id::text,
		       amount_cents, reason, status, COALESCE(stripe_refund_id, ''),
		       processed_at, rejected_at, created_at, updated_at
		FROM refunds`)
	if err != nil {
		return nil, fmt.Errorf("billing: load refunds: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan refund", func(rows pgx.Rows) (*Refund, error) {
		ref := &Refund{}
		err := rows.Scan(
			&ref.ID, &ref.OrgID, &ref.InvoiceID, &ref.PaymentID,
			&ref.AmountCents, &ref.Reason, &ref.Status, &ref.StripeRefundID,
			&ref.ProcessedAt, &ref.RejectedAt, &ref.CreatedAt, &ref.UpdatedAt,
		)
		return ref, err
	})
}

// ─── Ledger Events (append-only INSERT) ──────────────────────────────────────

func (r *Repository) InsertLedgerEvent(ctx context.Context, ev *LedgerEvent) error {
	return insertLedgerEvent(ctx, r.db(), ev)
}

func insertLedgerEvent(ctx context.Context, q dbExec, ev *LedgerEvent) error {
	_, err := q.Exec(ctx, `
		INSERT INTO ledger_events (id, org_id, event_type, invoice_id, payment_id, refund_id,
		                           household_id, amount_cents, description, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO NOTHING`,
		ev.ID, ev.OrgID, ev.EventType,
		dbutil.NullableString(ev.InvoiceID), dbutil.NullableString(ev.PaymentID), dbutil.NullableString(ev.RefundID),
		dbutil.NullableString(ev.HouseholdID),
		ev.AmountCents, dbutil.NullableString(ev.Description),
		ev.CreatedAt,
	)
	return err
}

func (r *Repository) LoadLedgerEvents(ctx context.Context) ([]*LedgerEvent, error) {
	rows, err := r.db().Query(ctx, `
		SELECT id::text, org_id::text, event_type,
		       COALESCE(invoice_id::text, ''), COALESCE(payment_id::text, ''), COALESCE(refund_id::text, ''),
		       COALESCE(household_id, ''), amount_cents, COALESCE(description, ''),
		       created_at
		FROM ledger_events ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("billing: load ledger_events: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan ledger_event", func(rows pgx.Rows) (*LedgerEvent, error) {
		ev := &LedgerEvent{}
		err := rows.Scan(
			&ev.ID, &ev.OrgID, &ev.EventType,
			&ev.InvoiceID, &ev.PaymentID, &ev.RefundID,
			&ev.HouseholdID, &ev.AmountCents, &ev.Description,
			&ev.CreatedAt,
		)
		return ev, err
	})
}

func (r *Repository) UpsertHouseholdAutopayProfile(ctx context.Context, profile *HouseholdAutopayProfile) error {
	_, err := r.db().Exec(ctx, `
		INSERT INTO household_autopay_profiles (
			id, org_id, household_id, enabled, payment_method_type, payment_method_brand,
			payment_method_last4, payment_method_label, payment_method_external_ref,
			payment_method_exp_month, payment_method_exp_year, trigger_days_before_due,
			created_at, updated_at, updated_by
		)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (org_id, household_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			payment_method_type = EXCLUDED.payment_method_type,
			payment_method_brand = EXCLUDED.payment_method_brand,
			payment_method_last4 = EXCLUDED.payment_method_last4,
			payment_method_label = EXCLUDED.payment_method_label,
			payment_method_external_ref = EXCLUDED.payment_method_external_ref,
			payment_method_exp_month = EXCLUDED.payment_method_exp_month,
			payment_method_exp_year = EXCLUDED.payment_method_exp_year,
			trigger_days_before_due = EXCLUDED.trigger_days_before_due,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by`,
		profile.ID,
		profile.OrgID,
		profile.HouseholdID,
		profile.Enabled,
		profile.PaymentMethodType,
		dbutil.NullableString(profile.PaymentMethodBrand),
		dbutil.NullableString(profile.PaymentMethodLast4),
		profile.PaymentMethodLabel,
		dbutil.NullableString(profile.PaymentMethodExternalRef),
		profile.PaymentMethodExpMonth,
		profile.PaymentMethodExpYear,
		profile.TriggerDaysBeforeDue,
		profile.CreatedAt,
		profile.UpdatedAt,
		dbutil.NullableString(profile.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadHouseholdAutopayProfiles(ctx context.Context) ([]*HouseholdAutopayProfile, error) {
	rows, err := r.db().Query(ctx, `
		SELECT id::text, org_id::text, household_id, enabled, payment_method_type,
		       COALESCE(payment_method_brand, ''), COALESCE(payment_method_last4, ''),
		       payment_method_label, COALESCE(payment_method_external_ref, ''),
		       COALESCE(payment_method_exp_month, 0), COALESCE(payment_method_exp_year, 0),
		       trigger_days_before_due, created_at, updated_at, COALESCE(updated_by, '')
		FROM household_autopay_profiles`)
	if err != nil {
		return nil, fmt.Errorf("billing: load household_autopay_profiles: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan household_autopay_profile", func(rows pgx.Rows) (*HouseholdAutopayProfile, error) {
		profile := &HouseholdAutopayProfile{}
		if err := rows.Scan(
			&profile.ID,
			&profile.OrgID,
			&profile.HouseholdID,
			&profile.Enabled,
			&profile.PaymentMethodType,
			&profile.PaymentMethodBrand,
			&profile.PaymentMethodLast4,
			&profile.PaymentMethodLabel,
			&profile.PaymentMethodExternalRef,
			&profile.PaymentMethodExpMonth,
			&profile.PaymentMethodExpYear,
			&profile.TriggerDaysBeforeDue,
			&profile.CreatedAt,
			&profile.UpdatedAt,
			&profile.UpdatedBy,
		); err != nil {
			return profile, err
		}
		return profile, nil
	})
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	invoices []*Invoice,
	items []*InvoiceItem,
	payments []*Payment,
	refunds []*Refund,
	ledger []*LedgerEvent,
	autopayProfiles []*HouseholdAutopayProfile,
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
	if ledger, err = r.LoadLedgerEvents(ctx); err != nil {
		return
	}
	autopayProfiles, err = r.LoadHouseholdAutopayProfiles(ctx)
	return
}
