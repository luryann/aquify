package handlers

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/authn"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

const (
	cov4xAuthOrg   = "org-cov4x-auth"
	cov4xAuthKey   = "cov4x-state-signing-key"
	cov4xAuthKid   = "cov4x-kid"
	cov4xAuthClien = "cov4x-client"
)

func cov4xAuthTokens() tokenResolver {
	return tokenResolver{
		"admin":      {UserID: "u-admin", OrgID: cov4xAuthOrg, Roles: []string{"admin"}, MFAAuthenticated: true},
		"coach":      {UserID: "u-coach", OrgID: cov4xAuthOrg, Roles: []string{"coach"}},
		"adminNoMFA": {UserID: "u-admin2", OrgID: cov4xAuthOrg, Roles: []string{"admin"}},
		"user":       {UserID: "u-mfa", Email: "mfa@example.com", OrgID: cov4xAuthOrg, Roles: []string{"parent"}},
		"userMFA":    {UserID: "u-mfa", Email: "mfa@example.com", OrgID: cov4xAuthOrg, Roles: []string{"parent"}, MFAAuthenticated: true},
		"nobodyID":   {UserID: "u-nobody", OrgID: cov4xAuthOrg, Roles: []string{"parent"}},
	}
}

func cov4xAuthRouter(h *AuthHandlers) http.Handler {
	r := chi.NewRouter()
	r.Route("/auth", func(a chi.Router) {
		h.RegisterPublicRoutes(a)
		h.RegisterNoCSRFRoutes(a)
		a.Get("/oidc/authorize", h.OIDCAuthorize)
		a.Post("/oidc/callback", h.OIDCCallback)
		a.Group(func(p chi.Router) {
			p.Use(appmiddleware.Auth(cov4xAuthTokens()))
			p.Use(appmiddleware.RequireOrgScope)
			h.RegisterProtectedRoutes(p)
		})
	})
	return r
}

func cov4xAuthDo(h http.Handler, method, path, token, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cov4xAuthOrg)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func cov4xCookie(rr *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func cov4xTOTP(secret string, at time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		panic(err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code)
}

func cov4xDevAuth(t *testing.T) (*authn.Service, *AuthHandlers) {
	t.Helper()
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true})
	h := NewAuthHandlersWithAudit(svc, audit.NewService(), true)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true})
	return svc, h
}

func TestCov4xAuthProtectedRoutesRequirePrincipal(t *testing.T) {
	t.Parallel()
	svc := authn.NewService(authn.Config{})
	h := NewAuthHandlers(svc, false)
	router := cov4xBareRouter(h.RegisterProtectedRoutes)
	for _, rt := range []struct{ method, path string }{
		{"GET", "/session"}, {"GET", "/mfa/factors"}, {"POST", "/mfa/enroll"}, {"POST", "/mfa/challenge"},
		{"GET", "/roles/assignments"}, {"POST", "/roles/assign"},
	} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`)))
		if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "unauthorized" {
			t.Errorf("%s %s: want 401 unauthorized, got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
	}
	// The dev-only issue route is not mounted unless explicitly allowed.
	pub := cov4xBareRouter(h.RegisterPublicRoutes)
	rr := httptest.NewRecorder()
	pub.ServeHTTP(rr, httptest.NewRequest("POST", "/sessions/issue", strings.NewReader(`{}`)))
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("issue route must not be mounted when disallowed, got %d", rr.Code)
	}
}

func TestCov4xAuthSessionAndRoleEndpoints(t *testing.T) {
	t.Parallel()
	_, h := cov4xDevAuth(t)
	router := cov4xAuthRouter(h)

	rr := cov4xAuthDo(router, "GET", "/auth/session", "admin", "")
	expectStatus(t, rr, http.StatusOK, "session")
	if !strings.Contains(rr.Body.String(), `"user_id":"u-admin"`) || !strings.Contains(rr.Body.String(), `"mfa_authenticated":true`) {
		t.Fatalf("unexpected session payload: %s", rr.Body.String())
	}

	rr = cov4xAuthDo(router, "GET", "/auth/mfa/factors", "user", "")
	expectStatus(t, rr, http.StatusOK, "mfa factors")
	if !strings.Contains(rr.Body.String(), `"count":0`) {
		t.Fatalf("expected no factors: %s", rr.Body.String())
	}

	// Role visibility is admin-only.
	expectStatus(t, cov4xAuthDo(router, "GET", "/auth/roles/assignments", "coach", ""), http.StatusOK-200+http.StatusForbidden, "coach list assignments")
	rr = cov4xAuthDo(router, "GET", "/auth/roles/assignments", "admin", "")
	expectStatus(t, rr, http.StatusOK, "admin list assignments")

	// Assigning needs the admin permission; coach is refused, malformed bodies are 400.
	body := `{"org_id":"` + cov4xAuthOrg + `","target_user_id":"u-x","role":"coach","reason":"promotion"}`
	rr = cov4xAuthDo(router, "POST", "/auth/roles/assign", "coach", body)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "forbidden" {
		t.Fatalf("coach assign: want 403 forbidden, got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/roles/assign", "admin", `{}`)
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "validation_failed" {
		t.Fatalf("empty assign: want 400, got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/roles/assign", "adminNoMFA", body)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "mfa_required" {
		t.Fatalf("admin without MFA: want 403 mfa_required, got %d %s", rr.Code, rr.Body.String())
	}
	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"body org mismatch", `{"org_id":"other-org","target_user_id":"u-x","role":"coach","reason":"r"}`, 403, "scope_violation"},
		{"unknown role", `{"org_id":"` + cov4xAuthOrg + `","target_user_id":"u-x","role":"janitor","reason":"r"}`, 400, "invalid_role"},
		{"blank reason", `{"org_id":"` + cov4xAuthOrg + `","target_user_id":"u-x","role":"coach","reason":"  "}`, 400, "invalid_role_assignment"},
	} {
		rr = cov4xAuthDo(router, "POST", "/auth/roles/assign", "admin", tc.body)
		if rr.Code != tc.status || cov4xErrCode(t, rr) != tc.code {
			t.Errorf("%s: want %d %s, got %d %s", tc.name, tc.status, tc.code, rr.Code, rr.Body.String())
		}
	}
	rr = cov4xAuthDo(router, "POST", "/auth/roles/assign", "admin", body)
	expectStatus(t, rr, http.StatusCreated, "admin assign")
	rr = cov4xAuthDo(router, "GET", "/auth/roles/assignments", "admin", "")
	if !strings.Contains(rr.Body.String(), `"target_user_id":"u-x"`) {
		t.Fatalf("assignment not listed: %s", rr.Body.String())
	}
}

func TestCov4xAuthMFAFlow(t *testing.T) {
	t.Parallel()
	_, h := cov4xDevAuth(t)
	router := cov4xAuthRouter(h)

	rr := cov4xAuthDo(router, "POST", "/auth/mfa/enroll", "user", `{"factor_type":"totp","label":"phone"}`)
	expectStatus(t, rr, http.StatusCreated, "enroll totp")
	var enrolled struct {
		Factor struct {
			ID            string `json:"id"`
			ChallengeHint string `json:"challenge_hint"`
		} `json:"factor"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &enrolled); err != nil || enrolled.Factor.ID == "" || enrolled.Factor.ChallengeHint == "" {
		t.Fatalf("enroll response missing factor/hint: %s", rr.Body.String())
	}
	factorID, secret := enrolled.Factor.ID, enrolled.Factor.ChallengeHint

	rr = cov4xAuthDo(router, "POST", "/auth/mfa/enroll", "nobodyID", `{"factor_type":"sms"}`)
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "validation_failed" {
		t.Fatalf("unsupported factor type: got %d %s", rr.Code, rr.Body.String())
	}

	rr = cov4xAuthDo(router, "GET", "/auth/mfa/factors", "user", "")
	if !strings.Contains(rr.Body.String(), `"count":1`) || !strings.Contains(rr.Body.String(), factorID) {
		t.Fatalf("factor should be listed: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), secret) {
		t.Fatalf("listing must never leak the TOTP secret: %s", rr.Body.String())
	}

	challenge := func(factor, code string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"factor_id": factor, "challenge_code": code})
		return cov4xAuthDo(router, "POST", "/auth/mfa/challenge", "user", string(b))
	}
	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"missing fields", `{}`, 400, "validation_failed"},
	} {
		rr := cov4xAuthDo(router, "POST", "/auth/mfa/challenge", "user", tc.body)
		if rr.Code != tc.status || cov4xErrCode(t, rr) != tc.code {
			t.Errorf("%s: want %d %s, got %d %s", tc.name, tc.status, tc.code, rr.Code, rr.Body.String())
		}
	}
	rr = challenge("mfa_unknown", "123456")
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "invalid_mfa_factor" {
		t.Fatalf("unknown factor: got %d %s", rr.Code, rr.Body.String())
	}
	rr = challenge(factorID, "000000")
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "invalid_mfa_code" {
		t.Fatalf("wrong code: got %d %s", rr.Code, rr.Body.String())
	}

	// A correct code verifies the factor and rotates to an MFA session.
	code := cov4xTOTP(secret, time.Now())
	rr = challenge(factorID, code)
	expectStatus(t, rr, http.StatusOK, "correct totp")
	if !strings.Contains(rr.Body.String(), `"status":"verified"`) || strings.Contains(rr.Body.String(), "atk_") {
		t.Fatalf("unexpected verify body (tokens must stay in cookies): %s", rr.Body.String())
	}
	if cov4xCookie(rr, appmiddleware.AccessTokenCookieName) == nil || cov4xCookie(rr, appmiddleware.CSRFTokenCookieName) == nil {
		t.Fatal("expected access and csrf cookies on verified challenge")
	}
	// The same code cannot be replayed.
	rr = challenge(factorID, code)
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "invalid_mfa_code" {
		t.Fatalf("replayed code: got %d %s", rr.Code, rr.Body.String())
	}

	// With a verified factor, a non-MFA session may not add another.
	rr = cov4xAuthDo(router, "POST", "/auth/mfa/enroll", "user", `{"factor_type":"backup_code"}`)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "mfa_required" {
		t.Fatalf("enroll without mfa: got %d %s", rr.Code, rr.Body.String())
	}
	expectStatus(t, cov4xAuthDo(router, "POST", "/auth/mfa/enroll", "userMFA", `{"factor_type":"backup_code"}`), http.StatusCreated, "enroll with mfa")
}

func TestCov4xAuthMFALockoutAndCSRFFailure(t *testing.T) {
	t.Parallel()
	_, h := cov4xDevAuth(t)
	router := cov4xAuthRouter(h)
	rr := cov4xAuthDo(router, "POST", "/auth/mfa/enroll", "user", `{"factor_type":"totp"}`)
	expectStatus(t, rr, http.StatusCreated, "enroll")
	var enrolled struct {
		Factor struct {
			ID            string `json:"id"`
			ChallengeHint string `json:"challenge_hint"`
		} `json:"factor"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &enrolled)

	b, _ := json.Marshal(map[string]string{"factor_id": enrolled.Factor.ID, "challenge_code": "000000"})
	for i := 0; i < 5; i++ {
		rr = cov4xAuthDo(router, "POST", "/auth/mfa/challenge", "user", string(b))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i, rr.Code)
		}
	}
	rr = cov4xAuthDo(router, "POST", "/auth/mfa/challenge", "user", string(b))
	if rr.Code != http.StatusTooManyRequests || cov4xErrCode(t, rr) != "mfa_locked" {
		t.Fatalf("locked factor: want 429 mfa_locked, got %d %s", rr.Code, rr.Body.String())
	}

	// A CSRF factory failure after a good code is a 500 and issues no cookies.
	_, h2 := cov4xDevAuth(t)
	h2.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true, NewCSRFToken: func() (string, error) { return "", errors.New("entropy exhausted") }})
	router2 := cov4xAuthRouter(h2)
	rr = cov4xAuthDo(router2, "POST", "/auth/mfa/enroll", "user", `{"factor_type":"totp"}`)
	_ = json.Unmarshal(rr.Body.Bytes(), &enrolled)
	b, _ = json.Marshal(map[string]string{"factor_id": enrolled.Factor.ID, "challenge_code": cov4xTOTP(enrolled.Factor.ChallengeHint, time.Now())})
	rr = cov4xAuthDo(router2, "POST", "/auth/mfa/challenge", "user", string(b))
	if rr.Code != http.StatusInternalServerError || cov4xErrCode(t, rr) != "csrf_generation_failed" {
		t.Fatalf("csrf failure: got %d %s", rr.Code, rr.Body.String())
	}
	if cov4xCookie(rr, appmiddleware.AccessTokenCookieName) != nil {
		t.Fatal("no session cookie may be issued when the CSRF token cannot be minted")
	}
}

func TestCov4xAuthSessionLifecycleInMemory(t *testing.T) {
	t.Parallel()
	_, h := cov4xDevAuth(t)
	router := cov4xAuthRouter(h)

	issueBody := `{"user_id":"u-life","org_id":"` + cov4xAuthOrg + `","roles":["parent"]}`
	rr := cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", issueBody)
	expectStatus(t, rr, http.StatusCreated, "issue")
	refresh := cov4xCookie(rr, appmiddleware.RefreshTokenCookieName)
	if refresh == nil || strings.Contains(rr.Body.String(), refresh.Value) {
		t.Fatalf("refresh token must be a cookie only: %s", rr.Body.String())
	}

	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"empty body", `{}`, 400, "validation_failed"},
		{"blank user id", `{"user_id":"  ","org_id":"` + cov4xAuthOrg + `","roles":["parent"]}`, 400, "invalid_session_request"},
	} {
		rr := cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", tc.body)
		if rr.Code != tc.status || cov4xErrCode(t, rr) != tc.code {
			t.Errorf("issue %s: want %d %s, got %d %s", tc.name, tc.status, tc.code, rr.Code, rr.Body.String())
		}
	}

	// Refresh by cookie rotates; the old token is then revoked.
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", "", refresh)
	expectStatus(t, rr, http.StatusOK, "refresh")
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", "", refresh)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("rotated token reuse: want 401, got %d %s", rr.Code, rr.Body.String())
	}
	if c := cov4xErrCode(t, rr); c != "session_not_found" && c != "session_revoked" {
		t.Fatalf("rotated token reuse code = %q", c)
	}

	// Unknown tokens, empty bodies and malformed bodies are rejected distinctly.
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", `{"refresh_token":"rtk_unknown"}`)
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "session_not_found" {
		t.Fatalf("unknown token: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", `{}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty refresh body: got %d %s", rr.Code, rr.Body.String())
	}

	// A revoked-then-refreshed session reports session_revoked.
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", issueBody)
	tok := cov4xCookie(rr, appmiddleware.RefreshTokenCookieName).Value
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/revoke", "", `{"refresh_token":"`+tok+`"}`)
	expectStatus(t, rr, http.StatusNoContent, "revoke")
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", `{"refresh_token":"`+tok+`"}`)
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "session_revoked" {
		t.Fatalf("revoked token: got %d %s", rr.Code, rr.Body.String())
	}

	// Malformed bodies on revoke/logout answer 400; revoke clears cookies first.
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/revoke", "", `{not json`)
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "invalid_request_body" {
		t.Fatalf("revoke malformed: got %d %s", rr.Code, rr.Body.String())
	}
	if c := cov4xCookie(rr, appmiddleware.AccessTokenCookieName); c == nil || c.MaxAge >= 0 {
		t.Fatalf("revoke must clear the access cookie even on error: %+v", c)
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/logout", "", `{not json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("logout malformed: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/logout", "", ``)
	expectStatus(t, rr, http.StatusOK, "logout without tokens")
	if !strings.Contains(rr.Body.String(), `"redirect_url":"/login"`) {
		t.Fatalf("logout body: %s", rr.Body.String())
	}

	// Revoke and logout honour the refresh cookie without a body.
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", issueBody)
	c1 := cov4xCookie(rr, appmiddleware.RefreshTokenCookieName)
	expectStatus(t, cov4xAuthDo(router, "POST", "/auth/sessions/revoke", "", "", c1), http.StatusNoContent, "revoke by cookie")
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", "", c1)
	if cov4xErrCode(t, rr) != "session_revoked" {
		t.Fatalf("cookie-revoked session: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", issueBody)
	c2 := cov4xCookie(rr, appmiddleware.RefreshTokenCookieName)
	expectStatus(t, cov4xAuthDo(router, "POST", "/auth/sessions/logout", "", "", c2), http.StatusOK, "logout by cookie")
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", "", c2)
	if cov4xErrCode(t, rr) != "session_revoked" {
		t.Fatalf("logged-out session: got %d %s", rr.Code, rr.Body.String())
	}

	// Refresh keeps an existing CSRF cookie value instead of rotating it.
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", issueBody)
	tok = cov4xCookie(rr, appmiddleware.RefreshTokenCookieName).Value
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", `{"refresh_token":"`+tok+`"}`,
		&http.Cookie{Name: appmiddleware.CSRFTokenCookieName, Value: "keep-me"})
	expectStatus(t, rr, http.StatusOK, "refresh with csrf cookie")
	if !strings.Contains(rr.Body.String(), `"csrf_token":"keep-me"`) {
		t.Fatalf("csrf token should be preserved: %s", rr.Body.String())
	}
}

func TestCov4xAuthCSRFFailuresAbortIssueAndRefresh(t *testing.T) {
	t.Parallel()
	_, h := cov4xDevAuth(t)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true, NewCSRFToken: func() (string, error) { return "", errors.New("entropy exhausted") }})
	router := cov4xAuthRouter(h)
	rr := cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", `{"user_id":"u-c","org_id":"`+cov4xAuthOrg+`","roles":["parent"]}`)
	if rr.Code != http.StatusInternalServerError || cov4xErrCode(t, rr) != "csrf_generation_failed" {
		t.Fatalf("issue: got %d %s", rr.Code, rr.Body.String())
	}
	if cov4xCookie(rr, appmiddleware.AccessTokenCookieName) != nil {
		t.Fatal("no cookies may be set when csrf generation fails")
	}
	// A session minted directly can still not be refreshed without a CSRF token.
	tokens, err := h.service.IssueSession(authn.SessionInput{UserID: "u-c", OrgID: cov4xAuthOrg, Roles: []string{"parent"}})
	if err != nil {
		t.Fatal(err)
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", `{"refresh_token":"`+tokens.RefreshToken+`"}`)
	if rr.Code != http.StatusInternalServerError || cov4xErrCode(t, rr) != "csrf_generation_failed" {
		t.Fatalf("refresh: got %d %s", rr.Code, rr.Body.String())
	}
}

// With the session store unreachable, refresh asks the client to retry
// instead of signing it out, and revocation reports failure.
func TestCov4xAuthSessionStoreOutage(t *testing.T) {
	t.Parallel()
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true})
	svc.SetPool(testutil.ClosedPool(t))
	h := NewAuthHandlersWithAudit(svc, nil, true)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true})
	router := cov4xAuthRouter(h)

	rr := cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", `{"refresh_token":"rtk_abc"}`)
	if rr.Code != http.StatusServiceUnavailable || cov4xErrCode(t, rr) != "service_unavailable" || rr.Header().Get("Retry-After") != "1" {
		t.Fatalf("refresh outage: got %d %s retry=%q", rr.Code, rr.Body.String(), rr.Header().Get("Retry-After"))
	}
	if strings.Contains(rr.Body.String(), "127.0.0.1") {
		t.Fatalf("outage cause must not reach the client: %s", rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/revoke", "", `{"refresh_token":"rtk_abc"}`)
	if rr.Code != http.StatusServiceUnavailable || cov4xErrCode(t, rr) != "session_revocation_failed" {
		t.Fatalf("revoke outage: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/logout", "", `{"refresh_token":"rtk_abc"}`)
	if rr.Code != http.StatusServiceUnavailable || cov4xErrCode(t, rr) != "session_revocation_failed" {
		t.Fatalf("logout outage: got %d %s", rr.Code, rr.Body.String())
	}
	// MFA writes/reads that need the store fail loudly too.
	rr = cov4xAuthDo(router, "POST", "/auth/mfa/enroll", "user", `{"factor_type":"totp"}`)
	if rr.Code != http.StatusInternalServerError || cov4xErrCode(t, rr) != "mfa_enroll_failed" {
		t.Fatalf("enroll outage: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/mfa/challenge", "user", `{"factor_id":"mfa_x","challenge_code":"123456"}`)
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "mfa_challenge_failed" {
		t.Fatalf("challenge outage: got %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov4xAuthDatabaseBackedSessions(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true})
	svc.SetPool(pool)
	h := NewAuthHandlersWithAudit(svc, audit.NewService(), true)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true})
	router := cov4xAuthRouter(h)
	ctx := context.Background()

	orgA, orgB, suspended := uuid.NewString(), uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, pool, orgA, orgB, suspended)
	if _, err := pool.Exec(ctx, `UPDATE orgs SET status = 'suspended' WHERE id = $1::uuid`, suspended); err != nil {
		t.Fatal(err)
	}
	user := uuid.NewString()
	issue := func(org string) *httptest.ResponseRecorder {
		return cov4xAuthDo(router, "POST", "/auth/sessions/issue", "", `{"user_id":"`+user+`","org_id":"`+org+`","roles":["parent"]}`)
	}

	rr := issue(suspended)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "org_unavailable" {
		t.Fatalf("suspended org: got %d %s", rr.Code, rr.Body.String())
	}
	rr = issue(orgA)
	expectStatus(t, rr, http.StatusCreated, "issue in org A")
	refresh := cov4xCookie(rr, appmiddleware.RefreshTokenCookieName)
	rr = issue(orgB)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "org_mismatch" {
		t.Fatalf("user moved to another org: got %d %s", rr.Code, rr.Body.String())
	}

	// An expired session cannot be refreshed.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	rr = cov4xAuthDo(router, "POST", "/auth/sessions/refresh", "", "", refresh)
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "session_expired" {
		t.Fatalf("expired session: got %d %s", rr.Code, rr.Body.String())
	}
}

// --- OIDC ---

type cov4xProvider struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	mu   sync.Mutex
	next func() map[string]any // claims for the next token response; nil => 500
}

func cov4xNewProvider(t *testing.T) *cov4xProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &cov4xProvider{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		next := p.next
		p.mu.Unlock()
		if next == nil {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "provider-access", "token_type": "Bearer", "expires_in": 3600,
			"id_token": p.sign(t, next()),
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": cov4xAuthKid, "kty": "RSA", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *cov4xProvider) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	hb, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": cov4xAuthKid, "typ": "JWT"})
	cb, _ := json.Marshal(claims)
	in := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return in + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (p *cov4xProvider) config() authn.OIDCConfig {
	return authn.OIDCConfig{
		AuthorizeURL: p.srv.URL + "/auth", TokenURL: p.srv.URL + "/token", JWKSURL: p.srv.URL + "/jwks",
		RedirectURL: "http://localhost:3000/callback", ClientID: cov4xAuthClien, ClientSecret: "secret",
		ExpectedIssuer: p.srv.URL, StateSigningKey: cov4xAuthKey,
	}
}

func (p *cov4xProvider) claims(nonce, sub, org string, roles []string) map[string]any {
	c := map[string]any{
		"sub": sub, "email": "oidc@example.com", "name": "Oidc User", "iss": p.srv.URL,
		"aud": []string{cov4xAuthClien}, "exp": time.Now().Add(10 * time.Minute).Unix(),
		"iat": time.Now().Add(-time.Minute).Unix(), "nonce": nonce, "org_id": org,
	}
	if roles != nil {
		c["roles"] = roles
	}
	return c
}

// begin runs the authorize step and returns the signed state and its binding cookie.
func cov4xBeginOIDC(t *testing.T, router http.Handler) (string, *http.Cookie) {
	t.Helper()
	rr := cov4xAuthDo(router, "GET", "/auth/oidc/authorize?login_hint=a%40b.c&org_id=x", "", "")
	if rr.Code != http.StatusFound {
		t.Fatalf("authorize: want 302, got %d %s", rr.Code, rr.Body.String())
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := loc.Query().Get("state")
	cookie := cov4xCookie(rr, "aquify_oidc_state")
	if state == "" || cookie == nil || !cookie.HttpOnly || cookie.MaxAge != 600 || cookie.Path != "/api/v1/auth/oidc" {
		t.Fatalf("authorize must bind state to the browser: state=%q cookie=%+v", state, cookie)
	}
	if loc.Query().Get("login_hint") != "a@b.c" || loc.Query().Get("client_id") == "" {
		t.Fatalf("authorize redirect missing params: %s", loc)
	}
	return state, cookie
}

func cov4xCallback(router http.Handler, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"code": "auth-code", "state": state})
	if cookie == nil {
		return cov4xAuthDo(router, "POST", "/auth/oidc/callback", "", string(body))
	}
	return cov4xAuthDo(router, "POST", "/auth/oidc/callback", "", string(body), cookie)
}

func TestCov4xOIDCAuthorizeErrors(t *testing.T) {
	t.Parallel()
	h := NewAuthHandlers(authn.NewService(authn.Config{}), false)
	rr := cov4xAuthDo(cov4xAuthRouter(h), "GET", "/auth/oidc/authorize", "", "")
	if rr.Code != http.StatusServiceUnavailable || cov4xErrCode(t, rr) != "oidc_not_configured" {
		t.Fatalf("unconfigured authorize: got %d %s", rr.Code, rr.Body.String())
	}
	h = NewAuthHandlers(authn.NewService(authn.Config{OIDC: authn.OIDCConfig{
		AuthorizeURL: "http://[::1", RedirectURL: "http://x/cb", ClientID: "c", StateSigningKey: "k",
	}}), false)
	rr = cov4xAuthDo(cov4xAuthRouter(h), "GET", "/auth/oidc/authorize", "", "")
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "invalid_oidc_authorize" {
		t.Fatalf("bad authorize url: got %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov4xOIDCCallbackOutcomes(t *testing.T) {
	prov := cov4xNewProvider(t)
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true, OIDC: prov.config()})
	h := NewAuthHandlersWithAudit(svc, nil, false)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true})
	router := cov4xAuthRouter(h)

	run := func(roles []string, org string, mutate func(map[string]any)) *httptest.ResponseRecorder {
		state, cookie := cov4xBeginOIDC(t, router)
		nonce, err := svc.OIDCStateNonce(state)
		if err != nil {
			t.Fatal(err)
		}
		prov.mu.Lock()
		prov.next = func() map[string]any {
			c := prov.claims(nonce, "oidc-sub", org, roles)
			if mutate != nil {
				mutate(c)
			}
			return c
		}
		prov.mu.Unlock()
		return cov4xCallback(router, state, cookie)
	}

	// Happy path.
	rr := run([]string{"parent", "offline_access"}, cov4xAuthOrg, nil)
	expectStatus(t, rr, http.StatusOK, "callback")
	var ok struct {
		User struct {
			UserID string   `json:"user_id"`
			OrgID  string   `json:"org_id"`
			Roles  []string `json:"roles"`
		} `json:"user"`
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &ok); err != nil {
		t.Fatal(err)
	}
	if ok.User.UserID != "oidc-sub" || ok.User.OrgID != cov4xAuthOrg || len(ok.User.Roles) != 1 || ok.User.Roles[0] != "parent" || ok.CSRF == "" {
		t.Fatalf("unexpected callback result: %s", rr.Body.String())
	}
	if cov4xCookie(rr, appmiddleware.AccessTokenCookieName) == nil {
		t.Fatal("callback must set the session cookie")
	}
	if c := cov4xCookie(rr, "aquify_oidc_state"); c == nil || c.MaxAge >= 0 {
		t.Fatalf("binding cookie must be cleared: %+v", c)
	}

	// Failures. Each one still clears the single-use binding cookie.
	cases := []struct {
		name   string
		roles  []string
		mutate func(map[string]any)
		status int
		code   string
	}{
		{"no application role", []string{"offline_access"}, nil, 403, "no_app_role"},
		{"nonce mismatch", []string{"parent"}, func(c map[string]any) { c["nonce"] = "other" }, 400, "invalid_oidc_callback"},
	}
	for _, tc := range cases {
		rr := run(tc.roles, cov4xAuthOrg, tc.mutate)
		if rr.Code != tc.status || cov4xErrCode(t, rr) != tc.code {
			t.Errorf("%s: want %d %s, got %d %s", tc.name, tc.status, tc.code, rr.Code, rr.Body.String())
		}
		if c := cov4xCookie(rr, "aquify_oidc_state"); c == nil || c.MaxAge >= 0 {
			t.Errorf("%s: binding cookie not cleared", tc.name)
		}
	}

	// A provider outage is a 503 with no upstream details.
	state, cookie := cov4xBeginOIDC(t, router)
	prov.mu.Lock()
	prov.next = nil
	prov.mu.Unlock()
	rr = cov4xCallback(router, state, cookie)
	if rr.Code != http.StatusServiceUnavailable || cov4xErrCode(t, rr) != "oidc_provider_unavailable" || strings.Contains(rr.Body.String(), prov.srv.URL) {
		t.Fatalf("provider outage: got %d %s", rr.Code, rr.Body.String())
	}

	// State not bound to this browser (no cookie / wrong cookie) is refused outright.
	state, cookie = cov4xBeginOIDC(t, router)
	rr = cov4xCallback(router, state, nil)
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "invalid_oidc_state" {
		t.Fatalf("unbound state: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xCallback(router, state, &http.Cookie{Name: cookie.Name, Value: "someone-elses-nonce"})
	if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "invalid_oidc_state" {
		t.Fatalf("foreign binding: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov4xAuthDo(router, "POST", "/auth/oidc/callback", "", `{}`)
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "validation_failed" {
		t.Fatalf("empty callback body: got %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov4xOIDCCallbackCSRFFailure(t *testing.T) {
	prov := cov4xNewProvider(t)
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true, OIDC: prov.config()})
	h := NewAuthHandlersWithAudit(svc, nil, false)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true, NewCSRFToken: func() (string, error) { return "", errors.New("entropy exhausted") }})
	router := cov4xAuthRouter(h)
	state, cookie := cov4xBeginOIDC(t, router)
	nonce, _ := svc.OIDCStateNonce(state)
	prov.next = func() map[string]any { return prov.claims(nonce, "oidc-sub", cov4xAuthOrg, []string{"parent"}) }
	rr := cov4xCallback(router, state, cookie)
	if rr.Code != http.StatusInternalServerError || cov4xErrCode(t, rr) != "csrf_generation_failed" {
		t.Fatalf("csrf failure: got %d %s", rr.Code, rr.Body.String())
	}
	if cov4xCookie(rr, appmiddleware.AccessTokenCookieName) != nil {
		t.Fatal("no session cookie when the CSRF token cannot be minted")
	}
}

// Authorize works with only the authorize settings, but the callback then
// reports that token exchange is not configured.
func TestCov4xOIDCCallbackNotConfigured(t *testing.T) {
	t.Parallel()
	svc := authn.NewService(authn.Config{OIDC: authn.OIDCConfig{
		AuthorizeURL: "https://idp.test/auth", RedirectURL: "http://localhost/cb", ClientID: "c", StateSigningKey: cov4xAuthKey,
	}})
	h := NewAuthHandlers(svc, false)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true})
	router := cov4xAuthRouter(h)
	state, cookie := cov4xBeginOIDC(t, router)
	rr := cov4xCallback(router, state, cookie)
	if rr.Code != http.StatusServiceUnavailable || cov4xErrCode(t, rr) != "oidc_not_configured" {
		t.Fatalf("not configured: got %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov4xOIDCCallbackOrgRejections(t *testing.T) {
	prov := cov4xNewProvider(t)
	pool := testutil.IsolatedSchemaPool(t)
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true, OIDC: prov.config()})
	svc.SetPool(pool)
	h := NewAuthHandlersWithAudit(svc, nil, false)
	h.SetSessionCookies(appmiddleware.SessionCookies{Insecure: true})
	router := cov4xAuthRouter(h)
	ctx := context.Background()

	orgA, orgB, suspended := uuid.NewString(), uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, pool, orgA, orgB, suspended)
	if _, err := pool.Exec(ctx, `UPDATE orgs SET status = 'suspended' WHERE id = $1::uuid`, suspended); err != nil {
		t.Fatal(err)
	}
	sub := uuid.NewString()
	signIn := func(org string) *httptest.ResponseRecorder {
		state, cookie := cov4xBeginOIDC(t, router)
		nonce, _ := svc.OIDCStateNonce(state)
		prov.mu.Lock()
		prov.next = func() map[string]any { return prov.claims(nonce, sub, org, []string{"parent"}) }
		prov.mu.Unlock()
		return cov4xCallback(router, state, cookie)
	}
	expectStatus(t, signIn(orgA), http.StatusOK, "first sign-in")
	rr := signIn(orgB)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "org_mismatch" {
		t.Fatalf("org mismatch: got %d %s", rr.Code, rr.Body.String())
	}
	rr = signIn(suspended)
	if rr.Code != http.StatusForbidden || cov4xErrCode(t, rr) != "org_unavailable" {
		t.Fatalf("suspended org: got %d %s", rr.Code, rr.Body.String())
	}
}

// A factor that cannot be persisted is reported as a 400
// and nothing is retained in memory.
func TestCov4xAuthEnrollPersistenceFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true})
	svc.SetPool(pool)
	h := NewAuthHandlers(svc, false)
	org, user := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	for _, stmt := range []string{
		`CREATE FUNCTION cov4x_refuse_factor() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov4x: factor store refused'; END $$`,
		`CREATE TRIGGER cov4x_refuse_factor BEFORE INSERT ON mfa_factors FOR EACH ROW EXECUTE FUNCTION cov4x_refuse_factor()`,
	} {
		if _, err := pool.Exec(context.Background(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	router := cov4xRouter(tokenResolver{"u": {UserID: user, OrgID: org, Roles: []string{"parent"}}}, h.RegisterProtectedRoutes)
	req := httptest.NewRequest("POST", "/mfa/enroll", strings.NewReader(`{"factor_type":"totp"}`))
	req.Header.Set("Authorization", "Bearer u")
	req.Header.Set("X-Org-Id", org)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "invalid_mfa_factor" {
		t.Fatalf("persist failure: got %d %s", rr.Code, rr.Body.String())
	}
	if got := svc.ListMFAFactors(org, user); len(got) != 0 {
		t.Fatalf("unpersisted factor must not be retained: %+v", got)
	}
}
