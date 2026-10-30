package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

func usersG2Setup(t *testing.T) (*keycloak.StubClient, http.Handler) {
	t.Helper()
	kc := keycloak.NewStubClient()
	add := func(id, role string, enabled bool) {
		kc.AddUser(keycloak.User{ID: id, Email: id + "@example.com", Enabled: enabled, Attributes: map[string][]string{"org_id": {"org-1"}}})
		if role != "" {
			if err := kc.AssignRealmRole(context.Background(), id, role); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("admin-1", "admin", true) // the caller (adminResolver)
	add("u-coach", "coach", true)
	for i := 0; i < 5; i++ {
		add(fmt.Sprintf("p-%d", i), "parent", true)
	}
	return kc, newUserLinkRouter(NewUserHandlers(kc, nil))
}

func usersG2Req(router http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer t")
	req.Header.Set("X-Org-Id", "org-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestDisableUserGuards(t *testing.T) {
	_, router := usersG2Setup(t)
	if rec := usersG2Req(router, http.MethodPost, "/users/admin-1/disable"); rec.Code != http.StatusConflict {
		t.Fatalf("self/last-admin disable: want 409, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := usersG2Req(router, http.MethodPost, "/users/p-0/disable"); rec.Code != http.StatusOK {
		t.Fatalf("non-admin disable: want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestDisableLastEnabledAdminRefused(t *testing.T) {
	kc, router := usersG2Setup(t)
	kc.AddUser(keycloak.User{ID: "admin-2", Email: "a2@example.com", Enabled: true, Attributes: map[string][]string{"org_id": {"org-1"}}})
	_ = kc.AssignRealmRole(context.Background(), "admin-2", "admin")
	// Two admins: disabling the other one is allowed, then it is the last.
	if rec := usersG2Req(router, http.MethodPost, "/users/admin-2/disable"); rec.Code != http.StatusConflict {
		// admin-1 is the caller and still enabled, so admin-2 may be disabled.
		if rec.Code != http.StatusOK {
			t.Fatalf("disable second admin: got %d %s", rec.Code, rec.Body.String())
		}
	} else {
		t.Fatalf("disabling one of two admins must be allowed: %s", rec.Body.String())
	}
	kc.AddUser(keycloak.User{ID: "admin-3", Email: "a3@example.com", Enabled: true, Attributes: map[string][]string{"org_id": {"org-1"}}})
	_ = kc.AssignRealmRole(context.Background(), "admin-3", "admin")
	_ = kc.DisableUser(context.Background(), "admin-1")
	// Only admin-3 remains enabled besides disabled ones: refuse.
	if rec := usersG2Req(router, http.MethodPost, "/users/admin-3/disable"); rec.Code != http.StatusConflict {
		t.Fatalf("last enabled admin disable: want 409, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveRoleNonAdminTargetIsNotLastAdminConflict(t *testing.T) {
	_, router := usersG2Setup(t)
	// admin-1 is the only admin; removing "admin" from a non-admin coach is a no-op, not a 409.
	rec := usersG2Req(router, http.MethodDelete, "/users/u-coach/roles/admin")
	if rec.Code == http.StatusConflict {
		t.Fatalf("spurious last_admin conflict for non-admin target: %s", rec.Body.String())
	}
	// Removing it from the real last admin is still refused.
	if rec := usersG2Req(router, http.MethodDelete, "/users/admin-1/roles/admin"); rec.Code != http.StatusConflict {
		t.Fatalf("last admin removal: want 409, got %d", rec.Code)
	}
}

func TestListUsersFiltersBeforePaginating(t *testing.T) {
	_, router := usersG2Setup(t)
	// 5 parents among 7 users; a page of 2 filtered by role must return 2 parents, total 5.
	rec := usersG2Req(router, http.MethodGet, "/users?role=parent&limit=2&offset=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Count int `json:"count"`
		Total int `json:"total"`
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Count != 2 || body.Total != 5 {
		t.Fatalf("want count=2 total=5, got %+v", body)
	}
	for _, u := range body.Users {
		if !strings.HasPrefix(u.ID, "p-") {
			t.Fatalf("non-parent in role-filtered page: %s", u.ID)
		}
	}
}
