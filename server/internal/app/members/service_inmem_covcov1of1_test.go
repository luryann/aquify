package members

import (
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov1xOrg = "org-cov1x"

func cov1xStr(s string) *string { return &s }
func cov1xBool(b bool) *bool    { return &b }

func cov1xAdmin() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "cov1x-admin", OrgID: cov1xOrg, Roles: []string{"admin"}}
}

func cov1xCoach() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "cov1x-coach", OrgID: cov1xOrg, Roles: []string{"coach"}}
}

func cov1xParent(userID, householdID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: userID, OrgID: cov1xOrg, HouseholdID: householdID, Roles: []string{"parent"}}
}

func cov1xSwimmerPrincipal(userID, householdID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: userID, OrgID: cov1xOrg, HouseholdID: householdID, Roles: []string{"swimmer"}}
}

type cov1xEnv struct {
	svc   *Service
	admin *appmiddleware.Principal
	hh    Household
	sw    Swimmer
}

// cov1xNewEnv builds an in-memory service holding one household with one adult swimmer.
func cov1xNewEnv(t *testing.T) cov1xEnv {
	t.Helper()
	svc := NewService()
	admin := cov1xAdmin()
	hh, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Alpha"})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	sw, _, err := svc.CreateSwimmer(admin, cov1xOrg, CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Ada", LastName: "Alpha", DateOfBirth: "2000-01-02", Gender: "female",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}
	return cov1xEnv{svc: svc, admin: admin, hh: hh, sw: sw}
}

func (e cov1xEnv) newSwimmer(t *testing.T, hhID, first string) Swimmer {
	t.Helper()
	sw, _, err := e.svc.CreateSwimmer(e.admin, cov1xOrg, CreateSwimmerInput{
		HouseholdID: hhID, FirstName: first, LastName: "Zed", DateOfBirth: "2001-03-04", Gender: "male",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer(%s): %v", first, err)
	}
	return sw
}

func TestCov1xCheckLengths(t *testing.T) {
	t.Parallel()
	if err := checkLengths("a", [2]any{"abc", 3}, "b", [2]any{"héllo", 5}); err != nil {
		t.Fatalf("within limits must pass: %v", err)
	}
	err := checkLengths("a", [2]any{"abc", 3}, "name", [2]any{"abcd", 3})
	if !errors.Is(err, ErrFieldTooLong) || !strings.Contains(err.Error(), "name must be 3 characters or fewer") {
		t.Fatalf("over-limit err = %v", err)
	}
	// A value that is not a [2]any pair is ignored; a dangling key is ignored.
	if err := checkLengths("a", "not-a-pair", "dangling"); err != nil {
		t.Fatalf("non-pair values are skipped, got %v", err)
	}
}

func TestCov1xNilPoolServiceIsInMemoryAndRefreshIsNoop(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil)
	if svc.repo != nil {
		t.Fatal("nil pool must yield an in-memory service")
	}
	if err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if err := svc.RefreshOrg(t.Context(), cov1xOrg); err != nil {
		t.Fatalf("RefreshOrg: %v", err)
	}
	if id := svc.newID("p_"); !strings.HasPrefix(id, "p_") || len(id) != len("p_")+24 {
		t.Fatalf("in-memory ids keep the prefix-token format, got %q", id)
	}
	if got := randomToken(4); len(got) != 8 {
		t.Fatalf("randomToken(4) = %q, want 8 hex chars", got)
	}
}

func TestCov1xCreateHouseholdRules(t *testing.T) {
	t.Parallel()
	svc := NewService()

	if _, err := svc.CreateHousehold(cov1xSwimmerPrincipal("s", ""), CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "X"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer create err = %v, want ErrForbidden", err)
	}
	if _, err := svc.CreateHousehold(cov1xAdmin(), CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "   "}); err == nil || !strings.Contains(err.Error(), "household_name are required") {
		t.Fatalf("blank name err = %v", err)
	}
	if _, err := svc.CreateHousehold(cov1xAdmin(), CreateHouseholdInput{OrgID: "", HouseholdName: "X"}); err == nil {
		t.Fatal("blank org must be rejected")
	}

	type link struct{ user, hh string }
	var links []link
	svc.SetParentHouseholdLinkedHook(func(u, h string) { links = append(links, link{u, h}) })

	parent := cov1xParent("cov1x-par", "")
	first, err := svc.CreateHousehold(parent, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "First", Status: " PAUSED "})
	if err != nil {
		t.Fatal(err)
	}
	if first.KeycloakUserID != "cov1x-par" {
		t.Fatalf("a parent's first household links their user, got %q", first.KeycloakUserID)
	}
	if first.Status != "paused" {
		t.Fatalf("status normalised, got %q", first.Status)
	}
	second, err := svc.CreateHousehold(parent, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if second.KeycloakUserID != "" {
		t.Fatalf("later households are not linked durably, got %q", second.KeycloakUserID)
	}
	explicit, err := svc.CreateHousehold(cov1xParent("cov1x-par2", ""), CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Third", KeycloakUserID: " kc-explicit "})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.KeycloakUserID != "kc-explicit" {
		t.Fatalf("explicit keycloak id kept, got %q", explicit.KeycloakUserID)
	}
	if len(links) != 3 || links[0] != (link{"cov1x-par", first.ID}) || links[1].hh != second.ID {
		t.Fatalf("link hook calls = %+v", links)
	}
}

func TestCov1xEnsureSeedHousehold(t *testing.T) {
	t.Parallel()
	svc := NewService()
	seed := Household{ID: "cov1x-seed", OrgID: cov1xOrg, HouseholdName: "Seed"}
	if err := svc.EnsureSeedHousehold(seed, " cov1x-seed-parent "); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetHousehold(cov1xAdmin(), cov1xOrg, "cov1x-seed")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" || got.CreatedAt.IsZero() || got.CreatedAt != got.UpdatedAt {
		t.Fatalf("seed defaults wrong: %+v", got)
	}
	// The seeded parent can reach the household through the association index.
	if list, err := svc.ListHouseholds(cov1xParent("cov1x-seed-parent", ""), cov1xOrg); err != nil || len(list) != 1 || list[0].ID != "cov1x-seed" {
		t.Fatalf("seeded parent list = %+v, %v", list, err)
	}
	// Re-seeding leaves the record untouched and a blank parent associates nobody.
	seed.HouseholdName = "Changed"
	if err := svc.EnsureSeedHousehold(seed, "  "); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetHousehold(cov1xAdmin(), cov1xOrg, "cov1x-seed"); got.HouseholdName != "Seed" {
		t.Fatalf("existing record must not be overwritten, got %q", got.HouseholdName)
	}
	if len(svc.parentHouseholdIndex) != 1 {
		t.Fatalf("blank parent must not be indexed: %v", svc.parentHouseholdIndex)
	}
}

func TestCov1xUpdateHousehold(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	parent := cov1xParent("cov1x-p", e.hh.ID)

	if _, err := svc.UpdateHousehold(cov1xSwimmerPrincipal("s", e.hh.ID), cov1xOrg, e.hh.ID, UpdateHouseholdInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer err = %v", err)
	}
	if _, err := svc.UpdateHousehold(e.admin, cov1xOrg, "missing", UpdateHouseholdInput{}); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := svc.UpdateHousehold(e.admin, "other-org", e.hh.ID, UpdateHouseholdInput{}); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("cross-org err = %v", err)
	}
	if _, err := svc.UpdateHousehold(cov1xParent("stranger", "elsewhere"), cov1xOrg, e.hh.ID, UpdateHouseholdInput{}); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("foreign parent err = %v", err)
	}
	if _, err := svc.UpdateHousehold(parent, cov1xOrg, e.hh.ID, UpdateHouseholdInput{Status: cov1xStr("archived")}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent status err = %v", err)
	}
	if _, err := svc.UpdateHousehold(e.admin, cov1xOrg, e.hh.ID, UpdateHouseholdInput{HouseholdName: cov1xStr("  ")}); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty name err = %v", err)
	}
	if got, _ := svc.GetHousehold(e.admin, cov1xOrg, e.hh.ID); got.HouseholdName != "Alpha" {
		t.Fatalf("a rejected update must not change state, got %q", got.HouseholdName)
	}

	got, err := svc.UpdateHousehold(e.admin, cov1xOrg, e.hh.ID, UpdateHouseholdInput{
		HouseholdName:    cov1xStr(" Beta "),
		PrimaryEmail:     cov1xStr(" Beta@Example.COM "),
		PrimaryPhoneE164: cov1xStr(" +15550001111 "),
		Status:           cov1xStr(" Archived "),
		Notes:            cov1xStr(" note "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.HouseholdName != "Beta" || got.PrimaryEmail != "beta@example.com" || got.PrimaryPhoneE164 != "+15550001111" ||
		got.Status != "archived" || got.Notes != "note" || got.UpdatedBy != e.admin.UserID {
		t.Fatalf("update result wrong: %+v", got)
	}
	got, err = svc.UpdateHousehold(parent, cov1xOrg, e.hh.ID, UpdateHouseholdInput{HouseholdName: cov1xStr("Gamma")})
	if err != nil || got.HouseholdName != "Gamma" || got.Status != "archived" || got.UpdatedBy != "cov1x-p" {
		t.Fatalf("parent rename = %+v, %v", got, err)
	}
}

func TestCov1xDeleteAndGetHousehold(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc

	if _, err := svc.DeleteHousehold(cov1xCoach(), cov1xOrg, e.hh.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach delete err = %v", err)
	}
	if _, err := svc.DeleteHousehold(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := svc.DeleteHousehold(e.admin, cov1xOrg, e.hh.ID); !errors.Is(err, ErrHouseholdHasSwimmers) {
		t.Fatalf("household with swimmers err = %v", err)
	}
	empty, err := svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	if !svc.HouseholdExists(cov1xOrg, empty.ID) || svc.HouseholdExists("other-org", empty.ID) || svc.HouseholdExists(cov1xOrg, "nope") {
		t.Fatal("HouseholdExists must be org-scoped")
	}
	if _, err := svc.GetHousehold(cov1xSwimmerPrincipal("s", ""), cov1xOrg, empty.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer get err = %v", err)
	}
	if _, err := svc.GetHousehold(e.admin, cov1xOrg, "nope"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("get missing err = %v", err)
	}
	if _, err := svc.GetHousehold(cov1xParent("stranger", ""), cov1xOrg, empty.ID); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("foreign parent get err = %v", err)
	}
	if got, err := svc.GetHousehold(e.admin, cov1xOrg, " "+empty.ID+" "); err != nil || got.ID != empty.ID {
		t.Fatalf("admin get = %+v, %v", got, err)
	}
	deleted, err := svc.DeleteHousehold(e.admin, cov1xOrg, empty.ID)
	if err != nil || deleted.ID != empty.ID {
		t.Fatalf("delete = %+v, %v", deleted, err)
	}
	if svc.HouseholdExists(cov1xOrg, empty.ID) {
		t.Fatal("deleted household must be gone")
	}
}

func TestCov1xListHouseholdsScoping(t *testing.T) {
	t.Parallel()
	svc := NewService()
	admin := cov1xAdmin()
	mk := func(name string) Household {
		h, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: name})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	b1, b2, a := mk("Bravo"), mk("Bravo"), mk("Alpha")
	if _, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: "org-cov1x-other", HouseholdName: "Elsewhere"}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ListHouseholds(&appmiddleware.Principal{UserID: "x", OrgID: cov1xOrg, Roles: []string{"bogus"}}, cov1xOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unknown role err = %v", err)
	}
	all, err := svc.ListHouseholds(admin, cov1xOrg)
	if err != nil || len(all) != 3 || all[0].ID != a.ID {
		t.Fatalf("admin list = %+v, %v", all, err)
	}
	wantTie := []string{b1.ID, b2.ID}
	if wantTie[0] > wantTie[1] {
		wantTie[0], wantTie[1] = wantTie[1], wantTie[0]
	}
	if all[1].ID != wantTie[0] || all[2].ID != wantTie[1] {
		t.Fatalf("equal names must tie-break on ID: %v", []string{all[1].ID, all[2].ID})
	}

	if _, err := svc.ListHouseholds(cov1xSwimmerPrincipal("s", "  "), cov1xOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer without household err = %v", err)
	}
	if list, err := svc.ListHouseholds(cov1xSwimmerPrincipal("s", b2.ID), cov1xOrg); err != nil || len(list) != 1 || list[0].ID != b2.ID {
		t.Fatalf("swimmer list = %+v, %v", list, err)
	}
	if err := svc.AssociateParentToHousehold(admin, cov1xOrg, "cov1x-par", a.ID); err != nil {
		t.Fatal(err)
	}
	if list, err := svc.ListHouseholds(cov1xParent("cov1x-par", b1.ID), cov1xOrg); err != nil || len(list) != 2 {
		t.Fatalf("parent sees indexed + principal household, got %+v, %v", list, err)
	}
}

func TestCov1xContacts(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	in := AddContactInput{Relationship: " Mother ", FirstName: " Mia ", LastName: " Moss ", Email: " MIA@Example.com ", PhoneE164: " +1555 ", Notes: " n "}

	if _, err := svc.AddContact(cov1xSwimmerPrincipal("s", e.hh.ID), cov1xOrg, e.hh.ID, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer add err = %v", err)
	}
	if _, err := svc.AddContact(e.admin, cov1xOrg, "missing", in); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("missing hh err = %v", err)
	}
	if _, err := svc.AddContact(cov1xParent("stranger", ""), cov1xOrg, e.hh.ID, in); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("foreign parent err = %v", err)
	}
	if _, err := svc.AddContact(e.admin, cov1xOrg, e.hh.ID, AddContactInput{FirstName: "A", LastName: "B"}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing relationship err = %v", err)
	}
	long := in
	long.Notes = strings.Repeat("n", maxNotesLength+1)
	if _, err := svc.AddContact(e.admin, cov1xOrg, e.hh.ID, long); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("long notes err = %v", err)
	}

	c1, err := svc.AddContact(e.admin, cov1xOrg, e.hh.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Relationship != "mother" || c1.FirstName != "Mia" || c1.Email != "mia@example.com" || c1.PhoneE164 != "+1555" || c1.Notes != "n" {
		t.Fatalf("contact not normalised: %+v", c1)
	}
	mustAdd := func(first, last string, primary bool) Contact {
		c, err := svc.AddContact(e.admin, cov1xOrg, e.hh.ID, AddContactInput{Relationship: "aunt", FirstName: first, LastName: last, IsPrimary: primary})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	prim := mustAdd("Pat", "Zulu", true)
	sameLast := mustAdd("Abe", "Moss", false)
	able := mustAdd("Aaron", "Able", false)
	// A contact of another household never shows up here.
	hhOther, err := svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddContact(e.admin, cov1xOrg, hhOther.ID, AddContactInput{Relationship: "aunt", FirstName: "Zed", LastName: "Zzz"}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 64; i++ { // map order is random; make both primary-vs-not comparisons occur
		list, err := svc.ListContacts(e.admin, cov1xOrg, e.hh.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 4 || list[0].ID != prim.ID || list[1].ID != able.ID || list[2].ID != sameLast.ID || list[3].ID != c1.ID {
			t.Fatalf("contact order = %+v", list)
		}
	}
	if _, err := svc.ListContacts(cov1xSwimmerPrincipal("s", e.hh.ID), cov1xOrg, e.hh.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer list err = %v", err)
	}
	if _, err := svc.ListContacts(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("list missing err = %v", err)
	}
	if _, err := svc.ListContacts(cov1xParent("stranger", ""), cov1xOrg, e.hh.ID); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("list foreign parent err = %v", err)
	}
	if got := svc.SnapshotContacts(cov1xOrg); len(got) != 5 {
		t.Fatalf("SnapshotContacts = %d", len(got))
	}
	if got := svc.SnapshotContacts("other"); len(got) != 0 {
		t.Fatalf("other org contacts = %d", len(got))
	}

	for i := 4; i < maxContactsPerHousehold; i++ {
		mustAdd("Fill", "Er", false)
	}
	if _, err := svc.AddContact(e.admin, cov1xOrg, e.hh.ID, in); !errors.Is(err, ErrFieldTooLong) || !strings.Contains(err.Error(), "at most 20 contacts") {
		t.Fatalf("contact cap err = %v", err)
	}
}

func TestCov1xCreateSwimmerRules(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	base := CreateSwimmerInput{HouseholdID: e.hh.ID, FirstName: "Kit", LastName: "Lee", DateOfBirth: "2010-05-06", Gender: "Male"}
	with := func(f func(*CreateSwimmerInput)) CreateSwimmerInput { in := base; f(&in); return in }

	var created []Swimmer
	svc.SetSwimmerCreatedHook(func(sw Swimmer) { created = append(created, sw) })

	cases := []struct {
		name string
		p    *appmiddleware.Principal
		in   CreateSwimmerInput
		is   error
		sub  string
	}{
		{"role", cov1xSwimmerPrincipal("s", e.hh.ID), base, ErrForbidden, ""},
		{"dob", e.admin, with(func(i *CreateSwimmerInput) { i.DateOfBirth = "05/06/2010" }), ErrInvalidDateOfBirth, ""},
		{"gender", e.admin, with(func(i *CreateSwimmerInput) { i.Gender = "robot" }), nil, "gender must be one of"},
		{"household", e.admin, with(func(i *CreateSwimmerInput) { i.HouseholdID = "nope" }), ErrHouseholdNotFound, ""},
		{"scope", cov1xParent("stranger", ""), base, ErrParentHouseholdScope, ""},
		{"names", e.admin, with(func(i *CreateSwimmerInput) { i.FirstName = "  " }), nil, "are required"},
		{"toolong", e.admin, with(func(i *CreateSwimmerInput) { i.PreferredName = strings.Repeat("p", maxShortFieldLength+1) }), ErrFieldTooLong, ""},
		{"medical", cov1xParent("par", e.hh.ID), with(func(i *CreateSwimmerInput) { i.MedicalFlags = []string{"asthma"} }), ErrMedicalFlagsRestricted, ""},
	}
	for _, tc := range cases {
		_, _, err := svc.CreateSwimmer(tc.p, cov1xOrg, tc.in)
		switch {
		case tc.is != nil && !errors.Is(err, tc.is):
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.is)
		case tc.sub != "" && (err == nil || !strings.Contains(err.Error(), tc.sub)):
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.sub)
		}
	}
	if len(created) != 0 {
		t.Fatalf("hook must not fire on failure: %+v", created)
	}

	first, ev, err := svc.CreateSwimmer(e.admin, cov1xOrg, with(func(i *CreateSwimmerInput) {
		i.SwimmerExternalID, i.USASwimmingID, i.MedicalFlags = "EXT-9", "USA-9", []string{" Asthma ", "asthma"}
		i.RegistrationStatus = " PENDING "
	}))
	if err != nil {
		t.Fatal(err)
	}
	if first.Gender != "male" || first.RegistrationStatus != "pending" || len(first.MedicalFlags) != 1 || first.MedicalFlags[0] != "asthma" {
		t.Fatalf("swimmer not normalised: %+v", first)
	}
	if ev.Action != "swimmer.profile_created" || ev.SwimmerID != first.ID || ev.After["guardian_consent"] != false {
		t.Fatalf("audit event = %+v", ev)
	}
	if len(created) != 1 || created[0].ID != first.ID {
		t.Fatalf("hook calls = %+v", created)
	}
	if sw, _, err := svc.CreateSwimmer(e.admin, cov1xOrg, base); err != nil || sw.RegistrationStatus != "active" {
		t.Fatalf("default status = %+v, %v", sw, err)
	}
	if _, _, err := svc.CreateSwimmer(e.admin, cov1xOrg, with(func(i *CreateSwimmerInput) { i.SwimmerExternalID = "ext-9" })); err == nil || !strings.Contains(err.Error(), "swimmer_external_id") {
		t.Fatalf("dup external id err = %v", err)
	}
	if _, _, err := svc.CreateSwimmer(e.admin, cov1xOrg, with(func(i *CreateSwimmerInput) { i.USASwimmingID = "usa-9" })); err == nil || !strings.Contains(err.Error(), "usa_swimming_id") {
		t.Fatalf("dup usa id err = %v", err)
	}
	// The same external id in another org is fine.
	otherAdmin := &appmiddleware.Principal{UserID: "o", OrgID: "org-cov1x-b", Roles: []string{"admin"}}
	oh, err := svc.CreateHousehold(otherAdmin, CreateHouseholdInput{OrgID: "org-cov1x-b", HouseholdName: "O"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateSwimmer(otherAdmin, "org-cov1x-b", with(func(i *CreateSwimmerInput) { i.HouseholdID = oh.ID; i.SwimmerExternalID = "EXT-9" })); err != nil {
		t.Fatalf("uniqueness is per org: %v", err)
	}
}

func TestCov1xUpdateSwimmerRejections(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	other := e.newSwimmer(t, e.hh.ID, "Other")
	if _, _, err := svc.UpdateSwimmer(e.admin, cov1xOrg, other.ID, UpdateSwimmerInput{SwimmerExternalID: cov1xStr("EXT-A"), USASwimmingID: cov1xStr("USA-A")}); err != nil {
		t.Fatal(err)
	}
	parent := cov1xParent("cov1x-par", e.hh.ID)
	swimmer := cov1xSwimmerPrincipal("cov1x-sw", e.hh.ID)
	dual := &appmiddleware.Principal{UserID: "cov1x-dual", OrgID: cov1xOrg, HouseholdID: e.hh.ID, Roles: []string{"parent", "swimmer"}}
	adminParent := &appmiddleware.Principal{UserID: "cov1x-adm-par", OrgID: cov1xOrg, Roles: []string{"admin", "parent"}}

	cases := []struct {
		name string
		p    *appmiddleware.Principal
		id   string
		in   UpdateSwimmerInput
		is   error
		sub  string
	}{
		{"role", &appmiddleware.Principal{UserID: "x", OrgID: cov1xOrg, Roles: []string{"bogus"}}, e.sw.ID, UpdateSwimmerInput{}, ErrForbidden, ""},
		{"missing", e.admin, "nope", UpdateSwimmerInput{}, ErrSwimmerNotFound, ""},
		{"parent scope", cov1xParent("stranger", ""), e.sw.ID, UpdateSwimmerInput{}, ErrParentHouseholdScope, ""},
		{"swimmer other household", cov1xSwimmerPrincipal("s", "elsewhere"), e.sw.ID, UpdateSwimmerInput{Notes: cov1xStr("x")}, ErrForbidden, ""},
		{"swimmer no household", cov1xSwimmerPrincipal("s", ""), e.sw.ID, UpdateSwimmerInput{Notes: cov1xStr("x")}, ErrForbidden, ""},
		{"swimmer protected field", swimmer, e.sw.ID, UpdateSwimmerInput{Notes: cov1xStr("x"), FirstName: cov1xStr("Z")}, ErrForbidden, ""},
		{"swimmer bio too long", swimmer, e.sw.ID, UpdateSwimmerInput{Notes: cov1xStr(strings.Repeat("b", maxSwimmerBioLength+1))}, ErrSwimmerBioTooLong, ""},
		{"dob", e.admin, e.sw.ID, UpdateSwimmerInput{DateOfBirth: cov1xStr("nope")}, ErrInvalidDateOfBirth, ""},
		{"gender", e.admin, e.sw.ID, UpdateSwimmerInput{Gender: cov1xStr("robot")}, nil, "gender must be one of"},
		{"ext id taken", e.admin, e.sw.ID, UpdateSwimmerInput{SwimmerExternalID: cov1xStr("ext-a")}, nil, "swimmer_external_id"},
		{"usa id taken", e.admin, e.sw.ID, UpdateSwimmerInput{USASwimmingID: cov1xStr("usa-a")}, nil, "usa_swimming_id"},
		{"parent status", parent, e.sw.ID, UpdateSwimmerInput{RegistrationStatus: cov1xStr("active")}, ErrForbidden, ""},
		{"parent medical", parent, e.sw.ID, UpdateSwimmerInput{MedicalFlags: &[]string{"x"}}, ErrMedicalFlagsRestricted, ""},
		{"notes too long", e.admin, e.sw.ID, UpdateSwimmerInput{Notes: cov1xStr(strings.Repeat("n", maxNotesLength+1))}, ErrFieldTooLong, ""},
		{"allergies too long", e.admin, e.sw.ID, UpdateSwimmerInput{Allergies: cov1xStr(strings.Repeat("a", maxSwimmerBioLength+1))}, ErrAllergiesTooLong, ""},
		{"consent by staff", e.admin, e.sw.ID, UpdateSwimmerInput{GuardianConsent: cov1xBool(true)}, ErrForbidden, ""},
		{"consent by parent+swimmer", dual, e.sw.ID, UpdateSwimmerInput{GuardianConsent: cov1xBool(true)}, ErrForbidden, ""},
		{"consent by unrelated parent+admin", adminParent, e.sw.ID, UpdateSwimmerInput{GuardianConsent: cov1xBool(true)}, ErrParentHouseholdScope, ""},
		{"blank names", e.admin, e.sw.ID, UpdateSwimmerInput{FirstName: cov1xStr(" "), LastName: cov1xStr("L")}, nil, "cannot be empty"},
	}
	for _, tc := range cases {
		_, _, err := svc.UpdateSwimmer(tc.p, cov1xOrg, tc.id, tc.in)
		if err == nil {
			t.Errorf("%s: expected an error", tc.name)
			continue
		}
		if tc.is != nil && !errors.Is(err, tc.is) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.is)
		}
		if tc.sub != "" && !strings.Contains(err.Error(), tc.sub) {
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.sub)
		}
	}
	// None of the rejected updates changed the stored swimmer.
	got, err := svc.GetSwimmer(e.admin, cov1xOrg, e.sw.ID, true)
	if err != nil || got.FirstName != "Ada" || got.Notes != "" || got.Allergies != "" || got.GuardianConsentAt != nil {
		t.Fatalf("swimmer mutated by rejected updates: %+v, %v", got, err)
	}
}

func TestCov1xUpdateSwimmerApplies(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	parent := cov1xParent("cov1x-par", e.hh.ID)

	// Setting a field to its own current value is not a conflict; clearing is allowed.
	if _, _, err := svc.UpdateSwimmer(e.admin, cov1xOrg, e.sw.ID, UpdateSwimmerInput{SwimmerExternalID: cov1xStr("EXT-1"), USASwimmingID: cov1xStr("USA-1")}); err != nil {
		t.Fatal(err)
	}
	got, ev, err := svc.UpdateSwimmer(e.admin, cov1xOrg, e.sw.ID, UpdateSwimmerInput{
		FirstName:                    cov1xStr(" Bea "),
		LastName:                     cov1xStr(" Bee "),
		PreferredName:                cov1xStr(" B "),
		DateOfBirth:                  cov1xStr("2001-02-03"),
		Gender:                       cov1xStr(" Non_Binary "),
		SwimmerExternalID:            cov1xStr("EXT-1"),
		USASwimmingID:                cov1xStr("usa-1"),
		RegistrationStatus:           cov1xStr(" Inactive "),
		MedicalFlags:                 &[]string{" Asthma", "asthma", "Latex "},
		Notes:                        cov1xStr(" nn "),
		EmergencyContactName:         cov1xStr(" EC "),
		EmergencyContactPhone:        cov1xStr(" 555 "),
		EmergencyContactRelationship: cov1xStr(" aunt "),
		Allergies:                    cov1xStr(" peanuts "),
		PortalAccessDisabled:         cov1xBool(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstName != "Bea" || got.LastName != "Bee" || got.PreferredName != "B" || got.DateOfBirth != "2001-02-03" ||
		got.Gender != "non_binary" || got.USASwimmingID != "usa-1" || got.RegistrationStatus != "inactive" ||
		strings.Join(got.MedicalFlags, ",") != "asthma,latex" || got.Notes != "nn" || got.EmergencyContactName != "EC" ||
		got.EmergencyContactPhone != "555" || got.EmergencyContactRelationship != "aunt" || got.Allergies != "peanuts" || !got.PortalAccessDisabled {
		t.Fatalf("update not applied: %+v", got)
	}
	if ev.Action != "swimmer.profile_updated" || ev.Before["first_name"] != "Ada" || ev.After["first_name"] != "Bea" {
		t.Fatalf("audit before/after wrong: %+v", ev)
	}
	if disabled, err := svc.SwimmerPortalDisabled(cov1xOrg, e.sw.ID); err != nil || !disabled {
		t.Fatalf("portal disabled = %v, %v", disabled, err)
	}
	// Clearing identifiers is allowed.
	if got, _, err := svc.UpdateSwimmer(e.admin, cov1xOrg, e.sw.ID, UpdateSwimmerInput{SwimmerExternalID: cov1xStr(""), USASwimmingID: cov1xStr("")}); err != nil || got.SwimmerExternalID != "" || got.USASwimmingID != "" {
		t.Fatalf("clear ids = %+v, %v", got, err)
	}
	// A swimmer may edit only their own bio.
	got, _, err = svc.UpdateSwimmer(cov1xSwimmerPrincipal("cov1x-sw", e.hh.ID), cov1xOrg, e.sw.ID, UpdateSwimmerInput{Notes: cov1xStr("  I love fly  ")})
	if err != nil || got.Notes != "I love fly" {
		t.Fatalf("swimmer bio = %+v, %v", got, err)
	}

	// Consent: a parent of the household records it; withdrawing unlinks the account.
	if err := svc.LinkSwimmerToUser(cov1xOrg, e.sw.ID, "kc-ada"); err != nil {
		t.Fatal(err)
	}
	got, ev, err = svc.UpdateSwimmer(parent, cov1xOrg, e.sw.ID, UpdateSwimmerInput{GuardianConsent: cov1xBool(true)})
	if err != nil {
		t.Fatal(err)
	}
	if got.GuardianConsentAt == nil || got.GuardianConsentBy != "cov1x-par" || got.GuardianConsentVersion != GuardianConsentVersion || ev.After["guardian_consent"] != true {
		t.Fatalf("consent not recorded: %+v", got)
	}
	got, _, err = svc.UpdateSwimmer(parent, cov1xOrg, e.sw.ID, UpdateSwimmerInput{GuardianConsent: cov1xBool(false), PortalAccessDisabled: cov1xBool(false)})
	if err != nil {
		t.Fatal(err)
	}
	if got.GuardianConsentAt != nil || got.GuardianConsentBy != "" || got.GuardianConsentVersion != "" || got.KeycloakUserID != "" || got.PortalAccessDisabled {
		t.Fatalf("withdrawal must clear consent and unlink: %+v", got)
	}
	if _, ok := svc.GetSwimmerByKeycloakUserID("kc-ada"); ok {
		t.Fatal("the unlinked account must no longer resolve")
	}
}

func TestCov1xDeleteSwimmerAndReads(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: "gold", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	bystander := e.newSwimmer(t, e.hh.ID, "Bystander")
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, bystander.ID, AssignSwimmerGroupInput{GroupCode: "mid"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DeleteSwimmer(cov1xCoach(), cov1xOrg, e.sw.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach delete err = %v", err)
	}
	if _, _, err := svc.DeleteSwimmer(e.admin, cov1xOrg, "nope"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if !svc.HasPrimaryGroup(cov1xOrg, e.sw.ID) || svc.HasPrimaryGroup(cov1xOrg, "nope") {
		t.Fatal("HasPrimaryGroup wrong")
	}
	if hid, ok := svc.CachedSwimmerHouseholdID(cov1xOrg, e.sw.ID); !ok || hid != e.hh.ID {
		t.Fatalf("CachedSwimmerHouseholdID = %q, %v", hid, ok)
	}
	if _, ok := svc.CachedSwimmerHouseholdID("other", e.sw.ID); ok {
		t.Fatal("cached lookup must be org scoped")
	}
	if hid, ok := svc.SwimmerHouseholdID(cov1xOrg, e.sw.ID); !ok || hid != e.hh.ID {
		t.Fatalf("SwimmerHouseholdID = %q, %v", hid, ok)
	}
	if _, ok := svc.SwimmerHouseholdID(cov1xOrg, "nope"); ok {
		t.Fatal("unknown swimmer must not resolve")
	}
	if info, ok := svc.SwimmerEligibility(cov1xOrg, e.sw.ID); !ok || info.Gender != "female" || info.DateOfBirth != "2000-01-02" || info.FirstName != "Ada" || info.LastName != "Alpha" {
		t.Fatalf("SwimmerEligibility = %+v, %v", info, ok)
	}
	if _, ok := svc.SwimmerEligibility(cov1xOrg, "nope"); ok {
		t.Fatal("unknown swimmer has no eligibility")
	}
	snap, err := svc.SnapshotRoster(cov1xOrg)
	if err != nil || len(snap.Households) != 1 || len(snap.Swimmers) != 2 || len(snap.GroupLinks) != 2 {
		t.Fatalf("roster = %+v, %v", snap, err)
	}
	if len(svc.SnapshotHouseholds(cov1xOrg)) != 1 || len(svc.SnapshotHouseholds("x")) != 0 ||
		len(svc.SnapshotSwimmers(cov1xOrg)) != 2 || len(svc.SnapshotSwimmers("x")) != 0 ||
		len(svc.SnapshotSwimmerGroupLinks(cov1xOrg)) != 2 || len(svc.SnapshotSwimmerGroupLinks("x")) != 0 {
		t.Fatal("snapshots must be org scoped")
	}

	deleted, deps, err := svc.DeleteSwimmer(e.admin, cov1xOrg, e.sw.ID)
	if err != nil || deps != nil || deleted.ID != e.sw.ID {
		t.Fatalf("delete = %+v, %v, %v", deleted, deps, err)
	}
	if _, err := svc.GetSwimmer(e.admin, cov1xOrg, e.sw.ID, true); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("deleted swimmer get err = %v", err)
	}
	if links := svc.SnapshotSwimmerGroupLinks(cov1xOrg); len(links) != 1 || links[0].SwimmerID != bystander.ID {
		t.Fatalf("deleting a swimmer must delete only their group links: %+v", links)
	}
	last := svc.auditEvents[len(svc.auditEvents)-1]
	if last.Action != "swimmer.deleted" || last.Before["first_name"] != "Ada" {
		t.Fatalf("delete audit = %+v", last)
	}
}

func TestCov1xGetSwimmerAndViewRedaction(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	if _, _, err := svc.UpdateSwimmer(e.admin, cov1xOrg, e.sw.ID, UpdateSwimmerInput{
		MedicalFlags: &[]string{"asthma"}, Allergies: cov1xStr("nuts"), EmergencyContactName: cov1xStr("Eve"),
	}); err != nil {
		t.Fatal(err)
	}
	parent := cov1xParent("cov1x-par", e.hh.ID)

	if _, err := svc.GetSwimmer(cov1xSwimmerPrincipal("s", e.hh.ID), cov1xOrg, e.sw.ID, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer get err = %v", err)
	}
	if _, err := svc.GetSwimmer(e.admin, cov1xOrg, "nope", true); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := svc.GetSwimmer(cov1xParent("stranger", ""), cov1xOrg, e.sw.ID, true); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("foreign parent err = %v", err)
	}
	v, err := svc.GetSwimmer(parent, cov1xOrg, e.sw.ID, true)
	if err != nil || v.MedicalFlagsRedacted || len(v.MedicalFlags) != 1 || v.Allergies != "nuts" {
		t.Fatalf("parent with medical = %+v, %v", v, err)
	}
	v, err = svc.GetSwimmer(parent, cov1xOrg, e.sw.ID, false)
	if err != nil || !v.MedicalFlagsRedacted || v.MedicalFlags != nil || v.Allergies != "nuts" {
		t.Fatalf("parent without medical = %+v, %v", v, err)
	}
	// A swimmer listing sees neither medical flags nor allergies / emergency details.
	list, err := svc.ListSwimmers(cov1xSwimmerPrincipal("s", e.hh.ID), cov1xOrg, ListSwimmersInput{IncludeMedical: true})
	if err != nil || len(list) != 1 || !list[0].MedicalFlagsRedacted || list[0].Allergies != "" || list[0].EmergencyContactName != "" {
		t.Fatalf("swimmer view = %+v, %v", list, err)
	}
	if svc.canViewMedicalLocked(nil, e.hh.ID) {
		t.Fatal("nil principal must not view medical data")
	}
	if svc.canViewMedicalLocked(cov1xParent("stranger", ""), e.hh.ID) {
		t.Fatal("foreign parent must not view medical data")
	}
	if err := svc.enforceParentHouseholdAccess(nil, e.hh.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil principal access err = %v", err)
	}
}

func TestCov1xAssignSwimmerToGroup(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	assign := func(code string, primary bool) SwimmerGroupLink {
		t.Helper()
		l, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: code, IsPrimary: primary})
		if err != nil {
			t.Fatalf("assign %s: %v", code, err)
		}
		return l
	}
	if _, err := svc.AssignSwimmerToGroup(cov1xParent("p", e.hh.ID), cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: "a"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent err = %v", err)
	}
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, "nope", AssignSwimmerGroupInput{GroupCode: "a"}); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("missing swimmer err = %v", err)
	}
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: "  "}); err == nil || !strings.Contains(err.Error(), "group_code is required") {
		t.Fatalf("blank code err = %v", err)
	}
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: strings.Repeat("g", maxGroupCodeLength+1)}); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("long code err = %v", err)
	}

	a := assign(" a ", true)
	if a.GroupCode != "A" || !a.IsPrimary {
		t.Fatalf("link A = %+v", a)
	}
	b := assign("b", false)
	if again := assign("B", false); again.ID != b.ID || again.IsPrimary {
		t.Fatalf("re-assign must be a no-op, got %+v", again)
	}
	if promoted := assign("b", true); promoted.ID != b.ID || !promoted.IsPrimary || promoted.UpdatedBy != e.admin.UserID {
		t.Fatalf("promotion = %+v", promoted)
	}
	if again := assign("b", true); again.ID != b.ID {
		t.Fatalf("already-primary re-assign = %+v", again)
	}
	c := assign("c", true)

	links := svc.SnapshotSwimmerGroupLinks(cov1xOrg)
	primaries := map[string]bool{}
	for _, l := range links {
		primaries[l.GroupCode] = l.IsPrimary
	}
	if len(links) != 3 || primaries["A"] || primaries["B"] || !primaries["C"] || c.GroupCode != "C" {
		t.Fatalf("exactly the newest link must be primary: %v", primaries)
	}
}

func TestCov1xListSwimmersFilters(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	hh2, err := svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	sw2 := e.newSwimmer(t, hh2.ID, "Bob")
	if _, _, err := svc.UpdateSwimmer(e.admin, cov1xOrg, sw2.ID, UpdateSwimmerInput{RegistrationStatus: cov1xStr("pending")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: "Gold"}); err != nil {
		t.Fatal(err)
	}
	// A swimmer in another org never leaks in.
	other := &appmiddleware.Principal{UserID: "o", OrgID: "org-cov1x-c", Roles: []string{"admin"}}
	oh, _ := svc.CreateHousehold(other, CreateHouseholdInput{OrgID: "org-cov1x-c", HouseholdName: "O"})
	if _, _, err := svc.CreateSwimmer(other, "org-cov1x-c", CreateSwimmerInput{HouseholdID: oh.ID, FirstName: "O", LastName: "O", DateOfBirth: "2000-01-01", Gender: "male"}); err != nil {
		t.Fatal(err)
	}

	ids := func(p *appmiddleware.Principal, in ListSwimmersInput) ([]string, error) {
		list, err := svc.ListSwimmers(p, cov1xOrg, in)
		out := make([]string, 0, len(list))
		for _, v := range list {
			out = append(out, v.ID)
		}
		return out, err
	}
	if got, err := ids(e.admin, ListSwimmersInput{}); err != nil || len(got) != 2 || got[0] > got[1] {
		t.Fatalf("all = %v, %v", got, err)
	}
	if got, _ := ids(e.admin, ListSwimmersInput{HouseholdID: hh2.ID}); len(got) != 1 || got[0] != sw2.ID {
		t.Fatalf("household filter = %v", got)
	}
	if got, _ := ids(e.admin, ListSwimmersInput{RegistrationStatus: " PENDING "}); len(got) != 1 || got[0] != sw2.ID {
		t.Fatalf("status filter = %v", got)
	}
	if got, _ := ids(e.admin, ListSwimmersInput{GroupCode: " gold "}); len(got) != 1 || got[0] != e.sw.ID {
		t.Fatalf("group filter = %v", got)
	}
	if _, err := ids(&appmiddleware.Principal{UserID: "x", OrgID: cov1xOrg, Roles: []string{"bogus"}}, ListSwimmersInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bogus role err = %v", err)
	}
	if _, err := ids(cov1xSwimmerPrincipal("s", ""), ListSwimmersInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer without household err = %v", err)
	}
	if _, err := ids(cov1xSwimmerPrincipal("s", e.hh.ID), ListSwimmersInput{HouseholdID: hh2.ID}); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("swimmer foreign household err = %v", err)
	}
	if got, err := ids(cov1xSwimmerPrincipal("s", e.hh.ID), ListSwimmersInput{HouseholdID: e.hh.ID}); err != nil || len(got) != 1 || got[0] != e.sw.ID {
		t.Fatalf("swimmer own household = %v, %v", got, err)
	}
	parent := cov1xParent("cov1x-par", e.hh.ID)
	if _, err := ids(parent, ListSwimmersInput{HouseholdID: hh2.ID}); !errors.Is(err, ErrParentHouseholdScope) {
		t.Fatalf("parent foreign household err = %v", err)
	}
	if got, err := ids(parent, ListSwimmersInput{}); err != nil || len(got) != 1 || got[0] != e.sw.ID {
		t.Fatalf("parent scoped list = %v, %v", got, err)
	}
}

func TestCov1xListSwimmerGroupLinks(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	hh2, _ := svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Beta"})
	sw2 := e.newSwimmer(t, hh2.ID, "Bob")
	for _, in := range []AssignSwimmerGroupInput{{GroupCode: "zeta", IsPrimary: true}, {GroupCode: "beta"}, {GroupCode: "alpha"}} {
		if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.AssignSwimmerToGroup(e.admin, cov1xOrg, sw2.ID, AssignSwimmerGroupInput{GroupCode: "mid"}); err != nil {
		t.Fatal(err)
	}
	// Orphan and foreign-org links must be ignored.
	svc.swimmerGroupLinks["cov1x-orphan"] = SwimmerGroupLink{ID: "cov1x-orphan", OrgID: cov1xOrg, SwimmerID: "ghost", GroupCode: "X"}
	svc.swimmerGroupLinks["cov1x-foreign"] = SwimmerGroupLink{ID: "cov1x-foreign", OrgID: "elsewhere", SwimmerID: e.sw.ID, GroupCode: "X"}
	svc.swimmerGroupLinks["cov1x-xorg"] = SwimmerGroupLink{ID: "cov1x-xorg", OrgID: cov1xOrg, SwimmerID: "cov1x-alien", GroupCode: "X"}
	svc.swimmers["cov1x-alien"] = Swimmer{ID: "cov1x-alien", OrgID: "elsewhere"}

	if _, err := svc.ListSwimmerGroupLinks(&appmiddleware.Principal{UserID: "x", OrgID: cov1xOrg, Roles: []string{"bogus"}}, cov1xOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bogus role err = %v", err)
	}
	if _, err := svc.ListSwimmerGroupLinks(cov1xSwimmerPrincipal("s", ""), cov1xOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer without household err = %v", err)
	}
	want := []string{"ZETA", "ALPHA", "BETA", "MID"}
	if sw2.ID < e.sw.ID {
		want = []string{"MID", "ZETA", "ALPHA", "BETA"}
	}
	for i := 0; i < 64; i++ { // random map order: exercise both primary-vs-secondary comparisons
		links, err := svc.ListSwimmerGroupLinks(e.admin, cov1xOrg)
		if err != nil || len(links) != 4 {
			t.Fatalf("admin links = %+v, %v", links, err)
		}
		for j, l := range links {
			if l.GroupCode != want[j] {
				t.Fatalf("order must be swimmer id, primary first, then group code: got %s at %d, want %v", l.GroupCode, j, want)
			}
		}
	}
	if links, err := svc.ListSwimmerGroupLinks(cov1xSwimmerPrincipal("s", hh2.ID), cov1xOrg); err != nil || len(links) != 1 || links[0].SwimmerID != sw2.ID {
		t.Fatalf("swimmer links = %+v, %v", links, err)
	}
	if links, err := svc.ListSwimmerGroupLinks(cov1xParent("cov1x-par", e.hh.ID), cov1xOrg); err != nil || len(links) != 3 {
		t.Fatalf("parent links = %+v, %v", links, err)
	}
}

func TestCov1xAssociateParentToHousehold(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	if err := svc.AssociateParentToHousehold(cov1xParent("p", e.hh.ID), cov1xOrg, "p", e.hh.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent err = %v", err)
	}
	if err := svc.AssociateParentToHousehold(e.admin, "other-org", "p", e.hh.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("org mismatch err = %v", err)
	}
	if err := svc.AssociateParentToHousehold(e.admin, cov1xOrg, " ", e.hh.ID); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("blank parent err = %v", err)
	}
	if err := svc.AssociateParentToHousehold(e.admin, cov1xOrg, "p", "missing"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("missing hh err = %v", err)
	}
	var linked []string
	svc.SetParentHouseholdLinkedHook(func(u, h string) { linked = append(linked, u+"@"+h) })
	if err := svc.AssociateParentToHousehold(cov1xCoach(), cov1xOrg, " p ", " "+e.hh.ID+" "); err != nil {
		t.Fatal(err)
	}
	if len(linked) != 1 || linked[0] != "p@"+e.hh.ID {
		t.Fatalf("hook = %v", linked)
	}
	if _, err := svc.GetHousehold(cov1xParent("p", ""), cov1xOrg, e.hh.ID); err != nil {
		t.Fatalf("associated parent must reach the household: %v", err)
	}
}

func TestCov1xKeycloakLinks(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	hh2, _ := svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Beta"})

	if _, ok := svc.GetHouseholdByKeycloakUserID(""); ok {
		t.Fatal("blank user resolves nothing")
	}
	if err := svc.LinkHouseholdToUser(cov1xOrg, e.hh.ID, "kc-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkHouseholdToUser(cov1xOrg, e.hh.ID, "kc-1"); err != nil {
		t.Fatalf("re-linking the same household is fine: %v", err)
	}
	if err := svc.LinkHouseholdToUser(cov1xOrg, hh2.ID, "kc-1"); err == nil || !strings.Contains(err.Error(), "already linked to another household") {
		t.Fatalf("dup link err = %v", err)
	}
	if err := svc.LinkHouseholdToUser(cov1xOrg, "missing", "kc-2"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if err := svc.LinkHouseholdToUser("other-org", e.hh.ID, "kc-2"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("cross-org err = %v", err)
	}
	if h, ok := svc.GetHouseholdByKeycloakUserID("kc-1"); !ok || h.ID != e.hh.ID {
		t.Fatalf("lookup = %+v, %v", h, ok)
	}
	if err := svc.UnlinkHouseholdFromUser(cov1xOrg, e.hh.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.GetHouseholdByKeycloakUserID("kc-1"); ok {
		t.Fatal("unlinked user must not resolve")
	}

	sw2 := e.newSwimmer(t, e.hh.ID, "Bob")
	if _, ok := svc.GetSwimmerByKeycloakUserID(""); ok {
		t.Fatal("blank user resolves nothing")
	}
	if err := svc.LinkSwimmerToUser(cov1xOrg, e.sw.ID, "kc-s"); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkSwimmerToUser(cov1xOrg, sw2.ID, "kc-s"); err == nil || !strings.Contains(err.Error(), "already linked to another swimmer") {
		t.Fatalf("dup swimmer link err = %v", err)
	}
	if err := svc.LinkSwimmerToUser(cov1xOrg, "missing", "kc-x"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if err := svc.LinkSwimmerToUser("other-org", e.sw.ID, "kc-x"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("cross-org err = %v", err)
	}
	if sw, ok := svc.GetSwimmerByKeycloakUserID("kc-s"); !ok || sw.ID != e.sw.ID {
		t.Fatalf("swimmer lookup = %+v, %v", sw, ok)
	}
	if _, ok := svc.GetSwimmerByKeycloakUserID("kc-unknown"); ok {
		t.Fatal("unknown user must not resolve")
	}
	if err := svc.UnlinkSwimmerFromUser(cov1xOrg, e.sw.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.GetSwimmerByKeycloakUserID("kc-s"); ok {
		t.Fatal("unlinked swimmer must not resolve")
	}
}

func TestCov1xGuardianConsentHelpers(t *testing.T) {
	t.Parallel()
	if !swimmerNeedsGuardianConsent(Swimmer{DateOfBirth: "garbage"}) {
		t.Fatal("an unparseable DOB counts as under 13")
	}
	now := time.Now().UTC()
	if swimmerNeedsGuardianConsent(Swimmer{DateOfBirth: "garbage", GuardianConsentAt: &now}) {
		t.Fatal("recorded consent satisfies the requirement")
	}
	e := cov1xNewEnv(t)
	if disabled, err := e.svc.SwimmerPortalDisabled(cov1xOrg, "unknown"); err != nil || disabled {
		t.Fatalf("unknown swimmer = %v, %v", disabled, err)
	}
	kid := time.Now().UTC().AddDate(-9, 0, 0).Format("2006-01-02")
	sw, _, err := e.svc.CreateSwimmer(e.admin, cov1xOrg, CreateSwimmerInput{HouseholdID: e.hh.ID, FirstName: "Kid", LastName: "K", DateOfBirth: kid, Gender: "male"})
	if err != nil {
		t.Fatal(err)
	}
	if disabled, _ := e.svc.SwimmerPortalDisabled(cov1xOrg, sw.ID); !disabled {
		t.Fatal("under-13 without consent must fail closed")
	}
}

func TestCov1xNormalizeFlags(t *testing.T) {
	t.Parallel()
	got := normalizeFlags([]string{" Asthma ", "", "asthma", "ASTHMA", "Latex", "  "})
	if strings.Join(got, ",") != "asthma,latex" {
		t.Fatalf("normalizeFlags = %v", got)
	}
	if got := normalizeFlags(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil flags must normalise to an empty non-nil slice, got %#v", got)
	}
}

func TestCov1xHouseholdErasureAndAnonymize(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	svc := e.svc
	hh, err := svc.UpdateHousehold(e.admin, cov1xOrg, e.hh.ID, UpdateHouseholdInput{PrimaryEmail: cov1xStr("Fam@Example.com"), PrimaryPhoneE164: cov1xStr("+1555"), Notes: cov1xStr("private")})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkHouseholdToUser(cov1xOrg, hh.ID, "kc-hh"); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkSwimmerToUser(cov1xOrg, e.sw.ID, "kc-sw"); err != nil {
		t.Fatal(err)
	}
	contact, err := svc.AddContact(e.admin, cov1xOrg, hh.ID, AddContactInput{Relationship: "dad", FirstName: "Dan", LastName: "Doe", Email: "dan@example.com", PhoneE164: "+1556", Notes: "pickup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddContact(e.admin, cov1xOrg, hh.ID, AddContactInput{Relationship: "mom", FirstName: "May", LastName: "Doe"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UpdateSwimmer(e.admin, cov1xOrg, e.sw.ID, UpdateSwimmerInput{
		SwimmerExternalID: cov1xStr("EXT"), USASwimmingID: cov1xStr("USA"), MedicalFlags: &[]string{"asthma"}, Notes: cov1xStr("secret"),
		Allergies: cov1xStr("nuts"), EmergencyContactName: cov1xStr("E"), EmergencyContactPhone: cov1xStr("1"), EmergencyContactRelationship: cov1xStr("r"),
	}); err != nil {
		t.Fatal(err)
	}
	// An unrelated household with its own swimmer must be left alone.
	hh2, _ := svc.CreateHousehold(e.admin, CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Keep"})
	keeper := e.newSwimmer(t, hh2.ID, "Keeper")
	// An audit row for the erased swimmer but from another org is retained as is.
	svc.auditEvents = append(svc.auditEvents, SwimmerAuditEvent{OrgID: "elsewhere", SwimmerID: e.sw.ID, After: map[string]any{"first_name": "Stay"}})

	if _, err := svc.GetHouseholdErasureContext(cov1xOrg, "missing"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("erasure ctx missing err = %v", err)
	}
	ctx, err := svc.GetHouseholdErasureContext(cov1xOrg, " "+hh.ID+" ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ctx.KeycloakUserIDs, ",") != "kc-hh,kc-sw" {
		t.Fatalf("keycloak ids = %v", ctx.KeycloakUserIDs)
	}
	if len(ctx.ContactEmails) != 2 || ctx.ContactEmails[0] != "fam@example.com" || ctx.ContactEmails[1] != contact.Email {
		t.Fatalf("contact emails = %v", ctx.ContactEmails)
	}

	if err := svc.AnonymizeHousehold(cov1xOrg, "missing"); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("anonymize missing err = %v", err)
	}
	if err := svc.AnonymizeHousehold(cov1xOrg, hh.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetHousehold(e.admin, cov1xOrg, hh.ID)
	if got.HouseholdName != "Deleted Household" || got.PrimaryEmail != "" || got.PrimaryPhoneE164 != "" || got.Notes != "" ||
		got.KeycloakUserID != "" || got.UpdatedBy != "compliance_erasure" {
		t.Fatalf("household not scrubbed: %+v", got)
	}
	for _, c := range svc.SnapshotContacts(cov1xOrg) {
		if c.FirstName != "Deleted" || c.LastName != "Contact" || c.Email != "" || c.PhoneE164 != "" || c.Notes != "" {
			t.Fatalf("contact not scrubbed: %+v", c)
		}
	}
	sw, _ := svc.GetSwimmer(e.admin, cov1xOrg, e.sw.ID, true)
	if sw.FirstName != "Deleted" || sw.LastName != "Swimmer" || sw.SwimmerExternalID != "" || sw.USASwimmingID != "" || sw.MedicalFlags != nil ||
		sw.Notes != "" || sw.Allergies != "" || sw.EmergencyContactName != "" || sw.KeycloakUserID != "" {
		t.Fatalf("swimmer not scrubbed: %+v", sw)
	}
	if k, _ := svc.GetSwimmer(e.admin, cov1xOrg, keeper.ID, true); k.FirstName != "Keeper" {
		t.Fatalf("unrelated swimmer touched: %+v", k)
	}
	for _, ev := range svc.auditEvents {
		switch {
		case ev.OrgID == "elsewhere":
			if ev.After["first_name"] != "Stay" {
				t.Fatalf("foreign-org audit must be kept: %+v", ev)
			}
		case ev.SwimmerID == e.sw.ID:
			for _, m := range []map[string]any{ev.Before, ev.After} {
				if m == nil {
					continue
				}
				if m["first_name"] != "" || m["date_of_birth"] != "" || m["medical_flags"] != nil || m["gender"] == "" {
					t.Fatalf("audit snapshot not scrubbed (non-PII must remain): %+v", m)
				}
			}
		case ev.SwimmerID == keeper.ID:
			if ev.After["first_name"] != "Keeper" {
				t.Fatalf("unrelated audit scrubbed: %+v", ev)
			}
		}
	}
	if scrubAuditPII(nil) != nil {
		t.Fatal("nil map stays nil")
	}
}

func TestCov1xLinkRequestsInMemory(t *testing.T) {
	t.Parallel()
	svc := &Service{} // zero value: the lazily created map branch
	if _, err := svc.CreateLinkRequest(cov1xOrg, "u1", strings.Repeat("n", maxLinkRequestNameLen+1), ""); !errors.Is(err, ErrLinkRequestInvalid) {
		t.Fatalf("long name err = %v", err)
	}
	if _, err := svc.CreateLinkRequest(cov1xOrg, "u1", "", strings.Repeat("n", maxLinkRequestNoteLen+1)); !errors.Is(err, ErrLinkRequestInvalid) {
		t.Fatalf("long note err = %v", err)
	}
	r1, err := svc.CreateLinkRequest(cov1xOrg, "u1", " Ada ", " hi ")
	if err != nil {
		t.Fatal(err)
	}
	if r1.RequestedSwimmerName != "Ada" || r1.Note != "hi" || r1.Status != "pending" || !strings.HasPrefix(r1.ID, "slr-") {
		t.Fatalf("request = %+v", r1)
	}
	if again, err := svc.CreateLinkRequest(cov1xOrg, "u1", "different", ""); err != nil || again.ID != r1.ID {
		t.Fatalf("a user has one pending request: %+v, %v", again, err)
	}
	r2, err := svc.CreateLinkRequest(cov1xOrg, "u2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateLinkRequest("org-cov1x-other", "u3", "", ""); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.ListPendingLinkRequests(cov1xOrg)
	if err != nil || len(pending) != 2 || pending[0].ID != r1.ID || pending[1].ID != r2.ID {
		t.Fatalf("pending = %+v, %v", pending, err)
	}

	if err := svc.ResolveLinkRequest(cov1xOrg, "missing", "adm", "sw"); !errors.Is(err, ErrSwimmerLinkRequestNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if err := svc.ResolveLinkRequest("org-cov1x-other", r1.ID, "adm", "sw"); !errors.Is(err, ErrSwimmerLinkRequestNotFound) {
		t.Fatalf("cross-org err = %v", err)
	}
	if err := svc.ResolveLinkRequest(cov1xOrg, r1.ID, "adm", "sw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResolveLinkRequest(cov1xOrg, r1.ID, "adm", "sw"); !errors.Is(err, ErrSwimmerLinkRequestNotFound) {
		t.Fatalf("second resolve err = %v", err)
	}
	if pending, _ := svc.ListPendingLinkRequests(cov1xOrg); len(pending) != 1 || pending[0].ID != r2.ID {
		t.Fatalf("resolved request must leave the queue: %+v", pending)
	}
	// A resolved request no longer blocks a fresh one from the same user.
	if fresh, err := svc.CreateLinkRequest(cov1xOrg, "u1", "", ""); err != nil || fresh.ID == r1.ID {
		t.Fatalf("fresh request = %+v, %v", fresh, err)
	}
}

func TestCov1xRuntimeSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv(t)
	if _, err := e.svc.AddContact(e.admin, cov1xOrg, e.hh.ID, AddContactInput{Relationship: "mom", FirstName: "M", LastName: "L"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AssignSwimmerToGroup(e.admin, cov1xOrg, e.sw.ID, AssignSwimmerGroupInput{GroupCode: "g", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AssociateParentToHousehold(e.admin, cov1xOrg, "par", e.hh.ID); err != nil {
		t.Fatal(err)
	}
	data, err := e.svc.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	fresh := NewService()
	if err := fresh.LoadRuntimeSnapshot(nil); err != nil {
		t.Fatalf("empty snapshot is a no-op: %v", err)
	}
	if err := fresh.LoadRuntimeSnapshot([]byte("{not json")); err == nil {
		t.Fatal("corrupt snapshot must error")
	}
	if len(fresh.SnapshotHouseholds(cov1xOrg)) != 0 {
		t.Fatal("a failed load must not partially apply")
	}
	if err := fresh.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if len(fresh.SnapshotHouseholds(cov1xOrg)) != 1 || len(fresh.SnapshotContacts(cov1xOrg)) != 1 ||
		len(fresh.SnapshotSwimmers(cov1xOrg)) != 1 || len(fresh.SnapshotSwimmerGroupLinks(cov1xOrg)) != 1 {
		t.Fatal("snapshot did not restore all collections")
	}
	if len(fresh.auditEvents) != len(e.svc.auditEvents) || len(fresh.auditEvents) == 0 {
		t.Fatalf("audit events = %d, want %d", len(fresh.auditEvents), len(e.svc.auditEvents))
	}
	if _, err := fresh.GetHousehold(cov1xParent("par", ""), cov1xOrg, e.hh.ID); err != nil {
		t.Fatalf("parent index must be restored: %v", err)
	}
}
