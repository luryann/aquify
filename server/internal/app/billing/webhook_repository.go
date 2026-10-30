package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
)

// webhookLease is how long a claimed webhook delivery is considered in flight
// before another delivery may take it over (crash recovery).
const webhookLease = 2 * time.Minute

// isUUIDString reports whether s has the canonical 8-4-4-4-12 hex shape.
func isUUIDString(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
		default:
			return false
		}
	}
	return true
}

// claimWebhookEvent inserts the event or re-claims it for processing. It
// returns false for terminal events and ErrWebhookInFlight for an active lease.
func (r *Repository) claimWebhookEvent(ctx context.Context, rec *WebhookEventRecord) (bool, error) {
	payload := string(rec.Payload)
	if payload == "" {
		payload = "{}"
	}
	var org interface{}
	if isUUIDString(rec.OrgID) {
		org = rec.OrgID
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO webhook_events (id, org_id, provider, event_id, event_type, payload, processed, retryable,
		       processing_until, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::jsonb, FALSE, FALSE, now() + make_interval(secs => $7), $8)
		ON CONFLICT (provider, event_id) DO UPDATE
		   SET processing_until = EXCLUDED.processing_until
		 WHERE NOT webhook_events.processed
		   AND (COALESCE(webhook_events.error_message, '') = '' OR webhook_events.retryable)
		   AND (webhook_events.processing_until IS NULL OR webhook_events.processing_until < now())
		RETURNING id::text`,
		rec.ID, org, rec.Provider, rec.EventID, rec.EventType, payload, webhookLease.Seconds(), rec.CreatedAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var terminal bool
		if err := r.pool.QueryRow(ctx, `SELECT processed OR (COALESCE(error_message, '') <> '' AND NOT retryable) FROM webhook_events WHERE provider = $1 AND event_id = $2`, rec.Provider, rec.EventID).Scan(&terminal); err != nil {
			return false, err
		}
		if !terminal {
			return false, ErrWebhookInFlight
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("billing: claim webhook event: %w", err)
	}
	return true, nil
}

// finishWebhookEvent records the outcome of a claimed delivery and releases its lease.
func (r *Repository) finishWebhookEvent(ctx context.Context, rec *WebhookEventRecord) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE webhook_events
		   SET processed = $3, processed_at = $4, error_message = NULLIF($5, ''), retryable = $6,
		       processing_until = NULL
		 WHERE provider = $1 AND event_id = $2`,
		rec.Provider, rec.EventID, rec.Processed, rec.ProcessedAt, rec.ErrorMessage, rec.Retryable)
	if err != nil {
		return fmt.Errorf("billing: finish webhook event: %w", err)
	}
	return nil
}

// loadWebhookEvents returns the most recent webhook events, for one org (empty orgID means all orgs).
func (r *Repository) loadWebhookEvents(ctx context.Context, orgID string, limit int) ([]*WebhookEventRecord, error) {
	var org interface{}
	if orgID != "" {
		// A non-empty org that is not a UUID can match nothing; never fall
		// through to the unfiltered (all orgs) query.
		if !isUUIDString(orgID) {
			return []*WebhookEventRecord{}, nil
		}
		org = orgID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(org_id::text, ''), provider, event_id, event_type, payload::text, processed,
		       processed_at, COALESCE(error_message, ''), retryable, created_at
		  FROM webhook_events
		 WHERE ($1::uuid IS NULL OR org_id = $1::uuid)
		 ORDER BY created_at DESC LIMIT $2`, org, limit)
	if err != nil {
		return nil, fmt.Errorf("billing: load webhook events: %w", err)
	}
	return dbutil.CollectRows(rows, "billing: scan webhook event", func(rows pgx.Rows) (*WebhookEventRecord, error) {
		rec := &WebhookEventRecord{}
		var payload string
		if err := rows.Scan(&rec.ID, &rec.OrgID, &rec.Provider, &rec.EventID, &rec.EventType, &payload,
			&rec.Processed, &rec.ProcessedAt, &rec.ErrorMessage, &rec.Retryable, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Payload = json.RawMessage(payload)
		return rec, nil
	})
}
