#!/usr/bin/env bash
# ============================================================================
# Datasource Manager (数据源管理) Production Deployment Script
# 数据源管理生产部署脚本
# ============================================================================

set -euo pipefail

# Dockerfile 要求构建上下文为 console/（非模块子目录）
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONSOLE_DIR="$(cd "$MODULE_DIR/.." && pwd)"

IMAGE_NAME="${DATASOURCE_MGR_IMAGE:-privshield-datasource-mgr:0.1.0}"
CONTAINER_NAME="${DATASOURCE_MGR_CONTAINER:-privshield-datasource-mgr}"
HOST="${DATASOURCE_MGR_HOST:-0.0.0.0}"
PORT="${DATASOURCE_MGR_PORT:-8083}"
# P63 fix: SQLite data directory for persistent storage (default: named volume)
DATA_DIR="${DATASOURCE_MGR_DATA_DIR:-${CONTAINER_NAME}-data}"

echo "=========================================="
echo "  Deploy Datasource Manager (数据源管理)"
echo "=========================================="

# Build image (build context = console/ for shared pkg/ dependency)
echo "[1/3] Building Docker image: $IMAGE_NAME ..."
docker build -f "$MODULE_DIR/Dockerfile" -t "$IMAGE_NAME" "$CONSOLE_DIR"

# Stop old container
echo "[2/3] Removing old container (if exists)..."
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# Run new container
# P63 fix: mount data volume for SQLite persistence
# P64 fix: add post-deploy health check verification
echo "[3/3] Starting container on port $PORT ..."
docker run -d \
  --name "$CONTAINER_NAME" \
  -p "${PORT}:8083" \
  -v "${DATA_DIR}:/app/data" \
  -e DATASOURCE_MGR_HOST="$HOST" \
  -e DATASOURCE_MGR_PORT=8083 \
  -e PRIVACY_AGENT_REST_HOST="${PRIVACY_AGENT_REST_HOST:-privshield-agent}" \
  -e PRIVACY_REST_PORT="${PRIVACY_REST_PORT:-8079}" \
  -e PRIVACY_AGENT_API_KEY="${PRIVACY_AGENT_API_KEY:-}" \
  -e DATASOURCE_MGR_DB_PATH="${DATASOURCE_MGR_DB_PATH:-/app/data/datasource-mgr.db}" \
  --restart unless-stopped \
  "$IMAGE_NAME"

# P64 fix: wait for container to become healthy
echo -n "Waiting for datasource-mgr to be healthy"
for i in $(seq 1 30); do
  if curl -sf --max-time 3 "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; then
    echo " OK"
    echo ""
    echo "Datasource Manager deployed successfully!"
    echo "  Health: http://127.0.0.1:${PORT}/api/health"
    echo "  List:   http://127.0.0.1:${PORT}/api/datasources"
    echo "  Data:   ${DATA_DIR} → /app/data (SQLite persistent)"
    exit 0
  fi
  echo -n "."
  sleep 1
done
echo " TIMEOUT"
echo "WARNING: container started but health check did not respond within 30s"
echo "  Logs: docker logs $CONTAINER_NAME"
exit 1
