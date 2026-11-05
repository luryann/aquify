package offboarding

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// restoreChunkRows is how many NDJSON rows go to the database per round trip.
const restoreChunkRows = 500

var ErrRestoreOrgMissing = errors.New("the organization's registry row must exist (a deleted org keeps its tombstone)")

// RestoreResult summarises a restore.
type RestoreResult struct {
	RowsRestored map[string]int `json:"rows_restored"`
	// Skipped are archive tables ignored because they are not tenant tables here.
	Skipped []string `json:"skipped,omitempty"`
}

// Restore loads an export archive back into the database for one org: the
// single-org recovery path (a full-database restore would roll back every club).
//
// Rules: only tables that exist and carry org_id are loaded; every row's org_id
// must equal orgID (rows naming another org are dropped, never inserted); rows
// that already exist are left alone; credentials were never exported, so people
// re-enrol MFA and sign in again; stored files are not part of the archive (its
// manifest lists them). The org comes back suspended: an operator resumes it
// deliberately after checking the data.
func (s *Service) Restore(ctx context.Context, orgID string, archive []byte) (*RestoreResult, error) {
	if s.Pool == nil {
		return nil, ErrNoDatabase
	}
	var status string
	if err := s.Pool.QueryRow(ctx, `SELECT status FROM orgs WHERE id = $1::uuid`, orgID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRestoreOrgMissing
		}
		return nil, err
	}
	if s.PlatformOrgID != "" && orgs.CanonicalID(orgID) == orgs.CanonicalID(s.PlatformOrgID) {
		return nil, ErrProtectedOrg
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	known, err := s.tenantTables(ctx)
	if err != nil {
		return nil, err
	}
	isTenant := map[string]bool{}
	for _, t := range known {
		isTenant[t] = true
	}
	res := &RestoreResult{RowsRestored: map[string]int{}}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, ".ndjson")
		if name == f.Name {
			continue // files.json manifest
		}
		if _, excluded := exportExcluded[name]; excluded || !isTenant[name] {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		files[name] = f
	}
	pending := make([]string, 0, len(files))
	for t := range files {
		pending = append(pending, t)
	}
	sort.Strings(pending)
	for pass := 0; len(pending) > 0 && pass < len(files)+2; pass++ {
		var stuck []string
		for _, table := range pending {
			n, err := s.loadTable(ctx, table, orgID, files[table])
			if err != nil {
				stuck = append(stuck, table)
				continue
			}
			res.RowsRestored[table] += n
		}
		if len(stuck) == len(pending) {
			return res, fmt.Errorf("could not restore %v (missing parent rows)", stuck)
		}
		pending = stuck
	}
	// Back to suspended (never straight to active); a purged org leaves 'deleted'.
	if _, err := s.Pool.Exec(ctx, `UPDATE orgs SET status = 'suspended', suspended_at = now(), offboarding_at = NULL WHERE id = $1::uuid`, orgID); err != nil {
		return res, err
	}
	return res, nil
}

// loadTable inserts one table's rows in a single transaction (all or nothing).
func (s *Service) loadTable(ctx context.Context, table, orgID string, f *zip.File) (int, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	tx, err := tenantdb.Pool{P: s.Pool}.For(orgID).Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := fmt.Sprintf(`INSERT INTO %[1]s SELECT r.* FROM json_populate_recordset(NULL::%[1]s, $1::json) r WHERE r.org_id = $2::uuid ON CONFLICT DO NOTHING`, pgx.Identifier{table}.Sanitize())
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	n := 0
	chunk := make([]string, 0, restoreChunkRows)
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		tag, err := tx.Exec(ctx, q, "["+strings.Join(chunk, ",")+"]", orgID)
		if err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
		n += int(tag.RowsAffected())
		chunk = chunk[:0]
		return nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		chunk = append(chunk, line)
		if len(chunk) == restoreChunkRows {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if err := flush(); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}
