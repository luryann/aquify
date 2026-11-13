package offboarding

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/luryann/aquify/server/internal/testutil"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/storage"
)

// cov1xStore is a real in-memory store with injectable failures for the bulk
// operations offboarding relies on.
type cov1xStore struct {
	*storage.StubStorage
	listErr, putErr, deleteErr error
}

func (s *cov1xStore) ListPrefix(ctx context.Context, p string) ([]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.StubStorage.ListPrefix(ctx, p)
}

func (s *cov1xStore) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.StubStorage.PutObject(ctx, key, ct, data)
}

func (s *cov1xStore) DeletePrefix(ctx context.Context, p string) (int, error) {
	n, err := s.StubStorage.DeletePrefix(ctx, p)
	if s.deleteErr != nil {
		return n, s.deleteErr
	}
	return n, err
}

// cov1xBareStore hides the optional PrefixStore capability.
type cov1xBareStore struct{ storage.ObjectStorage }

func cov1xExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func cov1xCount(t *testing.T, pool *pgxpool.Pool, table, orgID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE org_id = $1::uuid`, orgID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func cov1xStatus(t *testing.T, f *fixture, id string) *orgs.Org {
	t.Helper()
	o, err := f.reg.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// cov1xBoom installs a trigger function that always raises.
func cov1xBoom(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cov1xExec(t, pool, `CREATE FUNCTION cov1x_boom() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x boom'; END $$`)
}

// cov1xTextOrgTable adds a tenant-looking table whose org_id cannot be compared to a uuid,
// so every export/purge/restore statement against it fails.
func cov1xTextOrgTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cov1xExec(t, pool, `CREATE TABLE cov1x_textorg (org_id text NOT NULL, id int NOT NULL)`)
}

// cov1xDeferTable adds a tenant table whose commit always fails (deferred constraint trigger).
func cov1xDeferTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cov1xBoom(t, pool)
	cov1xExec(t, pool, `CREATE TABLE cov1x_defer (org_id uuid NOT NULL, id int NOT NULL)`)
}

func cov1xArmDefer(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cov1xExec(t, pool, `CREATE CONSTRAINT TRIGGER cov1x_defer_block AFTER INSERT OR DELETE ON cov1x_defer DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cov1x_boom()`)
}

// cov1xMemService wires a service whose registry is in memory (so org lookups work) but
// whose database pool is closed, to exercise database failure paths.
func cov1xMemService(t *testing.T) (*Service, *orgs.Org) {
	t.Helper()
	ctx := context.Background()
	reg := orgs.NewService()
	o, err := reg.Create(ctx, orgs.CreateInput{Slug: "cov1x-mem", Name: "Mem"})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{orgs.StatusActive, orgs.StatusOffboarding} {
		if o, err = reg.SetStatus(ctx, o.ID, st); err != nil {
			t.Fatal(err)
		}
	}
	svc := &Service{
		Pool: testutil.ClosedPool(t), Orgs: reg, Store: storage.NewStubStorage("http://stub"), Keycloak: keycloak.NewStubClient(),
		CoolingOff: time.Hour, Now: func() time.Time { return time.Now().Add(2 * time.Hour) },
	}
	return svc, o
}

// cov1xOffboard moves org o into offboarding and past the cooling-off period.
func cov1xOffboard(t *testing.T, f *fixture, o *orgs.Org) {
	t.Helper()
	if _, err := f.reg.SetStatus(context.Background(), o.ID, orgs.StatusOffboarding); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Hour)
}

func TestCov1xServiceDefaults(t *testing.T) {
	t.Parallel()
	if got := (&Service{}).cooling(); got != DefaultCoolingOff {
		t.Fatalf("default cooling-off = %v, want %v", got, DefaultCoolingOff)
	}
	if got := (&Service{CoolingOff: time.Minute}).cooling(); got != time.Minute {
		t.Fatalf("configured cooling-off = %v", got)
	}
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	if (&Service{Logger: custom}).logger() != custom {
		t.Fatal("a configured logger must be used")
	}
	if (&Service{}).logger() != slog.Default() {
		t.Fatal("the default logger is slog.Default")
	}
	before := time.Now()
	got := (&Service{}).now()
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("default clock must be time.Now, got %v", got)
	}
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	if !(&Service{Now: func() time.Time { return fixed }}).now().Equal(fixed) {
		t.Fatal("an injected clock must be used")
	}
}

func TestCov1xEveryOperationNeedsADatabase(t *testing.T) {
	t.Parallel()
	s := &Service{}
	ctx := context.Background()
	if _, err := s.Export(ctx, uuid.NewString()); !errors.Is(err, ErrNoDatabase) {
		t.Fatalf("Export: %v", err)
	}
	if _, err := s.Purge(ctx, uuid.NewString(), "x"); !errors.Is(err, ErrNoDatabase) {
		t.Fatalf("Purge: %v", err)
	}
	if _, err := s.Restore(ctx, uuid.NewString(), nil); !errors.Is(err, ErrNoDatabase) {
		t.Fatalf("Restore: %v", err)
	}
}

func TestCov1xExportAndPurgeUnknownOrgAreNotFound(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Export(context.Background(), uuid.NewString())
	if !errors.Is(err, orgs.ErrNotFound) || res != nil {
		t.Fatalf("export unknown org: %+v %v", res, err)
	}
	if _, err := f.svc.Purge(context.Background(), uuid.NewString(), "x"); !errors.Is(err, orgs.ErrNotFound) {
		t.Fatalf("purge unknown org: %v", err)
	}
}

func TestCov1xExportSurfacesDatabaseFailure(t *testing.T) {
	t.Parallel()
	svc, o := cov1xMemService(t)
	res, err := svc.Export(context.Background(), o.ID)
	if err == nil || res != nil || !strings.Contains(err.Error(), "closed pool") {
		t.Fatalf("a failing database must fail the export: %+v %v", res, err)
	}
}

func TestCov1xExportNamesTheTableThatFailed(t *testing.T) {
	f := newFixture(t)
	cov1xTextOrgTable(t, f.pool)
	res, err := f.svc.Export(context.Background(), f.a.ID)
	if err == nil || res != nil || !strings.HasPrefix(err.Error(), "export cov1x_textorg:") {
		t.Fatalf("export must name the failing table: %+v %v", res, err)
	}
	if keys, _ := f.store.ListPrefix(context.Background(), "offboarding/"); len(keys) != 0 {
		t.Fatalf("a failed export must not store a partial archive: %v", keys)
	}
}

func TestCov1xExportStorageFailures(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boom := errors.New("cov1x storage down")

	f.svc.Store = &cov1xStore{StubStorage: f.store, listErr: boom}
	if res, err := f.svc.Export(ctx, f.a.ID); !errors.Is(err, boom) || res != nil {
		t.Fatalf("a listing failure must fail the export: %+v %v", res, err)
	}

	f.svc.Store = &cov1xStore{StubStorage: f.store, putErr: boom}
	res, err := f.svc.Export(ctx, f.a.ID)
	if !errors.Is(err, boom) || res != nil || !strings.Contains(err.Error(), "store export") {
		t.Fatalf("a failed upload must be reported as such: %+v %v", res, err)
	}
	if keys, _ := f.store.ListPrefix(ctx, "offboarding/"); len(keys) != 0 {
		t.Fatalf("nothing may be stored after a failed upload: %v", keys)
	}
}

func TestCov1xExportWithoutPrefixStoreHasEmptyManifest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.svc.Store = cov1xBareStore{f.store}
	res, err := f.svc.Export(ctx, f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 0 || res.Tables["households"] != 1 {
		t.Fatalf("tables must still export, files cannot be listed: %+v", res)
	}
	data, _, err := f.store.GetObject(ctx, res.Key)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, zf := range zr.File {
		if zf.Name != "files.json" {
			continue
		}
		rc, _ := zf.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		if strings.TrimSpace(string(b)) != "[]" {
			t.Fatalf("manifest = %q, want []", b)
		}
		return
	}
	t.Fatal("files.json missing from the archive")
}

func TestCov1xTableHelpersReportCancelledContexts(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	zw := zip.NewWriter(io.Discard)
	if n, err := f.svc.writeTable(ctx, zw, "households", f.a.ID); !errors.Is(err, context.Canceled) || n != 0 {
		t.Fatalf("writeTable: %d %v", n, err)
	}
	if n, err := f.svc.deleteRows(ctx, "households", f.a.ID); !errors.Is(err, context.Canceled) || n != 0 {
		t.Fatalf("deleteRows: %d %v", n, err)
	}
	if cov1xCount(t, f.pool, "households", f.a.ID) != 1 {
		t.Fatal("a cancelled delete must not remove anything")
	}
}

func TestCov1xPurgeRefusesStoresThatCannotBulkDelete(t *testing.T) {
	f := newFixture(t)
	cov1xOffboard(t, f, f.a)
	f.svc.Store = cov1xBareStore{f.store}
	if _, err := f.svc.Purge(context.Background(), f.a.ID, "club-a"); !errors.Is(err, ErrStorageUnsupport) {
		t.Fatalf("purge needs bulk delete: %v", err)
	}
	// Refused before claiming: the org can still be cancelled.
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusOffboarding {
		t.Fatalf("a refused purge must not claim the org, status=%s", got.Status)
	}
	if cov1xCount(t, f.pool, "households", f.a.ID) != 1 {
		t.Fatal("nothing may be deleted")
	}
}

func TestCov1xPurgeStopsWhenClaimFails(t *testing.T) {
	f := newFixture(t)
	cov1xOffboard(t, f, f.a)
	cov1xBoom(t, f.pool)
	cov1xExec(t, f.pool, `CREATE TRIGGER cov1x_block BEFORE UPDATE ON orgs FOR EACH ROW WHEN (NEW.status = 'purging') EXECUTE FUNCTION cov1x_boom()`)
	res, err := f.svc.Purge(context.Background(), f.a.ID, "club-a")
	if err == nil || res != nil || !strings.Contains(err.Error(), "cov1x boom") {
		t.Fatalf("claim failure must abort the purge: %+v %v", res, err)
	}
	if cov1xCount(t, f.pool, "households", f.a.ID) != 1 {
		t.Fatal("nothing irreversible may happen before the claim succeeds")
	}
	if ok, _ := f.store.Exists(context.Background(), "documents/"+f.a.ID+"/sub/file-a.pdf"); !ok {
		t.Fatal("files must survive an unclaimed purge")
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusOffboarding {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestCov1xPurgeIdentityProviderFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("listing users fails", func(t *testing.T) {
		f := newFixture(t)
		cov1xOffboard(t, f, f.a)
		boom := errors.New("cov1x keycloak list")
		f.kc.ListUsersErr = boom
		res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
		if !errors.Is(err, boom) || res != nil || !strings.Contains(err.Error(), "list identity users") {
			t.Fatalf("%+v %v", res, err)
		}
		if ok, _ := f.store.Exists(ctx, "documents/"+f.a.ID+"/sub/file-a.pdf"); !ok {
			t.Fatal("files must not be deleted before identity users are handled")
		}
		if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusPurging {
			t.Fatalf("a claimed purge stays retryable in purging, got %s", got.Status)
		}
	})
	t.Run("deleting a user fails", func(t *testing.T) {
		f := newFixture(t)
		cov1xOffboard(t, f, f.a)
		boom := errors.New("cov1x keycloak delete")
		f.kc.DeleteUserErr = boom
		res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
		if !errors.Is(err, boom) || res == nil || res.UsersDeleted != 0 || !strings.Contains(err.Error(), "delete identity user") {
			t.Fatalf("%+v %v", res, err)
		}
		if cov1xCount(t, f.pool, "households", f.a.ID) != 1 {
			t.Fatal("rows must survive a failed identity cleanup")
		}
	})
	t.Run("an already-removed user is not an error", func(t *testing.T) {
		f := newFixture(t)
		cov1xOffboard(t, f, f.a)
		f.kc.DeleteUserErr = keycloak.ErrUserNotFound
		res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
		if err != nil || res.UsersDeleted != 1 {
			t.Fatalf("%+v %v", res, err)
		}
		if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusDeleted {
			t.Fatalf("status = %s", got.Status)
		}
	})
}

func TestCov1xPurgeDeletesEveryPageOfIdentityUsers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 250; i++ {
		if _, err := f.kc.CreateUser(ctx, fmt.Sprintf("bulk%03d@example.com", i), "B", "U", f.a.ID); err != nil {
			t.Fatal(err)
		}
	}
	cov1xOffboard(t, f, f.a)
	res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
	if err != nil {
		t.Fatal(err)
	}
	if res.UsersDeleted != 251 {
		t.Fatalf("users deleted = %d, want 251 (more than one listing page)", res.UsersDeleted)
	}
	if left, _ := f.kc.ListUsers(ctx, f.a.ID, "", 500, 0); len(left) != 0 {
		t.Fatalf("org A users left: %d", len(left))
	}
	if left, _ := f.kc.ListUsers(ctx, f.b.ID, "", 500, 0); len(left) != 1 {
		t.Fatalf("org B users must survive: %d", len(left))
	}
}

func TestCov1xPurgeReportsStorageDeleteFailure(t *testing.T) {
	f := newFixture(t)
	cov1xOffboard(t, f, f.a)
	boom := errors.New("cov1x delete prefix")
	f.svc.Store = &cov1xStore{StubStorage: f.store, deleteErr: boom}
	res, err := f.svc.Purge(context.Background(), f.a.ID, "club-a")
	if !errors.Is(err, boom) || res == nil || res.FilesDeleted != 1 || res.UsersDeleted != 1 {
		t.Fatalf("partial progress must be reported: %+v %v", res, err)
	}
	if cov1xCount(t, f.pool, "households", f.a.ID) != 1 {
		t.Fatal("database rows must not be deleted after a storage failure")
	}
}

func TestCov1xPurgeSurfacesDatabaseFailure(t *testing.T) {
	t.Parallel()
	svc, o := cov1xMemService(t)
	if err := svc.Store.PutObject(context.Background(), "documents/"+o.ID+"/a.pdf", "application/pdf", []byte("x")); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Purge(context.Background(), o.ID, "cov1x-mem")
	if err == nil || !strings.Contains(err.Error(), "closed pool") || res == nil || res.FilesDeleted != 1 {
		t.Fatalf("a failing database must stop the purge after files: %+v %v", res, err)
	}
	got, _ := svc.Orgs.Get(context.Background(), o.ID)
	if got.Status != orgs.StatusPurging || got.Slug == "" {
		t.Fatalf("the org must stay in purging with its slug until the purge finishes: %+v", got)
	}
}

func TestCov1xPurgeGivesUpOnRowsThatCannotBeDeletedAndCanBeRetried(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cov1xTextOrgTable(t, f.pool)
	cov1xOffboard(t, f, f.a)
	res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
	if err == nil || !strings.Contains(err.Error(), "could not delete rows from [cov1x_textorg]") {
		t.Fatalf("stuck tables must be named: %+v %v", res, err)
	}
	if res.RowsDeleted["households"] != 1 {
		t.Fatalf("deletable tables are still purged: %+v", res.RowsDeleted)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusPurging || got.Slug != "club-a" {
		t.Fatalf("must remain retryable: %+v", got)
	}
	cov1xExec(t, f.pool, `DROP TABLE cov1x_textorg`)
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); err != nil {
		t.Fatalf("retry after fixing the cause: %v", err)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusDeleted {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestCov1xPurgeRollsBackATableWhoseCommitFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cov1xDeferTable(t, f.pool)
	cov1xExec(t, f.pool, `INSERT INTO cov1x_defer (org_id, id) VALUES ($1::uuid, 1)`, f.a.ID)
	cov1xArmDefer(t, f.pool)
	cov1xOffboard(t, f, f.a)
	_, err := f.svc.Purge(ctx, f.a.ID, "club-a")
	if err == nil || !strings.Contains(err.Error(), "could not delete rows from [cov1x_defer]") {
		t.Fatalf("a failed commit must leave the table stuck: %v", err)
	}
	if cov1xCount(t, f.pool, "cov1x_defer", f.a.ID) != 1 {
		t.Fatal("a failed commit must leave the rows in place")
	}
}

func TestCov1xPurgeTombstoneFailureIsRetryable(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cov1xOffboard(t, f, f.a)
	cov1xBoom(t, f.pool)
	cov1xExec(t, f.pool, `CREATE TRIGGER cov1x_block BEFORE UPDATE ON orgs FOR EACH ROW WHEN (NEW.status = 'deleted') EXECUTE FUNCTION cov1x_boom()`)
	res, err := f.svc.Purge(ctx, f.a.ID, "club-a")
	if err == nil || !strings.Contains(err.Error(), "cov1x boom") || res == nil || res.RowsDeleted["households"] != 1 {
		t.Fatalf("tombstone failure: %+v %v", res, err)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusPurging || got.Slug != "club-a" {
		t.Fatalf("slug and status change together or not at all: %+v", got)
	}
	cov1xExec(t, f.pool, `DROP TRIGGER cov1x_block ON orgs`)
	if _, err := f.svc.Purge(ctx, f.a.ID, "club-a"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusDeleted || got.Slug != "" {
		t.Fatalf("tombstone after retry: %+v", got)
	}
}

func TestCov1xPurgeWithoutIdentityProviderLogsCompletion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var buf bytes.Buffer
	f.svc.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	f.svc.Keycloak = nil
	cov1xOffboard(t, f, f.a)
	res, err := f.svc.Purge(ctx, f.a.ID, "  club-a  ")
	if err != nil {
		t.Fatal(err)
	}
	if res.UsersDeleted != 0 || res.FilesDeleted != 1 {
		t.Fatalf("%+v", res)
	}
	if left, _ := f.kc.ListUsers(ctx, f.a.ID, "", 50, 0); len(left) != 1 {
		t.Fatal("without an identity provider no users are touched")
	}
	logged := buf.String()
	if !strings.Contains(logged, `"msg":"org purged"`) || !strings.Contains(logged, f.a.ID) {
		t.Fatalf("completion must be logged with the org id: %s", logged)
	}
	var kept []string
	for k := range purgeKept {
		kept = append(kept, k)
	}
	sort.Strings(kept)
	for _, tbl := range kept {
		if tbl == "orgs" || tbl == "org_data_versions" {
			continue
		}
		if cov1xCount(t, f.pool, tbl, f.a.ID) == 0 {
			t.Errorf("%s is retained by a purge but is empty", tbl)
		}
	}
}
