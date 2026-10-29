package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// flakyKC wraps the stub and fails selected Keycloak calls.
type flakyKC struct {
	*keycloak.StubClient
	failRoles  bool
	failUpdate bool
	failAssign bool
	failEmail  bool
}

var errKCBlip = errors.New("keycloak blip")

func (f *flakyKC) GetUserRealmRoles(ctx context.Context, id string) ([]keycloak.Role, error) {
	if f.failRoles {
		return nil, errKCBlip
	}
	return f.StubClient.GetUserRealmRoles(ctx, id)
}

// Role-membership lookups share the failRoles switch: the last-admin guard now
// counts admins through one members-of-role query instead of per-user lookups.
func (f *flakyKC) ListUsersWithRealmRole(ctx context.Context, orgID, role string) ([]keycloak.User, error) {
	if f.failRoles {
		return nil, errKCBlip
	}
	return f.StubClient.ListUsersWithRealmRole(ctx, orgID, role)
}

func (f *flakyKC) UpdateUser(ctx context.Context, id string, fields keycloak.UpdateUserFields) error {
	if f.failUpdate {
		return errKCBlip
	}
	return f.StubClient.UpdateUser(ctx, id, fields)
}

func (f *flakyKC) AssignRealmRole(ctx context.Context, id, role string) error {
	if f.failAssign {
		return errKCBlip
	}
	return f.StubClient.AssignRealmRole(ctx, id, role)
}

func (f *flakyKC) SendVerificationEmail(ctx context.Context, id string) error {
	if f.failEmail {
		return errKCBlip
	}
	return f.StubClient.SendVerificationEmail(ctx, id)
}

func pendingUser(kc keycloak.Client) {
	kc.(interface{ AddUser(keycloak.User) }).AddUser(keycloak.User{
		ID: "pending-user", Email: "pending@example.com",
		Attributes: map[string][]string{"org_id": {"org-1"}, "registration_status": {"pending_approval"}, "requested_role": {"coach"}},
	})
}

func TestDeleteUserFailsClosedWhenRoleLookupFails(t *testing.T) {
	kc := &flakyKC{StubClient: keycloak.NewStubClient()}
	kc.AddUser(keycloak.User{ID: "u1", Attributes: map[string][]string{"org_id": {"org-test"}}})
	_ = kc.AssignRealmRole(context.Background(), "u1", "admin")
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, true)

	kc.failRoles = true // Keycloak blip during the guard
	rr := doRequest(router, http.MethodDelete, "/api/v1/users/u1", token, "", "org-test")
	if rr.Code < 500 {
		t.Fatalf("want a 5xx (could not verify), got %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := kc.GetUser(context.Background(), "u1"); err != nil {
		t.Fatalf("the last admin was deleted during a role-lookup failure: %v", err)
	}
}

func TestRemoveAdminRoleFailsClosedWhenAdminCountUnknown(t *testing.T) {
	kc := &flakyKC{StubClient: keycloak.NewStubClient()}
	for _, id := range []string{"u1", "u2"} {
		kc.AddUser(keycloak.User{ID: id, Attributes: map[string][]string{"org_id": {"org-test"}}})
		_ = kc.AssignRealmRole(context.Background(), id, "admin")
	}
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, true)

	kc.failRoles = true
	rr := doRequest(router, http.MethodDelete, "/api/v1/users/u1/roles/admin", token, "", "org-test")
	if rr.Code < 500 {
		t.Fatalf("want 5xx when admin count cannot be verified, got %d: %s", rr.Code, rr.Body.String())
	}
	kc.failRoles = false
	roles, _ := kc.GetUserRealmRoles(context.Background(), "u1")
	if len(roles) == 0 {
		t.Fatal("admin role was removed despite the failed guard")
	}
}

func TestListUsersSurfacesRoleLookupFailure(t *testing.T) {
	kc := &flakyKC{StubClient: keycloak.NewStubClient()}
	kc.AddUser(keycloak.User{ID: "u1", Enabled: true, Attributes: map[string][]string{"org_id": {"org-test"}}})
	router := newStep30BRouter(t, kc)
	token := issueStep30BToken(t, router, "admin1", "org-test", []string{"admin"}, true)

	kc.failRoles = true
	rr := doRequest(router, http.MethodGet, "/api/v1/users?role=coach", token, "", "org-test")
	if rr.Code < 500 {
		t.Fatalf("user list must fail rather than render roles as empty, got %d: %s", rr.Code, rr.Body.String())
	}
	rr = doRequest(router, http.MethodGet, "/api/v1/users/staff", token, "", "org-test")
	if rr.Code < 500 {
		t.Fatalf("staff list must fail rather than drop users, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestApproveUserReportsPartialFailures(t *testing.T) {
	// Role assignment failure is surfaced (not "approved").
	kc := &flakyKC{StubClient: keycloak.NewStubClient(), failAssign: true}
	pendingUser(kc.StubClient)
	router := newStep30BRouter(t, kc)
	token := issueStep30FToken(t, router, "admin-1", "org-1", []string{"admin"})
	rr := doRequest(router, http.MethodPost, "/api/v1/users/pending-user/approve", token, "", "org-1")
	if rr.Code < 500 || strings.Contains(rr.Body.String(), `"approved"`) {
		t.Fatalf("role assignment failure must not report approval: %d %s", rr.Code, rr.Body.String())
	}

	// Attribute write failure is surfaced.
	kc2 := &flakyKC{StubClient: keycloak.NewStubClient(), failUpdate: true}
	pendingUser(kc2.StubClient)
	router2 := newStep30BRouter(t, kc2)
	token2 := issueStep30FToken(t, router2, "admin-1", "org-1", []string{"admin"})
	rr = doRequest(router2, http.MethodPost, "/api/v1/users/pending-user/approve", token2, "", "org-1")
	if rr.Code < 500 {
		t.Fatalf("attribute update failure must be surfaced, got %d: %s", rr.Code, rr.Body.String())
	}

	// A failed welcome email is a warning on an otherwise successful approval.
	kc3 := &flakyKC{StubClient: keycloak.NewStubClient(), failEmail: true}
	pendingUser(kc3.StubClient)
	router3 := newStep30BRouter(t, kc3)
	token3 := issueStep30FToken(t, router3, "admin-1", "org-1", []string{"admin"})
	rr = doRequest(router3, http.MethodPost, "/api/v1/users/pending-user/approve", token3, "", "org-1")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "verification_email_failed") {
		t.Fatalf("want 200 with email warning, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSelfRegisterRollsBackWhenAttributesCannotBeWritten(t *testing.T) {
	kc := &flakyKC{StubClient: keycloak.NewStubClient(), failUpdate: true}
	router := newStep30BRouter(t, kc)
	body := `{"first_name":"Jane","last_name":"Doe","email":"jane@example.com","org_id":"org-1","requested_role":"coach"}`
	rr := doRequest(router, http.MethodPost, "/api/v1/register", "", body, "")
	if rr.Code < 500 {
		t.Fatalf("want 5xx when registration attributes cannot be saved, got %d: %s", rr.Code, rr.Body.String())
	}
	users, _ := kc.ListUsers(context.Background(), "", "jane@example.com", 10, 0)
	if len(users) != 0 {
		t.Fatalf("half-registered user left behind: %+v", users)
	}
}
