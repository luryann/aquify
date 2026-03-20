#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INPUT_DIR="${INPUT_DIR:-}"
REPORT_PATH="${REPORT_PATH:-${ROOT_DIR}/cutover-rehearsal-report.json}"
ORG_ID="${ORG_ID:-org-cutover-rehearsal}"
ACTOR_ID="${ACTOR_ID:-cutover-rehearsal}"

if [[ -z "${INPUT_DIR}" ]]; then
  echo "INPUT_DIR is required. Example: make cutover-rehearsal INPUT_DIR=/absolute/path/to/export-dir" >&2
  exit 1
fi

cd "${ROOT_DIR}/server"
go run ./cmd/migrate dry-run \
  -input-dir "${INPUT_DIR}" \
  -report "${REPORT_PATH}" \
  -org-id "${ORG_ID}" \
  -actor-id "${ACTOR_ID}"
