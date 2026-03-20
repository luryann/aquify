package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type authErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

type staticResolver struct {
	principals map[string]*Principal
}

func (r staticResolver) ResolvePrincipal(token string) (*Principal, bool) {
	principal, ok := r.principals[token]
	if !ok {
		return nil, false
	}
	cloned := *principal
	cloned.Roles = append([]string{}, principal.Roles...)
	return &cloned, true
}

func TestAuthRejectsUnknownBearerToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	rr := httptest.NewRecorder()

	h := Auth(staticResolver{principals: map[string]*Principal{}})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called for unknown tokens")
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	var envelope authErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if envelope.Error.Code != "unauthorized" {
		t.Fatalf("expected error code unauthorized, got %q", envelope.Error.Code)
	}
}

func TestAuthAcceptsResolverToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req.Header.Set("Authorization", "Bearer valid-admin-token")
	rr := httptest.NewRecorder()

	resolver := staticResolver{principals: map[string]*Principal{
		"valid-admin-token": {
			UserID: "admin-user",
			OrgID:  "org-demo",
			Roles:  []string{"admin"},
		},
	}}

	h := Auth(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || principal == nil {
			t.Fatal("expected principal in context")
		}
		if principal.UserID != "admin-user" {
			t.Fatalf("expected admin-user, got %q", principal.UserID)
		}
		if len(principal.Roles) != 1 || principal.Roles[0] != "admin" {
			t.Fatalf("unexpected roles: %#v", principal.Roles)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestAuthAcceptsAccessTokenCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/ping", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "cookie-admin-token"})
	rr := httptest.NewRecorder()

	resolver := staticResolver{principals: map[string]*Principal{
		"cookie-admin-token": {
			UserID: "admin-user",
			OrgID:  "org-demo",
			Roles:  []string{"admin"},
		},
	}}

	h := Auth(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || principal == nil {
			t.Fatal("expected principal in context")
		}
		if principal.UserID != "admin-user" {
			t.Fatalf("expected admin-user, got %q", principal.UserID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}
