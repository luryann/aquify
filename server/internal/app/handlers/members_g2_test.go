package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
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
