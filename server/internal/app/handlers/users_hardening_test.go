package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/keycloak"
)

type fakeRevoker struct{ ids []string }

func (f *fakeRevoker) RevokeUserSessions(_ context.Context, id string) (int, error) {
	f.ids = append(f.ids, id)
	return 1, nil
}

func TestCreateUserRejectsMalformedFields(t *testing.T) {
	_, router := usersG2Setup(t)
	for _, body := range []string{
		`{"email":"not-an-email","password":"longenough1"}`,
		`{"email":"a@example.com","first_name":"` + strings.Repeat("x", 101) + `","password":"longenough1"}`,
	} {
		rec := userLinkPost(router, "/users", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400 for %s, got %d %s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestDisableAndDeleteRevokeSessions(t *testing.T) {
	kc, router := usersG2Setup(t)
	_ = kc
	rev := &fakeRevoker{}
	h := NewUserHandlers(kc, nil)
	h.SetSessionRevoker(rev)
	router = newUserLinkRouter(h)
	if rec := usersG2Req(router, http.MethodPost, "/users/p-0/disable"); rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodDelete, "/users/p-1", nil)
	req.Header.Set("Authorization", "Bearer t")
	req.Header.Set("X-Org-Id", "org-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusForbidden {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if len(rev.ids) < 1 || rev.ids[0] != "p-0" {
		t.Fatalf("disable did not revoke sessions: %v", rev.ids)
	}
	if rec.Code == http.StatusOK && (len(rev.ids) != 2 || rev.ids[1] != "p-1") {
		t.Fatalf("delete did not revoke sessions: %v", rev.ids)
	}
}

func TestWriteKeycloakAdminErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{&keycloak.StatusError{Op: "create user", Status: 400, Body: "x"}, http.StatusBadRequest},
		{&keycloak.StatusError{Op: "list users", Status: 502, Body: "x"}, http.StatusServiceUnavailable},
		{errors.Join(errors.New("dial"), &keycloak.StatusError{Status: 503}), http.StatusServiceUnavailable},
		{errors.New("some internal bug"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeKeycloakAdminError(rec, httptest.NewRequest("GET", "/", nil), c.err, "fail")
		if rec.Code != c.want {
			t.Fatalf("%v: want %d got %d", c.err, c.want, rec.Code)
		}
	}
}
