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

## Common Development Commands

| Task | Command |
|------|---------|
| Start API + admin together | `make dev` |
| Start API only | `make dev-api` |
| Start admin only | `make dev-admin` |
| Run all tests + admin build | `make test` |
| Run P0 regression tests only | `make test-p0` |
| Run server tests only (fast) | `cd server && go test ./...` |
| Run a single Go test | `cd server && go test ./internal/app -run TestFooBar` |
| Run meetfiles parser tests | `cd internal/meetfiles && go test ./...` |
| Lint everything | `make lint` |
| Apply DB migrations | `make db-up` |
| Roll back one migration | `make db-down` |
| Migration status | `make db-status` |
| Start local infra (Docker) | `make infra-up` |
| Destroy local infra + volumes | `make infra-reset` |

> **`internal/meetfiles` is a separate Go module** — always `cd` into it before running `go test` or `go vet`.

> **`make test` runs the full admin production build**, which is slow. Use `cd server && go test ./...` during backend iteration.

> Admin lint is configured with `next lint --max-warnings=0`; warnings fail the lint gate and should be fixed rather than waived unless a TODO item explicitly allows a documented waiver.

## PostgreSQL Runtime Notes

- Two local DB modes: Docker (`make infra-up`) or local PostgreSQL service (Postgres.app/Homebrew).
- Default fallback DSN: `postgres://postgres:postgres@localhost:5432/aquify?sslmode=disable`
- Override in `.env.local` via `DATABASE_URL` if local auth differs.
- Check connectivity: `pg_isready -h 127.0.0.1 -p 5432`
- MFA schema compatibility: older DBs may have a legacy `mfa_factors` shape (UUID ids, `secret_encrypted`) while authn code expects `totp_secret` + `backup_codes`; run latest migrations to reconcile before debugging MFA load warnings.
- API startup now runs a repo-column schema validation pass after opening the DB pool; if startup logs `database schema validation failed`, apply the latest migrations before debugging repository load errors.
- In non-development environments (`APP_ENV` not explicitly `development`, `local`, or `test`), API startup is fail-closed for runtime dependencies: unset/invalid `APP_ENV` is treated as strict, `DATABASE_URL` must connect and pass schema validation, and object storage must be explicitly configured/reachable (`STORAGE_ENDPOINT`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY`, `STORAGE_BUCKET`).
- `/api/v1/health/live` remains a shallow process health probe, while `/api/v1/health/ready` now performs live dependency checks and returns `503` when configured critical dependencies are unavailable. In local/test flows, readiness reports explicit `disabled`, `ready (stub)`, or `ready (in-memory)` states instead of placeholder `not-configured` values.

## Planned Tech Stack (locked unless noted)

**Backend:** Go 1.24, Chi router, PostgreSQL 16, sqlc + pgx, go-playground/validator, Asynq + Redis for task queues, S3-compatible object storage

**Frontend:** Next.js (TypeScript), Tailwind CSS, shadcn/ui, TanStack Query, React Hook Form + Zod

**Auth:** Keycloak (self-hosted OIDC) — **locked**

**Payments:** Stripe | **Email:** Postmark | **Observability:** OpenTelemetry, Prometheus + Grafana + Loki, Cloudflare (edge/WAF)

## Architecture

### Monorepo Layout
```
root/
  server/             # Go API backend
  admin/              # Next.js admin + portal frontend
  internal/meetfiles/ # Standalone CL2/SD3/ZIP/HY3 parsing library (own Go module)
  db/migrations/      # Goose-based PostgreSQL migrations
  deploy/             # Docker Compose, Dockerfiles, deploy/rollback scripts
  docs/               # Planning docs, OpenAPI spec, runbooks
  tests/              # k6 load tests
  Makefile
  docker-compose.yml  # Local infra: Postgres, Redis, MinIO, Mailpit
```

### Backend Domain Pattern
Each domain lives in `server/internal/app/<domain>/` and follows the same structure:
- `service.go` — business logic; exported `Service` struct with `NewService()` and `NewServiceWithPool(pool)` constructors
- `repository.go` — pgx/v5 DB read/write; loaded into in-memory maps on startup (write-through cache)
- `runtime_snapshot.go` — legacy JSON snapshot bridge (backward compat; DB writes are canonical)

Domains: `members`, `groups`, `meets`, `imports`, `exports`, `performance`, `billing`, `comms`, `compliance`, `documents`, `training`, `audit`

HTTP handlers live in `server/internal/app/handlers/<domain>.go` and are thin: validate → call service → respond.

**`NewRouterWithServices`** in `server.go` is the main wiring point — it takes all 17 service + provider dependencies. Tests call `NewRouter(cfg, logger)` which passes all-nil services.

### Frontend Layout
- `admin/app/(admin)/` — admin-only pages (auth-gated to owner/admin/coach)
- `admin/app/(portal)/portal/` — parent portal pages
- `admin/app/(swimmer)/swim/` — swimmer portal pages
- `admin/lib/adminData.ts` — all fetch helpers and shared types; add new API calls here
- `admin/lib/roles.ts` — nav sections and role-to-nav mapping; add new nav items here
- `admin/components/layout/Sidebar.tsx` — resizable sidebar (width persisted to localStorage)
- QR code typing: `qrcode` does not ship types in this repo setup; use `admin/types/qrcode.d.ts` (and keep `**/*.d.ts` in `admin/tsconfig.json` include list) instead of changing dependency setup.
- Self-service access request flow is live: public page `admin/app/register/page.tsx`, backend `POST /api/v1/register` in `server/internal/app/handlers/users.go`, admin review queue `admin/app/(admin)/members/approvals/page.tsx`, and sidebar pending badge in `admin/components/layout/Sidebar.tsx`.
- Parent portal document uploads are now a real two-step flow: request `POST /api/v1/documents/{id}/upload-url`, upload to the presigned object-storage URL, then finalize with `POST /api/v1/documents/{id}/attach`. Attach now validates both submission-scoped object-key prefixes and actual object existence in storage.
- Admin meet creation now uses `POST /api/v1/meets/parse-entry-file` to parse `.sd3/.cl2/.zip/.hy3` files through the backend and return derived session drafts; the UI no longer injects mock parsed sessions locally.
- `/api/v1/openapi` now serves a packaged spec artifact from `server/internal/openapi/spec/openapi.yaml` so production binaries do not depend on repo-relative runtime file paths. Keep `docs/api/openapi.yaml` as the editable source of truth and update the packaged copy alongside it.

### Auth / RBAC
- OIDC via Keycloak. Dev stub tokens controlled by `AUTH_ENABLE_DEV_STUB_TOKENS` (default `true` only when `APP_ENV` is explicitly `development`, `local`, or `test`; must be `false` in prod).
- `middleware.Auth` accepts `Authorization: Bearer ...` or `aquify_access_token` cookie.
- All protected routes require `X-Org-Id` header matching the principal's `OrgID`. The `membershipScopeFromRequest` helper enforces this — mismatches return 403.
- Protected router wiring now enforces org scope centrally with `RequireOrgScope` immediately after auth middleware. Handler-level scope helpers should treat `OrgIDFromContext` as already validated against the principal and only add narrower domain/body scope checks on top.
- CSRF required on cookie-backed mutations via `X-CSRF-Token` header.
- CORS must allow the configured CSRF header (default `X-CSRF-Token`) so browser cookie-auth mutation preflights can pass before CSRF validation runs.
- Cookie-auth session endpoints (`oidcCallback`, `/auth/sessions/issue`, `/auth/sessions/refresh`, and MFA challenge replacement sessions) must not return access or refresh token values in JSON bodies. Issue/rotate those secrets only through secure HttpOnly cookies and return non-sensitive metadata such as `expires_at`.
- Roles: `owner > admin > coach > billing > parent > swimmer > read_only_staff`. Deny-by-default.

### Key Design Constraints (locked — see DECISIONS.md)
- Parent account is required for all household-critical workflows; swimmer account is optional and must never be required for registration, commitments, billing, or documents.
- Meet entry flow: parent `committed_yes` → per-swimmer event selection → coach override before lock (audit logged).
- Stub dev tokens must be gated by `NEXT_PUBLIC_APP_ENV != production` AND `NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS != false` in frontend.

### State Machines
Explicit lifecycle states with validated transitions:
- **Meet:** `draft → published → commitment_open → commitment_closed → entry_finalized → completed → archived`
- **Commitment:** `invited → committed_yes | committed_no → coach_reviewed → locked`
- **Entry:** `draft → submitted → approved → exported → updated_from_results → archived`
- **File Import:** `uploaded → scanning → parsing → matching → conflicts_pending → applied → failed → archived`
- **File Export:** `pending → generating → ready → failed → expired`
- **Document submission:** `pending → submitted → verified | rejected`

### File Interoperability
`internal/meetfiles/` handles SD3, CL2, ZIP, and HY3 parsing. HY3 is a ZIP container; inner payload selected by precedence (SD3 > CL2, then lexicographic). Swimmer matching: ID first, then name+DOB+gender fallback.

### Persistence Notes
- When `DATABASE_URL` is set, `NewServiceWithPool(pool)` loads all records from Postgres into in-memory maps on startup; every write persists to DB before updating memory.
- For `members`, `groups`, and `training`, DB-backed mode now re-syncs request state from Postgres on live read/write paths so a second API instance observes creates/updates/deletes without requiring a process restart; shared-DB conflicts must be returned to callers instead of being logged and ignored.
- Auth/session lifecycle is also DB-backed in `NewServiceWithPool(pool)`: the `sessions` table now stores hashed access/refresh tokens plus principal snapshot metadata, so access resolution, refresh rotation, revoke/logout invalidation, and expiry checks survive cross-instance routing and API restarts.
- In-memory mode (nil pool) is preserved for all existing tests — no DB required to run tests.
- The `runtime_snapshots` JSON bridge (`server/internal/runtimepersist`) is retained for backward compat but is not canonical.

### Async Jobs
Asynq queues over Redis: `imports`, `billing`, `email`, `reconcile`, plus `default` for cleanup/compliance jobs. `REDIS_URL` now enables the durable backend used by both API admin endpoints and the dedicated `worker` process mode in `server/cmd/api/main.go`; outside explicit dev/test envs, startup now requires Redis to be configured. Jobs metrics, DLQ visibility, and retry/delete actions are Redis-backed instead of process-local, and `server/internal/app/server_step32b_test.go` proves queued jobs survive API process restart.

## Design System
Defined in `design-language-guide.html`. Brand accent: `#eb5d1e`. Fonts: Instrument Serif (display), Plus Jakarta Sans (body), JetBrains Mono (code). Dark mode supported.

## Document Authority (conflict resolution)
1. `DECISIONS.md` — locked architectural decisions; ground truth
2. `docs/decisions/*.md` — ADR-style records
3. `docs/planning/Developer Handoff Master Document.txt` — comprehensive implementation spec
4. `TODO.md` — atomic task list

## Definition of Done (apply to every task)
- API/docs updated if behavior changed
- Audit events included for privileged actions
- Role checks enforced and tested
- Metrics/logging added for new background jobs
