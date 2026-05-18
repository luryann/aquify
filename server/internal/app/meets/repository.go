package meets

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// ─── Meets ────────────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeet(ctx context.Context, m Meet) error {
	return upsertMeet(ctx, r.pool, m)
}

func upsertMeet(ctx context.Context, db execer, m Meet) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meets (
			id, org_id, name, meet_type, course, start_date, end_date,
			entry_deadline, commitment_deadline, host_org, location,
			self_signup_enabled, max_entries_per_swimmer,
			status, commitment_open_at, commitment_closed_at, entry_finalized_at,
			created_at, updated_at, created_by, updated_by
		)
		VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6, $7,
			$8, $9, $10, $11,
			$12, $13,
			$14, $15, $16, $17,
			$18, $19, $20::uuid, $21::uuid
		)
		ON CONFLICT (id) DO UPDATE SET
		  name                    = EXCLUDED.name,
		  meet_type               = EXCLUDED.meet_type,
		  course                  = EXCLUDED.course,
		  start_date              = EXCLUDED.start_date,
		  end_date                = EXCLUDED.end_date,
		  entry_deadline          = EXCLUDED.entry_deadline,
		  commitment_deadline     = EXCLUDED.commitment_deadline,
		  host_org                = EXCLUDED.host_org,
		  location                = EXCLUDED.location,
		  self_signup_enabled     = EXCLUDED.self_signup_enabled,
		  max_entries_per_swimmer = EXCLUDED.max_entries_per_swimmer,
		  status                  = EXCLUDED.status,
		  commitment_open_at      = EXCLUDED.commitment_open_at,
		  commitment_closed_at    = EXCLUDED.commitment_closed_at,
		  entry_finalized_at      = EXCLUDED.entry_finalized_at,
		  updated_at              = EXCLUDED.updated_at,
		  updated_by              = EXCLUDED.updated_by`,
		m.ID, m.OrgID, m.Name, m.MeetType, m.Course,
		nullableString(m.StartDate), nullableString(m.EndDate),
		nullableString(m.EntryDeadline), nullableString(m.CommitmentDeadline),
		strings.TrimSpace(m.HostOrg), strings.TrimSpace(m.Location),
		m.SelfSignupEnabled, m.MaxEntriesPerSwimmer,
		m.Status, m.CommitmentOpenAt, m.CommitmentClosedAt, m.EntryFinalizedAt,
		m.CreatedAt, m.UpdatedAt,
		nullableString(m.CreatedBy), nullableString(m.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadMeets(ctx context.Context) ([]Meet, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, name, meet_type, course,
		       COALESCE(start_date::text, ''), COALESCE(end_date::text, ''),
		       COALESCE(entry_deadline::text, ''), COALESCE(commitment_deadline::text, ''),
		       COALESCE(host_org, ''), COALESCE(location, ''),
		       self_signup_enabled, COALESCE(max_entries_per_swimmer, 0),
		       status, commitment_open_at, commitment_closed_at, entry_finalized_at,
		       created_at, updated_at,
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
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
			&m.StartDate, &m.EndDate,
			&m.EntryDeadline, &m.CommitmentDeadline,
			&m.HostOrg, &m.Location,
			&m.SelfSignupEnabled, &m.MaxEntriesPerSwimmer,
			&m.Status, &m.CommitmentOpenAt, &m.CommitmentClosedAt, &m.EntryFinalizedAt,
			&m.CreatedAt, &m.UpdatedAt,
			&m.CreatedBy, &m.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("meets: scan meet: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─── Meet Sessions ────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeetSession(ctx context.Context, s MeetSession) error {
	return upsertMeetSession(ctx, r.pool, s)
}

func upsertMeetSession(ctx context.Context, db execer, s MeetSession) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meet_sessions (
			id, org_id, meet_id, session_number, name, session_date, warm_up_time, start_time, notes,
			created_at, updated_at, created_by, updated_by
		)
		VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9,
			$10, $11, $12::uuid, $13::uuid
		)
		ON CONFLICT (id) DO UPDATE SET
		  session_number = EXCLUDED.session_number,
		  name           = EXCLUDED.name,
		  session_date   = EXCLUDED.session_date,
		  warm_up_time   = EXCLUDED.warm_up_time,
		  start_time     = EXCLUDED.start_time,
		  notes          = EXCLUDED.notes,
		  updated_at     = EXCLUDED.updated_at,
		  updated_by     = EXCLUDED.updated_by`,
		s.ID, s.OrgID, s.MeetID, s.SessionNumber, s.Name,
		nullableString(s.SessionDate), nullableString(s.WarmupTime), nullableString(s.StartTime), s.Notes,
		s.CreatedAt, s.UpdatedAt,
		nullableString(s.CreatedBy), nullableString(s.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadMeetSessions(ctx context.Context) ([]MeetSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, session_number,
		       name, COALESCE(session_date::text, ''), COALESCE(warm_up_time::text, ''), COALESCE(start_time::text, ''),
		       COALESCE(notes, ''), created_at, updated_at,
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
		FROM meet_sessions
		WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("meets: load meet_sessions: %w", err)
	}
	defer rows.Close()
	var out []MeetSession
	for rows.Next() {
		var s MeetSession
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.MeetID, &s.SessionNumber,
			&s.Name, &s.SessionDate, &s.WarmupTime, &s.StartTime,
			&s.Notes, &s.CreatedAt, &s.UpdatedAt,
			&s.CreatedBy, &s.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("meets: scan meet_session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ─── Meet Events ──────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeetEvent(ctx context.Context, e MeetEvent) error {
	return upsertMeetEvent(ctx, r.pool, e)
}

func upsertMeetEvent(ctx context.Context, db execer, e MeetEvent) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meet_events (
			id, org_id, meet_id, session_id, event_number, stroke, distance, course, gender,
			min_age, max_age, entry_fee_cents, is_relay,
			created_at, updated_at, created_by, updated_by
		)
		VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16::uuid, $17::uuid
		)
		ON CONFLICT (id) DO UPDATE SET
		  session_id       = EXCLUDED.session_id,
		  event_number     = EXCLUDED.event_number,
		  stroke           = EXCLUDED.stroke,
		  distance         = EXCLUDED.distance,
		  course           = EXCLUDED.course,
		  gender           = EXCLUDED.gender,
		  min_age          = EXCLUDED.min_age,
		  max_age          = EXCLUDED.max_age,
		  entry_fee_cents  = EXCLUDED.entry_fee_cents,
		  is_relay         = EXCLUDED.is_relay,
		  updated_at       = EXCLUDED.updated_at,
		  updated_by       = EXCLUDED.updated_by`,
		e.ID, e.OrgID, e.MeetID, nullableString(e.SessionID),
		e.EventNumber, e.Stroke, e.Distance, e.Course, e.Gender,
		e.MinAge, e.MaxAge, e.EntryFeeCents, e.IsRelay,
		e.CreatedAt, e.UpdatedAt,
		nullableString(e.CreatedBy), nullableString(e.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadMeetEvents(ctx context.Context) ([]MeetEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text,
		       COALESCE(session_id::text, ''), event_number, distance, stroke,
		       COALESCE(gender, ''), COALESCE(course, ''), min_age, max_age,
		       entry_fee_cents, is_relay, created_at, updated_at,
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
		FROM meet_events
		WHERE deleted_at IS NULL`)
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
			&e.MinAge, &e.MaxAge, &e.EntryFeeCents, &e.IsRelay,
			&e.CreatedAt, &e.UpdatedAt, &e.CreatedBy, &e.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("meets: scan meet_event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) CreateMeetBundle(ctx context.Context, meet Meet, sessions []MeetSession, events []MeetEvent) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin meet bundle transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := upsertMeet(ctx, tx, meet); err != nil {
		return err
	}
	for _, session := range sessions {
		if err := upsertMeetSession(ctx, tx, session); err != nil {
			return err
		}
	}
	for _, event := range events {
		if err := upsertMeetEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit meet bundle transaction: %w", err)
	}
	return nil
}

// ─── Commitments ──────────────────────────────────────────────────────────────

func (r *Repository) UpsertCommitment(ctx context.Context, c Commitment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO commitments (id, org_id, meet_id, household_id, swimmer_id, status, notes, committed_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
		  status       = EXCLUDED.status,
		  notes        = EXCLUDED.notes,
		  committed_at = EXCLUDED.committed_at,
		  updated_at   = EXCLUDED.updated_at`,
		c.ID, c.OrgID, c.MeetID, c.HouseholdID, c.SwimmerID,
		c.Status, nullableString(c.Notes), c.CommittedAt,
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadCommitments(ctx context.Context) ([]Commitment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, household_id::text, swimmer_id::text,
		       status, COALESCE(notes, ''), committed_at,
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
	swimmers []swimmerRef,
	households []householdRef,
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
	if entries, err = r.LoadEntries(ctx); err != nil {
		return
	}
	if swimmers, err = r.LoadSwimmerRefs(ctx); err != nil {
		return
	}
	households, err = r.LoadHouseholdRefs(ctx)
	return
}

func (r *Repository) LoadSwimmerRefs(ctx context.Context) ([]swimmerRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text
		FROM swimmers
		WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("meets: load swimmer refs: %w", err)
	}
	defer rows.Close()

	var out []swimmerRef
	for rows.Next() {
		var swimmer swimmerRef
		if err := rows.Scan(&swimmer.ID, &swimmer.OrgID, &swimmer.HouseholdID); err != nil {
			return nil, fmt.Errorf("meets: scan swimmer ref: %w", err)
		}
		out = append(out, swimmer)
	}
	return out, rows.Err()
}

func (r *Repository) LoadHouseholdRefs(ctx context.Context) ([]householdRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text
		FROM households
		WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("meets: load household refs: %w", err)
	}
	defer rows.Close()

	var out []householdRef
	for rows.Next() {
		var household householdRef
		if err := rows.Scan(&household.ID, &household.OrgID); err != nil {
			return nil, fmt.Errorf("meets: scan household ref: %w", err)
		}
		out = append(out, household)
	}
	return out, rows.Err()
}
