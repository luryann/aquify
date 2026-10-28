#!/usr/bin/env bash
# rollback.sh — Roll back the production deployment to the previously running
#               image tag by restoring the .env.production.prev backup.
#
# Usage:
#   rollback.sh
#
# No arguments required. The script reads the previous image tag from
# .env.production.prev and restores it.

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

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

ensure_log_dir

log "====== ROLLBACK START: operator=$(whoami) ======"

cd "${DEPLOY_DIR}" || die "Cannot cd to ${DEPLOY_DIR}"

# ---------------------------------------------------------------------------
# Step 1: Verify backup exists
# ---------------------------------------------------------------------------

if [ ! -f "${ENV_BACKUP}" ]; then
  die "No backup env file at ${ENV_BACKUP}. Cannot roll back automatically. Manual intervention required."
fi

# Read current and previous image tags for logging
CURRENT_TAG=$(grep '^IMAGE_TAG=' "${ENV_FILE}" 2>/dev/null | cut -d= -f2 || echo "unknown")
PREV_TAG=$(grep '^IMAGE_TAG=' "${ENV_BACKUP}" 2>/dev/null | cut -d= -f2 || echo "unknown")

log "Rolling back from IMAGE_TAG=${CURRENT_TAG} to IMAGE_TAG=${PREV_TAG}"

# ---------------------------------------------------------------------------
# Step 2: Restore backup env file
# ---------------------------------------------------------------------------

log "Restoring ${ENV_BACKUP} -> ${ENV_FILE}"
cp "${ENV_BACKUP}" "${ENV_FILE}"

# ---------------------------------------------------------------------------
# Step 3: Pull rolled-back images
# ---------------------------------------------------------------------------

log "Pulling images for tag ${PREV_TAG}..."
if ! IMAGE_TAG="${PREV_TAG}" docker compose -f "${COMPOSE_FILE}" pull ${APP_SERVICES}; then
  # Image pull failed but we still try to restart with what's cached locally
  log "WARNING: docker compose pull failed — attempting restart with locally cached images"
fi

# ---------------------------------------------------------------------------
# Step 4: Restart services
# ---------------------------------------------------------------------------

log "Restarting application services with rolled-back image..."
if ! IMAGE_TAG="${PREV_TAG}" docker compose -f "${COMPOSE_FILE}" \
    up -d --no-deps --remove-orphans ${APP_SERVICES}; then
  die "docker compose up failed during rollback. Manual intervention required."
fi

# ---------------------------------------------------------------------------
# Step 5: Health verification
# ---------------------------------------------------------------------------

if ! wait_for_health; then
  log "CRITICAL: Health check failed even after rollback to IMAGE_TAG=${PREV_TAG}."
  log "Current container status:"
  docker compose -f "${COMPOSE_FILE}" ps 2>&1 | tee -a "${DEPLOY_LOG}" || true
  die "Platform is unhealthy after rollback. Immediate manual intervention required."
fi

# ---------------------------------------------------------------------------
# Step 6: Log success
# ---------------------------------------------------------------------------

log "====== ROLLBACK SUCCESS: IMAGE_TAG=${PREV_TAG} (was ${CURRENT_TAG}) ======"
