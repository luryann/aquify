package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/dareaquatics/aquify/server/internal/postmark"
	"github.com/dareaquatics/aquify/server/internal/storage"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/jackc/pgx/v5/pgxpool"
)

type readinessStripeClient struct {
	*stripelib.StubClient
	err error
}

func (c readinessStripeClient) Ready(_ context.Context) error {
	return c.err
}

type readinessPostmarkClient struct {
	*postmark.StubClient
	err error
}

func (c readinessPostmarkClient) Ready(_ context.Context) error {
	return c.err
}

type readinessKeycloakClient struct {
	*keycloak.StubClient
	err error
}

func (c readinessKeycloakClient) Ready(_ context.Context) error {
	return c.err
}

func TestHealthReadyReportsActualLocalDependencies(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}

	if payload.Status != "ready" {
		t.Fatalf("status = %q, want ready", payload.Status)
	}
	if payload.Checks["database"] != "disabled" {
		t.Fatalf("database = %q, want disabled", payload.Checks["database"])
	}
	if payload.Checks["queue"] != "ready (in-memory)" {
		t.Fatalf("queue = %q, want ready (in-memory)", payload.Checks["queue"])
	}
	if payload.Checks["storage"] != "ready (stub)" {
		t.Fatalf("storage = %q, want ready (stub)", payload.Checks["storage"])
	}
}

func TestHealthReadyReturns503WhenStripeProbeFails(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServices(
		Config{
			AppEnv: "production",
			Stripe: StripeConfig{SecretKey: "sk_live_example"},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, nil, nil, nil, nil, nil, nil,
		readinessStripeClient{StubClient: stripelib.NewStubClient(), err: errors.New("stripe probe failed")},
		nil, nil, nil, nil, nil, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertReadinessCheckContains(t, rr.Body.Bytes(), "stripe", "stripe probe failed")
}

func TestHealthReadyReportsReadyWhenStripeProbeSucceeds(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServices(
		Config{
			AppEnv: "production",
			Stripe: StripeConfig{SecretKey: "sk_live_example"},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, nil, nil, nil, nil, nil, nil,
		readinessStripeClient{StubClient: stripelib.NewStubClient()},
		nil, nil, nil, nil, nil, nil, nil)

	assertReadinessCheckEquals(t, serveReady(t, router), "stripe", "ready")
}

func TestHealthReadyReturns503WhenPostmarkProbeFails(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServices(
		Config{
			AppEnv: "production",
			Postmark: PostmarkConfig{
				ServerToken: "pm_live_example",
				FromEmail:   "noreply@example.com",
			},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, nil, nil, nil, nil, nil, nil, nil,
		comms.NewServiceWithClient(readinessPostmarkClient{StubClient: postmark.NewStubClient(), err: errors.New("postmark probe failed")}),
		nil, nil, nil, nil, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertReadinessCheckContains(t, rr.Body.Bytes(), "postmark", "postmark probe failed")
}

func TestHealthReadyReportsReadyWhenPostmarkProbeSucceeds(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServices(
		Config{
			AppEnv: "production",
			Postmark: PostmarkConfig{
				ServerToken: "pm_live_example",
				FromEmail:   "noreply@example.com",
			},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, nil, nil, nil, nil, nil, nil, nil,
		comms.NewServiceWithClient(readinessPostmarkClient{StubClient: postmark.NewStubClient()}),
		nil, nil, nil, nil, nil, nil)

	assertReadinessCheckEquals(t, serveReady(t, router), "postmark", "ready")
}

func TestHealthReadyReturns503WhenKeycloakProbeFails(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServices(
		Config{
			AppEnv: "production",
			Keycloak: KeycloakAdminConfig{
				AdminURL:     "https://keycloak.example.com",
				Realm:        "aquify",
				ClientID:     "admin-cli",
				ClientSecret: "secret",
			},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		readinessKeycloakClient{StubClient: keycloak.NewStubClient(), err: errors.New("keycloak probe failed")})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertReadinessCheckContains(t, rr.Body.Bytes(), "keycloak", "keycloak probe failed")
}

func TestHealthReadyReportsReadyWhenKeycloakProbeSucceeds(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServices(
		Config{
			AppEnv: "production",
			Keycloak: KeycloakAdminConfig{
				AdminURL:     "https://keycloak.example.com",
				Realm:        "aquify",
				ClientID:     "admin-cli",
				ClientSecret: "secret",
			},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		readinessKeycloakClient{StubClient: keycloak.NewStubClient()})

	assertReadinessCheckEquals(t, serveReady(t, router), "keycloak", "ready")
}

func TestHealthReadyReturns503WhenDatabasePoolUnavailable(t *testing.T) {
	prevOpenDBPool := openDBPool
	openDBPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return nil, errors.New("dial failed")
	}
	t.Cleanup(func() {
		openDBPool = prevOpenDBPool
	})

	router := NewRouter(Config{
		AppEnv:      "test",
		DatabaseURL: "postgres://example.invalid/aquify",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertReadinessCheckContains(t, rr.Body.Bytes(), "database", "database pool unavailable")
}

func TestHealthReadyReturns503WhenRedisConfiguredWithoutDurableBackend(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{
		AppEnv: "test",
		Worker: WorkerConfig{
			RedisURL: "redis://example.invalid:6379/0",
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertReadinessCheckContains(t, rr.Body.Bytes(), "queue", "durable redis backend not active")
}

func TestHealthReadyReturns503WhenConfiguredStorageUsesStub(t *testing.T) {
	prevNewObjectStorageClient := newObjectStorageClient
	newObjectStorageClient = func(cfg storage.Config) (storage.ObjectStorage, error) {
		return storage.NewStubStorage(cfg.BaseURL()), nil
	}
	t.Cleanup(func() {
		newObjectStorageClient = prevNewObjectStorageClient
	})

	router := NewRouter(Config{
		AppEnv: "test",
		Storage: StorageConfig{
			Endpoint:  "minio.example.invalid:9000",
			AccessKey: "minio",
			SecretKey: "miniosecret",
			Bucket:    "aquify",
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
	assertReadinessCheckContains(t, rr.Body.Bytes(), "storage", "stub storage active")
}

func assertReadinessCheckContains(t *testing.T, body []byte, name, want string) {
	t.Helper()

	var payload struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if payload.Status != "not_ready" {
		t.Fatalf("status = %q, want not_ready", payload.Status)
	}
	if !strings.Contains(payload.Checks[name], want) {
		t.Fatalf("%s = %q, want substring %q", name, payload.Checks[name], want)
	}
}

func assertReadinessCheckEquals(t *testing.T, body []byte, name, want string) {
	t.Helper()

	var payload struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if payload.Status != "ready" {
		t.Fatalf("status = %q, want ready", payload.Status)
	}
	if payload.Checks[name] != want {
		t.Fatalf("%s = %q, want %q", name, payload.Checks[name], want)
	}
}

func serveReady(t *testing.T, router http.Handler) []byte {
	t.Helper()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	return rr.Body.Bytes()
}
