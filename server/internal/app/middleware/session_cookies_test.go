package middleware

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewCSRFTokenReturnsErrorWhenEntropyUnavailable(t *testing.T) {
	originalRandRead := csrfTokenRandRead
	csrfTokenRandRead = func(_ []byte) (int, error) {
		return 0, errors.New("rng unavailable")
	}
	t.Cleanup(func() {
		csrfTokenRandRead = originalRandRead
	})

	token, err := NewCSRFToken()
	if err == nil {
		t.Fatal("expected csrf token generation to fail when entropy is unavailable")
	}
	if token != "" {
		t.Fatalf("expected no csrf token fallback output, got %q", token)
	}
}

func TestSessionCookiesAreSecureByDefaultAndInsecureOnlyWhenConfigured(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		insecure   bool
		wantSecure bool
	}{{false, true}, {true, false}} {
		rec := httptest.NewRecorder()
		SessionCookies{Insecure: tc.insecure}.Set(rec, "a", "r", "c", time.Now().Add(time.Hour))
		cookies := rec.Result().Cookies()
		if len(cookies) != 3 {
			t.Fatalf("cookies = %d", len(cookies))
		}
		for _, c := range cookies {
			if c.Secure != tc.wantSecure {
				t.Errorf("insecure=%v cookie %s Secure=%v", tc.insecure, c.Name, c.Secure)
			}
		}
		clear := httptest.NewRecorder()
		SessionCookies{Insecure: tc.insecure}.Clear(clear)
		for _, c := range clear.Result().Cookies() {
			if c.Secure != tc.wantSecure {
				t.Errorf("clear insecure=%v cookie %s Secure=%v", tc.insecure, c.Name, c.Secure)
			}
		}
	}
}
