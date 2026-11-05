#!/usr/bin/env bash
# Sandbox test for promote.sh + rollback.sh + lib.sh: runs them against a temp
# deploy dir with stub docker/curl, so no server or Docker is needed.
#   deploy/scripts/test-promote-rollback.sh
# It proves script LOGIC (ordering, env-file handling, failure paths). It does not
# prove that real Docker Compose accepts the commands; that is the VM acceptance run.
set -euo pipefail
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(mktemp -d)"; trap 'chmod -R u+w "${ROOT}" 2>/dev/null || true; rm -rf "${ROOT}"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

# fresh <name>: a new sandbox with scripts, stubs and a read-only secrets file.
fresh() {
  T="${ROOT}/$1"; mkdir -p "${T}/bin" "${T}/aquify"
  cp "${SRC}/promote.sh" "${SRC}/rollback.sh" "${SRC}/lib.sh" "${T}/aquify/"
  cat > "${T}/bin/docker" <<'STUB'
#!/usr/bin/env bash
echo "docker $*" >> "$STUB_LOG"
case " $* " in
  *" aquify-migrate "*) [ -e "$SANDBOX/migrate_fail" ] && exit 1 ;;
  *" pull "*) [ -e "$SANDBOX/pull_fail" ] && exit 1 ;;
  *" ps "*) [ -e "$SANDBOX/worker_down" ] || echo stub-container-id ;;
esac
exit 0
STUB
  printf '#!/usr/bin/env bash\ncat "$HEALTH_STATUS_FILE"\n' > "${T}/bin/curl"
  chmod +x "${T}/bin/"*
  printf 'POSTGRES_PASSWORD=not-a-real-secret\n' > "${T}/aquify/.env.production"
  chmod 440 "${T}/aquify/.env.production"   # root-owned in production: scripts must only read it
  printf 'IMAGE_TAG=sha-good1\nGHCR_NAMESPACE=acme\n' > "${T}/aquify/release.env"
  echo 200 > "${T}/status"
  : > "${T}/stub.log"
  export SANDBOX="${T}" STUB_LOG="${T}/stub.log" HEALTH_STATUS_FILE="${T}/status" DEPLOY_DIR="${T}/aquify" \
    DEPLOY_LOG="${T}/deploy.log" HEALTH_TIMEOUT_SECONDS=1 HEALTH_RETRY_INTERVAL=1 PATH="${T}/bin:${PATH}"
}
tag() { grep '^IMAGE_TAG=' "$1" | cut -d= -f2; }
run_promote() { "${T}/aquify/promote.sh" "$1" >"${T}/out.log" 2>&1; }

# 1. Healthy promote -------------------------------------------------------
fresh healthy
run_promote sha-good2 || { cat "${T}/out.log"; fail "healthy promote should succeed"; }
[ "$(tag "${T}/aquify/release.env")" = sha-good2 ] || fail "release.env not promoted"
[ "$(tag "${T}/aquify/release.env.good")" = sha-good2 ] || fail "good not recorded after health"
[ "$(tag "${T}/aquify/release.env.prev")" = sha-good1 ] || fail "prev should be the old good tag"
grep -q '^GHCR_NAMESPACE=acme$' "${T}/aquify/release.env" || fail "namespace not preserved"
[ "$(cat "${T}/aquify/.env.production")" = 'POSTGRES_PASSWORD=not-a-real-secret' ] || fail "secrets file was modified"
# every compose call carries both env files (interpolation does not come from env_file)
while read -r line; do
  case "${line}" in
    "docker compose "*) [[ "${line}" == *"--env-file ${T}/aquify/.env.production --env-file ${T}/aquify/release.env"* ]] \
      || fail "compose call without both env files: ${line}" ;;
  esac
done < "${T}/stub.log"
# ordering: pull < migrate < up
pull_n="$(grep -n ' pull ' "${T}/stub.log" | head -1 | cut -d: -f1 || true)"
mig_n="$(grep -n 'aquify-migrate' "${T}/stub.log" | head -1 | cut -d: -f1 || true)"
up_n="$(grep -n ' up -d ' "${T}/stub.log" | head -1 | cut -d: -f1 || true)"
{ [ -n "${pull_n}" ] && [ -n "${mig_n}" ] && [ -n "${up_n}" ] && [ "${pull_n}" -lt "${mig_n}" ] && [ "${mig_n}" -lt "${up_n}" ]; } \
  || fail "expected pull < migrate < up, got ${pull_n:-?} ${mig_n:-?} ${up_n:-?}"
grep -q -- '--profile tools run --rm aquify-migrate' "${T}/stub.log" || fail "migrate service not run via the tools profile"
[ ! -d "${T}/aquify/.deploy.lock" ] || fail "deploy lock left behind"

# 2. Failed migration: abort before touching running containers --------------
fresh migfail
touch "${T}/migrate_fail"
run_promote sha-bad && fail "failed migration must fail the promote"
grep -q ' up -d ' "${T}/stub.log" && fail "containers were recreated despite a failed migration"
[ "$(tag "${T}/aquify/release.env")" = sha-good1 ] || fail "release.env not restored after failed migration"

# 3. Unhealthy API: rollback restores the good release, twice in a row --------
fresh unhealthy
echo 500 > "${T}/status"
for attempt in 1 2; do
  run_promote sha-bad && fail "unhealthy promote should fail"
  [ "$(tag "${T}/aquify/release.env")" = sha-good1 ] || fail "attempt ${attempt}: rollback did not restore the good release"
  [ "$(tag "${T}/aquify/release.env.prev")" = sha-good1 ] || fail "attempt ${attempt}: rollback target was poisoned"
done

# 4. Worker down after the swap is a failed deploy ----------------------------
fresh workerdown
touch "${T}/worker_down"
run_promote sha-bad && fail "a dead worker must fail the promote"

# 5. Pull failure leaves the release untouched --------------------------------
fresh pullfail
touch "${T}/pull_fail"
run_promote sha-bad && fail "pull failure must fail the promote"
[ "$(tag "${T}/aquify/release.env")" = sha-good1 ] || fail "release.env changed after a failed pull"

# 6. Lock: a live holder blocks a second deploy; a dead holder is taken over ---
fresh locking
mkdir "${T}/aquify/.deploy.lock"; echo "$$" > "${T}/aquify/.deploy.lock/pid"
run_promote sha-good2 && fail "second deploy must be refused while the lock is held"
grep -q 'Another deploy/rollback is running' "${T}/deploy.log" || fail "lock refusal not logged"
echo 999999 > "${T}/aquify/.deploy.lock/pid"
run_promote sha-good2 || { cat "${T}/out.log"; fail "stale lock should be taken over"; }

# 7. Input validation ---------------------------------------------------------
fresh validation
run_promote 'x|s&y' && fail "hostile tag must be rejected"
run_promote '' && fail "empty tag must be rejected"
[ "$(tag "${T}/aquify/release.env")" = sha-good1 ] || fail "release.env changed by a rejected tag"

# 8. First deploy needs a namespace, then works --------------------------------
fresh firstdeploy
rm "${T}/aquify/release.env"
run_promote sha-first && fail "first deploy without GHCR_NAMESPACE must fail"
GHCR_NAMESPACE=acme run_promote sha-first || { cat "${T}/out.log"; fail "first deploy with namespace should succeed"; }
[ "$(tag "${T}/aquify/release.env")" = sha-first ] || fail "first deploy tag not written"

echo "promote/rollback sandbox tests passed"
