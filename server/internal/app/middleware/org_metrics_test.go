package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/telemetry"
)

// Per-org request metrics only count requests that passed the principal/org
// check, so a spoofed X-Org-Id can never create series, and the label is bounded.
func TestOrgRequestMetricsOnlyCountAuthenticatedOrgs(t *testing.T) {
	m := telemetry.NewMetrics()
	r := chi.NewRouter()
	r.Use(PrometheusMetrics(m))
	r.Group(func(g chi.Router) {
		g.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(ContextWithPrincipal(req.Context(), &Principal{UserID: "u", OrgID: "org-real", Roles: []string{"admin"}})))
			})
		})
		g.Use(RequireOrgScope)
		g.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})

	do := func(org string) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-Org-Id", org)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
	do("org-real")
	do("org-real")
	for i := 0; i < 500; i++ { // spoofed headers, rejected before they are authenticated
		do("spoof-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)))
	}

	rr := httptest.NewRecorder()
	m.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	var series []string
	for _, line := range strings.Split(rr.Body.String(), "\n") {
		if strings.HasPrefix(line, "aquify_http_org_requests_total{") {
			series = append(series, line)
		}
	}
	if len(series) != 1 || !strings.Contains(series[0], `org="org-real"`) || !strings.HasSuffix(series[0], " 2") {
		t.Fatalf("only the authenticated org may have a series (2 requests), got %v", series)
	}
}
