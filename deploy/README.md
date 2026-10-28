# DARE Platform — Deployment Guide

This document covers deploying the DARE platform to a self-managed VPS behind
Cloudflare, using Docker Compose and GitHub Actions CI/CD.

---

## Prerequisites

### On the VPS
- Ubuntu 22.04 LTS or Debian 12
- Docker Engine 25+ and Docker Compose V2 (`docker compose`, not `docker-compose`)
- SSH access as a non-root deploy user (`dare`)
- Ports 80/443 open to Cloudflare IP ranges only (via `ufw` or `iptables`)
- Internal ports (8080, 8081, 3000, 3001, 9090, etc.) firewalled from public access

### On the Developer/CI Machine
- `ssh` and `scp` for manual operations
- `docker` with GHCR login for manual image operations
- `gh` CLI for GitHub Actions interactions

---

## VPS Directory Structure

After first-time setup the server has the following layout:

```
/opt/dare/
  docker-compose.production.yml   # Production compose file (pulled from repo)
  docker-compose.staging.yml      # Staging compose file
  .env.production                 # Production secrets (root:dare 640, NOT in repo)
  .env.staging                    # Staging secrets (root:dare 640, NOT in repo)
  scripts/
    promote.sh                    # Promote an image tag to production
    rollback.sh                   # Revert to previous image tag
    backup.sh                     # Daily backup routine
    smoke.sh                      # Post-deploy smoke tests
  keycloak/
    production-realm.json         # Keycloak realm config
    staging-realm.json

/var/log/dare/
  deploys.log                     # Deploy history
  backup.log                      # Backup run log
  smoke.log                       # Smoke test log
  ops.log                         # Manual ops notes

/var/backups/dare/
  dare_backup_<timestamp>.tar.gz  # Daily local backup archives (7-day retention)
```

---

## First-Time Server Setup

### 1. Install Docker

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker dare
```

### 2. Create the dare system user

```bash
sudo useradd -r -m -d /opt/dare -s /bin/bash dare
sudo mkdir -p /opt/dare /var/log/dare /var/backups/dare
sudo chown -R dare:dare /opt/dare /var/log/dare /var/backups/dare
```

### 3. Copy compose files and scripts

```bash
# From your workstation
scp deploy/docker-compose.production.yml dare@your-vps:/opt/dare/
scp deploy/docker-compose.staging.yml dare@your-vps:/opt/dare/
scp -r deploy/scripts/ dare@your-vps:/opt/dare/scripts/
scp -r deploy/prometheus/ dare@your-vps:/opt/dare/prometheus/
scp -r deploy/grafana/ dare@your-vps:/opt/dare/grafana/
scp -r deploy/loki/ dare@your-vps:/opt/dare/loki/
chmod +x /opt/dare/scripts/*.sh
```

### 4. Create .env.production

```bash
sudo nano /opt/dare/.env.production
# Fill in all secrets per deploy/secrets/README.md
sudo chown root:dare /opt/dare/.env.production
sudo chmod 640 /opt/dare/.env.production
```

### 5. Install the systemd units

```bash
sudo cp deploy/systemd/dare.service /etc/systemd/system/
sudo cp deploy/systemd/dare-backup.service /etc/systemd/system/
sudo cp deploy/systemd/dare-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dare dare-backup.timer
```

### 6. Run database migrations

```bash
# Ensure the postgres container is up first
sudo systemctl start dare
# Then run goose migrations
docker exec dare-production-dare-api-1 /dare-api migrate up
```

### 7. Configure Cloudflare

- Point DNS for `api.dare.club`, `app.dare.club`, `auth.dare.club` to the VPS IP.
- Enable Cloudflare proxy (orange cloud).
- Create Page Rules or Transform Rules to proxy `/api/*` to port 8080 and `/` to port 3000.
- Configure SSL/TLS to "Full (strict)" mode.
- Add firewall rule to allow only Cloudflare IP ranges on ports 80/443.

---

## Staging vs Production Environments

| Aspect | Staging | Production |
|---|---|---|
| Compose file | `docker-compose.staging.yml` | `docker-compose.production.yml` |
| Env file | `.env.staging` | `.env.production` |
| Mailpit | Yes (catches all outbound email) | No (real Postmark) |
| Uptime Kuma | Yes (internal status page) | No (external status provider) |
| Image tag | `latest-staging` or `sha-<short>` | Promoted via `promote.sh` |
| PostgreSQL connections | max 100 | max 200 |
| Redis AOF | everysec | always (stronger durability) |
| Restart policy | `unless-stopped` | `always` |
| Resource limits | Reduced | Production-scale |

---

## How CI/CD Promotes Code

### Automatic staging deploys (on every push to `main`)

1. GitHub Actions (`.github/workflows/deploy-staging.yml`) builds and pushes
   `ghcr.io/luryann/dare-api:sha-<short>` and `dare-admin:sha-<short>`.
2. SSH into the staging VPS and run `docker compose pull + up -d`.
3. Wait up to 180s for `/api/v1/health/live` to return 200.
4. Run `/opt/dare/scripts/smoke.sh`.
5. Notify Slack on success or failure when `SLACK_WEBHOOK` is configured.

Images default to the repository owner namespace so `GITHUB_TOKEN` can publish
them. Both workflows accept the Actions variable `GHCR_NAMESPACE` to override
that namespace (use lowercase). For a different owner, also configure the
`GHCR_TOKEN` secret with package access to that owner; its account must match
the workflow actor used for registry login. Private images require a registry
login on each VPS with a token that can read those packages.

Before the first deploy with this change, sync the updated Compose files to
`/opt/dare` on both hosts. They use `${GHCR_NAMESPACE}` (required; compose fails if unset) for API, admin,
and worker images. Workflows pass the selected namespace over SSH; manual
deploys and rollbacks must export the same `GHCR_NAMESPACE`. To make manual `docker compose` and `scripts/rollback.sh` runs work, put `GHCR_NAMESPACE=<owner>` in `/opt/dare/.env` (compose reads it automatically).

### Manual production deploys

1. Identify the staging SHA that passed all smoke tests, e.g. `sha-abc1234`.
2. Go to GitHub Actions → "Deploy to Production" → "Run workflow".
3. Enter the `image_tag` (`sha-abc1234`) and type `DEPLOY` to confirm.
4. GitHub requires a manual approval from an environment reviewer (configured in
   GitHub Settings → Environments → production).
5. The workflow SSHes to the production VPS and runs `/opt/dare/scripts/promote.sh sha-abc1234`.
6. `promote.sh` backs up `.env.production`, updates `IMAGE_TAG`, pulls images,
   does a rolling restart, and verifies health. Auto-rollback fires on failure.

---

## Manual Deploy Steps

If CI is unavailable or you need to deploy manually:

```bash
ssh dare@prod-vps
cd /opt/dare

# Promote a specific tag
./scripts/promote.sh sha-abc1234

# Or pull latest-staging and restart everything
IMAGE_TAG=latest-staging docker compose -f docker-compose.production.yml pull
IMAGE_TAG=latest-staging docker compose -f docker-compose.production.yml up -d --remove-orphans
```

---

## Rollback

To roll back to the previous version immediately:

```bash
ssh dare@prod-vps
/opt/dare/scripts/rollback.sh
```

This restores `.env.production.prev`, pulls the old image, restarts app containers,
and verifies health. The operation takes under 2 minutes in normal circumstances.

For an emergency rollback from CI, re-run "Deploy to Production" with the last
known-good image tag.

---

## Monitoring Endpoints

| Service | URL | Notes |
|---|---|---|
| API liveness | `https://api.dare.club/api/v1/health/live` | Public |
| API readiness | `https://api.dare.club/api/v1/health/ready` | Public |
| Prometheus | `http://vps-ip:9090` | Firewalled, VPN/SSH tunnel required |
| Grafana | `https://grafana.dare.club` | Admin login required |
| Keycloak Admin | `https://auth.dare.club/admin` | Admin login required |
| MinIO Console | `https://minio.dare.club` | Admin login required |

---

## Backup Schedule

Backups run daily at 02:00 local time via the `dare-backup.timer` systemd unit.

Each backup includes:
- PostgreSQL full dump (gzip-compressed)
- Redis RDB snapshot
- Encrypted copy of `.env.production`

Local retention: 7 days. Remote retention (restic/S3): 30 daily, 12 weekly,
12 monthly snapshots.

To verify backup health:
```bash
sudo systemctl status dare-backup.timer
journalctl -u dare-backup.service -n 50
```

To trigger a manual backup:
```bash
sudo systemctl start dare-backup.service
```

---

## Service Management

```bash
# Start / stop / restart the entire platform
sudo systemctl start dare
sudo systemctl stop dare
sudo systemctl restart dare

# Rolling restart of app containers only (zero-downtime)
sudo systemctl reload dare

# View logs
journalctl -u dare -f
journalctl -u dare-backup -n 100

# Check container status
cd /opt/dare && docker compose -f docker-compose.production.yml ps
```

### Application health and internal API routing

API containers use `/dare-api healthcheck http://localhost:8080/api/v1/health/live`;
this works in the distroless image without a shell or wget. Admin containers
serve `/api/health`. Public API health paths include the `/api/v1` prefix.

The admin server proxies `/api/v1/*` using `API_INTERNAL_URL` at request time.
Compose sets it to `http://dare-api:8080`; set it explicitly for other hosting
layouts. It is a server-only setting and can change when promoting an image.
