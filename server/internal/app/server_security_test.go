package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/authn"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestIssueSessionSetsSecureCookies(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, map[string]any{
		"user_id": "cookie-user",
		"org_id":  "org-demo",
		"roles":   []string{"admin"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	cookies := rr.Result().Cookies()
	if len(cookies) < 3 {
		t.Fatalf("expected session cookies to be set, got %d", len(cookies))
	}

	found := map[string]*http.Cookie{}
	for _, cookie := range cookies {
		found[cookie.Name] = cookie
	}

	for _, name := range []string{
		appmiddleware.AccessTokenCookieName,
		appmiddleware.RefreshTokenCookieName,
		appmiddleware.CSRFTokenCookieName,
	} {
		if found[name] == nil {
			t.Fatalf("expected cookie %s to be set", name)
		}
		if !found[name].Secure {
			t.Fatalf("expected cookie %s to be Secure", name)
		}
		if found[name].SameSite != http.SameSiteStrictMode {
			t.Fatalf("expected cookie %s to use SameSite=Strict", name)
		}
	}
	if !found[appmiddleware.AccessTokenCookieName].HttpOnly {
		t.Fatal("expected access token cookie to be HttpOnly")
	}
	if !found[appmiddleware.RefreshTokenCookieName].HttpOnly {
		t.Fatal("expected refresh token cookie to be HttpOnly")
	}
	if found[appmiddleware.CSRFTokenCookieName].HttpOnly {
		t.Fatal("expected CSRF cookie to be readable by the browser")
	}
}

func TestIssueSessionFailsWhenCSRFTokenGenerationFails(t *testing.T) {
	router := NewRouter(failingCSRFConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, map[string]any{
		"user_id": "cookie-user",
		"org_id":  "org-demo",
		"roles":   []string{"admin"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Fatalf("expected no cookies when csrf generation fails, got %#v", rr.Result().Cookies())
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if envelope.Error.Code != "csrf_generation_failed" {
		t.Fatalf("expected csrf_generation_failed, got %q", envelope.Error.Code)
	}
}

func TestIssueSessionDisabledOutsideDevMode(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "production"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, map[string]any{
		"user_id": "cookie-user",
		"org_id":  "org-demo",
		"roles":   []string{"admin"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when session issuance is disabled, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIssueSessionRejectsUnknownFieldsTrailingPayloadAndOversizedBodies(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "unknown field",
			body:       `{"user_id":"cookie-user","org_id":"org-demo","roles":["admin"],"unexpected":true}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_request_body",
		},
		{
			name:       "trailing payload",
			body:       `{"user_id":"cookie-user","org_id":"org-demo","roles":["admin"]}{"extra":true}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_request_body",
		},
		{
			name:       "oversized body",
			body:       `{"user_id":"cookie-user","org_id":"org-demo","roles":["admin"],"name":"` + strings.Repeat("x", 1<<20) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_body_too_large",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rr.Code, rr.Body.String())
			}

			var env apiErrorEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if env.Error.Code != tc.wantCode {
				t.Fatalf("expected %s, got %q", tc.wantCode, env.Error.Code)
			}
		})
	}
}

func TestRefreshSessionWithCookieRequiresCSRF(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", nil)
	req.AddCookie(session.refreshCookie)
	req.AddCookie(session.csrfCookie)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if envelope.Error.Code != "csrf_required" {
		t.Fatalf("expected csrf_required, got %q", envelope.Error.Code)
	}
}

func TestRefreshSessionWithoutCookieFailsWhenCSRFTokenGenerationFails(t *testing.T) {
	// The same router must issue the session first, so the failing factory is
	// armed only after issuance; the per-router factory defers to a flag.
	var failCSRF atomic.Bool
	cfg := Config{AppEnv: "test"}
	cfg.Auth.CSRFTokenFactory = func() (string, error) {
		if failCSRF.Load() {
			return "", errors.New("rng unavailable")
		}
		return appmiddleware.NewCSRFToken()
	}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)
	failCSRF.Store(true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", encodeJSONBody(t, map[string]any{
		"refresh_token": session.refreshCookie.Value,
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Fatalf("expected no replacement cookies when csrf generation fails, got %#v", rr.Result().Cookies())
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if envelope.Error.Code != "csrf_generation_failed" {
		t.Fatalf("expected csrf_generation_failed, got %q", envelope.Error.Code)
	}
}

func TestRefreshSessionWithCookieAndCSRFSucceeds(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", nil)
	req.AddCookie(session.refreshCookie)
	req.AddCookie(session.csrfCookie)
	req.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCORSPreflightAllowsCSRFHeaderForCookieAuthMutation(t *testing.T) {
	t.Parallel()

	const origin = "https://admin.example.test"
	router := NewRouter(Config{
		AppEnv:         "test",
		AllowedOrigins: []string{origin},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/sessions/refresh", nil)
	preflight.Header.Set("Origin", origin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "Content-Type, X-CSRF-Token")
	preflightRR := httptest.NewRecorder()
	router.ServeHTTP(preflightRR, preflight)

	if preflightRR.Code != http.StatusOK {
		t.Fatalf("expected preflight 200, got %d: %s", preflightRR.Code, preflightRR.Body.String())
	}
	if got := preflightRR.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("expected allow origin %q, got %q", origin, got)
	}
	if got := preflightRR.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials to be allowed, got %q", got)
	}
	if got := preflightRR.Header().Get("Access-Control-Allow-Headers"); !headerListContains(got, "X-CSRF-Token") {
		t.Fatalf("expected X-CSRF-Token in Access-Control-Allow-Headers, got %q", got)
	}

	mutation := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", nil)
	mutation.Header.Set("Origin", origin)
	mutation.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	mutation.AddCookie(session.refreshCookie)
	mutation.AddCookie(session.csrfCookie)
	mutationRR := httptest.NewRecorder()
	router.ServeHTTP(mutationRR, mutation)

	if mutationRR.Code != http.StatusOK {
		t.Fatalf("expected mutation 200, got %d: %s", mutationRR.Code, mutationRR.Body.String())
	}
	if got := mutationRR.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("expected mutation allow origin %q, got %q", origin, got)
	}
	if got := mutationRR.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected mutation credentials to be allowed, got %q", got)
	}
}

func TestCORSPreflightAllowsConfiguredCSRFHeaderForCookieAuthMutation(t *testing.T) {
	t.Parallel()

	const (
		origin         = "https://admin.example.test"
		customCSRFName = "X-Aquify-CSRF"
	)
	router := NewRouter(Config{
		AppEnv:         "test",
		AllowedOrigins: []string{origin},
		Security: SecurityConfig{
			CSRFHeaderName: customCSRFName,
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/sessions/refresh", nil)
	preflight.Header.Set("Origin", origin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "Content-Type, "+customCSRFName)
	preflightRR := httptest.NewRecorder()
	router.ServeHTTP(preflightRR, preflight)

	if preflightRR.Code != http.StatusOK {
		t.Fatalf("expected preflight 200, got %d: %s", preflightRR.Code, preflightRR.Body.String())
	}
	if got := preflightRR.Header().Get("Access-Control-Allow-Headers"); !headerListContains(got, customCSRFName) {
		t.Fatalf("expected %s in Access-Control-Allow-Headers, got %q", customCSRFName, got)
	}

	mutation := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", nil)
	mutation.Header.Set("Origin", origin)
	mutation.Header.Set(customCSRFName, session.csrfCookie.Value)
	mutation.AddCookie(session.refreshCookie)
	mutation.AddCookie(session.csrfCookie)
	mutationRR := httptest.NewRecorder()
	router.ServeHTTP(mutationRR, mutation)

	if mutationRR.Code != http.StatusOK {
		t.Fatalf("expected mutation 200, got %d: %s", mutationRR.Code, mutationRR.Body.String())
	}
	if got := mutationRR.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("expected mutation allow origin %q, got %q", origin, got)
	}
}

func TestLogoutSessionWithCookieNoCSRFSucceedsAndClearsCookies(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/logout", nil)
	req.AddCookie(session.accessCookie)
	req.AddCookie(session.refreshCookie)
	req.AddCookie(session.csrfCookie)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	assertSessionCookiesCleared(t, rr)

	var response logoutEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode logout response: %v", err)
	}
	if response.Status != "logged_out" {
		t.Fatalf("expected logged_out, got %q", response.Status)
	}
	if response.RedirectURL != "/login" {
		t.Fatalf("expected /login redirect, got %q", response.RedirectURL)
	}
	if response.LogoutURL != "" {
		t.Fatalf("expected empty logout_url without oidc metadata, got %q", response.LogoutURL)
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	currentReq.AddCookie(session.accessCookie)
	currentRR := httptest.NewRecorder()
	router.ServeHTTP(currentRR, currentReq)
	if currentRR.Code != http.StatusUnauthorized {
		t.Fatalf("expected old access token to be unauthorized after logout, got %d: %s", currentRR.Code, currentRR.Body.String())
	}
}

func TestLogoutSessionWithoutRefreshCookieStillClearsCookies(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/logout", nil)
	req.AddCookie(session.accessCookie)
	req.AddCookie(session.csrfCookie)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	assertSessionCookiesCleared(t, rr)

	var response logoutEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode logout response: %v", err)
	}
	if response.RedirectURL != "/login" {
		t.Fatalf("expected /login redirect, got %q", response.RedirectURL)
	}
}

func TestLogoutSessionRejectsMalformedOptionalJSONBody(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	tests := []struct {
		name string
		body string
	}{
		{
			name: "unknown field",
			body: `{"refresh_token":"token","unexpected":true}`,
		},
		{
			name: "trailing payload",
			body: `{"refresh_token":"token"}{"extra":true}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/logout", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}

			var env apiErrorEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if env.Error.Code != "invalid_request_body" {
				t.Fatalf("expected invalid_request_body, got %q", env.Error.Code)
			}
		})
	}
}

func TestLogoutSessionReturnsOIDCLogoutURLWhenConfigured(t *testing.T) {
	t.Parallel()

	authService := authn.NewService(authn.Config{
		OIDC: authn.OIDCConfig{
			EndSessionURL:         "https://issuer.example.com/logout",
			PostLogoutRedirectURL: "http://localhost:3000/login",
		},
	})
	session, err := authService.IssueSession(authn.SessionInput{
		UserID:      "cookie-user",
		OrgID:       "org-demo",
		Roles:       []string{"admin"},
		OIDCIDToken: "provider-id-token",
	})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	router := NewRouterWithServices(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{
		Auth: authService,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/logout", nil)
	req.AddCookie(&http.Cookie{Name: appmiddleware.AccessTokenCookieName, Value: session.AccessToken})
	req.AddCookie(&http.Cookie{Name: appmiddleware.RefreshTokenCookieName, Value: session.RefreshToken})
	csrfToken, err := appmiddleware.NewCSRFToken()
	if err != nil {
		t.Fatalf("generate csrf token: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: appmiddleware.CSRFTokenCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	assertSessionCookiesCleared(t, rr)

	var response logoutEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode logout response: %v", err)
	}
	if response.LogoutURL == "" {
		t.Fatal("expected oidc logout_url in response")
	}

	logoutURL, err := url.Parse(response.LogoutURL)
	if err != nil {
		t.Fatalf("parse logout url: %v", err)
	}
	if got := logoutURL.Query().Get("id_token_hint"); got != "provider-id-token" {
		t.Fatalf("expected id_token_hint=provider-id-token, got %q", got)
	}
	if got := logoutURL.Query().Get("post_logout_redirect_uri"); got != "http://localhost:3000/login" {
		t.Fatalf("expected post_logout_redirect_uri, got %q", got)
	}
}

func TestSecurityHeadersMiddlewareAddsBaselineHeaders(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := rr.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Fatalf("expected CSP header, got %q", got)
	}
}

func TestGlobalRateLimiterReturns429(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Security: SecurityConfig{
			RateLimit: RateLimitConfig{
				GlobalRequests: 2,
				GlobalWindow:   10 * time.Second,
				AuthRequests:   10,
				AuthWindow:     10 * time.Second,
			},
		},
	}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i+1, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthRateLimiterOnlyCountsCredentialWrites(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv: "test",
		Security: SecurityConfig{
			RateLimit: RateLimitConfig{
				GlobalRequests: 100,
				GlobalWindow:   10 * time.Second,
				AuthRequests:   2,
				AuthWindow:     10 * time.Second,
			},
		},
	}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Session reads happen on every page load and must not exhaust the
	// strict auth budget.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		req.RemoteAddr = "203.0.113.20:1234"
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			t.Fatalf("GET /auth/session %d was rate limited", i+1)
		}
	}

	codes := []int{}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", nil)
		req.RemoteAddr = "203.0.113.20:1234"
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		codes = append(codes, rr.Code)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("third credential write should be rate limited, got %v", codes)
	}
}

type issuedSessionCookies struct {
	accessCookie  *http.Cookie
	refreshCookie *http.Cookie
	csrfCookie    *http.Cookie
}

type logoutEnvelope struct {
	Status      string `json:"status"`
	LogoutURL   string `json:"logout_url"`
	RedirectURL string `json:"redirect_url"`
}

func TestCurrentSessionEndpointAcceptsCookieAuthWithOrgHeader(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.AddCookie(session.accessCookie)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response struct {
		User struct {
			UserID string   `json:"user_id"`
			OrgID  string   `json:"org_id"`
			Roles  []string `json:"roles"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode current session response: %v", err)
	}
	if response.User.UserID != "cookie-user" {
		t.Fatalf("expected cookie-user, got %q", response.User.UserID)
	}
	if response.User.OrgID != "org-demo" {
		t.Fatalf("expected org-demo, got %q", response.User.OrgID)
	}
	if len(response.User.Roles) != 1 || response.User.Roles[0] != "admin" {
		t.Fatalf("unexpected roles: %#v", response.User.Roles)
	}
}

func TestMFAChallengeSetsReplacementSessionCookies(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{
		AppEnv: "test",
		Auth: AuthConfig{
			EnableDevStubTokens: true,
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	enrollReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/enroll", encodeJSONBody(t, map[string]any{
		"factor_type": "backup_code",
		"label":       "Primary backup",
	}))
	enrollReq.Header.Set("Content-Type", "application/json")
	enrollReq.Header.Set("X-Org-Id", "org-demo")
	enrollReq.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	enrollReq.AddCookie(session.accessCookie)
	enrollReq.AddCookie(session.csrfCookie)
	rrEnroll := httptest.NewRecorder()
	router.ServeHTTP(rrEnroll, enrollReq)
	if rrEnroll.Code != http.StatusCreated {
		t.Fatalf("enroll mfa: expected 201, got %d: %s", rrEnroll.Code, rrEnroll.Body.String())
	}

	var enrollResp struct {
		Factor struct {
			ID    string   `json:"id"`
			Codes []string `json:"codes"`
		} `json:"factor"`
	}
	if err := json.Unmarshal(rrEnroll.Body.Bytes(), &enrollResp); err != nil {
		t.Fatalf("decode enroll response: %v", err)
	}
	if len(enrollResp.Factor.Codes) == 0 {
		t.Fatalf("enroll mfa: expected backup codes in response, got none")
	}

	challengeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/challenge", encodeJSONBody(t, map[string]any{
		"factor_id":      enrollResp.Factor.ID,
		"challenge_code": enrollResp.Factor.Codes[0],
	}))
	challengeReq.Header.Set("Content-Type", "application/json")
	challengeReq.Header.Set("X-Org-Id", "org-demo")
	challengeReq.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	challengeReq.AddCookie(session.accessCookie)
	challengeReq.AddCookie(session.csrfCookie)
	rrChallenge := httptest.NewRecorder()
	router.ServeHTTP(rrChallenge, challengeReq)
	if rrChallenge.Code != http.StatusOK {
		t.Fatalf("challenge mfa: expected 200, got %d: %s", rrChallenge.Code, rrChallenge.Body.String())
	}

	found := map[string]*http.Cookie{}
	for _, cookie := range rrChallenge.Result().Cookies() {
		found[cookie.Name] = cookie
	}
	if found[appmiddleware.AccessTokenCookieName] == nil || found[appmiddleware.RefreshTokenCookieName] == nil || found[appmiddleware.CSRFTokenCookieName] == nil {
		t.Fatalf("expected replacement session cookies, got %#v", found)
	}
}

func issueSessionWithCookies(t *testing.T, router http.Handler) issuedSessionCookies {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, map[string]any{
		"user_id": "cookie-user",
		"org_id":  "org-demo",
		"roles":   []string{"admin"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issue session: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var out issuedSessionCookies
	for _, cookie := range rr.Result().Cookies() {
		switch cookie.Name {
		case appmiddleware.AccessTokenCookieName:
			out.accessCookie = cookie
		case appmiddleware.RefreshTokenCookieName:
			out.refreshCookie = cookie
		case appmiddleware.CSRFTokenCookieName:
			out.csrfCookie = cookie
		}
	}
	if out.accessCookie == nil || out.refreshCookie == nil || out.csrfCookie == nil {
		t.Fatal("expected access, refresh, and csrf cookies to be issued")
	}
	return out
}

func assertSessionCookiesCleared(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	found := map[string]*http.Cookie{}
	for _, cookie := range rr.Result().Cookies() {
		found[cookie.Name] = cookie
	}

	for _, name := range []string{
		appmiddleware.AccessTokenCookieName,
		appmiddleware.RefreshTokenCookieName,
		appmiddleware.CSRFTokenCookieName,
	} {
		cookie := found[name]
		if cookie == nil {
			t.Fatalf("expected %s clear cookie to be set", name)
		}
		if cookie.MaxAge >= 0 {
			t.Fatalf("expected %s clear cookie MaxAge < 0, got %d", name, cookie.MaxAge)
		}
	}
}

func headerListContains(headerValue, expected string) bool {
	for _, part := range strings.Split(headerValue, ",") {
		if strings.EqualFold(strings.TrimSpace(part), expected) {
			return true
		}
	}
	return false
}

// failingCSRFConfig returns a config whose router can never mint a CSRF token.
// The factory lives on this router's config, so parallel tests are unaffected.
func failingCSRFConfig() Config {
	cfg := Config{AppEnv: "test"}
	cfg.Auth.CSRFTokenFactory = func() (string, error) { return "", errors.New("rng unavailable") }
	return cfg
}
