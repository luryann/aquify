#!/usr/bin/env bash
# Native (no Docker, no Homebrew, nothing system-level) third-party integrations,
# installed and run entirely from the gitignored .tools/ folder.
#
# Usage: scripts/integrations-local.sh {install|up|down|status|migrate}
#
# Services (all bound to 127.0.0.1):
#   PostgreSQL 5432 | Redis 6379 | MinIO 9000 (console 9001) | Mailpit SMTP 1025 / UI 8025 | Keycloak 8180
#
# Env vars the API needs to use these services (put in .env.local, APP_ENV=development):
#   DATABASE_URL=postgres://postgres@localhost:5432/aquify?sslmode=disable   (trust auth, no password)
#   REDIS_URL=redis://127.0.0.1:6379
#   STORAGE_ENDPOINT=127.0.0.1:9000 STORAGE_ACCESS_KEY=minioadmin STORAGE_SECRET_KEY=minioadmin
#   STORAGE_BUCKET=<bucket; create with: MC_CONFIG_DIR=.tools/run/mc .tools/minio/bin/mc ...>
#   OIDC_* : issuer http://localhost:8180/realms/aquify, realm aquify,
#            client id aquify-id, client secret aquify-local-dev-secret
#   KEYCLOAK_ADMIN_* : service-account client aquify-admin, secret aquify-local-admin-secret
#   POSTMARK_SERVER_TOKEN=POSTMARK_API_TEST  (Postmark's sandbox token; accepts sends, delivers nothing)
#   STRIPE_*: a real sk_test_... key is required for positive payment paths; the stripe CLI is
#             installed at .tools/stripe-cli/stripe for `listen`/webhook forwarding.
# Mail sent by Keycloak (realm SMTP localhost:1025) is captured in Mailpit.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS="$ROOT/.tools"
RUN="$TOOLS/run"
LOGS="$RUN/logs"
PIDS="$RUN/pids"
KEYCLOAK_VERSION="${KEYCLOAK_VERSION:-$(sed -n 's/^KEYCLOAK_VERSION ?= *//p' "$ROOT/Makefile" | head -1)}"
KEYCLOAK_VERSION="${KEYCLOAK_VERSION:-26.5.6}"
KC_DIR="$TOOLS/keycloak-$KEYCLOAK_VERSION"
REDIS_VERSION="${REDIS_VERSION:-7.4.2}"
GOOSE_VERSION="${GOOSE_VERSION:-v3.24.1}"

GO="$TOOLS/go/bin/go"
PG_BIN="$TOOLS/pg/bin"
PGDATA="$RUN/pgdata"
REDIS_BIN="$TOOLS/redis/bin"
MINIO_BIN="$TOOLS/minio/bin"
MAILPIT="$TOOLS/mailpit/mailpit"
STRIPE="$TOOLS/stripe-cli/stripe"
GOOSE="$TOOLS/bin/goose"

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux) OS=linux ;;
  *) echo "unsupported OS $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64|amd64) ARCH=amd64 ;;
  *) echo "unsupported arch $(uname -m)" >&2; exit 1 ;;
esac

if [ "$OS" = darwin ]; then
  JAVA_HOME_DIR="$TOOLS/jdk/Contents/Home"
else
  JAVA_HOME_DIR="$TOOLS/jdk"
fi

export GOMODCACHE="$TOOLS/gomodcache"
export GOCACHE="$TOOLS/gocache"

log() { printf '[integrations] %s\n' "$*"; }
mkdir -p "$LOGS" "$PIDS"

listening() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

wait_for() { # name timeout cmd...
  local name=$1 timeout=$2; shift 2
  local i=0
  while ! "$@" >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -ge "$timeout" ]; then log "$name did not become ready in ${timeout}s (see $LOGS)"; return 1; fi
    sleep 1
  done
  log "$name ready"
}

spawn() { # name logfile cmd...  (fully detached; PID recorded)
  local name=$1 logfile=$2; shift 2
  nohup "$@" </dev/null >>"$logfile" 2>&1 &
  echo $! >"$PIDS/$name.pid"
}

kill_pidfile() {
  local name=$1 f="$PIDS/$1.pid" pid
  [ -f "$f" ] || { log "$name: no pid file"; return 0; }
  pid=$(cat "$f")
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    for _ in $(seq 1 30); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
    kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
    log "$name stopped"
  else
    log "$name: stale pid file"
  fi
  rm -f "$f"
}

# ---------------------------------------------------------------- install

install_jdk() {
  [ -x "$JAVA_HOME_DIR/bin/java" ] && { log "jdk present"; return; }
  local jos=$OS jarch
  [ "$OS" = darwin ] && jos=mac
  jarch=aarch64; [ "$ARCH" = amd64 ] && jarch=x64
  log "downloading Temurin 21 JDK"
  mkdir -p "$TOOLS/jdk"
  curl -fL "https://api.adoptium.net/v3/binary/latest/21/ga/$jos/$jarch/jdk/hotspot/normal/eclipse" | tar -xz --strip-components=1 -C "$TOOLS/jdk"
}

install_keycloak() {
  if [ ! -x "$KC_DIR/bin/kc.sh" ]; then
    log "downloading Keycloak $KEYCLOAK_VERSION"
    curl -fL -o "$TOOLS/keycloak-$KEYCLOAK_VERSION.zip" "https://github.com/keycloak/keycloak/releases/download/$KEYCLOAK_VERSION/keycloak-$KEYCLOAK_VERSION.zip"
    unzip -q "$TOOLS/keycloak-$KEYCLOAK_VERSION.zip" -d "$TOOLS"
    rm -f "$TOOLS/keycloak-$KEYCLOAK_VERSION.zip"
  else
    log "keycloak present"
  fi
  rm -rf "$KC_DIR/themes/aquify"
  cp -R "$ROOT/deploy/keycloak/themes/aquify" "$KC_DIR/themes/aquify"
}

install_redis() {
  [ -x "$REDIS_BIN/redis-server" ] && [ -x "$REDIS_BIN/redis-cli" ] && { log "redis present"; return; }
  local src="$TOOLS/redis-src"
  if [ ! -x "$src/src/redis-server" ]; then
    log "building Redis $REDIS_VERSION"
    mkdir -p "$src"
    curl -fL "https://download.redis.io/releases/redis-$REDIS_VERSION.tar.gz" | tar -xz --strip-components=1 -C "$src"
    # make can linger after finishing; poll for the binaries instead of trusting exit.
    (cd "$src" && nohup make -j BUILD_TLS=no </dev/null >"$LOGS/redis-build.log" 2>&1 &)
    for _ in $(seq 1 600); do
      [ -x "$src/src/redis-server" ] && [ -x "$src/src/redis-cli" ] && break
      sleep 1
    done
    sleep 2
  fi
  [ -x "$src/src/redis-server" ] || { log "redis build failed (see $LOGS/redis-build.log)"; return 1; }
  mkdir -p "$REDIS_BIN"
  cp "$src/src/redis-server" "$src/src/redis-cli" "$REDIS_BIN/"
}

install_minio() {
  [ -x "$MINIO_BIN/minio" ] && [ -x "$MINIO_BIN/mc" ] && { log "minio present"; return; }
  # dl.min.io returns 410 Gone (MinIO OSS archived), so build from source.
  log "building minio + mc from source (go install)"
  mkdir -p "$MINIO_BIN"
  GOBIN="$MINIO_BIN" "$GO" install github.com/minio/minio@latest
  GOBIN="$MINIO_BIN" "$GO" install github.com/minio/mc@latest
}

install_mailpit() {
  [ -x "$MAILPIT" ] && { log "mailpit present"; return; }
  log "downloading mailpit"
  mkdir -p "$TOOLS/mailpit"
  curl -fL "https://github.com/axllent/mailpit/releases/latest/download/mailpit-$OS-$ARCH.tar.gz" | tar -xz -C "$TOOLS/mailpit" mailpit
}

install_stripe() {
  [ -x "$STRIPE" ] && { log "stripe cli present"; return; }
  local tag ver sos sarch
  tag=$(curl -fsSL https://api.github.com/repos/stripe/stripe-cli/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
  ver=${tag#v}
  [ "$OS" = darwin ] && sos=mac-os || sos=linux
  if [ "$OS" = darwin ]; then sarch=$ARCH; else sarch=x86_64; [ "$ARCH" = arm64 ] && sarch=arm64; fi
  log "downloading stripe cli $ver"
  mkdir -p "$TOOLS/stripe-cli"
  curl -fL "https://github.com/stripe/stripe-cli/releases/download/$tag/stripe_${ver}_${sos}_${sarch}.tar.gz" | tar -xz -C "$TOOLS/stripe-cli" stripe
}

install_goose() {
  [ -x "$GOOSE" ] && { log "goose present"; return; }
  log "building goose $GOOSE_VERSION"
  mkdir -p "$TOOLS/bin"
  GOBIN="$TOOLS/bin" "$GO" install "github.com/pressly/goose/v3/cmd/goose@$GOOSE_VERSION"
}

cmd_install() {
  [ -x "$GO" ] || { log "repo-local Go missing at $GO (install Go into .tools/go first)"; exit 1; }
  [ -x "$PG_BIN/postgres" ] || { log "repo-local PostgreSQL missing at $PG_BIN (install into .tools/pg first)"; exit 1; }
  install_jdk
  install_keycloak
  install_redis
  install_minio
  install_mailpit
  install_stripe
  install_goose
  log "install complete"
}

# --------------------------------------------------------------------- up
pg_running() { "$PG_BIN/pg_ctl" -D "$PGDATA" status >/dev/null 2>&1; }

pg_start() { "$PG_BIN/pg_ctl" -D "$PGDATA" -l "$LOGS/postgres.log" -o "-p 5432 -k $RUN" -w start >/dev/null; }

up_pg() {
  if listening 5432; then log "postgres already listening on 5432"; return; fi
  local fresh=0
  if [ ! -f "$PGDATA/PG_VERSION" ]; then
    log "initialising postgres data dir"
    mkdir -p "$PGDATA"
    "$PG_BIN/initdb" -D "$PGDATA" -U postgres --auth=trust -E UTF8 >>"$LOGS/postgres.log" 2>&1
    fresh=1
  fi
  pg_start
  if [ "$fresh" = 1 ]; then
    "$PG_BIN/pg_ctl" -D "$PGDATA" stop -m fast >/dev/null
    echo "CREATE DATABASE aquify;" | "$PG_BIN/postgres" --single -D "$PGDATA" postgres >>"$LOGS/postgres.log" 2>&1
    pg_start
  fi
  wait_for postgres 30 pg_running
}

up_redis() {
  if listening 6379; then log "redis already listening on 6379"; return; fi
  mkdir -p "$RUN/redis"
  spawn redis "$LOGS/redis.log" "$REDIS_BIN/redis-server" --port 6379 --dir "$RUN/redis" --appendonly yes --bind 127.0.0.1
  wait_for redis 20 "$REDIS_BIN/redis-cli" -h 127.0.0.1 ping
}

up_minio() {
  if listening 9000; then log "minio already listening on 9000"; return; fi
  mkdir -p "$RUN/minio" "$RUN/mc"
  MINIO_ROOT_USER=minioadmin MINIO_ROOT_PASSWORD=minioadmin \
    spawn minio "$LOGS/minio.log" "$MINIO_BIN/minio" server "$RUN/minio" --address 127.0.0.1:9000 --console-address 127.0.0.1:9001
  wait_for minio 30 curl -fsS http://127.0.0.1:9000/minio/health/live
}

up_mailpit() {
  if listening 1025; then log "mailpit already listening on 1025"; return; fi
  spawn mailpit "$LOGS/mailpit.log" "$MAILPIT" --smtp 127.0.0.1:1025 --listen 127.0.0.1:8025 --database "$RUN/mailpit.db"
  wait_for mailpit 20 curl -fsS http://127.0.0.1:8025/api/v1/info
}

up_keycloak() {
  if listening 8180; then log "keycloak already listening on 8180"; return; fi
  [ -x "$KC_DIR/bin/kc.sh" ] || { log "keycloak not installed; run install"; return 1; }
  mkdir -p "$KC_DIR/data/import"
  rm -rf "$KC_DIR/themes/aquify"
  cp -R "$ROOT/deploy/keycloak/themes/aquify" "$KC_DIR/themes/aquify"
  if [ ! -f "$KC_DIR/data/h2/keycloakdb.mv.db" ]; then
    log "first-time keycloak start: importing realm"
    cp "$ROOT/deploy/keycloak/realm-aquify-local.json" "$KC_DIR/data/import/realm-aquify-local.json"
  else
    rm -f "$KC_DIR/data/import/realm-aquify-local.json"
  fi
  JAVA_HOME="$JAVA_HOME_DIR" KC_BOOTSTRAP_ADMIN_USERNAME=admin KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
    JAVA_OPTS_APPEND="--add-opens java.base/java.lang=ALL-UNNAMED" \
    spawn keycloak "$LOGS/keycloak.log" "$KC_DIR/bin/kc.sh" start-dev --http-port=8180 --import-realm
  wait_for keycloak 180 curl -fsS http://127.0.0.1:8180/realms/aquify
}

cmd_up() {
  up_pg
  up_redis
  up_minio
  up_mailpit
  up_keycloak
  log "all services up"
}

# ------------------------------------------------------------------- down
cmd_down() {
  if pg_running; then "$PG_BIN/pg_ctl" -D "$PGDATA" stop -m fast >/dev/null && log "postgres stopped"; else log "postgres not running (or not managed here)"; fi
  if [ -x "$REDIS_BIN/redis-cli" ] && "$REDIS_BIN/redis-cli" -h 127.0.0.1 ping >/dev/null 2>&1 && [ -f "$PIDS/redis.pid" ]; then
    "$REDIS_BIN/redis-cli" -h 127.0.0.1 shutdown save >/dev/null 2>&1 || true
    log "redis stopped"
    rm -f "$PIDS/redis.pid"
  else
    kill_pidfile redis
  fi
  kill_pidfile keycloak
  kill_pidfile mailpit
  kill_pidfile minio
}

# ----------------------------------------------------------------- status
cmd_status() {
  printf '%-10s %-6s %-8s %s\n' SERVICE PORT STATE HEALTH
  row() { # name port healthcmd...
    local name=$1 port=$2; shift 2
    local state=down health=-
    if listening "$port"; then
      state=up
      if "$@" >/dev/null 2>&1; then health=ok; else health=unhealthy; fi
    fi
    printf '%-10s %-6s %-8s %s\n' "$name" "$port" "$state" "$health"
  }
  row postgres 5432 pg_running
  row redis 6379 "$REDIS_BIN/redis-cli" -h 127.0.0.1 ping
  row minio 9000 curl -fsS http://127.0.0.1:9000/minio/health/live
  row mailpit 8025 curl -fsS http://127.0.0.1:8025/api/v1/info
  row smtp 1025 true
  row keycloak 8180 curl -fsS http://127.0.0.1:8180/realms/aquify
}

# ---------------------------------------------------------------- migrate
cmd_migrate() {
  install_goose
  local dsn="${DATABASE_URL:-postgres://postgres@localhost:5432/aquify?sslmode=disable}"
  (cd "$ROOT" && "$GOOSE" -dir db/migrations postgres "$dsn" up)
}

case "${1:-}" in
  install) cmd_install ;;
  up) cmd_up ;;
  down) cmd_down ;;
  status) cmd_status ;;
  migrate) cmd_migrate ;;
  *) echo "usage: $0 {install|up|down|status|migrate}" >&2; exit 2 ;;
esac
