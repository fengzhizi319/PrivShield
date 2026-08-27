#!/usr/bin/env bash
# ============================================================================
# Audit Log (脱敏审计日志) Docker Container Stop Script
# 脱敏审计日志服务 Docker 独立容器停止脚本
#
# 用法 / Usage:
#   bash ./scripts/stop-docker.sh
# ============================================================================

set -euo pipefail

CONTAINER_NAME="${AUDIT_LOG_CONTAINER:-privshield-audit-log}"

echo "=========================================="
echo "  Stop Audit Log Container"
echo "=========================================="

if docker ps -a --format '{{.Names}}' | grep -Eq "^${CONTAINER_NAME}\$"; then
    echo "🛑 停止并删除容器 [$CONTAINER_NAME]..."
    docker rm -f "$CONTAINER_NAME"
    echo "✅ 容器 [$CONTAINER_NAME] 已成功停止并清理。"
else
    echo "ℹ️  容器 [$CONTAINER_NAME] 未在运行。"
fi
