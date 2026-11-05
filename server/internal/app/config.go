package app

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/luryann/aquify/server/internal/storage"
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
	// Comms limits.
	Comms CommsConfig
	// Cache bounds the per-club in-memory caches.
	Cache      CacheConfig
	Compliance ComplianceConfig
	Worker     WorkerConfig
	Security   SecurityConfig
	Telemetry  TelemetryConfig
	Keycloak   KeycloakAdminConfig
	// Platform configures the operator plane (tenant provisioning/suspension).
	Platform PlatformConfig
}

// CacheConfig bounds how many clubs' rows the member, group, meet and billing
// services keep in process memory.
type CacheConfig struct {
	// OrgIdleTTL evicts a club's rows after this long without a request
	// (CACHE_ORG_IDLE_TTL, default 30m, 0 = never).
	OrgIdleTTL time.Duration
	// MaxOrgs caps how many clubs are cached per service; the least recently served
	// are evicted first (CACHE_MAX_ORGS, default 200, 0 = unlimited).
	MaxOrgs int
}

// CommsConfig holds per-org email limits.
type CommsConfig struct {
	// OrgDailyEmailCap caps emails per org per rolling 24h (COMMS_ORG_DAILY_EMAIL_CAP,
	// default 20000, 0 = unlimited).
	OrgDailyEmailCap int
	// OrgDocumentStorageBytes caps document files per org (ORG_DOCUMENT_STORAGE_BYTES,
	// default 10 GiB, 0 = unlimited).
	OrgDocumentStorageBytes int64
}

// PlatformConfig holds the operator-plane settings. With OrgID empty the
// /api/v1/platform routes are not mounted at all (fail closed).
type PlatformConfig struct {
	// OrgID is the UUID of the dedicated platform org whose members may hold the
	// platform_admin role. It is registered in the tenant registry at startup.
	OrgID string
	// OffboardingCoolingOff is how long an org stays in offboarding before it can
	// be permanently deleted (PLATFORM_OFFBOARDING_COOLING_OFF, default 336h).
	OffboardingCoolingOff time.Duration
}

// KeycloakAdminConfig holds credentials for the Keycloak Admin REST API.
// Used by the keycloak.AdminClient to manage users and roles on behalf of admins.
type KeycloakAdminConfig struct {
	// AdminURL is the base URL of the Keycloak instance (e.g. "http://localhost:8180").
	AdminURL string
	// Realm is the target Keycloak realm name (e.g. "aquify").
	Realm string
	// ClientID is the service-account OAuth2 client ID for admin operations.
	ClientID string `json:"-"`
	// ClientSecret is the service-account OAuth2 client secret.
	ClientSecret string `json:"-"`
}

// TelemetryConfig holds OpenTelemetry and Prometheus observability settings.
type TelemetryConfig struct {
	// ServiceName is the OTel service.name attribute. Default: "aquify-api".
	ServiceName string
	// ServiceVersion is the deployed artifact version. Default: "dev".
	ServiceVersion string
	// OTLPEndpoint is the OTLP HTTP endpoint. When empty, a no-op tracer is used.
	OTLPEndpoint string
	// SampleRate is the OTel trace sampling rate (0.0–1.0). Default: 1.0.
	SampleRate float64
	// MetricsAddr is the dedicated address for the Prometheus /metrics listener.
	// Default: ":9090" only in explicit development/local/test environments.
	// Set to "" to disable the Prometheus endpoint entirely.
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
	// BillingAutomationInterval is how often the recurring billing run and
	// late-fee sweep jobs fire (each is idempotent). Default: 24h; 0 disables.
	BillingAutomationInterval string
	// ScheduledOrgIDs is the list of orgs that receive org-scoped scheduled
	// jobs such as billing reconciliation and integrity checks.
	ScheduledOrgIDs []string
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
	// ConnectWebhookSecret signs the Connect webhook endpoint (events on clubs'
	// connected accounts); the platform endpoint uses WebhookSecret.
	ConnectWebhookSecret string `json:"-"`
	// ConnectRequired makes every club use its own connected account: a club
	// that has not finished onboarding cannot take online payments (no fallback to
	// the platform account). STRIPE_CONNECT_REQUIRED unset = required outside
	// explicit development/test environments.
	ConnectRequired *bool
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

// ClientConfig converts the app-level storage settings into the object storage client's config.
func (c StorageConfig) ClientConfig() storage.Config {
	return storage.Config(c)
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
	// AllowInsecureSessionCookies drops the Secure attribute from session
	// cookies (SESSION_COOKIE_SECURE=false) so plain-HTTP local dev can receive
	// them. The zero value is Secure; startup rejects it outside explicit
	// development/local/test environments.
	AllowInsecureSessionCookies bool
	// CSRFTokenFactory overrides CSRF token generation (tests); nil uses the
	// production crypto/rand factory.
	CSRFTokenFactory func() (string, error)
	OIDC             OIDCConfig
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
	// OrgRequests / OrgWindow cap authenticated API requests per organization
	// (across all of its users and IPs), so one busy club cannot starve the rest.
	// OrgRequests <= 0 disables the per-org limit.
	OrgRequests int
	OrgWindow   time.Duration
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
	appEnv := NormalizeAppEnv(getEnvTrimmed("APP_ENV", ""))
	enableDevStubTokensDefault := isExplicitDevelopmentEnv(appEnv)
	metricsAddrDefault := ""
	if isExplicitDevelopmentEnv(appEnv) {
		metricsAddrDefault = ":9090"
	}
	allowedOriginsDefault := ""
	if isExplicitDevelopmentEnv(appEnv) {
		allowedOriginsDefault = defaultAllowedOrigin
	}
	return Config{
		Addr:           getEnv("API_ADDR", defaultAPIAddr),
		AllowedOrigins: splitCSV(getEnv("API_ALLOWED_ORIGINS", allowedOriginsDefault)),
		AppEnv:         appEnv,
		DatabaseURL:    getEnvTrimmed("DATABASE_URL", ""),
		Storage: StorageConfig{
			Endpoint:  strings.TrimSpace(firstEnvWithFallback("localhost:9000", "STORAGE_ENDPOINT", "S3_ENDPOINT")),
			AccessKey: strings.TrimSpace(firstEnvWithFallback("", "STORAGE_ACCESS_KEY", "S3_ACCESS_KEY_ID")),
			SecretKey: strings.TrimSpace(firstEnvWithFallback("", "STORAGE_SECRET_KEY", "S3_SECRET_ACCESS_KEY")),
			Bucket:    strings.TrimSpace(firstEnvWithFallback("aquify", "STORAGE_BUCKET", "MINIO_BUCKET")),
			UseSSL:    getEnvBool("STORAGE_USE_SSL", getEnvBool("S3_USE_SSL", false)),
			Region:    strings.TrimSpace(firstEnvWithFallback("us-east-1", "STORAGE_REGION", "S3_REGION")),
		},
		Auth: AuthConfig{
			EnableDevStubTokens:         getEnvBool("AUTH_ENABLE_DEV_STUB_TOKENS", enableDevStubTokensDefault),
			AllowDevSessionIssue:        getEnvBool("AUTH_ALLOW_DEV_SESSION_ISSUE", enableDevStubTokensDefault),
			AllowInsecureSessionCookies: !getEnvBool("SESSION_COOKIE_SECURE", true),
			OIDC: OIDCConfig{
				AuthorizeURL:          getEnvTrimmed("OIDC_AUTHORIZE_URL", ""),
				TokenURL:              getEnvTrimmed("OIDC_TOKEN_URL", ""),
				JWKSURL:               getEnvTrimmed("OIDC_JWKS_URL", ""),
				EndSessionURL:         getEnvTrimmed("OIDC_END_SESSION_URL", ""),
				RedirectURL:           getEnvTrimmed("OIDC_REDIRECT_URL", ""),
				PostLogoutRedirectURL: getEnvTrimmed("OIDC_POST_LOGOUT_REDIRECT_URL", ""),
				ClientID:              getEnvTrimmed("OIDC_CLIENT_ID", ""),
				ClientSecret:          getEnvTrimmed("OIDC_CLIENT_SECRET", ""),
				ExpectedIssuer:        getEnvTrimmed("OIDC_EXPECTED_ISSUER", ""),
				StateSigningKey:       getEnvTrimmed("OIDC_STATE_SIGNING_KEY", ""),
				HTTPTimeout:           getEnvDuration("OIDC_HTTP_TIMEOUT", 10*time.Second),
			},
		},
		Stripe: StripeConfig{
			SecretKey:                         getEnvTrimmed("STRIPE_SECRET_KEY", ""),
			WebhookSecret:                     getEnvTrimmed("STRIPE_WEBHOOK_SECRET", ""),
			ConnectWebhookSecret:              getEnvTrimmed("STRIPE_CONNECT_WEBHOOK_SECRET", ""),
			ConnectRequired:                   getEnvOptionalBool("STRIPE_CONNECT_REQUIRED"),
			Currency:                          strings.ToLower(getEnvTrimmed("STRIPE_CURRENCY", "usd")),
			ReconciliationAlertThresholdCents: getEnvInt("STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS", 100),
		},
		Postmark: PostmarkConfig{
			ServerToken:            getEnvTrimmed("POSTMARK_SERVER_TOKEN", ""),
			FromName:               getEnvTrimmed("POSTMARK_FROM_NAME", ""),
			FromEmail:              getEnvTrimmed("POSTMARK_FROM_EMAIL", ""),
			WebhookAuthHeaderName:  getEnvTrimmed("POSTMARK_WEBHOOK_AUTH_HEADER_NAME", ""),
			WebhookAuthHeaderValue: getEnvTrimmed("POSTMARK_WEBHOOK_AUTH_HEADER_VALUE", ""),
			WebhookBasicUser:       getEnvTrimmed("POSTMARK_WEBHOOK_BASIC_USER", ""),
			WebhookBasicPass:       getEnvTrimmed("POSTMARK_WEBHOOK_BASIC_PASS", ""),
		},
		Compliance: ComplianceConfig{
			AuditRetentionDays: getEnvInt("AUDIT_RETENTION_DAYS", 2555),
			ExportTTLDays:      getEnvInt("COMPLIANCE_EXPORT_TTL_DAYS", 30),
			DeletionReviewDays: getEnvInt("COMPLIANCE_DELETION_REVIEW_DAYS", 30),
		},
		Worker: WorkerConfig{
			RedisURL:                  getEnvTrimmed("REDIS_URL", ""),
			ConcurrencyPerQueue:       getEnvInt("WORKER_CONCURRENCY_PER_QUEUE", 2),
			ReconcileInterval:         getEnv("WORKER_RECONCILE_INTERVAL", "24h"),
			IntegrityInterval:         getEnv("WORKER_INTEGRITY_INTERVAL", "24h"),
			CleanupInterval:           getEnv("WORKER_CLEANUP_INTERVAL", "6h"),
			BillingAutomationInterval: getEnv("WORKER_BILLING_AUTOMATION_INTERVAL", "24h"),
			ScheduledOrgIDs:           splitCSV(getEnv("WORKER_SCHEDULED_ORG_IDS", "")),
		},
		Security: SecurityConfig{
			CSRFHeaderName: getEnvTrimmed("SECURITY_CSRF_HEADER_NAME", "X-CSRF-Token"),
			RateLimit: RateLimitConfig{
				GlobalRequests:    getEnvInt("SECURITY_RATE_LIMIT_GLOBAL_REQUESTS", 1200),
				GlobalWindow:      getEnvDuration("SECURITY_RATE_LIMIT_GLOBAL_WINDOW", time.Minute),
				OrgRequests:       getEnvInt("SECURITY_RATE_LIMIT_ORG_REQUESTS", 6000),
				OrgWindow:         getEnvDuration("SECURITY_RATE_LIMIT_ORG_WINDOW", time.Minute),
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
			ServiceName:    getEnvTrimmed("OTEL_SERVICE_NAME", "aquify-api"),
			ServiceVersion: getEnvTrimmed("OTEL_SERVICE_VERSION", "dev"),
			OTLPEndpoint:   getEnvTrimmed("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			SampleRate:     getEnvFloat64("OTEL_TRACES_SAMPLER_ARG", 1.0),
			MetricsAddr:    getEnvTrimmed("METRICS_ADDR", metricsAddrDefault),
		},
		Cache:    CacheConfig{OrgIdleTTL: getEnvDuration("CACHE_ORG_IDLE_TTL", 30*time.Minute), MaxOrgs: getEnvInt("CACHE_MAX_ORGS", 200)},
		Comms:    CommsConfig{OrgDailyEmailCap: getEnvInt("COMMS_ORG_DAILY_EMAIL_CAP", 20000), OrgDocumentStorageBytes: int64(getEnvInt("ORG_DOCUMENT_STORAGE_BYTES", 10<<30))},
		Platform: PlatformConfig{OrgID: getEnvTrimmed("PLATFORM_ORG_ID", ""), OffboardingCoolingOff: getEnvDuration("PLATFORM_OFFBOARDING_COOLING_OFF", 14*24*time.Hour)},
		Keycloak: KeycloakAdminConfig{
			AdminURL:     getEnvTrimmed("KEYCLOAK_ADMIN_URL", ""),
			Realm:        getEnvTrimmed("KEYCLOAK_REALM", ""),
			ClientID:     getEnvTrimmed("KEYCLOAK_ADMIN_CLIENT_ID", ""),
			ClientSecret: getEnvTrimmed("KEYCLOAK_ADMIN_CLIENT_SECRET", ""),
		},
	}
}

// ValidateDevAuthConfig returns an error when dev auth shortcuts
// (AUTH_ENABLE_DEV_STUB_TOKENS or AUTH_ALLOW_DEV_SESSION_ISSUE) are enabled
// outside of explicitly designated dev/local/test environments. This prevents
// misconfigured deployments from accidentally exposing unauthenticated access.
func ValidateDevAuthConfig(cfg Config) error {
	appEnv := NormalizeAppEnv(cfg.AppEnv)
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
	if cfg.Auth.AllowInsecureSessionCookies {
		return fmt.Errorf(
			"SESSION_COOKIE_SECURE=false is not permitted when APP_ENV=%q; non-Secure session cookies are only allowed in development, local or test",
			cfg.AppEnv,
		)
	}
	return nil
}

// ValidateAllowedOrigins requires API_ALLOWED_ORIGINS outside development: it
// must be set and contain only https, non-localhost origins, otherwise CORS and
// Stripe/Connect return-URL checks silently reject the real admin origin.
func ValidateAllowedOrigins(cfg Config) error {
	if isExplicitDevelopmentEnv(NormalizeAppEnv(cfg.AppEnv)) {
		return nil
	}
	if len(cfg.AllowedOrigins) == 0 {
		return fmt.Errorf("API_ALLOWED_ORIGINS must be set when APP_ENV=%q (comma-separated https admin origins)", cfg.AppEnv)
	}
	for _, origin := range cfg.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("API_ALLOWED_ORIGINS entry %q must be an https origin when APP_ENV=%q", origin, cfg.AppEnv)
		}
		if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || strings.HasSuffix(origin, "/") {
			return fmt.Errorf("API_ALLOWED_ORIGINS entry %q must be a bare origin (scheme://host[:port]) with no trailing slash or path when APP_ENV=%q", origin, cfg.AppEnv)
		}
		switch strings.ToLower(u.Hostname()) {
		case "localhost", "127.0.0.1", "::1":
			return fmt.Errorf("API_ALLOWED_ORIGINS entry %q must not be a loopback origin when APP_ENV=%q", origin, cfg.AppEnv)
		}
	}
	return nil
}

// ValidateAppEnvConfig requires APP_ENV to be explicit and recognized at
// startup so staging/production deployments cannot silently run with typoed or
// inherited local defaults.
func ValidateAppEnvConfig(cfg Config) error {
	appEnv := NormalizeAppEnv(cfg.AppEnv)
	if appEnv == "" {
		return fmt.Errorf("APP_ENV must be set explicitly; allowed values: development, local, test, staging, production")
	}
	if isRecognizedAppEnv(appEnv) {
		return nil
	}
	return fmt.Errorf("APP_ENV=%q is invalid; allowed values: development, local, test, staging, production", cfg.AppEnv)
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

// getEnvTrimmed is getEnv with surrounding whitespace removed from the result.
func getEnvTrimmed(key, fallback string) string {
	return strings.TrimSpace(getEnv(key, fallback))
}

// parseEnv reads and parses an environment variable, returning fallback when
// it is unset, blank, or fails to parse.
func parseEnv[T any](key string, fallback T, parse func(string) (T, error)) T {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := parse(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	return parseEnv(key, fallback, func(v string) (bool, error) { return strconv.ParseBool(strings.ToLower(v)) })
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	return parseEnv(key, fallback, time.ParseDuration)
}

func getEnvInt(key string, fallback int) int {
	return parseEnv(key, fallback, strconv.Atoi)
}

func getEnvInt64(key string, fallback int64) int64 {
	return parseEnv(key, fallback, func(v string) (int64, error) { return strconv.ParseInt(v, 10, 64) })
}

func getEnvFloat64(key string, fallback float64) float64 {
	return parseEnv(key, fallback, func(v string) (float64, error) { return strconv.ParseFloat(v, 64) })
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

func isRecognizedAppEnv(appEnv string) bool {
	switch NormalizeAppEnv(appEnv) {
	case "development", "local", "test", "staging", "production":
		return true
	default:
		return false
	}
}

// getEnvOptionalBool returns nil when the variable is unset or empty.
func getEnvOptionalBool(key string) *bool {
	if strings.TrimSpace(os.Getenv(key)) == "" {
		return nil
	}
	v := getEnvBool(key, false)
	return &v
}
