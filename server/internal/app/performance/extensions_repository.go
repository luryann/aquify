package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Persistence for team records, swimmer goals, and relay lineups. Postgres is
// canonical: every write lands in the DB before the in-memory maps change.

func nullableDate(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// ─── Team record entries ─────────────────────────────────────────────────────

func (r *Repository) UpsertRecordEntry(ctx context.Context, e *RecordEntry) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO team_record_entries (id, org_id, course, gender, age_group, distance, stroke, time_ms,
		    swimmer_id, swimmer_name, swimmer_age, swim_date, meet_id, meet_name, result_id, source, status,
		    locked, verified, verified_by, verified_at, notes, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9::uuid, $10, $11, $12::date, $13::uuid, $14, $15::uuid, $16, $17,
		    $18, $19, $20, $21, $22, $23, $24, $25, $26)
		ON CONFLICT (id) DO UPDATE SET
		  time_ms      = EXCLUDED.time_ms,
		  swimmer_name = EXCLUDED.swimmer_name,
		  swimmer_age  = EXCLUDED.swimmer_age,
		  swim_date    = EXCLUDED.swim_date,
		  meet_name    = EXCLUDED.meet_name,
		  status       = EXCLUDED.status,
		  locked       = EXCLUDED.locked,
		  verified     = EXCLUDED.verified,
		  verified_by  = EXCLUDED.verified_by,
		  verified_at  = EXCLUDED.verified_at,
		  notes        = EXCLUDED.notes,
		  updated_at   = EXCLUDED.updated_at,
		  updated_by   = EXCLUDED.updated_by`,
		e.ID, e.OrgID, e.Course, e.Gender, e.AgeGroup, e.Distance, e.Stroke, e.TimeMs,
		nullableString(e.SwimmerID), e.SwimmerName, nullableInt(e.SwimmerAge), e.SwimDate,
		nullableString(e.MeetID), e.MeetName, nullableString(e.ResultID), e.Source, e.Status,
		e.Locked, e.Verified, e.VerifiedBy, e.VerifiedAt, e.Notes,
		e.CreatedAt, e.UpdatedAt, e.CreatedBy, e.UpdatedBy,
	)
	return err
}

func (r *Repository) DeleteRecordEntry(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM team_record_entries WHERE id = $1::uuid`, id)
	return err
}

func (r *Repository) LoadRecordEntries(ctx context.Context) ([]*RecordEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, course, gender, age_group, distance, stroke, time_ms,
		       COALESCE(swimmer_id::text, ''), swimmer_name, swimmer_age, to_char(swim_date, 'YYYY-MM-DD'),
		       COALESCE(meet_id::text, ''), meet_name, COALESCE(result_id::text, ''), source, status,
		       locked, verified, verified_by, verified_at, notes, created_at, updated_at, created_by, updated_by
		FROM team_record_entries`)
	if err != nil {
		return nil, fmt.Errorf("performance: load team_record_entries: %w", err)
	}
	defer rows.Close()
	var out []*RecordEntry
	for rows.Next() {
		e := &RecordEntry{}
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Course, &e.Gender, &e.AgeGroup, &e.Distance, &e.Stroke, &e.TimeMs,
			&e.SwimmerID, &e.SwimmerName, &e.SwimmerAge, &e.SwimDate,
			&e.MeetID, &e.MeetName, &e.ResultID, &e.Source, &e.Status,
			&e.Locked, &e.Verified, &e.VerifiedBy, &e.VerifiedAt, &e.Notes,
			&e.CreatedAt, &e.UpdatedAt, &e.CreatedBy, &e.UpdatedBy); err != nil {
			return nil, fmt.Errorf("performance: scan team_record_entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── Goals ───────────────────────────────────────────────────────────────────

func (r *Repository) UpsertGoal(ctx context.Context, g *Goal) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO swimmer_goals (id, org_id, swimmer_id, distance, stroke, course, target_time_ms,
		    baseline_time_ms, deadline, notes, status, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9::date, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
		  target_time_ms = EXCLUDED.target_time_ms,
		  deadline       = EXCLUDED.deadline,
		  notes          = EXCLUDED.notes,
		  status         = EXCLUDED.status,
		  updated_at     = EXCLUDED.updated_at,
		  updated_by     = EXCLUDED.updated_by`,
		g.ID, g.OrgID, g.SwimmerID, g.Distance, g.Stroke, g.Course, g.TargetTimeMs,
		nullableInt(g.BaselineTimeMs), nullableDate(g.Deadline), g.Notes, g.Status,
		g.CreatedAt, g.UpdatedAt, g.CreatedBy, g.UpdatedBy,
	)
	return err
}

func (r *Repository) DeleteGoal(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM swimmer_goals WHERE id = $1::uuid`, id)
	return err
}

func (r *Repository) LoadGoals(ctx context.Context) ([]*Goal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, swimmer_id::text, distance, stroke, course, target_time_ms, baseline_time_ms,
		       COALESCE(to_char(deadline, 'YYYY-MM-DD'), ''), notes, status, created_at, updated_at, created_by, updated_by
		FROM swimmer_goals`)
	if err != nil {
		return nil, fmt.Errorf("performance: load swimmer_goals: %w", err)
	}
	defer rows.Close()
	var out []*Goal
	for rows.Next() {
		g := &Goal{}
		if err := rows.Scan(&g.ID, &g.OrgID, &g.SwimmerID, &g.Distance, &g.Stroke, &g.Course, &g.TargetTimeMs, &g.BaselineTimeMs,
			&g.Deadline, &g.Notes, &g.Status, &g.CreatedAt, &g.UpdatedAt, &g.CreatedBy, &g.UpdatedBy); err != nil {
			return nil, fmt.Errorf("performance: scan swimmer_goal: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ─── Relay lineups ───────────────────────────────────────────────────────────

func (r *Repository) UpsertRelayLineup(ctx context.Context, l *RelayLineup) error {
	teams, err := json.Marshal(l.Teams)
	if err != nil {
		return fmt.Errorf("marshal relay teams: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO relay_lineups (id, org_id, meet_id, name, course, gender, age_group, relay_type, distance,
		    teams, notes, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
		  meet_id    = EXCLUDED.meet_id,
		  name       = EXCLUDED.name,
		  course     = EXCLUDED.course,
		  gender     = EXCLUDED.gender,
		  age_group  = EXCLUDED.age_group,
		  relay_type = EXCLUDED.relay_type,
		  distance   = EXCLUDED.distance,
		  teams      = EXCLUDED.teams,
		  notes      = EXCLUDED.notes,
		  updated_at = EXCLUDED.updated_at,
		  updated_by = EXCLUDED.updated_by`,
		l.ID, l.OrgID, l.MeetID, l.Name, l.Course, l.Gender, l.AgeGroup, l.RelayType, l.Distance,
		string(teams), l.Notes, l.CreatedAt, l.UpdatedAt, l.CreatedBy, l.UpdatedBy,
	)
	return err
}

func (r *Repository) DeleteRelayLineup(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM relay_lineups WHERE id = $1::uuid`, id)
	return err
}

func (r *Repository) LoadRelayLineups(ctx context.Context) ([]*RelayLineup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id, meet_id::text, name, course, gender, age_group, relay_type, distance,
		       teams::text, notes, created_at, updated_at, created_by, updated_by
		FROM relay_lineups`)
	if err != nil {
		return nil, fmt.Errorf("performance: load relay_lineups: %w", err)
	}
	defer rows.Close()
	var out []*RelayLineup
	for rows.Next() {
		l := &RelayLineup{}
		var teams string
		if err := rows.Scan(&l.ID, &l.OrgID, &l.MeetID, &l.Name, &l.Course, &l.Gender, &l.AgeGroup, &l.RelayType, &l.Distance,
			&teams, &l.Notes, &l.CreatedAt, &l.UpdatedAt, &l.CreatedBy, &l.UpdatedBy); err != nil {
			return nil, fmt.Errorf("performance: scan relay_lineup: %w", err)
		}
		if err := json.Unmarshal([]byte(teams), &l.Teams); err != nil {
			return nil, fmt.Errorf("performance: decode relay teams: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// loadExtensionsLocked hydrates records, goals and lineups. Caller holds s.mu.
func (s *Service) loadExtensionsLocked(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	entries, err := s.repo.LoadRecordEntries(ctx)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s.recordEntries[e.ID] = e
	}
	goals, err := s.repo.LoadGoals(ctx)
	if err != nil {
		return err
	}
	for _, g := range goals {
		s.goals[g.ID] = g
	}
	lineups, err := s.repo.LoadRelayLineups(ctx)
	if err != nil {
		return err
	}
	for _, l := range lineups {
		s.lineups[l.ID] = l
	}
	return nil
}
