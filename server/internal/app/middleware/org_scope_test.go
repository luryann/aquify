package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireOrgScopeRejectsMissingHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req = req.WithContext(context.WithValue(req.Context(), principalContextKey{}, &Principal{UserID: "admin-user", OrgID: "org-demo", Roles: []string{"admin"}}))
	rr := httptest.NewRecorder()

	h := RequireOrgScope(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called when org scope is missing")
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var env authErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if env.Error.Code != "missing_org_scope" {
		t.Fatalf("expected missing_org_scope, got %q", env.Error.Code)
	}
}

func TestRequireOrgScopeRejectsCrossOrgPrincipal(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req = req.WithContext(context.WithValue(req.Context(), principalContextKey{}, &Principal{UserID: "admin-user", OrgID: "org-other", Roles: []string{"admin"}}))
	rr := httptest.NewRecorder()

	h := RequireOrgScope(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called on org mismatch")
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	var env authErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if env.Error.Code != "scope_violation" {
		t.Fatalf("expected scope_violation, got %q", env.Error.Code)
	}
}

func TestRequireOrgScopeStoresNormalizedOrgID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req.Header.Set("X-Org-Id", "  org-demo  ")
	req = req.WithContext(context.WithValue(req.Context(), principalContextKey{}, &Principal{UserID: "admin-user", OrgID: "org-demo", Roles: []string{"admin"}}))
	rr := httptest.NewRecorder()

	h := RequireOrgScope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := OrgIDFromContext(r.Context())
		if !ok {
			t.Fatal("expected org id in context")
		}
		if orgID != "org-demo" {
			t.Fatalf("expected normalized org id org-demo, got %q", orgID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}
