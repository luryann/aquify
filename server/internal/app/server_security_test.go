package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

type issuedSessionCookies struct {
	accessCookie  *http.Cookie
	refreshCookie *http.Cookie
	csrfCookie    *http.Cookie
}

func TestCurrentSessionEndpointAcceptsCookieAuthWithoutOrgHeader(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(session.accessCookie)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response struct {
		User struct {
			UserID string `json:"user_id"`
			OrgID  string `json:"org_id"`
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
			ID            string `json:"id"`
			ChallengeHint string `json:"challenge_hint"`
		} `json:"factor"`
	}
	if err := json.Unmarshal(rrEnroll.Body.Bytes(), &enrollResp); err != nil {
		t.Fatalf("decode enroll response: %v", err)
	}

	challengeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/challenge", encodeJSONBody(t, map[string]any{
		"factor_id":      enrollResp.Factor.ID,
		"challenge_code": enrollResp.Factor.ChallengeHint,
	}))
	challengeReq.Header.Set("Content-Type", "application/json")
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
