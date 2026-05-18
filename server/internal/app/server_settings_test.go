package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeSettingsEndpointReturnsSanitizedConfig(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv: "development",
		Auth: AuthConfig{
			EnableDevStubTokens:  true,
			AllowDevSessionIssue: true,
			OIDC: OIDCConfig{
				ClientID:       "dare-admin",
				RedirectURL:    "http://localhost:3000/auth/callback",
				ExpectedIssuer: "https://issuer.example.com",
				ClientSecret:   "super-secret",
			},
		},
		Storage: StorageConfig{
			Endpoint: "localhost:9000",
			Bucket:   "aquify",
			UseSSL:   false,
			Region:   "us-east-1",
			AccessKey: "hidden",
			SecretKey: "hidden-secret",
		},
		Stripe: StripeConfig{
			Currency:                          "usd",
			ReconciliationAlertThresholdCents: 125,
			SecretKey:                         "sk_test_hidden",
		},
	}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/runtime", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var env struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	auth, _ := env.Settings["auth"].(map[string]any)
	storage, _ := env.Settings["storage"].(map[string]any)
	stripe, _ := env.Settings["stripe"].(map[string]any)
	if auth["oidc_client_id"] != "dare-admin" {
		t.Fatalf("expected oidc_client_id dare-admin, got %#v", auth["oidc_client_id"])
	}
	if _, ok := auth["client_secret"]; ok {
		t.Fatal("did not expect client_secret in runtime settings payload")
	}
	if storage["endpoint"] != "localhost:9000" {
		t.Fatalf("expected storage endpoint localhost:9000, got %#v", storage["endpoint"])
	}
	if _, ok := storage["secret_key"]; ok {
		t.Fatal("did not expect storage secret key in runtime settings payload")
	}
	if stripe["currency"] != "usd" {
		t.Fatalf("expected stripe currency usd, got %#v", stripe["currency"])
	}
	if _, ok := stripe["secret_key"]; ok {
		t.Fatal("did not expect stripe secret key in runtime settings payload")
	}
}
