#!/bin/bash
# 启动 Go 引擎 Agent (开发模式)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

cd "$PROJECT_ROOT/engine-go"

echo "=== PrivShield Go Engine Agent (Dev) ==="
echo "REST:  http://127.0.0.1:8079"
echo "gRPC:  127.0.0.1:50051"
echo ""

export PRIVACY_REST_HOST="${PRIVACY_REST_HOST:-127.0.0.1}"
export PRIVACY_REST_PORT="${PRIVACY_REST_PORT:-8079}"
export PRIVACY_GRPC_PORT="${PRIVACY_GRPC_PORT:-50051}"
export PRIVACY_LOG_LEVEL="${PRIVACY_LOG_LEVEL:-DEBUG}"

go run ./cmd/privshield-agent "$@"
