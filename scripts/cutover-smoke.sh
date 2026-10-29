#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
ORG_ID="${ORG_ID:-}"

if [[ -z "${API_BASE_URL}" || -z "${AUTH_TOKEN}" || -z "${ORG_ID}" ]]; then
  echo "API_BASE_URL, AUTH_TOKEN, and ORG_ID are required." >&2
  echo "Example: make cutover-smoke API_BASE_URL=https://api.example.com AUTH_TOKEN=... ORG_ID=org-prod" >&2
  exit 1
fi

BODY_FILE="$(mktemp)"
trap 'rm -f "${BODY_FILE}"' EXIT

check() {
  local name="$1"
  local url="$2"
  local expected="${3:-200}"
  local headers=("${@:4}")

  local code
  code="$(curl -sS -o "${BODY_FILE}" -w "%{http_code}" ${headers[@]+"${headers[@]}"} "${url}")"
  if [[ "${code}" != "${expected}" ]]; then
    echo "FAIL ${name}: expected ${expected}, got ${code}" >&2
    cat "${BODY_FILE}" >&2
    exit 1
  fi
  echo "PASS ${name}"
}

check "health live" "${API_BASE_URL}/api/v1/health/live"
check "health ready" "${API_BASE_URL}/api/v1/health/ready"
check "openapi" "${API_BASE_URL}/api/v1/openapi"
check \
  "authz ping" \
  "${API_BASE_URL}/api/v1/authz/ping" \
  200 \
  -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "X-Org-Id: ${ORG_ID}"
