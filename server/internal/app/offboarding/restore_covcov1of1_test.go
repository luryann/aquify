package offboarding

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/orgs"
)

// cov1xZip builds an archive with the given file contents in a stable order.
func cov1xZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(files[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func cov1xFirstFile(t *testing.T, archive []byte) *zip.File {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	return zr.File[0]
}

func TestCov1xRestoreUnknownOrgIsReported(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Restore(context.Background(), uuid.NewString(), cov1xZip(t, map[string]string{"files.json": "[]"}))
	if !errors.Is(err, ErrRestoreOrgMissing) || res != nil {
		t.Fatalf("a restore needs the registry row: %+v %v", res, err)
	}
}

func TestCov1xRestoreSurfacesDatabaseFailure(t *testing.T) {
	t.Parallel()
	svc, o := cov1xMemService(t)
	res, err := svc.Restore(context.Background(), o.ID, nil)
	if err == nil || res != nil || errors.Is(err, ErrRestoreOrgMissing) || !strings.Contains(err.Error(), "closed pool") {
		t.Fatalf("a database failure is not 'org missing': %+v %v", res, err)
	}
}

func TestCov1xRestoreRefusesThePlatformOrg(t *testing.T) {
	f := newFixture(t)
	f.svc.PlatformOrgID = strings.ToUpper(f.a.ID)
	res, err := f.svc.Restore(context.Background(), f.a.ID, cov1xZip(t, map[string]string{"files.json": "[]"}))
	if !errors.Is(err, ErrProtectedOrg) || res != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestCov1xRestoreRejectsAnUnreadableArchive(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Restore(context.Background(), f.a.ID, []byte("definitely not a zip"))
	if err == nil || res != nil || !strings.HasPrefix(err.Error(), "read archive:") {
		t.Fatalf("%+v %v", res, err)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusActive {
		t.Fatalf("a rejected archive must not change the org: %s", got.Status)
	}
}

func TestCov1xRestoreReportsTableDiscoveryFailure(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var acquires atomic.Int32
	cfg := f.pool.Config().Copy()
	// The first acquire (org lookup) succeeds; the second (table discovery) cancels the context.
	cfg.BeforeAcquire = func(context.Context, *pgx.Conn) bool {
		if acquires.Add(1) >= 2 {
			cancel()
			return false
		}
		return true
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := &Service{Pool: pool, Orgs: f.reg, Store: f.store}
	res, err := svc.Restore(ctx, f.a.ID, cov1xZip(t, map[string]string{"files.json": "[]"}))
	if !errors.Is(err, context.Canceled) || res != nil {
		t.Fatalf("%+v %v", res, err)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusActive {
		t.Fatalf("a failed restore must not suspend the org: %s", got.Status)
	}
}

func TestCov1xRestoreSkipsSecretsAndUnknownTablesAndSuspendsTheOrg(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := uuid.NewString()
	archive := cov1xZip(t, map[string]string{
		"files.json":            "[]",
		"nosuchtable.ndjson":    `{"org_id":"` + f.a.ID + `"}` + "\n",
		"sessions.ndjson":       `{"org_id":"` + f.a.ID + `","id":"` + other + `"}` + "\n",
		"households.ndjson":     "\n   \n", // blank lines only
		"org_invites.ndjson":    `{"org_id":"` + f.a.ID + `","code_hash":"smuggled"}` + "\n",
		"swimmers.ndjson":       "",
		"mfa_factors.ndjson":    "{}\n",
		"webhook_events.ndjson": "{}\n",
	})
	res, err := f.svc.Restore(ctx, f.a.ID, archive)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mfa_factors", "nosuchtable", "org_invites", "sessions", "webhook_events"}
	got := append([]string(nil), res.Skipped...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("skipped = %v, want %v", got, want)
	}
	if res.RowsRestored["households"] != 0 || res.RowsRestored["swimmers"] != 0 {
		t.Fatalf("empty files restore nothing: %+v", res.RowsRestored)
	}
	if cov1xCount(t, f.pool, "org_invites", f.a.ID) != 1 {
		t.Fatal("credential tables in an archive must never be loaded")
	}
	o := cov1xStatus(t, f, f.a.ID)
	if o.Status != orgs.StatusSuspended || o.SuspendedAt == nil || o.OffboardingAt != nil {
		t.Fatalf("a restored org is suspended with the offboarding clock cleared: %+v", o)
	}
}

func TestCov1xRestoreLoadsLargeTablesInChunks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	const bulk = 2*restoreChunkRows + 100
	cov1xExec(t, f.pool, `INSERT INTO households (id, org_id, household_name, status) SELECT gen_random_uuid(), $1::uuid, 'bulk ' || g, 'active' FROM generate_series(1, $2::int) g`, f.a.ID, bulk)
	exp, err := f.svc.Export(ctx, f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Tables["households"] != bulk+1 {
		t.Fatalf("export households = %d", exp.Tables["households"])
	}
	archive, _, err := f.store.GetObject(ctx, exp.Key)
	if err != nil {
		t.Fatal(err)
	}
	cov1xExec(t, f.pool, `DELETE FROM households WHERE org_id = $1::uuid`, f.a.ID)
	res, err := f.svc.Restore(ctx, f.a.ID, archive)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsRestored["households"] != bulk+1 || cov1xCount(t, f.pool, "households", f.a.ID) != bulk+1 {
		t.Fatalf("restored %d, stored %d, want %d", res.RowsRestored["households"], cov1xCount(t, f.pool, "households", f.a.ID), bulk+1)
	}
	if cov1xCount(t, f.pool, "households", f.b.ID) != 1 {
		t.Fatal("org B must be untouched")
	}
}

func TestCov1xRestoreFailsAsAWholeWhenATableCannotLoad(t *testing.T) {
	ctx := context.Background()
	t.Run("a bad row in the final chunk", func(t *testing.T) {
		f := newFixture(t)
		archive := cov1xZip(t, map[string]string{"households.ndjson": `{"id":"not-a-uuid","org_id":"` + f.a.ID + `"}` + "\n"})
		res, err := f.svc.Restore(ctx, f.a.ID, archive)
		if err == nil || !strings.Contains(err.Error(), "could not restore [households]") || res == nil {
			t.Fatalf("%+v %v", res, err)
		}
		if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusActive {
			t.Fatalf("a failed restore leaves the org alone: %s", got.Status)
		}
	})
	t.Run("a bad row inside a full chunk", func(t *testing.T) {
		f := newFixture(t)
		line := `{"id":"not-a-uuid","org_id":"` + f.a.ID + `"}`
		body := strings.Repeat(line+"\n", restoreChunkRows+5)
		archive := cov1xZip(t, map[string]string{"households.ndjson": body})
		n, err := f.svc.loadTable(ctx, "households", f.a.ID, cov1xFirstFile(t, archive))
		if err == nil || n != 0 || !strings.HasPrefix(err.Error(), "households:") {
			t.Fatalf("%d %v", n, err)
		}
		if cov1xCount(t, f.pool, "households", f.a.ID) != 1 {
			t.Fatal("a failed table load is all-or-nothing")
		}
	})
	t.Run("a table whose org column cannot match", func(t *testing.T) {
		f := newFixture(t)
		cov1xTextOrgTable(t, f.pool)
		archive := cov1xZip(t, map[string]string{"cov1x_textorg.ndjson": `{"org_id":"` + f.a.ID + `","id":1}` + "\n"})
		_, err := f.svc.Restore(ctx, f.a.ID, archive)
		if err == nil || !strings.Contains(err.Error(), "could not restore [cov1x_textorg]") {
			t.Fatal(err)
		}
	})
	t.Run("the commit is refused", func(t *testing.T) {
		f := newFixture(t)
		cov1xDeferTable(t, f.pool)
		cov1xArmDefer(t, f.pool)
		archive := cov1xZip(t, map[string]string{"cov1x_defer.ndjson": `{"org_id":"` + f.a.ID + `","id":1}` + "\n"})
		_, err := f.svc.Restore(ctx, f.a.ID, archive)
		if err == nil || !strings.Contains(err.Error(), "could not restore [cov1x_defer]") {
			t.Fatal(err)
		}
		if cov1xCount(t, f.pool, "cov1x_defer", f.a.ID) != 0 {
			t.Fatal("a refused commit must leave no rows behind")
		}
	})
}

func TestCov1xRestoreReportsFailureToSuspend(t *testing.T) {
	f := newFixture(t)
	cov1xBoom(t, f.pool)
	cov1xExec(t, f.pool, `CREATE TRIGGER cov1x_block BEFORE UPDATE ON orgs FOR EACH ROW WHEN (NEW.status = 'suspended') EXECUTE FUNCTION cov1x_boom()`)
	res, err := f.svc.Restore(context.Background(), f.a.ID, cov1xZip(t, map[string]string{"files.json": "[]"}))
	if err == nil || !strings.Contains(err.Error(), "cov1x boom") || res == nil {
		t.Fatalf("%+v %v", res, err)
	}
	if got := cov1xStatus(t, f, f.a.ID); got.Status != orgs.StatusActive {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestCov1xLoadTableErrorPaths(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	t.Run("unsupported compression", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		raw := []byte("xx")
		w, err := zw.CreateRaw(&zip.FileHeader{Name: "households.ndjson", Method: 99, CRC32: crc32.ChecksumIEEE(raw), CompressedSize64: uint64(len(raw)), UncompressedSize64: uint64(len(raw))})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(raw)
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		n, err := f.svc.loadTable(ctx, "households", f.a.ID, cov1xFirstFile(t, buf.Bytes()))
		if !errors.Is(err, zip.ErrAlgorithm) || n != 0 {
			t.Fatalf("%d %v", n, err)
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		archive := cov1xZip(t, map[string]string{"households.ndjson": "{}\n"})
		n, err := f.svc.loadTable(cctx, "households", f.a.ID, cov1xFirstFile(t, archive))
		if !errors.Is(err, context.Canceled) || n != 0 {
			t.Fatalf("%d %v", n, err)
		}
	})
	t.Run("a line longer than the scanner limit", func(t *testing.T) {
		archive := cov1xZip(t, map[string]string{"households.ndjson": strings.Repeat("a", 9<<20)})
		n, err := f.svc.loadTable(ctx, "households", f.a.ID, cov1xFirstFile(t, archive))
		if !errors.Is(err, bufio.ErrTooLong) || n != 0 {
			t.Fatalf("%d %v", n, err)
		}
	})
	t.Run("rows naming another org are dropped", func(t *testing.T) {
		line := fmt.Sprintf(`{"id":%q,"org_id":%q,"household_name":"intruder","status":"active"}`, uuid.NewString(), f.b.ID)
		archive := cov1xZip(t, map[string]string{"households.ndjson": line + "\n"})
		n, err := f.svc.loadTable(ctx, "households", f.a.ID, cov1xFirstFile(t, archive))
		if err != nil || n != 0 {
			t.Fatalf("%d %v", n, err)
		}
		if cov1xCount(t, f.pool, "households", f.b.ID) != 1 {
			t.Fatal("org B must be untouched")
		}
	})
}
