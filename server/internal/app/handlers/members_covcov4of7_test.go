package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov4xMembersOrg = "org-cov4x-members"

func cov4xMembersTokens(org string) tokenResolver {
	return tokenResolver{
		"admin":     {UserID: "u-admin", OrgID: org, Roles: []string{"admin"}},
		"coach":     {UserID: "u-coach", OrgID: org, Roles: []string{"coach"}},
		"parent":    {UserID: "u-parent", OrgID: org, Roles: []string{"parent"}},
		"parent2":   {UserID: "u-parent2", OrgID: org, HouseholdID: "hh-someone-else", Roles: []string{"parent"}},
		"guest":     {UserID: "u-guest", OrgID: org, Roles: []string{"guest"}},
		"swimmer":   {UserID: "u-swim", OrgID: org, HouseholdID: "hh-someone-else", Roles: []string{"swimmer"}},
		"swimmerNH": {UserID: "u-swim2", OrgID: org, Roles: []string{"swimmer"}},
	}
}

type cov4xMembersEnv struct {
	t      *testing.T
	org    string
	svc    *members.Service
	audits *audit.Service
	h      http.Handler
	hhID   string
	swID   string
}

func cov4xNewMembersEnv(t *testing.T, svc *members.Service, org string) *cov4xMembersEnv {
	t.Helper()
	auditSvc := audit.NewService()
	handlers := NewMembershipHandlersWithAudit(svc, auditSvc)
	return &cov4xMembersEnv{
		t: t, org: org, svc: svc, audits: auditSvc,
		h: cov4xRouter(cov4xMembersTokens(org), handlers.RegisterProtectedRoutes),
	}
}

func (e *cov4xMembersEnv) do(method, path, token, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Org-Id", e.org)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)
	return rr
}

func cov4xErrCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error body %q: %v", rr.Body.String(), err)
	}
	return env.Error.Code
}

// seed creates a household and swimmer through the service as admin.
func (e *cov4xMembersEnv) seed() {
	e.t.Helper()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: e.org, Roles: []string{"admin"}}
	hh, err := e.svc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: e.org, HouseholdName: "Cov4x Family"})
	if err != nil {
		e.t.Fatal(err)
	}
	sw, _, err := e.svc.CreateSwimmer(admin, e.org, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Sam", LastName: "Cov", DateOfBirth: "2012-03-04", Gender: "female",
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.hhID, e.swID = hh.ID, sw.ID
}

func TestCov4xMembersRoutesRequireAuthContext(t *testing.T) {
	t.Parallel()
	h := NewMembershipHandlers(members.NewService())
	router := cov4xBareRouter(h.RegisterProtectedRoutes)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/households"},
		{http.MethodGet, "/households"},
		{http.MethodGet, "/households/x"},
		{http.MethodPatch, "/households/x"},
		{http.MethodDelete, "/households/x"},
		{http.MethodPost, "/households/x/contacts"},
		{http.MethodGet, "/households/x/contacts"},
		{http.MethodPost, "/swimmers"},
		{http.MethodGet, "/swimmers/x"},
		{http.MethodPatch, "/swimmers/x"},
		{http.MethodDelete, "/swimmers/x"},
		{http.MethodPost, "/swimmers/x/groups"},
		{http.MethodGet, "/swimmer-groups"},
		{http.MethodGet, "/swimmers"},
	}
	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized || cov4xErrCode(t, rr) != "unauthorized" {
			t.Errorf("%s %s: want 401 unauthorized, got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
	}
}

func TestCov4xMembersErrorMatrix(t *testing.T) {
	t.Parallel()
	e := cov4xNewMembersEnv(t, members.NewService(), cov4xMembersOrg)
	e.seed()
	hh, sw := e.hhID, e.swID

	// The first parent creates their own household so parent scoping has a target.
	rr := e.do(http.MethodPost, "/households", "parent", `{"org_id":"`+e.org+`","household_name":"Parent Home"}`)
	expectStatus(t, rr, http.StatusCreated, "parent creates household")
	var created struct {
		Household members.Household `json:"household"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	parentHH := created.Household.ID

	validSwimmer := func(household string) string {
		return `{"org_id":"` + e.org + `","household_id":"` + household + `","first_name":"Kim","last_name":"Lee","date_of_birth":"2013-05-06","gender":"male"}`
	}

	rr = e.do(http.MethodPost, "/swimmers", "parent", validSwimmer(parentHH))
	expectStatus(t, rr, http.StatusCreated, "parent creates own swimmer")
	var ps struct {
		Swimmer struct {
			ID string `json:"id"`
		} `json:"swimmer"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &ps); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, method, path, token, body string
		status                          int
		code                            string
	}{
		{"create household empty body", "POST", "/households", "admin", `{}`, 400, "validation_failed"},
		{"create household body org mismatch", "POST", "/households", "admin", `{"org_id":"other","household_name":"X"}`, 403, "scope_violation"},
		{"create household whitespace name", "POST", "/households", "admin", `{"org_id":"` + e.org + `","household_name":"   "}`, 400, "invalid_household_request"},
		{"create household guest", "POST", "/households", "guest", `{"org_id":"` + e.org + `","household_name":"X"}`, 403, "forbidden"},
		{"list households guest", "GET", "/households", "guest", ``, 403, "forbidden"},
		{"list households swimmer without household", "GET", "/households", "swimmerNH", ``, 403, "forbidden"},
		{"get household missing", "GET", "/households/nope", "admin", ``, 404, "household_not_found"},
		{"get household other parent", "GET", "/households/" + hh, "parent", ``, 403, "scope_violation"},
		{"get household guest", "GET", "/households/" + hh, "guest", ``, 403, "forbidden"},
		{"update household invalid status", "PATCH", "/households/" + hh, "admin", `{"status":"bogus"}`, 400, "validation_failed"},
		{"update household missing", "PATCH", "/households/nope", "admin", `{"notes":"n"}`, 404, "household_not_found"},
		{"update household other parent", "PATCH", "/households/" + hh, "parent", `{"notes":"n"}`, 403, "scope_violation"},
		{"update household guest", "PATCH", "/households/" + hh, "guest", `{"notes":"n"}`, 403, "forbidden"},
		{"update household empty name", "PATCH", "/households/" + hh, "admin", `{"household_name":" "}`, 400, "invalid_household_update"},
		{"delete household missing", "DELETE", "/households/nope", "admin", ``, 404, "household_not_found"},
		{"delete household with swimmers", "DELETE", "/households/" + hh, "admin", ``, 409, "household_has_swimmers"},
		{"delete household coach", "DELETE", "/households/" + hh, "coach", ``, 403, "forbidden"},
		{"add contact empty body", "POST", "/households/" + hh + "/contacts", "admin", `{}`, 400, "validation_failed"},
		{"add contact missing household", "POST", "/households/nope/contacts", "admin", `{"relationship":"mom","first_name":"A","last_name":"B"}`, 404, "household_not_found"},
		{"add contact other parent", "POST", "/households/" + hh + "/contacts", "parent", `{"relationship":"mom","first_name":"A","last_name":"B"}`, 403, "scope_violation"},
		{"add contact guest", "POST", "/households/" + hh + "/contacts", "guest", `{"relationship":"mom","first_name":"A","last_name":"B"}`, 403, "forbidden"},
		{"add contact blank name", "POST", "/households/" + hh + "/contacts", "admin", `{"relationship":"mom","first_name":" ","last_name":"B"}`, 400, "invalid_contact_request"},
		{"list contacts missing household", "GET", "/households/nope/contacts", "admin", ``, 404, "household_not_found"},
		{"list contacts other parent", "GET", "/households/" + hh + "/contacts", "parent", ``, 403, "scope_violation"},
		{"list contacts guest", "GET", "/households/" + hh + "/contacts", "guest", ``, 403, "forbidden"},
		{"create swimmer empty body", "POST", "/swimmers", "admin", `{}`, 400, "validation_failed"},
		{"create swimmer org mismatch", "POST", "/swimmers", "admin", `{"org_id":"other","household_id":"x","first_name":"A","last_name":"B","date_of_birth":"2013-05-06","gender":"male"}`, 403, "scope_violation"},
		{"create swimmer missing household", "POST", "/swimmers", "admin", validSwimmer("nope"), 404, "household_not_found"},
		{"create swimmer other parent", "POST", "/swimmers", "parent", validSwimmer(hh), 403, "scope_violation"},
		{"create swimmer parent medical flags", "POST", "/swimmers", "parent", strings.Replace(validSwimmer(parentHH), `"gender"`, `"medical_flags":["asthma"],"gender"`, 1), 403, "medical_flags_restricted"},
		{"create swimmer bad dob", "POST", "/swimmers", "admin", strings.Replace(validSwimmer(hh), "2013-05-06", "05/06/2013", 1), 400, "invalid_date_of_birth"},
		{"create swimmer guest", "POST", "/swimmers", "guest", validSwimmer(hh), 403, "forbidden"},
		{"create swimmer blank name", "POST", "/swimmers", "admin", strings.Replace(validSwimmer(hh), `"Kim"`, `" "`, 1), 400, "invalid_swimmer_request"},
		{"get swimmer missing", "GET", "/swimmers/nope", "admin", ``, 404, "swimmer_not_found"},
		{"get swimmer other parent", "GET", "/swimmers/" + sw, "parent", ``, 403, "scope_violation"},
		{"get swimmer guest", "GET", "/swimmers/" + sw, "guest", ``, 403, "forbidden"},
		{"update swimmer invalid gender", "PATCH", "/swimmers/" + sw, "admin", `{"gender":"x"}`, 400, "validation_failed"},
		{"update swimmer missing", "PATCH", "/swimmers/nope", "admin", `{"notes":"n"}`, 404, "swimmer_not_found"},
		{"update swimmer other parent", "PATCH", "/swimmers/" + sw, "parent", `{"notes":"n"}`, 403, "scope_violation"},
		{"update swimmer parent medical flags", "PATCH", "/swimmers/" + ps.Swimmer.ID, "parent", `{"medical_flags":["asthma"]}`, 403, "medical_flags_restricted"},
		{"update swimmer bad dob", "PATCH", "/swimmers/" + sw, "admin", `{"date_of_birth":"nope"}`, 400, "invalid_date_of_birth"},
		{"update swimmer guest", "PATCH", "/swimmers/" + sw, "guest", `{"notes":"n"}`, 403, "forbidden"},
		{"update swimmer blank name", "PATCH", "/swimmers/" + sw, "admin", `{"first_name":" "}`, 400, "invalid_swimmer_update"},
		{"delete swimmer missing", "DELETE", "/swimmers/nope", "admin", ``, 404, "swimmer_not_found"},
		{"delete swimmer coach", "DELETE", "/swimmers/" + sw, "coach", ``, 403, "forbidden"},
		{"assign group empty body", "POST", "/swimmers/" + sw + "/groups", "admin", `{}`, 400, "validation_failed"},
		{"assign group missing swimmer", "POST", "/swimmers/nope/groups", "admin", `{"group_code":"AG1"}`, 404, "swimmer_not_found"},
		{"assign group parent", "POST", "/swimmers/" + sw + "/groups", "parent", `{"group_code":"AG1"}`, 403, "forbidden"},
		{"assign group blank code", "POST", "/swimmers/" + sw + "/groups", "admin", `{"group_code":"  "}`, 400, "invalid_group_assignment"},
		{"list swimmers guest", "GET", "/swimmers", "guest", ``, 403, "forbidden"},
		{"list swimmers foreign household filter", "GET", "/swimmers?household_id=" + hh, "swimmer", ``, 403, "scope_violation"},
		{"list swimmers parent foreign household", "GET", "/swimmers?household_id=" + hh, "parent", ``, 403, "scope_violation"},
		{"list swimmer groups guest", "GET", "/swimmer-groups", "guest", ``, 403, "forbidden"},
	}
	for _, tc := range cases {
		rr := e.do(tc.method, tc.path, tc.token, tc.body)
		if rr.Code != tc.status || cov4xErrCode(t, rr) != tc.code {
			t.Errorf("%s: want %d %q, got %d %s", tc.name, tc.status, tc.code, rr.Code, rr.Body.String())
		}
	}
}

func TestCov4xMembersHappyPathsAndAudit(t *testing.T) {
	t.Parallel()
	e := cov4xNewMembersEnv(t, members.NewService(), cov4xMembersOrg)
	e.seed()

	rr := e.do(http.MethodGet, "/households/"+e.hhID, "admin", "")
	expectStatus(t, rr, http.StatusOK, "get household")
	var gh struct {
		Status    string            `json:"status"`
		Household members.Household `json:"household"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &gh); err != nil {
		t.Fatal(err)
	}
	if gh.Status != "ok" || gh.Household.ID != e.hhID || gh.Household.HouseholdName != "Cov4x Family" {
		t.Fatalf("unexpected household payload: %+v", gh)
	}

	rr = e.do(http.MethodGet, "/households?limit=1", "coach", "")
	expectStatus(t, rr, http.StatusOK, "list households")
	var lh struct {
		Count, Total, Limit int
		Households          []struct {
			ID string `json:"id"`
		} `json:"households"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &lh)
	if lh.Count != 1 || lh.Total != 1 || lh.Limit != 1 || lh.Households[0].ID != e.hhID {
		t.Fatalf("unexpected households page: %s", rr.Body.String())
	}

	// Contacts: add then list.
	expectStatus(t, e.do(http.MethodPost, "/households/"+e.hhID+"/contacts", "admin",
		`{"relationship":"mother","first_name":"Mo","last_name":"Cov"}`), http.StatusCreated, "add contact")
	rr = e.do(http.MethodGet, "/households/"+e.hhID+"/contacts", "coach", "")
	expectStatus(t, rr, http.StatusOK, "list contacts")
	var lc struct {
		Count       int    `json:"count"`
		HouseholdID string `json:"household_id"`
		Contacts    []struct {
			FirstName string `json:"first_name"`
		} `json:"contacts"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &lc)
	if lc.Count != 1 || lc.HouseholdID != e.hhID || lc.Contacts[0].FirstName != "Mo" {
		t.Fatalf("unexpected contacts: %s", rr.Body.String())
	}

	// Swimmer fetch with medical flag opt-in.
	rr = e.do(http.MethodGet, "/swimmers/"+e.swID+"?include_medical=TRUE", "admin", "")
	expectStatus(t, rr, http.StatusOK, "get swimmer")
	if !strings.Contains(rr.Body.String(), `"first_name":"Sam"`) {
		t.Fatalf("swimmer payload missing name: %s", rr.Body.String())
	}

	// Group assignment and the two list endpoints.
	expectStatus(t, e.do(http.MethodPost, "/swimmers/"+e.swID+"/groups", "coach", `{"group_code":"ag1","is_primary":true}`), http.StatusCreated, "assign group")
	rr = e.do(http.MethodGet, "/swimmer-groups", "admin", "")
	expectStatus(t, rr, http.StatusOK, "list swimmer groups")
	if !strings.Contains(rr.Body.String(), `"group_code":"AG1"`) {
		t.Fatalf("expected normalised group code AG1: %s", rr.Body.String())
	}
	rr = e.do(http.MethodGet, "/swimmers?group_code=AG1&include_medical=true", "admin", "")
	expectStatus(t, rr, http.StatusOK, "list swimmers by group")
	if !strings.Contains(rr.Body.String(), e.swID) {
		t.Fatalf("expected swimmer in group listing: %s", rr.Body.String())
	}

	// Swimmer-role bio edit is allowed but the response and audit snapshots are medical-free.
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: e.org, Roles: []string{"admin"}}
	if _, _, err := e.svc.UpdateSwimmer(admin, e.org, e.swID, members.UpdateSwimmerInput{
		MedicalFlags: testutil.StringsPtr("asthma"), Allergies: testutil.StringPtr("peanuts"),
	}); err != nil {
		t.Fatal(err)
	}
	// Put the swimmer token in the swimmer's household by using a dedicated router.
	tokens := tokenResolver{"sw": {UserID: "u-swim", OrgID: e.org, HouseholdID: e.hhID, Roles: []string{"swimmer"}}}
	swRouter := cov4xRouter(tokens, NewMembershipHandlersWithAudit(e.svc, e.audits).RegisterProtectedRoutes)
	req := httptest.NewRequest(http.MethodPatch, "/swimmers/"+e.swID, strings.NewReader(`{"notes":"Loves butterfly"}`))
	req.Header.Set("Authorization", "Bearer sw")
	req.Header.Set("X-Org-Id", e.org)
	rr = httptest.NewRecorder()
	swRouter.ServeHTTP(rr, req)
	expectStatus(t, rr, http.StatusOK, "swimmer bio edit")
	body := rr.Body.String()
	if strings.Contains(body, "asthma") || strings.Contains(body, "peanuts") || !strings.Contains(body, `"medical_flags_redacted":true`) || !strings.Contains(body, "Loves butterfly") {
		t.Fatalf("swimmer response must be redacted but include the new bio: %s", body)
	}

	// Admin deletes the swimmer, then the now-empty household; both are audited.
	expectStatus(t, e.do(http.MethodDelete, "/swimmers/"+e.swID, "admin", ""), http.StatusOK, "delete swimmer")
	rr = e.do(http.MethodDelete, "/households/"+e.hhID, "admin", "")
	expectStatus(t, rr, http.StatusOK, "delete household")
	if !strings.Contains(rr.Body.String(), `"status":"deleted"`) {
		t.Fatalf("unexpected delete body: %s", rr.Body.String())
	}
	for _, action := range []string{audit.ActionMembersSwimmerDelete, audit.ActionMembersHouseholdDelete} {
		evs, err := e.audits.Query(audit.Filter{OrgID: e.org, Action: action})
		if err != nil || len(evs) != 1 {
			t.Fatalf("want one %s audit event, got %d (%v)", action, len(evs), err)
		}
	}
}

func TestCov4xBuildSwimmerDeleteDependencyMessage(t *testing.T) {
	t.Parallel()
	if got := buildSwimmerDeleteDependencyMessage(nil); got != "Swimmer cannot be deleted while related records still exist." {
		t.Fatalf("empty message = %q", got)
	}
	got := buildSwimmerDeleteDependencyMessage([]members.SwimmerDeleteDependency{
		{Code: "registrations", Label: "registrations", Count: 2},
		{Code: "results", Label: "results", Count: 1},
	})
	want := "Swimmer cannot be deleted while related records still exist: registrations (2), results (1)."
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestCov4xWithoutMedicalKeys(t *testing.T) {
	t.Parallel()
	if withoutMedicalKeys(nil) != nil {
		t.Fatal("nil snapshot must stay nil")
	}
	out := withoutMedicalKeys(map[string]any{"medical_flags": []string{"a"}, "allergies": "x", "notes": "keep"})
	if len(out) != 1 || out["notes"] != "keep" {
		t.Fatalf("medical keys not stripped: %+v", out)
	}
}

// With the database gone, service failures must surface as 500 internal_error
// (never a client-error status, and never the raw cause).
func TestCov4xMembersDatabaseFailureReturns500(t *testing.T) {
	base := testutil.IsolatedSchemaPool(t)
	pool, err := pgxpool.NewWithConfig(context.Background(), base.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	org := uuid.NewString()
	svc := members.NewServiceWithPool(pool)
	e := cov4xNewMembersEnv(t, svc, org)
	e.seed()
	pool.Close()

	cases := []struct{ method, path, body string }{
		{"GET", "/households", ""},
		{"GET", "/households/" + e.hhID, ""},
		{"DELETE", "/households/" + e.hhID, ""},
		{"GET", "/households/" + e.hhID + "/contacts", ""},
		{"GET", "/swimmers/" + e.swID, ""},
		{"DELETE", "/swimmers/" + e.swID, ""},
		{"GET", "/swimmer-groups", ""},
	}
	// ListSwimmers reports a non-network service failure as a bad filter.
	rr := e.do("GET", "/swimmers", "admin", "")
	if rr.Code != http.StatusBadRequest || cov4xErrCode(t, rr) != "invalid_swimmer_filter" {
		t.Errorf("GET /swimmers: want 400 invalid_swimmer_filter, got %d %s", rr.Code, rr.Body.String())
	}
	for _, tc := range cases {
		rr = e.do(tc.method, tc.path, "admin", tc.body)
		if rr.Code != http.StatusInternalServerError || cov4xErrCode(t, rr) != "internal_error" {
			t.Errorf("%s %s: want 500 internal_error, got %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
}

// A swimmer with related records cannot be deleted; the 409 names the blockers.
func TestCov4xDeleteSwimmerWithDependenciesReturnsConflict(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := members.NewServiceWithPool(pool)
	e := cov4xNewMembersEnv(t, svc, org)
	e.seed()
	ctx := context.Background()
	var groupID string
	if err := pool.QueryRow(ctx, `INSERT INTO groups (org_id, code, name) VALUES ($1, 'AG1', 'Age Group') RETURNING id::text`, org).Scan(&groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO registrations (org_id, swimmer_id, household_id, group_id, season) VALUES ($1, $2, $3, $4, '2026')`,
		org, e.swID, e.hhID, groupID); err != nil {
		t.Fatalf("seed registration: %v", err)
	}

	rr := e.do(http.MethodDelete, "/swimmers/"+e.swID, "admin", "")
	expectStatus(t, rr, http.StatusConflict, "delete swimmer with registration")
	if cov4xErrCode(t, rr) != "swimmer_has_dependencies" || !strings.Contains(rr.Body.String(), "registrations (1)") {
		t.Fatalf("unexpected conflict body: %s", rr.Body.String())
	}
}
