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

	h := RejectImpersonationHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestRejectImpersonationHeadersRejectsWhenFeatureDisabled(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, false, &Principal{
		UserID:           "owner-1",
		OrgID:            "org-demo",
		Roles:            []string{"owner"},
		MFAAuthenticated: true,
	}, "target-1", "Support escalation")

	assertImpersonationErrorCode(t, rr, http.StatusForbidden, "impersonation_disabled")
}

func TestRejectImpersonationHeadersRequiresAuthenticatedPrincipal(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, true, nil, "target-1", "Support escalation")

	assertImpersonationErrorCode(t, rr, http.StatusUnauthorized, "unauthorized")
}

func TestRejectImpersonationHeadersRequiresMFA(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, true, &Principal{
		UserID:           "owner-1",
		OrgID:            "org-demo",
		Roles:            []string{"owner"},
		MFAAuthenticated: false,
	}, "target-1", "Support escalation")

	assertImpersonationErrorCode(t, rr, http.StatusForbidden, "mfa_required")
}

func TestRejectImpersonationHeadersRequiresReason(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, true, &Principal{
		UserID:           "owner-1",
		OrgID:            "org-demo",
		Roles:            []string{"owner"},
		MFAAuthenticated: true,
	}, "target-1", "")

	assertImpersonationErrorCode(t, rr, http.StatusForbidden, "impersonation_reason_required")
}

func TestRejectImpersonationHeadersRequiresTargetUserID(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, true, &Principal{
		UserID:           "owner-1",
		OrgID:            "org-demo",
		Roles:            []string{"owner"},
		MFAAuthenticated: true,
	}, "", "Support escalation")

	assertImpersonationErrorCode(t, rr, http.StatusForbidden, "impersonation_target_required")
}

func TestRejectImpersonationHeadersRestrictsToOwners(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, true, &Principal{
		UserID:           "admin-1",
		OrgID:            "org-demo",
		Roles:            []string{"admin"},
		MFAAuthenticated: true,
	}, "target-1", "Support escalation")

	assertImpersonationErrorCode(t, rr, http.StatusForbidden, "forbidden")
}

func TestRejectImpersonationHeadersFailsClosedUntilFeatureIsImplemented(t *testing.T) {
	t.Parallel()

	rr := serveImpersonationRequest(t, true, &Principal{
		UserID:           "owner-1",
		OrgID:            "org-demo",
		Roles:            []string{"owner"},
		MFAAuthenticated: true,
	}, "target-1", "Support escalation")

	assertImpersonationErrorCode(t, rr, http.StatusForbidden, "impersonation_unavailable")
}

func serveImpersonationRequest(t *testing.T, enabled bool, principal *Principal, targetUserID, reason string) *httptest.ResponseRecorder {
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

	h := RejectImpersonationHeaders(enabled)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
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
