package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/compliance"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/training"
)

// Tenancy tracker item 0.2: seed org A across the main domains, then prove that an
// org B user cannot read, list, or change any of it through ANY route: every GET
// list route is swept for A's data, every route that takes an id is hit with each
// of A's ids, and every mutating route must refuse. The sweep walks the router, so
// a new route is covered automatically.

const matrixMarker = "ZMARKERORGA"

type matrixSeed struct {
	orgA, orgB string
	ids        map[string]string // entity -> id (only ids that appear in URLs)
	secondary  []string          // ids that may appear inside bodies
}

func seedOrgA(t *testing.T) (RouterDeps, matrixSeed) {
	t.Helper()
	const orgA, orgB = "org-matrix-a", "org-matrix-b"
	admin := &appmiddleware.Principal{UserID: "seed-admin", OrgID: orgA, Roles: []string{"admin"}, MFAAuthenticated: true}
	ms, gs, mt, bs, cs, ds, ts, comp := members.NewService(), groups.NewService(), meets.NewService(), billing.NewService(), comms.NewService(), documents.NewService(), training.NewService(), compliance.NewService()
	seed := matrixSeed{orgA: orgA, orgB: orgB, ids: map[string]string{}}

	hh, err := ms.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: orgA, HouseholdName: matrixMarker + " household", PrimaryEmail: "a@matrix.example"})
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["household"] = hh.ID
	sw, _, err := ms.CreateSwimmer(admin, orgA, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: matrixMarker, LastName: "Swimmer", DateOfBirth: "2012-03-04", Gender: "female"})
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["swimmer"] = sw.ID
	g, _, err := gs.CreateGroup(admin, orgA, groups.CreateGroupInput{Code: "MTRX", Name: matrixMarker + " group", Type: "competitive"})
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["group"] = g.ID
	m, _, err := mt.CreateMeet(admin, orgA, meets.CreateMeetInput{Name: matrixMarker + " meet", MeetType: "dual_meet", Course: "SCY", StartDate: "2026-12-01", EndDate: "2026-12-01", Location: "Pool"})
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["meet"] = m.ID
	inv, err := bs.CreateInvoice(orgA, "seed-admin", admin, billing.CreateInvoiceInput{HouseholdID: hh.ID, InvoiceNumber: matrixMarker + "-INV"})
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["invoice"] = inv.ID
	ann, err := cs.CreateAnnouncement(comms.CreateAnnouncementInput{OrgID: orgA, Title: matrixMarker + " news", Body: matrixMarker + " body", Audience: "all", CreatedBy: "seed-admin"})
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["announcement"] = ann.ID
	dt, err := ds.CreateDocumentType(orgA, matrixMarker+" waiver", "waiver", "", "household", true, true, nil, "seed-admin")
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["document_type"] = dt.ID
	pr, err := ts.Create(orgA, "2026-12-02", g.Code, "water", matrixMarker+" practice", 3000, nil, training.PracticeStrokes{Free: 3000}, "", "seed-admin")
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["practice"] = pr.ID
	ex, err := comp.CreateDataExportRequest(orgA, hh.ID, "seed-admin")
	if err != nil {
		t.Fatal(err)
	}
	seed.ids["export_request"] = ex.ID
	seed.secondary = []string{hh.ID, sw.ID}
	return RouterDeps{Members: ms, Groups: gs, Meets: mt, Billing: bs, Comms: cs, Documents: ds, Training: ts, Compliance: comp}, seed
}

func matrixCall(router http.Handler, method, path, token, org string) (int, string) {
	var body io.Reader
	if method != http.MethodGet && method != http.MethodDelete {
		body = bytes.NewReader([]byte(`{}`))
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", org)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

type matrixRoute struct{ method, template string }

func walkRoutes(t *testing.T, router http.Handler) []matrixRoute {
	t.Helper()
	var out []matrixRoute
	if err := chi.Walk(router.(chi.Routes), func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		out = append(out, matrixRoute{method, strings.ReplaceAll(pattern, "/*/", "/")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// routes the sweep must not call: streaming, probes, public/pre-auth surfaces and
// session/auth bookkeeping that is not tenant data.
func skipMatrixRoute(r matrixRoute) bool {
	if _, public := publicRoutes[r.method+" "+r.template]; public {
		return true
	}
	for _, frag := range []string{"/stream", "/openapi", "/auth/", "/dev-storage", "/platform/", "/metrics", "/webhooks/"} {
		if strings.Contains(r.template, frag) {
			return true
		}
	}
	return false
}

func TestOrgBCannotReachOrgAThroughAnyRoute(t *testing.T) {
	deps, seed := seedOrgA(t)
	cfg := Config{}
	cfg.Security.RateLimit = RateLimitConfig{GlobalRequests: 10_000_000, AuthRequests: 10_000_000, StripeRequests: 10_000_000,
		GlobalWindow: time.Minute, AuthWindow: time.Minute, StripeWindow: time.Minute}
	router := NewRouterWithServices(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), deps)

	tokens := map[string]string{
		"A admin": issueSessionToken(t, router, seed.orgA, []string{"admin"}, true),
		"B admin": issueSessionToken(t, router, seed.orgB, []string{"admin"}, true),
		"B coach": issueSessionToken(t, router, seed.orgB, []string{"coach"}, true),
	}
	routes := walkRoutes(t, router)

	leaks := func(body string, substituted string) bool {
		if strings.Contains(body, matrixMarker) {
			return true
		}
		for _, id := range seed.secondary {
			if id != substituted && strings.Contains(body, id) {
				return true
			}
		}
		return false
	}

	// 1. Sanity: A's own admin sees its marker, so a pass below is meaningful.
	seen := 0
	for _, r := range routes {
		if r.method != http.MethodGet || strings.Contains(r.template, "{") || skipMatrixRoute(r) {
			continue
		}
		if _, body := matrixCall(router, r.method, r.template, tokens["A admin"], seed.orgA); strings.Contains(body, matrixMarker) {
			seen++
		}
	}
	if seen < 5 {
		t.Fatalf("org A's admin saw its marker on only %d list routes; the seed or the sweep is broken", seen)
	}

	// 2. B must see none of A's data on any list route.
	listed := 0
	for _, who := range []string{"B admin", "B coach"} {
		for _, r := range routes {
			if r.method != http.MethodGet || strings.Contains(r.template, "{") || skipMatrixRoute(r) {
				continue
			}
			listed++
			if code, body := matrixCall(router, r.method, r.template, tokens[who], seed.orgB); leaks(body, "") {
				t.Errorf("%s: GET %s (%d) leaked org A data: %.200s", who, r.template, code, body)
			}
		}
	}

	// 3. Every route that takes ids, hit with each of A's ids as B.
	probed := 0
	for _, r := range routes {
		if !strings.Contains(r.template, "{") || skipMatrixRoute(r) {
			continue
		}
		for entity, id := range seed.ids {
			path := paramPath(r.template, id)
			probed++
			code, body := matrixCall(router, r.method, path, tokens["B admin"], seed.orgB)
			if leaks(body, id) {
				t.Errorf("B admin: %s %s (%s id) leaked org A data: %.200s", r.method, path, entity, body)
			}
			if r.method != http.MethodGet && code >= 200 && code < 300 {
				t.Errorf("B admin: %s %s (%s id) succeeded with %d; a foreign id must be refused", r.method, path, entity, code)
			}
		}
	}
	if listed < 20 || probed < 200 {
		t.Fatalf("sweep too small (lists=%d probes=%d); route discovery is broken", listed, probed)
	}
	t.Logf("swept %d list requests and %d id probes", listed, probed)
}

// paramPath substitutes every {param} in a route template with id.
func paramPath(template, id string) string {
	var b strings.Builder
	for i := 0; i < len(template); i++ {
		if template[i] != '{' {
			b.WriteByte(template[i])
			continue
		}
		end := strings.IndexByte(template[i:], '}')
		b.WriteString(id)
		i += end
	}
	return strings.ReplaceAll(b.String(), "*", "x")
}

var _ = json.Marshal
