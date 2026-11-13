package jobhandlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/reports"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/luryann/aquify/server/internal/workers"
)

// Artifacts whose storage key is blank have no object to remove: they are
// expired in the index and the sweep must not try to delete "" from storage or fail.
func TestCov1xPurgeExpiredFilesSkipsBlankStorageKeys(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	store := cov1xNewDelStore(nil)

	exportsService := exports.NewServiceWithPool(pool, store)
	exportID := uuid.NewString()
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO file_exports (id, org_id, export_type, status, filename, size_bytes, storage_key, expires_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, 'standard_sd3', 'ready', 'f.sd3', 0, '   ', $3, now(), now())`, exportID, org, past); err != nil {
		t.Fatal(err)
	}

	reportsService := reports.NewService(nil, nil, nil, nil)
	reportsService.SetStorage(store)
	snap, _ := json.Marshal(map[string]any{"files": map[string]reports.StoredArtifact{
		"a1": {ID: "a1", OrgID: org, Kind: "k", Filename: "x.csv", StorageKey: "  ", ExpiresAt: past, CreatedAt: past},
	}})
	if err := reportsService.LoadRuntimeSnapshot(snap); err != nil {
		t.Fatal(err)
	}

	err := PurgeExpiredFilesHandler(cov1xDiscardLogger(), exportsService, reportsService)(context.Background(),
		cov1xTask(t, workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{OrgID: org}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	after, err := reportsService.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var left struct {
		Files map[string]reports.StoredArtifact `json:"files"`
	}
	if err := json.Unmarshal(after, &left); err != nil || len(left.Files) != 0 {
		t.Fatalf("report artifact not expired: %v err=%v", left.Files, err)
	}
	expired, err := exportsService.ExpireArtifactsForOrg(org, time.Now().UTC())
	if err != nil || len(expired) != 0 {
		t.Fatalf("export should already be expired: %v err=%v", expired, err)
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM file_exports WHERE id = $1::uuid`, exportID).Scan(&status); err != nil || status != "expired" {
		t.Fatalf("db status = %q err=%v, want expired", status, err)
	}
}
