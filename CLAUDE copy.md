# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important Note:
IF the item you are tasked with requires ANY sort of front end development, call the frontend-design skill immediately. Do not proceed without this skill. In addition, when using frontend-design skill, you must follow the frontend_aesthetics rules.

<frontend_aesthetics>
You tend to converge toward generic, "on distribution" outputs. In frontend design, this creates what users call the "AI slop" aesthetic. Avoid this: make creative, distinctive frontends that surprise and delight. Focus on:
 
Typography: Choose fonts that are beautiful, unique, and interesting. Avoid generic fonts like Arial and Inter; opt instead for distinctive choices that elevate the frontend's aesthetics.
 
Color & Theme: Commit to a cohesive aesthetic. Use CSS variables for consistency. Dominant colors with sharp accents outperform timid, evenly-distributed palettes. Draw from IDE themes and cultural aesthetics for inspiration.
 
Motion: Use animations for effects and micro-interactions. Prioritize CSS-only solutions for HTML. Use Motion library for React when available. Focus on high-impact moments: one well-orchestrated page load with staggered reveals (animation-delay) creates more delight than scattered micro-interactions.
 
Backgrounds: Create atmosphere and depth rather than defaulting to solid colors. Layer CSS gradients, use geometric patterns, or add contextual effects that match the overall aesthetic.
 
Avoid generic AI-generated aesthetics:
- Overused font families (Inter, Roboto, Arial, system fonts)
- Clichéd color schemes (particularly purple gradients on white backgrounds)
- Predictable layouts and component patterns
- Cookie-cutter design that lacks context-specific character
 
Interpret creatively and make unexpected choices that feel genuinely designed for the context. Vary between light and dark themes, different fonts, different aesthetics. You still tend to converge on common choices (Space Grotesk, for example) across generations. Avoid this: it is critical that you think outside the box!
</frontend_aesthetics>

## Project Overview

**DARE Platform** — a full-stack swim club operations platform intended to replace TeamUnify/SportsEngine Motion. Covers member/household management, meet scheduling and entries, CL2/SD3/ZIP file interoperability, performance tracking, billing (Stripe), communications, and admin reporting.

**Current status:** Bootstrap, local infrastructure setup, DB migration foundation, API skeleton middleware foundation, STEP 5 auth/session/MFA/RBAC endpoints, STEP 6 membership/household foundations, STEP 7 groups/eligibility/registrations, a follow-up STEP 5 hardening pass, STEP 8 meet setup/commitments/entries, STEP 9 file interoperability data model, STEP 10 CL2/SD3/ZIP/HY3 parsing engine, STEP 11 SD3/SDIF export workflows, STEP 12 best times/performance history, STEP 13 billing core/ledger, STEP 14 Stripe integration, STEP 15 communications module, STEP 16 audit/compliance primitives, STEP 17 background jobs and queues, STEP 18 API contract and docs, STEP 21 reporting pack, STEP 22 security hardening, STEP 23 test strategy execution, STEP 25 observability & support readiness, STEP 29A persistent DB-backed repositories, STEP 29B artifact storage/async durability, STEP 29F parent/swimmer portal live-data conversion, STEP 29G document center, STEP 29H training/practices, STEP 29I deployment automation, STEP 29J domains/edge/operational controls, STEP 29K backup/restore/DR, STEP 29L performance/accessibility/compatibility, and STEP 29M training/support/launch simulation are complete. STEP 29A is now fully complete: all 9 domain services (members, groups, meets, imports, exports, performance, billing, comms, compliance) have DB-backed repositories using pgx/v5 with a write-through cache pattern — NewServiceWithPool(pool) loads all records from normalized Postgres tables into in-memory maps on startup, and every mutating operation persists to the DB before updating memory. The runtime_snapshots bridge remains for backward compat but DB writes are now canonical. The four STEP 29A TODO items are done and the STEP 29A planning doc checkpoints are complete. STEP 24 is partially complete: the migration packet now includes a source-to-target mapping template and required legacy export inventory under `docs/migration/`, and the repo now has a Step 24 migration CLI skeleton under `server/cmd/migrate` plus `server/internal/migration` that can dry-run CSV imports for households, swimmers, groups, historical best times, open invoices, and payment history with per-step idempotency and JSON validation report output. STEP 26 now has execution-readiness artifacts: `docs/planning/STEP26-cutover-execution-plan.md`, `docs/runbooks/cutover.md`, `docs/runbooks/hypercare.md`, `docs/migration/cutover-signoff-template.md`, plus `make cutover-rehearsal`, `make cutover-smoke`, `make cutover-certify`, and `make cutover-billing-check` wrappers for the migration rehearsal, smoke, certification, and reconciliation workflows. STEP 0 through STEP 18, STEP 21 through STEP 25, and STEP 29B from `TODO.md` are complete (STEP 24 is partially complete — dry-run execution on real data is blocked on a staging environment). Step 23 now includes a reusable Go service unit-test harness under `server/internal/testutil`, a `DATABASE_URL`/`psql`-driven real-DB integration harness, OpenAPI example contract checks, parser fixture-directory coverage under `internal/meetfiles/testdata/fixtures`, golden-file regression coverage for standard and extended SD3 exporter output plus deterministic import reports, HY3 import finalization/report integration coverage, RBAC matrix tests for sensitive routes, billing ledger invariant tests, P0 happy-path tests for registration-to-payment, meet-commit-to-SD3-export, and results-to-best-time updates, a duplicate-finalize chaos test, a large-import performance regression test, and a dedicated CI P0 gate. The repository now includes scaffolding, local Postgres/Redis/MinIO/Mailpit infrastructure, Goose-based migration tooling, identity + membership schema migrations, default role seeding, a Chi-based `/api/v1` router with auth, members household/swimmer flows, groups/eligibility/registrations, full meet lifecycle/commitment/entry workflows, file import/export job management with conflict resolution, the `internal/meetfiles` standalone Go parsing library for SD3/SDIF/CL2/ZIP/HY3 files, swimmer results and performance history, the billing core (invoices/payments/refunds/ledger), the communications module (announcements, email campaigns with Postmark, delivery tracking, suppression list), audit/compliance primitives, the background job infrastructure (in-memory worker server, scheduler, DLQ, metrics, admin endpoints), the core v1 reporting pack with JSON/CSV endpoints for membership status, meet fees, invoice aging, payment reconciliation, and registration status, plus hardened session/cookie handling, CSRF protection for cookie-backed auth mutations, baseline security headers, in-memory rate limiting, upload filename/size/MIME validation, malware-scan hook integration for imports, CI secret/dependency scanning, a v1 threat model document, the expanded Step 23 regression suite, the first Step 24 migration prep artifacts, the Step 26 cutover/hypercare launch packet, the Step 29A `runtime_snapshots` persistence bridge that loads/saves members, groups, meets, imports, exports, performance, billing, comms, compliance, and report artifact state via `DATABASE_URL` + `psql`, and the Step 29B real S3-compatible object storage integration plus queued export artifact generation and cleanup.

## PostgreSQL Runtime Notes (important)

- This repo supports two local DB runtime modes:
  - Docker mode: `make infra-up` starts Postgres 16 in compose.
  - Non-Docker mode: local PostgreSQL service (Postgres.app/Homebrew).
- `psql` alone is not enough; a running PostgreSQL server must be reachable on `DATABASE_URL`.
- Default fallback DSN in `Makefile` is `postgres://postgres:postgres@localhost:5432/aquify?sslmode=disable`.
- If local auth differs (common with Postgres.app), update `.env.local` `DATABASE_URL` to your local user and credentials.
- Connectivity checks:
  - `pg_isready -h 127.0.0.1 -p 5432`
  - `psql -h 127.0.0.1 -p 5432 -d postgres -c "select current_user, current_database();"`
- Migration commands:
  - `make db-up`
  - `make db-down`
  - `make db-status`

## Document Authority (conflict resolution order)

1. `DECISIONS.md` — locked architectural decisions; treat as ground truth
2. `docs/decisions/*.md` — ADR-style decision records created in STEP 0
3. `docs/planning/Developer Handoff Master Document.txt` — comprehensive implementation spec
4. `docs/planning/Plan.txt` — original scoping document
5. `TODO.md` — atomic task list; use to track implementation progress

If `SYSTEM.md` is present, it contains the current session instruction for what to work on next.

## Planned Tech Stack (locked unless noted)

**Backend:** Go 1.24, Chi router, PostgreSQL 16, sqlc + pgx, go-playground/validator, Asynq + Redis for task queues, S3-compatible object storage

**Frontend:** Next.js (TypeScript), Tailwind CSS, shadcn/ui, TanStack Query, React Hook Form + Zod

**Auth:** Keycloak (self-hosted OIDC) — **locked**

**Payments:** Stripe — config TBD (account ID, webhook secrets, refund policy not yet provided)

**Email:** Postmark (transactional)

**Observability:** OpenTelemetry, Prometheus + Grafana + Loki, Cloudflare (edge/WAF)

## Planned Monorepo Structure

```
root/
  server/             # Go API backend
  admin/              # Next.js admin portal
  db/migrations/      # PostgreSQL migrations
  internal/meetfiles/ # CL2/SD3/ZIP parsing engines
  docs/               # Planning docs and OpenAPI spec
  Makefile            # Dev targets (make dev, test, lint, infra-up/down/reset, db-up/down/status)
  docker-compose.yml  # Local infra: Postgres, Redis, MinIO (S3), Mailpit
  .env.example
```

## Current Bootstrap Artifacts (Implemented)
See current_artifacts.md

## Key Architectural Patterns

**Database conventions:** Every table has `id`, `created_at`, `updated_at`, `created_by`, `updated_by`. Business tables include `org_id` for multi-tenancy. Financial/audit tables are append-only. Soft deletes use `deleted_at`/`deleted_by`.

**API:** Versioned at `/api/v1`, OpenAPI 3.1 spec in `/docs/api/openapi.yaml`, consistent error shape, cursor or offset pagination, idempotency keys on mutations.

**Auth/RBAC:** OIDC via Keycloak. Roles: owner, admin, coach, parent, swimmer (read-only), billing, read-only staff. Deny-by-default on all routes; resource-scope checks required. MFA required for admin/coach privileged actions (refunds, etc.).
- OIDC callback now performs provider token exchange and validates signed state + nonce/audience/expiry before issuing sessions.
- Local `stub-*` bearer tokens are gated by `AUTH_ENABLE_DEV_STUB_TOKENS` and should be disabled outside development.
- MFA challenge no longer accepts a fixed static code; verification is factor-based (`totp` and `backup_code`).

**Portal account policy (locked):** Parent account is required for all household-critical workflows. Swimmer account is optional and must not be required for registration, commitments, billing, or document/compliance completion.
**Meet entry policy (locked):** Parent commitment `yes` must allow per-swimmer event selection, and coaches may override selections before lock with required audit logging.

**State machines:** Meet, Commitment, Entry, Invoice, Payment, and File Import each have explicit lifecycle states with validated transitions — see Developer Handoff Master Document for full transition tables.
- Meet: `draft → published → commitment_open → commitment_closed → entry_finalized → completed → archived`
- Commitment: `invited → committed_yes | committed_no → coach_reviewed → locked`
- Entry: `draft → submitted → approved → exported → updated_from_results → archived`
- File Import: `uploaded → scanning → parsing → matching → conflicts_pending → applied → failed → archived`
- Match Conflict: `open → resolved_manual | resolved_auto | rejected → closed`
- File Export: `pending → generating → ready → failed → expired`

**File interoperability:** `internal/meetfiles/` handles SD3, CL2, ZIP, and HY3 parsing. HY3 is treated as a ZIP container; the inner SD3 or CL2 payload is selected by deterministic precedence (SD3 > CL2, then lexicographic) and routed to the appropriate parser. Matching logic: swimmer ID first, then name+DOB+gender fallback. Produces deterministic import reports and conflict queues.

**Async jobs (Asynq queues):** `imports`, `billing`, `email`, `reconcile` — each with retry policies and DLQ visibility.

**Billing ledger:** Append-only `ledger_events` table. Stripe webhooks processed idempotently; nightly reconciliation job alerts on mismatch.
**Reporting pack:** `/api/v1/reports/membership-status`, `/api/v1/reports/meet-fees`, `/api/v1/reports/invoice-aging`, `/api/v1/reports/payment-reconciliation`, and `/api/v1/reports/registration-status` are implemented. Each endpoint supports JSON by default and CSV via `?format=csv`, and each successful run emits a report audit event. Canonical report view definitions live in `docs/planning/STEP21-report-view-definitions.md`.

## Observability Runtime Notes (STEP 25)

- OpenTelemetry tracing is initialised at startup by `server/internal/telemetry`. When `OTEL_EXPORTER_OTLP_ENDPOINT` is not set, a no-op tracer is used (safe for tests and local dev).
- Prometheus metrics are exposed at `/metrics` (port same as the API). In production, firewall this path to the Prometheus scraper IP range.
- New env vars:
  - `OTEL_SERVICE_NAME` (default: `dare-api`)
  - `OTEL_SERVICE_VERSION` (default: `dev`)
  - `OTEL_EXPORTER_OTLP_ENDPOINT` — OTLP HTTP endpoint; leave blank to disable tracing
  - `OTEL_TRACES_SAMPLER_ARG` — float 0.0–1.0 sampling rate (default: `1.0`)
  - `METRICS_ADDR` — address for the Prometheus metrics port (currently unused; metrics served on API port at `/metrics`)
- Every HTTP request log line now includes `user_id`, `org_id`, `trace_id`, and `span_id` from context when available.
- Background job handlers (`parse_import`, `billing_reconcile`, `nightly_integrity`) now create OTel spans with relevant attributes.
- Grafana dashboard JSON files live in `docs/observability/grafana/`.
- Prometheus alert rules live in `docs/observability/prometheus/alerts.yaml`.
- On-call runbook lives in `docs/runbooks/on-call.md`.

## Design System

Defined in `design-language-guide.html`. Brand accent: `#eb5d1e` (orangey-red). Fonts: Instrument Serif (display), Plus Jakarta Sans (body), JetBrains Mono (code). Dark mode supported.

## Frontend Routing Notes

- Admin legacy dashboard-prefixed routes are preserved via redirects:
  - `/dashboard/commitments` -> `/commitments`
  - `/dashboard/entries` -> `/entries`
  - `/dashboard/performance/visualization` -> `/performance/visualization`
- Parent meet detail page at `/portal/meets/[id]` includes meet info, meet files, and session-grouped event signup.
- Swimmer meet detail page is available at `/swim/meets/[id]` with meet files and session-grouped swimmer-facing meet selection UI.
- Step 29E admin live-data conversion is complete:
  - admin list/detail screens for households, swimmers, groups, registrations, meets, imports, exports, invoices, ledger, settings, reports, audit, jobs, billing, performance, compliance, announcements, campaigns, and suppression now source their primary content from live backend APIs instead of hard-coded mock datasets
  - member/group/meet/import/invoice detail pages remain free of the disabled placeholder launch-critical actions that were removed during remediation
  - the frontend now consumes the broader Step 29E backend slice, including:
    - member/registration/group detail reads
    - meet/import/invoice/ledger detail reads
    - runtime settings and role-assignment reads
    - report summary and artifact generation endpoints
    - audit event reads
    - jobs queue/metrics/DLQ reads and DLQ retry/delete mutations
    - announcement and campaign list/detail reads plus draft/publish/archive/create/send flows
    - compliance export/deletion request reads and create flows
    - suppression list/create/delete flows
    - swimmer best-time/progression reads and time-standard CRUD flows
  - targeted Go API coverage, Step 29E admin helper regression tests, and the `admin` production build all pass for this slice
- Step 29F parent portal and swimmer portal live-data conversion is complete:
  - all portal pages in `admin/app/(portal)/portal/` now use live API-backed state: dashboard, household, household/swimmers/[id], registrations, meets, meets/[id], billing, billing/invoices/[id], communications, and calendar
  - all swimmer portal pages in `admin/app/(swimmer)/swim/` now use live API-backed state: swim (dashboard), performance, schedule, profile, meets/[id]
  - swimmer identity is resolved by filtering `fetchSwimmers` by `household_id === session.householdId`, first match used
  - best times are pivoted from `BestTimeRecord[]` by (event_distance + stroke → event key) × course (SCY/SCM/LCM)
  - commitment status mapping: `invited` → pending, `committed_yes` → yes, `committed_no` → no, `coach_reviewed/locked` → closed
  - documents page is now live-data backed via Step 29G (document_types + document_submissions APIs); registrations/new page still uses stub data

## Definition of Done (apply to every task)

- Code merged with tests
- API/docs updated if behavior changed
- Audit events included for privileged actions
- Role checks enforced and tested
- Metrics/logging added for new background jobs
- Feature flag or rollback plan documented

## Auth Hardening Runtime Configuration

- `APP_ENV` defaults to `development`.
- `AUTH_ENABLE_DEV_STUB_TOKENS` defaults to `true` in local development and must be set to `false` for non-dev environments.
- `AUTH_ALLOW_DEV_SESSION_ISSUE` defaults to `true` only in local development/test flows and must remain `false` for non-dev environments.
- OIDC browser login requires: `OIDC_AUTHORIZE_URL`, `OIDC_TOKEN_URL`, `OIDC_JWKS_URL`, `OIDC_REDIRECT_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and `OIDC_STATE_SIGNING_KEY`.
- Optional OIDC settings: `OIDC_EXPECTED_ISSUER`, `OIDC_HTTP_TIMEOUT` (default `10s`).
- Session endpoints now also issue cookie-backed sessions:
  - access cookie: `aquify_access_token` (`Secure`, `HttpOnly`, `SameSite=Strict`)
  - refresh cookie: `aquify_refresh_token` (`Secure`, `HttpOnly`, `SameSite=Strict`)
  - CSRF cookie: `aquify_csrf_token` (`Secure`, readable by browser JS, `SameSite=Strict`)
- The frontend now bootstraps signed-in state from `GET /api/v1/auth/session`; this route accepts cookie auth without requiring `X-Org-Id`.
- `GET /api/v1/auth/mfa/factors` returns the enrolled factors for the current user so the browser MFA page can complete real step-up flows.
- `POST /api/v1/auth/sessions/issue` is a development/test-only bootstrap route and must stay disabled in non-development environments.
- Cookie-backed `POST /api/v1/auth/sessions/refresh` and `POST /api/v1/auth/sessions/revoke` require the CSRF cookie value in header `X-CSRF-Token` by default.
- `middleware.Auth` accepts either `Authorization: Bearer ...` or the `aquify_access_token` cookie.
- Frontend dev stub login/role switching must be gated by `NEXT_PUBLIC_APP_ENV != production` plus `NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS != false`; production should not rely on `NODE_ENV` alone.

## Security Runtime Notes

- Baseline response headers are set by middleware: CSP, `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, and HSTS on HTTPS requests.
- In-memory rate limiting is enabled:
  - global default: `240` requests per `1m`
  - auth-route default: `20` requests per `1m`
- New security env vars:
  - `SECURITY_CSRF_HEADER_NAME`
  - `SECURITY_RATE_LIMIT_GLOBAL_REQUESTS`
  - `SECURITY_RATE_LIMIT_GLOBAL_WINDOW`
  - `SECURITY_RATE_LIMIT_AUTH_REQUESTS`
  - `SECURITY_RATE_LIMIT_AUTH_WINDOW`
  - `SECURITY_UPLOAD_MAX_BYTES`
  - `SECURITY_UPLOAD_ALLOWED_MIMES`
  - `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK`
  - `SECURITY_IMPERSONATION_ENABLED`
- Import upload hardening now validates:
  - safe basename-only filenames
  - declared file size against the configured cap
  - declared MIME type against the allowlist
  - malware-scan hook result before finalization continues
  - `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK=true` rejects missing or `stub` scanners
- Admin impersonation is explicitly rejected by default. If `SECURITY_IMPERSONATION_ENABLED=true`, owner role, MFA, and a reason are still required, and the request is rejected as not implemented until full safeguards land.

## Make Quality Gates

- `make test` runs `go test ./...` in `server/` and `npm run build` in `admin/`.
- `make lint` runs `go vet ./...` in `server/` and `npm run lint` in `admin/`.

## Runtime Persistence Notes (STEP 29A — complete)

- Step 29A planning notes live in `docs/planning/STEP29A-persistence-execution-plan.md` and `docs/planning/STEP29A-db-repo-implementation.md`.
- All 9 domain services now have DB-backed repositories using pgx/v5 (`github.com/jackc/pgx/v5/pgxpool`).
- When `DATABASE_URL` is set, `server.go` calls `NewServiceWithPool(pool)` for each service. On startup each service loads all records from the normalized Postgres tables into in-memory maps. Every subsequent write persists to the DB before updating memory (write-through cache).
- In-memory mode (nil pool / no DATABASE_URL) is preserved for all existing tests.
- Per-domain repository files live in `server/internal/app/<domain>/repository.go`.
- DB pool initialization lives in `server/internal/db/pool.go`.
- Integration tests for the DB-backed round-trip are in `server/internal/app/server_db_repos_test.go`.
- The legacy `runtime_snapshots` JSON bridge (`server/internal/runtimepersist`) is retained for backward compatibility but DB writes via the repositories are now the canonical persistence path.
- The `server/internal/app/runtime_persistence_integration_test.go` tests the original snapshot bridge and continues to pass.

## Artifact Storage Notes (STEP 29B)

- Step 29B tracking/notes live in `docs/planning/STEP29B-artifact-durability-plan.md`.
- `server/internal/storage` now supports a real MinIO/S3-compatible client using `github.com/minio/minio-go/v7`, with an in-memory stub retained for tests and fallback development flows.
- Storage config accepts both the repo's `STORAGE_*` env vars and the local MinIO-style aliases already used in `.env.local.example` (`S3_ENDPOINT`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_REGION`, `MINIO_BUCKET`).
- Import upload URLs now come from the object storage client, and import finalization verifies that the uploaded object exists in storage before continuing.
- Export requests now create a pending job and enqueue `workers.TypeGenerateExport`; the background worker generates the SD3/SDIF payload, uploads it to object storage, and then marks the export `ready`.
- Generated CSV report requests now persist the CSV to object storage and return artifact metadata plus a presigned download URL instead of streaming inline CSV bytes.
- The cleanup worker now expires stale import upload windows plus expired export/report artifacts and deletes their storage objects.

## Provider Runtime Notes (STEP 29C)

- Step 29C tracking/notes live in `docs/planning/STEP29C-provider-hardening-plan.md`.
- `server/internal/stripe` now supports a real Stripe SDK client alongside the stub. Local/test flows may use the stub client, but staging/production startup must fail if Stripe cannot initialize from `STRIPE_SECRET_KEY`.
- Stripe webhook signature verification still requires `STRIPE_WEBHOOK_SECRET`.
- Billing refund creation now performs local eligibility validation before attempting a real Stripe refund for Stripe-backed payments, then records the returned `stripe_refund_id` on the local refund record.
- Postmark outbound sends now attach `org_id` and `campaign_id` metadata for webhook correlation.
- Local/test flows may use the Postmark stub client, but staging/production startup must fail if Postmark cannot initialize from `POSTMARK_SERVER_TOKEN` and `POSTMARK_FROM_EMAIL`.
- `POST /api/v1/webhooks/postmark` now applies Postmark `Delivery`, `Bounce`, `SpamComplaint`, and `SubscriptionChange` events back into delivery/suppression state, including scoped bounce/complaint suppression updates when org/email metadata is present but the delivery row is missing.
- Production Postmark webhooks should be protected with either:
  - `POSTMARK_WEBHOOK_AUTH_HEADER_NAME` + `POSTMARK_WEBHOOK_AUTH_HEADER_VALUE` using Postmark custom webhook headers, or
  - `POSTMARK_WEBHOOK_BASIC_USER` + `POSTMARK_WEBHOOK_BASIC_PASS` using Postmark webhook HTTP auth.

## Migration Prep Notes

- Step 24 planning/status is tracked in `docs/planning/STEP24-migration-prep-plan.md`.
- Migration packet operator docs live in `docs/migration/`.
- `server/cmd/migrate` currently supports:
  - `go run ./cmd/migrate plan`
  - `go run ./cmd/migrate dry-run -input-dir <dir> -report <file>`
- The migration CLI currently expects these CSV files in the input directory:
  - `households.csv`
  - `swimmers.csv`
  - `groups.csv`
  - `historical_best_times.csv`
  - `open_invoices.csv`
  - `payment_history.csv`
- Dry-run output is a JSON validation report with per-step processed/imported/skipped/duplicate counts plus warnings/errors.
- Remaining Step 24 items are blocked on real legacy exports and a staging environment: anonymized dry-run execution, source-system row-count comparison, billing total comparison, and manual remediation logging.

## Cutover Readiness Notes

- Step 26 planning/status is tracked in `docs/planning/STEP26-cutover-execution-plan.md`.
- Cutover operator runbooks live in `docs/runbooks/cutover.md` and `docs/runbooks/hypercare.md`.
- Stakeholder approvals should be recorded in `docs/migration/cutover-signoff-template.md`.
- New helper commands:
  - `make cutover-rehearsal INPUT_DIR=/absolute/path/to/export-dir`
  - `make cutover-smoke API_BASE_URL=https://api.example.com AUTH_TOKEN=... ORG_ID=...`
  - `make cutover-certify`
- `make cutover-billing-check API_BASE_URL=https://api.example.com AUTH_TOKEN=... ORG_ID=...`
- These artifacts prepare Step 26 execution, but the actual cutover checklist items in `TODO.md` remain real-world launch actions and should only be marked complete during the production cutover window.

## Document Center Notes (STEP 29G — complete)

- Step 29G planning/status lives in `docs/planning/STEP29G-document-center-plan.md`.
- New `server/internal/app/documents/` package provides the document center service with DB-backed write-through cache (same pattern as other 29A services).
- Two new DB migrations: `20260322000100_create_document_types.sql` and `20260322000200_create_document_submissions.sql`.
- HTTP endpoints:
  - `GET/POST /api/v1/document-types` — list/create document type catalog (owner/admin CRUD; parent/coach read)
  - `PUT/DELETE /api/v1/document-types/{typeID}` — update/soft-delete (owner/admin)
  - `GET/POST /api/v1/documents` — list/create submissions (parent sees own household only)
  - `GET /api/v1/documents/{id}` — get submission
  - `POST /api/v1/documents/{id}/attach` — attach uploaded file, transitions to `submitted`
  - `POST /api/v1/documents/{id}/verify` — admin verify, transitions to `verified`
  - `POST /api/v1/documents/{id}/reject` — admin reject, transitions back to `rejected`
- Document submission state machine: `pending → submitted → verified | rejected` (rejected → submitted on re-attach)
- Admin UI at `/compliance/documents` manages document type catalog and reviews submissions.
- Parent portal `/portal/documents` now fetches live document types and household submissions; upload stubs call the attach endpoint.

## Training / Practices Notes (STEP 29H — complete)

- DB migration: `db/migrations/20260323000100_create_practices.sql` — `practices` table with org_id, date, group_id, type, description, yardage_target, attendance, per-stroke yardage columns, notes, and audit fields.
- Go service: `server/internal/app/training/` — write-through cache pattern (same as documents); `NewService()` / `NewServiceWithPool(pool)` constructors.
- HTTP handlers: `server/internal/app/handlers/training.go` — `GET/POST /api/v1/practices` and `GET/PUT/DELETE /api/v1/practices/{id}`. Roles: coach/admin/owner write; coach/admin/owner/parent/swimmer read.
- `NewRouterWithServices` now accepts a 16th `trainingService *training.Service` parameter (before `workerServer`). All test call sites updated.
- Frontend:
  - `admin/lib/adminData.ts`: Added `PracticeRecord`, `fetchPractices`, `createPractice`, `updatePractice`, `deletePractice`.
  - Admin training page (`admin/app/(admin)/training/page.tsx`): Wired to real API; removed DEMO_WORKOUTS; add/delete via API with loading states.
  - Parent calendar (`admin/app/(portal)/portal/calendar/page.tsx`): Now fetches practices and includes them as `'practice'` CalEvents alongside meets.
  - Swimmer schedule (`admin/app/(swimmer)/swim/schedule/page.tsx`): Now fetches practices and shows them in the upcoming events list; removed "practices not shown here" banner; added 'practice' to EventType and filter buttons.
- Tests: `server/internal/app/server_training_test.go` — CRUD, role enforcement, org isolation, invalid type.

## Deployment and Operations Notes (STEP 29I–29M — complete)

- Planning notes live in `docs/planning/STEP29IJKLM-deployment-ops-plan.md`.
- **Docker Compose deploy files:** `deploy/docker-compose.staging.yml` and `deploy/docker-compose.production.yml` — all services (API, admin, worker, Postgres, Redis, MinIO, Keycloak, Prometheus, Grafana, Loki, Mailpit [staging only]).
- **Dockerfiles:** `server/Dockerfile` (distroless runtime, non-root) and `admin/Dockerfile` (Next.js standalone mode). `admin/next.config.ts` has `output: 'standalone'` set.
- **Process supervision:** `deploy/systemd/dare.service` + `dare-backup.service` + `dare-backup.timer` for VPS deployment.
- **Deploy scripts:** `deploy/scripts/promote.sh` (rolling deploy with auto-rollback on health failure), `deploy/scripts/rollback.sh`, `deploy/scripts/backup.sh` (pg_dump + MinIO + GPG + rclone offsite), `deploy/scripts/restore.sh`, `deploy/scripts/smoke.sh`.
- **CI/CD:** `.github/workflows/deploy-staging.yml` (auto on main merge) and `.github/workflows/deploy-production.yml` (manual workflow_dispatch with DEPLOY confirm + GitHub environment approval).
- **Domain plan:** `docs/ops/domain-plan.md` — 7 subdomains with Cloudflare Full Strict TLS.
- **Cloudflare WAF:** `docs/ops/cloudflare-waf-config.md` — OWASP CRS + custom rules matching backend rate limits.
- **Staging guide:** `docs/ops/staging-environment-guide.md` — port allocation, env config, smoke-test gate.
- **Rollback procedure:** `docs/ops/rollback-procedure.md`.
- **Operator config surfaces:** `docs/ops/operator-config-surfaces.md` — 3-tier classification.
- **Backup/DR:** `docs/ops/backup-restore-runbook.md` (RTO < 4h, RPO < 24h), `docs/ops/disaster-recovery-runbook.md` (5 DR scenarios), `docs/ops/restore-drill-checklist.md`.
- **Load tests:** `tests/load/k6/` — 4 k6 scripts (admin-browse, import-upload, webhook-burst, portal-load) with thresholds.
- **Accessibility audit:** `docs/testing/accessibility-audit.md` — WCAG 2.2 AA checklist with 10 pre-launch issues.
- **Browser matrix:** `docs/testing/browser-compatibility-matrix.md`.
- **Training docs:** `docs/training/` — operator guide, product admin guide, finance guide, support runbook, launch owner matrix, manual workflow validation checklist.

## Known Production Readiness Gaps

- Step 29A planning/status is tracked in `docs/planning/STEP29A-persistence-execution-plan.md`.
- Step 29B planning/status is tracked in `docs/planning/STEP29B-artifact-durability-plan.md`.
- Step 29 is intentionally chunked in `TODO.md` into sub-steps `29A` through `29N` so production-readiness work can be executed in small, auditable batches.
- The app is not yet a fully ready production system because Step 29C onward is still open, several provider/runtime integrations are still scaffolded or stub-backed, and the current Step 29A persistence layer is a whole-domain snapshot bridge rather than the final normalized repository implementation.
- Admin, parent portal, and swimmer portal UI were all converted to live API-backed state in Step 29E (admin) and Step 29F (parent + swimmer portals). Step 29G added the document center backend and wired the parent portal `documents/page.tsx` to live APIs. The `registrations/new/page.tsx` in the parent portal still uses stub state because no backend API for self-service registration creation exists yet.
- Real launch readiness still requires finishing Step 24 migration validation, Step 26 cutover execution, and Step 27 stabilization in addition to the new Step 29 backlog.

## Admin Panel Fix Notes (STEP 99A — complete)

- **Root cause of 403 Forbidden**: Frontend `DEV_SESSIONS` used `orgId: 'org-dev'` but backend stub principals used `OrgID: "org-demo"`. The `membershipScopeFromRequest` helper in handlers enforces `principal.OrgID == X-Org-Id header`, so all protected API calls returned 403. Fixed by updating `admin/lib/session.tsx` DEV_SESSIONS to use `orgId: 'org-demo'` to match the backend.
- **Dashboard** (`admin/app/(admin)/dashboard/page.tsx`): Converted from server component to client component; now fetches live stats from `fetchSwimmers`, `fetchHouseholds`, `fetchMeets`, `fetchInvoices`, `fetchImports`, `fetchGroups`, and `fetchAuditEvents`. Shows loading skeletons while fetching.
- **Commitments** (`admin/app/(admin)/commitments/page.tsx`): Fully live-data backed. Fetches all meets, swimmers, and households, then fetches commitments per meet (in parallel for meets in commitment/entry statuses). Joins swimmer names and household names from the combined data. Drops `respondedAt` and `eventSelections` columns (not available in `CommitmentRecord` from the API).
- **Entries** (`admin/app/(admin)/entries/page.tsx`): Fully live-data backed. Fetches meets, swimmers, then entries and events per meet in parallel. Formats seed times from hundredths-of-seconds. Event names built from distance + stroke + gender/age_group.
- **Practice Scheduler** (`admin/app/(admin)/practices/page.tsx`): No backend API exists for practices; added a demo-data warning banner. UI remains functional for demonstration.
- **Autopay** (`admin/app/(admin)/billing/autopay/page.tsx`): No backend API for autopay management; added a demo-data warning banner.
- **Stripe billing** (`admin/app/(admin)/billing/stripe/page.tsx`): Removed hardcoded mock data; now fetches live payment reconciliation data from `fetchPaymentReconciliationReport`. Removed Webhook Events tab (no webhook log API); shows live reconciliation cards, unreconciled row count, and alert state.

## UI Improvements (STEP 99B — complete)

- **Login page** (`admin/app/login/page.tsx`): Redesigned to a simple centered card — removed the left marketing/visual panel entirely. Dark minimal centered box with grain background, same SSO flow and dev stub section preserved.
- **Landing page** (`admin/app/page.tsx`): Root `/` is now a barebones landing page (DARE wordmark + tagline + "Sign in →" button) instead of a server-side redirect to `/login`. Placeholder for future marketing content.
- **Dashboard performance snapshot** (`admin/app/(admin)/dashboard/page.tsx`): Added a "Performance Snapshot" section at the bottom of the dashboard. Fetches `fetchPerformanceBestTimes` in parallel with other dashboard data. Shows top 5 events by swimmer coverage with mini bar charts, and top 5 swimmers by best-time count.
- **Commitment notes** (`admin/app/(portal)/portal/meets/[id]/page.tsx`): Parents/swimmers can now add an optional "Message for coaches" textarea when committing yes/no. Note is included in the commitment respond API call body (`note` field). Shows read-only when commitment is locked. `CommitmentRecord.note` field added to `adminData.ts`.
- **Commitments swimmer drill-down** (`admin/app/(admin)/commitments/page.tsx`): Clicking a swimmer name in the commitments table opens a right-side slide-over panel showing: swimmer info, commitment/meet status badges, family note (if present), event selections (read-only with checkmarks for events that have entries), and entry records. Panel lazy-loads events and entries per meet with caching. Footer has "Open Meet" and "Swimmer Profile" action buttons.
- **Entries add-entry panel** (`admin/app/(admin)/entries/page.tsx`): Added "Add Entry" button in the entries page header. Opens a 3-step slide-over: (1) select swimmer, (2) select meet (only ENTRY_STATUSES meets), (3) select events from a gender-filtered checklist per session. Saves by POSTing to `/api/v1/orgs/{orgId}/meets/{meetId}/entries` for each selected event. Refreshes the entries list on success.

## Admin UI Enhancements (STEP 99D — complete)

- **Resizable sidebar** (`admin/components/layout/Sidebar.tsx`): Drag handle added to the right edge of the sidebar. Drag to any width between 160px–380px; width is persisted to `localStorage` (`aquify-sidebar-width`) and synced to `--sidebar-w` CSS variable. Collapse toggle (52px) is preserved. Transitions are disabled during active drag to prevent jitter.
- **Seasons page** (`admin/app/(admin)/seasons/page.tsx`): New page at `/seasons` under System nav (owner/admin). Shows current and next season cards with editable name, start date, end date, and status (active/upcoming/completed). Includes a progress bar for the active season, a season goals textarea, and a timeline overview visualization. Seasons use local state (no backend API yet).
- **Training page** (`admin/app/(admin)/training/page.tsx`): New page at `/training` under a new "Training" nav section (owner/admin/coach). Two tabs — Schedule (log workouts with date, group, type, description, yardage, attendance, per-stroke yardage breakdown) and Stats (weekly per-group stats cards with stroke breakdown bars and attendance). Uses live groups from the API with a fallback to demo groups.
- **Session builder fix** (`admin/app/(admin)/meets/new/page.tsx`): Replaced the unlabeled 5-column input grid with properly labeled `SessionCard` components. Each card shows Day Label, Session Name, Date, Warm-up Time, and Start Time with FieldLabel above each input. Remove button is per-card and only visible when more than one session exists.
- **Meet file uploads** (`admin/app/(admin)/meets/new/page.tsx`): Added two upload zones to the Basic Info step — "Meet Package Files" (PDF/images/docs) and "Meet Entry File" (.cl2/.sd3/.zip/.hy3). The entry file zone simulates parsing and auto-populates sessions in the Schedule step with a success banner. File lists show name + size with individual remove buttons.
- **Nav updated** (`admin/lib/roles.ts`): Added `training` nav item to new "Training" section (owner/admin/coach) and `seasons` nav item to System section (owner/admin). `NAV_SECTIONS` updated to include `Training`.
