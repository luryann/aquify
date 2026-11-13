package roster

import (
	"reflect"
	"testing"

	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov1xOrg = "org-cov1x"

func cov1xAdmin() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "admin-1", OrgID: cov1xOrg, Roles: []string{"admin"}}
}

type cov1xFixture struct {
	dir        *MembersDirectory
	hhA, hhB   string
	swA, swB   string
	swC        string
	groupID    string
	groupLower string
}

func cov1xNewFixture(t *testing.T) cov1xFixture {
	t.Helper()
	ms := members.NewService()
	gs := groups.NewService()
	admin := cov1xAdmin()

	grp, _, err := gs.CreateGroup(admin, cov1xOrg, groups.CreateGroupInput{Code: "gold", Name: "Gold"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	hhA, err := ms.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Zeta Family", PrimaryEmail: "z@example.test", KeycloakUserID: "kc-parent"})
	if err != nil {
		t.Fatalf("household A: %v", err)
	}
	hhB, err := ms.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov1xOrg, HouseholdName: "Alpha Family", PrimaryEmail: "a@example.test"})
	if err != nil {
		t.Fatalf("household B: %v", err)
	}
	mk := func(hh, first, last, pref string) string {
		sw, _, err := ms.CreateSwimmer(admin, cov1xOrg, members.CreateSwimmerInput{
			HouseholdID: hh, FirstName: first, LastName: last, PreferredName: pref,
			DateOfBirth: "2012-03-04", Gender: "female",
		})
		if err != nil {
			t.Fatalf("create swimmer %s: %v", first, err)
		}
		return sw.ID
	}
	swA := mk(hhA.ID, "Zoe", "Zeta", "")
	swB := mk(hhB.ID, "Amy", "Alpha", "Ames")
	swC := mk(hhB.ID, "Bea", "Alpha", "")
	if _, err := ms.AssignSwimmerToGroup(admin, cov1xOrg, swA, members.AssignSwimmerGroupInput{GroupCode: "gold", IsPrimary: true}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if _, err := ms.AssignSwimmerToGroup(admin, cov1xOrg, swA, members.AssignSwimmerGroupInput{GroupCode: "silver"}); err != nil {
		t.Fatalf("assign2: %v", err)
	}
	return cov1xFixture{
		dir: NewMembersDirectory(ms, gs), hhA: hhA.ID, hhB: hhB.ID,
		swA: swA, swB: swB, swC: swC, groupID: grp.ID, groupLower: grp.Code,
	}
}

func TestCov1xMembersDirectorySwimmers(t *testing.T) {
	f := cov1xNewFixture(t)
	got, err := f.dir.Swimmers(cov1xOrg)
	if err != nil {
		t.Fatalf("Swimmers: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 swimmers, got %d", len(got))
	}
	// Sorted by last name then first name: Alpha Amy, Alpha Bea, Zeta Zoe.
	if got[0].ID != f.swB || got[1].ID != f.swC || got[2].ID != f.swA {
		t.Fatalf("unexpected order: %+v", got)
	}
	if got[0].DisplayName != "Ames Alpha" {
		t.Errorf("preferred name not used: %q", got[0].DisplayName)
	}
	if len(got[0].GroupCodes) != 0 || got[0].PrimaryGroup != "" {
		t.Errorf("ungrouped swimmer should have no groups: %+v", got[0])
	}
	if !reflect.DeepEqual(got[2].GroupCodes, []string{"GOLD", "SILVER"}) || got[2].PrimaryGroup != "GOLD" {
		t.Errorf("group data wrong: %+v", got[2])
	}
	if !got[2].InGroup("gold") || got[2].InGroup("bronze") {
		t.Error("InGroup canonical comparison failed")
	}
	if !got[2].InAnyGroup([]string{"bronze", " silver "}) || got[2].InAnyGroup([]string{"bronze"}) || got[2].InAnyGroup(nil) {
		t.Error("InAnyGroup wrong")
	}

	other, err := f.dir.Swimmers("org-other")
	if err != nil || len(other) != 0 {
		t.Errorf("other org should be empty: %v %v", other, err)
	}

	var nilDir *MembersDirectory
	if out, err := nilDir.Swimmers(cov1xOrg); err != nil || out == nil || len(out) != 0 {
		t.Errorf("nil dir: %v %v", out, err)
	}
	if out, err := NewMembersDirectory(nil, nil).Swimmers(cov1xOrg); err != nil || len(out) != 0 {
		t.Errorf("nil members: %v %v", out, err)
	}
}

func TestCov1xHouseholdIDsForPrincipal(t *testing.T) {
	f := cov1xNewFixture(t)

	if ids, err := f.dir.HouseholdIDsForPrincipal(nil, cov1xOrg); err != nil || len(ids) != 0 {
		t.Errorf("nil principal: %v %v", ids, err)
	}
	staff := &appmiddleware.Principal{UserID: "c", OrgID: cov1xOrg, Roles: []string{"coach"}}
	if ids, err := f.dir.HouseholdIDsForPrincipal(staff, cov1xOrg); err != nil || len(ids) != 0 {
		t.Errorf("staff: %v %v", ids, err)
	}

	// Parent resolved via Keycloak link; mixed admin role must not widen the set.
	parent := &appmiddleware.Principal{UserID: "kc-parent", OrgID: cov1xOrg, Roles: []string{"PARENT", "admin"}, HouseholdID: " " + f.hhA + " "}
	ids, err := f.dir.HouseholdIDsForPrincipal(parent, cov1xOrg)
	if err != nil || !reflect.DeepEqual(ids, []string{f.hhA}) {
		t.Errorf("parent ids = %v err=%v", ids, err)
	}

	// Swimmer-only principal without a household is forbidden by members.
	sw := &appmiddleware.Principal{UserID: "s1", OrgID: cov1xOrg, Roles: []string{"swimmer"}}
	if _, err := f.dir.HouseholdIDsForPrincipal(sw, cov1xOrg); err == nil {
		t.Error("expected error for swimmer without household")
	}
	sw.HouseholdID = f.hhB
	if ids, err := f.dir.HouseholdIDsForPrincipal(sw, cov1xOrg); err != nil || !reflect.DeepEqual(ids, []string{f.hhB}) {
		t.Errorf("swimmer ids = %v err=%v", ids, err)
	}

	// Without a members service the principal's own household is returned (deduped, trimmed).
	bare := NewMembersDirectory(nil, nil)
	p := &appmiddleware.Principal{UserID: "p", Roles: []string{"parent"}, HouseholdID: " hh-1 "}
	if ids, _ := bare.HouseholdIDsForPrincipal(p, cov1xOrg); !reflect.DeepEqual(ids, []string{"hh-1"}) {
		t.Errorf("bare ids = %v", ids)
	}
	p.HouseholdID = " "
	if ids, _ := bare.HouseholdIDsForPrincipal(p, cov1xOrg); len(ids) != 0 {
		t.Errorf("blank household should yield none: %v", ids)
	}
	var nilDir *MembersDirectory
	p.HouseholdID = "hh-2"
	if ids, _ := nilDir.HouseholdIDsForPrincipal(p, cov1xOrg); !reflect.DeepEqual(ids, []string{"hh-2"}) {
		t.Errorf("nil dir ids = %v", ids)
	}
}

func TestCov1xResolveGroupCode(t *testing.T) {
	f := cov1xNewFixture(t)
	if got := f.dir.ResolveGroupCode(cov1xOrg, "  "); got != "" {
		t.Errorf("blank = %q", got)
	}
	if got := f.dir.ResolveGroupCode(cov1xOrg, f.groupID); got != "GOLD" {
		t.Errorf("by id = %q", got)
	}
	if got := f.dir.ResolveGroupCode(cov1xOrg, " bronze "); got != "BRONZE" {
		t.Errorf("unknown ref = %q", got)
	}
	var nilDir *MembersDirectory
	if got := nilDir.ResolveGroupCode(cov1xOrg, "x"); got != "X" {
		t.Errorf("nil dir = %q", got)
	}
	if got := NewMembersDirectory(nil, nil).ResolveGroupCode(cov1xOrg, "y"); got != "Y" {
		t.Errorf("nil groups = %q", got)
	}
}

func TestCov1xMembersHouseholdNames(t *testing.T) {
	f := cov1xNewFixture(t)
	got, err := f.dir.HouseholdNames(cov1xOrg)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{f.hhA: "Zeta Family", f.hhB: "Alpha Family"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	var nilDir *MembersDirectory
	if out, err := nilDir.HouseholdNames(cov1xOrg); err != nil || len(out) != 0 {
		t.Errorf("nil dir: %v %v", out, err)
	}
}

func TestCov1xStaticDirectoryAndHelpers(t *testing.T) {
	d := &StaticDirectory{
		SwimmerList: []Swimmer{
			{ID: "2", OrgID: "o", HouseholdID: "h1", FirstName: "b", LastName: "Smith"},
			{ID: "1", OrgID: "o", HouseholdID: "h1", FirstName: "a", LastName: "smith"},
			{ID: "3", OrgID: "o", HouseholdID: "h2", FirstName: "a", LastName: "Jones"},
			{ID: "4", OrgID: "x", HouseholdID: "h1", FirstName: "a", LastName: "Z"},
			{ID: "0", OrgID: "o", HouseholdID: "h1", FirstName: "a", LastName: "Smith"},
		},
		ParentHouseholds: map[string][]string{"u": {"h2", " ", "h2", "h3"}},
		GroupIDs:         map[string]string{"gid": "gold"},
		Households:       map[string]string{"h1": "One"},
	}
	list, _ := d.Swimmers("o")
	if len(list) != 4 {
		t.Fatalf("org filter: %d", len(list))
	}
	names, _ := d.HouseholdNames("o")
	names["h1"] = "mutated"
	if again, _ := d.HouseholdNames("o"); again["h1"] != "One" {
		t.Error("HouseholdNames must return a copy")
	}

	in := SwimmersInHousehold(list, "h1")
	var order []string
	for _, s := range in {
		order = append(order, s.ID)
	}
	// smith/a ids 0 and 1 (ID tie-break), then smith/b.
	if !reflect.DeepEqual(order, []string{"0", "1", "2"}) {
		t.Errorf("order = %v", order)
	}
	if got := SwimmersInHousehold(list, "none"); got == nil || len(got) != 0 {
		t.Errorf("empty household = %v", got)
	}
	if sw, ok := SwimmerByID(list, " 3 "); !ok || sw.LastName != "Jones" {
		t.Errorf("SwimmerByID = %v %v", sw, ok)
	}
	if _, ok := SwimmerByID(list, "nope"); ok {
		t.Error("expected miss")
	}

	if ids, _ := d.HouseholdIDsForPrincipal(nil, "o"); ids != nil {
		t.Errorf("nil principal = %v", ids)
	}
	p := &appmiddleware.Principal{UserID: "u", Roles: []string{"parent"}, HouseholdID: "h1"}
	if ids, _ := d.HouseholdIDsForPrincipal(p, "o"); !reflect.DeepEqual(ids, []string{"h1", "h2", "h3"}) {
		t.Errorf("parent = %v", ids)
	}
	s := &appmiddleware.Principal{UserID: "u", Roles: []string{"swimmer"}, HouseholdID: "h1"}
	if ids, _ := d.HouseholdIDsForPrincipal(s, "o"); !reflect.DeepEqual(ids, []string{"h1"}) {
		t.Errorf("swimmer = %v", ids)
	}
	c := &appmiddleware.Principal{UserID: "u", Roles: []string{"coach"}, HouseholdID: "h1"}
	if ids, _ := d.HouseholdIDsForPrincipal(c, "o"); len(ids) != 0 {
		t.Errorf("coach = %v", ids)
	}

	if d.ResolveGroupCode("o", " gid ") != "GOLD" || d.ResolveGroupCode("o", " silver") != "SILVER" {
		t.Error("ResolveGroupCode")
	}

	if !IsStaff(&appmiddleware.Principal{Roles: []string{" Coach "}}) || IsStaff(p) || IsStaff(nil) {
		t.Error("IsStaff")
	}
	if HasAnyRole(&appmiddleware.Principal{}, "admin") {
		t.Error("no roles")
	}

	if DisplayName(" Pat ", "Patricia", " Lee ") != "Pat Lee" ||
		DisplayName("", " Patricia ", "Lee") != "Patricia Lee" ||
		DisplayName("", "", "") != "" {
		t.Error("DisplayName")
	}

	// SortByName: case-insensitive on last, then first.
	l := []Swimmer{{ID: "a", FirstName: "B", LastName: "x"}, {ID: "b", FirstName: "a", LastName: "X"}, {ID: "c", FirstName: "z", LastName: "A"}}
	SortByName(l)
	if l[0].ID != "c" || l[1].ID != "b" || l[2].ID != "a" {
		t.Errorf("sort = %+v", l)
	}
}
