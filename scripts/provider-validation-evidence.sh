#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EVIDENCE_DIR="${EVIDENCE_DIR:-${ROOT_DIR}/evidence/provider-validation-${STAMP}}"
API_BASE_URL="${API_BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
ORG_ID="${ORG_ID:-}"

EXPECT_STRIPE_WEBHOOK_EVENT_ID="${EXPECT_STRIPE_WEBHOOK_EVENT_ID:-}"
CONFIRM_STRIPE_REFUND="${CONFIRM_STRIPE_REFUND:-false}"
REFUND_INVOICE_ID="${REFUND_INVOICE_ID:-}"
REFUND_PAYMENT_ID="${REFUND_PAYMENT_ID:-}"
REFUND_AMOUNT_CENTS="${REFUND_AMOUNT_CENTS:-}"

POSTMARK_CAMPAIGN_ID="${POSTMARK_CAMPAIGN_ID:-}"
CONFIRM_POSTMARK_SEND="${CONFIRM_POSTMARK_SEND:-false}"
REQUIRE_COMPLETE_PROVIDER_VALIDATION="${REQUIRE_COMPLETE_PROVIDER_VALIDATION:-true}"

if [[ -z "${API_BASE_URL}" || -z "${AUTH_TOKEN}" || -z "${ORG_ID}" ]]; then
  echo "API_BASE_URL, AUTH_TOKEN, and ORG_ID are required." >&2
  exit 1
fi

if [[ "${REQUIRE_COMPLETE_PROVIDER_VALIDATION}" == "true" ]]; then
  if [[ -z "${EXPECT_STRIPE_WEBHOOK_EVENT_ID}" ]]; then
    echo "EXPECT_STRIPE_WEBHOOK_EVENT_ID is required when REQUIRE_COMPLETE_PROVIDER_VALIDATION=true." >&2
    exit 1
  fi
  if [[ "${CONFIRM_STRIPE_REFUND}" != "true" ]]; then
    echo "CONFIRM_STRIPE_REFUND=true is required when REQUIRE_COMPLETE_PROVIDER_VALIDATION=true." >&2
    exit 1
  fi
  if [[ -z "${POSTMARK_CAMPAIGN_ID}" || "${CONFIRM_POSTMARK_SEND}" != "true" ]]; then
    echo "POSTMARK_CAMPAIGN_ID and CONFIRM_POSTMARK_SEND=true are required when REQUIRE_COMPLETE_PROVIDER_VALIDATION=true." >&2
    exit 1
  fi
fi

mkdir -p "${EVIDENCE_DIR}"

api() {
  local method="$1"
  local path="$2"
  local output="$3"
  local body="${4:-}"
  local code
  if [[ -n "${body}" ]]; then
    code="$(curl -sS -o "${output}" -w "%{http_code}" \
      -X "${method}" "${API_BASE_URL}${path}" \
      -H "Authorization: Bearer ${AUTH_TOKEN}" \
      -H "X-Org-Id: ${ORG_ID}" \
      -H "Accept: application/json" \
      -H "Content-Type: application/json" \
      --data "${body}")"
  else
    code="$(curl -sS -o "${output}" -w "%{http_code}" \
      -X "${method}" "${API_BASE_URL}${path}" \
      -H "Authorization: Bearer ${AUTH_TOKEN}" \
      -H "X-Org-Id: ${ORG_ID}" \
      -H "Accept: application/json")"
  fi
  printf '%s' "${code}" > "${output}.status"
  case "${code}" in
    2*) return 0 ;;
    *)
      echo "Request ${method} ${path} failed with HTTP ${code}" >&2
      cat "${output}" >&2 || true
      return 1
      ;;
  esac
}

cat > "${EVIDENCE_DIR}/metadata.json" <<EOF
{
  "generated_at": "${STAMP}",
  "api_base_url": "${API_BASE_URL}",
  "org_id": "${ORG_ID}",
  "expect_stripe_webhook_event_id": "${EXPECT_STRIPE_WEBHOOK_EVENT_ID}",
  "confirm_stripe_refund": "${CONFIRM_STRIPE_REFUND}",
  "refund_invoice_id": "${REFUND_INVOICE_ID}",
  "refund_payment_id": "${REFUND_PAYMENT_ID}",
  "refund_amount_cents": "${REFUND_AMOUNT_CENTS}",
  "postmark_campaign_id": "${POSTMARK_CAMPAIGN_ID}",
  "confirm_postmark_send": "${CONFIRM_POSTMARK_SEND}",
  "require_complete_provider_validation": "${REQUIRE_COMPLETE_PROVIDER_VALIDATION}"
}
EOF

api GET "/api/v1/health/ready" "${EVIDENCE_DIR}/health-ready.json"
api GET "/api/v1/stripe/reconciliation" "${EVIDENCE_DIR}/stripe-reconciliation.json"
if ! grep -Eq '"stripe_reported_cents"[[:space:]]*:' "${EVIDENCE_DIR}/stripe-reconciliation.json"; then
  echo "Stripe reconciliation response did not include stripe_reported_cents." >&2
  exit 1
fi
api GET "/api/v1/stripe/webhook-events" "${EVIDENCE_DIR}/stripe-webhook-events.json"

stripe_webhook_result="not_requested"
if [[ -n "${EXPECT_STRIPE_WEBHOOK_EVENT_ID}" ]]; then
  if grep -Fq "${EXPECT_STRIPE_WEBHOOK_EVENT_ID}" "${EVIDENCE_DIR}/stripe-webhook-events.json"; then
    stripe_webhook_result="found"
  else
    stripe_webhook_result="missing"
    echo "Expected Stripe webhook event ${EXPECT_STRIPE_WEBHOOK_EVENT_ID} was not present." >&2
  fi
fi

stripe_refund_result="not_requested"
if [[ "${CONFIRM_STRIPE_REFUND}" == "true" ]]; then
  if [[ -z "${REFUND_INVOICE_ID}" || -z "${REFUND_PAYMENT_ID}" || -z "${REFUND_AMOUNT_CENTS}" ]]; then
    echo "REFUND_INVOICE_ID, REFUND_PAYMENT_ID, and REFUND_AMOUNT_CENTS are required when CONFIRM_STRIPE_REFUND=true." >&2
    exit 1
  fi
  api POST "/api/v1/invoices/${REFUND_INVOICE_ID}/refunds" "${EVIDENCE_DIR}/stripe-refund.json" \
    "{\"payment_id\":\"${REFUND_PAYMENT_ID}\",\"amount_cents\":${REFUND_AMOUNT_CENTS},\"reason\":\"staging provider validation refund\"}"
  stripe_refund_result="executed"
fi

postmark_result="not_requested"
if [[ -n "${POSTMARK_CAMPAIGN_ID}" ]]; then
  if [[ "${CONFIRM_POSTMARK_SEND}" == "true" ]]; then
    api POST "/api/v1/email-campaigns/${POSTMARK_CAMPAIGN_ID}/send" "${EVIDENCE_DIR}/postmark-send.json" "{}"
    postmark_result="send_executed"
  else
    postmark_result="delivery_check_only"
  fi
  api GET "/api/v1/email-campaigns/${POSTMARK_CAMPAIGN_ID}/deliveries" "${EVIDENCE_DIR}/postmark-deliveries.json"
  if [[ "${CONFIRM_POSTMARK_SEND}" == "true" ]]; then
    if ! grep -Eq '"provider_message_id"[[:space:]]*:[[:space:]]*"[^"]+"' "${EVIDENCE_DIR}/postmark-deliveries.json"; then
      echo "Postmark deliveries response did not include a provider_message_id after send." >&2
      exit 1
    fi
  fi
fi

cat > "${EVIDENCE_DIR}/validation-summary.md" <<EOF
# Provider Validation Evidence

- Generated at: ${STAMP}
- API: ${API_BASE_URL}
- Org: ${ORG_ID}
- Readiness: health-ready.json
- Stripe reconciliation / balance fetch: stripe-reconciliation.json
- Stripe webhook event list: stripe-webhook-events.json
- Expected Stripe webhook event: ${stripe_webhook_result}
- Stripe refund validation: ${stripe_refund_result}
- Postmark delivery validation: ${postmark_result}

Passing criteria:
- Readiness is HTTP 200 and reports provider dependencies ready for the target environment.
- Stripe reconciliation succeeds and includes a live Stripe-reported balance.
- A real Stripe webhook event ID from the staging/provider dashboard is present when EXPECT_STRIPE_WEBHOOK_EVENT_ID is supplied.
- Refund validation is executed with live Stripe test credentials when CONFIRM_STRIPE_REFUND=true.
- Postmark delivery validation includes provider message/delivery records after CONFIRM_POSTMARK_SEND=true.
EOF

if [[ "${stripe_webhook_result}" == "missing" ]]; then
  exit 1
fi
if [[ "${REQUIRE_COMPLETE_PROVIDER_VALIDATION}" == "true" ]]; then
  if [[ "${stripe_webhook_result}" != "found" || "${stripe_refund_result}" != "executed" || "${postmark_result}" != "send_executed" ]]; then
    echo "Complete provider validation did not pass." >&2
    exit 1
  fi
fi

echo "Provider validation evidence written to ${EVIDENCE_DIR}"
