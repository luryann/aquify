#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EVIDENCE_DIR="${EVIDENCE_DIR:-${ROOT_DIR}/evidence/parallel-meet-cycles-${STAMP}}"
API_BASE_URL="${API_BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
ORG_ID="${ORG_ID:-}"
MEET_IDS="${MEET_IDS:-}"
NO_MANUAL_PATCHING_ATTESTATION="${NO_MANUAL_PATCHING_ATTESTATION:-false}"

if [[ -z "${API_BASE_URL}" || -z "${AUTH_TOKEN}" || -z "${ORG_ID}" || -z "${MEET_IDS}" ]]; then
  echo "API_BASE_URL, AUTH_TOKEN, ORG_ID, and MEET_IDS are required." >&2
  echo "MEET_IDS must contain at least two comma-separated meet IDs." >&2
  exit 1
fi

IFS=',' read -r -a meet_ids <<< "${MEET_IDS}"
if [[ "${#meet_ids[@]}" -lt 2 ]]; then
  echo "At least two meet IDs are required to prove two parallel meet cycles." >&2
  exit 1
fi

if [[ "${NO_MANUAL_PATCHING_ATTESTATION}" != "true" ]]; then
  echo "NO_MANUAL_PATCHING_ATTESTATION=true is required." >&2
  exit 1
fi

mkdir -p "${EVIDENCE_DIR}"

PYTHON_BIN="${PYTHON_BIN:-}"
if [[ -z "${PYTHON_BIN}" ]]; then
  if command -v python3 >/dev/null 2>&1; then
    PYTHON_BIN="python3"
  elif command -v python >/dev/null 2>&1; then
    PYTHON_BIN="python"
  else
    echo "python3 or python is required to validate captured meet-cycle JSON." >&2
    exit 1
  fi
fi

api() {
  local path="$1"
  local output="$2"
  local code
  code="$(curl -sS -o "${output}" -w "%{http_code}" \
    "${API_BASE_URL}${path}" \
    -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -H "X-Org-Id: ${ORG_ID}" \
    -H "Accept: application/json")"
  printf '%s' "${code}" > "${output}.status"
  case "${code}" in
    2*) return 0 ;;
    *)
      echo "Request ${path} failed with HTTP ${code}" >&2
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
  "meet_ids": "${MEET_IDS}",
  "no_manual_patching_attestation": "${NO_MANUAL_PATCHING_ATTESTATION}"
}
EOF

for raw_meet_id in "${meet_ids[@]}"; do
  meet_id="$(echo "${raw_meet_id}" | xargs)"
  meet_dir="${EVIDENCE_DIR}/meet-${meet_id}"
  mkdir -p "${meet_dir}"
  api "/api/v1/meets/${meet_id}" "${meet_dir}/meet.json"
  api "/api/v1/meets/${meet_id}/sessions" "${meet_dir}/sessions.json"
  api "/api/v1/meets/${meet_id}/events" "${meet_dir}/events.json"
  api "/api/v1/meets/${meet_id}/commitments" "${meet_dir}/commitments.json"
  api "/api/v1/meets/${meet_id}/entries" "${meet_dir}/entries.json"
  api "/api/v1/meets/${meet_id}/exports" "${meet_dir}/exports.json"
done

"${PYTHON_BIN}" - "$EVIDENCE_DIR" "${meet_ids[@]}" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
meet_ids = [item.strip() for item in sys.argv[2:] if item.strip()]
failures = []
rows = []

def load(path):
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)

def count_collection(payload, keys):
    for key in keys:
        value = payload.get(key)
        if isinstance(value, list):
            return len(value)
    if isinstance(payload, list):
        return len(payload)
    return 0

for meet_id in meet_ids:
    meet_dir = root / f"meet-{meet_id}"
    meet_payload = load(meet_dir / "meet.json")
    meet = meet_payload.get("meet", meet_payload)
    status = str(meet.get("status", ""))
    sessions = count_collection(load(meet_dir / "sessions.json"), ["sessions"])
    events = count_collection(load(meet_dir / "events.json"), ["events"])
    commitments = count_collection(load(meet_dir / "commitments.json"), ["commitments"])
    entries = count_collection(load(meet_dir / "entries.json"), ["entries"])
    exports = count_collection(load(meet_dir / "exports.json"), ["exports"])
    rows.append((meet_id, status, sessions, events, commitments, entries, exports))
    if status not in {"completed", "archived"}:
        failures.append(f"{meet_id}: status {status!r} is not completed or archived")
    if sessions <= 0:
        failures.append(f"{meet_id}: no sessions captured")
    if events <= 0:
        failures.append(f"{meet_id}: no events captured")
    if commitments <= 0:
        failures.append(f"{meet_id}: no commitments captured")
    if entries <= 0:
        failures.append(f"{meet_id}: no entries captured")
    if exports <= 0:
        failures.append(f"{meet_id}: no export artifacts captured")

with (root / "cycle-validation-summary.md").open("w", encoding="utf-8") as handle:
    handle.write("# Parallel Meet Cycle Evidence\n\n")
    handle.write("| Meet ID | Status | Sessions | Events | Commitments | Entries | Exports |\n")
    handle.write("|---|---:|---:|---:|---:|---:|---:|\n")
    for row in rows:
        handle.write("| {} | {} | {} | {} | {} | {} | {} |\n".format(*row))
    handle.write("\nNo manual patching attestation: true\n")
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

echo "Parallel meet-cycle evidence written to ${EVIDENCE_DIR}"
