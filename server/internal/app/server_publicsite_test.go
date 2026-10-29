package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

func TestPublicTeamSite(t *testing.T) {
	t.Parallel()
	const org = "org-pubsite"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Members: members.NewService(),
		Meets:   meets.NewService(),
		Exports: exports.NewService(),
	})
	admin := issueSessionTokenForUser(t, router, "admin-user", org, []string{"admin"}, true)
	parent := issueSessionTokenForUser(t, router, "parent-user", org, []string{"parent"}, false)

	do := func(token, method, path string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		if token != "" {
			req.Header.Set("X-Org-Id", org)
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	must := func(rr *httptest.ResponseRecorder, want int, what string) map[string]any {
		t.Helper()
		if rr.Code != want {
			t.Fatalf("%s: want %d got %d %s", what, want, rr.Code, rr.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
		return out
	}

	// Unconfigured and disabled sites are 404 without any credentials.
	must(do("", http.MethodGet, "/api/v1/public/orgs/dare/site", nil), http.StatusNotFound, "unknown slug")
	must(do(admin, http.MethodPut, "/api/v1/org-settings", map[string]any{"team_name": "DARE Aquatics", "public_slug": "dare"}), http.StatusOK, "set slug")
	must(do("", http.MethodGet, "/api/v1/public/orgs/dare/site", nil), http.StatusNotFound, "disabled site")

	// Only admins may configure; slugs are validated.
	must(do(parent, http.MethodPut, "/api/v1/org-settings", map[string]any{"public_site_enabled": true}), http.StatusForbidden, "parent settings")
	must(do(admin, http.MethodPut, "/api/v1/org-settings", map[string]any{"public_slug": "Bad Slug!"}), http.StatusUnprocessableEntity, "bad slug")

	// Content: one public + one private announcement, public + private + group events, a future and a draft meet.
	pubAnn := must(do(admin, http.MethodPost, "/api/v1/announcements", map[string]any{"title": "Public news", "body": "Come see us", "audience": "all", "public": true}), http.StatusCreated, "pub ann")
	privAnn := must(do(admin, http.MethodPost, "/api/v1/announcements", map[string]any{"title": "Private secret", "body": "Parents only", "audience": "all"}), http.StatusCreated, "priv ann")
	draftAnn := must(do(admin, http.MethodPost, "/api/v1/announcements", map[string]any{"title": "Draft public", "body": "unpublished", "audience": "all", "public": true}), http.StatusCreated, "draft ann")
	_ = draftAnn
	must(do(admin, http.MethodPost, "/api/v1/announcements/"+pubAnn["id"].(string)+"/publish", nil), http.StatusOK, "publish pub")
	must(do(admin, http.MethodPost, "/api/v1/announcements/"+privAnn["id"].(string)+"/publish", nil), http.StatusOK, "publish priv")
	must(do(parent, http.MethodPatch, "/api/v1/announcements/"+privAnn["id"].(string), map[string]any{"public": true}), http.StatusForbidden, "parent toggle")

	start := time.Now().UTC().Add(72 * time.Hour)
	ev := func(title string, public bool, audience string) {
		body := map[string]any{"title": title, "category": "social", "starts_at": start.Format(time.RFC3339), "audience": audience, "public": public}
		if audience == "groups" {
			body["group_codes"] = []string{"AGE1"}
		}
		must(do(admin, http.MethodPost, "/api/v1/club-events", body), http.StatusCreated, "event "+title)
	}
	ev("Open pool party", true, "all")
	ev("Private banquet", false, "all")
	ev("Group-only public", true, "groups")

	meetDate := start.Format("2006-01-02")
	mk := func(name string, publish bool) {
		out := must(do(admin, http.MethodPost, "/api/v1/meets", map[string]any{"name": name, "meet_type": "invitational", "course": "SCY", "start_date": meetDate, "end_date": meetDate, "location": "Aqua Center"}), http.StatusCreated, "meet")
		id := out["meet"].(map[string]any)["id"].(string)
		if publish {
			must(do(admin, http.MethodPost, "/api/v1/meets/"+id+"/transition", map[string]any{"status": "published"}), http.StatusOK, "publish meet")
		}
	}
	mk("Spring Invitational", true)
	mk("Hidden Draft Meet", false)

	must(do(admin, http.MethodPut, "/api/v1/org-settings", map[string]any{"public_site_enabled": true, "public_contact_email": "hello@dare.example"}), http.StatusOK, "enable")

	rr := do("", http.MethodGet, "/api/v1/public/orgs/dare/site", nil)
	body := rr.Body.String()
	site := must(rr, http.StatusOK, "public site")
	for _, banned := range []string{"Private secret", "Draft public", "Private banquet", "Group-only public", "Hidden Draft Meet", "date_of_birth", "household", "created_by", "org_id", "parent-user", "admin-user"} {
		if strings.Contains(body, banned) {
			t.Fatalf("public payload leaked %q: %s", banned, body)
		}
	}
	// Allowlist, not blacklist: every key at every depth must be a known public field.
	assertPublicKeysAllowed(t, site)
	for _, want := range []string{"Public news", "Open pool party", "Spring Invitational", "DARE Aquatics", "hello@dare.example", "/register"} {
		if !strings.Contains(body, want) {
			t.Fatalf("public payload missing %q: %s", want, body)
		}
	}
	if n := len(site["announcements"].([]any)); n != 1 {
		t.Fatalf("want 1 announcement, got %d", n)
	}
	if n := len(site["coaches"].([]any)); n != 0 {
		t.Fatalf("coaches must be hidden by default, got %d", n)
	}

	// Toggling an announcement private removes it; disabling the site 404s again.
	must(do(admin, http.MethodPatch, "/api/v1/announcements/"+pubAnn["id"].(string), map[string]any{"public": false}), http.StatusOK, "untoggle")
	site = must(do("", http.MethodGet, "/api/v1/public/orgs/dare/site", nil), http.StatusOK, "site again")
	if n := len(site["announcements"].([]any)); n != 0 {
		t.Fatalf("want 0 announcements after untoggle, got %d", n)
	}
	must(do(admin, http.MethodPut, "/api/v1/org-settings", map[string]any{"public_site_enabled": false}), http.StatusOK, "disable")
	must(do("", http.MethodGet, "/api/v1/public/orgs/dare/site", nil), http.StatusNotFound, "disabled again")
}

// publicAllowedKeys mirrors the documented public payload; a new field must be
// added here on purpose (and in handlers/publicsite_leak_test.go).
var publicAllowedKeys = map[string][]string{
	"":              {"club", "announcements", "meets", "events", "records", "coaches"},
	"club":          {"name", "slug", "about", "contact_email", "contact_phone", "join_url"},
	"announcements": {"id", "title", "body", "published_at"},
	"meets":         {"id", "name", "start_date", "end_date", "location", "host_org", "course"},
	"events":        {"id", "title", "category", "description", "location", "starts_at", "ends_at", "all_day"},
	"records":       {"label", "course", "gender", "age_group", "distance", "stroke", "time_ms", "swimmer_name", "swim_date", "meet_name"},
	"coaches":       {"name", "role"},
}

func assertPublicKeysAllowed(t *testing.T, site map[string]any) {
	t.Helper()
	check := func(section string, obj map[string]any) {
		allowed := map[string]bool{}
		for _, k := range publicAllowedKeys[section] {
			allowed[k] = true
		}
		for k := range obj {
			if !allowed[k] {
				t.Errorf("public payload %q contains non-allowlisted key %q", section, k)
			}
		}
	}
	check("", site)
	for section, v := range site {
		switch x := v.(type) {
		case map[string]any:
			check(section, x)
		case []any:
			for _, el := range x {
				if obj, ok := el.(map[string]any); ok {
					check(section, obj)
				}
			}
		}
	}
}
