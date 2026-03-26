# DARE Platform — Staging Environment Guide

## Purpose and Scope

The staging environment is a production-like deployment running on the same VPS as production, using separate container ports, compose file, and DNS entries. Its purpose is:

- Validate every release against real (anonymized) data before it reaches production.
- Run the full smoke test gate (`make cutover-smoke`) as a required CI step.
- Allow engineers to test Stripe, Postmark, and Keycloak flows with real provider test-mode credentials without touching production accounts.
- Serve as the restore drill target for quarterly DR exercises (see `restore-drill-checklist.md`).

Staging is **not** a development sandbox. It runs with `AUTH_ENABLE_DEV_STUB_TOKENS=false` and real provider credentials (test mode keys). Changes made to staging data are disposable — it is refreshed monthly from an anonymized production snapshot.

---

## Server Setup

Staging and production share the same VPS but are isolated by:

- Separate Docker Compose project: `docker-compose.staging.yml`
- Separate environment file: `.env.staging`
- Separate container port bindings (see table below)
- Separate nginx server blocks

### Port Allocation

| Service | Production port | Staging port |
|---|---|---|
| Go API | 8080 | 8180 |
| Next.js admin | 3000 | 3100 |
| Uptime Kuma | 3001 | (shared with production) |
| PostgreSQL | 5432 | 5433 |
| Redis | 6379 | 6380 |
| MinIO API | 9000 | 9100 |
| MinIO console | 9001 | 9101 |
| Mailpit SMTP | 1025 | 1125 |
| Mailpit UI | 8025 | 8125 |

### Compose Project

Run staging with a separate project name to avoid container naming conflicts:

```bash
docker compose -f /opt/dare/docker-compose.staging.yml \
               --project-name dare-staging \
               --env-file /opt/dare/.env.staging \
               up -d
```

---

## DNS

Both staging DNS entries point to the same VPS IP as production, proxied through Cloudflare:

```
staging-api.dare.club      A    203.0.113.10    Proxied
staging-admin.dare.club    A    203.0.113.10    Proxied
```

nginx routes these hostnames to staging container ports (8180 and 3100 respectively).

### nginx server blocks

Add to `/etc/nginx/sites-available/dare-staging`:

```nginx
server {
    listen 443 ssl http2;
    server_name staging-api.dare.club;

    ssl_certificate     /etc/ssl/certs/dare-club-cloudflare-origin.pem;
    ssl_certificate_key /etc/ssl/private/dare-club-cloudflare-origin.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    location / {
        proxy_pass         http://127.0.0.1:8180;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}

server {
    listen 443 ssl http2;
    server_name staging-admin.dare.club;

    ssl_certificate     /etc/ssl/certs/dare-club-cloudflare-origin.pem;
    ssl_certificate_key /etc/ssl/private/dare-club-cloudflare-origin.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    location / {
        proxy_pass         http://127.0.0.1:3100;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

---

## Staging-Specific Configuration

All staging secrets live in `/opt/dare/.env.staging`. This file is **not** committed to the repository — it is managed via the team 1Password vault (see Access Credentials below).

The following variables differ from production:

```bash
# Environment identity
APP_ENV=staging
LOG_LEVEL=debug

# Auth — no dev stub tokens on staging
AUTH_ENABLE_DEV_STUB_TOKENS=false
AUTH_ALLOW_DEV_SESSION_ISSUE=false

# OIDC — staging Keycloak realm
OIDC_AUTHORIZE_URL=https://auth.dare.club/realms/dare-staging/protocol/openid-connect/auth
OIDC_TOKEN_URL=https://auth.dare.club/realms/dare-staging/protocol/openid-connect/token
OIDC_JWKS_URL=https://auth.dare.club/realms/dare-staging/protocol/openid-connect/certs
OIDC_REDIRECT_URL=https://staging-admin.dare.club/auth/callback
OIDC_EXPECTED_ISSUER=https://auth.dare.club/realms/dare-staging
OIDC_CLIENT_ID=dare-staging-admin
# OIDC_CLIENT_SECRET → from 1Password

# Database — separate staging Postgres instance
DATABASE_URL=postgres://dare_staging:CHANGEME@localhost:5433/dare_staging?sslmode=disable

# Redis — staging instance
REDIS_URL=redis://localhost:6380/0

# Object storage — staging MinIO bucket
S3_ENDPOINT=http://localhost:9100
MINIO_BUCKET=dare-staging
# S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY → from 1Password

# Stripe — TEST MODE keys (not live keys)
STRIPE_SECRET_KEY=sk_test_...       # from 1Password
STRIPE_WEBHOOK_SECRET=whsec_test_... # from 1Password

# Postmark — sandbox stream
POSTMARK_SERVER_TOKEN=...           # from 1Password (sandbox stream token)
POSTMARK_FROM_EMAIL=noreply-staging@dare.club
# POSTMARK_STREAM should be set to the sandbox stream ID

# Email preview via Mailpit (staging only)
# Mailpit UI: http://localhost:8125
# Mailpit SMTP: localhost:1125

# Frontend
NEXT_PUBLIC_APP_ENV=staging
NEXT_PUBLIC_API_BASE_URL=https://staging-api.dare.club
NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS=false
```

---

## Deploying to Staging

### Automated: CI Auto-Deploy on main Merge

Every merge to `main` triggers the CI pipeline (GitHub Actions), which:

1. Runs `make test` and `make lint`.
2. Builds and pushes Docker images tagged `main-<sha>` to the container registry.
3. SSHs to the VPS and runs the staging deploy script:

```bash
ssh deploy@vps.dare.club "
  cd /opt/dare &&
  IMAGE_TAG=main-${GITHUB_SHA} docker compose \
    -f docker-compose.staging.yml \
    --project-name dare-staging \
    --env-file .env.staging \
    up -d --pull always dare-api dare-admin dare-worker
"
```

4. Runs `make cutover-smoke API_BASE_URL=https://staging-api.dare.club AUTH_TOKEN=$STAGING_SMOKE_TOKEN ORG_ID=org-staging` as a required post-deploy gate.
5. If smoke tests fail, the pipeline fails and the team is alerted via Slack `#dare-deploys`.

### Manual Deploy

To force a specific image tag to staging:

```bash
ssh deploy@vps.dare.club
cd /opt/dare
IMAGE_TAG=main-abc1234 docker compose \
  -f docker-compose.staging.yml \
  --project-name dare-staging \
  --env-file .env.staging \
  up -d --pull always dare-api dare-admin dare-worker
```

---

## Smoke Test Gate

Before any production deploy, the staging smoke test must pass:

```bash
make cutover-smoke \
  API_BASE_URL=https://staging-api.dare.club \
  AUTH_TOKEN=<staging-smoke-token-from-1password> \
  ORG_ID=org-staging
```

This runs the smoke test suite from `deploy/scripts/smoke-test.sh`, which verifies:

- `GET /healthz` returns 200
- `GET /api/v1/auth/session` returns a valid session
- `GET /api/v1/orgs/{orgId}/members` returns a list
- `GET /api/v1/orgs/{orgId}/meets` returns a list
- Import upload endpoint returns 200
- Invoice endpoint returns 200
- Audit log endpoint returns 200

All checks must return HTTP 2xx for the gate to pass.

---

## Staging Data Policy

- Staging uses an **anonymized copy of production data**, refreshed monthly on the first Sunday of each month.
- Anonymization replaces: first names, last names, email addresses, phone numbers, and Stripe customer IDs.
- Stripe payment history in staging uses Stripe test-mode records only — no real payment data is copied.
- A freshly anonymized dump is stored in the team's secure storage (B2 bucket `dare-backups-staging`).
- The monthly refresh procedure:
  1. Pull the latest production anonymized dump from B2.
  2. Drop and recreate the staging DB: `psql -h localhost -p 5433 -c "DROP DATABASE dare_staging; CREATE DATABASE dare_staging;"`
  3. Restore: `pg_restore -h localhost -p 5433 -d dare_staging < /tmp/staging-anon-dump.dump`
  4. Run any pending migrations: `make db-up DATABASE_URL=postgres://...@localhost:5433/dare_staging`
  5. Reset MinIO staging bucket with anonymized file fixtures.

---

## Access Credentials

All staging secrets are stored in the **DARE Engineering** 1Password vault:

| Secret | 1Password item |
|---|---|
| `.env.staging` full file | `dare-staging-env` |
| Staging smoke test auth token | `dare-staging-smoke-token` |
| Staging Stripe test keys | `dare-stripe-test-keys` |
| Staging Postmark sandbox token | `dare-postmark-sandbox` |
| Staging MinIO credentials | `dare-minio-staging` |
| Staging OIDC client secret | `dare-oidc-staging` |
| VPS SSH deploy key | `dare-vps-deploy-key` |

Request access from the engineering lead. Do not store secrets in plaintext outside of 1Password or the server's `/opt/dare/.env.staging` (chmod 600, owned by `dare` user).

---

## Resetting Staging

To fully reset staging to a clean state:

```bash
ssh deploy@vps.dare.club
cd /opt/dare

# Stop staging services
docker compose -f docker-compose.staging.yml --project-name dare-staging down -v

# Drop and recreate staging DB
psql postgres://postgres:postgres@localhost:5433/postgres \
  -c "DROP DATABASE IF EXISTS dare_staging; CREATE DATABASE dare_staging;"

# Re-run migrations
DATABASE_URL=postgres://dare_staging:CHANGEME@localhost:5433/dare_staging \
  make db-up

# Restart staging services
IMAGE_TAG=main-latest docker compose \
  -f docker-compose.staging.yml \
  --project-name dare-staging \
  --env-file .env.staging \
  up -d
```

After reset, re-run the anonymized data restore if a data-representative environment is needed.

---

## Mailpit Email Preview

Staging routes all outbound email through Mailpit (SMTP relay running locally on port 1125). To view emails sent during staging testing:

```bash
ssh -L 8125:localhost:8125 deploy@vps.dare.club
```

Then open `http://localhost:8125` in a browser. All staging emails appear here — no real email is ever delivered.

For testing Postmark-specific webhook flows (bounce, delivery, spam), use the Postmark sandbox stream message stream, which accepts test sends without real delivery.
