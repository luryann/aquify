package billing

import (
	"context"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ─── Transactions & locking ──────────────────────────────────────────────────

func (r *Repository) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	if r.tx != nil {
		return fn(r.tx)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("billing: commit tx: %w", err)
	}
	return nil
}

// withOrgLock holds a session-level Postgres advisory lock scoped to
// (purpose, org) for the duration of fn, serialising billing runs and late-fee
// sweeps across API instances and the worker process.
func (r *Repository) withOrgLock(ctx context.Context, purpose, orgID string, fn func() error) error {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("billing: acquire lock conn: %w", err)
	}
	defer conn.Release()
	key := "billing:" + purpose + ":" + orgID
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("billing: advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key)
	}()
	return fn()
}

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}

// isUniqueViolation reports a Postgres unique-constraint violation, optionally
// restricted to one constraint name.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

func nullableDate(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// ─── Load ────────────────────────────────────────────────────────────────────

// loadRows runs a read query and passes each row to scan. Errors are wrapped
// with what names the thing being loaded (for example "fee schedules").
func loadRows(ctx context.Context, q dbExec, what, query string, scan func(pgx.Rows) error) error {
	rows, err := q.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("billing: load %s: %w", what, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("billing: scan %s: %w", what, err)
		}
	}
	return rows.Err()
}

func (r *Repository) loadFinanceState(ctx context.Context) (*financeState, error) {
	f := newFinanceState()
	q := r.db()

	if err := loadRows(ctx, q, "fee schedules", `
		SELECT id::text, org_id::text, name, kind, group_code, season, frequency, amount_cents,
		       start_date::text, COALESCE(end_date::text, ''), proration, auto_bill, active, description,
		       created_at, updated_at, created_by, updated_by
		FROM billing_fee_schedules`, func(rows pgx.Rows) error {
		sc := &FeeSchedule{}
		if err := rows.Scan(&sc.ID, &sc.OrgID, &sc.Name, &sc.Kind, &sc.GroupCode, &sc.Season, &sc.Frequency,
			&sc.AmountCents, &sc.StartDate, &sc.EndDate, &sc.Proration, &sc.AutoBill, &sc.Active, &sc.Description,
			&sc.CreatedAt, &sc.UpdatedAt, &sc.CreatedBy, &sc.UpdatedBy); err != nil {
			return err
		}
		f.schedules[sc.ID] = sc
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "discount rules", `
		SELECT id::text, org_id::text, name, applies_to, min_position, max_position, discount_type,
		       percent_bps, amount_cents, active, created_at, updated_at, created_by, updated_by
		FROM billing_discount_rules`, func(rows pgx.Rows) error {
		rule := &DiscountRule{}
		if err := rows.Scan(&rule.ID, &rule.OrgID, &rule.Name, &rule.AppliesTo, &rule.MinPosition, &rule.MaxPosition,
			&rule.DiscountType, &rule.PercentBps, &rule.AmountCents, &rule.Active,
			&rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy, &rule.UpdatedBy); err != nil {
			return err
		}
		f.rules[rule.ID] = rule
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "adjustments", `
		SELECT id::text, org_id::text, household_id, swimmer_id, amount_cents, reason, status,
		       COALESCE(applied_invoice_id::text, ''), COALESCE(applied_run_id::text, ''),
		       created_at, updated_at, created_by, updated_by
		FROM billing_adjustments`, func(rows pgx.Rows) error {
		a := &Adjustment{}
		if err := rows.Scan(&a.ID, &a.OrgID, &a.HouseholdID, &a.SwimmerID, &a.AmountCents, &a.Reason, &a.Status,
			&a.AppliedInvoiceID, &a.AppliedRunID, &a.CreatedAt, &a.UpdatedAt, &a.CreatedBy, &a.UpdatedBy); err != nil {
			return err
		}
		f.adjustments[a.ID] = a
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "runs", `
		SELECT id::text, org_id::text, as_of::text, invoice_status, COALESCE(due_date::text, ''), schedule_ids,
		       trigger, fingerprint, household_count, invoice_count, charge_count, skipped_count,
		       gross_cents, discount_cents, adjustment_cents, credit_applied_cents, net_cents, created_at, created_by
		FROM billing_runs`, func(rows pgx.Rows) error {
		run := &BillingRun{}
		if err := rows.Scan(&run.ID, &run.OrgID, &run.AsOf, &run.InvoiceStatus, &run.DueDate, &run.ScheduleIDs,
			&run.Trigger, &run.Fingerprint, &run.HouseholdCount, &run.InvoiceCount, &run.ChargeCount, &run.SkippedCount,
			&run.GrossCents, &run.DiscountCents, &run.AdjustmentCents, &run.CreditAppliedCents, &run.NetCents,
			&run.CreatedAt, &run.CreatedBy); err != nil {
			return err
		}
		f.runs[run.ID] = run
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "charges", `
		SELECT id::text, org_id::text, run_id::text, invoice_id::text, household_id, swimmer_id, schedule_id::text,
		       period_key, period_start::text, period_end::text, gross_cents, discount_cents, net_cents,
		       sibling_position, released_at, created_at
		FROM billing_charges ORDER BY created_at, id`, func(rows pgx.Rows) error {
		c := &BillingCharge{}
		if err := rows.Scan(&c.ID, &c.OrgID, &c.RunID, &c.InvoiceID, &c.HouseholdID, &c.SwimmerID, &c.ScheduleID,
			&c.PeriodKey, &c.PeriodStart, &c.PeriodEnd, &c.GrossCents, &c.DiscountCents, &c.NetCents,
			&c.SiblingPosition, &c.ReleasedAt, &c.CreatedAt); err != nil {
			return err
		}
		f.indexCharge(c)
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "credit entries", `
		SELECT id::text, org_id::text, household_id, amount_cents, kind, COALESCE(invoice_id::text, ''),
		       COALESCE(payment_id::text, ''), COALESCE(refund_id::text, ''), reason, stripe_ref, balance_after_cents,
		       created_at, created_by
		FROM household_credit_entries ORDER BY created_at, id`, func(rows pgx.Rows) error {
		e := &CreditEntry{}
		if err := rows.Scan(&e.ID, &e.OrgID, &e.HouseholdID, &e.AmountCents, &e.Kind, &e.InvoiceID, &e.PaymentID,
			&e.RefundID, &e.Reason, &e.StripeRef, &e.BalanceAfterCents, &e.CreatedAt, &e.CreatedBy); err != nil {
			return err
		}
		f.credits = append(f.credits, e)
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "credit balances", `SELECT org_id::text, household_id, balance_cents FROM household_credit_balances`, func(rows pgx.Rows) error {
		var orgID, householdID string
		var balance int
		if err := rows.Scan(&orgID, &householdID, &balance); err != nil {
			return err
		}
		f.balances[balanceKey(orgID, householdID)] = balance
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "settings", `
		SELECT org_id::text, late_fee_enabled, late_fee_grace_days, late_fee_type, late_fee_flat_cents,
		       late_fee_percent_bps, late_fee_max_cents, auto_run_enabled, auto_run_day,
		       auto_run_invoice_status, auto_run_due_days, updated_at, updated_by
		FROM billing_settings`, func(rows pgx.Rows) error {
		st := &BillingSettings{}
		if err := rows.Scan(&st.OrgID, &st.LateFeeEnabled, &st.LateFeeGraceDays, &st.LateFeeType, &st.LateFeeFlatCents,
			&st.LateFeePercentBps, &st.LateFeeMaxCents, &st.AutoRunEnabled, &st.AutoRunDay,
			&st.AutoRunInvoiceStatus, &st.AutoRunDueDays, &st.UpdatedAt, &st.UpdatedBy); err != nil {
			return err
		}
		f.settings[st.OrgID] = st
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadRows(ctx, q, "late fees", `
		SELECT id::text, org_id::text, invoice_id::text, item_id::text, amount_cents, applied_at
		FROM billing_late_fee_applications`, func(rows pgx.Rows) error {
		app := &LateFeeApplication{}
		if err := rows.Scan(&app.ID, &app.OrgID, &app.InvoiceID, &app.ItemID, &app.AmountCents, &app.AppliedAt); err != nil {
			return err
		}
		f.lateFees[app.OrgID+"|"+app.InvoiceID] = app
		return nil
	}); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *Repository) upsertFeeSchedule(ctx context.Context, sc *FeeSchedule) error {
	_, err := r.db().Exec(ctx, `
		INSERT INTO billing_fee_schedules (id, org_id, name, kind, group_code, season, frequency, amount_cents,
		       start_date, end_date, proration, auto_bill, active, description, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9::date, $10::date, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (id) DO UPDATE SET
		  name = EXCLUDED.name, kind = EXCLUDED.kind, group_code = EXCLUDED.group_code, season = EXCLUDED.season,
		  frequency = EXCLUDED.frequency, amount_cents = EXCLUDED.amount_cents, start_date = EXCLUDED.start_date,
		  end_date = EXCLUDED.end_date, proration = EXCLUDED.proration, auto_bill = EXCLUDED.auto_bill,
		  active = EXCLUDED.active, description = EXCLUDED.description, updated_at = EXCLUDED.updated_at,
		  updated_by = EXCLUDED.updated_by`,
		sc.ID, sc.OrgID, sc.Name, sc.Kind, sc.GroupCode, sc.Season, sc.Frequency, sc.AmountCents,
		sc.StartDate, nullableDate(sc.EndDate), sc.Proration, sc.AutoBill, sc.Active, sc.Description,
		sc.CreatedAt, sc.UpdatedAt, sc.CreatedBy, sc.UpdatedBy)
	return err
}

func (r *Repository) upsertDiscountRule(ctx context.Context, rule *DiscountRule) error {
	_, err := r.db().Exec(ctx, `
		INSERT INTO billing_discount_rules (id, org_id, name, applies_to, min_position, max_position, discount_type,
		       percent_bps, amount_cents, active, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
		  name = EXCLUDED.name, applies_to = EXCLUDED.applies_to, min_position = EXCLUDED.min_position,
		  max_position = EXCLUDED.max_position, discount_type = EXCLUDED.discount_type,
		  percent_bps = EXCLUDED.percent_bps, amount_cents = EXCLUDED.amount_cents, active = EXCLUDED.active,
		  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
		rule.ID, rule.OrgID, rule.Name, rule.AppliesTo, rule.MinPosition, rule.MaxPosition, rule.DiscountType,
		rule.PercentBps, rule.AmountCents, rule.Active, rule.CreatedAt, rule.UpdatedAt, rule.CreatedBy, rule.UpdatedBy)
	return err
}

func upsertAdjustment(ctx context.Context, q dbExec, a *Adjustment) error {
	_, err := q.Exec(ctx, `
		INSERT INTO billing_adjustments (id, org_id, household_id, swimmer_id, amount_cents, reason, status,
		       applied_invoice_id, applied_run_id, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8::uuid, $9::uuid, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
		  status = EXCLUDED.status, applied_invoice_id = EXCLUDED.applied_invoice_id,
		  applied_run_id = EXCLUDED.applied_run_id, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
		a.ID, a.OrgID, a.HouseholdID, a.SwimmerID, a.AmountCents, a.Reason, a.Status,
		dbutil.NullableString(a.AppliedInvoiceID), dbutil.NullableString(a.AppliedRunID),
		a.CreatedAt, a.UpdatedAt, a.CreatedBy, a.UpdatedBy)
	return err
}

// claimAdjustment flips a pending adjustment to applied; it reports false when
// another writer already consumed it.
func claimAdjustment(ctx context.Context, q dbExec, a *Adjustment) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE billing_adjustments
		SET status = 'applied', applied_invoice_id = $2::uuid, applied_run_id = $3::uuid, updated_at = $4, updated_by = $5
		WHERE id = $1::uuid AND status = 'pending'`,
		a.ID, a.AppliedInvoiceID, a.AppliedRunID, a.UpdatedAt, a.UpdatedBy)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) upsertSettings(ctx context.Context, st *BillingSettings) error {
	_, err := r.db().Exec(ctx, `
		INSERT INTO billing_settings (org_id, late_fee_enabled, late_fee_grace_days, late_fee_type, late_fee_flat_cents,
		       late_fee_percent_bps, late_fee_max_cents, auto_run_enabled, auto_run_day, auto_run_invoice_status,
		       auto_run_due_days, updated_at, updated_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (org_id) DO UPDATE SET
		  late_fee_enabled = EXCLUDED.late_fee_enabled, late_fee_grace_days = EXCLUDED.late_fee_grace_days,
		  late_fee_type = EXCLUDED.late_fee_type, late_fee_flat_cents = EXCLUDED.late_fee_flat_cents,
		  late_fee_percent_bps = EXCLUDED.late_fee_percent_bps, late_fee_max_cents = EXCLUDED.late_fee_max_cents,
		  auto_run_enabled = EXCLUDED.auto_run_enabled, auto_run_day = EXCLUDED.auto_run_day,
		  auto_run_invoice_status = EXCLUDED.auto_run_invoice_status, auto_run_due_days = EXCLUDED.auto_run_due_days,
		  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
		st.OrgID, st.LateFeeEnabled, st.LateFeeGraceDays, st.LateFeeType, st.LateFeeFlatCents,
		st.LateFeePercentBps, st.LateFeeMaxCents, st.AutoRunEnabled, st.AutoRunDay, st.AutoRunInvoiceStatus,
		st.AutoRunDueDays, st.UpdatedAt, st.UpdatedBy)
	return err
}

// ─── Runs & charges ──────────────────────────────────────────────────────────

func upsertRun(ctx context.Context, q dbExec, run *BillingRun) error {
	ids := run.ScheduleIDs
	if ids == nil {
		ids = []string{}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO billing_runs (id, org_id, as_of, invoice_status, due_date, schedule_ids, trigger, fingerprint,
		       household_count, invoice_count, charge_count, skipped_count, gross_cents, discount_cents,
		       adjustment_cents, credit_applied_cents, net_cents, created_at, created_by)
		VALUES ($1::uuid, $2::uuid, $3::date, $4, $5::date, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (id) DO UPDATE SET
		  household_count = EXCLUDED.household_count, invoice_count = EXCLUDED.invoice_count,
		  charge_count = EXCLUDED.charge_count, skipped_count = EXCLUDED.skipped_count,
		  gross_cents = EXCLUDED.gross_cents, discount_cents = EXCLUDED.discount_cents,
		  adjustment_cents = EXCLUDED.adjustment_cents, credit_applied_cents = EXCLUDED.credit_applied_cents,
		  net_cents = EXCLUDED.net_cents`,
		run.ID, run.OrgID, run.AsOf, run.InvoiceStatus, nullableDate(run.DueDate), ids, run.Trigger, run.Fingerprint,
		run.HouseholdCount, run.InvoiceCount, run.ChargeCount, run.SkippedCount, run.GrossCents, run.DiscountCents,
		run.AdjustmentCents, run.CreditAppliedCents, run.NetCents, run.CreatedAt, run.CreatedBy)
	return err
}

// insertCharge inserts an idempotency charge row; it reports false when an
// active charge with the same key already exists (another writer billed it).
func insertCharge(ctx context.Context, q dbExec, c *BillingCharge) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO billing_charges (id, org_id, run_id, invoice_id, household_id, swimmer_id, schedule_id, period_key,
		       period_start, period_end, gross_cents, discount_cents, net_cents, sibling_position, created_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7::uuid, $8, $9::date, $10::date, $11, $12, $13, $14, $15)
		ON CONFLICT (org_id, schedule_id, period_key, household_id, swimmer_id) WHERE released_at IS NULL DO NOTHING`,
		c.ID, c.OrgID, c.RunID, c.InvoiceID, c.HouseholdID, c.SwimmerID, c.ScheduleID, c.PeriodKey,
		c.PeriodStart, c.PeriodEnd, c.GrossCents, c.DiscountCents, c.NetCents, c.SiblingPosition, c.CreatedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func releaseCharges(ctx context.Context, q dbExec, invoiceID string, at time.Time) error {
	_, err := q.Exec(ctx, `UPDATE billing_charges SET released_at = $2 WHERE invoice_id = $1::uuid AND released_at IS NULL`, invoiceID, at)
	return err
}

func reopenAdjustments(ctx context.Context, q dbExec, invoiceID string, at time.Time, actorID string) error {
	_, err := q.Exec(ctx, `
		UPDATE billing_adjustments SET status = 'pending', applied_invoice_id = NULL, applied_run_id = NULL,
		       updated_at = $2, updated_by = $3
		WHERE applied_invoice_id = $1::uuid AND status = 'applied'`, invoiceID, at, actorID)
	return err
}

// ─── Credits & late fees ─────────────────────────────────────────────────────

// adjustCreditBalance atomically moves the household balance by delta and
// returns the new balance. The table's CHECK keeps the balance non-negative,
// so a concurrent over-spend fails with ErrInsufficientCredit.
func adjustCreditBalance(ctx context.Context, q dbExec, orgID, householdID string, delta int, at time.Time) (int, error) {
	var balance int
	if delta < 0 {
		// The balance CHECK would reject a negative insert tuple before ON
		// CONFLICT is considered, so debits are a plain conditional UPDATE.
		err := q.QueryRow(ctx, `
			UPDATE household_credit_balances SET balance_cents = balance_cents + $3, updated_at = $4
			WHERE org_id = $1::uuid AND household_id = $2
			RETURNING balance_cents`, orgID, householdID, delta, at).Scan(&balance)
		if errors.Is(err, pgx.ErrNoRows) || isCheckViolation(err) {
			return 0, ErrInsufficientCredit
		}
		return balance, err
	}
	err := q.QueryRow(ctx, `
		INSERT INTO household_credit_balances (org_id, household_id, balance_cents, updated_at)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (org_id, household_id) DO UPDATE
		  SET balance_cents = household_credit_balances.balance_cents + EXCLUDED.balance_cents,
		      updated_at = EXCLUDED.updated_at
		RETURNING balance_cents`, orgID, householdID, delta, at).Scan(&balance)
	if err != nil {
		if isCheckViolation(err) {
			return 0, ErrInsufficientCredit
		}
		return 0, err
	}
	return balance, nil
}

func insertCreditEntry(ctx context.Context, q dbExec, e *CreditEntry) error {
	_, err := q.Exec(ctx, `
		INSERT INTO household_credit_entries (id, org_id, household_id, amount_cents, kind, invoice_id, payment_id,
		       refund_id, reason, stripe_ref, balance_after_cents, created_at, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::uuid, $7::uuid, $8::uuid, $9, $10, $11, $12, $13)`,
		e.ID, e.OrgID, e.HouseholdID, e.AmountCents, e.Kind, dbutil.NullableString(e.InvoiceID), dbutil.NullableString(e.PaymentID),
		dbutil.NullableString(e.RefundID), e.Reason, e.StripeRef, e.BalanceAfterCents, e.CreatedAt, e.CreatedBy)
	return err
}

// insertLateFee records the per-invoice late fee; false means one exists.
func insertLateFee(ctx context.Context, q dbExec, app *LateFeeApplication) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO billing_late_fee_applications (id, org_id, invoice_id, item_id, amount_cents, applied_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6)
		ON CONFLICT (org_id, invoice_id) DO NOTHING`,
		app.ID, app.OrgID, app.InvoiceID, app.ItemID, app.AmountCents, app.AppliedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
