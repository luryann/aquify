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

**Current status:** Bootstrap, local infrastructure setup, DB migration foundation, API skeleton middleware foundation, STEP 5 auth/session/MFA/RBAC endpoints, STEP 6 membership/household foundations, STEP 7 groups/eligibility/registrations, a follow-up STEP 5 hardening pass, STEP 8 meet setup/commitments/entries, STEP 9 file interoperability data model, STEP 10 CL2/SD3/ZIP/HY3 parsing engine, STEP 11 SD3/SDIF export workflows, STEP 12 best times/performance history, STEP 13 billing core/ledger, STEP 14 Stripe integration, STEP 15 communications module, STEP 16 audit/compliance primitives, STEP 17 background jobs and queues, STEP 18 API contract and docs, and STEP 21 reporting pack are complete. STEP 0 through STEP 18 and STEP 21 from `TODO.md` are complete; repository now includes scaffolding, local Postgres/Redis/MinIO/Mailpit infrastructure, Goose-based migration tooling, identity + membership schema migrations, default role seeding, a Chi-based `/api/v1` router with auth, members household/swimmer flows, groups/eligibility/registrations, full meet lifecycle/commitment/entry workflows, file import/export job management with conflict resolution, the `internal/meetfiles` standalone Go parsing library for SD3/SDIF/CL2/ZIP/HY3 files, swimmer results and performance history, the billing core (invoices/payments/refunds/ledger), the communications module (announcements, email campaigns with Postmark, delivery tracking, suppression list), audit/compliance primitives, the background job infrastructure (in-memory worker server, scheduler, DLQ, metrics, admin endpoints), and the core v1 reporting pack with JSON/CSV endpoints for membership status, meet fees, invoice aging, payment reconciliation, and registration status.

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

## Design System

Defined in `design-language-guide.html`. Brand accent: `#eb5d1e` (orangey-red). Fonts: Instrument Serif (display), Plus Jakarta Sans (body), JetBrains Mono (code). Dark mode supported.

## Frontend Routing Notes

- Admin legacy dashboard-prefixed routes are preserved via redirects:
  - `/dashboard/commitments` -> `/commitments`
  - `/dashboard/entries` -> `/entries`
  - `/dashboard/performance/visualization` -> `/performance/visualization`
- Parent meet detail page at `/portal/meets/[id]` includes meet info, meet files, and session-grouped event signup.
- Swimmer meet detail page is available at `/swim/meets/[id]` with meet files and session-grouped swimmer-facing meet selection UI.

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
- OIDC callback requires: `OIDC_TOKEN_URL`, `OIDC_JWKS_URL`, `OIDC_REDIRECT_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and `OIDC_STATE_SIGNING_KEY`.
- Optional OIDC settings: `OIDC_EXPECTED_ISSUER`, `OIDC_HTTP_TIMEOUT` (default `10s`).

## Make Quality Gates

- `make test` runs `go test ./...` in `server/` and `npm run build` in `admin/`.
- `make lint` runs `go vet ./...` in `server/` and `npm run lint` in `admin/`.
