# Aquify — production deployment guide

Single Linux host (Ubuntu 22.04/24.04 or Debian 12), Docker Compose, GitHub Actions
for images. This guide matches the files in this directory; every claim below is
backed by a test or is listed under **What is not yet verified**.

Related: `docs/production-readiness-tracker.md` (issue list and status),
`deploy/secrets/README.md` (secret handling), `docs/runbooks/multi-tenancy-operations.md`.

---

## 1. Layout on the server

```
/opt/aquify/
  docker-compose.production.yml
  .env.production        infra secrets + compose ${VARS}   root:aquify 640   (never written by scripts)
  .env.app               API/worker runtime config         root:aquify 640
  .env.backup            backup settings                   root:aquify 640
  release.env            IMAGE_TAG, GHCR_NAMESPACE         aquify  644       (rewritten by promote/rollback)
  caddy/Caddyfile        ingress routes
  postgres/init/         role bootstrap (roles.sql.in, 10-bootstrap.sh)
  keycloak/production-realm.json     rendered, see 4
  prometheus/ grafana/ loki/ docs/observability/   monitoring config
  scripts/               promote.sh rollback.sh backup.sh restore.sh preflight.sh lib.sh smoke.sh
/var/log/aquify/  /var/backups/aquify/
```

Why two secret files: `.env.production` feeds compose variable interpolation and holds the
Postgres superuser and schema-owner passwords; only `.env.app` is handed to the API and worker
containers, so those privileged passwords never reach the long-running application.
`release.env` is separate so deploy automation never has to write a root-owned secrets file.

## 2. Network and DNS

Open 80/443 (and 443/udp) to the internet; nothing else. All other published ports are bound to
127.0.0.1 (enforced by a test). Create A/AAAA records for the four names under your domain, all
pointing at the server: `app.`, `api.`, `auth.`, `files.`. The `caddy` service obtains and renews
certificates automatically. For a rehearsal VM without DNS use a wildcard name such as
`203-0-113-10.sslip.io` as `AQUIFY_DOMAIN`.

If Cloudflare proxies the names, see the note at the top of `caddy/Caddyfile`.

## 3. First-time setup

```bash
# 3.1 host
curl -fsSL https://get.docker.com | sh
sudo useradd -r -m -d /opt/aquify -s /bin/bash aquify
sudo usermod -aG docker aquify
sudo mkdir -p /var/log/aquify /var/backups/aquify && sudo chown -R aquify:aquify /opt/aquify /var/log/aquify /var/backups/aquify

# 3.2 Loki logging driver (every service logs through it; compose will not start without it)
sudo docker plugin install grafana/loki-docker-driver:latest --alias loki --grant-all-permissions

# 3.3 copy the deploy bundle from a checkout of the release you are installing
rsync -a deploy/ aquify@HOST:/opt/aquify/
rsync -a docs/observability/ aquify@HOST:/opt/aquify/docs/observability/   # alert rules + dashboards, mounted by compose
```

```bash
# 3.4 configuration (as root)
cd /opt/aquify
install -m 640 -o root -g aquify env/production.env.example .env.production
install -m 640 -o root -g aquify env/app.env.example .env.app
# edit both: replace every CHANGE_ME and example.com value. Secrets: openssl rand -hex 24 (or 32)
printf 'IMAGE_TAG=sha-XXXXXXX\nGHCR_NAMESPACE=your-ghcr-owner\n' > release.env && chown aquify: release.env
# 3.5 validate (prints every problem; exit 0 only when clean)
sudo -u aquify ./scripts/preflight.sh
```

`preflight.sh` checks placeholders, secret format and uniqueness, file permissions, that
`DATABASE_URL` uses the restricted app role and not the owner/superuser, that every hostname derives
from `AQUIFY_DOMAIN`, https everywhere, no dev switches, and (when Docker is present)
`docker compose config`.

```bash
# 3.6 infrastructure first, then the schema, then the app
cd /opt/aquify
C="docker compose --project-directory /opt/aquify --env-file .env.production --env-file release.env -f docker-compose.production.yml"
$C build minio                      # MinIO has no published image any more: built from pinned source (minio/Dockerfile, ~3 min)
$C up -d postgres redis minio       # first start runs postgres/init/10-bootstrap.sh: creates the roles
$C --profile tools run --rm aquify-migrate          # = aquify-api migrate up, as the schema-owner role
# 3.7 MinIO: create the bucket and a scoped service account, put its keys in .env.app
#     (mc alias set ...; mc mb local/aquify; mc admin user svcacct add --access-key ... local minioadmin-user)
# 3.8 Keycloak realm
python3 keycloak/render_realm.py --app-env .env.app --out keycloak/production-realm.json
# 3.9 everything
sudo cp systemd/aquify*.service systemd/aquify-backup.timer /etc/systemd/system/ && sudo systemctl daemon-reload
sudo systemctl enable --now aquify aquify-backup.timer
```

Database roles (`postgres/init/roles.sql.in`): `aquify_owner` owns the schema and is used only by
migrations; `aquify_app` is what the API/worker use (NOSUPERUSER, NOBYPASSRLS, DML only, so
row-level security always applies); `aquify_backup` is read-only with BYPASSRLS for `pg_dump`;
`keycloak` owns its own database. The API refuses to start if its `DATABASE_URL` role is a
superuser or has BYPASSRLS. To rotate a database password: change it in `.env.production`, then
`$C exec postgres bash /docker-entrypoint-initdb.d/10-bootstrap.sh` (idempotent), update `.env.app`
if it is the app role, and restart.

## 4. Keycloak

`keycloak/render_realm.py` builds the production realm from `realm-aquify-local.json`: no seed
users, client secrets taken from `.env.app`, https redirect URIs, TLS required, e-mail verification
on, Postmark SMTP. It refuses to write a realm containing development values (tested). Keycloak
imports it only on first start of an empty realm. Create the first administrator by hand through an
SSH tunnel to `127.0.0.1:8081` (the admin console is not exposed publicly), with an `org_id`
attribute and the `admin` realm role, and enrol MFA.

## 5. Releases

Images are built by `.github/workflows/deploy-staging.yml` (API, admin for staging, and
`aquify-admin-production`, which carries `NEXT_PUBLIC_APP_ENV=production`; browser-bundle values are
fixed at build time so production must not reuse the staging admin image). Production:
GitHub Actions → "Deploy to Production" → image tag + `DEPLOY`, approved by an environment reviewer.
On the host that runs `scripts/promote.sh <tag>`:

1. takes a deploy lock; validates the tag; records the last healthy release as the rollback target
2. writes the tag to `release.env` (never touches the secrets files); pulls images
3. **runs the migration with the new image before any running container changes**; a failed
   migration aborts with the old release still serving
4. recreates api/admin/worker; requires `/api/v1/health/ready` = 200 and a running worker
5. otherwise runs `rollback.sh`

Migrations apply before the swap and rollback does not undo them, so every migration must be
backward compatible with the previous release (add columns/tables first; drop in a later release).
A destructive mistake is recovered from backup, not by rollback.

Manual: `./scripts/promote.sh sha-abc1234`, `./scripts/rollback.sh`.
Staging uses the same flow: `deploy-staging.yml` calls `promote.sh` with `COMPOSE_FILE=docker-compose.staging.yml`
and `SECRETS_ENV=.env.staging`. The staging host needs the same file split as production: `.env.staging`
(compose interpolation: the role passwords, `MINIO_*`, `KEYCLOAK_*`, `GRAFANA_*`), `.env.staging.app`
(api/worker runtime config, `APP_ENV=staging`), `release.env`, `postgres/init/`, `minio/` and a rendered
`keycloak/staging-realm.json` (`render_realm.py`). Staging compose validates with `docker compose config`
but a staging host deploy has **not** been run.

## 6. Backup and restore

`scripts/backup.sh` (daily, `aquify-backup.timer`): `pg_dump` of the app database as the read-only
role inside the postgres container (verified readable), a MinIO mirror, a Keycloak realm export and
a manifest, then one **gpg-encrypted** archive, uploaded with rclone; the plaintext snapshot is
deleted. Any failed step fails the run and alerts. Settings in `.env.backup`
(`MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `GPG_RECIPIENT`, `RCLONE_REMOTE`, optional `ALERT_WEBHOOK`).

Not covered: **Keycloak users and credentials** (Keycloak's own database), Redis jobs, and the
`.env.*` files. Escrow the env files and the GPG private key separately and take a `pg_dump` of the
`keycloak` database if accounts must survive host loss.

`scripts/restore.sh <timestamp|archive> [--yes] [--skip-objects] [--dry-run]` works on the same host or
a fresh one (bootstrap the roles, then restore): verifies the snapshot first, stops api/worker/admin
**and keycloak**, restores in one transaction that aborts on the first error, checks the schema
version and club count against the manifest, restores objects (missing tools are an error), runs
migrations, starts services and requires readiness.

## 7. Operating

```bash
sudo systemctl status aquify ; journalctl -u aquify -f
$C ps ; $C logs -f aquify-api
curl -fsS https://api.<domain>/api/v1/health/ready
SMOKE_AUTH_TOKEN=... SMOKE_ORG_ID=... API_BASE_URL=https://api.<domain> ./scripts/smoke.sh
```

Metrics: API and worker serve Prometheus metrics on `METRICS_ADDR` (`:9091`) inside the compose
network; Prometheus/Grafana bind to 127.0.0.1 (use an SSH tunnel). Redis is the job broker with
`noeviction`: at its 512 MB cap enqueues fail loudly rather than dropping jobs; alert on memory use.

## 8. Tests that guard this directory

| Test | Command | Proves |
|---|---|---|
| Compose/workflow policy, templates, realm, ingress | `cd server && go test ./internal/deploycheck` | strict YAML, env split, noeviction, only ingress public, templates match compose and code, realm has no seed users |
| Role model | same package, needs `DATABASE_URL` (superuser, disposable DB) | roles bootstrap; all migrations run as the non-superuser owner; app role is subject to RLS and cannot do DDL |
| Migrations | `cd server && go test ./internal/dbmigrate ./cmd/api` | fresh apply, idempotent, concurrent-safe, failure rolls back |
| Promote/rollback | `bash deploy/scripts/test-promote-rollback.sh` | ordering, env files, lock, failure paths (stub docker) |
| Preflight | `bash deploy/scripts/test-preflight.sh` | each misconfiguration is caught |
| Backup/restore | `bash deploy/scripts/test-backup-restore.sh` | encrypted archive, no plaintext left, ordering, all-or-nothing, failure paths (stub tools) |
| Bootstrap wrapper | `bash deploy/postgres/test-bootstrap.sh` | input validation |

## 9. Local rehearsal and what it does not prove

`scripts/rehearse-local.sh <dir under $HOME>` stages a production-shaped deployment on a dev machine
(synthetic secrets, `*.localtest.me`, Caddy internal CA via `rehearsal.override.yml`). Docker for
macOS without Docker Desktop: `. .tools/docker/env.sh` (Colima + docker + compose, all under the
gitignored `.tools/docker`). The stage dir must live under `$HOME`; Colima shares nothing else, so
bind mounts elsewhere arrive as empty directories.

Run this way (arm64 images, Colima VM): `docker compose config`, image builds, role bootstrap by the
real postgres image, migrations, Caddy routing and blocked Keycloak paths, Keycloak realm import, the
Loki driver, Prometheus scraping, presigned upload/download through `files.<domain>`, Redis restart and
`noeviction` at the cap, encrypted backup, restore onto emptied volumes, and promote / failed-migration /
rollback with the real scripts. **Not run:** ACME certificate issuance on a public name, amd64 images,
Stripe and Postmark with real test accounts (readiness reports 503 until valid keys are present), a
browser sign-in with MFA, and the restore on a second host. See
`docs/production-readiness-tracker.md` for the per-item record.
