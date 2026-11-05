#!/usr/bin/env bash
# Sandbox test for preflight.sh: builds a valid env set from the real templates,
# requires it to pass, then breaks it one way at a time and requires each break to
# be caught. No Docker, no network, no real secrets (all values are generated here).
#   deploy/scripts/test-preflight.sh
set -euo pipefail
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENVDIR="${SRC}/../env"
ROOT="$(mktemp -d)"; trap 'chmod -R u+w "${ROOT}" 2>/dev/null || true; rm -rf "${ROOT}"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
rand() { openssl rand -hex "${1:-16}"; }

setkey() { # setkey <file> <KEY> <VALUE>
  awk -v k="$2" -v v="$3" 'BEGIN{FS=OFS="="} $1==k{print k"="v; found=1; next} {print} END{if(!found) print k"="v}' "$1" > "$1.tmp" && cat "$1.tmp" > "$1" && rm -f "$1.tmp"
}

# good <dir>: a complete, valid deployment directory built from the templates.
good() {
  D="${ROOT}/$1"; mkdir -p "${D}"
  cp "${ENVDIR}/production.env.example" "${D}/.env.production"
  cp "${ENVDIR}/app.env.example" "${D}/.env.app"
  for k in POSTGRES_PASSWORD AQUIFY_OWNER_DB_PASSWORD AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_PASSWORD \
           KEYCLOAK_DB_PASSWORD REDIS_PASSWORD MINIO_ROOT_PASSWORD KEYCLOAK_ADMIN_PASSWORD GRAFANA_ADMIN_PASSWORD; do
    setkey "${D}/.env.production" "$k" "$(rand 16)"
  done
  setkey "${D}/.env.production" MINIO_ROOT_USER "minioroot$(rand 4)"
  setkey "${D}/.env.production" AQUIFY_DOMAIN "aquify-club.test"
  setkey "${D}/.env.production" ACME_EMAIL "ops@aquify-club.test"
  app_pw="$(awk -F= '$1=="AQUIFY_APP_DB_PASSWORD"{print $2}' "${D}/.env.production")"
  redis_pw="$(awk -F= '$1=="REDIS_PASSWORD"{print $2}' "${D}/.env.production")"
  setkey "${D}/.env.app" DATABASE_URL "postgres://aquify_app:${app_pw}@postgres:5432/aquify?sslmode=disable"
  setkey "${D}/.env.app" REDIS_URL "redis://:${redis_pw}@redis:6379/0"
  for k in INTEGRATIONS_ENCRYPTION_KEY OIDC_STATE_SIGNING_KEY; do setkey "${D}/.env.app" "$k" "$(rand 32)"; done
  for k in STORAGE_ACCESS_KEY STORAGE_SECRET_KEY OIDC_CLIENT_SECRET KEYCLOAK_ADMIN_CLIENT_SECRET POSTMARK_SERVER_TOKEN \
           POSTMARK_WEBHOOK_AUTH_HEADER_VALUE; do setkey "${D}/.env.app" "$k" "$(rand 16)"; done
  setkey "${D}/.env.app" STRIPE_SECRET_KEY "sk_live_$(rand 12)"
  setkey "${D}/.env.app" STRIPE_WEBHOOK_SECRET "whsec_$(rand 12)"
  setkey "${D}/.env.app" STRIPE_CONNECT_WEBHOOK_SECRET "whsec_$(rand 12)"
  # Real hostnames instead of the example.com placeholders.
  sed -i.bak 's/example\.com/aquify-club.test/g' "${D}/.env.app" && rm -f "${D}/.env.app.bak"
  chmod 640 "${D}/.env.production" "${D}/.env.app"
}

pre() { DEPLOY_DIR="${D}" "${SRC}/preflight.sh" 2>&1; }
expect_pass() { local out; out="$(pre)" || { echo "${out}"; fail "$1: expected PREFLIGHT PASSED"; }; }
expect_fail() { # expect_fail <label> <grep-pattern>
  local out
  if out="$(pre)"; then echo "${out}"; fail "$1: preflight passed but should have failed"; fi
  echo "${out}" | grep -qE "$2" || { echo "${out}"; fail "$1: failed, but not for the expected reason ($2)"; }
}

# The untouched templates must NOT pass.
D="${ROOT}/raw"; mkdir -p "${D}"
cp "${ENVDIR}/production.env.example" "${D}/.env.production"; cp "${ENVDIR}/app.env.example" "${D}/.env.app"
chmod 640 "${D}"/.env.*
expect_fail "raw templates" "placeholder values"

good ok;  expect_pass "valid set"

good perms;   chmod 644 "${D}/.env.production";                       expect_fail "world-readable secrets" "readable by other users"
good su;      setkey "${D}/.env.production" AQUIFY_APP_DB_USER aquify_admin; expect_fail "app role == superuser" "role names must all differ"
good dburole; setkey "${D}/.env.app" DATABASE_URL "postgres://aquify_owner:$(rand 16)@postgres:5432/aquify"; expect_fail "app uses the owner role" "not AQUIFY_APP_DB_USER"
good dbpw;    setkey "${D}/.env.app" DATABASE_URL "postgres://aquify_app:$(rand 16)@postgres:5432/aquify";       expect_fail "wrong db password" "does not match AQUIFY_APP_DB_PASSWORD"
good redispw; setkey "${D}/.env.app" REDIS_URL "redis://:$(rand 16)@redis:6379/0";                              expect_fail "wrong redis password" "does not carry REDIS_PASSWORD"
good http;    setkey "${D}/.env.app" API_ALLOWED_ORIGINS "http://app.aquify-club.test";                         expect_fail "http origin" "bare https origin"
good local;   setkey "${D}/.env.app" API_ALLOWED_ORIGINS "https://localhost";                                   expect_fail "loopback origin" "loopback"
good short;   setkey "${D}/.env.app" OIDC_STATE_SIGNING_KEY "tooshort";                                         expect_fail "short signing key" "at least 32"
good root;    setkey "${D}/.env.app" STORAGE_ACCESS_KEY "$(awk -F= '$1=="MINIO_ROOT_USER"{print $2}' "${D}/.env.production")"; expect_fail "root minio creds" "MinIO root user"
good nossl;   setkey "${D}/.env.app" STORAGE_USE_SSL false;                                                     expect_fail "plain-http storage" "STORAGE_USE_SSL"
good devsw;   echo "AUTH_ENABLE_DEV_STUB_TOKENS=true" >> "${D}/.env.app";                                       expect_fail "dev switch present" "development switch"
good reuse;   setkey "${D}/.env.production" REDIS_PASSWORD "$(awk -F= '$1=="POSTGRES_PASSWORD"{print $2}' "${D}/.env.production")"
              setkey "${D}/.env.app" REDIS_URL "redis://:$(awk -F= '$1=="POSTGRES_PASSWORD"{print $2}' "${D}/.env.production")@redis:6379/0"; expect_fail "reused password" "unique per service"
good missing; setkey "${D}/.env.app" STRIPE_CONNECT_WEBHOOK_SECRET "";                                          expect_fail "missing required key" "STRIPE_CONNECT_WEBHOOK_SECRET is not set"
good host1;   setkey "${D}/.env.app" STORAGE_ENDPOINT "files.other.test";                                       expect_fail "storage host off-domain" "STORAGE_ENDPOINT must be files.aquify-club.test"
good host2;   setkey "${D}/.env.app" OIDC_EXPECTED_ISSUER "https://auth.other.test/realms/aquify";            expect_fail "issuer off-domain" "OIDC_EXPECTED_ISSUER must be"
good host3;   setkey "${D}/.env.app" API_ALLOWED_ORIGINS "https://www.aquify-club.test";                       expect_fail "origin not app.<domain>" "API_ALLOWED_ORIGINS must be https://app.aquify-club.test"
good baddom;  setkey "${D}/.env.production" AQUIFY_DOMAIN "not a domain";                                      expect_fail "invalid domain" "not a valid DNS name"
good absent;  rm -f "${D}/.env.app";                                                                            expect_fail "missing file" "is missing"

good stripe;  setkey "${D}/.env.app" STRIPE_SECRET_KEY "sk_test_$(rand 12)"
              expect_fail "stripe test key" "test key"
              AQUIFY_ALLOW_STRIPE_TEST=1 DEPLOY_DIR="${D}" "${SRC}/preflight.sh" >/dev/null 2>&1 || fail "stripe test key must pass with AQUIFY_ALLOW_STRIPE_TEST=1"

echo "preflight sandbox tests passed"
