#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) Production Deployment Script
# 数据服务调度中枢生产部署脚本
# ============================================================================

set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE_NAME="${SERVICE_HUB_IMAGE:-privshield-service-hub:0.1.0}"
CONTAINER_NAME="${SERVICE_HUB_CONTAINER:-privshield-service-hub}"
HOST="${SERVICE_HUB_HOST:-0.0.0.0}"
PORT="${SERVICE_HUB_PORT:-8082}"

echo "=========================================="
echo "  Deploy Service Hub (调度中枢)"
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
  -p "${PORT}:8082" \
  -e SERVICE_HUB_HOST="$HOST" \
  -e SERVICE_HUB_PORT=8082 \
  -e PRIVACY_AGENT_REST_HOST="${PRIVACY_AGENT_REST_HOST:-privshield-agent}" \
  -e PRIVACY_REST_PORT="${PRIVACY_REST_PORT:-8079}" \
  -e PRIVACY_AGENT_API_KEY="${PRIVACY_AGENT_API_KEY:-}" \
  -e SERVICE_HUB_MAX_QUEUE="${SERVICE_HUB_MAX_QUEUE:-1000}" \
  -e SERVICE_HUB_SCHEDULE_TIMEOUT="${SERVICE_HUB_SCHEDULE_TIMEOUT:-30}" \
  --restart unless-stopped \
  "$IMAGE_NAME"

echo ""
echo "Service Hub deployed successfully!"
echo "  Health: http://127.0.0.1:${PORT}/api/health"
echo "  Status: http://127.0.0.1:${PORT}/api/hub/status"
