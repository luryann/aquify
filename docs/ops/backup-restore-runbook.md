# DARE Platform — Backup and Restore Runbook

## Overview

This runbook covers the daily backup process, retention policy, offsite storage, and restore procedures for the DARE swim club platform. The backup system protects three data stores: the PostgreSQL relational database, the MinIO object storage (imported files, exported artifacts, document uploads), and the Keycloak identity realm.

---

## Backup Schedule

| Job | Schedule | Trigger |
|---|---|---|
| Full backup | Daily at **02:00 UTC** | `dare-backup.timer` systemd timer |
| Offsite sync (included) | Within each daily backup run | `rclone copy` step in `backup.sh` |
| Backup health check alert | Within each run | Prometheus textfile metric + Slack webhook |

The timer definition lives at `/etc/systemd/system/dare-backup.timer`. To check timer status:

```bash
systemctl status dare-backup.timer
systemctl status dare-backup.service
journalctl -u dare-backup.service -n 100
```

---

## Backup Contents

Each backup snapshot in `/var/backups/dare/YYYYMMDD-HHMMSS/` contains:

| File | Description |
|---|---|
| `postgres.dump` | pg_dump custom-format compressed dump (level 9). Use `pg_restore` to restore. |
| `minio/` | mc mirror of the production MinIO bucket. Subdirectory structure mirrors bucket. |
| `keycloak-realm-dare.json` | Keycloak partial-export: clients, groups, roles. Does not include user passwords (those are hashed in the Postgres DB). |

The snapshot directory is archived to `dare-backup-YYYYMMDD-HHMMSS.tar.gz`, GPG-encrypted to `dare-backup-YYYYMMDD-HHMMSS.tar.gz.gpg`, then uploaded offsite.

---

## Retention Policy

### Local Retention (VPS)

| Artifact | Retention |
|---|---|
| Raw snapshot directories | 7 days (pruned after archiving) |
| Compressed + encrypted archive files | 30 days |

### Offsite Retention (Backblaze B2 recommended)

| Tier | Schedule | Retention |
|---|---|---|
| Daily | Every day except Sunday + 1st of month | 30 copies |
| Weekly | Every Sunday | 12 copies |
| Monthly | Every 1st of month | 3 copies |

This gives a recovery window of up to 3 months for the monthly tier and approximately 90 days for daily restores.

---

## Offsite Storage

**Recommended provider:** Backblaze B2 — lowest cost per GB for infrequently-accessed backup data.

rclone configuration (`~/.config/rclone/rclone.conf` on VPS):

```ini
[b2]
type = b2
account = <B2_ACCOUNT_ID>
key = <B2_APPLICATION_KEY>
```

The rclone remote path in the backup script should be `b2:dare-backups-offsite`.

Alternative: Any S3-compatible provider works with `rclone` — AWS S3, Cloudflare R2, Wasabi.

---

## Recovery Objectives

| Metric | Target |
|---|---|
| RTO (Recovery Time Objective) | < 4 hours for complete platform restore |
| RPO (Recovery Point Objective) | < 24 hours (daily backups); < 1 hour with PostgreSQL WAL archiving (future) |

The 4-hour RTO assumes a VPS is already provisioned. Provisioning a new VPS from scratch adds approximately 30–60 minutes.

---

## Backup Integrity Verification

Run a monthly integrity check to confirm backups are valid and restorable before an actual incident requires them.

### Automated integrity check (monthly)

```bash
# Download latest backup from offsite to a temp location
rclone copy b2:dare-backups-offsite/daily/ /tmp/dare-backup-verify/ --max-age 24h

# List archives found
ls -lh /tmp/dare-backup-verify/

# Test GPG decryption (does not restore data)
ARCHIVE="$(ls /tmp/dare-backup-verify/*.gpg | tail -1)"
gpg --batch --decrypt --output /tmp/test-restore.tar.gz "${ARCHIVE}"

# Test tar extraction (does not restore data)
tar -tzf /tmp/test-restore.tar.gz | head -20

# Verify postgres.dump is a valid pg_dump file
tar -xzf /tmp/test-restore.tar.gz -C /tmp/ --wildcards '*/postgres.dump'
pg_restore --list /tmp/*/postgres.dump | head -20

echo "Backup integrity OK"
rm -rf /tmp/dare-backup-verify /tmp/test-restore.tar.gz /tmp/2*
```

The quarterly restore drill (see `restore-drill-checklist.md`) performs a full restore onto a clean staging VM, which is the strongest integrity guarantee.

---

## Monitoring

The backup script writes a Prometheus node_exporter textfile metric on success:

```
dare_backup_last_success_timestamp  <unix epoch>
```

Create a Prometheus alerting rule to fire if this metric is more than 26 hours old (allowing slack for timing variation):

```yaml
# In docs/observability/prometheus/alerts.yaml
- alert: DareBackupMissing
  expr: time() - dare_backup_last_success_timestamp > 93600
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "DARE backup has not succeeded in > 26 hours"
    runbook_url: "https://github.com/your-org/aquify/blob/main/docs/ops/backup-restore-runbook.md"
```

The backup script also sends a Slack alert to `#dare-alerts` on any failure — configure `ALERT_WEBHOOK` in `.env.backup`.

---

## Restore Procedures by Scenario

### Scenario 1: Accidental Data Deletion

A user or admin accidentally deleted a meet, household, or other record that needs recovery.

For small deletions, consider a **partial restore** (Scenario 4) rather than a full restore.

Steps:
1. Identify the backup timestamp just before the deletion: check audit log or estimate from the report time.
2. Run restore on a **staging clone** first to verify the data is present and extract the needed rows.
3. For targeted recovery, export the affected rows from the staging restore with `psql -c "COPY ..."`.
4. Import the extracted rows into production with a transaction-wrapped INSERT/UPDATE.
5. Document the recovery in the incident log.

**Avoid a full production restore** for single-record deletions unless the scope is very large.

### Scenario 2: Database Corruption

Symptoms: PostgreSQL log errors, `pg_dump` failing, data integrity check failures.

```bash
# 1. Identify a clean backup timestamp
ls -lt /var/backups/dare/ | head -10

# 2. Stop application services
docker compose -f /opt/dare/docker-compose.production.yml stop dare-api dare-worker

# 3. Run the restore script
/opt/dare/deploy/scripts/restore.sh <backup-timestamp>

# 4. Verify row counts post-restore (compare against pre-corruption metrics)
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM members.households;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM members.swimmers;"
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM billing.invoices;"
```

### Scenario 3: Complete Server Loss

The VPS is unreachable and cannot be recovered.

1. **Provision a new VPS** (same provider or alternative). Minimum spec: 4 vCPU, 8 GB RAM, 100 GB SSD.
2. **Install dependencies:** Docker, Docker Compose, nginx, certbot, mc, rclone, goose, gpg.
3. **Copy configuration:**
   ```bash
   # From 1Password, retrieve and install:
   # - .env.production
   # - .env.backup
   # - rclone config (~/.config/rclone/rclone.conf)
   # - GPG private key for backup decryption
   # - nginx site configs
   # - Cloudflare Origin Certificate + private key
   ```
4. **Clone repo and pull images:**
   ```bash
   git clone git@github.com:your-org/aquify.git /opt/dare
   cd /opt/dare
   docker compose -f docker-compose.production.yml pull
   ```
5. **Restore database and object storage:**
   ```bash
   # Download latest backup from offsite
   rclone copy b2:dare-backups-offsite/daily/ /var/backups/dare/ --max-age 26h
   # Run restore
   /opt/dare/deploy/scripts/restore.sh <latest-timestamp>
   ```
6. **Update DNS** in Cloudflare A records to point to the new VPS IP.
7. **Verify health:** `curl https://api.dare.club/healthz`
8. **Notify stakeholders** of recovery completion and expected data age.

DNS propagation through Cloudflare is typically < 1 minute.

### Scenario 4: Partial Restore (Single Table)

To recover data for a specific table without overwriting the entire database:

```bash
# 1. Extract the backup to a temporary working directory
TIMESTAMP=20260323-020015
tar -xzf /var/backups/dare/dare-backup-${TIMESTAMP}.tar.gz.gpg  # decrypt first if needed
# or if already decrypted:
tar -xzf /var/backups/dare/dare-backup-${TIMESTAMP}.tar.gz -C /tmp/

# 2. Restore ONLY the target table to a temporary database
createdb -h localhost dare_recovery_tmp
pg_restore \
  --no-owner \
  --table=<target_table> \
  --dbname=postgres://localhost/dare_recovery_tmp \
  /tmp/${TIMESTAMP}/postgres.dump

# 3. Connect to the recovery DB and extract needed rows
psql postgres://localhost/dare_recovery_tmp \
  -c "COPY (SELECT * FROM <target_table> WHERE ...) TO '/tmp/recovery_rows.csv' CSV HEADER;"

# 4. Import rows into production within a transaction
psql "$DATABASE_URL" -c "BEGIN;"
psql "$DATABASE_URL" -c "\\COPY <target_table> FROM '/tmp/recovery_rows.csv' CSV HEADER;"
# Review and COMMIT or ROLLBACK
psql "$DATABASE_URL" -c "COMMIT;"

# 5. Clean up
dropdb -h localhost dare_recovery_tmp
rm -rf /tmp/${TIMESTAMP} /tmp/recovery_rows.csv
```

---

## Backup Failure Response

If a backup failure Slack alert fires in `#dare-alerts`:

1. SSH to VPS and check the backup log: `tail -100 /var/log/dare/backup.log`
2. Identify the failing step (PostgreSQL, MinIO, Keycloak, upload, or prune).
3. Fix the root cause (e.g., disk full, DB unreachable, rclone credentials expired).
4. Run the backup script manually to verify the fix: `sudo /opt/dare/deploy/scripts/backup.sh`
5. Confirm the Prometheus metric updates: `cat /var/lib/node_exporter/dare_backup.prom`
6. If the issue persists for > 24 hours (two missed backup windows), escalate to engineering lead.

> **SLA reminder:** Two consecutive missed backups means the RPO guarantee is no longer met. Treat this as a P1 incident.
