package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func idemKey(t *testing.T, orgID, header string) (string, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if header != "" {
		req.Header.Set(stripeIdempotencyHeader, header)
	}
	rr := httptest.NewRecorder()
	key, ok := stripeIdempotencyKeyFromRequest(rr, req, orgID)
	if ok {
		return key, http.StatusOK
	}
	return "", rr.Code
}

// Tenancy tracker item 4.5: the Stripe idempotency key is namespaced by org so
// two clubs on one Stripe account can never replay each other's requests.
func TestStripeIdempotencyKeyIsOrgScoped(t *testing.T) {
	a, _ := idemKey(t, "org-a", "pay-123")
	b, _ := idemKey(t, "org-b", "pay-123")
	if a == b || !strings.HasPrefix(a, "org-a:") || !strings.HasPrefix(b, "org-b:") {
		t.Fatalf("keys must differ per org: %q %q", a, b)
	}
	if _, code := idemKey(t, "org-a", ""); code != http.StatusBadRequest {
		t.Fatalf("missing key: %d", code)
	}
	if _, code := idemKey(t, "org-a", strings.Repeat("x", 129)); code != http.StatusBadRequest {
		t.Fatalf("oversized key: %d", code)
	}
}
