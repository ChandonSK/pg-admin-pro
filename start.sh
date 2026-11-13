#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT_DIR}"

# Read APP_ADDR from .env if present (defaults to :8080)
APP_ADDR=":8080"
if [[ -f "${ROOT_DIR}/.env" ]]; then
  env_addr="$(grep -E '^APP_ADDR=' "${ROOT_DIR}/.env" | tail -n 1 | cut -d '=' -f 2- | tr -d '[:space:]"'\''')" || true
  if [[ -n "${env_addr}" ]]; then
    APP_ADDR="${env_addr}"
  fi
fi

PORT="${APP_ADDR##*:}"
HOST="${APP_ADDR%:*}"
if [[ -z "${HOST}" || "${HOST}" == "0.0.0.0" || "${HOST}" == "::" || "${HOST}" == "[::]" || "${HOST}" == "${APP_ADDR}" ]]; then
  HOST="localhost"
fi
if [[ -z "${PORT}" || "${PORT}" == "${APP_ADDR}" ]]; then
  PORT="8080"
fi
URL="http://${HOST}:${PORT}"

# Open browser automatically once the server starts listening
(
  for _ in $(seq 1 60); do
    if (echo > "/dev/tcp/127.0.0.1/${PORT}") >/dev/null 2>&1; then
      if command -v xdg-open >/dev/null 2>&1; then
        xdg-open "${URL}" >/dev/null 2>&1 &
      elif command -v open >/dev/null 2>&1; then
        open "${URL}" >/dev/null 2>&1 &
      fi
      exit 0
    fi
    sleep 0.25
  done
) &

echo "Starting pg-admin-pro -> ${URL}"
exec go run .

