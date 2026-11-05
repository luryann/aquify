package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeChecker struct {
	active bool
	err    error
}

func (f fakeChecker) IsActive(context.Context, string) (bool, error) { return f.active, f.err }

func runGate(t *testing.T, c OrgStatusChecker) int {
	t.Helper()
	h := RequireActiveOrg(c)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r = r.WithContext(context.WithValue(r.Context(), orgContextKey{}, "org-1"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr.Code
}

func TestRequireActiveOrg(t *testing.T) {
	if got := runGate(t, fakeChecker{active: true}); got != http.StatusNoContent {
		t.Fatalf("active org: %d", got)
	}
	if got := runGate(t, fakeChecker{active: false}); got != http.StatusForbidden {
		t.Fatalf("inactive org: %d", got)
	}
	if got := runGate(t, fakeChecker{err: errors.New("db down")}); got != http.StatusServiceUnavailable {
		t.Fatalf("checker error must fail closed: %d", got)
	}
	if got := runGate(t, nil); got != http.StatusNoContent {
		t.Fatalf("nil checker disables the gate: %d", got)
	}
}

func TestRequireActiveOrgWithoutScopeIsRejected(t *testing.T) {
	h := RequireActiveOrg(fakeChecker{active: true})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing org scope: %d", rr.Code)
	}
}

func TestOrgRateLimiterIsPerOrgAndIgnoresUnscopedRequests(t *testing.T) {
	h := NewOrgRateLimiter(2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	call := func(org string) int {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		if org != "" {
			r = r.WithContext(context.WithValue(r.Context(), orgContextKey{}, org))
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Code
	}
	if call("a") != http.StatusNoContent || call("a") != http.StatusNoContent {
		t.Fatal("first two requests of an org pass")
	}
	if call("a") != http.StatusTooManyRequests {
		t.Fatal("third request of the same org must be limited")
	}
	if call("b") != http.StatusNoContent {
		t.Fatal("another org is unaffected by org a's budget")
	}
	for i := 0; i < 5; i++ {
		if call("") != http.StatusNoContent {
			t.Fatal("a request without org scope is not this limiter's business")
		}
	}
	off := NewOrgRateLimiter(0, time.Minute)
	if off(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})) == nil {
		t.Fatal("disabled limiter returns the handler unchanged")
	}
}
