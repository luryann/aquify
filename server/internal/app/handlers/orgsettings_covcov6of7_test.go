package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov6xOrgSettingsReadWriteAndValidation(t *testing.T) {
	t.Parallel()
	a := audit.NewService()
	svc := orgsettings.NewService()
	h := NewOrgSettingsHandlers(svc, a)

	// Two clubs share one service so slug uniqueness is observable.
	clubA := cov6xNewTenant(h.RegisterRoutes)
	clubB := cov6xNewTenant(h.RegisterRoutes)

	rr := clubA.do(http.MethodGet, "/org-settings", "parent", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = clubA.do(http.MethodPut, "/org-settings", "coach", `{"team_name":"x"}`)
	cov6xWant(t, rr, http.StatusForbidden, "Only admin may update")

	rr = clubA.do(http.MethodGet, "/org-settings", "coach", "")
	cov6xWant(t, rr, http.StatusOK, `"swimmer_accounts_enabled":true`)

	rr = clubA.do(http.MethodPut, "/org-settings", "admin", `{"team_name":`)
	cov6xWant(t, rr, http.StatusBadRequest, "")
	rr = clubA.do(http.MethodPut, "/org-settings", "admin", `{"bogus_field":1}`)
	cov6xWant(t, rr, http.StatusBadRequest, "")

	for _, tc := range []struct{ body, code string }{
		{`{"public_slug":"Not A Slug!"}`, "invalid_slug"},
		{`{"public_site_enabled":true}`, "invalid_slug"},
		{`{"timezone":"Mars/Olympus"}`, "invalid_profile"},
		{`{"currency":"dollars!"}`, "invalid_profile"},
		{`{"legal_name":"` + strings.Repeat("x", 121) + `"}`, "invalid_profile"},
	} {
		rr = clubA.do(http.MethodPut, "/org-settings", "admin", tc.body)
		cov6xWant(t, rr, http.StatusUnprocessableEntity, tc.code)
	}

	rr = clubA.do(http.MethodPut, "/org-settings", "admin",
		`{"team_code":"ABC","team_name":"Aqua Club","lsc_code":"pc","public_slug":"aqua-club","timezone":"America/Chicago","currency":"USD","swimmer_accounts_enabled":false}`)
	cov6xWant(t, rr, http.StatusOK, `"public_slug":"aqua-club"`)
	body := rr.Body.String()
	for _, want := range []string{`"lsc_code":"PC"`, `"currency":"usd"`, `"swimmer_accounts_enabled":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("update response missing %s: %s", want, body)
		}
	}

	// Another club cannot claim the same public slug.
	rr = clubB.do(http.MethodPut, "/org-settings", "admin", `{"public_slug":"aqua-club"}`)
	cov6xWant(t, rr, http.StatusConflict, "slug_taken")

	events := cov6xAuditOrgActions(t, a, clubA.org)
	if len(events["org_settings.update"]) != 1 {
		t.Fatalf("expected one org_settings.update audit event, got %d", len(events["org_settings.update"]))
	}
}

func TestCov6xOrgSettingsStorageFailures(t *testing.T) {
	t.Run("settings table unreadable", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		tn := cov6xNewTenant(NewOrgSettingsHandlers(orgsettings.NewServiceWithPool(pool), nil).RegisterRoutes)
		cov6xBreakTable(t, pool, "org_settings")

		rr := tn.do(http.MethodGet, "/org-settings", "admin", "")
		cov6xWant(t, rr, http.StatusInternalServerError, "Failed to load org settings.")
		rr = tn.do(http.MethodPut, "/org-settings", "admin", `{"team_name":"x"}`)
		cov6xWant(t, rr, http.StatusInternalServerError, "Failed to update org settings.")
	})

	t.Run("swimmer access flag unreadable", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		tn := cov6xNewTenant(NewOrgSettingsHandlers(orgsettings.NewServiceWithPool(pool), nil).RegisterRoutes)
		tn.seedUsers(t, pool)
		// A trigger nulls the flag on every write, so the row stays readable
		// and writable but the flag can no longer be scanned into a bool.
		for _, q := range []string{
			`ALTER TABLE org_settings ALTER COLUMN swimmer_accounts_enabled DROP NOT NULL`,
			`CREATE FUNCTION cov6x_null_flag() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.swimmer_accounts_enabled := NULL; RETURN NEW; END $$`,
			`CREATE TRIGGER cov6x_null_flag BEFORE INSERT OR UPDATE ON org_settings FOR EACH ROW EXECUTE FUNCTION cov6x_null_flag()`,
		} {
			if _, err := pool.Exec(t.Context(), q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		rr := tn.do(http.MethodPut, "/org-settings", "admin", `{"team_name":"x"}`)
		cov6xWant(t, rr, http.StatusInternalServerError, "Failed to update org settings.")
		rr = tn.do(http.MethodGet, "/org-settings", "admin", "")
		cov6xWant(t, rr, http.StatusInternalServerError, "Failed to load org settings.")
	})
}
