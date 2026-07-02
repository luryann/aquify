package app

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// ValidateMetricsConfig enforces safer Prometheus exposure defaults.
// Metrics are disabled unless METRICS_ADDR is explicitly configured, and
// production-like environments receive a loud warning when the listener binds
// on wildcard/public interfaces.
func ValidateMetricsConfig(cfg Config, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	metricsAddr := strings.TrimSpace(cfg.Telemetry.MetricsAddr)
	if metricsAddr == "" {
		return nil
	}
	if metricsAddr == strings.TrimSpace(cfg.Addr) {
		return fmt.Errorf("METRICS_ADDR must not match API_ADDR; bind Prometheus metrics on a dedicated listener or disable METRICS_ADDR")
	}
	if !isExplicitDevelopmentEnv(cfg.AppEnv) {
		logger.Warn("metrics endpoint enabled outside development",
			"app_env", cfg.AppEnv,
			"metrics_addr", metricsAddr,
			"exposure", metricsExposureClass(metricsAddr),
			"path", "/metrics",
			"action", "restrict the metrics listener to trusted scraper networks or disable METRICS_ADDR",
		)
	}
	return nil
}

func metricsExposureClass(addr string) string {
	if metricsAddrBindsWildcard(addr) {
		return "wildcard"
	}
	return "scoped"
}

func metricsAddrBindsWildcard(addr string) bool {
	host := metricsAddrHost(addr)
	switch strings.TrimSpace(strings.Trim(host, "[]")) {
	case "", "0.0.0.0", "::":
		return true
	default:
		return false
	}
}

func metricsAddrHost(addr string) string {
	if strings.TrimSpace(addr) == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	if strings.Count(addr, ":") == 0 {
		return addr
	}
	if strings.HasPrefix(addr, ":") {
		return ""
	}
	return addr
}
