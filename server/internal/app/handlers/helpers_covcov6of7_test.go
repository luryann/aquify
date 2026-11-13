package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/audit"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov6xTenant is a throwaway club for tests that need a real (UUID) org id,
// e.g. Postgres-backed services in an isolated schema.
type cov6xTenant struct {
	org    string
	router http.Handler
	tokens tokenResolver
}

func cov6xTokensFor(org string) tokenResolver {
	return tokenResolver{
		"admin":    {UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true},
		"coach":    {UserID: uuid.NewString(), OrgID: org, Roles: []string{"coach"}},
		"parent":   {UserID: uuid.NewString(), OrgID: org, HouseholdID: uuid.NewString(), Roles: []string{"parent"}},
		"stranger": {UserID: uuid.NewString(), OrgID: org, Roles: []string{"guest"}},
	}
}

// cov6xNewTenant builds a fresh org and a router mounting register behind the
// real Auth + RequireOrgScope middleware.
func cov6xNewTenant(register func(chi.Router)) cov6xTenant {
	org := uuid.NewString()
	tokens := cov6xTokensFor(org)
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return cov6xTenant{org: org, router: r, tokens: tokens}
}

func (tn cov6xTenant) do(method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", tn.org)
	rr := httptest.NewRecorder()
	tn.router.ServeHTTP(rr, req)
	return rr
}

// cov6xBreakTable renames the org_id column of a tenant table inside the
// pool's isolated schema, so every later statement against it fails. It
// exercises the handler's "storage failed" paths against the real
// Postgres-backed service. The table is deliberately not dropped: the pool's
// search_path falls back to public, and a dropped table would silently
// resolve to the shared one. The isolated schema is dropped by the harness on
// cleanup.
func cov6xBreakTable(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `ALTER TABLE `+table+` RENAME COLUMN org_id TO org_id_broken`); err != nil {
		t.Fatalf("break %s: %v", table, err)
	}
}

// cov6xAuditOrgActions groups the audit events recorded for org by action.
func cov6xAuditOrgActions(t *testing.T, a *audit.Service, org string) map[string][]audit.AuditEvent {
	t.Helper()
	events, err := a.Query(audit.Filter{OrgID: org})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	out := map[string][]audit.AuditEvent{}
	for _, e := range events {
		out[e.Action] = append(out[e.Action], e)
	}
	return out
}

// seedUsers inserts a users row for every token principal, so columns that
// reference users(id) (updated_by, created_by, ...) accept their ids.
func (tn cov6xTenant) seedUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	testutil.SeedOrgs(t, pool, tn.org)
	for name, p := range tn.tokens {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1::uuid, $2::uuid, $3, $4)`,
			p.UserID, tn.org, name+"-"+p.UserID+"@example.test", "sub-"+p.UserID); err != nil {
			t.Fatalf("seed user %s: %v", name, err)
		}
	}
}
