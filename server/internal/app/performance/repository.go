package performance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the performance domain.
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

// ─── Results ─────────────────────────────────────────────────────────────────

func (r *Repository) UpsertResult(ctx context.Context, res *Result) error {
	var finTimeMs interface{}
	if res.FinTimeMs != nil {
		finTimeMs = *res.FinTimeMs
	}
	var heat, lane, place interface{}
	if res.Heat != nil {
		heat = *res.Heat
	}
	if res.Lane != nil {
		lane = *res.Lane
	}
	if res.Place != nil {
		place = *res.Place
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO results (id, org_id, swimmer_id, meet_id, distance, stroke, course, gender,
		                     fin_time_ms, is_official, status, heat, lane, place, dq, dq_code, dq_note, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (id) DO UPDATE SET
		  fin_time_ms = EXCLUDED.fin_time_ms,
		  is_official = EXCLUDED.is_official,
		  status      = EXCLUDED.status,
		  heat        = EXCLUDED.heat,
		  lane        = EXCLUDED.lane,
		  place       = EXCLUDED.place,
		  dq          = EXCLUDED.dq,
		  dq_code     = EXCLUDED.dq_code,
		  dq_note     = EXCLUDED.dq_note,
		  updated_at  = EXCLUDED.updated_at`,
		res.ID, res.OrgID, res.SwimmerID,
		nullableString(res.MeetID),
		res.Distance, res.Stroke, res.Course, res.Gender,
		finTimeMs, res.IsOfficial,
		res.Status,
		heat, lane, place,
		res.DQ,
		nullableString(res.DQCode), nullableString(res.DQNote),
		res.CreatedAt, res.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadResults(ctx context.Context) ([]*Result, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, swimmer_id::text, COALESCE(meet_id::text, ''),
		       distance, stroke, course, gender,
		       fin_time_ms, is_official, status,
		       heat, lane, place, dq, COALESCE(dq_code, ''), COALESCE(dq_note, ''),
		       created_at, updated_at
		FROM results WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("performance: load results: %w", err)
	}
	defer rows.Close()
	var out []*Result
	for rows.Next() {
		res := &Result{}
		var finTimeMs *int
		var heat, lane, place *int
		if err := rows.Scan(
			&res.ID, &res.OrgID, &res.SwimmerID, &res.MeetID,
			&res.Distance, &res.Stroke, &res.Course, &res.Gender,
			&finTimeMs, &res.IsOfficial, &res.Status,
			&heat, &lane, &place,
			&res.DQ, &res.DQCode, &res.DQNote,
			&res.CreatedAt, &res.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("performance: scan result: %w", err)
		}
		res.FinTimeMs = finTimeMs
		res.Heat = heat
		res.Lane = lane
		res.Place = place
		out = append(out, res)
	}
	return out, rows.Err()
}

// ─── Best Times ───────────────────────────────────────────────────────────────

func (r *Repository) UpsertBestTime(ctx context.Context, bt *BestTime) error {
	var bestTimeMs interface{}
	if bt.BestTimeMs != nil {
		bestTimeMs = *bt.BestTimeMs
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO best_times (id, org_id, swimmer_id, event_distance, stroke, course,
		                        best_time_ms, result_id, recorded_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
		  best_time_ms = EXCLUDED.best_time_ms,
		  result_id    = EXCLUDED.result_id,
		  recorded_at  = EXCLUDED.recorded_at,
		  updated_at   = EXCLUDED.updated_at`,
		bt.ID, bt.OrgID, bt.SwimmerID,
		bt.EventDistance, bt.Stroke, bt.Course,
		bestTimeMs,
		nullableString(bt.ResultID), bt.RecordedAt,
		bt.CreatedAt, bt.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadBestTimes(ctx context.Context) ([]*BestTime, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, swimmer_id::text, event_distance, stroke, course,
		       best_time_ms, COALESCE(result_id::text, ''), recorded_at, created_at, updated_at
		FROM best_times`)
	if err != nil {
		return nil, fmt.Errorf("performance: load best_times: %w", err)
	}
	defer rows.Close()
	var out []*BestTime
	for rows.Next() {
		bt := &BestTime{}
		var bestTimeMs *int
		if err := rows.Scan(
			&bt.ID, &bt.OrgID, &bt.SwimmerID,
			&bt.EventDistance, &bt.Stroke, &bt.Course,
			&bestTimeMs, &bt.ResultID, &bt.RecordedAt,
			&bt.CreatedAt, &bt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("performance: scan best_time: %w", err)
		}
		bt.BestTimeMs = bestTimeMs
		out = append(out, bt)
	}
	return out, rows.Err()
}

// ─── Time Standards ───────────────────────────────────────────────────────────

func (r *Repository) UpsertTimeStandard(ctx context.Context, ts *TimeStandard) error {
	var ageMin, ageMax interface{}
	if ts.AgeMin != nil {
		ageMin = *ts.AgeMin
	}
	if ts.AgeMax != nil {
		ageMax = *ts.AgeMax
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO time_standards (id, org_id, name, standard_code, gender, age_min, age_max,
		                            event_distance, stroke, course, time_ms, season_year, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
		  name          = EXCLUDED.name,
		  standard_code = EXCLUDED.standard_code,
		  time_ms       = EXCLUDED.time_ms,
		  updated_at    = EXCLUDED.updated_at`,
		ts.ID, ts.OrgID, ts.Name, ts.StandardCode, ts.Gender,
		ageMin, ageMax,
		ts.EventDistance, ts.Stroke, ts.Course, ts.TimeMs,
		ts.SeasonYear,
		ts.CreatedAt, ts.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadTimeStandards(ctx context.Context) ([]*TimeStandard, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, name, COALESCE(standard_code, ''), gender,
		       age_min, age_max, event_distance, stroke, course, time_ms, season_year,
		       is_active, created_at, updated_at
		FROM time_standards WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("performance: load time_standards: %w", err)
	}
	defer rows.Close()
	var out []*TimeStandard
	for rows.Next() {
		ts := &TimeStandard{}
		if err := rows.Scan(
			&ts.ID, &ts.OrgID, &ts.Name, &ts.StandardCode, &ts.Gender,
			&ts.AgeMin, &ts.AgeMax, &ts.EventDistance, &ts.Stroke, &ts.Course, &ts.TimeMs,
			&ts.SeasonYear, &ts.IsActive,
			&ts.CreatedAt, &ts.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("performance: scan time_standard: %w", err)
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	results []*Result,
	bestTimes []*BestTime,
	standards []*TimeStandard,
	err error,
) {
	if results, err = r.LoadResults(ctx); err != nil {
		return
	}
	if bestTimes, err = r.LoadBestTimes(ctx); err != nil {
		return
	}
	standards, err = r.LoadTimeStandards(ctx)
	return
}
