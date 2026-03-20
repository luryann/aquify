# DARE Platform

DARE Platform is a full-stack swim club operations system replacing TeamUnify/SportsEngine Motion workflows.

## Monorepo Layout

- `server/` Go API service
- `admin/` Next.js TypeScript admin app
- `db/migrations/` SQL migrations
- `internal/meetfiles/` CL2/SD3/ZIP interoperability code
- `docs/` planning, decisions, and specs

## Local Setup

1. Ensure prerequisites are installed:
- Go 1.24+
- Node.js 22+ and npm
- One PostgreSQL runtime option:
  - Docker Desktop (or Docker Engine + Compose plugin), or
  - Local PostgreSQL server (Postgres.app/Homebrew/PostgreSQL installer)

2. Copy environment variables:
- `cp .env.example .env`
- `cp .env.local.example .env.local`

3. Install admin dependencies:
- `cd admin && npm install`

4. Start local infrastructure (Docker path):
- `make infra-up`

5. If not using Docker, start local PostgreSQL and create the database:
- `pg_isready -h 127.0.0.1 -p 5432`
- `createdb aquify` (if it does not exist yet)
- If you are using Postgres.app default user auth, set `DATABASE_URL` in `.env.local` to your macOS user (example: `postgres://<your-user>@localhost:5432/aquify?sslmode=disable`)

6. Apply DB migrations:
- `make db-up`
- `make db-status`

7. Run both services:
- `make dev`

8. Open the apps:
- Admin UI: [http://localhost:3000](http://localhost:3000)
- API health: [http://localhost:8080/api/v1/health/live](http://localhost:8080/api/v1/health/live)

## Developer Commands

- `make dev` run API + admin together
- `make dev-api` run only API
- `make dev-admin` run only admin
- `make test` run API tests and admin production build verification
- `make lint` run API static analysis and admin lint checks
- `make infra-up` start local Postgres/Redis/MinIO/Mailpit
- `make infra-down` stop local infrastructure
- `make infra-reset` stop infrastructure and delete local data volumes
- `make db-up` apply DB migrations with Goose
- `make db-down` roll back the most recent DB migration
- `make db-status` show DB migration status
- `make cutover-rehearsal INPUT_DIR=/path/to/export-dir` run the Step 24 migration CLI against a final export set
- `make cutover-smoke API_BASE_URL=https://api.example.com AUTH_TOKEN=... ORG_ID=...` run post-cutover HTTP smoke checks
- `make cutover-certify` run the import/export certification fixture suite
- `make cutover-billing-check API_BASE_URL=https://api.example.com AUTH_TOKEN=... ORG_ID=...` run the billing reconciliation endpoint

## Local Infrastructure Ports and Credentials

| Service  | Port(s) | Default Credentials | Notes |
|---|---:|---|---|
| Postgres | `5432` | `postgres` / `postgres` | Database name: `aquify` |
| Redis | `6379` | n/a | Local development cache/queue |
| MinIO (S3 API) | `9000` | `minioadmin` / `minioadmin` | Endpoint: `http://localhost:9000` |
| MinIO Console | `9001` | `minioadmin` / `minioadmin` | Web console for buckets/objects |
| Mailpit SMTP | `1025` | n/a | SMTP capture endpoint |
| Mailpit UI | `8025` | n/a | Web inbox for captured mail |

Credentials and ports can be changed in `.env.local` (used by compose via `make infra-*`).

## Auth Configuration Notes

- `AUTH_ENABLE_DEV_STUB_TOKENS` defaults to `true` in development and should be `false` outside local development.
- OIDC browser login flow requires these variables: `OIDC_AUTHORIZE_URL`, `OIDC_TOKEN_URL`, `OIDC_JWKS_URL`, `OIDC_REDIRECT_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and `OIDC_STATE_SIGNING_KEY`.
- Callback validation enforces signed `state` payload verification and cryptographic `id_token` validation (JWKS signature + `iss`/`aud`/`exp`/`iat`, plus `azp` when multiple audiences are present).
- The frontend now bootstraps auth state from `GET /api/v1/auth/session` and uses cookie-backed refresh/revoke flows with the `aquify_csrf_token` cookie echoed in `X-CSRF-Token`.
- Production builds should set `NEXT_PUBLIC_APP_ENV=production` and `NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS=false` so stub login and role-switching affordances stay hidden.
- Supported MFA enrollment/challenge factors are currently `totp` and `backup_code`.

## Database Migrations

The project uses [Goose](https://github.com/pressly/goose) for SQL migrations. `make db-up`, `make db-down`, and `make db-status` run Goose via `go run` and read `DATABASE_URL` from `.env.local` (falling back to `.env`, then a local default DSN).

### Troubleshooting

- `dial tcp 127.0.0.1:5432: connect: connection refused`
  - PostgreSQL server is not running. Start Docker infra (`make infra-up`) or start your local PostgreSQL service.
- `password authentication failed for user "postgres"`
  - Your local server likely does not use a `postgres/postgres` login. Update `DATABASE_URL` to your actual local DB user.
- Verify connectivity quickly:
  - `pg_isready -h 127.0.0.1 -p 5432`
  - `psql -h 127.0.0.1 -p 5432 -d postgres -c "select current_user, current_database();"`
