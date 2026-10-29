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
#   2. Backs up the current .env.production
#   3. Updates IMAGE_TAG in .env.production
#   4. Pulls new images
#   5. Does a rolling restart of dare-api, dare-admin, dare-worker
#   6. Waits for the health endpoint to respond 200
#   7. If health fails: auto-rolls back using the .env backup
#   8. Logs all events to /var/log/dare/deploys.log

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DEPLOY_DIR="/opt/dare"
ENV_FILE="${DEPLOY_DIR}/.env.production"
ENV_BACKUP="${ENV_FILE}.prev"
COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.production.yml"
HEALTH_URL="http://localhost:8080/api/v1/health/live"
HEALTH_TIMEOUT_SECONDS=60
HEALTH_RETRY_INTERVAL=5
DEPLOY_LOG="/var/log/dare/deploys.log"
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

rollback() {
  local reason="${1:-health check failed}"
  log "ROLLBACK triggered: ${reason}"

  if [ ! -f "${ENV_BACKUP}" ]; then
    log "WARNING: No backup env file found at ${ENV_BACKUP}; cannot auto-rollback."
    return 1
  fi

  log "Restoring ${ENV_BACKUP} -> ${ENV_FILE}"
  cp "${ENV_BACKUP}" "${ENV_FILE}"

  # Compose interpolates IMAGE_TAG from the shell, not from the env file, so pass
  # the previous tag explicitly (as rollback.sh does).
  local prev_tag
  prev_tag="$(grep '^IMAGE_TAG=' "${ENV_BACKUP}" | cut -d= -f2 || true)"

  log "Pulling rolled-back images..."
  IMAGE_TAG="${prev_tag:-latest}" docker compose -f "${COMPOSE_FILE}" pull ${APP_SERVICES}

  log "Restarting services with rolled-back image..."
  IMAGE_TAG="${prev_tag:-latest}" docker compose -f "${COMPOSE_FILE}" up -d --no-deps --remove-orphans ${APP_SERVICES}

  if wait_for_health; then
    log "Rollback succeeded — platform is healthy on previous image."
  else
    log "CRITICAL: Rollback completed but health check still failing. Manual intervention required."
  fi
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
log "====== PROMOTE START: IMAGE_TAG=${IMAGE_TAG} operator=$(whoami) ======"

# Change to deploy directory
cd "${DEPLOY_DIR}" || die "Cannot cd to ${DEPLOY_DIR}"

# Validate env file exists
[ -f "${ENV_FILE}" ] || die "Env file not found: ${ENV_FILE}"

# ---------------------------------------------------------------------------
# Step 1: Backup current env
# ---------------------------------------------------------------------------

log "Backing up ${ENV_FILE} -> ${ENV_BACKUP}"
cp "${ENV_FILE}" "${ENV_BACKUP}"

# Capture the old image tag for logging
OLD_TAG=$(grep '^IMAGE_TAG=' "${ENV_FILE}" | cut -d= -f2 || echo "unknown")
log "Current IMAGE_TAG: ${OLD_TAG} -> ${IMAGE_TAG}"

# ---------------------------------------------------------------------------
# Step 2: Update IMAGE_TAG in env file
# ---------------------------------------------------------------------------

if grep -q '^IMAGE_TAG=' "${ENV_FILE}"; then
  # Replace existing IMAGE_TAG line
  sed -i "s|^IMAGE_TAG=.*|IMAGE_TAG=${IMAGE_TAG}|" "${ENV_FILE}"
else
  # Append IMAGE_TAG
  echo "IMAGE_TAG=${IMAGE_TAG}" >> "${ENV_FILE}"
fi

log "Updated IMAGE_TAG in ${ENV_FILE}"

# ---------------------------------------------------------------------------
# Step 3: Pull new images
# ---------------------------------------------------------------------------

log "Pulling images for tag ${IMAGE_TAG}..."
if ! IMAGE_TAG="${IMAGE_TAG}" docker compose -f "${COMPOSE_FILE}" pull ${APP_SERVICES}; then
  log "Image pull failed — restoring backup and aborting"
  cp "${ENV_BACKUP}" "${ENV_FILE}"
  die "docker compose pull failed for IMAGE_TAG=${IMAGE_TAG}"
fi

log "Image pull succeeded"

# ---------------------------------------------------------------------------
# Step 4: Rolling restart of application containers
# ---------------------------------------------------------------------------

log "Starting rolling restart of application services: ${APP_SERVICES}"
if ! IMAGE_TAG="${IMAGE_TAG}" docker compose -f "${COMPOSE_FILE}" \
    up -d --no-deps --remove-orphans ${APP_SERVICES}; then
  log "docker compose up failed — triggering rollback"
  rollback "compose up failed"
  die "Deploy failed; rollback attempted"
fi

log "Container restart issued — waiting for health..."

# ---------------------------------------------------------------------------
# Step 5: Health verification
# ---------------------------------------------------------------------------

if ! wait_for_health; then
  log "Health check failed after ${HEALTH_TIMEOUT_SECONDS}s — triggering rollback"
  rollback "health check failed after ${HEALTH_TIMEOUT_SECONDS}s"
  die "Deploy failed: health check did not pass. Rollback to ${OLD_TAG} initiated."
fi

# ---------------------------------------------------------------------------
# Step 6: Log success
# ---------------------------------------------------------------------------

log "====== PROMOTE SUCCESS: IMAGE_TAG=${IMAGE_TAG} (was ${OLD_TAG}) ======"
