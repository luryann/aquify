package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const (
	cov6xOrg = "org-cov6x"
)

func cov6xTokens() tokenResolver {
	return tokenResolver{
		"admin":      {UserID: "u-admin", OrgID: cov6xOrg, Roles: []string{"admin"}, MFAAuthenticated: true},
		"adminnomfa": {UserID: "u-admin2", OrgID: cov6xOrg, Roles: []string{"admin"}},
		"coach":      {UserID: "u-coach", OrgID: cov6xOrg, Roles: []string{"coach"}},
		"parent":     {UserID: "u-parent", OrgID: cov6xOrg, HouseholdID: "hh-1", Roles: []string{"parent"}},
		"swimmer":    {UserID: "u-swimmer", OrgID: cov6xOrg, HouseholdID: "hh-1", SwimmerID: "sw-1", Roles: []string{"swimmer"}},
		"stranger":   {UserID: "u-stranger", OrgID: cov6xOrg, Roles: []string{"guest"}},
	}
}

// cov6xRouter mounts routes behind the real Auth + RequireOrgScope middleware.
func cov6xRouter(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov6xTokens()))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

func cov6xDo(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cov6xOrg)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func cov6xWant(t *testing.T, rr *httptest.ResponseRecorder, status int, bodySubstr string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("want status %d, got %d: %s", status, rr.Code, rr.Body.String())
	}
	if bodySubstr != "" && !strings.Contains(rr.Body.String(), bodySubstr) {
		t.Fatalf("want body containing %q, got %s", bodySubstr, rr.Body.String())
	}
}

type cov6xRoute struct{ method, path string }

// cov6xWalkRoutes lists every route registered by register, with chi
// parameters replaced by a placeholder segment.
func cov6xWalkRoutes(t *testing.T, register func(chi.Router)) []cov6xRoute {
	t.Helper()
	r := chi.NewRouter()
	register(r)
	var out []cov6xRoute
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		path := route
		for {
			i := strings.Index(path, "{")
			if i < 0 {
				break
			}
			j := strings.Index(path[i:], "}")
			path = path[:i] + "x" + path[i+j+1:]
		}
		path = strings.ReplaceAll(path, "/*", "")
		out = append(out, cov6xRoute{method, path})
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no routes registered")
	}
	return out
}

// cov6xAssertScopeGuards proves every route registered by register refuses a
// request with no authenticated principal (401) and one that is authenticated
// but never passed through the org-scope middleware (400 missing_org_scope).
func cov6xAssertScopeGuards(t *testing.T, register func(chi.Router)) {
	t.Helper()
	routes := cov6xWalkRoutes(t, register)

	bare := chi.NewRouter()
	register(bare)

	authOnly := chi.NewRouter()
	authOnly.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov6xTokens()))
		register(p)
	})

	for _, rt := range routes {
		rr := cov6xDo(bare, rt.method, rt.path, "", "{}")
		if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "unauthorized") {
			t.Errorf("%s %s without principal: got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}

		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		authOnly.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing_org_scope") {
			t.Errorf("%s %s without org scope: got %d %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestCov6xScopeGuardsAcrossHandlers(t *testing.T) {
	t.Parallel()

	cases := map[string]func(chi.Router){
		"billing": func(r chi.Router) { NewBillingHandlers(nil).RegisterRoutes(r) },
		"performance_extensions": func(r chi.Router) {
			(&PerformanceHandlers{}).registerExtensionRoutes(r)
		},
		"results_import": func(r chi.Router) {
			h := &PerformanceHandlers{}
			r.Post("/meets/{meetID}/results/import", h.importMeetResults)
		},
		"time_standards_import": func(r chi.Router) {
			h := &PerformanceHandlers{}
			r.Post("/time-standards/import", h.importTimeStandards)
		},
		"reports":         func(r chi.Router) { NewReportsHandlers(nil, nil).RegisterRoutes(r) },
		"compliance":      func(r chi.Router) { NewComplianceHandlers(nil, nil).RegisterRoutes(r) },
		"seasons":         func(r chi.Router) { NewSeasonsHandlers(nil, nil).RegisterRoutes(r) },
		"wellness":        func(r chi.Router) { NewWellnessHandlers(nil, nil).RegisterRoutes(r) },
		"orgsettings":     func(r chi.Router) { NewOrgSettingsHandlers(nil, nil).RegisterRoutes(r) },
		"payment_account": func(r chi.Router) { NewPaymentAccountHandlers(nil, nil, nil, nil, false).RegisterRoutes(r) },
		"attendance":      func(r chi.Router) { NewAttendanceSummaryHandlers(nil, nil, nil).RegisterRoutes(r) },
	}
	for name, register := range cases {
		register := register
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cov6xAssertScopeGuards(t, register)
		})
	}
}
