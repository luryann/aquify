package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/authn"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func newStep30BRouter(t *testing.T, kc keycloak.Client) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		audit.NewService(), nil, nil, nil, nil,
		kc,
	)
}

func issueStep30BToken(t *testing.T, router http.Handler, userID, orgID string, roles []string, mfa bool) string {
	t.Helper()
	payload := map[string]any{
		"user_id":           userID,
		"org_id":            orgID,
		"roles":             roles,
		"mfa_authenticated": mfa,
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issueStep30BToken: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

func do30B(router http.Handler, method, path, token, body, orgID string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if orgID != "" {
		req.Header.Set("X-Org-Id", orgID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ─── List users ───────────────────────────────────────────────────────────────

func TestUserMgmt_ListUsers_AdminCanList(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com", FirstName: "Alice", LastName: "Doe",
		Enabled: true, Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	users := resp["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("want 1 user, got %d", len(users))
	}
	if int(resp["limit"].(float64)) != 50 || int(resp["offset"].(float64)) != 0 {
		t.Fatalf("expected default pagination metadata limit=50 offset=0, got limit=%v offset=%v", resp["limit"], resp["offset"])
	}
}

func TestUserMgmt_ListUsers_SeedsDevStubUsersAndIncludesSourceMetadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{
			AppEnv: "development",
			Auth: AuthConfig{
				EnableDevStubTokens: true,
			},
		},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		audit.NewService(), nil, nil, nil, nil,
		nil,
	)
	token := issueStep30BToken(t, router, authn.LocalDevAdminUserID, authn.LocalDevOrgID, []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users", token, "", authn.LocalDevOrgID)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Count     int    `json:"count"`
		Source    string `json:"source"`
		IsStubbed bool   `json:"is_stubbed"`
		Users     []struct {
			Email string   `json:"email"`
			Roles []string `json:"roles"`
		} `json:"users"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Source != "stub" || !resp.IsStubbed {
		t.Fatalf("expected stub metadata, got source=%q is_stubbed=%t", resp.Source, resp.IsStubbed)
	}
	if resp.Count != len(authn.LocalDevUsers()) {
		t.Fatalf("expected %d seeded users, got %d", len(authn.LocalDevUsers()), resp.Count)
	}
}

func TestUserMgmt_ListUsers_CoachForbidden(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "coach1", "org-test", []string{"coach"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users", token, "", "org-test")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}

func TestUserMgmt_ListUsers_NoAuth(t *testing.T) {
	router := newStep30BRouter(t, keycloak.NewStubClient())
	rr := do30B(router, http.MethodGet, "/api/v1/users", "", "", "org-test")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestUserMgmt_ListUsers_ClampsPaginationLimit(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{ID: "u1", Email: "a@example.com", Attributes: map[string][]string{"org_id": {"org-test"}}})
	kc.AddUser(keycloak.User{ID: "u2", Email: "b@example.com", Attributes: map[string][]string{"org_id": {"org-test"}}})
	kc.AddUser(keycloak.User{ID: "u3", Email: "c@example.com", Attributes: map[string][]string{"org_id": {"org-test"}}})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users?limit=999&offset=1", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Count  int `json:"count"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Users  []struct {
			ID string `json:"id"`
		} `json:"users"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Limit != 200 || resp.Offset != 1 {
		t.Fatalf("expected clamped pagination limit=200 offset=1, got limit=%d offset=%d", resp.Limit, resp.Offset)
	}
	if resp.Count != 2 || len(resp.Users) != 2 {
		t.Fatalf("expected paginated user page count=2 len=2, got count=%d len=%d", resp.Count, len(resp.Users))
	}
}

// ─── Get user ─────────────────────────────────────────────────────────────────

func TestUserMgmt_GetUser_Found(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com",
		Enabled:    true,
		Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users/u1", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_GetUser_NotFound(t *testing.T) {
	router := newStep30BRouter(t, keycloak.NewStubClient())
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users/nonexistent", token, "", "org-test")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestUserMgmt_GetUser_WrongOrg(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com",
		Attributes: map[string][]string{"org_id": {"other-org"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users/u1", token, "", "org-test")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 (wrong org), got %d", rr.Code)
	}
}

// ─── Invite user ──────────────────────────────────────────────────────────────

func TestUserMgmt_Invite_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	body := `{"first_name":"Bob","last_name":"Smith","email":"bob@example.com","role":"coach"}`
	rr := do30B(router, http.MethodPost, "/api/v1/users/invite", token, body, "org-test")
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["user_id"] == nil || resp["user_id"] == "" {
		t.Fatal("expected user_id in response")
	}

	// Verify verification email was sent
	if len(kc.VerificationsSent()) == 0 {
		t.Fatal("expected verification email to be sent")
	}
}

func TestUserMgmt_Invite_DuplicateEmail(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "bob@example.com",
		Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	// Make CreateUser return conflict for that email
	kc.CreateUserErr = keycloak.ErrConflict
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	body := `{"email":"bob@example.com"}`
	rr := do30B(router, http.MethodPost, "/api/v1/users/invite", token, body, "org-test")
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_Invite_NonAdminForbidden(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "parent1", "org-test", []string{"parent"}, false)

	body := `{"email":"new@example.com"}`
	rr := do30B(router, http.MethodPost, "/api/v1/users/invite", token, body, "org-test")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}

func TestUserMgmt_CreateUser_NoSilentStubFallbackWhenRealOIDCConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{
			AppEnv: "development",
			Auth: AuthConfig{
				EnableDevStubTokens: false,
				OIDC: OIDCConfig{
					AuthorizeURL:    "http://localhost:8180/realms/aquify/protocol/openid-connect/auth",
					RedirectURL:     "http://localhost:3000/auth/callback",
					ClientID:        "aquify-id",
					StateSigningKey: "state-signing-key",
				},
			},
		},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		audit.NewService(), nil, nil, nil, nil,
		nil,
	)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	body := `{"first_name":"Bob","last_name":"Smith","email":"bob@example.com","password":"ChangeMe123!","role":"parent"}`
	rr := do30B(router, http.MethodPost, "/api/v1/users", token, body, "org-test")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var env apiErrorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if env.Error.Code != "keycloak_unavailable" {
		t.Fatalf("want keycloak_unavailable, got %q", env.Error.Code)
	}
}

// ─── Update user ──────────────────────────────────────────────────────────────

func TestUserMgmt_UpdateUser_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com",
		Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	body := `{"first_name":"Alice","last_name":"Updated"}`
	rr := do30B(router, http.MethodPut, "/api/v1/users/u1", token, body, "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Disable / Enable user ────────────────────────────────────────────────────

func TestUserMgmt_DisableEnable_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodPost, "/api/v1/users/u1/disable", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("disable: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = do30B(router, http.MethodPost, "/api/v1/users/u1/enable", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("enable: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Delete user ──────────────────────────────────────────────────────────────

func TestUserMgmt_DeleteUser_RequiresMFA(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	// admin but no MFA
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodDelete, "/api/v1/users/u1", token, "", "org-test")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 (mfa required), got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_DeleteUser_WithMFA_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, true)

	rr := do30B(router, http.MethodDelete, "/api/v1/users/u1", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_DeleteUser_LastAdminBlocked(t *testing.T) {
	kc := keycloak.NewStubClient()
	// Only one admin in the org, and it's the delete target.
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	_ = kc.AssignRealmRole(nil, "u1", "admin") //nolint

	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, true)

	rr := do30B(router, http.MethodDelete, "/api/v1/users/u1", token, "", "org-test")
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 (last admin), got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_DeleteUser_NonAdminTargetAllowedEvenAsSoleAdminOrg(t *testing.T) {
	kc := keycloak.NewStubClient()
	// One admin (not the delete target) and one coach (the delete target).
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	_ = kc.AssignRealmRole(nil, "u1", "admin") //nolint
	kc.AddUser(keycloak.User{
		ID: "u2", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	_ = kc.AssignRealmRole(nil, "u2", "coach") //nolint

	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, true)

	rr := do30B(router, http.MethodDelete, "/api/v1/users/u2", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 (deleting non-admin doesn't need the last-admin guard), got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Roles ────────────────────────────────────────────────────────────────────

func TestUserMgmt_ListRoles_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodGet, "/api/v1/users/u1/roles", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_AssignRole_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	body := `{"role":"coach"}`
	rr := do30B(router, http.MethodPost, "/api/v1/users/u1/roles", token, body, "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_RemoveRole_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	// Add a second admin so we can remove from u1
	kc.AddUser(keycloak.User{
		ID: "u2", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	_ = kc.AssignRealmRole(nil, "u1", "coach") //nolint
	_ = kc.AssignRealmRole(nil, "u2", "admin") //nolint
	_ = kc.AssignRealmRole(nil, "u1", "admin") //nolint

	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	// Remove coach role (not the last admin)
	rr := do30B(router, http.MethodDelete, "/api/v1/users/u1/roles/coach", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserMgmt_RemoveRole_LastAdminBlocked(t *testing.T) {
	kc := keycloak.NewStubClient()
	// Only one admin in the org
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	_ = kc.AssignRealmRole(nil, "u1", "admin") //nolint

	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodDelete, "/api/v1/users/u1/roles/admin", token, "", "org-test")
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 (last admin), got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Password reset ───────────────────────────────────────────────────────────

func TestUserMgmt_PasswordReset_Success(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodPost, "/api/v1/users/u1/password-reset", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(kc.PasswordResetsSent()) != 1 {
		t.Fatalf("expected 1 password reset email, got %d", len(kc.PasswordResetsSent()))
	}
}

func TestUserMgmt_PasswordReset_NonAdminForbidden(t *testing.T) {
	kc := keycloak.NewStubClient()
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "parent1", "org-test", []string{"parent"}, false)

	rr := do30B(router, http.MethodPost, "/api/v1/users/u1/password-reset", token, "", "org-test")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}

// ─── RBAC matrix: verify all routes require admin ────────────────────────────

func TestUserMgmt_RBACMatrix(t *testing.T) {
	nonAdminRoles := []string{"coach", "parent", "swimmer"}
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	router := newStep30BRouter(t, kc)

	routes := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users/u1"},
		{http.MethodPost, "/api/v1/users/invite"},
		{http.MethodPut, "/api/v1/users/u1"},
		{http.MethodPost, "/api/v1/users/u1/disable"},
		{http.MethodPost, "/api/v1/users/u1/enable"},
		{http.MethodGet, "/api/v1/users/u1/roles"},
		{http.MethodPost, "/api/v1/users/u1/roles"},
		{http.MethodDelete, "/api/v1/users/u1/roles/coach"},
		{http.MethodPost, "/api/v1/users/u1/password-reset"},
	}

	for _, role := range nonAdminRoles {
		token := issueStep30BToken(t, router, "user1", "org-test", []string{role}, false)
		for _, route := range routes {
			rr := do30B(router, route.method, route.path, token, "{}", "org-test")
			if rr.Code != http.StatusForbidden {
				t.Errorf("role=%s %s %s: want 403, got %d", role, route.method, route.path, rr.Code)
			}
		}
	}
}

// ─── Org isolation ────────────────────────────────────────────────────────────

func TestUserMgmt_OrgIsolation(t *testing.T) {
	kc := keycloak.NewStubClient()
	// User belongs to different org
	kc.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com",
		Attributes: map[string][]string{"org_id": {"other-org"}},
	})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	tests := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/users/u1"},
		{http.MethodPut, "/api/v1/users/u1"},
		{http.MethodPost, "/api/v1/users/u1/disable"},
		{http.MethodPost, "/api/v1/users/u1/enable"},
		{http.MethodGet, "/api/v1/users/u1/roles"},
		{http.MethodPost, "/api/v1/users/u1/roles"},
		{http.MethodPost, "/api/v1/users/u1/password-reset"},
	}

	for _, tt := range tests {
		rr := do30B(router, tt.method, tt.path, token, `{"role":"coach"}`, "org-test")
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s %s: want 403 (org isolation), got %d", tt.method, tt.path, rr.Code)
		}
	}
}

// ─── Missing X-Org-Id header ─────────────────────────────────────────────────

func TestUserMgmt_MissingOrgHeader(t *testing.T) {
	router := newStep30BRouter(t, keycloak.NewStubClient())
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	// No X-Org-Id header
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 (missing org), got %d", rr.Code)
	}
}

// ─── Audit event emission ─────────────────────────────────────────────────────

func TestUserMgmt_AuditEvents_InviteEmitsEvent(t *testing.T) {
	kc := keycloak.NewStubClient()
	auditSvc := audit.NewService()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		auditSvc, nil, nil, nil, nil,
		kc,
	)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	body := `{"email":"new@example.com","first_name":"New","last_name":"User"}`
	rr := do30B(router, http.MethodPost, "/api/v1/users/invite", token, body, "org-test")
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	events, _ := auditSvc.Query(audit.Filter{OrgID: "org-test"})
	found := false
	for _, e := range events {
		if e.Action == audit.ActionAuthUserInvite {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected audit event auth.user.invite not found")
	}
}

func TestUserMgmt_AuditEvents_DisableEmitsEvent(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{
		ID: "u1", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-test"}},
	})
	auditSvc := audit.NewService()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		auditSvc, nil, nil, nil, nil,
		kc,
	)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, false)

	rr := do30B(router, http.MethodPost, "/api/v1/users/u1/disable", token, "", "org-test")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	events, _ := auditSvc.Query(audit.Filter{OrgID: "org-test"})
	found := false
	for _, e := range events {
		if e.Action == audit.ActionAuthUserDisable {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audit event %s not found", audit.ActionAuthUserDisable)
	}
}
