#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】一键优雅停止并移除所有 Docker 容器服务
# Stop and remove all running Docker services and Compose stacks
#
# 用法 / Usage: ./console/scripts/docker-stop.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================================================"
echo "🛑 [Docker Mode] 正在停止并清理所有 Privacy Local Agent 容器..."
echo "============================================================================"

# 1. 使用 Docker Compose 停止并清理 Compose 服务
cd "$PROJECT_ROOT/deploy/docker-compose"
docker compose --profile llm down --remove-orphans 2>/dev/null || true

# 2. 清理单独使用 docker run 启动的独立容器
docker rm -f PrivShield PrivShield-vllm privacy-console-web privacy-console-backend-go privacy-console-backend-python 2>/dev/null || true

echo "✅ 所有 Docker 容器已成功停止与清理！"
echo "============================================================================"
