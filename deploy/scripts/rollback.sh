#!/usr/bin/env bash
# rollback.sh — Return the application containers to the previous release.
#
# Usage:
#   rollback.sh
#
# Restores release.env.prev (the last release that passed its health gate when
# the failed promote began) and recreates aquify-api / aquify-admin / aquify-worker.
#
# This does NOT undo database migrations. Releases must keep the schema backward
# compatible with the previous release (see promote.sh); a destructive migration
# is recovered from a backup, not by rollback.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

ensure_log_dir
acquire_lock

log "====== ROLLBACK START: operator=$(whoami) ======"
cd "${DEPLOY_DIR}" || die "Cannot cd to ${DEPLOY_DIR}"

[ -f "${RELEASE_PREV}" ] || die "No rollback target at ${RELEASE_PREV}. Manual intervention required."
[ -f "${SECRETS_ENV}" ] || die "Secrets env file not found: ${SECRETS_ENV}"

CURRENT_TAG="$(release_value "${RELEASE_ENV}" IMAGE_TAG)"
PREV_TAG="$(release_value "${RELEASE_PREV}" IMAGE_TAG)"
[ -n "${PREV_TAG}" ] || die "${RELEASE_PREV} has no IMAGE_TAG"
log "Rolling back from IMAGE_TAG=${CURRENT_TAG:-unknown} to IMAGE_TAG=${PREV_TAG}"

cp "${RELEASE_PREV}" "${RELEASE_ENV}"

log "Pulling images for tag ${PREV_TAG}..."
if ! compose pull ${APP_SERVICES}; then
  log "WARNING: docker compose pull failed — attempting restart with locally cached images"
fi

log "Recreating application services with the previous release..."
if ! compose up -d --no-deps --remove-orphans ${APP_SERVICES}; then
  die "docker compose up failed during rollback. Manual intervention required."
fi

if ! wait_for_ready; then
  log "CRITICAL: not ready even after rollback to IMAGE_TAG=${PREV_TAG}."
  compose ps 2>&1 | tee -a "${DEPLOY_LOG}" || true
  die "Platform is unhealthy after rollback. Immediate manual intervention required."
fi
if ! worker_running; then
  die "Worker is not running after rollback to IMAGE_TAG=${PREV_TAG}. Manual intervention required."
fi

log "====== ROLLBACK SUCCESS: IMAGE_TAG=${PREV_TAG} (was ${CURRENT_TAG:-unknown}) ======"
