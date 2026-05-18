package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	appopenapi "github.com/dareaquatics/aquify/server/internal/openapi"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

var newObjectStorageClient = func(cfg storage.Config) (storage.ObjectStorage, error) {
	return storage.NewClient(cfg)
}

var validatePackagedOpenAPI = appopenapi.Validate

func isExplicitDevelopmentEnv(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "development", "local", "test":
		return true
	default:
		return false
	}
}

func requiresStrictRuntimeDependencies(appEnv string) bool {
	return !isExplicitDevelopmentEnv(appEnv)
}

// ValidateStartupDependencies verifies that non-development runtimes have the
// required external dependencies available before the API begins serving.
func ValidateStartupDependencies(ctx context.Context, cfg Config, logger *slog.Logger) error {
	var errs []string
	if err := validateOpenAPIStartupDependency(); err != nil {
		errs = append(errs, err.Error())
	}
	if !requiresStrictRuntimeDependencies(cfg.AppEnv) {
		if len(errs) == 0 {
			return nil
		}
		return fmt.Errorf("startup dependency validation failed: %s", strings.Join(errs, "; "))
	}
	if logger == nil {
		logger = slog.Default()
	}

	if err := validateDatabaseStartupDependency(ctx, cfg); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateStorageStartupDependency(cfg); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) == 0 {
		logger.Info("startup dependencies validated", "app_env", cfg.AppEnv)
		return nil
	}
	return fmt.Errorf("startup dependency validation failed: %s", strings.Join(errs, "; "))
}

func validateOpenAPIStartupDependency() error {
	if err := validatePackagedOpenAPI(); err != nil {
		return fmt.Errorf("openapi: packaged spec validation failed: %w", err)
	}
	return nil
}

// ValidateOIDCLogoutConfig ensures browser OIDC login setups also declare the
// upstream logout endpoints needed to terminate the provider SSO session.
func ValidateOIDCLogoutConfig(cfg Config, logger *slog.Logger) error {
	if !oidcBrowserLoginConfigured(cfg.Auth.OIDC) {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	var missing []string
	if strings.TrimSpace(cfg.Auth.OIDC.EndSessionURL) == "" {
		missing = append(missing, "OIDC_END_SESSION_URL")
	}
	if strings.TrimSpace(cfg.Auth.OIDC.PostLogoutRedirectURL) == "" {
		missing = append(missing, "OIDC_POST_LOGOUT_REDIRECT_URL")
	}
	if len(missing) == 0 {
		return nil
	}

	if isExplicitDevelopmentEnv(cfg.AppEnv) {
		logger.Warn(
			"oidc logout config incomplete; logout will only clear Aquify cookies and may reopen the previous Keycloak account",
			"app_env", cfg.AppEnv,
			"missing_env", missing,
		)
		return nil
	}

	return fmt.Errorf(
		"oidc logout config incomplete: browser OIDC login is configured but %s %s missing; set both so logout can terminate the upstream Keycloak session instead of reopening the previous account",
		strings.Join(missing, " and "),
		pluralize(len(missing), "is", "are"),
	)
}

// ValidateKeycloakAdminConfig ensures that real browser OIDC configurations
// do not silently fall back to the in-memory Keycloak stub for admin user
// management flows.
func ValidateKeycloakAdminConfig(cfg Config) error {
	if !requiresKeycloakAdminConfig(cfg) {
		return nil
	}

	var missing []string
	if strings.TrimSpace(cfg.Keycloak.AdminURL) == "" {
		missing = append(missing, "KEYCLOAK_ADMIN_URL")
	}
	if strings.TrimSpace(cfg.Keycloak.Realm) == "" {
		missing = append(missing, "KEYCLOAK_REALM")
	}
	if strings.TrimSpace(cfg.Keycloak.ClientID) == "" {
		missing = append(missing, "KEYCLOAK_ADMIN_CLIENT_ID")
	}
	if strings.TrimSpace(cfg.Keycloak.ClientSecret) == "" {
		missing = append(missing, "KEYCLOAK_ADMIN_CLIENT_SECRET")
	}
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf(
		"keycloak admin config incomplete: browser OIDC login is configured with AUTH_ENABLE_DEV_STUB_TOKENS=false, but %s %s missing; set them so /users and other Keycloak-backed user management endpoints talk to the real Keycloak Admin API instead of a local stub",
		strings.Join(missing, " and "),
		pluralize(len(missing), "is", "are"),
	)
}

// MaybeApplyDevelopmentRedisFallback clears the configured Redis URL when the
// durable queue backend fails to initialize in an explicit local/test runtime.
// This lets the API continue with the existing in-memory worker fallback while
// keeping strict Redis startup behavior in non-development environments.
func MaybeApplyDevelopmentRedisFallback(cfg Config, logger *slog.Logger, err error) (Config, bool, error) {
	if err == nil {
		return cfg, false, nil
	}
	if !isExplicitDevelopmentEnv(cfg.AppEnv) || strings.TrimSpace(cfg.Worker.RedisURL) == "" {
		return cfg, false, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("worker backend init failed; falling back to in-memory queue for local development", "error", err)
	cfg.Worker.RedisURL = ""
	return cfg, true, nil
}

func validateDatabaseStartupDependency(ctx context.Context, cfg Config) error {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("database: DATABASE_URL must be set in non-development environments")
	}

	pool, err := openDBPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: failed to connect using DATABASE_URL: %w", err)
	}
	if pool == nil {
		return errors.New("database: DATABASE_URL did not produce a connection pool")
	}
	defer closeDBPool(pool)

	if err := validateDBSchema(ctx, pool); err != nil {
		return fmt.Errorf("database: schema validation failed: %w", err)
	}
	return nil
}

func validateStorageStartupDependency(cfg Config) error {
	storageCfg := storage.Config{
		Endpoint:  cfg.Storage.Endpoint,
		AccessKey: cfg.Storage.AccessKey,
		SecretKey: cfg.Storage.SecretKey,
		Bucket:    cfg.Storage.Bucket,
		UseSSL:    cfg.Storage.UseSSL,
		Region:    cfg.Storage.Region,
	}

	if !storageCfg.Enabled() {
		return errors.New("object storage: STORAGE_ENDPOINT, STORAGE_ACCESS_KEY, STORAGE_SECRET_KEY, and STORAGE_BUCKET must be set in non-development environments")
	}

	client, err := newObjectStorageClient(storageCfg)
	if err != nil {
		return fmt.Errorf("object storage: initialization failed: %w", err)
	}
	if _, ok := client.(*storage.StubStorage); ok {
		return errors.New("object storage: stub storage fallback is not allowed in non-development environments")
	}
	return nil
}

func oidcBrowserLoginConfigured(cfg OIDCConfig) bool {
	return strings.TrimSpace(cfg.AuthorizeURL) != "" &&
		strings.TrimSpace(cfg.RedirectURL) != "" &&
		strings.TrimSpace(cfg.ClientID) != "" &&
		strings.TrimSpace(cfg.StateSigningKey) != ""
}

func requiresKeycloakAdminConfig(cfg Config) bool {
	return oidcBrowserLoginConfigured(cfg.Auth.OIDC) && !cfg.Auth.EnableDevStubTokens
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
