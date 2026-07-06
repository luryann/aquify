package keycloak_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// ─── Mock Keycloak server ─────────────────────────────────────────────────────

// mockKeycloak simulates the Keycloak Admin REST API for testing AdminClient.
// It uses a single catch-all handler and dispatches by method + path.
type mockKeycloak struct {
	users                map[string]*keycloak.User
	roles                map[string]*keycloak.Role
	userRoles            map[string][]string // userID → []roleName
	disableOrgQueryFilter bool
	emptyOrgQueryResults bool
	alwaysOmitAttributes bool

	tokenCalls int
}

func newMockKeycloak() *mockKeycloak {
	return &mockKeycloak{
		users:     make(map[string]*keycloak.User),
		roles:     make(map[string]*keycloak.Role),
		userRoles: make(map[string][]string),
	}
}

func (mk *mockKeycloak) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const realm = "test-realm"

	// Token endpoint.
	if r.Method == http.MethodPost && r.URL.Path == "/realms/"+realm+"/protocol/openid-connect/token" {
		mk.tokenCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token-" + strings.Repeat("x", 16),
			"expires_in":   300,
			"token_type":   "Bearer",
		})
		return
	}

	adminPrefix := "/admin/realms/" + realm
	if !strings.HasPrefix(r.URL.Path, adminPrefix) {
		http.NotFound(w, r)
		return
	}
	subPath := strings.TrimPrefix(r.URL.Path, adminPrefix)

	switch {
	// ── /users (collection) ────────────────────────────────────────
	case subPath == "/users":
		switch r.Method {
		case http.MethodGet:
			mk.handleListUsers(w, r)
		case http.MethodPost:
			mk.handleCreateUser(w, r, realm)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}

	// ── /users/{id} and sub-paths ──────────────────────────────────
	case strings.HasPrefix(subPath, "/users/"):
		rest := strings.TrimPrefix(subPath, "/users/")
		parts := strings.SplitN(rest, "/", 4)
		userID := parts[0]

		switch {
		// /users/{id}/role-mappings/realm
		case len(parts) >= 3 && parts[1] == "role-mappings" && parts[2] == "realm":
			mk.handleRoleMappings(w, r, userID)

		// /users/{id}/execute-actions-email
		case len(parts) >= 2 && parts[1] == "execute-actions-email":
			if r.Method != http.MethodPut {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		// /users/{id}
		case len(parts) == 1:
			switch r.Method {
			case http.MethodGet:
				mk.handleGetUser(w, userID)
			case http.MethodPut:
				mk.handleUpdateUser(w, r, userID)
			case http.MethodDelete:
				mk.handleDeleteUser(w, userID)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		default:
			http.NotFound(w, r)
		}

	// ── /roles/{name} ─────────────────────────────────────────────
	case strings.HasPrefix(subPath, "/roles/"):
		roleName := strings.TrimPrefix(subPath, "/roles/")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		role, ok := mk.roles[roleName]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(role)

	// ── /roles (collection) ────────────────────────────────────────
	case subPath == "/roles":
		switch r.Method {
		case http.MethodGet:
			var out []keycloak.Role
			for _, r := range mk.roles {
				out = append(out, *r)
			}
			if out == nil {
				out = []keycloak.Role{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			name := payload["name"]
			if _, exists := mk.roles[name]; exists {
				http.Error(w, "conflict", http.StatusConflict)
				return
			}
			mk.roles[name] = &keycloak.Role{ID: "role-" + name, Name: name}
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}

	default:
		http.NotFound(w, r)
	}
}

func (mk *mockKeycloak) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := strings.ToLower(q.Get("search"))
	orgAttr := q.Get("q") // e.g. "org_id:org-1"
	includeAttributes := strings.EqualFold(strings.TrimSpace(q.Get("briefRepresentation")), "false")
	var orgID string
	if !mk.disableOrgQueryFilter && strings.HasPrefix(orgAttr, "org_id:") {
		if mk.emptyOrgQueryResults {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]keycloak.User{})
			return
		}
		orgID = strings.TrimPrefix(orgAttr, "org_id:")
	}

	var out []keycloak.User
	for _, u := range mk.users {
		if orgID != "" && u.OrgID() != orgID {
			continue
		}
		if search != "" &&
			!strings.Contains(strings.ToLower(u.Email), search) &&
			!strings.Contains(strings.ToLower(u.FirstName), search) &&
			!strings.Contains(strings.ToLower(u.LastName), search) {
			continue
		}
		copy := *u
		if !includeAttributes || mk.alwaysOmitAttributes {
			copy.Attributes = nil
		}
		out = append(out, copy)
	}
	if out == nil {
		out = []keycloak.User{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (mk *mockKeycloak) handleCreateUser(w http.ResponseWriter, r *http.Request, realm string) {
	var payload map[string]any
	json.NewDecoder(r.Body).Decode(&payload)
	email, _ := payload["email"].(string)
	for _, u := range mk.users {
		if strings.EqualFold(u.Email, email) {
			http.Error(w, "conflict", http.StatusConflict)
			return
		}
	}
	id := "user-" + email
	attrs := make(map[string][]string)
	if a, ok := payload["attributes"].(map[string]any); ok {
		for k, v := range a {
			if vals, ok := v.([]any); ok {
				var ss []string
				for _, sv := range vals {
					if s, ok := sv.(string); ok {
						ss = append(ss, s)
					}
				}
				attrs[k] = ss
			}
		}
	}
	mk.users[id] = &keycloak.User{
		ID:         id,
		Email:      email,
		FirstName:  strVal(payload["firstName"]),
		LastName:   strVal(payload["lastName"]),
		Enabled:    boolVal(payload["enabled"], true),
		Attributes: attrs,
	}
	mk.userRoles[id] = nil
	w.Header().Set("Location", "/admin/realms/"+realm+"/users/"+id)
	w.WriteHeader(http.StatusCreated)
}

func (mk *mockKeycloak) handleGetUser(w http.ResponseWriter, userID string) {
	u, ok := mk.users[userID]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u)
}

func (mk *mockKeycloak) handleUpdateUser(w http.ResponseWriter, r *http.Request, userID string) {
	u, ok := mk.users[userID]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var patch map[string]any
	json.NewDecoder(r.Body).Decode(&patch)
	if v, ok := patch["firstName"]; ok {
		u.FirstName = strVal(v)
	}
	if v, ok := patch["lastName"]; ok {
		u.LastName = strVal(v)
	}
	if v, ok := patch["email"]; ok {
		u.Email = strVal(v)
	}
	if v, ok := patch["enabled"]; ok {
		u.Enabled = boolVal(v, true)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (mk *mockKeycloak) handleDeleteUser(w http.ResponseWriter, userID string) {
	if _, ok := mk.users[userID]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	delete(mk.users, userID)
	delete(mk.userRoles, userID)
	w.WriteHeader(http.StatusNoContent)
}

func (mk *mockKeycloak) handleRoleMappings(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		var roles []keycloak.Role
		for _, name := range mk.userRoles[userID] {
			if role, ok := mk.roles[name]; ok {
				roles = append(roles, *role)
			}
		}
		if roles == nil {
			roles = []keycloak.Role{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(roles)
	case http.MethodPost:
		var toAdd []keycloak.Role
		json.NewDecoder(r.Body).Decode(&toAdd)
		for _, role := range toAdd {
			mk.userRoles[userID] = append(mk.userRoles[userID], role.Name)
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		var toRemove []keycloak.Role
		json.NewDecoder(r.Body).Decode(&toRemove)
		removeSet := make(map[string]bool)
		for _, rr := range toRemove {
			removeSet[rr.Name] = true
		}
		var remaining []string
		for _, name := range mk.userRoles[userID] {
			if !removeSet[name] {
				remaining = append(remaining, name)
			}
		}
		mk.userRoles[userID] = remaining
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestServer(t *testing.T) (*httptest.Server, *mockKeycloak) {
	t.Helper()
	mk := newMockKeycloak()
	srv := httptest.NewServer(mk)
	t.Cleanup(srv.Close)
	return srv, mk
}

func newTestClient(t *testing.T, srv *httptest.Server) *keycloak.AdminClient {
	t.Helper()
	c, err := keycloak.NewAdminClient(keycloak.AdminConfig{
		AdminURL:     srv.URL,
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		HTTPTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	return c
}

func strVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolVal(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// ─── AdminClient tests ────────────────────────────────────────────────────────

func TestAdminClient_InvalidConfig(t *testing.T) {
	_, err := keycloak.NewAdminClient(keycloak.AdminConfig{})
	if !errors.Is(err, keycloak.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestAdminClient_CreateAndGetUser(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	id, err := c.CreateUser(ctx, "alice@example.com", "Alice", "Smith", "org-1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id == "" {
		t.Fatal("CreateUser returned empty ID")
	}

	u, err := c.GetUser(ctx, id)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", u.Email)
	}
	if u.OrgID() != "org-1" {
		t.Errorf("org_id = %q, want org-1", u.OrgID())
	}
}

func TestAdminClient_CreateUser_Conflict(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	_, err := c.CreateUser(ctx, "bob@example.com", "Bob", "Jones", "org-1")
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	_, err = c.CreateUser(ctx, "bob@example.com", "Bob", "Jones", "org-1")
	if !errors.Is(err, keycloak.ErrConflict) {
		t.Fatalf("second CreateUser: expected ErrConflict, got %v", err)
	}
}

func TestAdminClient_GetUser_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	_, err := c.GetUser(ctx, "does-not-exist")
	if !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestAdminClient_ListUsers(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{
		ID: "u1", Email: "carol@example.com", FirstName: "Carol",
		Attributes: map[string][]string{"org_id": {"org-1"}},
	}
	mk.users["u2"] = &keycloak.User{
		ID: "u2", Email: "dave@example.com", FirstName: "Dave",
		Attributes: map[string][]string{"org_id": {"org-2"}},
	}

	users, err := c.ListUsers(ctx, "org-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Email != "carol@example.com" {
		t.Errorf("expected carol, got %s", users[0].Email)
	}
}

func TestAdminClient_ListUsers_FallsBackWhenOrgQueryReturnsNoMatches(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.disableOrgQueryFilter = true
	mk.users["u1"] = &keycloak.User{
		ID: "u1", Email: "carol@example.com", FirstName: "Carol",
		Attributes: map[string][]string{"org_id": {"org-1"}},
	}
	mk.users["u2"] = &keycloak.User{
		ID: "u2", Email: "dave@example.com", FirstName: "Dave",
		Attributes: map[string][]string{"org_id": {"org-2"}},
	}

	users, err := c.ListUsers(ctx, "org-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers fallback: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 fallback user, got %d", len(users))
	}
	if users[0].Email != "carol@example.com" {
		t.Errorf("expected carol after fallback, got %s", users[0].Email)
	}
}

func TestAdminClient_ListUsers_FallsBackWhenOrgQueryReturnsEmptyPage(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.emptyOrgQueryResults = true
	mk.users["u1"] = &keycloak.User{
		ID: "u1", Email: "carol@example.com", FirstName: "Carol",
		Attributes: map[string][]string{"org_id": {"org-1"}},
	}
	mk.users["u2"] = &keycloak.User{
		ID: "u2", Email: "dave@example.com", FirstName: "Dave",
		Attributes: map[string][]string{"org_id": {"org-2"}},
	}

	users, err := c.ListUsers(ctx, "org-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers empty-page fallback: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 fallback user after empty org query, got %d", len(users))
	}
	if users[0].Email != "carol@example.com" {
		t.Errorf("expected carol after empty-page fallback, got %s", users[0].Email)
	}
}

func TestAdminClient_ListUsers_HydratesUsersWhenListResponseOmitsAttributes(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.alwaysOmitAttributes = true
	mk.users["u1"] = &keycloak.User{
		ID: "u1", Email: "carol@example.com", FirstName: "Carol",
		Attributes: map[string][]string{"org_id": {"org-1"}},
	}
	mk.users["u2"] = &keycloak.User{
		ID: "u2", Email: "dave@example.com", FirstName: "Dave",
		Attributes: map[string][]string{"org_id": {"org-2"}},
	}

	users, err := c.ListUsers(ctx, "org-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers hydrate attrs: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 hydrated user, got %d", len(users))
	}
	if users[0].Email != "carol@example.com" {
		t.Errorf("expected carol after hydration, got %s", users[0].Email)
	}
}

func TestAdminClient_UpdateUser(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "old@example.com", FirstName: "Old", Enabled: true}

	err := c.UpdateUser(ctx, "u1", keycloak.UpdateUserFields{
		FirstName: "New",
		Email:     "new@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if mk.users["u1"].FirstName != "New" {
		t.Errorf("first name not updated: %s", mk.users["u1"].FirstName)
	}
	if mk.users["u1"].Email != "new@example.com" {
		t.Errorf("email not updated: %s", mk.users["u1"].Email)
	}
}

func TestAdminClient_DisableEnableUser(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "x@example.com", Enabled: true}

	if err := c.DisableUser(ctx, "u1"); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if mk.users["u1"].Enabled {
		t.Error("user should be disabled")
	}

	if err := c.EnableUser(ctx, "u1"); err != nil {
		t.Fatalf("EnableUser: %v", err)
	}
	if !mk.users["u1"].Enabled {
		t.Error("user should be enabled")
	}
}

func TestAdminClient_DeleteUser(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "del@example.com"}

	if err := c.DeleteUser(ctx, "u1"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, ok := mk.users["u1"]; ok {
		t.Error("user should be deleted from store")
	}

	if err := c.DeleteUser(ctx, "u1"); !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestAdminClient_SendVerificationEmail(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "verify@example.com"}

	if err := c.SendVerificationEmail(ctx, "u1"); err != nil {
		t.Fatalf("SendVerificationEmail: %v", err)
	}
}

func TestAdminClient_SendPasswordResetEmail(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "reset@example.com"}

	if err := c.SendPasswordResetEmail(ctx, "u1"); err != nil {
		t.Fatalf("SendPasswordResetEmail: %v", err)
	}
}

func TestAdminClient_RealmRoles(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "roles@example.com"}
	mk.roles["coach"] = &keycloak.Role{ID: "role-coach", Name: "coach"}
	mk.roles["admin"] = &keycloak.Role{ID: "role-admin", Name: "admin"}

	if err := c.AssignRealmRole(ctx, "u1", "coach"); err != nil {
		t.Fatalf("AssignRealmRole coach: %v", err)
	}
	if err := c.AssignRealmRole(ctx, "u1", "admin"); err != nil {
		t.Fatalf("AssignRealmRole admin: %v", err)
	}

	roles, err := c.GetUserRealmRoles(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUserRealmRoles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}

	if err := c.RemoveRealmRole(ctx, "u1", "coach"); err != nil {
		t.Fatalf("RemoveRealmRole: %v", err)
	}
	roles, _ = c.GetUserRealmRoles(ctx, "u1")
	if len(roles) != 1 || roles[0].Name != "admin" {
		t.Errorf("expected only admin role, got %v", roles)
	}
}

func TestAdminClient_AssignRole_NotFound(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "r@example.com"}

	err := c.AssignRealmRole(ctx, "u1", "nonexistent-role")
	if !errors.Is(err, keycloak.ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}

func TestAdminClient_EnsureRealmRolesExist(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	// Seed one existing role.
	mk.roles["admin"] = &keycloak.Role{ID: "role-admin", Name: "admin"}

	if err := c.EnsureRealmRolesExist(ctx, keycloak.DefaultRealmRoles); err != nil {
		t.Fatalf("EnsureRealmRolesExist: %v", err)
	}

	for _, name := range keycloak.DefaultRealmRoles {
		if _, ok := mk.roles[name]; !ok {
			t.Errorf("role %q not created", name)
		}
	}
}

func TestAdminClient_TokenCaching(t *testing.T) {
	srv, mk := newTestServer(t)
	c := newTestClient(t, srv)
	ctx := context.Background()

	mk.users["u1"] = &keycloak.User{ID: "u1", Email: "tok@example.com"}

	// Multiple calls should reuse the cached token.
	for i := 0; i < 5; i++ {
		_, err := c.GetUser(ctx, "u1")
		if err != nil {
			t.Fatalf("GetUser call %d: %v", i, err)
		}
	}
	if mk.tokenCalls != 1 {
		t.Errorf("expected 1 token fetch (cached), got %d", mk.tokenCalls)
	}
}

// ─── StubClient tests ─────────────────────────────────────────────────────────

func TestStubClient_CreateAndGet(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	id, err := s.CreateUser(ctx, "alice@example.com", "Alice", "Smith", "org-1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, err := s.GetUser(ctx, id)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("email = %q", u.Email)
	}
	if u.OrgID() != "org-1" {
		t.Errorf("org_id = %q", u.OrgID())
	}
}

func TestStubClient_ConflictOnDuplicateEmail(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	_, err := s.CreateUser(ctx, "bob@example.com", "Bob", "Jones", "org-1")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = s.CreateUser(ctx, "bob@example.com", "Bob2", "Jones", "org-2")
	if !errors.Is(err, keycloak.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestStubClient_RoleManagement(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	id, _ := s.CreateUser(ctx, "coach@example.com", "Coach", "User", "org-1")

	if err := s.AssignRealmRole(ctx, id, "coach"); err != nil {
		t.Fatalf("AssignRealmRole: %v", err)
	}

	roles, err := s.GetUserRealmRoles(ctx, id)
	if err != nil {
		t.Fatalf("GetUserRealmRoles: %v", err)
	}
	if len(roles) != 1 || roles[0].Name != "coach" {
		t.Errorf("unexpected roles: %v", roles)
	}

	if err := s.RemoveRealmRole(ctx, id, "coach"); err != nil {
		t.Fatalf("RemoveRealmRole: %v", err)
	}
	roles, _ = s.GetUserRealmRoles(ctx, id)
	if len(roles) != 0 {
		t.Errorf("expected no roles, got %v", roles)
	}
}

func TestStubClient_AssignUnknownRole(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	id, _ := s.CreateUser(ctx, "x@example.com", "X", "Y", "org-1")
	err := s.AssignRealmRole(ctx, id, "not-a-real-role")
	if !errors.Is(err, keycloak.ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}

func TestStubClient_DisableEnable(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	id, _ := s.CreateUser(ctx, "toggle@example.com", "T", "U", "org-1")

	s.DisableUser(ctx, id)
	u, _ := s.GetUser(ctx, id)
	if u.Enabled {
		t.Error("should be disabled")
	}

	s.EnableUser(ctx, id)
	u, _ = s.GetUser(ctx, id)
	if !u.Enabled {
		t.Error("should be enabled")
	}
}

func TestStubClient_SendEmails(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	id, _ := s.CreateUser(ctx, "email@example.com", "E", "M", "org-1")

	if err := s.SendVerificationEmail(ctx, id); err != nil {
		t.Fatalf("SendVerificationEmail: %v", err)
	}
	if err := s.SendPasswordResetEmail(ctx, id); err != nil {
		t.Fatalf("SendPasswordResetEmail: %v", err)
	}

	if vl := s.VerificationsSent(); len(vl) != 1 || vl[0] != id {
		t.Errorf("VerificationsSent = %v", vl)
	}
	if pl := s.PasswordResetsSent(); len(pl) != 1 || pl[0] != id {
		t.Errorf("PasswordResetsSent = %v", pl)
	}
}

func TestStubClient_EnsureRealmRolesExist(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	extra := append(keycloak.DefaultRealmRoles, "custom-role")
	if err := s.EnsureRealmRolesExist(ctx, extra); err != nil {
		t.Fatalf("EnsureRealmRolesExist: %v", err)
	}
	// custom-role should now be assignable.
	id, _ := s.CreateUser(ctx, "test@example.com", "T", "T", "org-1")
	if err := s.AssignRealmRole(ctx, id, "custom-role"); err != nil {
		t.Fatalf("AssignRealmRole custom-role after ensure: %v", err)
	}
}

func TestStubClient_ActionsLog(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	id, _ := s.CreateUser(ctx, "log@example.com", "L", "G", "org-1")
	s.AssignRealmRole(ctx, id, "admin")
	s.DisableUser(ctx, id)
	s.DeleteUser(ctx, id)

	actions := s.Actions()
	// create_user, assign_role, update_user (disable), delete_user
	wantActions := []string{"create_user", "assign_role", "update_user", "delete_user"}
	if len(actions) != len(wantActions) {
		t.Fatalf("expected %d actions, got %d: %+v", len(wantActions), len(actions), actions)
	}
	for i, want := range wantActions {
		if actions[i].Action != want {
			t.Errorf("action[%d] = %q, want %q", i, actions[i].Action, want)
		}
	}
}

func TestStubClient_ListUsers_Filter(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()

	s.AddUser(keycloak.User{
		ID: "u1", Email: "alice@example.com", FirstName: "Alice",
		Attributes: map[string][]string{"org_id": {"org-1"}},
	})
	s.AddUser(keycloak.User{
		ID: "u2", Email: "bob@example.com", FirstName: "Bob",
		Attributes: map[string][]string{"org_id": {"org-2"}},
	})

	users, err := s.ListUsers(ctx, "org-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 || users[0].ID != "u1" {
		t.Errorf("expected only u1, got %v", users)
	}
}

func TestStubClient_DeleteUser_NotFound(t *testing.T) {
	s := keycloak.NewStubClient()
	err := s.DeleteUser(context.Background(), "nonexistent")
	if !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestStubClient_ErrorOverrides(t *testing.T) {
	s := keycloak.NewStubClient()
	ctx := context.Background()
	sentinel := errors.New("injected error")

	s.ListUsersErr = sentinel
	_, err := s.ListUsers(ctx, "", "", 10, 0)
	if !errors.Is(err, sentinel) {
		t.Errorf("ListUsers: expected sentinel, got %v", err)
	}

	s.ListUsersErr = nil
	s.CreateUserErr = sentinel
	_, err = s.CreateUser(ctx, "e@x.com", "E", "X", "org-1")
	if !errors.Is(err, sentinel) {
		t.Errorf("CreateUser: expected sentinel, got %v", err)
	}
}

func TestStubClient_GetUser_NotFound(t *testing.T) {
	s := keycloak.NewStubClient()
	_, err := s.GetUser(context.Background(), "nobody")
	if !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}
