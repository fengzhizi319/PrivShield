#!/usr/bin/env bash
# ============================================================================
# Audit Log Health Check Script
# 脱敏审计日志健康检查脚本
# ============================================================================

set -euo pipefail

HOST="${AUDIT_LOG_HOST:-127.0.0.1}"
PORT="${AUDIT_LOG_PORT:-8084}"
BASE_URL="http://${HOST}:${PORT}"

echo "=== Audit Log Health Check ==="
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

# Audit stats
echo -n "Stats (/api/audit/stats): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/audit/stats" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED"
fi

echo ""

# Snapshot count
echo -n "Snapshots (/api/audit/snapshots): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/audit/snapshots?limit=1" 2>/dev/null); then
    echo "OK"
else
    echo "FAILED"
fi
