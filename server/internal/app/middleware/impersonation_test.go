package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectImpersonationHeadersAllowsRequestsWithoutImpersonationHeaders(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	rr := httptest.NewRecorder()

	h := RejectImpersonationHeaders()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

// Every impersonation attempt is refused with the same 403 regardless of the
// caller's role, MFA state, headers present, or whether a principal exists.
func TestRejectImpersonationHeadersRejectsUnconditionally(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		principal *Principal
		target    string
		reason    string
	}{
		{"admin with mfa and reason", &Principal{UserID: "admin-1", OrgID: "org-demo", Roles: []string{"admin"}, MFAAuthenticated: true}, "target-1", "Support escalation"},
		{"admin without mfa", &Principal{UserID: "admin-1", OrgID: "org-demo", Roles: []string{"admin"}}, "target-1", "Support escalation"},
		{"coach", &Principal{UserID: "coach-1", OrgID: "org-demo", Roles: []string{"coach"}, MFAAuthenticated: true}, "target-1", "Support escalation"},
		{"missing reason", &Principal{UserID: "admin-1", OrgID: "org-demo", Roles: []string{"admin"}, MFAAuthenticated: true}, "target-1", ""},
		{"missing target", &Principal{UserID: "admin-1", OrgID: "org-demo", Roles: []string{"admin"}, MFAAuthenticated: true}, "", "Support escalation"},
		{"anonymous", nil, "target-1", "Support escalation"},
	}
	for _, tc := range cases {
		rr := serveImpersonationRequest(t, tc.principal, tc.target, tc.reason)
		assertImpersonationErrorCode(t, rr, http.StatusForbidden, "impersonation_unavailable")
	}
}

func serveImpersonationRequest(t *testing.T, principal *Principal, targetUserID, reason string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	if targetUserID != "" {
		req.Header.Set("X-Impersonate-User-Id", targetUserID)
	}
	if reason != "" {
		req.Header.Set("X-Impersonate-Reason", reason)
	}
	if principal != nil {
		req = req.WithContext(context.WithValue(req.Context(), principalContextKey{}, principal))
	}
	rr := httptest.NewRecorder()

	h := RejectImpersonationHeaders()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called for impersonation attempts")
	}))
	h.ServeHTTP(rr, req)

	return rr
}

func assertImpersonationErrorCode(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rr.Code != wantStatus {
		t.Fatalf("expected status %d, got %d: %s", wantStatus, rr.Code, rr.Body.String())
	}

	var envelope authErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("expected error code %q, got %q", wantCode, envelope.Error.Code)
	}
}
