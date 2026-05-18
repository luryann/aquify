package imports

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the imports domain.
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

// ─── File Imports ─────────────────────────────────────────────────────────────

func (r *Repository) UpsertFileImport(ctx context.Context, imp *FileImport) error {
	var reportJSON []byte
	if imp.ImportReport != nil {
		var err error
		reportJSON, err = json.Marshal(imp.ImportReport)
		if err != nil {
			return fmt.Errorf("imports: marshal import report: %w", err)
		}
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO file_imports (id, org_id, meet_id, filename, file_type, status,
		                          storage_key, row_count, error_count, conflict_count,
		                          import_report, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
		  status         = EXCLUDED.status,
		  storage_key    = EXCLUDED.storage_key,
		  row_count      = EXCLUDED.row_count,
		  error_count    = EXCLUDED.error_count,
		  conflict_count = EXCLUDED.conflict_count,
		  import_report  = EXCLUDED.import_report,
		  updated_at     = EXCLUDED.updated_at`,
		imp.ID, imp.OrgID, nullableString(imp.MeetID),
		imp.Filename, imp.FileType, imp.Status,
		nullableString(imp.StorageKey),
		imp.RowCount, imp.ErrorCount, imp.ConflictCount,
		reportJSON,
		imp.CreatedAt, imp.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadFileImports(ctx context.Context) ([]*FileImport, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, COALESCE(meet_id::text, ''),
		       filename, file_type, status,
		       COALESCE(storage_key, ''), row_count, error_count, conflict_count,
		       import_report, created_at, updated_at
		FROM file_imports WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("imports: load file_imports: %w", err)
	}
	defer rows.Close()
	var out []*FileImport
	for rows.Next() {
		imp := &FileImport{}
		var reportJSON []byte
		if err := rows.Scan(
			&imp.ID, &imp.OrgID, &imp.MeetID,
			&imp.Filename, &imp.FileType, &imp.Status,
			&imp.StorageKey, &imp.RowCount, &imp.ErrorCount, &imp.ConflictCount,
			&reportJSON, &imp.CreatedAt, &imp.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("imports: scan file_import: %w", err)
		}
		if len(reportJSON) > 0 {
			var report ImportReport
			if err := json.Unmarshal(reportJSON, &report); err == nil {
				imp.ImportReport = &report
			}
		}
		out = append(out, imp)
	}
	return out, rows.Err()
}

// ─── Match Conflicts ──────────────────────────────────────────────────────────

func (r *Repository) UpsertMatchConflict(ctx context.Context, c *MatchConflict) error {
	candidateIDs := make([]string, len(c.CandidateSwimmerIDs))
	copy(candidateIDs, c.CandidateSwimmerIDs)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO match_conflicts (id, import_id, org_id, row_number, raw_data,
		                             status, conflict_type, candidate_swimmer_ids,
		                             resolved_swimmer_id, resolver_id, resolution_note,
		                             resolved_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8::uuid[], $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
		  status              = EXCLUDED.status,
		  resolved_swimmer_id = EXCLUDED.resolved_swimmer_id,
		  resolver_id         = EXCLUDED.resolver_id,
		  resolution_note     = EXCLUDED.resolution_note,
		  resolved_at         = EXCLUDED.resolved_at,
		  updated_at          = EXCLUDED.updated_at`,
		c.ID, c.ImportID, c.OrgID,
		c.RowNumber, nullableString(c.RawData),
		c.Status, c.ConflictType,
		candidateIDs,
		nullableString(c.ResolvedSwimmerID),
		nullableString(c.ResolverID),
		nullableString(c.ResolutionNote),
		c.ResolvedAt,
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadMatchConflicts(ctx context.Context) ([]*MatchConflict, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, import_id::text, org_id::text, COALESCE(row_number, 0),
		       COALESCE(raw_data, ''), status, conflict_type,
		       COALESCE(candidate_swimmer_ids::text[], '{}'),
		       COALESCE(resolved_swimmer_id::text, ''), COALESCE(resolver_id::text, ''),
		       COALESCE(resolution_note, ''), resolved_at, created_at, updated_at
		FROM match_conflicts`)
	if err != nil {
		return nil, fmt.Errorf("imports: load match_conflicts: %w", err)
	}
	defer rows.Close()
	var out []*MatchConflict
	for rows.Next() {
		c := &MatchConflict{}
		if err := rows.Scan(
			&c.ID, &c.ImportID, &c.OrgID, &c.RowNumber,
			&c.RawData, &c.Status, &c.ConflictType,
			&c.CandidateSwimmerIDs,
			&c.ResolvedSwimmerID, &c.ResolverID, &c.ResolutionNote,
			&c.ResolvedAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("imports: scan match_conflict: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) ([]*FileImport, []*MatchConflict, error) {
	imports, err := r.LoadFileImports(ctx)
	if err != nil {
		return nil, nil, err
	}
	conflicts, err := r.LoadMatchConflicts(ctx)
	if err != nil {
		return nil, nil, err
	}
	return imports, conflicts, nil
}
