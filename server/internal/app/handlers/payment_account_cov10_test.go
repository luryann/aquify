package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/luryann/aquify/server/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/payments"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
)

// cov10gPayStripe overrides only the connected-account calls the payment
// account handlers use; every other Client method would panic on the nil
// embedded interface, which proves the handlers touch nothing else.
type cov10gPayStripe struct {
	stripelib.Client
	createID    string
	createErr   error
	onCreate    func()
	linkURL     string
	linkErr     error
	live        *stripelib.ConnectedAccount
	liveErr     error
	createCalls int
}

func (c *cov10gPayStripe) CreateConnectedAccount(_ context.Context, _ stripelib.ConnectedAccountParams) (*stripelib.ConnectedAccount, error) {
	c.createCalls++
	if c.onCreate != nil {
		c.onCreate()
	}
	if c.createErr != nil {
		return nil, c.createErr
	}
	return &stripelib.ConnectedAccount{ID: c.createID}, nil
}

func (c *cov10gPayStripe) CreateAccountLink(_ context.Context, _, _, _ string) (string, error) {
	return c.linkURL, c.linkErr
}

func (c *cov10gPayStripe) GetConnectedAccount(_ context.Context, _ string) (*stripelib.ConnectedAccount, error) {
	return c.live, c.liveErr
}

func cov10gPayMount(t *testing.T, accounts *payments.Service, client stripelib.Client, required bool) (http.Handler, *audit.Service) {
	t.Helper()
	aud := audit.NewService()
	h := NewPaymentAccountHandlers(accounts, client, aud, []string{"https://club.example"}, required)
	return g2Router(t, g2Tokens, func(r chi.Router) { h.RegisterRoutes(r) }), aud
}

func cov10gPayDecode(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return out
}

func TestCov10gPaymentAccountGetStates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("not connected reports requirement", func(t *testing.T) {
		t.Parallel()
		router, _ := cov10gPayMount(t, payments.NewService(), &cov10gPayStripe{}, true)
		rr := g2Do(router, http.MethodGet, "/billing/payment-account/", "admin", "")
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		got := cov10gPayDecode(t, rr)
		if got["connected"] != false || got["required"] != true || got["can_take_payments"] != false {
			t.Fatalf("unexpected view %v", got)
		}
	})

	t.Run("coach is denied", func(t *testing.T) {
		t.Parallel()
		router, _ := cov10gPayMount(t, payments.NewService(), &cov10gPayStripe{}, true)
		if rr := g2Do(router, http.MethodGet, "/billing/payment-account/", "coach", ""); rr.Code != http.StatusForbidden {
			t.Fatalf("coach status %d, want 403", rr.Code)
		}
	})

	t.Run("live flags sync back into the store", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		if _, err := accounts.Create(ctx, engOrg, "acct_sync"); err != nil {
			t.Fatal(err)
		}
		client := &cov10gPayStripe{live: &stripelib.ConnectedAccount{ID: "acct_sync", ChargesEnabled: true, PayoutsEnabled: true, DetailsSubmitted: true}}
		router, _ := cov10gPayMount(t, accounts, client, true)
		got := cov10gPayDecode(t, g2Do(router, http.MethodGet, "/billing/payment-account/", "admin", ""))
		if got["charges_enabled"] != true || got["can_take_payments"] != true || got["stripe_account_id"] != "acct_sync" {
			t.Fatalf("view not refreshed: %v", got)
		}
		stored, err := accounts.Get(ctx, engOrg)
		if err != nil || !stored.ChargesEnabled || !stored.PayoutsEnabled || !stored.DetailsSubmitted {
			t.Fatalf("store not updated: %+v err=%v", stored, err)
		}
	})

	t.Run("unchanged live flags keep the stored account", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		created, err := accounts.Create(ctx, engOrg, "acct_same")
		if err != nil {
			t.Fatal(err)
		}
		client := &cov10gPayStripe{live: &stripelib.ConnectedAccount{ID: "acct_same"}}
		router, _ := cov10gPayMount(t, accounts, client, false)
		got := cov10gPayDecode(t, g2Do(router, http.MethodGet, "/billing/payment-account/", "admin", ""))
		if got["connected"] != true || got["charges_enabled"] != false || got["can_take_payments"] != false {
			t.Fatalf("unexpected view %v", got)
		}
		stored, _ := accounts.Get(ctx, engOrg)
		if !stored.UpdatedAt.Equal(created.UpdatedAt) {
			t.Fatalf("UpdatedAt changed although nothing differed")
		}
	})

	t.Run("stripe lookup failure falls back to stored flags", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		if _, err := accounts.Create(ctx, engOrg, "acct_down"); err != nil {
			t.Fatal(err)
		}
		if _, err := accounts.UpdateStatus(ctx, "acct_down", true, false, true); err != nil {
			t.Fatal(err)
		}
		client := &cov10gPayStripe{liveErr: errors.New("stripe unreachable")}
		router, _ := cov10gPayMount(t, accounts, client, true)
		got := cov10gPayDecode(t, g2Do(router, http.MethodGet, "/billing/payment-account/", "admin", ""))
		if got["charges_enabled"] != true || got["payouts_enabled"] != false || got["details_submitted"] != true {
			t.Fatalf("stored flags not served on stripe failure: %v", got)
		}
	})

	t.Run("store failure is a 500", func(t *testing.T) {
		t.Parallel()
		router, _ := cov10gPayMount(t, payments.NewServiceWithPool(testutil.ClosedPool(t)), &cov10gPayStripe{}, true)
		rr := g2Do(router, http.MethodGet, "/billing/payment-account/", "admin", "")
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "Could not load the payment account.") {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
	})
}

func TestCov10gPaymentAccountRefreshKeepsAccountWhenStoreCannotUpdate(t *testing.T) {
	t.Parallel()
	client := &cov10gPayStripe{live: &stripelib.ConnectedAccount{ID: "acct_ghost", ChargesEnabled: true}}
	h := NewPaymentAccountHandlers(payments.NewService(), client, audit.NewService(), nil, true)
	ghost := &payments.Account{OrgID: engOrg, StripeAccountID: "acct_ghost"}
	got := h.refresh(httptest.NewRequest(http.MethodGet, "/", nil), ghost)
	if got != ghost || got.ChargesEnabled {
		t.Fatalf("refresh must return the original account when UpdateStatus fails, got %+v", got)
	}

	// A nil live account is also ignored.
	client.live = nil
	if got := h.refresh(httptest.NewRequest(http.MethodGet, "/", nil), ghost); got != ghost {
		t.Fatalf("nil live account must keep stored account")
	}
}

func TestCov10gPaymentAccountOnboard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const body = `{"return_url":"https://club.example/done"}`

	t.Run("unknown field rejected", func(t *testing.T) {
		t.Parallel()
		router, _ := cov10gPayMount(t, payments.NewService(), &cov10gPayStripe{}, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", `{"return_url":"https://club.example/x","bogus":1}`)
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid_request") {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("untrusted refresh url rejected", func(t *testing.T) {
		t.Parallel()
		client := &cov10gPayStripe{}
		router, _ := cov10gPayMount(t, payments.NewService(), client, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", `{"return_url":"https://club.example/x","refresh_url":"https://evil.example/y"}`)
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid_return_url") {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if client.createCalls != 0 {
			t.Fatalf("no Stripe account may be created for a rejected request")
		}
	})

	t.Run("creates account, audits and returns link", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		client := &cov10gPayStripe{createID: "acct_new", linkURL: "https://connect.stripe.test/onboard"}
		router, aud := cov10gPayMount(t, accounts, client, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		got := cov10gPayDecode(t, rr)
		if got["url"] != "https://connect.stripe.test/onboard" {
			t.Fatalf("url = %v", got["url"])
		}
		if acc, _ := got["account"].(map[string]any); acc["stripe_account_id"] != "acct_new" || acc["connected"] != true {
			t.Fatalf("account view = %v", got["account"])
		}
		if stored, err := accounts.Get(ctx, engOrg); err != nil || stored.StripeAccountID != "acct_new" {
			t.Fatalf("stored account = %+v err=%v", stored, err)
		}
		events, err := aud.Query(audit.Filter{OrgID: engOrg})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range events {
			if e.Action == audit.ActionBillingPaymentAccountCreate {
				found = true
			}
		}
		if !found {
			t.Fatalf("payment account creation was not audited")
		}
	})

	t.Run("stripe account creation failure is a 502", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		client := &cov10gPayStripe{createErr: errors.New("stripe down")}
		router, _ := cov10gPayMount(t, accounts, client, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", body)
		if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), "Could not create the Stripe account.") {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if _, err := accounts.Get(ctx, engOrg); !errors.Is(err, payments.ErrNotFound) {
			t.Fatalf("no account may be stored after a Stripe failure, err=%v", err)
		}
	})

	t.Run("lost race reuses the winner's account", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		client := &cov10gPayStripe{createID: "acct_loser", linkURL: "https://connect.stripe.test/link"}
		client.onCreate = func() {
			if _, err := accounts.Create(ctx, engOrg, "acct_winner"); err != nil {
				t.Errorf("seed winner: %v", err)
			}
		}
		router, _ := cov10gPayMount(t, accounts, client, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		acc, _ := cov10gPayDecode(t, rr)["account"].(map[string]any)
		if acc["stripe_account_id"] != "acct_winner" {
			t.Fatalf("expected the winner's account, got %v", acc)
		}
	})

	t.Run("stripe account owned by another org is a 500", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		if _, err := accounts.Create(ctx, "org-other", "acct_taken"); err != nil {
			t.Fatal(err)
		}
		client := &cov10gPayStripe{createID: "acct_taken"}
		router, _ := cov10gPayMount(t, accounts, client, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", body)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "Could not set up the payment account.") {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("existing account with link failure is a 502", func(t *testing.T) {
		t.Parallel()
		accounts := payments.NewService()
		if _, err := accounts.Create(ctx, engOrg, "acct_exist"); err != nil {
			t.Fatal(err)
		}
		client := &cov10gPayStripe{linkErr: errors.New("link refused")}
		router, _ := cov10gPayMount(t, accounts, client, true)
		rr := g2Do(router, http.MethodPost, "/billing/payment-account/onboarding", "admin", body)
		if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), "Could not create the onboarding link.") {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if client.createCalls != 0 {
			t.Fatalf("an existing account must not be recreated")
		}
	})
}
