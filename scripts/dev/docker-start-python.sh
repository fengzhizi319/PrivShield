#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动 Agent + Python 后端代理 + React 控制台 UI
# Launch Privacy Agent, Python Console Backend & Web UI in Docker Compose
#
# 用法 / Usage: ./scripts/dev/docker-start-python.sh [--build] [--no-build]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BUILD_FLAG="--build"

for arg in "$@"; do
    case "$arg" in
        --no-build)
            BUILD_FLAG=""
            ;;
        --build)
            BUILD_FLAG="--build"
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [--build] [--no-build]"
            echo ""
            echo "选项 / Options:"
            echo "  --no-build   跳过镜像构建，使用本地已有镜像"
            echo "  --build      启动前重新构建本地镜像 (默认)"
            echo "  -h, --help   显示帮助信息"
            exit 0
            ;;
    esac
done

echo "============================================================================"
echo "🚀 [Docker Mode] 正在启动 Agent + Python 后端代理 + Web 控制台全套容器..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"

# shellcheck disable=SC2086
docker compose up -d $BUILD_FLAG PrivShield console-backend-python console-web

echo ""
echo "✅ 容器服务已全面启动！"
echo "   - React 控制台 Web UI     : http://localhost:5173"
echo "   - Python 代理后端 REST API : http://localhost:8080"
echo "   - Privacy Agent REST      : http://localhost:8079"
echo "   - Privacy Agent gRPC      : localhost:50051"
echo "============================================================================"
