# aquify Platform — Secrets Management

This document describes how production secrets are managed for the aquify platform.

## Guiding Principle

No secret ever enters version control. The repository contains only example files
(`.env.example`, `.env.staging.example`) with placeholder values. Real credentials
live exclusively on production infrastructure, managed by the procedures below.

---

## Secret Storage

### Production VPS

All runtime secrets are stored in a single env file on the server:

```
/opt/aquify/.env.production
```

Ownership and permissions:
```
chown root:aquify /opt/aquify/.env.production
chmod 640 /opt/aquify/.env.production
```

Only `root` and members of the `aquify` system group can read this file. The `aquify`
service user (which runs the systemd unit) is a member of that group. The file is
**never** committed to git and is **never** included in Docker images.

### Staging VPS

Same pattern at `/opt/aquify/.env.staging`, owned `root:aquify`, mode `640`.

---

## Secret Categories

### Database Credentials
| Variable | Description |
|---|---|
| `POSTGRES_PASSWORD` | PostgreSQL superuser / application user password |
| `DATABASE_URL` | Full DSN including credentials: `postgres://aquify:<pw>@postgres:5432/aquify?sslmode=disable` |

### Redis
| Variable | Description |
|---|---|
| `REDIS_PASSWORD` | Redis AUTH password |
| `REDIS_URL` | Full URL: `redis://:${REDIS_PASSWORD}@redis:6379/0` |

### Object Storage (MinIO / S3-compatible)
| Variable | Description |
|---|---|
| `MINIO_ROOT_USER` | MinIO root user (equivalent to AWS Access Key ID) |
| `MINIO_ROOT_PASSWORD` | MinIO root password (equivalent to AWS Secret Access Key) |
| `STORAGE_ENDPOINT` | Storage endpoint (required outside development; `S3_ENDPOINT` is a legacy alias) |
| `STORAGE_ACCESS_KEY` | App-scoped access key (not root key) |
| `STORAGE_SECRET_KEY` | App-scoped secret key |
| `STORAGE_REGION` | Region (use `us-east-1` for MinIO) |
| `STORAGE_BUCKET` | Target bucket name |
| `INTEGRATIONS_ENCRYPTION_KEY` | Required at startup outside development; encrypts stored integration credentials |
| `WORKER_SCHEDULED_ORG_IDS` | Required in staging/production for org-scoped scheduled jobs |
| `PLATFORM_ORG_ID` | Optional; enables the `/api/v1/platform/*` operator plane |
| `STRIPE_CONNECT_REQUIRED` | Defaults to on outside development; each club uses its own Stripe Connect account |

### Stripe (Payments)
| Variable | Description |
|---|---|
| `STRIPE_SECRET_KEY` | Live secret key (`sk_live_...`) |
| `STRIPE_WEBHOOK_SECRET` | Webhook endpoint signing secret (`whsec_...`) |

### Postmark (Email)
| Variable | Description |
|---|---|
| `POSTMARK_SERVER_TOKEN` | Server API token |
| `POSTMARK_FROM_EMAIL` | Verified sender address |
| `POSTMARK_WEBHOOK_AUTH_HEADER_NAME` | Custom header name for webhook auth |
| `POSTMARK_WEBHOOK_AUTH_HEADER_VALUE` | Custom header value for webhook auth |

### Keycloak / OIDC
| Variable | Description |
|---|---|
| `KEYCLOAK_ADMIN_PASSWORD` | Keycloak admin console password |
| `OIDC_CLIENT_ID` | aquify app client ID in Keycloak realm |
| `OIDC_CLIENT_SECRET` | aquify app client secret |
| `OIDC_STATE_SIGNING_KEY` | HMAC key for signing OAuth2 state parameter (32+ random bytes, hex-encoded) |
| `OIDC_AUTHORIZE_URL` | Authorization endpoint |
| `OIDC_TOKEN_URL` | Token endpoint |
| `OIDC_JWKS_URL` | JWKS endpoint for token validation |
| `OIDC_REDIRECT_URL` | Registered callback URL |

### Session / CSRF
| Variable | Description |
|---|---|
| `SESSION_SIGNING_KEY` | HMAC key for session token signing (32+ random bytes, hex-encoded) |
| `SESSION_ENCRYPTION_KEY` | AES key for session payload encryption (32 bytes, hex-encoded) |
| `SECURITY_CSRF_HEADER_NAME` | CSRF header name (default: `X-CSRF-Token`) |

### Observability
| Variable | Description |
|---|---|
| `GRAFANA_ADMIN_PASSWORD` | Grafana admin password |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint for traces |

---

## Secret Generation

Use a cryptographically secure method to generate secrets. Examples:

```bash
# 32-byte hex key (for signing/encryption keys)
openssl rand -hex 32

# 64-character base64 string (for API tokens)
openssl rand -base64 48

# Random password (for DB, Redis, MinIO)
openssl rand -base64 24 | tr -d '=+/' | cut -c1-32
```

Never reuse secrets across environments. Staging and production must always have
distinct credentials.

---

## Secret Rotation Procedure

1. **Generate new value**
   ```bash
   openssl rand -hex 32   # or appropriate command for the secret type
   ```

2. **Update .env.production on the VPS**
   ```bash
   sudo nano /opt/aquify/.env.production
   # Edit the relevant variable, save and exit
   ```

3. **Restart the affected service**
   ```bash
   sudo systemctl reload aquify        # for app secrets (triggers ExecReload)
   # or for a full restart:
   sudo systemctl restart aquify
   ```

4. **Verify health**
   ```bash
   curl -sf https://api.aquify.club/api/v1/health/live && echo OK
   ```

5. **Revoke the old secret** at the provider (Stripe dashboard, Postmark, etc.)

6. **Log the rotation event** in the ops log:
   ```bash
   echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) SECRET_ROTATION STRIPE_WEBHOOK_SECRET rotated by $(whoami)" \
     >> /var/log/aquify/ops.log
   ```

For `POSTGRES_PASSWORD` rotation, also run:
```bash
docker exec -it aquify-production-postgres-1 \
  psql -U aquify -c "ALTER USER aquify PASSWORD 'newpassword';"
```
Then update `DATABASE_URL` and `POSTGRES_PASSWORD` in `.env.production` and restart.

---

## CI/CD Secrets

Secrets needed during the GitHub Actions deploy pipeline are stored as
**GitHub Actions Secrets** (Settings → Secrets and variables → Actions):

| Secret name | Used for |
|---|---|
| `STAGING_SSH_HOST` | Staging VPS hostname |
| `STAGING_SSH_USER` | SSH username on staging VPS |
| `STAGING_SSH_KEY` | Private SSH key for staging deploy user |
| `PRODUCTION_SSH_HOST` | Production VPS hostname |
| `PRODUCTION_SSH_USER` | SSH username on production VPS |
| `PRODUCTION_SSH_KEY` | Private SSH key for production deploy user |
| `GHCR_TOKEN` | Optional registry token for a different package owner; defaults to `GITHUB_TOKEN` |
| `SLACK_WEBHOOK` | Optional Slack incoming webhook; notifications are skipped when unset |
| `SMOKE_TEST_AUTH_TOKEN` | Bearer token for post-deploy smoke tests |
| `SMOKE_TEST_ORG_ID` | Org ID for smoke test requests |

The deploy workflow does **not** copy `.env.production` during deploys. That file
is provisioned once during server setup and owned by the ops team. Image tags are
passed with the registry namespace (`GHCR_NAMESPACE`) at deploy time.

---

## HashiCorp Vault — Future Migration Path

When the team grows beyond 3 engineers, migrate to Vault with AppRole auth:

1. **Deploy Vault** in HA mode with auto-unseal (AWS KMS or GCP Cloud KMS).
2. **Create a `aquify-production` AppRole** with a policy granting read-only access
   to `secret/aquify/production/*`.
3. **Replace `.env.production`** with a Vault Agent sidecar that writes a tempfs
   env file at container start using `vault agent -config=/etc/vault-agent.hcl`.
4. **Rotate secrets automatically** using Vault's dynamic secrets for PostgreSQL
   (short-lived credentials) and the AWS secrets engine for S3 access keys.
5. **Audit log all secret reads** via Vault's audit device to the SIEM.

Short-term migration steps:
- Phase 1: Move `STRIPE_SECRET_KEY` and `OIDC_CLIENT_SECRET` to Vault first.
- Phase 2: Migrate DB credentials to dynamic secrets.
- Phase 3: Retire `.env.production` entirely.

---

## What NEVER Goes in the Repository

The following must NEVER be committed to git under any circumstances:

- `.env.production`
- `.env.staging`
- Any file containing a real API key, token, or password
- Private SSH keys
- TLS private keys or certificates
- Stripe live keys (`sk_live_*`, `rk_live_*`)
- Postmark server tokens
- Keycloak client secrets
- Session signing or encryption keys
- MinIO root credentials

The `.gitignore` at the repo root blocks `*.env.production`, `*.env.staging`, and
`*.pem`. Always run `git diff --staged` before committing to confirm no secrets
are staged. The CI pipeline runs `gitleaks` on every push as an automated guard.
