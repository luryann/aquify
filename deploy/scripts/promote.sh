#!/usr/bin/env bash
# promote.sh — Promote an image tag to production: migrate the database, swap the
#              application containers, verify readiness, roll back on failure.
#
# Usage:
#   promote.sh <image_tag>        e.g. promote.sh sha-abc1234
#
# Steps:
#   1. Take the deploy lock; validate the tag
#   2. Record the last healthy release (release.env.good) as the rollback target
#   3. Write the new tag to release.env (never touches the secrets file)
#   4. Pull the new images
#   5. Run `aquify-api migrate up` with the NEW image BEFORE touching running
#      containers; a failed migration aborts the deploy with the old release still up
#   6. Recreate aquify-api / aquify-admin / aquify-worker
#   7. Require API readiness (dependency checks) and a running worker
#   8. On failure run rollback.sh; on success mark the release good
#
# Migrations are applied before the swap, so the previous release briefly runs
# against the new schema, and rollback does not undo migrations. Every migration
# must therefore be backward compatible with the release before it
# (expand now, contract in a later release).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

ensure_log_dir

if [ $# -lt 1 ] || [ -z "${1}" ]; then
  die "Usage: promote.sh <image_tag>  (e.g. promote.sh sha-abc1234)"
fi
IMAGE_TAG="${1}"
# Tags go into an env file and a compose image reference; allow only what a
# registry tag can contain.
if [[ ! "${IMAGE_TAG}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]]; then
  die "Invalid image tag '${IMAGE_TAG}' (allowed: letters, digits, . _ -)"
fi

acquire_lock
log "====== PROMOTE START: IMAGE_TAG=${IMAGE_TAG} operator=$(whoami) ======"

cd "${DEPLOY_DIR}" || die "Cannot cd to ${DEPLOY_DIR}"
[ -f "${SECRETS_ENV}" ] || die "Secrets env file not found: ${SECRETS_ENV}"

# GHCR namespace: keep the one already deployed, else take it from the caller.
NAMESPACE="$(release_value "${RELEASE_ENV}" GHCR_NAMESPACE)"
NAMESPACE="${NAMESPACE:-${GHCR_NAMESPACE:-}}"
[ -n "${NAMESPACE}" ] || die "GHCR_NAMESPACE unknown: set it in ${RELEASE_ENV} or the environment (first deploy)"

# ---------------------------------------------------------------------------
# Step 1: rollback target = last release that passed its health gate
# ---------------------------------------------------------------------------
if [ ! -f "${RELEASE_GOOD}" ]; then
  if [ -f "${RELEASE_ENV}" ]; then
    log "No last-known-good release yet; treating the current ${RELEASE_ENV} as good"
    cp "${RELEASE_ENV}" "${RELEASE_GOOD}"
  else
    log "First deploy: no previous release to roll back to"
  fi
fi
if [ -f "${RELEASE_GOOD}" ]; then
  cp "${RELEASE_GOOD}" "${RELEASE_PREV}"
fi
OLD_TAG="$(release_value "${RELEASE_ENV}" IMAGE_TAG)"
OLD_TAG="${OLD_TAG:-none}"
log "Current IMAGE_TAG: ${OLD_TAG} -> ${IMAGE_TAG}"

# ---------------------------------------------------------------------------
# Step 2: write the new release
# ---------------------------------------------------------------------------
write_release "${RELEASE_ENV}" "${IMAGE_TAG}" "${NAMESPACE}"

restore_good_release() {
  if [ -f "${RELEASE_GOOD}" ]; then
    cp "${RELEASE_GOOD}" "${RELEASE_ENV}"
  fi
}

# ---------------------------------------------------------------------------
# Step 3: pull
# ---------------------------------------------------------------------------
log "Pulling images for tag ${IMAGE_TAG}..."
if ! compose pull ${APP_SERVICES}; then
  restore_good_release
  die "docker compose pull failed for IMAGE_TAG=${IMAGE_TAG}; release unchanged"
fi

# ---------------------------------------------------------------------------
# Step 4: migrate (before any running container is touched)
# ---------------------------------------------------------------------------
log "Applying database migrations with ${IMAGE_TAG}..."
if ! compose --profile tools run --rm aquify-migrate; then
  restore_good_release
  die "Database migration failed; running containers were NOT changed. Fix the migration and re-run."
fi

# ---------------------------------------------------------------------------
# Step 5: swap application containers
# ---------------------------------------------------------------------------
log "Recreating application services: ${APP_SERVICES}"
if ! compose up -d --no-deps --remove-orphans ${APP_SERVICES}; then
  log "docker compose up failed — running rollback.sh"
  "${SCRIPT_DIR}/rollback.sh" || log "CRITICAL: rollback.sh failed; manual intervention required"
  die "Deploy failed; rollback attempted"
fi

# ---------------------------------------------------------------------------
# Step 6: verify
# ---------------------------------------------------------------------------
if ! wait_for_ready; then
  log "Readiness failed after ${HEALTH_TIMEOUT_SECONDS}s — running rollback.sh"
  "${SCRIPT_DIR}/rollback.sh" || log "CRITICAL: rollback.sh failed; manual intervention required"
  die "Deploy failed: API not ready. Rollback to ${OLD_TAG} initiated."
fi
if ! worker_running; then
  log "Worker is not running — running rollback.sh"
  "${SCRIPT_DIR}/rollback.sh" || log "CRITICAL: rollback.sh failed; manual intervention required"
  die "Deploy failed: worker not running. Rollback to ${OLD_TAG} initiated."
fi

# Only now is this release the rollback target.
cp "${RELEASE_ENV}" "${RELEASE_GOOD}"
log "====== PROMOTE SUCCESS: IMAGE_TAG=${IMAGE_TAG} (was ${OLD_TAG}) ======"
