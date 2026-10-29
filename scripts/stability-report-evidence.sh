#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/evidence-api.sh"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EVIDENCE_DIR="${EVIDENCE_DIR:-${ROOT_DIR}/evidence/stability-report-${STAMP}}"
API_BASE_URL="${API_BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
ORG_ID="${ORG_ID:-}"
STABILITY_WINDOW_LABEL="${STABILITY_WINDOW_LABEL:-post-launch}"
NO_OPEN_P0_P1_ATTESTATION="${NO_OPEN_P0_P1_ATTESTATION:-false}"
SUPPORT_INCIDENTS_FILE="${SUPPORT_INCIDENTS_FILE:-}"
PROMETHEUS_URL="${PROMETHEUS_URL:-}"
REQUIRE_SUPPORT_INCIDENTS_FILE="${REQUIRE_SUPPORT_INCIDENTS_FILE:-true}"

if [[ -z "${API_BASE_URL}" || -z "${AUTH_TOKEN}" || -z "${ORG_ID}" ]]; then
  echo "API_BASE_URL, AUTH_TOKEN, and ORG_ID are required." >&2
  exit 1
fi

if [[ "${NO_OPEN_P0_P1_ATTESTATION}" != "true" ]]; then
  echo "NO_OPEN_P0_P1_ATTESTATION=true is required for a passing stability report." >&2
  exit 1
fi

if [[ "${REQUIRE_SUPPORT_INCIDENTS_FILE}" == "true" && -z "${SUPPORT_INCIDENTS_FILE}" ]]; then
  echo "SUPPORT_INCIDENTS_FILE is required when REQUIRE_SUPPORT_INCIDENTS_FILE=true." >&2
  exit 1
fi

PYTHON_BIN="$(evidence_python "summarize stability evidence")"

mkdir -p "${EVIDENCE_DIR}"

api() {
  local name="$1"
  evidence_request GET "$2" "${EVIDENCE_DIR}/${name}.json"
}

cat > "${EVIDENCE_DIR}/metadata.json" <<EOF
{
  "generated_at": "${STAMP}",
  "api_base_url": "${API_BASE_URL}",
  "org_id": "${ORG_ID}",
  "stability_window_label": "${STABILITY_WINDOW_LABEL}",
  "no_open_p0_p1_attestation": "${NO_OPEN_P0_P1_ATTESTATION}",
  "support_incidents_file": "${SUPPORT_INCIDENTS_FILE}",
  "prometheus_url": "${PROMETHEUS_URL}",
  "require_support_incidents_file": "${REQUIRE_SUPPORT_INCIDENTS_FILE}"
}
EOF

api "health-live" "/api/v1/health/live"
api "health-ready" "/api/v1/health/ready"
api "jobs-queues" "/api/v1/jobs/queues"
api "jobs-metrics" "/api/v1/jobs/metrics"
api "jobs-dlq" "/api/v1/jobs/dlq"
api "stripe-reconciliation" "/api/v1/stripe/reconciliation"
api "stripe-webhook-events" "/api/v1/stripe/webhook-events"
api "audit-events" "/api/v1/audit-events?limit=50"

if [[ -n "${SUPPORT_INCIDENTS_FILE}" ]]; then
  if [[ ! -f "${SUPPORT_INCIDENTS_FILE}" ]]; then
    echo "SUPPORT_INCIDENTS_FILE does not exist: ${SUPPORT_INCIDENTS_FILE}" >&2
    exit 1
  fi
  cp "${SUPPORT_INCIDENTS_FILE}" "${EVIDENCE_DIR}/support-incidents-export"
else
  cat > "${EVIDENCE_DIR}/support-incidents.md" <<EOF
# Support Incidents

No external support incident export was supplied.

Operator attestation: no open P0/P1 incidents for ${STABILITY_WINDOW_LABEL}.
EOF
fi

if [[ -n "${PROMETHEUS_URL}" ]]; then
  metrics_dir="${EVIDENCE_DIR}/prometheus"
  mkdir -p "${metrics_dir}"
  prometheus_query() {
    curl -sS -G "${PROMETHEUS_URL}/api/v1/query" --data-urlencode "query=$2" -o "${metrics_dir}/$1.json"
  }
  prometheus_query api-5xx-rate 'sum(rate(http_requests_total{status=~"5.."}[15m]))'
  prometheus_query api-p95-latency 'histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[15m])) by (le))'
  prometheus_query worker-queue-depth 'sum(aquify_worker_queue_depth)'
fi

"${PYTHON_BIN}" - "${EVIDENCE_DIR}" "${STABILITY_WINDOW_LABEL}" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
window = sys.argv[2]
failures = []

def read_status(name):
    status_path = root / f"{name}.json.status"
    try:
        return status_path.read_text(encoding="utf-8").strip()
    except FileNotFoundError:
        return ""

def load(name):
    with (root / f"{name}.json").open("r", encoding="utf-8") as handle:
        return json.load(handle)

for name in [
    "health-live",
    "health-ready",
    "jobs-queues",
    "jobs-metrics",
    "jobs-dlq",
    "stripe-reconciliation",
    "stripe-webhook-events",
    "audit-events",
]:
    status = read_status(name)
    if not status.startswith("2"):
        failures.append(f"{name}: HTTP status {status or 'missing'}")

dlq = load("jobs-dlq")
dlq_count = int(dlq.get("count", 0))
if dlq_count != 0:
    failures.append(f"jobs-dlq: {dlq_count} dead-letter tasks are present")

reconciliation = load("stripe-reconciliation").get("reconciliation", {})
if reconciliation.get("alert_triggered") is True:
    failures.append("stripe-reconciliation: alert_triggered is true")

webhook_count = int(load("stripe-webhook-events").get("count", 0))
audit_count = len(load("audit-events").get("events", []))

with (root / "stability-report.md").open("w", encoding="utf-8") as handle:
    handle.write("# Post-Launch Stability Report\n\n")
    handle.write(f"- Window: {window}\n")
    handle.write("- Health live: health-live.json\n")
    handle.write("- Health ready: health-ready.json\n")
    handle.write("- Queue stats: jobs-queues.json\n")
    handle.write("- Job metrics: jobs-metrics.json\n")
    handle.write(f"- Dead-letter queue count: {dlq_count}\n")
    handle.write("- Stripe reconciliation: stripe-reconciliation.json\n")
    handle.write(f"- Stripe webhook events observed: {webhook_count}\n")
    handle.write(f"- Recent audit events sampled: {audit_count}\n")
    handle.write("- Support incident evidence: support-incidents.md or supplied support incident export\n")
    handle.write("- P0/P1 attestation: true\n")
    if (root / "prometheus").exists():
        handle.write("- Prometheus snapshots: prometheus/\n")
    if failures:
        handle.write("\n## Failures\n\n")
        for failure in failures:
            handle.write(f"- {failure}\n")
    else:
        handle.write("\nResult: PASS\n")

if failures:
    for failure in failures:
        print(failure, file=sys.stderr)
    sys.exit(1)
PY

echo "Stability report evidence written to ${EVIDENCE_DIR}"
