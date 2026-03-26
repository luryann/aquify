package training

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the training domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Upsert inserts or updates a practice record.
func (r *Repository) Upsert(ctx context.Context, p *Practice) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO practices (
			id, org_id, date, group_id, type, description, yardage_target,
			attendance, strokes_free, strokes_back, strokes_breast, strokes_fly, strokes_im,
			notes, created_at, updated_at, created_by, updated_by
		) VALUES (
			$1::uuid, $2, $3::date, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18
		)
		ON CONFLICT (id) DO UPDATE SET
			date           = EXCLUDED.date,
			group_id       = EXCLUDED.group_id,
			type           = EXCLUDED.type,
			description    = EXCLUDED.description,
			yardage_target = EXCLUDED.yardage_target,
			attendance     = EXCLUDED.attendance,
			strokes_free   = EXCLUDED.strokes_free,
			strokes_back   = EXCLUDED.strokes_back,
			strokes_breast = EXCLUDED.strokes_breast,
			strokes_fly    = EXCLUDED.strokes_fly,
			strokes_im     = EXCLUDED.strokes_im,
			notes          = EXCLUDED.notes,
			updated_at     = EXCLUDED.updated_at,
			updated_by     = EXCLUDED.updated_by`,
		p.ID, p.OrgID, p.Date, nullableString(p.GroupID), p.Type, p.Description, p.YardageTarget,
		p.Attendance, p.Strokes.Free, p.Strokes.Back, p.Strokes.Breast, p.Strokes.Fly, p.Strokes.IM,
		p.Notes, p.CreatedAt, p.UpdatedAt, p.CreatedBy, p.UpdatedBy,
	)
	return err
}

// Delete permanently removes a practice by ID.
func (r *Repository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM practices WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListByOrg(ctx context.Context, orgID string) ([]*Practice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, date::text, COALESCE(group_id, ''), type,
		       COALESCE(description, ''), yardage_target,
		       attendance,
		       strokes_free, strokes_back, strokes_breast, strokes_fly, strokes_im,
		       COALESCE(notes, ''),
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM practices
		WHERE org_id = $1
		ORDER BY date DESC, created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("training: list practices: %w", err)
	}
	defer rows.Close()

	return scanPractices(rows)
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (*Practice, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id::text, org_id, date::text, COALESCE(group_id, ''), type,
		       COALESCE(description, ''), yardage_target,
		       attendance,
		       strokes_free, strokes_back, strokes_breast, strokes_fly, strokes_im,
		       COALESCE(notes, ''),
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM practices
		WHERE id = $1::uuid AND org_id = $2`,
		id, orgID,
	)
	p := &Practice{}
	if err := row.Scan(
		&p.ID, &p.OrgID, &p.Date, &p.GroupID, &p.Type,
		&p.Description, &p.YardageTarget,
		&p.Attendance,
		&p.Strokes.Free, &p.Strokes.Back, &p.Strokes.Breast, &p.Strokes.Fly, &p.Strokes.IM,
		&p.Notes,
		&p.CreatedAt, &p.UpdatedAt,
		&p.CreatedBy, &p.UpdatedBy,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("training: get practice: %w", err)
	}
	return p, nil
}

// LoadAll loads every practice from the DB.
func (r *Repository) LoadAll(ctx context.Context) ([]*Practice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, date::text, COALESCE(group_id, ''), type,
		       COALESCE(description, ''), yardage_target,
		       attendance,
		       strokes_free, strokes_back, strokes_breast, strokes_fly, strokes_im,
		       COALESCE(notes, ''),
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM practices
		ORDER BY date DESC, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("training: load practices: %w", err)
	}
	defer rows.Close()

	return scanPractices(rows)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func scanPractices(rows pgx.Rows) ([]*Practice, error) {
	var out []*Practice
	for rows.Next() {
		p := &Practice{}
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Date, &p.GroupID, &p.Type,
			&p.Description, &p.YardageTarget,
			&p.Attendance,
			&p.Strokes.Free, &p.Strokes.Back, &p.Strokes.Breast, &p.Strokes.Fly, &p.Strokes.IM,
			&p.Notes,
			&p.CreatedAt, &p.UpdatedAt,
			&p.CreatedBy, &p.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("training: scan practice: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
