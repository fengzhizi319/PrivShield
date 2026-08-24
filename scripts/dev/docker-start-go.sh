#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动 Agent + Go 后端代理 + React 控制台 UI
# Launch Privacy Agent, Go Console Backend & Web UI in Docker Compose
#
# 用法 / Usage: ./scripts/dev/docker-start-go.sh [--build] [--no-build]
# ============================================================================

set -euo pipefail

# ── 解析脚本所在目录，定位项目根目录 ────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BUILD_FLAG="--build"   # 默认启动前重新构建镜像

# ── 解析命令行参数 ─────────────────────────────────────────────────────
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
echo "🚀 [Docker Mode] 正在启动 Agent + Go 后端代理 + Web 控制台全套容器..."
echo "============================================================================"

# ── 进入 docker-compose 目录，仅启动指定的 3 个服务 ──────────────────────
# PrivShield          : 隐私计算 Agent（REST + gRPC）
# console-backend-go  : Go BFF 代理网关（端口 8081）
# console-web         : React 前端 UI（端口 5173）
cd "$PROJECT_ROOT/deploy/docker-compose"

# shellcheck disable=SC2086
docker compose up -d $BUILD_FLAG PrivShield console-backend-go console-web

echo ""
echo "✅ 容器服务已全面启动！"
echo "   - React 控制台 Web UI : http://localhost:5173"
echo "   - Go 代理后端 REST API : http://localhost:8081"
echo "   - Privacy Agent REST  : http://localhost:8079"
echo "   - Privacy Agent gRPC  : localhost:50051"
echo "============================================================================"
