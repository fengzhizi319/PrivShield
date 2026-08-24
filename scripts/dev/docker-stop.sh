#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】一键停止、清理控制台及全栈相关容器
# Stop & Cleanup Console & Full Stack Docker Containers
#
# 用法 / Usage: ./scripts/dev/docker-stop.sh [--volumes]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
VOLUMES_FLAG=""

for arg in "$@"; do
    case "$arg" in
        --volumes|-v)
            VOLUMES_FLAG="--volumes"
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [--volumes]"
            echo ""
            echo "选项 / Options:"
            echo "  --volumes, -v   同时删除关联的命名卷 (持久化数据将被清空)"
            echo "  -h, --help      显示帮助信息"
            exit 0
            ;;
    esac
done

echo "============================================================================"
echo "🛑 [Docker Mode] 正在停止并清理 PrivShield 容器服务..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"

# shellcheck disable=SC2086
docker compose --profile llm --profile monitoring down $VOLUMES_FLAG

echo ""
echo "✅ 所有相关容器与网络已成功停止与清理！"
echo "============================================================================"
