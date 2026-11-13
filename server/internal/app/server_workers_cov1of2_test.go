package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/postmark"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/luryann/aquify/server/internal/workers"
	"github.com/luryann/aquify/server/internal/workers/jobhandlers"
)

func TestCov1xAllowedCORSHeaders(t *testing.T) {
	t.Parallel()
	base := []string{"Accept", "Authorization", "Content-Type", "X-Org-Id", "X-Request-Id"}
	cases := []struct {
		name string
		csrf string
		want []string
	}{
		{"default csrf header is appended", "", append(slices.Clone(base), "X-CSRF-Token")},
		{"custom csrf header is appended", " X-Custom-Csrf ", append(slices.Clone(base), "X-Custom-Csrf")},
		{"header already allowed is not duplicated", "x-request-id", base},
	}
	for _, tc := range cases {
		cfg := Config{}
		cfg.Security.CSRFHeaderName = tc.csrf
		if got := allowedCORSHeaders(cfg); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

type cov1xHandlerRecorder struct {
	mu    sync.Mutex
	types []string
}

func (r *cov1xHandlerRecorder) RegisterHandler(taskType string, _ workers.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types = append(r.types, taskType)
}

func TestCov1xRegisterWorkerHandlers(t *testing.T) {
	t.Parallel()
	registerWorkerHandlers(nil, slog.Default(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, 0) // nil registrar is a no-op

	rec := &cov1xHandlerRecorder{}
	registerWorkerHandlers(rec, slog.Default(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, 0)
	want := []string{
		workers.TypeGenerateExport, workers.TypeBillingReconcile, workers.TypeNightlyIntegrity, workers.TypeBillingRecurringRun,
		workers.TypeBillingLateFees, workers.TypePurgeExpiredFiles, workers.TypeProcessDataExport, workers.TypeProcessDataDeletion,
	}
	if !slices.Equal(rec.types, want) {
		t.Fatalf("registered %v, want %v", rec.types, want)
	}
}

func TestCov1xBuildWorkersDefaultsLoggerAndRegistersHandlers(t *testing.T) {
	t.Parallel()
	cfg := Config{AppEnv: "test"}
	cfg.Worker.ReconcileInterval = "1h"
	ws, sched := BuildWorkersWithMetrics(cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if ws == nil || sched == nil {
		t.Fatal("expected a worker server and scheduler")
	}
}

func TestCov1xRegisterDurableScheduledJobsIgnoresNilBackend(t *testing.T) {
	t.Parallel()
	// A nil backend (no Redis) must be tolerated rather than dereferenced.
	RegisterDurableScheduledJobs(nil, Config{}, nil)
}

type cov1xJobRecorder struct{ jobs []workers.ScheduledJob }

func (r *cov1xJobRecorder) Register(j workers.ScheduledJob) { r.jobs = append(r.jobs, j) }

func TestCov1xScheduledJobFanOutPropagatesListerErrorsAndSkipsDisabledJobs(t *testing.T) {
	t.Parallel()
	cfg := Config{AppEnv: "production"}
	cfg.Worker.ReconcileInterval = "1h"
	cfg.Worker.IntegrityInterval = "not-a-duration" // invalid: the job is disabled
	boom := errors.New("registry unavailable")
	rec := &cov1xJobRecorder{}
	registerScheduledJobs(rec, cfg, func(context.Context) ([]string, error) { return nil, boom })
	if len(rec.jobs) != 1 || rec.jobs[0].Name != "billing-reconcile" {
		t.Fatalf("only the reconcile job should be registered, got %+v", rec.jobs)
	}
	if _, err := rec.jobs[0].FanOut(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("fan-out must surface the lister error, got %v", err)
	}
}

func TestCov1xScheduledOrgIDsTrimsAndDeduplicates(t *testing.T) {
	t.Parallel()
	cfg := Config{AppEnv: "production"}
	cfg.Worker.ScheduledOrgIDs = []string{" org-a ", "", "org-b", "org-a", "   "}
	if got := scheduledOrgIDs(cfg); !slices.Equal(got, []string{"org-a", "org-b"}) {
		t.Fatalf("got %v", got)
	}
	// No override in production means no orgs; development falls back to the demo org.
	if got := scheduledOrgIDs(Config{AppEnv: "production"}); len(got) != 0 {
		t.Fatalf("production must not invent orgs: %v", got)
	}
	if got := scheduledOrgIDs(Config{AppEnv: "development"}); !slices.Equal(got, []string{"org-demo"}) {
		t.Fatalf("development fallback: %v", got)
	}
}

func TestCov1xOrgListerFor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := orgs.NewService()

	override := Config{AppEnv: "production"}
	override.Worker.ScheduledOrgIDs = []string{"org-fixed"}
	if ids, err := OrgListerFor(override, registry)(ctx); err != nil || !slices.Equal(ids, []string{"org-fixed"}) {
		t.Fatalf("operator override: %v %v", ids, err)
	}

	// Empty registry: development falls back to the demo org, production gets nothing.
	if ids, err := OrgListerFor(Config{AppEnv: "development"}, registry)(ctx); err != nil || !slices.Equal(ids, []string{"org-demo"}) {
		t.Fatalf("development fallback: %v %v", ids, err)
	}
	if ids, err := OrgListerFor(Config{AppEnv: "production"}, registry)(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("production empty registry: %v %v", ids, err)
	}

	o, err := registry.Create(ctx, orgs.CreateInput{Name: "Active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.SetStatus(ctx, o.ID, orgs.StatusActive); err != nil {
		t.Fatal(err)
	}
	if ids, err := OrgListerFor(Config{AppEnv: "development"}, registry)(ctx); err != nil || !slices.Equal(ids, []string{o.ID}) {
		t.Fatalf("active org must win over the fallback: %v %v", ids, err)
	}
}

func TestCov1xOpenOrgRegistry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// No database: an in-memory registry and a harmless close func.
	reg, closeFn := OpenOrgRegistry(Config{AppEnv: "test"}, logger)
	if _, err := reg.Create(context.Background(), orgs.CreateInput{Name: "Mem"}); err != nil {
		t.Fatal(err)
	}
	closeFn()

	// With a database the registry is Postgres-backed and close releases the pool.
	pool := testutil.IsolatedSchemaPool(t)
	cov1xUseDBPool(t, pool)
	closed := 0
	closeDBPool = func(*pgxpool.Pool) { closed++ }
	reg, closeFn = OpenOrgRegistry(Config{AppEnv: "test", DatabaseURL: "postgres://fixture.invalid/aquify"}, logger)
	o, err := reg.Create(context.Background(), orgs.CreateInput{Slug: "cov1x-db", Name: "DB club"})
	if err != nil {
		t.Fatal(err)
	}
	var persisted string
	if err := pool.QueryRow(context.Background(), `SELECT name FROM orgs WHERE id = $1::uuid`, o.ID).Scan(&persisted); err != nil || persisted != "DB club" {
		t.Fatalf("org not persisted: %q %v", persisted, err)
	}
	closeFn()
	if closed != 1 {
		t.Fatalf("close func must release the pool exactly once, closed %d", closed)
	}
}

func TestCov1xOpenValidatedDBPoolStrictFailuresExit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	origExit, origSchema := exitOnStartupFailure, validateDBSchema
	t.Cleanup(func() { exitOnStartupFailure, validateDBSchema = origExit, origSchema })
	var exits []string
	exitOnStartupFailure = func(_ *slog.Logger, msg string, _ error) { exits = append(exits, msg) }
	cfg := Config{AppEnv: "production", DatabaseURL: "postgres://fixture.invalid/aquify"}

	// A schema that fails validation is fatal in production, and the pool is released first.
	pool := testutil.IsolatedSchemaPool(t)
	cov1xUseDBPool(t, pool)
	closed := 0
	closeDBPool = func(*pgxpool.Pool) { closed++ }
	validateDBSchema = func(context.Context, *pgxpool.Pool) error { return errors.New("schema drift") }
	if got := openValidatedDBPool(context.Background(), cfg, logger); got != nil {
		t.Fatal("a failed validation must not return a pool")
	}
	if len(exits) != 1 || !strings.Contains(exits[0], "schema validation failed") || closed != 1 {
		t.Fatalf("exits=%v closed=%d", exits, closed)
	}

	// The real role check refuses a superuser connection (the test database user is one).
	exits, closed = nil, 0
	validateDBSchema = func(context.Context, *pgxpool.Pool) error { return nil }
	var isSuper bool
	if err := pool.QueryRow(context.Background(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&isSuper); err != nil || !isSuper {
		t.Skip("the test database role is not privileged, so the default role check would pass")
	}
	if got := openValidatedDBPool(context.Background(), cfg, logger); got != nil {
		t.Fatal("a privileged role must not return a pool")
	}
	if len(exits) != 1 || !strings.Contains(exits[0], "role check failed") || closed != 1 {
		t.Fatalf("exits=%v closed=%d", exits, closed)
	}
}

func TestCov1xDefaultPoolSeamsAgainstRealDatabase(t *testing.T) {
	// With no seams replaced, a strict environment connects to DATABASE_URL, runs the
	// real schema and role checks, and closes the pool it refused.
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	origExit := exitOnStartupFailure
	t.Cleanup(func() { exitOnStartupFailure = origExit })
	var exits []string
	exitOnStartupFailure = func(_ *slog.Logger, msg string, _ error) { exits = append(exits, msg) }
	cfg := Config{AppEnv: "staging", DatabaseURL: url}
	got := openValidatedDBPool(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got != nil {
		got.Close()
	}
	if len(exits) != 1 {
		t.Skipf("the test database role is unprivileged or the schema is not migrated; exits=%v", exits)
	}
	if !strings.Contains(exits[0], "role check failed") && !strings.Contains(exits[0], "schema validation failed") {
		t.Fatalf("unexpected exit reason %q", exits[0])
	}
}

func TestCov1xResolveProviderClients(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	injected := keycloak.NewStubClient()
	if got := resolveKeycloakClient(Config{}, logger, injected); got != keycloak.Client(injected) {
		t.Fatal("an injected client must win")
	}
	cfg := Config{AppEnv: "production", Keycloak: KeycloakAdminConfig{AdminURL: "https://kc.example.test", Realm: "r", ClientID: "c", ClientSecret: "s"}}
	if _, ok := resolveKeycloakClient(cfg, logger, nil).(*keycloak.AdminClient); !ok {
		t.Fatal("complete config must build the real admin client")
	}
	if _, ok := resolveKeycloakClient(Config{AppEnv: "development"}, logger, nil).(*keycloak.StubClient); !ok {
		t.Fatal("development without config falls back to the stub")
	}
	if _, ok := resolveKeycloakClient(Config{AppEnv: "production"}, logger, nil).(*keycloak.UnavailableClient); !ok {
		t.Fatal("production without config must fail closed")
	}

	pm := Config{AppEnv: "production", Postmark: PostmarkConfig{ServerToken: "tok", FromEmail: "club@example.test", FromName: "Club"}}
	if _, ok := resolvePostmarkClient(pm, logger, "production").(*postmark.HTTPClient); !ok {
		t.Fatal("complete config must build the real Postmark client")
	}
	if _, ok := resolvePostmarkClient(Config{}, logger, "development").(*postmark.StubClient); !ok {
		t.Fatal("development without config falls back to the stub")
	}
	if _, ok := resolvePostmarkClient(Config{}, logger, "production").(unavailablePostmarkClient); !ok {
		t.Fatal("production without config must fail closed")
	}
}

func TestCov1xRunHouseholdErasersStopsAtFirstFailure(t *testing.T) {
	householdErasersMu.Lock()
	prev := householdErasers
	householdErasersMu.Unlock()
	t.Cleanup(func() { setHouseholdErasers(prev...) })

	var calls []string
	boom := errors.New("eraser failed")
	setHouseholdErasers(
		func(_ context.Context, org, hh string) error { calls = append(calls, "first:"+org+"/"+hh); return nil },
		func(context.Context, string, string) error { calls = append(calls, "second"); return boom },
		func(context.Context, string, string) error { calls = append(calls, "third"); return nil },
	)
	err := runHouseholdErasers(context.Background(), "org-1", "hh-1")
	if !errors.Is(err, boom) {
		t.Fatalf("expected the second eraser's error, got %v", err)
	}
	if !slices.Equal(calls, []string{"first:org-1/hh-1", "second"}) {
		t.Fatalf("erasers must run in order and stop at the first failure: %v", calls)
	}

	setHouseholdErasers([]jobhandlers.HouseholdEraser{}...)
	if err := runHouseholdErasers(context.Background(), "org-1", "hh-1"); err != nil {
		t.Fatalf("no erasers is not an error: %v", err)
	}
}
