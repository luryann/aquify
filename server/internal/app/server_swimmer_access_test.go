package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestSwimmerSignInControls(t *testing.T) {
	t.Parallel()

	const org = "org-swim-access"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memberSvc := members.NewService()
	router := NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, RouterDeps{Members: memberSvc})

	adminPrincipal := &appmiddleware.Principal{UserID: "seed-admin", OrgID: org, Roles: []string{"admin"}}
	hhA, err := memberSvc.CreateHousehold(adminPrincipal, members.CreateHouseholdInput{OrgID: org, HouseholdName: "A"})
	if err != nil {
		t.Fatal(err)
	}
	hhB, err := memberSvc.CreateHousehold(adminPrincipal, members.CreateHouseholdInput{OrgID: org, HouseholdName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(hh members.Household, first, kc string) members.Swimmer {
		sw, _, err := memberSvc.CreateSwimmer(adminPrincipal, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: first, LastName: "S", DateOfBirth: "2012-01-01", Gender: "female"})
		if err != nil {
			t.Fatal(err)
		}
		if err := memberSvc.LinkSwimmerToUser(org, sw.ID, kc); err != nil {
			t.Fatal(err)
		}
		return sw
	}
	swA := mk(hhA, "Ann", "kc-ann")
	mk(hhA, "Amy", "kc-amy")
	mk(hhB, "Bea", "kc-bea")

	adminTok := issueSessionTokenForUser(t, router, "adm", org, []string{"admin"}, true)
	parentA := issueSessionTokenWithHousehold(t, router, "par-a", org, hhA.ID, []string{"parent"}, false)
	parentB := issueSessionTokenWithHousehold(t, router, "par-b", org, hhB.ID, []string{"parent"}, false)
	annTok := issueSessionTokenWithHousehold(t, router, "kc-ann", org, hhA.ID, []string{"swimmer"}, false)
	amyTok := issueSessionTokenWithHousehold(t, router, "kc-amy", org, hhA.ID, []string{"swimmer"}, false)

	do := func(method, path, tok string, body any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", org)
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	expect := func(name string, rr *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if rr.Code != status {
			t.Fatalf("%s: want %d got %d body=%s", name, status, rr.Code, rr.Body.String())
		}
		if code != "" {
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(rr.Body.Bytes(), &env)
			if env.Error.Code != code && !strings.Contains(rr.Body.String(), code) {
				t.Fatalf("%s: want code %s body=%s", name, code, rr.Body.String())
			}
		}
	}
	setPortal := func(tok, id string, disabled bool) *httptest.ResponseRecorder {
		return do(http.MethodPatch, "/api/v1/members/swimmers/"+id, tok, map[string]any{"portal_access_disabled": disabled})
	}

	// Default: enabled.
	rr := do(http.MethodGet, "/api/v1/org-settings", adminTok, nil)
	expect("get settings", rr, http.StatusOK, "")
	var env struct {
		OrgSettings struct {
			Enabled bool `json:"swimmer_accounts_enabled"`
		} `json:"org_settings"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if !env.OrgSettings.Enabled {
		t.Fatal("swimmer_accounts_enabled should default to true")
	}
	expect("swimmer default", do(http.MethodGet, "/api/v1/groups", annTok, nil), http.StatusOK, "")

	// Swimmer cannot toggle; foreign parent cannot; own parent can.
	expect("swimmer toggle", setPortal(annTok, swA.ID, true), http.StatusForbidden, "")
	expect("foreign parent toggle", setPortal(parentB, swA.ID, true), http.StatusForbidden, "")
	expect("own parent toggle", setPortal(parentA, swA.ID, true), http.StatusOK, "")

	// Per-swimmer: only Ann blocked; parent and sibling unaffected.
	expect("ann blocked", do(http.MethodGet, "/api/v1/groups", annTok, nil), http.StatusForbidden, "swimmer_access_disabled")
	expect("amy ok", do(http.MethodGet, "/api/v1/groups", amyTok, nil), http.StatusOK, "")
	expect("parent ok", do(http.MethodGet, "/api/v1/groups", parentA, nil), http.StatusOK, "")
	expect("ann session readable", do(http.MethodGet, "/api/v1/auth/session", annTok, nil), http.StatusOK, "")
	expect("admin re-enable", setPortal(adminTok, swA.ID, false), http.StatusOK, "")
	expect("ann ok again", do(http.MethodGet, "/api/v1/groups", annTok, nil), http.StatusOK, "")

	// Global off: all swimmers blocked, parents/admin unaffected; only admin writes.
	expect("parent org write", do(http.MethodPut, "/api/v1/org-settings", parentA, map[string]any{"swimmer_accounts_enabled": false}), http.StatusForbidden, "")
	expect("admin global off", do(http.MethodPut, "/api/v1/org-settings", adminTok, map[string]any{"swimmer_accounts_enabled": false}), http.StatusOK, "")
	expect("ann global block", do(http.MethodGet, "/api/v1/groups", annTok, nil), http.StatusForbidden, "swimmer_access_disabled")
	expect("amy global block", do(http.MethodGet, "/api/v1/groups", amyTok, nil), http.StatusForbidden, "swimmer_access_disabled")
	expect("parent global ok", do(http.MethodGet, "/api/v1/groups", parentA, nil), http.StatusOK, "")
	expect("session still readable", do(http.MethodGet, "/api/v1/auth/session", amyTok, nil), http.StatusOK, "")
	expect("admin global on", do(http.MethodPut, "/api/v1/org-settings", adminTok, map[string]any{"swimmer_accounts_enabled": true}), http.StatusOK, "")
	expect("amy back", do(http.MethodGet, "/api/v1/groups", amyTok, nil), http.StatusOK, "")
}
