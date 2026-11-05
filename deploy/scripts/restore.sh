#!/usr/bin/env bash
# restore.sh — restore the application database and storage objects from a backup
# made by backup.sh. Works on the same host or on a fresh one.
#
# Usage:  restore.sh <backup-timestamp | /path/to/archive> [--dry-run] [--skip-objects] [--yes]
#   restore.sh 20261104-020000
#   restore.sh /var/backups/aquify/aquify-backup-20261104-020000.tar.gz.gpg --yes
#
# Order, and why:
#   1. Find the archive (local, else rclone) and decrypt it into a private work dir.
#   2. Check the snapshot is complete and the dump is readable BEFORE touching anything.
#   3. Stop EVERY writer: api, worker, admin and keycloak (a live Keycloak keeps writing
#      sessions into the database being replaced).
#   4. pg_restore as the bootstrap superuser, --single-transaction --exit-on-error: the
#      restore is all-or-nothing and any error aborts it. (Superuser is required because
#      row-level security is FORCEd on tenant tables; the data load would otherwise be
#      refused. Ownership and grants come back from the dump.)
#   5. Compare schema version and club count with the backup's manifest.
#   6. Restore objects (unless --skip-objects). A missing mc, credentials or snapshot
#      directory is a hard failure, never a silent skip.
#   7. Run migrations with the current release (the dump may be from an older schema).
#   8. Start the services and require readiness plus a running worker.
#
# The plaintext work dir is removed on exit, success or failure. Keycloak USERS are not
# in this archive (see backup.sh); this script restores application data only.

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/aquify}"
DEPLOY_LOG="${RESTORE_LOG:-/var/log/aquify/restores.log}"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

BACKUP_ENV="${BACKUP_ENV:-${DEPLOY_DIR}/.env.backup}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/aquify}"
WRITERS="aquify-api aquify-worker aquify-admin keycloak"

TARGET=""; DRY_RUN=false; SKIP_OBJECTS=false; ASSUME_YES=false
for arg in "$@"; do
  case "${arg}" in
    --dry-run) DRY_RUN=true ;;
    --skip-objects) SKIP_OBJECTS=true ;;
    --yes) ASSUME_YES=true ;;
    -*) die "unknown option ${arg}" ;;
    *) [[ -z "${TARGET}" ]] || die "only one backup may be given"; TARGET="${arg}" ;;
  esac
done
[[ -n "${TARGET}" ]] || die "usage: restore.sh <backup-timestamp|archive> [--dry-run] [--skip-objects] [--yes]"

bget() { envget "${BACKUP_ENV}" "$1"; }
PG_DB="$(envget "${SECRETS_ENV}" POSTGRES_DB)"; PG_DB="${PG_DB:-aquify}"
PG_SUPER="$(envget "${SECRETS_ENV}" POSTGRES_USER)"
PG_SUPER_PASS="$(envget "${SECRETS_ENV}" POSTGRES_PASSWORD)"
MINIO_ENDPOINT="$(bget MINIO_ENDPOINT)"; MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://127.0.0.1:9000}"
MINIO_ACCESS_KEY="$(bget MINIO_ACCESS_KEY)"; MINIO_SECRET_KEY="$(bget MINIO_SECRET_KEY)"
MINIO_BUCKET="$(bget MINIO_BUCKET)"; MINIO_BUCKET="${MINIO_BUCKET:-aquify}"
RCLONE_REMOTE="$(bget RCLONE_REMOTE)"

ensure_log_dir
acquire_lock
WORK_DIR=""
cleanup() { [[ -z "${WORK_DIR}" ]] || { find "${WORK_DIR}" -mindepth 1 -delete 2>/dev/null || true; rmdir "${WORK_DIR}" 2>/dev/null || true; }; release_lock; }
trap cleanup EXIT

[[ -f "${SECRETS_ENV}" ]] || die "secrets env file not found: ${SECRETS_ENV}"
[[ -f "${RELEASE_ENV}" ]] || die "release env not found: ${RELEASE_ENV} (needed by compose)"
[[ -n "${PG_SUPER}" && -n "${PG_SUPER_PASS}" ]] || die "POSTGRES_USER / POSTGRES_PASSWORD missing from ${SECRETS_ENV}"

log "====== RESTORE START: ${TARGET} operator=$(whoami) dry-run=${DRY_RUN} ======"

# ---- 1. locate + decrypt ---------------------------------------------------
ARCHIVE=""
if [[ -f "${TARGET}" ]]; then
  ARCHIVE="${TARGET}"
else
  for cand in "${BACKUP_DIR}/aquify-backup-${TARGET}.tar.gz.gpg" "${BACKUP_DIR}/aquify-backup-${TARGET}.tar.gz"; do
    [[ -f "${cand}" ]] && { ARCHIVE="${cand}"; break; }
  done
fi
if [[ -z "${ARCHIVE}" && -n "${RCLONE_REMOTE}" ]] && command -v rclone >/dev/null 2>&1; then
  mkdir -p "${BACKUP_DIR}"
  for tier in daily weekly monthly; do
    for ext in tar.gz.gpg tar.gz; do
      if [[ "${DRY_RUN}" == "true" ]]; then continue; fi
      if rclone copyto "${RCLONE_REMOTE}/${tier}/aquify-backup-${TARGET}.${ext}" "${BACKUP_DIR}/aquify-backup-${TARGET}.${ext}" >/dev/null 2>&1; then
        ARCHIVE="${BACKUP_DIR}/aquify-backup-${TARGET}.${ext}"; break 2
      fi
    done
  done
fi
[[ -n "${ARCHIVE}" ]] || die "no archive found for '${TARGET}' locally or offsite"
log "Archive: ${ARCHIVE}"

if [[ "${DRY_RUN}" == "true" ]]; then
  log "[dry-run] would decrypt, verify, stop ${WRITERS}, pg_restore, check manifest, restore objects, migrate, start services"
  log "====== RESTORE DRY-RUN COMPLETE ======"
  exit 0
fi

mkdir -p "${BACKUP_DIR}"
WORK_DIR="$(mktemp -d "${BACKUP_DIR}/.restore-XXXXXX")"
chmod 700 "${WORK_DIR}"
if [[ "${ARCHIVE}" == *.gpg ]]; then
  command -v gpg >/dev/null 2>&1 || die "gpg is required to decrypt ${ARCHIVE}"
  gpg --batch --yes --decrypt --output "${WORK_DIR}/archive.tar.gz" "${ARCHIVE}" >> "${DEPLOY_LOG}" 2>&1 \
    || die "gpg decryption failed (is the private key in this user's keyring?)"
  TARBALL="${WORK_DIR}/archive.tar.gz"
else
  TARBALL="${ARCHIVE}"
fi
tar -xzf "${TARBALL}" -C "${WORK_DIR}" || die "archive is not a valid tar.gz"
SNAP="$(find "${WORK_DIR}" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
[[ -n "${SNAP}" ]] || die "archive contains no snapshot directory"

# ---- 2. validate the snapshot before changing anything ----------------------
[[ -s "${SNAP}/postgres.dump" ]] || die "snapshot has no postgres.dump"
[[ -s "${SNAP}/manifest.txt" ]] || die "snapshot has no manifest.txt (not made by this backup.sh?)"
want_version="$(envget "${SNAP}/manifest.txt" goose_version)"
want_orgs="$(envget "${SNAP}/manifest.txt" org_count)"
[[ -n "${want_version}" && -n "${want_orgs}" ]] || die "manifest.txt is incomplete"
compose up -d postgres || die "could not start postgres"
for _ in $(seq 1 30); do compose exec -T postgres pg_isready -U "${PG_SUPER}" -d "${PG_DB}" >/dev/null 2>&1 && break; sleep 2; done
compose exec -T postgres pg_restore --list < "${SNAP}/postgres.dump" >/dev/null || die "postgres.dump is unreadable; nothing was changed"
if [[ "${SKIP_OBJECTS}" != "true" ]]; then
  [[ -d "${SNAP}/minio" ]] || die "snapshot has no minio/ directory (use --skip-objects only if that is intended)"
  command -v mc >/dev/null 2>&1 || die "'mc' is required to restore objects (use --skip-objects only if that is intended)"
  [[ -n "${MINIO_ACCESS_KEY}" && -n "${MINIO_SECRET_KEY}" ]] || die "MINIO_ACCESS_KEY / MINIO_SECRET_KEY missing from ${BACKUP_ENV}"
fi
log "Snapshot verified: schema ${want_version}, ${want_orgs} club(s)"

if [[ "${ASSUME_YES}" != "true" ]]; then
  read -r -p "This REPLACES database '${PG_DB}' on this host with the backup. Type RESTORE to continue: " answer
  [[ "${answer}" == "RESTORE" ]] || die "aborted by operator"
fi

# ---- 3. stop every writer ---------------------------------------------------
log "Stopping writers: ${WRITERS}"
compose stop ${WRITERS} || die "could not stop all services; refusing to restore under live writers"

# ---- 4. database -------------------------------------------------------------
log "Restoring PostgreSQL (single transaction, exit on first error)..."
if ! PGPASSWORD="${PG_SUPER_PASS}" compose exec -T -e PGPASSWORD postgres \
      pg_restore -U "${PG_SUPER}" -d "${PG_DB}" --clean --if-exists --single-transaction --exit-on-error --no-password \
      < "${SNAP}/postgres.dump"; then
  die "pg_restore failed; the transaction was rolled back and the database is unchanged. Services are STOPPED."
fi

# ---- 5. manifest check ---------------------------------------------------------
q() { PGPASSWORD="${PG_SUPER_PASS}" compose exec -T -e PGPASSWORD postgres psql -U "${PG_SUPER}" -d "${PG_DB}" -Atq -c "$1"; }
got_version="$(q "SELECT COALESCE(max(version_id), 0) FROM goose_db_version")" || die "cannot read the restored schema version"
got_orgs="$(q "SELECT count(*) FROM orgs")" || die "cannot read the restored club count"
[[ "${got_version}" == "${want_version}" ]] || die "schema version after restore is ${got_version}, manifest says ${want_version}"
[[ "${got_orgs}" == "${want_orgs}" ]] || die "club count after restore is ${got_orgs}, manifest says ${want_orgs}"
log "Database matches the manifest (schema ${got_version}, ${got_orgs} club(s))"

# ---- 6. objects -------------------------------------------------------------------
if [[ "${SKIP_OBJECTS}" == "true" ]]; then
  log "WARNING: --skip-objects: stored files were NOT restored"
else
  log "Restoring objects into ${MINIO_BUCKET}..."
  compose up -d minio || die "could not start minio"
  # Exported inside a subshell so the secret is only in mc's environment, never in an argv.
  ( export MC_HOST_aquifydst="${MINIO_ENDPOINT/\/\//\/\/${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@}"
    mc mb --ignore-existing "aquifydst/${MINIO_BUCKET}" >> "${DEPLOY_LOG}" 2>&1 ) || die "cannot reach MinIO / create the bucket"
  ( export MC_HOST_aquifydst="${MINIO_ENDPOINT/\/\//\/\/${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@}"
    mc mirror --overwrite --preserve "${SNAP}/minio" "aquifydst/${MINIO_BUCKET}" >> "${DEPLOY_LOG}" 2>&1 ) \
    || die "object restore failed"
fi

# ---- 7. migrate to the current release ---------------------------------------------
log "Applying migrations for the current release..."
compose --profile tools run --rm aquify-migrate || die "migration after restore failed"

# ---- 8. start and verify ----------------------------------------------------------------
log "Starting services..."
# The API validates Keycloak's JWKS through the ingress at startup, so keycloak AND caddy must
# be up first (caddy is not started by `up keycloak`: found by running a restore on a clean
# stack). api/worker are started before admin so compose does not wait on admin's
# "api healthy" condition while the API is still retrying Keycloak; admin follows readiness.
compose up -d keycloak caddy || die "keycloak/ingress did not start"
compose up -d --no-deps aquify-api aquify-worker || die "api/worker did not start"
wait_for_ready || die "API did not become ready after restore"
compose up -d --no-deps aquify-admin || die "admin did not start"
worker_running || die "worker is not running after restore"

log "====== RESTORE SUCCESS: ${TARGET} (schema ${got_version}, ${got_orgs} club(s)) ======"
