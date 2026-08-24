#!/usr/bin/env bash
# ============================================================================
# Datasource Manager Health Check Script
# 数据源管理健康检查脚本
# ============================================================================

set -euo pipefail

HOST="${DATASOURCE_MGR_HOST:-127.0.0.1}"
PORT="${DATASOURCE_MGR_PORT:-8083}"
BASE_URL="http://${HOST}:${PORT}"

echo "=== Datasource Manager Health Check ==="
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

# List datasources
echo -n "DataSources (/api/datasources): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/datasources" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED"
fi
