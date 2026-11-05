package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app/billing"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/payments"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
)

func connectRouter(t *testing.T, billingSvc *billing.Service, stub *stripelib.StubClient, accounts *payments.Service) http.Handler {
	t.Helper()
	required := true
	cfg := Config{
		Auth:           AuthConfig{EnableDevStubTokens: true},
		Stripe:         StripeConfig{WebhookSecret: "whsec_test", Currency: "usd", ReconciliationAlertThresholdCents: 100, ConnectRequired: &required},
		AllowedOrigins: []string{"https://example.com"},
	}
	return NewRouterWithServices(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Billing: billingSvc, Stripe: stub, Payments: accounts})
}

func issueOpenInvoice(t *testing.T, svc *billing.Service, org, number string, cents int) *billing.Invoice {
	t.Helper()
	p := &appmiddleware.Principal{UserID: "actor-1", OrgID: org, Roles: []string{"admin"}}
	inv, err := svc.CreateInvoice(org, "actor-1", p, billing.CreateInvoiceInput{HouseholdID: "hh-" + org, InvoiceNumber: number})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddInvoiceItem(org, "actor-1", p, inv.ID, billing.AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.IssueInvoice(org, "actor-1", p, inv.ID); err != nil {
		t.Fatal(err)
	}
	return inv
}

func stripeCall(t *testing.T, router http.Handler, token, org, method, path, idemKey string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", org)
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

// Tenancy tracker item 5.1: with Connect required, a club cannot take payments
// until its own Stripe account finishes onboarding, and then every payment is
// created on that club's account.
func TestPaymentsRequireTheClubsOwnConnectedAccount(t *testing.T) {
	stub := stripelib.NewStubClient()
	stub.AccountsNotReady = true
	svc := billing.NewService()
	accounts := payments.NewService()
	router := connectRouter(t, svc, stub, accounts)
	inv := issueOpenInvoice(t, svc, "org-a", "INV-A", 5000)
	admin := issueSessionToken(t, router, "org-a", []string{"admin"}, true)
	pay := func(key string) (int, map[string]any) {
		return stripeCall(t, router, admin, "org-a", "POST", "/api/v1/stripe/payment-intents", key,
			map[string]any{"invoice_id": inv.ID, "amount_cents": 5000})
	}

	if code, out := pay("k1"); code != http.StatusConflict {
		t.Fatalf("no account yet must refuse payments: %d %v", code, out)
	}
	code, out := stripeCall(t, router, admin, "org-a", "POST", "/api/v1/billing/payment-account/onboarding", "",
		map[string]any{"return_url": "https://example.com/billing"})
	if code != http.StatusOK || out["url"] == "" {
		t.Fatalf("onboarding: %d %v", code, out)
	}
	if code, _ := pay("k2"); code != http.StatusConflict {
		t.Fatalf("an unfinished account must still refuse payments: %d", code)
	}
	acct, err := accounts.Get(context.Background(), "org-a")
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := stripeCall(t, router, admin, "org-a", "POST", "/api/v1/billing/payment-account/onboarding", "", map[string]any{"return_url": "https://evil.example/x"}); c != http.StatusBadRequest {
		t.Fatalf("return_url outside the trusted origins: %d", c)
	}

	stub.MarkAccountReady(acct.StripeAccountID)
	code, view := stripeCall(t, router, admin, "org-a", "GET", "/api/v1/billing/payment-account", "", nil)
	if code != http.StatusOK || view["can_take_payments"] != true {
		t.Fatalf("status refresh from Stripe: %d %v", code, view)
	}
	if code, out := pay("k3"); code != http.StatusCreated {
		t.Fatalf("ready account: %d %v", code, out)
	}
	if got := stub.IntentAccounts(); len(got) != 1 || got[0] != acct.StripeAccountID {
		t.Fatalf("the intent must be created on the club's connected account: %v", got)
	}

	// Another club's admin cannot use this club's account or invoice.
	otherAdmin := issueSessionToken(t, router, "org-b", []string{"admin"}, true)
	if code, _ := stripeCall(t, router, otherAdmin, "org-b", "POST", "/api/v1/stripe/payment-intents", "k4", map[string]any{"invoice_id": inv.ID, "amount_cents": 5000}); code == http.StatusCreated {
		t.Fatal("org B must not create a payment for org A's invoice")
	}
	if _, err := accounts.Get(context.Background(), "org-b"); err == nil {
		t.Fatal("org B must not inherit org A's account")
	}
	parent := issueSessionToken(t, router, "org-a", []string{"parent"}, false)
	if code, _ := stripeCall(t, router, parent, "org-a", "GET", "/api/v1/billing/payment-account", "", nil); code != http.StatusForbidden {
		t.Fatalf("only admins manage the payment account: %d", code)
	}
}

func connectWebhook(router http.Handler, eventID, account string, obj map[string]any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(map[string]any{
		"id": eventID, "type": "payment_intent.succeeded", "created": time.Now().Unix(), "account": account,
		"data": map[string]any{"object": obj},
	})
	return postWebhook(router, payload, "")
}

// A Connect event is attributed by the connected account, never by metadata
// alone: metadata that names another club's org is refused, and the money is
// credited to the account's own org with the account recorded for refunds.
func TestConnectWebhookIsAttributedByAccountNotMetadata(t *testing.T) {
	stub := stripelib.NewStubClient()
	svc := billing.NewService()
	accounts := payments.NewService()
	ctx := context.Background()
	_, _ = accounts.Create(ctx, "org-a", "acct_A")
	_, _ = accounts.Create(ctx, "org-b", "acct_B")
	router := connectRouter(t, svc, stub, accounts)
	invA := issueOpenInvoice(t, svc, "org-a", "INV-A", 5000)
	invB := issueOpenInvoice(t, svc, "org-b", "INV-B", 5000)

	// Account A's event claims org B's invoice in metadata: must not pay it.
	rr := connectWebhook(router, "evt_forged", "acct_A", map[string]any{
		"id": "pi_forged", "amount": float64(5000), "metadata": map[string]any{"org_id": "org-b", "invoice_id": invB.ID}})
	if rr.Code != http.StatusOK {
		t.Fatalf("forged event is acknowledged (permanent failure): %d %s", rr.Code, rr.Body.String())
	}
	pb := &appmiddleware.Principal{UserID: "x", OrgID: "org-b", Roles: []string{"admin"}}
	if list, _ := svc.ListPayments("org-b", pb, invB.ID); len(list) != 0 {
		t.Fatalf("org B's invoice must stay unpaid, got %d payments", len(list))
	}

	// An event from an account that belongs to no club is also refused.
	connectWebhook(router, "evt_orphan", "acct_UNKNOWN", map[string]any{
		"id": "pi_orphan", "amount": float64(5000), "metadata": map[string]any{"org_id": "org-a", "invoice_id": invA.ID}})
	pa := &appmiddleware.Principal{UserID: "x", OrgID: "org-a", Roles: []string{"admin"}}
	if list, _ := svc.ListPayments("org-a", pa, invA.ID); len(list) != 0 {
		t.Fatal("an unattributable connect event must not pay anything")
	}

	// The genuine event pays A's invoice and remembers the account.
	rr = connectWebhook(router, "evt_ok", "acct_A", map[string]any{
		"id": "pi_ok", "amount": float64(5000), "metadata": map[string]any{"org_id": "org-a", "invoice_id": invA.ID}})
	if rr.Code != http.StatusOK {
		t.Fatalf("genuine event: %d", rr.Code)
	}
	list, _ := svc.ListPayments("org-a", pa, invA.ID)
	if len(list) != 1 || list[0].StripeAccountID != "acct_A" {
		t.Fatalf("payment must be recorded with its account: %+v", list)
	}
}

func TestReconciliationUsesTheClubsConnectedAccount(t *testing.T) {
	stub := stripelib.NewStubClient()
	svc := billing.NewService()
	accounts := payments.NewService()
	_, _ = accounts.Create(context.Background(), "org-a", "acct_A")
	router := connectRouter(t, svc, stub, accounts)
	svc.SetStripeAccountResolver(func(org string) (string, error) {
		if org == "org-a" {
			return "acct_A", nil
		}
		return "", nil
	})
	inv := issueOpenInvoice(t, svc, "org-a", "INV-R", 4000)
	connectWebhook(router, "evt_r", "acct_A", map[string]any{"id": "pi_r", "amount": float64(4000), "metadata": map[string]any{"org_id": "org-a", "invoice_id": inv.ID}})
	stub.SetAccountBalance("acct_A", 4000)
	stub.BalanceCents = 999999 // the platform balance must be irrelevant to this club

	admin := issueSessionToken(t, router, "org-a", []string{"admin"}, true)
	code, out := stripeCall(t, router, admin, "org-a", "GET", "/api/v1/stripe/reconciliation", "", nil)
	rep, _ := out["reconciliation"].(map[string]any)
	if code != http.StatusOK || rep == nil || rep["has_mismatch"] != false || rep["stripe_reported_cents"] != float64(4000) {
		t.Fatalf("reconciliation must compare against the connected account's balance: %d %v", code, out)
	}
}

func TestReconciliationNeverFallsBackToPlatformAccount(t *testing.T) {
	for name, resolverErr := range map[string]error{
		"lookup error":  errors.New("db down"),
		"not connected": billing.ErrPaymentAccountNotConnected,
	} {
		t.Run(name, func(t *testing.T) {
			stub := stripelib.NewStubClient()
			svc := billing.NewService()
			accounts := payments.NewService()
			router := connectRouter(t, svc, stub, accounts)
			svc.SetStripeAccountResolver(func(string) (string, error) { return "", resolverErr })
			stub.BalanceCents = 999999 // platform funds must never reach a club admin
			admin := issueSessionToken(t, router, "org-a", []string{"admin"}, true)
			code, out := stripeCall(t, router, admin, "org-a", "GET", "/api/v1/stripe/reconciliation", "", nil)
			if code == http.StatusOK {
				t.Fatalf("reconciliation must not succeed without a resolved account: %v", out)
			}
			if _, leaked := out["reconciliation"]; leaked {
				t.Fatalf("platform balance leaked: %v", out)
			}
			if errors.Is(resolverErr, billing.ErrPaymentAccountNotConnected) && code != http.StatusConflict {
				t.Fatalf("want 409 payments_not_enabled, got %d %v", code, out)
			}
		})
	}
}

// A refund is issued against the account the payment was taken on, so a club that
// connects Stripe later can still refund its earlier platform-account payments.
func TestRefundGoesToTheAccountThePaymentWasTakenOn(t *testing.T) {
	stub := stripelib.NewStubClient()
	svc := billing.NewService()
	accounts := payments.NewService()
	_, _ = accounts.Create(context.Background(), "org-a", "acct_A")
	router := connectRouter(t, svc, stub, accounts)
	invConnect := issueOpenInvoice(t, svc, "org-a", "INV-C", 5000)
	invLegacy := issueOpenInvoice(t, svc, "org-a", "INV-L", 3000)
	connectWebhook(router, "evt_c", "acct_A", map[string]any{"id": "pi_connect", "amount": float64(5000), "metadata": map[string]any{"org_id": "org-a", "invoice_id": invConnect.ID}})
	// A payment from before the club connected: recorded on the platform account.
	p := &appmiddleware.Principal{UserID: "actor-1", OrgID: "org-a", Roles: []string{"admin"}, MFAAuthenticated: true}
	legacy, _, err := svc.HandleStripePaymentIntentSucceeded("org-a", "pi_legacy", invLegacy.ID, 3000, "stripe-webhook")
	if err != nil {
		t.Fatal(err)
	}

	admin := issueSessionToken(t, router, "org-a", []string{"admin"}, true)
	connectPays, _ := svc.ListPayments("org-a", p, invConnect.ID)
	refund := func(invID, payID string) int {
		code, _ := stripeCall(t, router, admin, "org-a", "POST", "/api/v1/invoices/"+invID+"/refunds", "",
			map[string]any{"payment_id": payID, "amount_cents": 1000, "reason": "test"})
		return code
	}
	if c := refund(invConnect.ID, connectPays[0].ID); c != http.StatusCreated {
		t.Fatalf("connect refund: %d", c)
	}
	if c := refund(invLegacy.ID, legacy.ID); c != http.StatusCreated {
		t.Fatalf("legacy refund: %d", c)
	}
	got := stub.CreatedRefunds()
	if len(got) != 2 || got[0].StripeAccount != "acct_A" || got[1].StripeAccount != "" {
		t.Fatalf("refund accounts: %+v", got)
	}
}

// Each club bills in its own currency (tracker 5.4); a club with none set uses the deployment default.
func TestPaymentUsesTheClubsOwnCurrency(t *testing.T) {
	stub := stripelib.NewStubClient()
	svc := billing.NewService()
	router := newStripeRouter(t, svc, stub) // deployment default: usd, Connect not required
	inv := issueOpenInvoice(t, svc, "org-cad", "INV-CAD", 5000)
	inv2 := issueOpenInvoice(t, svc, "org-usd", "INV-USD", 5000)
	adminCAD := issueSessionToken(t, router, "org-cad", []string{"admin"}, true)
	adminUSD := issueSessionToken(t, router, "org-usd", []string{"admin"}, true)
	if code, out := stripeCall(t, router, adminCAD, "org-cad", "PUT", "/api/v1/org-settings", "", map[string]any{"currency": "CAD"}); code != http.StatusOK {
		t.Fatalf("set currency: %d %v", code, out)
	}
	pay := func(tok, org, invID, key string) string {
		code, out := stripeCall(t, router, tok, org, "POST", "/api/v1/stripe/payment-intents", key, map[string]any{"invoice_id": invID, "amount_cents": 5000})
		if code != http.StatusCreated {
			t.Fatalf("payment intent: %d %v", code, out)
		}
		pi, _ := out["payment_intent"].(map[string]any)
		cur, _ := pi["currency"].(string)
		return cur
	}
	if got := pay(adminCAD, "org-cad", inv.ID, "k-cad"); got != "cad" {
		t.Fatalf("a club with its own currency bills in it, got %q", got)
	}
	if got := pay(adminUSD, "org-usd", inv2.ID, "k-usd"); got != "usd" {
		t.Fatalf("another club keeps the default, got %q", got)
	}
	if code, _ := stripeCall(t, router, adminCAD, "org-cad", "PUT", "/api/v1/org-settings", "", map[string]any{"currency": "not-a-code"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid currency must be rejected: %d", code)
	}
}
