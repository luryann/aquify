package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/luryann/aquify/server/internal/dbutil"
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
	_, err := r.at(e.OrgID).Exec(ctx, `
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
		dbutil.NullableString(e.SwimmerID), e.SwimmerName, nullableInt(e.SwimmerAge), e.SwimDate,
		dbutil.NullableString(e.MeetID), e.MeetName, dbutil.NullableString(e.ResultID), e.Source, e.Status,
		e.Locked, e.Verified, e.VerifiedBy, e.VerifiedAt, e.Notes,
		e.CreatedAt, e.UpdatedAt, e.CreatedBy, e.UpdatedBy,
	)
	return err
}

func (r *Repository) DeleteRecordEntry(ctx context.Context, orgID, id string) error {
	_, err := r.at(orgID).Exec(ctx, `DELETE FROM team_record_entries WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	return err
}

func (r *Repository) LoadRecordEntries(ctx context.Context, orgID string) ([]*RecordEntry, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id, course, gender, age_group, distance, stroke, time_ms,
		       COALESCE(swimmer_id::text, ''), swimmer_name, swimmer_age, to_char(swim_date, 'YYYY-MM-DD'),
		       COALESCE(meet_id::text, ''), meet_name, COALESCE(result_id::text, ''), source, status,
		       locked, verified, verified_by, verified_at, notes, created_at, updated_at, created_by, updated_by
		FROM team_record_entries WHERE org_id = $1::uuid`, orgID)
	if err != nil {
		return nil, fmt.Errorf("performance: load team_record_entries: %w", err)
	}
	return dbutil.CollectRows(rows, "performance: scan team_record_entry", func(rows pgx.Rows) (*RecordEntry, error) {
		e := &RecordEntry{}
		err := rows.Scan(&e.ID, &e.OrgID, &e.Course, &e.Gender, &e.AgeGroup, &e.Distance, &e.Stroke, &e.TimeMs,
			&e.SwimmerID, &e.SwimmerName, &e.SwimmerAge, &e.SwimDate,
			&e.MeetID, &e.MeetName, &e.ResultID, &e.Source, &e.Status,
			&e.Locked, &e.Verified, &e.VerifiedBy, &e.VerifiedAt, &e.Notes,
			&e.CreatedAt, &e.UpdatedAt, &e.CreatedBy, &e.UpdatedBy)
		return e, err
	})
}

// ─── Goals ───────────────────────────────────────────────────────────────────

func (r *Repository) UpsertGoal(ctx context.Context, g *Goal) error {
	_, err := r.at(g.OrgID).Exec(ctx, `
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

func (r *Repository) DeleteGoal(ctx context.Context, orgID, id string) error {
	_, err := r.at(orgID).Exec(ctx, `DELETE FROM swimmer_goals WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	return err
}

func (r *Repository) LoadGoals(ctx context.Context, orgID string) ([]*Goal, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id, swimmer_id::text, distance, stroke, course, target_time_ms, baseline_time_ms,
		       COALESCE(to_char(deadline, 'YYYY-MM-DD'), ''), notes, status, created_at, updated_at, created_by, updated_by
		FROM swimmer_goals WHERE org_id = $1::uuid`, orgID)
	if err != nil {
		return nil, fmt.Errorf("performance: load swimmer_goals: %w", err)
	}
	return dbutil.CollectRows(rows, "performance: scan swimmer_goal", func(rows pgx.Rows) (*Goal, error) {
		g := &Goal{}
		err := rows.Scan(&g.ID, &g.OrgID, &g.SwimmerID, &g.Distance, &g.Stroke, &g.Course, &g.TargetTimeMs, &g.BaselineTimeMs,
			&g.Deadline, &g.Notes, &g.Status, &g.CreatedAt, &g.UpdatedAt, &g.CreatedBy, &g.UpdatedBy)
		return g, err
	})
}

// ─── Relay lineups ───────────────────────────────────────────────────────────

func (r *Repository) UpsertRelayLineup(ctx context.Context, l *RelayLineup) error {
	teams, err := json.Marshal(l.Teams)
	if err != nil {
		return fmt.Errorf("marshal relay teams: %w", err)
	}
	_, err = r.at(l.OrgID).Exec(ctx, `
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

func (r *Repository) DeleteRelayLineup(ctx context.Context, orgID, id string) error {
	_, err := r.at(orgID).Exec(ctx, `DELETE FROM relay_lineups WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	return err
}

func (r *Repository) LoadRelayLineups(ctx context.Context, orgID string) ([]*RelayLineup, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id, meet_id::text, name, course, gender, age_group, relay_type, distance,
		       teams::text, notes, created_at, updated_at, created_by, updated_by
		FROM relay_lineups WHERE org_id = $1::uuid`, orgID)
	if err != nil {
		return nil, fmt.Errorf("performance: load relay_lineups: %w", err)
	}
	return dbutil.CollectRows(rows, "performance: scan relay_lineup", func(rows pgx.Rows) (*RelayLineup, error) {
		l := &RelayLineup{}
		var teams string
		if err := rows.Scan(&l.ID, &l.OrgID, &l.MeetID, &l.Name, &l.Course, &l.Gender, &l.AgeGroup, &l.RelayType, &l.Distance,
			&teams, &l.Notes, &l.CreatedAt, &l.UpdatedAt, &l.CreatedBy, &l.UpdatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(teams), &l.Teams); err != nil {
			return nil, fmt.Errorf("decode relay teams: %w", err)
		}
		return l, nil
	})
}

// clubExtensions is one club's records, goals and relay lineups as read from Postgres.
type clubExtensions struct {
	entries []*RecordEntry
	goals   []*Goal
	lineups []*RelayLineup
}

// loadExtensions reads one club's records, goals and relay lineups.
func (r *Repository) loadExtensions(ctx context.Context, orgID string) (clubExtensions, error) {
	var out clubExtensions
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	if out.entries, err = r.LoadRecordEntries(ctx, orgID); err != nil {
		return out, err
	}
	if out.goals, err = r.LoadGoals(ctx, orgID); err != nil {
		return out, err
	}
	out.lineups, err = r.LoadRelayLineups(ctx, orgID)
	return out, err
}
