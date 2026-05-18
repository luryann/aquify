package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIAddr       = ":8080"
	defaultAllowedOrigin = "http://localhost:3000,http://127.0.0.1:3000"
)

type Config struct {
	Addr           string
	AllowedOrigins []string
	AppEnv         string
	DatabaseURL    string
	Auth           AuthConfig
	Storage        StorageConfig
	Stripe         StripeConfig
	Postmark       PostmarkConfig
	Compliance     ComplianceConfig
	Worker         WorkerConfig
	Security       SecurityConfig
	Telemetry      TelemetryConfig
	Keycloak       KeycloakAdminConfig
}

// KeycloakAdminConfig holds credentials for the Keycloak Admin REST API.
// Used by the keycloak.AdminClient to manage users and roles on behalf of admins.
type KeycloakAdminConfig struct {
	// AdminURL is the base URL of the Keycloak instance (e.g. "http://localhost:8180").
	AdminURL string
	// Realm is the target Keycloak realm name (e.g. "dare").
	Realm string
	// ClientID is the service-account OAuth2 client ID for admin operations.
	ClientID string `json:"-"`
	// ClientSecret is the service-account OAuth2 client secret.
	ClientSecret string `json:"-"`
}

// TelemetryConfig holds OpenTelemetry and Prometheus observability settings.
type TelemetryConfig struct {
	// ServiceName is the OTel service.name attribute. Default: "dare-api".
	ServiceName string
	// ServiceVersion is the deployed artifact version. Default: "dev".
	ServiceVersion string
	// OTLPEndpoint is the OTLP HTTP endpoint. When empty, a no-op tracer is used.
	OTLPEndpoint string
	// SampleRate is the OTel trace sampling rate (0.0–1.0). Default: 1.0.
	SampleRate float64
	// MetricsAddr is the address to serve the Prometheus /metrics endpoint.
	// Default: ":9090". Set to "" to disable.
	MetricsAddr string
}

// WorkerConfig holds background job worker settings.
type WorkerConfig struct {
	// RedisURL points at the durable Redis broker used by Asynq workers.
	RedisURL string
	// ConcurrencyPerQueue is the number of goroutines processing each queue.
	// Default: 2.
	ConcurrencyPerQueue int
	// ReconcileInterval is how often the nightly billing reconciliation runs.
	// Default: 24h. Set to 0 to disable.
	ReconcileInterval string
	// IntegrityInterval is how often the nightly integrity check runs.
	// Default: 24h. Set to 0 to disable.
	IntegrityInterval string
	// CleanupInterval is how often the expired-file purge job runs.
	// Default: 6h. Set to 0 to disable.
	CleanupInterval string
}

// ComplianceConfig holds retention and compliance job settings.
type ComplianceConfig struct {
	// AuditRetentionDays is how many days audit events are kept before archival/purge.
	// Default: 2555 (~7 years, suitable for financial-record retention laws).
	AuditRetentionDays int
	// ExportTTLDays is how many days a generated data-export download link remains valid.
	// Default: 30.
	ExportTTLDays int
	// DeletionReviewDays is how many days an admin has to review a deletion request.
	// Default: 30.
	DeletionReviewDays int
}

// PostmarkConfig holds Postmark email send settings.
type PostmarkConfig struct {
	// ServerToken is the Postmark server API token.
	// json:"-" prevents accidental serialisation of the live credential.
	ServerToken string `json:"-"`
	// FromName is the default sender display name.
	FromName string
	// FromEmail is the default sender address.
	FromEmail string
	// WebhookAuthHeaderName is an optional custom header name that Postmark will send.
	WebhookAuthHeaderName string
	// WebhookAuthHeaderValue is the expected value for WebhookAuthHeaderName.
	WebhookAuthHeaderValue string `json:"-"`
	// WebhookBasicUser is the optional Postmark webhook basic auth username.
	WebhookBasicUser string
	// WebhookBasicPass is the optional Postmark webhook basic auth password.
	WebhookBasicPass string `json:"-"`
}

// StripeConfig holds Stripe API and webhook settings.
type StripeConfig struct {
	// SecretKey is the Stripe API secret key (sk_live_... or sk_test_...).
	// json:"-" prevents accidental serialisation of the live credential.
	SecretKey string `json:"-"`
	// WebhookSecret is the endpoint signing secret (whsec_...).
	// json:"-" prevents accidental serialisation of the live credential.
	WebhookSecret string `json:"-"`
	// Currency is the default ISO 4217 currency code.
	Currency string
	// ReconciliationAlertThresholdCents is the ledger vs Stripe mismatch amount
	// above which the reconciliation endpoint sets alert_triggered=true.
	ReconciliationAlertThresholdCents int
}

type StorageConfig struct {
	Endpoint string
	// AccessKey is masked from JSON serialisation to avoid credential leaks.
	AccessKey string `json:"-"`
	// SecretKey is masked from JSON serialisation to avoid credential leaks.
	SecretKey string `json:"-"`
	Bucket    string
	UseSSL    bool
	Region    string
}

func (c StorageConfig) BaseURL() string {
	scheme := "http"
	if c.UseSSL {
		scheme = "https"
	}
	endpoint := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(c.Endpoint), "http://"), "https://")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	bucket := strings.TrimSpace(c.Bucket)
	if bucket == "" {
		bucket = "aquify"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, endpoint, bucket)
}

type AuthConfig struct {
	EnableDevStubTokens  bool
	AllowDevSessionIssue bool
	OIDC                 OIDCConfig
}

type SecurityConfig struct {
	CSRFHeaderName string
	RateLimit      RateLimitConfig
	Uploads        UploadSecurityConfig
	Impersonation  ImpersonationConfig
}

type RateLimitConfig struct {
	GlobalRequests int
	GlobalWindow   time.Duration
	AuthRequests   int
	AuthWindow     time.Duration
	// StripeRequests / StripeWindow apply a tighter per-IP limit on
	// cost-driving Stripe write endpoints (payment-intents, checkout-sessions).
	StripeRequests int
	StripeWindow   time.Duration
	// TrustedProxyCIDRs is the list of CIDR ranges whose requests may set
	// X-Forwarded-For. When empty, X-Forwarded-For is never trusted.
	TrustedProxyCIDRs []string
}

type UploadSecurityConfig struct {
	MaxBytes               int64
	AllowedMIMEs           []string
	RequireMalwareScanHook bool
}

type ImpersonationConfig struct {
	Enabled bool
}

type OIDCConfig struct {
	AuthorizeURL          string
	TokenURL              string
	JWKSURL               string
	EndSessionURL         string
	RedirectURL           string
	PostLogoutRedirectURL string
	ClientID              string
	ClientSecret          string
	ExpectedIssuer        string
	StateSigningKey       string
	HTTPTimeout           time.Duration
}

func LoadConfig() Config {
	// Default to empty (production-safe) when APP_ENV is not explicitly set.
	// Dev auth flags are only enabled when the environment is explicitly
	// "development", "local", or "test".
	appEnv := strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", "")))
	enableDevStubTokensDefault := isExplicitDevelopmentEnv(appEnv)
	return Config{
		Addr:           getEnv("API_ADDR", defaultAPIAddr),
		AllowedOrigins: splitCSV(getEnv("API_ALLOWED_ORIGINS", defaultAllowedOrigin)),
		AppEnv:         appEnv,
		DatabaseURL:    strings.TrimSpace(getEnv("DATABASE_URL", "")),
		Storage: StorageConfig{
			Endpoint:  strings.TrimSpace(firstEnvWithFallback("localhost:9000", "STORAGE_ENDPOINT", "S3_ENDPOINT")),
			AccessKey: strings.TrimSpace(firstEnvWithFallback("", "STORAGE_ACCESS_KEY", "S3_ACCESS_KEY_ID")),
			SecretKey: strings.TrimSpace(firstEnvWithFallback("", "STORAGE_SECRET_KEY", "S3_SECRET_ACCESS_KEY")),
			Bucket:    strings.TrimSpace(firstEnvWithFallback("aquify", "STORAGE_BUCKET", "MINIO_BUCKET")),
			UseSSL:    getEnvBool("STORAGE_USE_SSL", getEnvBool("S3_USE_SSL", false)),
			Region:    strings.TrimSpace(firstEnvWithFallback("us-east-1", "STORAGE_REGION", "S3_REGION")),
		},
		Auth: AuthConfig{
			EnableDevStubTokens:  getEnvBool("AUTH_ENABLE_DEV_STUB_TOKENS", enableDevStubTokensDefault),
			AllowDevSessionIssue: getEnvBool("AUTH_ALLOW_DEV_SESSION_ISSUE", enableDevStubTokensDefault),
			OIDC: OIDCConfig{
				AuthorizeURL:          strings.TrimSpace(getEnv("OIDC_AUTHORIZE_URL", "")),
				TokenURL:              strings.TrimSpace(getEnv("OIDC_TOKEN_URL", "")),
				JWKSURL:               strings.TrimSpace(getEnv("OIDC_JWKS_URL", "")),
				EndSessionURL:         strings.TrimSpace(getEnv("OIDC_END_SESSION_URL", "")),
				RedirectURL:           strings.TrimSpace(getEnv("OIDC_REDIRECT_URL", "")),
				PostLogoutRedirectURL: strings.TrimSpace(getEnv("OIDC_POST_LOGOUT_REDIRECT_URL", "")),
				ClientID:              strings.TrimSpace(getEnv("OIDC_CLIENT_ID", "")),
				ClientSecret:          strings.TrimSpace(getEnv("OIDC_CLIENT_SECRET", "")),
				ExpectedIssuer:        strings.TrimSpace(getEnv("OIDC_EXPECTED_ISSUER", "")),
				StateSigningKey:       strings.TrimSpace(getEnv("OIDC_STATE_SIGNING_KEY", "")),
				HTTPTimeout:           getEnvDuration("OIDC_HTTP_TIMEOUT", 10*time.Second),
			},
		},
		Stripe: StripeConfig{
			SecretKey:                         strings.TrimSpace(getEnv("STRIPE_SECRET_KEY", "")),
			WebhookSecret:                     strings.TrimSpace(getEnv("STRIPE_WEBHOOK_SECRET", "")),
			Currency:                          strings.ToLower(strings.TrimSpace(getEnv("STRIPE_CURRENCY", "usd"))),
			ReconciliationAlertThresholdCents: getEnvInt("STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS", 100),
		},
		Postmark: PostmarkConfig{
			ServerToken:            strings.TrimSpace(getEnv("POSTMARK_SERVER_TOKEN", "")),
			FromName:               strings.TrimSpace(getEnv("POSTMARK_FROM_NAME", "")),
			FromEmail:              strings.TrimSpace(getEnv("POSTMARK_FROM_EMAIL", "")),
			WebhookAuthHeaderName:  strings.TrimSpace(getEnv("POSTMARK_WEBHOOK_AUTH_HEADER_NAME", "")),
			WebhookAuthHeaderValue: strings.TrimSpace(getEnv("POSTMARK_WEBHOOK_AUTH_HEADER_VALUE", "")),
			WebhookBasicUser:       strings.TrimSpace(getEnv("POSTMARK_WEBHOOK_BASIC_USER", "")),
			WebhookBasicPass:       strings.TrimSpace(getEnv("POSTMARK_WEBHOOK_BASIC_PASS", "")),
		},
		Compliance: ComplianceConfig{
			AuditRetentionDays: getEnvInt("AUDIT_RETENTION_DAYS", 2555),
			ExportTTLDays:      getEnvInt("COMPLIANCE_EXPORT_TTL_DAYS", 30),
			DeletionReviewDays: getEnvInt("COMPLIANCE_DELETION_REVIEW_DAYS", 30),
		},
		Worker: WorkerConfig{
			RedisURL:            strings.TrimSpace(getEnv("REDIS_URL", "")),
			ConcurrencyPerQueue: getEnvInt("WORKER_CONCURRENCY_PER_QUEUE", 2),
			ReconcileInterval:   getEnv("WORKER_RECONCILE_INTERVAL", "24h"),
			IntegrityInterval:   getEnv("WORKER_INTEGRITY_INTERVAL", "24h"),
			CleanupInterval:     getEnv("WORKER_CLEANUP_INTERVAL", "6h"),
		},
		Security: SecurityConfig{
			CSRFHeaderName: strings.TrimSpace(getEnv("SECURITY_CSRF_HEADER_NAME", "X-CSRF-Token")),
			RateLimit: RateLimitConfig{
				GlobalRequests:    getEnvInt("SECURITY_RATE_LIMIT_GLOBAL_REQUESTS", 240),
				GlobalWindow:      getEnvDuration("SECURITY_RATE_LIMIT_GLOBAL_WINDOW", time.Minute),
				AuthRequests:      getEnvInt("SECURITY_RATE_LIMIT_AUTH_REQUESTS", 20),
				AuthWindow:        getEnvDuration("SECURITY_RATE_LIMIT_AUTH_WINDOW", time.Minute),
				StripeRequests:    getEnvInt("SECURITY_RATE_LIMIT_STRIPE_REQUESTS", 10),
				StripeWindow:      getEnvDuration("SECURITY_RATE_LIMIT_STRIPE_WINDOW", time.Minute),
				TrustedProxyCIDRs: splitCSV(getEnv("SECURITY_TRUSTED_PROXY_CIDRS", "")),
			},
			Uploads: UploadSecurityConfig{
				MaxBytes:               getEnvInt64("SECURITY_UPLOAD_MAX_BYTES", 25<<20),
				AllowedMIMEs:           splitCSV(getEnv("SECURITY_UPLOAD_ALLOWED_MIMES", "application/octet-stream,application/zip,application/x-zip-compressed,text/plain,text/csv")),
				RequireMalwareScanHook: getEnvBool("SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK", false),
			},
			Impersonation: ImpersonationConfig{
				Enabled: getEnvBool("SECURITY_IMPERSONATION_ENABLED", false),
			},
		},
		Telemetry: TelemetryConfig{
			ServiceName:    strings.TrimSpace(getEnv("OTEL_SERVICE_NAME", "dare-api")),
			ServiceVersion: strings.TrimSpace(getEnv("OTEL_SERVICE_VERSION", "dev")),
			OTLPEndpoint:   strings.TrimSpace(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "")),
			SampleRate:     getEnvFloat64("OTEL_TRACES_SAMPLER_ARG", 1.0),
			MetricsAddr:    strings.TrimSpace(getEnv("METRICS_ADDR", ":9090")),
		},
		Keycloak: KeycloakAdminConfig{
			AdminURL:     strings.TrimSpace(getEnv("KEYCLOAK_ADMIN_URL", "")),
			Realm:        strings.TrimSpace(getEnv("KEYCLOAK_REALM", "")),
			ClientID:     strings.TrimSpace(getEnv("KEYCLOAK_ADMIN_CLIENT_ID", "")),
			ClientSecret: strings.TrimSpace(getEnv("KEYCLOAK_ADMIN_CLIENT_SECRET", "")),
		},
	}
}

// ValidateDevAuthConfig returns an error when dev auth shortcuts
// (AUTH_ENABLE_DEV_STUB_TOKENS or AUTH_ALLOW_DEV_SESSION_ISSUE) are enabled
// outside of explicitly designated dev/local/test environments. This prevents
// misconfigured deployments from accidentally exposing unauthenticated access.
func ValidateDevAuthConfig(cfg Config) error {
	appEnv := strings.ToLower(strings.TrimSpace(cfg.AppEnv))
	isDevEnv := isExplicitDevelopmentEnv(appEnv)
	if isDevEnv {
		return nil
	}
	if cfg.Auth.EnableDevStubTokens {
		return fmt.Errorf(
			"AUTH_ENABLE_DEV_STUB_TOKENS=true is not permitted when APP_ENV=%q; set APP_ENV=development for local development",
			cfg.AppEnv,
		)
	}
	if cfg.Auth.AllowDevSessionIssue {
		return fmt.Errorf(
			"AUTH_ALLOW_DEV_SESSION_ISSUE=true is not permitted when APP_ENV=%q; set APP_ENV=development for local development",
			cfg.AppEnv,
		)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstEnvWithFallback(fallback string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvFloat64(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var parsed float64
	if _, err := fmt.Sscanf(value, "%f", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
