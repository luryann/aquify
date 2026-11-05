package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
)

func platformRequest(t *testing.T, router http.Handler, token, orgHeader, method, path, body string) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if orgHeader != "" {
		req.Header.Set("X-Org-Id", orgHeader)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

// Tenancy tracker items 2.6 / 3.3: the operator plane is reachable only by a
// platform_admin of the platform org with MFA; it provisions and suspends orgs.
func TestPlatformPlaneProvisionsAndSuspendsOrgs(t *testing.T) {
	platformOrg := uuid.NewString()
	registry := orgs.NewService()
	registry.SetStatusCacheTTL(0)
	cfg := Config{}
	cfg.Platform.OrgID = platformOrg
	router := NewRouterWithServices(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Orgs: registry, Keycloak: keycloak.NewStubClient()})

	operator := issueSession(t, router, sessionSpec{UserID: "op-1", OrgID: platformOrg, Roles: []string{"platform_admin"}, MFA: true})
	noMFA := issueSession(t, router, sessionSpec{UserID: "op-2", OrgID: platformOrg, Roles: []string{"platform_admin"}, MFA: false})
	orgAdminOfClub := issueSession(t, router, sessionSpec{UserID: "adm-1", OrgID: platformOrg, Roles: []string{"admin"}, MFA: true})
	otherOrg := uuid.NewString()
	foreignOperator := issueSession(t, router, sessionSpec{UserID: "op-3", OrgID: otherOrg, Roles: []string{"platform_admin"}, MFA: true})

	body := `{"name":"Aqua Club","slug":"aqua-club","admin_email":"boss@example.com","admin_first_name":"B","admin_last_name":"O"}`
	if code, _ := platformRequest(t, router, noMFA, platformOrg, "POST", "/api/v1/platform/orgs", body); code != http.StatusForbidden {
		t.Fatalf("operator without MFA: %d", code)
	}
	if code, _ := platformRequest(t, router, orgAdminOfClub, platformOrg, "POST", "/api/v1/platform/orgs", body); code != http.StatusForbidden {
		t.Fatalf("plain admin of the platform org: %d", code)
	}
	if code, _ := platformRequest(t, router, foreignOperator, otherOrg, "POST", "/api/v1/platform/orgs", body); code != http.StatusForbidden {
		t.Fatalf("platform_admin role from a non-platform org must be refused: %d", code)
	}

	code, res := platformRequest(t, router, operator, platformOrg, "POST", "/api/v1/platform/orgs", body)
	if code != http.StatusCreated {
		t.Fatalf("provision: %d %v", code, res)
	}
	org := res["org"].(map[string]any)
	id := org["id"].(string)
	if org["status"] != "active" {
		t.Fatalf("org status: %v", org["status"])
	}
	if code, _ := platformRequest(t, router, operator, platformOrg, "POST", "/api/v1/platform/orgs", body); code != http.StatusConflict {
		t.Fatalf("repeat provision: %d", code)
	}

	// The new club's admin can use the API; suspending the org blocks them at once.
	clubAdmin := issueSession(t, router, sessionSpec{UserID: "club-admin", OrgID: id, Roles: []string{"admin"}, MFA: true})
	if code, _ := platformRequest(t, router, clubAdmin, id, "GET", "/api/v1/members/households", ""); code != http.StatusOK {
		t.Fatalf("club admin before suspend: %d", code)
	}
	if code, _ := platformRequest(t, router, operator, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/suspend", ""); code != http.StatusOK {
		t.Fatalf("suspend: %d", code)
	}
	if code, _ := platformRequest(t, router, clubAdmin, id, "GET", "/api/v1/members/households", ""); code != http.StatusForbidden {
		t.Fatalf("club admin after suspend: %d", code)
	}
	if code, _ := platformRequest(t, router, operator, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/resume", ""); code != http.StatusOK {
		t.Fatalf("resume: %d", code)
	}
	if code, _ := platformRequest(t, router, clubAdmin, id, "GET", "/api/v1/members/households", ""); code != http.StatusOK {
		t.Fatalf("club admin after resume: %d", code)
	}
	if code, _ := platformRequest(t, router, operator, platformOrg, "POST", "/api/v1/platform/orgs/"+platformOrg+"/suspend", ""); code != http.StatusConflict {
		t.Fatalf("platform org must be protected: %d", code)
	}
	code, list := platformRequest(t, router, operator, platformOrg, "GET", "/api/v1/platform/orgs", "")
	if code != http.StatusOK || list["count"].(float64) < 2 {
		t.Fatalf("list: %d %v", code, list)
	}
}

func TestPlatformRoutesAreNotMountedWithoutPlatformOrg(t *testing.T) {
	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tok := issueSession(t, router, sessionSpec{UserID: "op", OrgID: "org-demo", Roles: []string{"platform_admin"}, MFA: true})
	if code, _ := platformRequest(t, router, tok, "org-demo", "GET", "/api/v1/platform/orgs", ""); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("platform routes must not exist without PLATFORM_ORG_ID: %d", code)
	}
}

func TestPlatformOffboardingLifecycleAndGuards(t *testing.T) {
	platformOrg := uuid.NewString()
	registry := orgs.NewService()
	registry.SetStatusCacheTTL(0)
	cfg := Config{}
	cfg.Platform.OrgID = platformOrg
	router := NewRouterWithServices(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Orgs: registry, Keycloak: keycloak.NewStubClient()})
	op := issueSession(t, router, sessionSpec{UserID: "op", OrgID: platformOrg, Roles: []string{"platform_admin"}, MFA: true})
	_, res := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs", `{"name":"Gone Club","slug":"gone-club","admin_email":"a@gone.example","admin_first_name":"A","admin_last_name":"B"}`)
	id := res["org"].(map[string]any)["id"].(string)
	club := issueSession(t, router, sessionSpec{UserID: "club-admin", OrgID: id, Roles: []string{"admin"}, MFA: true})

	if c, _ := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs/"+platformOrg+"/offboard", ""); c != http.StatusConflict {
		t.Fatalf("the platform org cannot be offboarded: %d", c)
	}
	if c, _ := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/cancel-offboarding", ""); c != http.StatusConflict {
		t.Fatalf("cancelling something that is not offboarding: %d", c)
	}
	if c, _ := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/offboard", ""); c != http.StatusOK {
		t.Fatalf("offboard: %d", c)
	}
	if c, _ := platformRequest(t, router, club, id, "GET", "/api/v1/members/households", ""); c != http.StatusForbidden {
		t.Fatalf("an offboarding org loses access at once: %d", c)
	}
	// No database in this test: destructive steps are unavailable rather than silently skipped.
	if c, _ := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/purge", `{"confirm_slug":"gone-club"}`); c != http.StatusNotImplemented {
		t.Fatalf("purge without a database: %d", c)
	}
	if c, _ := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/cancel-offboarding", ""); c != http.StatusOK {
		t.Fatalf("cancel offboarding: %d", c)
	}
	if c, _ := platformRequest(t, router, op, platformOrg, "POST", "/api/v1/platform/orgs/"+id+"/resume", ""); c != http.StatusOK {
		t.Fatalf("resume: %d", c)
	}
	if c, _ := platformRequest(t, router, club, id, "GET", "/api/v1/members/households", ""); c != http.StatusOK {
		t.Fatalf("resumed org: %d", c)
	}
}
