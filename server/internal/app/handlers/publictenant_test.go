package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
)

func tenantRouter(t *testing.T, reg *orgs.Service, settings func(string) (*orgsettings.OrgSettings, error)) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	NewPublicTenantHandlers(reg, settings, nil).RegisterPublicRoutes(r)
	return r
}

func getTenant(h http.Handler, query string) (int, string) {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/public/tenant?"+query, nil))
	return rr.Code, rr.Body.String()
}

func TestPublicTenantResolvesSlugAndDomainWithoutLeakingOrgID(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	o, _ := reg.Create(ctx, orgs.CreateInput{Slug: "aqua-club", Name: "Aqua Club"})
	_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
	_ = reg.AddDomain(ctx, o.ID, "swim.example.org")
	h := tenantRouter(t, reg, nil)

	code, body := getTenant(h, "slug=AQUA-CLUB")
	if code != http.StatusOK {
		t.Fatalf("slug: %d %s", code, body)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(body), &got)
	if len(got) != 2 || got["slug"] != "aqua-club" || got["name"] != "Aqua Club" || strings.Contains(body, o.ID) {
		t.Fatalf("response must be exactly slug+name and never the org id: %s", body)
	}
	if code, _ := getTenant(h, "host=swim.example.org"); code != http.StatusNotFound {
		t.Fatalf("unverified domain must 404: %d", code)
	}
	_ = reg.VerifyDomain(ctx, o.ID, "swim.example.org")
	if code, body := getTenant(h, "host=swim.example.org"); code != http.StatusOK || !strings.Contains(body, "aqua-club") {
		t.Fatalf("verified domain: %d %s", code, body)
	}
}

func TestPublicTenantHidesInactiveAndUnknownIdentically(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	o, _ := reg.Create(ctx, orgs.CreateInput{Slug: "sus-club", Name: "Sus"})
	_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
	_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusSuspended)
	h := tenantRouter(t, reg, nil)
	c1, b1 := getTenant(h, "slug=sus-club")
	c2, b2 := getTenant(h, "slug=never-heard-of-it")
	if c1 != http.StatusNotFound || c2 != http.StatusNotFound {
		t.Fatalf("status: %d %d", c1, c2)
	}
	strip := func(s string) string { // request ids differ
		var m map[string]map[string]any
		_ = json.Unmarshal([]byte(s), &m)
		delete(m["error"], "request_id")
		out, _ := json.Marshal(m)
		return string(out)
	}
	if strip(b1) != strip(b2) {
		t.Fatalf("suspended and unknown must be indistinguishable:\n%s\n%s", b1, b2)
	}
	if c, _ := getTenant(h, ""); c != http.StatusBadRequest {
		t.Fatalf("no params: %d", c)
	}
}

func TestPublicTenantFallsBackToLegacySettingsSlug(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	o, _ := reg.Create(ctx, orgs.CreateInput{Name: "Legacy"}) // no registry slug
	_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
	settings := func(slug string) (*orgsettings.OrgSettings, error) {
		if slug == "legacy-club" {
			return &orgsettings.OrgSettings{OrgID: o.ID, PublicSlug: slug}, nil
		}
		return nil, nil
	}
	h := tenantRouter(t, reg, settings)
	if code, body := getTenant(h, "slug=legacy-club"); code != http.StatusOK || !strings.Contains(body, "legacy-club") {
		t.Fatalf("legacy slug: %d %s", code, body)
	}
	failing := func(string) (*orgsettings.OrgSettings, error) { return nil, errors.New("x") }
	if code, _ := getTenant(tenantRouter(t, reg, failing), "slug=other"); code != http.StatusNotFound {
		t.Fatalf("settings lookup failure falls through to 404: %d", code)
	}
}
