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
| Run critical backend coverage gate | `cd server && go run ./scripts/check_critical_coverage.go` |
| Run a single Go test | `cd server && go test ./internal/app -run TestFooBar` |
| Run meetfiles parser tests | `cd internal/meetfiles && go test ./...` |
| Lint everything | `make lint` |
| Apply DB migrations | `make db-up` |
| Roll back one migration | `make db-down` |
| Migration status | `make db-status` |
| Build launch evidence bundle | `make launch-evidence-bundle INPUT_DIR=/path/to/export SOURCE_COUNTS=/path/to/source-counts.json SOURCE_BILLING_TOTAL_CENTS=<cents> ORG_ID=<org> API_BASE_URL=<url> AUTH_TOKEN=<token>` |
| Preflight evidence env/files on Windows | `powershell -ExecutionPolicy Bypass -File scripts/evidence-preflight.ps1 -Mode final` |
| Verify completed launch evidence bundle | `make verify-launch-evidence EVIDENCE_DIR=/path/to/evidence/launch-<timestamp>` |
| Capture provider validation evidence | `make provider-validation-evidence API_BASE_URL=<url> AUTH_TOKEN=<token> ORG_ID=<org> EXPECT_STRIPE_WEBHOOK_EVENT_ID=<evt> CONFIRM_STRIPE_REFUND=true REFUND_INVOICE_ID=<invoice> REFUND_PAYMENT_ID=<payment> REFUND_AMOUNT_CENTS=<cents> POSTMARK_CAMPAIGN_ID=<campaign> CONFIRM_POSTMARK_SEND=true` |
| Capture two-meet-cycle evidence | `make parallel-meet-cycle-evidence API_BASE_URL=<url> AUTH_TOKEN=<token> ORG_ID=<org> MEET_IDS=<id1>,<id2> NO_MANUAL_PATCHING_ATTESTATION=true` |
| Capture post-launch stability evidence | `make stability-report-evidence API_BASE_URL=<url> AUTH_TOKEN=<token> ORG_ID=<org> STABILITY_WINDOW_LABEL=+24h NO_OPEN_P0_P1_ATTESTATION=true SUPPORT_INCIDENTS_FILE=/path/to/incidents.csv` |
| Start local infra (Docker) | `make infra-up` |
| Destroy local infra + volumes | `make infra-reset` |
| Start local Keycloak (Windows, native) | `powershell -ExecutionPolicy Bypass -File scripts/start-keycloak-local.ps1` (add `-Reset` to wipe data + re-import realm) |
| Start local Keycloak (macOS/Linux, native) | `make keycloak-local-up` / `make keycloak-local-reset` |

> **`internal/meetfiles` is a separate Go module** — always `cd` into it before running `go test` or `go vet`.

> **`make test` runs the full admin production build**, which is slow. Use `cd server && go test ./...` during backend iteration.

> `make verify-launch-evidence` is strict by default and is the command for final TODO closeout evidence. `VERIFY_LAUNCH_EVIDENCE_FLAGS` exists only for partial diagnostic bundles and must not be used to mark launch evidence complete.

> Admin lint is configured with `next lint --max-warnings=0`; warnings fail the lint gate and should be fixed rather than waived unless a TODO item explicitly allows a documented waiver.

## PostgreSQL Runtime Notes

- Two local DB modes: Docker (`make infra-up`) or local PostgreSQL service (Postgres.app/Homebrew).
- Default fallback DSN: `postgres://postgres:postgres@localhost:5432/aquify?sslmode=disable`
- Override in `.env.local` via `DATABASE_URL` if local auth differs.
- `server/cmd/api/main.go` only auto-loads `.env.local` when `APP_ENV` is already explicitly set in the process environment to `development` or `local`. Staging/production/test modes must not rely on repo-local env files for config resolution.
- Check connectivity: `pg_isready -h 127.0.0.1 -p 5432`
- MFA schema compatibility: older DBs may have a legacy `mfa_factors` shape (UUID ids, `secret_encrypted`) while authn code expects `totp_secret` + `backup_codes`; run latest migrations to reconcile before debugging MFA load warnings.
- API startup now runs a repo-column schema validation pass after opening the DB pool; if startup logs `database schema validation failed`, apply the latest migrations before debugging repository load errors.
- `APP_ENV` must now be set explicitly to one of `development`, `local`, `test`, `staging`, or `production`; typoed or missing values fail startup before other config checks.
- In non-development environments (`APP_ENV` not explicitly `development`, `local`, or `test`), API startup is fail-closed for runtime dependencies: `DATABASE_URL` must connect and pass schema validation, and object storage must be explicitly configured/reachable (`STORAGE_ENDPOINT`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY`, `STORAGE_BUCKET`).
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
  docker-compose.yml  # Local infra: Postgres, Redis, MinIO, Mailpit, Keycloak
```

### Backend Domain Pattern
Each domain lives in `server/internal/app/<domain>/` and follows the same structure:
- `service.go` — business logic; exported `Service` struct with `NewService()` and `NewServiceWithPool(pool)` constructors
- `repository.go` — pgx/v5 DB read/write; loaded into in-memory maps on startup (write-through cache)
- `runtime_snapshot.go` — legacy JSON snapshot bridge (backward compat; DB writes are canonical)

Domains: `members`, `groups`, `meets`, `imports`, `exports`, `performance`, `billing`, `comms`, `compliance`, `documents`, `training`, `audit`, `seasons`

- Admin `/seasons` is API-backed via `GET/POST /api/v1/seasons` and `GET/PUT/DELETE /api/v1/seasons/{id}` (read: admin/coach; write: admin, audit-logged). Season records live in `server/internal/app/seasons/` with helpers in `admin/lib/adminData.ts` (`fetchSeasons`/`createSeason`/`updateSeason`/`deleteSeason`); do not reintroduce local-only season state on that page.

HTTP handlers live in `server/internal/app/handlers/<domain>.go` and are thin: validate → call service → respond.

**`NewRouterWithServices(cfg, logger, RouterDeps{...})`** in `server.go` is the main wiring point — dependencies are named fields on `RouterDeps` (nil = default/stub), not positional parameters. Tests call `NewRouter(cfg, logger)` which passes all-nil services.

### Frontend Layout
- `admin/app/(admin)/` — admin-only pages (auth-gated to admin/coach)
- `admin/app/(portal)/portal/` — parent portal pages
- `admin/app/(swimmer)/swim/` — swimmer portal pages (always on; the old `NEXT_PUBLIC_SWIMMER_PORTAL_ENABLED` flag and "Coming Soon" screen were removed — do not reintroduce a portal gate, access is controlled by the `swimmer` role only)
- `admin/lib/adminData.ts` — all fetch helpers and shared types; add new API calls here
- `admin/lib/roles.ts` — nav sections and role-to-nav mapping; add new nav items here
- `admin/components/layout/Sidebar.tsx` — resizable sidebar (width persisted to localStorage)
- QR code typing: `qrcode` does not ship types in this repo setup; use `admin/types/qrcode.d.ts` (and keep `**/*.d.ts` in `admin/tsconfig.json` include list) instead of changing dependency setup.
- Self-service access request flow is live: public page `admin/app/register/page.tsx`, backend `POST /api/v1/register` in `server/internal/app/handlers/users.go`, admin review queue `admin/app/(admin)/members/approvals/page.tsx`, and sidebar pending badge in `admin/components/layout/Sidebar.tsx`.
- Parent portal document uploads are now a real two-step flow: request `POST /api/v1/documents/{id}/upload-url`, upload to the presigned object-storage URL, then finalize with `POST /api/v1/documents/{id}/attach`. Attach now validates both submission-scoped object-key prefixes and actual object existence in storage.
- Admin meet creation now uses `POST /api/v1/meets/parse-entry-file` to parse `.sd3/.cl2/.zip/.hy3` files through the backend and return derived session drafts; the UI no longer injects mock parsed sessions locally.
- `/api/v1/openapi` now serves a packaged spec artifact from `server/internal/openapi/spec/openapi.yaml` so production binaries do not depend on repo-relative runtime file paths. Keep `docs/api/openapi.yaml` as the editable source of truth and update the packaged copy alongside it.
- Admin fonts are bundled from local `@fontsource/*` packages; do not reintroduce `next/font/google` or direct Google Fonts URLs. `npm run check:offline-build` and the production `build`/`lint` scripts enforce that offline-safe constraint.
- JSON request parsing for handler bodies is centralized in `server/internal/app/handlers/json.go`. Use `decodeJSONBody`, `decodeOptionalJSONBody`, or `decodeAndValidateJSON` instead of ad hoc `json.NewDecoder(r.Body)` so every route keeps the same 1 MiB cap, `DisallowUnknownFields`, and single-object enforcement.
- Stripe webhook mutations load canonical database state under an invoice row lock and commit payment/refund, invoice, credit, and ledger writes in one transaction (`billing/stripe_transaction.go`). Treat `charge.amount_refunded` as cumulative; refund invoice payments first, then overpayment credit. Active webhook leases require a retry response, not a successful acknowledgement.
- Admin `/api/v1/*` requests use the streaming route proxy in `admin/lib/api-proxy.ts`. Set server-only `API_INTERNAL_URL` at runtime (Compose uses `http://dare-api:8080`); do not restore a build-time rewrite target. Preserve cookies, CSRF headers, redirects, and multiple Set-Cookie headers.
- API container probes run `/dare-api healthcheck http://localhost:8080/api/v1/health/live` without shell dependencies. Admin health is `/api/health`; all API health paths include `/api/v1`.
- Stripe payment creation writes (`POST /api/v1/stripe/payment-intents` and `POST /api/v1/stripe/checkout-sessions`) now require an `Idempotency-Key` header and pass it through to the Stripe client; preserve that contract on any future client/UI changes so retries stay exactly-once.
- Prometheus `/metrics` is no longer mounted on the main API router by default. Use `METRICS_ADDR` to start a dedicated metrics listener from `server/cmd/api/main.go`, and treat wildcard/non-dev bindings as an operational risk that must remain explicit.
- Communications campaign sends (`POST /api/v1/email-campaigns/{campaignID}/send`) no longer accept caller-supplied recipient lists. Recipients are resolved server-side from org membership data based on the campaign audience (`all` or eligible `group` registrations), and old payloads should fail loudly rather than bypassing that policy.
- CSRF token generation is fail-closed. `appmiddleware.NewCSRFToken()` now returns an error on entropy failure, and auth/session handlers must surface `csrf_generation_failed` rather than emitting any deterministic fallback token material.
- Admin impersonation headers remain fail-closed even when `SECURITY_IMPERSONATION_ENABLED=true`. Middleware enforces admin role, MFA, reason, and target-user presence, then returns `403 impersonation_unavailable` until audited session-switch implementation exists; do not reintroduce the old `501 impersonation_not_implemented` path.
- Frontend meet write helpers must use the backend route shape `/api/v1/meets/{meetId}/...`, not `/api/v1/orgs/{orgId}/meets/...`. Keep entry creation and commitment response requests centralized in `admin/lib/adminData.ts` so body fields stay aligned with the backend contract (`committed_yes` / `committed_no`, `notes`, no ad hoc `status` field).
- Frontend session state now preserves both `session.user.role` (highest-priority compatibility field) and `session.user.roles` (full backend role set). New nav/feature gating code should evaluate against `roles`, not the single `role`, so mixed-role users keep access to secondary capabilities like billing/admin surfaces.
- `/training` is the only live admin practices/workouts surface. The legacy `/practices` route now redirects to `/training`; do not reintroduce mock scheduler state or demo-only practice editing flows on a separate page.
- Handler-backed list endpoints in `members`, `groups`, and `meets` now expose a shared `limit`/`offset` contract with a default page size of `100` and a hard max of `200`, returning stable `count`, `total`, `limit`, and `offset` metadata. `/api/v1/users` keeps a lower default of `50` but shares the same `200` max; keep new list routes aligned with `server/internal/app/handlers/pagination.go` instead of open-coding pagination logic.
- Admin entry creation in `admin/app/(admin)/entries/page.tsx` must fail closed: do not swallow per-entry mutation errors inside `Promise.all`. Use `admin/lib/meet-entry-save.ts` so the panel only shows a saved state after every selected entry write succeeds, and surfaces an inline error if any request fails.
- Parent-portal meet event selection is now API-backed. `admin/app/(portal)/portal/meets/[id]/page.tsx` must load persisted commitment event selections with `fetchCommitmentEventSelections` and mutate them through `addCommitmentEventSelection` / `deleteCommitmentEventSelection`; do not fall back to local-only `selectedEvents` toggles. Keep the state helpers in `admin/lib/portal-meet-event-selections.ts` aligned with the backend selection records so refreshes reflect persisted state.
- Swimmer meet detail at `admin/app/(swimmer)/swim/meets/[id]/page.tsx` is also API-backed now. Load the swimmer's persisted commitment event selections with `fetchCommitmentEventSelections`, persist commitment changes through `respondToMeetCommitment`, and mutate event toggles through `addCommitmentEventSelection` / `deleteCommitmentEventSelection` instead of local-only state. Keep the single-swimmer state helpers in `admin/lib/swimmer-meet-detail-state.ts` aligned with backend selection records so refreshes reconcile to saved state.
- Meet files in the parent/swimmer meet detail pages are backed by meet-scoped export listings from `GET /api/v1/meets/{meetId}/exports`. Admin/coach keep broad access, but parent/swimmer access must stay household-scoped through the meet commitment read path in `server/internal/app/handlers/exports.go`; do not expose export-scoped download routes directly to portal/swimmer UIs. Use the authorized list response's `download_url` for file links in `admin/app/(portal)/portal/meets/[id]/page.tsx` and `admin/app/(swimmer)/swim/meets/[id]/page.tsx`.
- Swimmer profile bio is now live through the existing swimmer `notes` field. `swimmer` role may update only `notes` on swimmers in its own household via `PATCH /api/v1/members/swimmers/{id}`; do not broaden swimmer self-edit rights to names, registration status, medical flags, or external IDs. Keep the 400-character trimmed limit aligned between `server/internal/app/members/service.go`, `admin/lib/swimmer-bio.ts`, and `admin/app/(swimmer)/swim/profile/page.tsx`.
- Swimmer cut qualifications are live in `admin/app/(swimmer)/swim/performance/page.tsx`. Load active standards through `fetchTimeStandards`, filter them with `admin/lib/swimmer-cut-qualifications.ts`, and evaluate each applicable standard through `GET /api/v1/swimmers/{swimmerId}/cut-qualification?standard_id=...`. Keep the view-state/status logic centralized in `admin/lib/swimmer-performance-cuts.ts` so loading/error/empty/no-time/qualified branches stay deterministic and covered by tests.
- Parent portal registration detail cards now depend on live registration/document state, not a waiver placeholder. `GET /api/v1/registrations` must remain parent-readable with strict household scoping and waiver summary fields (`waiver_accepted`, `waiver_count`, `waiver_types`, `last_waiver_accepted_at`), and `admin/app/(portal)/portal/registrations/page.tsx` should derive the supporting document rows from `fetchDocumentTypes` + `fetchDocumentSubmissions` through `admin/lib/portal-registration-status.ts`.
- Frontend collection-style portal/swimmer pages should use `admin/lib/collection-load-state.ts` and `admin/lib/frontend-telemetry.ts` for load failures instead of swallowing errors into empty/demo state. If a page fetch fails, show an explicit banner/unavailable message plus a retry affordance; only render “no data” states when the load completed cleanly with zero rows.
- Parent/swimmer portal read paths now rely on scoped access through the existing handlers, not frontend fallbacks. `GET /api/v1/members/households` is household-scoped for `parent` and `swimmer`, `GET /api/v1/groups` is readable by `swimmer`, and the route matrix in `server/internal/app/server_step23_test.go` is the backend contract proving parent/swimmer access across members, groups, meets, practices, announcements, and per-swimmer performance endpoints while preserving foreign-household denial.
- Parent portal invoice payment at `admin/app/(portal)/portal/billing/invoices/[id]/page.tsx` is live. Launch checkout through `admin/lib/portal-invoice-checkout.ts` so the UI always creates a real `POST /api/v1/stripe/checkout-sessions` session, redirects via Stripe's returned `url`, and shows explicit success/cancel/error banners instead of local placeholders. Backend billing reads and parent checkout authorization are household-scoped in `server/internal/app/billing/service.go` and `server/internal/app/handlers/stripe.go`; do not loosen that to org-wide invoice access for `parent`.
- Portal communications preferences at `admin/app/(portal)/portal/communications/page.tsx` are backed by `GET/PUT /api/v1/communication-preferences`. Keep the event catalog aligned with `server/internal/app/comms/service.go` and `admin/lib/portal-communications-preferences.ts`, and treat preference persistence as user-scoped with household metadata carried from the authenticated principal for parent/swimmer sessions.
- Billing autopay is now live through `GET/PUT /api/v1/households/{householdID}/autopay`, `POST /api/v1/households/{householdID}/autopay/opt-out`, and `GET /api/v1/autopay/households`. Keep portal/admin UI state aligned with `admin/lib/autopay.ts`, treat parent access as household-scoped only, and preserve the backend next-charge preview contract that derives scheduled date/amount from the earliest open issued invoice plus `trigger_days_before_due`.
- Meet entry pipeline: `POST /meets/{id}/lock-commitments` locks only `committed_yes`/`coach_reviewed` commitments (declines and non-responses keep their status) and turns each locked commitment's event selections into `submitted` entries. Coaches approve them on `/entries` (approve/reject call the API; reject = scratch with a reason), then finalize and download the host file via `POST /meets/{id}/entries/sd3` (admin/coach, requires `entry_finalized`/`completed` and an org-settings team code; marks approved entries `exported`). Manual events can be added from the meet's Events tab (`POST /meets/{id}/events`).
- Results import: `POST /meets/{id}/results/import` (multipart SD3/CL2/HY3/ZIP) matches swims to club swimmers by USA Swimming ID, then name + DOB (gender tiebreak), records official results (best times recompute), skips swims already recorded for that swimmer/event/meet, and returns unmatched rows. UI: "Import Results File" on the meet Overview tab.
- Cross-domain hooks wired in `server.go`: creating a swimmer invites them to any meet with commitments open (and seeds the groups scope index in in-memory mode); activating a registration adds the swimmer to that group's roster (primary if they have none). Group codes are canonical UPPERCASE in both `groups` and `swimmer_groups` — compare codes exactly, never lowercase them.
- Coach-visible staff names come from `GET /api/v1/users/staff` (admin+coach, names/roles only). Use `fetchStaff` / `useStaffNames` instead of `/users` (admin-only) anywhere a coach may render a user ID; never render raw user UUIDs.
- List fetchers for paginated routes (households, swimmers, swimmer-groups, groups, registrations, meets, sessions, events, commitments, selections, entries) go through `fetchAllPages` in `admin/lib/adminData.ts`; a bare GET returns only the first 100 rows.
- Raw `fetch` calls (multipart uploads, file downloads) must use `rawRequestAuthHeaders` from `admin/lib/api.ts` so cookie-auth mutations send `X-CSRF-Token`; local bearer stub tokens bypass CSRF and will not catch this.
- Date-only API fields (`start_date`, `date_of_birth`, `due_date`, …) must be parsed with `parseBackendDate` (`admin/lib/dates.ts`), not `new Date()`, or they render a day early west of UTC. Event eligibility (gender + age at meet start) lives in `admin/lib/event-eligibility.ts`; event labels in `admin/lib/meet-events.ts`.
- Compliance: creating a data export request enqueues `compliance:data_export` (archive includes the household's records via the collector wired in `server.go`); deletion requests need admin review — `POST .../data-deletion-requests/{id}/approve` (MFA required, enqueues `compliance:data_deletion`) or `/reject` (notes required).
- Local dev without MinIO: when object storage is the stub in `APP_ENV=development|local`, presigned URLs point at the API's `/api/v1/dev-storage/*` route so uploads/downloads work (in-memory, lost on restart). Dev stub tokens also seed the stub parent's household, and the frontend hydrates stub sessions from `GET /auth/session` so linked swimmer/household IDs match a real login.
- The API image builds from the repo root (`server/Dockerfile`, `server/Dockerfile.dockerignore`) because `server/go.mod` replaces the meetfiles module with `../internal/meetfiles`.
- Structured workouts & training analytics live in `server/internal/app/training/` (`workout.go` model/totals, `quickentry.go` shorthand parser, `plans.go` periodization, `analytics.go`) with routes in `handlers/training_workouts.go`: `/workouts` library (templates; parse/preview/duplicate/assign), `/practices/{id}/workout`, `/training/schedule`, `/training/analytics[/swimmers/{id}]`, `/seasons/{id}/training-phases|training-plan`, `/training-phases/{id}`. Zones are USA Swimming EN/SP (`REC, EN1–3, SP1–3`). Assigning copies a template into a practice-owned instance (`practices.workout_id`); while linked, `yardage_target`/strokes always derive from the workout. Parent/swimmer practice reads (`GET /practices`, `/practices/{id}`, `/practices/{id}/workout`, `/training/schedule`) are scoped to team-wide practices plus their household swimmers' groups. Analytics use ISO weeks, yards (metres ×1.0936) and attendance % = present/(present+absent). Frontend helpers: `admin/lib/trainingData.ts`, `admin/lib/workout-format.ts`; pool-deck view `/deck/workouts/{id}`.
- Team records / top times / goals / relays / progress live in `server/internal/app/performance/{records,toplists,goals,relays,progress}.go` (routes in `handlers/performance_extensions.go`, tables `team_record_entries`, `swimmer_goals`, `relay_lineups`). Records are per course × M/F × age group (`8U…17-18`, `OPEN`; age on the swim date = meet start date, else result date) × individual event; the current record is the fastest *accepted* entry and history is the progression. `UpsertResultWithRecords` flags breaks on `POST /results` and `POST /meets/{id}/results/import` (`new_team_records`); a locked record turns faster swims into `pending` entries for admin approve/reject. Record writes (manual/lock/verify/approve/reject/rebuild) are admin-only; records + `GET /performance/top-times` are readable by every role; goals (`/swimmers/{id}/goals`) are writable by staff for anyone and by parent/swimmer for their own household only; relay builder/lineups are admin/coach. Swimmer/meet facts come from `performance.Directory` (adapter `server/internal/app/performance_directory.go`). Frontend helpers: `admin/lib/performanceData.ts`; shared UI in `admin/components/performance/`.
- Volunteer jobs & service hours live in `server/internal/app/volunteer/` (routes `/api/v1/volunteer/*` in `handlers/volunteer.go`, wiring + billing adapter in `server/internal/app/volunteer_wiring.go`, tables `volunteer_{settings,jobs,signups,requirements,credits,buyouts}`). DB mode reads/writes Postgres directly (no cache); signup capacity is enforced under a `FOR UPDATE` lock on the job row — never add a read-then-insert path. Hours attribute to the job's explicit `season_id`, else the season whose dates contain the shift's UTC start date. Requirement precedence: household override > season requirement > org default (`/volunteer/settings`). Roles: admin everything; coach jobs/templates/check-in/credits/ledger reads (no requirements, settings writes, or buyouts); parent browse + sign up/cancel own household only, cancel blocked inside `cancellation_cutoff_hours` (staff may always cancel); swimmer read-only own household. Buyouts (`POST /volunteer/buyouts`, admin) bill through `billing.Service` (draft invoice + one `manual` item, optional issue) behind a unique reservation row — don't duplicate billing logic. Signups enqueue `email:volunteer_shift_reminder` 24h before the shift (handler registered on the worker backend in `wireVolunteer`; stale after a reschedule). Frontend: `admin/lib/volunteerData.ts`, `admin/lib/volunteer-state.ts`, `admin/components/volunteer/`, pages `/volunteer`, `/portal/volunteer`, meet detail Volunteers tab.
- Club finance lives in `server/internal/app/billing/finance_*.go` (handlers `handlers/billing_finance.go`, migration `20260713000100`): fee schedules (`/api/v1/billing/fee-schedules`; dues per UPPERCASE group code or one-time season registration fees; proration none/daily/half/waive), sibling discount tiers (`/billing/discount-rules`; best single tier per swimmer position, no stacking), pending one-off adjustments (`/billing/adjustments`), settings (`/billing/settings`: late fee + monthly auto-run). Runs: `POST /billing/runs/preview`, then `POST /billing/runs` with `expected_fingerprint` (409 `preview_stale` if anything changed). Never-double-bill guard = partial unique index on `billing_charges (org_id, schedule_id, period_key, household_id, swimmer_id) WHERE released_at IS NULL` plus a per-org Postgres advisory lock; voiding an invoice (draft or issued) releases its charges and re-pends its adjustments. Household account credit (`/households/{id}/credits`, admin+MFA writes; `/invoices/{id}/refunds-to-credit`; `excess_to_credit` on recorded payments) is a non-negative balance row + entry trail, auto-applied as `account_credit` payments when invoices are issued. Late fees are one per invoice (unique) via worker job `billing:late_fees`; `billing:recurring_run` commits auto-bill schedules (both org-scoped, `WORKER_BILLING_AUTOMATION_INTERVAL`, default 24h). Reports `/billing/reports/{aging,revenue,outstanding,deposits,quickbooks}` (`format=csv`; exports audited). Parent-readable for own household only: `/households/{id}/statement|upcoming-charges|credits`; all other finance routes admin-only, coach none. Finance reads re-load billing state from Postgres (`reloadLocked`) so API/worker instances agree; roster comes from `server/internal/app/billing_roster.go`. UI: `/billing/schedules`, `/billing/runs`, `/billing/reports`, portal `PortalStatement.tsx`; helpers in `admin/lib/financeData.ts`.
- Engagement domains: club events (`server/internal/app/events/`, routes `/api/v1/club-events*`, admin `/events`, portal `/portal/events`), household `.ics` feeds (`/api/v1/calendar/feed-token` GET/POST/DELETE for parent or admin+`household_id`; public `GET /api/v1/calendar/feed/{token}.ics`, only the SHA-256 hash is stored, the secret is shown once), wellness check-ins (`server/internal/app/wellness/`, `PUT /api/v1/wellness/checkins` swimmer-self or parent-household only, `GET /wellness/swimmers/{id}` staff+household, `GET /wellness/heatmap` staff) and `GET /api/v1/swimmers/{swimmerID}/attendance-summary` (`server/internal/app/attendance/`, reads training only via `List`/`ListAttendance`). Roster/household scoping goes through `roster.Directory` (`server/internal/app/roster/`); wiring lives in `server/internal/app/engagement.go`. RSVP fees bill through `billing.Service` (one issued invoice per paying RSVP, quantity = 1 + guest_count; leaving "yes" or cancelling the event voids it only if unpaid). All-day club events carry UTC-midnight timestamps. Pool-deck roll call `/attendance` queues marks in localStorage (`admin/lib/roll-call-queue.ts`) and retries; frontend helpers are in `admin/lib/engagementData.ts`.

- Installable PWA (frontend only): `admin/app/manifest.ts` (start_url `/login`, which forwards signed-in users by role), PNG icons in `admin/public/icons/` (regenerate with `node scripts/generate-pwa-icons.mjs`), and a hand-written `admin/public/sw.js` registered by `components/pwa/ServiceWorkerRegistrar.tsx` only in production builds or with `NEXT_PUBLIC_ENABLE_SW=true`. The SW caches only `/_next/static`, icons, and fonts (cache-first) and serves `/offline` for failed navigations; it must never cache `/api/` or navigations (authenticated content). Bump `VERSION` in `sw.js` to invalidate caches. The portal/swimmer layouts mount `InstallPrompt` (beforeinstallprompt, 30-day dismissal in `lib/install-prompt.ts`); keep it out of the admin shell.
- Public team website: `GET /api/v1/public/orgs/{slug}/site` (`handlers/publicsite.go`, wired by `publicsite_wiring.go`) is mounted in the unauthenticated route group with its own 60/min/IP limiter and returns only club name/about/contact, announcements with `public=true` (published, audience `all`), upcoming non-draft meets, club events with `public=true` (audience `all`, scheduled), current team records (swimmer names only, via the least-privileged `parent` reader) and coach names only when `public_show_coaches` is on. Disabled or unknown slugs return 404. Org settings carry `public_slug` (unique, `^[a-z0-9-]{3,40}$`), `public_site_enabled`, `public_show_coaches`, `public_about`, `public_contact_email/phone` (admin-only write, audited as `org_settings.update`; UI in `/settings` via `settings/PublicSitePanel.tsx`). Visibility toggles: `PATCH /api/v1/announcements/{id}` `{public}` (audit `comms.announcement.visibility`), `public` on announcement create and club-event bodies (migration `20260715000100`, column `is_public`). Never add fields to the public payload without a leak test in `server_publicsite_test.go`. Frontend routes `/club/[slug]` (home, `news`, `schedule`, `records`) fetch server-side through `fetchPublicSite` in `admin/lib/publicSiteData.ts` (`API_INTERNAL_URL` or `NEXT_PUBLIC_API_URL`); styles live in `admin/app/club/club.css` under `.cs`.
- USA Swimming SWIMS: `server/internal/app/usaswimming/swims.go` implements the documented Third-Party Vendor API (OAuth client credentials via `token_url`/`client_id`; `api_key` stores the client secret; tiers 1-3 per the USA Swimming API program page). It has NO swimmer-times listing — `OfficialTimes` returns `ErrUnsupported` (501) and `/sync` only works with the legacy assumed-contract client. Admin-only routes under `/integrations/usa-swimming/`: `clubs`, `roster-compare` (read-only), `registration-link`, Tier 2 `registrations` (POST, validated locally by `ValidateRegistration`) + `registrations/{id}/status`, Tier 3 `verify-members` / `verify-time` (403 `tier_not_authorized` below Tier 3). UI: `admin/app/(admin)/settings/IntegrationsPanel.tsx`. Response field names in `swims.go` are ASSUMED until a vendor Postman collection is available; verify then.

### Auth / RBAC
- OIDC via Keycloak. All local Keycloak configuration (realm import JSON + branded login theme) lives in `deploy/keycloak/`; the server distribution itself is auto-downloaded into gitignored `.tools/` by `scripts/start-keycloak-local.ps1` (Windows) or `make keycloak-local-up` (macOS/Linux), or runs as the `keycloak` docker-compose service. The realm import is self-contained (clients, roles, seed users with `org_id`, `aquify-admin` service account with realm-admin, Mailpit SMTP) — there is no separate bootstrap script. See `docs/ops/keycloak-setup.md`.
- Dev stub tokens controlled by `AUTH_ENABLE_DEV_STUB_TOKENS` (default `true` only when `APP_ENV` is explicitly `development`, `local`, or `test`; must be `false` in prod).
- `middleware.Auth` accepts `Authorization: Bearer ...` or `aquify_access_token` cookie.
- All protected routes require `X-Org-Id` header matching the principal's `OrgID`. The `membershipScopeFromRequest` helper enforces this — mismatches return 403.
- Protected router wiring now enforces org scope centrally with `RequireOrgScope` immediately after auth middleware. Handler-level scope helpers should treat `OrgIDFromContext` as already validated against the principal and only add narrower domain/body scope checks on top.
- CSRF required on cookie-backed mutations via `X-CSRF-Token` header. Requests carrying an `Authorization` header are bearer-authenticated (it wins over cookies in `extractAuthToken`) and are exempt, even if unrelated cookies ride along.
- `httpx.WriteError` logs the cause of every 5xx (pass the `error` as `details`) via `slog.Default()` (set in `cmd/api/main.go`) and never serialises error values to clients.
- CORS must allow the configured CSRF header (default `X-CSRF-Token`) so browser cookie-auth mutation preflights can pass before CSRF validation runs.
- Cookie-auth session endpoints (`oidcCallback`, `/auth/sessions/issue`, `/auth/sessions/refresh`, and MFA challenge replacement sessions) must not return access or refresh token values in JSON bodies. Issue/rotate those secrets only through secure HttpOnly cookies and return non-sensitive metadata such as `expires_at`.
- Roles: `admin > coach > parent > swimmer`. Deny-by-default. (Collapsed from a prior 7-role model — `owner`/`billing` were absorbed into `admin`; `read_only_staff`/`read-only` was removed entirely.)

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
Asynq queues over Redis: `imports`, `billing`, `email`, `reconcile`, plus `default` for cleanup/compliance jobs. `REDIS_URL` now enables the durable backend used by both API admin endpoints and the dedicated `worker` process mode in `server/cmd/api/main.go`; outside explicit dev/test envs, startup now requires Redis to be configured. Jobs metrics, DLQ visibility, and retry/delete actions are Redis-backed instead of process-local, and `server/internal/app/server_step32b_test.go` proves queued jobs survive API process restart. Org-scoped scheduled jobs such as billing reconciliation and nightly integrity require `WORKER_SCHEDULED_ORG_IDS` in staging/production; local/test mode falls back to `org-demo`.

## Design System
Defined in `design-language-guide.html`; live tokens in `admin/app/globals.css`. Brand accent: `#eb5d1e`. Dark mode supported.
- Fonts, one set per audience (bundled via `@fontsource/*`): admin/coach = Atkinson Hyperlegible Next; parent portal + public club site = Literata (display) + Figtree (body); swimmer portal = Barlow Condensed (display) + Lexend (body). Times/IDs/code use Atkinson Hyperlegible Mono everywhere. Portals switch fonts by overriding `--font-display`/`--font-body`/`--display-weight` under `[data-portal]`.
- Type sizes come only from the `--fs-2xs … --fs-4xl` role scale (11/12/13/14/15/17/20/24/30/38px); never hand-write a pixel `fontSize`. Display-face text uses `fontWeight: 'var(--display-weight)'`.
- Headings: `.page-title` (+ `.page-subtitle`) inside `.page-header` (with actions) or `.page-heading` (title only); `.section-title` for page sections, `.card-title` for card/panel headings. No inline typography overrides on these classes.
- Layout: every page root uses `.page` (one frame: fills the content area up to `--page-max` 1600px in admin, the 1200px `--portal-max` column in parent/swimmer). `.page-narrow`/`.page-form` are legacy aliases of the same frame; don't add per-page `maxWidth` caps; use `.prose` for long text measure. Spacing snaps to the 4px scale (`--space-*`); sections are separated by 32px, header-to-content by `--section-gap` (24px). Radii: controls 8, containers 12, overlays 16. No colored side-stripe borders on cards.

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
