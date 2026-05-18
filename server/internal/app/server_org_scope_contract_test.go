package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProtectedRoutesEnforceOrgScopeContracts(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	inScopeToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)
	crossOrgToken := issueSessionToken(t, router, "org-other", []string{"admin"}, true)

	routes := []struct {
		name          string
		method        string
		path          string
		body          map[string]any
		expectedMatch int
	}{
		{name: "authz ping", method: http.MethodGet, path: "/api/v1/authz/ping", expectedMatch: http.StatusOK},
		{name: "runtime settings", method: http.MethodGet, path: "/api/v1/settings/runtime", expectedMatch: http.StatusOK},
		{name: "member households", method: http.MethodGet, path: "/api/v1/members/households", expectedMatch: http.StatusOK},
		{name: "audit events", method: http.MethodGet, path: "/api/v1/audit-events", expectedMatch: http.StatusOK},
		{
			name:          "role assignments list",
			method:        http.MethodGet,
			path:          "/api/v1/auth/roles/assignments",
			expectedMatch: http.StatusOK,
		},
		{
			name:   "role assignment create",
			method: http.MethodPost,
			path:   "/api/v1/auth/roles/assign",
			body: map[string]any{
				"org_id":         "org-demo",
				"target_user_id": "user-123",
				"role":           "coach",
				"reason":         "Org scope contract coverage",
			},
			expectedMatch: http.StatusCreated,
		},
	}

	for _, route := range routes {
		route := route
		t.Run(route.name+"/missing header", func(t *testing.T) {
			t.Parallel()

			rr := doOrgScopeContractRequest(t, router, route.method, route.path, inScopeToken, "", route.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
			assertOrgScopeErrorCode(t, rr, "missing_org_scope")
		})

		t.Run(route.name+"/cross org", func(t *testing.T) {
			t.Parallel()

			rr := doOrgScopeContractRequest(t, router, route.method, route.path, crossOrgToken, "org-demo", route.body)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
			}
			assertOrgScopeErrorCode(t, rr, "scope_violation")
		})

		t.Run(route.name+"/matching org", func(t *testing.T) {
			t.Parallel()

			rr := doOrgScopeContractRequest(t, router, route.method, route.path, inScopeToken, "org-demo", route.body)
			if rr.Code != route.expectedMatch {
				t.Fatalf("expected %d, got %d: %s", route.expectedMatch, rr.Code, rr.Body.String())
			}
		})
	}
}

func doOrgScopeContractRequest(t *testing.T, router http.Handler, method, path, token, orgID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if orgID != "" {
		req.Header.Set("X-Org-Id", orgID)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func assertOrgScopeErrorCode(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()

	var env apiErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if env.Error.Code != want {
		t.Fatalf("expected error code %q, got %q", want, env.Error.Code)
	}
}
