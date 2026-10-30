package members

import (
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
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

func TestUpdateSwimmerSwimmerCanUpdateOwnBioOnly(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Bio Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}
	swimmer, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: household.ID,
		FirstName:   "Mia",
		LastName:    "Bio",
		DateOfBirth: "2011-01-01",
		Gender:      "female",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer returned error: %v", err)
	}

	swimmerPrincipal := testutil.SwimmerPrincipal("swimmer-bio", household.ID)
	updated, auditEvent, err := svc.UpdateSwimmer(swimmerPrincipal, testutil.DefaultOrgID, swimmer.ID, UpdateSwimmerInput{
		Notes: testutil.StringPtr("  Loves distance free and relay starts.  "),
	})
	if err != nil {
		t.Fatalf("UpdateSwimmer returned error: %v", err)
	}
	if updated.Notes != "Loves distance free and relay starts." {
		t.Fatalf("expected trimmed swimmer bio, got %q", updated.Notes)
	}
	if updated.UpdatedBy != "swimmer-bio" {
		t.Fatalf("expected UpdatedBy swimmer-bio, got %q", updated.UpdatedBy)
	}
	if auditEvent.Action != "swimmer.profile_updated" {
		t.Fatalf("expected profile_updated audit action, got %q", auditEvent.Action)
	}
}

func TestUpdateSwimmerSwimmerCannotEditProtectedFields(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Protected Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}
	swimmer, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: household.ID,
		FirstName:   "Nina",
		LastName:    "Protect",
		DateOfBirth: "2012-03-04",
		Gender:      "female",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer returned error: %v", err)
	}

	swimmerPrincipal := testutil.SwimmerPrincipal("swimmer-protect", household.ID)
	_, _, err = svc.UpdateSwimmer(swimmerPrincipal, testutil.DefaultOrgID, swimmer.ID, UpdateSwimmerInput{
		FirstName: testutil.StringPtr("Changed"),
	})
	testutil.RequireErrorIs(t, err, ErrForbidden)
}

func TestUpdateSwimmerSwimmerBioRejectsOverlongInput(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Long Bio Family",
	})
	if err != nil {
		t.Fatalf("CreateHousehold returned error: %v", err)
	}
	swimmer, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: household.ID,
		FirstName:   "Owen",
		LastName:    "Length",
		DateOfBirth: "2010-08-09",
		Gender:      "male",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer returned error: %v", err)
	}

	tooLong := strings.Repeat("a", maxSwimmerBioLength+1)
	swimmerPrincipal := testutil.SwimmerPrincipal("swimmer-length", household.ID)
	_, _, err = svc.UpdateSwimmer(swimmerPrincipal, testutil.DefaultOrgID, swimmer.ID, UpdateSwimmerInput{
		Notes: &tooLong,
	})
	testutil.RequireErrorIs(t, err, ErrSwimmerBioTooLong)
}

func TestListSwimmersRedactsMedicalFlagsForSwimmerRole(t *testing.T) {
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

	swimmerPrincipal := testutil.SwimmerPrincipal("swimmer-river", household.ID)
	results, err := svc.ListSwimmers(swimmerPrincipal, testutil.DefaultOrgID, ListSwimmersInput{
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
		t.Fatal("expected swimmer role medical flags to be redacted")
	}
	if len(results[0].MedicalFlags) != 0 {
		t.Fatalf("expected medical flags to be hidden, got %#v", results[0].MedicalFlags)
	}
}

func TestAnonymizeHouseholdScrubsEmergencyContactAndAllergies(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "Lane Family", PrimaryEmail: "lane@example.com"})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	swimmer, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: household.ID, FirstName: "Mia", LastName: "Lane", DateOfBirth: "2012-04-15", Gender: "female",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}
	name, phone, rel, allergies := "Pat Lane", "+15555550100", "aunt", "peanuts"
	if _, _, err := svc.UpdateSwimmer(admin, testutil.DefaultOrgID, swimmer.ID, UpdateSwimmerInput{
		EmergencyContactName: &name, EmergencyContactPhone: &phone, EmergencyContactRelationship: &rel, Allergies: &allergies,
	}); err != nil {
		t.Fatalf("UpdateSwimmer: %v", err)
	}

	if err := svc.AnonymizeHousehold(testutil.DefaultOrgID, household.ID); err != nil {
		t.Fatalf("AnonymizeHousehold: %v", err)
	}

	got, err := svc.GetSwimmer(admin, testutil.DefaultOrgID, swimmer.ID, true)
	if err != nil {
		t.Fatalf("GetSwimmer: %v", err)
	}
	if got.FirstName != "Deleted" || got.EmergencyContactName != "" || got.EmergencyContactPhone != "" ||
		got.EmergencyContactRelationship != "" || got.Allergies != "" {
		t.Fatalf("expected identifying fields scrubbed, got %+v", got.Swimmer)
	}
}

func TestUpdateHouseholdParentCannotChangeStatus(t *testing.T) {
	t.Parallel()

	svc := NewService()
	parent := testutil.ParentPrincipal("parent-1", "")
	household, err := svc.CreateHousehold(parent, CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "Lane Family"})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	archived := "archived"
	if _, err := svc.UpdateHousehold(parent, testutil.DefaultOrgID, household.ID, UpdateHouseholdInput{Status: &archived}); err != ErrForbidden {
		t.Fatalf("parent status change = %v, want ErrForbidden", err)
	}
	name := "Lane-Smith Family"
	if _, err := svc.UpdateHousehold(parent, testutil.DefaultOrgID, household.ID, UpdateHouseholdInput{HouseholdName: &name}); err != nil {
		t.Fatalf("parent rename should still work: %v", err)
	}
}

func TestSnapshotRosterIsOrgScoped(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	hh, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "Snap Family"})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := svc.SnapshotRoster(testutil.DefaultOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Households) != 1 || snap.Households[0].ID != hh.ID {
		t.Fatalf("expected the org's household in the snapshot, got %+v", snap.Households)
	}
	other, err := svc.SnapshotRoster("another-org")
	if err != nil || len(other.Households) != 0 || len(other.Swimmers) != 0 {
		t.Fatalf("snapshot leaked across orgs: %+v err=%v", other, err)
	}
}

func TestAssignSwimmerToGroupPromotesExistingLinkToPrimary(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	hh, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "Group Family"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Gus", LastName: "Group", DateOfBirth: "2012-01-02", Gender: "male",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignSwimmerToGroup(admin, testutil.DefaultOrgID, sw.ID, AssignSwimmerGroupInput{GroupCode: "age", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	second, err := svc.AssignSwimmerToGroup(admin, testutil.DefaultOrgID, sw.ID, AssignSwimmerGroupInput{GroupCode: "SENIOR"})
	if err != nil || second.IsPrimary {
		t.Fatalf("second group: %+v err=%v", second, err)
	}

	promoted, err := svc.AssignSwimmerToGroup(admin, testutil.DefaultOrgID, sw.ID, AssignSwimmerGroupInput{GroupCode: "SENIOR", IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	if promoted.ID != second.ID || !promoted.IsPrimary {
		t.Fatalf("existing link should be promoted in place, got %+v", promoted)
	}
	primaries := map[string]bool{}
	links := svc.SnapshotSwimmerGroupLinks(testutil.DefaultOrgID)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	for _, l := range links {
		primaries[l.GroupCode] = l.IsPrimary
	}
	if !primaries["SENIOR"] || primaries["AGE"] {
		t.Fatalf("primary should have moved to SENIOR, got %v", primaries)
	}
}

func TestAnonymizeHouseholdScrubsAuditNameSnapshots(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	household, _ := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "Lane Family"})
	if _, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: household.ID, FirstName: "Mia", LastName: "Lane", DateOfBirth: "2012-04-15", Gender: "female",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AnonymizeHousehold(testutil.DefaultOrgID, household.ID); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Mia", "Lane"} {
		if strings.Contains(string(snap), leak) {
			t.Fatalf("snapshot still contains %q after erasure: %s", leak, snap)
		}
	}
}

func TestAdminWhoIsAlsoParentIsNotHouseholdScoped(t *testing.T) {
	t.Parallel()

	svc := NewService()
	admin := testutil.AdminPrincipal()
	other, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: testutil.DefaultOrgID, HouseholdName: "Other Family"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateSwimmer(admin, testutil.DefaultOrgID, CreateSwimmerInput{
		HouseholdID: other.ID, FirstName: "Kai", LastName: "Other", DateOfBirth: "2012-04-15", Gender: "male",
	}); err != nil {
		t.Fatal(err)
	}

	dual := &appmiddleware.Principal{UserID: "dual-1", OrgID: testutil.DefaultOrgID, Roles: []string{"admin", "parent"}}
	hh, err := svc.ListHouseholds(dual, testutil.DefaultOrgID)
	if err != nil || len(hh) != 1 {
		t.Fatalf("ListHouseholds for admin+parent: %d households, err %v; want org-wide access", len(hh), err)
	}
	sw, err := svc.ListSwimmers(dual, testutil.DefaultOrgID, ListSwimmersInput{HouseholdID: other.ID})
	if err != nil || len(sw) != 1 {
		t.Fatalf("ListSwimmers for admin+parent: %d swimmers, err %v", len(sw), err)
	}
	// A pure parent is still limited to their own households.
	parent := testutil.ParentPrincipal("parent-9", "")
	if hh, _ := svc.ListHouseholds(parent, testutil.DefaultOrgID); len(hh) != 0 {
		t.Fatalf("pure parent saw %d foreign households", len(hh))
	}
}
