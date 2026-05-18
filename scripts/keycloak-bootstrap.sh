#!/usr/bin/env bash
set -euo pipefail

KEYCLOAK_URL="${KEYCLOAK_URL:-http://127.0.0.1:8180}"
KEYCLOAK_ADMIN_USER="${KEYCLOAK_ADMIN:-admin}"
KEYCLOAK_ADMIN_PASS="${KEYCLOAK_ADMIN_PASSWORD:-admin}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-aquify}"
KEYCLOAK_ADMIN_CLIENT_ID="${KEYCLOAK_ADMIN_CLIENT_ID:-aquify-admin}"
KEYCLOAK_ADMIN_CLIENT_SECRET="${KEYCLOAK_ADMIN_CLIENT_SECRET:-aquify-local-admin-secret}"
KEYCLOAK_LOCAL_DEV_ORG_ID="${KEYCLOAK_LOCAL_DEV_ORG_ID:-00000000-0000-0000-0000-00000000a001}"
REALM_FILE="${REALM_FILE:-$(cd "$(dirname "$0")/.." && pwd)/keycloak/realm-aquify-local.json}"
KC_BIN="${KC_BIN:-$(cd "$(dirname "$0")/.." && pwd)/keycloak-26.5.6/bin/kcadm.sh}"

kc_get_json() {
  local output
  output="$("${KC_BIN}" "$@" 2>/dev/null || true)"
  if [[ -z "${output//[[:space:]]/}" ]]; then
    return 0
  fi
  printf '%s\n' "${output}"
}

get_client_uuid() {
  kc_get_json get "realms/${KEYCLOAK_REALM}/clients?clientId=$1" | python3 -c '
import json, sys
target_client_id = sys.argv[1]
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
try:
    clients = json.loads(raw)
except json.JSONDecodeError:
    sys.exit(0)
if isinstance(clients, dict):
    clients = [clients]
for client in clients:
    if client.get("clientId") == target_client_id:
        print(client.get("id", ""))
        break
' "$1"
}

get_service_account_user_id() {
  kc_get_json get "realms/${KEYCLOAK_REALM}/clients/$1/service-account-user" | python3 -c '
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
try:
    user = json.loads(raw)
except json.JSONDecodeError:
    sys.exit(0)
print(user.get("id", ""))
'
}

ensure_admin_service_account_client() {
  echo "Ensuring admin service-account client '${KEYCLOAK_ADMIN_CLIENT_ID}' ..."

  local client_uuid
  client_uuid="$(get_client_uuid "${KEYCLOAK_ADMIN_CLIENT_ID}")"

  if [[ -z "${client_uuid}" ]]; then
    "${KC_BIN}" create "realms/${KEYCLOAK_REALM}/clients" \
      -s "clientId=${KEYCLOAK_ADMIN_CLIENT_ID}" \
      -s "name=Aquify Admin Service Account" \
      -s "enabled=true" \
      -s "protocol=openid-connect" \
      -s "publicClient=false" \
      -s "secret=${KEYCLOAK_ADMIN_CLIENT_SECRET}" \
      -s "standardFlowEnabled=false" \
      -s "implicitFlowEnabled=false" \
      -s "directAccessGrantsEnabled=false" \
      -s "serviceAccountsEnabled=true" \
      -s "fullScopeAllowed=true" >/dev/null
    client_uuid="$(get_client_uuid "${KEYCLOAK_ADMIN_CLIENT_ID}")"
    echo "  Created client '${KEYCLOAK_ADMIN_CLIENT_ID}'."
  else
    "${KC_BIN}" update "realms/${KEYCLOAK_REALM}/clients/${client_uuid}" \
      -s "clientId=${KEYCLOAK_ADMIN_CLIENT_ID}" \
      -s "name=Aquify Admin Service Account" \
      -s "enabled=true" \
      -s "protocol=openid-connect" \
      -s "publicClient=false" \
      -s "secret=${KEYCLOAK_ADMIN_CLIENT_SECRET}" \
      -s "standardFlowEnabled=false" \
      -s "implicitFlowEnabled=false" \
      -s "directAccessGrantsEnabled=false" \
      -s "serviceAccountsEnabled=true" \
      -s "fullScopeAllowed=true" >/dev/null
    echo "  Updated client '${KEYCLOAK_ADMIN_CLIENT_ID}'."
  fi

  if [[ -z "${client_uuid}" ]]; then
    echo "Failed to resolve client UUID for '${KEYCLOAK_ADMIN_CLIENT_ID}'." >&2
    exit 1
  fi

  local service_account_user_id
  service_account_user_id="$(get_service_account_user_id "${client_uuid}")"
  if [[ -z "${service_account_user_id}" ]]; then
    echo "Failed to resolve service-account user for '${KEYCLOAK_ADMIN_CLIENT_ID}'." >&2
    exit 1
  fi

  local realm_management_client_uuid
  realm_management_client_uuid="$(get_client_uuid "realm-management")"
  if [[ -z "${realm_management_client_uuid}" ]]; then
    echo "Failed to resolve realm-management client in realm '${KEYCLOAK_REALM}'." >&2
    exit 1
  fi

  local role_payload
  role_payload="$(
    kc_get_json get "realms/${KEYCLOAK_REALM}/clients/${realm_management_client_uuid}/roles" | python3 -c '
import json, sys
required = {"realm-admin"}
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
try:
    roles = json.loads(raw)
except json.JSONDecodeError:
    sys.exit(0)
if isinstance(roles, dict):
    roles = [roles]
selected = [role for role in roles if role.get("name") in required]
if len(selected) != len(required):
    sys.exit(1)
print(json.dumps(selected))
'
  )"
  if [[ -z "${role_payload}" ]]; then
    echo "Failed to resolve required realm-management roles in realm '${KEYCLOAK_REALM}'." >&2
    exit 1
  fi

  printf '%s\n' "${role_payload}" | "${KC_BIN}" create \
    "realms/${KEYCLOAK_REALM}/users/${service_account_user_id}/role-mappings/clients/${realm_management_client_uuid}" \
    -f - >/dev/null

  echo "  Ensured realm-management roles on service account."
}

get_user_id_by_email() {
  kc_get_json get "realms/${KEYCLOAK_REALM}/users?email=$1&exact=true&max=1" | python3 -c '
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
try:
    users = json.loads(raw)
except json.JSONDecodeError:
    sys.exit(0)
if isinstance(users, dict):
    users = [users]
if users:
    print(users[0].get("id", ""))
'
}

get_user_id_by_username() {
  kc_get_json get "realms/${KEYCLOAK_REALM}/users?username=$1&exact=true&max=1" | python3 -c '
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
try:
    users = json.loads(raw)
except json.JSONDecodeError:
    sys.exit(0)
if isinstance(users, dict):
    users = [users]
if users:
    print(users[0].get("id", ""))
'
}

ensure_local_dev_user_profile_attribute() {
  echo "Ensuring realm user-profile declares org_id ..."

  local current_profile updated_profile
  current_profile="$(kc_get_json get "realms/${KEYCLOAK_REALM}/users/profile")"
  if [[ -z "${current_profile}" ]]; then
    echo "  (warning: could not load users/profile for realm ${KEYCLOAK_REALM})"
    return
  fi

  updated_profile="$(printf '%s\n' "${current_profile}" | python3 -c '
import json, sys

profile = json.load(sys.stdin)
attrs = profile.setdefault("attributes", [])
for attr in attrs:
    if attr.get("name") == "org_id":
        print(json.dumps(profile))
        raise SystemExit(0)

attrs.append({
    "name": "org_id",
    "displayName": "org_id",
    "permissions": {"view": ["admin", "user"], "edit": ["admin"]},
    "multivalued": False,
})
print(json.dumps(profile))
')"

  printf '%s\n' "${updated_profile}" | "${KC_BIN}" update "realms/${KEYCLOAK_REALM}/users/profile" -f - >/dev/null
  echo "  org_id is declared in users/profile."
}

ensure_local_dev_user_org_attributes() {
  echo "Ensuring local dev users carry org_id=${KEYCLOAK_LOCAL_DEV_ORG_ID} ..."

  local seeds=(
    "owner@dare.local|owner@dare.local|Dev|Owner"
    "admin@dare.local|admin@dare.local|Dev|Admin"
    "coach@dare.local|coach@dare.local|Dev|Coach"
    "parent@dare.local|parent@dare.local|Dev|Parent"
    "swimmer@dare.local|swimmer@dare.local|Emma|Chen"
    "billing@dare.local|billing@dare.local|Dev|Billing"
    "readonly@dare.local|readonly@dare.local|Dev|Read Only"
  )

  local updated=0
  local seed username email first_name last_name user_id existing payload
  for seed in "${seeds[@]}"; do
    IFS='|' read -r username email first_name last_name <<<"${seed}"
    user_id="$(get_user_id_by_email "${email}")"
    if [[ -z "${user_id}" ]]; then
      user_id="$(get_user_id_by_username "${username}")"
    fi
    if [[ -z "${user_id}" ]]; then
      echo "  (warning: could not find ${username} in realm ${KEYCLOAK_REALM})"
      continue
    fi

    existing="$("${KC_BIN}" get "realms/${KEYCLOAK_REALM}/users/${user_id}" 2>/dev/null | python3 -c '
import json, sys
try:
    user = json.load(sys.stdin)
except Exception:
    sys.exit(0)
attrs = user.get("attributes") or {}
vals = attrs.get("org_id") or []
if vals:
    print(vals[0])
' || true)"
    if [[ "${existing}" == "${KEYCLOAK_LOCAL_DEV_ORG_ID}" ]]; then
      continue
    fi

    payload="$(python3 -c '
import json, sys
user_id, username, email, first_name, last_name, org_id = sys.argv[1:]
print(json.dumps({
    "id": user_id,
    "username": username,
    "email": email,
    "firstName": first_name,
    "lastName": last_name,
    "enabled": True,
    "emailVerified": email.endswith("@dare.local"),
    "attributes": {
        "org_id": [org_id],
    },
}))
' "${user_id}" "${username}" "${email}" "${first_name}" "${last_name}" "${KEYCLOAK_LOCAL_DEV_ORG_ID}")"

    printf '%s\n' "${payload}" | "${KC_BIN}" update "realms/${KEYCLOAK_REALM}/users/${user_id}" -f - >/dev/null
    echo "  Set org_id on ${username}"
    updated=$((updated + 1))
  done

  if [[ "${updated}" -eq 0 ]]; then
    echo "  org_id already present on all local dev seed users."
  fi
}

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
else
  echo "Creating realm '${KEYCLOAK_REALM}' from ${REALM_FILE} ..."
  "${KC_BIN}" delete "realms/${KEYCLOAK_REALM}" >/dev/null 2>&1 || true
  "${KC_BIN}" create realms -f "${REALM_FILE}" >/dev/null
fi

ensure_admin_service_account_client
ensure_local_dev_user_profile_attribute
ensure_local_dev_user_org_attributes

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
echo "OIDC Client ID: aquify-id"
echo "OIDC Client Secret: aquify-local-dev-secret"
echo "Admin Client ID: ${KEYCLOAK_ADMIN_CLIENT_ID}"
echo "Admin Client Secret: ${KEYCLOAK_ADMIN_CLIENT_SECRET}"
echo "Sample users (password: ChangeMe123!):"
echo "  owner@dare.local admin@dare.local coach@dare.local"
echo "  parent@dare.local swimmer@dare.local billing@dare.local readonly@dare.local"
