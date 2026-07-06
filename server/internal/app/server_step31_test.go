package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpenValidatedDBPoolReturnsNilWhenSchemaValidationFails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	originalOpenDBPool := openDBPool
	originalValidateDBSchema := validateDBSchema
	originalCloseDBPool := closeDBPool
	t.Cleanup(func() {
		openDBPool = originalOpenDBPool
		validateDBSchema = originalValidateDBSchema
		closeDBPool = originalCloseDBPool
	})

	fakePool := &pgxpool.Pool{}
	openDBPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return fakePool, nil
	}
	validateDBSchema = func(_ context.Context, pool *pgxpool.Pool) error {
		if pool != fakePool {
			t.Fatalf("validateDBSchema received unexpected pool pointer")
		}
		return errors.New("schema drift detected")
	}

	closed := false
	closeDBPool = func(pool *pgxpool.Pool) {
		if pool != fakePool {
			t.Fatalf("closeDBPool received unexpected pool pointer")
		}
		closed = true
	}

	got := openValidatedDBPool(context.Background(), Config{DatabaseURL: "postgres://example.invalid/aquify"}, logger)
	if got != nil {
		t.Fatal("expected nil pool when schema validation fails")
	}
	if !closed {
		t.Fatal("expected invalid DB pool to be closed")
	}
	if !strings.Contains(logs.String(), "DB-backed services disabled") {
		t.Fatalf("expected fallback log message, got %q", logs.String())
	}
}

func TestNewRouterWithServicesFallsBackToInMemoryWhenSchemaValidationFails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	originalOpenDBPool := openDBPool
	originalValidateDBSchema := validateDBSchema
	originalCloseDBPool := closeDBPool
	t.Cleanup(func() {
		openDBPool = originalOpenDBPool
		validateDBSchema = originalValidateDBSchema
		closeDBPool = originalCloseDBPool
	})

	openDBPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	validateDBSchema = func(_ context.Context, _ *pgxpool.Pool) error {
		return errors.New("schema drift detected")
	}
	closeDBPool = func(_ *pgxpool.Pool) {}

	router := NewRouterWithServices(
		Config{
			DatabaseURL: "postgres://example.invalid/aquify",
			Auth: AuthConfig{
				EnableDevStubTokens:  true,
				AllowDevSessionIssue: true,
			},
		},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	reqBody, err := json.Marshal(map[string]any{
		"user_id": "user-step31",
		"org_id":  "org-step31",
		"roles":   []string{"admin"},
	})
	if err != nil {
		t.Fatalf("marshal session issue request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected in-memory startup fallback to keep router usable, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(logs.String(), "DB-backed services disabled") {
		t.Fatalf("expected schema fallback log message, got %q", logs.String())
	}
}

func TestNewRouterWithServicesTreatsTypedNilWorkerBackendAsMissing(t *testing.T) {
	t.Parallel()

	var durableBackend *workers.RedisBackend

	router := NewRouterWithServices(
		Config{
			AppEnv: "test",
			Auth: AuthConfig{
				EnableDevStubTokens:  true,
				AllowDevSessionIssue: true,
			},
		},
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, durableBackend, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected typed-nil worker backend to fall back to in-memory queue, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"queue":"ready (in-memory)"`) {
		t.Fatalf("expected in-memory queue readiness, got %s", rr.Body.String())
	}
}
