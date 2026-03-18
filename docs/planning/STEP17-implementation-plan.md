# STEP 17 — Background Jobs and Queues Implementation Plan

## Objective
Build the Asynq-style background job infrastructure using a clean interface/stub pattern
consistent with the existing codebase (Stripe, Postmark, and MinIO all use this pattern).

## Architecture Decisions
- **Pattern**: Queue interface + in-memory stub worker (goroutines + channels).
  The real Asynq/Redis implementation is a drop-in replacement behind the same interface.
- **No new external deps**: Go stdlib only (channels, goroutines, sync, time).
  Asynq can be wired in as a separate file when Redis is available — the interface is API-compatible.
- **Queues**: `imports`, `billing`, `email`, `reconcile` (+ `default` fallback).
- **Retry policy**: per-queue configurable with exponential backoff (stub uses fixed delay).
- **DLQ**: tasks exceeding max retries move to the dead-letter queue (in-memory list).
- **Admin visibility**: HTTP endpoints under `/api/v1/jobs` (owner/admin only).
- **Metrics**: in-memory counters for enqueued/succeeded/failed/dlq per task type.
- **Scheduled jobs**: periodic cron-style jobs (billing reconcile, integrity, cleanup) run
  at configurable intervals via a scheduler goroutine inside the worker.

## Files to Create
1. `server/internal/workers/queue.go` — Queue interface, task types, queue name constants
2. `server/internal/workers/metrics.go` — In-memory job execution metrics counters
3. `server/internal/workers/worker.go` — Worker server (goroutine pool, retry, DLQ)
4. `server/internal/workers/scheduler.go` — Periodic/cron job scheduler
5. `server/internal/workers/handlers/imports.go` — Parse imported meet file handler
6. `server/internal/workers/handlers/exports.go` — Generate SD3 export file handler
7. `server/internal/workers/handlers/email.go` — Send campaign batch handler
8. `server/internal/workers/handlers/billing.go` — Billing reconciliation + integrity handler
9. `server/internal/workers/handlers/cleanup.go` — Purge expired temp files handler
10. `server/internal/app/handlers/jobs.go` — HTTP admin visibility handlers
11. `server/internal/app/server_step17_test.go` — Tests for jobs endpoints

## Files to Modify
1. `server/internal/app/config.go` — Add `WorkerConfig` (Redis URL, concurrency, intervals)
2. `server/internal/app/server.go` — Wire `workers.Server` into app (15th param)
3. `server/cmd/api/main.go` — Start worker goroutine alongside HTTP server

## Job Inventory
| Task Type | Queue | Trigger | Handler |
|---|---|---|---|
| `imports:parse` | imports | On finalize import | handlers/imports.go |
| `exports:generate` | imports | On export request | handlers/exports.go |
| `email:campaign_batch` | email | On campaign send | handlers/email.go |
| `billing:reconcile` | reconcile | Nightly / on-demand | handlers/billing.go |
| `integrity:nightly` | billing | Nightly | handlers/billing.go |
| `cleanup:purge_expired` | default | Nightly | handlers/cleanup.go |
| `compliance:data_export` | default | On export request | handlers/cleanup.go |
| `compliance:data_deletion` | default | On deletion request | handlers/cleanup.go |

## Admin HTTP Endpoints
- `GET /api/v1/jobs` — list active/completed/failed tasks (filterable by queue/status)
- `GET /api/v1/jobs/queues` — per-queue stats (depth, processing, succeeded, failed)
- `GET /api/v1/jobs/dlq` — dead-letter queue contents
- `POST /api/v1/jobs/dlq/{taskID}/retry` — re-enqueue a DLQ task
- `DELETE /api/v1/jobs/dlq/{taskID}` — delete a DLQ task permanently

## Retry Policies (defaults)
| Queue | Max Retries | Backoff |
|---|---|---|
| imports | 5 | 30s, 2m, 10m, 30m, 2h |
| billing | 3 | 1m, 10m, 1h |
| email | 3 | 30s, 5m, 30m |
| reconcile | 2 | 5m, 30m |
| default | 3 | 30s, 5m, 30m |

## Progress Tracking
- [x] queue.go
- [x] metrics.go
- [x] worker.go
- [x] scheduler.go
- [x] handlers/imports.go
- [x] handlers/exports.go
- [x] handlers/email.go
- [x] handlers/billing.go
- [x] handlers/cleanup.go
- [x] handlers/jobs.go (HTTP)
- [x] config.go updates
- [x] server.go updates
- [x] main.go updates
- [x] tests
- [x] CLAUDE.md update
- [x] TODO.md checkboxes
