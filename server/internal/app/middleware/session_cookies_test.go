package middleware

import (
	"errors"
	"testing"
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
