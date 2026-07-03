#!/usr/bin/env bash
# DARE Platform Backup Script
#
# Runs daily via systemd timer (dare-backup.service / dare-backup.timer).
# Backs up:
#   1. PostgreSQL database (pg_dump --format=custom --compress=9)
#   2. MinIO object storage (mc mirror to backup bucket)
#   3. Keycloak realm export (partial-export via Admin REST API)
#
# Creates a timestamped directory: /var/backups/dare/YYYYMMDD-HHMMSS/
# Archives and GPG-encrypts the snapshot, then uploads offsite via rclone.
#
# Retention:
#   Local:   7 days (raw snapshots pruned; archives kept 30 days)
#   Offsite: 30 daily, 12 weekly (Sunday), 3 monthly (1st of month)
#
# Failure alerting: sends Slack message to ALERT_WEBHOOK on any step failure.
# Prometheus: writes dare_backup_last_success_timestamp to node_exporter textfile.
#
# Usage:
#   backup.sh [--dry-run]
#
# Required environment (sourced from /opt/dare/.env.production or .env.backup):
#   DATABASE_URL                  PostgreSQL connection DSN
#   MINIO_ENDPOINT                MinIO server URL (e.g. http://localhost:9000)
#   MINIO_ACCESS_KEY              MinIO access key
#   MINIO_SECRET_KEY              MinIO secret key
#   MINIO_BUCKET                  Source bucket name
#   BACKUP_BUCKET                 Destination bucket for object mirror backup
#   KEYCLOAK_URL                  Keycloak server URL
#   KEYCLOAK_REALM                Realm name to export
#   KEYCLOAK_ADMIN_USER           Keycloak admin username
#   KEYCLOAK_ADMIN_PASS           Keycloak admin password
#   RCLONE_REMOTE                 rclone remote:path (e.g. b2:dare-backups-offsite)
#   GPG_RECIPIENT                 GPG key fingerprint or email for encryption
#   ALERT_WEBHOOK                 Slack incoming webhook URL for failure alerts
#
# Optional:
#   BACKUP_DIR                    Base local backup path (default: /var/backups/dare)
#   BACKUP_RETENTION_DAYS         Days to keep local archives (default: 30)
#   NODE_EXPORTER_TEXTFILE_DIR    Path to node_exporter textfile dir (default: /var/lib/node_exporter)

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DEPLOY_DIR="/opt/dare"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/dare}"
BACKUP_LOG="/var/log/dare/backup.log"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
NODE_EXPORTER_TEXTFILE_DIR="${NODE_EXPORTER_TEXTFILE_DIR:-/var/lib/node_exporter}"

# Load production env if not already set
# shellcheck source=/dev/null
[ -f "${DEPLOY_DIR}/.env.production" ] && source "${DEPLOY_DIR}/.env.production"
# Optionally load a dedicated backup credentials file (overrides above)
# shellcheck source=/dev/null
[ -f "${DEPLOY_DIR}/.env.backup" ] && source "${DEPLOY_DIR}/.env.backup"

DATABASE_URL="${DATABASE_URL:-}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://localhost:9000}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-}"
MINIO_BUCKET="${MINIO_BUCKET:-dare-production}"
BACKUP_BUCKET="${BACKUP_BUCKET:-dare-backup-objects}"
KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:8443}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-dare}"
KEYCLOAK_ADMIN_USER="${KEYCLOAK_ADMIN_USER:-admin}"
KEYCLOAK_ADMIN_PASS="${KEYCLOAK_ADMIN_PASS:-}"
RCLONE_REMOTE="${RCLONE_REMOTE:-}"
GPG_RECIPIENT="${GPG_RECIPIENT:-}"
ALERT_WEBHOOK="${ALERT_WEBHOOK:-}"

DRY_RUN=false
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=true

# ---------------------------------------------------------------------------
# Globals set at runtime
# ---------------------------------------------------------------------------

TIMESTAMP="$(date -u +%Y%m%d-%H%M%S)"
SNAPSHOT_DIR="${BACKUP_DIR}/${TIMESTAMP}"
ERRORS=()

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() {
  local level="$1"
  shift
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '%s [%-5s] %s\n' "${ts}" "${level}" "$*" | tee -a "${BACKUP_LOG}"
}

die() {
  log "ERROR" "$*"
  ERRORS+=("$*")
}

send_alert() {
  local msg="$1"
  if [[ -z "${ALERT_WEBHOOK}" ]]; then
    log "WARN" "ALERT_WEBHOOK not configured — Slack alert suppressed"
    return
  fi
  curl -s -X POST "${ALERT_WEBHOOK}" \
    -H "Content-Type: application/json" \
    --data "{\"text\":\":rotating_light: *DARE Backup FAILED* [${TIMESTAMP}]\n${msg}\nCheck ${BACKUP_LOG}\"}" \
    --max-time 10 \
    2>/dev/null || log "WARN" "Failed to send Slack alert (curl error)"
}

write_prometheus_metric() {
  local metric_file="${NODE_EXPORTER_TEXTFILE_DIR}/dare_backup.prom"
  if [[ -d "${NODE_EXPORTER_TEXTFILE_DIR}" ]]; then
    {
      echo "# HELP dare_backup_last_success_timestamp Unix timestamp of last successful DARE backup"
      echo "# TYPE dare_backup_last_success_timestamp gauge"
      echo "dare_backup_last_success_timestamp $(date +%s)"
    } > "${metric_file}.tmp" && mv "${metric_file}.tmp" "${metric_file}"
    log "INFO" "Prometheus metric written: ${metric_file}"
  fi
}

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

setup() {
  mkdir -p "${SNAPSHOT_DIR}" "$(dirname "${BACKUP_LOG}")"
  touch "${BACKUP_LOG}"
  log "INFO" "=== DARE Backup started: ${TIMESTAMP} ==="
  [[ "${DRY_RUN}" == "true" ]] && log "INFO" "DRY RUN MODE — no data will be written or uploaded"
}

# ---------------------------------------------------------------------------
# Step 1: PostgreSQL backup
# ---------------------------------------------------------------------------

backup_postgres() {
  log "INFO" "--- Step 1: PostgreSQL backup ---"

  if [[ -z "${DATABASE_URL}" ]]; then
    die "DATABASE_URL is not set — skipping PostgreSQL backup"
    return
  fi

  local dump_file="${SNAPSHOT_DIR}/postgres.dump"

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] pg_dump --format=custom --compress=9 → ${dump_file}"
    return
  fi

  log "INFO" "Running pg_dump (custom format, compression level 9)..."
  # Pass the DSN via PGDATABASE (libpq accepts a full connection URI there)
  # instead of as a command-line argument, so the embedded password never
  # appears in `ps` output visible to other local users.
  if PGDATABASE="${DATABASE_URL}" pg_dump \
      --format=custom \
      --compress=9 \
      --no-password \
      > "${dump_file}" 2>> "${BACKUP_LOG}"; then
    local sz
    sz="$(du -sh "${dump_file}" | cut -f1)"
    log "INFO" "pg_dump complete: ${dump_file} (${sz})"
  else
    die "pg_dump failed — see ${BACKUP_LOG}"
    rm -f "${dump_file}"
  fi
}

# ---------------------------------------------------------------------------
# Step 2: MinIO object storage backup
# ---------------------------------------------------------------------------

backup_minio() {
  log "INFO" "--- Step 2: MinIO object storage backup ---"

  if ! command -v mc &>/dev/null; then
    die "'mc' (MinIO client) not found in PATH — skipping MinIO backup"
    return
  fi

  if [[ -z "${MINIO_ACCESS_KEY}" || -z "${MINIO_SECRET_KEY}" ]]; then
    die "MINIO_ACCESS_KEY / MINIO_SECRET_KEY not set — skipping MinIO backup"
    return
  fi

  # Register a temporary mc alias scoped to this run
  mc alias set dare-src "${MINIO_ENDPOINT}" "${MINIO_ACCESS_KEY}" "${MINIO_SECRET_KEY}" \
    --api S3v4 2>> "${BACKUP_LOG}" \
    || { die "mc alias set failed"; return; }

  local minio_dir="${SNAPSHOT_DIR}/minio"
  mkdir -p "${minio_dir}"

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] mc mirror dare-src/${MINIO_BUCKET} → ${minio_dir}"
    return
  fi

  log "INFO" "Mirroring dare-src/${MINIO_BUCKET} → ${minio_dir}..."
  if mc mirror --preserve "dare-src/${MINIO_BUCKET}" "${minio_dir}" >> "${BACKUP_LOG}" 2>&1; then
    local cnt
    cnt="$(find "${minio_dir}" -type f | wc -l | tr -d ' ')"
    log "INFO" "MinIO mirror complete: ${cnt} objects"
  else
    die "mc mirror failed — see ${BACKUP_LOG}"
  fi
}

# ---------------------------------------------------------------------------
# Step 3: Keycloak realm export
# ---------------------------------------------------------------------------

backup_keycloak() {
  log "INFO" "--- Step 3: Keycloak realm export ---"

  if [[ -z "${KEYCLOAK_ADMIN_PASS}" ]]; then
    log "WARN" "KEYCLOAK_ADMIN_PASS not set — skipping Keycloak export"
    return
  fi

  local realm_file="${SNAPSHOT_DIR}/keycloak-realm-${KEYCLOAK_REALM}.json"

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Keycloak partial-export for realm ${KEYCLOAK_REALM} → ${realm_file}"
    return
  fi

  # Obtain admin token from master realm. Credentials are passed via stdin
  # rather than -d arguments so they never appear in `ps` output.
  log "INFO" "Requesting Keycloak admin token..."
  local token_json
  token_json="$(curl -sf -X POST \
    "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-binary @- \
    --max-time 30 2>> "${BACKUP_LOG}" <<EOF
client_id=admin-cli&username=${KEYCLOAK_ADMIN_USER}&password=${KEYCLOAK_ADMIN_PASS}&grant_type=password
EOF
)" \
    || { die "Keycloak token request failed (is Keycloak up?)"; return; }

  local access_token
  access_token="$(printf '%s' "${token_json}" | \
    python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null)" \
    || access_token=""

  if [[ -z "${access_token}" ]]; then
    die "Failed to parse Keycloak access token — check credentials"
    return
  fi

  log "INFO" "Exporting realm '${KEYCLOAK_REALM}' (clients + groups/roles)..."
  local http_code
  http_code="$(curl -sf -w "%{http_code}" -o "${realm_file}" \
    "${KEYCLOAK_URL}/admin/realms/${KEYCLOAK_REALM}/partial-export?exportClients=true&exportGroupsAndRoles=true" \
    -H "Authorization: Bearer ${access_token}" \
    --max-time 60 2>> "${BACKUP_LOG}")" \
    || http_code="error"

  if [[ "${http_code}" == "200" ]]; then
    local sz
    sz="$(du -sh "${realm_file}" | cut -f1)"
    log "INFO" "Keycloak export complete: ${realm_file} (${sz})"
  else
    die "Keycloak export HTTP status: ${http_code}"
    rm -f "${realm_file}"
  fi
}

# ---------------------------------------------------------------------------
# Step 4: Create archive and GPG-encrypt
# Returns the path to the final archive file (stdout)
# ---------------------------------------------------------------------------

create_archive() {
  log "INFO" "--- Step 4: Creating backup archive ---"

  local archive_tar="${BACKUP_DIR}/dare-backup-${TIMESTAMP}.tar.gz"
  local archive_out

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Would create ${archive_tar}$( [[ -n "${GPG_RECIPIENT}" ]] && echo '.gpg' )"
    echo ""
    return
  fi

  log "INFO" "Creating tar.gz: ${archive_tar}..."
  tar -czf "${archive_tar}" -C "${BACKUP_DIR}" "${TIMESTAMP}" 2>> "${BACKUP_LOG}" \
    || { die "tar failed"; echo ""; return; }

  local sz
  sz="$(du -sh "${archive_tar}" | cut -f1)"
  log "INFO" "Archive: ${archive_tar} (${sz})"

  if [[ -n "${GPG_RECIPIENT}" ]]; then
    archive_out="${archive_tar}.gpg"
    log "INFO" "GPG-encrypting for recipient: ${GPG_RECIPIENT}..."
    if gpg --batch --yes --trust-model always \
        --encrypt --recipient "${GPG_RECIPIENT}" \
        --output "${archive_out}" \
        "${archive_tar}" 2>> "${BACKUP_LOG}"; then
      log "INFO" "Encrypted: ${archive_out}"
      rm -f "${archive_tar}"
      echo "${archive_out}"
    else
      die "GPG encryption failed — unencrypted archive NOT uploaded"
      rm -f "${archive_out}" 2>/dev/null || true
      echo ""
    fi
  else
    log "WARN" "GPG_RECIPIENT not set — backup archive is NOT encrypted"
    echo "${archive_tar}"
  fi
}

# ---------------------------------------------------------------------------
# Step 5: Upload offsite via rclone (tiered by date)
# ---------------------------------------------------------------------------

upload_offsite() {
  local archive_path="$1"
  log "INFO" "--- Step 5: Offsite upload ---"

  if [[ -z "${archive_path}" ]]; then
    die "No archive path supplied to upload_offsite — skipping"
    return
  fi

  if ! command -v rclone &>/dev/null; then
    die "'rclone' not found in PATH — skipping offsite upload"
    return
  fi

  if [[ -z "${RCLONE_REMOTE}" ]]; then
    die "RCLONE_REMOTE not set — skipping offsite upload"
    return
  fi

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] rclone copy ${archive_path} → ${RCLONE_REMOTE}/<tier>/"
    return
  fi

  # Determine retention tier
  local day_of_week day_of_month tier
  day_of_week="$(date +%u)"    # 1=Mon … 7=Sun
  day_of_month="$(date +%d)"   # 01–31

  if [[ "${day_of_month}" == "01" ]]; then
    tier="monthly"
  elif [[ "${day_of_week}" == "7" ]]; then
    tier="weekly"
  else
    tier="daily"
  fi

  local remote_path="${RCLONE_REMOTE}/${tier}/"
  log "INFO" "Uploading to ${remote_path} (tier: ${tier})..."

  if rclone copy --progress "${archive_path}" "${remote_path}" >> "${BACKUP_LOG}" 2>&1; then
    log "INFO" "Offsite upload complete → ${remote_path}"
  else
    die "rclone upload failed — backup is local only"
  fi
}

# ---------------------------------------------------------------------------
# Step 6: Prune old local backups and archives
# ---------------------------------------------------------------------------

prune_old_backups() {
  log "INFO" "--- Step 6: Pruning old local data ---"

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "INFO" "[dry-run] Would prune snapshot dirs and archives older than ${BACKUP_RETENTION_DAYS} days"
    return
  fi

  # Prune snapshot directories (raw data, kept shorter than archive files)
  local removed_dirs=0
  while IFS= read -r -d '' d; do
    rm -rf "${d}"
    (( removed_dirs++ )) || true
    log "INFO" "Pruned snapshot dir: ${d}"
  done < <(find "${BACKUP_DIR}" -maxdepth 1 -type d -name "20*-??????" \
              -mtime "+${BACKUP_RETENTION_DAYS}" -print0 2>/dev/null)

  # Prune archive files older than retention
  local removed_files=0
  while IFS= read -r -d '' f; do
    rm -f "${f}"
    (( removed_files++ )) || true
    log "INFO" "Pruned archive: ${f}"
  done < <(find "${BACKUP_DIR}" -maxdepth 1 \( -name "dare-backup-*.tar.gz" -o -name "dare-backup-*.tar.gz.gpg" \) \
              -mtime "+${BACKUP_RETENTION_DAYS}" -print0 2>/dev/null)

  log "INFO" "Pruning complete: removed ${removed_dirs} snapshot dir(s), ${removed_files} archive file(s)"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
  setup
  backup_postgres  || true
  backup_minio     || true
  backup_keycloak  || true

  local archive
  archive="$(create_archive)"

  upload_offsite "${archive}"
  prune_old_backups

  if [[ "${#ERRORS[@]}" -gt 0 ]]; then
    local error_summary
    error_summary="$(printf '• %s\n' "${ERRORS[@]}")"
    log "ERROR" "Backup finished with ${#ERRORS[@]} error(s):"
    printf '%s\n' "${ERRORS[@]}" | while read -r e; do log "ERROR" "  ${e}"; done
    send_alert "Backup [${TIMESTAMP}] failed with ${#ERRORS[@]} error(s):\n${error_summary}"
    exit 1
  fi

  log "INFO" "=== DARE Backup completed successfully: ${TIMESTAMP} ==="
  write_prometheus_metric
  exit 0
}

main "$@"
