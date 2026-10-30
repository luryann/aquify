package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/dareaquatics/aquify/server/internal/postmark"
	"github.com/dareaquatics/aquify/server/internal/storage"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dependencyReadinessCheck struct {
	name  string
	check func(ctx context.Context) (string, error)
}

type appReadinessChecker struct {
	checks []dependencyReadinessCheck
}

func (c appReadinessChecker) CheckReadiness(ctx context.Context) handlers.ReadinessReport {
	report := handlers.ReadinessReport{
		Ready:  true,
		Checks: make(map[string]string, len(c.checks)),
	}
	for _, check := range c.checks {
		status, err := check.check(ctx)
		if err != nil {
			report.Ready = false
			report.Checks[check.name] = "not_ready: " + err.Error()
			continue
		}
		if strings.TrimSpace(status) == "" {
			status = "ready"
		}
		report.Checks[check.name] = status
	}
	return report
}

func buildReadinessChecker(cfg Config, dbPool *pgxpool.Pool, objectStore storage.ObjectStorage, stripeClient stripelib.Client, postmarkClient postmark.Client, workerBackend workers.Backend, kcClient keycloak.Client) handlers.ReadinessChecker {
	storageCfg := cfg.Storage.ClientConfig()

	return appReadinessChecker{
		checks: []dependencyReadinessCheck{
			{
				name: "database",
				check: func(ctx context.Context) (string, error) {
					if strings.TrimSpace(cfg.DatabaseURL) == "" {
						return "disabled", nil
					}
					if dbPool == nil {
						return "", fmt.Errorf("database pool unavailable")
					}
					return probeReady(ctx, dbPool.Ping)
				},
			},
			{
				name: "queue",
				check: func(ctx context.Context) (string, error) {
					if strings.TrimSpace(cfg.Worker.RedisURL) == "" {
						// A typed-nil *workers.Server in the interface is not a live
						// in-memory queue: check nil-ness before the type assertion.
						if isNilWorkerBackend(workerBackend) {
							return "disabled", nil
						}
						if _, ok := workerBackend.(*workers.Server); ok {
							return "ready (in-memory)", nil
						}
						return "ready", nil
					}
					if isNilWorkerBackend(workerBackend) {
						return "", fmt.Errorf("redis backend not initialized")
					}
					backend, ok := workerBackend.(*workers.RedisBackend)
					if !ok || backend == nil {
						return "", fmt.Errorf("durable redis backend not active")
					}
					return probeReady(ctx, backend.Ready)
				},
			},
			{
				name: "storage",
				check: func(ctx context.Context) (string, error) {
					if objectStore == nil {
						return "", fmt.Errorf("storage client unavailable")
					}
					if _, ok := objectStore.(*storage.StubStorage); ok {
						if storageCfg.Enabled() {
							return "", fmt.Errorf("stub storage active while object storage is configured")
						}
						return "ready (stub)", nil
					}
					if !storageCfg.Enabled() {
						return "disabled", nil
					}
					prober, ok := objectStore.(interface{ Ready(context.Context) error })
					if !ok {
						return "", fmt.Errorf("storage readiness probe unavailable")
					}
					return probeReady(ctx, prober.Ready)
				},
			},
			{
				name: "stripe",
				check: func(ctx context.Context) (string, error) {
					if strings.TrimSpace(cfg.Stripe.SecretKey) == "" {
						return "disabled", nil
					}
					if stripeClient == nil {
						return "", fmt.Errorf("stripe client unavailable")
					}
					switch client := stripeClient.(type) {
					case unavailableStripeClient:
						return "", client.err
					default:
						_, isStub := stripeClient.(*stripelib.StubClient)
						return probeClient(ctx, "stripe", stripeClient, isStub, requiresStrictRuntimeDependencies(cfg.AppEnv))
					}
				},
			},
			{
				name: "postmark",
				check: func(ctx context.Context) (string, error) {
					if strings.TrimSpace(cfg.Postmark.ServerToken) == "" || strings.TrimSpace(cfg.Postmark.FromEmail) == "" {
						return "disabled", nil
					}
					if postmarkClient == nil {
						return "", fmt.Errorf("postmark client unavailable")
					}
					switch client := postmarkClient.(type) {
					case unavailablePostmarkClient:
						return "", client.err
					default:
						_, isStub := postmarkClient.(*postmark.StubClient)
						return probeClient(ctx, "postmark", postmarkClient, isStub, requiresStrictRuntimeDependencies(cfg.AppEnv))
					}
				},
			},
			{
				name: "keycloak",
				check: func(ctx context.Context) (string, error) {
					if !keycloakAdminConfigured(cfg) {
						return "disabled", nil
					}
					if kcClient == nil {
						return "", fmt.Errorf("keycloak admin client unavailable")
					}
					switch client := kcClient.(type) {
					case *keycloak.StubClient:
						if requiresStrictRuntimeDependencies(cfg.AppEnv) {
							return "", fmt.Errorf("stub keycloak admin client active in strict environment")
						}
						return "ready (stub)", nil
					case interface{ Ready(context.Context) error }:
						return probeReady(ctx, client.Ready)
					default:
						return "", fmt.Errorf("keycloak readiness probe unavailable")
					}
				},
			},
		},
	}
}

func keycloakAdminConfigured(cfg Config) bool {
	return strings.TrimSpace(cfg.Keycloak.AdminURL) != "" &&
		strings.TrimSpace(cfg.Keycloak.Realm) != "" &&
		strings.TrimSpace(cfg.Keycloak.ClientID) != "" &&
		strings.TrimSpace(cfg.Keycloak.ClientSecret) != ""
}

// readinessProbeTimeout bounds each dependency probe so a hung dependency
// cannot stall the readiness endpoint.
const readinessProbeTimeout = 2 * time.Second

// probeReady runs a dependency's Ready probe under readinessProbeTimeout and
// reports "ready" alongside the probe's error.
func probeReady(ctx context.Context, ready func(context.Context) error) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, readinessProbeTimeout)
	defer cancel()
	return "ready", ready(probeCtx)
}

// probeClient checks an external provider client: a stub is only acceptable
// outside strict environments; anything else must expose a Ready probe.
func probeClient(ctx context.Context, name string, client any, isStub, strict bool) (string, error) {
	if isStub {
		if strict {
			return "", fmt.Errorf("stub %s client active in strict environment", name)
		}
		return "ready (stub)", nil
	}
	prober, ok := client.(interface{ Ready(context.Context) error })
	if !ok {
		return "", fmt.Errorf("%s readiness probe unavailable", name)
	}
	return probeReady(ctx, prober.Ready)
}
