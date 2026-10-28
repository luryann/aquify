package wellness

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists wellness check-ins. Postgres is canonical in DB mode.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const columns = `id::text, org_id, swimmer_id::text, household_id, checkin_date::text,
	sleep_hours::float8, sleep_quality, soreness, energy, stress, mood, note,
	submitted_by, submitted_role, created_at, updated_at`

// Upsert inserts or replaces the swimmer's check-in for the day.
func (r *Repository) Upsert(ctx context.Context, c *CheckIn) (*CheckIn, bool, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO wellness_checkins (
			id, org_id, swimmer_id, household_id, checkin_date, sleep_hours, sleep_quality,
			soreness, energy, stress, mood, note, submitted_by, submitted_role, created_at, updated_at
		) VALUES ($1::uuid, $2, $3::uuid, $4, $5::date, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (swimmer_id, checkin_date) DO UPDATE SET
			household_id = EXCLUDED.household_id, sleep_hours = EXCLUDED.sleep_hours,
			sleep_quality = EXCLUDED.sleep_quality, soreness = EXCLUDED.soreness,
			energy = EXCLUDED.energy, stress = EXCLUDED.stress, mood = EXCLUDED.mood,
			note = EXCLUDED.note, submitted_by = EXCLUDED.submitted_by,
			submitted_role = EXCLUDED.submitted_role, updated_at = EXCLUDED.updated_at
		WHERE wellness_checkins.org_id = EXCLUDED.org_id
		RETURNING `+columns+`, (xmax = 0) AS inserted`,
		c.ID, c.OrgID, c.SwimmerID, c.HouseholdID, c.Date, c.SleepHours, c.SleepQuality,
		c.Soreness, c.Energy, c.Stress, c.Mood, c.Note, c.SubmittedBy, c.SubmittedRole, c.CreatedAt, c.UpdatedAt,
	)
	out := &CheckIn{}
	var inserted bool
	if err := row.Scan(&out.ID, &out.OrgID, &out.SwimmerID, &out.HouseholdID, &out.Date,
		&out.SleepHours, &out.SleepQuality, &out.Soreness, &out.Energy, &out.Stress, &out.Mood, &out.Note,
		&out.SubmittedBy, &out.SubmittedRole, &out.CreatedAt, &out.UpdatedAt, &inserted); err != nil {
		return nil, false, fmt.Errorf("wellness: upsert check-in: %w", err)
	}
	return out, inserted, nil
}

// List loads check-ins for an org in a date window, optionally filtered to
// specific swimmers (nil = all).
func (r *Repository) List(ctx context.Context, orgID string, swimmerIDs []string, from, to string) ([]*CheckIn, error) {
	query := `SELECT ` + columns + ` FROM wellness_checkins
		WHERE org_id = $1 AND checkin_date BETWEEN $2::date AND $3::date`
	args := []any{orgID, from, to}
	if swimmerIDs != nil {
		query += ` AND swimmer_id::text = ANY($4)`
		args = append(args, swimmerIDs)
	}
	query += ` ORDER BY swimmer_id, checkin_date`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("wellness: list check-ins: %w", err)
	}
	defer rows.Close()
	out := []*CheckIn{}
	for rows.Next() {
		c := &CheckIn{}
		if err := rows.Scan(&c.ID, &c.OrgID, &c.SwimmerID, &c.HouseholdID, &c.Date,
			&c.SleepHours, &c.SleepQuality, &c.Soreness, &c.Energy, &c.Stress, &c.Mood, &c.Note,
			&c.SubmittedBy, &c.SubmittedRole, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("wellness: scan check-in: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
