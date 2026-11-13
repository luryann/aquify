package provisioning

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/keycloak"
)

type cov1xKC struct {
	keycloak.Client
	listErr, roleErr, verifyErr, resetErr error
	onAssign                              func()
	creates                               int
	verifySent, resetSent                 int
}

func (k *cov1xKC) ListUsers(ctx context.Context, orgID, s string, l, o int) ([]keycloak.User, error) {
	if k.listErr != nil {
		return nil, k.listErr
	}
	return k.Client.ListUsers(ctx, orgID, s, l, o)
}

func (k *cov1xKC) CreateUser(ctx context.Context, e, f, l, orgID string) (string, error) {
	k.creates++
	return k.Client.CreateUser(ctx, e, f, l, orgID)
}

func (k *cov1xKC) AssignRealmRole(ctx context.Context, id, role string) error {
	if k.onAssign != nil {
		k.onAssign()
	}
	if k.roleErr != nil {
		return k.roleErr
	}
	return k.Client.AssignRealmRole(ctx, id, role)
}

func (k *cov1xKC) SendVerificationEmail(_ context.Context, _ string) error {
	k.verifySent++
	return k.verifyErr
}

func (k *cov1xKC) SendPasswordResetEmail(_ context.Context, _ string) error {
	k.resetSent++
	return k.resetErr
}

type cov1xSettings struct {
	findErr, updErr error
	owner           string
}

func (s *cov1xSettings) FindBySlug(string) (*orgsettings.OrgSettings, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.owner != "" {
		return &orgsettings.OrgSettings{OrgID: s.owner}, nil
	}
	return nil, nil
}

func (s *cov1xSettings) Update(string, orgsettings.Updates, string) (*orgsettings.OrgSettings, error) {
	return nil, s.updErr
}

func TestCov1xSettingsLookupErrorAborts(t *testing.T) {
	boom := errors.New("settings down")
	reg := orgs.NewService()
	p := &Provisioner{Orgs: reg, Keycloak: keycloak.NewStubClient(), Settings: &cov1xSettings{findErr: boom}}
	res, err := p.Provision(context.Background(), in)
	if !errors.Is(err, boom) || res != nil {
		t.Fatalf("got %v %v", res, err)
	}
	if list, _ := reg.List(context.Background(), ""); len(list) != 0 {
		t.Fatalf("no org may be created: %d", len(list))
	}
}

func TestCov1xResumeRefusesSlugHeldByAnotherClub(t *testing.T) {
	reg := orgs.NewService()
	o, err := reg.Create(context.Background(), orgs.CreateInput{Slug: "aqua-club", Name: "Aqua Club"})
	if err != nil {
		t.Fatal(err)
	}
	p := &Provisioner{Orgs: reg, Keycloak: keycloak.NewStubClient(), Settings: &cov1xSettings{owner: "someone-else"}}
	if _, err := p.Provision(context.Background(), in); !errors.Is(err, orgs.ErrSlugTaken) {
		t.Fatalf("got %v", err)
	}
	if got, _ := reg.Get(context.Background(), o.ID); got.Status != orgs.StatusProvisioning {
		t.Fatalf("status must be untouched: %s", got.Status)
	}
}

func TestCov1xResumeAllowedWhenSettingsOwnerIsSameOrg(t *testing.T) {
	reg := orgs.NewService()
	o, _ := reg.Create(context.Background(), orgs.CreateInput{Slug: "aqua-club", Name: "Aqua Club"})
	p := &Provisioner{Orgs: reg, Keycloak: keycloak.NewStubClient(), Settings: &cov1xSettings{owner: o.ID}}
	res, err := p.Provision(context.Background(), in)
	if err != nil || !res.Resumed || res.Org.Status != orgs.StatusActive {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestCov1xSettingsUpdateErrorReturnsOrgSoFar(t *testing.T) {
	boom := errors.New("write failed")
	reg := orgs.NewService()
	p := &Provisioner{Orgs: reg, Keycloak: keycloak.NewStubClient(), Settings: &cov1xSettings{updErr: boom}}
	res, err := p.Provision(context.Background(), in)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "initialise club settings") {
		t.Fatalf("got %v", err)
	}
	if res == nil || res.Org == nil || res.Org.Status != orgs.StatusProvisioning || res.AdminUserID != "" {
		t.Fatalf("partial result: %+v", res)
	}
}

func TestCov1xNilSettingsAndBlankTeamCode(t *testing.T) {
	reg := orgs.NewService()
	p := &Provisioner{Orgs: reg, Keycloak: keycloak.NewStubClient()}
	noCode := in
	noCode.TeamCode = "  "
	res, err := p.Provision(context.Background(), noCode)
	if err != nil || res.Org.Status != orgs.StatusActive {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestCov1xListUsersErrorWrapped(t *testing.T) {
	boom := errors.New("kc list")
	p, _, _ := newProvisioner(&cov1xKC{Client: keycloak.NewStubClient(), listErr: boom})
	res, err := p.Provision(context.Background(), in)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "look up admin user") {
		t.Fatalf("got %v", err)
	}
	if res == nil || res.Org == nil {
		t.Fatal("org so far must be returned")
	}
}

func TestCov1xExistingAdminIsReusedNotRecreated(t *testing.T) {
	stub := keycloak.NewStubClient()
	kc := &cov1xKC{Client: stub}
	p, reg, _ := newProvisioner(kc)
	o, _ := reg.Create(context.Background(), orgs.CreateInput{Slug: "aqua-club", Name: "Aqua Club"})
	existing, err := stub.CreateUser(context.Background(), "boss@example.com", "B", "Oss", o.ID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Provision(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.AdminUserID != existing || kc.creates != 0 {
		t.Fatalf("admin %q want %q, creates=%d", res.AdminUserID, existing, kc.creates)
	}
}

func TestCov1xCreateUserGenericErrorWrapped(t *testing.T) {
	kc := &flakyKC{Client: keycloak.NewStubClient(), failCreate: 1}
	p, _, _ := newProvisioner(kc)
	_, err := p.Provision(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "create admin user") || errors.Is(err, ErrAdminEmailInUse) {
		t.Fatalf("got %v", err)
	}
}

func TestCov1xAssignRoleErrorWrapped(t *testing.T) {
	boom := errors.New("role denied")
	kc := &cov1xKC{Client: keycloak.NewStubClient(), roleErr: boom}
	p, reg, _ := newProvisioner(kc)
	_, err := p.Provision(context.Background(), in)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "assign admin role") {
		t.Fatalf("got %v", err)
	}
	if got, _ := reg.BySlug(context.Background(), "aqua-club"); got.Status != orgs.StatusProvisioning {
		t.Fatalf("must stay provisioning: %s", got.Status)
	}
	if kc.verifySent != 0 {
		t.Fatal("no email before the role is assigned")
	}
}

func TestCov1xEmailFailuresAreLoggedNotFatal(t *testing.T) {
	var buf bytes.Buffer
	kc := &cov1xKC{Client: keycloak.NewStubClient(), verifyErr: errors.New("smtp1"), resetErr: errors.New("smtp2")}
	p, _, _ := newProvisioner(kc)
	p.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	res, err := p.Provision(context.Background(), in)
	if err != nil || res.Org.Status != orgs.StatusActive {
		t.Fatalf("got %+v %v", res, err)
	}
	out := buf.String()
	if !strings.Contains(out, "verification email failed") || !strings.Contains(out, "password setup email failed") || !strings.Contains(out, "smtp1") || !strings.Contains(out, "smtp2") {
		t.Fatalf("log: %s", out)
	}
	if kc.verifySent != 1 || kc.resetSent != 1 {
		t.Fatalf("emails attempted: %d %d", kc.verifySent, kc.resetSent)
	}
}

func TestCov1xDefaultLoggerUsedWhenNoneSet(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	kc := &cov1xKC{Client: keycloak.NewStubClient(), verifyErr: errors.New("smtp")}
	p, _, _ := newProvisioner(kc)
	if _, err := p.Provision(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "verification email failed") {
		t.Fatalf("default logger not used: %q", buf.String())
	}
}

func TestCov1xActivationFailureWrapped(t *testing.T) {
	kc := &cov1xKC{Client: keycloak.NewStubClient()}
	p, reg, _ := newProvisioner(kc)
	// The org is deleted mid-run, so provisioning -> active is no longer valid.
	kc.onAssign = func() {
		o, _ := reg.BySlug(context.Background(), "aqua-club")
		if _, err := reg.SetStatus(context.Background(), o.ID, orgs.StatusDeleted); err != nil {
			t.Error(err)
		}
	}
	res, err := p.Provision(context.Background(), in)
	if !errors.Is(err, orgs.ErrInvalidStatus) || !strings.Contains(err.Error(), "activate organization") {
		t.Fatalf("got %v", err)
	}
	if res == nil || res.AdminUserID == "" {
		t.Fatalf("admin id must be reported: %+v", res)
	}
}

func TestCov1xRegistryLookupErrorPropagates(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close() // a closed pool fails every query deterministically, without any network
	p := &Provisioner{Orgs: orgs.NewServiceWithPool(pool), Keycloak: keycloak.NewStubClient()}
	res, err := p.Provision(context.Background(), in)
	if err == nil || errors.Is(err, orgs.ErrNotFound) || res != nil {
		t.Fatalf("got %v %v", res, err)
	}
}
