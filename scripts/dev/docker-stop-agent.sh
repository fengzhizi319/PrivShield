#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】停止并清理 Privacy Local Agent 容器
# Stop and remove Privacy Local Agent Docker container
#
# 用法 / Usage: ./scripts/dev/docker-stop-agent.sh
# ============================================================================

set -euo pipefail

echo "============================================================================"
echo "🛑 [Docker Mode] 正在停止 Privacy Local Agent 容器..."
echo "============================================================================"

docker rm -f privacy-local-agent 2>/dev/null || true

echo "✅ Privacy Local Agent 容器已成功停止与清理！"
echo "============================================================================"
