package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"

	"github.com/luryann/aquify/server/internal/app/groups"
)

const (
	cov5xGrpHH1  = "5c0a0000-0000-4000-8000-0000000000d1"
	cov5xGrpHH2  = "5c0a0000-0000-4000-8000-0000000000d2"
	cov5xGrpSw1  = "5c0a0000-0000-4000-8000-0000000000e1"
	cov5xGrpSw2  = "5c0a0000-0000-4000-8000-0000000000e2"
	cov5xCoachID = "5c0a0000-0000-4000-8000-0000000000f2"
)

func cov5xGroupsTokens() tokenResolver {
	return tokenResolver{
		"admin":   {UserID: "5c0a0000-0000-4000-8000-0000000000f1", OrgID: cov5xOrg, Roles: []string{"admin"}},
		"coach":   {UserID: cov5xCoachID, OrgID: cov5xOrg, Roles: []string{"coach"}},
		"coach2":  {UserID: "5c0a0000-0000-4000-8000-0000000000f3", OrgID: cov5xOrg, Roles: []string{"coach"}},
		"parent":  {UserID: "5c0a0000-0000-4000-8000-0000000000f4", OrgID: cov5xOrg, HouseholdID: cov5xGrpHH1, Roles: []string{"parent"}},
		"swimmer": {UserID: "5c0a0000-0000-4000-8000-0000000000f5", OrgID: cov5xOrg, HouseholdID: cov5xGrpHH1, Roles: []string{"swimmer"}},
		"guest":   {UserID: "5c0a0000-0000-4000-8000-0000000000f6", OrgID: cov5xOrg, Roles: []string{"guest"}},
	}
}

func cov5xGroupsRouter(h *GroupsHandlers) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov5xGroupsTokens()))
		p.Use(appmiddleware.RequireOrgScope)
		h.RegisterRoutes(p)
	})
	return r
}

// cov5xGroupsFixture returns a router over a real in-memory groups service with
// two households (one swimmer each) and the parent token linked to household 1.
func cov5xGroupsFixture() (http.Handler, *groups.Service) {
	svc := groups.NewService()
	svc.SeedSwimmer(cov5xOrg, cov5xGrpSw1, cov5xGrpHH1, "2014-03-01", "female", "active")
	svc.SeedSwimmer(cov5xOrg, cov5xGrpSw2, cov5xGrpHH2, "2012-03-01", "male", "active")
	svc.SeedParentHousehold("5c0a0000-0000-4000-8000-0000000000f4", cov5xGrpHH1)
	return cov5xGroupsRouter(NewGroupsHandlers(svc)), svc
}

func cov5xCreateGroup(t *testing.T, router http.Handler, token, body string) string {
	t.Helper()
	rr := cov5xDo(router, "POST", "/groups", token, body)
	cov5xWant(t, "create group", rr, http.StatusCreated, `"created"`)
	g := cov5xParse(t, rr)["group"].(map[string]any)
	return g["id"].(string)
}

type cov5xGroupRoute struct{ method, path, body string }

func cov5xGroupRoutes() []cov5xGroupRoute {
	return []cov5xGroupRoute{
		{"POST", "/groups", `{"code":"A","name":"A"}`},
		{"PATCH", "/groups/g1", `{"name":"x"}`},
		{"GET", "/groups", ""},
		{"GET", "/groups/g1", ""},
		{"POST", "/groups/g1/eligibility-rules", `{"rule_type":"gender"}`},
		{"GET", "/groups/g1/eligibility-rules", ""},
		{"GET", "/groups/g1/eligibility/s1", ""},
		{"GET", "/registrations", ""},
		{"POST", "/registrations", `{"household_id":"h","swimmer_id":"s","group_id":"g","season":"2031"}`},
		{"POST", "/registrations/r1/transition", `{"status":"active"}`},
		{"POST", "/registrations/r1/waivers", `{"waiver_type":"liability"}`},
	}
}

func TestCov5xGroupsRoutesRequirePrincipalAndOrgScope(t *testing.T) {
	t.Parallel()
	register := NewGroupsHandlers(groups.NewService()).RegisterRoutes
	bare := chi.NewRouter()
	register(bare)
	authOnly := chi.NewRouter()
	authOnly.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov5xGroupsTokens()))
		register(p)
	})
	for _, rt := range cov5xGroupRoutes() {
		rr := cov5xDo(bare, rt.method, rt.path, "", rt.body)
		if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "unauthorized") {
			t.Errorf("%s %s without principal: %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		authOnly.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_org_scope") {
			t.Errorf("%s %s without org scope: %d %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestCov5xGroupsRejectMalformedAndInvalidBodies(t *testing.T) {
	t.Parallel()
	router, _ := cov5xGroupsFixture()
	for _, rt := range []cov5xGroupRoute{
		{"POST", "/groups", `{"code":`},
		{"PATCH", "/groups/g1", `{"bogus":1}`},
		{"POST", "/groups/g1/eligibility-rules", `[]`},
		{"POST", "/registrations", `{`},
		{"POST", "/registrations/r1/transition", `nope`},
		{"POST", "/registrations/r1/waivers", `{"waiver_type":`},
	} {
		cov5xWant(t, rt.method+" "+rt.path, cov5xDo(router, rt.method, rt.path, "admin", rt.body), http.StatusBadRequest, "invalid_request_body")
	}
	for _, rt := range []cov5xGroupRoute{
		{"POST", "/groups", `{"code":"X"}`},
		{"POST", "/groups", `{"code":"X","name":"X","type":"swimming"}`},
		{"PATCH", "/groups/g1", `{"type":"swimming"}`},
		{"POST", "/groups/g1/eligibility-rules", `{"rule_type":"shoe_size"}`},
		{"POST", "/registrations", `{"household_id":"h"}`},
		{"POST", "/registrations/r1/transition", `{"status":"deleted"}`},
		{"POST", "/registrations/r1/waivers", `{}`},
	} {
		cov5xWant(t, rt.method+" "+rt.path, cov5xDo(router, rt.method, rt.path, "admin", rt.body), http.StatusBadRequest, "validation_failed")
	}
}

func TestCov5xGroupsRoleDenials(t *testing.T) {
	t.Parallel()
	router, svc := cov5xGroupsFixture()
	gid := cov5xCreateGroup(t, router, "coach", `{"code":"gold","name":"Gold"}`)
	reg, err := svc.CreateRegistration(&appmiddleware.Principal{UserID: "a", OrgID: cov5xOrg, Roles: []string{"admin"}}, cov5xOrg,
		groups.CreateRegistrationInput{HouseholdID: cov5xGrpHH2, SwimmerID: cov5xGrpSw2, GroupID: gid, Season: "2031"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		rt    cov5xGroupRoute
		want  string
	}{
		{"guest", cov5xGroupRoute{"POST", "/groups", `{"code":"B","name":"B"}`}, "not allowed to create groups"},
		{"guest", cov5xGroupRoute{"PATCH", "/groups/" + gid, `{"name":"x"}`}, "not allowed to update groups"},
		{"coach2", cov5xGroupRoute{"PATCH", "/groups/" + gid, `{"name":"x"}`}, "not allowed to update groups"},
		{"guest", cov5xGroupRoute{"GET", "/groups", ""}, "not allowed to list groups"},
		{"guest", cov5xGroupRoute{"GET", "/groups/" + gid, ""}, "not allowed to view groups"},
		{"coach2", cov5xGroupRoute{"GET", "/groups/" + gid, ""}, "not allowed to view groups"},
		{"coach", cov5xGroupRoute{"POST", "/groups/" + gid + "/eligibility-rules", `{"rule_type":"gender"}`}, "Only admin can attach"},
		{"guest", cov5xGroupRoute{"GET", "/groups/" + gid + "/eligibility-rules", ""}, "not allowed to view eligibility rules"},
		{"swimmer", cov5xGroupRoute{"GET", "/groups/" + gid + "/eligibility/" + cov5xGrpSw1, ""}, "not allowed to evaluate eligibility"},
		{"parent", cov5xGroupRoute{"GET", "/groups/" + gid + "/eligibility/" + cov5xGrpSw2, ""}, "not allowed to evaluate eligibility"},
		{"swimmer", cov5xGroupRoute{"GET", "/registrations", ""}, "not allowed to list registrations"},
		{"guest", cov5xGroupRoute{"POST", "/registrations", `{"household_id":"` + cov5xGrpHH1 + `","swimmer_id":"` + cov5xGrpSw1 + `","group_id":"` + gid + `","season":"2031"}`}, "not allowed to create registrations"},
		{"parent", cov5xGroupRoute{"POST", "/registrations", `{"household_id":"` + cov5xGrpHH2 + `","swimmer_id":"` + cov5xGrpSw2 + `","group_id":"` + gid + `","season":"2032"}`}, "not allowed to create registrations"},
		{"guest", cov5xGroupRoute{"POST", "/registrations/" + reg.ID + "/transition", `{"status":"active"}`}, "forbidden"},
		{"parent", cov5xGroupRoute{"POST", "/registrations/" + reg.ID + "/waivers", `{"waiver_type":"liability"}`}, "not allowed to accept waivers"},
		{"coach", cov5xGroupRoute{"POST", "/registrations/" + reg.ID + "/waivers", `{"waiver_type":"liability"}`}, "not allowed to accept waivers"},
	} {
		rr := cov5xDo(router, tc.rt.method, tc.rt.path, tc.token, tc.rt.body)
		cov5xWant(t, tc.token+" "+tc.rt.method+" "+tc.rt.path, rr, http.StatusForbidden, tc.want)
	}
}

func TestCov5xGroupsNotFoundMapping(t *testing.T) {
	t.Parallel()
	router, svc := cov5xGroupsFixture()
	gid := cov5xCreateGroup(t, router, "admin", `{"code":"silver","name":"Silver"}`)
	missing := "grp_missing"
	for _, tc := range []struct {
		rt   cov5xGroupRoute
		code string
	}{
		{cov5xGroupRoute{"PATCH", "/groups/" + missing, `{"name":"x"}`}, "group_not_found"},
		{cov5xGroupRoute{"GET", "/groups/" + missing, ""}, "group_not_found"},
		{cov5xGroupRoute{"POST", "/groups/" + missing + "/eligibility-rules", `{"rule_type":"gender"}`}, "group_not_found"},
		{cov5xGroupRoute{"GET", "/groups/" + missing + "/eligibility-rules", ""}, "group_not_found"},
		{cov5xGroupRoute{"GET", "/groups/" + missing + "/eligibility/" + cov5xGrpSw1, ""}, "group_not_found"},
		{cov5xGroupRoute{"GET", "/groups/" + gid + "/eligibility/unknown-swimmer", ""}, "swimmer_not_found"},
		{cov5xGroupRoute{"POST", "/registrations", `{"household_id":"` + cov5xGrpHH1 + `","swimmer_id":"` + cov5xGrpSw1 + `","group_id":"` + missing + `","season":"2031"}`}, "group_not_found"},
		{cov5xGroupRoute{"POST", "/registrations", `{"household_id":"no-such-household","swimmer_id":"` + cov5xGrpSw1 + `","group_id":"` + gid + `","season":"2031"}`}, "household_not_found"},
		{cov5xGroupRoute{"POST", "/registrations", `{"household_id":"` + cov5xGrpHH1 + `","swimmer_id":"no-such-swimmer","group_id":"` + gid + `","season":"2031"}`}, "swimmer_not_found"},
		{cov5xGroupRoute{"POST", "/registrations/reg_missing/transition", `{"status":"active"}`}, "registration_not_found"},
		{cov5xGroupRoute{"POST", "/registrations/reg_missing/waivers", `{"waiver_type":"liability"}`}, "registration_not_found"},
	} {
		rr := cov5xDo(router, tc.rt.method, tc.rt.path, "admin", tc.rt.body)
		cov5xWant(t, tc.rt.method+" "+tc.rt.path, rr, http.StatusNotFound, tc.code)
	}
	// Unknown ids never leak across orgs: another org's group is "not found" too.
	if _, err := svc.GetGroup(&appmiddleware.Principal{UserID: "a", Roles: []string{"admin"}}, "other-org", gid); err == nil {
		t.Fatal("a group must not be visible from another org")
	}
}

func TestCov5xGroupsWriteValidationAndConflicts(t *testing.T) {
	t.Parallel()
	router, _ := cov5xGroupsFixture()
	gid := cov5xCreateGroup(t, router, "admin", `{"code":" gold ","name":" Gold ","type":"masters"}`)

	rr := cov5xDo(router, "GET", "/groups/"+gid, "admin", "")
	cov5xWant(t, "get", rr, http.StatusOK, `"code":"GOLD"`)
	cov5xWant(t, "get name trimmed", rr, http.StatusOK, `"name":"Gold"`)

	cov5xWant(t, "duplicate code", cov5xDo(router, "POST", "/groups", "admin", `{"code":"gold","name":"Dup"}`), http.StatusConflict, "group_code_conflict")
	cov5xWant(t, "age range", cov5xDo(router, "POST", "/groups", "admin", `{"code":"AGE","name":"Age","min_age":12,"max_age":9}`), http.StatusBadRequest, "invalid_group_request")
	cov5xWant(t, "blank name", cov5xDo(router, "POST", "/groups", "admin", `{"code":"AGE","name":"   "}`), http.StatusBadRequest, "invalid_group_request")

	cov5xWant(t, "update age range", cov5xDo(router, "PATCH", "/groups/"+gid, "admin", `{"min_age":12,"max_age":9}`), http.StatusBadRequest, "invalid_group_update")
	cov5xWant(t, "update blank name", cov5xDo(router, "PATCH", "/groups/"+gid, "admin", `{"name":" "}`), http.StatusBadRequest, "invalid_group_update")
	rr = cov5xDo(router, "PATCH", "/groups/"+gid, "admin", `{"name":"Gold Squad","is_active":false}`)
	cov5xWant(t, "update", rr, http.StatusOK, `"updated"`)
	g := cov5xParse(t, rr)["group"].(map[string]any)
	if g["name"] != "Gold Squad" || g["is_active"] != false {
		t.Fatalf("update not applied: %v", g)
	}

	// active_only hides the group that was just deactivated.
	cov5xWant(t, "all groups", cov5xDo(router, "GET", "/groups", "admin", ""), http.StatusOK, `"total":1`)
	cov5xWant(t, "active only", cov5xDo(router, "GET", "/groups?active_only=true", "admin", ""), http.StatusOK, `"total":0`)

	// Registrations to a deactivated group are rejected.
	cov5xWant(t, "inactive group", cov5xDo(router, "POST", "/registrations", "admin",
		`{"household_id":"`+cov5xGrpHH1+`","swimmer_id":"`+cov5xGrpSw1+`","group_id":"`+gid+`","season":"2031"}`), http.StatusBadRequest, "invalid_registration_request")
}

func TestCov5xGroupsEligibilityRulesAndEvaluation(t *testing.T) {
	t.Parallel()
	router, _ := cov5xGroupsFixture()
	gid := cov5xCreateGroup(t, router, "admin", `{"code":"junior","name":"Junior"}`)

	cov5xWant(t, "bad age param", cov5xDo(router, "POST", "/groups/"+gid+"/eligibility-rules", "admin",
		`{"rule_type":"age_range","parameters":{"min_age":"ten"}}`), http.StatusBadRequest, "invalid_rule_request")
	cov5xWant(t, "bad allowed param", cov5xDo(router, "POST", "/groups/"+gid+"/eligibility-rules", "admin",
		`{"rule_type":"gender","parameters":{"allowed":"female"}}`), http.StatusBadRequest, "must be an array of strings")
	cov5xWant(t, "attach age rule", cov5xDo(router, "POST", "/groups/"+gid+"/eligibility-rules", "admin",
		`{"rule_type":"age_range","parameters":{"min_age":8,"max_age":12}}`), http.StatusCreated, `"attached"`)
	cov5xWant(t, "attach gender rule", cov5xDo(router, "POST", "/groups/"+gid+"/eligibility-rules", "admin",
		`{"rule_type":"gender","parameters":{"allowed":["female"]}}`), http.StatusCreated, `"attached"`)

	rr := cov5xDo(router, "GET", "/groups/"+gid+"/eligibility-rules", "coach", "")
	cov5xWant(t, "list rules", rr, http.StatusOK, `"count":2`)
	if body := cov5xParse(t, rr); body["group_id"] != gid || len(body["rules"].([]any)) != 2 {
		t.Fatalf("rules body: %v", body)
	}

	// Swimmer 1 (female, 12) passes both rules; swimmer 2 (male) fails the gender rule.
	rr = cov5xDo(router, "GET", "/groups/"+gid+"/eligibility/"+cov5xGrpSw1, "parent", "")
	cov5xWant(t, "eligible", rr, http.StatusOK, `"eligible":true`)
	rr = cov5xDo(router, "GET", "/groups/"+gid+"/eligibility/"+cov5xGrpSw2, "admin", "")
	cov5xWant(t, "ineligible", rr, http.StatusOK, `"eligible":false`)
}

func TestCov5xGroupsRegistrationLifecycle(t *testing.T) {
	t.Parallel()
	router, svc := cov5xGroupsFixture()
	var seenIP string
	h := NewGroupsHandlers(svc)
	h.SetClientAddressResolver(func(*http.Request) string { seenIP = "203.0.113.9"; return seenIP })
	router = cov5xGroupsRouter(h)
	gid := cov5xCreateGroup(t, router, "admin", `{"code":"gold","name":"Gold"}`)

	body := `{"household_id":"` + cov5xGrpHH1 + `","swimmer_id":"` + cov5xGrpSw1 + `","group_id":"` + gid + `","season":"2031","notes":" first "}`
	rr := cov5xDo(router, "POST", "/registrations", "parent", body)
	cov5xWant(t, "create", rr, http.StatusCreated, `"pending"`)
	regID := cov5xParse(t, rr)["registration"].(map[string]any)["id"].(string)

	cov5xWant(t, "duplicate", cov5xDo(router, "POST", "/registrations", "parent", body), http.StatusConflict, "duplicate_registration")
	cov5xWant(t, "bad date", cov5xDo(router, "POST", "/registrations", "admin",
		`{"household_id":"`+cov5xGrpHH1+`","swimmer_id":"`+cov5xGrpSw1+`","group_id":"`+gid+`","season":"2032","registration_date":"01/02/2032"}`), http.StatusBadRequest, "YYYY-MM-DD")

	// Parents only see their own household; staff see all.
	cov5xWant(t, "parent list", cov5xDo(router, "GET", "/registrations", "parent", ""), http.StatusOK, `"total":1`)
	cov5xWant(t, "admin list limit", cov5xDo(router, "GET", "/registrations?limit=1", "admin", ""), http.StatusOK, `"limit":1`)

	// A parent may not activate; staff may; illegal jumps are 422.
	cov5xWant(t, "parent activates", cov5xDo(router, "POST", "/registrations/"+regID+"/transition", "parent", `{"status":"active"}`), http.StatusForbidden, "withdrawn")
	cov5xWant(t, "pending to inactive", cov5xDo(router, "POST", "/registrations/"+regID+"/transition", "admin", `{"status":"inactive"}`), http.StatusUnprocessableEntity, "invalid_transition")
	rr = cov5xDo(router, "POST", "/registrations/"+regID+"/transition", "coach", `{"status":"active"}`)
	cov5xWant(t, "activate", rr, http.StatusOK, `"updated"`)
	cov5xWant(t, "activate audit", rr, http.StatusOK, "registration.status_changed")

	// Waivers: recorded once per type, with the server-observed address.
	rr = cov5xDo(router, "POST", "/registrations/"+regID+"/waivers", "parent", `{"waiver_type":"Liability","ip_address":"6.6.6.6"}`)
	cov5xWant(t, "waiver", rr, http.StatusCreated, `"accepted"`)
	waiver := cov5xParse(t, rr)["waiver"].(map[string]any)
	if waiver["ip_address"] != seenIP || waiver["waiver_type"] != "liability" {
		t.Fatalf("waiver must record the resolver's address, not the client's: %v", waiver)
	}
	cov5xWant(t, "waiver again", cov5xDo(router, "POST", "/registrations/"+regID+"/waivers", "parent", `{"waiver_type":"liability"}`), http.StatusConflict, "waiver_already_accepted")
}

// Reads that cannot reach storage must surface as opaque 500s.
func TestCov5xGroupsStorageFailuresAreInternalErrors(t *testing.T) {
	svc := groups.NewServiceWithPool(testutil.ClosedPool(t))
	router := cov5xGroupsRouter(NewGroupsHandlers(svc))
	for _, rt := range []cov5xGroupRoute{
		{"GET", "/groups", ""},
		{"GET", "/groups/g1", ""},
		{"GET", "/groups/g1/eligibility-rules", ""},
		{"GET", "/groups/g1/eligibility/s1", ""},
		{"GET", "/registrations", ""},
	} {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rr := cov5xDo(router, rt.method, rt.path, "admin", rt.body)
			cov5xWant(t, "storage failure", rr, http.StatusInternalServerError, "An internal error occurred.")
			if strings.Contains(rr.Body.String(), "closed pool") {
				t.Errorf("storage error leaked: %s", rr.Body.String())
			}
		})
	}
}

func TestCov5xGroupsWaiverAndTransitionFallbackErrors(t *testing.T) {
	t.Parallel()
	router, svc := cov5xGroupsFixture()
	gid := cov5xCreateGroup(t, router, "admin", `{"code":"gold","name":"Gold"}`)
	reg, err := svc.CreateRegistration(&appmiddleware.Principal{UserID: "a", OrgID: cov5xOrg, Roles: []string{"admin"}}, cov5xOrg,
		groups.CreateRegistrationInput{HouseholdID: cov5xGrpHH1, SwimmerID: cov5xGrpSw1, GroupID: gid, Season: "2031"})
	if err != nil {
		t.Fatal(err)
	}
	// Whitespace passes struct validation but not the service's own check.
	cov5xWant(t, "blank waiver", cov5xDo(router, "POST", "/registrations/"+reg.ID+"/waivers", "admin", `{"waiver_type":"   "}`),
		http.StatusBadRequest, "invalid_waiver_request")
}

// A write that Postgres rejects while the registration is transitioned is an
// infrastructure failure: opaque 500, and the registration keeps its status.
func TestCov5xGroupsTransitionPersistenceFailureIsInternalError(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org, hh, sw := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, keycloak_user_id) VALUES ($1::uuid, $2::uuid, 'Cov Family', $3::uuid)`, hh, org, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Kai', 'Cov', '2014-06-01', 'male', 'active')`, sw, org, hh); err != nil {
		t.Fatal(err)
	}
	svc := groups.NewServiceWithPool(pool)
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	grp, _, err := svc.CreateGroup(admin, org, groups.CreateGroupInput{Code: "gold", Name: "Gold"})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := svc.CreateRegistration(admin, org, groups.CreateRegistrationInput{HouseholdID: hh, SwimmerID: sw, GroupID: grp.ID, Season: "2031"})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE FUNCTION cov5x_freeze_regs() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'registrations are frozen'; END $$`,
		`CREATE TRIGGER cov5x_freeze BEFORE UPDATE ON registrations FOR EACH ROW EXECUTE FUNCTION cov5x_freeze_regs()`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokenResolver{"admin": admin}))
		p.Use(appmiddleware.RequireOrgScope)
		NewGroupsHandlers(svc).RegisterRoutes(p)
	})
	req := httptest.NewRequest("POST", "/registrations/"+reg.ID+"/transition", strings.NewReader(`{"status":"active"}`))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("X-Org-Id", org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	cov5xWant(t, "transition", rr, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rr.Body.String(), "frozen") {
		t.Errorf("database error leaked: %s", rr.Body.String())
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM registrations WHERE id = $1`, reg.ID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("registration must stay pending: %q %v", status, err)
	}
}
