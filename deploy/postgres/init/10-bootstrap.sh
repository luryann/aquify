#!/usr/bin/env bash
# Creates the restricted Aquify database roles and Keycloak's database.
#
# Run automatically by the postgres image on first start of an EMPTY data volume
# (it sits in /docker-entrypoint-initdb.d). To rotate passwords or repair roles on
# a running database, run it again as the bootstrap superuser:
#   docker compose exec postgres bash /docker-entrypoint-initdb.d/10-bootstrap.sh
# It is idempotent.
#
# Passwords must be 16+ characters of [A-Za-z0-9_-]: they travel in connection
# settings and in postgres:// URLs, so nothing that needs quoting or escaping is
# accepted (generate with `openssl rand -hex 24`).

set -Eeuo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PSQL="${PSQL:-psql}"

die() { echo "bootstrap: $*" >&2; exit 1; }
require() { [[ -n "${!1:-}" ]] || die "$1 is required"; }
ident() { [[ "${!1}" =~ ^[a-z][a-z0-9_]{2,40}$ ]] || die "$1 must match [a-z][a-z0-9_]{2,40}"; }
secret() { [[ "${!1}" =~ ^[A-Za-z0-9_-]{16,}$ ]] || die "$1 must be 16+ characters of [A-Za-z0-9_-]"; }

for v in POSTGRES_USER POSTGRES_DB AQUIFY_OWNER_DB_USER AQUIFY_OWNER_DB_PASSWORD AQUIFY_APP_DB_USER \
         AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_USER AQUIFY_BACKUP_DB_PASSWORD KEYCLOAK_DB_NAME \
         KEYCLOAK_DB_USER KEYCLOAK_DB_PASSWORD; do
  require "$v"
done
for v in POSTGRES_DB AQUIFY_OWNER_DB_USER AQUIFY_APP_DB_USER AQUIFY_BACKUP_DB_USER KEYCLOAK_DB_NAME KEYCLOAK_DB_USER; do
  ident "$v"
done
for v in AQUIFY_OWNER_DB_PASSWORD AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_PASSWORD KEYCLOAK_DB_PASSWORD; do
  secret "$v"
done
[[ "${AQUIFY_APP_DB_USER}" != "${POSTGRES_USER}" ]] || die "the application role must not be the bootstrap superuser (${POSTGRES_USER})"
[[ "${AQUIFY_OWNER_DB_USER}" != "${POSTGRES_USER}" ]] || die "the owner role must not be the bootstrap superuser (${POSTGRES_USER})"

# Values contain no spaces (validated above), so PGOPTIONS word-splitting is safe.
export PGOPTIONS="-c aquify.db_name=${POSTGRES_DB} \
-c aquify.owner_user=${AQUIFY_OWNER_DB_USER} -c aquify.owner_password=${AQUIFY_OWNER_DB_PASSWORD} \
-c aquify.app_user=${AQUIFY_APP_DB_USER} -c aquify.app_password=${AQUIFY_APP_DB_PASSWORD} \
-c aquify.backup_user=${AQUIFY_BACKUP_DB_USER} -c aquify.backup_password=${AQUIFY_BACKUP_DB_PASSWORD} \
-c aquify.kc_user=${KEYCLOAK_DB_USER} -c aquify.kc_password=${KEYCLOAK_DB_PASSWORD}"

run_sql() { "${PSQL}" -v ON_ERROR_STOP=1 --username "${POSTGRES_USER}" --dbname "$1" "${@:2}"; }

run_sql "${POSTGRES_DB}" -f "${HERE}/roles.sql.in"

# Keycloak's own database (CREATE DATABASE cannot run inside the DO block).
exists="$(run_sql postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '${KEYCLOAK_DB_NAME}'")"
if [[ "${exists}" != "1" ]]; then
  run_sql postgres -c "CREATE DATABASE \"${KEYCLOAK_DB_NAME}\" OWNER \"${KEYCLOAK_DB_USER}\""
fi
run_sql postgres -c "REVOKE ALL ON DATABASE \"${KEYCLOAK_DB_NAME}\" FROM PUBLIC"
echo "bootstrap: roles and keycloak database are in place"
