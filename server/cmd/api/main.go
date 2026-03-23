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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app"
	"github.com/dareaquatics/aquify/server/internal/telemetry"
)

func main() {
	cfg := app.LoadConfig()
	logger := slog.New(slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo},
	))

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

	// Build background workers (not started yet).
	ws, sched := app.BuildWorkers(cfg, logger, nil, nil, nil)

	if err := app.ValidateProviderConfig(cfg); err != nil {
		logger.Error("startup config error", "error", err)
		os.Exit(1)
	}

	// Build the HTTP router with the pre-configured worker server.
	handler := app.NewRouterWithServices(cfg, logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		ws,
	)

	// Validate non-development storage credentials before starting.
	if cfg.AppEnv != "development" && cfg.AppEnv != "local" {
		if cfg.Storage.AccessKey == "" || cfg.Storage.SecretKey == "" {
			logger.Error("startup config error",
				"error", "STORAGE_ACCESS_KEY and STORAGE_SECRET_KEY must be explicitly set in non-development environments")
			os.Exit(1)
		}
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start background workers.
	ws.Start()
	sched.Start()

	// Handle OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		logger.Info("shutdown signal received", "signal", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown error", "error", err)
		}

		// Flush remaining OTel spans before exit.
		if flushErr := telShutdown(shutdownCtx); flushErr != nil {
			logger.Warn("telemetry shutdown error", "error", flushErr)
		}

		sched.Stop()
		ws.Stop()
	}()

	logger.Info("api listening", "addr", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
	}
}
