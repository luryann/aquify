package groups

import (
	"errors"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestAttachEligibilityRuleRejectsBadParameterTypes(t *testing.T) {
	svc := NewService()
	svc.groups["g1"] = Group{ID: "g1", OrgID: "o", IsActive: true}
	admin := &appmiddleware.Principal{UserID: "a", OrgID: "o", Roles: []string{"admin"}}
	for _, in := range []AttachEligibilityRuleInput{
		{RuleType: "gender", Parameters: map[string]any{"allowed": "female"}},
		{RuleType: "age_range", Parameters: map[string]any{"max_age": "12"}},
	} {
		if _, _, err := svc.AttachEligibilityRule(admin, "o", "g1", in); err == nil {
			t.Fatalf("expected error for %v", in.Parameters)
		}
	}
	if _, _, err := svc.AttachEligibilityRule(admin, "o", "g1", AttachEligibilityRuleInput{RuleType: "gender", Parameters: map[string]any{"allowed": []any{"F"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestCoachCannotUpdateOtherCoachesGroup(t *testing.T) {
	svc := NewService()
	svc.groups["g1"] = Group{ID: "g1", OrgID: "o", CoachUserID: "coachB", IsActive: true}
	coachA := &appmiddleware.Principal{UserID: "coachA", OrgID: "o", Roles: []string{"coach"}}
	off := false
	if _, _, err := svc.UpdateGroup(coachA, "o", "g1", UpdateGroupInput{IsActive: &off}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.GetGroup(coachA, "o", "g1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("get got %v", err)
	}
}

func TestRegistrationInactiveGroupAndWithdrawnReregister(t *testing.T) {
	svc := NewService()
	svc.groups["g1"] = Group{ID: "g1", OrgID: "o", IsActive: false}
	svc.SeedSwimmer("o", "s1", "h1", "2012-01-01", "F", "active")
	admin := &appmiddleware.Principal{UserID: "a", OrgID: "o", Roles: []string{"admin"}}
	in := CreateRegistrationInput{SwimmerID: "s1", HouseholdID: "h1", GroupID: "g1", Season: "2026"}
	if _, err := svc.CreateRegistration(admin, "o", in); !errors.Is(err, ErrGroupInactive) {
		t.Fatalf("got %v", err)
	}
	g := svc.groups["g1"]
	g.IsActive = true
	svc.groups["g1"] = g
	reg, err := svc.CreateRegistration(admin, "o", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRegistration(admin, "o", in); !errors.Is(err, ErrDuplicateRegistration) {
		t.Fatalf("got %v", err)
	}
	r := svc.registrations[reg.ID]
	r.Status = "withdrawn"
	svc.registrations[reg.ID] = r
	again, err := svc.CreateRegistration(admin, "o", in)
	if err != nil || again.ID != reg.ID || again.Status != "pending" {
		t.Fatalf("re-register: %+v %v", again, err)
	}
}

func TestParentOwnsHouseholdViaDurableKeycloakLink(t *testing.T) {
	svc := NewService()
	svc.households["h1"] = householdRef{ID: "h1", OrgID: "o", KeycloakUserID: "p1"}
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if !svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{UserID: "p1"}, "h1") {
		t.Fatal("expected ownership")
	}
	if svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{UserID: "p2"}, "h1") {
		t.Fatal("unexpected ownership")
	}
}
