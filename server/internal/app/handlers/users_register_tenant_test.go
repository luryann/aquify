package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/keycloak"
)

type fakeClubs map[string]string

func (f fakeClubs) FindBySlug(slug string) (*orgsettings.OrgSettings, error) {
	if id, ok := f[slug]; ok {
		return &orgsettings.OrgSettings{OrgID: id, PublicSlug: slug}, nil
	}
	return nil, nil
}

func registerTenantRequest(t *testing.T, allowBody bool, body string) (*httptest.ResponseRecorder, *keycloak.StubClient) {
	t.Helper()
	kc := keycloak.NewStubClient()
	h := NewUserHandlers(kc, nil)
	h.SetClubResolver(fakeClubs{"aqua": "org-aqua"}, allowBody)
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr, kc
}

func TestSelfRegisterResolvesOrgFromSlugNotBody(t *testing.T) {
	rr, kc := registerTenantRequest(t, false, `{"first_name":"A","last_name":"B","email":"a@example.com","club_slug":"AQUA"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	users, _ := kc.ListUsers(t.Context(), "org-aqua", "", 50, 0)
	if len(users) != 1 {
		t.Fatalf("expected the user to land in org-aqua, got %d", len(users))
	}
}

func TestSelfRegisterRejectsBodyOrgIDOutsideDev(t *testing.T) {
	rr, _ := registerTenantRequest(t, false, `{"first_name":"A","last_name":"B","email":"a@example.com","org_id":"org-victim"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSelfRegisterSlugMismatchingOrgIDRejected(t *testing.T) {
	rr, _ := registerTenantRequest(t, false, `{"first_name":"A","last_name":"B","email":"a@example.com","club_slug":"aqua","org_id":"org-victim"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSelfRegisterUnknownSlug404(t *testing.T) {
	rr, _ := registerTenantRequest(t, true, `{"first_name":"A","last_name":"B","email":"a@example.com","club_slug":"nope"}`)
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "club_not_found") {
		t.Fatalf("want 404 club_not_found, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSelfRegisterBodyOrgIDAllowedInDev(t *testing.T) {
	rr, _ := registerTenantRequest(t, true, `{"first_name":"A","last_name":"B","email":"a@example.com","org_id":"org-1"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
}
