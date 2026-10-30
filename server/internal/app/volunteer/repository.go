package volunteer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the Postgres-backed store. Postgres is canonical: every call
// reads or writes the database directly so all API instances agree.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func isUUID(s string) bool {
	_, err := uuid.Parse(strings.TrimSpace(s))
	return err == nil
}

func nullable(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ─── Settings ─────────────────────────────────────────────────────────────────

func (r *Repository) getSettings(ctx context.Context, orgID string) (*Settings, bool, error) {
	st := &Settings{OrgID: orgID}
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT default_required_hours::float8, default_buyout_cents_per_hour, cancellation_cutoff_hours,
		       reminders_enabled, timezone, updated_at, updated_by
		FROM volunteer_settings WHERE org_id = $1`, orgID).
		Scan(&st.DefaultRequiredHours, &st.DefaultBuyoutCentsPerHour, &st.CancellationCutoffHours,
			&st.RemindersEnabled, &st.Timezone, &updatedAt, &st.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("volunteer: get settings: %w", err)
	}
	st.UpdatedAt = &updatedAt
	return st, true, nil
}

func (r *Repository) upsertSettings(ctx context.Context, st *Settings) error {
	updatedAt := time.Now().UTC()
	if st.UpdatedAt != nil {
		updatedAt = *st.UpdatedAt
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO volunteer_settings (org_id, default_required_hours, default_buyout_cents_per_hour,
			cancellation_cutoff_hours, reminders_enabled, timezone, updated_at, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (org_id) DO UPDATE SET
			default_required_hours = EXCLUDED.default_required_hours,
			default_buyout_cents_per_hour = EXCLUDED.default_buyout_cents_per_hour,
			cancellation_cutoff_hours = EXCLUDED.cancellation_cutoff_hours,
			reminders_enabled = EXCLUDED.reminders_enabled,
			timezone = EXCLUDED.timezone,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by`,
		st.OrgID, st.DefaultRequiredHours, st.DefaultBuyoutCentsPerHour, st.CancellationCutoffHours,
		st.RemindersEnabled, st.Timezone, updatedAt, st.UpdatedBy)
	if err != nil {
		return fmt.Errorf("volunteer: upsert settings: %w", err)
	}
	return nil
}

// ─── Jobs ─────────────────────────────────────────────────────────────────────

const jobColumns = `id::text, org_id, COALESCE(meet_id, ''), event_name, COALESCE(season_id::text, ''),
	role, title, description, location, starts_at, ends_at, capacity, hour_credit::float8,
	COALESCE(min_age, 0), requires_certification, created_at, updated_at, created_by, updated_by`

func scanJob(row pgx.Row) (*Job, error) {
	j := &Job{}
	err := row.Scan(&j.ID, &j.OrgID, &j.MeetID, &j.EventName, &j.SeasonID,
		&j.Role, &j.Title, &j.Description, &j.Location, &j.StartsAt, &j.EndsAt, &j.Capacity, &j.HourCredit,
		&j.MinAge, &j.RequiresCertification, &j.CreatedAt, &j.UpdatedAt, &j.CreatedBy, &j.UpdatedBy)
	if err != nil {
		return nil, err
	}
	j.StartsAt, j.EndsAt = j.StartsAt.UTC(), j.EndsAt.UTC()
	return j, nil
}

func (r *Repository) listJobs(ctx context.Context, orgID, meetID string) ([]*Job, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+jobColumns+` FROM volunteer_jobs
		WHERE org_id = $1 AND ($2 = '' OR meet_id = $2)
		ORDER BY starts_at, title, id`, orgID, meetID)
	if err != nil {
		return nil, fmt.Errorf("volunteer: list jobs: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "volunteer: scan job", func(rows pgx.Rows) (*Job, error) { return scanJob(rows) })
}

func (r *Repository) getJob(ctx context.Context, orgID, id string) (*Job, error) {
	if !isUUID(id) {
		return nil, ErrJobNotFound
	}
	j, err := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM volunteer_jobs WHERE id = $1::uuid AND org_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("volunteer: get job: %w", err)
	}
	return j, nil
}

func minAgeArg(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func (r *Repository) insertJobs(ctx context.Context, jobs []*Job) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("volunteer: begin insert jobs: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, j := range jobs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO volunteer_jobs (id, org_id, meet_id, event_name, season_id, role, title, description, location,
				starts_at, ends_at, capacity, hour_credit, min_age, requires_certification,
				created_at, updated_at, created_by, updated_by)
			VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			j.ID, j.OrgID, nullable(j.MeetID), j.EventName, nullable(j.SeasonID), j.Role, j.Title, j.Description, j.Location,
			j.StartsAt, j.EndsAt, j.Capacity, j.HourCredit, minAgeArg(j.MinAge), j.RequiresCertification,
			j.CreatedAt, j.UpdatedAt, j.CreatedBy, j.UpdatedBy); err != nil {
			return fmt.Errorf("volunteer: insert job: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("volunteer: commit jobs: %w", err)
	}
	return nil
}

func (r *Repository) updateJob(ctx context.Context, j *Job) error {
	if !isUUID(j.ID) {
		return ErrJobNotFound
	}
	// Lock the job row (same pattern as insertSignup) so the capacity check
	// cannot race a concurrent signup.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("volunteer: begin job update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked string
	err = tx.QueryRow(ctx, `SELECT id::text FROM volunteer_jobs WHERE id = $1::uuid AND org_id = $2 FOR UPDATE`,
		j.ID, j.OrgID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJobNotFound
	}
	if err != nil {
		return fmt.Errorf("volunteer: lock job: %w", err)
	}
	var filled int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM volunteer_signups
		WHERE job_id = $1::uuid AND status IN ('confirmed','worked','no_show')`, j.ID).Scan(&filled); err != nil {
		return fmt.Errorf("volunteer: count signups: %w", err)
	}
	if j.Capacity < filled {
		return ErrCapacityBelowFilled
	}
	tag, err := tx.Exec(ctx, `
		UPDATE volunteer_jobs SET event_name = $3, season_id = $4::uuid, role = $5, title = $6, description = $7,
			location = $8, starts_at = $9, ends_at = $10, capacity = $11, hour_credit = $12, min_age = $13,
			requires_certification = $14, updated_at = $15, updated_by = $16
		WHERE id = $1::uuid AND org_id = $2`,
		j.ID, j.OrgID, j.EventName, nullable(j.SeasonID), j.Role, j.Title, j.Description,
		j.Location, j.StartsAt, j.EndsAt, j.Capacity, j.HourCredit, minAgeArg(j.MinAge),
		j.RequiresCertification, j.UpdatedAt, j.UpdatedBy)
	if err != nil {
		return fmt.Errorf("volunteer: update job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("volunteer: commit job update: %w", err)
	}
	return nil
}

func (r *Repository) deleteJob(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return ErrJobNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM volunteer_jobs WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("volunteer: delete job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotFound
	}
	return nil
}

// ─── Signups ──────────────────────────────────────────────────────────────────

const signupColumns = `id::text, org_id, job_id::text, household_id, volunteer_name, COALESCE(swimmer_id, ''),
	status, hours_credited::float8, notes, signed_up_by, cancelled_at, cancelled_by, checked_in_at, checked_in_by,
	created_at, updated_at`

func scanSignup(row pgx.Row) (*Signup, error) {
	su := &Signup{}
	err := row.Scan(&su.ID, &su.OrgID, &su.JobID, &su.HouseholdID, &su.VolunteerName, &su.SwimmerID,
		&su.Status, &su.HoursCredited, &su.Notes, &su.SignedUpBy, &su.CancelledAt, &su.CancelledBy,
		&su.CheckedInAt, &su.CheckedInBy, &su.CreatedAt, &su.UpdatedAt)
	return su, err
}

func (r *Repository) listSignups(ctx context.Context, orgID string, f SignupFilter) ([]*Signup, error) {
	if f.JobID != "" && !isUUID(f.JobID) {
		return []*Signup{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+signupColumns+` FROM volunteer_signups
		WHERE org_id = $1
		  AND ($2 = '' OR job_id = NULLIF($2, '')::uuid)
		  AND ($3 = '' OR household_id = $3)
		  AND ($4 = '' OR status = $4)
		ORDER BY created_at, id`, orgID, f.JobID, f.HouseholdID, f.Status)
	if err != nil {
		return nil, fmt.Errorf("volunteer: list signups: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "volunteer: scan signup", func(rows pgx.Rows) (*Signup, error) { return scanSignup(rows) })
}

func (r *Repository) getSignup(ctx context.Context, orgID, id string) (*Signup, error) {
	if !isUUID(id) {
		return nil, ErrSignupNotFound
	}
	su, err := scanSignup(r.pool.QueryRow(ctx, `SELECT `+signupColumns+` FROM volunteer_signups WHERE id = $1::uuid AND org_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSignupNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("volunteer: get signup: %w", err)
	}
	return su, nil
}

// insertSignup locks the job row so concurrent signups for the same job are
// serialised; the capacity check and insert then happen in one transaction.
func (r *Repository) insertSignup(ctx context.Context, su *Signup) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("volunteer: begin signup: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var capacity int
	err = tx.QueryRow(ctx, `SELECT capacity FROM volunteer_jobs WHERE id = $1::uuid AND org_id = $2 FOR UPDATE`,
		su.JobID, su.OrgID).Scan(&capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJobNotFound
	}
	if err != nil {
		return fmt.Errorf("volunteer: lock job: %w", err)
	}
	var filled, dup int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE household_id = $2 AND lower(volunteer_name) = lower($3))
		FROM volunteer_signups
		WHERE job_id = $1::uuid AND status IN ('confirmed','worked','no_show')`,
		su.JobID, su.HouseholdID, su.VolunteerName).Scan(&filled, &dup); err != nil {
		return fmt.Errorf("volunteer: count signups: %w", err)
	}
	if dup > 0 {
		return ErrDuplicateSignup
	}
	if filled >= capacity {
		return ErrJobFull
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO volunteer_signups (id, org_id, job_id, household_id, volunteer_name, swimmer_id, status,
			hours_credited, notes, signed_up_by, created_at, updated_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		su.ID, su.OrgID, su.JobID, su.HouseholdID, su.VolunteerName, nullable(su.SwimmerID), su.Status,
		su.HoursCredited, su.Notes, su.SignedUpBy, su.CreatedAt, su.UpdatedAt); err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateSignup
		}
		return fmt.Errorf("volunteer: insert signup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("volunteer: commit signup: %w", err)
	}
	return nil
}

func (r *Repository) updateSignup(ctx context.Context, su *Signup, expectedStatus string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE volunteer_signups SET status = $3, hours_credited = $4, notes = $5, cancelled_at = $6,
			cancelled_by = $7, checked_in_at = $8, checked_in_by = $9, updated_at = $10
		WHERE id = $1::uuid AND org_id = $2 AND status = $11`,
		su.ID, su.OrgID, su.Status, su.HoursCredited, su.Notes, su.CancelledAt,
		su.CancelledBy, su.CheckedInAt, su.CheckedInBy, su.UpdatedAt, expectedStatus)
	if err != nil {
		return fmt.Errorf("volunteer: update signup: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either the row is gone or a concurrent writer already moved it.
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM volunteer_signups WHERE id = $1::uuid AND org_id = $2)`,
			su.ID, su.OrgID).Scan(&exists); err != nil {
			return fmt.Errorf("volunteer: recheck signup: %w", err)
		}
		if !exists {
			return ErrSignupNotFound
		}
		return ErrInvalidTransition
	}
	return nil
}

// ─── Requirements ─────────────────────────────────────────────────────────────

const requirementColumns = `id::text, org_id, season_id::text, COALESCE(household_id, ''), required_hours::float8,
	buyout_cents_per_hour, notes, updated_at, updated_by`

func scanRequirement(row pgx.Row) (*Requirement, error) {
	q := &Requirement{}
	err := row.Scan(&q.ID, &q.OrgID, &q.SeasonID, &q.HouseholdID, &q.RequiredHours,
		&q.BuyoutCentsPerHour, &q.Notes, &q.UpdatedAt, &q.UpdatedBy)
	return q, err
}

func (r *Repository) listRequirements(ctx context.Context, orgID, seasonID string) ([]*Requirement, error) {
	if seasonID != "" && !isUUID(seasonID) {
		return []*Requirement{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+requirementColumns+` FROM volunteer_requirements
		WHERE org_id = $1 AND ($2 = '' OR season_id = NULLIF($2, '')::uuid)
		ORDER BY season_id, household_id NULLS FIRST`, orgID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("volunteer: list requirements: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "volunteer: scan requirement", func(rows pgx.Rows) (*Requirement, error) { return scanRequirement(rows) })
}

func (r *Repository) upsertRequirement(ctx context.Context, q *Requirement) (*Requirement, error) {
	if !isUUID(q.SeasonID) {
		return nil, ErrSeasonNotFound
	}
	out, err := scanRequirement(r.pool.QueryRow(ctx, `
		INSERT INTO volunteer_requirements (id, org_id, season_id, household_id, required_hours,
			buyout_cents_per_hour, notes, updated_at, updated_by)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (org_id, season_id, household_id) DO UPDATE SET
			required_hours = EXCLUDED.required_hours,
			buyout_cents_per_hour = EXCLUDED.buyout_cents_per_hour,
			notes = EXCLUDED.notes,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by
		RETURNING `+requirementColumns,
		q.ID, q.OrgID, q.SeasonID, nullable(q.HouseholdID), q.RequiredHours,
		q.BuyoutCentsPerHour, q.Notes, q.UpdatedAt, q.UpdatedBy))
	if err != nil {
		return nil, fmt.Errorf("volunteer: upsert requirement: %w", err)
	}
	return out, nil
}

func (r *Repository) deleteRequirement(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return ErrRequirementNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM volunteer_requirements WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("volunteer: delete requirement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRequirementNotFound
	}
	return nil
}

// ─── Credits ──────────────────────────────────────────────────────────────────

func (r *Repository) listCredits(ctx context.Context, orgID, seasonID, householdID string) ([]*Credit, error) {
	if seasonID != "" && !isUUID(seasonID) {
		return []*Credit{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, season_id::text, household_id, hours::float8, reason, created_by, created_at
		FROM volunteer_credits
		WHERE org_id = $1 AND ($2 = '' OR season_id = NULLIF($2, '')::uuid) AND ($3 = '' OR household_id = $3)
		ORDER BY created_at, id`, orgID, seasonID, householdID)
	if err != nil {
		return nil, fmt.Errorf("volunteer: list credits: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "volunteer: scan credit", func(rows pgx.Rows) (*Credit, error) {
		c := &Credit{}
		err := rows.Scan(&c.ID, &c.OrgID, &c.SeasonID, &c.HouseholdID, &c.Hours, &c.Reason, &c.CreatedBy, &c.CreatedAt)
		return c, err
	})
}

func (r *Repository) insertCredit(ctx context.Context, c *Credit) error {
	if !isUUID(c.SeasonID) {
		return ErrSeasonNotFound
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO volunteer_credits (id, org_id, season_id, household_id, hours, reason, created_by, created_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8)`,
		c.ID, c.OrgID, c.SeasonID, c.HouseholdID, c.Hours, c.Reason, c.CreatedBy, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("volunteer: insert credit: %w", err)
	}
	return nil
}

// ─── Buyouts ──────────────────────────────────────────────────────────────────

func (r *Repository) listBuyouts(ctx context.Context, orgID, seasonID string) ([]*Buyout, error) {
	if seasonID != "" && !isUUID(seasonID) {
		return []*Buyout{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, season_id::text, household_id, status, hours_short::float8, rate_cents_per_hour,
		       amount_cents, invoice_id, invoice_item_id, created_by, created_at, updated_at
		FROM volunteer_buyouts
		WHERE org_id = $1 AND ($2 = '' OR season_id = NULLIF($2, '')::uuid)
		ORDER BY created_at, id`, orgID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("volunteer: list buyouts: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "volunteer: scan buyout", func(rows pgx.Rows) (*Buyout, error) {
		b := &Buyout{}
		err := rows.Scan(&b.ID, &b.OrgID, &b.SeasonID, &b.HouseholdID, &b.Status, &b.HoursShort, &b.RateCentsPerHour,
			&b.AmountCents, &b.InvoiceID, &b.InvoiceItemID, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
		return b, err
	})
}

func (r *Repository) reserveBuyout(ctx context.Context, b *Buyout) error {
	if !isUUID(b.SeasonID) {
		return ErrSeasonNotFound
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO volunteer_buyouts (id, org_id, season_id, household_id, status, hours_short, rate_cents_per_hour,
			amount_cents, invoice_id, invoice_item_id, created_by, created_at, updated_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		b.ID, b.OrgID, b.SeasonID, b.HouseholdID, b.Status, b.HoursShort, b.RateCentsPerHour,
		b.AmountCents, b.InvoiceID, b.InvoiceItemID, b.CreatedBy, b.CreatedAt, b.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrBuyoutExists
		}
		return fmt.Errorf("volunteer: reserve buyout: %w", err)
	}
	return nil
}

func (r *Repository) updateBuyout(ctx context.Context, b *Buyout) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE volunteer_buyouts SET status = $3, invoice_id = $4, invoice_item_id = $5, updated_at = $6
		WHERE id = $1::uuid AND org_id = $2`,
		b.ID, b.OrgID, b.Status, b.InvoiceID, b.InvoiceItemID, b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("volunteer: update buyout: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBuyoutNotFound
	}
	return nil
}

func (r *Repository) deleteBuyout(ctx context.Context, orgID, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM volunteer_buyouts WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("volunteer: delete buyout: %w", err)
	}
	return nil
}
