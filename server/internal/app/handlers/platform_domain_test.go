package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgs"
)

func TestPlatformDomainVerificationRequiresTheDNSChallenge(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	o, _ := reg.Create(ctx, orgs.CreateInput{Name: "A"})
	other, _ := reg.Create(ctx, orgs.CreateInput{Name: "B"})
	h := NewPlatformHandlers(reg, nil, nil, "platform")
	dns := map[string][]string{}
	h.LookupTXT = func(_ context.Context, name string) ([]string, error) {
		if v, ok := dns[name]; ok {
			return v, nil
		}
		return nil, errors.New("no such host")
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := appmiddleware.ContextWithPrincipal(req.Context(), &appmiddleware.Principal{UserID: "op", OrgID: "platform", Roles: []string{PlatformRole}, MFAAuthenticated: true})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	h.RegisterRoutes(r)
	do := func(path, body string) int {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rr, req)
		return rr.Code
	}
	if code := do("/platform/orgs/"+o.ID+"/domains", `{"domain":"swim.example.org"}`); code != http.StatusCreated {
		t.Fatalf("add: %d", code)
	}
	verify := "/platform/orgs/" + o.ID + "/domains/swim.example.org/verify"
	if code := do(verify, ""); code != http.StatusConflict {
		t.Fatalf("no TXT record: %d", code)
	}
	// A TXT record carrying another org's id must not verify this org's claim.
	dns[DomainChallengeName("swim.example.org")] = []string{DomainChallengeValue(other.ID)}
	if code := do(verify, ""); code != http.StatusConflict {
		t.Fatalf("wrong org in TXT record: %d", code)
	}
	dns[DomainChallengeName("swim.example.org")] = []string{"spf1 junk", DomainChallengeValue(o.ID)}
	if code := do(verify, ""); code != http.StatusOK {
		t.Fatalf("correct TXT record: %d", code)
	}
	if got, err := reg.ResolveDomain(ctx, "swim.example.org"); err != nil || got.ID != o.ID {
		t.Fatalf("domain should now resolve: %+v %v", got, err)
	}
}
