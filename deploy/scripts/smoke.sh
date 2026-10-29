#!/usr/bin/env bash
# smoke.sh — Post-deploy smoke tests for the DARE platform.
#
# Verifies the most critical API endpoints respond correctly after a deploy.
# Exits non-zero on any failure; intended to be run from CI after a deploy.
#
# Usage:
#   API_BASE_URL=https://api.dare.club \
#   SMOKE_AUTH_TOKEN=<bearer_token> \
#   SMOKE_ORG_ID=<org_id> \
#     smoke.sh
#
# Required environment:
#   API_BASE_URL        — base URL of the API (no trailing slash)
#   SMOKE_AUTH_TOKEN    — bearer token for authenticated endpoints
#   SMOKE_ORG_ID        — org ID for scoped endpoints

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
SMOKE_AUTH_TOKEN="${SMOKE_AUTH_TOKEN:-}"
SMOKE_ORG_ID="${SMOKE_ORG_ID:-}"
SMOKE_LOG="/var/log/dare/smoke.log"
PASS=0
FAIL=0

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() {
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "[${ts}] $*"
  mkdir -p "$(dirname "${SMOKE_LOG}")" 2>/dev/null || true
  echo "[${ts}] $*" >> "${SMOKE_LOG}" 2>/dev/null || true
}

check() {
  local name="$1"
  local expected_status="$2"
  local url="$3"
  shift 3
  local extra_args=("$@")

  local actual_status
  actual_status=$(curl -s -o /dev/null -w "%{http_code}" \
    --max-time 10 \
    "${extra_args[@]+"${extra_args[@]}"}" \
    "${url}" 2>/dev/null || echo "000")

  if [ "${actual_status}" = "${expected_status}" ]; then
    log "PASS [${name}]: GET ${url} -> HTTP ${actual_status}"
    PASS=$(( PASS + 1 ))
  else
    log "FAIL [${name}]: GET ${url} -> expected HTTP ${expected_status}, got ${actual_status}"
    FAIL=$(( FAIL + 1 ))
  fi
}

check_body() {
  local name="$1"
  local url="$2"
  local expected_fragment="$3"
  shift 3
  local extra_args=("$@")

  local body
  body=$(curl -s --max-time 10 \
    "${extra_args[@]+"${extra_args[@]}"}" \
    "${url}" 2>/dev/null || echo "")

  if echo "${body}" | grep -q "${expected_fragment}"; then
    log "PASS [${name}]: ${url} body contains '${expected_fragment}'"
    PASS=$(( PASS + 1 ))
  else
    log "FAIL [${name}]: ${url} body missing '${expected_fragment}'"
    FAIL=$(( FAIL + 1 ))
  fi
}

# curl args for authenticated checks; only used when both values are set.
AUTHED=(-H "Authorization: Bearer ${SMOKE_AUTH_TOKEN}" -H "X-Org-Id: ${SMOKE_ORG_ID}")

# ---------------------------------------------------------------------------
# Smoke tests
# ---------------------------------------------------------------------------

log "====== SMOKE TESTS: ${API_BASE_URL} ======"

# Public health endpoints
check "liveness"          "200" "${API_BASE_URL}/api/v1/health/live"
check "readiness"         "200" "${API_BASE_URL}/api/v1/health/ready"
check_body "liveness-ok"  "${API_BASE_URL}/api/v1/health/live" '"status"'

# Metrics endpoint (may be 200 or 404 depending on firewall; accept either)
METRICS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "${API_BASE_URL}/metrics" || echo "000")
if [ "${METRICS_STATUS}" = "200" ] || [ "${METRICS_STATUS}" = "403" ] || [ "${METRICS_STATUS}" = "404" ]; then
  log "PASS [metrics-reachable]: /metrics -> HTTP ${METRICS_STATUS} (expected 200/403/404)"
  PASS=$(( PASS + 1 ))
else
  log "FAIL [metrics-reachable]: /metrics -> HTTP ${METRICS_STATUS}"
  FAIL=$(( FAIL + 1 ))
fi

# Unauthenticated endpoints return 401
check "unauth-households" "401" "${API_BASE_URL}/api/v1/members/households"
check "unauth-swimmers"   "401" "${API_BASE_URL}/api/v1/members/swimmers"

if [ -n "${SMOKE_AUTH_TOKEN}" ] && [ -n "${SMOKE_ORG_ID}" ]; then
  # Auth session check
  check "session"         "200" "${API_BASE_URL}/api/v1/auth/session" \
    "${AUTHED[@]}"

  # Core member data
  check "households"      "200" "${API_BASE_URL}/api/v1/members/households" \
    "${AUTHED[@]}"

  check "swimmers"        "200" "${API_BASE_URL}/api/v1/members/swimmers" \
    "${AUTHED[@]}"

  check "groups"          "200" "${API_BASE_URL}/api/v1/groups" \
    "${AUTHED[@]}"

  # Meet management
  check "meets"           "200" "${API_BASE_URL}/api/v1/meets" \
    "${AUTHED[@]}"

  # Billing
  check "invoices"        "200" "${API_BASE_URL}/api/v1/invoices" \
    "${AUTHED[@]}"

  # Reporting
  check "report-membership" "200" \
    "${API_BASE_URL}/api/v1/reports/membership-status" \
    "${AUTHED[@]}"
else
  log "SKIP authenticated checks (SMOKE_AUTH_TOKEN or SMOKE_ORG_ID not set)"
fi

# ---------------------------------------------------------------------------
# Results
# ---------------------------------------------------------------------------

log "====== SMOKE RESULTS: PASS=${PASS} FAIL=${FAIL} ======"

if [ "${FAIL}" -gt 0 ]; then
  log "Smoke tests FAILED with ${FAIL} failure(s)"
  exit 1
fi

log "All smoke tests passed."
exit 0
