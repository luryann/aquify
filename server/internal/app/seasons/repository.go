package seasons

import (
	"context"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the seasons domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Upsert inserts or updates a season record.
func (r *Repository) Upsert(ctx context.Context, s *Season) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO seasons (
			id, org_id, name, start_date, end_date, status, goals,
			created_at, updated_at, created_by, updated_by
		) VALUES (
			$1::uuid, $2, $3, $4::date, $5::date, $6, $7,
			$8, $9, $10, $11
		)
		ON CONFLICT (id) DO UPDATE SET
			name       = EXCLUDED.name,
			start_date = EXCLUDED.start_date,
			end_date   = EXCLUDED.end_date,
			status     = EXCLUDED.status,
			goals      = EXCLUDED.goals,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by`,
		s.ID, s.OrgID, s.Name, dbutil.NullableString(s.StartDate), dbutil.NullableString(s.EndDate), s.Status, s.Goals,
		s.CreatedAt, s.UpdatedAt, s.CreatedBy, s.UpdatedBy,
	)
	return err
}

// Delete permanently removes a season by (org_id, id).
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM seasons WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListByOrg(ctx context.Context, orgID string) ([]*Season, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, name,
		       COALESCE(start_date::text, ''), COALESCE(end_date::text, ''),
		       status, goals,
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM seasons
		WHERE org_id = $1
		ORDER BY start_date DESC NULLS LAST, created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("seasons: list seasons: %w", err)
	}
	defer rows.Close()

	return scanSeasons(rows)
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (*Season, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `
		SELECT id::text, org_id, name,
		       COALESCE(start_date::text, ''), COALESCE(end_date::text, ''),
		       status, goals,
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM seasons
		WHERE id = $1::uuid AND org_id = $2`,
		id, orgID,
	)
	s := &Season{}
	if err := row.Scan(
		&s.ID, &s.OrgID, &s.Name,
		&s.StartDate, &s.EndDate,
		&s.Status, &s.Goals,
		&s.CreatedAt, &s.UpdatedAt,
		&s.CreatedBy, &s.UpdatedBy,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("seasons: get season: %w", err)
	}
	return s, nil
}

// LoadAll loads every season from the DB.
func (r *Repository) LoadAll(ctx context.Context) ([]*Season, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, name,
		       COALESCE(start_date::text, ''), COALESCE(end_date::text, ''),
		       status, goals,
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM seasons
		ORDER BY start_date DESC NULLS LAST, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("seasons: load seasons: %w", err)
	}
	defer rows.Close()

	return scanSeasons(rows)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func scanSeasons(rows pgx.Rows) ([]*Season, error) {
	return dbutil.CollectRows(rows, "seasons: scan season", func(rows pgx.Rows) (*Season, error) {
		s := &Season{}
		err := rows.Scan(
			&s.ID, &s.OrgID, &s.Name,
			&s.StartDate, &s.EndDate,
			&s.Status, &s.Goals,
			&s.CreatedAt, &s.UpdatedAt,
			&s.CreatedBy, &s.UpdatedBy,
		)
		return s, err
	})
}
