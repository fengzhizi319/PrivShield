#!/usr/bin/env bash
# ============================================================================
# Audit Log (脱敏审计日志) Production Deployment Script
# 脱敏审计日志生产部署脚本
# ============================================================================

set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE_NAME="${AUDIT_LOG_IMAGE:-privshield-audit-log:0.1.0}"
CONTAINER_NAME="${AUDIT_LOG_CONTAINER:-privshield-audit-log}"
HOST="${AUDIT_LOG_HOST:-0.0.0.0}"
PORT="${AUDIT_LOG_PORT:-8084}"

echo "=========================================="
echo "  Deploy Audit Log (脱敏审计日志)"
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
  -p "${PORT}:8084" \
  -e AUDIT_LOG_HOST="$HOST" \
  -e AUDIT_LOG_PORT=8084 \
  -e PRIVACY_AGENT_REST_HOST="${PRIVACY_AGENT_REST_HOST:-privshield-agent}" \
  -e PRIVACY_REST_PORT="${PRIVACY_REST_PORT:-8079}" \
  -e PRIVACY_AGENT_API_KEY="${PRIVACY_AGENT_API_KEY:-}" \
  -e AUDIT_LOG_MAX_ENTRIES="${AUDIT_LOG_MAX_ENTRIES:-100000}" \
  --restart unless-stopped \
  "$IMAGE_NAME"

echo ""
echo "Audit Log deployed successfully!"
echo "  Health: http://127.0.0.1:${PORT}/api/health"
echo "  Logs:   http://127.0.0.1:${PORT}/api/audit/logs"
echo "  Stats:  http://127.0.0.1:${PORT}/api/audit/stats"
