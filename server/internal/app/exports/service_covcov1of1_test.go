package exports

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/storage"
)

var (
	cov1xAdmin  = &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	cov1xCoach  = &appmiddleware.Principal{UserID: "coach-1", Roles: []string{"coach"}}
	cov1xParent = &appmiddleware.Principal{UserID: "parent-1", Roles: []string{"parent"}}
)

// cov1xStore wraps a real stub store and fails selected operations.
type cov1xStore struct {
	storage.ObjectStorage
	putErr error
	getErr error
}

func (c *cov1xStore) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if c.putErr != nil {
		return c.putErr
	}
	return c.ObjectStorage.PutObject(ctx, key, ct, data)
}

func (c *cov1xStore) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	if c.getErr != nil {
		return nil, "", c.getErr
	}
	return c.ObjectStorage.GetObject(ctx, key)
}

func cov1xGenerate(t *testing.T, svc *Service, org string) *FileExport {
	t.Helper()
	job, audit, err := svc.GenerateExport(cov1xAdmin, org, "m1", sampleReportInput(ReportTypeEvent, ""))
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}
	if audit.Action != "report.generated" || audit.ResourceID != job.ID || audit.Meta["meet_id"] != "m1" {
		t.Fatalf("unexpected audit event: %+v", audit)
	}
	return job
}

func TestCov1xGenerateExportForbiddenAndInvalid(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, _, err := svc.GenerateExport(cov1xParent, "org-1", "m1", sampleReportInput(ReportTypeEvent, "")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent err = %v, want ErrForbidden", err)
	}
	if _, _, err := svc.GenerateExport(cov1xAdmin, "org-1", "m1", sampleReportInput("bogus", "")); !errors.Is(err, ErrInvalidExportType) {
		t.Fatalf("bogus type err = %v", err)
	}
	if got := svc.ListExportsForAuthorizedMeet("org-1", ""); len(got) != 0 {
		t.Fatalf("failed requests must not create jobs, got %d", len(got))
	}
}

func TestCov1xGenerateExportReadyAndDownload(t *testing.T) {
	t.Parallel()
	svc := NewService()
	job := cov1xGenerate(t, svc, "org-1")
	if job.Status != "ready" || job.SizeBytes == 0 || job.DownloadURL == "" || job.CreatedBy != "admin-1" {
		t.Fatalf("job not ready: %+v", job)
	}
	got, content, err := svc.DownloadExport(cov1xCoach, "org-1", job.ID)
	if err != nil {
		t.Fatalf("DownloadExport: %v", err)
	}
	assertPDF(t, content, got.Filename, "spring-invitational_event_report.pdf")
	if len(content) != job.SizeBytes {
		t.Fatalf("size %d != stored %d", job.SizeBytes, len(content))
	}
	if _, _, err := svc.DownloadExport(cov1xParent, "org-1", job.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent download err = %v", err)
	}
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-2", job.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("cross-org download err = %v", err)
	}
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-1", "missing"); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("missing download err = %v", err)
	}
}

func TestCov1xDownloadStatesAndStoreErrors(t *testing.T) {
	t.Parallel()
	base := storage.NewStubStorage("")
	store := &cov1xStore{ObjectStorage: base}
	svc := NewServiceWithStorage(store)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	svc.nowFunc = func() time.Time { return now }
	job := cov1xGenerate(t, svc, "org-1")

	// Not ready.
	svc.mu.Lock()
	svc.exports[job.ID].Status = "generating"
	svc.mu.Unlock()
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-1", job.ID); !errors.Is(err, ErrExportNotReady) {
		t.Fatalf("generating err = %v, want ErrExportNotReady", err)
	}
	// Ready but past its deadline.
	svc.mu.Lock()
	svc.exports[job.ID].Status = "ready"
	svc.mu.Unlock()
	now = now.Add(8 * 24 * time.Hour)
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-1", job.ID); !errors.Is(err, ErrExportExpired) {
		t.Fatalf("past-deadline err = %v, want ErrExportExpired", err)
	}
	now = now.Add(-8 * 24 * time.Hour)

	// Object missing -> not found; other store errors are wrapped.
	store.getErr = storage.ErrObjectNotFound
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-1", job.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("missing object err = %v", err)
	}
	boom := errors.New("s3 down")
	store.getErr = boom
	_, _, err := svc.DownloadExport(cov1xAdmin, "org-1", job.ID)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "download export artifact") {
		t.Fatalf("store err = %v", err)
	}
}

func TestCov1xProcessExportErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &cov1xStore{ObjectStorage: storage.NewStubStorage("")}
	svc := NewServiceWithStorage(store)

	if err := svc.ProcessExport(ctx, "nope"); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
	svc.exports["exp"] = &FileExport{ID: "exp", OrgID: "o", Status: "expired"}
	if err := svc.ProcessExport(ctx, "exp"); !errors.Is(err, ErrExportExpired) {
		t.Fatalf("expired err = %v", err)
	}
	svc.exports["nopending"] = &FileExport{ID: "nopending", OrgID: "o", Status: "pending"}
	if err := svc.ProcessExport(ctx, "nopending"); err == nil || !strings.Contains(err.Error(), "no pending input") {
		t.Fatalf("no pending err = %v", err)
	}

	// Content generation failure marks the job failed.
	bad := sampleReportInput(ReportTypeEvent, "")
	bad.Meet.Name = " "
	svc.exports["badin"] = &FileExport{ID: "badin", OrgID: "o", Status: "pending", pendingInput: &bad}
	if err := svc.ProcessExport(ctx, "badin"); err == nil || !strings.Contains(err.Error(), "generate report content") {
		t.Fatalf("bad input err = %v", err)
	}
	if ex := svc.exports["badin"]; ex.Status != "failed" || !strings.Contains(ex.LastError, "meet name is required") {
		t.Fatalf("badin = %+v", ex)
	}

	// Storage failure marks the job failed with the cause.
	good := sampleReportInput(ReportTypeEvent, "")
	svc.exports["nostore"] = &FileExport{ID: "nostore", OrgID: "o", Status: "pending", StorageKey: "k", pendingInput: &good}
	store.putErr = errors.New("bucket full")
	if err := svc.ProcessExport(ctx, "nostore"); err == nil || !strings.Contains(err.Error(), "store export artifact") {
		t.Fatalf("store err = %v", err)
	}
	if ex := svc.exports["nostore"]; ex.Status != "failed" || ex.LastError != "bucket full" {
		t.Fatalf("nostore = %+v", ex)
	}
	// ProcessExportFor (memory mode) delegates to ProcessExport.
	store.putErr = nil
	svc.exports["nostore"].Status = "pending"
	svc.exports["nostore"].pendingInput = &good
	if err := svc.ProcessExportFor(ctx, "o", "nostore"); err != nil {
		t.Fatalf("ProcessExportFor: %v", err)
	}
	if ex := svc.exports["nostore"]; ex.Status != "ready" || ex.LastError != "" || ex.pendingInput != nil {
		t.Fatalf("after retry = %+v", ex)
	}
	if err := svc.ProcessExportFor(ctx, "o", "ghost"); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("ghost err = %v", err)
	}
}

func TestCov1xFailExportUnknownIsNoop(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.FailExport("ghost", errors.New("x"))
	svc.exports["a"] = &FileExport{ID: "a", Status: "generating"}
	svc.FailExport("a", errors.New("  trailing space \n"))
	if ex := svc.exports["a"]; ex.Status != "failed" || ex.LastError != "trailing space" {
		t.Fatalf("a = %+v", ex)
	}
}

func TestCov1xConfigureArtifactTTL(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.ConfigureArtifactTTL(0)
	svc.ConfigureArtifactTTL(-time.Hour)
	if svc.artifactTTL != 7*24*time.Hour {
		t.Fatalf("non-positive TTL must be ignored, got %v", svc.artifactTTL)
	}
	svc.ConfigureArtifactTTL(time.Hour)
	job := cov1xGenerate(t, svc, "org-1")
	if d := job.ExpiresAt.Sub(job.CreatedAt); d != time.Hour {
		t.Fatalf("expiry window = %v, want 1h", d)
	}
}

func TestCov1xListAndGet(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.SetQueue(nil)
	svc.SetOrgCacheLimits(time.Hour, 5)
	job := cov1xGenerate(t, svc, "org-1")
	if _, err := svc.ListExports(cov1xParent, "org-1", "m1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent list err = %v", err)
	}
	list, err := svc.ListExports(cov1xCoach, "org-1", "m1")
	if err != nil || len(list) != 1 || list[0].ID != job.ID {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if got, _ := svc.ListExports(cov1xCoach, "org-1", "other-meet"); len(got) != 0 {
		t.Fatalf("meet filter leaked %d rows", len(got))
	}
	if got, _ := svc.ListExports(cov1xCoach, "org-2", ""); len(got) != 0 {
		t.Fatalf("org filter leaked %d rows", len(got))
	}
	if _, err := svc.GetExport(cov1xParent, "org-1", job.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent get err = %v", err)
	}
	if _, err := svc.GetExport(cov1xAdmin, "org-2", job.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	if _, err := svc.GetExport(cov1xAdmin, "org-1", "ghost"); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("ghost get err = %v", err)
	}
	got, err := svc.GetExport(cov1xAdmin, "org-1", job.ID)
	if err != nil || got.ID != job.ID || got.pendingInput != nil {
		t.Fatalf("get = %+v err=%v", got, err)
	}
}

func TestCov1xListExportsForViewerHidesExpired(t *testing.T) {
	t.Parallel()
	svc := NewService()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	svc.nowFunc = func() time.Time { return now }
	svc.exports["fresh"] = &FileExport{ID: "fresh", OrgID: "o", MeetID: "m", ExportType: "entry_report", Status: "ready", StorageKey: "k", ExpiresAt: now.Add(time.Hour)}
	svc.exports["stale"] = &FileExport{ID: "stale", OrgID: "o", MeetID: "m", ExportType: "entry_report", Status: "ready", StorageKey: "k", ExpiresAt: now.Add(-time.Hour)}
	got := svc.ListExportsForViewer(cov1xParent, "o", "m")
	if len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("viewer list = %+v", got)
	}
	// A stale record is never given a download URL either.
	for _, ex := range svc.ListExportsForAuthorizedMeet("o", "m") {
		if ex.ID == "stale" && ex.DownloadURL != "" {
			t.Fatal("expired export must not get a download URL")
		}
	}
}

func TestCov1xExpireArtifacts(t *testing.T) {
	t.Parallel()
	svc := NewService()
	a := cov1xGenerate(t, svc, "org-a")
	b := cov1xGenerate(t, svc, "org-b")
	future := time.Now().Add(30 * 24 * time.Hour)

	// Already-expired, keyless, nil and unexpiring exports are not candidates.
	svc.mu.Lock()
	svc.exports["done"] = &FileExport{ID: "done", OrgID: "org-a", Status: "expired", StorageKey: "k", ExpiresAt: time.Now().Add(-time.Hour)}
	svc.exports["nokey"] = &FileExport{ID: "nokey", OrgID: "org-a", Status: "ready", ExpiresAt: time.Now().Add(-time.Hour)}
	svc.exports["zero"] = &FileExport{ID: "zero", OrgID: "org-a", Status: "ready", StorageKey: "k"}
	svc.exports["nil"] = nil
	svc.mu.Unlock()

	if got := svc.ExpireArtifacts(time.Now()); len(got) != 0 {
		t.Fatalf("nothing is due yet, expired %d", len(got))
	}
	got, err := svc.ExpireArtifactsForOrg("org-a", future)
	if err != nil || len(got) != 1 || got[0].ID != a.ID || got[0].Status != "expired" || got[0].DownloadURL != "" {
		t.Fatalf("org-a expiry = %+v err=%v", got, err)
	}
	if !got[0].UpdatedAt.Equal(future.UTC()) {
		t.Fatalf("UpdatedAt = %v, want cutoff %v", got[0].UpdatedAt, future.UTC())
	}
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-b", b.ID); err != nil {
		t.Fatalf("org-b must be untouched: %v", err)
	}
	if _, _, err := svc.DownloadExport(cov1xAdmin, "org-a", a.ID); !errors.Is(err, ErrExportExpired) {
		t.Fatalf("org-a download err = %v", err)
	}
	if _, err := svc.ExpireArtifactsForOrg("org-b", future); err != nil {
		t.Fatal(err)
	}
	// Objects are really deleted from the store.
	if _, _, err := svc.store.GetObject(context.Background(), a.StorageKey); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("object should be deleted, err=%v", err)
	}
}

func TestCov1xExpireArtifactsSkipsRecordsChangedConcurrently(t *testing.T) {
	t.Parallel()
	svc := NewService()
	job := cov1xGenerate(t, svc, "org-1")
	hook := &cov1xDeleteHook{ObjectStorage: svc.store, onDelete: func() {
		svc.mu.Lock()
		delete(svc.exports, job.ID)
		svc.mu.Unlock()
	}}
	svc.store = hook
	if got := svc.ExpireArtifacts(time.Now().Add(30 * 24 * time.Hour)); len(got) != 0 {
		t.Fatalf("a record removed mid-sweep must be skipped, got %+v", got)
	}
	if !hook.called {
		t.Fatal("delete hook never ran")
	}
}

type cov1xDeleteHook struct {
	storage.ObjectStorage
	onDelete func()
	called   bool
}

func (h *cov1xDeleteHook) Delete(key string) error {
	h.called = true
	h.onDelete()
	return h.ObjectStorage.Delete(key)
}

func TestCov1xExpireArtifactsNilStore(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithStorage(nil)
	svc.exports["x"] = &FileExport{ID: "x", OrgID: "o", Status: "ready", StorageKey: "k", ExpiresAt: time.Now().Add(-time.Hour)}
	got := svc.ExpireArtifacts(time.Now())
	if len(got) != 1 || got[0].Status != "expired" {
		t.Fatalf("got %+v", got)
	}
}

func TestCov1xExpireArtifactsForOrgRequiresOrgInDBMode(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.repo = newRepository(nil)
	if _, err := svc.ExpireArtifactsForOrg("", time.Now()); err == nil || !strings.Contains(err.Error(), "org id required") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xDeleteArtifact(t *testing.T) {
	t.Parallel()
	svc := NewService()
	ctx := context.Background()
	if err := svc.DeleteArtifact("  "); err != nil {
		t.Fatalf("blank key must be a no-op: %v", err)
	}
	if err := svc.store.PutObject(ctx, "exports/x.pdf", "application/pdf", []byte("pdf")); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteArtifact("exports/x.pdf"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.store.Exists(ctx, "exports/x.pdf"); ok {
		t.Fatal("object still exists")
	}
	svc.store = &cov1xFailDelete{ObjectStorage: svc.store}
	if err := svc.DeleteArtifact("k"); err == nil {
		t.Fatal("expected store delete error to propagate")
	}
}

type cov1xFailDelete struct{ storage.ObjectStorage }

func (cov1xFailDelete) Delete(string) error { return errors.New("delete failed") }

func TestCov1xCloneForResponseAndNewID(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if svc.cloneForResponse(nil) != nil {
		t.Fatal("nil in, nil out")
	}
	in := sampleReportInput(ReportTypeEvent, "")
	cp := svc.cloneForResponse(&FileExport{ID: "a", Status: "pending", pendingInput: &in})
	if cp.pendingInput != nil || cp.DownloadURL != "" {
		t.Fatalf("clone = %+v", cp)
	}
	svc.store = &cov1xPresignFail{ObjectStorage: svc.store}
	cp = svc.cloneForResponse(&FileExport{ID: "a", Status: "ready", StorageKey: "k"})
	if cp.DownloadURL != "" {
		t.Fatalf("presign failure must leave URL empty, got %q", cp.DownloadURL)
	}
	id1, id2 := svc.newID("export_"), svc.newID("export_")
	if !strings.HasPrefix(id1, "export_") || len(id1) != len("export_")+24 || id1 == id2 {
		t.Fatalf("ids %q %q", id1, id2)
	}
}

type cov1xPresignFail struct{ storage.ObjectStorage }

func (cov1xPresignFail) PresignedGetURL(string, time.Duration) (string, error) {
	return "", errors.New("no presign")
}

func TestCov1xSanitizeFilename(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                       "export",
		"   ":                    "export",
		"Spring  Invitational!!": "spring-invitational",
		"---":                    "export",
		"A_b/c 9":                "a-b-c-9",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCov1xRuntimeSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	svc := NewService()
	in := sampleReportInput(ReportTypeEntry, GroupingGroup)
	svc.exports["pend"] = &FileExport{ID: "pend", OrgID: "o", Status: "pending", pendingInput: &in}
	svc.exports["done"] = &FileExport{ID: "done", OrgID: "o", Status: "ready", Filename: "f.pdf"}
	svc.exports["nil"] = nil
	data, err := svc.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var probe runtimeSnapshot
	if err := json.Unmarshal(data, &probe); err != nil || len(probe.Exports) != 2 {
		t.Fatalf("snapshot = %s err=%v", data, err)
	}

	other := NewService()
	other.exports["stale"] = &FileExport{ID: "stale"}
	if err := other.LoadRuntimeSnapshot(nil); err != nil || len(other.exports) != 1 {
		t.Fatalf("empty data must be a no-op, err=%v n=%d", err, len(other.exports))
	}
	if err := other.LoadRuntimeSnapshot([]byte("{not json")); err == nil {
		t.Fatal("expected JSON error")
	}
	if len(other.exports) != 1 {
		t.Fatal("failed load must not clobber state")
	}
	if err := other.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if _, ok := other.exports["stale"]; ok || len(other.exports) != 2 {
		t.Fatalf("load must replace state, got %d", len(other.exports))
	}
	p := other.exports["pend"]
	if p.pendingInput == nil || p.pendingInput.Grouping != GroupingGroup || len(p.pendingInput.Entries) != len(in.Entries) {
		t.Fatalf("pending input lost: %+v", p.pendingInput)
	}
	if other.exports["done"].pendingInput != nil || other.exports["done"].Filename != "f.pdf" {
		t.Fatalf("done = %+v", other.exports["done"])
	}
	// The restored job can be processed to completion.
	if err := other.ProcessExport(context.Background(), "pend"); err != nil {
		t.Fatalf("process restored job: %v", err)
	}
}
