# DARE Platform — Disaster Recovery Runbook

## Scope

This runbook covers disaster recovery for the DARE platform across scenarios ranging from single-component failure to complete datacenter loss. It is intended for use by on-call engineers and the engineering lead during active incidents.

For routine restore operations (accidental deletion, single-table recovery), refer to `backup-restore-runbook.md`.

---

## Recovery Objectives

| Metric | Target | Notes |
|---|---|---|
| RTO — single component failure | < 1 hour | DB, MinIO, or Keycloak restart |
| RTO — VPS failure (existing backups) | < 4 hours | Provision + restore |
| RTO — Complete datacenter loss | < 8 hours | New region provision + full restore |
| RPO | < 24 hours | Based on daily backups |
| RPO with WAL archiving (future) | < 1 hour | Continuous archiving, not yet implemented |

---

## DR Contact Tree and Escalation

| Role | Responsibility | Escalation timeout |
|---|---|---|
| On-call engineer | First responder; assess scope within 15 min | 15 min |
| Engineering lead | Authorize VPS reprovisioning or data rollback | After 15 min if on-call cannot contain |
| Product lead | Stakeholder notification decisions; external comms | After 30 min of active incident |
| Billing contact | Coordinate with Stripe if billing data is affected | On billing/payment system impact |

Contact information for each role is stored in the **DARE Emergency Contacts** item in the team 1Password vault.

---

## Communication Protocol

### During an Active Incident

1. **Immediately** post to Slack `#dare-status`:
   ```
   [DR ACTIVE] <brief description of failure>
   Scope: <what is affected>
   Status: Investigating | Identified | Recovering
   On-call: @<name>
   Started: <time UTC>
   ```

2. **Update status page** (Uptime Kuma at `status.dare.club`):
   - Create an incident with affected components.
   - Status: Investigating → Identified → Recovering → Resolved.

3. **Every 20 minutes** during active recovery: post an update to `#dare-status`.

4. **If outage > 30 minutes**: engineering lead or product lead drafts a stakeholder email.

5. **On resolution**:
   - Post `[DR RESOLVED]` message with duration and brief summary.
   - Update status page to Resolved.
   - Schedule post-mortem within 48 hours.

---

## Recovery Scenario 1: Database Failure (Data Corruption)

**Symptoms:** PostgreSQL error log showing corruption, pg_dump failing, integrity check failures, API returning 500 on DB queries.

**Decision:** Restore from backup. Do not attempt in-place repair unless instructed by a PostgreSQL DBA.

### Steps

```bash
# 1. SSH to VPS
ssh deploy@vps.dare.club

# 2. Confirm the failure
docker logs dare-production-postgres-1 --tail=50
psql "$DATABASE_URL" -c "SELECT 1;" || echo "DB unreachable"

# 3. Stop application services (not the DB yet)
docker compose -f /opt/dare/docker-compose.production.yml stop dare-api dare-worker

# 4. Identify the most recent clean backup
ls -lt /var/backups/dare/ | head -5

# 5. Run restore (includes stopping DB internally via pg_restore connection)
/opt/dare/deploy/scripts/restore.sh <YYYYMMDD-HHMMSS>

# 6. Verify row counts against expected values (from last known metrics)
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM members.households;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM members.swimmers;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM meets.meets;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM billing.invoices;"
```

### Validation

- API health check: `curl https://api.dare.club/healthz`
- Admin login works
- Meet list loads
- Invoice list loads

---

## Recovery Scenario 2: VPS Failure (Server Down)

**Symptoms:** All DARE services unreachable (API, admin portal, status page). VPS provider dashboard shows server in error state.

**Decision:** Provision a new VPS and restore from latest offsite backup.

### Steps

#### 2.1 — Attempt VPS Recovery First (< 15 min)

If the VPS provider offers a web console or rescue mode, attempt:
- Hard reboot via provider control panel.
- Mount rescue disk and check filesystem with `fsck`.
- If data is intact, restart services: `docker compose -f /opt/dare/docker-compose.production.yml up -d`

If the VPS cannot be recovered within 15 minutes, proceed to full reprovisioning.

#### 2.2 — Provision New VPS

1. Create a new VPS at the same provider or an alternative (Hetzner, DigitalOcean, Linode).
   - Minimum: 4 vCPU, 8 GB RAM, 100 GB SSD, Ubuntu 24.04 LTS.
2. Record the new public IP.

#### 2.3 — Install Dependencies

```bash
# Connect to new VPS
ssh root@<new-ip>

# Install Docker
curl -fsSL https://get.docker.com | bash
usermod -aG docker dare

# Install tools
apt-get install -y nginx certbot python3-certbot-nginx gnupg2 git curl jq

# Install mc (MinIO client)
wget -q https://dl.min.io/client/mc/release/linux-amd64/mc -O /usr/local/bin/mc
chmod +x /usr/local/bin/mc

# Install rclone
curl https://rclone.org/install.sh | bash

# Install goose
go install github.com/pressly/goose/v3/cmd/goose@latest
# or download binary: https://github.com/pressly/goose/releases
```

#### 2.4 — Copy Configuration from 1Password

From the team 1Password vault, retrieve and install:
- `.env.production` → `/opt/dare/.env.production`
- `.env.backup` → `/opt/dare/.env.backup`
- rclone config → `~/.config/rclone/rclone.conf`
- GPG private key for backup decryption → import with `gpg --import`
- nginx site configs → `/etc/nginx/sites-available/`
- Cloudflare Origin Certificate → `/etc/ssl/certs/dare-club-cloudflare-origin.pem`
- Cloudflare Origin Private Key → `/etc/ssl/private/dare-club-cloudflare-origin.key`

```bash
mkdir -p /opt/dare /var/backups/dare /var/log/dare
chmod 700 /opt/dare
```

#### 2.5 — Clone Repo and Pull Images

```bash
cd /opt/dare
git clone git@github.com:your-org/aquify.git .
docker compose -f docker-compose.production.yml pull
```

#### 2.6 — Restore from Backup

```bash
# Download the latest daily backup
mkdir -p /var/backups/dare
rclone copy b2:dare-backups-offsite/daily/ /var/backups/dare/ --max-age 26h

# Identify the timestamp
ls /var/backups/dare/

# Run restore
/opt/dare/deploy/scripts/restore.sh <latest-timestamp>
```

#### 2.7 — Update DNS

In the Cloudflare dashboard, update ALL A records from the old VPS IP to the new VPS IP:

```
dare.club           A → <new-ip>
www.dare.club       A → <new-ip>
admin.dare.club     A → <new-ip>
api.dare.club       A → <new-ip>
status.dare.club    A → <new-ip>
staging-admin.dare.club  A → <new-ip>
staging-api.dare.club    A → <new-ip>
```

DNS propagation via Cloudflare is typically < 1 minute.

#### 2.8 — Verify

```bash
curl https://api.dare.club/healthz
curl https://admin.dare.club
```

---

## Recovery Scenario 3: Object Storage Failure (MinIO Down)

**Symptoms:** Import uploads failing, export downloads 404, document uploads failing. API and DB are healthy.

**Steps:**

```bash
# 1. Check MinIO container status
docker compose -f /opt/dare/docker-compose.production.yml ps dare-minio
docker logs dare-production-minio-1 --tail=50

# 2. Attempt restart first
docker compose -f /opt/dare/docker-compose.production.yml restart dare-minio
sleep 10
curl http://localhost:9000/minio/health/live && echo "MinIO healthy"

# 3. If restart doesn't resolve (disk issue, corruption):
#    Stop workers to prevent failed job retries
docker compose -f /opt/dare/docker-compose.production.yml stop dare-worker

# 4. Find the latest MinIO backup
TIMESTAMP=$(ls -t /var/backups/dare/ | head -1)

# 5. Restore MinIO objects
mc alias set dare-local http://localhost:9000 "$MINIO_ACCESS_KEY" "$MINIO_SECRET_KEY"
mc mirror --overwrite --preserve \
  "/var/backups/dare/${TIMESTAMP}/minio" \
  "dare-local/${MINIO_BUCKET}"

# 6. Restart worker
docker compose -f /opt/dare/docker-compose.production.yml start dare-worker
```

---

## Recovery Scenario 4: Keycloak Failure (Auth Down)

**Symptoms:** All login attempts failing, OIDC callback errors, `/api/v1/auth/session` returning 401 or 502.

**Steps:**

```bash
# 1. Check Keycloak container
docker compose -f /opt/dare/docker-compose.production.yml ps dare-keycloak
docker logs dare-production-keycloak-1 --tail=100

# 2. Attempt restart
docker compose -f /opt/dare/docker-compose.production.yml restart dare-keycloak
sleep 30
curl http://localhost:8443/realms/dare/.well-known/openid-configuration | jq .issuer

# 3. If Keycloak DB is corrupted (Keycloak uses its own embedded H2 or shared Postgres):
#    Stop Keycloak
docker compose -f /opt/dare/docker-compose.production.yml stop dare-keycloak

#    Find realm export from latest backup
TIMESTAMP=$(ls -t /var/backups/dare/ | head -1)
ls "/var/backups/dare/${TIMESTAMP}/keycloak-realm-dare.json"

#    Start Keycloak with realm import
# Option A — If Keycloak uses embedded H2: delete H2 data directory and re-import
#    docker volume rm dare-production_keycloak-data
#    docker compose up -d dare-keycloak
#    (Keycloak will re-initialize from scratch)
#    Then import realm via admin UI or CLI:

docker exec -it dare-production-keycloak-1 \
  /opt/keycloak/bin/kc.sh import \
  --file /tmp/keycloak-realm-dare.json \
  --override true

# 4. Verify OIDC discovery works
curl https://auth.dare.club/realms/dare/.well-known/openid-configuration | jq .issuer

# 5. Verify admin login works end-to-end
```

> **Note:** The realm export contains clients, groups, and roles, but NOT user credentials (passwords). Users who had accounts in Keycloak will need to go through "Forgot Password" flow after a full Keycloak restore from realm JSON. The user records themselves (names, emails) are stored in PostgreSQL and are not affected by the Keycloak restore.

---

## Recovery Scenario 5: Complete Datacenter Loss

**Symptoms:** VPS provider reporting full datacenter outage; all services unreachable.

This is the most severe scenario. Follow Scenario 2 (VPS Failure) with the additional step of selecting a **different datacenter region or provider**.

### Additional Considerations

- Check if the VPS provider has a data export feature — if the disk image is recoverable, use it instead of restoring from backup.
- If using a different provider, ensure the new VPS has comparable or better network egress to Cloudflare's anycast network.
- Once the new VPS is provisioned and DNS updated, update the nginx Cloudflare IP restriction list if the new provider uses different Cloudflare peering.
- After recovery, evaluate migrating to a multi-region setup or enabling VPS provider failover to prevent recurrence.

---

## DR Test Cadence

| Activity | Frequency | Owner |
|---|---|---|
| Backup integrity check (decrypt + list) | Monthly | On-call engineer rotation |
| Staging restore drill (full restore + smoke test) | Quarterly | Engineering lead |
| DR runbook review | Semi-annually | Engineering lead |
| Full datacenter failover drill | Annually (optional) | Engineering lead + product lead |

The staging restore drill procedure is documented in `restore-drill-checklist.md`.

---

## Recovery Validation Checklist

After any recovery scenario, verify the following before declaring the incident resolved:

- [ ] `GET https://api.dare.club/healthz` returns HTTP 200
- [ ] `GET https://admin.dare.club` loads the login page (HTTP 200 or 302)
- [ ] Admin login via OIDC completes without error
- [ ] Dashboard loads with member and meet counts
- [ ] `GET /api/v1/orgs/{orgId}/members` returns non-empty list
- [ ] `GET /api/v1/orgs/{orgId}/meets` returns non-empty list
- [ ] A test import upload completes without error (or is queued)
- [ ] Invoice list loads without error
- [ ] `POST /api/v1/auth/sessions` (login) succeeds with test credentials
- [ ] Outbound email test (send a test announcement or trigger a test campaign)
- [ ] Prometheus metrics endpoint returns data (`/metrics`)
- [ ] Uptime Kuma status page shows all monitors green
- [ ] Background job queue depth is reasonable (check admin Jobs page)

Mark all items checked before updating the status page to Resolved.
