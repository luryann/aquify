#!/usr/bin/env bash
# Shared HTTP helper for the evidence-capture scripts.
# Requires API_BASE_URL, AUTH_TOKEN and ORG_ID to be set by the caller.

# evidence_request METHOD PATH OUTPUT_FILE [JSON_BODY]
# Saves the response body to OUTPUT_FILE and the HTTP status to OUTPUT_FILE.status.
# Returns 0 on a 2xx response; otherwise prints the failure and the body to stderr and returns 1.
evidence_request() {
  local method="$1"
  local path="$2"
  local output="$3"
  local body="${4:-}"
  local -a args=(
    -X "${method}" "${API_BASE_URL}${path}"
    -H "Authorization: Bearer ${AUTH_TOKEN}"
    -H "X-Org-Id: ${ORG_ID}"
    -H "Accept: application/json"
  )
  if [[ -n "${body}" ]]; then
    args+=(-H "Content-Type: application/json" --data "${body}")
  fi
  local code
  code="$(curl -sS -o "${output}" -w "%{http_code}" "${args[@]}")"
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

# evidence_python PURPOSE
# Prints the python interpreter to use (PYTHON_BIN, else python3, else python); exits 1 if none.
evidence_python() {
  if [[ -n "${PYTHON_BIN:-}" ]]; then
    printf '%s' "${PYTHON_BIN}"
  elif command -v python3 >/dev/null 2>&1; then
    printf 'python3'
  elif command -v python >/dev/null 2>&1; then
    printf 'python'
  else
    echo "python3 or python is required to $1." >&2
    return 1
  fi
}
