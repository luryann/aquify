#!/usr/bin/env bash
# DARE Platform Restore Script
#
# Restores DB and MinIO objects from a backup snapshot.
#
# Usage:
#   restore.sh <backup-timestamp>
#
# Examples:
#   restore.sh 20260323-020015
#   restore.sh 20260323-020015 --dry-run
#
# The script will:
#   1. Locate the backup archive locally or download from offsite (rclone)
#   2. GPG-decrypt the archive if encrypted
#   3. Stop dare-api and dare-worker containers
#   4. Restore PostgreSQL from the custom-format dump
#   5. Restore MinIO objects via mc mirror
#   6. Restart services
#   7. Run health check
#   8. Log restore event to /var/log/dare/restores.log
#
# Required environment (sourced from /opt/dare/.env.production or .env.backup):
#   DATABASE_URL         PostgreSQL connection DSN
#   MINIO_ENDPOINT       MinIO server URL
#   MINIO_ACCESS_KEY     MinIO access key
#   MINIO_SECRET_KEY     MinIO secret key
#   MINIO_BUCKET         Production bucket name to restore into
#   RCLONE_REMOTE        rclone remote:path where offsite backups live
#   GPG_RECIPIENT        GPG key fingerprint (for decrypt — requires matching private key in keyring)
#
# Exit codes:
#   0  Restore completed successfully
#   1  Fatal error — manual intervention required

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DEPLOY_DIR="/opt/dare"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/dare}"
RESTORE_LOG="/var/log/dare/restores.log"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/health/live}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-120}"
HEALTH_RETRY_INTERVAL=5
COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.production.yml"
APP_SERVICES="dare-api dare-worker"

# Load production env
# shellcheck source=/dev/null
[ -f "${DEPLOY_DIR}/.env.production" ] && source "${DEPLOY_DIR}/.env.production"
# shellcheck source=/dev/null
[ -f "${DEPLOY_DIR}/.env.backup" ] && source "${DEPLOY_DIR}/.env.backup"

DATABASE_URL="${DATABASE_URL:-}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://localhost:9000}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-}"
MINIO_BUCKET="${MINIO_BUCKET:-dare-production}"
RCLONE_REMOTE="${RCLONE_REMOTE:-}"

DRY_RUN=false

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

usage() {
  echo "Usage: $0 <backup-timestamp> [--dry-run]"
  echo "  backup-timestamp  Format: YYYYMMDD-HHMMSS (e.g. 20260323-020015)"
  echo "  --dry-run         Show what would be done without making changes"
  exit 1
}

BACKUP_TIMESTAMP="${1:-}"
if [[ -z "${BACKUP_TIMESTAMP}" ]]; then
  usage
fi
shift

for arg in "$@"; do
  case "${arg}" in
    --dry-run) DRY_RUN=true ;;
    *) echo "Unknown argument: ${arg}"; usage ;;
  esac
done

# ---------------------------------------------------------------------------
# Globals
# ---------------------------------------------------------------------------

SNAPSHOT_DIR="${BACKUP_DIR}/${BACKUP_TIMESTAMP}"
RESTORE_START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OPERATOR="$(whoami)"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() {
  local level="$1"
  shift
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '%s [%-5s] %s\n' "${ts}" "${level}" "$*" | tee -a "${RESTORE_LOG}"
}

die() {
  log "ERROR" "$*"
  log "ERROR" "=== RESTORE ABORTED at step: ${CURRENT_STEP:-unknown} ==="
  exit 1
}

CURRENT_STEP="init"

wait_for_health() {
  local retries=$(( HEALTH_TIMEOUT / HEALTH_RETRY_INTERVAL ))
  log "INFO" "Waiting for health check at ${HEALTH_URL} (timeout: ${HEALTH_TIMEOUT}s)..."
  for i in $(seq 1 "${retries}"); do
    local code
    code="$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "${HEALTH_URL}" 2>/dev/null || echo "000")"
    if [[ "${code}" == "200" ]]; then
      log "INFO" "Health check passed (attempt ${i}/${retries})"
      return 0
    fi
    log "INFO" "Attempt ${i}/${retries}: HTTP ${code} — retrying in ${HEALTH_RETRY_INTERVAL}s..."
    sleep "${HEALTH_RETRY_INTERVAL}"
  done
  return 1
}

confirm() {
  local prompt="$1"
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Skipping confirmation: ${prompt}"
    return 0
  fi
  read -rp "${prompt} [yes/no]: " answer
  [[ "${answer}" == "yes" ]] || die "Restore aborted by operator"
}

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

mkdir -p "$(dirname "${RESTORE_LOG}")"
touch "${RESTORE_LOG}"
log "INFO" "=== DARE Restore started: timestamp=${BACKUP_TIMESTAMP} operator=${OPERATOR} dry_run=${DRY_RUN} ==="

# ---------------------------------------------------------------------------
# Step 1: Locate backup archive
# ---------------------------------------------------------------------------

CURRENT_STEP="locate-archive"
log "INFO" "--- Step 1: Locating backup archive ---"

ARCHIVE_FILE=""
# Check for GPG-encrypted archive first, then plain tar.gz
for candidate in \
  "${BACKUP_DIR}/dare-backup-${BACKUP_TIMESTAMP}.tar.gz.gpg" \
  "${BACKUP_DIR}/dare-backup-${BACKUP_TIMESTAMP}.tar.gz"; do
  if [[ -f "${candidate}" ]]; then
    ARCHIVE_FILE="${candidate}"
    log "INFO" "Found local archive: ${ARCHIVE_FILE}"
    break
  fi
done

# Also check if snapshot dir already exists (already extracted)
if [[ -z "${ARCHIVE_FILE}" && -d "${SNAPSHOT_DIR}" ]]; then
  log "INFO" "Snapshot directory already present at ${SNAPSHOT_DIR} — skipping download"
fi

if [[ -z "${ARCHIVE_FILE}" && ! -d "${SNAPSHOT_DIR}" ]]; then
  log "INFO" "Archive not found locally — attempting offsite download via rclone..."

  if ! command -v rclone &>/dev/null; then
    die "rclone not found and no local archive exists for ${BACKUP_TIMESTAMP}"
  fi
  if [[ -z "${RCLONE_REMOTE}" ]]; then
    die "RCLONE_REMOTE not set and no local archive exists for ${BACKUP_TIMESTAMP}"
  fi

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Would attempt rclone copy from ${RCLONE_REMOTE}/*/ for ${BACKUP_TIMESTAMP}"
  else
    # Try all tiers (daily/weekly/monthly)
    local found_offsite=false
    for tier in daily weekly monthly; do
      local remote_path="${RCLONE_REMOTE}/${tier}/dare-backup-${BACKUP_TIMESTAMP}.tar.gz.gpg"
      log "INFO" "Trying: rclone copy '${remote_path}' ${BACKUP_DIR}/"
      if rclone copy "${remote_path}" "${BACKUP_DIR}/" 2>> "${RESTORE_LOG}"; then
        ARCHIVE_FILE="${BACKUP_DIR}/dare-backup-${BACKUP_TIMESTAMP}.tar.gz.gpg"
        [[ -f "${ARCHIVE_FILE}" ]] && { found_offsite=true; break; }
      fi
    done

    if [[ "${found_offsite}" != "true" ]]; then
      die "Could not locate archive for timestamp ${BACKUP_TIMESTAMP} locally or offsite"
    fi
    log "INFO" "Downloaded from offsite: ${ARCHIVE_FILE}"
  fi
fi

# ---------------------------------------------------------------------------
# Step 2: Decrypt archive
# ---------------------------------------------------------------------------

CURRENT_STEP="decrypt"
log "INFO" "--- Step 2: Decrypting archive ---"

if [[ -n "${ARCHIVE_FILE}" ]] && [[ "${ARCHIVE_FILE}" == *.gpg ]]; then
  DECRYPTED_ARCHIVE="${ARCHIVE_FILE%.gpg}"

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Would decrypt ${ARCHIVE_FILE} → ${DECRYPTED_ARCHIVE}"
  else
    log "INFO" "Decrypting ${ARCHIVE_FILE}..."
    if gpg --batch --yes --decrypt --output "${DECRYPTED_ARCHIVE}" "${ARCHIVE_FILE}" 2>> "${RESTORE_LOG}"; then
      log "INFO" "Decryption complete: ${DECRYPTED_ARCHIVE}"
      ARCHIVE_FILE="${DECRYPTED_ARCHIVE}"
    else
      die "GPG decryption failed — verify private key is in keyring for the backup recipient"
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Step 3: Extract archive to snapshot directory
# ---------------------------------------------------------------------------

CURRENT_STEP="extract"
log "INFO" "--- Step 3: Extracting archive ---"

if [[ -n "${ARCHIVE_FILE}" ]] && [[ "${ARCHIVE_FILE}" == *.tar.gz ]]; then
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Would extract ${ARCHIVE_FILE} → ${BACKUP_DIR}/"
  else
    if [[ ! -d "${SNAPSHOT_DIR}" ]]; then
      log "INFO" "Extracting ${ARCHIVE_FILE}..."
      tar -xzf "${ARCHIVE_FILE}" -C "${BACKUP_DIR}" 2>> "${RESTORE_LOG}" \
        || die "tar extraction failed"
      log "INFO" "Extracted to ${SNAPSHOT_DIR}"
    else
      log "INFO" "Snapshot dir already exists — skipping extraction"
    fi
  fi
fi

# Validate snapshot directory contents
if [[ "${DRY_RUN}" != "true" ]]; then
  if [[ ! -d "${SNAPSHOT_DIR}" ]]; then
    die "Snapshot directory not found after extraction: ${SNAPSHOT_DIR}"
  fi
  log "INFO" "Snapshot contents:"
  ls -lh "${SNAPSHOT_DIR}" | tee -a "${RESTORE_LOG}"
fi

# ---------------------------------------------------------------------------
# Confirmation gate
# ---------------------------------------------------------------------------

echo ""
echo "========================================================================"
echo "  DARE RESTORE CONFIRMATION"
echo "  Backup timestamp : ${BACKUP_TIMESTAMP}"
echo "  Target database  : ${DATABASE_URL}"
echo "  Target MinIO     : ${MINIO_ENDPOINT}/${MINIO_BUCKET}"
echo "  Dry run          : ${DRY_RUN}"
echo ""
echo "  WARNING: This will OVERWRITE the current database and object storage."
echo "  Ensure you have verified the backup and notified the team."
echo "========================================================================"
echo ""
confirm "Are you sure you want to restore from ${BACKUP_TIMESTAMP}?"

# ---------------------------------------------------------------------------
# Step 4: Stop application services
# ---------------------------------------------------------------------------

CURRENT_STEP="stop-services"
log "INFO" "--- Step 4: Stopping application services ---"

if [[ "${DRY_RUN}" == "true" ]]; then
  log "INFO" "[dry-run] Would stop services: ${APP_SERVICES}"
else
  log "INFO" "Stopping: ${APP_SERVICES}..."
  # shellcheck disable=SC2086
  docker compose -f "${COMPOSE_FILE}" stop ${APP_SERVICES} \
    >> "${RESTORE_LOG}" 2>&1 \
    || die "Failed to stop application services"
  log "INFO" "Services stopped"
fi

# ---------------------------------------------------------------------------
# Step 5: Restore PostgreSQL
# ---------------------------------------------------------------------------

CURRENT_STEP="restore-postgres"
log "INFO" "--- Step 5: Restoring PostgreSQL ---"

PG_DUMP="${SNAPSHOT_DIR}/postgres.dump"

if [[ ! -f "${PG_DUMP}" ]] && [[ "${DRY_RUN}" != "true" ]]; then
  die "PostgreSQL dump not found: ${PG_DUMP}"
fi

if [[ "${DRY_RUN}" == "true" ]]; then
  log "INFO" "[dry-run] Would run pg_restore --clean --if-exists from ${PG_DUMP}"
else
  log "INFO" "Restoring from ${PG_DUMP}..."
  if pg_restore \
      --clean \
      --if-exists \
      --no-password \
      --verbose \
      --dbname "${DATABASE_URL}" \
      "${PG_DUMP}" 2>> "${RESTORE_LOG}"; then
    log "INFO" "PostgreSQL restore complete"
  else
    # pg_restore exits non-zero for ignorable warnings — check for actual errors
    if grep -q "ERROR:" "${RESTORE_LOG}" 2>/dev/null; then
      die "pg_restore reported errors — see ${RESTORE_LOG}"
    else
      log "WARN" "pg_restore exited non-zero but no ERROR lines found — likely warnings only"
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Step 6: Run pending migrations
# ---------------------------------------------------------------------------

CURRENT_STEP="run-migrations"
log "INFO" "--- Step 6: Running pending database migrations ---"

if command -v goose &>/dev/null; then
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Would run: goose up (database migrations)"
  else
    log "INFO" "Running goose up..."
    DATABASE_URL="${DATABASE_URL}" goose \
      -dir "${DEPLOY_DIR}/db/migrations" \
      postgres "${DATABASE_URL}" \
      up >> "${RESTORE_LOG}" 2>&1 \
      || die "goose up failed — database may be at wrong schema version"
    log "INFO" "Migrations applied"
  fi
else
  log "WARN" "'goose' not found in PATH — skipping migration step. Run 'make db-up' manually after restore."
fi

# ---------------------------------------------------------------------------
# Step 7: Restore MinIO
# ---------------------------------------------------------------------------

CURRENT_STEP="restore-minio"
log "INFO" "--- Step 7: Restoring MinIO object storage ---"

MINIO_BACKUP_DIR="${SNAPSHOT_DIR}/minio"

if [[ ! -d "${MINIO_BACKUP_DIR}" ]] && [[ "${DRY_RUN}" != "true" ]]; then
  log "WARN" "MinIO backup directory not found at ${MINIO_BACKUP_DIR} — skipping MinIO restore"
else
  if ! command -v mc &>/dev/null; then
    log "WARN" "'mc' not found — skipping MinIO restore"
  elif [[ -z "${MINIO_ACCESS_KEY}" || -z "${MINIO_SECRET_KEY}" ]]; then
    log "WARN" "MINIO_ACCESS_KEY / MINIO_SECRET_KEY not set — skipping MinIO restore"
  else
    if [[ "${DRY_RUN}" == "true" ]]; then
      log "INFO" "[dry-run] mc mirror ${MINIO_BACKUP_DIR} → dare-restore/${MINIO_BUCKET}"
    else
      mc alias set dare-restore "${MINIO_ENDPOINT}" "${MINIO_ACCESS_KEY}" "${MINIO_SECRET_KEY}" \
        --api S3v4 2>> "${RESTORE_LOG}" \
        || die "mc alias set failed"

      log "INFO" "Mirroring ${MINIO_BACKUP_DIR} → dare-restore/${MINIO_BUCKET}..."
      if mc mirror --overwrite --preserve "${MINIO_BACKUP_DIR}" "dare-restore/${MINIO_BUCKET}" \
          >> "${RESTORE_LOG}" 2>&1; then
        log "INFO" "MinIO restore complete"
      else
        die "mc mirror failed during restore — see ${RESTORE_LOG}"
      fi
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Step 8: Restart services
# ---------------------------------------------------------------------------

CURRENT_STEP="restart-services"
log "INFO" "--- Step 8: Restarting application services ---"

if [[ "${DRY_RUN}" == "true" ]]; then
  log "INFO" "[dry-run] Would start: ${APP_SERVICES}"
else
  log "INFO" "Starting: ${APP_SERVICES}..."
  # shellcheck disable=SC2086
  docker compose -f "${COMPOSE_FILE}" start ${APP_SERVICES} \
    >> "${RESTORE_LOG}" 2>&1 \
    || die "Failed to start application services after restore"
  log "INFO" "Services started"
fi

# ---------------------------------------------------------------------------
# Step 9: Health check
# ---------------------------------------------------------------------------

CURRENT_STEP="health-check"
log "INFO" "--- Step 9: Health check ---"

if [[ "${DRY_RUN}" == "true" ]]; then
  log "INFO" "[dry-run] Would poll ${HEALTH_URL} for up to ${HEALTH_TIMEOUT}s"
else
  if wait_for_health; then
    log "INFO" "Platform is healthy after restore"
  else
    die "Health check failed after restore — services may need manual intervention"
  fi
fi

# ---------------------------------------------------------------------------
# Step 10: Log restore event
# ---------------------------------------------------------------------------

CURRENT_STEP="log-restore"
log "INFO" "--- Step 10: Logging restore event ---"

RESTORE_END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
RESTORE_RECORD="{\"started\":\"${RESTORE_START}\",\"completed\":\"${RESTORE_END}\",\"backup_timestamp\":\"${BACKUP_TIMESTAMP}\",\"operator\":\"${OPERATOR}\",\"dry_run\":${DRY_RUN}}"

echo "${RESTORE_RECORD}" >> "${RESTORE_LOG}"
log "INFO" "Restore event recorded to ${RESTORE_LOG}"

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo ""
echo "========================================================================"
echo "  DARE RESTORE COMPLETE"
echo "  Backup timestamp : ${BACKUP_TIMESTAMP}"
echo "  Completed at     : ${RESTORE_END}"
echo "  Log              : ${RESTORE_LOG}"
echo "========================================================================"

log "INFO" "=== DARE Restore completed successfully: ${BACKUP_TIMESTAMP} ==="
exit 0
