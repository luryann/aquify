package compliance

import (
	"errors"
	"testing"
	"time"
)

func TestComplianceRequestLifecycleAndSnapshots(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithConfig(7, 14)
	exportReq, err := svc.CreateDataExportRequest("org-a", "household-1", "parent-1")
	if err != nil {
		t.Fatalf("CreateDataExportRequest() error = %v", err)
	}
	if exportReq.Status != "pending" || exportReq.ExpiresAt == nil {
		t.Fatalf("CreateDataExportRequest() = %+v, want pending request with expiry", exportReq)
	}

	deletionReq, err := svc.CreateDataDeletionRequest("org-a", "household-1", "parent-1", "  Leaving club  ")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest() error = %v", err)
	}
	if deletionReq.Status != "pending" || deletionReq.Reason != "Leaving club" || deletionReq.ExpiresAt == nil {
		t.Fatalf("CreateDataDeletionRequest() = %+v, want trimmed pending request with expiry", deletionReq)
	}

	if _, err := svc.GetDataExportRequest("org-b", exportReq.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("GetDataExportRequest() cross-org error = %v, want %v", err, ErrExportNotFound)
	}
	if _, err := svc.GetDataDeletionRequest("org-b", deletionReq.ID); !errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("GetDataDeletionRequest() cross-org error = %v, want %v", err, ErrDeletionNotFound)
	}

	snapshot, err := svc.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("ExportRuntimeSnapshot() error = %v", err)
	}
	restored := NewService()
	if err := restored.LoadRuntimeSnapshot(snapshot); err != nil {
		t.Fatalf("LoadRuntimeSnapshot() error = %v", err)
	}

	exports, err := restored.ListDataExportRequests("org-a", "household-1")
	if err != nil {
		t.Fatalf("ListDataExportRequests() error = %v", err)
	}
	if len(exports) != 1 || exports[0].ID != exportReq.ID {
		t.Fatalf("restored exports = %+v, want original export request", exports)
	}
	deletions, err := restored.ListDataDeletionRequests("org-a", "household-1")
	if err != nil {
		t.Fatalf("ListDataDeletionRequests() error = %v", err)
	}
	if len(deletions) != 1 || deletions[0].ID != deletionReq.ID {
		t.Fatalf("restored deletions = %+v, want original deletion request", deletions)
	}
}

func TestDataExportStatusTransitions(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithConfig(7, 14)
	req, err := svc.CreateDataExportRequest("org-a", "household-1", "parent-1")
	if err != nil {
		t.Fatalf("CreateDataExportRequest() error = %v", err)
	}

	processing, err := svc.MarkDataExportProcessing("org-a", req.ID)
	if err != nil {
		t.Fatalf("MarkDataExportProcessing() error = %v", err)
	}
	if processing.Status != "processing" {
		t.Fatalf("processing status = %q, want processing", processing.Status)
	}
	if _, err := svc.MarkDataExportProcessing("org-a", req.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second MarkDataExportProcessing() error = %v, want %v", err, ErrInvalidTransition)
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	ready, err := svc.CompleteDataExport("org-a", req.ID, "https://example.test/export.json", expiresAt)
	if err != nil {
		t.Fatalf("CompleteDataExport() error = %v", err)
	}
	if ready.Status != "ready" || ready.DownloadURL == "" || ready.CompletedAt == nil || ready.ExpiresAt == nil {
		t.Fatalf("CompleteDataExport() = %+v, want ready request with download metadata", ready)
	}
	if _, err := svc.CompleteDataExport("org-a", req.ID, "https://example.test/export.json", expiresAt); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second CompleteDataExport() error = %v, want %v", err, ErrInvalidTransition)
	}
	// A terminal (ready) request must not be force-failed.
	if _, err := svc.FailDataExport("org-a", req.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("FailDataExport() from ready error = %v, want %v", err, ErrInvalidTransition)
	}

	req2, err := svc.CreateDataExportRequest("org-a", "household-2", "parent-2")
	if err != nil {
		t.Fatalf("CreateDataExportRequest() error = %v", err)
	}
	if _, err := svc.MarkDataExportProcessing("org-a", req2.ID); err != nil {
		t.Fatalf("MarkDataExportProcessing() error = %v", err)
	}
	failed, err := svc.FailDataExport("org-a", req2.ID)
	if err != nil {
		t.Fatalf("FailDataExport() from processing error = %v", err)
	}
	if failed.Status != "failed" {
		t.Fatalf("failed status = %q, want failed", failed.Status)
	}
}

func TestDataDeletionStatusTransitions(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithConfig(7, 14)
	req, err := svc.CreateDataDeletionRequest("org-a", "household-1", "parent-1", "requested")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest() error = %v", err)
	}

	processing, err := svc.MarkDataDeletionProcessing("org-a", req.ID)
	if err != nil {
		t.Fatalf("MarkDataDeletionProcessing() error = %v", err)
	}
	if processing.Status != "processing" {
		t.Fatalf("processing status = %q, want processing", processing.Status)
	}
	if _, err := svc.MarkDataDeletionProcessing("org-a", req.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second MarkDataDeletionProcessing() error = %v, want %v", err, ErrInvalidTransition)
	}

	completed, err := svc.CompleteDataDeletion("org-a", req.ID, "completed by worker")
	if err != nil {
		t.Fatalf("CompleteDataDeletion() error = %v", err)
	}
	if completed.Status != "completed" || completed.CompletedAt == nil || completed.ReviewNotes != "completed by worker" {
		t.Fatalf("CompleteDataDeletion() = %+v, want completed request", completed)
	}
	if _, err := svc.CompleteDataDeletion("org-a", req.ID, "again"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second CompleteDataDeletion() error = %v, want %v", err, ErrInvalidTransition)
	}
	// A terminal (completed) request must not be force-failed.
	if _, err := svc.FailDataDeletion("org-a", req.ID, "failed after completion"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("FailDataDeletion() from completed error = %v, want %v", err, ErrInvalidTransition)
	}

	req2, err := svc.CreateDataDeletionRequest("org-a", "household-2", "parent-2", "requested")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest() error = %v", err)
	}
	if _, err := svc.MarkDataDeletionProcessing("org-a", req2.ID); err != nil {
		t.Fatalf("MarkDataDeletionProcessing() error = %v", err)
	}
	failed, err := svc.FailDataDeletion("org-a", req2.ID, "failed while processing")
	if err != nil {
		t.Fatalf("FailDataDeletion() from processing error = %v", err)
	}
	if failed.Status != "failed" || failed.ReviewNotes != "failed while processing" {
		t.Fatalf("failed deletion = %+v, want failed request with notes", failed)
	}
}
