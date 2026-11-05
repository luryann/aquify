#!/usr/bin/env bash
# preflight.sh — validate a production deployment directory BEFORE the first start
# or a promote. Read-only; prints every problem it finds and exits non-zero if any.
#
# Usage:
#   DEPLOY_DIR=/opt/aquify deploy/scripts/preflight.sh
#
# Options (environment):
#   AQUIFY_ALLOW_STRIPE_TEST=1   accept an sk_test_ Stripe key (staging / synthetic-data acceptance only)
#
# Env files are parsed line by line, never sourced, so a malformed or hostile value
# cannot run code here.

set -uo pipefail

DEPLOY_DIR="${DEPLOY_DIR:-/opt/aquify}"
INFRA="${INFRA_ENV:-${DEPLOY_DIR}/.env.production}"
APP="${APP_ENV_FILE:-${DEPLOY_DIR}/.env.app}"
COMPOSE_FILE="${COMPOSE_FILE:-${DEPLOY_DIR}/docker-compose.production.yml}"

FAILS=0
bad() { echo "FAIL: $*"; FAILS=$((FAILS + 1)); }
ok() { echo "ok:   $*"; }

# envget <file> <KEY>: last value of KEY (surrounding quotes stripped); empty if absent.
envget() {
  local line v
  line="$(grep -E "^[[:space:]]*$2=" "$1" 2>/dev/null | tail -n 1)" || true
  v="${line#*=}"
  v="${v%$'\r'}"
  v="${v#\"}"; v="${v%\"}"; v="${v#\'}"; v="${v%\'}"
  printf '%s' "${v}"
}

file_mode_ok() { # no access for "other"
  local mode
  mode="$(stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1" 2>/dev/null)" || return 1
  [ $(( 8#${mode} & 8#007 )) -eq 0 ]
}

check_file() {
  if [ ! -f "$1" ]; then bad "$1 is missing"; return 1; fi
  if ! file_mode_ok "$1"; then bad "$1 is readable by other users; chmod 640 and chown root:aquify"; fi
  # Assignments only: comments may legitimately mention the placeholder.
  local pending
  pending="$(grep -vE '^[[:space:]]*#' "$1" | grep -Ei '=.*(CHANGE_ME|change-me|changeme)' | cut -d= -f1 | tr '\n' ' ')" || true
  if [ -n "${pending}" ]; then
    bad "$1 still contains placeholder values for: ${pending}"
  fi
  return 0
}

need() { # need <file> <KEY>
  [ -n "$(envget "$1" "$2")" ] || bad "$2 is not set in $1"
}

secret_ok() { # secret_ok <file> <KEY> <minlen> : charset [A-Za-z0-9_-]
  local v
  v="$(envget "$1" "$2")"
  [ -n "${v}" ] || return 0   # reported by need
  [[ "${v}" =~ ^[A-Za-z0-9_-]+$ ]] || bad "$2 may only contain [A-Za-z0-9_-]"
  [ "${#v}" -ge "$3" ] || bad "$2 must be at least $3 characters (got ${#v})"
}

check_file "${INFRA}" && {
  for k in AQUIFY_DOMAIN ACME_EMAIL POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD AQUIFY_OWNER_DB_USER AQUIFY_OWNER_DB_PASSWORD \
           AQUIFY_APP_DB_USER AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_USER AQUIFY_BACKUP_DB_PASSWORD \
           KEYCLOAK_DB_NAME KEYCLOAK_DB_USER KEYCLOAK_DB_PASSWORD REDIS_PASSWORD MINIO_ROOT_USER \
           MINIO_ROOT_PASSWORD KEYCLOAK_ADMIN_USER KEYCLOAK_ADMIN_PASSWORD GRAFANA_ADMIN_USER GRAFANA_ADMIN_PASSWORD; do
    need "${INFRA}" "$k"
  done
  for k in POSTGRES_PASSWORD AQUIFY_OWNER_DB_PASSWORD AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_PASSWORD \
           KEYCLOAK_DB_PASSWORD REDIS_PASSWORD MINIO_ROOT_PASSWORD KEYCLOAK_ADMIN_PASSWORD GRAFANA_ADMIN_PASSWORD; do
    secret_ok "${INFRA}" "$k" 16
  done
  # The four database roles must be distinct, and the application must not be the superuser.
  su="$(envget "${INFRA}" POSTGRES_USER)"; ow="$(envget "${INFRA}" AQUIFY_OWNER_DB_USER)"
  ap="$(envget "${INFRA}" AQUIFY_APP_DB_USER)"; bk="$(envget "${INFRA}" AQUIFY_BACKUP_DB_USER)"
  kc="$(envget "${INFRA}" KEYCLOAK_DB_USER)"
  dups="$(printf '%s\n' "${su}" "${ow}" "${ap}" "${bk}" "${kc}" | grep -v '^$' | sort | uniq -d)"
  [ -z "${dups}" ] || bad "database role names must all differ (duplicate: ${dups})"
  pws="$(for k in POSTGRES_PASSWORD AQUIFY_OWNER_DB_PASSWORD AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_PASSWORD KEYCLOAK_DB_PASSWORD REDIS_PASSWORD MINIO_ROOT_PASSWORD; do envget "${INFRA}" "$k"; echo; done | grep -v '^$' | sort | uniq -d)"
  [ -z "${pws}" ] || bad "passwords must be unique per service (a value is reused)"
}

check_file "${APP}" && {
  for k in DATABASE_URL REDIS_URL API_ALLOWED_ORIGINS SECURITY_TRUSTED_PROXY_CIDRS STORAGE_ENDPOINT STORAGE_BUCKET \
           STORAGE_ACCESS_KEY STORAGE_SECRET_KEY INTEGRATIONS_ENCRYPTION_KEY OIDC_AUTHORIZE_URL OIDC_TOKEN_URL \
           OIDC_JWKS_URL OIDC_END_SESSION_URL OIDC_REDIRECT_URL OIDC_POST_LOGOUT_REDIRECT_URL OIDC_EXPECTED_ISSUER \
           OIDC_CLIENT_ID OIDC_CLIENT_SECRET OIDC_STATE_SIGNING_KEY KEYCLOAK_ADMIN_URL KEYCLOAK_REALM \
           KEYCLOAK_ADMIN_CLIENT_ID KEYCLOAK_ADMIN_CLIENT_SECRET STRIPE_SECRET_KEY STRIPE_WEBHOOK_SECRET \
           STRIPE_CONNECT_WEBHOOK_SECRET POSTMARK_SERVER_TOKEN POSTMARK_FROM_EMAIL; do
    need "${APP}" "$k"
  done
  for k in INTEGRATIONS_ENCRYPTION_KEY OIDC_STATE_SIGNING_KEY; do secret_ok "${APP}" "$k" 32; done

  # DATABASE_URL must use the restricted application role from .env.production.
  dburl="$(envget "${APP}" DATABASE_URL)"
  if [[ "${dburl}" =~ ^postgres(ql)?://([^:@/]+):([^@]+)@([^:/]+)(:[0-9]+)?/([^?]+) ]]; then
    u="${BASH_REMATCH[2]}"; p="${BASH_REMATCH[3]}"; h="${BASH_REMATCH[4]}"
    [ "${u}" = "$(envget "${INFRA}" AQUIFY_APP_DB_USER)" ] || bad "DATABASE_URL user '${u}' is not AQUIFY_APP_DB_USER (the app must not use the owner or superuser role)"
    [ "${p}" = "$(envget "${INFRA}" AQUIFY_APP_DB_PASSWORD)" ] || bad "DATABASE_URL password does not match AQUIFY_APP_DB_PASSWORD"
    [ "${h}" = "postgres" ] || bad "DATABASE_URL host is '${h}', expected the compose service name 'postgres'"
  else
    bad "DATABASE_URL is not a postgres://user:password@host/db URL"
  fi
  redisurl="$(envget "${APP}" REDIS_URL)"
  [[ "${redisurl}" == *":$(envget "${INFRA}" REDIS_PASSWORD)@"* ]] || bad "REDIS_URL does not carry REDIS_PASSWORD"

  # The app must use a scoped MinIO account, not the root credentials.
  [ "$(envget "${APP}" STORAGE_ACCESS_KEY)" != "$(envget "${INFRA}" MINIO_ROOT_USER)" ] || bad "STORAGE_ACCESS_KEY is the MinIO root user; create a scoped service account"
  [ "$(envget "${APP}" STORAGE_USE_SSL)" = "true" ] || bad "STORAGE_USE_SSL must be true (presigned URLs are used from browsers)"

  # Hostnames must all derive from AQUIFY_DOMAIN: a typo in any one of them breaks
  # login, uploads or CORS in a way that only shows up at runtime.
  dom="$(envget "${INFRA}" AQUIFY_DOMAIN)"
  if [ -n "${dom}" ] && [[ "${dom}" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ ]] && [[ "${dom}" == *.* ]]; then
    [ "$(envget "${APP}" API_ALLOWED_ORIGINS)" = "https://app.${dom}" ] || bad "API_ALLOWED_ORIGINS must be https://app.${dom}"
    [ "$(envget "${APP}" STORAGE_ENDPOINT)" = "files.${dom}" ] || bad "STORAGE_ENDPOINT must be files.${dom}"
    for k in OIDC_AUTHORIZE_URL OIDC_TOKEN_URL OIDC_JWKS_URL OIDC_END_SESSION_URL; do
      [[ "$(envget "${APP}" "$k")" == https://auth.${dom}/realms/* ]] || bad "$k must be under https://auth.${dom}/realms/"
    done
    [[ "$(envget "${APP}" OIDC_EXPECTED_ISSUER)" == "https://auth.${dom}/realms/"* ]] || bad "OIDC_EXPECTED_ISSUER must be https://auth.${dom}/realms/<realm>"
    [ "$(envget "${APP}" OIDC_REDIRECT_URL)" = "https://app.${dom}/auth/callback" ] || bad "OIDC_REDIRECT_URL must be https://app.${dom}/auth/callback"
  elif [ -n "${dom}" ]; then
    bad "AQUIFY_DOMAIN '${dom}' is not a valid DNS name"
  fi

  # https everywhere the browser or Stripe is involved.
  for k in OIDC_AUTHORIZE_URL OIDC_TOKEN_URL OIDC_JWKS_URL OIDC_END_SESSION_URL OIDC_REDIRECT_URL OIDC_POST_LOGOUT_REDIRECT_URL OIDC_EXPECTED_ISSUER; do
    [[ "$(envget "${APP}" "$k")" == https://* ]] || bad "$k must be an https:// URL"
  done
  IFS=',' read -ra origins <<< "$(envget "${APP}" API_ALLOWED_ORIGINS)"
  for o in "${origins[@]}"; do
    o="${o// /}"
    [[ "${o}" =~ ^https://[^/]+$ ]] || bad "API_ALLOWED_ORIGINS entry '${o}' must be a bare https origin"
    [[ "${o}" != *localhost* && "${o}" != *127.0.0.1* ]] || bad "API_ALLOWED_ORIGINS entry '${o}' is a loopback address"
  done
  if grep -E '^[[:space:]]*[A-Z_]+=.*(example\.com|localhost|127\.0\.0\.1)' "${APP}" | grep -vE '^[[:space:]]*(KEYCLOAK_ADMIN_URL|DATABASE_URL|REDIS_URL)=' >/dev/null; then
    bad "${APP} still contains example.com/localhost values"
  fi

  # Stripe live key unless explicitly acceptance-testing.
  sk="$(envget "${APP}" STRIPE_SECRET_KEY)"
  if [[ "${sk}" == sk_test_* || "${sk}" == rk_test_* ]]; then
    [ "${AQUIFY_ALLOW_STRIPE_TEST:-}" = "1" ] || bad "STRIPE_SECRET_KEY is a test key; production needs sk_live_/rk_live_ (set AQUIFY_ALLOW_STRIPE_TEST=1 only for synthetic acceptance runs)"
  elif [[ -n "${sk}" && "${sk}" != sk_live_* && "${sk}" != rk_live_* ]]; then
    bad "STRIPE_SECRET_KEY does not look like a Stripe secret key"
  fi
  # Dev switches must not be present at all.
  grep -E '^[[:space:]]*(AUTH_ENABLE_DEV_STUB_TOKENS|AUTH_ALLOW_DEV_SESSION_ISSUE|SESSION_COOKIE_SECURE)=' "${APP}" >/dev/null \
    && bad "${APP} sets a development switch (AUTH_*DEV*/SESSION_COOKIE_SECURE); remove it"
  grep -E '^[[:space:]]*APP_ENV=' "${APP}" >/dev/null && bad "APP_ENV is pinned in the compose file; remove it from ${APP}"
}

if [ -f "${COMPOSE_FILE}" ] && command -v docker >/dev/null 2>&1 && [ -f "${INFRA}" ]; then
  rel="${DEPLOY_DIR}/release.env"
  if [ -f "${rel}" ]; then
    if docker compose --project-directory "${DEPLOY_DIR}" --env-file "${INFRA}" --env-file "${rel}" -f "${COMPOSE_FILE}" --profile tools config -q 2>/tmp/aquify-preflight-compose.err; then
      ok "docker compose config accepts the file with these env files"
    else
      bad "docker compose config failed: $(head -3 /tmp/aquify-preflight-compose.err | tr '\n' ' ')"
    fi
  else
    echo "skip: ${rel} not present yet; compose config not checked"
  fi
else
  echo "skip: docker not available; compose config NOT validated (do this on the target host)"
fi

if [ "${FAILS}" -eq 0 ]; then
  echo "PREFLIGHT PASSED"
  exit 0
fi
echo "PREFLIGHT FAILED: ${FAILS} problem(s)"
exit 1
