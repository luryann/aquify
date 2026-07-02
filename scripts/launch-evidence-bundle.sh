#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EVIDENCE_DIR="${EVIDENCE_DIR:-${ROOT_DIR}/evidence/launch-${STAMP}}"
INPUT_DIR="${INPUT_DIR:-}"
SOURCE_COUNTS="${SOURCE_COUNTS:-}"
SOURCE_BILLING_TOTAL_CENTS="${SOURCE_BILLING_TOTAL_CENTS:-}"
ORG_ID="${ORG_ID:-}"
ACTOR_ID="${ACTOR_ID:-launch-evidence}"
API_BASE_URL="${API_BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
RUN_CERTIFICATION="${RUN_CERTIFICATION:-true}"
RUN_PROVIDER_VALIDATION="${RUN_PROVIDER_VALIDATION:-true}"
RUN_PARALLEL_MEET_EVIDENCE="${RUN_PARALLEL_MEET_EVIDENCE:-false}"
RUN_STABILITY_REPORT="${RUN_STABILITY_REPORT:-false}"
EXPECT_STRIPE_WEBHOOK_EVENT_ID="${EXPECT_STRIPE_WEBHOOK_EVENT_ID:-}"
CONFIRM_STRIPE_REFUND="${CONFIRM_STRIPE_REFUND:-false}"
REFUND_INVOICE_ID="${REFUND_INVOICE_ID:-}"
REFUND_PAYMENT_ID="${REFUND_PAYMENT_ID:-}"
REFUND_AMOUNT_CENTS="${REFUND_AMOUNT_CENTS:-}"
POSTMARK_CAMPAIGN_ID="${POSTMARK_CAMPAIGN_ID:-}"
CONFIRM_POSTMARK_SEND="${CONFIRM_POSTMARK_SEND:-false}"
REQUIRE_COMPLETE_PROVIDER_VALIDATION="${REQUIRE_COMPLETE_PROVIDER_VALIDATION:-true}"
MEET_IDS="${MEET_IDS:-}"
NO_MANUAL_PATCHING_ATTESTATION="${NO_MANUAL_PATCHING_ATTESTATION:-false}"
STABILITY_WINDOW_LABEL="${STABILITY_WINDOW_LABEL:-post-launch}"
NO_OPEN_P0_P1_ATTESTATION="${NO_OPEN_P0_P1_ATTESTATION:-false}"
SUPPORT_INCIDENTS_FILE="${SUPPORT_INCIDENTS_FILE:-}"
PROMETHEUS_URL="${PROMETHEUS_URL:-}"
REQUIRE_SUPPORT_INCIDENTS_FILE="${REQUIRE_SUPPORT_INCIDENTS_FILE:-true}"

usage() {
  cat >&2 <<'USAGE'
Required:
  INPUT_DIR=/absolute/path/to/anonymized-or-final-export
  ORG_ID=<staging-or-prod-org-id>

Optional:
  SOURCE_COUNTS=/absolute/path/to/source-counts.json
  SOURCE_BILLING_TOTAL_CENTS=<integer cents>
  API_BASE_URL=https://api.staging.example.com
  AUTH_TOKEN=<admin-or-owner-token>
  EVIDENCE_DIR=/absolute/path/to/evidence-dir
  RUN_CERTIFICATION=false
  RUN_PROVIDER_VALIDATION=false
  EXPECT_STRIPE_WEBHOOK_EVENT_ID=evt_...
  CONFIRM_STRIPE_REFUND=true
  REFUND_INVOICE_ID=inv_...
  REFUND_PAYMENT_ID=pay_...
  REFUND_AMOUNT_CENTS=100
  POSTMARK_CAMPAIGN_ID=campaign_...
  CONFIRM_POSTMARK_SEND=true
  REQUIRE_COMPLETE_PROVIDER_VALIDATION=true
  RUN_PARALLEL_MEET_EVIDENCE=true
  RUN_STABILITY_REPORT=true
  MEET_IDS=meet_1,meet_2
  NO_MANUAL_PATCHING_ATTESTATION=true
  STABILITY_WINDOW_LABEL=+24h
  NO_OPEN_P0_P1_ATTESTATION=true
  SUPPORT_INCIDENTS_FILE=/absolute/path/to/support-incidents.csv
  PROMETHEUS_URL=https://prometheus.example.com
  REQUIRE_SUPPORT_INCIDENTS_FILE=true

source-counts.json example:
  {
    "households.csv": 125,
    "swimmers.csv": 210,
    "groups.csv": 12,
    "historical_best_times.csv": 6400,
    "open_invoices.csv": 58,
    "payment_history.csv": 410
  }
USAGE
}

if [[ -z "${INPUT_DIR}" || -z "${ORG_ID}" ]]; then
  usage
  exit 1
fi

mkdir -p "${EVIDENCE_DIR}"

run_and_capture() {
  local name="$1"
  shift
  echo "== ${name} =="
  {
    echo "$ ${*}"
    "$@"
  } >"${EVIDENCE_DIR}/${name}.log" 2>&1
}

write_metadata() {
  json_escape() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//$'\n'/\\n}"
    printf '%s' "${value}"
  }

  cat >"${EVIDENCE_DIR}/metadata.json" <<EOF
{
  "generated_at": "$(json_escape "${STAMP}")",
  "input_dir": "$(json_escape "${INPUT_DIR}")",
  "org_id": "$(json_escape "${ORG_ID}")",
  "actor_id": "$(json_escape "${ACTOR_ID}")",
  "api_base_url": "$(json_escape "${API_BASE_URL}")",
  "source_counts": "$(json_escape "${SOURCE_COUNTS}")",
  "source_billing_total_cents": "$(json_escape "${SOURCE_BILLING_TOTAL_CENTS}")",
  "run_certification": "$(json_escape "${RUN_CERTIFICATION}")",
  "run_provider_validation": "$(json_escape "${RUN_PROVIDER_VALIDATION}")",
  "expect_stripe_webhook_event_id": "$(json_escape "${EXPECT_STRIPE_WEBHOOK_EVENT_ID}")",
  "confirm_stripe_refund": "$(json_escape "${CONFIRM_STRIPE_REFUND}")",
  "refund_invoice_id": "$(json_escape "${REFUND_INVOICE_ID}")",
  "refund_payment_id": "$(json_escape "${REFUND_PAYMENT_ID}")",
  "refund_amount_cents": "$(json_escape "${REFUND_AMOUNT_CENTS}")",
  "postmark_campaign_id": "$(json_escape "${POSTMARK_CAMPAIGN_ID}")",
  "confirm_postmark_send": "$(json_escape "${CONFIRM_POSTMARK_SEND}")",
  "require_complete_provider_validation": "$(json_escape "${REQUIRE_COMPLETE_PROVIDER_VALIDATION}")",
  "run_parallel_meet_evidence": "$(json_escape "${RUN_PARALLEL_MEET_EVIDENCE}")",
  "meet_ids": "$(json_escape "${MEET_IDS}")",
  "no_manual_patching_attestation": "$(json_escape "${NO_MANUAL_PATCHING_ATTESTATION}")",
  "run_stability_report": "$(json_escape "${RUN_STABILITY_REPORT}")",
  "stability_window_label": "$(json_escape "${STABILITY_WINDOW_LABEL}")",
  "no_open_p0_p1_attestation": "$(json_escape "${NO_OPEN_P0_P1_ATTESTATION}")",
  "support_incidents_file": "$(json_escape "${SUPPORT_INCIDENTS_FILE}")",
  "prometheus_url": "$(json_escape "${PROMETHEUS_URL}")",
  "require_support_incidents_file": "$(json_escape "${REQUIRE_SUPPORT_INCIDENTS_FILE}")"
}
EOF
}

write_metadata

MIGRATE_ARGS=(
  "run" "./cmd/migrate" "dry-run"
  "-input-dir" "${INPUT_DIR}"
  "-report" "${EVIDENCE_DIR}/migration-report.json"
  "-remediation-log" "${EVIDENCE_DIR}/remediation-log.md"
  "-org-id" "${ORG_ID}"
  "-actor-id" "${ACTOR_ID}"
)

if [[ -n "${SOURCE_COUNTS}" ]]; then
  MIGRATE_ARGS+=("-source-counts" "${SOURCE_COUNTS}")
fi
if [[ -n "${SOURCE_BILLING_TOTAL_CENTS}" ]]; then
  MIGRATE_ARGS+=("-source-billing-total-cents" "${SOURCE_BILLING_TOTAL_CENTS}")
fi

(
  cd "${ROOT_DIR}/server"
  run_and_capture "migration-dry-run" go "${MIGRATE_ARGS[@]}"
)

if [[ "${RUN_CERTIFICATION}" == "true" ]]; then
  run_and_capture "cutover-certification" "${ROOT_DIR}/scripts/cutover-certification.sh"
fi

if [[ -n "${API_BASE_URL}" && -n "${AUTH_TOKEN}" ]]; then
  (
    export API_BASE_URL AUTH_TOKEN ORG_ID
    run_and_capture "cutover-smoke" "${ROOT_DIR}/scripts/cutover-smoke.sh"
    run_and_capture "cutover-billing-check" "${ROOT_DIR}/scripts/cutover-billing-check.sh"
    if [[ "${RUN_PROVIDER_VALIDATION}" == "true" ]]; then
      PROVIDER_EVIDENCE_DIR="${EVIDENCE_DIR}/provider-validation"
      mkdir -p "${PROVIDER_EVIDENCE_DIR}"
      run_and_capture "provider-validation" env \
        EVIDENCE_DIR="${PROVIDER_EVIDENCE_DIR}" \
        EXPECT_STRIPE_WEBHOOK_EVENT_ID="${EXPECT_STRIPE_WEBHOOK_EVENT_ID}" \
        CONFIRM_STRIPE_REFUND="${CONFIRM_STRIPE_REFUND}" \
        REFUND_INVOICE_ID="${REFUND_INVOICE_ID}" \
        REFUND_PAYMENT_ID="${REFUND_PAYMENT_ID}" \
        REFUND_AMOUNT_CENTS="${REFUND_AMOUNT_CENTS}" \
        POSTMARK_CAMPAIGN_ID="${POSTMARK_CAMPAIGN_ID}" \
        CONFIRM_POSTMARK_SEND="${CONFIRM_POSTMARK_SEND}" \
        REQUIRE_COMPLETE_PROVIDER_VALIDATION="${REQUIRE_COMPLETE_PROVIDER_VALIDATION}" \
        bash "${ROOT_DIR}/scripts/provider-validation-evidence.sh"
    fi
    if [[ "${RUN_PARALLEL_MEET_EVIDENCE}" == "true" ]]; then
      PARALLEL_EVIDENCE_DIR="${EVIDENCE_DIR}/parallel-meet-cycles"
      mkdir -p "${PARALLEL_EVIDENCE_DIR}"
      run_and_capture "parallel-meet-cycles" env EVIDENCE_DIR="${PARALLEL_EVIDENCE_DIR}" MEET_IDS="${MEET_IDS}" NO_MANUAL_PATCHING_ATTESTATION="${NO_MANUAL_PATCHING_ATTESTATION}" bash "${ROOT_DIR}/scripts/parallel-meet-cycle-evidence.sh"
    fi
    if [[ "${RUN_STABILITY_REPORT}" == "true" ]]; then
      STABILITY_EVIDENCE_DIR="${EVIDENCE_DIR}/stability-report"
      mkdir -p "${STABILITY_EVIDENCE_DIR}"
      run_and_capture "stability-report" env \
        EVIDENCE_DIR="${STABILITY_EVIDENCE_DIR}" \
        STABILITY_WINDOW_LABEL="${STABILITY_WINDOW_LABEL}" \
        NO_OPEN_P0_P1_ATTESTATION="${NO_OPEN_P0_P1_ATTESTATION}" \
        SUPPORT_INCIDENTS_FILE="${SUPPORT_INCIDENTS_FILE}" \
        PROMETHEUS_URL="${PROMETHEUS_URL}" \
        REQUIRE_SUPPORT_INCIDENTS_FILE="${REQUIRE_SUPPORT_INCIDENTS_FILE}" \
        bash "${ROOT_DIR}/scripts/stability-report-evidence.sh"
    fi
  )
else
  cat >"${EVIDENCE_DIR}/provider-validation-required.md" <<'EOF'
# Provider Validation Required

`API_BASE_URL` and `AUTH_TOKEN` were not supplied, so live API smoke,
Stripe reconciliation, webhook, refund, balance-fetch, and Postmark delivery
validation were not executed by this evidence bundle.

Re-run this script against staging or production with live-equivalent provider
credentials and archive the resulting `cutover-smoke.log`,
`cutover-billing-check.log`, Stripe provider event IDs, and Postmark message IDs.
EOF
fi

cat >"${EVIDENCE_DIR}/manifest.txt" <<EOF
Launch evidence bundle generated at ${STAMP}

Required review artifacts:
- metadata.json
- migration-dry-run.log
- migration-report.json
- remediation-log.md
- cutover-certification.log (when RUN_CERTIFICATION=true)
- cutover-smoke.log (when API_BASE_URL and AUTH_TOKEN are supplied)
- cutover-billing-check.log (when API_BASE_URL and AUTH_TOKEN are supplied)
- provider-validation/ and provider-validation.log (when RUN_PROVIDER_VALIDATION=true)
- parallel-meet-cycles/ and parallel-meet-cycles.log (when RUN_PARALLEL_MEET_EVIDENCE=true)
- stability-report/ and stability-report.log (when RUN_STABILITY_REPORT=true)
- provider-validation-required.md (when live provider validation was skipped)
EOF

echo "Launch evidence bundle written to ${EVIDENCE_DIR}"
