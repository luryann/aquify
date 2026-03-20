#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
ORG_ID="${ORG_ID:-}"

if [[ -z "${API_BASE_URL}" || -z "${AUTH_TOKEN}" || -z "${ORG_ID}" ]]; then
  echo "API_BASE_URL, AUTH_TOKEN, and ORG_ID are required." >&2
  echo "Example: make cutover-billing-check API_BASE_URL=https://api.example.com AUTH_TOKEN=... ORG_ID=org-prod" >&2
  exit 1
fi

curl -sS "${API_BASE_URL}/api/v1/stripe/reconciliation" \
  -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "X-Org-Id: ${ORG_ID}" \
  -H "Accept: application/json"
echo
