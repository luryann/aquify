package audit

import (
	"context"
	"encoding/json"
	"fmt"

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

func (r *repository) ListAll(ctx context.Context) ([]AuditEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, actor_user_id, actor_roles, action, resource_type,
		       resource_id, before_state, after_state, metadata, occurred_at
		FROM audit_events
		ORDER BY occurred_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("audit: list events: %w", err)
	}
	defer rows.Close()

	var out []AuditEvent
	for rows.Next() {
		var ev AuditEvent
		var metadata []byte
		if err := rows.Scan(
			&ev.ID, &ev.OrgID, &ev.ActorUserID, &ev.ActorRoles, &ev.Action, &ev.ResourceType,
			&ev.ResourceID, &ev.BeforeState, &ev.AfterState, &metadata, &ev.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("audit: scan event: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &ev.Metadata); err != nil {
				return nil, fmt.Errorf("audit: decode metadata: %w", err)
			}
		}
		if ev.Metadata == nil {
			ev.Metadata = map[string]string{}
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: iterate events: %w", err)
	}
	return out, nil
}

func nullableRawMessage(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
