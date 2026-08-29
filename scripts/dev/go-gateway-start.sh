#!/usr/bin/env bash
# 启动 Go 引擎 Gateway (开发模式)
set -euo pipefail
export CGO_ENABLED=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
PROJECT_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || (cd "$SCRIPT_DIR/../.." && pwd -P))"

cd "$PROJECT_ROOT"

echo "=== PrivShield Go Engine Gateway (Dev) ==="
echo "Gateway REST: http://127.0.0.1:8000"
echo "Backends:     127.0.0.1:8079"
echo ""

export GATEWAY_HOST="${GATEWAY_HOST:-127.0.0.1}"
export GATEWAY_PORT="${GATEWAY_PORT:-8000}"
export GATEWAY_BACKENDS="${GATEWAY_BACKENDS:-127.0.0.1:8079}"
export GATEWAY_STRATEGY="${GATEWAY_STRATEGY:-p2c}"
export PRIVACY_LOG_LEVEL="${PRIVACY_LOG_LEVEL:-INFO}"

exec go run ./engine-go/cmd/privshield-gateway "$@"
