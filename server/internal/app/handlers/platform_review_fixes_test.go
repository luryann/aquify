package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
)

type stubSupport struct {
	resetErr, revokeErr error
}

func (s stubSupport) RevokeUserSessions(context.Context, string, string) (int, error) {
	return 0, s.revokeErr
}
func (s stubSupport) ResetMFA(context.Context, string, string) (int, error) { return 2, s.resetErr }
func (s stubSupport) VerifiedMFAUsers(context.Context, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

type supportRig struct {
	h          *PlatformHandlers
	router     http.Handler
	kc         *keycloak.StubClient
	audit      *audit.Service
	platformID string
	clubID     string
}

func newSupportRig(t *testing.T, support PlatformSupport) *supportRig {
	t.Helper()
	ctx := context.Background()
	reg := orgs.NewService()
	reg.SetStatusCacheTTL(0)
	r := &supportRig{kc: keycloak.NewStubClient(), audit: audit.NewService(), platformID: uuid.NewString(), clubID: uuid.NewString()}
	for _, o := range []orgs.CreateInput{
		{ID: r.platformID, Slug: "platform", Name: "Platform"},
		{ID: r.clubID, Slug: "aqua", Name: "Aqua"},
	} {
		if _, err := reg.Create(ctx, o); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.SetStatus(ctx, o.ID, orgs.StatusActive); err != nil {
			t.Fatal(err)
		}
	}
	r.h = NewPlatformHandlers(reg, nil, r.audit, r.platformID)
	r.h.SetSupport(r.kc, support)
	r.router = cov5xMount(r.h, r.platformID)
	return r
}

func (r *supportRig) addUser(id, email string, roles ...string) {
	r.kc.AddUser(keycloak.User{ID: id, Email: email, FirstName: "F", LastName: "L", Enabled: true,
		Attributes: map[string][]string{"org_id": {r.clubID}}})
	for _, role := range roles {
		_ = r.kc.EnsureRealmRolesExist(context.Background(), []string{role})
		_ = r.kc.AssignRealmRole(context.Background(), id, role)
	}
}

func (r *supportRig) post(path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Who", cov5xOperator)
	rr := httptest.NewRecorder()
	r.router.ServeHTTP(rr, req)
	return rr
}

// A multi-step action that changes state and then fails must still be audited.
func TestPlatformUserActionAuditsPartialFailure(t *testing.T) {
	t.Parallel()
	rig := newSupportRig(t, stubSupport{revokeErr: errors.New("db down")})
	rig.addUser("u-coach", "coach@aqua.example", "coach")

	rr := rig.post("/platform/orgs/"+rig.clubID+"/users/u-coach/reset-mfa", `{"reason":"locked out"}`)
	if rr.Code < 500 {
		t.Fatalf("reset-mfa with a failing second step: %d %s", rr.Code, rr.Body.String())
	}
	for name, orgID := range map[string]string{"club log": rig.clubID, "operator log": rig.platformID} {
		evs, err := rig.audit.Query(audit.Filter{OrgID: orgID, ActionPrefix: "platform.user.mfa_reset", Limit: 10})
		if err != nil || len(evs) != 1 {
			t.Fatalf("%s: want one mfa_reset event, got %d (%v)", name, len(evs), err)
		}
		meta := evs[0].Metadata
		if meta["outcome"] != "failed" || meta["removed_factors"] != "2" || meta["reason"] != "locked out" {
			t.Fatalf("%s metadata: %v", name, meta)
		}
	}
}

// Purge and restore are operator-only: nothing is written into the club's own log.
func TestPlatformPurgeAndRestoreAreNotMirroredIntoTheClubLog(t *testing.T) {
	t.Parallel()
	rig := newSupportRig(t, stubSupport{})
	for _, action := range []string{audit.ActionPlatformOrgPurge, audit.ActionPlatformOrgRestore} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = req.WithContext(cov5xPrincipalContext(req.Context(), rig.platformID))
		rig.h.record(req, action, rig.clubID, map[string]string{"k": "v"})
		if evs, _ := rig.audit.Query(audit.Filter{OrgID: rig.clubID, ActionPrefix: action, Limit: 5}); len(evs) != 0 {
			t.Fatalf("%s was mirrored into the club's log: %+v", action, evs)
		}
		if evs, _ := rig.audit.Query(audit.Filter{OrgID: rig.platformID, ActionPrefix: action, Limit: 5}); len(evs) != 1 {
			t.Fatalf("%s missing from the operator log", action)
		}
	}
	// Ordinary lifecycle events still reach the club.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(cov5xPrincipalContext(req.Context(), rig.platformID))
	rig.h.record(req, audit.ActionPlatformOrgSuspend, rig.clubID, nil)
	if evs, _ := rig.audit.Query(audit.Filter{OrgID: rig.clubID, ActionPrefix: audit.ActionPlatformOrgSuspend, Limit: 5}); len(evs) != 1 {
		t.Fatalf("suspend should be mirrored into the club's log: %+v", evs)
	}
}

// The exact account can sit past the first page of substring hits.
func TestPlatformAddAdminFindsExistingUserPastFirstPage(t *testing.T) {
	t.Parallel()
	rig := newSupportRig(t, stubSupport{})
	for i := 0; i < 60; i++ {
		rig.addUser(fmt.Sprintf("a-%03d", i), fmt.Sprintf("banjo%d@aqua.example", i))
	}
	rig.addUser("z-target", "jo@aqua.example", "coach")

	rr := rig.post("/platform/orgs/"+rig.clubID+"/admins", `{"email":"jo@aqua.example","first_name":"Jo","last_name":"K","reason":"admin left"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"promoted":true`) {
		t.Fatalf("existing user should be promoted, got %d %s", rr.Code, rr.Body.String())
	}
	roles, _ := rig.kc.GetUserRealmRoles(context.Background(), "z-target")
	has := false
	for _, r := range roles {
		has = has || r.Name == "admin"
	}
	if !has {
		t.Fatalf("z-target did not receive the admin role: %v", roles)
	}
}

// Role labels are resolved for the returned page only.
func TestPlatformListUsersResolvesRolesForThePageOnly(t *testing.T) {
	t.Parallel()
	rig := newSupportRig(t, stubSupport{})
	rig.addUser("u1", "a@aqua.example", "coach", "parent")
	rig.addUser("u2", "b@aqua.example", "swimmer")
	req := httptest.NewRequest(http.MethodGet, "/platform/orgs/"+rig.clubID+"/users?limit=1", nil)
	req.Header.Set("X-Test-Who", cov5xOperator)
	rr := httptest.NewRecorder()
	rig.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"roles":["coach","parent"]`) || strings.Contains(rr.Body.String(), "b@aqua.example") {
		t.Fatalf("page 1: %d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/platform/orgs/"+rig.clubID+"/users?role=bogus", nil)
	req.Header.Set("X-Test-Who", cov5xOperator)
	rr = httptest.NewRecorder()
	rig.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown role filter: %d", rr.Code)
	}
}

func cov5xPrincipalContext(ctx context.Context, platformID string) context.Context {
	return appmiddleware.ContextWithPrincipal(ctx, &appmiddleware.Principal{UserID: "op-1", OrgID: platformID, Roles: []string{PlatformRole}, MFAAuthenticated: true})
}
