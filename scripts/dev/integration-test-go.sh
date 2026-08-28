#!/usr/bin/env bash
# ============================================================================
# Integration Test Script for Go Engine
# Go 原生引擎集成测试脚本
#
# 与 integration-test-new-modules.sh 的区别：
#   - integration-test-new-modules.sh 测试三个 Go 微服务 + Python Agent
#   - 本脚本测试 Go 原生引擎的 REST API 端点（与 Python 引擎对齐验证）
#
# 测试内容：
#   1. Go Agent 健康检查（/health, /livez, /readyz）
#   2. Masking 隐私脱敏 API
#   3. Differential Privacy 差分隐私 API
#   4. K-Anonymity K-匿名 API
#   5. Query Obfuscation 查询混淆 API
#   6. 动态分类分级 API
#
# 前置条件：
#   - Go Agent 已启动（go-engine-start.sh / docker-start-go-agent.sh）
#   - REST: 8079, gRPC: 50051
#
# Usage:
#   bash scripts/dev/integration-test-go.sh
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

AGENT_URL="${PRIVSHIELD_AGENT_URL:-http://127.0.0.1:8079}"

PASS_COUNT=0
FAIL_COUNT=0

assert_http() {
    local desc="$1"
    local method="$2"
    local url="$3"
    local expected_code="${4:-200}"
    local body="${5:-}"

    local code
    if [[ -n "$body" ]]; then
        code=$(curl --noproxy "*" -s -o /tmp/go_test_resp.json -w "%{http_code}" \
            -X "$method" -H "Content-Type: application/json" -d "$body" \
            --max-time 10 "${url}" 2>/dev/null || echo "000")
    else
        code=$(curl --noproxy "*" -s -o /tmp/go_test_resp.json -w "%{http_code}" \
            -X "$method" --max-time 10 "${url}" 2>/dev/null || echo "000")
    fi
    code="${code: -3}"

    if [ "$code" = "$expected_code" ]; then
        log_info "$desc (HTTP ${code})"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "$desc (HTTP ${code}, expected ${expected_code})"
        log_warn "Response: $(cat /tmp/go_test_resp.json 2>/dev/null | head -c 200)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

echo "============================================================================"
echo "🧪 Go Engine 集成测试"
echo "   Agent URL: ${AGENT_URL}"
echo "============================================================================"

# ── 1. 健康检查 ─────────────────────────────────────────────────────────
log_step "1. 健康检查端点"
assert_http "GET /health" "GET" "${AGENT_URL}/health"
assert_http "GET /livez" "GET" "${AGENT_URL}/livez"
assert_http "GET /readyz" "GET" "${AGENT_URL}/readyz"

# ── 2. Masking 脱敏 ─────────────────────────────────────────────────────
log_step "2. Masking 隐私脱敏"
assert_http "POST /mask" "POST" "${AGENT_URL}/mask" "200" \
    '{"record": {"name": "张三", "phone": "13800138000", "email": "zhangsan@example.com", "id_card": "110101199001011234"}}'

assert_http "POST /mask/fields" "POST" "${AGENT_URL}/mask/fields" "200" \
    '{"fields": {"name": "李四", "phone": "13900139000"}, "field_names": ["name", "phone"]}'

# ── 3. Differential Privacy 差分隐私 ────────────────────────────────────
log_step "3. Differential Privacy 差分隐私"
assert_http "POST /dp/laplace" "POST" "${AGENT_URL}/dp/laplace" "200" \
    '{"value": 100.0, "sensitivity": 1.0, "epsilon": 1.0}'

assert_http "POST /dp/gaussian" "POST" "${AGENT_URL}/dp/gaussian" "200" \
    '{"value": 100.0, "sensitivity": 1.0, "epsilon": 1.0, "delta": 0.0001}'

assert_http "POST /dp/count" "POST" "${AGENT_URL}/dp/count" "200" \
    '{"count": 100, "sensitivity": 1, "epsilon": 1.0}'

assert_http "POST /dp/sum" "POST" "${AGENT_URL}/dp/sum" "200" \
    '{"values": [1.0, 2.0, 3.0], "sensitivity": 3.0, "epsilon": 1.0}'

assert_http "POST /dp/mean" "POST" "${AGENT_URL}/dp/mean" "200" \
    '{"values": [1.0, 2.0, 3.0, 4.0, 5.0], "sensitivity": 1.0, "epsilon": 1.0}'

assert_http "POST /dp/histogram" "POST" "${AGENT_URL}/dp/histogram" "200" \
    '{"true_counts": {"A": 10, "B": 20, "C": 30}, "epsilon": 1.0}'

# ── 4. K-Anonymity K-匿名 ──────────────────────────────────────────────
log_step "4. K-Anonymity K-匿名"
assert_http "POST /kano/generalize" "POST" "${AGENT_URL}/kano/generalize" "200" \
    '{"records": [{"age": 25, "city": "Beijing"}, {"age": 26, "city": "Shanghai"}], "k": 2, "quasi_identifiers": ["age", "city"]}'

# ── 5. Query Obfuscation 查询混淆 ──────────────────────────────────────
log_step "5. Query Obfuscation 查询混淆"
assert_http "POST /qol/obfuscate" "POST" "${AGENT_URL}/qol/obfuscate" "200" \
    '{"query": "SELECT * FROM patients WHERE name = '\''张三'\''", "num_dummies": 3}'

# ── 6. LDP 本地差分隐私 ────────────────────────────────────────────────
log_step "6. LDP 本地差分隐私"
assert_http "POST /ldp/perturb_binary" "POST" "${AGENT_URL}/ldp/perturb_binary" "200" \
    '{"value": true, "epsilon": 1.0}'

assert_http "POST /ldp/perturb_categorical" "POST" "${AGENT_URL}/ldp/perturb_categorical" "200" \
    '{"value": "A", "domain": ["A", "B", "C"], "epsilon": 1.0}'

# ── 7. 文件处理 ────────────────────────────────────────────────────────
log_step "7. 文件隐私处理"
assert_http "POST /file/mask" "POST" "${AGENT_URL}/file/mask" "200" \
    '{"content": "name,phone\n张三,13800138000\n李四,13900139000", "file_type": "csv"}'

# ── 8. Profile 推荐 ────────────────────────────────────────────────────
log_step "8. Privacy Profile 推荐"
assert_http "POST /profile/recommend" "POST" "${AGENT_URL}/profile/recommend" "200" \
    '{"data_type": "medical", "fields": ["name", "age", "diagnosis"]}'

# ── 9. Ops 诊断 ────────────────────────────────────────────────────────
log_step "9. Ops 运维诊断"
assert_http "GET /ops/diagnostics" "GET" "${AGENT_URL}/ops/diagnostics"

# ── 10. Metrics 指标 ───────────────────────────────────────────────────
log_step "10. Prometheus 指标端点"
assert_http "GET /metrics" "GET" "${AGENT_URL}/metrics"

# ── 结果汇总 ────────────────────────────────────────────────────────────
echo ""
echo "============================================================================"
echo "📊 Go Engine 集成测试结果汇总"
echo "   ✅ 通过: ${PASS_COUNT}"
echo "   ❌ 失败: ${FAIL_COUNT}"
echo "   📝 总计: $((PASS_COUNT + FAIL_COUNT))"
echo "============================================================================"

if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}✅ 所有测试通过！Go 引擎集成测试成功。${NC}"
    exit 0
else
    echo -e "${RED}❌ 存在 ${FAIL_COUNT} 个失败测试，请检查 Go 引擎服务状态。${NC}"
    exit 1
fi
