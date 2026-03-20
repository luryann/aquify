# STEP 25 — Observability & Support Readiness: Implementation Plan

## Goal
Instrument the DARE platform API and background jobs with OpenTelemetry tracing and
Prometheus metrics, enrich structured logging, ship Grafana dashboard definitions,
Prometheus alert rules, and produce an on-call runbook.

## Tasks

| # | Task | Status |
|---|------|--------|
| 1 | Add OpenTelemetry SDK and Prometheus client to go.mod | pending |
| 2 | Create `server/internal/telemetry` package (tracer + meter setup) | pending |
| 3 | Add OTel HTTP middleware for API request tracing | pending |
| 4 | Add Prometheus `/metrics` endpoint | pending |
| 5 | Add tracing spans inside ParseImportHandler | pending |
| 6 | Add tracing spans inside BillingReconcileHandler | pending |
| 7 | Enhance StructuredLogger middleware with org_id / user_id from context | pending |
| 8 | Grafana dashboard: API error rates | pending |
| 9 | Grafana dashboard: queue depth and job failures | pending |
| 10 | Grafana dashboard: import success/failure rates | pending |
| 11 | Grafana dashboard: webhook processing lag | pending |
| 12 | Prometheus alert rules: import failure spike | pending |
| 13 | Prometheus alert rules: payment webhook failure spike | pending |
| 14 | Prometheus alert rules: DB connection saturation | pending |
| 15 | On-call runbook document | pending |
| 16 | Update TODO.md to mark STEP 25 complete | pending |
| 17 | Update CLAUDE.md with STEP 25 status | pending |

## Architecture Decisions

### Telemetry Package
- `server/internal/telemetry/` — setup OTel SDK (OTLP gRPC or HTTP exporter, configurable via env)
- Exports: `SetupTracer(cfg TelemetryConfig) (shutdown func(), err error)`
- Exports: `SetupMeter()` for Prometheus-compatible metrics
- Env vars: `OTEL_SERVICE_NAME`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_TRACES_SAMPLER`, `METRICS_ADDR`
- No-op tracer when endpoint not configured (safe default for tests)

### HTTP Instrumentation
- Wrap the chi router with `otelhttp.NewHandler` (contrib/instrumentation/net/http/otelhttp)
- Each span tagged with: `http.method`, `http.route`, `http.status_code`, `org.id`, `user.id`

### Prometheus Metrics (custom counters/histograms)
- `http_requests_total{method,route,status}` — counter
- `http_request_duration_seconds{method,route}` — histogram
- `job_executions_total{type,status}` — counter
- `job_duration_seconds{type}` — histogram
- `import_files_total{status}` — counter
- `webhook_processing_lag_seconds` — histogram
- Expose via `/metrics` endpoint (Prometheus text format)

### Structured Logging Enrichment
- Add `org_id` and `user_id` from request context (via auth middleware) to every log line
- Auth middleware already sets claims on context; logging middleware reads them

### Dashboards (Grafana JSON)
- `docs/observability/grafana/api-error-rates.json`
- `docs/observability/grafana/queue-depth-job-failures.json`
- `docs/observability/grafana/import-success-failure.json`
- `docs/observability/grafana/webhook-lag.json`

### Prometheus Alert Rules
- `docs/observability/prometheus/alerts.yaml`
  - `ImportFailureSpike` — >10 failures in 5m
  - `PaymentWebhookFailureSpike` — >5 webhook failures in 5m
  - `DBConnectionSaturation` — pg_stat_activity count > 80% of max_connections

### On-call Runbook
- `docs/runbooks/on-call.md`

## Files to Create/Modify

**New:**
- `server/internal/telemetry/telemetry.go`
- `server/internal/telemetry/metrics.go`
- `server/internal/app/middleware/telemetry.go`
- `docs/observability/grafana/api-error-rates.json`
- `docs/observability/grafana/queue-depth-job-failures.json`
- `docs/observability/grafana/import-success-failure.json`
- `docs/observability/grafana/webhook-lag.json`
- `docs/observability/prometheus/alerts.yaml`
- `docs/runbooks/on-call.md`

**Modified:**
- `server/internal/app/middleware/logging.go` — enrich with org_id/user_id
- `server/internal/workers/jobhandlers/imports.go` — add tracing span
- `server/internal/workers/jobhandlers/billing.go` — add tracing span
- `server/internal/app/server.go` — wire OTel middleware + /metrics endpoint
- `server/internal/app/config.go` — add TelemetryConfig
- `server/go.mod` — add OTel + Prometheus deps
- `TODO.md` — mark tasks done
- `CLAUDE.md` — update status
