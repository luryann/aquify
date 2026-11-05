#!/usr/bin/env bash
# lib.sh — helpers shared by promote.sh, rollback.sh (and usable by restore.sh).
# Source this file; do not execute it.
#
# Two env files, deliberately separate:
#   .env.production  secrets and static config. Owned root:aquify, mode 640. The
#                    deploy scripts only ever READ it (it feeds both compose
#                    variable interpolation and the api/worker env_file).
#   release.env      IMAGE_TAG + GHCR_NAMESPACE only. Owned by the deploy user;
#                    this is the one file promote/rollback rewrite.
# Compose does not interpolate ${VARS} from a service's env_file, so every compose
# call must pass both files with --env-file. Always go through compose() below.

DEPLOY_DIR="${DEPLOY_DIR:-/opt/aquify}"
SECRETS_ENV="${SECRETS_ENV:-${DEPLOY_DIR}/.env.production}"
RELEASE_ENV="${RELEASE_ENV:-${DEPLOY_DIR}/release.env}"
RELEASE_GOOD="${RELEASE_ENV}.good"   # last release that passed its health gate
RELEASE_PREV="${RELEASE_ENV}.prev"   # rollback target recorded at promote time
COMPOSE_FILE="${COMPOSE_FILE:-${DEPLOY_DIR}/docker-compose.production.yml}"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/api/v1/health/ready}"
HEALTH_TIMEOUT_SECONDS="${HEALTH_TIMEOUT_SECONDS:-90}"
HEALTH_RETRY_INTERVAL="${HEALTH_RETRY_INTERVAL:-5}"
DEPLOY_LOG="${DEPLOY_LOG:-/var/log/aquify/deploys.log}"
LOCK_DIR="${LOCK_DIR:-${DEPLOY_DIR}/.deploy.lock}"
APP_SERVICES="aquify-api aquify-admin aquify-worker"

# Logs go to stderr (and the log file) so a function whose stdout is captured with
# $(...) returns only its value; backup.sh once captured log lines as a file path.
log() {
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "[${ts}] $*" | tee -a "${DEPLOY_LOG}" >&2
}

# envget <file> <KEY>: value of KEY from an env file ("" when absent). Env files are
# parsed, never sourced, so a malformed value cannot run code.
envget() {
  local line v
  line="$(grep -E "^[[:space:]]*$2=" "$1" 2>/dev/null | tail -n 1)" || true
  v="${line#*=}"
  v="${v%$'\r'}"
  v="${v#\"}"; v="${v%\"}"; v="${v#\'}"; v="${v%\'}"
  printf '%s' "${v}"
}

die() {
  log "ERROR: $*"
  exit 1
}

ensure_log_dir() {
  mkdir -p "$(dirname "${DEPLOY_LOG}")"
  touch "${DEPLOY_LOG}"
}

# compose <args...>: docker compose with both env files and the production file.
compose() {
  docker compose \
    --project-directory "${DEPLOY_DIR}" \
    --env-file "${SECRETS_ENV}" \
    --env-file "${RELEASE_ENV}" \
    -f "${COMPOSE_FILE}" "$@"
}

# release_value <file> <KEY>: value of KEY in an env file ("" when absent).
release_value() {
  local v
  v="$(grep "^$2=" "$1" 2>/dev/null | tail -n 1 | cut -d= -f2-)" || true
  printf '%s' "${v}"
}

# write_release <file> <tag> <namespace>: whole-file write, nothing else in it.
write_release() {
  printf 'IMAGE_TAG=%s\nGHCR_NAMESPACE=%s\n' "$2" "$3" > "$1"
}

# One deploy/rollback at a time. mkdir is atomic and portable; a stale lock whose
# recorded pid is dead is taken over so a crashed deploy does not wedge the host.
acquire_lock() {
  # promote.sh runs rollback.sh as a child while holding the lock.
  if [ "${AQUIFY_DEPLOY_LOCK_HELD:-}" = "1" ]; then
    return 0
  fi
  if ! mkdir "${LOCK_DIR}" 2>/dev/null; then
    local holder
    holder="$(cat "${LOCK_DIR}/pid" 2>/dev/null || true)"
    if [ -n "${holder}" ] && kill -0 "${holder}" 2>/dev/null; then
      die "Another deploy/rollback is running (pid ${holder}); refusing to start"
    fi
    log "Taking over stale deploy lock (previous holder ${holder:-unknown} is gone)"
    rm -f "${LOCK_DIR}/pid"
    rmdir "${LOCK_DIR}" 2>/dev/null || true
    mkdir "${LOCK_DIR}" 2>/dev/null || die "Could not acquire deploy lock ${LOCK_DIR}"
  fi
  echo "$$" > "${LOCK_DIR}/pid"
  export AQUIFY_DEPLOY_LOCK_HELD=1
  trap release_lock EXIT
}

release_lock() {
  rm -f "${LOCK_DIR}/pid"
  rmdir "${LOCK_DIR}" 2>/dev/null || true
}

# Ready means the API's dependency checks pass (not merely that the process is up).
wait_for_ready() {
  local retries=$(( HEALTH_TIMEOUT_SECONDS / HEALTH_RETRY_INTERVAL ))
  [ "${retries}" -ge 1 ] || retries=1
  log "Waiting for readiness at ${HEALTH_URL} (timeout ${HEALTH_TIMEOUT_SECONDS}s)..."
  local i status
  for i in $(seq 1 "${retries}"); do
    status=$(curl -s -o /dev/null -w "%{http_code}" --max-time 4 "${HEALTH_URL}" 2>/dev/null || echo "000")
    if [ "${status}" = "200" ]; then
      log "Readiness passed (attempt ${i}/${retries})"
      return 0
    fi
    log "Attempt ${i}/${retries}: HTTP ${status} — retrying in ${HEALTH_RETRY_INTERVAL}s..."
    sleep "${HEALTH_RETRY_INTERVAL}"
  done
  return 1
}

# The worker has no HTTP endpoint: require that its container is running.
worker_running() {
  local ids
  ids="$(compose ps --status running -q aquify-worker 2>/dev/null || true)"
  [ -n "${ids}" ]
}
