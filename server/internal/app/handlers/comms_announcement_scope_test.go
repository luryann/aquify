package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
)

// Parents and swimmers must only see published announcements; drafts and
// archived ones are staff-only, both in the list and by direct ID.
func TestAnnouncementReadsHideUnpublishedFromPortalRoles(t *testing.T) {
	svc := comms.NewService()
	draft, err := svc.CreateAnnouncement(comms.CreateAnnouncementInput{OrgID: "org-1", Title: "Draft", Body: "b", Audience: "all", CreatedBy: "c"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	pub, err := svc.CreateAnnouncement(comms.CreateAnnouncementInput{OrgID: "org-1", Title: "Live", Body: "b", Audience: "all", CreatedBy: "c"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.PublishAnnouncement("org-1", pub.ID, "c"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	resolver := tokenResolver{
		"parent": {UserID: "p", OrgID: "org-1", HouseholdID: "hh", Roles: []string{"parent"}},
		"coach":  {UserID: "c", OrgID: "org-1", Roles: []string{"coach"}},
	}
	router := chi.NewRouter()
	router.Use(appmiddleware.Auth(resolver), appmiddleware.RequireOrgScope)
	NewCommsHandlers(svc, nil, nil).RegisterRoutes(router)
	get := func(token, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", "org-1")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	expectStatus(t, get("parent", "/announcements/"+draft.ID), http.StatusNotFound, "parent draft by id")
	expectStatus(t, get("parent", "/announcements/"+pub.ID), http.StatusOK, "parent published by id")
	expectStatus(t, get("coach", "/announcements/"+draft.ID), http.StatusOK, "coach draft by id")

	list := get("parent", "/announcements")
	expectStatus(t, list, http.StatusOK, "parent list")
	if strings.Contains(list.Body.String(), draft.ID) || !strings.Contains(list.Body.String(), pub.ID) {
		t.Fatalf("parent list should contain only the published announcement: %s", list.Body.String())
	}
	if !strings.Contains(get("coach", "/announcements").Body.String(), draft.ID) {
		t.Fatal("coach list should include drafts")
	}
}
