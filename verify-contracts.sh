#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[1/4] Regenerating backend Swagger docs"
(cd "$ROOT_DIR/backend" && ./update-swagger.sh)

echo "[2/4] Running backend tests"
(cd "$ROOT_DIR/backend" && go test ./...)

echo "[3/4] Building frontend (includes TypeScript checks)"
(cd "$ROOT_DIR/frontend" && npm run build)

echo "[4/4] Contract verification completed successfully"
