package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/luryann/aquify/server/internal/app/orgs"
)

// Tenancy tracker items 2.3 / 7.1: an org that is suspended (or otherwise not
// active) is refused on every protected route, immediately, and is restored on
// resume. Other orgs are unaffected.
func TestSuspendedOrgIsRefusedOnProtectedRoutes(t *testing.T) {
	registry := orgs.NewService()
	registry.SetStatusCacheTTL(0)
	ctx := context.Background()
	mk := func(name string) *orgs.Org {
		o, err := registry.Create(ctx, orgs.CreateInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.SetStatus(ctx, o.ID, orgs.StatusActive); err != nil {
			t.Fatal(err)
		}
		return o
	}
	a, b := mk("A"), mk("B")
	router := NewRouterWithServices(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Orgs: registry})
	tokA := issueSessionToken(t, router, a.ID, []string{"admin"}, true)
	tokB := issueSessionToken(t, router, b.ID, []string{"admin"}, true)

	get := func(tok, org string) int {
		return doOrgScopeContractRequest(t, router, http.MethodGet, "/api/v1/members/households", tok, org, nil).Code
	}
	if got := get(tokA, a.ID); got != http.StatusOK {
		t.Fatalf("active org A: %d", got)
	}
	if _, err := registry.SetStatus(ctx, a.ID, orgs.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	rr := doOrgScopeContractRequest(t, router, http.MethodGet, "/api/v1/members/households", tokA, a.ID, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("suspended org A must be refused, got %d", rr.Code)
	}
	assertOrgScopeErrorCode(t, rr, "org_unavailable")
	if got := get(tokB, b.ID); got != http.StatusOK {
		t.Fatalf("org B must be unaffected by A's suspension: %d", got)
	}
	if _, err := registry.SetStatus(ctx, a.ID, orgs.StatusActive); err != nil {
		t.Fatal(err)
	}
	if got := get(tokA, a.ID); got != http.StatusOK {
		t.Fatalf("resumed org A: %d", got)
	}
}
