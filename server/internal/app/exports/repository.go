package exports

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// Repository handles DB persistence for the exports domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// at binds orgID to every statement so row-level security on file_exports applies.
func (r *Repository) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: r.pool}.For(orgID) }

// ─── File Exports ─────────────────────────────────────────────────────────────

func (r *Repository) UpsertFileExport(ctx context.Context, ex *FileExport) error {
	_, err := r.at(ex.OrgID).Exec(ctx, `
		INSERT INTO file_exports (id, org_id, meet_id, export_type, status, filename,
		                          size_bytes, storage_key, expires_at, last_error,
		                          created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  status      = EXCLUDED.status,
		  filename    = EXCLUDED.filename,
		  size_bytes  = EXCLUDED.size_bytes,
		  storage_key = EXCLUDED.storage_key,
		  last_error  = EXCLUDED.last_error,
		  updated_at  = EXCLUDED.updated_at`,
		ex.ID, ex.OrgID, dbutil.NullableString(ex.MeetID),
		ex.ExportType, ex.Status, ex.Filename,
		ex.SizeBytes, dbutil.NullableString(ex.StorageKey),
		ex.ExpiresAt, dbutil.NullableString(ex.LastError),
		ex.CreatedAt, ex.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadFileExports(ctx context.Context, orgID string) ([]*FileExport, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, COALESCE(meet_id::text, ''),
		       export_type, status, filename,
		       size_bytes, COALESCE(storage_key, ''), expires_at,
		       COALESCE(last_error, ''), created_at, updated_at
		FROM file_exports WHERE org_id = $1::uuid AND deleted_at IS NULL`, orgID)
	if err != nil {
		return nil, fmt.Errorf("exports: load file_exports: %w", err)
	}
	return dbutil.CollectRows(rows, "exports: scan file_export", func(rows pgx.Rows) (*FileExport, error) {
		ex := &FileExport{}
		err := rows.Scan(
			&ex.ID, &ex.OrgID, &ex.MeetID,
			&ex.ExportType, &ex.Status, &ex.Filename,
			&ex.SizeBytes, &ex.StorageKey, &ex.ExpiresAt,
			&ex.LastError, &ex.CreatedAt, &ex.UpdatedAt,
		)
		return ex, err
	})
}
