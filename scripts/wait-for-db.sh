#!/usr/bin/env bash
set -euo pipefail

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-${POSTGRES_USER:-postgres}}"
DB_NAME="${DB_NAME:-${POSTGRES_DB:-aquify}}"
WAIT_TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-60}"

echo "Waiting for Postgres at ${DB_HOST}:${DB_PORT} (timeout: ${WAIT_TIMEOUT_SECONDS}s)..."

for ((i = 1; i <= WAIT_TIMEOUT_SECONDS; i++)); do
  if command -v pg_isready >/dev/null 2>&1; then
    if pg_isready -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" >/dev/null 2>&1; then
      echo "Postgres is ready."
      exit 0
    fi
  elif command -v nc >/dev/null 2>&1; then
    if nc -z "${DB_HOST}" "${DB_PORT}" >/dev/null 2>&1; then
      echo "Postgres port is reachable."
      exit 0
    fi
  else
    echo "Neither pg_isready nor nc is available; cannot check DB readiness."
    exit 1
  fi

  sleep 1
done

echo "Timed out waiting for Postgres after ${WAIT_TIMEOUT_SECONDS}s."
exit 1
