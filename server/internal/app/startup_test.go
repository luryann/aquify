package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeObjectStorage struct{}

func (fakeObjectStorage) PresignedPutURL(string, time.Duration) (string, error) { return "", nil }
func (fakeObjectStorage) PresignedGetURL(string, time.Duration) (string, error) { return "", nil }
func (fakeObjectStorage) PutObject(context.Context, string, string, []byte) error {
	return nil
}
func (fakeObjectStorage) GetObject(context.Context, string) ([]byte, string, error) {
	return nil, "", nil
}
func (fakeObjectStorage) Exists(context.Context, string) (bool, error) { return false, nil }
func (fakeObjectStorage) Delete(string) error                          { return nil }

func TestValidateStartupDependenciesAllowsDevelopmentFallback(t *testing.T) {
	t.Parallel()

	if err := ValidateStartupDependencies(context.Background(), Config{AppEnv: "development"}, slog.Default()); err != nil {
		t.Fatalf("expected development startup validation to allow fallbacks, got %v", err)
	}
}

func TestMaybeApplyDevelopmentRedisFallbackClearsRedisURLInDevelopment(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg := Config{
		AppEnv: "development",
		Worker: WorkerConfig{
			RedisURL: "redis://localhost:6379/0",
		},
	}

	updated, fellBack, err := MaybeApplyDevelopmentRedisFallback(cfg, logger, errors.New("dial tcp [::1]:6379: connect: connection refused"))
	if err != nil {
		t.Fatalf("expected development fallback, got %v", err)
	}
	if !fellBack {
		t.Fatal("expected development fallback to be applied")
	}
	if updated.Worker.RedisURL != "" {
		t.Fatalf("expected Redis URL to be cleared for local fallback, got %q", updated.Worker.RedisURL)
	}
	if !strings.Contains(logs.String(), "falling back to in-memory queue") {
		t.Fatalf("expected fallback warning log, got %q", logs.String())
	}
}

func TestMaybeApplyDevelopmentRedisFallbackKeepsStrictRedisOutsideDevelopment(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv: "production",
		Worker: WorkerConfig{
			RedisURL: "redis://localhost:6379/0",
		},
	}
	originalErr := errors.New("dial tcp [::1]:6379: connect: connection refused")

	updated, fellBack, err := MaybeApplyDevelopmentRedisFallback(cfg, slog.Default(), originalErr)
	if !errors.Is(err, originalErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if fellBack {
		t.Fatal("expected strict runtime to reject Redis fallback")
	}
	if updated.Worker.RedisURL == "" {
		t.Fatal("expected Redis URL to remain configured outside development")
	}
}

func TestValidateStartupDependenciesRequiresDatabaseAndStorageOutsideDev(t *testing.T) {
	t.Parallel()

	err := ValidateStartupDependencies(context.Background(), Config{AppEnv: "production"}, slog.Default())
	if err == nil {
		t.Fatal("expected strict startup validation to fail without runtime dependencies")
	}
	if got := err.Error(); !strings.Contains(got, "DATABASE_URL must be set") || !strings.Contains(got, "STORAGE_ENDPOINT") {
		t.Fatalf("expected actionable missing dependency errors, got %q", got)
	}
}

func TestValidateStartupDependenciesRequiresDatabaseAndStorageWhenAppEnvUnset(t *testing.T) {
	t.Parallel()

	err := ValidateStartupDependencies(context.Background(), Config{}, slog.Default())
	if err == nil {
		t.Fatal("expected strict startup validation to fail when APP_ENV is unset")
	}
	if got := err.Error(); !strings.Contains(got, "DATABASE_URL must be set") || !strings.Contains(got, "STORAGE_ENDPOINT") {
		t.Fatalf("expected actionable missing dependency errors, got %q", got)
	}
}

func TestValidateStartupDependenciesRequiresDatabaseAndStorageWhenAppEnvInvalid(t *testing.T) {
	t.Parallel()

	err := ValidateStartupDependencies(context.Background(), Config{AppEnv: "prodution"}, slog.Default())
	if err == nil {
		t.Fatal("expected strict startup validation to fail when APP_ENV is invalid")
	}
	if got := err.Error(); !strings.Contains(got, "DATABASE_URL must be set") || !strings.Contains(got, "STORAGE_ENDPOINT") {
		t.Fatalf("expected actionable missing dependency errors, got %q", got)
	}
}

func TestValidateStartupDependenciesFailsWhenDatabaseConnectionFails(t *testing.T) {
	originalOpenDBPool := openDBPool
	originalNewObjectStorageClient := newObjectStorageClient
	t.Cleanup(func() {
		openDBPool = originalOpenDBPool
		newObjectStorageClient = originalNewObjectStorageClient
	})

	openDBPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return nil, errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")
	}
	newObjectStorageClient = func(cfg storage.Config) (storage.ObjectStorage, error) {
		return fakeObjectStorage{}, nil
	}

	err := ValidateStartupDependencies(context.Background(), Config{
		AppEnv:      "production",
		DatabaseURL: "postgres://example.invalid/aquify",
		Storage: StorageConfig{
			Endpoint:  "storage.example.com",
			AccessKey: "access",
			SecretKey: "secret",
			Bucket:    "aquify",
		},
	}, slog.Default())
	if err == nil {
		t.Fatal("expected strict startup validation to fail when DB connection fails")
	}
	if got := err.Error(); !strings.Contains(got, "failed to connect using DATABASE_URL") {
		t.Fatalf("expected actionable DB connection error, got %q", got)
	}
}

func TestValidateStartupDependenciesFailsWhenStorageInitializationFails(t *testing.T) {
	originalOpenDBPool := openDBPool
	originalValidateDBSchema := validateDBSchema
	originalCloseDBPool := closeDBPool
	originalNewObjectStorageClient := newObjectStorageClient
	t.Cleanup(func() {
		openDBPool = originalOpenDBPool
		validateDBSchema = originalValidateDBSchema
		closeDBPool = originalCloseDBPool
		newObjectStorageClient = originalNewObjectStorageClient
	})

	fakePool := &pgxpool.Pool{}
	openDBPool = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return fakePool, nil
	}
	validateDBSchema = func(_ context.Context, pool *pgxpool.Pool) error {
		if pool != fakePool {
			t.Fatalf("validateDBSchema received unexpected pool pointer")
		}
		return nil
	}
	closeDBPool = func(_ *pgxpool.Pool) {}
	newObjectStorageClient = func(cfg storage.Config) (storage.ObjectStorage, error) {
		return nil, errors.New("lookup storage.example.com: no such host")
	}

	err := ValidateStartupDependencies(context.Background(), Config{
		AppEnv:      "production",
		DatabaseURL: "postgres://example.invalid/aquify",
		Storage: StorageConfig{
			Endpoint:  "storage.example.com",
			AccessKey: "access",
			SecretKey: "secret",
			Bucket:    "aquify",
		},
	}, slog.Default())
	if err == nil {
		t.Fatal("expected strict startup validation to fail when object storage init fails")
	}
	if got := err.Error(); !strings.Contains(got, "object storage: initialization failed") {
		t.Fatalf("expected actionable storage init error, got %q", got)
	}
}

func TestValidateStartupDependenciesAllowsExplicitLocalAndTestFallbacks(t *testing.T) {
	t.Parallel()

	for _, appEnv := range []string{"local", "test"} {
		appEnv := appEnv
		t.Run(appEnv, func(t *testing.T) {
			t.Parallel()

			if err := ValidateStartupDependencies(context.Background(), Config{AppEnv: appEnv}, slog.Default()); err != nil {
				t.Fatalf("expected %s startup validation to allow fallbacks, got %v", appEnv, err)
			}
		})
	}
}

func TestValidateOIDCLogoutConfigWarnsInDevelopmentWhenLogoutVarsMissing(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	err := ValidateOIDCLogoutConfig(Config{
		AppEnv: "development",
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				AuthorizeURL:    "http://localhost:8180/realms/aquify/protocol/openid-connect/auth",
				RedirectURL:     "http://localhost:3000/auth/callback",
				ClientID:        "aquify-id",
				StateSigningKey: "state-signing-key",
			},
		},
	}, logger)
	if err != nil {
		t.Fatalf("expected development OIDC logout validation to warn only, got %v", err)
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "OIDC_END_SESSION_URL") || !strings.Contains(logOutput, "OIDC_POST_LOGOUT_REDIRECT_URL") {
		t.Fatalf("expected warning to list missing OIDC logout env vars, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "previous Keycloak account") {
		t.Fatalf("expected warning to explain cross-account risk, got %q", logOutput)
	}
}

func TestValidateOIDCLogoutConfigFailsOutsideDevelopmentWhenLogoutVarsMissing(t *testing.T) {
	t.Parallel()

	err := ValidateOIDCLogoutConfig(Config{
		AppEnv: "production",
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				AuthorizeURL:          "https://auth.example.com/realms/dare/protocol/openid-connect/auth",
				RedirectURL:           "https://app.example.com/auth/callback",
				ClientID:              "aquify-id",
				StateSigningKey:       "state-signing-key",
				PostLogoutRedirectURL: "https://app.example.com/login",
			},
		},
	}, slog.Default())
	if err == nil {
		t.Fatal("expected strict OIDC logout validation to fail when end-session URL is missing")
	}
	if got := err.Error(); !strings.Contains(got, "OIDC_END_SESSION_URL") || !strings.Contains(got, "previous account") {
		t.Fatalf("expected actionable OIDC logout validation error, got %q", got)
	}
}

func TestValidateOIDCLogoutConfigAllowsFullyConfiguredBrowserOIDC(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	err := ValidateOIDCLogoutConfig(Config{
		AppEnv: "production",
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				AuthorizeURL:          "https://auth.example.com/realms/dare/protocol/openid-connect/auth",
				EndSessionURL:         "https://auth.example.com/realms/dare/protocol/openid-connect/logout",
				RedirectURL:           "https://app.example.com/auth/callback",
				PostLogoutRedirectURL: "https://app.example.com/login",
				ClientID:              "aquify-id",
				StateSigningKey:       "state-signing-key",
			},
		},
	}, logger)
	if err != nil {
		t.Fatalf("expected fully configured browser OIDC logout validation to pass, got %v", err)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no warning logs for fully configured browser OIDC logout, got %q", logs.String())
	}
}
