package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestAddContactWritesAuditEvent(t *testing.T) {
	svc := members.NewService()
	adminP := &appmiddleware.Principal{UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}}
	hh, err := svc.CreateHousehold(adminP, members.CreateHouseholdInput{OrgID: engOrg, HouseholdName: "Smith"})
	if err != nil {
		t.Fatal(err)
	}
	auditSvc := audit.NewService()
	h := NewMembershipHandlersWithAudit(svc, auditSvc)
	r := g2Router(t, g2Tokens, h.RegisterProtectedRoutes)
	rr := g2Do(r, http.MethodPost, "/households/"+hh.ID+"/contacts", "admin",
		`{"relationship":"guardian","first_name":"Pat","last_name":"Smith"}`)
	expectStatus(t, rr, http.StatusCreated, "add contact")
	var out struct {
		Contact members.Contact `json:"contact"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	evs, err := auditSvc.Query(audit.Filter{OrgID: engOrg, Action: "members.contact.create"})
	if err != nil || len(evs) != 1 || evs[0].ResourceID != out.Contact.ID {
		t.Fatalf("want one members.contact.create audit event for %q, got %+v (%v)", out.Contact.ID, evs, err)
	}
}

func TestHouseholdPrimaryEmailNormalisation(t *testing.T) {
	svc := members.NewService()
	adminP := &appmiddleware.Principal{UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}}
	h := NewMembershipHandlers(svc)
	r := g2Router(t, g2Tokens, h.RegisterProtectedRoutes)

	create := func(email string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"org_id": engOrg, "household_name": "Lee", "primary_email": email})
		return g2Do(r, http.MethodPost, "/households", "admin", string(body))
	}
	expectStatus(t, create("not-an-email"), http.StatusBadRequest, "create invalid email")
	expectStatus(t, create("  also bad  "), http.StatusBadRequest, "create padded invalid email")
	expectStatus(t, create(""), http.StatusCreated, "create empty email")
	rr := create("  Pat@Example.com ")
	expectStatus(t, rr, http.StatusCreated, "create padded email")
	var out struct {
		Household members.Household `json:"household"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Household.PrimaryEmail != "pat@example.com" {
		t.Fatalf("padded email not normalised: %q", out.Household.PrimaryEmail)
	}

	patch := func(raw string) *httptest.ResponseRecorder {
		return g2Do(r, http.MethodPatch, "/households/"+out.Household.ID, "admin", `{"primary_email":`+raw+`}`)
	}
	expectStatus(t, patch(`"nope"`), http.StatusBadRequest, "update invalid email")
	expectStatus(t, patch(`" Sam@Example.com "`), http.StatusOK, "update padded email")
	got, err := svc.GetHousehold(adminP, engOrg, out.Household.ID)
	if err != nil || got.PrimaryEmail != "sam@example.com" {
		t.Fatalf("padded update not normalised: %+v %v", got, err)
	}
	expectStatus(t, patch(`""`), http.StatusOK, "update clears email")
	got, _ = svc.GetHousehold(adminP, engOrg, out.Household.ID)
	if got.PrimaryEmail != "" {
		t.Fatalf("email not cleared: %q", got.PrimaryEmail)
	}
}
