package middleware

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
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/luryann/aquify/server/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var cov1xOK = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

func cov1xErrCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rr.Body.String(), err)
	}
	if body.Error.Code != "" {
		return body.Error.Code
	}
	return body.Code
}

// ---- auth.go ----

func TestCov1xExtractAuthToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		header    string
		cookie    string
		wantToken string
		wantMsg   string
	}{
		{"bearer", "Bearer abc", "", "abc", ""},
		{"bearer trimmed", "  Bearer   abc  ", "", "abc", ""},
		{"wrong scheme", "Basic xyz", "", "", "Authorization header must use Bearer token format."},
		{"bare scheme", "Bearer ", "", "", "Authorization header must use Bearer token format."},
		{"cookie", "", " cookie-tok ", "cookie-tok", ""},
		{"blank cookie", "", "   ", "", "Missing Authorization header or session cookie."},
		{"nothing", "", "", "", "Missing Authorization header or session cookie."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: tc.cookie})
			}
			tok, code, msg := ExtractAuthToken(req)
			if tok != tc.wantToken || msg != tc.wantMsg {
				t.Fatalf("got (%q,%q,%q), want token %q msg %q", tok, code, msg, tc.wantToken, tc.wantMsg)
			}
			if tc.wantToken == "" && code != "unauthorized" {
				t.Fatalf("code = %q, want unauthorized", code)
			}
		})
	}
}

type cov1xResolver struct{ p *Principal }

func (r cov1xResolver) ResolvePrincipal(token string) (*Principal, bool) {
	if token == "good" {
		return r.p, true
	}
	return nil, false
}

type cov1xErrResolver struct {
	p   *Principal
	err error
}

func (r cov1xErrResolver) ResolvePrincipal(string) (*Principal, bool) { return nil, false }
func (r cov1xErrResolver) ResolvePrincipalErr(token string) (*Principal, error) {
	if r.err != nil {
		return nil, r.err
	}
	if token == "good" {
		return r.p, nil
	}
	return nil, nil
}

func TestCov1xAuthResolvers(t *testing.T) {
	t.Parallel()
	want := &Principal{UserID: "u1", OrgID: "o1", Roles: []string{"admin"}}
	run := func(res PrincipalResolver, token string) *httptest.ResponseRecorder {
		var got *Principal
		h := Auth(res)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, _ = PrincipalFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code == http.StatusNoContent && got != want {
			t.Fatalf("principal in context = %+v, want %+v", got, want)
		}
		return rr
	}

	if rr := run(cov1xResolver{want}, "good"); rr.Code != http.StatusNoContent {
		t.Fatalf("plain resolver good: %d", rr.Code)
	}
	if rr := run(cov1xResolver{want}, "bad"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("plain resolver bad: %d", rr.Code)
	}
	if rr := run(cov1xResolver{want}, ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: %d", rr.Code)
	}
	if rr := run(cov1xErrResolver{p: want}, "good"); rr.Code != http.StatusNoContent {
		t.Fatalf("err resolver good: %d", rr.Code)
	}
	if rr := run(cov1xErrResolver{p: want}, "bad"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("err resolver unknown token: %d", rr.Code)
	}
	rr := run(cov1xErrResolver{err: errors.New("store down")}, "good")
	if rr.Code != http.StatusServiceUnavailable || rr.Header().Get("Retry-After") != "1" {
		t.Fatalf("err resolver store down: %d retry=%q", rr.Code, rr.Header().Get("Retry-After"))
	}
	if code := cov1xErrCode(t, rr); code != "service_unavailable" {
		t.Fatalf("code = %q", code)
	}
	// A nil resolver cannot authenticate anyone.
	if rr := run(nil, "good"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("nil resolver: %d", rr.Code)
	}
}

func TestCov1xRoleHelpers(t *testing.T) {
	t.Parallel()
	p := func(roles ...string) *Principal { return &Principal{Roles: roles} }
	if HasAnyRole(nil, "admin") {
		t.Fatal("nil principal must hold no roles")
	}
	if !HasAnyRole(p("coach", "parent"), "admin", "parent") || HasAnyRole(p("coach"), "admin") || HasAnyRole(p(), "admin") {
		t.Fatal("HasAnyRole mismatch")
	}
	if !IsParentOnly(p("parent")) || IsParentOnly(p("parent", "coach")) || IsParentOnly(p("swimmer")) {
		t.Fatal("IsParentOnly mismatch")
	}
	if !IsSwimmerOnly(p("swimmer")) || IsSwimmerOnly(p("swimmer", "parent")) || IsSwimmerOnly(p("admin")) {
		t.Fatal("IsSwimmerOnly mismatch")
	}
}

// ---- csrf.go ----

func TestCov1xCSRFProtection(t *testing.T) {
	t.Parallel()
	h := CSRFProtection("  ")(cov1xOK) // blank header name falls back to X-CSRF-Token
	custom := CSRFProtection("X-Custom")(cov1xOK)

	mk := func(method string, cookieHeader bool, csrfCookie, header string) *http.Request {
		req := httptest.NewRequest(method, "/x", nil)
		if cookieHeader {
			req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "tok"})
		}
		if csrfCookie != "" {
			req.AddCookie(&http.Cookie{Name: CSRFTokenCookieName, Value: csrfCookie})
		}
		if header != "" {
			req.Header.Set("X-CSRF-Token", header)
		}
		return req
	}
	cases := []struct {
		name string
		req  *http.Request
		want int
		code string
	}{
		{"GET is safe", mk(http.MethodGet, true, "", ""), http.StatusNoContent, ""},
		{"bearer POST exempt", func() *http.Request {
			r := mk(http.MethodPost, true, "", "")
			r.Header.Set("Authorization", "Bearer x")
			return r
		}(), http.StatusNoContent, ""},
		{"no cookies at all", mk(http.MethodPost, false, "", ""), http.StatusNoContent, ""},
		{"missing csrf cookie", mk(http.MethodPost, true, "", "tok"), http.StatusForbidden, "csrf_required"},
		{"missing header", mk(http.MethodDelete, true, "abc", ""), http.StatusForbidden, "csrf_required"},
		{"mismatch", mk(http.MethodPatch, true, "abc", "abd"), http.StatusForbidden, "csrf_invalid"},
		{"match", mk(http.MethodPut, true, "abc", "abc"), http.StatusNoContent, ""},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, tc.req)
		if rr.Code != tc.want {
			t.Fatalf("%s: status %d, want %d", tc.name, rr.Code, tc.want)
		}
		if tc.code != "" && cov1xErrCode(t, rr) != tc.code {
			t.Fatalf("%s: code %q, want %q", tc.name, cov1xErrCode(t, rr), tc.code)
		}
	}

	// A custom header name is honoured and the default one is ignored.
	req := mk(http.MethodPost, true, "abc", "abc")
	rr := httptest.NewRecorder()
	custom.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("custom header name ignored default header: %d", rr.Code)
	}
	req.Header.Set("X-Custom", "abc")
	rr = httptest.NewRecorder()
	custom.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("custom header rejected: %d", rr.Code)
	}
}

// ---- logging.go ----

func cov1xLogRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode log %q: %v", buf.String(), err)
	}
	return rec
}

func TestCov1xStructuredLogger(t *testing.T) {
	t.Parallel()

	t.Run("principal reported by Auth", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		h := RequestID(StructuredLogger(logger)(Auth(cov1xResolver{&Principal{UserID: "u9", OrgID: "o9"}})(cov1xOK)))
		req := httptest.NewRequest(http.MethodGet, "/things", nil)
		req.Header.Set("Authorization", "Bearer good")
		req.Header.Set("X-Request-Id", "rid-1")
		req.Header.Set("User-Agent", "cov-agent")
		h.ServeHTTP(httptest.NewRecorder(), req)
		rec := cov1xLogRecord(t, &buf)
		if rec["msg"] != "http_request" || rec["user_id"] != "u9" || rec["org_id"] != "o9" ||
			rec["request_id"] != "rid-1" || rec["path"] != "/things" || rec["method"] != "GET" ||
			rec["status"] != float64(204) || rec["user_agent"] != "cov-agent" {
			t.Fatalf("unexpected record: %v", rec)
		}
		if _, ok := rec["trace_id"]; ok {
			t.Fatal("trace_id must be absent without a span")
		}
	})

	t.Run("principal from context and trace ids", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		sc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
			TraceFlags: trace.FlagsSampled,
		})
		// Inner handler replaces the request's context principal on the
		// original request via a pointer-shared context value is not
		// possible, so inject principal and span before the logger.
		inject := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := ContextWithPrincipal(r.Context(), &Principal{UserID: "ctx-u", OrgID: "ctx-o"})
				ctx = trace.ContextWithSpanContext(ctx, sc)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
		h := inject(StructuredLogger(logger)(cov1xOK))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a", nil))
		rec := cov1xLogRecord(t, &buf)
		if rec["user_id"] != "ctx-u" || rec["org_id"] != "ctx-o" {
			t.Fatalf("principal not logged from context: %v", rec)
		}
		if rec["trace_id"] != sc.TraceID().String() || rec["span_id"] != sc.SpanID().String() {
			t.Fatalf("trace ids not logged: %v", rec)
		}
	})

	t.Run("anonymous and nil logger", func(t *testing.T) {
		t.Parallel()
		// A nil logger falls back to slog.Default; the request still completes.
		rr := httptest.NewRecorder()
		StructuredLogger(nil)(cov1xOK).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/anon", nil))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status %d", rr.Code)
		}
		var buf bytes.Buffer
		StructuredLogger(slog.New(slog.NewJSONHandler(&buf, nil)))(cov1xOK).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/anon", nil))
		rec := cov1xLogRecord(t, &buf)
		if _, ok := rec["user_id"]; ok || rec["path"] != "/anon" {
			t.Fatalf("unexpected record: %v", rec)
		}
	})

	t.Run("feed token path is redacted", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		r := chi.NewRouter()
		r.Use(StructuredLogger(logger))
		r.Get("/feed/{token}.ics", func(w http.ResponseWriter, _ *http.Request) {})
		r.Get("/plain/{id}", func(w http.ResponseWriter, _ *http.Request) {})
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/feed/SECRET.ics", nil))
		rec := cov1xLogRecord(t, &buf)
		if rec["path"] != "/feed/{token}.ics" || strings.Contains(buf.String(), "SECRET") {
			t.Fatalf("secret leaked or pattern missing: %s", buf.String())
		}
		buf.Reset()
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/plain/42", nil))
		if rec = cov1xLogRecord(t, &buf); rec["path"] != "/plain/42" {
			t.Fatalf("non-secret route must log actual path: %v", rec)
		}
	})
}

func TestCov1xRecordLoggedPrincipal(t *testing.T) {
	t.Parallel()
	holder := &loggedPrincipal{}
	ctx := context.WithValue(context.Background(), loggedPrincipalKey{}, holder)
	recordLoggedPrincipal(ctx, nil)
	if u, o := holder.get(); u != "" || o != "" {
		t.Fatal("nil principal must not record")
	}
	recordLoggedPrincipal(context.Background(), &Principal{UserID: "x"}) // no holder: no panic
	recordLoggedPrincipal(ctx, &Principal{UserID: "u", OrgID: "o"})
	if u, o := holder.get(); u != "u" || o != "o" {
		t.Fatalf("got %q %q", u, o)
	}
}

// ---- org_scope.go ----

func TestCov1xRequireOrgScope(t *testing.T) {
	t.Parallel()
	var gotOrg string
	h := RequireOrgScope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrg, _ = OrgIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	do := func(path, header string, p *Principal) *httptest.ResponseRecorder {
		gotOrg = ""
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if header != "" {
			req.Header.Set("X-Org-Id", header)
		}
		if p != nil {
			req = req.WithContext(ContextWithPrincipal(req.Context(), p))
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	p := &Principal{UserID: "u", OrgID: "org-a"}

	if rr := do("/x", "org-a", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("no principal: %d", rr.Code)
	}
	if rr := do("/x", "org-a", nil); cov1xErrCode(t, rr) != "unauthorized" {
		t.Fatal("expected unauthorized code")
	}
	if rr := do("/x", "", p); rr.Code != http.StatusBadRequest || cov1xErrCode(t, rr) != "missing_org_scope" {
		t.Fatalf("missing header: %d", rr.Code)
	}
	// The session endpoint may omit the header, but only if the principal has an org.
	if rr := do("/api/v1/auth/session", "", p); rr.Code != http.StatusNoContent || gotOrg != "org-a" {
		t.Fatalf("session path: %d org=%q", rr.Code, gotOrg)
	}
	if rr := do("/api/v1/auth/session", "", &Principal{UserID: "u"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("session path without principal org: %d", rr.Code)
	}
	rr := do("/x", "org-b", p)
	if rr.Code != http.StatusForbidden || cov1xErrCode(t, rr) != "scope_violation" {
		t.Fatalf("mismatch: %d", rr.Code)
	}
	if rr := do("/x", " org-a ", p); rr.Code != http.StatusNoContent || gotOrg != "org-a" {
		t.Fatalf("match: %d org=%q", rr.Code, gotOrg)
	}
	// A typed-nil principal in context is treated as missing.
	nilReq := httptest.NewRequest(http.MethodGet, "/x", nil)
	nilReq = nilReq.WithContext(ContextWithPrincipal(nilReq.Context(), nil))
	nilReq.Header.Set("X-Org-Id", "org-a")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, nilReq)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("nil principal: %d", rr.Code)
	}
}

// ---- permissions.go ----

func TestCov1xCheckPermission(t *testing.T) {
	t.Parallel()
	admin := &Principal{Roles: []string{"admin"}, MFAAuthenticated: true}
	if err := CheckPermission(nil, ActionBillingRefund); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("nil: %v", err)
	}
	err := CheckPermission(admin, Action("nope"))
	if !errors.Is(err, ErrPermissionDenied) || !strings.Contains(err.Error(), `unknown action "nope"`) {
		t.Fatalf("unknown action: %v", err)
	}
	if err := CheckPermission(&Principal{Roles: []string{"coach"}, MFAAuthenticated: true}, ActionAssignRole); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("non-admin: %v", err)
	}
	if err := CheckPermission(&Principal{Roles: []string{"admin"}}, ActionBillingVoid); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("no mfa: %v", err)
	}
	for _, a := range []Action{ActionAssignRole, ActionBillingRefund, ActionBillingVoid, ActionBillingWriteOff, ActionBillingPaymentRecord} {
		if err := CheckPermission(admin, a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
}

// ---- rate_limit.go ----

func TestCov1xParseTrustedNets(t *testing.T) {
	t.Parallel()
	nets := parseTrustedNets([]string{"", "  ", "10.0.0.0/8", "192.168.1.1", "::1", "not-an-ip", "bad/cidr", "2001:db8::/32"})
	var got []string
	for _, n := range nets {
		got = append(got, n.String())
	}
	want := []string{"10.0.0.0/8", "192.168.1.1/32", "::1/128", "2001:db8::/32"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if parseTrustedNets(nil) != nil {
		t.Fatal("empty input must yield nil")
	}
	if isTrustedIP("garbage", nets) || isTrustedIP("8.8.8.8", nets) || !isTrustedIP("10.1.2.3", nets) || !isTrustedIP("::1", nets) {
		t.Fatal("isTrustedIP mismatch")
	}
}

func TestCov1xAllowWindowing(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := &rateLimiter{entries: map[string]*rateLimitEntry{}, limit: 2, window: 10 * time.Second, sweepInterval: 10 * time.Second}

	for i := 0; i < 2; i++ {
		if ok, _ := rl.allow("a", base); !ok {
			t.Fatalf("request %d should pass", i)
		}
	}
	ok, retry := rl.allow("a", base.Add(9500*time.Millisecond))
	if ok || retry != time.Second {
		t.Fatalf("blocked with sub-second remainder must clamp to 1s: ok=%v retry=%v", ok, retry)
	}
	ok, retry = rl.allow("a", base.Add(2*time.Second))
	if ok || retry != 8*time.Second {
		t.Fatalf("ok=%v retry=%v, want blocked/8s", ok, retry)
	}
	// Another key is independent; later sweep drops the expired "a" entry.
	if ok, _ := rl.allow("b", base.Add(5*time.Second)); !ok {
		t.Fatal("independent key must pass")
	}
	if ok, _ := rl.allow("c", base.Add(30*time.Second)); !ok {
		t.Fatal("new key after sweep must pass")
	}
	if _, present := rl.entries["a"]; present {
		t.Fatal("expired entry should have been swept")
	}
	if _, present := rl.entries["b"]; present {
		t.Fatal("expired entry b should have been swept")
	}
	// An expired entry that was not swept yet restarts its window.
	rl2 := &rateLimiter{entries: map[string]*rateLimitEntry{}, limit: 1, window: time.Second, sweepInterval: time.Hour}
	rl2.allow("k", base)
	if ok, _ := rl2.allow("k", base); ok {
		t.Fatal("second request in window must be blocked")
	}
	if ok, _ := rl2.allow("k", base.Add(2*time.Second)); !ok {
		t.Fatal("window should have reset")
	}
}

func TestCov1xNewRateLimiterMiddleware(t *testing.T) {
	t.Parallel()
	h := NewRateLimiter(0, 0, []string{"10.0.0.0/8"})(cov1xOK) // defaults: 60 per minute
	send := func(remote, xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	for i := 0; i < 60; i++ {
		if rr := send("203.0.113.5:1000", ""); rr.Code != http.StatusNoContent {
			t.Fatalf("request %d: %d", i, rr.Code)
		}
	}
	rr := send("203.0.113.5:2000", "")
	if rr.Code != http.StatusTooManyRequests || cov1xErrCode(t, rr) != "rate_limited" {
		t.Fatalf("61st request: %d", rr.Code)
	}
	if ra := rr.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q", ra)
	}
	// An untrusted peer cannot dodge the limit by spoofing X-Forwarded-For.
	if rr := send("203.0.113.5:3000", "198.51.100.1"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed XFF from untrusted peer: %d", rr.Code)
	}
	// A trusted proxy forwarding a different client is keyed separately.
	if rr := send("10.0.0.1:80", "198.51.100.1"); rr.Code != http.StatusNoContent {
		t.Fatalf("trusted proxy client: %d", rr.Code)
	}
}

func TestCov1xRateLimiterUsesTrustedProxyContext(t *testing.T) {
	t.Parallel()
	rl := &rateLimiter{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:99"
	if got := rl.clientAddress(req); got != "192.0.2.1" {
		t.Fatalf("direct: %q", got)
	}
	req = req.WithContext(context.WithValue(req.Context(), clientInfoKey{}, clientInfo{ip: "198.51.100.9"}))
	if got := rl.clientAddress(req); got != "198.51.100.9" {
		t.Fatalf("from context: %q", got)
	}
}

func TestCov1xResolveClientAddress(t *testing.T) {
	t.Parallel()
	rl := &rateLimiter{trustedNets: parseTrustedNets([]string{"10.0.0.0/8", "172.16.0.0/12"})}
	mk := func(remote string, xff ...string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		for _, v := range xff {
			req.Header.Add("X-Forwarded-For", v)
		}
		return req
	}
	cases := []struct {
		name string
		req  *http.Request
		want string
	}{
		{"direct", mk("203.0.113.1:1"), "203.0.113.1"},
		{"untrusted ignores xff", mk("203.0.113.1:1", "1.1.1.1"), "203.0.113.1"},
		{"trusted no header", mk("10.0.0.1:1"), "10.0.0.1"},
		{"trusted rightmost untrusted", mk("10.0.0.1:1", "6.6.6.6, 1.1.1.1, 10.0.0.2"), "1.1.1.1"},
		{"multiple header lines", mk("10.0.0.1:1", "6.6.6.6", "2.2.2.2, 172.16.0.1"), "2.2.2.2"},
		{"all trusted -> leftmost", mk("10.0.0.1:1", "10.0.0.9, 172.16.0.3"), "10.0.0.9"},
		{"blank hops ignored", mk("10.0.0.1:1", " , ,"), "10.0.0.1"},
		{"hostless remote", mk("somehost"), "somehost"},
		{"empty remote", mk(""), "unknown"},
		{"port only remote", mk(":80"), ":80"},
	}
	for _, tc := range cases {
		if got := rl.resolveClientAddress(tc.req); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := remoteIP(" 1.2.3.4:80 "); got != "1.2.3.4" {
		t.Fatalf("remoteIP = %q", got)
	}
	// Trusted peer given with no port.
	if got := rl.resolveClientAddress(mk("10.0.0.1", "9.9.9.9")); got != "9.9.9.9" {
		t.Fatalf("portless trusted peer: %q", got)
	}
	// Whitespace-only remote address falls back to "unknown".
	if got := rl.resolveClientAddress(mk("   ")); got != "unknown" {
		t.Fatalf("whitespace remote: %q", got)
	}
}

func TestCov1xClientAddressResolver(t *testing.T) {
	t.Parallel()
	resolve := NewClientAddressResolver([]string{"10.0.0.0/8"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.1.1:5"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := resolve(req); got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
}

func TestCov1xOrgRateLimiter(t *testing.T) {
	t.Parallel()
	// Disabled limiter is a pass-through.
	h := NewOrgRateLimiter(0, 0)(cov1xOK)
	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("disabled limiter blocked: %d", rr.Code)
		}
	}

	h = NewOrgRateLimiter(2, 0)(cov1xOK) // window defaults to a minute
	withOrg := func(org string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if org == "" {
			return req
		}
		return req.WithContext(context.WithValue(req.Context(), orgContextKey{}, org))
	}
	for i := 0; i < 5; i++ { // requests with no org are never limited
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, withOrg(""))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("no-org request limited: %d", rr.Code)
		}
	}
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, withOrg("org-1"))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("org-1 request %d: %d", i, rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withOrg("org-1"))
	if rr.Code != http.StatusTooManyRequests || cov1xErrCode(t, rr) != "org_rate_limited" || rr.Header().Get("Retry-After") == "" {
		t.Fatalf("over limit: %d %s", rr.Code, rr.Body.String())
	}
	// Other orgs have their own budget.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, withOrg("org-2"))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("org-2: %d", rr.Code)
	}
	// Empty org id in context is treated as no org.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req.WithContext(context.WithValue(req.Context(), orgContextKey{}, "")))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("empty org: %d", rr.Code)
	}
}

// ---- rbac.go ----

func TestCov1xRBACStub(t *testing.T) {
	t.Parallel()
	h := RBACStub("admin", "coach")(cov1xOK)
	do := func(p *Principal) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if p != nil {
			req = req.WithContext(ContextWithPrincipal(req.Context(), p))
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	if rr := do(nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("no principal: %d", rr.Code)
	}
	if rr := do(&Principal{Roles: []string{"parent", "coach"}}); rr.Code != http.StatusNoContent {
		t.Fatalf("allowed: %d", rr.Code)
	}
	rr := do(&Principal{Roles: []string{"parent"}})
	if rr.Code != http.StatusForbidden || cov1xErrCode(t, rr) != "forbidden" {
		t.Fatalf("forbidden: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "required_roles") || !strings.Contains(rr.Body.String(), "parent") {
		t.Fatalf("details missing: %s", rr.Body.String())
	}
	nilReq := httptest.NewRequest(http.MethodGet, "/", nil)
	nilReq = nilReq.WithContext(ContextWithPrincipal(nilReq.Context(), nil))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, nilReq)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("typed nil principal: %d", rr.Code)
	}
}

// ---- recovery.go ----

func TestCov1xRecoverer(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	rr := httptest.NewRecorder()
	RequestID(Recoverer(logger)(boom)).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusInternalServerError || cov1xErrCode(t, rr) != "internal_error" {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "kaboom") {
		t.Fatal("panic value must not reach the client")
	}
	rec := cov1xLogRecord(t, &buf)
	if rec["msg"] != "panic_recovered" || rec["panic"] != "kaboom" || rec["request_id"] == nil || rec["request_id"] == "" || rec["stack"] == nil || rec["stack"] == "" {
		t.Fatalf("log record: %v", rec)
	}
	// Non-panicking handlers pass through untouched, and nil logger is accepted.
	rr = httptest.NewRecorder()
	Recoverer(nil)(cov1xOK).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("pass-through: %d", rr.Code)
	}
	// Nil logger still recovers (falls back to slog.Default).
	rr = httptest.NewRecorder()
	Recoverer(nil)(boom).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("nil logger panic: %d", rr.Code)
	}
}

// ---- request_id.go ----

func TestCov1xRequestID(t *testing.T) {
	t.Parallel()
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = chimiddleware.GetReqID(r.Context())
	}))
	do := func(in string) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if in != "" {
			req.Header.Set("X-Request-Id", in)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("X-Request-Id"); got != seen {
			t.Fatalf("header %q != context %q", got, seen)
		}
		return seen
	}
	if got := do("abc-1_2.3"); got != "abc-1_2.3" {
		t.Fatalf("valid id replaced: %q", got)
	}
	for _, bad := range []string{"has space", "semi;colon", strings.Repeat("a", 129), ""} {
		got := do(bad)
		if got == bad || len(got) != 24 {
			t.Fatalf("bad id %q -> %q", bad, got)
		}
	}
	if got := do(strings.Repeat("a", 128)); len(got) != 128 {
		t.Fatalf("128-char id should be accepted, got %d chars", len(got))
	}
	if a, b := newRequestID(), newRequestID(); a == b {
		t.Fatal("ids must be unique")
	}
}

// ---- session_cookies.go ----

func TestCov1xSessionCookies(t *testing.T) {
	t.Parallel()
	// NewToken uses the injected factory, or the default.
	boom := errors.New("entropy")
	if _, err := (SessionCookies{NewCSRFToken: func() (string, error) { return "", boom }}).NewToken(); !errors.Is(err, boom) {
		t.Fatalf("injected error: %v", err)
	}
	if tok, _ := (SessionCookies{NewCSRFToken: func() (string, error) { return "fixed", nil }}).NewToken(); tok != "fixed" {
		t.Fatalf("injected token: %q", tok)
	}
	tok, err := SessionCookies{}.NewToken()
	if err != nil || len(tok) != 64 {
		t.Fatalf("default token: %q %v", tok, err)
	}

	// Set with an already-expired time clamps MaxAge to 0 (Go renders it as no Max-Age).
	rr := httptest.NewRecorder()
	SessionCookies{Insecure: true}.Set(rr, "a", "r", "c", time.Now().Add(-time.Hour))
	cookies := rr.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("cookies: %d", len(cookies))
	}
	for _, c := range cookies {
		if c.MaxAge != 0 || c.Secure || c.SameSite != http.SameSiteStrictMode {
			t.Fatalf("expired set cookie %+v", c)
		}
	}
	// Normal Set marks tokens HttpOnly except the CSRF cookie and is Secure by default.
	rr = httptest.NewRecorder()
	SessionCookies{}.Set(rr, "a", "r", "c", time.Now().Add(time.Hour))
	for _, c := range rr.Result().Cookies() {
		if !c.Secure || c.MaxAge <= 0 || c.HttpOnly != (c.Name != CSRFTokenCookieName) {
			t.Fatalf("cookie %+v", c)
		}
	}
	// Clear expires all three.
	rr = httptest.NewRecorder()
	SessionCookies{}.Clear(rr)
	cleared := rr.Result().Cookies()
	if len(cleared) != 3 {
		t.Fatalf("cleared: %d", len(cleared))
	}
	for _, c := range cleared {
		if c.MaxAge >= 0 || c.Value != "" || c.HttpOnly != (c.Name != CSRFTokenCookieName) {
			t.Fatalf("cleared cookie %+v", c)
		}
	}
}

// ---- swimmer_access.go ----

func TestCov1xRequireSwimmerAccess(t *testing.T) {
	t.Parallel()
	var calls int
	var gotOrg, gotSwimmer string
	checker := SwimmerAccessChecks(func(org, sw string) (bool, error) {
		calls++
		gotOrg, gotSwimmer = org, sw
		switch sw {
		case "off":
			return true, nil
		case "err":
			return false, errors.New("db down")
		}
		return false, nil
	})
	h := RequireSwimmerAccess(checker)(cov1xOK)
	do := func(p *Principal) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if p != nil {
			req = req.WithContext(ContextWithPrincipal(req.Context(), p))
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	if rr := do(nil); rr.Code != http.StatusNoContent || calls != 0 {
		t.Fatalf("no principal: %d calls=%d", rr.Code, calls)
	}
	if rr := do(&Principal{Roles: []string{"swimmer", "parent"}, SwimmerID: "off"}); rr.Code != http.StatusNoContent || calls != 0 {
		t.Fatalf("mixed role must bypass: %d calls=%d", rr.Code, calls)
	}
	if rr := do(&Principal{OrgID: "o", SwimmerID: "ok", Roles: []string{"swimmer"}}); rr.Code != http.StatusNoContent || gotOrg != "o" || gotSwimmer != "ok" {
		t.Fatalf("enabled swimmer: %d %q %q", rr.Code, gotOrg, gotSwimmer)
	}
	rr := do(&Principal{OrgID: "o", SwimmerID: "off", Roles: []string{"swimmer"}})
	if rr.Code != http.StatusForbidden || cov1xErrCode(t, rr) != "swimmer_access_disabled" {
		t.Fatalf("disabled: %d", rr.Code)
	}
	rr = do(&Principal{OrgID: "o", SwimmerID: "err", Roles: []string{"swimmer"}})
	if rr.Code != http.StatusInternalServerError || cov1xErrCode(t, rr) != "internal_error" {
		t.Fatalf("checker error: %d", rr.Code)
	}
	// Typed-nil principal passes through.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(ContextWithPrincipal(req.Context(), nil))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("typed nil: %d", rr.Code)
	}
	// A nil checker leaves the handler unwrapped.
	rr = httptest.NewRecorder()
	RequireSwimmerAccess(nil)(cov1xOK).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("nil checker: %d", rr.Code)
	}
}

// ---- telemetry.go ----

// Not parallel: it swaps the process-global tracer provider and restores it.
func TestCov1xOTelTracingSpanNames(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev); _ = tp.Shutdown(context.Background()) })

	h := OTelTracing("cov-svc")(cov1xOK)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/before-routing", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("no route context: %d", rr.Code)
	}
	// With a matched chi pattern in context the formatter takes the route branch.
	rctx := chi.NewRouteContext()
	rctx.RoutePatterns = []string{"/members/{memberID}"}
	req := httptest.NewRequest(http.MethodGet, "/members/7", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("route context: %d", rr.Code)
	}
	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(spans))
	}
	if spans[0].Name() != "GET" || spans[1].Name() != "GET /members/{memberID}" {
		t.Fatalf("span names = %q, %q", spans[0].Name(), spans[1].Name())
	}
}

func TestCov1xPrometheusMetrics(t *testing.T) {
	t.Parallel()
	m := telemetry.NewMetrics()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
		SpanID:  trace.SpanID{9, 9, 9, 9, 9, 9, 9, 9},
	})
	withSpan := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(trace.ContextWithSpanContext(r.Context(), sc)))
		})
	}
	r := chi.NewRouter()
	r.Use(withSpan, PrometheusMetrics(m))
	r.Get("/silent/{id}", func(http.ResponseWriter, *http.Request) {}) // never writes: status 0 -> 200
	r.Get("/teapot", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	for _, path := range []string{"/silent/1", "/silent/2", "/teapot", "/nowhere"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if got := cov1xCounter(t, m.HTTPRequestsTotal.WithLabelValues("GET", "/silent/{id}", "200")); got != 2 {
		t.Fatalf("silent route count = %v, want 2", got)
	}
	if got := cov1xCounter(t, m.HTTPRequestsTotal.WithLabelValues("GET", "/teapot", "418")); got != 1 {
		t.Fatalf("teapot count = %v", got)
	}
	if got := cov1xCounter(t, m.HTTPRequestsTotal.WithLabelValues("GET", "unmatched", "404")); got != 1 {
		t.Fatalf("unmatched count = %v", got)
	}
}

func cov1xCounter(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var out dto.Metric
	if err := c.Write(&out); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return out.GetCounter().GetValue()
}
