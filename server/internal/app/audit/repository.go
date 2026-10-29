package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// repository handles DB persistence for audit events. Rows are append-only —
// the audit_events table itself rejects UPDATE/DELETE at the database level.
type repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}

func (r *repository) Insert(ctx context.Context, ev AuditEvent) error {
	metadata, err := json.Marshal(ev.Metadata)
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, org_id, actor_user_id, actor_roles, action, resource_type,
			resource_id, before_state, after_state, metadata, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO NOTHING`,
		ev.ID, ev.OrgID, ev.ActorUserID, ev.ActorRoles, ev.Action, ev.ResourceType,
		ev.ResourceID, nullableRawMessage(ev.BeforeState), nullableRawMessage(ev.AfterState), metadata, ev.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("audit: insert event: %w", err)
	}
	return nil
}

// Query returns events matching f, newest first, straight from Postgres so
// every API instance sees the same log and memory does not grow with it.
// Cursor semantics match the in-memory path: results start after the event
// whose ID is f.Cursor, and an unknown cursor yields no rows.
func (r *repository) Query(ctx context.Context, f Filter) ([]AuditEvent, error) {
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.OrgID != "" {
		add("org_id = $%d", f.OrgID)
	}
	if f.ActorUserID != "" {
		add("actor_user_id = $%d", f.ActorUserID)
	}
	if f.Action != "" {
		add("lower(action) = lower($%d)", f.Action)
	}
	if f.ResourceType != "" {
		add("lower(resource_type) = lower($%d)", f.ResourceType)
	}
	if f.ResourceID != "" {
		add("resource_id = $%d", f.ResourceID)
	}
	if f.From != nil {
		add("occurred_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("occurred_at <= $%d", *f.To)
	}
	if f.Cursor != "" {
		add("(occurred_at, id) < (SELECT occurred_at, id FROM audit_events WHERE id = $%d)", f.Cursor)
	}
	query := `
		SELECT id, org_id, actor_user_id, actor_roles, action, resource_type,
		       resource_id, before_state, after_state, metadata, occurred_at
		FROM audit_events`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY occurred_at DESC, id DESC"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query events: %w", err)
	}
	events, err := dbutil.CollectRowsNonNil(rows, "audit: scan event", func(rows pgx.Rows) (AuditEvent, error) {
		var ev AuditEvent
		var metadata []byte
		if err := rows.Scan(
			&ev.ID, &ev.OrgID, &ev.ActorUserID, &ev.ActorRoles, &ev.Action, &ev.ResourceType,
			&ev.ResourceID, &ev.BeforeState, &ev.AfterState, &metadata, &ev.OccurredAt,
		); err != nil {
			return ev, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &ev.Metadata); err != nil {
				return ev, fmt.Errorf("decode metadata: %w", err)
			}
		}
		if ev.Metadata == nil {
			ev.Metadata = map[string]string{}
		}
		return ev, nil
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

func nullableRawMessage(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
