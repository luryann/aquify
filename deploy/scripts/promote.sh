#!/usr/bin/env bash
# promote.sh — Promote an image tag to production with health verification and
#              automatic rollback on failure.
#
# Usage:
#   promote.sh <image_tag>
#
# Example:
#   promote.sh sha-abc1234
#
# The script:
#   1. Validates the supplied image tag
#   2. Records the last env that passed a health check (.env.production.good)
#      as the rollback target (.env.production.prev), so a retry or re-run of a
#      bad tag can never overwrite the rollback state
#   3. Updates IMAGE_TAG in .env.production
#   4. Pulls new images
#   5. Does a rolling restart of dare-api, dare-admin, dare-worker
#   6. Waits for the health endpoint to respond 200
#   7. If health fails: runs rollback.sh (one implementation for manual and automatic rollback)
#   8. Logs all events to /var/log/dare/deploys.log

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

# Overridable for tests; production uses the defaults.
DEPLOY_DIR="${DEPLOY_DIR:-/opt/dare}"
ENV_FILE="${DEPLOY_DIR}/.env.production"
ENV_BACKUP="${ENV_FILE}.prev"
ENV_GOOD="${ENV_FILE}.good"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.production.yml"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/api/v1/health/live}"
HEALTH_TIMEOUT_SECONDS="${HEALTH_TIMEOUT_SECONDS:-60}"
HEALTH_RETRY_INTERVAL="${HEALTH_RETRY_INTERVAL:-5}"
DEPLOY_LOG="${DEPLOY_LOG:-/var/log/dare/deploys.log}"
APP_SERVICES="dare-api dare-admin dare-worker"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() {
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "[${ts}] $*" | tee -a "${DEPLOY_LOG}"
}

die() {
  log "ERROR: $*"
  exit 1
}

ensure_log_dir() {
  mkdir -p "$(dirname "${DEPLOY_LOG}")"
  touch "${DEPLOY_LOG}"
}

wait_for_health() {
  local retries=$(( HEALTH_TIMEOUT_SECONDS / HEALTH_RETRY_INTERVAL ))
  log "Waiting for health at ${HEALTH_URL} (timeout ${HEALTH_TIMEOUT_SECONDS}s)..."
  for i in $(seq 1 "${retries}"); do
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" --max-time 4 "${HEALTH_URL}" 2>/dev/null || echo "000")
    if [ "${status}" = "200" ]; then
      log "Health check passed (attempt ${i}/${retries})"
      return 0
    fi
    log "Attempt ${i}/${retries}: HTTP ${status} — retrying in ${HEALTH_RETRY_INTERVAL}s..."
    sleep "${HEALTH_RETRY_INTERVAL}"
  done
  return 1
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

ensure_log_dir

# Validate argument
if [ $# -lt 1 ] || [ -z "${1}" ]; then
  die "Usage: promote.sh <image_tag>  (e.g. promote.sh sha-abc1234)"
fi

IMAGE_TAG="${1}"
# Tags go into a sed-free env rewrite and a compose image reference; allow only
# what a registry tag can contain.
if [[ ! "${IMAGE_TAG}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]]; then
  die "Invalid image tag '${IMAGE_TAG}' (allowed: letters, digits, . _ -)"
fi
log "====== PROMOTE START: IMAGE_TAG=${IMAGE_TAG} operator=$(whoami) ======"

# Change to deploy directory
cd "${DEPLOY_DIR}" || die "Cannot cd to ${DEPLOY_DIR}"

# Validate env file exists
[ -f "${ENV_FILE}" ] || die "Env file not found: ${ENV_FILE}"

# ---------------------------------------------------------------------------
# Step 1: Backup current env
# ---------------------------------------------------------------------------

# The rollback target is the last env that passed a health check, never simply
# "whatever was deployed a moment ago" - otherwise re-running promote after a
# failed attempt would overwrite the backup with the bad configuration.
if [ ! -f "${ENV_GOOD}" ]; then
  log "No last-known-good env yet; treating the current ${ENV_FILE} as good"
  cp "${ENV_FILE}" "${ENV_GOOD}"
fi
log "Recording rollback target ${ENV_GOOD} -> ${ENV_BACKUP}"
cp "${ENV_GOOD}" "${ENV_BACKUP}"

# Capture the old image tag for logging
OLD_TAG=$(grep '^IMAGE_TAG=' "${ENV_FILE}" | cut -d= -f2 || echo "unknown")
log "Current IMAGE_TAG: ${OLD_TAG} -> ${IMAGE_TAG}"

# ---------------------------------------------------------------------------
# Step 2: Update IMAGE_TAG in env file
# ---------------------------------------------------------------------------

# Rewrite without sed -i (not portable) and without interpolating the tag into a
# regex replacement; cat > keeps the file's owner and mode.
ENV_TMP="$(mktemp)"
grep -v '^IMAGE_TAG=' "${ENV_FILE}" > "${ENV_TMP}" || true
echo "IMAGE_TAG=${IMAGE_TAG}" >> "${ENV_TMP}"
cat "${ENV_TMP}" > "${ENV_FILE}"
rm -f "${ENV_TMP}"

log "Updated IMAGE_TAG in ${ENV_FILE}"

# ---------------------------------------------------------------------------
# Step 3: Pull new images
# ---------------------------------------------------------------------------

log "Pulling images for tag ${IMAGE_TAG}..."
if ! IMAGE_TAG="${IMAGE_TAG}" docker compose -f "${COMPOSE_FILE}" pull ${APP_SERVICES}; then
  log "Image pull failed — restoring last-known-good env and aborting"
  cp "${ENV_GOOD}" "${ENV_FILE}"
  die "docker compose pull failed for IMAGE_TAG=${IMAGE_TAG}"
fi

log "Image pull succeeded"

# ---------------------------------------------------------------------------
# Step 4: Rolling restart of application containers
# ---------------------------------------------------------------------------

log "Starting rolling restart of application services: ${APP_SERVICES}"
if ! IMAGE_TAG="${IMAGE_TAG}" docker compose -f "${COMPOSE_FILE}" \
    up -d --no-deps --remove-orphans ${APP_SERVICES}; then
  log "docker compose up failed — running rollback.sh"
  "${SCRIPT_DIR}/rollback.sh" || log "CRITICAL: rollback.sh failed; manual intervention required"
  die "Deploy failed; rollback attempted"
fi

log "Container restart issued — waiting for health..."

# ---------------------------------------------------------------------------
# Step 5: Health verification
# ---------------------------------------------------------------------------

if ! wait_for_health; then
  log "Health check failed after ${HEALTH_TIMEOUT_SECONDS}s — running rollback.sh"
  "${SCRIPT_DIR}/rollback.sh" || log "CRITICAL: rollback.sh failed; manual intervention required"
  die "Deploy failed: health check did not pass. Rollback to ${OLD_TAG} initiated."
fi

# ---------------------------------------------------------------------------
# Step 6: Log success
# ---------------------------------------------------------------------------

# Only now is this env the new rollback target.
cp "${ENV_FILE}" "${ENV_GOOD}"
log "====== PROMOTE SUCCESS: IMAGE_TAG=${IMAGE_TAG} (was ${OLD_TAG}) ======"
