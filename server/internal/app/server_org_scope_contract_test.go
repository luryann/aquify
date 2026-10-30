package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestProtectedRoutesEnforceOrgScopeContracts(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	inScopeToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)
	crossOrgToken := issueSessionToken(t, router, "org-other", []string{"admin"}, true)

	routes := []struct {
		name          string
		method        string
		path          string
		body          map[string]any
		expectedMatch int
	}{
		{name: "authz ping", method: http.MethodGet, path: "/api/v1/authz/ping", expectedMatch: http.StatusOK},
		{name: "runtime settings", method: http.MethodGet, path: "/api/v1/settings/runtime", expectedMatch: http.StatusOK},
		{name: "member households", method: http.MethodGet, path: "/api/v1/members/households", expectedMatch: http.StatusOK},
		{name: "audit events", method: http.MethodGet, path: "/api/v1/audit-events", expectedMatch: http.StatusOK},
		{
			name:          "role assignments list",
			method:        http.MethodGet,
			path:          "/api/v1/auth/roles/assignments",
			expectedMatch: http.StatusOK,
		},
		{
			name:   "role assignment create",
			method: http.MethodPost,
			path:   "/api/v1/auth/roles/assign",
			body: map[string]any{
				"org_id":         "org-demo",
				"target_user_id": "user-123",
				"role":           "coach",
				"reason":         "Org scope contract coverage",
			},
			expectedMatch: http.StatusCreated,
		},
	}

	for _, route := range routes {
		route := route
		t.Run(route.name+"/missing header", func(t *testing.T) {
			t.Parallel()

			rr := doOrgScopeContractRequest(t, router, route.method, route.path, inScopeToken, "", route.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
			assertOrgScopeErrorCode(t, rr, "missing_org_scope")
		})

		t.Run(route.name+"/cross org", func(t *testing.T) {
			t.Parallel()

			rr := doOrgScopeContractRequest(t, router, route.method, route.path, crossOrgToken, "org-demo", route.body)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
			}
			assertOrgScopeErrorCode(t, rr, "scope_violation")
		})

		t.Run(route.name+"/matching org", func(t *testing.T) {
			t.Parallel()

			rr := doOrgScopeContractRequest(t, router, route.method, route.path, inScopeToken, "org-demo", route.body)
			if rr.Code != route.expectedMatch {
				t.Fatalf("expected %d, got %d: %s", route.expectedMatch, rr.Code, rr.Body.String())
			}
		})
	}
}

func doOrgScopeContractRequest(t *testing.T, router http.Handler, method, path, token, orgID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if orgID != "" {
		req.Header.Set("X-Org-Id", orgID)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func assertOrgScopeErrorCode(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()

	var env apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if env.Error.Code != want {
		t.Fatalf("expected error code %q, got %q", want, env.Error.Code)
	}
}

// publicRoutes is the explicit list of routes mounted outside the
// authenticated, org-scoped group ("METHOD /path/template"). Every other route
// the router exposes must reject a missing X-Org-Id and a cross-org header, so
// a newly mounted protected route cannot silently escape org scoping: it either
// passes the scope contract or has to be listed here on purpose.
var publicRoutes = map[string]string{
	"GET /api/v1/auth/oidc/authorize":     "OIDC login redirect (pre-auth)",
	"POST /api/v1/auth/oidc/callback":     "OIDC code exchange, protected by signed state",
	"GET /api/v1/auth/session":            "session hydration; the org comes from the session itself",
	"POST /api/v1/auth/sessions/issue":    "dev/test session issue (pre-auth)",
	"POST /api/v1/auth/sessions/logout":   "terminates the caller's own session",
	"POST /api/v1/auth/sessions/refresh":  "refresh-token rotation (pre-auth)",
	"POST /api/v1/auth/sessions/revoke":   "revokes the caller's own token",
	"GET /api/v1/calendar/feed/{token}":   "public .ics feed; the token is the credential",
	"GET /api/v1/health/live":             "liveness probe",
	"GET /api/v1/health/ready":            "readiness probe",
	"GET /api/v1/openapi":                 "public API spec",
	"GET /api/v1/public/orgs/{slug}/site": "public team website feed",
	"POST /api/v1/register":               "self-service access request (pre-auth)",
	"POST /api/v1/webhooks/postmark":      "provider webhook, authenticated by signature",
	"POST /api/v1/webhooks/stripe":        "provider webhook, authenticated by signature",
}

func concreteRoutePath(template string) string {
	var b strings.Builder
	for i := 0; i < len(template); i++ {
		if template[i] != '{' {
			b.WriteByte(template[i])
			continue
		}
		end := strings.IndexByte(template[i:], '}')
		b.WriteString("x")
		i += end
	}
	return strings.ReplaceAll(b.String(), "*", "x")
}

func TestEveryNonPublicRouteEnforcesOrgScope(t *testing.T) {
	t.Parallel()

	// Limits are lifted: this test fires several requests per route.
	cfg := Config{}
	cfg.Security.RateLimit = RateLimitConfig{GlobalRequests: 1_000_000, AuthRequests: 1_000_000, StripeRequests: 1_000_000,
		GlobalWindow: time.Minute, AuthWindow: time.Minute, StripeWindow: time.Minute}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	routes, ok := router.(chi.Routes)
	if !ok {
		t.Fatalf("router %T does not expose its routes", router)
	}
	type route struct{ method, template string }
	var all []route
	if err := chi.Walk(routes, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		all = append(all, route{method, strings.ReplaceAll(pattern, "/*/", "/")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(all) < 100 {
		t.Fatalf("walked only %d routes; the walk is not covering the router", len(all))
	}

	seenPublic := map[string]bool{}
	protected := 0
	for _, rt := range all {
		key := rt.method + " " + rt.template
		path := concreteRoutePath(rt.template)

		// Fresh sessions per route: logout/revoke routes invalidate their own token.
		inScopeToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)
		crossOrgToken := issueSessionToken(t, router, "org-other", []string{"admin"}, true)
		missing := doOrgScopeContractRequest(t, router, rt.method, path, inScopeToken, "", map[string]any{})
		isScoped := missing.Code == http.StatusBadRequest && strings.Contains(missing.Body.String(), `"missing_org_scope"`)
		if reason, public := publicRoutes[key]; public {
			seenPublic[key] = true
			if isScoped {
				t.Errorf("%s is listed as public (%s) but actually enforces org scope; remove it from publicRoutes", key, reason)
			}
			continue
		}
		if !isScoped {
			t.Errorf("%s: missing X-Org-Id returned %d (%s), want 400 missing_org_scope; if the route is intentionally public add it to publicRoutes", key, missing.Code, strings.TrimSpace(missing.Body.String()))
			continue
		}
		protected++
		cross := doOrgScopeContractRequest(t, router, rt.method, path, crossOrgToken, "org-demo", map[string]any{})
		if cross.Code != http.StatusForbidden || !strings.Contains(cross.Body.String(), `"scope_violation"`) {
			t.Errorf("%s: cross-org request returned %d (%s), want 403 scope_violation", key, cross.Code, strings.TrimSpace(cross.Body.String()))
		}
	}
	for key := range publicRoutes {
		if !seenPublic[key] {
			t.Errorf("publicRoutes lists %q but the router has no such route; remove the stale entry", key)
		}
	}
	if protected < 100 {
		t.Fatalf("only %d protected routes verified", protected)
	}
}
