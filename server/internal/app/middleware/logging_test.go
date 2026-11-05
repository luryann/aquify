package middleware

import (
	"bytes"
	"errors"
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

type loggingStubResolver struct{}

func (loggingStubResolver) ResolvePrincipal(string) (*Principal, bool) {
	return &Principal{UserID: "user-9", OrgID: "org-9", Roles: []string{"admin"}}, true
}

func TestStructuredLoggerRecordsPrincipalFromAuth(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(StructuredLogger(logger), Auth(loggingStubResolver{}))
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer t")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if !strings.Contains(out, "user_id=user-9") || !strings.Contains(out, "org_id=org-9") {
		t.Fatalf("access log missing principal fields: %s", out)
	}
}

type unavailableResolver struct{}

func (unavailableResolver) ResolvePrincipal(string) (*Principal, bool) { return nil, false }
func (unavailableResolver) ResolvePrincipalErr(string) (*Principal, error) {
	return nil, errors.New("session store unavailable")
}

func TestAuthAnswers503WhenSessionStoreUnavailable(t *testing.T) {
	t.Parallel()

	h := Auth(unavailableResolver{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("want 503 with Retry-After, got %d %v", rec.Code, rec.Header())
	}
}
