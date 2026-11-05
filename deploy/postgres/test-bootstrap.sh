#!/usr/bin/env bash
# Sandbox test for init/10-bootstrap.sh input handling, with a stub psql.
# It proves the wrapper validates its inputs and calls psql with the right script;
# the SQL itself is proven against a real Postgres by
# server/internal/deploycheck (TestRoleBootstrapGivesTheAppRoleRowLevelSecurityAndNoDDL).
#   deploy/postgres/test-bootstrap.sh
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(mktemp -d)"; trap 'rm -rf "${ROOT}"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
mkdir -p "${ROOT}/bin"
cat > "${ROOT}/bin/psql" <<'STUB'
#!/usr/bin/env bash
echo "psql $*" >> "$STUB_LOG"
echo "PGOPTIONS=${PGOPTIONS:-}" >> "$STUB_LOG"
exit 0
STUB
chmod +x "${ROOT}/bin/psql"
export STUB_LOG="${ROOT}/psql.log" PSQL="${ROOT}/bin/psql"

pw() { openssl rand -hex 12; }
base_env() {
  export POSTGRES_USER=aquify_admin POSTGRES_DB=aquify AQUIFY_OWNER_DB_USER=aquify_owner AQUIFY_OWNER_DB_PASSWORD="$(pw)" \
    AQUIFY_APP_DB_USER=aquify_app AQUIFY_APP_DB_PASSWORD="$(pw)" AQUIFY_BACKUP_DB_USER=aquify_backup AQUIFY_BACKUP_DB_PASSWORD="$(pw)" \
    KEYCLOAK_DB_NAME=keycloak KEYCLOAK_DB_USER=keycloak KEYCLOAK_DB_PASSWORD="$(pw)"
  : > "${STUB_LOG}"
}
run() { bash "${HERE}/init/10-bootstrap.sh" >"${ROOT}/out" 2>&1; }

base_env
run || { cat "${ROOT}/out"; fail "valid environment should succeed"; }
grep -q 'roles.sql.in' "${STUB_LOG}" || fail "roles.sql.in was not applied"
grep -q 'CREATE DATABASE "keycloak" OWNER "keycloak"' "${STUB_LOG}" || fail "keycloak database was not created"
grep -q 'aquify.app_user=aquify_app' "${STUB_LOG}" || fail "role names not passed as settings"
! grep -q -- '-c .*PASSWORD' "${STUB_LOG}" || fail "a password appeared in a psql argument"

reject() { # reject <label> <env override...>
  local label="$1"; shift
  base_env
  env "$@" bash "${HERE}/init/10-bootstrap.sh" >"${ROOT}/out" 2>&1 && fail "${label}: should have been rejected"
  [ ! -s "${STUB_LOG}" ] || fail "${label}: psql ran despite invalid input"
}
reject "short password"        AQUIFY_APP_DB_PASSWORD=short
reject "password with quote"   "AQUIFY_APP_DB_PASSWORD=aaaaaaaaaaaaaaaa'bbbb"
reject "password with space"   "AQUIFY_OWNER_DB_PASSWORD=aaaaaaaa bbbbbbbbbb"
reject "uppercase role"        AQUIFY_APP_DB_USER=Aquify_App
reject "injection in role"     'AQUIFY_APP_DB_USER=x;drop'
reject "app is the superuser"  AQUIFY_APP_DB_USER=aquify_admin
reject "owner is the superuser" AQUIFY_OWNER_DB_USER=aquify_admin
reject "missing variable"      KEYCLOAK_DB_PASSWORD=

echo "postgres bootstrap sandbox tests passed"
