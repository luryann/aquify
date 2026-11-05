package provisioning

import (
	"context"
	"errors"
	"testing"

	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/keycloak"
)

type flakyKC struct {
	keycloak.Client
	failCreate int
}

func (f *flakyKC) CreateUser(ctx context.Context, email, first, last, orgID string) (string, error) {
	if f.failCreate > 0 {
		f.failCreate--
		return "", errors.New("keycloak blip")
	}
	return f.Client.CreateUser(ctx, email, first, last, orgID)
}

type memSettings struct {
	calls  map[string]orgsettings.Updates
	owners map[string]string // public slug -> org id
}

func (m *memSettings) FindBySlug(slug string) (*orgsettings.OrgSettings, error) {
	if id, ok := m.owners[slug]; ok {
		return &orgsettings.OrgSettings{OrgID: id, PublicSlug: slug}, nil
	}
	return nil, nil
}

func (m *memSettings) Update(orgID string, u orgsettings.Updates, _ string) (*orgsettings.OrgSettings, error) {
	m.calls[orgID] = u
	return &orgsettings.OrgSettings{OrgID: orgID}, nil
}

func newProvisioner(kc keycloak.Client) (*Provisioner, *orgs.Service, *memSettings) {
	reg := orgs.NewService()
	st := &memSettings{calls: map[string]orgsettings.Updates{}, owners: map[string]string{}}
	return &Provisioner{Orgs: reg, Keycloak: kc, Settings: st}, reg, st
}

var in = Input{Name: "Aqua Club", Slug: "aqua-club", AdminEmail: "Boss@Example.com", AdminFirstName: "B", AdminLastName: "Oss", TeamCode: "AQ"}

func TestProvisionCreatesActiveOrgWithAdmin(t *testing.T) {
	kc := keycloak.NewStubClient()
	p, reg, st := newProvisioner(kc)
	res, err := p.Provision(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Org.Status != orgs.StatusActive || res.AdminUserID == "" || res.Resumed {
		t.Fatalf("result: %+v", res)
	}
	if got, _ := reg.BySlug(context.Background(), "aqua-club"); got == nil || got.Status != orgs.StatusActive {
		t.Fatalf("registry: %+v", got)
	}
	u, err := kc.GetUser(context.Background(), res.AdminUserID)
	if err != nil || u.Attributes["org_id"][0] != res.Org.ID {
		t.Fatalf("admin must be bound to the new org: %+v %v", u, err)
	}
	roles, _ := kc.GetUserRealmRoles(context.Background(), res.AdminUserID)
	hasAdmin := false
	for _, r := range roles {
		hasAdmin = hasAdmin || r.Name == "admin"
	}
	if !hasAdmin {
		t.Fatalf("admin role missing: %+v", roles)
	}
	if up := st.calls[res.Org.ID]; up.TeamName == nil || *up.TeamName != "Aqua Club" || up.PublicSlug == nil || *up.PublicSlug != "aqua-club" || up.TeamCode == nil {
		t.Fatalf("settings: %+v", up)
	}
}

func TestProvisionResumesAfterPartialFailure(t *testing.T) {
	kc := &flakyKC{Client: keycloak.NewStubClient(), failCreate: 1}
	p, reg, _ := newProvisioner(kc)
	res, err := p.Provision(context.Background(), in)
	if err == nil || res == nil || res.Org == nil {
		t.Fatalf("first run must fail but return the org so far: %+v %v", res, err)
	}
	if got, _ := reg.BySlug(context.Background(), "aqua-club"); got.Status != orgs.StatusProvisioning {
		t.Fatalf("org must stay provisioning after failure: %s", got.Status)
	}
	res2, err := p.Provision(context.Background(), in)
	if err != nil || !res2.Resumed || res2.Org.Status != orgs.StatusActive || res2.Org.ID != res.Org.ID {
		t.Fatalf("resume: %+v %v", res2, err)
	}
	list, _ := reg.List(context.Background(), "")
	if len(list) != 1 {
		t.Fatalf("resume must not create a second org, got %d", len(list))
	}
}

func TestProvisionRefusesExistingSlugAndForeignAdminEmail(t *testing.T) {
	kc := keycloak.NewStubClient()
	p, _, _ := newProvisioner(kc)
	if _, err := p.Provision(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Provision(context.Background(), in); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("repeat of a finished provision: %v", err)
	}
	other := in
	other.Slug = "other-club"
	if _, err := p.Provision(context.Background(), other); !errors.Is(err, ErrAdminEmailInUse) {
		t.Fatalf("admin email of another org must be refused: %v", err)
	}
}

func TestProvisionValidatesInput(t *testing.T) {
	p, _, _ := newProvisioner(keycloak.NewStubClient())
	for name, mod := range map[string]func(*Input){
		"no name":    func(i *Input) { i.Name = "" },
		"bad slug":   func(i *Input) { i.Slug = "A B" },
		"bad email":  func(i *Input) { i.AdminEmail = "nope" },
		"empty mail": func(i *Input) { i.AdminEmail = "" },
	} {
		bad := in
		mod(&bad)
		if _, err := p.Provision(context.Background(), bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestProvisionRefusesASlugAnotherClubUsesOnItsPublicSite(t *testing.T) {
	p, reg, st := newProvisioner(keycloak.NewStubClient())
	st.owners["aqua-club"] = "00000000-0000-0000-0000-00000000c0de" // another club's public_slug
	if _, err := p.Provision(context.Background(), in); !errors.Is(err, orgs.ErrSlugTaken) {
		t.Fatalf("a slug held in another club's settings must be refused: %v", err)
	}
	if list, _ := reg.List(context.Background(), ""); len(list) != 0 {
		t.Fatalf("nothing may be created: %d orgs", len(list))
	}
}
