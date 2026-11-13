package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// cov1xExit is the panic value used by the fake osExit.
type cov1xExit int

// cov1xFreeAddr returns a loopback address that was free a moment ago.
func cov1xFreeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// cov1xBusyAddr returns an address that is occupied for the test's lifetime.
func cov1xBusyAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().String()
}

// cov1xBaseEnv gives every main() run a hermetic environment: in-memory mode,
// no inherited Redis/Postgres, no metrics listener unless a test opts in.
func cov1xBaseEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"APP_ENV":                      "test",
		"DATABASE_URL":                 "",
		"REDIS_URL":                    "",
		"METRICS_ADDR":                 "",
		"API_ADDR":                     cov1xFreeAddr(t),
		"OTEL_EXPORTER_OTLP_ENDPOINT":  "",
		"KEYCLOAK_REALM":               "",
		"OIDC_JWKS_URL":                "",
		"OIDC_AUTHORIZE_URL":           "",
		"OIDC_CLIENT_ID":               "",
		"OIDC_TOKEN_URL":               "",
		"WORKER_SCHEDULED_ORG_IDS":     "",
		"AUTH_ENABLE_DEV_STUB_TOKENS":  "",
		"SESSION_COOKIE_SECURE":        "",
		"AUTH_ALLOW_DEV_SESSION_ISSUE": "",
	} {
		t.Setenv(k, v)
	}
}

// cov1xCapture redirects os.Stdout/os.Stderr while fn runs and returns both.
func cov1xCapture(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	ro, wo, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	re, we, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = wo, we
	var wg sync.WaitGroup
	var outBuf, errBuf bytes.Buffer
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(&outBuf, ro) }()
	go func() { defer wg.Done(); _, _ = io.Copy(&errBuf, re) }()
	defer func() {
		os.Stdout, os.Stderr = origOut, origErr
		_ = wo.Close()
		_ = we.Close()
		wg.Wait()
		stdout, stderr = outBuf.String(), errBuf.String()
	}()
	fn()
	return
}

// cov1xRunMain runs main() in-process with the given argv. It returns the exit
// code requested through osExit, or -1 if main returned normally. When sigs is
// true SIGTERM is delivered until main returns, standing in for the platform's
// stop signal.
func cov1xRunMain(t *testing.T, sigs bool, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	origArgs, origExit := os.Args, osExit
	os.Args = append([]string{"aquify-api"}, args...)
	osExit = func(c int) { panic(cov1xExit(c)) }
	t.Cleanup(func() { os.Args, osExit = origArgs, origExit })

	// Holding our own SIGTERM subscription for the whole run means a signal that
	// arrives before main subscribes cannot kill the test binary.
	guard := make(chan os.Signal, 64)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	stop := make(chan struct{})
	var sender sync.WaitGroup
	if sigs {
		sender.Add(1)
		go func() {
			defer sender.Done()
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
				}
			}
		}()
	}

	code = -1
	stdout, stderr = cov1xCapture(t, func() {
		defer func() {
			if r := recover(); r != nil {
				c, ok := r.(cov1xExit)
				if !ok {
					panic(r)
				}
				code = int(c)
			}
		}()
		main()
	})
	close(stop)
	sender.Wait()
	return code, stdout, stderr
}

func TestCov1xCheckHealthReportsConnectionFailure(t *testing.T) {
	err := checkHealth("http://" + cov1xFreeAddr(t) + "/live")
	if err == nil {
		t.Fatal("expected a connection error for a closed port")
	}
	if strings.Contains(err.Error(), "health endpoint returned") {
		t.Fatalf("connection failure must not look like a status failure: %v", err)
	}
}

func TestCov1xCheckHealthStatusMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	err := checkHealth(srv.URL)
	if err == nil || err.Error() != "health endpoint returned 503" {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xMainHealthcheckModes(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer bad.Close()

	if code, _, _ := cov1xRunMain(t, false, "healthcheck", ok.URL); code != -1 {
		t.Fatalf("healthy probe exit = %d, want normal return", code)
	}
	if code, _, _ := cov1xRunMain(t, false, "healthcheck", bad.URL); code != 1 {
		t.Fatalf("unhealthy probe exit = %d, want 1", code)
	}
	if code, _, _ := cov1xRunMain(t, false, "healthcheck"); code != 1 {
		t.Fatalf("missing URL exit = %d, want 1", code)
	}
	if code, _, _ := cov1xRunMain(t, false, "healthcheck", ok.URL, "extra"); code != 1 {
		t.Fatalf("extra arg exit = %d, want 1", code)
	}
}

func TestCov1xMainMigrateSubcommandPropagatesExitCode(t *testing.T) {
	cov1xBaseEnv(t)
	code, _, errOut := cov1xRunMain(t, false, " MiGrAtE ", "sideways")
	if code != 2 || !strings.Contains(errOut, "usage: aquify-api migrate") {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
}

func TestCov1xMainRejectsInvalidConfigBeforeServing(t *testing.T) {
	cov1xBaseEnv(t)
	t.Setenv("APP_ENV", "prodution")
	code, out, _ := cov1xRunMain(t, false)
	if code != 1 || !strings.Contains(out, "startup config error") || !strings.Contains(out, "is invalid") {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}

func TestCov1xMainServesAndShutsDownOnSignal(t *testing.T) {
	cov1xBaseEnv(t)
	t.Setenv("METRICS_ADDR", cov1xFreeAddr(t))
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://"+cov1xFreeAddr(t))
	code, out, _ := cov1xRunMain(t, true)
	if code != -1 {
		t.Fatalf("graceful shutdown must return normally, got exit %d; out=%q", code, out)
	}
	for _, want := range []string{"api listening", "metrics listening", "shutdown signal received"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
}

func TestCov1xMainWithDurableRedisBackendAndExplicitMode(t *testing.T) {
	cov1xBaseEnv(t)
	cov1xUseFakeRedis(t)
	code, out, _ := cov1xRunMain(t, true, "API")
	if code != -1 {
		t.Fatalf("exit=%d out=%q", code, out)
	}
	if !strings.Contains(out, "api listening") {
		t.Fatalf("api did not start with the Redis backend:\n%s", out)
	}
	// Outside production the API silently falls back to an in-memory queue when Redis is
	// unusable, and still reports "api listening": make sure the durable backend was really used.
	if strings.Contains(out, "falling back to in-memory queue") {
		t.Fatalf("api fell back to the in-memory queue instead of using Redis:\n%s", out)
	}
}

func TestCov1xMainFallsBackToInMemoryQueueInDevelopment(t *testing.T) {
	cov1xBaseEnv(t)
	t.Setenv("REDIS_URL", "redis://127.0.0.1:1/0")
	code, out, _ := cov1xRunMain(t, true)
	if code != -1 || !strings.Contains(out, "falling back to in-memory queue") {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}

func TestCov1xMainFailsWhenListenAddressIsTaken(t *testing.T) {
	cov1xBaseEnv(t)
	t.Setenv("API_ADDR", cov1xBusyAddr(t))
	code, out, _ := cov1xRunMain(t, false)
	if code != 1 || !strings.Contains(out, "server failed") {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}

func TestCov1xMainLogsWhenMetricsListenerCannotBind(t *testing.T) {
	cov1xBaseEnv(t)
	t.Setenv("METRICS_ADDR", cov1xBusyAddr(t))
	code, out, _ := cov1xRunMain(t, true)
	if code != -1 {
		t.Fatalf("API must keep serving without metrics, exit=%d", code)
	}
	if !strings.Contains(out, "metrics server failed") {
		t.Fatalf("expected metrics bind failure to be logged:\n%s", out)
	}
}

func TestCov1xWorkerModeRequiresRedis(t *testing.T) {
	cov1xBaseEnv(t)
	code, out, _ := cov1xRunMain(t, false, "worker")
	if code != 1 || !strings.Contains(out, "worker mode requires REDIS_URL") {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}

func TestCov1xWorkerModeRunsAndDrainsOnSignal(t *testing.T) {
	cov1xBaseEnv(t)
	cov1xUseFakeRedis(t)
	t.Setenv("METRICS_ADDR", cov1xFreeAddr(t))
	code, out, _ := cov1xRunMain(t, true, "worker")
	if code != -1 {
		t.Fatalf("exit=%d out=%q", code, out)
	}
	for _, want := range []string{"worker metrics listening", "worker shutdown signal received"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
}

func TestCov1xWorkerMetricsBindFailureIsLogged(t *testing.T) {
	cov1xBaseEnv(t)
	cov1xUseFakeRedis(t)
	t.Setenv("METRICS_ADDR", cov1xBusyAddr(t))
	code, out, _ := cov1xRunMain(t, true, "worker")
	if code != -1 || !strings.Contains(out, "worker metrics server failed") {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}

func TestCov1xLoadDotEnvWarnsAboutMalformedFileAndIgnoresMissingOne(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.env")
	if err := os.WriteFile(bad, []byte("AQUIFY_COV1X_BAD=\"never closed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("AQUIFY_COV1X_BAD") })
	_, errOut := cov1xCapture(t, func() { loadDotEnv(bad) })
	if !strings.Contains(errOut, "warning: ignoring "+bad) {
		t.Fatalf("expected a warning for the malformed file, got %q", errOut)
	}
	if _, set := os.LookupEnv("AQUIFY_COV1X_BAD"); set {
		t.Fatal("a malformed file must not be half-applied")
	}
	_, errOut = cov1xCapture(t, func() { loadDotEnv(filepath.Join(dir, "absent.env")) })
	if errOut != "" {
		t.Fatalf("a missing file must be silent, got %q", errOut)
	}
}

// cov1xMigrationDSN creates an isolated schema in the shared database and
// returns a DSN whose search_path is confined to it.
func cov1xMigrationDSN(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if base == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	schema := "cov1x_mig_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = conn.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = conn.Close(ctx)
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func cov1xUseMigrations(t *testing.T, fsys fs.FS) {
	t.Helper()
	old := migrationSource
	migrationSource = func() fs.FS { return fsys }
	t.Cleanup(func() { migrationSource = old })
}

func TestCov1xRunMigrateUpReportsAppliedBeforeAFailingMigration(t *testing.T) {
	dsn := cov1xMigrationDSN(t)
	cov1xUseMigrations(t, fstest.MapFS{
		"00001_ok.sql":  {Data: []byte("-- +goose Up\nCREATE TABLE cov1x_ok (id int);\n-- +goose Down\nDROP TABLE cov1x_ok;\n")},
		"00002_bad.sql": {Data: []byte("-- +goose Up\nSELECT * FROM cov1x_no_such_table;\n-- +goose Down\nSELECT 1;\n")},
	})
	env := envOf(map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": dsn})
	var out, errb bytes.Buffer
	code := runMigrate(context.Background(), []string{"up"}, env, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "applied 00001_ok.sql") || strings.Contains(out.String(), "00002_bad.sql") {
		t.Fatalf("stdout should list only the migration that succeeded: %q", out.String())
	}
	if !strings.Contains(errb.String(), "migrate:") || !strings.Contains(errb.String(), "cov1x_no_such_table") {
		t.Fatalf("stderr should carry the failure: %q", errb.String())
	}
	if strings.Contains(errb.String(), "postgres://") {
		t.Fatalf("stderr must not leak the connection string: %q", errb.String())
	}

	// Status now shows one applied and one pending migration, and fails.
	out.Reset()
	errb.Reset()
	code = runMigrate(context.Background(), []string{"status"}, env, &out, &errb)
	if code != 1 {
		t.Fatalf("status exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "applied  00001_ok.sql") || !strings.Contains(out.String(), "PENDING  00002_bad.sql") {
		t.Fatalf("status listing = %q", out.String())
	}
	if !strings.Contains(errb.String(), "migrate status: 1 pending") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestCov1xRunMigrateStatusFailsWhenDatabaseUnreachable(t *testing.T) {
	cov1xUseMigrations(t, fstest.MapFS{
		"00001_ok.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")},
	})
	dsn := fmt.Sprintf("postgres://owner:cov1x-secret@%s/db?sslmode=disable", cov1xFreeAddr(t))
	env := envOf(map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": dsn})
	var out, errb bytes.Buffer
	code := runMigrate(context.Background(), []string{"status"}, env, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "migrate:") {
		t.Fatalf("exit=%d stderr=%q", code, errb.String())
	}
	if strings.Contains(errb.String(), "cov1x-secret") {
		t.Fatalf("the password must be scrubbed from errors: %q", errb.String())
	}
	if out.Len() != 0 {
		t.Fatalf("nothing should be listed on failure: %q", out.String())
	}
}
