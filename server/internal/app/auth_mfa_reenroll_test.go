package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// An abandoned (never verified) enrollment must not lock the user out: its
// secret was shown once, so a fresh enroll has to be allowed on a plain session.
func TestEnrollMFAAllowsReenrollAfterAbandonedUnverifiedFactor(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test", Auth: AuthConfig{EnableDevStubTokens: true}},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	enroll := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/enroll", encodeJSONBody(t, map[string]any{
			"factor_type": "totp",
			"label":       "Authenticator",
		}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
		req.AddCookie(session.accessCookie)
		req.AddCookie(session.csrfCookie)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	for i := 0; i < 2; i++ {
		if rr := enroll(); rr.Code != http.StatusCreated {
			t.Fatalf("enroll #%d: expected 201, got %d: %s", i+1, rr.Code, rr.Body.String())
		}
	}
}
