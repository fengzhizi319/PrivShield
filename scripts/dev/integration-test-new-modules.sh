#!/usr/bin/env bash
# ============================================================================
# Integration Test Script for Three New Microservice Modules
# 三个中台微服务模块的集成测试脚本
#
# 测试内容：
#   1. 三个模块的健康检查
#   2. 模块间联动（service-hub → agent, datasource-mgr → agent, audit-log）
#   3. 端到端流水线：数据源注册 → 分类分级 → 调度脱敏 → 审计记录
#
# 前置条件：
#   - 三个模块已启动（dev-start-new-modules.sh 或 e2e-start-all-services.sh）
#   - PrivShield Agent 已运行（REST: 8079）
#
# Usage:
#   bash scripts/dev/integration-test-new-modules.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[PASS]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[FAIL]${NC}  $*"; }
log_step()  { echo -e "${CYAN}[STEP]${NC}  $*"; }

SERVICE_HUB_URL="${SERVICE_HUB_URL:-http://127.0.0.1:8082}"
DATASOURCE_MGR_URL="${DATASOURCE_MGR_URL:-http://127.0.0.1:8083}"
AUDIT_LOG_URL="${AUDIT_LOG_URL:-http://127.0.0.1:8084}"
AGENT_URL="${PRIVSHIELD_AGENT_URL:-http://127.0.0.1:8079}"

PASS_COUNT=0
FAIL_COUNT=0

assert_status() {
    local desc="$1"
    local expected="$2"
    local actual="$3"

    if [ "$actual" = "$expected" ]; then
        log_info "$desc (HTTP $actual)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "$desc (expected HTTP $expected, got $actual)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_json_field() {
    local desc="$1"
    local json="$2"
    local field="$3"
    local expected="$4"

    local actual
    actual=$(echo "$json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('$field',''))" 2>/dev/null || echo "")

    if [ "$actual" = "$expected" ]; then
        log_info "$desc ($field=$actual)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "$desc (expected $field=$expected, got $field=$actual)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║   Integration Test: service-hub / datasource-mgr / audit-log  ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# ── Phase 1: Health Checks ─────────────────────────────────────────────
log_step "Phase 1: Health Checks"
echo ""

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${SERVICE_HUB_URL}/api/health" 2>/dev/null || echo "000")
assert_status "service-hub health" "200" "$STATUS"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${DATASOURCE_MGR_URL}/api/health" 2>/dev/null || echo "000")
assert_status "datasource-mgr health" "200" "$STATUS"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${AUDIT_LOG_URL}/api/health" 2>/dev/null || echo "000")
assert_status "audit-log health" "200" "$STATUS"

echo ""

# ── Phase 2: Agent Connectivity ────────────────────────────────────────
log_step "Phase 2: Agent Connectivity"
echo ""

AGENT_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${AGENT_URL}/health" 2>/dev/null || echo "000")
assert_status "PrivShield Agent health" "200" "$AGENT_STATUS"

echo ""

# ── Phase 3: datasource-mgr Operations ─────────────────────────────────
log_step "Phase 3: datasource-mgr Operations"
echo ""

DS_RESP=$(curl -s -X POST "${DATASOURCE_MGR_URL}/api/datasources" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "test-yibao-db",
        "type": "csv",
        "connection_info": "{\"path\": \"data/yibao.csv\"}",
        "description": "Integration test datasource"
    }')
assert_json_field "Create datasource" "$DS_RESP" "code" "0"

DS_ID=$(echo "$DS_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('id',''))" 2>/dev/null || echo "")

if [ -n "$DS_ID" ]; then
    log_info "Created datasource ID: $DS_ID"

    LIST_RESP=$(curl -s "${DATASOURCE_MGR_URL}/api/datasources")
    assert_json_field "List datasources" "$LIST_RESP" "code" "0"

    TEST_CONN_RESP=$(curl -s -X POST "${DATASOURCE_MGR_URL}/api/datasources/${DS_ID}/test-connection")
    assert_json_field "Test connection" "$TEST_CONN_RESP" "code" "0"

    AUTO_PROBE_RESP=$(curl -s -X POST "${DATASOURCE_MGR_URL}/api/datasources/${DS_ID}/auto-probe")
    assert_json_field "Auto probe" "$AUTO_PROBE_RESP" "code" "0"
fi

echo ""

# ── Phase 4: service-hub Pipeline Execution ────────────────────────────
log_step "Phase 4: service-hub Pipeline Execution"
echo ""

TASK_RESP=$(curl -s -X POST "${SERVICE_HUB_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{
        "title": "Integration Test Pipeline Task",
        "description": "E2E pipeline test via integration-test-new-modules.sh",
        "submitter": "qa-automation",
        "dataset_name": "yibao_settlement",
        "record_count": 5,
        "purpose": "medical_insurance_analytics",
        "data": [
            {"id": "YB001", "name": "张三", "id_card": "110101199003072345", "phone": "13800138000", "diagnosis": "高血压", "fee": 1250.50},
            {"id": "YB002", "name": "李四", "id_card": "310104198512154567", "phone": "13912345678", "diagnosis": "2型糖尿病", "fee": 3400.00}
        ]
    }')
assert_json_field "Submit pipeline task" "$TASK_RESP" "code" "0"

TASK_ID=$(echo "$TASK_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('id',''))" 2>/dev/null || echo "")

if [ -n "$TASK_ID" ]; then
    log_info "Created Task ID: $TASK_ID"

    TASK_DETAIL=$(curl -s "${SERVICE_HUB_URL}/api/tasks/${TASK_ID}")
    assert_json_field "Get task detail" "$TASK_DETAIL" "code" "0"

    TASK_STATUS=$(echo "$TASK_DETAIL" | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('status',''))" 2>/dev/null || echo "")
    log_info "Task execution status: $TASK_STATUS"
fi

echo ""

# ── Phase 5: audit-log Verification ────────────────────────────────────
log_step "Phase 5: audit-log Verification"
echo ""

AUDIT_LOGS_RESP=$(curl -s "${AUDIT_LOG_URL}/api/logs?page=1&page_size=10")
assert_json_field "List audit logs" "$AUDIT_LOGS_RESP" "code" "0"

AUDIT_STATS_RESP=$(curl -s "${AUDIT_LOG_URL}/api/stats")
assert_json_field "Audit stats" "$AUDIT_STATS_RESP" "code" "0"

REPORT_RESP=$(curl -s -X POST "${AUDIT_LOG_URL}/api/reports/generate" \
    -H "Content-Type: application/json" \
    -d '{
        "title": "Integration Test Compliance Report",
        "time_range": "24h"
    }')
assert_json_field "Generate compliance report" "$REPORT_RESP" "code" "0"

echo ""

# ── Summary ───────────────────────────────────────────────────────────
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                    Test Summary                              ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo -e "║  Passed: ${GREEN}${PASS_COUNT}${NC}                                                  ║"
echo -e "║  Failed: ${RED}${FAIL_COUNT}${NC}                                                  ║"
echo "╚══════════════════════════════════════════════════════════════╝"

if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
fi
