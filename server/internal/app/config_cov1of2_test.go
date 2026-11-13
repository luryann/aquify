package app

import (
	"strings"
	"testing"
)

func TestCov1xStorageBaseURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  StorageConfig
		want string
	}{
		{"defaults when blank", StorageConfig{}, "http://localhost:9000/aquify"},
		{"ssl strips http prefix", StorageConfig{Endpoint: " http://minio.example.test:9000 ", Bucket: " club ", UseSSL: true}, "https://minio.example.test:9000/club"},
		{"strips https prefix", StorageConfig{Endpoint: "https://s3.example.test", Bucket: "b"}, "http://s3.example.test/b"},
	}
	for _, tc := range cases {
		if got := tc.cfg.BaseURL(); got != tc.want {
			t.Errorf("%s: BaseURL() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCov1xLoadConfigDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "  Development ")
	t.Setenv("API_ALLOWED_ORIGINS", "")
	t.Setenv("METRICS_ADDR", "")
	cfg := LoadConfig()
	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q", cfg.AppEnv)
	}
	if cfg.Telemetry.MetricsAddr != ":9090" {
		t.Fatalf("development metrics default = %q", cfg.Telemetry.MetricsAddr)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "http://localhost:3000" {
		t.Fatalf("development origins = %v", cfg.AllowedOrigins)
	}
}

func TestCov1xLoadConfigProductionHasNoImplicitDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_ALLOWED_ORIGINS", "")
	t.Setenv("METRICS_ADDR", "")
	t.Setenv("AUTH_ENABLE_DEV_STUB_TOKENS", "")
	cfg := LoadConfig()
	if cfg.Telemetry.MetricsAddr != "" || len(cfg.AllowedOrigins) != 0 || cfg.Auth.EnableDevStubTokens {
		t.Fatalf("production must not inherit dev defaults: %+v", cfg)
	}
}

func TestCov1xValidateDevAuthConfig(t *testing.T) {
	t.Parallel()
	prod := func(mut func(*Config)) Config {
		c := Config{AppEnv: "production"}
		mut(&c)
		return c
	}
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"clean production", prod(func(*Config) {}), ""},
		{"dev allows everything", Config{AppEnv: "development", Auth: AuthConfig{EnableDevStubTokens: true, AllowDevSessionIssue: true, AllowInsecureSessionCookies: true}}, ""},
		{"stub tokens", prod(func(c *Config) { c.Auth.EnableDevStubTokens = true }), "AUTH_ENABLE_DEV_STUB_TOKENS=true is not permitted"},
		{"session issue", prod(func(c *Config) { c.Auth.AllowDevSessionIssue = true }), "AUTH_ALLOW_DEV_SESSION_ISSUE=true is not permitted"},
		{"insecure cookies", prod(func(c *Config) { c.Auth.AllowInsecureSessionCookies = true }), "SESSION_COOKIE_SECURE=false is not permitted"},
	}
	for _, tc := range cases {
		err := ValidateDevAuthConfig(tc.cfg)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: error %v, want containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestCov1xValidateAllowedOrigins(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"development skips", Config{AppEnv: "development"}, ""},
		{"empty", Config{AppEnv: "production"}, "API_ALLOWED_ORIGINS must be set"},
		{"http scheme", Config{AppEnv: "production", AllowedOrigins: []string{"http://admin.example.com"}}, "must be an https origin"},
		{"no host", Config{AppEnv: "production", AllowedOrigins: []string{"https://"}}, "must be an https origin"},
		{"userinfo", Config{AppEnv: "production", AllowedOrigins: []string{"https://user@admin.example.com"}}, "bare origin"},
		{"fragment", Config{AppEnv: "production", AllowedOrigins: []string{"https://admin.example.com#x"}}, "bare origin"},
		{"localhost", Config{AppEnv: "production", AllowedOrigins: []string{"https://LocalHost"}}, "loopback"},
		{"ipv4 loopback", Config{AppEnv: "staging", AllowedOrigins: []string{"https://127.0.0.1:8443"}}, "loopback"},
		{"ipv6 loopback", Config{AppEnv: "staging", AllowedOrigins: []string{"https://[::1]"}}, "loopback"},
		{"valid", Config{AppEnv: "production", AllowedOrigins: []string{"https://admin.example.com"}}, ""},
	}
	for _, tc := range cases {
		err := ValidateAllowedOrigins(tc.cfg)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: error %v, want containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestCov1xEnvHelpers(t *testing.T) {
	t.Setenv("COV1X_FIRST", "  ")
	t.Setenv("COV1X_SECOND", " second ")
	if got := firstEnvWithFallback("fb", "COV1X_FIRST", "COV1X_SECOND"); got != "second" {
		t.Fatalf("firstEnvWithFallback = %q", got)
	}
	if got := firstEnvWithFallback("fb", "COV1X_FIRST", "COV1X_UNSET"); got != "fb" {
		t.Fatalf("fallback = %q", got)
	}

	t.Setenv("COV1X_INT64", " 9000000000 ")
	t.Setenv("COV1X_FLOAT", "2.5")
	t.Setenv("COV1X_BAD", "not-a-number")
	if got := getEnvInt64("COV1X_INT64", 1); got != 9000000000 {
		t.Fatalf("getEnvInt64 = %d", got)
	}
	if got := getEnvInt64("COV1X_BAD", 7); got != 7 {
		t.Fatalf("getEnvInt64 bad = %d", got)
	}
	if got := getEnvFloat64("COV1X_FLOAT", 1); got != 2.5 {
		t.Fatalf("getEnvFloat64 = %v", got)
	}
	if got := getEnvFloat64("COV1X_BAD", 1.5); got != 1.5 {
		t.Fatalf("getEnvFloat64 bad = %v", got)
	}
	if got := getEnvInt("COV1X_BAD", 3); got != 3 {
		t.Fatalf("getEnvInt bad = %d", got)
	}
	if got := getEnvBool("COV1X_BAD", true); !got {
		t.Fatal("unparseable bool must fall back")
	}

	if getEnvOptionalBool("COV1X_UNSET") != nil {
		t.Fatal("unset optional bool must be nil")
	}
	t.Setenv("COV1X_OPT", "TRUE")
	if v := getEnvOptionalBool("COV1X_OPT"); v == nil || !*v {
		t.Fatalf("optional bool = %v", v)
	}
	t.Setenv("COV1X_OPT", "false")
	if v := getEnvOptionalBool("COV1X_OPT"); v == nil || *v {
		t.Fatalf("optional bool false = %v", v)
	}
}
