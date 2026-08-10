#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动 Agent + Python 后端代理 + React 控制台 UI
# Launch Privacy Agent, Python Console Backend & Web UI in Docker Compose
#
# 用法 / Usage: ./console/scripts/docker-start-python.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================================================"
echo "🚀 [Docker Mode] 正在启动 Agent + Python 后端代理 + Web 控制台全套容器..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"

docker compose up -d privacy-local-agent console-backend-python console-web

echo ""
echo "✅ 容器服务已全面启动！"
echo "   - React 控制台 Web UI     : http://localhost:5173"
echo "   - Python 代理后端 REST API : http://localhost:8080"
echo "   - Privacy Agent REST      : http://localhost:8079"
echo "   - Privacy Agent gRPC      : localhost:50051"
echo "============================================================================"
