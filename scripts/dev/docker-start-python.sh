#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动 Agent + Go 后端代理 + React 控制台 UI (重定向自 Python 后端)
# Launch Privacy Agent, Go Console Backend & Web UI in Docker Compose
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
echo "💡 注意: Python BFF 已统一迁移收敛至高性能 Go gRPC BFF (console/bff-go)。"
echo "   正在自动转调 Go 后端启动脚本: $SCRIPT_DIR/docker-start-go.sh"
echo ""

exec "$SCRIPT_DIR/docker-start-go.sh" "$@"
