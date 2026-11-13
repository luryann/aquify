package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/authn"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/postmark"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
)

func TestCov1xUnavailableStripeClientFailsEveryOperation(t *testing.T) {
	t.Parallel()
	boom := errors.New("stripe is not configured")
	c := unavailableStripeClient{err: boom}
	ctx := context.Background()

	if err := c.Ready(ctx); !errors.Is(err, boom) {
		t.Errorf("Ready: %v", err)
	}
	if r, err := c.CreatePaymentIntent(ctx, stripelib.PaymentIntentParams{}); r != nil || !errors.Is(err, boom) {
		t.Errorf("CreatePaymentIntent: %v %v", r, err)
	}
	if r, err := c.CreateCheckoutSession(ctx, stripelib.CheckoutSessionParams{}); r != nil || !errors.Is(err, boom) {
		t.Errorf("CreateCheckoutSession: %v %v", r, err)
	}
	if r, err := c.ParseWebhookEvent([]byte("{}"), "sig", "secret"); r != nil || !errors.Is(err, boom) {
		t.Errorf("ParseWebhookEvent: %v %v", r, err)
	}
	if n, err := c.FetchBalanceCents(ctx, "acct_x"); n != 0 || !errors.Is(err, boom) {
		t.Errorf("FetchBalanceCents: %v %v", n, err)
	}
	if r, err := c.CreateConnectedAccount(ctx, stripelib.ConnectedAccountParams{}); r != nil || !errors.Is(err, boom) {
		t.Errorf("CreateConnectedAccount: %v %v", r, err)
	}
	if u, err := c.CreateAccountLink(ctx, "a", "r", "f"); u != "" || !errors.Is(err, boom) {
		t.Errorf("CreateAccountLink: %q %v", u, err)
	}
	if r, err := c.GetConnectedAccount(ctx, "acct_x"); r != nil || !errors.Is(err, boom) {
		t.Errorf("GetConnectedAccount: %v %v", r, err)
	}
	if r, err := c.CreateRefund(ctx, stripelib.RefundParams{}); r != nil || !errors.Is(err, boom) {
		t.Errorf("CreateRefund: %v %v", r, err)
	}
}

func TestCov1xUnavailablePostmarkClientFailsEveryOperation(t *testing.T) {
	t.Parallel()
	boom := errors.New("postmark is not configured")
	c := unavailablePostmarkClient{err: boom}
	ctx := context.Background()
	if err := c.Ready(ctx); !errors.Is(err, boom) {
		t.Errorf("Ready: %v", err)
	}
	if _, err := c.SendEmail(ctx, postmark.SendEmailRequest{}); !errors.Is(err, boom) {
		t.Errorf("SendEmail: %v", err)
	}
	if res, err := c.SendBatch(ctx, []postmark.SendEmailRequest{{}}); res != nil || !errors.Is(err, boom) {
		t.Errorf("SendBatch: %v %v", res, err)
	}
}

func TestCov1xSeedLocalDevKeycloakStub(t *testing.T) {
	t.Parallel()
	seedLocalDevKeycloakStub(nil) // must be a no-op, not a panic
	stub := keycloak.NewStubClient()
	seedLocalDevKeycloakStub(stub)
	seeded := authn.LocalDevUsers()
	users, err := stub.ListUsers(context.Background(), authn.LocalDevOrgID, "", 100, 0)
	if err != nil || len(users) != len(seeded) {
		t.Fatalf("expected %d seeded users, got %v %v", len(seeded), users, err)
	}
	roles, err := stub.GetUserRealmRoles(context.Background(), seeded[0].UserID)
	if err != nil || len(roles) != len(seeded[0].Roles) {
		t.Fatalf("expected roles %v assigned, got %v %v", seeded[0].Roles, roles, err)
	}
}

func TestCov1xValidateProviderConfigRequiresWebhookSecretsAndIssuer(t *testing.T) {
	t.Parallel()
	cfg := validProductionProviderConfig()
	cfg.Stripe.WebhookSecret = ""
	if err := ValidateProviderConfig(cfg); err == nil || !strings.Contains(err.Error(), "STRIPE_WEBHOOK_SECRET must be set") {
		t.Fatalf("missing stripe webhook secret: %v", err)
	}
	cfg = validProductionProviderConfig()
	cfg.Auth.OIDC.ExpectedIssuer = ""
	if err := ValidateProviderConfig(cfg); err == nil || !strings.Contains(err.Error(), "OIDC_EXPECTED_ISSUER must be set in non-development environments so ID token issuer is validated") {
		t.Fatalf("missing issuer: %v", err)
	}
	// With dev stub tokens on and no browser login, there are no OIDC problems to report.
	devStub := Config{AppEnv: "staging"}
	devStub.Auth.EnableDevStubTokens = true
	if problems := oidcProviderProblems(devStub); problems != nil {
		t.Fatalf("unexpected problems %v", problems)
	}
}

func TestCov1xValidateWorkerConfig(t *testing.T) {
	t.Parallel()
	if err := ValidateWorkerConfig(Config{AppEnv: "development"}); err != nil {
		t.Fatalf("development needs no Redis: %v", err)
	}
	if err := ValidateWorkerConfig(Config{AppEnv: "production"}); err == nil || !strings.Contains(err.Error(), "REDIS_URL must be configured") {
		t.Fatalf("production without Redis: %v", err)
	}
	cfg := Config{AppEnv: "production", Worker: WorkerConfig{RedisURL: "redis://r"}}
	if err := ValidateWorkerConfig(cfg); err == nil || !strings.Contains(err.Error(), "DATABASE_URL (tenant registry) or WORKER_SCHEDULED_ORG_IDS") {
		t.Fatalf("production without a tenant source: %v", err)
	}
	cfg.DatabaseURL = "postgres://x"
	if err := ValidateWorkerConfig(cfg); err != nil {
		t.Fatalf("complete worker config rejected: %v", err)
	}
	cfg.Worker.ReconcileInterval = "soon"
	if err := ValidateWorkerConfig(cfg); err == nil || !strings.Contains(err.Error(), "WORKER_RECONCILE_INTERVAL") {
		t.Fatalf("bad interval: %v", err)
	}
}

// cov1xFakeKeycloak serves the two endpoints startup validation touches: the
// client-credentials token and realm role lookups.
func cov1xFakeKeycloak(t *testing.T, roleStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-token", "expires_in": 300})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/admin/realms/") && strings.Contains(r.URL.Path, "/roles/"):
			if r.Header.Get("Authorization") != "Bearer fixture-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(roleStatus)
			if roleStatus == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]any{"name": "role"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCov1xValidateKeycloakStartup(t *testing.T) {
	t.Parallel()
	if err := ValidateKeycloakStartup(Config{}, nil); err != nil {
		t.Fatalf("no realm configured means nothing to check: %v", err)
	}

	jwks := func(status int) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		t.Cleanup(srv.Close)
		return srv
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	cfg := Config{Keycloak: KeycloakAdminConfig{Realm: "aquify"}}
	cfg.Auth.OIDC.JWKSURL = jwks(http.StatusOK).URL
	if err := ValidateKeycloakStartup(cfg, logger); err != nil {
		t.Fatalf("reachable JWKS: %v", err)
	}
	if !strings.Contains(logs.String(), "JWKS endpoint reachable") {
		t.Fatalf("missing success log: %q", logs.String())
	}
	if err := ValidateKeycloakStartup(cfg, nil); err != nil { // nil logger falls back to the default
		t.Fatalf("nil logger: %v", err)
	}

	cfg.Auth.OIDC.JWKSURL = jwks(http.StatusServiceUnavailable).URL
	if err := ValidateKeycloakStartup(cfg, logger); err == nil || !strings.Contains(err.Error(), "returned HTTP 503") {
		t.Fatalf("non-200 JWKS: %v", err)
	}

	down := httptest.NewServer(http.NotFoundHandler())
	cfg.Auth.OIDC.JWKSURL = down.URL
	down.Close()
	if err := ValidateKeycloakStartup(cfg, logger); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("unreachable JWKS: %v", err)
	}

	cfg.Auth.OIDC.JWKSURL = "http://bad host/\x7f"
	if err := ValidateKeycloakStartup(cfg, logger); err == nil || !strings.Contains(err.Error(), "invalid JWKS URL") {
		t.Fatalf("malformed JWKS URL: %v", err)
	}
}

func TestCov1xValidateKeycloakStartupAdminRoles(t *testing.T) {
	t.Parallel()
	adminCfg := func(url string) Config {
		return Config{Keycloak: KeycloakAdminConfig{AdminURL: url, Realm: "aquify", ClientID: "svc", ClientSecret: "secret"}}
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	if err := ValidateKeycloakStartup(adminCfg(cov1xFakeKeycloak(t, http.StatusOK).URL), logger); err != nil {
		t.Fatalf("roles present: %v", err)
	}
	if !strings.Contains(logs.String(), "realm roles verified") {
		t.Fatalf("missing verification log: %q", logs.String())
	}

	// A role lookup failure only warns while the stub is an acceptable fallback...
	failing := cov1xFakeKeycloak(t, http.StatusInternalServerError)
	logs.Reset()
	if err := ValidateKeycloakStartup(adminCfg(failing.URL), logger); err != nil {
		t.Fatalf("a degraded Keycloak must only warn in dev-style config: %v", err)
	}
	if !strings.Contains(logs.String(), "could not verify/create realm roles") {
		t.Fatalf("missing warning: %q", logs.String())
	}

	// ...but is fatal once real browser login is configured without stub tokens.
	strict := adminCfg(failing.URL)
	strict.Auth.OIDC = OIDCConfig{AuthorizeURL: "https://idp.test/auth", RedirectURL: "https://app.test/cb", ClientID: "c", StateSigningKey: strings.Repeat("k", 32)}
	if err := ValidateKeycloakStartup(strict, logger); err == nil || !strings.Contains(err.Error(), "admin client could not verify/create realm roles") {
		t.Fatalf("strict config must fail: %v", err)
	}

	// Whitespace-only admin URL passes the emptiness check but not client construction.
	if err := ValidateKeycloakStartup(adminCfg("   "), logger); err == nil || !strings.Contains(err.Error(), "failed to create admin client") {
		t.Fatalf("invalid admin config: %v", err)
	}
}
