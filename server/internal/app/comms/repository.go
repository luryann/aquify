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

func (r *Repository) UpsertDelivery(ctx context.Context, d *EmailDelivery) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO email_deliveries (
			id, org_id, campaign_id, recipient_email, recipient_user_id, status,
			provider_message_id, error_message, sent_at, delivered_at, bounced_at,
			created_at, updated_at
		)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::uuid, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			campaign_id = EXCLUDED.campaign_id,
			recipient_email = EXCLUDED.recipient_email,
			recipient_user_id = EXCLUDED.recipient_user_id,
			status = EXCLUDED.status,
			provider_message_id = EXCLUDED.provider_message_id,
			error_message = EXCLUDED.error_message,
			sent_at = EXCLUDED.sent_at,
			delivered_at = EXCLUDED.delivered_at,
			bounced_at = EXCLUDED.bounced_at,
			updated_at = EXCLUDED.updated_at`,
		d.ID,
		d.OrgID,
		nullableString(d.CampaignID),
		d.RecipientEmail,
		nullableString(d.RecipientUserID),
		d.Status,
		nullableString(d.ProviderMessageID),
		nullableString(d.ErrorMessage),
		d.SentAt,
		d.DeliveredAt,
		d.BouncedAt,
		d.CreatedAt,
		d.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadDeliveries(ctx context.Context) ([]*EmailDelivery, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, COALESCE(campaign_id::text, ''),
		       recipient_email, COALESCE(recipient_user_id::text, ''), status,
		       COALESCE(provider_message_id, ''), COALESCE(error_message, ''),
		       sent_at, delivered_at, bounced_at, created_at, updated_at
		FROM email_deliveries`)
	if err != nil {
		return nil, fmt.Errorf("comms: load email_deliveries: %w", err)
	}
	defer rows.Close()

	var out []*EmailDelivery
	for rows.Next() {
		d := &EmailDelivery{}
		if err := rows.Scan(
			&d.ID,
			&d.OrgID,
			&d.CampaignID,
			&d.RecipientEmail,
			&d.RecipientUserID,
			&d.Status,
			&d.ProviderMessageID,
			&d.ErrorMessage,
			&d.SentAt,
			&d.DeliveredAt,
			&d.BouncedAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("comms: scan email_delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

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

func (r *Repository) UpsertNotificationPreference(ctx context.Context, preference *NotificationPreference) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_preferences (
			id, org_id, user_id, household_id, event_id, email_enabled, push_enabled, created_at, updated_at, updated_by
		)
		VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (org_id, user_id, event_id) DO UPDATE SET
			household_id = EXCLUDED.household_id,
			email_enabled = EXCLUDED.email_enabled,
			push_enabled = EXCLUDED.push_enabled,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by`,
		preference.ID,
		preference.OrgID,
		preference.UserID,
		nullableString(preference.HouseholdID),
		preference.EventID,
		preference.EmailEnabled,
		preference.PushEnabled,
		preference.CreatedAt,
		preference.UpdatedAt,
		nullableString(preference.UpdatedBy),
	)
	return err
}

func (r *Repository) LoadNotificationPreferences(ctx context.Context) ([]*NotificationPreference, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, user_id, COALESCE(household_id, ''), event_id,
		       email_enabled, push_enabled, created_at, updated_at, COALESCE(updated_by, '')
		FROM notification_preferences`)
	if err != nil {
		return nil, fmt.Errorf("comms: load notification_preferences: %w", err)
	}
	defer rows.Close()

	var out []*NotificationPreference
	for rows.Next() {
		preference := &NotificationPreference{}
		if err := rows.Scan(
			&preference.ID,
			&preference.OrgID,
			&preference.UserID,
			&preference.HouseholdID,
			&preference.EventID,
			&preference.EmailEnabled,
			&preference.PushEnabled,
			&preference.CreatedAt,
			&preference.UpdatedAt,
			&preference.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("comms: scan notification_preference: %w", err)
		}
		out = append(out, preference)
	}
	return out, rows.Err()
}

// ─── Messages ─────────────────────────────────────────────────────────────────

func (r *Repository) UpsertMessage(ctx context.Context, m *Message) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO messages (id, org_id, household_id, swimmer_id, sender_user_id, sender_role, body, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
		  body       = EXCLUDED.body,
		  updated_at = EXCLUDED.updated_at`,
		m.ID, m.OrgID, m.HouseholdID, m.SwimmerID, m.SenderUserID, m.SenderRole, m.Body,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadMessages(ctx context.Context) ([]*Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text,
		       sender_user_id::text, sender_role, body, created_at, updated_at
		FROM messages`)
	if err != nil {
		return nil, fmt.Errorf("comms: load messages: %w", err)
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		m := &Message{}
		if err := rows.Scan(
			&m.ID, &m.OrgID, &m.HouseholdID, &m.SwimmerID,
			&m.SenderUserID, &m.SenderRole, &m.Body, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("comms: scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	announcements []*Announcement,
	campaigns []*EmailCampaign,
	deliveries []*EmailDelivery,
	suppressions []*SuppressionEntry,
	preferences []*NotificationPreference,
	messages []*Message,
	err error,
) {
	if announcements, err = r.LoadAnnouncements(ctx); err != nil {
		return
	}
	if campaigns, err = r.LoadCampaigns(ctx); err != nil {
		return
	}
	if deliveries, err = r.LoadDeliveries(ctx); err != nil {
		return
	}
	if suppressions, err = r.LoadSuppressionEntries(ctx); err != nil {
		return
	}
	if preferences, err = r.LoadNotificationPreferences(ctx); err != nil {
		return
	}
	messages, err = r.LoadMessages(ctx)
	return
}
