#!/usr/bin/env bash
# rehearse-local.sh — stage a production-shaped deployment directory for a LOCAL
# rehearsal (Colima/Docker Desktop, synthetic data, no public DNS).
#
#   . .tools/docker/env.sh            # docker + compose + colima from the repo
#   deploy/scripts/rehearse-local.sh /path/to/empty/dir   # prints next steps
#
# What it does: copies the deploy bundle, generates synthetic secrets from the real
# templates, renders the production Keycloak realm, runs preflight.sh, and writes a
# compose override (rehearsal.override.yml) that is NEVER part of the production files:
#   * Caddy uses its internal CA (`tls internal`) instead of ACME
#   * app./api./auth./files.<domain> resolve inside the network via a caddy alias, and the
#     api/worker trust Caddy's CA, so server-side OIDC discovery works
#   * the schema-migrating, Keycloak and admin images come from local builds
# Colima/Lima share only $HOME with the VM: stage the directory under $HOME, not /tmp, or every
# bind mount (init scripts, Caddyfile, loki.yml) arrives as an empty directory.
# The domain is *.localtest.me (public DNS name that always resolves to 127.0.0.1).
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "${HERE}/../.." && pwd)"
OUT="${1:?usage: rehearse-local.sh <empty-output-dir>}"
DOMAIN="${REHEARSAL_DOMAIN:-club.localtest.me}"
TAG="${REHEARSAL_IMAGE_TAG:-rehearsal}"
[ -z "$(ls -A "${OUT}" 2>/dev/null || true)" ] || { echo "refusing: ${OUT} is not empty" >&2; exit 1; }
mkdir -p "${OUT}/docs"
rsync -a --exclude 'scripts/test-*' "${REPO}/deploy/" "${OUT}/"
rsync -a "${REPO}/docs/observability/" "${OUT}/docs/observability/"

rand() { openssl rand -hex "${1:-16}"; }
setkey() { awk -v k="$2" -v v="$3" 'BEGIN{FS=OFS="="} $1==k{print k"="v; f=1; next} {print} END{if(!f) print k"="v}' "$1" > "$1.tmp" && cat "$1.tmp" > "$1" && rm -f "$1.tmp"; }
getkey() { awk -F= -v k="$2" '$1==k{print substr($0,length(k)+2)}' "$1"; }

P="${OUT}/.env.production"; A="${OUT}/.env.app"
cp "${REPO}/deploy/env/production.env.example" "${P}"
cp "${REPO}/deploy/env/app.env.example" "${A}"
for k in POSTGRES_PASSWORD AQUIFY_OWNER_DB_PASSWORD AQUIFY_APP_DB_PASSWORD AQUIFY_BACKUP_DB_PASSWORD \
         KEYCLOAK_DB_PASSWORD REDIS_PASSWORD MINIO_ROOT_PASSWORD KEYCLOAK_ADMIN_PASSWORD GRAFANA_ADMIN_PASSWORD; do
  setkey "${P}" "$k" "$(rand 16)"
done
setkey "${P}" MINIO_ROOT_USER "minioroot$(rand 4)"
setkey "${P}" AQUIFY_DOMAIN "${DOMAIN}"
setkey "${P}" ACME_EMAIL "ops@${DOMAIN}"
setkey "${A}" DATABASE_URL "postgres://aquify_app:$(getkey "${P}" AQUIFY_APP_DB_PASSWORD)@postgres:5432/aquify?sslmode=disable"
setkey "${A}" REDIS_URL "redis://:$(getkey "${P}" REDIS_PASSWORD)@redis:6379/0"
for k in INTEGRATIONS_ENCRYPTION_KEY OIDC_STATE_SIGNING_KEY; do setkey "${A}" "$k" "$(rand 32)"; done
for k in OIDC_CLIENT_SECRET KEYCLOAK_ADMIN_CLIENT_SECRET POSTMARK_SERVER_TOKEN POSTMARK_WEBHOOK_AUTH_HEADER_VALUE; do
  setkey "${A}" "$k" "$(rand 16)"
done
# Scoped MinIO service-account keys; created in MinIO by the "next steps" below.
setkey "${A}" STORAGE_ACCESS_KEY "aquifyapp$(rand 4)"
setkey "${A}" STORAGE_SECRET_KEY "$(rand 20)"
setkey "${A}" STRIPE_SECRET_KEY "sk_test_$(rand 12)"
setkey "${A}" STRIPE_WEBHOOK_SECRET "whsec_$(rand 12)"
setkey "${A}" STRIPE_CONNECT_WEBHOOK_SECRET "whsec_$(rand 12)"
sed -i.bak "s/example\.com/${DOMAIN}/g" "${A}" && rm -f "${A}.bak"
chmod 640 "${P}" "${A}"
printf 'IMAGE_TAG=%s\nGHCR_NAMESPACE=local\n' "${TAG}" > "${OUT}/release.env"

python3 -I "${OUT}/keycloak/render_realm.py" --app-env "${A}" --out "${OUT}/keycloak/production-realm.json"

cat > "${OUT}/rehearsal.override.yml" <<YML
# LOCAL REHEARSAL ONLY. Not a production file.
services:
  caddy:
    command: caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
    volumes:
      - ./caddy/Caddyfile.rehearsal:/etc/caddy/Caddyfile:ro
    networks:
      aquify-net:
        aliases: [files.${DOMAIN}, auth.${DOMAIN}, app.${DOMAIN}, api.${DOMAIN}]
  aquify-api:    &trust
    volumes: [ "./caddy-root.crt:/caddy-root.crt:ro" ]
    environment: { SSL_CERT_FILE: /caddy-root.crt }
  aquify-worker: *trust
YML
# Same routes, internal CA, no ACME.
{ echo "{"; echo "	local_certs"; echo "	skip_install_trust"; echo "}"; sed '/^{$/,/^}$/d' "${OUT}/caddy/Caddyfile"; } > "${OUT}/caddy/Caddyfile.rehearsal"

echo "== preflight =="
AQUIFY_ALLOW_STRIPE_TEST=1 DEPLOY_DIR="${OUT}" "${OUT}/scripts/preflight.sh" || echo "(preflight reported problems above)"
echo
echo "staged ${OUT}. Next (see deploy/README.md for the real-host equivalents):"
echo "  cd ${OUT}; C='docker compose --project-directory . --env-file .env.production --env-file release.env -f docker-compose.production.yml -f rehearsal.override.yml'"
echo "  \$C up -d postgres redis minio caddy      # then create the bucket + scoped MinIO service account"
echo "  \$C exec caddy cat /data/caddy/pki/authorities/local/root.crt > caddy-root.crt   # the app containers trust this CA"
echo "  \$C --profile tools run --rm aquify-migrate && \$C up -d"
