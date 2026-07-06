package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type rbacErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func TestRBACStubAllowsAuthorizedRole(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	ctx := context.WithValue(req.Context(), principalContextKey{}, &Principal{
		UserID: "coach-user",
		Roles:  []string{"coach"},
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h := RBACStub("admin", "coach")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestRBACStubRejectsUnauthorizedRole(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	ctx := context.WithValue(req.Context(), principalContextKey{}, &Principal{
		UserID: "parent-user",
		Roles:  []string{"parent"},
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h := RBACStub("admin", "coach")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called for unauthorized role")
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	var envelope rbacErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if envelope.Error.Code != "forbidden" {
		t.Fatalf("expected error code forbidden, got %q", envelope.Error.Code)
	}
}
