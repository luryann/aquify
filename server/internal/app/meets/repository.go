package meets

import (
	"context"
	"fmt"
	"github.com/luryann/aquify/server/internal/dbutil"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// Repository handles DB persistence for the meets domain.
type Repository struct {
	pool *pgxpool.Pool
}

// at binds orgID to every statement so row-level security on the meets tables applies.
// An empty org sees nothing.
func (r *Repository) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: r.pool}.For(orgID) }

// OrgIDs lists every registered club (the registry is not tenant data) so an all-club
// load can read each club under its own binding.
func (r *Repository) OrgIDs(ctx context.Context) ([]string, error) {
	return dbutil.ListOrgIDs(ctx, r.pool)
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// ─── Meets ────────────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeet(ctx context.Context, m Meet) error {
	return upsertMeet(ctx, r.at(m.OrgID), m)
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
		dbutil.NullableString(m.StartDate), dbutil.NullableString(m.EndDate),
		dbutil.NullableString(m.EntryDeadline), dbutil.NullableString(m.CommitmentDeadline),
		strings.TrimSpace(m.HostOrg), strings.TrimSpace(m.Location),
		m.SelfSignupEnabled, m.MaxEntriesPerSwimmer,
		m.Status, m.CommitmentOpenAt, m.CommitmentClosedAt, m.EntryFinalizedAt,
		m.CreatedAt, m.UpdatedAt,
		dbutil.NullableString(m.CreatedBy), dbutil.NullableString(m.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadMeets(ctx context.Context, orgID string) ([]Meet, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, name, meet_type, course,
		       COALESCE(start_date::text, ''), COALESCE(end_date::text, ''),
		       COALESCE(entry_deadline::text, ''), COALESCE(commitment_deadline::text, ''),
		       COALESCE(host_org, ''), COALESCE(location, ''),
		       self_signup_enabled, COALESCE(max_entries_per_swimmer, 0),
		       status, commitment_open_at, commitment_closed_at, entry_finalized_at,
		       created_at, updated_at,
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
		FROM meets WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load meets: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan meet", func(rows pgx.Rows) (Meet, error) {
		var m Meet
		err := rows.Scan(
			&m.ID, &m.OrgID, &m.Name, &m.MeetType, &m.Course,
			&m.StartDate, &m.EndDate,
			&m.EntryDeadline, &m.CommitmentDeadline,
			&m.HostOrg, &m.Location,
			&m.SelfSignupEnabled, &m.MaxEntriesPerSwimmer,
			&m.Status, &m.CommitmentOpenAt, &m.CommitmentClosedAt, &m.EntryFinalizedAt,
			&m.CreatedAt, &m.UpdatedAt,
			&m.CreatedBy, &m.UpdatedBy,
		)
		return m, err
	})
}

// ─── Meet Sessions ────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeetSession(ctx context.Context, s MeetSession) error {
	return upsertMeetSession(ctx, r.at(s.OrgID), s)
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
		dbutil.NullableString(s.SessionDate), dbutil.NullableString(s.WarmupTime), dbutil.NullableString(s.StartTime), s.Notes,
		s.CreatedAt, s.UpdatedAt,
		dbutil.NullableString(s.CreatedBy), dbutil.NullableString(s.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadMeetSessions(ctx context.Context, orgID string) ([]MeetSession, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, session_number,
		       name, COALESCE(session_date::text, ''), COALESCE(warm_up_time::text, ''), COALESCE(start_time::text, ''),
		       COALESCE(notes, ''), created_at, updated_at,
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
		FROM meet_sessions
		WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load meet_sessions: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan meet_session", func(rows pgx.Rows) (MeetSession, error) {
		var s MeetSession
		err := rows.Scan(
			&s.ID, &s.OrgID, &s.MeetID, &s.SessionNumber,
			&s.Name, &s.SessionDate, &s.WarmupTime, &s.StartTime,
			&s.Notes, &s.CreatedAt, &s.UpdatedAt,
			&s.CreatedBy, &s.UpdatedBy,
		)
		return s, err
	})
}

// ─── Meet Events ──────────────────────────────────────────────────────────────

func (r *Repository) UpsertMeetEvent(ctx context.Context, e MeetEvent) error {
	return upsertMeetEvent(ctx, r.at(e.OrgID), e)
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
		e.ID, e.OrgID, e.MeetID, dbutil.NullableString(e.SessionID),
		e.EventNumber, e.Stroke, e.Distance, e.Course, e.Gender,
		e.MinAge, e.MaxAge, e.EntryFeeCents, e.IsRelay,
		e.CreatedAt, e.UpdatedAt,
		dbutil.NullableString(e.CreatedBy), dbutil.NullableString(e.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadMeetEvents(ctx context.Context, orgID string) ([]MeetEvent, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text,
		       COALESCE(session_id::text, ''), event_number, distance, stroke,
		       COALESCE(gender, ''), COALESCE(course, ''), min_age, max_age,
		       entry_fee_cents, is_relay, created_at, updated_at,
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
		FROM meet_events
		WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load meet_events: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan meet_event", func(rows pgx.Rows) (MeetEvent, error) {
		var e MeetEvent
		err := rows.Scan(
			&e.ID, &e.OrgID, &e.MeetID, &e.SessionID,
			&e.EventNumber, &e.Distance, &e.Stroke, &e.Gender, &e.Course,
			&e.MinAge, &e.MaxAge, &e.EntryFeeCents, &e.IsRelay,
			&e.CreatedAt, &e.UpdatedAt, &e.CreatedBy, &e.UpdatedBy,
		)
		return e, err
	})
}

func (r *Repository) CreateMeetBundle(ctx context.Context, meet Meet, sessions []MeetSession, events []MeetEvent) error {
	tx, err := r.at(meet.OrgID).Begin(ctx)
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
	_, err := r.at(c.OrgID).Exec(ctx, `
		INSERT INTO commitments (id, org_id, meet_id, household_id, swimmer_id, status, notes, committed_at,
		                         coach_reviewed_at, locked_at, created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10, $11, $12, $13::uuid, $14::uuid)
		ON CONFLICT (id) DO UPDATE SET
		  status            = EXCLUDED.status,
		  notes             = EXCLUDED.notes,
		  committed_at      = EXCLUDED.committed_at,
		  coach_reviewed_at = EXCLUDED.coach_reviewed_at,
		  locked_at         = EXCLUDED.locked_at,
		  updated_at        = EXCLUDED.updated_at,
		  updated_by        = EXCLUDED.updated_by`,
		c.ID, c.OrgID, c.MeetID, c.HouseholdID, c.SwimmerID,
		c.Status, c.Notes, c.CommittedAt, c.CoachReviewedAt, c.LockedAt,
		c.CreatedAt, c.UpdatedAt,
		dbutil.NullableString(c.CreatedBy), dbutil.NullableString(c.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadCommitments(ctx context.Context, orgID string) ([]Commitment, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, household_id::text, swimmer_id::text,
		       status, COALESCE(notes, ''), committed_at, coach_reviewed_at, locked_at,
		       created_at, updated_at, COALESCE(created_by::text, ''), COALESCE(updated_by::text, '')
		FROM commitments WHERE ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load commitments: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan commitment", func(rows pgx.Rows) (Commitment, error) {
		var c Commitment
		err := rows.Scan(
			&c.ID, &c.OrgID, &c.MeetID, &c.HouseholdID, &c.SwimmerID,
			&c.Status, &c.Notes, &c.CommittedAt, &c.CoachReviewedAt, &c.LockedAt,
			&c.CreatedAt, &c.UpdatedAt, &c.CreatedBy, &c.UpdatedBy,
		)
		return c, err
	})
}

// ─── Event selections ─────────────────────────────────────────────────────────

func (r *Repository) UpsertEventSelection(ctx context.Context, sel EventSelection) error {
	var seedTime interface{}
	if sel.SeedTimeMs != nil {
		seedTime = *sel.SeedTimeMs
	}
	_, err := r.at(sel.OrgID).Exec(ctx, `
		INSERT INTO commitment_event_selections (id, org_id, commitment_id, meet_event_id, seed_time_ms, created_at, created_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7::uuid)
		ON CONFLICT (id) DO UPDATE SET
		  seed_time_ms = EXCLUDED.seed_time_ms`,
		sel.ID, sel.OrgID, sel.CommitmentID, sel.MeetEventID, seedTime, sel.CreatedAt, dbutil.NullableString(sel.CreatedBy),
	)
	return err
}

func (r *Repository) DeleteEventSelection(ctx context.Context, orgID, id string) error {
	_, err := r.at(orgID).Exec(ctx, `DELETE FROM commitment_event_selections WHERE id = $1::uuid AND org_id = $2::uuid`, id, orgID)
	return err
}

func (r *Repository) LoadEventSelections(ctx context.Context, orgID string) ([]EventSelection, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, commitment_id::text, meet_event_id::text, seed_time_ms, created_at, COALESCE(created_by::text, '')
		FROM commitment_event_selections WHERE ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load event selections: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan event selection", func(rows pgx.Rows) (EventSelection, error) {
		var sel EventSelection
		var seedTime *int
		if err := rows.Scan(&sel.ID, &sel.OrgID, &sel.CommitmentID, &sel.MeetEventID, &seedTime, &sel.CreatedAt, &sel.CreatedBy); err != nil {
			return sel, err
		}
		sel.SeedTimeMs = seedTime
		return sel, nil
	})
}

// ─── Entries ──────────────────────────────────────────────────────────────────

func (r *Repository) UpsertEntry(ctx context.Context, e Entry) error {
	// Bound to the entry's org: the sub-selects below read users, which row-level security
	// hides from an unbound session (the actor columns would silently become NULL).
	return upsertEntry(ctx, tenantdb.Pool{P: r.pool}.For(e.OrgID), e)
}

func upsertEntry(ctx context.Context, db execer, e Entry) error {
	var seedTime interface{}
	if e.SeedTimeMs != nil {
		seedTime = *e.SeedTimeMs
	}
	// entry_time_ms is intentionally not written: the service does not model
	// it, and listing it in the conflict clause used to reset any stored value
	// to NULL on every update.
	//
	// commitment_id / actor columns are FKs; they are resolved through
	// sub-selects so an unknown or non-UUID value (stub principals, deleted
	// users or commitments) stores NULL instead of failing the whole write.
	// NOTE: untested against Postgres (no database available in the review
	// environment).
	_, err := db.Exec(ctx, `
		INSERT INTO entries (id, org_id, meet_id, swimmer_id, meet_event_id, status, seed_time_ms, notes, created_at, updated_at,
		                     commitment_id, is_relay, proposed_by, approved_by, created_by, updated_by, scratch_reason)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10,
		        (SELECT id FROM commitments WHERE id::text = NULLIF($11::text, '')),
		        $12,
		        (SELECT id FROM users WHERE id::text = NULLIF($13::text, '')),
		        (SELECT id FROM users WHERE id::text = NULLIF($14::text, '')),
		        (SELECT id FROM users WHERE id::text = NULLIF($15::text, '')),
		        (SELECT id FROM users WHERE id::text = NULLIF($16::text, '')),
		        $17)
		ON CONFLICT (id) DO UPDATE SET
		  meet_event_id  = EXCLUDED.meet_event_id,
		  status         = EXCLUDED.status,
		  seed_time_ms   = EXCLUDED.seed_time_ms,
		  notes          = EXCLUDED.notes,
		  commitment_id  = EXCLUDED.commitment_id,
		  is_relay       = EXCLUDED.is_relay,
		  proposed_by    = EXCLUDED.proposed_by,
		  approved_by    = EXCLUDED.approved_by,
		  updated_by     = EXCLUDED.updated_by,
		  scratch_reason = EXCLUDED.scratch_reason,
		  updated_at     = EXCLUDED.updated_at`,
		e.ID, e.OrgID, e.MeetID, e.SwimmerID, e.MeetEventID,
		e.Status, seedTime,
		e.OverrideReason,
		e.CreatedAt, e.UpdatedAt,
		e.CommitmentID, e.IsRelay, e.ProposedBy, e.ApprovedBy, e.CreatedBy, e.UpdatedBy,
		e.ScratchReason,
	)
	return err
}

func (r *Repository) LoadEntries(ctx context.Context, orgID string) ([]Entry, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, meet_id::text, swimmer_id::text, meet_event_id::text,
		       status, seed_time_ms, COALESCE(notes, ''),
		       COALESCE(commitment_id::text, ''), is_relay,
		       COALESCE(proposed_by::text, ''), COALESCE(approved_by::text, ''),
		       COALESCE(created_by::text, ''), COALESCE(updated_by::text, ''),
		       COALESCE(scratch_reason, ''),
		       created_at, updated_at
		FROM entries WHERE ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load entries: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan entry", func(rows pgx.Rows) (Entry, error) {
		var e Entry
		var seedMs *int
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.MeetID, &e.SwimmerID, &e.MeetEventID,
			&e.Status, &seedMs, &e.OverrideReason,
			&e.CommitmentID, &e.IsRelay,
			&e.ProposedBy, &e.ApprovedBy, &e.CreatedBy, &e.UpdatedBy,
			&e.ScratchReason,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return e, err
		}
		e.SeedTimeMs = seedMs
		return e, nil
	})
}

// ─── Batch load ───────────────────────────────────────────────────────────────

// LoadAll loads one club (orgID set) or, with an empty orgID, every club one at a time
// under that club's own binding (an unbound read would see nothing under RLS).
func (r *Repository) LoadAll(ctx context.Context, orgID string) (
	meets []Meet,
	sessions []MeetSession,
	events []MeetEvent,
	commitments []Commitment,
	entries []Entry,
	swimmers []swimmerRef,
	households []householdRef,
	selections []EventSelection,
	err error,
) {
	if orgID != "" {
		return r.loadOrg(ctx, orgID)
	}
	orgs, err := r.OrgIDs(ctx)
	if err != nil {
		return
	}
	for _, o := range orgs {
		m, se, ev, co, en, sw, hh, sel, lerr := r.loadOrg(ctx, o)
		if lerr != nil {
			err = lerr
			return
		}
		meets = append(meets, m...)
		sessions = append(sessions, se...)
		events = append(events, ev...)
		commitments = append(commitments, co...)
		entries = append(entries, en...)
		swimmers = append(swimmers, sw...)
		households = append(households, hh...)
		selections = append(selections, sel...)
	}
	return
}

func (r *Repository) loadOrg(ctx context.Context, orgID string) (
	meets []Meet,
	sessions []MeetSession,
	events []MeetEvent,
	commitments []Commitment,
	entries []Entry,
	swimmers []swimmerRef,
	households []householdRef,
	selections []EventSelection,
	err error,
) {
	if meets, err = r.LoadMeets(ctx, orgID); err != nil {
		return
	}
	if sessions, err = r.LoadMeetSessions(ctx, orgID); err != nil {
		return
	}
	if events, err = r.LoadMeetEvents(ctx, orgID); err != nil {
		return
	}
	if commitments, err = r.LoadCommitments(ctx, orgID); err != nil {
		return
	}
	if entries, err = r.LoadEntries(ctx, orgID); err != nil {
		return
	}
	if swimmers, err = r.LoadSwimmerRefs(ctx, orgID); err != nil {
		return
	}
	if households, err = r.LoadHouseholdRefs(ctx, orgID); err != nil {
		return
	}
	selections, err = r.LoadEventSelections(ctx, orgID)
	return
}

func (r *Repository) LoadSwimmerRefs(ctx context.Context, orgID string) ([]swimmerRef, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text
		FROM swimmers
		WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load swimmer refs: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan swimmer ref", func(rows pgx.Rows) (swimmerRef, error) {
		var swimmer swimmerRef
		err := rows.Scan(&swimmer.ID, &swimmer.OrgID, &swimmer.HouseholdID)
		return swimmer, err
	})
}

func (r *Repository) LoadHouseholdRefs(ctx context.Context, orgID string) ([]householdRef, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text
		FROM households
		WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("meets: load household refs: %w", err)
	}
	return dbutil.CollectRows(rows, "meets: scan household ref", func(rows pgx.Rows) (householdRef, error) {
		var household householdRef
		err := rows.Scan(&household.ID, &household.OrgID)
		return household, err
	})
}
