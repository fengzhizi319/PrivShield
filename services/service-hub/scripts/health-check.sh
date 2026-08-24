#!/usr/bin/env bash
# ============================================================================
# Service Hub Health Check Script
# 数据服务调度中枢健康检查脚本
# ============================================================================

set -euo pipefail

HOST="${SERVICE_HUB_HOST:-127.0.0.1}"
PORT="${SERVICE_HUB_PORT:-8082}"
BASE_URL="http://${HOST}:${PORT}"

echo "=== Service Hub Health Check ==="
echo ""

# Health endpoint
echo -n "Health (/api/health): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/health" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED (unreachable)"
fi

echo ""

# Hub status
echo -n "Hub Status (/api/hub/status): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/hub/status" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED"
fi

echo ""

# Pipeline status
echo -n "Pipeline (/api/hub/pipeline): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/hub/pipeline" 2>/dev/null); then
    echo "OK"
else
    echo "FAILED"
fi
