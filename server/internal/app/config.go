package app

import (
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
	Auth           AuthConfig
	Storage        StorageConfig
	Stripe         StripeConfig
	Postmark       PostmarkConfig
	Compliance     ComplianceConfig
	Worker         WorkerConfig
}

// WorkerConfig holds background job worker settings.
type WorkerConfig struct {
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
}

type AuthConfig struct {
	EnableDevStubTokens bool
	OIDC                OIDCConfig
}

type OIDCConfig struct {
	TokenURL        string
	JWKSURL         string
	RedirectURL     string
	ClientID        string
	ClientSecret    string
	ExpectedIssuer  string
	StateSigningKey string
	HTTPTimeout     time.Duration
}

func LoadConfig() Config {
	appEnv := strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", "development")))
	enableDevStubTokensDefault := appEnv == "development" || appEnv == "local"
	return Config{
		Addr:           getEnv("API_ADDR", defaultAPIAddr),
		AllowedOrigins: splitCSV(getEnv("API_ALLOWED_ORIGINS", defaultAllowedOrigin)),
		AppEnv:         appEnv,
		Storage: StorageConfig{
			Endpoint:  strings.TrimSpace(getEnv("STORAGE_ENDPOINT", "localhost:9000")),
			AccessKey: strings.TrimSpace(getEnv("STORAGE_ACCESS_KEY", "")),
			SecretKey: strings.TrimSpace(getEnv("STORAGE_SECRET_KEY", "")),
			Bucket:    strings.TrimSpace(getEnv("STORAGE_BUCKET", "aquify")),
			UseSSL:    getEnvBool("STORAGE_USE_SSL", false),
		},
		Auth: AuthConfig{
			EnableDevStubTokens: getEnvBool("AUTH_ENABLE_DEV_STUB_TOKENS", enableDevStubTokensDefault),
			OIDC: OIDCConfig{
				TokenURL:        strings.TrimSpace(getEnv("OIDC_TOKEN_URL", "")),
				JWKSURL:         strings.TrimSpace(getEnv("OIDC_JWKS_URL", "")),
				RedirectURL:     strings.TrimSpace(getEnv("OIDC_REDIRECT_URL", "")),
				ClientID:        strings.TrimSpace(getEnv("OIDC_CLIENT_ID", "")),
				ClientSecret:    strings.TrimSpace(getEnv("OIDC_CLIENT_SECRET", "")),
				ExpectedIssuer:  strings.TrimSpace(getEnv("OIDC_EXPECTED_ISSUER", "")),
				StateSigningKey: strings.TrimSpace(getEnv("OIDC_STATE_SIGNING_KEY", "")),
				HTTPTimeout:     getEnvDuration("OIDC_HTTP_TIMEOUT", 10*time.Second),
			},
		},
		Stripe: StripeConfig{
			SecretKey:                         strings.TrimSpace(getEnv("STRIPE_SECRET_KEY", "")),
			WebhookSecret:                     strings.TrimSpace(getEnv("STRIPE_WEBHOOK_SECRET", "")),
			Currency:                          strings.ToLower(strings.TrimSpace(getEnv("STRIPE_CURRENCY", "usd"))),
			ReconciliationAlertThresholdCents: getEnvInt("STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS", 100),
		},
		Postmark: PostmarkConfig{
			ServerToken: strings.TrimSpace(getEnv("POSTMARK_SERVER_TOKEN", "")),
			FromName:    strings.TrimSpace(getEnv("POSTMARK_FROM_NAME", "")),
			FromEmail:   strings.TrimSpace(getEnv("POSTMARK_FROM_EMAIL", "")),
		},
		Compliance: ComplianceConfig{
			AuditRetentionDays: getEnvInt("AUDIT_RETENTION_DAYS", 2555),
			ExportTTLDays:      getEnvInt("COMPLIANCE_EXPORT_TTL_DAYS", 30),
			DeletionReviewDays: getEnvInt("COMPLIANCE_DELETION_REVIEW_DAYS", 30),
		},
		Worker: WorkerConfig{
			ConcurrencyPerQueue: getEnvInt("WORKER_CONCURRENCY_PER_QUEUE", 2),
			ReconcileInterval:   getEnv("WORKER_RECONCILE_INTERVAL", "24h"),
			IntegrityInterval:   getEnv("WORKER_INTEGRITY_INTERVAL", "24h"),
			CleanupInterval:     getEnv("WORKER_CLEANUP_INTERVAL", "6h"),
		},
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
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
