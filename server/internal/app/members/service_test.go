package members

import (
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/testutil"
)

func TestCreateHouseholdParentAssociatesAccessAndNormalizesFields(t *testing.T) {
	t.Parallel()

	svc := NewService()
	parent := testutil.ParentPrincipal("parent-1", "")

	household, err := svc.CreateHousehold(parent, CreateHouseholdInput{
		OrgID:            testutil.DefaultOrgID,
		HouseholdName:    "  Lane Family  ",
		PrimaryEmail:     "  TEST@EXAMPLE.COM ",
		PrimaryPhoneE164: "  +15555550123 ",
		Notes:            "  prefers text  ",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}

	if household.HouseholdName != "Lane Family" {
		t.Fatalf("expected trimmed household name, got %q", household.HouseholdName)
	}
	if household.PrimaryEmail != "test@example.com" {
		t.Fatalf("expected normalized email, got %q", household.PrimaryEmail)
	}
	if household.Status != "active" {
		t.Fatalf("expected default active status, got %q", household.Status)
	}
	testutil.RequireRecentTime(t, household.CreatedAt, 2*time.Second)

	swimmers, err := svc.ListSwimmers(parent, testutil.DefaultOrgID, ListSwimmersInput{
		HouseholdID: household.ID,
	})
	if err != nil {
		t.Fatalf("parent should have access to the newly created household: %v", err)
	}
	if len(swimmers) != 0 {
		t.Fatalf("expected no swimmers yet, got %d", len(swimmers))
	}
}

func TestCreateSwimmerParentCannotSetMedicalFlags(t *testing.T) {
	t.Parallel()

	svc := NewService()
	parent := testutil.ParentPrincipal("parent-1", "")
	household, err := svc.CreateHousehold(parent, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Lane Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}

	_, _, err = svc.CreateSwimmer(parent, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID:   household.ID,
		FirstName:     "Avery",
		LastName:      "Lane",
		DateOfBirth:   "2012-07-04",
		Gender:        "female",
		MedicalFlags:  []string{"Peanut", "peanut"},
		PreferredName: "Ave",
	})
	testutil.RequireErrorIs(t, err, ErrMedicalFlagsRestricted)
}

func TestAssociateParentToHouseholdAllowsScopedAccess(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Shore Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}

	if err := svc.AssociateParentToHousehold(admin, testutil.DefaultOrgID, "parent-2", household.ID); err != nil {
		t.Fatalf("AssociateParentToHousehold returned error: %v", err)
	}

	parent := testutil.ParentPrincipal("parent-2", household.ID)
	swimmer, auditEvent, err := svc.CreateSwimmer(parent, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Mila",
		LastName:           "Shore",
		DateOfBirth:        "2011-02-03",
		Gender:             "female",
		SwimmerExternalID:  "ABC123",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer returned error: %v", err)
	}

	if swimmer.CreatedBy != "parent-2" {
		t.Fatalf("expected parent to be recorded as creator, got %q", swimmer.CreatedBy)
	}
	if auditEvent.Action != "swimmer.profile_created" {
		t.Fatalf("expected create audit action, got %q", auditEvent.Action)
	}
	if auditEvent.After["household_id"] != household.ID {
		t.Fatalf("expected audit event to include household_id %q, got %#v", household.ID, auditEvent.After["household_id"])
	}
}

func TestUpdateSwimmerParentCannotChangeRegistrationStatus(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Current Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}
	if err := svc.AssociateParentToHousehold(admin, testutil.DefaultOrgID, "parent-3", household.ID); err != nil {
		t.Fatalf("AssociateParentToHousehold returned error: %v", err)
	}
	swimmer, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Kai",
		LastName:           "Current",
		DateOfBirth:        "2010-09-10",
		Gender:             "male",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer returned error: %v", err)
	}

	parent := testutil.ParentPrincipal("parent-3", household.ID)
	_, _, err = svc.UpdateSwimmer(parent, testutil.DefaultOrgID, swimmer.ID, UpdateSwimmerInput{
		RegistrationStatus: testutil.StringPtr("inactive"),
	})
	testutil.RequireErrorIs(t, err, ErrForbidden)
}

func TestListSwimmersRedactsMedicalFlagsForBillingRole(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "River Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}
	swimmer, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Noah",
		LastName:           "River",
		DateOfBirth:        "2013-05-11",
		Gender:             "male",
		MedicalFlags:       []string{"Asthma", "asthma", " inhaler "},
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer returned error: %v", err)
	}

	results, err := svc.ListSwimmers(testutil.BillingPrincipal(), testutil.DefaultOrgID, ListSwimmersInput{
		IncludeMedical: true,
	})
	if err != nil {
		t.Fatalf("ListSwimmers returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 swimmer, got %d", len(results))
	}
	if results[0].ID != swimmer.ID {
		t.Fatalf("expected swimmer %q, got %q", swimmer.ID, results[0].ID)
	}
	if !results[0].MedicalFlagsRedacted {
		t.Fatal("expected billing role medical flags to be redacted")
	}
	if len(results[0].MedicalFlags) != 0 {
		t.Fatalf("expected medical flags to be hidden, got %#v", results[0].MedicalFlags)
	}
}
