package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"strings"

	"github.com/jackc/pgx/v5"
)

const workoutColumns = `id::text, org_id, title, description, course, is_template, tags,
	sections, COALESCE(source_workout_id::text, ''), created_at, updated_at, created_by, updated_by`

func scanWorkout(row pgx.Row) (*Workout, error) {
	w := &Workout{}
	var sections []byte
	if err := row.Scan(&w.ID, &w.OrgID, &w.Title, &w.Description, &w.Course, &w.IsTemplate, &w.Tags,
		&sections, &w.SourceWorkoutID, &w.CreatedAt, &w.UpdatedAt, &w.CreatedBy, &w.UpdatedBy); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(sections, &w.Sections); err != nil {
		return nil, fmt.Errorf("training: decode workout sections: %w", err)
	}
	if w.Tags == nil {
		w.Tags = []string{}
	}
	if w.Sections == nil {
		w.Sections = []WorkoutSection{}
	}
	w.Totals = ComputeTotals(w.Sections, w.Course)
	return w, nil
}

func upsertWorkout(ctx context.Context, q querier, w *Workout) error {
	sections, err := json.Marshal(w.Sections)
	if err != nil {
		return fmt.Errorf("training: encode workout sections: %w", err)
	}
	tags := w.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err = q.Exec(ctx, `
		INSERT INTO training_workouts (
			id, org_id, title, description, course, is_template, tags, sections,
			total_distance, source_workout_id, created_at, updated_at, created_by, updated_by
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10::uuid, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			title          = EXCLUDED.title,
			description    = EXCLUDED.description,
			course         = EXCLUDED.course,
			is_template    = EXCLUDED.is_template,
			tags           = EXCLUDED.tags,
			sections       = EXCLUDED.sections,
			total_distance = EXCLUDED.total_distance,
			updated_at     = EXCLUDED.updated_at,
			updated_by     = EXCLUDED.updated_by
		WHERE training_workouts.org_id = EXCLUDED.org_id`,
		w.ID, w.OrgID, w.Title, w.Description, w.Course, w.IsTemplate, tags, sections,
		w.Totals.Distance, dbutil.NullableString(w.SourceWorkoutID), w.CreatedAt, w.UpdatedAt, w.CreatedBy, w.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("training: upsert workout: %w", err)
	}
	return nil
}

func getWorkout(ctx context.Context, q querier, orgID, id string) (*Workout, error) {
	w, err := scanWorkout(q.QueryRow(ctx, `SELECT `+workoutColumns+` FROM training_workouts WHERE id = $1::uuid AND org_id = $2`, id, orgID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkoutNotFound
		}
		return nil, fmt.Errorf("training: get workout: %w", err)
	}
	return w, nil
}

func deleteWorkout(ctx context.Context, q querier, orgID, id string) error {
	tag, err := q.Exec(ctx, `DELETE FROM training_workouts WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("training: delete workout: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWorkoutNotFound
	}
	return nil
}

// ListWorkouts returns workouts matching the filter, most recently updated first.
func (r *Repository) ListWorkouts(ctx context.Context, orgID string, f WorkoutFilter) ([]*Workout, error) {
	clauses := []string{"org_id = $1"}
	args := []any{orgID}
	if f.TemplatesOnly {
		clauses = append(clauses, "is_template")
	}
	if tag := strings.ToLower(strings.TrimSpace(f.Tag)); tag != "" {
		args = append(args, tag)
		clauses = append(clauses, fmt.Sprintf("$%d = ANY(tags)", len(args)))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+escapeLike(strings.ToLower(q))+"%")
		n := len(args)
		clauses = append(clauses, fmt.Sprintf("(lower(title) LIKE $%d OR lower(description) LIKE $%d OR EXISTS (SELECT 1 FROM unnest(tags) t WHERE t LIKE $%d))", n, n, n))
	}
	rows, err := r.pool.Query(ctx, `SELECT `+workoutColumns+` FROM training_workouts WHERE `+strings.Join(clauses, " AND ")+` ORDER BY updated_at DESC, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("training: list workouts: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "training: scan workout", func(rows pgx.Rows) (*Workout, error) {
		return scanWorkout(rows)
	})
}

// WorkoutsByIDs loads the given workouts (org-scoped) keyed by ID.
func (r *Repository) WorkoutsByIDs(ctx context.Context, orgID string, ids []string) (map[string]*Workout, error) {
	out := map[string]*Workout{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+workoutColumns+` FROM training_workouts WHERE org_id = $1 AND id = ANY($2::uuid[])`, orgID, ids)
	if err != nil {
		return nil, fmt.Errorf("training: load workouts: %w", err)
	}
	workouts, err := dbutil.CollectRows(rows, "training: scan workout", func(rows pgx.Rows) (*Workout, error) {
		return scanWorkout(rows)
	})
	if err != nil {
		return nil, err
	}
	for _, w := range workouts {
		out[w.ID] = w
	}
	return out, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// AttendanceForPractices returns attendance rows for the given practices.
func (r *Repository) AttendanceForPractices(ctx context.Context, orgID string, practiceIDs []string) ([]*PracticeAttendance, error) {
	if len(practiceIDs) == 0 {
		return []*PracticeAttendance{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, practice_id::text, swimmer_id::text, status,
		       marked_at, marked_by, created_at, updated_at
		FROM practice_attendance
		WHERE org_id = $1 AND practice_id = ANY($2::uuid[])`, orgID, practiceIDs)
	if err != nil {
		return nil, fmt.Errorf("training: attendance for practices: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "training: scan attendance", func(rows pgx.Rows) (*PracticeAttendance, error) {
		rec := &PracticeAttendance{}
		err := rows.Scan(&rec.ID, &rec.OrgID, &rec.PracticeID, &rec.SwimmerID, &rec.Status,
			&rec.MarkedAt, &rec.MarkedBy, &rec.CreatedAt, &rec.UpdatedAt)
		return rec, err
	})
}

// ─── Plan phases ──────────────────────────────────────────────────────────────

const phaseColumns = `id::text, org_id, season_id::text, group_id, phase, name, start_date::text, end_date::text,
	weekly_yardage_target, focus, created_at, updated_at, created_by, updated_by`

func scanPhase(row pgx.Row) (*PlanPhase, error) {
	p := &PlanPhase{}
	if err := row.Scan(&p.ID, &p.OrgID, &p.SeasonID, &p.GroupID, &p.Phase, &p.Name, &p.StartDate, &p.EndDate,
		&p.WeeklyYardageTarget, &p.Focus, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy); err != nil {
		return nil, err
	}
	return p, nil
}

// ListPhases returns phases for a season (all groups) ordered by group/start.
func (r *Repository) ListPhases(ctx context.Context, orgID, seasonID string) ([]*PlanPhase, error) {
	return listPhases(ctx, r.pool, orgID, seasonID)
}

func listPhases(ctx context.Context, q querier, orgID, seasonID string) ([]*PlanPhase, error) {
	rows, err := q.Query(ctx, `SELECT `+phaseColumns+` FROM training_plan_phases
		WHERE org_id = $1 AND season_id = $2::uuid ORDER BY group_id, start_date, id`, orgID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("training: list phases: %w", err)
	}
	return dbutil.CollectRowsNonNil(rows, "training: scan phase", func(rows pgx.Rows) (*PlanPhase, error) {
		return scanPhase(rows)
	})
}

func getPhase(ctx context.Context, q querier, orgID, id string) (*PlanPhase, error) {
	p, err := scanPhase(q.QueryRow(ctx, `SELECT `+phaseColumns+` FROM training_plan_phases WHERE id = $1::uuid AND org_id = $2`, id, orgID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPhaseNotFound
		}
		return nil, fmt.Errorf("training: get phase: %w", err)
	}
	return p, nil
}

// SavePhase upserts a phase after re-checking overlap under an advisory lock
// scoped to (org, season, group) so concurrent writers cannot race past it.
func (r *Repository) SavePhase(ctx context.Context, p *PlanPhase) error {
	return r.withTx(ctx, func(q querier) error {
		if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "training_phase:"+p.OrgID+":"+p.SeasonID+":"+p.GroupID); err != nil {
			return fmt.Errorf("training: lock phases: %w", err)
		}
		existing, err := listPhases(ctx, q, p.OrgID, p.SeasonID)
		if err != nil {
			return err
		}
		if err := checkPhaseOverlap(existing, p); err != nil {
			return err
		}
		_, err = q.Exec(ctx, `
			INSERT INTO training_plan_phases (
				id, org_id, season_id, group_id, phase, name, start_date, end_date,
				weekly_yardage_target, focus, created_at, updated_at, created_by, updated_by
			) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7::date, $8::date, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (id) DO UPDATE SET
				group_id              = EXCLUDED.group_id,
				phase                 = EXCLUDED.phase,
				name                  = EXCLUDED.name,
				start_date            = EXCLUDED.start_date,
				end_date              = EXCLUDED.end_date,
				weekly_yardage_target = EXCLUDED.weekly_yardage_target,
				focus                 = EXCLUDED.focus,
				updated_at            = EXCLUDED.updated_at,
				updated_by            = EXCLUDED.updated_by
			WHERE training_plan_phases.org_id = EXCLUDED.org_id`,
			p.ID, p.OrgID, p.SeasonID, p.GroupID, p.Phase, p.Name, p.StartDate, p.EndDate,
			p.WeeklyYardageTarget, p.Focus, p.CreatedAt, p.UpdatedAt, p.CreatedBy, p.UpdatedBy)
		if err != nil {
			return fmt.Errorf("training: save phase: %w", err)
		}
		return nil
	})
}

// DeletePhase removes one phase (org-scoped).
func (r *Repository) DeletePhase(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM training_plan_phases WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("training: delete phase: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPhaseNotFound
	}
	return nil
}
