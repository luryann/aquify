# aquify

> Full-stack swim club operations platform replacing TeamUnify/SportsEngine Motion.

![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Node.js](https://img.shields.io/badge/Node.js-22%2B-339933?logo=node.js&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)
![Next.js](https://img.shields.io/badge/Next.js-TypeScript-000000?logo=next.js&logoColor=white)
![License](https://img.shields.io/badge/License-Proprietary-red)

## Overview

aquify is the team's operational system of record: memberships, meets, money, messaging, and data exchange with swim meet software. It covers:

- **Member & household management** — parents, swimmers, coaches, groups, and registrations
- **Meet operations** — meet setup, commitment tracking, event entry approvals, and coach overrides
- **Meet-file interoperability** — `.cl2`, `.sd3`, `.zip` import; Standard/Extended SD3 export; SDIF workflows; HY3 as a ZIP container
- **Performance tracking** — results, best times, time standards, and swimmer progression
- **Billing & payments** — invoices, payments, refunds, append-only ledger, and Stripe integration
- **Communications** — announcements and Postmark email campaigns with delivery/suppression tracking
- **Training & engagement** — structured workouts, attendance, training analytics, volunteer jobs and service hours, club events, calendar feeds, and wellness check-ins
- **Portals** — admin/coach dashboard, parent portal, swimmer portal (`/swim`, always on), a public team website, and an installable PWA
- **Admin reporting** — membership status, meet fees, invoice aging, payment reconciliation, and registration status in JSON or CSV
- **Audit & compliance** — audit events, GDPR-style deletion/export requests, and RBAC across all routes
- **Background jobs** — Asynq + Redis queues for imports, billing, email, and reconciliation with DLQ visibility

## Tech Stack

| Layer | Technology |
|---|---|
| API | Go 1.24, [Chi](https://github.com/go-chi/chi) router |
| Database | PostgreSQL 16, [pgx/v5](https://github.com/jackc/pgx), [Goose](https://github.com/pressly/goose) migrations |
| Admin UI | Next.js (TypeScript), Tailwind CSS, shadcn/ui, TanStack Query |
| Auth | Keycloak (self-hosted OIDC) |
| Payments | Stripe |
| Email | Postmark |
| Object Storage | S3-compatible (MinIO in local dev) |
| Task Queues | [Asynq](https://github.com/hibiken/asynq) + Redis |
| Observability | OpenTelemetry, Prometheus, Grafana, Loki |

## Monorepo Layout

```
root/
  server/             # Go API backend (Chi router, pgx, Asynq)
  admin/              # Next.js TypeScript admin portal
  db/migrations/      # Goose SQL migration files
  internal/meetfiles/ # Standalone CL2/SD3/ZIP/HY3 parsing library
  docs/               # Planning docs, OpenAPI spec, runbooks, ADRs
  scripts/            # Shell helpers (cutover, smoke checks, etc.)
  Makefile            # All dev targets
  docker-compose.yml  # Local infra: Postgres, Redis, MinIO, Mailpit
  .env.example        # Shared env template
  .env.local.example  # Local infrastructure env template
```

## Local Setup

### Prerequisites

- Go 1.24+
- Node.js 22+ and npm
- One PostgreSQL runtime option:
  - **Docker path:** Docker Desktop (or Docker Engine + Compose plugin)
  - **Native path:** Local PostgreSQL server (Postgres.app, Homebrew, or installer)

### Steps

1. **Copy environment files:**
   ```sh
   cp .env.example .env
   cp .env.local.example .env.local
   ```

2. **Install admin dependencies:**
   ```sh
   cd admin && npm install
   ```

3. **Start local infrastructure (Docker path):**
   ```sh
   make infra-up
   ```

   _If not using Docker, start your local PostgreSQL service and create the database:_
   ```sh
   pg_isready -h 127.0.0.1 -p 5432
   createdb aquify   # if it does not exist yet
   ```
   Then set `DATABASE_URL` in `.env.local` to match your local credentials (e.g., `postgres://<your-user>@localhost:5432/aquify?sslmode=disable` for Postgres.app).

4. **Apply DB migrations:**
   ```sh
   make db-up
   make db-status
   ```

5. **Run both services:**
   ```sh
   make dev
   ```

6. **Open the apps:**
   - Admin UI: [http://localhost:3000](http://localhost:3000)
   - API health: [http://localhost:8080/api/v1/health/live](http://localhost:8080/api/v1/health/live)

## Developer Commands

| Command | Description |
|---|---|
| `make dev` | Run API + admin together |
| `make dev-api` | Run only the Go API |
| `make dev-admin` | Run only the Next.js admin |
| `make test` | Run API tests and admin production build check |
| `make test-p0` | Run P0 happy-path / regression tests only |
| `make lint` | Run Go vet, admin lint, and OpenAPI spec validation |
| `make infra-up` | Start local Postgres/Redis/MinIO/Mailpit via Docker Compose |
| `make infra-down` | Stop local infrastructure containers |
| `make infra-reset` | Stop infrastructure and delete local data volumes |
| `make db-up` | Apply DB migrations with Goose |
| `make db-down` | Roll back the most recent DB migration |
| `make db-status` | Show DB migration status |
| `make cutover-rehearsal INPUT_DIR=/path/to/export-dir` | Run the migration CLI against a final export set |
| `make cutover-smoke API_BASE_URL=... AUTH_TOKEN=... ORG_ID=...` | Run post-cutover HTTP smoke checks |
| `make cutover-certify` | Run the import/export certification fixture suite |
| `make cutover-billing-check API_BASE_URL=... AUTH_TOKEN=... ORG_ID=...` | Run the billing reconciliation endpoint |

## Local Infrastructure Ports and Credentials

| Service | Port(s) | Default Credentials | Notes |
|---|---:|---|---|
| Postgres | `5432` | `postgres` / `postgres` | Database name: `aquify` |
| Redis | `6379` | — | Local development cache/queue |
| MinIO (S3 API) | `9000` | `minioadmin` / `minioadmin` | Endpoint: `http://localhost:9000` |
| MinIO Console | `9001` | `minioadmin` / `minioadmin` | Web console for buckets/objects |
| Mailpit SMTP | `1025` | — | SMTP capture endpoint |
| Mailpit UI | `8025` | — | Web inbox for captured emails |

Credentials and ports can be overridden in `.env.local`, which is read by `docker-compose.yml` via `make infra-*`.

## Auth Configuration

- `AUTH_ENABLE_DEV_STUB_TOKENS` defaults to `true` in development and **must be `false` in all non-local environments**.
- OIDC browser login requires: `OIDC_AUTHORIZE_URL`, `OIDC_TOKEN_URL`, `OIDC_JWKS_URL`, `OIDC_REDIRECT_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and `OIDC_STATE_SIGNING_KEY`.
- Cross-account logout also requires `OIDC_END_SESSION_URL`; set `OIDC_POST_LOGOUT_REDIRECT_URL` to the app route Keycloak should return to after ending the upstream SSO session.
- Callback validation enforces signed `state` payload verification and cryptographic `id_token` validation (JWKS signature + `iss`/`aud`/`exp`/`iat`).
- The frontend bootstraps auth state from `GET /api/v1/auth/session` and uses cookie-backed refresh/revoke flows. The `aquify_csrf_token` cookie value must be echoed in the `X-CSRF-Token` header.
- Production builds must set `NEXT_PUBLIC_APP_ENV=production` and `NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS=false` so stub login and role-switching affordances stay hidden.
- Supported MFA factors: `totp` and `backup_code`.

## Database Migrations

Migrations use [Goose](https://github.com/pressly/goose) and live in `db/migrations/`. All `make db-*` targets run Goose via `go run` and read `DATABASE_URL` from `.env.local` (falling back to `.env`, then a hardcoded local default).

### Troubleshooting

| Error | Cause | Fix |
|---|---|---|
| `dial tcp 127.0.0.1:5432: connect: connection refused` | PostgreSQL not running | `make infra-up` or start local PostgreSQL |
| `password authentication failed for user "postgres"` | Wrong local DB credentials | Update `DATABASE_URL` in `.env.local` |

Quick connectivity check:
```sh
pg_isready -h 127.0.0.1 -p 5432
psql -h 127.0.0.1 -p 5432 -d postgres -c "select current_user, current_database();"
```

## Security Notes

- Baseline response headers are enforced by middleware (CSP, `X-Frame-Options`, `X-Content-Type-Options`, HSTS on HTTPS, etc.).
- In-memory rate limiting is enabled: **240 req/min** global, **20 req/min** for auth routes.
- Import upload validation checks filename safety, file size (capped via `SECURITY_UPLOAD_MAX_BYTES`), allowed MIME types, and an optional malware-scan hook (`SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK`).
- Admin impersonation is disabled by default; requires `SECURITY_IMPERSONATION_ENABLED=true` plus owner role, MFA, and a stated reason.

## Observability

- OpenTelemetry tracing is initialised at startup. Set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable export; omit it to use a no-op tracer (safe for local dev and tests).
- Prometheus metrics are available at `/metrics`. Firewall this path to the Prometheus scraper IP range in production.
- Grafana dashboard JSON files live in `docs/observability/grafana/`.
- Prometheus alert rules live in `docs/observability/prometheus/alerts.yaml`.
- On-call runbook: [`docs/runbooks/on-call.md`](docs/runbooks/on-call.md).

## Key Documents

| Document | Purpose |
|---|---|
| [`DECISIONS.md`](DECISIONS.md) | Locked architectural decisions — ground truth for conflicts |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Contribution workflow and standards |
| [`TODO.md`](TODO.md) | Atomic task list and implementation progress |
| [`docs/planning/`](docs/planning/) | Developer Handoff Master Document and step plans |
| [`docs/decisions/`](docs/decisions/) | ADR-style decision records |
| [`docs/api/openapi.yaml`](docs/api/openapi.yaml) | OpenAPI 3.1 specification |
| [`docs/runbooks/`](docs/runbooks/) | Cutover, hypercare, and on-call runbooks |
| [`docs/migration/`](docs/migration/) | Migration packet and cutover signoff templates |

## License

Copyright © 2026 Ryan Lu. All rights reserved. See [`LICENSE`](LICENSE) for terms.
