package members

import (
	"errors"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func TestGuardianConsentGatesUnder13Accounts(t *testing.T) {
	t.Parallel()
	const org = "org-consent"
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "adm", OrgID: org, Roles: []string{"admin"}}
	hh, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
	if err != nil {
		t.Fatal(err)
	}
	parent := &appmiddleware.Principal{UserID: "par", OrgID: org, HouseholdID: hh.ID, Roles: []string{"parent"}}

	young := time.Now().UTC().AddDate(-10, 0, 0).Format("2006-01-02")
	old := time.Now().UTC().AddDate(-14, 0, 0).Format("2006-01-02")
	mk := func(name, dob string) Swimmer {
		sw, _, err := svc.CreateSwimmer(admin, org, CreateSwimmerInput{HouseholdID: hh.ID, FirstName: name, LastName: "S", DateOfBirth: dob, Gender: "female"})
		if err != nil {
			t.Fatal(err)
		}
		return sw
	}
	kid, teen := mk("Kid", young), mk("Teen", old)

	// Under 13 without consent: cannot link, and sign-in fails closed.
	if err := svc.LinkSwimmerToUser(org, kid.ID, "kc-kid"); !errors.Is(err, ErrGuardianConsentRequired) {
		t.Fatalf("link without consent = %v, want ErrGuardianConsentRequired", err)
	}
	// 13+ is unaffected.
	if err := svc.LinkSwimmerToUser(org, teen.ID, "kc-teen"); err != nil {
		t.Fatalf("teen link: %v", err)
	}

	yes, no := true, false
	// Staff cannot grant consent on a parent's behalf.
	if _, _, err := svc.UpdateSwimmer(admin, org, kid.ID, UpdateSwimmerInput{GuardianConsent: &yes}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin grant = %v, want ErrForbidden", err)
	}
	// A parent from another household, and a coach who also holds the parent role, cannot consent.
	otherHH, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: org, HouseholdName: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	stranger := &appmiddleware.Principal{UserID: "par2", OrgID: org, HouseholdID: otherHH.ID, Roles: []string{"parent"}}
	if _, _, err := svc.UpdateSwimmer(stranger, org, kid.ID, UpdateSwimmerInput{GuardianConsent: &yes}); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("other-household parent grant = %v, want ErrParentHouseholdScope", err)
	}
	coachParent := &appmiddleware.Principal{UserID: "cp", OrgID: org, HouseholdID: otherHH.ID, Roles: []string{"coach", "parent"}}
	if _, _, err := svc.UpdateSwimmer(coachParent, org, kid.ID, UpdateSwimmerInput{GuardianConsent: &yes}); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("coach+parent other-household grant = %v, want ErrParentHouseholdScope", err)
	}
	childSelf := &appmiddleware.Principal{UserID: "kid", OrgID: org, HouseholdID: hh.ID, Roles: []string{"swimmer", "parent"}}
	if _, _, err := svc.UpdateSwimmer(childSelf, org, kid.ID, UpdateSwimmerInput{GuardianConsent: &yes}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer self-consent = %v, want ErrForbidden", err)
	}
	got, _, err := svc.UpdateSwimmer(parent, org, kid.ID, UpdateSwimmerInput{GuardianConsent: &yes})
	if err != nil || got.GuardianConsentAt == nil || got.GuardianConsentBy != "par" || got.GuardianConsentVersion != GuardianConsentVersion {
		t.Fatalf("parent grant = %+v, %v", got, err)
	}
	if err := svc.LinkSwimmerToUser(org, kid.ID, "kc-kid"); err != nil {
		t.Fatalf("link with consent: %v", err)
	}
	if blocked, _ := svc.SwimmerPortalDisabled(org, kid.ID); blocked {
		t.Fatal("consented swimmer should be able to sign in")
	}

	// Withdrawing consent unlinks the account and blocks sign-in again.
	got, _, err = svc.UpdateSwimmer(parent, org, kid.ID, UpdateSwimmerInput{GuardianConsent: &no})
	if err != nil || got.GuardianConsentAt != nil || got.KeycloakUserID != "" {
		t.Fatalf("withdraw = %+v, %v", got, err)
	}
	if blocked, _ := svc.SwimmerPortalDisabled(org, kid.ID); !blocked {
		t.Fatal("swimmer without consent must be blocked")
	}
}
