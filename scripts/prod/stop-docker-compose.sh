#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield 生产级 Docker Compose 优雅停服脚本
# Gracefully stop PrivShield Production Docker Compose Stack
#
# 用法 / Usage:
#   ./scripts/prod/stop-docker-compose.sh [选项]
#
# 选项 / Options:
#   --volumes, -v        同时清理挂载的匿名数据卷 (注意：慎用，避免持久化预算丢失)
#   --remove-orphans     清理孤儿容器
#   -h, --help           显示帮助信息
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_DIR="$PROJECT_ROOT/deploy/docker-compose"

EXTRA_FLAGS=()

for arg in "$@"; do
    case "$arg" in
        --volumes|-v)
            EXTRA_FLAGS+=("--volumes")
            ;;
        --remove-orphans)
            EXTRA_FLAGS+=("--remove-orphans")
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -v, --volumes        同时移除持久化数据卷 (默认保留数据卷)"
            echo "  --remove-orphans     移除未在 compose 文件中定义的孤儿容器"
            echo "  -h, --help           显示帮助信息并退出"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $arg" >&2
            echo "   请运行 $0 --help 查看帮助" >&2
            exit 1
            ;;
    esac
done

echo "============================================================================"
echo "🛑 【生产模式】正在优雅停止 PrivShield 生产级容器集群..."
echo "============================================================================"

cd "$COMPOSE_DIR"

# 停止全 Profile 服务（含 llm 与 monitoring）
docker compose --profile llm --profile monitoring down "${EXTRA_FLAGS[@]}"

echo ""
echo "✅ PrivShield 生产容器集群已安全停止。"
echo "============================================================================"
