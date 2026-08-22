#!/usr/bin/env bash
# ============================================================================
# Datasource Manager (数据源管理) Production Deployment Script
# 数据源管理生产部署脚本
# ============================================================================

set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE_NAME="${DATASOURCE_MGR_IMAGE:-privshield-datasource-mgr:0.1.0}"
CONTAINER_NAME="${DATASOURCE_MGR_CONTAINER:-privshield-datasource-mgr}"
HOST="${DATASOURCE_MGR_HOST:-0.0.0.0}"
PORT="${DATASOURCE_MGR_PORT:-8083}"

echo "=========================================="
echo "  Deploy Datasource Manager (数据源管理)"
echo "=========================================="

# Build image
echo "[1/3] Building Docker image: $IMAGE_NAME ..."
docker build -t "$IMAGE_NAME" .

# Stop old container
echo "[2/3] Removing old container (if exists)..."
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# Run new container
echo "[3/3] Starting container on port $PORT ..."
docker run -d \
  --name "$CONTAINER_NAME" \
  -p "${PORT}:8083" \
  -e DATASOURCE_MGR_HOST="$HOST" \
  -e DATASOURCE_MGR_PORT=8083 \
  -e PRIVACY_AGENT_REST_HOST="${PRIVACY_AGENT_REST_HOST:-privshield-agent}" \
  -e PRIVACY_REST_PORT="${PRIVACY_REST_PORT:-8079}" \
  -e PRIVACY_AGENT_API_KEY="${PRIVACY_AGENT_API_KEY:-}" \
  --restart unless-stopped \
  "$IMAGE_NAME"

echo ""
echo "Datasource Manager deployed successfully!"
echo "  Health: http://127.0.0.1:${PORT}/api/health"
echo "  List:   http://127.0.0.1:${PORT}/api/datasources"
