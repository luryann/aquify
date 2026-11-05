# aquify Platform — Secrets Management

This document describes how production secrets are managed for the aquify platform.

## Guiding Principle

No secret ever enters version control. The repository contains only example files
(`.env.example`, `.env.staging.example`) with placeholder values. Real credentials
live exclusively on production infrastructure, managed by the procedures below.

---

## Secret Storage

Production secrets live on the server in three files under `/opt/aquify/`, created from the
templates in `deploy/env/` (owner `root:aquify`, mode `640`, never committed, never in an image):

| File | Contains | Used by |
|---|---|---|
| `.env.production` | infrastructure secrets: Postgres superuser/owner/app/backup/Keycloak-DB passwords, Redis, MinIO root, Keycloak bootstrap admin, Grafana, `AQUIFY_DOMAIN`, `ACME_EMAIL` | compose variable interpolation (`--env-file`), host scripts. **Not** handed to the app containers |
| `.env.app` | API/worker runtime config: `DATABASE_URL` (restricted app role), `REDIS_URL`, storage keys, OIDC, Keycloak admin client, Stripe, Postmark, signing/encryption keys | `aquify-api`, `aquify-worker` only (`env_file`) |
| `.env.backup` | `MINIO_ACCESS_KEY`/`SECRET_KEY`, `GPG_RECIPIENT`, `RCLONE_REMOTE`, `ALERT_WEBHOOK` | `backup.sh`, `restore.sh` |

`release.env` (image tag and registry namespace, no secrets) is separate and owned by the deploy
user, so deploy automation never writes a secrets file. The scripts parse env files; they never
`source` them.

`deploy/scripts/preflight.sh` validates all of the above (placeholders, formats, permissions,
consistency between files). The templates are checked against the compose file and the server
source by `go test ./internal/deploycheck`, so a documented variable that nothing reads (the old
`SESSION_SIGNING_KEY`/`SESSION_ENCRYPTION_KEY`) fails the build.

### What each secret is

- **Database roles** (`.env.production`): four distinct passwords for `aquify_admin` (bootstrap
  superuser, break-glass only), `aquify_owner` (schema owner, migrations only), `aquify_app`
  (API/worker; the same value appears in `DATABASE_URL`), `aquify_backup` (read-only), plus the
  `keycloak` role. 16+ characters of `[A-Za-z0-9_-]`.
- **`INTEGRATIONS_ENCRYPTION_KEY`, `OIDC_STATE_SIGNING_KEY`**: 32+ characters. Losing the first
  makes stored third-party credentials unreadable; escrow it.
- **MinIO**: root credentials are administration only. The app uses a scoped service account.
- **Stripe**: `sk_live_...`, the platform webhook secret and the Connect webhook secret.
- **Postmark**: server token (also used as Keycloak's SMTP credentials), from address, webhook auth.
- **Keycloak**: `OIDC_CLIENT_SECRET` and `KEYCLOAK_ADMIN_CLIENT_SECRET` are written into the
  realm by `keycloak/render_realm.py`, so they only need to be set once, in `.env.app`.

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
   sudo nano /opt/aquify/.env.production   # or .env.app for application secrets
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

For a database role password (`AQUIFY_*_DB_PASSWORD`, `KEYCLOAK_DB_PASSWORD`, `POSTGRES_PASSWORD`):
edit `.env.production`, recreate the container so it sees the new value (`... up -d postgres`; a brief database restart), then re-apply the roles (idempotent, sets every password again):
```bash
docker compose --project-directory /opt/aquify --env-file .env.production --env-file release.env \
  -f docker-compose.production.yml exec postgres bash /docker-entrypoint-initdb.d/10-bootstrap.sh
```
If it is the application role, also change the password inside `DATABASE_URL` in `.env.app`
(they must match; `preflight.sh` checks), then restart the api and worker.

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
