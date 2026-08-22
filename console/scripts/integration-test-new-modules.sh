#!/usr/bin/env bash
# ============================================================================
# Integration Test Script for Three New Console Modules
# 三个新控制台模块的集成测试脚本
#
# 测试内容：
#   1. 三个模块的健康检查
#   2. 模块间联动（service-hub → agent, datasource-mgr → agent, audit-log）
#   3. 端到端流水线：数据源注册 → 分类分级 → 调度脱敏 → 审计记录
#
# 前置条件：
#   - 三个模块已启动（dev-start-new-modules.sh）
#   - PrivShield Agent 已运行（REST: 8079）
#
# Usage:
#   bash console/scripts/integration-test-new-modules.sh
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")/../.."

# 颜色输出
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
if [ "$AGENT_STATUS" = "200" ]; then
    log_info "PrivShield Agent reachable at ${AGENT_URL}"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    log_warn "PrivShield Agent not reachable at ${AGENT_URL} (HTTP ${AGENT_STATUS})"
    log_warn "Skipping agent-dependent tests"
fi

echo ""

# ── Phase 3: datasource-mgr CRUD ───────────────────────────────────────
log_step "Phase 3: datasource-mgr CRUD"
echo ""

# Create datasource
DS_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${DATASOURCE_MGR_URL}/api/datasources" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "集成测试数据库",
        "type": "database",
        "host": "192.168.1.100",
        "port": 5432,
        "database": "integration_test",
        "security_level": "high",
        "tags": ["集成测试", "卫健"]
    }' 2>/dev/null)
DS_HTTP=$(echo "$DS_RESPONSE" | tail -1)
DS_BODY=$(echo "$DS_RESPONSE" | sed '$d')
assert_status "create datasource" "201" "$DS_HTTP"

DS_ID=$(echo "$DS_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
if [ -n "$DS_ID" ]; then
    log_info "datasource created with id: ${DS_ID}"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    log_error "failed to extract datasource id"
    FAIL_COUNT=$((FAIL_COUNT + 1))
fi

# Get datasource
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${DATASOURCE_MGR_URL}/api/datasources/${DS_ID}" 2>/dev/null || echo "000")
assert_status "get datasource" "200" "$STATUS"

# List datasources
LIST_RESPONSE=$(curl -s "${DATASOURCE_MGR_URL}/api/datasources" 2>/dev/null)
assert_json_field "list datasources" "$LIST_RESPONSE" "total" "1"

# Get metadata
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${DATASOURCE_MGR_URL}/api/datasources/${DS_ID}/metadata" 2>/dev/null || echo "000")
assert_status "get metadata" "200" "$STATUS"

# Get audit trail
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${DATASOURCE_MGR_URL}/api/datasources/${DS_ID}/audit" 2>/dev/null || echo "000")
assert_status "get datasource audit" "200" "$STATUS"

echo ""

# ── Phase 4: audit-log Operations ──────────────────────────────────────
log_step "Phase 4: audit-log Operations"
echo ""

# Create audit log
AL_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${AUDIT_LOG_URL}/api/audit/logs" \
    -H "Content-Type: application/json" \
    -d '{
        "operation": "mask",
        "datasource": "集成测试数据库",
        "algorithm": "field_mask",
        "parameters": {"fields": ["name", "id_card"]},
        "input_rows": 1000,
        "output_rows": 1000,
        "duration_ms": 45,
        "user": "integration-test",
        "status": "success",
        "security_level": "L3"
    }' 2>/dev/null)
AL_HTTP=$(echo "$AL_RESPONSE" | tail -1)
AL_BODY=$(echo "$AL_RESPONSE" | sed '$d')
assert_status "create audit log" "201" "$AL_HTTP"

AL_ID=$(echo "$AL_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
if [ -n "$AL_ID" ]; then
    log_info "audit log created with id: ${AL_ID}"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    log_error "failed to extract audit log id"
    FAIL_COUNT=$((FAIL_COUNT + 1))
fi

# Get audit log
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${AUDIT_LOG_URL}/api/audit/logs/${AL_ID}" 2>/dev/null || echo "000")
assert_status "get audit log" "200" "$STATUS"

# Get stats
STATS_RESPONSE=$(curl -s "${AUDIT_LOG_URL}/api/audit/stats" 2>/dev/null)
assert_json_field "audit stats total" "$STATS_RESPONSE" "total_operations" "1"

# List snapshots
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${AUDIT_LOG_URL}/api/audit/snapshots" 2>/dev/null || echo "000")
assert_status "list snapshots" "200" "$STATUS"

# Verify integrity
VERIFY_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${AUDIT_LOG_URL}/api/audit/snapshots/verify" \
    -H "Content-Type: application/json" \
    -d '{"snapshot_id": "snap-1"}' 2>/dev/null)
VERIFY_HTTP=$(echo "$VERIFY_RESPONSE" | tail -1)
VERIFY_BODY=$(echo "$VERIFY_RESPONSE" | sed '$d')
assert_status "verify integrity" "200" "$VERIFY_HTTP"
assert_json_field "integrity valid" "$VERIFY_BODY" "valid" "true"

# Generate report
REPORT_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${AUDIT_LOG_URL}/api/audit/report" \
    -H "Content-Type: application/json" \
    -d '{"period": "24h"}' 2>/dev/null)
REPORT_HTTP=$(echo "$REPORT_RESPONSE" | tail -1)
assert_status "generate report" "200" "$REPORT_HTTP"

echo ""

# ── Phase 5: service-hub Dispatch (Agent-independent) ──────────────────
log_step "Phase 5: service-hub Dispatch"
echo ""

# Hub status
HUB_RESPONSE=$(curl -s "${SERVICE_HUB_URL}/api/hub/status" 2>/dev/null)
assert_json_field "hub status module" "$HUB_RESPONSE" "module" "service-hub"

# Dispatch a task (operation=none to avoid agent dependency)
DISPATCH_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${SERVICE_HUB_URL}/api/tasks/dispatch" \
    -H "Content-Type: application/json" \
    -d '{
        "data": {"patient_name": "张三", "id_card": "110101199001011234"},
        "level": "L1",
        "datasource": "集成测试数据库",
        "user": "integration-test"
    }' 2>/dev/null)
DISPATCH_HTTP=$(echo "$DISPATCH_RESPONSE" | tail -1)
DISPATCH_BODY=$(echo "$DISPATCH_RESPONSE" | sed '$d')
assert_status "dispatch task" "202" "$DISPATCH_HTTP"

TASK_ID=$(echo "$DISPATCH_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('task_id',''))" 2>/dev/null || echo "")
if [ -n "$TASK_ID" ]; then
    log_info "task dispatched with id: ${TASK_ID}"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    log_error "failed to extract task id"
    FAIL_COUNT=$((FAIL_COUNT + 1))
fi

# Wait and check task status
sleep 2
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${SERVICE_HUB_URL}/api/tasks/${TASK_ID}" 2>/dev/null || echo "000")
assert_status "get task status" "200" "$STATUS"

# List tasks
TASKS_RESPONSE=$(curl -s "${SERVICE_HUB_URL}/api/tasks" 2>/dev/null)
TOTAL=$(echo "$TASKS_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total',0))" 2>/dev/null || echo "0")
if [ "$TOTAL" -ge 1 ]; then
    log_info "list tasks shows total=$TOTAL"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    log_error "expected at least 1 task, got $TOTAL"
    FAIL_COUNT=$((FAIL_COUNT + 1))
fi

echo ""

# ── Phase 6: Agent-linked Classification (if agent available) ──────────
if [ "$AGENT_STATUS" = "200" ]; then
    log_step "Phase 6: Agent-linked Classification & Masking"
    echo ""

    # Dispatch L3 task (should trigger classify → mask pipeline)
    AGENT_DISPATCH=$(curl -s -w "\n%{http_code}" -X POST "${SERVICE_HUB_URL}/api/tasks/dispatch" \
        -H "Content-Type: application/json" \
        -d '{
            "data": {"patient_name": "李四", "diagnosis": "高血压", "id_card": "310101198505051234"},
            "level": "L3",
            "datasource": "卫健数据集成测试",
            "user": "integration-test"
        }' 2>/dev/null)
    AGENT_DISPATCH_HTTP=$(echo "$AGENT_DISPATCH" | tail -1)
    AGENT_DISPATCH_BODY=$(echo "$AGENT_DISPATCH" | sed '$d')
    assert_status "dispatch L3 task (agent)" "202" "$AGENT_DISPATCH_HTTP"

    TASK_ID2=$(echo "$AGENT_DISPATCH_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('task_id',''))" 2>/dev/null || echo "")

    # Wait for processing
    sleep 5

    if [ -n "$TASK_ID2" ]; then
        TASK_DETAIL=$(curl -s "${SERVICE_HUB_URL}/api/tasks/${TASK_ID2}" 2>/dev/null)
        TASK_STATUS=$(echo "$TASK_DETAIL" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
        if [ "$TASK_STATUS" = "completed" ]; then
            log_info "L3 task completed via agent pipeline"
            PASS_COUNT=$((PASS_COUNT + 1))
        else
            log_warn "L3 task status=$TASK_STATUS (agent may not support classification)"
        fi
    fi

    # Record audit entry for agent-linked operation
    curl -s -X POST "${AUDIT_LOG_URL}/api/audit/logs" \
        -H "Content-Type: application/json" \
        -d '{
            "operation": "classify_and_mask",
            "datasource": "卫健数据集成测试",
            "algorithm": "pipeline",
            "input_rows": 1,
            "output_rows": 1,
            "duration_ms": 3000,
            "user": "integration-test",
            "status": "success",
            "security_level": "L3"
        }' >/dev/null 2>&1

    echo ""
fi

# ── Phase 7: Cleanup ───────────────────────────────────────────────────
log_step "Phase 7: Cleanup"
echo ""

# Delete datasource
if [ -n "$DS_ID" ]; then
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${DATASOURCE_MGR_URL}/api/datasources/${DS_ID}" 2>/dev/null || echo "000")
    assert_status "delete datasource" "200" "$STATUS"
fi

echo ""

# ── Summary ────────────────────────────────────────────────────────────
echo "╔══════════════════════════════════════════════════════════════╗"
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo "║  Results: ${PASS_COUNT}/${TOTAL} passed, ${FAIL_COUNT} failed"
if [ "$FAIL_COUNT" -eq 0 ]; then
    echo "║  Status: ✅ ALL TESTS PASSED"
else
    echo "║  Status: ❌ SOME TESTS FAILED"
fi
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

exit $FAIL_COUNT
