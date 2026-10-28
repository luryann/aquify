package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFProtectionBearerRequestWithStrayCookiesIsAllowed(t *testing.T) {
	h := CSRFProtection("X-CSRF-Token")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/read", nil)
	req.Header.Set("Authorization", "Bearer stub-admin")
	req.Header.Set("Cookie", "other_app_session=abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("bearer request with unrelated cookies: got %d, want 204", rec.Code)
	}
}

func TestCSRFProtectionCookieRequestWithoutTokenIsRejected(t *testing.T) {
	h := CSRFProtection("X-CSRF-Token")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/read", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "session"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cookie-auth mutation without CSRF: got %d, want 403", rec.Code)
	}
}
