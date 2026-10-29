package performance

import (
	"context"
	"fmt"
	"time"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the performance domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Results ─────────────────────────────────────────────────────────────────

func (r *Repository) UpsertResult(ctx context.Context, res *Result) error {
	var finTimeMs interface{}
	if res.FinTimeMs != nil {
		finTimeMs = *res.FinTimeMs
	}
	var heat, lane, place, seedTimeMs interface{}
	if res.SeedTimeMs != nil {
		seedTimeMs = *res.SeedTimeMs
	}
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
		                     fin_time_ms, is_official, status, heat, lane, place, dq, dq_code, dq_note, created_at, updated_at,
		                     meet_event_id, event_number, seed_time_ms)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
		        (SELECT id FROM meet_events WHERE id::text = $20), $21, $22)
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
		dbutil.NullableString(res.MeetID),
		res.Distance, res.Stroke, res.Course, res.Gender,
		finTimeMs, res.IsOfficial,
		res.Status,
		heat, lane, place,
		res.DQ,
		dbutil.NullableString(res.DQCode), dbutil.NullableString(res.DQNote),
		res.CreatedAt, res.UpdatedAt,
		res.MeetEventID, dbutil.NullableString(res.EventNumber), seedTimeMs,
	)
	return err
}

// UpsertSplits stores a result's splits in one transaction.
func (r *Repository) UpsertSplits(ctx context.Context, splits []*Split) error {
	if len(splits) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, sp := range splits {
		if _, err := tx.Exec(ctx, `
			INSERT INTO splits (id, org_id, result_id, split_number, split_distance, split_time_ms, cumulative_time_ms, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (result_id, split_number) DO UPDATE SET
			  split_distance = EXCLUDED.split_distance,
			  split_time_ms = EXCLUDED.split_time_ms,
			  cumulative_time_ms = EXCLUDED.cumulative_time_ms,
			  updated_at = EXCLUDED.updated_at`,
			sp.ID, sp.OrgID, sp.ResultID, sp.SplitNumber, sp.SplitDistance, sp.SplitTimeMs, sp.CumulativeTimeMs, sp.CreatedAt, sp.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) LoadResults(ctx context.Context) ([]*Result, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, swimmer_id::text, COALESCE(meet_id::text, ''),
		       COALESCE(meet_event_id::text, ''), COALESCE(event_number, ''), seed_time_ms,
		       distance, stroke, course, gender,
		       fin_time_ms, is_official, status,
		       heat, lane, place, dq, COALESCE(dq_code, ''), COALESCE(dq_note, ''),
		       created_at, updated_at
		FROM results WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("performance: load results: %w", err)
	}
	return dbutil.CollectRows(rows, "performance: scan result", func(rows pgx.Rows) (*Result, error) {
		res := &Result{}
		var finTimeMs *int
		var heat, lane, place *int
		if err := rows.Scan(
			&res.ID, &res.OrgID, &res.SwimmerID, &res.MeetID,
			&res.MeetEventID, &res.EventNumber, &res.SeedTimeMs,
			&res.Distance, &res.Stroke, &res.Course, &res.Gender,
			&finTimeMs, &res.IsOfficial, &res.Status,
			&heat, &lane, &place,
			&res.DQ, &res.DQCode, &res.DQNote,
			&res.CreatedAt, &res.UpdatedAt,
		); err != nil {
			return res, err
		}
		res.FinTimeMs = finTimeMs
		res.Heat = heat
		res.Lane = lane
		res.Place = place
		return res, nil
	})
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
		dbutil.NullableString(bt.ResultID), bt.RecordedAt,
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
	return dbutil.CollectRows(rows, "performance: scan best_time", func(rows pgx.Rows) (*BestTime, error) {
		bt := &BestTime{}
		var bestTimeMs *int
		if err := rows.Scan(
			&bt.ID, &bt.OrgID, &bt.SwimmerID,
			&bt.EventDistance, &bt.Stroke, &bt.Course,
			&bestTimeMs, &bt.ResultID, &bt.RecordedAt,
			&bt.CreatedAt, &bt.UpdatedAt,
		); err != nil {
			return bt, err
		}
		bt.BestTimeMs = bestTimeMs
		return bt, nil
	})
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
		INSERT INTO time_standards (id, org_id, name, standard_code, description, gender, age_min, age_max,
		                            event_distance, stroke, course, time_ms, season_year, is_active, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
		  name          = EXCLUDED.name,
		  standard_code = EXCLUDED.standard_code,
		  description   = EXCLUDED.description,
		  time_ms       = EXCLUDED.time_ms,
		  is_active     = EXCLUDED.is_active,
		  updated_at    = EXCLUDED.updated_at`,
		ts.ID, ts.OrgID, ts.Name, ts.StandardCode, dbutil.NullableString(ts.Description), ts.Gender,
		ageMin, ageMax,
		ts.EventDistance, ts.Stroke, ts.Course, ts.TimeMs,
		ts.SeasonYear, ts.IsActive,
		ts.CreatedAt, ts.UpdatedAt,
	)
	return err
}

// SoftDeleteTimeStandard marks a time standard deleted so it is not reloaded.
func (r *Repository) SoftDeleteTimeStandard(ctx context.Context, orgID, id string, deletedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE time_standards SET deleted_at = $3, updated_at = $3
		WHERE id = $1::uuid AND org_id = $2::uuid AND deleted_at IS NULL`,
		id, orgID, deletedAt,
	)
	return err
}

func (r *Repository) LoadTimeStandards(ctx context.Context) ([]*TimeStandard, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, name, COALESCE(standard_code, ''), COALESCE(description, ''), gender,
		       age_min, age_max, event_distance, stroke, course, time_ms, season_year,
		       is_active, created_at, updated_at
		FROM time_standards WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("performance: load time_standards: %w", err)
	}
	return dbutil.CollectRows(rows, "performance: scan time_standard", func(rows pgx.Rows) (*TimeStandard, error) {
		ts := &TimeStandard{}
		err := rows.Scan(
			&ts.ID, &ts.OrgID, &ts.Name, &ts.StandardCode, &ts.Description, &ts.Gender,
			&ts.AgeMin, &ts.AgeMax, &ts.EventDistance, &ts.Stroke, &ts.Course, &ts.TimeMs,
			&ts.SeasonYear, &ts.IsActive,
			&ts.CreatedAt, &ts.UpdatedAt,
		)
		return ts, err
	})
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
