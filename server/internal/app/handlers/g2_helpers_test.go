package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/go-chi/chi/v5"
)

func g2Router(t *testing.T, tokens tokenResolver, register func(chi.Router)) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

func g2Do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", engOrg)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

var g2Tokens = tokenResolver{
	"admin": {UserID: "u-admin", OrgID: engOrg, Roles: []string{"admin"}},
	"coach": {UserID: "u-coach", OrgID: engOrg, Roles: []string{"coach"}},
}
