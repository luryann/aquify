#!/usr/bin/env bash
# Sandbox test for promote.sh + rollback.sh: runs them against a temp deploy dir
# with stub docker/curl, so no server is needed.
#   deploy/scripts/test-promote-rollback.sh
set -euo pipefail
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "${T}"' EXIT
mkdir -p "${T}/bin" "${T}/dare"
cp "${SRC}/promote.sh" "${SRC}/rollback.sh" "${T}/dare/"
printf '#!/usr/bin/env bash\necho "docker $* [IMAGE_TAG=${IMAGE_TAG:-}]" >> "$STUB_LOG"\n' > "${T}/bin/docker"
printf '#!/usr/bin/env bash\ncat "$HEALTH_STATUS_FILE"\n' > "${T}/bin/curl"
chmod +x "${T}/bin/"*
printf 'IMAGE_TAG=sha-good1\nFOO=bar\n' > "${T}/dare/.env.production"
export STUB_LOG="${T}/stub.log" HEALTH_STATUS_FILE="${T}/status" DEPLOY_DIR="${T}/dare" DEPLOY_LOG="${T}/deploy.log" \
  HEALTH_TIMEOUT_SECONDS=1 HEALTH_RETRY_INTERVAL=1 PATH="${T}/bin:${PATH}"
fail() { echo "FAIL: $*" >&2; exit 1; }
tag() { grep '^IMAGE_TAG=' "$1" | cut -d= -f2; }

echo 200 > "${T}/status"
"${T}/dare/promote.sh" sha-good2 >/dev/null 2>&1 || fail "healthy promote should succeed"
[ "$(tag "${T}/dare/.env.production")" = sha-good2 ] || fail "env not promoted"
[ "$(tag "${T}/dare/.env.production.good")" = sha-good2 ] || fail "good not recorded after health"
[ "$(tag "${T}/dare/.env.production.prev")" = sha-good1 ] || fail "prev should be the old good tag"

echo 500 > "${T}/status"
for attempt in 1 2; do
  "${T}/dare/promote.sh" sha-bad >/dev/null 2>&1 && fail "unhealthy promote should fail"
  [ "$(tag "${T}/dare/.env.production")" = sha-good2 ] || fail "attempt ${attempt}: rollback did not restore the good env"
  [ "$(tag "${T}/dare/.env.production.prev")" = sha-good2 ] || fail "attempt ${attempt}: rollback target was poisoned"
done

"${T}/dare/promote.sh" 'x|s&y' >/dev/null 2>&1 && fail "hostile tag must be rejected"
[ "$(grep -c '^FOO=bar' "${T}/dare/.env.production")" = 1 ] || fail "other env lines must survive"
echo "promote/rollback sandbox tests passed"
