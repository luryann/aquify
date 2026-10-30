package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

func TestUSASwimmingIntegrationRolesTestAndIdempotentSync(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	perf := performance.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{Members: members.NewService(), Performance: perf})
	org := "org-usa"
	admin := issueSessionTokenForUser(t, router, "admin-user", org, []string{"admin"}, true)
	coach := issueSessionTokenForUser(t, router, "coach-user", org, []string{"coach"}, false)
	parent := issueSessionTokenForUser(t, router, "parent-user", org, []string{"parent"}, false)

	do := func(token, method, path string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", org)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	base := "/api/v1/integrations/usa-swimming"

	for _, tok := range []string{coach, parent} {
		for _, c := range [][2]string{{"GET", base}, {"PUT", base}, {"POST", base + "/test"}, {"POST", base + "/sync"}} {
			if rr := do(tok, c[0], c[1], map[string]any{}); rr.Code != http.StatusForbidden {
				t.Fatalf("%s %s as non-admin = %d", c[0], c[1], rr.Code)
			}
		}
	}

	// Not ready until configured.
	if rr := do(admin, "POST", base+"/sync", nil); rr.Code != http.StatusConflict {
		t.Fatalf("sync unconfigured = %d", rr.Code)
	}
	// Bad URL rejected; key never echoed.
	if rr := do(admin, "PUT", base, map[string]any{"base_url": "http://evil.example.com"}); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("http url = %d %s", rr.Code, rr.Body)
	}
	rr := do(admin, "PUT", base, map[string]any{"base_url": "stub", "api_key": "super-secret-key-1234", "enabled": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "super-secret") || !strings.Contains(rr.Body.String(), `"api_key_hint":"1234"`) {
		t.Fatalf("key not redacted: %s", rr.Body)
	}
	if rr := do(admin, "GET", base, nil); strings.Contains(rr.Body.String(), "super-secret") {
		t.Fatal("get leaked key")
	}
	if rr := do(admin, "POST", base+"/test", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("test = %d %s", rr.Code, rr.Body)
	}

	var hh struct{ Household struct{ ID string } }
	_ = json.Unmarshal(do(admin, "POST", "/api/v1/members/households", map[string]any{"org_id": org, "household_name": "Stub"}).Body.Bytes(), &hh)
	var sw struct{ Swimmer struct{ ID string } }
	_ = json.Unmarshal(do(admin, "POST", "/api/v1/members/swimmers", map[string]any{
		"org_id": org, "household_id": hh.Household.ID, "first_name": "Sam", "last_name": "Stub",
		"date_of_birth": "2010-05-05", "gender": "male", "usa_swimming_id": "STUB0001",
	}).Body.Bytes(), &sw)
	if sw.Swimmer.ID == "" {
		t.Fatal("swimmer not created")
	}

	type syncResp struct{ Imported, Duplicates int }
	run := func() syncResp {
		rr := do(admin, "POST", base+"/sync", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("sync = %d %s", rr.Code, rr.Body)
		}
		var out syncResp
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
		return out
	}
	if r1 := run(); r1.Imported != 2 || r1.Duplicates != 0 {
		t.Fatalf("first sync = %+v", r1)
	}
	if r2 := run(); r2.Imported != 0 || r2.Duplicates != 2 {
		t.Fatalf("second sync = %+v", r2)
	}
	rr = do(admin, "GET", "/api/v1/swimmers/"+sw.Swimmer.ID+"/best-times", nil)
	if !strings.Contains(rr.Body.String(), "58120") {
		t.Fatalf("best time missing: %s", rr.Body)
	}
}
