package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/telemetry"
)

func TestPrometheusMetricsRouteDisabledByDefault(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServicesAndMetrics(
		Config{AppEnv: "test"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		telemetry.NewMetrics(),
		false,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected /metrics to be disabled, got %d", rr.Code)
	}
}

func TestPrometheusMetricsRouteCanBeMountedExplicitly(t *testing.T) {
	t.Parallel()

	router := NewRouterWithServicesAndMetrics(
		Config{AppEnv: "test"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		telemetry.NewMetrics(),
		true,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected /metrics to be enabled, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "# HELP go_goroutines") {
		t.Fatalf("expected Prometheus metrics payload, got %q", rr.Body.String())
	}
}
