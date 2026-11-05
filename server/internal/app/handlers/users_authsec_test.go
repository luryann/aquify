package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/keycloak"
)

func TestDeleteUserRefusesSelfDelete(t *testing.T) {
	_, router := usersG2Setup(t)
	rec := usersG2Req(router, http.MethodDelete, "/users/admin-1")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cannot_delete_self") {
		t.Fatalf("self delete: want 409 cannot_delete_self, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveAdminRoleIgnoresDisabledAdmins(t *testing.T) {
	kc, router := usersG2Setup(t)
	kc.AddUser(keycloak.User{ID: "admin-2", Email: "a2@example.com", Enabled: false, Attributes: map[string][]string{"org_id": {"org-1"}}})
	_ = kc.AssignRealmRole(context.Background(), "admin-2", "admin")
	// admin-1 is the only enabled admin; the disabled admin must not count.
	rec := usersG2Req(router, http.MethodDelete, "/users/admin-1/roles/admin")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "last_admin") {
		t.Fatalf("remove last enabled admin role: want 409 last_admin, got %d %s", rec.Code, rec.Body.String())
	}
}

type nonMFAAdminResolver struct{}

func (nonMFAAdminResolver) ResolvePrincipal(string) (*appmiddleware.Principal, bool) {
	return &appmiddleware.Principal{UserID: "admin-1", OrgID: "org-1", Roles: []string{"admin"}}, true
}

func TestCreateAndInviteStaffRolesRequireMFA(t *testing.T) {
	kc, _ := usersG2Setup(t)
	r := chi.NewRouter()
	r.Use(appmiddleware.Auth(nonMFAAdminResolver{}), appmiddleware.RequireOrgScope)
	NewUserHandlers(kc, nil).RegisterRoutes(r)

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer t")
		req.Header.Set("X-Org-Id", "org-1")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	for _, role := range []string{"admin", "coach"} {
		if rec := post("/users", `{"email":"x`+role+`@example.com","first_name":"A","last_name":"B","role":"`+role+`","password":"password123"}`); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "mfa_required") {
			t.Fatalf("create %s without MFA: want 403 mfa_required, got %d %s", role, rec.Code, rec.Body.String())
		}
		if rec := post("/users/invite", `{"email":"i`+role+`@example.com","first_name":"A","last_name":"B","role":"`+role+`"}`); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "mfa_required") {
			t.Fatalf("invite %s without MFA: want 403 mfa_required, got %d %s", role, rec.Code, rec.Body.String())
		}
	}
	if rec := post("/users/invite", `{"email":"ip@example.com","first_name":"A","last_name":"B","role":"parent"}`); rec.Code != http.StatusCreated {
		t.Fatalf("invite parent without MFA: want 201, got %d %s", rec.Code, rec.Body.String())
	}
}
