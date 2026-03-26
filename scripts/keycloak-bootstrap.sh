#!/usr/bin/env bash
set -euo pipefail

KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:8180}"
KEYCLOAK_ADMIN_USER="${KEYCLOAK_ADMIN:-admin}"
KEYCLOAK_ADMIN_PASS="${KEYCLOAK_ADMIN_PASSWORD:-admin}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-aquify}"
REALM_FILE="${REALM_FILE:-$(cd "$(dirname "$0")/.." && pwd)/keycloak/realm-aquify-local.json}"
KC_BIN="${KC_BIN:-$(cd "$(dirname "$0")/.." && pwd)/keycloak-26.5.6/bin/kcadm.sh}"

if [[ ! -f "${REALM_FILE}" ]]; then
  echo "Realm file not found: ${REALM_FILE}" >&2
  exit 1
fi

if [[ ! -x "${KC_BIN}" ]]; then
  echo "Keycloak admin CLI not found or not executable: ${KC_BIN}" >&2
  echo "Expected local install at keycloak-26.5.6/bin/kcadm.sh" >&2
  exit 1
fi

echo "Waiting for Keycloak at ${KEYCLOAK_URL} ..."
until curl -sf "${KEYCLOAK_URL}/realms/master/.well-known/openid-configuration" >/dev/null; do
  sleep 2
done

echo "Logging into Keycloak admin API ..."
"${KC_BIN}" config credentials --server "${KEYCLOAK_URL}" --realm master --user "${KEYCLOAK_ADMIN_USER}" --password "${KEYCLOAK_ADMIN_PASS}" >/dev/null

REALM_EXISTS=$("${KC_BIN}" get "realms/${KEYCLOAK_REALM}" --realm master --fields realm 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print('yes' if d.get('realm') else 'no')" 2>/dev/null || echo "no")

if [[ "${REALM_EXISTS}" == "yes" && "${KEYCLOAK_FORCE_RESET:-0}" != "1" ]]; then
  echo "Realm '${KEYCLOAK_REALM}' already exists — skipping recreation (users/MFA preserved)."
  echo "  To force a full reset: KEYCLOAK_FORCE_RESET=1 make keycloak-bootstrap"
  echo "Bootstrap complete (no changes)."
  exit 0
fi

echo "Creating realm '${KEYCLOAK_REALM}' from ${REALM_FILE} ..."
"${KC_BIN}" delete "realms/${KEYCLOAK_REALM}" >/dev/null 2>&1 || true
"${KC_BIN}" create realms -f "${REALM_FILE}" >/dev/null

# Disable OTP in the browser authentication flow for local dev.
# By default Keycloak's browser flow includes a "Browser - Conditional OTP" step.
# Without this change, users are prompted for a TOTP code they cannot set up,
# because new accounts have no OTP enrolled. For local dev we just disable the step.
echo "Disabling OTP step in browser authentication flow ..."
EXECUTIONS=$("${KC_BIN}" get "authentication/flows/browser/executions" --realm "${KEYCLOAK_REALM}" 2>/dev/null || echo "[]")
OTP_EXEC_ID=$(echo "${EXECUTIONS}" | python3 -c "
import sys, json
execs = json.load(sys.stdin)
for e in execs:
    # Match the conditional OTP sub-flow or the OTP form execution
    display = e.get('displayName', '') or e.get('providerId', '')
    if 'otp' in display.lower() or 'OTP' in display:
        print(e.get('id', ''))
        break
" 2>/dev/null || true)

if [[ -n "${OTP_EXEC_ID}" ]]; then
  "${KC_BIN}" update "authentication/flows/browser/executions" \
    --realm "${KEYCLOAK_REALM}" \
    -s "id=${OTP_EXEC_ID}" \
    -s "requirement=DISABLED" >/dev/null 2>&1 || \
    echo "  (warning: could not disable OTP execution ${OTP_EXEC_ID} — disable it manually in the Keycloak admin console)"
  echo "  OTP step disabled (execution ${OTP_EXEC_ID})."
else
  echo "  (warning: could not find OTP execution in browser flow — disable it manually if prompted for OTP during login)"
fi

echo "Bootstrap complete."
echo "Realm: ${KEYCLOAK_REALM}"
echo "Client ID: aquify-id"
echo "Client Secret: aquify-local-dev-secret"
echo "Sample users (password: ChangeMe123!):"
echo "  owner@dare.local admin@dare.local coach@dare.local"
echo "  parent@dare.local swimmer@dare.local billing@dare.local readonly@dare.local"
