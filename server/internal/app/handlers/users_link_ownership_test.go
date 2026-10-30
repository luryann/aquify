package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/go-chi/chi/v5"
)

type adminResolver struct{}

func (adminResolver) ResolvePrincipal(string) (*appmiddleware.Principal, bool) {
	return &appmiddleware.Principal{UserID: "admin-1", OrgID: "org-1", Roles: []string{"admin"}, MFAAuthenticated: true}, true
}

func newUserLinkRouter(h *UserHandlers) http.Handler {
	r := chi.NewRouter()
	r.Use(appmiddleware.Auth(adminResolver{}), appmiddleware.RequireOrgScope)
	h.RegisterRoutes(r)
	return r
}

func userLinkPost(router http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	req.Header.Set("X-Org-Id", "org-1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// Unlinking must only sever a link that belongs to the user in the URL; it used
// to clear whichever household/swimmer id was in the body.
func TestUnlinkRoutesRejectLinksOwnedByAnotherUser(t *testing.T) {
	kc := keycloak.NewStubClient()
	for _, id := range []string{"u-a", "u-b"} {
		kc.AddUser(keycloak.User{ID: id, Email: id + "@example.com", Enabled: true, Attributes: map[string][]string{"org_id": {"org-1"}}})
	}
	svc := members.NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", OrgID: "org-1", Roles: []string{"admin"}}
	hh, err := svc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: "org-1", HouseholdName: "B Family", PrimaryEmail: "b@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := svc.CreateSwimmer(admin, "org-1", members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "Bea", LastName: "B", DateOfBirth: "2012-01-01", Gender: "female", RegistrationStatus: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkHouseholdToUser("org-1", hh.ID, "u-b"); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkSwimmerToUser("org-1", sw.ID, "u-b"); err != nil {
		t.Fatal(err)
	}
	router := newUserLinkRouter(NewUserHandlersWithMembers(kc, nil, svc))

	rec := userLinkPost(router, "/users/u-a/household/unlink", `{"household_id":"`+hh.ID+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("household unlink via wrong user: want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, found := svc.GetHouseholdByKeycloakUserID("u-b"); !found {
		t.Fatal("u-b's household link was severed through u-a")
	}
	rec = userLinkPost(router, "/users/u-a/swimmer/unlink", `{"swimmer_id":"`+sw.ID+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("swimmer unlink via wrong user: want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, found := svc.GetSwimmerByKeycloakUserID("u-b"); !found {
		t.Fatal("u-b's swimmer link was severed through u-a")
	}

	// The owner can still unlink.
	if rec = userLinkPost(router, "/users/u-b/household/unlink", `{"household_id":"`+hh.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("owner household unlink: %d %s", rec.Code, rec.Body.String())
	}
	if rec = userLinkPost(router, "/users/u-b/swimmer/unlink", `{"swimmer_id":"`+sw.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("owner swimmer unlink: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGetUserFailsWhenRoleLookupFails(t *testing.T) {
	kc := keycloak.NewStubClient()
	kc.AddUser(keycloak.User{ID: "u-a", Email: "a@example.com", Enabled: true, Attributes: map[string][]string{"org_id": {"org-1"}}})
	kc.GetUserRealmRolesErr = context.DeadlineExceeded
	router := newUserLinkRouter(NewUserHandlers(kc, nil))

	req := httptest.NewRequest(http.MethodGet, "/users/u-a", nil)
	req.Header.Set("Authorization", "Bearer t")
	req.Header.Set("X-Org-Id", "org-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("a role lookup failure must not render as an empty role list: got %d %s", rec.Code, rec.Body.String())
	}
}
