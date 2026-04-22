#!/bin/sh

set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
PATH="/usr/local/go/bin:$PATH"

for key in DATABASE_URL MEMGRAPH_URI MEMGRAPH_USER MEMGRAPH_PASS; do
  eval value="\${$key-}"
  if [ -z "$value" ]; then
    echo "[senado-sync] missing env: $key" >&2
    exit 2
  fi
done

echo "[senado-sync] checking postgres connectivity" >&2
if ! timeout 30s /bin/sh -c "until pg_isready -d \"$DATABASE_URL\" >/dev/null 2>&1; do sleep 2; done"; then
  echo "[senado-sync] postgres not ready within timeout" >&2
  exit 1
fi

echo "[senado-sync] checking memgraph connectivity" >&2
MEMGRAPH_HOSTPORT="${MEMGRAPH_URI#bolt://}"
if [ "$MEMGRAPH_HOSTPORT" = "$MEMGRAPH_URI" ]; then
  MEMGRAPH_HOSTPORT="${MEMGRAPH_URI#neo4j://}"
fi
MEMGRAPH_HOST="${MEMGRAPH_HOSTPORT%:*}"
MEMGRAPH_PORT="${MEMGRAPH_HOSTPORT##*:}"
if ! timeout 30s /bin/sh -c "until nc -z \"$MEMGRAPH_HOST\" \"$MEMGRAPH_PORT\" >/dev/null 2>&1; do sleep 2; done"; then
  echo "[senado-sync] memgraph not ready within timeout" >&2
  exit 1
fi

echo "[senado-sync] starting worker" >&2
cd "$BACKEND_DIR"
go run ./workers/senado/cmd --persist-db
