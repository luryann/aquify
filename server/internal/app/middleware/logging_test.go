package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestStructuredLoggerDoesNotLogPathSecrets(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(StructuredLogger(logger))
	r.Get("/api/v1/calendar/feed/{token}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/feed/SECRET123.ics", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if strings.Contains(out, "SECRET123") {
		t.Fatalf("feed token leaked into access log: %s", out)
	}
	if !strings.Contains(out, "/api/v1/calendar/feed/{token}") {
		t.Fatalf("expected route pattern in access log: %s", out)
	}
}
