#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <compose-file> <service> [extra docker compose args...]" >&2
  exit 2
fi

COMPOSE_FILE="$1"
SERVICE="$2"
shift 2

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOCK_DIR="${LOCK_DIR:-/tmp}"
LOG_DIR="${LOG_DIR:-$ROOT_DIR/logs/workers}"

mkdir -p "$LOCK_DIR" "$LOG_DIR"

LOCK_FILE="$LOCK_DIR/corruption-center-${SERVICE}.lock"
LOG_FILE="$LOG_DIR/${SERVICE}.log"

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  COMPOSE_CMD=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_CMD=(docker-compose)
else
  echo "docker compose not found" >&2
  exit 127
fi

{
  flock -n 9 || {
    echo "[$(date -u +%FT%TZ)] ${SERVICE} skipped (already running)" >> "$LOG_FILE"
    exit 0
  }

  echo "[$(date -u +%FT%TZ)] ${SERVICE} started" >> "$LOG_FILE"
  if (cd "$ROOT_DIR" && "${COMPOSE_CMD[@]}" -f "$COMPOSE_FILE" --profile workers run --rm "$SERVICE" "$@") >> "$LOG_FILE" 2>&1; then
    echo "[$(date -u +%FT%TZ)] ${SERVICE} finished ok" >> "$LOG_FILE"
  else
    status=$?
    echo "[$(date -u +%FT%TZ)] ${SERVICE} failed status=${status}" >> "$LOG_FILE"
    exit "$status"
  fi
} 9>"$LOCK_FILE"
