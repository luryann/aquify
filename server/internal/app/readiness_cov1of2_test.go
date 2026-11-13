package app

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/postmark"
	"github.com/luryann/aquify/server/internal/storage"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/workers"
)

// cov1xReadyStripe is a Stripe client with a controllable Ready probe.
type cov1xReadyStripe struct {
	unavailableStripeClient
	readyErr error
}

func (c cov1xReadyStripe) Ready(context.Context) error { return c.readyErr }

type cov1xReadyPostmark struct {
	unavailablePostmarkClient
	readyErr error
}

func (c cov1xReadyPostmark) Ready(context.Context) error { return c.readyErr }

type cov1xKeycloakProbe struct {
	keycloak.Client
	err error
}

func (k cov1xKeycloakProbe) Ready(context.Context) error { return k.err }

type cov1xKeycloakNoProbe struct{ keycloak.Client }

type cov1xStorageProbe struct {
	storage.ObjectStorage
	err error
}

func (s cov1xStorageProbe) Ready(context.Context) error { return s.err }

type cov1xFakeBackend struct{ workers.Backend }

func cov1xReadinessChecks(t *testing.T, cfg Config, pool *pgxpool.Pool, store storage.ObjectStorage, sc stripelib.Client, pm postmark.Client, wb workers.Backend, kc keycloak.Client) map[string]string {
	t.Helper()
	return buildReadinessChecker(cfg, pool, store, sc, pm, wb, kc).CheckReadiness(context.Background()).Checks
}

func TestCov1xAppReadinessCheckerDefaultsBlankStatusAndReportsFailures(t *testing.T) {
	t.Parallel()
	checker := appReadinessChecker{checks: []dependencyReadinessCheck{
		{name: "blank", check: func(context.Context) (string, error) { return "  ", nil }},
		{name: "custom", check: func(context.Context) (string, error) { return "ready (custom)", nil }},
		{name: "bad", check: func(context.Context) (string, error) { return "", errors.New("boom") }},
	}}
	report := checker.CheckReadiness(context.Background())
	if report.Ready {
		t.Fatal("report must not be ready when a check fails")
	}
	want := map[string]string{"blank": "ready", "custom": "ready (custom)", "bad": "not_ready: boom"}
	for k, v := range want {
		if report.Checks[k] != v {
			t.Fatalf("check %s = %q, want %q", k, report.Checks[k], v)
		}
	}
}

func TestCov1xReadinessDatabase(t *testing.T) {
	cfg := Config{DatabaseURL: "postgres://x"}
	if got := cov1xReadinessChecks(t, cfg, nil, nil, nil, nil, nil, nil)["database"]; got != "not_ready: database pool unavailable" {
		t.Fatalf("nil pool: %q", got)
	}
	live := openTestPool(t)
	if got := cov1xReadinessChecks(t, cfg, live, nil, nil, nil, nil, nil)["database"]; got != "ready" {
		t.Fatalf("live pool: %q", got)
	}
	closed := openTestPool(t)
	closed.Close()
	if got := cov1xReadinessChecks(t, cfg, closed, nil, nil, nil, nil, nil)["database"]; got == "ready" || got == "" {
		t.Fatalf("closed pool should be not ready, got %q", got)
	}
}

func TestCov1xReadinessQueue(t *testing.T) {
	t.Parallel()
	// Redis not configured: in-memory server is ready, an arbitrary backend is plain ready.
	if got := cov1xReadinessChecks(t, Config{}, nil, nil, nil, nil, workers.NewServer(workers.ServerConfig{}), nil)["queue"]; got != "ready (in-memory)" {
		t.Fatalf("in-memory: %q", got)
	}
	if got := cov1xReadinessChecks(t, Config{}, nil, nil, nil, nil, cov1xFakeBackend{}, nil)["queue"]; got != "ready" {
		t.Fatalf("custom backend: %q", got)
	}
	// Redis configured but no backend / wrong backend / unreachable client.
	cfg := Config{Worker: WorkerConfig{RedisURL: "redis://configured"}}
	if got := cov1xReadinessChecks(t, cfg, nil, nil, nil, nil, nil, nil)["queue"]; got != "not_ready: redis backend not initialized" {
		t.Fatalf("nil backend: %q", got)
	}
	if got := cov1xReadinessChecks(t, cfg, nil, nil, nil, nil, cov1xFakeBackend{}, nil)["queue"]; got != "not_ready: durable redis backend not active" {
		t.Fatalf("wrong backend: %q", got)
	}
	if got := cov1xReadinessChecks(t, cfg, nil, nil, nil, nil, &workers.RedisBackend{}, nil)["queue"]; got != "not_ready: redis client unavailable" {
		t.Fatalf("zero backend: %q", got)
	}
}

func TestCov1xReadinessQueueLiveRedis(t *testing.T) {
	url := cov1xRedisURL(t)
	backend, err := workers.NewRedisBackend(workers.RedisBackendConfig{RedisURL: url})
	if err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	cfg := Config{Worker: WorkerConfig{RedisURL: url}}
	if got := cov1xReadinessChecks(t, cfg, nil, nil, nil, nil, backend, nil)["queue"]; got != "ready" {
		t.Fatalf("live redis: %q", got)
	}
}

func TestCov1xReadinessStorage(t *testing.T) {
	t.Parallel()
	configured := Config{Storage: StorageConfig{Endpoint: "s3.example.test", AccessKey: "a", SecretKey: "b", Bucket: "c"}}
	cases := []struct {
		name  string
		cfg   Config
		store storage.ObjectStorage
		want  string
	}{
		{"nil", Config{}, nil, "not_ready: storage client unavailable"},
		{"stub unconfigured", Config{}, storage.NewStubStorage(""), "ready (stub)"},
		{"stub while configured", configured, storage.NewStubStorage(""), "not_ready: stub storage active while object storage is configured"},
		{"real but unconfigured", Config{}, fakeObjectStorage{}, "disabled"},
		{"no probe", configured, fakeObjectStorage{}, "not_ready: storage readiness probe unavailable"},
		{"probe ok", configured, cov1xStorageProbe{}, "ready"},
		{"probe error", configured, cov1xStorageProbe{err: errors.New("bucket gone")}, "not_ready: bucket gone"},
	}
	for _, tc := range cases {
		if got := cov1xReadinessChecks(t, tc.cfg, nil, tc.store, nil, nil, nil, nil)["storage"]; got != tc.want {
			t.Errorf("%s: storage = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCov1xReadinessStripeAndPostmark(t *testing.T) {
	t.Parallel()
	stripeCfg := func(env string) Config {
		return Config{AppEnv: env, Stripe: StripeConfig{SecretKey: "sk_test_fake"}}
	}
	pmCfg := func(env string) Config {
		return Config{AppEnv: env, Postmark: PostmarkConfig{ServerToken: "tok", FromEmail: "a@example.test"}}
	}
	failure := errors.New("provider down")
	stripeCases := []struct {
		name   string
		cfg    Config
		client stripelib.Client
		want   string
	}{
		{"disabled", Config{}, nil, "disabled"},
		{"nil client", stripeCfg("test"), nil, "not_ready: stripe client unavailable"},
		{"unavailable", stripeCfg("production"), unavailableStripeClient{err: failure}, "not_ready: provider down"},
		{"stub dev", stripeCfg("development"), stripelib.NewStubClient(), "ready (stub)"},
		{"stub strict", stripeCfg("production"), stripelib.NewStubClient(), "not_ready: stub stripe client active in strict environment"},
		{"probe ok", stripeCfg("production"), cov1xReadyStripe{}, "ready"},
		{"probe fails", stripeCfg("production"), cov1xReadyStripe{readyErr: failure}, "not_ready: provider down"},
	}
	for _, tc := range stripeCases {
		if got := cov1xReadinessChecks(t, tc.cfg, nil, nil, tc.client, nil, nil, nil)["stripe"]; got != tc.want {
			t.Errorf("stripe %s = %q, want %q", tc.name, got, tc.want)
		}
	}
	pmCases := []struct {
		name   string
		cfg    Config
		client postmark.Client
		want   string
	}{
		{"disabled", Config{}, nil, "disabled"},
		{"nil client", pmCfg("test"), nil, "not_ready: postmark client unavailable"},
		{"unavailable", pmCfg("production"), unavailablePostmarkClient{err: failure}, "not_ready: provider down"},
		{"stub dev", pmCfg("local"), postmark.NewStubClient(), "ready (stub)"},
		{"stub strict", pmCfg("staging"), postmark.NewStubClient(), "not_ready: stub postmark client active in strict environment"},
		{"probe ok", pmCfg("production"), cov1xReadyPostmark{}, "ready"},
		{"probe fails", pmCfg("production"), cov1xReadyPostmark{readyErr: failure}, "not_ready: provider down"},
	}
	for _, tc := range pmCases {
		if got := cov1xReadinessChecks(t, tc.cfg, nil, nil, nil, tc.client, nil, nil)["postmark"]; got != tc.want {
			t.Errorf("postmark %s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCov1xReadinessKeycloak(t *testing.T) {
	t.Parallel()
	cfgFor := func(env string) Config {
		return Config{AppEnv: env, Keycloak: KeycloakAdminConfig{AdminURL: "https://kc.example.test", Realm: "r", ClientID: "c", ClientSecret: "s"}}
	}
	cases := []struct {
		name   string
		cfg    Config
		client keycloak.Client
		want   string
	}{
		{"disabled", Config{}, nil, "disabled"},
		{"nil client", cfgFor("test"), nil, "not_ready: keycloak admin client unavailable"},
		{"stub dev", cfgFor("development"), keycloak.NewStubClient(), "ready (stub)"},
		{"stub strict", cfgFor("production"), keycloak.NewStubClient(), "not_ready: stub keycloak admin client active in strict environment"},
		{"probe ok", cfgFor("production"), cov1xKeycloakProbe{}, "ready"},
		{"probe fails", cfgFor("production"), cov1xKeycloakProbe{err: errors.New("realm down")}, "not_ready: realm down"},
		{"no probe", cfgFor("production"), cov1xKeycloakNoProbe{}, "not_ready: keycloak readiness probe unavailable"},
	}
	for _, tc := range cases {
		if got := cov1xReadinessChecks(t, tc.cfg, nil, nil, nil, nil, nil, tc.client)["keycloak"]; got != tc.want {
			t.Errorf("keycloak %s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// cov1xRedisURL returns REDIS_URL or skips, following the repo convention of
// only running Redis-backed tests when a broker is configured.
func cov1xRedisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set; skipping Redis-backed test")
	}
	return url
}

func TestCov1xProbeClientRequiresReadyProbe(t *testing.T) {
	t.Parallel()
	// probeClient takes an untyped client; a value without Ready cannot be probed.
	status, err := probeClient(context.Background(), "widget", struct{}{}, false, true)
	if err == nil || err.Error() != "widget readiness probe unavailable" || status != "" {
		t.Fatalf("got %q, %v", status, err)
	}
	status, err = probeClient(context.Background(), "widget", cov1xReadyPostmark{}, false, true)
	if err != nil || status != "ready" {
		t.Fatalf("got %q, %v", status, err)
	}
}
