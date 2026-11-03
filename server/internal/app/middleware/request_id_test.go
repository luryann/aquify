package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRequestIDIsOpaque(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = chimiddleware.GetReqID(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if len(seen) != 24 || strings.ContainsAny(seen, "/.") || rec.Header().Get("X-Request-Id") != seen {
		t.Fatalf("unexpected id %q header %q", seen, rec.Header().Get("X-Request-Id"))
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "bad id/with spaces")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen == "bad id/with spaces" {
		t.Fatal("unsafe incoming id honoured")
	}
	req.Header.Set("X-Request-Id", "client-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "client-123" {
		t.Fatalf("expected honoured id, got %q", seen)
	}
}
