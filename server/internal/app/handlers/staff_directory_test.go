package handlers

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/keycloak"
)

type countingKC struct {
	keycloak.Client
	listCalls atomic.Int32
	roleCalls atomic.Int32
	// adminMemberCalls counts "members of the admin role" queries: exactly one
	// per uncached ListStaff.
	adminMemberCalls atomic.Int32
}

func (c *countingKC) ListUsersWithRealmRole(ctx context.Context, orgID, role string) ([]keycloak.User, error) {
	if role == "admin" {
		c.adminMemberCalls.Add(1)
	}
	return c.Client.ListUsersWithRealmRole(ctx, orgID, role)
}

func (c *countingKC) ListUsers(ctx context.Context, orgID, search string, limit, offset int) ([]keycloak.User, error) {
	c.listCalls.Add(1)
	return c.Client.ListUsers(ctx, orgID, search, limit, offset)
}

func (c *countingKC) GetUserRealmRoles(ctx context.Context, userID string) ([]keycloak.Role, error) {
	c.roleCalls.Add(1)
	return c.Client.GetUserRealmRoles(ctx, userID)
}

func newStaffKC(t *testing.T) *countingKC {
	t.Helper()
	stub := keycloak.NewStubClient()
	add := func(id, first, last, email, role string, enabled bool) {
		stub.AddUser(keycloak.User{ID: id, Email: email, FirstName: first, LastName: last, Enabled: enabled,
			Attributes: map[string][]string{"org_id": {"org-1"}}})
		if role != "" {
			if err := stub.AssignRealmRole(context.Background(), id, role); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("u-coach", "Cara", "Coach", "cara@example.com", "coach", true)
	add("u-admin", "Adam", "Admin", "adam@example.com", "admin", true)
	add("u-noname", "", "", "noname@example.com", "coach", true)
	add("u-disabled", "Dee", "Gone", "dee@example.com", "coach", false)
	add("u-parent", "Pat", "Parent", "pat@example.com", "parent", true)
	return &countingKC{Client: stub}
}

func TestStaffDirectoryIsTheSingleSourceForStaffRules(t *testing.T) {
	kc := newStaffKC(t)
	staff, err := NewStaffDirectory(kc).ListStaff(context.Background(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range staff {
		ids = append(ids, m.ID)
	}
	// Authenticated /users/staff view keeps the blank-name account (email fallback).
	if len(ids) != 3 || ids[0] != "u-admin" || ids[1] != "u-coach" || ids[2] != "u-noname" {
		t.Fatalf("staff ids = %v (sorted by display name, enabled admin/coach only)", ids)
	}
	if staff[2].DisplayName() != "noname@example.com" || staff[2].PersonalName() != "" {
		t.Fatalf("blank-name staff: display=%q personal=%q", staff[2].DisplayName(), staff[2].PersonalName())
	}
}

func TestCachedStaffListerCachesPerOrgWithinTTLAndExpires(t *testing.T) {
	kc := newStaffKC(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := NewCachedStaffLister(NewStaffDirectory(kc), time.Minute, func() time.Time { return now })

	for i := 0; i < 50; i++ {
		if _, err := cache.ListStaff(context.Background(), "org-1"); err != nil {
			t.Fatal(err)
		}
	}
	if got := kc.adminMemberCalls.Load(); got != 1 {
		t.Fatalf("Keycloak list calls after 50 requests = %d, want 1", got)
	}

	now = now.Add(59 * time.Second)
	_, _ = cache.ListStaff(context.Background(), "org-1")
	if kc.adminMemberCalls.Load() != 1 {
		t.Fatal("cache expired early")
	}
	now = now.Add(2 * time.Second)
	_, _ = cache.ListStaff(context.Background(), "org-1")
	if kc.adminMemberCalls.Load() != 2 {
		t.Fatalf("cache should refresh after TTL: list=%d", kc.adminMemberCalls.Load())
	}
	_, _ = cache.ListStaff(context.Background(), "org-2")
	if kc.adminMemberCalls.Load() != 3 {
		t.Fatal("cache must be per org")
	}
}

func TestCountOrgAdminsUsesOneRoleMembershipQueryNotPerUserLookups(t *testing.T) {
	kc := newStaffKC(t)
	h := &UserHandlers{kc: kc}
	got, err := h.countOrgAdmins(context.Background(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("admins = %d, want 1", got)
	}
	if kc.listCalls.Load() != 0 || kc.roleCalls.Load() != 0 {
		t.Fatalf("countOrgAdmins must not list users or look roles up per user (list=%d role=%d)", kc.listCalls.Load(), kc.roleCalls.Load())
	}

	failing := keycloak.NewStubClient()
	failing.GetUserRealmRolesErr = context.DeadlineExceeded
	if _, err := (&UserHandlers{kc: failing}).countOrgAdmins(context.Background(), "org-1"); err == nil {
		t.Fatal("a lookup failure must be returned (fail closed), not folded into a count")
	}
}

func TestStaffDirectoryFindsStaffBeyondFirst500Accounts(t *testing.T) {
	stub := keycloak.NewStubClient()
	for i := 0; i < 700; i++ {
		stub.AddUser(keycloak.User{ID: fmt.Sprintf("a-parent-%04d", i), Email: fmt.Sprintf("p%04d@example.com", i), Enabled: true,
			Attributes: map[string][]string{"org_id": {"org-1"}}})
	}
	stub.AddUser(keycloak.User{ID: "z-late-coach", Email: "late@example.com", FirstName: "Late", LastName: "Coach", Enabled: true,
		Attributes: map[string][]string{"org_id": {"org-1"}}})
	if err := stub.AssignRealmRole(context.Background(), "z-late-coach", "coach"); err != nil {
		t.Fatal(err)
	}
	staff, err := NewStaffDirectory(stub).ListStaff(context.Background(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(staff) != 1 || staff[0].ID != "z-late-coach" || len(staff[0].Roles) != 1 || staff[0].Roles[0] != "coach" {
		t.Fatalf("staff = %+v, want the coach beyond the first 500 accounts", staff)
	}
}
