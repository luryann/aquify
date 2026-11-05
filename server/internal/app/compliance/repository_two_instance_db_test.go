package compliance

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// The API and worker processes each hold their own Service on one database.
// Per-row loads (id is a TEXT column) must see and update the other's writes.
func TestDBTwoInstanceExportLifecycle(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	api := NewServiceWithPool(pool, 30, 30)
	worker := NewServiceWithPool(pool, 30, 30)

	req, err := api.CreateDataExportRequest(org, hh, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := worker.GetDataExportRequest(org, req.ID)
	if err != nil || got.ID != req.ID || got.Status != "pending" {
		t.Fatalf("worker get = %+v err=%v", got, err)
	}
	if _, err := worker.MarkDataExportProcessing(org, req.ID); err != nil {
		t.Fatalf("processing: %v", err)
	}
	exp := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := worker.CompleteDataExport(org, req.ID, "https://example.test/x.zip", exp); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got, err = api.GetDataExportRequest(org, req.ID); err != nil || got.Status != "ready" || got.DownloadURL == "" {
		t.Fatalf("api get after complete = %+v err=%v", got, err)
	}

	req2, err := api.CreateDataExportRequest(org, hh, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.FailDataExport(org, req2.ID); err != nil {
		t.Fatalf("fail on pending: %v", err)
	}
	if got, err = api.GetDataExportRequest(org, req2.ID); err != nil || got.Status != "failed" {
		t.Fatalf("api get after fail = %+v err=%v", got, err)
	}
}

func TestDBTwoInstanceDeletionGet(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	api := NewServiceWithPool(pool, 30, 30)
	worker := NewServiceWithPool(pool, 30, 30)

	req, err := api.CreateDataDeletionRequest(org, hh, "parent-1", "leaving")
	if err != nil {
		t.Fatal(err)
	}
	got, err := worker.GetDataDeletionRequest(org, req.ID)
	if err != nil || got.ID != req.ID || got.Status != "pending" {
		t.Fatalf("worker get = %+v err=%v", got, err)
	}
	if _, err := api.ApproveDataDeletion(org, req.ID, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got, err = worker.GetDataDeletionRequest(org, req.ID); err != nil || got.Status != "approved" {
		t.Fatalf("worker get after approve = %+v err=%v", got, err)
	}
}
