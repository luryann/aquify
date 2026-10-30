package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/authn"
)

// Every environment predicate must classify spelling variants identically, so
// no path can fail open while another fails closed for the same APP_ENV.
func TestAppEnvPredicatesAgreeAcrossSpellingVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw       string
		dev       bool
		seeds     bool
		recognize bool
	}{
		{"development", true, true, true},
		{" Development\t", true, true, true},
		{"LOCAL", true, true, true},
		{"test", true, false, true},
		{" TEST ", true, false, true},
		{"staging", false, false, true},
		{" Production ", false, false, true},
		{"prod", false, false, false},
		{"", false, false, false},
	}
	for _, tc := range cases {
		if got := isExplicitDevelopmentEnv(tc.raw); got != tc.dev {
			t.Errorf("isExplicitDevelopmentEnv(%q)=%v want %v", tc.raw, got, tc.dev)
		}
		if got := allowsProviderStubFallback(tc.raw); got != tc.dev {
			t.Errorf("allowsProviderStubFallback(%q)=%v want %v", tc.raw, got, tc.dev)
		}
		if got := requiresStrictRuntimeDependencies(tc.raw); got == tc.dev {
			t.Errorf("requiresStrictRuntimeDependencies(%q)=%v", tc.raw, got)
		}
		if got := seedsLocalDevFixtures(tc.raw); got != tc.seeds {
			t.Errorf("seedsLocalDevFixtures(%q)=%v want %v", tc.raw, got, tc.seeds)
		}
		if got := isRecognizedAppEnv(tc.raw); got != tc.recognize {
			t.Errorf("isRecognizedAppEnv(%q)=%v want %v", tc.raw, got, tc.recognize)
		}
	}
}

func TestLoadConfigNormalizesAppEnvOnce(t *testing.T) {
	t.Setenv("APP_ENV", "  Production ")
	if got := LoadConfig().AppEnv; got != "production" {
		t.Fatalf("AppEnv=%q", got)
	}
}

func TestEffectiveAppEnvDefaultsEmptyToTestEverywhere(t *testing.T) {
	t.Parallel()
	if got := effectiveAppEnv(Config{}); got != "test" {
		t.Fatalf("empty -> %q", got)
	}
	if got := effectiveAppEnv(Config{AppEnv: " STAGING "}); got != "staging" {
		t.Fatalf("staging -> %q", got)
	}
	// workerOrgIDs and the router both derive their environment from it, so an
	// empty Config yields the dev org fallback and staging does not.
	if ids := scheduledOrgIDs(Config{}); len(ids) != 1 || ids[0] != "org-demo" {
		t.Fatalf("empty env scheduled orgs = %v", ids)
	}
	if ids := scheduledOrgIDs(Config{AppEnv: " Staging "}); len(ids) != 0 {
		t.Fatalf("staging scheduled orgs = %v", ids)
	}
}

func TestValidateDevAuthConfigRejectsInsecureSessionCookiesOutsideDevEnvs(t *testing.T) {
	cfg := Config{AppEnv: "production"}
	cfg.Auth.AllowInsecureSessionCookies = true
	if err := ValidateDevAuthConfig(cfg); err == nil {
		t.Fatal("expected insecure session cookies to be rejected in production")
	}
	cfg.AppEnv = "local"
	if err := ValidateDevAuthConfig(cfg); err != nil {
		t.Fatalf("local env should allow insecure cookies: %v", err)
	}
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	if !LoadConfig().Auth.AllowInsecureSessionCookies {
		t.Fatal("SESSION_COOKIE_SECURE=false must set AllowInsecureSessionCookies")
	}
}

// The coach list must work with the default (non-injected) Keycloak client: the
// public site wiring has to see the resolved stub, not the nil dependency.
func TestPublicTeamSiteListsCoachesWithDefaultKeycloakClient(t *testing.T) {
	t.Parallel()
	const org = authn.LocalDevOrgID
	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	admin := issueSessionTokenForUser(t, router, "admin-user", org, []string{"admin"}, true)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/org-settings", encodeJSONBody(t, map[string]any{
		"team_name": "DARE Aquatics", "public_slug": "dare", "public_site_enabled": true, "public_show_coaches": true,
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", org)
	req.Header.Set("Authorization", "Bearer "+admin)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/public/orgs/dare/site", nil))
	var site struct {
		Coaches []map[string]any `json:"coaches"`
	}
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &site) != nil {
		t.Fatalf("site: %d %s", rr.Code, rr.Body.String())
	}
	if len(site.Coaches) == 0 {
		t.Fatalf("public_show_coaches is on but no coaches were returned: %s", rr.Body.String())
	}
}
