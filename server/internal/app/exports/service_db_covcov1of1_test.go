package exports

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xHookedPool returns a second pool on the same isolated schema whose
// BeforeAcquire hook runs fn with the 1-based acquire count.
func cov1xHookedPool(t *testing.T, base *pgxpool.Pool, fn func(n int32)) *pgxpool.Pool {
	t.Helper()
	cfg := base.Config().Copy()
	var n atomic.Int32
	cfg.BeforeAcquire = func(context.Context, *pgx.Conn) bool {
		fn(n.Add(1))
		return true
	}
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestCov1xDBGenerateAndCrossInstanceVisibility(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	store := storage.NewStubStorage("")
	a := NewServiceWithPool(pool, store)
	b := NewServiceWithPool(pool, store)
	org := uuid.NewString()

	job, _, err := a.GenerateExport(cov1xAdmin, org, "", sampleReportInput(ReportTypeEntry, GroupingSession))
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}
	if _, err := uuid.Parse(job.ID); err != nil {
		t.Fatalf("DB mode must use uuid ids, got %q", job.ID)
	}

	// Instance B has never seen the club; reads load it from Postgres.
	got, err := b.GetExport(cov1xCoach, org, job.ID)
	if err != nil || got.Status != "ready" || got.Filename != job.Filename || got.SizeBytes != job.SizeBytes || got.MeetID != "" {
		t.Fatalf("B get = %+v err=%v", got, err)
	}
	dl, content, err := b.DownloadExport(cov1xAdmin, org, job.ID)
	if err != nil || dl.ID != job.ID {
		t.Fatalf("B download err=%v", err)
	}
	assertPDF(t, content, dl.Filename, "spring-invitational_entries_by_session.pdf")
	if list := b.ListExportsForAuthorizedMeet(org, ""); len(list) != 1 {
		t.Fatalf("B list = %d", len(list))
	}
	// Another club sees nothing.
	if list := b.ListExportsForAuthorizedMeet(uuid.NewString(), ""); len(list) != 0 {
		t.Fatalf("other org list = %d", len(list))
	}
	// Blank org is a no-op sync.
	if err := b.syncOrg(""); err != nil {
		t.Fatal(err)
	}

	// B expires the artifact; A observes the expired state after a refresh.
	future := time.Now().Add(30 * 24 * time.Hour)
	exp, err := b.ExpireArtifactsForOrg(org, future)
	if err != nil || len(exp) != 1 {
		t.Fatalf("expire = %+v err=%v", exp, err)
	}
	if _, _, err := a.DownloadExport(cov1xAdmin, org, job.ID); err == nil {
		t.Fatal("A should not be able to download an export B expired")
	}
	if _, err := a.ExpireArtifactsForOrg("", future); err == nil {
		t.Fatal("DB mode requires an org id")
	}
}

func TestCov1xDBProcessExportForLoadsAndKeepsPendingInput(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	store := storage.NewStubStorage("")
	svc := NewServiceWithPool(pool, store)
	org := uuid.NewString()
	ctx := context.Background()

	// A pending row persisted by another process: the worker has no input for it.
	other := NewServiceWithPool(pool, store)
	ex := &FileExport{ID: uuid.NewString(), OrgID: org, ExportType: ReportTypeEvent, Status: "pending",
		Filename: "x.pdf", StorageKey: "exports/x.pdf", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := other.repo.UpsertFileExport(ctx, ex); err != nil {
		t.Fatal(err)
	}
	err := svc.ProcessExportFor(ctx, org, ex.ID)
	if err == nil || !strings.Contains(err.Error(), "no pending input") {
		t.Fatalf("err = %v", err)
	}

	// An in-flight job in memory keeps its pending input across a refresh swap.
	in := sampleReportInput(ReportTypeEvent, "")
	svc.mu.Lock()
	svc.exports[ex.ID].pendingInput = &in
	svc.mu.Unlock()
	if err := svc.syncOrg(org); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessExportFor(ctx, org, ex.ID); err != nil {
		t.Fatalf("process after refresh: %v", err)
	}
	// ready state was persisted.
	fresh, err := other.repo.LoadFileExports(ctx, org)
	if err != nil || len(fresh) != 1 || fresh[0].Status != "ready" || fresh[0].SizeBytes == 0 {
		t.Fatalf("persisted = %+v err=%v", fresh, err)
	}
	if err := svc.ProcessExportFor(ctx, org, uuid.NewString()); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
}

func TestCov1xDBSyncOrgFallsBackToLockedLoadAfterWriterConflicts(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := NewServiceWithPool(admin, storage.NewStubStorage(""))
	if err := svc.repo.UpsertFileExport(context.Background(), &FileExport{ID: uuid.NewString(), OrgID: org, ExportType: ReportTypeEvent, Status: "ready",
		Filename: "f.pdf", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Every lock-free read is invalidated by a writer finishing during the load.
	bumps := 0
	hooked := cov1xHookedPool(t, admin, func(int32) {
		if bumps < 3 {
			bumps++
			svc.mu.Lock()
			svc.mu.Unlock()
		}
	})
	svc.repo = newRepository(hooked)
	if err := svc.syncOrg(org); err != nil {
		t.Fatalf("syncOrg: %v", err)
	}
	if bumps != 3 {
		t.Fatalf("expected 3 conflicting reads, saw %d", bumps)
	}
	if list := svc.ListExportsForAuthorizedMeet(org, ""); len(list) != 1 {
		t.Fatalf("list after locked load = %d", len(list))
	}
}

func TestCov1xDBSyncOrgErrors(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	ctx := context.Background()

	// First (lock-free) load fails.
	svc := NewServiceWithPool(admin, storage.NewStubStorage(""))
	if _, err := admin.Exec(ctx, `ALTER TABLE file_exports RENAME COLUMN filename TO filename_gone`); err != nil {
		t.Fatal(err)
	}
	err := svc.syncOrg(org)
	if err == nil || !strings.Contains(err.Error(), "load file_exports") {
		t.Fatalf("err = %v", err)
	}
	// Read paths that cannot return an error serve the cache and log.
	if list := svc.ListExportsForAuthorizedMeet(org, ""); len(list) != 0 {
		t.Fatalf("list = %d", len(list))
	}
	if _, err := svc.GetExport(cov1xAdmin, org, "x"); err == nil {
		t.Fatal("GetExport must surface load failure")
	}
	if _, _, err := svc.DownloadExport(cov1xAdmin, org, "x"); err == nil {
		t.Fatal("DownloadExport must surface load failure")
	}
	if _, err := svc.ExpireArtifactsForOrg(org, time.Now()); err == nil {
		t.Fatal("ExpireArtifactsForOrg must surface load failure")
	}
	if err := svc.ProcessExportFor(ctx, org, "x"); err == nil {
		t.Fatal("ProcessExportFor must surface load failure")
	}
	if _, err := admin.Exec(ctx, `ALTER TABLE file_exports RENAME COLUMN filename_gone TO filename`); err != nil {
		t.Fatal(err)
	}

	// The locked fallback load fails: break the table after the three conflicts.
	broken := false
	hooked := cov1xHookedPool(t, admin, func(n int32) {
		if n <= 3 {
			svc.mu.Lock()
			svc.mu.Unlock()
		} else if !broken {
			broken = true
			if _, err := admin.Exec(ctx, `ALTER TABLE file_exports RENAME COLUMN filename TO filename_gone`); err != nil {
				t.Errorf("rename: %v", err)
			}
		}
	})
	svc.repo = newRepository(hooked)
	if err := svc.syncOrg(org); err == nil || !strings.Contains(err.Error(), "load file_exports") {
		t.Fatalf("locked fallback err = %v", err)
	}
	if !broken {
		t.Fatal("fallback load never ran")
	}
}

func TestCov1xDBPersistFailures(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	ctx := context.Background()
	svc := NewServiceWithPool(admin, storage.NewStubStorage(""))
	if _, err := admin.Exec(ctx, `ALTER TABLE file_exports RENAME COLUMN filename TO filename_gone`); err != nil {
		t.Fatal(err)
	}
	// Creation fails closed and leaves nothing in memory.
	if _, _, err := svc.GenerateExport(cov1xAdmin, org, "", sampleReportInput(ReportTypeEvent, "")); err == nil || !strings.Contains(err.Error(), "persist file export") {
		t.Fatalf("err = %v", err)
	}
	if len(svc.exports) != 0 {
		t.Fatal("unpersisted export must not be cached")
	}
	// ProcessExport persistence failure on the ready write, and FailExport / expiry logging paths.
	in := sampleReportInput(ReportTypeEvent, "")
	svc.exports["j"] = &FileExport{ID: uuid.NewString(), OrgID: org, Status: "pending", StorageKey: "k", pendingInput: &in, ExpiresAt: time.Now().Add(-time.Hour)}
	id := svc.exports["j"].ID
	svc.exports[id] = svc.exports["j"]
	if err := svc.ProcessExport(ctx, id); err == nil || !strings.Contains(err.Error(), "persist ready file export") {
		t.Fatalf("err = %v", err)
	}
	svc.FailExport(id, errors.New("boom"))
	if svc.exports[id].Status != "failed" || svc.exports[id].LastError != "boom" {
		t.Fatalf("state = %+v", svc.exports[id])
	}
	svc.exports[id].Status = "ready"
	if got := svc.expireArtifacts(org, time.Now()); len(got) != 1 || got[0].Status != "expired" {
		t.Fatalf("expire despite persist failure = %+v", got)
	}
}

func TestCov1xDBLoadFileExportsScanError(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	ctx := context.Background()
	repo := newRepository(admin)
	if err := repo.UpsertFileExport(ctx, &FileExport{ID: uuid.NewString(), OrgID: org, ExportType: ReportTypeEvent, Status: "ready",
		Filename: "f.pdf", ExpiresAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// expires_at is nullable in the schema but the loader scans into time.Time.
	if _, err := admin.Exec(ctx, `UPDATE file_exports SET expires_at = NULL WHERE org_id = $1::uuid`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadFileExports(ctx, org); err == nil || !strings.Contains(err.Error(), "scan file_export") {
		t.Fatalf("err = %v", err)
	}
	// Soft-deleted rows are excluded.
	if _, err := admin.Exec(ctx, `UPDATE file_exports SET expires_at = now(), deleted_at = now() WHERE org_id = $1::uuid`, org); err != nil {
		t.Fatal(err)
	}
	if rows, err := repo.LoadFileExports(ctx, org); err != nil || len(rows) != 0 {
		t.Fatalf("rows = %d err=%v", len(rows), err)
	}
}
