package comms

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// Repository handles DB persistence for the comms domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// at binds orgID to every statement so row-level security on the comms tables applies.
// An empty org sees nothing.
func (r *Repository) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: r.pool}.For(orgID) }

// OrgIDs lists every registered club (the registry is not tenant data) so an all-club
// load can read each club under its own binding.
func (r *Repository) OrgIDs(ctx context.Context) ([]string, error) {
	return dbutil.ListOrgIDs(ctx, r.pool)
}

// ─── Announcements ────────────────────────────────────────────────────────────

func (r *Repository) UpsertAnnouncement(ctx context.Context, a *Announcement) error {
	_, err := r.at(a.OrgID).Exec(ctx, `
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

func (r *Repository) LoadAnnouncements(ctx context.Context, orgID string) ([]*Announcement, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, title, body, status, audience,
		       COALESCE(group_id::text, ''), published_at, archived_at,
		       created_at, updated_at, is_public
		FROM announcements WHERE deleted_at IS NULL`+orgClause("org_id", orgID, true), orgArgs(orgID)...)
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
	_, err := r.at(c.OrgID).Exec(ctx, `
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

// ClaimCampaignForSend atomically moves a campaign draft -> sending (or
// re-claims one stuck in 'sending' with no heartbeat for staleAfter) and
// reports whether this caller won the claim.
func (r *Repository) ClaimCampaignForSend(ctx context.Context, orgID, campaignID string, staleAfter time.Duration) (bool, error) {
	var id string
	err := r.at(orgID).QueryRow(ctx, `
		UPDATE email_campaigns SET status = 'sending', updated_at = now()
		WHERE id = $1::uuid AND org_id = $2::uuid AND deleted_at IS NULL
		  AND (status = 'draft'
		       OR (status = 'sending' AND updated_at < now() - make_interval(secs => $3::double precision)))
		RETURNING id::text`, campaignID, orgID, staleAfter.Seconds()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// TouchSendingCampaign refreshes updated_at on a campaign being sent so it is
// not treated as a stuck claim.
func (r *Repository) TouchSendingCampaign(ctx context.Context, orgID, campaignID string) error {
	_, err := r.at(orgID).Exec(ctx, `
		UPDATE email_campaigns SET updated_at = now()
		WHERE id = $1::uuid AND org_id = $2::uuid AND status = 'sending'`, campaignID, orgID)
	return err
}

func (r *Repository) LoadCampaigns(ctx context.Context, orgID string) ([]*EmailCampaign, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, name, subject, body_html, COALESCE(body_text, ''),
		       from_name, from_email, audience, COALESCE(group_id::text, ''), status,
		       sent_at, total_recipients, sent_count, failed_count, suppressed_count,
		       created_at, updated_at
		FROM email_campaigns WHERE deleted_at IS NULL`+orgClause("org_id", orgID, true), orgArgs(orgID)...)
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
	_, err := r.at(d.OrgID).Exec(ctx, `
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

const deliverySelect = `
		SELECT id::text, org_id::text, COALESCE(campaign_id::text, ''),
		       recipient_email, COALESCE(recipient_user_id::text, ''), status,
		       COALESCE(provider_message_id, ''), COALESCE(error_message, ''),
		       sent_at, delivered_at, bounced_at, created_at, updated_at
		FROM email_deliveries`

func (r *Repository) LoadDeliveries(ctx context.Context, orgID string) ([]*EmailDelivery, error) {
	return r.queryDeliveries(ctx, orgID, deliverySelect+orgClause("org_id", orgID, false), orgArgs(orgID)...)
}

func (r *Repository) queryDeliveries(ctx context.Context, orgID, q string, args ...any) ([]*EmailDelivery, error) {
	rows, err := r.at(orgID).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("comms: load email_deliveries: %w", err)
	}
	return dbutil.CollectRows(rows, "comms: scan email_delivery", func(rows pgx.Rows) (*EmailDelivery, error) {
		d := &EmailDelivery{}
		err := rows.Scan(
			&d.ID, &d.OrgID, &d.CampaignID, &d.RecipientEmail, &d.RecipientUserID, &d.Status,
			&d.ProviderMessageID, &d.ErrorMessage, &d.SentAt, &d.DeliveredAt, &d.BouncedAt,
			&d.CreatedAt, &d.UpdatedAt,
		)
		return d, err
	})
}

func (r *Repository) UpsertSuppressionEntry(ctx context.Context, e *SuppressionEntry) error {
	_, err := r.at(e.OrgID).Exec(ctx, `
		INSERT INTO suppression_list (id, org_id, email, reason, created_at, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
		ON CONFLICT (org_id, email) DO UPDATE SET
		  reason     = EXCLUDED.reason`,
		e.ID, e.OrgID, e.Email, e.Reason,
		e.CreatedAt, uuidOrNil(e.CreatedBy),
	)
	return err
}

func (r *Repository) DeleteSuppressionEntry(ctx context.Context, orgID, email string) error {
	_, err := r.at(orgID).Exec(ctx, `
		DELETE FROM suppression_list WHERE org_id = $1::uuid AND email = $2`,
		orgID, email,
	)
	return err
}

func (r *Repository) LoadSuppressionEntries(ctx context.Context, orgID string) ([]*SuppressionEntry, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, email, reason, created_at, COALESCE(created_by::text, '')
		FROM suppression_list`+orgClause("org_id", orgID, false), orgArgs(orgID)...)
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
	return upsertNotificationPreference(ctx, r.at(preference.OrgID), preference)
}

// UpsertNotificationPreferences writes every preference in one transaction.
func (r *Repository) UpsertNotificationPreferences(ctx context.Context, prefs []*NotificationPreference) error {
	if len(prefs) == 0 {
		return nil
	}
	tx, err := r.at(prefs[0].OrgID).Begin(ctx) // one request is one club's preferences
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, p := range prefs {
		if err := upsertNotificationPreference(ctx, tx, p); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func upsertNotificationPreference(ctx context.Context, db execer, preference *NotificationPreference) error {
	_, err := db.Exec(ctx, `
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

func (r *Repository) LoadNotificationPreferences(ctx context.Context, orgID string) ([]*NotificationPreference, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, user_id, COALESCE(household_id, ''), event_id,
		       email_enabled, push_enabled, created_at, updated_at, COALESCE(updated_by, '')
		FROM notification_preferences`+orgClause("org_id", orgID, false), orgArgs(orgID)...)
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
	_, err := r.at(m.OrgID).Exec(ctx, `
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

func (r *Repository) LoadMessages(ctx context.Context, orgID string) ([]*Message, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text,
		       sender_user_id::text, sender_role, body, edited_at, created_at, updated_at
		FROM messages`+orgClause("org_id", orgID, false), orgArgs(orgID)...)
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

// EditMessageTx writes the history row and the new body in one transaction.
func (r *Repository) EditMessageTx(ctx context.Context, orgID string, e *MessageEdit, body string, editedAt time.Time) error {
	tx, err := r.at(orgID).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_edits (id, org_id, message_id, previous_body, edited_at, edited_by)
		SELECT $1::uuid, m.org_id, m.id, $3, $4, $5::uuid FROM messages m WHERE m.id = $2::uuid
		ON CONFLICT (id) DO NOTHING`,
		e.ID, e.MessageID, e.PreviousBody, e.EditedAt, e.EditedBy,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE messages SET body = $2, edited_at = $3, updated_at = $3 WHERE id = $1::uuid AND org_id = $4::uuid`,
		e.MessageID, body, editedAt, orgID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpdateMessageBody(ctx context.Context, orgID, messageID, body string, editedAt time.Time) error {
	_, err := r.at(orgID).Exec(ctx, `
		UPDATE messages SET body = $2, edited_at = $3, updated_at = $3 WHERE id = $1::uuid AND org_id = $4::uuid`,
		messageID, body, editedAt, orgID,
	)
	return err
}

func (r *Repository) LoadMessageEdits(ctx context.Context, orgID string) ([]*MessageEdit, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, message_id::text, previous_body, edited_at, edited_by::text
		FROM message_edits`+messageOrgClause(orgID), orgArgs(orgID)...)
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

func (r *Repository) UpsertMessageReaction(ctx context.Context, orgID string, react *MessageReaction) error {
	_, err := r.at(orgID).Exec(ctx, `
		INSERT INTO message_reactions (id, org_id, message_id, user_id, emoji, created_at)
		SELECT $1::uuid, m.org_id, m.id, $3::uuid, $4, $5 FROM messages m WHERE m.id = $2::uuid
		ON CONFLICT (message_id, user_id, emoji) DO NOTHING`,
		react.ID, react.MessageID, react.UserID, react.Emoji, react.CreatedAt,
	)
	return err
}

// DeleteMessage removes a message (attachments/edits/reactions cascade via FK).
func (r *Repository) DeleteMessage(ctx context.Context, orgID, messageID string) error {
	_, err := r.at(orgID).Exec(ctx, `DELETE FROM messages WHERE id = $1::uuid AND org_id = $2::uuid`, messageID, orgID)
	return err
}

func (r *Repository) DeleteMessageReaction(ctx context.Context, orgID, messageID, userID, emoji string) error {
	_, err := r.at(orgID).Exec(ctx, `
		DELETE FROM message_reactions WHERE message_id = $1::uuid AND user_id = $2::uuid AND emoji = $3`,
		messageID, userID, emoji,
	)
	return err
}

func (r *Repository) LoadMessageReactions(ctx context.Context, orgID string) ([]*MessageReaction, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, message_id::text, user_id::text, emoji, created_at
		FROM message_reactions`+messageOrgClause(orgID), orgArgs(orgID)...)
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

func (r *Repository) InsertMessageAttachment(ctx context.Context, orgID string, a *MessageAttachment) error {
	_, err := r.at(orgID).Exec(ctx, `
		INSERT INTO message_attachments (id, org_id, message_id, object_key, filename, mime_type, size_bytes, created_at)
		SELECT $1::uuid, m.org_id, m.id, $3, $4, $5, $6, $7 FROM messages m WHERE m.id = $2::uuid
		ON CONFLICT (id) DO NOTHING`,
		a.ID, a.MessageID, a.ObjectKey, a.Filename, a.MimeType, a.SizeBytes, a.CreatedAt,
	)
	return err
}

func (r *Repository) LoadMessageAttachments(ctx context.Context, orgID string) ([]*MessageAttachment, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, message_id::text, object_key, filename, mime_type, size_bytes, created_at
		FROM message_attachments`+messageOrgClause(orgID), orgArgs(orgID)...)
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
	_, err := r.at(read.OrgID).Exec(ctx, `
		INSERT INTO message_reads (id, org_id, household_id, swimmer_id, user_id, last_read_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6)
		ON CONFLICT (org_id, household_id, swimmer_id, user_id) DO UPDATE SET
			last_read_at = EXCLUDED.last_read_at`,
		read.ID, read.OrgID, read.HouseholdID, read.SwimmerID, read.UserID, read.LastReadAt,
	)
	return err
}

func (r *Repository) LoadMessageReads(ctx context.Context, orgID string) ([]*MessageRead, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text, user_id::text, last_read_at
		FROM message_reads`+orgClause("org_id", orgID, false), orgArgs(orgID)...)
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
	tx, err := r.at(orgID).Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin erase tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	type stmt struct {
		name string
		sql  string
		args []any
	}
	stmts := []stmt{
		{"messages", `DELETE FROM messages WHERE org_id = $1::uuid AND household_id = $2::uuid`, []any{orgID, householdID}},
		{"message reads", `DELETE FROM message_reads WHERE org_id = $1::uuid AND household_id = $2::uuid`, []any{orgID, householdID}},
		{"notification preferences", `DELETE FROM notification_preferences WHERE org_id = $1::uuid AND household_id = $2`, []any{orgID, householdID}},
	}
	if len(emails) > 0 {
		stmts = append(stmts, stmt{"email deliveries", `DELETE FROM email_deliveries WHERE org_id = $1::uuid AND lower(recipient_email) = ANY($2::text[])`, []any{orgID, emails}})
	}
	for _, st := range stmts {
		if _, err := tx.Exec(ctx, st.sql, st.args...); err != nil {
			return fmt.Errorf("erase %s: %w", st.name, err)
		}
	}
	return tx.Commit(ctx)
}

// ─── Batch load ───────────────────────────────────────────────────────────────

// LoadAll loads one club (orgID set) or, with an empty orgID, every club one at a time
// under that club's own binding (an unbound read would see nothing under RLS).
func (r *Repository) LoadAll(ctx context.Context, orgID string) (
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
	if orgID != "" {
		return r.loadOrg(ctx, orgID)
	}
	orgs, err := r.OrgIDs(ctx)
	if err != nil {
		return
	}
	for _, o := range orgs {
		a, c, d, su, pr, m, me, mr, ma, rd, lerr := r.loadOrg(ctx, o)
		if lerr != nil {
			err = lerr
			return
		}
		announcements = append(announcements, a...)
		campaigns = append(campaigns, c...)
		deliveries = append(deliveries, d...)
		suppressions = append(suppressions, su...)
		preferences = append(preferences, pr...)
		messages = append(messages, m...)
		messageEdits = append(messageEdits, me...)
		messageReactions = append(messageReactions, mr...)
		messageAttachments = append(messageAttachments, ma...)
		messageReads = append(messageReads, rd...)
	}
	return
}

func (r *Repository) loadOrg(ctx context.Context, orgID string) (
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
	if announcements, err = r.LoadAnnouncements(ctx, orgID); err != nil {
		return
	}
	if campaigns, err = r.LoadCampaigns(ctx, orgID); err != nil {
		return
	}
	if deliveries, err = r.LoadDeliveries(ctx, orgID); err != nil {
		return
	}
	if suppressions, err = r.LoadSuppressionEntries(ctx, orgID); err != nil {
		return
	}
	if preferences, err = r.LoadNotificationPreferences(ctx, orgID); err != nil {
		return
	}
	if messages, err = r.LoadMessages(ctx, orgID); err != nil {
		return
	}
	if messageEdits, err = r.LoadMessageEdits(ctx, orgID); err != nil {
		return
	}
	if messageReactions, err = r.LoadMessageReactions(ctx, orgID); err != nil {
		return
	}
	if messageAttachments, err = r.LoadMessageAttachments(ctx, orgID); err != nil {
		return
	}
	messageReads, err = r.LoadMessageReads(ctx, orgID)
	return
}

// uuidOrNil returns the value only when it is a valid UUID. System actors such
// as "postmark-webhook" are not user IDs and are stored as NULL.
func uuidOrNil(s string) any {
	if _, err := uuid.Parse(s); err != nil {
		return nil
	}
	return s
}

// orgClause returns a WHERE/AND fragment restricting col to the org, or "" for
// an unscoped (all orgs) load. Pair with orgArgs.
func orgClause(col, orgID string, hasWhere bool) string {
	if orgID == "" {
		return ""
	}
	kw := " WHERE "
	if hasWhere {
		kw = " AND "
	}
	return kw + col + " = $1::uuid"
}

func orgArgs(orgID string) []any {
	if orgID == "" {
		return nil
	}
	return []any{orgID}
}

// messageOrgClause scopes message child tables (no org_id column) via messages.
func messageOrgClause(orgID string) string {
	if orgID == "" {
		return ""
	}
	return " WHERE org_id = $1::uuid"
}

// FindDeliveryByProviderMessageID returns the delivery with the given provider
// message id (optionally scoped to an org), or nil.
func (r *Repository) FindDeliveryByProviderMessageID(ctx context.Context, orgID, messageID string) ([]*EmailDelivery, error) {
	if orgID == "" {
		return nil, nil // a provider event naming no club cannot be attributed to one
	}
	q := deliverySelect + ` WHERE provider_message_id = $1`
	args := []any{messageID}
	if orgID != "" {
		q += ` AND org_id = $2::uuid`
		args = append(args, orgID)
	}
	return r.queryDeliveries(ctx, orgID, q, args...)
}

// FindDeliveryByCampaignRecipient returns deliveries for a campaign+recipient.
func (r *Repository) FindDeliveryByCampaignRecipient(ctx context.Context, orgID, campaignID, email string) ([]*EmailDelivery, error) {
	return r.queryDeliveries(ctx, orgID, deliverySelect+` WHERE org_id = $1::uuid AND campaign_id = $2::uuid AND recipient_email = $3 ORDER BY created_at DESC`, orgID, campaignID, email)
}

// LoadSuppressionEntry returns the single suppression row for org+email, or nil.
func (r *Repository) LoadSuppressionEntry(ctx context.Context, orgID, email string) (*SuppressionEntry, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, email, reason, created_at, COALESCE(created_by::text, '')
		FROM suppression_list WHERE org_id = $1::uuid AND email = $2`, orgID, email)
	if err != nil {
		return nil, fmt.Errorf("comms: load suppression entry: %w", err)
	}
	list, err := dbutil.CollectRows(rows, "comms: scan suppression_entry", func(rows pgx.Rows) (*SuppressionEntry, error) {
		e := &SuppressionEntry{}
		err := rows.Scan(&e.ID, &e.OrgID, &e.Email, &e.Reason, &e.CreatedAt, &e.CreatedBy)
		return e, err
	})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}
