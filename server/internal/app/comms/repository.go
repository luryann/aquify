package comms

import (
	"context"
	"fmt"
	"time"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the comms domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Announcements ────────────────────────────────────────────────────────────

func (r *Repository) UpsertAnnouncement(ctx context.Context, a *Announcement) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO announcements (id, org_id, title, body, status, audience, group_id,
		                           published_at, archived_at, created_at, updated_at, is_public)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  title       = EXCLUDED.title,
		  body        = EXCLUDED.body,
		  status      = EXCLUDED.status,
		  published_at = EXCLUDED.published_at,
		  archived_at = EXCLUDED.archived_at,
		  is_public   = EXCLUDED.is_public,
		  updated_at  = EXCLUDED.updated_at`,
		a.ID, a.OrgID, a.Title, a.Body, a.Status, a.Audience,
		dbutil.NullableString(a.GroupID), a.PublishedAt, a.ArchivedAt,
		a.CreatedAt, a.UpdatedAt, a.Public,
	)
	return err
}

func (r *Repository) LoadAnnouncements(ctx context.Context) ([]*Announcement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, title, body, status, audience,
		       COALESCE(group_id::text, ''), published_at, archived_at,
		       created_at, updated_at, is_public
		FROM announcements WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("comms: load announcements: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan announcement", func(rows pgx.Rows) (*Announcement, error) {
		a := &Announcement{}
		err := rows.Scan(
			&a.ID, &a.OrgID, &a.Title, &a.Body, &a.Status, &a.Audience,
			&a.GroupID, &a.PublishedAt, &a.ArchivedAt,
			&a.CreatedAt, &a.UpdatedAt, &a.Public,
		)
		return a, err
	})
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
		c.FromName, c.FromEmail, c.Audience, dbutil.NullableString(c.GroupID),
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
	return dbutil.CollectRows(rows, "comms: scan campaign", func(rows pgx.Rows) (*EmailCampaign, error) {
		c := &EmailCampaign{}
		err := rows.Scan(
			&c.ID, &c.OrgID, &c.Name, &c.Subject, &c.BodyHTML, &c.BodyText,
			&c.FromName, &c.FromEmail, &c.Audience, &c.GroupID, &c.Status,
			&c.SentAt, &c.TotalRecipients, &c.SentCount, &c.FailedCount, &c.SuppressedCount,
			&c.CreatedAt, &c.UpdatedAt,
		)
		return c, err
	})
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
		dbutil.NullableString(d.CampaignID),
		d.RecipientEmail,
		dbutil.NullableString(d.RecipientUserID),
		d.Status,
		dbutil.NullableString(d.ProviderMessageID),
		dbutil.NullableString(d.ErrorMessage),
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
	return dbutil.CollectRows(rows, "comms: scan email_delivery", func(rows pgx.Rows) (*EmailDelivery, error) {
		d := &EmailDelivery{}
		err := rows.Scan(
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
		)
		return d, err
	})
}

func (r *Repository) UpsertSuppressionEntry(ctx context.Context, e *SuppressionEntry) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO suppression_list (id, org_id, email, reason, created_at, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
		ON CONFLICT (org_id, email) DO UPDATE SET
		  reason     = EXCLUDED.reason`,
		e.ID, e.OrgID, e.Email, e.Reason,
		e.CreatedAt, dbutil.NullableString(e.CreatedBy),
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
	return dbutil.CollectRows(rows, "comms: scan suppression_entry", func(rows pgx.Rows) (*SuppressionEntry, error) {
		e := &SuppressionEntry{}
		err := rows.Scan(
			&e.ID, &e.OrgID, &e.Email, &e.Reason, &e.CreatedAt, &e.CreatedBy,
		)
		return e, err
	})
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
		dbutil.NullableString(preference.HouseholdID),
		preference.EventID,
		preference.EmailEnabled,
		preference.PushEnabled,
		preference.CreatedAt,
		preference.UpdatedAt,
		dbutil.NullableString(preference.UpdatedBy),
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
	return dbutil.CollectRows(rows, "comms: scan notification_preference", func(rows pgx.Rows) (*NotificationPreference, error) {
		preference := &NotificationPreference{}
		err := rows.Scan(
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
		)
		return preference, err
	})
}

// ─── Messages ─────────────────────────────────────────────────────────────────

func (r *Repository) UpsertMessage(ctx context.Context, m *Message) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO messages (id, org_id, household_id, swimmer_id, sender_user_id, sender_role, body, edited_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
		  body       = EXCLUDED.body,
		  edited_at  = EXCLUDED.edited_at,
		  updated_at = EXCLUDED.updated_at`,
		m.ID, m.OrgID, m.HouseholdID, m.SwimmerID, m.SenderUserID, m.SenderRole, m.Body, m.EditedAt,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadMessages(ctx context.Context) ([]*Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text,
		       sender_user_id::text, sender_role, body, edited_at, created_at, updated_at
		FROM messages`)
	if err != nil {
		return nil, fmt.Errorf("comms: load messages: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan message", func(rows pgx.Rows) (*Message, error) {
		m := &Message{}
		err := rows.Scan(
			&m.ID, &m.OrgID, &m.HouseholdID, &m.SwimmerID,
			&m.SenderUserID, &m.SenderRole, &m.Body, &m.EditedAt, &m.CreatedAt, &m.UpdatedAt,
		)
		return m, err
	})
}

// ─── Message edits ────────────────────────────────────────────────────────────

func (r *Repository) InsertMessageEdit(ctx context.Context, e *MessageEdit) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO message_edits (id, message_id, previous_body, edited_at, edited_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid)
		ON CONFLICT (id) DO NOTHING`,
		e.ID, e.MessageID, e.PreviousBody, e.EditedAt, e.EditedBy,
	)
	return err
}

func (r *Repository) UpdateMessageBody(ctx context.Context, messageID, body string, editedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE messages SET body = $2, edited_at = $3, updated_at = $3 WHERE id = $1::uuid`,
		messageID, body, editedAt,
	)
	return err
}

func (r *Repository) LoadMessageEdits(ctx context.Context) ([]*MessageEdit, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, message_id::text, previous_body, edited_at, edited_by::text
		FROM message_edits`)
	if err != nil {
		return nil, fmt.Errorf("comms: load message_edits: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan message_edit", func(rows pgx.Rows) (*MessageEdit, error) {
		e := &MessageEdit{}
		err := rows.Scan(&e.ID, &e.MessageID, &e.PreviousBody, &e.EditedAt, &e.EditedBy)
		return e, err
	})
}

// ─── Message reactions ─────────────────────────────────────────────────────────

func (r *Repository) UpsertMessageReaction(ctx context.Context, react *MessageReaction) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO message_reactions (id, message_id, user_id, emoji, created_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
		ON CONFLICT (message_id, user_id, emoji) DO NOTHING`,
		react.ID, react.MessageID, react.UserID, react.Emoji, react.CreatedAt,
	)
	return err
}

func (r *Repository) DeleteMessageReaction(ctx context.Context, messageID, userID, emoji string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM message_reactions WHERE message_id = $1::uuid AND user_id = $2::uuid AND emoji = $3`,
		messageID, userID, emoji,
	)
	return err
}

func (r *Repository) LoadMessageReactions(ctx context.Context) ([]*MessageReaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, message_id::text, user_id::text, emoji, created_at
		FROM message_reactions`)
	if err != nil {
		return nil, fmt.Errorf("comms: load message_reactions: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan message_reaction", func(rows pgx.Rows) (*MessageReaction, error) {
		react := &MessageReaction{}
		err := rows.Scan(&react.ID, &react.MessageID, &react.UserID, &react.Emoji, &react.CreatedAt)
		return react, err
	})
}

// ─── Message attachments ───────────────────────────────────────────────────────

func (r *Repository) InsertMessageAttachment(ctx context.Context, a *MessageAttachment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO message_attachments (id, message_id, object_key, filename, mime_type, size_bytes, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING`,
		a.ID, a.MessageID, a.ObjectKey, a.Filename, a.MimeType, a.SizeBytes, a.CreatedAt,
	)
	return err
}

func (r *Repository) LoadMessageAttachments(ctx context.Context) ([]*MessageAttachment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, message_id::text, object_key, filename, mime_type, size_bytes, created_at
		FROM message_attachments`)
	if err != nil {
		return nil, fmt.Errorf("comms: load message_attachments: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan message_attachment", func(rows pgx.Rows) (*MessageAttachment, error) {
		a := &MessageAttachment{}
		err := rows.Scan(&a.ID, &a.MessageID, &a.ObjectKey, &a.Filename, &a.MimeType, &a.SizeBytes, &a.CreatedAt)
		return a, err
	})
}

// ─── Message reads ──────────────────────────────────────────────────────────────

func (r *Repository) UpsertMessageRead(ctx context.Context, read *MessageRead) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO message_reads (id, org_id, household_id, swimmer_id, user_id, last_read_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6)
		ON CONFLICT (org_id, household_id, swimmer_id, user_id) DO UPDATE SET
			last_read_at = EXCLUDED.last_read_at`,
		read.ID, read.OrgID, read.HouseholdID, read.SwimmerID, read.UserID, read.LastReadAt,
	)
	return err
}

func (r *Repository) LoadMessageReads(ctx context.Context) ([]*MessageRead, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text, user_id::text, last_read_at
		FROM message_reads`)
	if err != nil {
		return nil, fmt.Errorf("comms: load message_reads: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan message_read", func(rows pgx.Rows) (*MessageRead, error) {
		read := &MessageRead{}
		err := rows.Scan(&read.ID, &read.OrgID, &read.HouseholdID, &read.SwimmerID, &read.UserID, &read.LastReadAt)
		return read, err
	})
}

// ─── Household erasure (compliance right-to-erasure) ──────────────────────────
//
// These delete household-scoped comms records outright rather than
// anonymizing them, per the owner's SEC-012 decision that comms/documents/
// storage/auth are fully erased (only billing/financial records are
// retained). Messages cascade-delete their edits/reactions/attachments via
// FK ON DELETE CASCADE; email_deliveries has no household_id column, so
// callers must match by the household's captured contact emails instead.

// EraseHousehold deletes a household's messages (edits/reactions/attachments
// cascade), read watermarks, notification preferences and email deliveries in
// a single transaction so a compliance erasure is all-or-nothing.
func (r *Repository) EraseHousehold(ctx context.Context, orgID, householdID string, emails []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin erase tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	stmts := []struct {
		name string
		sql  string
		args []any
	}{
		{"messages", `DELETE FROM messages WHERE org_id = $1::uuid AND household_id = $2::uuid`, []any{orgID, householdID}},
		{"message reads", `DELETE FROM message_reads WHERE org_id = $1::uuid AND household_id = $2::uuid`, []any{orgID, householdID}},
		{"notification preferences", `DELETE FROM notification_preferences WHERE org_id = $1::uuid AND household_id = $2`, []any{orgID, householdID}},
	}
	if len(emails) > 0 {
		stmts = append(stmts, struct {
			name string
			sql  string
			args []any
		}{"email deliveries", `DELETE FROM email_deliveries WHERE org_id = $1::uuid AND lower(recipient_email) = ANY($2::text[])`, []any{orgID, emails}})
	}
	for _, st := range stmts {
		if _, err := tx.Exec(ctx, st.sql, st.args...); err != nil {
			return fmt.Errorf("erase %s: %w", st.name, err)
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) DeleteMessagesByHousehold(ctx context.Context, orgID, householdID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM messages WHERE org_id = $1::uuid AND household_id = $2::uuid`,
		orgID, householdID,
	)
	return err
}

func (r *Repository) DeleteMessageReadsByHousehold(ctx context.Context, orgID, householdID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM message_reads WHERE org_id = $1::uuid AND household_id = $2::uuid`,
		orgID, householdID,
	)
	return err
}

func (r *Repository) DeleteNotificationPreferencesByHousehold(ctx context.Context, orgID, householdID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM notification_preferences WHERE org_id = $1::uuid AND household_id = $2`,
		orgID, householdID,
	)
	return err
}

func (r *Repository) DeleteEmailDeliveriesByRecipientEmails(ctx context.Context, orgID string, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		DELETE FROM email_deliveries WHERE org_id = $1::uuid AND lower(recipient_email) = ANY($2::text[])`,
		orgID, emails,
	)
	return err
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	announcements []*Announcement,
	campaigns []*EmailCampaign,
	deliveries []*EmailDelivery,
	suppressions []*SuppressionEntry,
	preferences []*NotificationPreference,
	messages []*Message,
	messageEdits []*MessageEdit,
	messageReactions []*MessageReaction,
	messageAttachments []*MessageAttachment,
	messageReads []*MessageRead,
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
	if messages, err = r.LoadMessages(ctx); err != nil {
		return
	}
	if messageEdits, err = r.LoadMessageEdits(ctx); err != nil {
		return
	}
	if messageReactions, err = r.LoadMessageReactions(ctx); err != nil {
		return
	}
	if messageAttachments, err = r.LoadMessageAttachments(ctx); err != nil {
		return
	}
	messageReads, err = r.LoadMessageReads(ctx)
	return
}
