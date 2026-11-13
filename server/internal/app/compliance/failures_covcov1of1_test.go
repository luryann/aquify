package compliance

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
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xDeadRepo returns a repository whose every statement fails immediately
// (port 1 refuses connections), standing in for an unreachable database.
func cov1xDeadRepo(t *testing.T) *Repository {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return newRepository(pool)
}

func cov1xDeadService(t *testing.T) *Service {
	t.Helper()
	svc := NewServiceWithConfig(7, 14)
	svc.repo = cov1xDeadRepo(t)
	return svc
}

func cov1xExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// cov1xBreakSQL replaces a table in the test's isolated schema with a view that
// lacks the expected columns (a plain DROP would fall through to public.<table>
// on the search_path), so every statement against it fails.
func cov1xBreakSQL(table string) string {
	return "DROP TABLE " + table + " CASCADE; CREATE VIEW " + table + " AS SELECT 1 AS cov1x_broken"
}

func cov1xBreakTable(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	cov1xExec(t, pool, cov1xBreakSQL(table))
}

// ─── Repository failure paths ─────────────────────────────────────────────────

func TestCov1xRepositoryReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()
	repo := cov1xDeadRepo(t)
	ctx := context.Background()

	if _, err := repo.LoadDataExportRequests(ctx, "o"); err == nil || !strings.Contains(err.Error(), "load data_export_requests") {
		t.Fatalf("LoadDataExportRequests err = %v", err)
	}
	if _, err := repo.LoadDataDeletionRequests(ctx, "o"); err == nil || !strings.Contains(err.Error(), "load data_deletion_requests") {
		t.Fatalf("LoadDataDeletionRequests err = %v", err)
	}
	if _, err := repo.GetDataExportRequest(ctx, "o", "i"); err == nil || !strings.Contains(err.Error(), "get data_export_request") {
		t.Fatalf("GetDataExportRequest err = %v", err)
	}
	if _, err := repo.GetDataDeletionRequest(ctx, "o", "i"); err == nil || !strings.Contains(err.Error(), "get data_deletion_request") {
		t.Fatalf("GetDataDeletionRequest err = %v", err)
	}
	exports, deletions, err := repo.LoadOrg(ctx, "o")
	if err == nil || exports != nil || deletions != nil {
		t.Fatalf("LoadOrg = %v, %v, %v; want error and no data", exports, deletions, err)
	}
	req := &DataDeletionRequest{ID: "i", OrgID: "o", Status: "approved"}
	err = repo.TransitionDataDeletionRequest(ctx, req, []string{"pending"})
	if err == nil || errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("TransitionDataDeletionRequest err = %v, want a database error", err)
	}
}

func TestCov1xRepositoryLoadOrgDeletionTableFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cov1xBreakTable(t, pool, "data_deletion_requests")
	repo := newRepository(pool)

	exports, _, err := repo.LoadOrg(context.Background(), uuid.NewString())
	if err == nil || !strings.Contains(err.Error(), "load data_deletion_requests") {
		t.Fatalf("LoadOrg err = %v, want deletion load failure", err)
	}
	if len(exports) != 0 {
		t.Fatalf("exports = %v, want none for an empty org", exports)
	}
}

func TestCov1xRepositoryTransitionRejectsMovedState(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool, 30, 30)
	created, err := svc.CreateDataDeletionRequest(org, hh, "parent-1", "leaving")
	if err != nil {
		t.Fatal(err)
	}

	moved := *created
	moved.Status = "approved"
	moved.UpdatedAt = time.Now().UTC()
	// The stored row is "pending"; a caller that believes it is "reviewing" loses.
	err = svc.repo.TransitionDataDeletionRequest(context.Background(), &moved, []string{"reviewing"})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidTransition)
	}
	got, err := svc.repo.GetDataDeletionRequest(context.Background(), org, created.ID)
	if err != nil || got == nil || got.Status != "pending" {
		t.Fatalf("stored row = %+v err=%v, want it untouched (pending)", got, err)
	}

	// With the right precondition the same change applies.
	if err := svc.repo.TransitionDataDeletionRequest(context.Background(), &moved, []string{"pending"}); err != nil {
		t.Fatalf("transition from pending: %v", err)
	}
	if got, _ = svc.repo.GetDataDeletionRequest(context.Background(), org, created.ID); got == nil || got.Status != "approved" {
		t.Fatalf("stored row = %+v, want approved", got)
	}
}

func TestCov1xRepositoryGetReturnsNilWhenAbsent(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	repo := newRepository(pool)
	org := uuid.NewString()

	if got, err := repo.GetDataExportRequest(context.Background(), org, uuid.NewString()); got != nil || err != nil {
		t.Fatalf("GetDataExportRequest = %v, %v; want nil, nil", got, err)
	}
	if got, err := repo.GetDataDeletionRequest(context.Background(), org, uuid.NewString()); got != nil || err != nil {
		t.Fatalf("GetDataDeletionRequest = %v, %v; want nil, nil", got, err)
	}
}

// ─── Service failure paths with an unreachable database ──────────────────────

func TestCov1xServiceSurfacesDatabaseErrors(t *testing.T) {
	t.Parallel()
	svc := cov1xDeadService(t)

	if _, err := svc.GetDataExportRequest("o", "i"); err == nil || errors.Is(err, ErrExportNotFound) {
		t.Fatalf("GetDataExportRequest err = %v, want database error", err)
	}
	if _, err := svc.GetDataDeletionRequest("o", "i"); err == nil || errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("GetDataDeletionRequest err = %v, want database error", err)
	}
	if _, err := svc.ListDataExportRequests("o", ""); err == nil {
		t.Fatal("ListDataExportRequests succeeded with unreachable database")
	}
	if _, err := svc.ListDataDeletionRequests("o", ""); err == nil {
		t.Fatal("ListDataDeletionRequests succeeded with unreachable database")
	}
	if _, err := svc.MarkDataExportProcessing("o", "i"); err == nil || errors.Is(err, ErrExportNotFound) {
		t.Fatalf("MarkDataExportProcessing err = %v, want database error", err)
	}
	if _, err := svc.ApproveDataDeletion("o", "i", "n"); err == nil || errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("ApproveDataDeletion err = %v, want database error", err)
	}
}

// ─── Service behaviour against real Postgres ─────────────────────────────────

func TestCov1xSyncEvictsRowsRemovedFromDatabase(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	orgA, orgB, hh := uuid.NewString(), uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool, 30, 30)

	exp, err := svc.CreateDataExportRequest(orgA, hh, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	del, err := svc.CreateDataDeletionRequest(orgA, hh, "parent-1", "bye")
	if err != nil {
		t.Fatal(err)
	}

	// A lookup from another club finds nothing and must not evict club A's entry.
	if _, err := svc.GetDataExportRequest(orgB, exp.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("cross-org export get err = %v", err)
	}
	if _, err := svc.GetDataDeletionRequest(orgB, del.ID); !errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("cross-org deletion get err = %v", err)
	}
	svc.mu.RLock()
	_, expCached := svc.exports[exp.ID]
	_, delCached := svc.deletions[del.ID]
	svc.mu.RUnlock()
	if !expCached || !delCached {
		t.Fatalf("cross-org lookup evicted club A's cache (export=%v deletion=%v)", expCached, delCached)
	}

	// Another process removes the rows; the owning club's next read drops the stale copy.
	cov1xExec(t, pool, `DELETE FROM data_export_requests WHERE id = $1`, exp.ID)
	cov1xExec(t, pool, `DELETE FROM data_deletion_requests WHERE id = $1`, del.ID)
	if _, err := svc.GetDataExportRequest(orgA, exp.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("export get after delete err = %v", err)
	}
	if _, err := svc.GetDataDeletionRequest(orgA, del.ID); !errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("deletion get after delete err = %v", err)
	}
	svc.mu.RLock()
	_, expCached = svc.exports[exp.ID]
	_, delCached = svc.deletions[del.ID]
	svc.mu.RUnlock()
	if expCached || delCached {
		t.Fatalf("stale cache entries remain (export=%v deletion=%v)", expCached, delCached)
	}
	if _, err := svc.MarkDataExportProcessing(orgA, exp.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("transition of removed export err = %v", err)
	}
	if _, err := svc.MarkDataDeletionProcessing(orgA, del.ID); !errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("transition of removed deletion err = %v", err)
	}
}

func TestCov1xTransitionPersistFailureLeavesCacheUntouched(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	svc := NewServiceWithPool(pool, 30, 30)

	exp, err := svc.CreateDataExportRequest(org, hh, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	del, err := svc.CreateDataDeletionRequest(org, hh, "parent-1", "bye")
	if err != nil {
		t.Fatal(err)
	}
	// Make the database refuse the target statuses so the write fails after the
	// in-memory precondition checks pass.
	cov1xExec(t, pool, `ALTER TABLE data_export_requests ADD CONSTRAINT cov1x_no_processing CHECK (status <> 'processing')`)
	cov1xExec(t, pool, `ALTER TABLE data_deletion_requests ADD CONSTRAINT cov1x_no_approved CHECK (status <> 'approved')`)

	if _, err := svc.MarkDataExportProcessing(org, exp.ID); err == nil || errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("MarkDataExportProcessing err = %v, want database error", err)
	}
	if got, err := svc.GetDataExportRequest(org, exp.ID); err != nil || got.Status != "pending" {
		t.Fatalf("export after failed write = %+v err=%v, want pending", got, err)
	}
	if _, err := svc.ApproveDataDeletion(org, del.ID, "ok"); err == nil || errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ApproveDataDeletion err = %v, want database error", err)
	}
	if got, err := svc.GetDataDeletionRequest(org, del.ID); err != nil || got.Status != "pending" || got.ReviewNotes != "" {
		t.Fatalf("deletion after failed write = %+v err=%v, want untouched pending", got, err)
	}
}

func TestCov1xReloadFallsBackToLockedLoadAfterRepeatedWriterConflicts(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	other := NewServiceWithPool(admin, 30, 30)
	exp, err := other.CreateDataExportRequest(org, hh, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	del, err := other.CreateDataDeletionRequest(org, hh, "parent-1", "bye")
	if err != nil {
		t.Fatal(err)
	}

	var svc *Service
	var acquires atomic.Int32
	cfg := admin.Config().Copy()
	// LoadOrg issues two statements per attempt. For the three lock-free attempts
	// (acquires 1-6) a writer section "finishes" during the read, which forces a
	// re-read; the fallback load (acquires 7-8) runs under the lock untouched.
	cfg.BeforeAcquire = func(context.Context, *pgx.Conn) bool {
		n := acquires.Add(1)
		if n <= 6 && n%2 == 1 {
			svc.mu.Lock()
			svc.mu.Unlock()
		}
		return true
	}
	hooked, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hooked.Close)
	svc = NewServiceWithPool(hooked, 30, 30)

	exports, err := svc.ListDataExportRequests(org, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(exports) != 1 || exports[0].ID != exp.ID {
		t.Fatalf("exports = %+v, want the single stored request", exports)
	}
	if got := acquires.Load(); got != 8 {
		t.Fatalf("LoadOrg ran %d statements, want 8 (3 conflicting attempts + locked fallback)", got)
	}
	deletions, err := svc.ListDataDeletionRequests(org, hh)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletions) != 1 || deletions[0].ID != del.ID {
		t.Fatalf("deletions = %+v, want the single stored request", deletions)
	}
}

func TestCov1xReloadFallbackReportsLoadError(t *testing.T) {
	admin := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()

	var svc *Service
	var acquires atomic.Int32
	cfg := admin.Config().Copy()
	cfg.BeforeAcquire = func(ctx context.Context, _ *pgx.Conn) bool {
		n := acquires.Add(1)
		switch {
		case n <= 6 && n%2 == 1:
			svc.mu.Lock()
			svc.mu.Unlock()
		case n == 7:
			// Break the schema right before the locked fallback's first read.
			if _, err := admin.Exec(ctx, cov1xBreakSQL("data_export_requests")); err != nil {
				t.Errorf("break table: %v", err)
			}
		}
		return true
	}
	hooked, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hooked.Close)
	svc = NewServiceWithPool(hooked, 30, 30)

	_, err = svc.ListDataExportRequests(org, "")
	if err == nil || !strings.Contains(err.Error(), "load data_export_requests") {
		t.Fatalf("err = %v, want the locked fallback's load failure", err)
	}
	if got := acquires.Load(); got != 7 {
		t.Fatalf("statements = %d, want 7 (3 conflicting attempts, then the failing locked read)", got)
	}
}

// ─── Small helpers ───────────────────────────────────────────────────────────

func TestCov1xRejectDataDeletion(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithConfig(7, 14)
	req, err := svc.CreateDataDeletionRequest("org-a", "hh-1", "parent-1", "bye")
	if err != nil {
		t.Fatal(err)
	}

	for _, notes := range []string{"", "   \t"} {
		if _, err := svc.RejectDataDeletion("org-a", req.ID, notes); err == nil || !strings.Contains(err.Error(), "review notes are required") {
			t.Fatalf("RejectDataDeletion(%q) err = %v, want notes-required error", notes, err)
		}
	}
	if got, _ := svc.GetDataDeletionRequest("org-a", req.ID); got.Status != "pending" {
		t.Fatalf("status after rejected attempts = %q, want pending", got.Status)
	}

	rejected, err := svc.RejectDataDeletion("org-a", req.ID, "  retention obligation  ")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" || rejected.ReviewNotes != "retention obligation" {
		t.Fatalf("rejected = %+v", rejected)
	}
	// A rejected request cannot be approved or processed afterwards.
	if _, err := svc.ApproveDataDeletion("org-a", req.ID, "x"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("approve after reject err = %v", err)
	}
	if _, err := svc.MarkDataDeletionProcessing("org-a", req.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("processing after reject err = %v", err)
	}
	if _, err := svc.RejectDataDeletion("org-b", req.ID, "n"); !errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("cross-org reject err = %v", err)
	}
}

func TestCov1xAppendSystemNotes(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, existing, msg, want string }{
		{"blank message keeps notes", "admin note", "  ", "admin note"},
		{"no existing notes", "", "  worker done ", "worker done"},
		{"both are joined", "admin note", "worker done", "admin note\n\n[system] worker done"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		if got := appendSystemNotes(tc.existing, tc.msg); got != tc.want {
			t.Errorf("%s: appendSystemNotes(%q, %q) = %q, want %q", tc.name, tc.existing, tc.msg, got, tc.want)
		}
	}
}

func TestCov1xTransitionsOnUnknownRequests(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithConfig(7, 14)
	if _, err := svc.MarkDataExportProcessing("o", "missing"); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("export err = %v", err)
	}
	if _, err := svc.FailDataDeletion("o", "missing", "n"); !errors.Is(err, ErrDeletionNotFound) {
		t.Fatalf("deletion err = %v", err)
	}
	if exp, err := svc.ListDataExportRequests("o", "hh"); err != nil || exp == nil || len(exp) != 0 {
		t.Fatalf("empty export list = %#v err=%v, want non-nil empty", exp, err)
	}
	if del, err := svc.ListDataDeletionRequests("o", "hh"); err != nil || del == nil || len(del) != 0 {
		t.Fatalf("empty deletion list = %#v err=%v, want non-nil empty", del, err)
	}
}

func TestCov1xListFiltersByOrgAndHouseholdNewestFirst(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithConfig(7, 14)
	older, _ := svc.CreateDataExportRequest("org-a", "hh-1", "p")
	_, _ = svc.CreateDataExportRequest("org-a", "hh-2", "p")
	_, _ = svc.CreateDataExportRequest("org-b", "hh-1", "p")
	// Force a deterministic ordering instead of relying on clock resolution.
	svc.exports[older.ID].RequestedAt = time.Now().UTC().Add(-time.Hour)
	newer, _ := svc.CreateDataExportRequest("org-a", "hh-1", "p")

	got, err := svc.ListDataExportRequests("org-a", "hh-1")
	if err != nil || len(got) != 2 || got[0].ID != newer.ID || got[1].ID != older.ID {
		t.Fatalf("export list = %+v err=%v, want [newer, older] for org-a/hh-1", got, err)
	}
	if all, _ := svc.ListDataExportRequests("org-a", ""); len(all) != 3 {
		t.Fatalf("org-a export count = %d, want 3", len(all))
	}

	d1, _ := svc.CreateDataDeletionRequest("org-a", "hh-1", "p", "r")
	_, _ = svc.CreateDataDeletionRequest("org-a", "hh-2", "p", "r")
	_, _ = svc.CreateDataDeletionRequest("org-b", "hh-1", "p", "r")
	svc.deletions[d1.ID].RequestedAt = time.Now().UTC().Add(-time.Hour)
	d3, _ := svc.CreateDataDeletionRequest("org-a", "hh-1", "p", "r")
	gd, err := svc.ListDataDeletionRequests("org-a", "hh-1")
	if err != nil || len(gd) != 2 || gd[0].ID != d3.ID || gd[1].ID != d1.ID {
		t.Fatalf("deletion list = %+v err=%v, want [newer, older] for org-a/hh-1", gd, err)
	}
	if all, _ := svc.ListDataDeletionRequests("org-a", ""); len(all) != 3 {
		t.Fatalf("org-a deletion count = %d, want 3", len(all))
	}
}

func TestCov1xHouseholdDataCollector(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithConfig(7, 14)
	if got, err := svc.CollectHouseholdData("o", "h"); got != nil || err != nil {
		t.Fatalf("unwired collector = %v, %v; want nil, nil", got, err)
	}
	boom := errors.New("collector failed")
	svc.SetHouseholdDataCollector(func(orgID, householdID string) (map[string]any, error) {
		if householdID == "bad" {
			return nil, boom
		}
		return map[string]any{"org": orgID, "household": householdID}, nil
	})
	got, err := svc.CollectHouseholdData("o", "h")
	if err != nil || got["org"] != "o" || got["household"] != "h" {
		t.Fatalf("collector = %v, %v", got, err)
	}
	if _, err := svc.CollectHouseholdData("o", "bad"); !errors.Is(err, boom) {
		t.Fatalf("collector error = %v, want %v", err, boom)
	}
}

func TestCov1xNewIDIsRandomHex(t *testing.T) {
	t.Parallel()
	a, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newID()
	if len(a) != 32 || a == b {
		t.Fatalf("newID gave %q and %q, want distinct 32-char hex ids", a, b)
	}
	// In DB mode genID yields a UUID so ids fit the uuid-typed columns.
	dbSvc := NewServiceWithConfig(1, 1)
	dbSvc.repo = &Repository{}
	if _, err := uuid.Parse(dbSvc.genID()); err != nil {
		t.Fatalf("DB-mode genID is not a UUID: %v", err)
	}
}

// ─── Runtime snapshot ────────────────────────────────────────────────────────

func TestCov1xLoadRuntimeSnapshotEdgeCases(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithConfig(7, 14)
	req, err := svc.CreateDataExportRequest("org-a", "hh-1", "p")
	if err != nil {
		t.Fatal(err)
	}

	// Empty data is a no-op and keeps existing state.
	if err := svc.LoadRuntimeSnapshot(nil); err != nil {
		t.Fatalf("empty snapshot err = %v", err)
	}
	if _, err := svc.GetDataExportRequest("org-a", req.ID); err != nil {
		t.Fatalf("state lost after empty snapshot: %v", err)
	}
	// Malformed JSON is rejected and does not clobber state.
	if err := svc.LoadRuntimeSnapshot([]byte(`{"exports":`)); err == nil {
		t.Fatal("malformed snapshot accepted")
	}
	if _, err := svc.GetDataExportRequest("org-a", req.ID); err != nil {
		t.Fatalf("state lost after malformed snapshot: %v", err)
	}
	// A valid snapshot replaces state wholesale.
	if err := svc.LoadRuntimeSnapshot([]byte(`{"exports":{},"deletions":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetDataExportRequest("org-a", req.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("export survived replacement snapshot: %v", err)
	}
}

func TestCov1xNewServiceWithPoolNilIsInMemory(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil, 5, 6)
	if svc.repo != nil || svc.exportTTLDays != 5 || svc.deletionReviewDays != 6 {
		t.Fatalf("service = %+v, want in-memory with configured TTLs", svc)
	}
	req, err := svc.CreateDataExportRequest("org-a", "hh-1", "p")
	if err != nil || req.Status != "pending" {
		t.Fatalf("create = %+v err=%v", req, err)
	}
}
