#!/usr/bin/env bash
# backup.sh — encrypted backup of PostgreSQL, MinIO objects and the Keycloak realm.
#
# Usage:  backup.sh [--dry-run]
# Run daily by aquify-backup.timer as the deploy user.
#
# What is captured (one directory, then one encrypted archive):
#   postgres.dump        pg_dump custom format of the application database, taken by
#                        the read-only backup role INSIDE the postgres container
#                        (no host pg client, no published database port)
#   minio/               mirror of the storage bucket
#   keycloak-realm-*.json  Keycloak partial export (clients, roles, groups)
#   manifest.txt         schema version and club count, checked again by restore.sh
#
# NOT captured, by design: Keycloak USERS and credentials live in Keycloak's own
# database (keycloak) which is NOT included here; take a separate pg_dump of it if
# user accounts must survive host loss (see deploy/README.md "Backup coverage").
# Redis (job queue) is not backed up; AOF protects restarts, not host loss.
# Secrets files (.env.*) are not in the archive; escrow them separately.
#
# Safety rules:
#   * The archive is ALWAYS encrypted (gpg --recipient GPG_RECIPIENT). Without a
#     recipient the run fails; ALLOW_UNENCRYPTED_BACKUP=1 exists only for
#     synthetic-data rehearsals and is logged as an error-level warning.
#   * The plaintext snapshot directory is removed once the archive exists.
#   * Any failed step fails the whole run (non-zero exit + alert), never a warning.
#
# Config: infrastructure secrets from .env.production, the rest from .env.backup
# (root:aquify 640): MINIO_ACCESS_KEY MINIO_SECRET_KEY [MINIO_ENDPOINT MINIO_BUCKET]
# GPG_RECIPIENT [RCLONE_REMOTE ALERT_WEBHOOK KEYCLOAK_URL KEYCLOAK_REALM].

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/aquify}"
DEPLOY_LOG="${BACKUP_LOG:-/var/log/aquify/backup.log}"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

BACKUP_ENV="${BACKUP_ENV:-${DEPLOY_DIR}/.env.backup}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/aquify}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
NODE_EXPORTER_TEXTFILE_DIR="${NODE_EXPORTER_TEXTFILE_DIR:-/var/lib/node_exporter}"

DRY_RUN=false
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=true

bget() { envget "${BACKUP_ENV}" "$1"; }
PG_DB="$(envget "${SECRETS_ENV}" POSTGRES_DB)"; PG_DB="${PG_DB:-aquify}"
PG_USER="$(envget "${SECRETS_ENV}" AQUIFY_BACKUP_DB_USER)"
PG_PASS="$(envget "${SECRETS_ENV}" AQUIFY_BACKUP_DB_PASSWORD)"
MINIO_ENDPOINT="$(bget MINIO_ENDPOINT)"; MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://127.0.0.1:9000}"
MINIO_ACCESS_KEY="$(bget MINIO_ACCESS_KEY)"
MINIO_SECRET_KEY="$(bget MINIO_SECRET_KEY)"
MINIO_BUCKET="$(bget MINIO_BUCKET)"; MINIO_BUCKET="${MINIO_BUCKET:-aquify}"
KEYCLOAK_URL="$(bget KEYCLOAK_URL)"; KEYCLOAK_URL="${KEYCLOAK_URL:-http://127.0.0.1:8081}"
KEYCLOAK_REALM="$(bget KEYCLOAK_REALM)"; KEYCLOAK_REALM="${KEYCLOAK_REALM:-aquify}"
KC_ADMIN_USER="$(envget "${SECRETS_ENV}" KEYCLOAK_ADMIN_USER)"
KC_ADMIN_PASS="$(envget "${SECRETS_ENV}" KEYCLOAK_ADMIN_PASSWORD)"
GPG_RECIPIENT="$(bget GPG_RECIPIENT)"
RCLONE_REMOTE="$(bget RCLONE_REMOTE)"
ALERT_WEBHOOK="$(bget ALERT_WEBHOOK)"

TIMESTAMP="$(date -u +%Y%m%d-%H%M%S)"
SNAPSHOT_DIR="${BACKUP_DIR}/${TIMESTAMP}"
ERRORS=()

record_failure() { log "ERROR: $*"; ERRORS+=("$*"); }

send_alert() {
  [[ -n "${ALERT_WEBHOOK}" ]] || { log "WARN: ALERT_WEBHOOK not set; no alert sent"; return 0; }
  curl -s -X POST "${ALERT_WEBHOOK}" -H "Content-Type: application/json" --max-time 10 \
    --data "{\"text\":\"aquify backup FAILED [${TIMESTAMP}]: $1\"}" >/dev/null 2>&1 \
    || log "WARN: failed to deliver the alert"
}

write_metric() {
  [[ -d "${NODE_EXPORTER_TEXTFILE_DIR}" ]] || return 0
  {
    echo "# HELP aquify_backup_last_success_timestamp Unix time of the last successful aquify backup"
    echo "# TYPE aquify_backup_last_success_timestamp gauge"
    echo "aquify_backup_last_success_timestamp $(date +%s)"
  } > "${NODE_EXPORTER_TEXTFILE_DIR}/aquify_backup.prom.tmp" \
    && mv "${NODE_EXPORTER_TEXTFILE_DIR}/aquify_backup.prom.tmp" "${NODE_EXPORTER_TEXTFILE_DIR}/aquify_backup.prom"
}

# psql_scalar <sql>: one value from the database as the backup role.
psql_scalar() {
  PGPASSWORD="${PG_PASS}" compose exec -T -e PGPASSWORD postgres \
    psql -U "${PG_USER}" -d "${PG_DB}" -Atq -c "$1"
}

backup_postgres() {
  log "--- PostgreSQL ---"
  if [[ -z "${PG_USER}" || -z "${PG_PASS}" ]]; then
    record_failure "AQUIFY_BACKUP_DB_USER / AQUIFY_BACKUP_DB_PASSWORD missing from ${SECRETS_ENV}"; return 0
  fi
  local dump="${SNAPSHOT_DIR}/postgres.dump"
  if [[ "${DRY_RUN}" == "true" ]]; then log "[dry-run] pg_dump ${PG_DB} -> ${dump}"; return 0; fi

  # The password reaches the container through the docker CLI's environment
  # (-e PGPASSWORD without a value), so it never appears in a process listing.
  if ! PGPASSWORD="${PG_PASS}" compose exec -T -e PGPASSWORD postgres \
        pg_dump -U "${PG_USER}" -d "${PG_DB}" --format=custom --compress=9 --no-password > "${dump}"; then
    record_failure "pg_dump failed"; rm -f "${dump}"; return 0
  fi
  # A dump that cannot be read back is not a backup.
  if ! compose exec -T postgres pg_restore --list < "${dump}" > /dev/null; then
    record_failure "pg_restore --list could not read the dump; backup is unusable"; rm -f "${dump}"; return 0
  fi
  local version orgs
  version="$(psql_scalar "SELECT COALESCE(max(version_id), 0) FROM goose_db_version" 2>/dev/null)" || version=""
  orgs="$(psql_scalar "SELECT count(*) FROM orgs" 2>/dev/null)" || orgs=""
  if [[ -z "${version}" || -z "${orgs}" ]]; then
    record_failure "could not read the schema version / club count for the manifest"; return 0
  fi
  {
    echo "timestamp=${TIMESTAMP}"
    echo "database=${PG_DB}"
    echo "goose_version=${version}"
    echo "org_count=${orgs}"
  } > "${SNAPSHOT_DIR}/manifest.txt"
  log "pg_dump verified ($(du -h "${dump}" | cut -f1)); schema ${version}, ${orgs} club(s)"
}

backup_minio() {
  log "--- MinIO ---"
  if ! command -v mc >/dev/null 2>&1; then record_failure "'mc' (MinIO client) is not installed"; return 0; fi
  if [[ -z "${MINIO_ACCESS_KEY}" || -z "${MINIO_SECRET_KEY}" ]]; then
    record_failure "MINIO_ACCESS_KEY / MINIO_SECRET_KEY missing from ${BACKUP_ENV}"; return 0
  fi
  if [[ "${DRY_RUN}" == "true" ]]; then log "[dry-run] mc mirror ${MINIO_BUCKET}"; return 0; fi
  # Credentials go through mc's environment alias form, not a persisted config.
  local alias_env="MC_HOST_aquifysrc=${MINIO_ENDPOINT/\/\//\/\/${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@}"
  mkdir -p "${SNAPSHOT_DIR}/minio"
  if ! env "${alias_env}" mc mirror --preserve "aquifysrc/${MINIO_BUCKET}" "${SNAPSHOT_DIR}/minio" >> "${DEPLOY_LOG}" 2>&1; then
    record_failure "mc mirror failed"
  fi
}

backup_keycloak() {
  log "--- Keycloak realm ---"
  if [[ -z "${KC_ADMIN_PASS}" ]]; then record_failure "KEYCLOAK_ADMIN_PASSWORD missing from ${SECRETS_ENV}"; return 0; fi
  if ! command -v python3 >/dev/null 2>&1; then record_failure "python3 is required to read the Keycloak token"; return 0; fi
  local realm_file="${SNAPSHOT_DIR}/keycloak-realm-${KEYCLOAK_REALM}.json"
  if [[ "${DRY_RUN}" == "true" ]]; then log "[dry-run] export realm ${KEYCLOAK_REALM}"; return 0; fi
  local token_json access_token
  # printf, not a heredoc: a heredoc appends a newline that ends up inside grant_type and
  # Keycloak answers unsupported_grant_type (found by running against real Keycloak). The
  # body goes through stdin so the password never appears in the process list. The password
  # charset is restricted to [A-Za-z0-9_-] by preflight.sh, so no URL-encoding is needed.
  token_json="$(printf 'client_id=admin-cli&username=%s&password=%s&grant_type=password' "${KC_ADMIN_USER}" "${KC_ADMIN_PASS}" \
    | curl -sf -X POST "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
      -H "Content-Type: application/x-www-form-urlencoded" --data-binary @- --max-time 30
  )" || { record_failure "Keycloak token request failed (is Keycloak up?)"; return 0; }
  access_token="$(printf '%s' "${token_json}" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null)" || access_token=""
  if [[ -z "${access_token}" ]]; then record_failure "no access token in the Keycloak response"; return 0; fi
  if ! curl -sf -o "${realm_file}" --max-time 60 -H "Authorization: Bearer ${access_token}" \
        "${KEYCLOAK_URL}/admin/realms/${KEYCLOAK_REALM}/partial-export?exportClients=true&exportGroupsAndRoles=true" \
        -X POST; then
    record_failure "Keycloak realm export failed"; rm -f "${realm_file}"
  fi
}

# create_archive: sets ARCHIVE_PATH to the final archive ("" when none was made).
# It must run in the main shell, NOT inside $(...): record_failure appends to ERRORS,
# and a subshell would silently drop those failures and let the run exit 0.
ARCHIVE_PATH=""
create_archive() {
  ARCHIVE_PATH=""
  local tarball="${BACKUP_DIR}/aquify-backup-${TIMESTAMP}.tar.gz"
  if [[ "${DRY_RUN}" == "true" ]]; then log "[dry-run] archive ${tarball}"; return 0; fi
  if [[ ! -s "${SNAPSHOT_DIR}/postgres.dump" ]]; then
    record_failure "no database dump in the snapshot; refusing to create an archive"; return 0
  fi
  if ! tar -czf "${tarball}" -C "${BACKUP_DIR}" "${TIMESTAMP}"; then
    record_failure "tar failed"; rm -f "${tarball}"; return 0
  fi
  if [[ -n "${GPG_RECIPIENT}" ]]; then
    if gpg --batch --yes --trust-model always --encrypt --recipient "${GPG_RECIPIENT}" \
         --output "${tarball}.gpg" "${tarball}" >> "${DEPLOY_LOG}" 2>&1; then
      rm -f "${tarball}"
      ARCHIVE_PATH="${tarball}.gpg"
    else
      record_failure "gpg encryption failed; no unencrypted archive was kept"
      rm -f "${tarball}" "${tarball}.gpg"
    fi
  elif [[ "${ALLOW_UNENCRYPTED_BACKUP:-}" == "1" ]]; then
    log "ERROR-LEVEL WARNING: ALLOW_UNENCRYPTED_BACKUP=1; the archive is NOT encrypted (rehearsal use only)"
    ARCHIVE_PATH="${tarball}"
  else
    record_failure "GPG_RECIPIENT is not set: refusing to write an unencrypted backup"
    rm -f "${tarball}"
  fi
}

upload_offsite() {
  local archive="$1"
  if [[ -z "${archive}" ]]; then return 0; fi
  if [[ -z "${RCLONE_REMOTE}" ]]; then record_failure "RCLONE_REMOTE not set: the backup is on this host only"; return 0; fi
  if ! command -v rclone >/dev/null 2>&1; then record_failure "'rclone' is not installed"; return 0; fi
  [[ "${DRY_RUN}" == "true" ]] && { log "[dry-run] rclone copy ${archive}"; return 0; }
  local dom dow tier
  dom="$(date +%d)"; dow="$(date +%u)"
  if [[ "${dom}" == "01" ]]; then tier=monthly; elif [[ "${dow}" == "7" ]]; then tier=weekly; else tier=daily; fi
  if ! rclone copy "${archive}" "${RCLONE_REMOTE}/${tier}/" >> "${DEPLOY_LOG}" 2>&1; then
    record_failure "rclone upload failed; the backup is on this host only"; return 0
  fi
  log "Uploaded to ${RCLONE_REMOTE}/${tier}/"
  # Offsite retention: daily 30d, weekly 12 weeks, monthly 12 months.
  local spec name age rc
  for spec in "daily 30d" "weekly 84d" "monthly 366d"; do
    read -r name age <<< "${spec}"
    # rclone exits 3 ("directory not found") for a tier that has not been written yet (the
    # weekly/monthly folders do not exist until their first upload); that is not a failure.
    # Found by running against a real rclone: every daily run reported two false errors.
    rc=0
    rclone delete --min-age "${age}" "${RCLONE_REMOTE}/${name}/" >> "${DEPLOY_LOG}" 2>&1 || rc=$?
    if [[ "${rc}" != "0" && "${rc}" != "3" ]]; then record_failure "offsite retention pruning failed for ${name}"; fi
  done
}

prune_local() {
  [[ "${DRY_RUN}" == "true" ]] && return 0
  find "${BACKUP_DIR}" -maxdepth 1 -type f \( -name 'aquify-backup-*.tar.gz' -o -name 'aquify-backup-*.tar.gz.gpg' \) \
    -mtime "+${BACKUP_RETENTION_DAYS}" -delete 2>/dev/null || true
}

main() {
  ensure_log_dir
  mkdir -p "${SNAPSHOT_DIR}"
  chmod 700 "${SNAPSHOT_DIR}"
  log "=== aquify backup started: ${TIMESTAMP} ==="
  backup_postgres
  backup_minio
  backup_keycloak

  create_archive
  local archive="${ARCHIVE_PATH}"
  # The plaintext snapshot never outlives the run.
  if [[ "${DRY_RUN}" != "true" ]]; then
    find "${SNAPSHOT_DIR}" -mindepth 1 -delete 2>/dev/null || true
    rmdir "${SNAPSHOT_DIR}" 2>/dev/null || true
  fi
  upload_offsite "${archive}"
  prune_local

  if (( ${#ERRORS[@]} > 0 )); then
    log "Backup finished with ${#ERRORS[@]} error(s)"
    send_alert "$(printf '%s; ' "${ERRORS[@]}")"
    exit 1
  fi
  log "=== aquify backup completed: ${TIMESTAMP} (${archive:-dry-run}) ==="
  write_metric
}

main "$@"
