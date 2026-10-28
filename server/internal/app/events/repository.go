package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists club events, RSVPs and calendar feed tokens. Postgres
// is canonical in DB mode: every read goes to the database so concurrent API
// instances observe each other's writes immediately.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const eventColumns = `id::text, org_id, title, category, description, location,
	starts_at, ends_at, all_day, audience, group_codes, rsvp_enabled, rsvp_mode,
	rsvp_deadline, capacity, fee_cents, status, created_at, updated_at, created_by, updated_by, is_public`

func scanEvent(row pgx.Row) (*Event, error) {
	e := &Event{}
	var capacity *int32
	if err := row.Scan(
		&e.ID, &e.OrgID, &e.Title, &e.Category, &e.Description, &e.Location,
		&e.StartsAt, &e.EndsAt, &e.AllDay, &e.Audience, &e.GroupCodes, &e.RSVPEnabled, &e.RSVPMode,
		&e.RSVPDeadline, &capacity, &e.FeeCents, &e.Status, &e.CreatedAt, &e.UpdatedAt, &e.CreatedBy, &e.UpdatedBy, &e.Public,
	); err != nil {
		return nil, err
	}
	if capacity != nil {
		c := int(*capacity)
		e.Capacity = &c
	}
	if e.GroupCodes == nil {
		e.GroupCodes = []string{}
	}
	e.StartsAt = e.StartsAt.UTC()
	if e.EndsAt != nil {
		t := e.EndsAt.UTC()
		e.EndsAt = &t
	}
	if e.RSVPDeadline != nil {
		t := e.RSVPDeadline.UTC()
		e.RSVPDeadline = &t
	}
	return e, nil
}

// UpsertEvent inserts or replaces an event row.
func (r *Repository) UpsertEvent(ctx context.Context, e *Event) error {
	var capacity any
	if e.Capacity != nil {
		capacity = *e.Capacity
	}
	codes := e.GroupCodes
	if codes == nil {
		codes = []string{}
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO club_events (
			id, org_id, title, category, description, location, starts_at, ends_at, all_day,
			audience, group_codes, rsvp_enabled, rsvp_mode, rsvp_deadline, capacity, fee_cents,
			status, created_at, updated_at, created_by, updated_by, is_public
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22
		)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title, category = EXCLUDED.category, description = EXCLUDED.description,
			location = EXCLUDED.location, starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at,
			all_day = EXCLUDED.all_day, audience = EXCLUDED.audience, group_codes = EXCLUDED.group_codes,
			rsvp_enabled = EXCLUDED.rsvp_enabled, rsvp_mode = EXCLUDED.rsvp_mode,
			rsvp_deadline = EXCLUDED.rsvp_deadline, capacity = EXCLUDED.capacity,
			fee_cents = EXCLUDED.fee_cents, status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by,
			is_public = EXCLUDED.is_public
		WHERE club_events.org_id = EXCLUDED.org_id`,
		e.ID, e.OrgID, e.Title, e.Category, e.Description, e.Location, e.StartsAt, e.EndsAt, e.AllDay,
		e.Audience, codes, e.RSVPEnabled, e.RSVPMode, e.RSVPDeadline, capacity, e.FeeCents,
		e.Status, e.CreatedAt, e.UpdatedAt, e.CreatedBy, e.UpdatedBy, e.Public,
	)
	if err != nil {
		return fmt.Errorf("club events: upsert event: %w", err)
	}
	return nil
}

// GetEvent loads one event scoped to org.
func (r *Repository) GetEvent(ctx context.Context, orgID, id string) (*Event, error) {
	if _, err := parseUUID(id); err != nil {
		return nil, ErrNotFound
	}
	e, err := scanEvent(r.pool.QueryRow(ctx, `SELECT `+eventColumns+` FROM club_events WHERE id = $1::uuid AND org_id = $2`, id, orgID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("club events: get event: %w", err)
	}
	return e, nil
}

// ListEvents loads all events for an org.
func (r *Repository) ListEvents(ctx context.Context, orgID string) ([]*Event, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+eventColumns+` FROM club_events WHERE org_id = $1 ORDER BY starts_at, id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("club events: list events: %w", err)
	}
	defer rows.Close()
	out := []*Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("club events: scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteEvent removes an event; RSVPs cascade.
func (r *Repository) DeleteEvent(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM club_events WHERE id = $1::uuid AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("club events: delete event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const rsvpColumns = `id::text, org_id, event_id::text, household_id, swimmer_id, response, guest_count,
	note, invoice_id, billed_quantity, responded_by, created_at, updated_at`

func scanRSVP(row pgx.Row) (*RSVP, error) {
	r := &RSVP{}
	if err := row.Scan(&r.ID, &r.OrgID, &r.EventID, &r.HouseholdID, &r.SwimmerID, &r.Response, &r.GuestCount,
		&r.Note, &r.InvoiceID, &r.BilledQuantity, &r.RespondedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return r, nil
}

func collectRSVPs(rows pgx.Rows) ([]*RSVP, error) {
	defer rows.Close()
	out := []*RSVP{}
	for rows.Next() {
		r, err := scanRSVP(rows)
		if err != nil {
			return nil, fmt.Errorf("club events: scan rsvp: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRSVPs loads RSVPs for one event.
func (r *Repository) ListRSVPs(ctx context.Context, orgID, eventID string) ([]*RSVP, error) {
	if _, err := parseUUID(eventID); err != nil {
		return []*RSVP{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+rsvpColumns+` FROM club_event_rsvps
		WHERE org_id = $1 AND event_id = $2::uuid ORDER BY household_id, swimmer_id`, orgID, eventID)
	if err != nil {
		return nil, fmt.Errorf("club events: list rsvps: %w", err)
	}
	return collectRSVPs(rows)
}

// ListRSVPsForEvents loads RSVPs for several events.
func (r *Repository) ListRSVPsForEvents(ctx context.Context, orgID string, eventIDs []string) ([]*RSVP, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+rsvpColumns+` FROM club_event_rsvps
		WHERE org_id = $1 AND event_id::text = ANY($2) ORDER BY household_id, swimmer_id`, orgID, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("club events: list rsvps: %w", err)
	}
	return collectRSVPs(rows)
}

// RespondTx upserts an RSVP inside a transaction that locks the event row,
// so concurrent responses cannot oversubscribe capacity.
func (r *Repository) RespondTx(ctx context.Context, orgID, eventID, householdID, swimmerID string, build func(existing *RSVP, othersYes int) (*RSVP, error)) (*RSVP, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("club events: begin rsvp tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM club_events WHERE id = $1::uuid AND org_id = $2 FOR UPDATE`, eventID, orgID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("club events: lock event: %w", err)
	}
	existing, err := scanRSVP(tx.QueryRow(ctx, `SELECT `+rsvpColumns+` FROM club_event_rsvps
		WHERE event_id = $1::uuid AND household_id = $2 AND swimmer_id = $3`, eventID, householdID, swimmerID))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("club events: load rsvp: %w", err)
		}
		existing = nil
	}
	var othersYes int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(1 + guest_count), 0)::int FROM club_event_rsvps
		WHERE event_id = $1::uuid AND response = 'yes' AND NOT (household_id = $2 AND swimmer_id = $3)`,
		eventID, householdID, swimmerID).Scan(&othersYes); err != nil {
		return nil, fmt.Errorf("club events: sum headcount: %w", err)
	}
	next, err := build(existing, othersYes)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO club_event_rsvps (
			id, org_id, event_id, household_id, swimmer_id, response, guest_count, note,
			invoice_id, billed_quantity, responded_by, created_at, updated_at
		) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (event_id, household_id, swimmer_id) DO UPDATE SET
			response = EXCLUDED.response, guest_count = EXCLUDED.guest_count, note = EXCLUDED.note,
			responded_by = EXCLUDED.responded_by, updated_at = EXCLUDED.updated_at`,
		next.ID, next.OrgID, next.EventID, next.HouseholdID, next.SwimmerID, next.Response, next.GuestCount, next.Note,
		next.InvoiceID, next.BilledQuantity, next.RespondedBy, next.CreatedAt, next.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("club events: upsert rsvp: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("club events: commit rsvp: %w", err)
	}
	return next, nil
}

// UpdateRSVPBilling records the fee invoice linked to an RSVP.
func (r *Repository) UpdateRSVPBilling(ctx context.Context, orgID, rsvpID, invoiceID string, qty int) error {
	_, err := r.pool.Exec(ctx, `UPDATE club_event_rsvps SET invoice_id = $3, billed_quantity = $4
		WHERE id = $1::uuid AND org_id = $2`, rsvpID, orgID, invoiceID, qty)
	if err != nil {
		return fmt.Errorf("club events: update rsvp billing: %w", err)
	}
	return nil
}

// ─── Feed tokens ──────────────────────────────────────────────────────────────

const tokenColumns = `id::text, org_id, household_id, token_hash, created_at, created_by, last_used_at, revoked_at`

func scanToken(row pgx.Row) (*FeedToken, error) {
	t := &FeedToken{}
	if err := row.Scan(&t.ID, &t.OrgID, &t.HouseholdID, &t.TokenHash, &t.CreatedAt, &t.CreatedBy, &t.LastUsedAt, &t.RevokedAt); err != nil {
		return nil, err
	}
	return t, nil
}

// RotateFeedToken revokes the live token (if any) and inserts tok atomically.
func (r *Repository) RotateFeedToken(ctx context.Context, tok *FeedToken, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("club events: begin token tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE calendar_feed_tokens SET revoked_at = $3
		WHERE org_id = $1 AND household_id = $2 AND revoked_at IS NULL`, tok.OrgID, tok.HouseholdID, now); err != nil {
		return fmt.Errorf("club events: revoke token: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO calendar_feed_tokens (id, org_id, household_id, token_hash, created_at, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)`, tok.ID, tok.OrgID, tok.HouseholdID, tok.TokenHash, tok.CreatedAt, tok.CreatedBy); err != nil {
		return fmt.Errorf("club events: insert token: %w", err)
	}
	return tx.Commit(ctx)
}

// ActiveFeedToken returns the household's live token or nil.
func (r *Repository) ActiveFeedToken(ctx context.Context, orgID, householdID string) (*FeedToken, error) {
	t, err := scanToken(r.pool.QueryRow(ctx, `SELECT `+tokenColumns+` FROM calendar_feed_tokens
		WHERE org_id = $1 AND household_id = $2 AND revoked_at IS NULL`, orgID, householdID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("club events: active token: %w", err)
	}
	return t, nil
}

// RevokeFeedToken revokes the household's live token.
func (r *Repository) RevokeFeedToken(ctx context.Context, orgID, householdID string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE calendar_feed_tokens SET revoked_at = $3
		WHERE org_id = $1 AND household_id = $2 AND revoked_at IS NULL`, orgID, householdID, now)
	if err != nil {
		return fmt.Errorf("club events: revoke token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFeedTokenMissing
	}
	return nil
}

// ResolveFeedToken loads a live token by hash and stamps last_used_at
// (at most once a minute, to keep calendar-client polling cheap).
func (r *Repository) ResolveFeedToken(ctx context.Context, hash string, now time.Time) (*FeedToken, error) {
	t, err := scanToken(r.pool.QueryRow(ctx, `SELECT `+tokenColumns+` FROM calendar_feed_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL`, hash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFeedTokenMissing
		}
		return nil, fmt.Errorf("club events: resolve token: %w", err)
	}
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) > time.Minute {
		_, _ = r.pool.Exec(ctx, `UPDATE calendar_feed_tokens SET last_used_at = $2 WHERE id = $1::uuid`, t.ID, now)
		t.LastUsedAt = &now
	}
	return t, nil
}

func parseUUID(id string) (uuid.UUID, error) {
	return uuid.Parse(id)
}
