package jobhandlers

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/luryann/aquify/server/internal/app/exports"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/workers"
)

// cov1xToggleStore fails PutObject while failPut is set.
type cov1xToggleStore struct {
	*storage.StubStorage
	failPut atomic.Bool
}

func (s *cov1xToggleStore) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if s.failPut.Load() {
		return errors.New("storage unavailable")
	}
	return s.StubStorage.PutObject(ctx, key, ct, data)
}

func TestCov1xGenerateExportHandlerRejectsBadInput(t *testing.T) {
	t.Parallel()
	svc := exports.NewServiceWithStorage(storage.NewStubStorage(""))
	cases := []struct {
		name    string
		svc     *exports.Service
		payload string
		want    string
	}{
		{"bad json", svc, `{`, "generate export: decode payload"},
		{"missing export id", svc, `{"org_id":"o"}`, "missing export_id"},
		{"missing org id", svc, `{"export_id":"e"}`, "missing org_id"},
		{"nil service", nil, `{"org_id":"o","export_id":"e"}`, "exports service is not configured"},
	}
	for _, tc := range cases {
		err := GenerateExportHandler(cov1xDiscardLogger(), tc.svc)(context.Background(), cov1xRawTask(workers.TypeGenerateExport, tc.payload))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCov1xGenerateExportHandlerUnknownExport(t *testing.T) {
	t.Parallel()
	svc := exports.NewServiceWithStorage(storage.NewStubStorage(""))
	err := GenerateExportHandler(cov1xDiscardLogger(), svc)(context.Background(),
		cov1xTask(t, workers.TypeGenerateExport, exports.GenerateExportTaskPayload{OrgID: "o", ExportID: "nope"}))
	if !errors.Is(err, exports.ErrExportNotFound) {
		t.Fatalf("err = %v, want ErrExportNotFound", err)
	}
}

// A failed synchronous generation leaves the export retryable; the worker must
// then render it and mark it ready.
func TestCov1xGenerateExportHandlerRetriesFailedExport(t *testing.T) {
	t.Parallel()
	const org, meet = "org-cov1x-exp", "meet-cov1x-exp"
	store := &cov1xToggleStore{StubStorage: storage.NewStubStorage("")}
	svc := exports.NewServiceWithStorage(store)
	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: org, Roles: []string{"admin"}}

	store.failPut.Store(true)
	_, _, err := svc.GenerateExport(principal, org, meet, exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet:       exports.ReportMeet{Name: "Retry Meet", StartDate: "2026-03-15", EndDate: "2026-03-16", Course: "SCY"},
	})
	if err == nil {
		t.Fatal("expected generation to fail while storage is down")
	}
	list := svc.ListExportsForAuthorizedMeet(org, meet)
	if len(list) != 1 || list[0].Status != "failed" {
		t.Fatalf("exports = %+v, want one failed export", list)
	}
	store.failPut.Store(false)

	logger, buf := cov1xBufLogger()
	if err := GenerateExportHandler(logger, svc)(context.Background(),
		cov1xTask(t, workers.TypeGenerateExport, exports.GenerateExportTaskPayload{OrgID: org, ExportID: list[0].ID})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	after := svc.ListExportsForAuthorizedMeet(org, meet)
	if len(after) != 1 || after[0].Status != "ready" {
		t.Fatalf("exports after worker = %+v, want ready", after)
	}
	if ok, _ := store.Exists(context.Background(), after[0].StorageKey); !ok {
		t.Fatal("rendered export object missing from storage")
	}
	if !strings.Contains(buf.String(), "generate export: completed") {
		t.Fatalf("expected completion log, got %q", buf.String())
	}
}
