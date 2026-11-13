package jobhandlers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/compliance"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/reports"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/luryann/aquify/server/internal/workers"
)

// cov1xDelStore records Delete calls per key and lets a test fail chosen calls.
type cov1xDelStore struct {
	*storage.StubStorage
	mu     sync.Mutex
	calls  map[string]int
	failOn func(key string, nth int) error
}

func cov1xNewDelStore(failOn func(key string, nth int) error) *cov1xDelStore {
	return &cov1xDelStore{StubStorage: storage.NewStubStorage(""), calls: map[string]int{}, failOn: failOn}
}

func (s *cov1xDelStore) Delete(key string) error {
	s.mu.Lock()
	s.calls[key]++
	n := s.calls[key]
	s.mu.Unlock()
	if s.failOn != nil {
		if err := s.failOn(key, n); err != nil {
			return err
		}
	}
	return s.StubStorage.Delete(key)
}

func (s *cov1xDelStore) deleteCount(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[key]
}

// cov1xHookPutStore runs a hook before every PutObject.
type cov1xHookPutStore struct {
	*storage.StubStorage
	hook func() error
}

func (s *cov1xHookPutStore) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if s.hook != nil {
		if err := s.hook(); err != nil {
			return err
		}
	}
	return s.StubStorage.PutObject(ctx, key, ct, data)
}

func cov1xAdminFor(org string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "admin-cov1x", OrgID: org, Roles: []string{"admin"}}
}

// ---------------------------------------------------------------- purge

func TestCov1xParseCutoffTime(t *testing.T) {
	t.Parallel()
	if got := parseCutoffTime(""); time.Since(got) > time.Minute || time.Since(got) < 0 {
		t.Fatalf("empty cutoff = %v, want ~now", got)
	}
	want := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	if got := parseCutoffTime("2026-05-06T09:08:09+02:00"); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("offset cutoff = %v, want %v in UTC", got, want)
	}
	if got := parseCutoffTime("not-a-time"); time.Since(got) > time.Minute || time.Since(got) < 0 {
		t.Fatalf("invalid cutoff = %v, want fallback to ~now", got)
	}
}

func TestCov1xPurgeExpiredFilesRejectsBadPayload(t *testing.T) {
	t.Parallel()
	err := PurgeExpiredFilesHandler(cov1xDiscardLogger(), nil, nil)(context.Background(), cov1xRawTask(workers.TypePurgeExpiredFiles, `{`))
	if err == nil || !strings.Contains(err.Error(), "purge expired files: decode payload") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xPurgeExpiredFilesNilServicesIsNoop(t *testing.T) {
	t.Parallel()
	logger, buf := cov1xBufLogger()
	err := PurgeExpiredFilesHandler(logger, nil, nil)(context.Background(),
		cov1xTask(t, workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{OrgID: "o", CutoffTime: "garbage"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !strings.Contains(buf.String(), "purge expired files: completed") {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestCov1xPurgeExpiredFilesReportsSafetyNetDeleteFailures(t *testing.T) {
	t.Parallel()
	const org = "org-cov1x-purge"
	// The services delete once while expiring; the handler deletes again as a
	// safety net. Fail only that second delete.
	store := cov1xNewDelStore(func(key string, nth int) error {
		if nth >= 2 {
			return errors.New("delete refused")
		}
		return nil
	})
	exportsService := exports.NewServiceWithStorage(store)
	reportsService := reports.NewService(nil, nil, nil, nil)
	reportsService.SetStorage(store)
	p := cov1xAdminFor(org)

	rec, _, err := exportsService.GenerateExport(p, org, "meet-1", exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet:       exports.ReportMeet{Name: "M", StartDate: "2026-03-15", EndDate: "2026-03-16", Course: "SCY"},
	})
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}
	art, err := reportsService.StoreCSVArtifact(org, "invoice_aging", time.Now().UTC(), &reports.CSVDocument{Filename: "a.csv", Data: []byte("a\n1\n")})
	if err != nil {
		t.Fatalf("StoreCSVArtifact: %v", err)
	}

	logger, buf := cov1xBufLogger()
	cutoff := time.Now().UTC().Add(365 * 24 * time.Hour)
	err = PurgeExpiredFilesHandler(logger, exportsService, reportsService)(context.Background(),
		cov1xTask(t, workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{OrgID: org, CutoffTime: cutoff.Format(time.RFC3339)}))
	if err == nil {
		t.Fatal("expected an error when the safety-net delete fails")
	}
	for _, key := range []string{rec.StorageKey, art.StorageKey} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not list failed key %q", err, key)
		}
	}
	if !strings.Contains(err.Error(), "2 artifact delete(s) failed") {
		t.Errorf("err = %v", err)
	}
	logs := buf.String()
	if !strings.Contains(logs, "delete export artifact failed") || !strings.Contains(logs, "delete report artifact failed") {
		t.Errorf("expected warn logs for both kinds, got %q", logs)
	}
	// The failed keys are sorted so the message is deterministic.
	first, second := rec.StorageKey, art.StorageKey
	if first > second {
		first, second = second, first
	}
	if strings.Index(err.Error(), first) > strings.Index(err.Error(), second) {
		t.Errorf("failed keys not sorted in %q", err)
	}
}

func TestCov1xPurgeExpiredFilesSurfacesExportSyncFailure(t *testing.T) {
	t.Parallel()
	svc := exports.NewServiceWithPool(testutil.ClosedPool(t), storage.NewStubStorage(""))
	err := PurgeExpiredFilesHandler(cov1xDiscardLogger(), svc, nil)(context.Background(),
		cov1xTask(t, workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{OrgID: uuid.NewString()}))
	if err == nil || !strings.Contains(err.Error(), "purge expired files:") {
		t.Fatalf("err = %v", err)
	}
}

// ---------------------------------------------------------------- data export

func TestCov1xProcessDataExportRejectsBadInput(t *testing.T) {
	t.Parallel()
	svc := compliance.NewService()
	store := storage.NewStubStorage("")
	cases := []struct {
		name    string
		svc     *compliance.Service
		store   storage.ObjectStorage
		payload string
		want    string
	}{
		{"bad json", svc, store, `{`, "process data export: decode payload"},
		{"missing fields", svc, store, `{"request_id":"r","org_id":"o"}`, "missing required fields"},
		{"nil compliance", nil, store, `{"request_id":"r","org_id":"o","household_id":"h"}`, "compliance service is not configured"},
		{"nil storage", svc, nil, `{"request_id":"r","org_id":"o","household_id":"h"}`, "object storage is not configured"},
	}
	for _, tc := range cases {
		err := ProcessDataExportHandler(cov1xDiscardLogger(), tc.svc, tc.store)(context.Background(), cov1xRawTask(workers.TypeProcessDataExport, tc.payload))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCov1xProcessDataExportRequestErrors(t *testing.T) {
	t.Parallel()
	const org = "org-cov1x-exp"
	svc := compliance.NewService()
	store := storage.NewStubStorage("")
	h := ProcessDataExportHandler(cov1xDiscardLogger(), svc, store)

	err := h(context.Background(), cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: "missing", OrgID: org, HouseholdID: "hh"}))
	if !errors.Is(err, compliance.ErrExportNotFound) || !strings.Contains(err.Error(), "load request missing") {
		t.Fatalf("missing request err = %v", err)
	}

	req, err := svc.CreateDataExportRequest(org, "hh-real", "admin")
	if err != nil {
		t.Fatal(err)
	}
	err = h(context.Background(), cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: "hh-other"}))
	if err == nil || !strings.Contains(err.Error(), "household mismatch") {
		t.Fatalf("mismatch err = %v", err)
	}
	if got, _ := svc.GetDataExportRequest(org, req.ID); got.Status != "pending" {
		t.Fatalf("mismatched job must not touch the request, status = %q", got.Status)
	}

	// A completed export cannot be re-processed.
	task := cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: "hh-real"})
	if err := h(context.Background(), task); err != nil {
		t.Fatalf("first run: %v", err)
	}
	err = h(context.Background(), task)
	if !errors.Is(err, compliance.ErrInvalidTransition) || !strings.Contains(err.Error(), "mark processing") {
		t.Fatalf("re-run err = %v", err)
	}
}

func TestCov1xProcessDataExportIncludesCollectedHouseholdData(t *testing.T) {
	t.Parallel()
	const org, hh = "org-cov1x-exp2", "hh-collect"
	svc := compliance.NewService()
	svc.SetHouseholdDataCollector(func(o, h string) (map[string]any, error) {
		return map[string]any{"swimmers": []string{"Ann"}, "org": o, "hh": h}, nil
	})
	store := storage.NewStubStorage("")
	req, _ := svc.CreateDataExportRequest(org, hh, "admin")
	err := ProcessDataExportHandler(cov1xDiscardLogger(), svc, store)(context.Background(),
		cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: hh}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	data, _, err := store.GetObject(context.Background(), complianceExportStorageKey(org, req.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"swimmers"`) || !strings.Contains(string(data), `"Ann"`) || !strings.Contains(string(data), `"hh": "hh-collect"`) {
		t.Fatalf("artifact missing collected data: %s", data)
	}
}

func TestCov1xProcessDataExportFailurePathsMarkRequestFailed(t *testing.T) {
	t.Parallel()
	const org, hh = "org-cov1x-exp3", "hh-fail"

	t.Run("collector error", func(t *testing.T) {
		t.Parallel()
		svc := compliance.NewService()
		boom := errors.New("collector down")
		svc.SetHouseholdDataCollector(func(string, string) (map[string]any, error) { return nil, boom })
		req, _ := svc.CreateDataExportRequest(org, hh, "admin")
		err := ProcessDataExportHandler(cov1xDiscardLogger(), svc, storage.NewStubStorage(""))(context.Background(),
			cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: hh}))
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), "collect household data") {
			t.Fatalf("err = %v", err)
		}
		if got, _ := svc.GetDataExportRequest(org, req.ID); got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}
	})

	t.Run("unmarshalable artifact", func(t *testing.T) {
		t.Parallel()
		svc := compliance.NewService()
		svc.SetHouseholdDataCollector(func(string, string) (map[string]any, error) {
			return map[string]any{"bad": make(chan int)}, nil
		})
		req, _ := svc.CreateDataExportRequest(org, hh, "admin")
		err := ProcessDataExportHandler(cov1xDiscardLogger(), svc, storage.NewStubStorage(""))(context.Background(),
			cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: hh}))
		if err == nil || !strings.Contains(err.Error(), "marshal artifact") {
			t.Fatalf("err = %v", err)
		}
		if got, _ := svc.GetDataExportRequest(org, req.ID); got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}
	})

	t.Run("storage error", func(t *testing.T) {
		t.Parallel()
		svc := compliance.NewService()
		store := &cov1xHookPutStore{StubStorage: storage.NewStubStorage(""), hook: func() error { return errors.New("bucket gone") }}
		req, _ := svc.CreateDataExportRequest(org, hh, "admin")
		err := ProcessDataExportHandler(cov1xDiscardLogger(), svc, store)(context.Background(),
			cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: hh}))
		if err == nil || !strings.Contains(err.Error(), "store artifact") || !strings.Contains(err.Error(), "bucket gone") {
			t.Fatalf("err = %v", err)
		}
		if got, _ := svc.GetDataExportRequest(org, req.ID); got.Status != "failed" || got.DownloadURL != "" {
			t.Fatalf("request = %+v, want failed without a download key", got)
		}
	})

	t.Run("complete transition rejected", func(t *testing.T) {
		t.Parallel()
		svc := compliance.NewService()
		req, _ := svc.CreateDataExportRequest(org, hh, "admin")
		// Another actor fails the request while the artifact is being stored.
		store := &cov1xHookPutStore{StubStorage: storage.NewStubStorage(""), hook: func() error {
			_, err := svc.FailDataExport(org, req.ID)
			return err
		}}
		err := ProcessDataExportHandler(cov1xDiscardLogger(), svc, store)(context.Background(),
			cov1xTask(t, workers.TypeProcessDataExport, ProcessDataExportPayload{RequestID: req.ID, OrgID: org, HouseholdID: hh}))
		if !errors.Is(err, compliance.ErrInvalidTransition) || !strings.Contains(err.Error(), "complete request") {
			t.Fatalf("err = %v", err)
		}
	})
}

// ---------------------------------------------------------------- data deletion

type cov1xDelEnv struct {
	org       string
	comp      *compliance.Service
	mem       *members.Service
	hhID      string
	reqID     string
	principal *appmiddleware.Principal
}

func cov1xNewDelEnv(t *testing.T, org, email string) *cov1xDelEnv {
	t.Helper()
	return cov1xNewDelEnvWith(t, org, email, members.NewService())
}

func cov1xNewDelEnvWith(t *testing.T, org, email string, mem *members.Service) *cov1xDelEnv {
	t.Helper()
	e := &cov1xDelEnv{org: org, comp: compliance.NewService(), mem: mem, principal: cov1xAdminFor(org)}
	hh, err := e.mem.CreateHousehold(e.principal, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Cov1x Household", PrimaryEmail: email})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	e.hhID = hh.ID
	req, err := e.comp.CreateDataDeletionRequest(org, hh.ID, "admin", "requested")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest: %v", err)
	}
	e.reqID = req.ID
	return e
}

func (e *cov1xDelEnv) task(t *testing.T) *workers.Task {
	return cov1xTask(t, workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{RequestID: e.reqID, OrgID: e.org, HouseholdID: e.hhID})
}

func (e *cov1xDelEnv) status(t *testing.T) *compliance.DataDeletionRequest {
	t.Helper()
	got, err := e.comp.GetDataDeletionRequest(e.org, e.reqID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (e *cov1xDelEnv) requireFailedAt(t *testing.T, err error, step string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "process data deletion: "+step) {
		t.Fatalf("err = %v, want failure at %q", err, step)
	}
	got := e.status(t)
	if got.Status != "failed" || !strings.Contains(got.ReviewNotes, "Erasure failed at "+step) {
		t.Fatalf("request = %q / %q, want failed at %q", got.Status, got.ReviewNotes, step)
	}
	hh, herr := e.mem.GetHousehold(e.principal, e.org, e.hhID)
	if herr == nil && hh.HouseholdName == "Deleted Household" {
		t.Fatal("household anonymized despite a failed erasure step")
	}
}

func TestCov1xProcessDataDeletionRejectsBadInput(t *testing.T) {
	t.Parallel()
	comp, mem := compliance.NewService(), members.NewService()
	ok := `{"request_id":"r","org_id":"o","household_id":"h"}`
	cases := []struct {
		name    string
		comp    *compliance.Service
		mem     *members.Service
		payload string
		want    string
	}{
		{"bad json", comp, mem, `{`, "process data deletion: decode payload"},
		{"missing fields", comp, mem, `{"request_id":"r"}`, "missing required fields"},
		{"nil compliance", nil, mem, ok, "compliance service is not configured"},
		{"nil members", comp, nil, ok, "members service is not configured"},
	}
	for _, tc := range cases {
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), tc.comp, tc.mem, nil, nil, nil, nil)(context.Background(), cov1xRawTask(workers.TypeProcessDataDeletion, tc.payload))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ProcessDataDeletionHandler(cov1xDiscardLogger(), comp, mem, nil, nil, nil, nil)(ctx, cov1xRawTask(workers.TypeProcessDataDeletion, ok))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx err = %v", err)
	}
}

func TestCov1xProcessDataDeletionRequestErrors(t *testing.T) {
	t.Parallel()
	e := cov1xNewDelEnv(t, "org-cov1x-del1", "a@example.test")
	h := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, nil)

	err := h(context.Background(), cov1xTask(t, workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{RequestID: "nope", OrgID: e.org, HouseholdID: e.hhID}))
	if !errors.Is(err, compliance.ErrDeletionNotFound) || !strings.Contains(err.Error(), "load request nope") {
		t.Fatalf("missing request err = %v", err)
	}
	err = h(context.Background(), cov1xTask(t, workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{RequestID: e.reqID, OrgID: e.org, HouseholdID: "other-hh"}))
	if err == nil || !strings.Contains(err.Error(), "household mismatch") {
		t.Fatalf("mismatch err = %v", err)
	}
	if got := e.status(t); got.Status != "pending" {
		t.Fatalf("mismatched job touched the request: %q", got.Status)
	}

	if err := h(context.Background(), e.task(t)); err != nil {
		t.Fatalf("first run: %v", err)
	}
	err = h(context.Background(), e.task(t))
	if !errors.Is(err, compliance.ErrInvalidTransition) || !strings.Contains(err.Error(), "mark processing") {
		t.Fatalf("re-run err = %v", err)
	}
}

func TestCov1xProcessDataDeletionFailsWhenHouseholdUnknown(t *testing.T) {
	t.Parallel()
	const org = "org-cov1x-del2"
	comp := compliance.NewService()
	req, _ := comp.CreateDataDeletionRequest(org, "ghost-hh", "admin", "r")
	err := ProcessDataDeletionHandler(cov1xDiscardLogger(), comp, members.NewService(), nil, nil, nil, nil)(context.Background(),
		cov1xTask(t, workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{RequestID: req.ID, OrgID: org, HouseholdID: "ghost-hh"}))
	if !errors.Is(err, members.ErrHouseholdNotFound) || !strings.Contains(err.Error(), "capture erasure context") {
		t.Fatalf("err = %v", err)
	}
	got, _ := comp.GetDataDeletionRequest(org, req.ID)
	if got.Status != "failed" || !strings.Contains(got.ReviewNotes, "Erasure failed at capture erasure context") {
		t.Fatalf("request = %+v", got)
	}
}

func TestCov1xProcessDataDeletionPurgesPriorExportArtifacts(t *testing.T) {
	t.Parallel()
	e := cov1xNewDelEnv(t, "org-cov1x-del3", "a@example.test")
	const key = "compliance/org-cov1x-del3/exports/old.json"
	store := cov1xNewDelStore(nil)
	if err := store.PutObject(context.Background(), key, "application/json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	done, _ := e.comp.CreateDataExportRequest(e.org, e.hhID, "admin")
	_, _ = e.comp.MarkDataExportProcessing(e.org, done.ID)
	if _, err := e.comp.CompleteDataExport(e.org, done.ID, key, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	urlReq, _ := e.comp.CreateDataExportRequest(e.org, e.hhID, "admin")
	_, _ = e.comp.MarkDataExportProcessing(e.org, urlReq.ID)
	const presigned = "https://storage.example.test/signed?x=1"
	if _, err := e.comp.CompleteDataExport(e.org, urlReq.ID, presigned, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, store, nil)(context.Background(), e.task(t)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if exists, _ := store.Exists(context.Background(), key); exists {
		t.Fatal("prior export artifact must be purged")
	}
	if store.deleteCount(key) != 1 || store.deleteCount(presigned) != 0 {
		t.Fatalf("deletes: key=%d presigned=%d, want 1 and 0 (URLs are not object keys)", store.deleteCount(key), store.deleteCount(presigned))
	}
	if got := e.status(t); got.Status != "completed" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestCov1xProcessDataDeletionWithoutObjectStoreSkipsPurge(t *testing.T) {
	t.Parallel()
	e := cov1xNewDelEnv(t, "org-cov1x-del4", "a@example.test")
	done, _ := e.comp.CreateDataExportRequest(e.org, e.hhID, "admin")
	_, _ = e.comp.MarkDataExportProcessing(e.org, done.ID)
	_, _ = e.comp.CompleteDataExport(e.org, done.ID, "compliance/x.json", time.Now().Add(time.Hour))
	if err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, nil)(context.Background(), e.task(t)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := e.status(t); got.Status != "completed" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestCov1xProcessDataDeletionPurgeFailureKeepsRequestRetryable(t *testing.T) {
	t.Parallel()
	e := cov1xNewDelEnv(t, "org-cov1x-del5", "a@example.test")
	const key = "compliance/org-cov1x-del5/exports/old.json"
	store := cov1xNewDelStore(func(string, int) error { return errors.New("s3 down") })
	done, _ := e.comp.CreateDataExportRequest(e.org, e.hhID, "admin")
	_, _ = e.comp.MarkDataExportProcessing(e.org, done.ID)
	_, _ = e.comp.CompleteDataExport(e.org, done.ID, key, time.Now().Add(time.Hour))
	err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, store, nil)(context.Background(), e.task(t))
	e.requireFailedAt(t, err, "purge object storage")
	if !strings.Contains(err.Error(), "purge object storage key "+key) {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xProcessDataDeletionPostEraseObjectPurgeFailures(t *testing.T) {
	t.Parallel()
	// The first delete of a key (from the pre-erase listing) succeeds; the
	// delete of the key returned by the erase step fails.
	t.Run("comms attachments", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del6", "a@example.test")
		const key = "messages/org-cov1x-del6/a.png"
		store := cov1xNewDelStore(func(k string, nth int) error {
			if k == key && nth >= 2 {
				return errors.New("second delete refused")
			}
			return nil
		})
		_ = store.PutObject(context.Background(), key, "image/png", []byte("x"))
		cs := comms.NewService()
		cs.SetStorage(store)
		if _, err := cs.SendMessage(comms.CreateMessageInput{
			OrgID: e.org, HouseholdID: e.hhID, Body: "hi", SenderUserID: "u1", SenderRole: "parent",
			Attachments: []comms.AttachmentInput{{ObjectKey: key, Filename: "a.png", MimeType: "image/png", SizeBytes: 1}},
		}); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, cs, nil, store, nil)(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "purge object storage")
		if store.deleteCount(key) != 2 {
			t.Fatalf("delete count = %d, want 2", store.deleteCount(key))
		}
	})

	t.Run("document files", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del7", "a@example.test")
		const key = "documents/org-cov1x-del7/w.pdf"
		store := cov1xNewDelStore(func(k string, nth int) error {
			if k == key && nth >= 2 {
				return errors.New("second delete refused")
			}
			return nil
		})
		_ = store.PutObject(context.Background(), key, "application/pdf", []byte("x"))
		ds := documents.NewService()
		dt, err := ds.CreateDocumentType(e.org, "Waiver", "waiver", "", "household", true, true, nil, "admin")
		if err != nil {
			t.Fatal(err)
		}
		sub, err := ds.CreateSubmission(e.org, dt.ID, e.hhID, "", "admin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ds.AttachFile(e.org, sub.ID, key, "w.pdf", "application/pdf", 1, "admin"); err != nil {
			t.Fatal(err)
		}
		err = ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, ds, store, nil)(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "purge object storage")
		if store.deleteCount(key) != 2 {
			t.Fatalf("delete count = %d, want 2", store.deleteCount(key))
		}
	})
}

func TestCov1xProcessDataDeletionDatabaseBackedStepFailures(t *testing.T) {

	t.Run("export artifact listing fails", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		org := uuid.NewString()
		comp := compliance.NewServiceWithPool(pool, 7, 30)
		mem := members.NewService()
		hh, err := mem.CreateHousehold(cov1xAdminFor(org), members.CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
		if err != nil {
			t.Fatal(err)
		}
		req, err := comp.CreateDataDeletionRequest(org, hh.ID, "admin", "r")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), `DROP TABLE data_export_requests CASCADE; CREATE VIEW data_export_requests AS SELECT 1 AS cov1x_only_column`); err != nil {
			t.Fatal(err)
		}
		err = ProcessDataDeletionHandler(cov1xDiscardLogger(), comp, mem, nil, nil, nil, nil)(context.Background(),
			cov1xTask(t, workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{RequestID: req.ID, OrgID: org, HouseholdID: hh.ID}))
		if err == nil || !strings.Contains(err.Error(), "list data export artifacts") {
			t.Fatalf("err = %v", err)
		}
		got, _ := comp.GetDataDeletionRequest(org, req.ID)
		if got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}
	})

	t.Run("comms key listing fails", func(t *testing.T) {
		e := cov1xNewDelEnv(t, uuid.NewString(), "a@example.test")
		cs := comms.NewServiceWithPool(testutil.ClosedPool(t))
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, cs, nil, nil, nil)(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "list comms object keys")
	})

	t.Run("document key listing fails", func(t *testing.T) {
		e := cov1xNewDelEnv(t, uuid.NewString(), "a@example.test")
		ds := documents.NewServiceWithPool(testutil.ClosedPool(t))
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, ds, nil, nil)(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "list document object keys")
	})

	t.Run("comms erase fails", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		e := cov1xNewDelEnv(t, uuid.NewString(), "parent@example.test")
		ctx := context.Background()
		if _, err := pool.Exec(ctx, `INSERT INTO email_deliveries (org_id, recipient_email) VALUES ($1::uuid, 'parent@example.test')`, e.org); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `CREATE FUNCTION cov1x_reject_delivery_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$;
			CREATE TRIGGER cov1x_reject_delivery_delete BEFORE DELETE ON email_deliveries FOR EACH ROW EXECUTE FUNCTION cov1x_reject_delivery_delete()`); err != nil {
			t.Fatal(err)
		}
		cs := comms.NewServiceWithPool(pool)
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, cs, nil, nil, nil)(ctx, e.task(t))
		e.requireFailedAt(t, err, "erase comms records")
	})

	t.Run("document erase fails", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		e := cov1xNewDelEnvWith(t, uuid.NewString(), "a@example.test", members.NewServiceWithPool(pool))
		ds := documents.NewServiceWithPool(pool)
		dt, err := ds.CreateDocumentType(e.org, "Waiver", "waiver", "", "household", true, true, nil, "admin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ds.CreateSubmission(e.org, dt.ID, e.hhID, "", "admin"); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), `CREATE FUNCTION cov1x_reject_sub_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$;
			CREATE TRIGGER cov1x_reject_sub_delete BEFORE DELETE ON document_submissions FOR EACH ROW EXECUTE FUNCTION cov1x_reject_sub_delete()`); err != nil {
			t.Fatal(err)
		}
		err = ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, ds, nil, nil)(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "erase document submissions")
	})
}

func TestCov1xProcessDataDeletionExtraErasers(t *testing.T) {
	t.Parallel()

	t.Run("runs each eraser and skips nil", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del8", "a@example.test")
		var got [][2]string
		eraser := func(_ context.Context, org, hh string) error {
			got = append(got, [2]string{org, hh})
			return nil
		}
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, nil, nil, eraser)(context.Background(), e.task(t))
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		if len(got) != 1 || got[0] != [2]string{e.org, e.hhID} {
			t.Fatalf("eraser calls = %v", got)
		}
	})

	t.Run("eraser failure", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del9", "a@example.test")
		boom := errors.New("rsvp erase failed")
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, nil,
			func(context.Context, string, string) error { return boom })(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "erase household records")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want wrapped cause", err)
		}
	})

	t.Run("anonymize fails when household vanished", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del10", "a@example.test")
		vanish := func(context.Context, string, string) error {
			_, err := e.mem.DeleteHousehold(e.principal, e.org, e.hhID)
			return err
		}
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, nil, vanish)(context.Background(), e.task(t))
		if !errors.Is(err, members.ErrHouseholdNotFound) || !strings.Contains(err.Error(), "anonymize household "+e.hhID) {
			t.Fatalf("err = %v", err)
		}
		if got := e.status(t); got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}
	})

	t.Run("complete rejected when request already failed", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del11", "a@example.test")
		failIt := func(context.Context, string, string) error {
			_, err := e.comp.FailDataDeletion(e.org, e.reqID, "operator cancelled")
			return err
		}
		err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, nil, failIt)(context.Background(), e.task(t))
		if !errors.Is(err, compliance.ErrInvalidTransition) || !strings.Contains(err.Error(), "complete request "+e.reqID) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCov1xProcessDataDeletionKeycloakHandling(t *testing.T) {
	t.Parallel()

	t.Run("delete failure aborts before anonymizing", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del12", "a@example.test")
		kc := keycloak.NewStubClient()
		uid, err := kc.CreateUser(context.Background(), "a@example.test", "A", "B", e.org)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.mem.LinkHouseholdToUser(e.org, e.hhID, uid); err != nil {
			t.Fatal(err)
		}
		kc.DeleteUserErr = errors.New("keycloak 503")
		err = ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, kc)(context.Background(), e.task(t))
		e.requireFailedAt(t, err, "delete keycloak user "+uid)
	})

	t.Run("already deleted user is tolerated", func(t *testing.T) {
		t.Parallel()
		e := cov1xNewDelEnv(t, "org-cov1x-del13", "a@example.test")
		kc := keycloak.NewStubClient()
		if err := e.mem.LinkHouseholdToUser(e.org, e.hhID, "ghost-kc-user"); err != nil {
			t.Fatal(err)
		}
		if err := ProcessDataDeletionHandler(cov1xDiscardLogger(), e.comp, e.mem, nil, nil, nil, kc)(context.Background(), e.task(t)); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if got := e.status(t); got.Status != "completed" {
			t.Fatalf("status = %q", got.Status)
		}
	})
}
