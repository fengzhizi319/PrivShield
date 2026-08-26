#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】停止 PrivShield 调度之眼全栈测试集群 (Docker Compose)
# Stop PrivShield App-LZ Full Stack in Docker Compose
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "正在停止 PrivShield App-LZ 容器集群..."

cd "$PROJECT_ROOT/deploy/docker-compose"
docker compose -f docker-compose.app-lz.yml down 2>/dev/null || true

# 清理独立容器
docker rm -f PrivShield privshield-service-hub privshield-datasource-mgr privshield-audit-log privshield-app-lz-bff privshield-app-lz-web 2>/dev/null || true

echo "PrivShield App-LZ 容器服务已全部停止。"
