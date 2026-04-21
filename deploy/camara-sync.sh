#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"

required_env=(DATABASE_URL MEMGRAPH_URI MEMGRAPH_USER MEMGRAPH_PASS)
for key in "${required_env[@]}"; do
  if [[ -z "${!key:-}" ]]; then
    echo "[camara-sync] missing env: $key" >&2
    exit 2
  fi
done

echo "[camara-sync] checking postgres connectivity" >&2
if ! timeout 30s bash -c "until pg_isready -d \"$DATABASE_URL\" >/dev/null 2>&1; do sleep 2; done"; then
  echo "[camara-sync] postgres not ready within timeout" >&2
  exit 1
fi

echo "[camara-sync] checking memgraph connectivity" >&2
MEMGRAPH_HOSTPORT="${MEMGRAPH_URI#bolt://}"
if [[ "$MEMGRAPH_HOSTPORT" == "$MEMGRAPH_URI" ]]; then
  MEMGRAPH_HOSTPORT="${MEMGRAPH_URI#neo4j://}"
fi
MEMGRAPH_HOST="${MEMGRAPH_HOSTPORT%:*}"
MEMGRAPH_PORT="${MEMGRAPH_HOSTPORT##*:}"
if ! timeout 30s bash -c "until nc -z \"$MEMGRAPH_HOST\" \"$MEMGRAPH_PORT\" >/dev/null 2>&1; do sleep 2; done"; then
  echo "[camara-sync] memgraph not ready within timeout" >&2
  exit 1
fi

echo "[camara-sync] starting worker" >&2
cd "$BACKEND_DIR"
go run ./workers/camara/cmd --persist-db
