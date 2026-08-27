#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) Docker Container Stop Script
# 数据服务调度中枢 Docker 独立容器停止脚本
#
# 用法 / Usage:
#   bash ./scripts/stop-docker.sh
# ============================================================================

set -euo pipefail

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -h, --help    显示帮助信息并退出"
            echo ""
            echo "环境变量 / Env vars:"
            echo "  SERVICE_HUB_CONTAINER  容器名称 (默认: privshield-service-hub)"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

CONTAINER_NAME="${SERVICE_HUB_CONTAINER:-privshield-service-hub}"

echo "=========================================="
echo "  Stop Service Hub Container"
echo "=========================================="

if docker ps -a --format '{{.Names}}' | grep -Eq "^${CONTAINER_NAME}\$"; then
    echo "🛑 停止并删除容器 [$CONTAINER_NAME]..."
    docker rm -f "$CONTAINER_NAME"
    echo "✅ 容器 [$CONTAINER_NAME] 已成功停止并清理。"
else
    echo "ℹ️  容器 [$CONTAINER_NAME] 未在运行。"
fi
