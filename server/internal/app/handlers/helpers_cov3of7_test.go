package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

// Fixed UUID identifiers keep these fixtures valid for both the in-memory and
// the Postgres-backed services.
const (
	cov3xOrg        = "3c0a0000-0000-4000-8000-000000000001"
	cov3xAdminID    = "3c0a0000-0000-4000-8000-0000000000a1"
	cov3xCoachID    = "3c0a0000-0000-4000-8000-0000000000a2"
	cov3xParentID   = "3c0a0000-0000-4000-8000-0000000000a3"
	cov3xParent2ID  = "3c0a0000-0000-4000-8000-0000000000a4"
	cov3xSwimmerUID = "3c0a0000-0000-4000-8000-0000000000a5"
	cov3xHH1        = "3c0a0000-0000-4000-8000-0000000000b1"
	cov3xHH2        = "3c0a0000-0000-4000-8000-0000000000b2"
	cov3xSw1        = "3c0a0000-0000-4000-8000-0000000000c1"
	cov3xSw2        = "3c0a0000-0000-4000-8000-0000000000c2"
)

func cov3xTokens() tokenResolver {
	return tokenResolver{
		"admin":    {UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}},
		"coach":    {UserID: cov3xCoachID, OrgID: cov3xOrg, Roles: []string{"coach"}},
		"parent":   {UserID: cov3xParentID, OrgID: cov3xOrg, HouseholdID: cov3xHH1, Roles: []string{"parent"}},
		"parent2":  {UserID: cov3xParent2ID, OrgID: cov3xOrg, HouseholdID: cov3xHH2, Roles: []string{"parent"}},
		"parent0":  {UserID: cov3xParent2ID, OrgID: cov3xOrg, Roles: []string{"parent"}},
		"swimmer":  {UserID: cov3xSwimmerUID, OrgID: cov3xOrg, HouseholdID: cov3xHH1, SwimmerID: cov3xSw1, Roles: []string{"swimmer"}},
		"stranger": {UserID: cov3xSwimmerUID, OrgID: cov3xOrg, Roles: []string{"guest"}},
	}
}

// cov3xRouter mounts routes behind the real Auth + RequireOrgScope middleware.
func cov3xRouter(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov3xTokens()))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

// cov3xRouterWith is cov3xRouter with a caller-supplied token table.
func cov3xRouterWith(tokens tokenResolver, register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

// cov3xInsertUser creates the users row that created_by/updated_by foreign
// keys point at, inside the test's private schema.
func cov3xInsertUser(t *testing.T, pool *pgxpool.Pool, orgID, userID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $1, 'Test User', 'active', now(), now())`, userID, orgID, userID+"@example.test"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

// cov3xAuthOnlyRouter authenticates but never installs the org scope.
func cov3xAuthOnlyRouter(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov3xTokens()))
		register(p)
	})
	return r
}

// cov3xBareRouter mounts routes with no auth middleware at all.
func cov3xBareRouter(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	register(r)
	return r
}

func cov3xDo(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cov3xOrg)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

type cov3xRoute struct{ method, path, body string }

// cov3xAssertScopeGuards proves every route refuses a request that has no
// principal (401) and one that lacks the org scope (400 missing_org_scope).
func cov3xAssertScopeGuards(t *testing.T, register func(chi.Router), routes []cov3xRoute) {
	t.Helper()
	bare := cov3xBareRouter(register)
	authOnly := cov3xAuthOnlyRouter(register)
	for _, rt := range routes {
		rr := cov3xDo(bare, rt.method, rt.path, "", rt.body)
		if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "unauthorized") {
			t.Errorf("%s %s without principal: got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		authOnly.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_org_scope") {
			t.Errorf("%s %s without org scope: got %d %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}

func cov3xWant(t *testing.T, rr *httptest.ResponseRecorder, status int, codeSubstr string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("want status %d, got %d: %s", status, rr.Code, rr.Body.String())
	}
	if codeSubstr != "" && !strings.Contains(rr.Body.String(), codeSubstr) {
		t.Fatalf("want body containing %q, got %s", codeSubstr, rr.Body.String())
	}
}

func cov3xJSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, rr.Body.String())
	}
	return out
}

// cov3xBlockWrites makes every INSERT/UPDATE/DELETE on table fail inside the
// test's private schema, while reads keep working. It lets handler tests reach
// the "persistence failed" branches without mocking the service under test.
func cov3xBlockWrites(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	ctx := context.Background()
	fn := "cov3x_block_" + table
	stmts := []string{
		`CREATE FUNCTION ` + fn + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov3x blocked write'; END $$`,
		`CREATE TRIGGER ` + fn + ` BEFORE INSERT OR UPDATE OR DELETE ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION ` + fn + `()`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("block writes on %s: %v", table, err)
		}
	}
}
