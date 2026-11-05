package compliance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// Repository handles DB persistence for the compliance domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// at binds orgID to every statement so row-level security on the compliance
// tables applies. An empty org sees nothing.
func (r *Repository) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: r.pool}.For(orgID) }

// ─── Data Export Requests ─────────────────────────────────────────────────────

func (r *Repository) UpsertDataExportRequest(ctx context.Context, req *DataExportRequest) error {
	_, err := r.at(req.OrgID).Exec(ctx, `
		INSERT INTO data_export_requests (id, org_id, household_id, requested_by, status,
		                                  download_url, requested_at, completed_at, expires_at,
		                                  created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
		  status       = EXCLUDED.status,
		  download_url = EXCLUDED.download_url,
		  completed_at = EXCLUDED.completed_at,
		  expires_at   = EXCLUDED.expires_at,
		  updated_at   = EXCLUDED.updated_at`,
		req.ID, req.OrgID, req.HouseholdID, req.RequestedBy, req.Status,
		dbutil.NullableString(req.DownloadURL), req.RequestedAt, req.CompletedAt, req.ExpiresAt,
		req.CreatedAt, req.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadDataExportRequests(ctx context.Context, orgID string) ([]*DataExportRequest, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, requested_by, status,
		       COALESCE(download_url, ''), requested_at, completed_at, expires_at,
		       created_at, updated_at
		FROM data_export_requests WHERE org_id = $1::uuid AND deleted_at IS NULL`, orgID)
	if err != nil {
		return nil, fmt.Errorf("compliance: load data_export_requests: %w", err)
	}
	return dbutil.CollectRows(rows, "compliance: scan data_export_request", func(rows pgx.Rows) (*DataExportRequest, error) {
		req := &DataExportRequest{}
		err := rows.Scan(
			&req.ID, &req.OrgID, &req.HouseholdID, &req.RequestedBy, &req.Status,
			&req.DownloadURL, &req.RequestedAt, &req.CompletedAt, &req.ExpiresAt,
			&req.CreatedAt, &req.UpdatedAt,
		)
		return req, err
	})
}

// ─── Data Deletion Requests ───────────────────────────────────────────────────

func (r *Repository) UpsertDataDeletionRequest(ctx context.Context, req *DataDeletionRequest) error {
	_, err := r.at(req.OrgID).Exec(ctx, `
		INSERT INTO data_deletion_requests (id, org_id, household_id, requested_by, status,
		                                    reason, review_notes, requested_at, completed_at,
		                                    expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  status       = EXCLUDED.status,
		  review_notes = EXCLUDED.review_notes,
		  completed_at = EXCLUDED.completed_at,
		  updated_at   = EXCLUDED.updated_at`,
		req.ID, req.OrgID, req.HouseholdID, req.RequestedBy, req.Status,
		dbutil.NullableString(req.Reason), dbutil.NullableString(req.ReviewNotes),
		req.RequestedAt, req.CompletedAt, req.ExpiresAt,
		req.CreatedAt, req.UpdatedAt,
	)
	return err
}

// TransitionDataDeletionRequest applies a status change only when the stored
// row is still in one of the allowed states, so concurrent approve/reject on
// different instances cannot both succeed. Zero rows affected means the state
// moved underneath the caller (ErrInvalidTransition).
func (r *Repository) TransitionDataDeletionRequest(ctx context.Context, req *DataDeletionRequest, from []string) error {
	tag, err := r.at(req.OrgID).Exec(ctx, `
		UPDATE data_deletion_requests
		SET status = $3, review_notes = $4, completed_at = $5, updated_at = $6
		WHERE id = $1 AND org_id = $2 AND status = ANY($7::text[]) AND deleted_at IS NULL`,
		req.ID, req.OrgID, req.Status, dbutil.NullableString(req.ReviewNotes), req.CompletedAt, req.UpdatedAt, from)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) LoadDataDeletionRequests(ctx context.Context, orgID string) ([]*DataDeletionRequest, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, requested_by, status,
		       COALESCE(reason, ''), COALESCE(review_notes, ''),
		       requested_at, completed_at, expires_at,
		       created_at, updated_at
		FROM data_deletion_requests WHERE org_id = $1::uuid AND deleted_at IS NULL`, orgID)
	if err != nil {
		return nil, fmt.Errorf("compliance: load data_deletion_requests: %w", err)
	}
	return dbutil.CollectRows(rows, "compliance: scan data_deletion_request", func(rows pgx.Rows) (*DataDeletionRequest, error) {
		req := &DataDeletionRequest{}
		err := rows.Scan(
			&req.ID, &req.OrgID, &req.HouseholdID, &req.RequestedBy, &req.Status,
			&req.Reason, &req.ReviewNotes,
			&req.RequestedAt, &req.CompletedAt, &req.ExpiresAt,
			&req.CreatedAt, &req.UpdatedAt,
		)
		return req, err
	})
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadOrg(ctx context.Context, orgID string) (
	exports []*DataExportRequest,
	deletions []*DataDeletionRequest,
	err error,
) {
	if exports, err = r.LoadDataExportRequests(ctx, orgID); err != nil {
		return
	}
	deletions, err = r.LoadDataDeletionRequests(ctx, orgID)
	return
}

// GetDataExportRequest loads one export request; (nil, nil) when absent.
func (r *Repository) GetDataExportRequest(ctx context.Context, orgID, id string) (*DataExportRequest, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, requested_by, status,
		       COALESCE(download_url, ''), requested_at, completed_at, expires_at,
		       created_at, updated_at
		FROM data_export_requests WHERE id = $1 AND org_id = $2::uuid AND deleted_at IS NULL`, id, orgID)
	if err != nil {
		return nil, fmt.Errorf("compliance: get data_export_request: %w", err)
	}
	list, err := dbutil.CollectRows(rows, "compliance: scan data_export_request", func(rows pgx.Rows) (*DataExportRequest, error) {
		req := &DataExportRequest{}
		err := rows.Scan(
			&req.ID, &req.OrgID, &req.HouseholdID, &req.RequestedBy, &req.Status,
			&req.DownloadURL, &req.RequestedAt, &req.CompletedAt, &req.ExpiresAt,
			&req.CreatedAt, &req.UpdatedAt,
		)
		return req, err
	})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

// GetDataDeletionRequest loads one deletion request; (nil, nil) when absent.
func (r *Repository) GetDataDeletionRequest(ctx context.Context, orgID, id string) (*DataDeletionRequest, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, requested_by, status,
		       COALESCE(reason, ''), COALESCE(review_notes, ''),
		       requested_at, completed_at, expires_at,
		       created_at, updated_at
		FROM data_deletion_requests WHERE id = $1 AND org_id = $2::uuid AND deleted_at IS NULL`, id, orgID)
	if err != nil {
		return nil, fmt.Errorf("compliance: get data_deletion_request: %w", err)
	}
	list, err := dbutil.CollectRows(rows, "compliance: scan data_deletion_request", func(rows pgx.Rows) (*DataDeletionRequest, error) {
		req := &DataDeletionRequest{}
		err := rows.Scan(
			&req.ID, &req.OrgID, &req.HouseholdID, &req.RequestedBy, &req.Status,
			&req.Reason, &req.ReviewNotes,
			&req.RequestedAt, &req.CompletedAt, &req.ExpiresAt,
			&req.CreatedAt, &req.UpdatedAt,
		)
		return req, err
	})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}
