#!/bin/sh

set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
export PATH="/usr/local/go/bin:$PATH"

for key in DATABASE_URL; do
  eval value="\${$key-}"
  if [ -z "$value" ]; then
    echo "[datajud-watcher] missing env: $key" >&2
    exit 2
  fi
done

ENABLE_WRITES="${ENABLE_WRITES:-true}"
VERIFY_TPU="${VERIFY_TPU:-true}"
STRICT_VERIFY="${STRICT_VERIFY:-false}"
POLL_LIMIT="${DATAJUD_POLL_LIMIT:-100}"
PROBE_CASE="${DATAJUD_PROBE_CASE:-}"
PROBE_TRIBUNAL="${DATAJUD_PROBE_TRIBUNAL:-}"

if [ "$ENABLE_WRITES" = "true" ]; then
  # MEMGRAPH_USER/PASS are optional (auth-less dev Memgraph)
  for key in MEMGRAPH_URI; do
    eval value="\${$key-}"
    if [ -z "$value" ]; then
      echo "[datajud-watcher] missing env for write mode: $key" >&2
      exit 2
    fi
  done
fi

if [ -z "${DATAJUD_API_KEY:-}" ]; then
  echo "[datajud-watcher] DATAJUD_API_KEY is not set; the worker requires it and would exit immediately." >&2
  echo "[datajud-watcher] Get the current public key at https://datajud-wiki.cnj.jus.br/api-publica/acesso/ (it rotates)" >&2
  echo "[datajud-watcher] and export DATAJUD_API_KEY before running. Failing this run so cron surfaces the stopped poller." >&2
  exit 1
fi

echo "[datajud-watcher] checking postgres connectivity" >&2
if ! timeout 30s /bin/sh -c "until pg_isready -d \"$DATABASE_URL\" >/dev/null 2>&1; do sleep 2; done"; then
  echo "[datajud-watcher] postgres not ready within timeout" >&2
  exit 1
fi

if [ "$ENABLE_WRITES" = "true" ]; then
  echo "[datajud-watcher] checking memgraph connectivity" >&2
  MEMGRAPH_HOSTPORT="${MEMGRAPH_URI#bolt://}"
  if [ "$MEMGRAPH_HOSTPORT" = "$MEMGRAPH_URI" ]; then
    MEMGRAPH_HOSTPORT="${MEMGRAPH_URI#neo4j://}"
  fi
  MEMGRAPH_HOST="${MEMGRAPH_HOSTPORT%:*}"
  MEMGRAPH_PORT="${MEMGRAPH_HOSTPORT##*:}"
  if command -v nc >/dev/null 2>&1; then
    if ! timeout 30s /bin/sh -c "until nc -z \"$MEMGRAPH_HOST\" \"$MEMGRAPH_PORT\" >/dev/null 2>&1; do sleep 2; done"; then
      echo "[datajud-watcher] memgraph not ready within timeout" >&2
      exit 1
    fi
  else
    echo "[datajud-watcher] nc not available; skipping preflight memgraph socket check" >&2
  fi
fi

echo "[datajud-watcher] starting worker" >&2
cd "$BACKEND_DIR"

CMD="go run ./workers/datajud/cmd --verify-tpu=${VERIFY_TPU} --strict-verify=${STRICT_VERIFY} --poll-limit=${POLL_LIMIT}"
if [ "$ENABLE_WRITES" = "true" ]; then
  CMD="$CMD --enable-writes"
fi
if [ -n "$PROBE_CASE" ] && [ -n "$PROBE_TRIBUNAL" ]; then
  CMD="$CMD --probe-case=$PROBE_CASE --probe-tribunal=$PROBE_TRIBUNAL"
fi

eval "$CMD"
