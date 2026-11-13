package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xUseDBPool makes openValidatedDBPool hand out pool for the test, the same
// way production hands out the pool opened from DATABASE_URL.
func cov1xUseDBPool(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	origOpen, origClose := openDBPool, closeDBPool
	t.Cleanup(func() { openDBPool, closeDBPool = origOpen, origClose })
	openDBPool = func(context.Context, string) (*pgxpool.Pool, error) { return pool, nil }
	closeDBPool = func(*pgxpool.Pool) {}
}

func TestCov1xRouterWithDatabasePoolServesReadiness(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cov1xUseDBPool(t, pool)
	router := NewRouterWithServices(Config{AppEnv: "test", DatabaseURL: "postgres://fixture.invalid/aquify", Auth: AuthConfig{EnableDevStubTokens: true}},
		slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ready: %d %s", rr.Code, rr.Body.String())
	}
}
