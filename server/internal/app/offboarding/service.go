// Package offboarding exports and permanently removes one org's data. It works
// against the whole schema (every table with an org_id), so new tenant tables are
// covered without code changes.
package offboarding

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

var (
	ErrNotOffboarding   = errors.New("organization must be in the offboarding state")
	ErrCoolingOff       = errors.New("the cooling-off period has not passed")
	ErrConfirmMismatch  = errors.New("confirmation does not match the organization's slug")
	ErrProtectedOrg     = errors.New("this organization cannot be offboarded")
	ErrNoDatabase       = errors.New("offboarding needs a database")
	ErrStorageUnsupport = errors.New("object storage does not support bulk delete")
)

// DefaultCoolingOff is how long an org stays in offboarding before it can be purged.
const DefaultCoolingOff = 14 * 24 * time.Hour

// Tables never exported: they hold credentials or secrets.
var exportExcluded = map[string]string{
	"sessions":                  "session token hashes",
	"mfa_factors":               "TOTP secrets and backup codes",
	"calendar_feed_tokens":      "feed token hashes",
	"org_invites":               "invite code hashes",
	"usa_swimming_integrations": "encrypted third-party credentials",
	"org_payment_accounts":      "payment provider account link",
	"webhook_events":            "raw provider payloads",
}

// Tables a purge leaves in place. audit_events is append-only by design and is
// retained as the record of what happened; orgs keeps a tombstone so the id and
// slug are never reused and the audit rows keep their parent.
var purgeKept = map[string]string{
	"audit_events": "immutable audit trail is retained",
	// Change counters only ever grow; purging them could rewind a counter to a value another
	// process remembers, so a cache would treat changed data as unchanged.
	"org_data_versions": "change counters must never rewind",
	"orgs":              "tombstone keeps the id reserved (the slug is released)",
}

// Service performs exports and purges.
type Service struct {
	Pool       *pgxpool.Pool
	Orgs       *orgs.Service
	Store      storage.ObjectStorage
	Keycloak   keycloak.Client
	Logger     *slog.Logger
	CoolingOff time.Duration
	Now        func() time.Time
	// PlatformOrgID can never be offboarded.
	PlatformOrgID string
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) cooling() time.Duration {
	if s.CoolingOff > 0 {
		return s.CoolingOff
	}
	return DefaultCoolingOff
}

// tenantTables lists every base table in the current schema that has an org_id.
func (s *Service) tenantTables(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT c.table_name FROM information_schema.columns c
JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
WHERE c.table_schema = current_schema() AND c.column_name = 'org_id' ORDER BY c.table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ExportResult describes a finished export.
type ExportResult struct {
	Key    string         `json:"key"`
	Tables map[string]int `json:"tables"`
	Files  int            `json:"files"`
	Bytes  int            `json:"bytes"`
}

// ExportKeyPrefix is where an org's offboarding archives live. It is deliberately
// outside storagePrefixes: Purge must not delete the archive that exists so the
// org can be restored afterwards.
func ExportKeyPrefix(orgID string) string { return "offboarding/" + orgID + "/export-" }

// storagePrefixes are the object-key prefixes an org owns (and Purge deletes).
func storagePrefixes(orgID string) []string {
	return []string{"documents/" + orgID + "/", "messages/" + orgID + "/", "exports/" + orgID + "/", "reports/" + orgID + "/", "compliance/" + orgID + "/"}
}

// Export writes a zip (one NDJSON file per table plus a manifest of stored files)
// to object storage and returns its key. Credentials are excluded.
func (s *Service) Export(ctx context.Context, orgID string) (*ExportResult, error) {
	if s.Pool == nil {
		return nil, ErrNoDatabase
	}
	if _, err := s.Orgs.Get(ctx, orgID); err != nil {
		return nil, err
	}
	tables, err := s.tenantTables(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	res := &ExportResult{Tables: map[string]int{}}
	for _, table := range tables {
		if _, skip := exportExcluded[table]; skip {
			continue
		}
		n, err := s.writeTable(ctx, zw, table, orgID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", table, err)
		}
		if n > 0 {
			res.Tables[table] = n
		}
	}
	// Manifest of stored files with 7-day download links.
	files := []map[string]string{}
	if ps, ok := s.Store.(storage.PrefixStore); ok {
		for _, prefix := range storagePrefixes(orgID) {
			keys, err := ps.ListPrefix(ctx, prefix)
			if err != nil {
				return nil, err
			}
			for _, k := range keys {
				url, _ := s.Store.PresignedGetURL(k, 7*24*time.Hour)
				files = append(files, map[string]string{"key": k, "download_url": url})
			}
		}
	}
	res.Files = len(files)
	w, err := zw.Create("files.json")
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(w).Encode(files); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	res.Bytes = buf.Len()
	res.Key = fmt.Sprintf("offboarding/%s/export-%s.zip", orgID, s.now().UTC().Format("20060102T150405Z"))
	if err := s.Store.PutObject(ctx, res.Key, "application/zip", buf.Bytes()); err != nil {
		return nil, fmt.Errorf("store export: %w", err)
	}
	return res, nil
}

func (s *Service) writeTable(ctx context.Context, zw *zip.Writer, table, orgID string) (int, error) {
	// Bound to the org so row-level security (where enabled) lets exactly this org's rows through.
	rows, err := tenantdb.Pool{P: s.Pool}.For(orgID).Query(ctx, fmt.Sprintf(`SELECT row_to_json(t)::text FROM %s t WHERE org_id = $1::uuid`, pgx.Identifier{table}.Sanitize()), orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var w interface{ Write([]byte) (int, error) }
	n := 0
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return n, err
		}
		if w == nil {
			fw, err := zw.Create(table + ".ndjson")
			if err != nil {
				return n, err
			}
			w = fw
		}
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// PurgeResult summarises a purge.
type PurgeResult struct {
	RowsDeleted  map[string]int `json:"rows_deleted"`
	FilesDeleted int            `json:"files_deleted"`
	UsersDeleted int            `json:"users_deleted"`
}

// Purge permanently deletes the org's data. Preconditions: not the platform org,
// status offboarding for at least the cooling-off period, and confirmSlug typed
// exactly. The org row stays as a tombstone (status deleted, slug released).
func (s *Service) Purge(ctx context.Context, orgID, confirmSlug string) (*PurgeResult, error) {
	if s.Pool == nil {
		return nil, ErrNoDatabase
	}
	if s.PlatformOrgID != "" && orgs.CanonicalID(orgID) == orgs.CanonicalID(s.PlatformOrgID) {
		return nil, ErrProtectedOrg
	}
	org, err := s.Orgs.Get(ctx, orgID)
	if err != nil {
		return nil, err
	}
	// A purge that failed part-way leaves the org in purging and may be retried.
	if (org.Status != orgs.StatusOffboarding && org.Status != orgs.StatusPurging) || org.OffboardingAt == nil {
		return nil, ErrNotOffboarding
	}
	if s.now().Sub(*org.OffboardingAt) < s.cooling() {
		return nil, fmt.Errorf("%w: eligible after %s", ErrCoolingOff, org.OffboardingAt.Add(s.cooling()).UTC().Format(time.RFC3339))
	}
	if org.Slug == "" || strings.TrimSpace(confirmSlug) != org.Slug {
		return nil, ErrConfirmMismatch
	}
	ps, ok := s.Store.(storage.PrefixStore)
	if !ok {
		return nil, ErrStorageUnsupport
	}

	res := &PurgeResult{RowsDeleted: map[string]int{}}

	// Claim the purge before anything irreversible: the guarded transition fails if a
	// concurrent cancel-offboarding already moved the org, and once claimed cancel is refused.
	if org.Status == orgs.StatusOffboarding {
		if _, err := s.Orgs.SetStatus(ctx, orgID, orgs.StatusPurging); err != nil {
			return nil, err
		}
	}

	// 1. Identity provider users of this org (before rows that reference them go).
	if s.Keycloak != nil {
		users, err := s.listAllUsers(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("list identity users: %w", err)
		}
		for _, u := range users {
			if err := s.Keycloak.DeleteUser(ctx, u.ID); err != nil && !errors.Is(err, keycloak.ErrUserNotFound) {
				return res, fmt.Errorf("delete identity user %s: %w", u.ID, err)
			}
			res.UsersDeleted++
		}
	}

	// 2. Stored files.
	for _, prefix := range storagePrefixes(orgID) {
		n, err := ps.DeletePrefix(ctx, prefix)
		res.FilesDeleted += n
		if err != nil {
			return res, err
		}
	}

	// 3. Database rows, children first (retry order until everything is gone).
	tables, err := s.tenantTables(ctx)
	if err != nil {
		return res, err
	}
	pending := []string{}
	for _, t := range tables {
		if _, keep := purgeKept[t]; !keep {
			pending = append(pending, t)
		}
	}
	sort.Strings(pending)
	for pass := 0; len(pending) > 0 && pass < len(tables)+2; pass++ {
		var stuck []string
		for _, t := range pending {
			n, err := s.deleteRows(ctx, t, orgID)
			if err != nil {
				stuck = append(stuck, t)
				continue
			}
			if n > 0 {
				res.RowsDeleted[t] += n
			}
		}
		if len(stuck) == len(pending) {
			return res, fmt.Errorf("could not delete rows from %v (still referenced)", stuck)
		}
		pending = stuck
	}

	// 4. Tombstone: free the slug and domains, mark deleted.
	if err := s.Orgs.Tombstone(ctx, orgID); err != nil {
		return res, err
	}
	s.logger().Info("org purged", "org_id", orgID, "files", res.FilesDeleted, "users", res.UsersDeleted)
	return res, nil
}

// deleteRows removes one table's rows inside a savepointed transaction so an FK
// refusal leaves nothing half-deleted and the caller can retry later in the pass.
func (s *Service) deleteRows(ctx context.Context, table, orgID string) (int, error) {
	tx, err := tenantdb.Pool{P: s.Pool}.For(orgID).Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE org_id = $1::uuid`, pgx.Identifier{table}.Sanitize()), orgID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Service) listAllUsers(ctx context.Context, orgID string) ([]keycloak.User, error) {
	var all []keycloak.User
	for offset := 0; ; offset += 200 {
		batch, err := s.Keycloak.ListUsers(ctx, orgID, "", 200, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 200 {
			return all, nil
		}
	}
}
