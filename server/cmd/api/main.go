//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  AUTHOR: RYAN LU
//  OWNER: RYAN LU
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  THIS SOFTWARE MAY BE LICENSED BY RYAN LU TO DARE AQUATICS AND TO
//  OTHER TEAMS, ORGANIZATIONS, OR AUTHORIZED USERS ONLY IF AND TO THE
//  EXTENT EXPRESSLY APPROVED BY RYAN LU.
//
//  NO PERSON OR ENTITY MAY ACCESS, INSTALL, COPY, USE, MODIFY,
//  DISTRIBUTE, DISCLOSE, HOST, SELL, SUBLICENSE, TRANSFER, REVERSE
//  ENGINEER, DECOMPILE, DISASSEMBLE, OR OTHERWISE EXPLOIT THIS SOURCE
//  FILE OR THE SOFTWARE UNLESS: (1) RYAN LU HAS GIVEN PRIOR EXPRESS
//  PERMISSION OR ENTERED INTO A SEPARATE LICENSE OR OTHER WRITTEN
//  AUTHORIZATION, AND (2) THE PERSON OR ENTITY HAS AGREED TO THE
//  APPLICABLE LICENSE TERMS BEFORE ANY SUCH ACCESS OR USE.
//
//  ANY LICENSE GRANTED BY RYAN LU IS LIMITED, REVOCABLE,
//  NON-EXCLUSIVE, NON-TRANSFERABLE, AND NON-SUBLICENSABLE UNLESS
//  RYAN LU EXPRESSLY AGREES OTHERWISE IN A SEPARATE SIGNED WRITING.
//
//  DARE AQUATICS, IF AUTHORIZED, MAY USE THE SOFTWARE SOLELY WITHIN
//  THE SCOPE OF ITS LICENSE. NO MODIFICATION IS PERMITTED EXCEPT WITH
//  RYAN LU'S PRIOR EXPRESS WRITTEN PERMISSION. THE SAME RULE APPLIES
//  TO ANY OTHER LICENSEE UNLESS ITS LICENSE EXPRESSLY STATES
//  OTHERWISE.
//
//  ACCESS IS RESTRICTED TO AUTHORIZED PERSONNEL AND APPROVED
//  ENVIRONMENTS ONLY. UNAUTHORIZED ACCESS, USE, DISCLOSURE,
//  DUPLICATION, OR DISTRIBUTION IS STRICTLY PROHIBITED AND MAY RESULT
//  IN TERMINATION OF RIGHTS AND LEGAL LIABILITY.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED BY RYAN LU IN A SEPARATE WRITTEN
//  AGREEMENT ARE RESERVED BY RYAN LU.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app"
	"github.com/dareaquatics/aquify/server/internal/telemetry"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/joho/godotenv"
)

var loadDotEnvFile = loadDotEnv

// loadDotEnv reads a .env-style file with the standard godotenv parser (export
// prefixes, quoting, escapes and inline comments) and sets any unset
// environment variables. Existing env vars (even empty ones) are never
// overwritten. A missing file is ignored; a malformed file is reported on
// stderr instead of being half-applied silently.
func loadDotEnv(path string) {
	values, err := godotenv.Read(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "warning: ignoring %s: %v\n", path, err)
		}
		return
	}
	for key, val := range values {
		if _, exists := os.LookupEnv(key); key != "" && !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func shouldLoadLocalEnvFiles(appEnv string) bool {
	switch app.NormalizeAppEnv(appEnv) {
	case "development", "local":
		return true
	default:
		return false
	}
}

func loadLocalEnvFiles(appEnv string) {
	if !shouldLoadLocalEnvFiles(appEnv) {
		return
	}
	// Load .env.local from the project root (one level up from server/).
	// Existing environment variables always take precedence.
	loadDotEnvFile("../.env.local")
	loadDotEnvFile(".env.local")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if len(os.Args) != 3 {
			os.Exit(1)
		}
		if err := checkHealth(os.Args[2]); err != nil {
			slog.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	loadLocalEnvFiles(os.Getenv("APP_ENV"))

	cfg := app.LoadConfig()
	logger := slog.New(slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo},
	))
	// httpx.WriteError logs 5xx causes through the default logger.
	slog.SetDefault(logger)
	mode := "api"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		mode = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}

	// Initialise OpenTelemetry tracing.
	telCfg := telemetry.Config{
		ServiceName:    cfg.Telemetry.ServiceName,
		ServiceVersion: cfg.Telemetry.ServiceVersion,
		OTLPEndpoint:   cfg.Telemetry.OTLPEndpoint,
		SampleRate:     cfg.Telemetry.SampleRate,
	}
	telShutdown, err := telemetry.Setup(context.Background(), telCfg, logger)
	if err != nil {
		logger.Error("telemetry setup failed", "error", err)
		os.Exit(1)
	}

	exitOnError := func(msg string, err error) {
		if err != nil {
			logger.Error(msg, "error", err)
			os.Exit(1)
		}
	}
	exitOnError("startup config error", app.ValidateAppEnvConfig(cfg))
	exitOnError("startup config error", app.ValidateDevAuthConfig(cfg))
	exitOnError("startup config error", app.ValidateOIDCLogoutConfig(cfg, logger))
	exitOnError("startup config error", app.ValidateKeycloakAdminConfig(cfg))
	exitOnError("startup config error", app.ValidateWorkerConfig(cfg))
	exitOnError("startup config error", app.ValidateProviderConfig(cfg))
	exitOnError("startup config error", app.ValidateMetricsConfig(cfg, logger))
	exitOnError("keycloak startup validation failed", app.ValidateKeycloakStartup(cfg, logger))

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer startupCancel()
	exitOnError("startup dependency validation failed", app.ValidateStartupDependencies(startupCtx, cfg, logger))

	var backend *workers.RedisBackend
	if strings.TrimSpace(cfg.Worker.RedisURL) != "" {
		backend, err = workers.NewRedisBackend(workers.RedisBackendConfig{
			RedisURL:    cfg.Worker.RedisURL,
			Logger:      logger,
			Concurrency: cfg.Worker.ConcurrencyPerQueue,
		})
		if err != nil {
			cfg, _, err = app.MaybeApplyDevelopmentRedisFallback(cfg, logger, err)
			if err != nil {
				logger.Error("worker backend init failed", "error", err)
				os.Exit(1)
			}
		}
		if backend != nil {
			defer func() {
				if err := backend.Close(); err != nil {
					logger.Warn("worker backend close failed", "error", err)
				}
			}()
		}
	}

	if mode == "worker" {
		runWorker(cfg, logger, backend, telShutdown)
		return
	}

	metrics := telemetry.NewMetrics()

	// Build the HTTP router with the configured durable backend when available.
	handler := app.NewRouterWithServices(cfg, logger, app.RouterDeps{
		Metrics: metrics,
		Workers: backend,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	var metricsServer *http.Server
	if strings.TrimSpace(cfg.Telemetry.MetricsAddr) != "" {
		metricsServer = &http.Server{
			Addr:              cfg.Telemetry.MetricsAddr,
			Handler:           metrics.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
	}

	// Handle OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// ListenAndServe returns as soon as Shutdown starts; main must wait for the
	// drain and telemetry flush below to finish before it exits.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		sig := <-sigCh
		logger.Info("shutdown signal received", "signal", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown error", "error", err)
		}
		if metricsServer != nil {
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				logger.Error("metrics server shutdown error", "error", err)
			}
		}

		// Flush remaining OTel spans before exit.
		if flushErr := telShutdown(shutdownCtx); flushErr != nil {
			logger.Warn("telemetry shutdown error", "error", flushErr)
		}
	}()

	if metricsServer != nil {
		go func() {
			logger.Info("metrics listening", "addr", metricsServer.Addr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server failed", "error", err)
			}
		}()
	}

	logger.Info("api listening", "addr", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		return
	}
	<-shutdownDone
}

// runWorker serves the durable job queue until a termination signal arrives,
// then drains in-flight jobs and flushes telemetry.
func runWorker(cfg app.Config, logger *slog.Logger, backend *workers.RedisBackend, telShutdown func(context.Context) error) {
	if backend == nil {
		logger.Error("worker mode requires REDIS_URL")
		os.Exit(1)
	}
	// The worker process records job executions; it serves them on METRICS_ADDR
	// the same way the API does so Prometheus can scrape both.
	metrics := telemetry.NewMetrics()
	var metricsServer *http.Server
	if strings.TrimSpace(cfg.Telemetry.MetricsAddr) != "" {
		metricsServer = &http.Server{
			Addr:              cfg.Telemetry.MetricsAddr,
			Handler:           metrics.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			logger.Info("worker metrics listening", "addr", metricsServer.Addr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("worker metrics server failed", "error", err)
			}
		}()
	}
	app.NewRouterWithServices(cfg, logger, app.RouterDeps{
		Metrics: metrics,
		Workers: backend,
	})
	app.RegisterDurableScheduledJobs(backend, cfg)
	if err := backend.Start(); err != nil {
		logger.Error("worker start failed", "error", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	logger.Info("worker shutdown signal received", "signal", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := backend.Stop(shutdownCtx); err != nil {
		logger.Error("worker shutdown error", "error", err)
	}
	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("worker metrics shutdown error", "error", err)
		}
	}
	if flushErr := telShutdown(shutdownCtx); flushErr != nil {
		logger.Warn("telemetry shutdown error", "error", flushErr)
	}
}
