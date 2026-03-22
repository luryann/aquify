package comms

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the comms domain.
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

// ─── Announcements ────────────────────────────────────────────────────────────

func (r *Repository) UpsertAnnouncement(ctx context.Context, a *Announcement) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO announcements (id, org_id, title, body, status, audience, group_id,
		                           published_at, archived_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
		  title       = EXCLUDED.title,
		  body        = EXCLUDED.body,
		  status      = EXCLUDED.status,
		  published_at = EXCLUDED.published_at,
		  archived_at = EXCLUDED.archived_at,
		  updated_at  = EXCLUDED.updated_at`,
		a.ID, a.OrgID, a.Title, a.Body, a.Status, a.Audience,
		nullableString(a.GroupID), a.PublishedAt, a.ArchivedAt,
		a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadAnnouncements(ctx context.Context) ([]*Announcement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, title, body, status, audience,
		       COALESCE(group_id::text, ''), published_at, archived_at,
		       created_at, updated_at
		FROM announcements WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("comms: load announcements: %w", err)
	}
	defer rows.Close()
	var out []*Announcement
	for rows.Next() {
		a := &Announcement{}
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.Title, &a.Body, &a.Status, &a.Audience,
			&a.GroupID, &a.PublishedAt, &a.ArchivedAt,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("comms: scan announcement: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ─── Campaigns ────────────────────────────────────────────────────────────────

func (r *Repository) UpsertCampaign(ctx context.Context, c *EmailCampaign) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO email_campaigns (id, org_id, name, subject, body_html, body_text,
		                             from_name, from_email, audience, group_id, status,
		                             sent_at, total_recipients, sent_count, failed_count,
		                             suppressed_count, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (id) DO UPDATE SET
		  name             = EXCLUDED.name,
		  subject          = EXCLUDED.subject,
		  body_html        = EXCLUDED.body_html,
		  body_text        = EXCLUDED.body_text,
		  status           = EXCLUDED.status,
		  sent_at          = EXCLUDED.sent_at,
		  total_recipients = EXCLUDED.total_recipients,
		  sent_count       = EXCLUDED.sent_count,
		  failed_count     = EXCLUDED.failed_count,
		  suppressed_count = EXCLUDED.suppressed_count,
		  updated_at       = EXCLUDED.updated_at`,
		c.ID, c.OrgID, c.Name, c.Subject, c.BodyHTML, c.BodyText,
		c.FromName, c.FromEmail, c.Audience, nullableString(c.GroupID),
		c.Status, c.SentAt,
		c.TotalRecipients, c.SentCount, c.FailedCount, c.SuppressedCount,
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadCampaigns(ctx context.Context) ([]*EmailCampaign, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, name, subject, body_html, COALESCE(body_text, ''),
		       from_name, from_email, audience, COALESCE(group_id::text, ''), status,
		       sent_at, total_recipients, sent_count, failed_count, suppressed_count,
		       created_at, updated_at
		FROM email_campaigns WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("comms: load campaigns: %w", err)
	}
	defer rows.Close()
	var out []*EmailCampaign
	for rows.Next() {
		c := &EmailCampaign{}
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.Name, &c.Subject, &c.BodyHTML, &c.BodyText,
			&c.FromName, &c.FromEmail, &c.Audience, &c.GroupID, &c.Status,
			&c.SentAt, &c.TotalRecipients, &c.SentCount, &c.FailedCount, &c.SuppressedCount,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("comms: scan campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ─── Suppression List ─────────────────────────────────────────────────────────

func (r *Repository) UpsertSuppressionEntry(ctx context.Context, e *SuppressionEntry) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO suppression_list (id, org_id, email, reason, created_at, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
		ON CONFLICT (org_id, email) DO UPDATE SET
		  reason     = EXCLUDED.reason`,
		e.ID, e.OrgID, e.Email, e.Reason,
		e.CreatedAt, nullableString(e.CreatedBy),
	)
	return err
}

func (r *Repository) DeleteSuppressionEntry(ctx context.Context, orgID, email string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM suppression_list WHERE org_id = $1::uuid AND email = $2`,
		orgID, email,
	)
	return err
}

func (r *Repository) LoadSuppressionEntries(ctx context.Context) ([]*SuppressionEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, email, reason, created_at, COALESCE(created_by::text, '')
		FROM suppression_list`)
	if err != nil {
		return nil, fmt.Errorf("comms: load suppression_list: %w", err)
	}
	defer rows.Close()
	var out []*SuppressionEntry
	for rows.Next() {
		e := &SuppressionEntry{}
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.Email, &e.Reason, &e.CreatedAt, &e.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("comms: scan suppression_entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	announcements []*Announcement,
	campaigns []*EmailCampaign,
	suppressions []*SuppressionEntry,
	err error,
) {
	if announcements, err = r.LoadAnnouncements(ctx); err != nil {
		return
	}
	if campaigns, err = r.LoadCampaigns(ctx); err != nil {
		return
	}
	suppressions, err = r.LoadSuppressionEntries(ctx)
	return
}
