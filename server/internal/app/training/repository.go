package training

import (
	"context"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the training domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// withTx runs fn inside a transaction, committing on success.
func (r *Repository) withTx(ctx context.Context, fn func(q querier) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("training: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Upsert inserts or updates a practice record.
func (r *Repository) Upsert(ctx context.Context, p *Practice) error {
	return upsertPractice(ctx, r.pool, p)
}

func upsertPractice(ctx context.Context, q querier, p *Practice) error {
	_, err := q.Exec(ctx, `
		INSERT INTO practices (
			id, org_id, date, group_id, type, description, yardage_target,
			attendance, strokes_free, strokes_back, strokes_breast, strokes_fly, strokes_im,
			notes, created_at, updated_at, created_by, updated_by, workout_id
		) VALUES (
			$1::uuid, $2, $3::date, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19::uuid
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
			updated_by     = EXCLUDED.updated_by,
			workout_id     = EXCLUDED.workout_id`,
		p.ID, p.OrgID, p.Date, dbutil.NullableString(p.GroupID), p.Type, p.Description, p.YardageTarget,
		p.Attendance, p.Strokes.Free, p.Strokes.Back, p.Strokes.Breast, p.Strokes.Fly, p.Strokes.IM,
		p.Notes, p.CreatedAt, p.UpdatedAt, p.CreatedBy, p.UpdatedBy, dbutil.NullableString(p.WorkoutID),
	)
	return err
}

// Delete permanently removes a practice by (org_id, id) so a caller cannot
// delete another org's practice by guessing/observing its UUID.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM practices WHERE id = $1::uuid AND org_id = $2`, id, orgID)
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
		       COALESCE(created_by, ''), COALESCE(updated_by, ''),
		       COALESCE(workout_id::text, '')
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
	return getPractice(ctx, r.pool, orgID, id)
}

func getPractice(ctx context.Context, q querier, orgID, id string) (*Practice, error) {
	row := q.QueryRow(ctx, `
		SELECT id::text, org_id, date::text, COALESCE(group_id, ''), type,
		       COALESCE(description, ''), yardage_target,
		       attendance,
		       strokes_free, strokes_back, strokes_breast, strokes_fly, strokes_im,
		       COALESCE(notes, ''),
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, ''),
		       COALESCE(workout_id::text, '')
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
		&p.WorkoutID,
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
		       COALESCE(created_by, ''), COALESCE(updated_by, ''),
		       COALESCE(workout_id::text, '')
		FROM practices
		ORDER BY date DESC, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("training: load practices: %w", err)
	}
	defer rows.Close()

	return scanPractices(rows)
}

// UpsertAttendance inserts or updates one swimmer's attendance record for a practice.
func (r *Repository) UpsertAttendance(ctx context.Context, orgID, practiceID, swimmerID, status, markedBy string, markedAt time.Time) (*PracticeAttendance, error) {
	id := uuid.New().String()
	row := r.pool.QueryRow(ctx, `
		INSERT INTO practice_attendance (
			id, org_id, practice_id, swimmer_id, status, marked_at, marked_by
		) VALUES (
			$1::uuid, $2, $3::uuid, $4::uuid, $5, $6, $7
		)
		ON CONFLICT (practice_id, swimmer_id) DO UPDATE SET
			status     = EXCLUDED.status,
			marked_at  = EXCLUDED.marked_at,
			marked_by  = EXCLUDED.marked_by,
			updated_at = NOW()
		RETURNING id::text, org_id, practice_id::text, swimmer_id::text, status,
		          marked_at, marked_by, created_at, updated_at`,
		id, orgID, practiceID, swimmerID, status, markedAt, markedBy,
	)
	rec := &PracticeAttendance{}
	if err := row.Scan(
		&rec.ID, &rec.OrgID, &rec.PracticeID, &rec.SwimmerID, &rec.Status,
		&rec.MarkedAt, &rec.MarkedBy, &rec.CreatedAt, &rec.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("training: upsert attendance: %w", err)
	}
	return rec, nil
}

// ListAttendanceForPractice returns all attendance rows recorded for a practice.
func (r *Repository) ListAttendanceForPractice(ctx context.Context, orgID, practiceID string) ([]*PracticeAttendance, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, practice_id::text, swimmer_id::text, status,
		       marked_at, marked_by, created_at, updated_at
		FROM practice_attendance
		WHERE org_id = $1 AND practice_id = $2::uuid`,
		orgID, practiceID,
	)
	if err != nil {
		return nil, fmt.Errorf("training: list attendance: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "training: scan attendance", func(rows pgx.Rows) (*PracticeAttendance, error) {
		rec := &PracticeAttendance{}
		err := rows.Scan(
			&rec.ID, &rec.OrgID, &rec.PracticeID, &rec.SwimmerID, &rec.Status,
			&rec.MarkedAt, &rec.MarkedBy, &rec.CreatedAt, &rec.UpdatedAt,
		)
		return rec, err
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func scanPractices(rows pgx.Rows) ([]*Practice, error) {
	return dbutil.CollectRows(rows, "training: scan practice", func(rows pgx.Rows) (*Practice, error) {
		p := &Practice{}
		err := rows.Scan(
			&p.ID, &p.OrgID, &p.Date, &p.GroupID, &p.Type,
			&p.Description, &p.YardageTarget,
			&p.Attendance,
			&p.Strokes.Free, &p.Strokes.Back, &p.Strokes.Breast, &p.Strokes.Fly, &p.Strokes.IM,
			&p.Notes,
			&p.CreatedAt, &p.UpdatedAt,
			&p.CreatedBy, &p.UpdatedBy,
			&p.WorkoutID,
		)
		return p, err
	})
}
