package jobhandlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

func TestPurgeExpiredFilesHandler_ExpiresImportsExportsAndReports(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := storage.NewStubStorage("")
	importsService := imports.NewServiceWithStorage(store)
	exportsService := exports.NewServiceWithStorage(store)
	reportsService := reports.NewService(nil, nil, nil, nil)
	reportsService.SetStorage(store)

	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: "org-cleanup", Roles: []string{"admin"}}

	importRecord, _, err := importsService.RequestUploadURL(principal, "org-cleanup", imports.RequestUploadURLInput{
		Filename:  "results.sd3",
		FileType:  "sd3",
		SizeBytes: 1024,
		MIMEType:  "application/octet-stream",
	})
	if err != nil {
		t.Fatalf("RequestUploadURL: %v", err)
	}
	if err := store.PutObject(context.Background(), importRecord.StorageKey, "application/octet-stream", []byte("import-payload")); err != nil {
		t.Fatalf("seed import object: %v", err)
	}
	importSnapshot, err := importsService.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("ExportRuntimeSnapshot(imports): %v", err)
	}
	var importState struct {
		Imports map[string]*imports.FileImport `json:"imports"`
	}
	if err := json.Unmarshal(importSnapshot, &importState); err != nil {
		t.Fatalf("decode import snapshot: %v", err)
	}
	expiredUploadAt := time.Now().UTC().Add(-time.Minute)
	importState.Imports[importRecord.ID].UploadURLExpiresAt = &expiredUploadAt
	if err := importsService.LoadRuntimeSnapshot(mustJSON(t, importState)); err != nil {
		t.Fatalf("LoadRuntimeSnapshot(imports): %v", err)
	}

	exportsService.ConfigureArtifactTTL(time.Millisecond)
	exportRecord, _, err := exportsService.GenerateExport(principal, "org-cleanup", "meet-cleanup", exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet: exports.ReportMeet{
			Name:      "Cleanup Meet",
			StartDate: "2026-03-15",
			EndDate:   "2026-03-16",
			Course:    "SCY",
		},
	})
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}

	reportsService.ConfigureArtifactTTL(time.Millisecond)
	reportArtifact, err := reportsService.StoreCSVArtifact("org-cleanup", "invoice_aging", time.Now().UTC(), &reports.CSVDocument{
		Filename: "invoice-aging.csv",
		Data:     []byte("invoice_id\ninv-1\n"),
	})
	if err != nil {
		t.Fatalf("StoreCSVArtifact: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	cutoff := time.Now().UTC()
	task, err := workers.NewTask(workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{
		OrgID:      "org-cleanup",
		CutoffTime: cutoff.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := PurgeExpiredFilesHandler(logger, importsService, exportsService, reportsService)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	importAfter, err := importsService.GetImport(principal, "org-cleanup", importRecord.ID)
	if err != nil {
		t.Fatalf("GetImport: %v", err)
	}
	if importAfter.Status != "expired" {
		t.Fatalf("import status = %q, want expired", importAfter.Status)
	}

	exportAfter, err := exportsService.GetExport(principal, "org-cleanup", exportRecord.ID)
	if err != nil {
		t.Fatalf("GetExport: %v", err)
	}
	if exportAfter.Status != "expired" {
		t.Fatalf("export status = %q, want expired", exportAfter.Status)
	}

	reportSnapshot, err := reportsService.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("ExportRuntimeSnapshot: %v", err)
	}
	var snapshot struct {
		Files map[string]reports.StoredArtifact `json:"files"`
	}
	if err := json.Unmarshal(reportSnapshot, &snapshot); err != nil {
		t.Fatalf("decode report snapshot: %v", err)
	}
	if len(snapshot.Files) != 0 {
		t.Fatalf("expected report artifacts to be purged, got %d", len(snapshot.Files))
	}

	for _, key := range []string{importRecord.StorageKey, exportRecord.StorageKey, reportArtifact.StorageKey} {
		exists, err := store.Exists(context.Background(), key)
		if err != nil {
			t.Fatalf("Exists(%q): %v", key, err)
		}
		if exists {
			t.Fatalf("expected storage key %q to be deleted", key)
		}
	}
}

func TestProcessDataExportHandler_CompletesRequestAndStoresArtifact(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	complianceService := compliance.NewService()
	store := storage.NewStubStorage("http://storage.test/aquify")
	req, err := complianceService.CreateDataExportRequest("org-export", "hh-export", "admin-user")
	if err != nil {
		t.Fatalf("CreateDataExportRequest: %v", err)
	}
	task, err := workers.NewTask(workers.TypeProcessDataExport, ProcessDataExportPayload{
		RequestID:   req.ID,
		OrgID:       "org-export",
		HouseholdID: "hh-export",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := ProcessDataExportHandler(logger, complianceService, store)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	after, err := complianceService.GetDataExportRequest("org-export", req.ID)
	if err != nil {
		t.Fatalf("GetDataExportRequest: %v", err)
	}
	if after.Status != "ready" {
		t.Fatalf("status = %q, want ready", after.Status)
	}
	if after.DownloadURL == "" {
		t.Fatal("DownloadURL is empty")
	}
	if after.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}

	key := complianceExportStorageKey("org-export", req.ID)
	data, contentType, err := store.GetObject(context.Background(), key)
	if err != nil {
		t.Fatalf("GetObject(%q): %v", key, err)
	}
	if contentType != "application/json; charset=utf-8" {
		t.Fatalf("contentType = %q", contentType)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	if artifact["request_id"] != req.ID || artifact["household_id"] != "hh-export" {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
}

func TestProcessDataDeletionHandler_CompletesRequest(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	complianceService := compliance.NewService()
	membersService := members.NewService()

	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: "org-delete", Roles: []string{"admin"}}
	household, err := membersService.CreateHousehold(principal, members.CreateHouseholdInput{
		OrgID:         "org-delete",
		HouseholdName: "The Delete Household",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	req, err := complianceService.CreateDataDeletionRequest("org-delete", household.ID, "admin-user", "requested")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest: %v", err)
	}
	task, err := workers.NewTask(workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{
		RequestID:   req.ID,
		OrgID:       "org-delete",
		HouseholdID: household.ID,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := ProcessDataDeletionHandler(logger, complianceService, membersService)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	anonymized, err := membersService.GetHousehold(principal, "org-delete", household.ID)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if anonymized.HouseholdName != "Deleted Household" {
		t.Fatalf("household not anonymized: %+v", anonymized)
	}

	after, err := complianceService.GetDataDeletionRequest("org-delete", req.ID)
	if err != nil {
		t.Fatalf("GetDataDeletionRequest: %v", err)
	}
	if after.Status != "completed" {
		t.Fatalf("status = %q, want completed", after.Status)
	}
	if after.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}
	if after.ReviewNotes == "" {
		t.Fatal("ReviewNotes is empty")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return data
}
