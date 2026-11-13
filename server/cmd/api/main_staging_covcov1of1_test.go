package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// cov1xStagingDatabase builds a throwaway database holding the real schema plus
// a restricted (non-superuser, non-BYPASSRLS) login role, which is exactly what
// startup validation demands outside development. It returns the role's DSN.
func cov1xStagingDatabase(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if base == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(b)
	dbName, role, pass := "cov1x_stg_"+suffix, "cov1x_stg_role_"+suffix, "pw-"+suffix

	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Skipf("cannot create scratch database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
		_, _ = admin.Exec(context.Background(), "DROP ROLE IF EXISTS "+role)
	})
	if _, err := admin.Exec(ctx, "CREATE ROLE "+role+" LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD '"+pass+"'"); err != nil {
		t.Skipf("cannot create scratch role: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	owner := *u
	owner.Path = "/" + dbName
	cov1xUseMigrations(t, os.DirFS(filepath.Join("..", "..", "..", "db", "migrations")))
	var out, errb strings.Builder
	env := envOf(map[string]string{"APP_ENV": "production", "MIGRATE_DATABASE_URL": owner.String()})
	if code := runMigrate(ctx, []string{"up"}, env, &out, &errb); code != 0 {
		t.Fatalf("migrate up exit=%d stderr=%s", code, errb.String())
	}

	scratch, err := pgx.Connect(ctx, owner.String())
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close(ctx)
	for _, stmt := range []string{
		"GRANT USAGE ON SCHEMA public TO " + role,
		"GRANT ALL ON ALL TABLES IN SCHEMA public TO " + role,
		"GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO " + role,
	} {
		if _, err := scratch.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	app := *u
	app.User = url.UserPassword(role, pass)
	app.Path = "/" + dbName
	return app.String()
}

// cov1xStagingEnv configures a complete, valid staging deployment against
// fabricated provider credentials and local stand-ins for Keycloak.
func cov1xStagingEnv(t *testing.T, dsn string) {
	t.Helper()
	cov1xBaseEnv(t)

	// One TLS server plays the OIDC JWKS endpoint and the Keycloak admin API.
	kc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"access_token":"cov1x-token","expires_in":300,"token_type":"Bearer"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"cov1x-role","name":"cov1x"}`))
	}))
	t.Cleanup(kc.Close)
	// main's JWKS probe uses http.DefaultClient; trust the fixture's certificate.
	orig := http.DefaultTransport
	http.DefaultTransport = kc.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = orig })

	// Minimal S3 stand-in: the bucket probe only needs a 200 for the bucket.
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`))
		}
	}))
	t.Cleanup(s3.Close)

	for k, v := range map[string]string{
		"APP_ENV":                       "staging",
		"DATABASE_URL":                  dsn,
		"REDIS_URL":                     "redis://127.0.0.1:1/0",
		"API_ALLOWED_ORIGINS":           "https://admin.cov1x.example",
		"AUTH_ENABLE_DEV_STUB_TOKENS":   "false",
		"AUTH_ALLOW_DEV_SESSION_ISSUE":  "false",
		"SESSION_COOKIE_SECURE":         "true",
		"STORAGE_ENDPOINT":              strings.TrimPrefix(s3.URL, "http://"),
		"STORAGE_ACCESS_KEY":            "cov1x-access",
		"STORAGE_SECRET_KEY":            "cov1x-secret-key",
		"STORAGE_BUCKET":                "cov1x-bucket",
		"INTEGRATIONS_ENCRYPTION_KEY":   "cov1x-integrations-key-0123456789ab",
		"STRIPE_SECRET_KEY":             "sk_test_cov1x",
		"STRIPE_WEBHOOK_SECRET":         "whsec_cov1x",
		"STRIPE_CONNECT_WEBHOOK_SECRET": "whsec_cov1xconnect",
		"POSTMARK_SERVER_TOKEN":         "cov1x-postmark",
		"POSTMARK_FROM_EMAIL":           "noreply@cov1x.example",
		"POSTMARK_WEBHOOK_BASIC_USER":   "cov1x",
		"POSTMARK_WEBHOOK_BASIC_PASS":   "cov1x-pass",
		"OIDC_AUTHORIZE_URL":            "https://id.cov1x.example/auth",
		"OIDC_TOKEN_URL":                "https://id.cov1x.example/token",
		"OIDC_JWKS_URL":                 kc.URL + "/certs",
		"OIDC_END_SESSION_URL":          "https://id.cov1x.example/logout",
		"OIDC_REDIRECT_URL":             "https://admin.cov1x.example/callback",
		"OIDC_POST_LOGOUT_REDIRECT_URL": "https://admin.cov1x.example/login",
		"OIDC_CLIENT_ID":                "cov1x-client",
		"OIDC_CLIENT_SECRET":            "cov1x-client-secret",
		"OIDC_EXPECTED_ISSUER":          "https://id.cov1x.example/realms/cov1x",
		"OIDC_STATE_SIGNING_KEY":        "cov1x-state-signing-key-0123456789abcdef",
		"KEYCLOAK_ADMIN_URL":            kc.URL + "/admin/realms/cov1x",
		"KEYCLOAK_REALM":                "cov1x",
		"KEYCLOAK_ADMIN_CLIENT_ID":      "cov1x-admin",
		"KEYCLOAK_ADMIN_CLIENT_SECRET":  "cov1x-admin-secret",
	} {
		t.Setenv(k, v)
	}
}

func TestCov1xMainRefusesToStartWhenDurableQueueIsUnavailableInStaging(t *testing.T) {
	dsn := cov1xStagingDatabase(t)
	cov1xStagingEnv(t, dsn)
	code, out, _ := cov1xRunMain(t, false)
	if code != 1 || !strings.Contains(out, "worker backend init failed") {
		t.Fatalf("staging must fail closed on an unreachable Redis; exit=%d out=%s", code, out)
	}
	if strings.Contains(out, "api listening") {
		t.Fatalf("API must not start without its queue:\n%s", out)
	}
}
