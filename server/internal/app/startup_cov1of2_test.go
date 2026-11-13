package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/storage"
)

// cov1xStubDeps swaps the package-level startup seams for the duration of one
// test. Callers must not use t.Parallel.
func cov1xStubDeps(t *testing.T, pool *pgxpool.Pool, poolErr, roleErr, schemaErr error, store storage.ObjectStorage, storeErr error) {
	t.Helper()
	origOpen, origRole, origSchema, origClose, origStore := openDBPool, validateDBRole, validateDBSchema, closeDBPool, newObjectStorageClient
	t.Cleanup(func() {
		openDBPool, validateDBRole, validateDBSchema, closeDBPool, newObjectStorageClient = origOpen, origRole, origSchema, origClose, origStore
	})
	openDBPool = func(context.Context, string) (*pgxpool.Pool, error) { return pool, poolErr }
	validateDBRole = func(context.Context, *pgxpool.Pool) error { return roleErr }
	validateDBSchema = func(context.Context, *pgxpool.Pool) error { return schemaErr }
	closeDBPool = func(*pgxpool.Pool) {}
	newObjectStorageClient = func(storage.Config) (storage.ObjectStorage, error) { return store, storeErr }
}

func cov1xStrictConfig() Config {
	return Config{
		AppEnv:      "production",
		DatabaseURL: "postgres://fixture.invalid/aquify",
		Storage:     StorageConfig{Endpoint: "storage.example.test", AccessKey: "ak", SecretKey: "sk", Bucket: "b"},
	}
}

func TestCov1xIsDevelopmentEnv(t *testing.T) {
	t.Parallel()
	for env, want := range map[string]bool{
		"development": true, " Local ": true, "TEST": true,
		"staging": false, "production": false, "": false, "prod": false,
	} {
		if got := IsDevelopmentEnv(env); got != want {
			t.Errorf("IsDevelopmentEnv(%q) = %v, want %v", env, got, want)
		}
	}
}

func TestCov1xValidateStartupDependenciesStrictSuccessLogs(t *testing.T) {
	t.Setenv("INTEGRATIONS_ENCRYPTION_KEY", "fixture-key")
	cov1xStubDeps(t, &pgxpool.Pool{}, nil, nil, nil, fakeObjectStorage{}, nil)
	// A nil logger falls back to slog.Default; the call must still succeed.
	if err := ValidateStartupDependencies(context.Background(), cov1xStrictConfig(), nil); err != nil {
		t.Fatalf("expected strict validation to pass with every dependency healthy: %v", err)
	}
}

func TestCov1xValidateStartupDependenciesAggregatesAllFailures(t *testing.T) {
	t.Setenv("INTEGRATIONS_ENCRYPTION_KEY", "")
	cov1xStubDeps(t, nil, nil, nil, nil, fakeObjectStorage{}, nil)
	cfg := cov1xStrictConfig()
	cfg.DatabaseURL = ""
	cfg.Storage = StorageConfig{}
	err := ValidateStartupDependencies(context.Background(), cfg, slog.Default())
	if err == nil {
		t.Fatal("expected failure")
	}
	for _, want := range []string{"DATABASE_URL must be set", "object storage: STORAGE_ENDPOINT", "INTEGRATIONS_ENCRYPTION_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
}

func TestCov1xValidateDatabaseStartupDependencyFailures(t *testing.T) {
	cfg := cov1xStrictConfig()
	pool := &pgxpool.Pool{}

	cov1xStubDeps(t, nil, nil, nil, nil, nil, nil)
	if err := validateDatabaseStartupDependency(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "did not produce a connection pool") {
		t.Fatalf("nil pool: %v", err)
	}

	roleErr := errors.New("role is superuser")
	cov1xStubDeps(t, pool, nil, roleErr, nil, nil, nil)
	if err := validateDatabaseStartupDependency(context.Background(), cfg); !errors.Is(err, roleErr) {
		t.Fatalf("role failure should be returned unchanged, got %v", err)
	}

	schemaErr := errors.New("missing column")
	cov1xStubDeps(t, pool, nil, nil, schemaErr, nil, nil)
	err := validateDatabaseStartupDependency(context.Background(), cfg)
	if !errors.Is(err, schemaErr) || !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("schema failure: %v", err)
	}

	cov1xStubDeps(t, pool, nil, nil, nil, nil, nil)
	if err := validateDatabaseStartupDependency(context.Background(), cfg); err != nil {
		t.Fatalf("healthy database: %v", err)
	}
}

func TestCov1xValidateStorageRejectsStubFallback(t *testing.T) {
	cov1xStubDeps(t, nil, nil, nil, nil, storage.NewStubStorage("http://stub"), nil)
	err := validateStorageStartupDependency(cov1xStrictConfig())
	if err == nil || !strings.Contains(err.Error(), "stub storage fallback is not allowed") {
		t.Fatalf("expected stub rejection, got %v", err)
	}
}

type cov1xErrRow struct{ err error }

func (r cov1xErrRow) Scan(...any) error { return r.err }

type cov1xRowQuerier struct{ row pgx.Row }

func (q cov1xRowQuerier) QueryRow(context.Context, string, ...any) pgx.Row { return q.row }

func TestCov1xValidateDatabaseRoleInspectionFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection reset")
	err := validateDatabaseRole(context.Background(), cov1xRowQuerier{row: cov1xErrRow{err: cause}})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "could not inspect the connection role") {
		t.Fatalf("got %v", err)
	}
}

func TestCov1xValidateOIDCLogoutConfig(t *testing.T) {
	t.Parallel()
	if err := ValidateOIDCLogoutConfig(Config{AppEnv: "production"}, nil); err != nil {
		t.Fatalf("no browser OIDC means nothing to validate: %v", err)
	}
	cfg := Config{AppEnv: "development"}
	cfg.Auth.OIDC = OIDCConfig{AuthorizeURL: "https://idp.example.test/auth", RedirectURL: "https://app.example.test/cb", ClientID: "c", StateSigningKey: strings.Repeat("k", 32)}
	// Development with missing logout URLs only warns, using the default logger when none is given.
	if err := ValidateOIDCLogoutConfig(cfg, nil); err != nil {
		t.Fatalf("development must only warn: %v", err)
	}
	cfg.AppEnv = "production"
	if err := ValidateOIDCLogoutConfig(cfg, nil); err == nil || !strings.Contains(err.Error(), "OIDC_END_SESSION_URL and OIDC_POST_LOGOUT_REDIRECT_URL are missing") {
		t.Fatalf("production must fail naming both vars, got %v", err)
	}
}

func TestCov1xValidateKeycloakAdminConfigComplete(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Auth.OIDC = OIDCConfig{AuthorizeURL: "https://idp.example.test/auth", RedirectURL: "https://app.example.test/cb", ClientID: "c", StateSigningKey: strings.Repeat("k", 32)}
	cfg.Keycloak = KeycloakAdminConfig{AdminURL: "https://kc.example.test", Realm: "r", ClientID: "c", ClientSecret: "s"}
	if err := ValidateKeycloakAdminConfig(cfg); err != nil {
		t.Fatalf("complete admin config rejected: %v", err)
	}
	cfg.Keycloak.ClientSecret = ""
	if err := ValidateKeycloakAdminConfig(cfg); err == nil || !strings.Contains(err.Error(), "KEYCLOAK_ADMIN_CLIENT_SECRET is missing") {
		t.Fatalf("expected single missing var, got %v", err)
	}
}

func TestCov1xRedisFallbackNoErrorAndNilLogger(t *testing.T) {
	t.Parallel()
	cfg := Config{AppEnv: "test", Worker: WorkerConfig{RedisURL: "redis://x"}}
	out, fell, err := MaybeApplyDevelopmentRedisFallback(cfg, nil, nil)
	if err != nil || fell || out.Worker.RedisURL != "redis://x" {
		t.Fatalf("nil error must leave config untouched: %+v %v %v", out, fell, err)
	}
	out, fell, err = MaybeApplyDevelopmentRedisFallback(cfg, nil, errors.New("refused"))
	if err != nil || !fell || out.Worker.RedisURL != "" {
		t.Fatalf("nil logger fallback: %+v %v %v", out, fell, err)
	}
}
