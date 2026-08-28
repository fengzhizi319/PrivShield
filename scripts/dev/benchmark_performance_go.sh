#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: benchmark_performance_go.sh
# 脚本说明: PrivShield Go 原生引擎隐私原语与分类漏斗性能基准测试工具。
#
# 与 benchmark_performance.sh 的区别：
#   - benchmark_performance.sh 使用 Python 多线程 HTTP 压测引擎
#   - 本脚本使用 Go 原生引擎作为压测目标，同时使用 Go 原生并发模型执行 HTTP 压测
#
# 执行步骤总览：
#   1. 解析命令行参数（--host、--port、--requests、--concurrency）
#   2. 对 Go Engine 脱敏原语（/mask）执行高并发吞吐压测
#   3. 对差分隐私原语（/dp/laplace）执行加噪性能压测
#   4. 对动态分类分级（/dynclassification/classify）执行端到端压测
#   5. 汇总结算 RPS 与延迟百分位数分布
#
# 用法 / Usage:
#   ./scripts/dev/benchmark_performance_go.sh [选项]
# ==============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

REST_HOST="${PRIVACY_REST_HOST:-127.0.0.1}"
REST_PORT="${PRIVACY_REST_PORT:-8079}"
NUM_REQUESTS=200
CONCURRENCY=10

usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  --host HOST          REST 服务主机 (默认: 127.0.0.1)
  --port PORT          REST 服务端口 (默认: 8079)
  -n, --requests NUM   基准测试请求总数 (默认: 200)
  -c, --concurrency C  并发线程数 (默认: 10)
  -h, --help           显示帮助信息并退出

使用示例:
  ./scripts/dev/benchmark_performance_go.sh
  ./scripts/dev/benchmark_performance_go.sh -n 500 -c 20
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --host) REST_HOST="$2"; shift 2 ;;
        --port) REST_PORT="$2"; shift 2 ;;
        -n|--requests) NUM_REQUESTS="$2"; shift 2 ;;
        -c|--concurrency) CONCURRENCY="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "未知参数: $1"; exit 1 ;;
    esac
done

BASE_URL="http://${REST_HOST}:${REST_PORT}"

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} PrivShield Go Engine 性能基准测试${NC}"
echo -e "${BLUE} 目标地址  : ${BASE_URL}${NC}"
echo -e "${BLUE} 请求总数  : ${NUM_REQUESTS}${NC}"
echo -e "${BLUE} 并发线程  : ${CONCURRENCY}${NC}"
echo -e "${BLUE}====================================================${NC}"

# 检查 Go Engine 是否可达
if ! curl -sf -o /dev/null "${BASE_URL}/health" 2>/dev/null; then
    echo -e "${RED}[错误] Go Engine 未在 ${BASE_URL} 响应，请先启动服务。${NC}"
    exit 1
fi

# 基准测试函数
run_bench() {
    local desc="$1"
    local endpoint="$2"
    local body="$3"

    echo -e "\n${YELLOW}[BENCH] ${desc}${NC}"
    echo -e "  端点: POST ${endpoint}"

    local tmp_dir
    tmp_dir=$(mktemp -d)
    local start_time end_time total_time

    start_time=$(date +%s%N)

    for (( i=0; i<NUM_REQUESTS; i++ )); do
        (
            local req_start req_end
            req_start=$(date +%s%N)
            curl -sf -o /dev/null -X POST \
                -H "Content-Type: application/json" \
                -d "$body" \
                --max-time 10 \
                "${BASE_URL}${endpoint}" 2>/dev/null
            req_end=$(date +%s%N)
            echo $(( (req_end - req_start) / 1000000 )) > "${tmp_dir}/latency_${i}.ms"
        ) &
        # 控制并发
        if (( (i + 1) % CONCURRENCY == 0 )); then
            wait
        fi
    done
    wait

    end_time=$(date +%s%N)
    total_time=$(( (end_time - start_time) / 1000000 ))

    # 统计延迟
    local latencies
    latencies=$(cat "${tmp_dir}"/latency_*.ms 2>/dev/null | sort -n)
    local count
    count=$(echo "$latencies" | wc -l | tr -d ' ')

    if [ "$count" -gt 0 ]; then
        local p50_idx=$(( count * 50 / 100 ))
        local p95_idx=$(( count * 95 / 100 ))
        local p99_idx=$(( count * 99 / 100 ))
        [ "$p50_idx" -lt 1 ] && p50_idx=1
        [ "$p95_idx" -lt 1 ] && p95_idx=1
        [ "$p99_idx" -lt 1 ] && p99_idx=1

        local p50 p95 p99 avg rps
        p50=$(echo "$latencies" | sed -n "${p50_idx}p")
        p95=$(echo "$latencies" | sed -n "${p95_idx}p")
        p99=$(echo "$latencies" | sed -n "${p99_idx}p")
        avg=$(echo "$latencies" | awk '{ sum += $1 } END { if (NR>0) printf "%.1f", sum/NR; else print "0" }')
        if [ "$total_time" -gt 0 ]; then
            rps=$(awk "BEGIN { printf \"%.1f\", $count / ($total_time / 1000.0) }")
        else
            rps="N/A"
        fi

        echo -e "  ${GREEN}成功请求: ${count}/${NUM_REQUESTS}${NC}"
        echo -e "  ${GREEN}总耗时   : ${total_time} ms${NC}"
        echo -e "  ${GREEN}RPS      : ${rps} req/s${NC}"
        echo -e "  ${CYAN}延迟 P50 : ${p50} ms${NC}"
        echo -e "  ${CYAN}延迟 P95 : ${p95} ms${NC}"
        echo -e "  ${CYAN}延迟 P99 : ${p99} ms${NC}"
        echo -e "  ${CYAN}平均延迟 : ${avg} ms${NC}"
    else
        echo -e "  ${RED}无成功请求${NC}"
    fi

    rm -rf "$tmp_dir"
}

# ── 1. Masking 脱敏基准 ────────────────────────────────────────────────
run_bench "Masking 脱敏 (MaskRecord)" "/mask" \
    '{"record": {"name": "张三", "phone": "13800138000", "email": "test@example.com", "id_card": "110101199001011234"}}'

# ── 2. Differential Privacy 基准 ───────────────────────────────────────
run_bench "DP Laplace 加噪" "/dp/laplace" \
    '{"value": 100.0, "sensitivity": 1.0, "epsilon": 1.0}'

run_bench "DP Gaussian 加噪" "/dp/gaussian" \
    '{"value": 100.0, "sensitivity": 1.0, "epsilon": 1.0, "delta": 0.0001}'

# ── 3. K-Anonymity 基准 ───────────────────────────────────────────────
run_bench "K-Anonymity 泛化" "/kano/generalize" \
    '{"records": [{"age": 25, "city": "Beijing"}, {"age": 26, "city": "Shanghai"}], "k": 2, "quasi_identifiers": ["age", "city"]}'

# ── 4. LDP 基准 ───────────────────────────────────────────────────────
run_bench "LDP Binary 扰动" "/ldp/perturb_binary" \
    '{"value": true, "epsilon": 1.0}'

# ── 5. Query Obfuscation 基准 ─────────────────────────────────────────
run_bench "Query Obfuscation 混淆" "/qol/obfuscate" \
    '{"query": "SELECT * FROM patients WHERE name = '\''张三'\''", "num_dummies": 3}'

# ── 6. 动态分类分级基准 ───────────────────────────────────────────────
run_bench "动态分类分级 (Layer-1 Rule)" "/dynclassification/classify" \
    '{"fields": {"name": "张三", "phone": "13800138000", "diagnosis": "高血压"}}'

echo ""
echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} Go Engine 性能基准测试完成${NC}"
echo -e "${BLUE}====================================================${NC}"
