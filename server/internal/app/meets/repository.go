package meets

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the meets domain.
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

// ─── Meets ────────────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeet(ctx context.Context, m Meet) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO meets (id, org_id, name, meet_type, course, venue, start_date, end_date,
		                   status, commitment_deadline, entry_deadline, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
		  name                = EXCLUDED.name,
		  meet_type           = EXCLUDED.meet_type,
		  course              = EXCLUDED.course,
		  venue               = EXCLUDED.venue,
		  start_date          = EXCLUDED.start_date,
		  end_date            = EXCLUDED.end_date,
		  status              = EXCLUDED.status,
		  commitment_deadline = EXCLUDED.commitment_deadline,
		  entry_deadline      = EXCLUDED.entry_deadline,
		  updated_at          = EXCLUDED.updated_at`,
		m.ID, m.OrgID, m.Name, m.MeetType, m.Course,
		nullableString(m.Location),
		nullableString(m.StartDate), nullableString(m.EndDate),
		m.Status,
		nullableString(m.CommitmentDeadline), nullableString(m.EntryDeadline),
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadMeets(ctx context.Context) ([]Meet, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, name, meet_type, course,
		       COALESCE(venue, ''), COALESCE(start_date, ''), COALESCE(end_date, ''),
		       status, COALESCE(commitment_deadline, ''), COALESCE(entry_deadline, ''),
		       created_at, updated_at
		FROM meets WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("meets: load meets: %w", err)
	}
	defer rows.Close()
	var out []Meet
	for rows.Next() {
		var m Meet
		if err := rows.Scan(
			&m.ID, &m.OrgID, &m.Name, &m.MeetType, &m.Course,
			&m.Location, &m.StartDate, &m.EndDate,
			&m.Status, &m.CommitmentDeadline, &m.EntryDeadline,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("meets: scan meet: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─── Meet Sessions ────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeetSession(ctx context.Context, s MeetSession) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO meet_sessions (id, org_id, meet_id, session_number, session_date, warm_up_time, start_time, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
		  session_number = EXCLUDED.session_number,
		  session_date   = EXCLUDED.session_date,
		  warm_up_time   = EXCLUDED.warm_up_time,
		  start_time     = EXCLUDED.start_time,
		  updated_at     = EXCLUDED.updated_at`,
		s.ID, s.OrgID, s.MeetID, s.SessionNumber,
		nullableString(s.SessionDate), nullableString(s.WarmupTime), nullableString(s.StartTime),
		s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadMeetSessions(ctx context.Context) ([]MeetSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, session_number,
		       COALESCE(session_date, ''), COALESCE(warm_up_time, ''), COALESCE(start_time, ''),
		       created_at, updated_at
		FROM meet_sessions`)
	if err != nil {
		return nil, fmt.Errorf("meets: load meet_sessions: %w", err)
	}
	defer rows.Close()
	var out []MeetSession
	for rows.Next() {
		var s MeetSession
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.MeetID, &s.SessionNumber,
			&s.SessionDate, &s.WarmupTime, &s.StartTime,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("meets: scan meet_session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ─── Meet Events ──────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeetEvent(ctx context.Context, e MeetEvent) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO meet_events (id, org_id, meet_id, session_id, event_number, distance, stroke, event_gender, age_group, course, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  event_number = EXCLUDED.event_number,
		  distance     = EXCLUDED.distance,
		  stroke       = EXCLUDED.stroke,
		  event_gender = EXCLUDED.event_gender,
		  course       = EXCLUDED.course,
		  updated_at   = EXCLUDED.updated_at`,
		e.ID, e.OrgID, e.MeetID, nullableString(e.SessionID),
		e.EventNumber, e.Distance, e.Stroke, e.Gender,
		nil, // age_group
		e.Course, e.CreatedAt, e.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadMeetEvents(ctx context.Context) ([]MeetEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text,
		       COALESCE(session_id::text, ''), event_number, distance, stroke,
		       COALESCE(event_gender, ''), COALESCE(course, ''),
		       created_at, updated_at
		FROM meet_events`)
	if err != nil {
		return nil, fmt.Errorf("meets: load meet_events: %w", err)
	}
	defer rows.Close()
	var out []MeetEvent
	for rows.Next() {
		var e MeetEvent
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.MeetID, &e.SessionID,
			&e.EventNumber, &e.Distance, &e.Stroke, &e.Gender, &e.Course,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("meets: scan meet_event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── Commitments ──────────────────────────────────────────────────────────────

func (r *Repository) UpsertCommitment(ctx context.Context, c Commitment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO commitments (id, org_id, meet_id, household_id, swimmer_id, status, commitment_response, responded_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
		  status              = EXCLUDED.status,
		  commitment_response = EXCLUDED.commitment_response,
		  responded_at        = EXCLUDED.responded_at,
		  updated_at          = EXCLUDED.updated_at`,
		c.ID, c.OrgID, c.MeetID, c.HouseholdID, c.SwimmerID,
		c.Status, nullableString(c.Notes), c.CommittedAt,
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadCommitments(ctx context.Context) ([]Commitment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, household_id::text, swimmer_id::text,
		       status, COALESCE(commitment_response, ''), responded_at,
		       created_at, updated_at
		FROM commitments`)
	if err != nil {
		return nil, fmt.Errorf("meets: load commitments: %w", err)
	}
	defer rows.Close()
	var out []Commitment
	for rows.Next() {
		var c Commitment
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.MeetID, &c.HouseholdID, &c.SwimmerID,
			&c.Status, &c.Notes, &c.CommittedAt,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("meets: scan commitment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ─── Entries ──────────────────────────────────────────────────────────────────

func (r *Repository) UpsertEntry(ctx context.Context, e Entry) error {
	var seedTime interface{}
	if e.SeedTimeMs != nil {
		seedTime = *e.SeedTimeMs
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO entries (id, org_id, meet_id, swimmer_id, meet_event_id, status, seed_time_ms, entry_time_ms, notes, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
		  status        = EXCLUDED.status,
		  seed_time_ms  = EXCLUDED.seed_time_ms,
		  entry_time_ms = EXCLUDED.entry_time_ms,
		  notes         = EXCLUDED.notes,
		  updated_at    = EXCLUDED.updated_at`,
		e.ID, e.OrgID, e.MeetID, e.SwimmerID, e.MeetEventID,
		e.Status, seedTime, nil,
		nullableString(e.OverrideReason),
		e.CreatedAt, e.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadEntries(ctx context.Context) ([]Entry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, swimmer_id::text, meet_event_id::text,
		       status, seed_time_ms, COALESCE(notes, ''),
		       created_at, updated_at
		FROM entries`)
	if err != nil {
		return nil, fmt.Errorf("meets: load entries: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var seedMs *int
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.MeetID, &e.SwimmerID, &e.MeetEventID,
			&e.Status, &seedMs, &e.OverrideReason,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("meets: scan entry: %w", err)
		}
		e.SeedTimeMs = seedMs
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	meets []Meet,
	sessions []MeetSession,
	events []MeetEvent,
	commitments []Commitment,
	entries []Entry,
	err error,
) {
	if meets, err = r.LoadMeets(ctx); err != nil {
		return
	}
	if sessions, err = r.LoadMeetSessions(ctx); err != nil {
		return
	}
	if events, err = r.LoadMeetEvents(ctx); err != nil {
		return
	}
	if commitments, err = r.LoadCommitments(ctx); err != nil {
		return
	}
	entries, err = r.LoadEntries(ctx)
	return
}
