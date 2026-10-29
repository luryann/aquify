package documents

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the documents domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Document Types ───────────────────────────────────────────────────────────

func (r *Repository) UpsertDocumentType(ctx context.Context, dt *DocumentType) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO document_types (id, org_id, name, category, description, is_required,
		                            applies_to, expiry_months, is_active,
		                            created_at, updated_at, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
		  name          = EXCLUDED.name,
		  category      = EXCLUDED.category,
		  description   = EXCLUDED.description,
		  is_required   = EXCLUDED.is_required,
		  applies_to    = EXCLUDED.applies_to,
		  expiry_months = EXCLUDED.expiry_months,
		  is_active     = EXCLUDED.is_active,
		  updated_at    = EXCLUDED.updated_at,
		  updated_by    = EXCLUDED.updated_by`,
		dt.ID, dt.OrgID, dt.Name, dt.Category, dbutil.NullableString(dt.Description),
		dt.IsRequired, dt.AppliesTo, dt.ExpiryMonths, dt.IsActive,
		dt.CreatedAt, dt.UpdatedAt, dt.CreatedBy, dt.UpdatedBy,
	)
	return err
}

func (r *Repository) SoftDeleteDocumentType(ctx context.Context, typeID, deletedBy string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE document_types SET deleted_at = now(), deleted_by = $2
		WHERE id = $1::uuid AND deleted_at IS NULL`,
		typeID, deletedBy,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		slog.Warn("documents: SoftDeleteDocumentType: no rows affected — type already deleted or not found in DB", "type_id", typeID)
	}
	return nil
}

func (r *Repository) LoadDocumentTypes(ctx context.Context) ([]*DocumentType, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, name, category,
		       COALESCE(description, ''), is_required, applies_to,
		       expiry_months, is_active, created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM document_types WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("documents: load document_types: %w", err)
	}
	return dbutil.CollectRows(rows, "documents: scan document_type", func(rows pgx.Rows) (*DocumentType, error) {
		dt := &DocumentType{}
		err := rows.Scan(
			&dt.ID, &dt.OrgID, &dt.Name, &dt.Category, &dt.Description,
			&dt.IsRequired, &dt.AppliesTo, &dt.ExpiryMonths, &dt.IsActive,
			&dt.CreatedAt, &dt.UpdatedAt, &dt.CreatedBy, &dt.UpdatedBy,
		)
		return dt, err
	})
}

// ─── Document Submissions ─────────────────────────────────────────────────────

func (r *Repository) UpsertSubmission(ctx context.Context, sub *DocumentSubmission) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO document_submissions (id, org_id, document_type_id, household_id, swimmer_id,
		                                  status, file_object_key, file_name, file_size_bytes, file_mime,
		                                  notes, review_notes, submitted_at, verified_at, verified_by,
		                                  expires_at, reminder_sent_at, created_at, updated_at,
		                                  created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5,
		        $6, $7, $8, $9, $10,
		        $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21)
		ON CONFLICT (id) DO UPDATE SET
		  status           = EXCLUDED.status,
		  file_object_key  = EXCLUDED.file_object_key,
		  file_name        = EXCLUDED.file_name,
		  file_size_bytes  = EXCLUDED.file_size_bytes,
		  file_mime        = EXCLUDED.file_mime,
		  notes            = EXCLUDED.notes,
		  review_notes     = EXCLUDED.review_notes,
		  submitted_at     = EXCLUDED.submitted_at,
		  verified_at      = EXCLUDED.verified_at,
		  verified_by      = EXCLUDED.verified_by,
		  expires_at       = EXCLUDED.expires_at,
		  reminder_sent_at = EXCLUDED.reminder_sent_at,
		  updated_at       = EXCLUDED.updated_at,
		  updated_by       = EXCLUDED.updated_by`,
		sub.ID, sub.OrgID, sub.DocumentTypeID, sub.HouseholdID, dbutil.NullableString(sub.SwimmerID),
		sub.Status, dbutil.NullableString(sub.FileObjectKey), dbutil.NullableString(sub.FileName),
		nullableInt64(sub.FileSizeBytes), dbutil.NullableString(sub.FileMIME),
		dbutil.NullableString(sub.Notes), dbutil.NullableString(sub.ReviewNotes),
		sub.SubmittedAt, sub.VerifiedAt, dbutil.NullableString(sub.VerifiedBy),
		sub.ExpiresAt, sub.ReminderSentAt,
		sub.CreatedAt, sub.UpdatedAt, sub.CreatedBy, sub.UpdatedBy,
	)
	return err
}

func (r *Repository) LoadSubmissions(ctx context.Context) ([]*DocumentSubmission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, document_type_id::text, household_id::text,
		       COALESCE(swimmer_id::text, ''), status,
		       COALESCE(file_object_key, ''), COALESCE(file_name, ''),
		       COALESCE(file_size_bytes, 0), COALESCE(file_mime, ''),
		       COALESCE(notes, ''), COALESCE(review_notes, ''),
		       submitted_at, verified_at, COALESCE(verified_by, ''),
		       expires_at, reminder_sent_at,
		       created_at, updated_at,
		       COALESCE(created_by, ''), COALESCE(updated_by, '')
		FROM document_submissions WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("documents: load document_submissions: %w", err)
	}
	return dbutil.CollectRows(rows, "documents: scan document_submission", func(rows pgx.Rows) (*DocumentSubmission, error) {
		sub := &DocumentSubmission{}
		err := rows.Scan(
			&sub.ID, &sub.OrgID, &sub.DocumentTypeID, &sub.HouseholdID,
			&sub.SwimmerID, &sub.Status,
			&sub.FileObjectKey, &sub.FileName, &sub.FileSizeBytes, &sub.FileMIME,
			&sub.Notes, &sub.ReviewNotes,
			&sub.SubmittedAt, &sub.VerifiedAt, &sub.VerifiedBy,
			&sub.ExpiresAt, &sub.ReminderSentAt,
			&sub.CreatedAt, &sub.UpdatedAt, &sub.CreatedBy, &sub.UpdatedBy,
		)
		return sub, err
	})
}

// DeleteSubmissionsByHousehold hard-deletes every document_submissions row
// for a household, as part of an approved compliance right-to-erasure
// request (a soft delete would leave file names/notes/reviewer notes
// queryable). The caller is responsible for purging the returned object
// keys from object storage.
func (r *Repository) DeleteSubmissionsByHousehold(ctx context.Context, orgID, householdID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM document_submissions WHERE org_id = $1::uuid AND household_id = $2::uuid`,
		orgID, householdID,
	)
	return err
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	types []*DocumentType,
	subs []*DocumentSubmission,
	err error,
) {
	if types, err = r.LoadDocumentTypes(ctx); err != nil {
		return
	}
	subs, err = r.LoadSubmissions(ctx)
	return
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func nullableInt64(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
