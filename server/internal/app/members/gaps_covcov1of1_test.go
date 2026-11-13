package members

import (
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestCov1xGapsListPendingBreaksCreatedAtTiesByID(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	svc := &Service{linkRequests: map[string]SwimmerLinkRequest{
		"slr-b": {ID: "slr-b", OrgID: cov1xOrg, UserID: "u2", Status: "pending", CreatedAt: ts},
		"slr-a": {ID: "slr-a", OrgID: cov1xOrg, UserID: "u1", Status: "pending", CreatedAt: ts},
		"slr-c": {ID: "slr-c", OrgID: cov1xOrg, UserID: "u3", Status: "pending", CreatedAt: ts.Add(-time.Hour)},
	}}
	got, err := svc.ListPendingLinkRequests(cov1xOrg)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if want := []string{"slr-c", "slr-a", "slr-b"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
	if !sort.StringsAreSorted(ids[1:]) {
		t.Fatalf("ties must be ordered by ID: %v", ids)
	}
}

func TestCov1xGapsErasureContextIgnoresOtherHouseholdContacts(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	other, err := e.svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddContact(e.admin, cov1xOrg, other.ID, AddContactInput{Relationship: "dad", FirstName: "Oz", LastName: "O", Email: "oz@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddContact(e.admin, cov1xOrg, e.hh.ID, AddContactInput{Relationship: "mom", FirstName: "Me", LastName: "M", Email: "me@example.com"}); err != nil {
		t.Fatal(err)
	}
	ctx, err := e.svc.GetHouseholdErasureContext(cov1xOrg, e.hh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ctx.ContactEmails, []string{"me@example.com"}) {
		t.Fatalf("emails = %v, want only own household's contact", ctx.ContactEmails)
	}
}

func TestCov1xGapsUpdateSwimmerRefusesSwimmerWithOnlyOperatorRole(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	// A swimmer who also holds only an operator role is still "swimmer only":
	// it may edit notes but never portal access or guardian consent.
	odd := &appmiddleware.Principal{UserID: "odd", OrgID: cov1xOrg, HouseholdID: e.hh.ID, Roles: []string{"swimmer", "platform_admin"}}
	yes, no := true, false
	if _, _, err := e.svc.UpdateSwimmer(odd, cov1xOrg, e.sw.ID, UpdateSwimmerInput{PortalAccessDisabled: &yes}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("portal toggle err = %v, want ErrForbidden", err)
	}
	if _, _, err := e.svc.UpdateSwimmer(odd, cov1xOrg, e.sw.ID, UpdateSwimmerInput{GuardianConsent: &no}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("consent withdraw err = %v, want ErrForbidden", err)
	}
}

func TestCov1xGapsDeleteSwimmerKeepsOtherSwimmersGroupLinks(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	buddy := e.newSwimmer(t, e.hh.ID, "Buddy")
	for _, id := range []string{e.sw.ID, buddy.ID} {
		if _, err := e.svc.AssignSwimmerToGroup(e.admin, cov1xOrg, id, AssignSwimmerGroupInput{GroupCode: "GOLD", IsPrimary: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := e.svc.DeleteSwimmer(e.admin, cov1xOrg, e.sw.ID); err != nil {
		t.Fatal(err)
	}
	links, err := e.svc.ListSwimmerGroupLinks(e.admin, cov1xOrg)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].SwimmerID != buddy.ID {
		t.Fatalf("links = %+v, want only buddy's", links)
	}
}
