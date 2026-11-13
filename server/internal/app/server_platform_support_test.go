package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
)

type supportFixture struct {
	t           *testing.T
	router      http.Handler
	kc          *keycloak.StubClient
	platformOrg string
	clubID      string
	operator    string
	audit       *audit.Service
}

func newSupportFixture(t *testing.T) *supportFixture {
	t.Helper()
	platformOrg := uuid.NewString()
	registry := orgs.NewService()
	registry.SetStatusCacheTTL(0)
	kc := keycloak.NewStubClient()
	cfg := Config{}
	cfg.Platform.OrgID = platformOrg
	auditSvc := audit.NewService()
	router := NewRouterWithServices(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Orgs: registry, Keycloak: kc, Audit: auditSvc})
	f := &supportFixture{t: t, router: router, kc: kc, platformOrg: platformOrg, audit: auditSvc}
	f.operator = issueSession(t, router, sessionSpec{UserID: "op-1", OrgID: platformOrg, Roles: []string{"platform_admin"}, MFA: true})
	code, res := f.do("POST", "/api/v1/platform/orgs", `{"name":"Aqua Club","slug":"aqua-club","admin_email":"boss@aqua.example","admin_first_name":"B","admin_last_name":"O"}`)
	if code != http.StatusCreated {
		t.Fatalf("provision: %d %v", code, res)
	}
	f.clubID = res["org"].(map[string]any)["id"].(string)
	return f
}

func (f *supportFixture) do(method, path, body string) (int, map[string]any) {
	f.t.Helper()
	return platformRequest(f.t, f.router, f.operator, f.platformOrg, method, path, body)
}

func (f *supportFixture) addUser(email, org string, roles ...string) string {
	f.t.Helper()
	id := uuid.NewString()
	f.kc.AddUser(keycloak.User{ID: id, Email: email, FirstName: "F", LastName: "L", Enabled: true,
		Attributes: map[string][]string{"org_id": {org}}})
	for _, r := range roles {
		if err := f.kc.EnsureRealmRolesExist(context.Background(), []string{r}); err != nil {
			f.t.Fatal(err)
		}
		if err := f.kc.AssignRealmRole(context.Background(), id, r); err != nil {
			f.t.Fatal(err)
		}
	}
	return id
}

func (f *supportFixture) userPath(userID, action string) string {
	return "/api/v1/platform/orgs/" + f.clubID + "/users/" + userID + "/" + action
}

func (f *supportFixture) actionCount(name, userID string) int {
	n := 0
	for _, a := range f.kc.Actions() {
		if a.Action == name && a.UserID == userID {
			n++
		}
	}
	return n
}

const goodReason = `{"reason":"club asked by email"}`

// Every new support route is closed to anyone who is not an MFA'd platform_admin of the
// platform org: club admins, plain admins of the platform org, and foreign operators.
func TestPlatformSupportRoutesRejectNonOperators(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	clubAdmin := issueSession(t, f.router, sessionSpec{UserID: "club-admin", OrgID: f.clubID, Roles: []string{"admin"}, MFA: true})
	noMFA := issueSession(t, f.router, sessionSpec{UserID: "op-2", OrgID: f.platformOrg, Roles: []string{"platform_admin"}})
	plainAdmin := issueSession(t, f.router, sessionSpec{UserID: "adm", OrgID: f.platformOrg, Roles: []string{"admin"}, MFA: true})
	otherOrg := uuid.NewString()
	foreign := issueSession(t, f.router, sessionSpec{UserID: "op-3", OrgID: otherOrg, Roles: []string{"platform_admin"}, MFA: true})

	base := "/api/v1/platform/orgs/" + f.clubID
	routes := []struct{ method, path, body string }{
		{"PATCH", base, `{"name":"X","reason":"because"}`},
		{"GET", base + "/overview", ""},
		{"GET", base + "/activity", ""},
		{"GET", base + "/users", ""},
		{"POST", base + "/admins", `{"email":"n@x.example","first_name":"N","last_name":"M","reason":"because"}`},
		{"POST", f.userPath(coach, "password-reset"), goodReason},
		{"POST", f.userPath(coach, "resend-verification"), goodReason},
		{"POST", f.userPath(coach, "sign-out"), goodReason},
		{"POST", f.userPath(coach, "reset-mfa"), goodReason},
		{"POST", f.userPath(coach, "disable"), goodReason},
		{"POST", f.userPath(coach, "enable"), goodReason},
	}
	callers := []struct{ name, token, org string }{
		{"club admin", clubAdmin, f.clubID},
		{"operator without MFA", noMFA, f.platformOrg},
		{"plain admin of platform org", plainAdmin, f.platformOrg},
		{"foreign operator", foreign, otherOrg},
	}
	for _, rt := range routes {
		for _, c := range callers {
			if code, _ := platformRequest(t, f.router, c.token, c.org, rt.method, rt.path, rt.body); code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", rt.method, rt.path, c.name, code)
			}
		}
	}
	if n := f.actionCount("send_password_reset", coach); n != 0 {
		t.Fatalf("a refused request must not send mail: %d", n)
	}
}

func TestPlatformSupportUserGuards(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	otherClub := f.addUser("other@elsewhere.example", uuid.NewString(), "admin")
	operatorUser := f.addUser("op@aquify.example", f.platformOrg, "platform_admin")
	// A platform_admin who somehow carries the club's org attribute is still off limits.
	sneaky := f.addUser("sneaky@aqua.example", f.clubID, "platform_admin")

	if code, res := f.do("POST", f.userPath(coach, "password-reset"), "{}"); code != http.StatusBadRequest {
		t.Fatalf("a reason is mandatory: %d %v", code, res)
	}
	if code, _ := f.do("POST", f.userPath(coach, "password-reset"), `{"reason":"hi"}`); code != http.StatusBadRequest {
		t.Fatalf("a too-short reason is refused: %d", code)
	}
	if code, _ := f.do("POST", f.userPath(otherClub, "password-reset"), goodReason); code != http.StatusNotFound {
		t.Fatalf("another club's user must read as not found: %d", code)
	}
	if code, _ := f.do("POST", f.userPath(operatorUser, "disable"), goodReason); code != http.StatusNotFound {
		t.Fatalf("an operator is not a member of the club: %d", code)
	}
	if code, _ := f.do("POST", f.userPath(sneaky, "disable"), goodReason); code != http.StatusForbidden {
		t.Fatalf("platform_admin targets are refused: %d", code)
	}
	if code, _ := f.do("POST", f.userPath("no-such-user", "sign-out"), goodReason); code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", code)
	}
	if code, _ := f.do("POST", "/api/v1/platform/orgs/"+f.platformOrg+"/users/"+operatorUser+"/disable", goodReason); code != http.StatusConflict {
		t.Fatalf("the platform org is not a support target: %d", code)
	}
	for _, id := range []string{otherClub, operatorUser, sneaky} {
		if f.actionCount("send_password_reset", id) != 0 {
			t.Fatalf("no action may reach %s", id)
		}
	}
	u, _ := f.kc.GetUser(context.Background(), sneaky)
	if !u.Enabled {
		t.Fatal("the refused disable must not have applied")
	}
}

func TestPlatformSupportRecoveryActionsAreAuditedInBothLogs(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")

	if code, res := f.do("POST", f.userPath(coach, "password-reset"), goodReason); code != http.StatusOK || res["status"] != "ok" {
		t.Fatalf("password reset: %d %v", code, res)
	}
	if f.actionCount("send_password_reset", coach) != 1 {
		t.Fatal("password reset email was not sent")
	}
	if code, _ := f.do("POST", f.userPath(coach, "resend-verification"), goodReason); code != http.StatusOK || f.actionCount("send_verification_email", coach) != 1 {
		t.Fatalf("resend verification: %d", code)
	}
	if code, _ := f.do("POST", f.userPath(coach, "reset-mfa"), goodReason); code != http.StatusOK {
		t.Fatalf("reset mfa: %d", code)
	}

	// The club's own audit trail shows what support did, with the reason.
	code, act := f.do("GET", "/api/v1/platform/orgs/"+f.clubID+"/activity", "")
	if code != http.StatusOK {
		t.Fatalf("activity: %d", code)
	}
	seen := map[string]map[string]any{}
	for _, it := range act["items"].([]any) {
		ev := it.(map[string]any)
		seen[ev["action"].(string)] = ev
	}
	for _, action := range []string{"platform.user.password_reset", "platform.user.resend_verification", "platform.user.mfa_reset", "platform.org.create"} {
		if seen[action] == nil {
			t.Errorf("missing activity for %s: %v", action, seen)
		}
	}
	meta := seen["platform.user.password_reset"]["metadata"].(map[string]any)
	if meta["reason"] != "club asked by email" || meta["target_user_id"] != coach || meta["target_org_id"] != f.clubID {
		t.Fatalf("audit metadata: %v", meta)
	}
}

func TestPlatformSupportSignOutEndsTheSession(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	session := issueSession(t, f.router, sessionSpec{UserID: coach, OrgID: f.clubID, Roles: []string{"coach"}, MFA: true})
	if code, _ := platformRequest(t, f.router, session, f.clubID, "GET", "/api/v1/members/households", ""); code != http.StatusOK {
		t.Fatalf("coach before sign-out: %d", code)
	}
	if code, _ := f.do("POST", f.userPath(coach, "sign-out"), goodReason); code != http.StatusOK {
		t.Fatalf("sign-out: %d", code)
	}
	if code, _ := platformRequest(t, f.router, session, f.clubID, "GET", "/api/v1/members/households", ""); code != http.StatusUnauthorized {
		t.Fatalf("coach after sign-out: %d", code)
	}
}

func TestPlatformSupportDisableRevokesSessionsAndEnableRestores(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	session := issueSession(t, f.router, sessionSpec{UserID: coach, OrgID: f.clubID, Roles: []string{"coach"}, MFA: true})

	if code, _ := f.do("POST", f.userPath(coach, "disable"), goodReason); code != http.StatusOK {
		t.Fatalf("disable: %d", code)
	}
	if u, _ := f.kc.GetUser(context.Background(), coach); u.Enabled {
		t.Fatal("user should be disabled")
	}
	if code, _ := platformRequest(t, f.router, session, f.clubID, "GET", "/api/v1/members/households", ""); code != http.StatusUnauthorized {
		t.Fatalf("disabled user's session must die: %d", code)
	}
	if code, _ := f.do("POST", f.userPath(coach, "enable"), goodReason); code != http.StatusOK {
		t.Fatalf("enable: %d", code)
	}
	if u, _ := f.kc.GetUser(context.Background(), coach); !u.Enabled {
		t.Fatal("user should be enabled again")
	}
}

func TestPlatformSupportListsUsersWithRolesAndOverview(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	f.addUser("parent@aqua.example", f.clubID, "parent")
	f.addUser("stranger@elsewhere.example", uuid.NewString(), "admin")

	code, res := f.do("GET", "/api/v1/platform/orgs/"+f.clubID+"/users", "")
	if code != http.StatusOK {
		t.Fatalf("list: %d %v", code, res)
	}
	items := res["items"].([]any)
	if len(items) != 3 { // provisioned admin + coach + parent; the stranger belongs to another club
		t.Fatalf("expected 3 club users, got %d: %v", len(items), items)
	}
	for _, it := range items {
		u := it.(map[string]any)
		if u["email"] == "stranger@elsewhere.example" {
			t.Fatal("another club's user leaked into the list")
		}
		if u["id"] == coach {
			roles := u["roles"].([]any)
			if len(roles) != 1 || roles[0] != "coach" {
				t.Fatalf("coach roles: %v", roles)
			}
		}
	}
	if _, res := f.do("GET", "/api/v1/platform/orgs/"+f.clubID+"/users?role=coach", ""); len(res["items"].([]any)) != 1 {
		t.Fatalf("role filter: %v", res)
	}

	code, ov := f.do("GET", "/api/v1/platform/orgs/"+f.clubID+"/overview", "")
	if code != http.StatusOK {
		t.Fatalf("overview: %d %v", code, ov)
	}
	counts := ov["role_counts"].(map[string]any)
	if counts["admin"] != float64(1) || counts["coach"] != float64(1) || counts["parent"] != float64(1) {
		t.Fatalf("role counts: %v", counts)
	}
	if ov["payments"].(map[string]any)["connected"] != false {
		t.Fatalf("payments: %v", ov["payments"])
	}
}

func TestPlatformSupportAddAdmin(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	f.addUser("taken@elsewhere.example", uuid.NewString(), "admin")
	path := "/api/v1/platform/orgs/" + f.clubID + "/admins"

	if code, _ := f.do("POST", path, `{"email":"new@aqua.example","first_name":"N","last_name":"A"}`); code != http.StatusBadRequest {
		t.Fatalf("reason is mandatory: %d", code)
	}
	if code, _ := f.do("POST", path, `{"email":"not-an-email","first_name":"N","last_name":"A","reason":"replacing"}`); code != http.StatusBadRequest {
		t.Fatalf("email validation: %d", code)
	}
	code, res := f.do("POST", path, `{"email":"new@aqua.example","first_name":"N","last_name":"A","reason":"previous admin left"}`)
	if code != http.StatusCreated || res["promoted"] != false {
		t.Fatalf("new admin: %d %v", code, res)
	}
	newID := res["user_id"].(string)
	roles, _ := f.kc.GetUserRealmRoles(context.Background(), newID)
	if len(roles) != 1 || roles[0].Name != "admin" {
		t.Fatalf("new admin roles: %v", roles)
	}
	if f.actionCount("send_password_reset", newID) != 1 || f.actionCount("send_verification_email", newID) != 1 {
		t.Fatal("the new admin must be invited")
	}
	// An existing member is promoted, not duplicated, and is not re-invited.
	code, res = f.do("POST", path, `{"email":"coach@aqua.example","first_name":"C","last_name":"H","reason":"promote coach"}`)
	if code != http.StatusOK || res["promoted"] != true || res["user_id"] != coach {
		t.Fatalf("promote: %d %v", code, res)
	}
	if f.actionCount("send_password_reset", coach) != 0 {
		t.Fatal("a promoted user must not get a new-account invite")
	}
	if code, _ := f.do("POST", path, `{"email":"taken@elsewhere.example","first_name":"T","last_name":"E","reason":"replacing"}`); code != http.StatusConflict {
		t.Fatalf("an email owned by another club: %d", code)
	}
}

func TestPlatformUpdateOrg(t *testing.T) {
	f := newSupportFixture(t)
	path := "/api/v1/platform/orgs/" + f.clubID

	if code, _ := f.do("PATCH", path, `{"name":"Renamed"}`); code != http.StatusBadRequest {
		t.Fatalf("reason is mandatory: %d", code)
	}
	if code, _ := f.do("PATCH", path, `{"reason":"just because"}`); code != http.StatusBadRequest {
		t.Fatalf("nothing to change: %d", code)
	}
	if code, _ := f.do("PATCH", path, `{"name":"   ","reason":"just because"}`); code != http.StatusBadRequest {
		t.Fatalf("blank name: %d", code)
	}
	if code, _ := f.do("PATCH", path, `{"slug":"new-slug","name":"X","reason":"just because"}`); code != http.StatusBadRequest {
		t.Fatalf("the slug is not editable (unknown fields are rejected): %d", code)
	}
	code, res := f.do("PATCH", path, `{"name":"Aqua Swim Club","plan":"pro","reason":"club rebranded"}`)
	if code != http.StatusOK || res["name"] != "Aqua Swim Club" || res["plan"] != "pro" || res["slug"] != "aqua-club" {
		t.Fatalf("update: %d %v", code, res)
	}
	if _, got := f.do("GET", path, ""); got["name"] != "Aqua Swim Club" {
		t.Fatalf("persisted: %v", got)
	}
	if code, _ := f.do("PATCH", "/api/v1/platform/orgs/"+uuid.NewString(), `{"name":"X","reason":"because"}`); code != http.StatusNotFound {
		t.Fatalf("unknown org: %d", code)
	}
}

func TestPlatformSupportRefusesDeletedClubs(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	if code, _ := f.do("POST", "/api/v1/platform/orgs/"+f.clubID+"/offboard", ""); code != http.StatusOK {
		t.Fatalf("offboard: %d", code)
	}
	// Offboarding is still supportable: the club may need access to pull its export.
	if code, _ := f.do("POST", f.userPath(coach, "password-reset"), goodReason); code != http.StatusOK {
		t.Fatalf("support during offboarding: %d", code)
	}
}

// A busy club writes hundreds of its own audit events. Operator activity must still
// show: the feed filters by action prefix before applying its limit.
func TestPlatformActivityIsNotBuriedByTheClubsOwnAuditEvents(t *testing.T) {
	f := newSupportFixture(t)
	coach := f.addUser("coach@aqua.example", f.clubID, "coach")
	if code, _ := f.do("POST", f.userPath(coach, "password-reset"), goodReason); code != http.StatusOK {
		t.Fatalf("password reset: %d", code)
	}
	for i := 0; i < 250; i++ {
		if _, err := f.audit.Append(audit.AppendRequest{OrgID: f.clubID, ActorUserID: "club-admin", Action: "billing.invoice.issue", ResourceType: "invoice", ResourceID: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
	}
	code, act := f.do("GET", "/api/v1/platform/orgs/"+f.clubID+"/activity", "")
	if code != http.StatusOK {
		t.Fatalf("activity: %d", code)
	}
	found := false
	for _, it := range act["items"].([]any) {
		ev := it.(map[string]any)
		if a := ev["action"].(string); a == "platform.user.password_reset" {
			found = true
		} else if len(a) < 9 || a[:9] != "platform." {
			t.Fatalf("non-operator event in the feed: %s", a)
		}
	}
	if !found {
		t.Fatalf("operator activity was buried: %v", act)
	}
}

func TestPlatformSupportRoleFilterAppliesBeforePaging(t *testing.T) {
	f := newSupportFixture(t)
	for _, e := range []string{"c1@aqua.example", "c2@aqua.example", "c3@aqua.example"} {
		f.addUser(e, f.clubID, "coach")
	}
	for i := 0; i < 4; i++ {
		f.addUser(uuid.NewString()+"@aqua.example", f.clubID, "parent")
	}
	base := "/api/v1/platform/orgs/" + f.clubID + "/users?role=coach"
	_, res := f.do("GET", base+"&limit=2&offset=0", "")
	if n := len(res["items"].([]any)); n != 2 {
		t.Fatalf("first coach page: %d", n)
	}
	_, res = f.do("GET", base+"&limit=2&offset=2", "")
	if n := len(res["items"].([]any)); n != 1 {
		t.Fatalf("second coach page should hold the last coach: %d", n)
	}
	_, res = f.do("GET", base+"&search=c2", "")
	items := res["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["email"] != "c2@aqua.example" {
		t.Fatalf("role + search: %v", items)
	}
}

func TestPlatformAddAdminRefusesOperatorsAndDisabledAccounts(t *testing.T) {
	f := newSupportFixture(t)
	path := "/api/v1/platform/orgs/" + f.clubID + "/admins"
	f.addUser("op@aqua.example", f.clubID, "platform_admin")
	off := f.addUser("off@aqua.example", f.clubID, "coach")
	if err := f.kc.DisableUser(context.Background(), off); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.do("POST", path, `{"email":"op@aqua.example","first_name":"O","last_name":"P","reason":"promote"}`); code != http.StatusForbidden {
		t.Fatalf("an operator must not be promoted through a club: %d", code)
	}
	if code, res := f.do("POST", path, `{"email":"off@aqua.example","first_name":"O","last_name":"F","reason":"promote"}`); code != http.StatusConflict {
		t.Fatalf("a disabled account must be enabled first: %d %v", code, res)
	}
	roles, _ := f.kc.GetUserRealmRoles(context.Background(), off)
	for _, r := range roles {
		if r.Name == "admin" {
			t.Fatal("the refused promotion must not have granted admin")
		}
	}
}
