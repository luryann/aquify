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

# Pass the DSN (which embeds the DB password) to psql via the PGDATABASE
# environment variable rather than a command-line argument — libpq accepts a
# full connection URI there — so the credential never appears in `ps` output.
export PGDATABASE="${DATABASE_URL}"

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

  # Validate HOUSEHOLD_ID is a well-formed UUID before it reaches any SQL string.
  if ! [[ "$HOUSEHOLD_ID" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]; then
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

  # Validate KC_USER_ID is a well-formed UUID before it reaches any SQL string.
  if ! [[ "$KC_USER_ID" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]; then
    echo "ERROR: KC_USER_ID from Keycloak is not a valid UUID: '$KC_USER_ID'" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # Check if another household already has this keycloak_user_id.
  # Use psql --set for parameterized variable substitution (:'varname' is quoted literal).
  EXISTING=$(psql --csv -t \
    --set="kc_user_id=$KC_USER_ID" \
    -c "SELECT id FROM households WHERE keycloak_user_id = :'kc_user_id' LIMIT 1" | tr -d '[:space:]')
  if [ -n "$EXISTING" ]; then
    echo "SKIP household ${HOUSEHOLD_ID}: keycloak_user_id ${KC_USER_ID} already linked to ${EXISTING}"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  echo "LINK household ${HOUSEHOLD_ID} → keycloak_user_id ${KC_USER_ID} (email=${PRIMARY_EMAIL})"
  if psql \
    --set="kc_user_id=$KC_USER_ID" \
    --set="hh_id=$HOUSEHOLD_ID" \
    -c "UPDATE households SET keycloak_user_id = :'kc_user_id', updated_at = now() WHERE id = :'hh_id'::uuid" \
    > /dev/null 2>&1; then
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
