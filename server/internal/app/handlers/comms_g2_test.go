package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

type commsG2Fixture struct {
	router       http.Handler
	hhA, swA     string
	hhB, swB     string
	annAll, annG string // group announcement is for the group only household A's swimmer is on
}

func newCommsG2Fixture(t *testing.T) commsG2Fixture {
	t.Helper()
	adminP := &appmiddleware.Principal{UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}}
	msvc, gsvc, csvc := members.NewService(), groups.NewService(), comms.NewService()
	mkHH := func(name string) (string, string) {
		hh, err := msvc.CreateHousehold(adminP, members.CreateHouseholdInput{OrgID: engOrg, HouseholdName: name})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := msvc.CreateSwimmer(adminP, engOrg, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: name, LastName: "S", DateOfBirth: "2012-01-01", Gender: "female", RegistrationStatus: "active"})
		if err != nil {
			t.Fatal(err)
		}
		return hh.ID, sw.ID
	}
	f := commsG2Fixture{}
	f.hhA, f.swA = mkHH("A")
	f.hhB, f.swB = mkHH("B")
	g, _, err := gsvc.CreateGroup(adminP, engOrg, groups.CreateGroupInput{Code: "SENIOR", Name: "Senior", Type: "competitive"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msvc.AssignSwimmerToGroup(adminP, engOrg, f.swA, members.AssignSwimmerGroupInput{GroupCode: "SENIOR", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	pub := func(in comms.CreateAnnouncementInput) string {
		in.OrgID, in.CreatedBy = engOrg, "u-admin"
		a, err := csvc.CreateAnnouncement(in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := csvc.PublishAnnouncement(engOrg, a.ID, "u-admin"); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	f.annAll = pub(comms.CreateAnnouncementInput{Title: "All", Body: "b", Audience: "all"})
	f.annG = pub(comms.CreateAnnouncementInput{Title: "Senior only", Body: "b", Audience: "group", GroupID: g.ID})

	tokens := tokenResolver{
		"admin":   {UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}},
		"parentA": {UserID: "u-pa", OrgID: engOrg, HouseholdID: f.hhA, Roles: []string{"parent"}},
		"parentB": {UserID: "u-pb", OrgID: engOrg, HouseholdID: f.hhB, Roles: []string{"parent"}},
	}
	h := NewCommsHandlers(csvc, msvc, gsvc)
	f.router = g2Router(t, tokens, h.RegisterRoutes)
	return f
}

func TestGroupAnnouncementsOnlyVisibleToMemberHouseholds(t *testing.T) {
	f := newCommsG2Fixture(t)
	listA := g2Do(f.router, http.MethodGet, "/announcements", "parentA", "")
	listB := g2Do(f.router, http.MethodGet, "/announcements", "parentB", "")
	expectStatus(t, listA, http.StatusOK, "list A")
	expectStatus(t, listB, http.StatusOK, "list B")
	if !strings.Contains(listA.Body.String(), f.annG) || !strings.Contains(listA.Body.String(), f.annAll) {
		t.Fatalf("member household should see both: %s", listA.Body.String())
	}
	if strings.Contains(listB.Body.String(), f.annG) || !strings.Contains(listB.Body.String(), f.annAll) {
		t.Fatalf("non-member household must not see the group announcement: %s", listB.Body.String())
	}
	expectStatus(t, g2Do(f.router, http.MethodGet, "/announcements/"+f.annG, "parentB", ""), http.StatusNotFound, "B by id")
	expectStatus(t, g2Do(f.router, http.MethodGet, "/announcements/"+f.annG, "parentA", ""), http.StatusOK, "A by id")
	expectStatus(t, g2Do(f.router, http.MethodGet, "/announcements/"+f.annG, "admin", ""), http.StatusOK, "admin by id")
}

func TestStaffMessageThreadMustExistInOrg(t *testing.T) {
	f := newCommsG2Fixture(t)
	send := func(hh, sw string) int {
		return g2Do(f.router, http.MethodPost, "/messages", "admin", `{"household_id":"`+hh+`","swimmer_id":"`+sw+`","body":"hi"}`).Code
	}
	if got := send("hh-nope", "sw-nope"); got == http.StatusCreated || got == http.StatusOK {
		t.Fatalf("staff message to nonexistent thread accepted: %d", got)
	}
	if got := send(f.hhA, f.swB); got == http.StatusCreated || got == http.StatusOK {
		t.Fatalf("staff message with mismatched household/swimmer accepted: %d", got)
	}
	if got := send(f.hhA, f.swA); got != http.StatusCreated && got != http.StatusOK {
		t.Fatalf("staff message to real thread rejected: %d", got)
	}
	if got := g2Do(f.router, http.MethodPost, "/messages/read", "admin", `{"household_id":"hh-nope","swimmer_id":"sw-nope"}`).Code; got == http.StatusOK || got == http.StatusNoContent {
		t.Fatalf("staff mark-read on nonexistent thread accepted: %d", got)
	}
}
