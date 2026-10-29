#!/usr/bin/env bash
# backfill-household-keycloak-ids.sh
#
# Matches existing households whose primary_email corresponds to a Keycloak
# user and sets keycloak_user_id where the match is unambiguous (exactly one
# Keycloak user found for that email, and the household does not already have a
# keycloak_user_id set).
#
# Prerequisites:
#   - KEYCLOAK_ADMIN_URL, KEYCLOAK_ADMIN_CLIENT_ID, KEYCLOAK_ADMIN_CLIENT_SECRET,
#     KEYCLOAK_REALM, DATABASE_URL must be set in the environment.
#   - psql and curl must be available.
#   - jq must be available.
#
# Usage:
#   export DATABASE_URL="postgres://..."
#   export KEYCLOAK_ADMIN_URL="https://keycloak.example.com"
#   export KEYCLOAK_ADMIN_CLIENT_ID="dare-admin"
#   export KEYCLOAK_ADMIN_CLIENT_SECRET="..."
#   export KEYCLOAK_REALM="dare"
#   ./scripts/backfill-household-keycloak-ids.sh

set -euo pipefail

: "${KEYCLOAK_ADMIN_URL:?KEYCLOAK_ADMIN_URL is required}"
: "${KEYCLOAK_ADMIN_CLIENT_ID:?KEYCLOAK_ADMIN_CLIENT_ID is required}"
: "${KEYCLOAK_ADMIN_CLIENT_SECRET:?KEYCLOAK_ADMIN_CLIENT_SECRET is required}"
: "${KEYCLOAK_REALM:?KEYCLOAK_REALM is required}"
: "${DATABASE_URL:?DATABASE_URL is required}"

# libpq does not expand a connection URI from PGDATABASE (only from a dbname
# argument), so split the DSN into the PG* variables psql reads. The DSN goes to
# python through the environment, so the password never appears in `ps` output.
eval "$(python3 -c '
import os, shlex, urllib.parse as u
d = u.urlparse(os.environ["DATABASE_URL"])
q = dict(u.parse_qsl(d.query))
env = {
    "PGHOST": d.hostname or "",
    "PGPORT": str(d.port or ""),
    "PGUSER": u.unquote(d.username or ""),
    "PGPASSWORD": u.unquote(d.password or ""),
    "PGDATABASE": u.unquote(d.path.lstrip("/")),
    "PGSSLMODE": q.get("sslmode", ""),
}
for k, v in env.items():
    if v:
        print("export %s=%s" % (k, shlex.quote(v)))
')"

# IDs are validated as UUIDs before they reach any SQL string.
is_uuid() {
  [[ "$1" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]
}

echo "==> Obtaining Keycloak admin token..."
# Pass the client secret via stdin rather than a -d argument so it never
# appears in this process's argv (visible to other local users via `ps`).
TOKEN_RESPONSE=$(curl -s -X POST \
  "${KEYCLOAK_ADMIN_URL}/realms/master/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-binary @- <<EOF
grant_type=client_credentials&client_id=${KEYCLOAK_ADMIN_CLIENT_ID}&client_secret=${KEYCLOAK_ADMIN_CLIENT_SECRET}
EOF
)

ADMIN_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
if [ -z "$ADMIN_TOKEN" ] || [ "$ADMIN_TOKEN" = "null" ]; then
  echo "ERROR: Failed to obtain Keycloak admin token." >&2
  echo "$TOKEN_RESPONSE" >&2
  exit 1
fi

echo "==> Fetching households without keycloak_user_id..."
HOUSEHOLDS=$(psql --csv -t -c \
  "SELECT id, primary_email FROM households WHERE keycloak_user_id IS NULL AND primary_email IS NOT NULL AND deleted_at IS NULL ORDER BY created_at")

MATCHED=0
SKIPPED=0
ERRORS=0

while IFS=',' read -r HOUSEHOLD_ID PRIMARY_EMAIL; do
  HOUSEHOLD_ID="${HOUSEHOLD_ID//\"/}"
  PRIMARY_EMAIL="${PRIMARY_EMAIL//\"/}"

  if [ -z "$PRIMARY_EMAIL" ]; then
    continue
  fi

  if ! is_uuid "$HOUSEHOLD_ID"; then
    echo "ERROR: HOUSEHOLD_ID is not a valid UUID: '$HOUSEHOLD_ID'" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # URL-encode the email for the query parameter.
  ENCODED_EMAIL=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$PRIMARY_EMAIL")

  KC_USERS=$(curl -s -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    "${KEYCLOAK_ADMIN_URL}/admin/realms/${KEYCLOAK_REALM}/users?email=${ENCODED_EMAIL}&exact=true&max=2")

  USER_COUNT=$(echo "$KC_USERS" | jq 'length')
  if [ "$USER_COUNT" -ne 1 ]; then
    echo "SKIP household ${HOUSEHOLD_ID} (email=${PRIMARY_EMAIL}): found ${USER_COUNT} Keycloak users"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  KC_USER_ID=$(echo "$KC_USERS" | jq -r '.[0].id')
  if [ -z "$KC_USER_ID" ] || [ "$KC_USER_ID" = "null" ]; then
    echo "SKIP household ${HOUSEHOLD_ID}: could not parse Keycloak user ID"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  if ! is_uuid "$KC_USER_ID"; then
    echo "ERROR: KC_USER_ID from Keycloak is not a valid UUID: '$KC_USER_ID'" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # Check if another household already has this keycloak_user_id.
  # Use psql --set for parameterized variable substitution (:'varname' is quoted literal).
  # psql does not interpolate :'var' inside a -c string, so the SQL goes in on stdin.
  EXISTING=$(psql --csv -t -v ON_ERROR_STOP=1 \
    --set="kc_user_id=$KC_USER_ID" <<'SQL' | tr -d '[:space:]'
SELECT id FROM households WHERE keycloak_user_id = :'kc_user_id' LIMIT 1;
SQL
)
  if [ -n "$EXISTING" ]; then
    echo "SKIP household ${HOUSEHOLD_ID}: keycloak_user_id ${KC_USER_ID} already linked to ${EXISTING}"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  echo "LINK household ${HOUSEHOLD_ID} → keycloak_user_id ${KC_USER_ID} (email=${PRIMARY_EMAIL})"
  if psql -v ON_ERROR_STOP=1 \
    --set="kc_user_id=$KC_USER_ID" \
    --set="hh_id=$HOUSEHOLD_ID" > /dev/null 2>&1 <<'SQL'
UPDATE households SET keycloak_user_id = :'kc_user_id', updated_at = now() WHERE id = :'hh_id'::uuid;
SQL
  then
    MATCHED=$((MATCHED + 1))
  else
    echo "ERROR: Failed to update household ${HOUSEHOLD_ID}" >&2
    ERRORS=$((ERRORS + 1))
  fi

done <<< "$HOUSEHOLDS"

echo ""
echo "==> Backfill complete: matched=${MATCHED} skipped=${SKIPPED} errors=${ERRORS}"
if [ "$ERRORS" -gt 0 ]; then
  exit 1
fi
