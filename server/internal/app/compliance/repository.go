package compliance

import (
	"context"
	"fmt"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the compliance domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Data Export Requests ─────────────────────────────────────────────────────

func (r *Repository) UpsertDataExportRequest(ctx context.Context, req *DataExportRequest) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO data_export_requests (id, org_id, household_id, requested_by, status,
		                                  download_url, requested_at, completed_at, expires_at,
		                                  created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11)
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

func (r *Repository) LoadDataExportRequests(ctx context.Context) ([]*DataExportRequest, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, requested_by, status,
		       COALESCE(download_url, ''), requested_at, completed_at, expires_at,
		       created_at, updated_at
		FROM data_export_requests WHERE deleted_at IS NULL`)
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
	_, err := r.pool.Exec(ctx, `
		INSERT INTO data_deletion_requests (id, org_id, household_id, requested_by, status,
		                                    reason, review_notes, requested_at, completed_at,
		                                    expires_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12)
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

func (r *Repository) LoadDataDeletionRequests(ctx context.Context) ([]*DataDeletionRequest, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, requested_by, status,
		       COALESCE(reason, ''), COALESCE(review_notes, ''),
		       requested_at, completed_at, expires_at,
		       created_at, updated_at
		FROM data_deletion_requests WHERE deleted_at IS NULL`)
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

func (r *Repository) LoadAll(ctx context.Context) (
	exports []*DataExportRequest,
	deletions []*DataDeletionRequest,
	err error,
) {
	if exports, err = r.LoadDataExportRequests(ctx); err != nil {
		return
	}
	deletions, err = r.LoadDataDeletionRequests(ctx)
	return
}
