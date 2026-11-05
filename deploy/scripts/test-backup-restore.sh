#!/usr/bin/env bash
# Sandbox test for backup.sh + restore.sh with stub docker/gpg/mc/curl/rclone.
#   deploy/scripts/test-backup-restore.sh
# Proves SCRIPT LOGIC: the archive path is clean, encryption is mandatory, the
# plaintext never survives, restore verifies before it changes anything, stops every
# writer first, is all-or-nothing, and fails (never silently skips) on missing pieces.
# It does NOT prove pg_dump/pg_restore/gpg/mc behave on a real host; that is the VM
# acceptance run (a real backup restored onto a second empty VM).
set -euo pipefail
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(mktemp -d)"; trap 'chmod -R u+w "${ROOT}" 2>/dev/null || true; rm -rf "${ROOT}"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
ORIG_PATH="${PATH}"   # each sandbox starts from this, so a stub deleted in one is gone in the next

fresh() {
  T="${ROOT}/$1"; mkdir -p "${T}/bin" "${T}/aquify" "${T}/backups" "${T}/state"
  cp "${SRC}/backup.sh" "${SRC}/restore.sh" "${SRC}/lib.sh" "${T}/aquify/"
  cat > "${T}/aquify/.env.production" <<'EOF'
POSTGRES_DB=aquify
POSTGRES_USER=aquify_admin
POSTGRES_PASSWORD=superuserpasswordxx
AQUIFY_BACKUP_DB_USER=aquify_backup
AQUIFY_BACKUP_DB_PASSWORD=backuppasswordxxxxx
KEYCLOAK_ADMIN_USER=admin
KEYCLOAK_ADMIN_PASSWORD=kcadminpasswordxxxx
EOF
  cat > "${T}/aquify/.env.backup" <<EOF
MINIO_ACCESS_KEY=minioaccess
MINIO_SECRET_KEY=miniosecretvalue
GPG_RECIPIENT=ops@aquify.test
RCLONE_REMOTE=${T}/remote
EOF
  printf 'IMAGE_TAG=sha-1\nGHCR_NAMESPACE=acme\n' > "${T}/aquify/release.env"
  chmod 640 "${T}/aquify/.env.production" "${T}/aquify/.env.backup"

  # docker: one stub that understands the few compose calls the scripts make.
  cat > "${T}/bin/docker" <<'STUB'
#!/usr/bin/env bash
echo "docker $*" >> "$STUB_LOG"
args=" $* "
case "${args}" in
  *" pg_dump "*)     [ -e "$SANDBOX/dump_fail" ] && exit 1; printf 'PGDMP-synthetic-dump-bytes'; exit 0 ;;
  *" pg_restore --list"*) cat >/dev/null; [ -e "$SANDBOX/dump_unreadable" ] && exit 1; exit 0 ;;
  *" pg_restore "*)  cat >/dev/null; [ -e "$SANDBOX/restore_fail" ] && exit 1; exit 0 ;;
  *"max(version_id)"*) echo "${GOOSE_VERSION:-133}"; exit 0 ;;
  *"count(*) FROM orgs"*) echo "${ORG_COUNT:-2}"; exit 0 ;;
  *" pg_isready "*)  exit 0 ;;
  *" ps "*)          echo stub-container; exit 0 ;;
esac
exit 0
STUB
  # gpg: "encrypt" = base64, "decrypt" = base64 -d; honours --output.
  cat > "${T}/bin/gpg" <<'STUB'
#!/usr/bin/env bash
out=""; in=""; mode=""
while [ $# -gt 0 ]; do
  case "$1" in
    --encrypt) mode=enc ;; --decrypt) mode=dec ;;
    --output) out="$2"; shift ;;
    --recipient) shift ;;
    --*) ;;
    *) in="$1" ;;
  esac
  shift
done
[ -e "$SANDBOX/gpg_fail" ] && exit 2
if [ "$mode" = enc ]; then base64 < "$in" > "$out"; else base64 -d < "$in" > "$out"; fi
STUB
  # mc: mirror copies a directory; remembers calls.
  cat > "${T}/bin/mc" <<'STUB'
#!/usr/bin/env bash
echo "mc $*" >> "$STUB_LOG"
if [ "$1" = mirror ]; then
  src=""; dst=""
  for a in "$@"; do case "$a" in mirror|--preserve|--overwrite) ;; *) if [ -z "$src" ]; then src="$a"; else dst="$a"; fi ;; esac; done
  case "$src" in aquifysrc/*) mkdir -p "$dst"; echo "object-bytes" > "$dst/doc.pdf" ;; esac
fi
exit 0
STUB
  cat > "${T}/bin/curl" <<'STUB'
#!/usr/bin/env bash
out=""; prev=""
for a in "$@"; do [ "$prev" = "-o" ] && out="$a"; prev="$a"; done
case " $* " in
  *"openid-connect/token"*)
    # Real Keycloak answers unsupported_grant_type when the body ends in a newline
    # (what a heredoc produced); curl -f then fails, exactly like here.
    body="$(cat; echo x)"; body="${body%x}"
    case "${body}" in *grant_type=password) echo '{"access_token":"stub-token"}' ;; *) exit 22 ;; esac ;;
  *) [ -n "$out" ] && echo '{"realm":"aquify"}' > "$out" ;;
esac
exit 0
STUB
  # rclone: remotes are local directories. copy <file> <dir>/ ; copyto <file> <file> ; delete is a no-op.
  cat > "${T}/bin/rclone" <<'STUB'
#!/usr/bin/env bash
echo "rclone $*" >> "$STUB_LOG"
[ -e "$SANDBOX/rclone_fail" ] && exit 1
case "$1" in
  copy)   mkdir -p "$3" && cp "$2" "$3" ;;
  copyto) [ -f "$2" ] || exit 1; mkdir -p "$(dirname "$3")" && cp "$2" "$3" ;;
  delete) case "$2$3" in *weekly*|*monthly*) exit 3 ;; esac; exit 0 ;;   # tier folder not written yet: rclone exit 3
esac
exit 0
STUB
  chmod +x "${T}/bin/"*
  : > "${T}/stub.log"
  export SANDBOX="${T}" STUB_LOG="${T}/stub.log" DEPLOY_DIR="${T}/aquify" BACKUP_DIR="${T}/backups" \
    BACKUP_LOG="${T}/backup.log" RESTORE_LOG="${T}/restore.log" DEPLOY_LOG="${T}/deploy.log" \
    NODE_EXPORTER_TEXTFILE_DIR="${T}/nodeexp" LOCK_DIR="${T}/aquify/.deploy.lock" \
    HEALTH_TIMEOUT_SECONDS=1 HEALTH_RETRY_INTERVAL=1 PATH="${T}/bin:${ORIG_PATH}"
  unset ALLOW_UNENCRYPTED_BACKUP GOOSE_VERSION ORG_COUNT || true
  # curl is also used by wait_for_ready; make readiness return 200 after the token calls.
  mkdir -p "${T}/nodeexp"
}
run_backup()  { "${T}/aquify/backup.sh"  >"${T}/b.out" 2>&1; }
run_restore() { "${T}/aquify/restore.sh" "$@" >"${T}/r.out" 2>&1; }
ready_ok() { # curl for the readiness probe: HTTP 200
  cat > "${T}/bin/curl" <<'STUB'
#!/usr/bin/env bash
case " $* " in
  *"-w"*) printf 200 ;;
  *"openid-connect/token"*)
    # Real Keycloak answers unsupported_grant_type when the body ends in a newline
    # (what a heredoc produced); curl -f then fails, exactly like here.
    body="$(cat; echo x)"; body="${body%x}"
    case "${body}" in *grant_type=password) echo '{"access_token":"stub-token"}' ;; *) exit 22 ;; esac ;;
  *) out=""; prev=""; for a in "$@"; do [ "$prev" = "-o" ] && out="$a"; prev="$a"; done; [ -n "$out" ] && echo '{}' > "$out" ;;
esac
exit 0
STUB
}
archives() { ls "${T}/backups" | grep -E '^aquify-backup-.*\.tar\.gz(\.gpg)?$' || true; }
first_line() { grep -n "$1" "${T}/stub.log" | head -1 | cut -d: -f1 || true; }

# ===== BACKUP ============================================================
fresh ok
run_backup || { cat "${T}/b.out"; fail "backup should succeed"; }
A="$(archives)"
[ "$(echo "${A}" | wc -l | tr -d ' ')" = 1 ] || fail "expected exactly one archive, got: ${A}"
[[ "${A}" == *.tar.gz.gpg ]] || fail "archive must be encrypted, got ${A}"
[ -z "$(find "${T}/backups" -mindepth 1 -maxdepth 1 -type d)" ] || fail "plaintext snapshot directory left behind"
ls "${T}/backups" | grep -qE '\.tar\.gz$' && fail "plaintext tarball left behind"
# the archive holds dump + manifest + objects + realm
mkdir -p "${T}/x"; base64 -d < "${T}/backups/${A}" | tar -xz -C "${T}/x"
SNAPDIR="$(find "${T}/x" -mindepth 1 -maxdepth 1 -type d)"
for f in postgres.dump manifest.txt minio/doc.pdf keycloak-realm-aquify.json; do [ -e "${SNAPDIR}/${f}" ] || fail "archive is missing ${f}"; done
grep -q '^goose_version=133$' "${SNAPDIR}/manifest.txt" || fail "manifest lacks the schema version"
grep -q '^org_count=2$' "${SNAPDIR}/manifest.txt" || fail "manifest lacks the club count"
# passwords never on a command line
grep -q 'backuppasswordxxxxx' "${T}/stub.log" && fail "backup password leaked into a command line"
grep -q 'superuserpasswordxx' "${T}/stub.log" && fail "superuser password leaked into a command line"
grep -q 'miniosecretvalue' "${T}/stub.log" && fail "minio secret leaked into mc arguments"
[ -f "${T}/nodeexp/aquify_backup.prom" ] || fail "success metric not written"
ls "${T}"/remote/*/"${A}" >/dev/null 2>&1 || fail "archive was not uploaded offsite"
fresh offsitefail; touch "${T}/rclone_fail"; run_backup && fail "an offsite upload failure must fail the run"
fresh noremote
printf 'MINIO_ACCESS_KEY=a\nMINIO_SECRET_KEY=b\nGPG_RECIPIENT=x\n' > "${T}/aquify/.env.backup"
run_backup && fail "no offsite remote must fail the run"
grep -q 'on this host only' "${T}/b.out" || fail "missing the host-only reason"

fresh nogpg
printf 'MINIO_ACCESS_KEY=a\nMINIO_SECRET_KEY=b\nRCLONE_REMOTE=%s/remote\n' "${T}" > "${T}/aquify/.env.backup"
run_backup && fail "backup without GPG_RECIPIENT must fail"
[ -z "$(archives)" ] || fail "an archive was written without encryption"
grep -q 'refusing to write an unencrypted backup' "${T}/b.out" || fail "missing the encryption reason"

fresh rehearsal
printf 'MINIO_ACCESS_KEY=a\nMINIO_SECRET_KEY=b\nRCLONE_REMOTE=%s/remote\n' "${T}" > "${T}/aquify/.env.backup"
ALLOW_UNENCRYPTED_BACKUP=1 run_backup || { cat "${T}/b.out"; fail "explicit rehearsal override should work"; }
[[ "$(archives)" == *.tar.gz ]] || fail "rehearsal archive missing"
grep -q 'NOT encrypted' "${T}/b.out" || fail "the unencrypted override must be loudly logged"

fresh gpgfail;   touch "${T}/gpg_fail";        run_backup && fail "gpg failure must fail the run"; [ -z "$(archives)" ] || fail "archive kept after gpg failure"
fresh dumpfail;  touch "${T}/dump_fail";       run_backup && fail "pg_dump failure must fail the run"; [ -z "$(archives)" ] || fail "archive written without a database dump"
fresh unreadable; touch "${T}/dump_unreadable"; run_backup && fail "an unreadable dump must fail the run"; [ -z "$(archives)" ] || fail "archive written from an unreadable dump"
fresh nomc;      rm "${T}/bin/mc";              run_backup && fail "missing mc must fail the run (no silent skip)"
fresh nokcpass;  grep -v KEYCLOAK_ADMIN_PASSWORD "${T}/aquify/.env.production" > "${T}/e" && cat "${T}/e" > "${T}/aquify/.env.production"
                 run_backup && fail "missing Keycloak admin password must fail the run"

# ===== RESTORE (round trip of a real backup.sh archive) ===============================
fresh rt
run_backup || { cat "${T}/b.out"; fail "setup backup failed"; }
ready_ok
TS="$(archives | sed -E 's/aquify-backup-(.*)\.tar\.gz\.gpg/\1/')"
: > "${T}/stub.log"
run_restore "${TS}" --yes || { cat "${T}/r.out"; fail "restore of a fresh backup should succeed"; }
stop_n="$(first_line ' stop ')"; rest_n="$(first_line 'pg_restore -U aquify_admin')"; mig_n="$(first_line 'aquify-migrate')"; up_n="$(first_line ' up -d --no-deps aquify-api')"
{ [ -n "${stop_n}" ] && [ -n "${rest_n}" ] && [ -n "${mig_n}" ] && [ -n "${up_n}" ] && [ "${stop_n}" -lt "${rest_n}" ] && [ "${rest_n}" -lt "${mig_n}" ] && [ "${mig_n}" -lt "${up_n}" ]; } \
  || fail "expected stop < pg_restore < migrate < start, got ${stop_n:-?} ${rest_n:-?} ${mig_n:-?} ${up_n:-?}"
for w in aquify-api aquify-worker aquify-admin keycloak; do grep ' stop ' "${T}/stub.log" | grep -q "${w}" || fail "restore did not stop ${w}"; done
grep -q ' up -d keycloak caddy' "${T}/stub.log" || fail "restore must start the ingress with keycloak (the API validates JWKS through it at startup)"
grep 'pg_restore -U' "${T}/stub.log" | grep -q -- '--single-transaction' || fail "restore must be a single transaction"
grep 'pg_restore -U' "${T}/stub.log" | grep -q -- '--exit-on-error' || fail "restore must stop on the first error"
grep -q 'mc mirror --overwrite --preserve' "${T}/stub.log" || fail "objects were not restored"
grep -q 'superuserpasswordxx' "${T}/stub.log" && fail "superuser password leaked into a command line"
[ -z "$(find "${T}/backups" -mindepth 1 -maxdepth 1 -name '.restore-*')" ] || fail "plaintext restore work dir left behind"

restore_case() { # restore_case <label> <setup-cmd> <reason-regex> <must-not-run-regex>
  fresh "rc-$1"; run_backup || fail "$1: setup backup failed"; ready_ok
  local ts; ts="$(archives | sed -E 's/aquify-backup-(.*)\.tar\.gz\.gpg/\1/')"; : > "${T}/stub.log"
  eval "$2"
  run_restore "${ts}" --yes && fail "$1: restore should have failed"
  grep -qE "$3" "${T}/r.out" || { cat "${T}/r.out"; fail "$1: failed, but not for the expected reason ($3)"; }
  [ -z "$4" ] || ! grep -qE "$4" "${T}/stub.log" || fail "$1: ran a step that must not run after this failure ($4)"
  [ -z "$(find "${T}/backups" -mindepth 1 -maxdepth 1 -name '.restore-*')" ] || fail "$1: plaintext work dir left behind"
}
restore_case unreadable 'touch "${T}/dump_unreadable"' 'unreadable' ' stop |pg_restore -U'
restore_case pgfail 'touch "${T}/restore_fail"' 'pg_restore failed' 'aquify-migrate|up -d --no-deps aquify-api'
restore_case badversion 'export GOOSE_VERSION=99' 'schema version after restore is 99' 'aquify-migrate|up -d --no-deps aquify-api'
restore_case badorgs 'export ORG_COUNT=7' 'club count after restore is 7' 'aquify-migrate|up -d --no-deps aquify-api'
restore_case nomc 'rm "${T}/bin/mc"' "'mc' is required" ' stop |pg_restore -U'
restore_case gpgfail 'touch "${T}/gpg_fail"' 'gpg decryption failed' ' stop |pg_restore -U'

fresh offsiterestore; run_backup >/dev/null 2>&1; ready_ok
ts="$(archives | sed -E 's/aquify-backup-(.*)\.tar\.gz\.gpg/\1/')"
mv "${T}/backups/aquify-backup-${ts}.tar.gz.gpg" "${T}/held-aside.gpg"   # local copy gone: only the remote has it
: > "${T}/stub.log"
run_restore "${ts}" --yes || { cat "${T}/r.out"; fail "restore from the offsite copy should succeed"; }
grep -q 'rclone copyto' "${T}/stub.log" || fail "the offsite archive was not fetched"

fresh noarchive; ready_ok
run_restore 19990101-000000 --yes && fail "restore of a missing archive must fail"
grep -q 'no archive found' "${T}/r.out" || fail "missing-archive reason not reported"

fresh confirm; run_backup >/dev/null 2>&1; ready_ok
ts="$(archives | sed -E 's/aquify-backup-(.*)\.tar\.gz\.gpg/\1/')"; : > "${T}/stub.log"
echo "no" | run_restore "${ts}" && fail "restore without the typed confirmation must abort"
grep -q ' stop ' "${T}/stub.log" && fail "services were stopped before confirmation"

fresh skipobj; run_backup >/dev/null 2>&1; ready_ok
ts="$(archives | sed -E 's/aquify-backup-(.*)\.tar\.gz\.gpg/\1/')"; rm "${T}/bin/mc"
run_restore "${ts}" --yes --skip-objects || { cat "${T}/r.out"; fail "--skip-objects should allow a database-only restore"; }
grep -q 'stored files were NOT restored' "${T}/r.out" || fail "skipping objects must be logged loudly"

echo "backup/restore sandbox tests passed"
