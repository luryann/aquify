package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func cov2xMetricsLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestCov2xValidateMetricsConfig(t *testing.T) {
	t.Parallel()

	t.Run("disabled when no address configured", func(t *testing.T) {
		t.Parallel()
		logger, logs := cov2xMetricsLogger()
		if err := ValidateMetricsConfig(Config{AppEnv: "production", Addr: ":8080"}, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if logs.Len() != 0 {
			t.Fatalf("disabled metrics must not warn, got %s", logs.String())
		}
	})

	t.Run("rejects sharing the API listener", func(t *testing.T) {
		t.Parallel()
		cfg := Config{AppEnv: "development", Addr: ":8080"}
		cfg.Telemetry.MetricsAddr = " :8080 "
		err := ValidateMetricsConfig(cfg, nil)
		if err == nil || !strings.Contains(err.Error(), "METRICS_ADDR must not match API_ADDR") {
			t.Fatalf("expected API_ADDR collision error, got %v", err)
		}
	})

	t.Run("development is silent", func(t *testing.T) {
		t.Parallel()
		logger, logs := cov2xMetricsLogger()
		cfg := Config{AppEnv: "development", Addr: ":8080"}
		cfg.Telemetry.MetricsAddr = "0.0.0.0:9100"
		if err := ValidateMetricsConfig(cfg, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if logs.Len() != 0 {
			t.Fatalf("development must not warn, got %s", logs.String())
		}
	})

	t.Run("production warns with exposure class", func(t *testing.T) {
		t.Parallel()
		for addr, class := range map[string]string{":9100": "wildcard", "10.0.0.5:9100": "scoped"} {
			logger, logs := cov2xMetricsLogger()
			cfg := Config{AppEnv: "production", Addr: ":8080"}
			cfg.Telemetry.MetricsAddr = addr
			if err := ValidateMetricsConfig(cfg, logger); err != nil {
				t.Fatalf("%s: unexpected error: %v", addr, err)
			}
			out := logs.String()
			if !strings.Contains(out, "metrics endpoint enabled outside development") || !strings.Contains(out, "exposure="+class) {
				t.Fatalf("%s: expected %s warning, got %s", addr, class, out)
			}
		}
	})
}

func TestCov2xMetricsAddrHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		addr, host string
		wildcard   bool
	}{
		{"", "", true},
		{"   ", "", true},
		{"127.0.0.1:9100", "127.0.0.1", false},
		{":9100", "", true},
		{"0.0.0.0:9100", "0.0.0.0", true},
		{"[::]:9100", "::", true},
		{"[::1]:9100", "::1", false},
		{"metrics.internal", "metrics.internal", false},
		{"::", "", true},
		{"[::]", "[::]", true},
		{"bad:host:name", "bad:host:name", false},
	}
	for _, c := range cases {
		if got := metricsAddrHost(c.addr); got != c.host {
			t.Errorf("metricsAddrHost(%q) = %q, want %q", c.addr, got, c.host)
		}
		if got := metricsAddrBindsWildcard(c.addr); got != c.wildcard {
			t.Errorf("metricsAddrBindsWildcard(%q) = %v, want %v", c.addr, got, c.wildcard)
		}
	}
	// A bare leading-colon value that SplitHostPort rejects ("::" aside) is wildcard.
	if got := metricsAddrHost(":"); got != "" {
		t.Errorf("metricsAddrHost(\":\") = %q, want empty", got)
	}
	if metricsExposureClass("localhost:1") != "scoped" || metricsExposureClass(":1") != "wildcard" {
		t.Error("exposure class mismatch")
	}
}
