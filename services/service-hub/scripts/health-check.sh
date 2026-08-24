#!/usr/bin/env bash
# ============================================================================
# Service Hub Health Check Script
# 数据服务调度中枢运行状态健康探针脚本
#
# 探测端点与检测目标：
#   1. /api/health:       服务本体存活状态与上游 Python Agent 连通性探针；
#   2. /api/hub/status:   调度中枢排队、活跃、成功与失败任务计数及运行时间；
#   3. /api/hub/pipeline: 6 阶段流水线活跃状态与 Agent 依赖可用性检查。
#
# 环境变量配置：
#   SERVICE_HUB_HOST: 调度中枢主机（默认 127.0.0.1）
#   SERVICE_HUB_PORT: 调度中枢端口（默认 8082）
#
# 使用方法：
#   bash ./scripts/health-check.sh
# ============================================================================

set -euo pipefail

HOST="${SERVICE_HUB_HOST:-127.0.0.1}"
PORT="${SERVICE_HUB_PORT:-8082}"
BASE_URL="http://${HOST}:${PORT}"

echo "=== Service Hub Health Check ==="
echo ""

# ── 1. 基础健康检查 (/api/health) ─────────────────────────────────────────────
# 验证 service-hub 后端自身及与上游 Agent 的网络连通性
echo -n "Health (/api/health): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/health" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED (unreachable)"
fi

echo ""

# ── 2. 调度中枢运行态指标 (/api/hub/status) ──────────────────────────────────
# 验证任务队列深度与历史执行统计
echo -n "Hub Status (/api/hub/status): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/hub/status" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED"
fi

echo ""

# ── 3. 流水线阶段状态 (/api/hub/pipeline) ────────────────────────────────────
# 验证流水线 6 个阶段（ingest/fetch/classify/desensitize/return/audit）的处理状态
echo -n "Pipeline (/api/hub/pipeline): "
if resp=$(curl -sf --max-time 5 "${BASE_URL}/api/hub/pipeline" 2>/dev/null); then
    echo "OK"
    echo "  $resp" | python3 -m json.tool 2>/dev/null || echo "  $resp"
else
    echo "FAILED"
fi

