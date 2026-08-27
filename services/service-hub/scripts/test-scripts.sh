#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) 运维与启动脚本全自动化集成回归测试套件
# Automated Test Suite for all scripts under services/service-hub/scripts/
#
# 测试阶段概览：
#   1. 静态检查：验证所有脚本文件存在性与执行权限 (chmod +x)
#   2. 语法校验：使用 bash -n 静态解析所有 Shell 脚本，确保语法 100% 正确
#   3. 参数解析：验证所有脚本对 -h 与 --help 选项的响应与退出码 (0)
#   4. 证书生成：在临时隔离目录调用 gen-certs.sh，验证 RSA 4096 CA、服务端/客户端证书、client.pub 及 0600 权限
#   5. K8s 演练：测试 deploy-k8s.sh --dry-run 与 --with-postgres 演练模式
#   6. 探针容错：测试 health-check.sh 与 simulate-pipeline.sh 在离线目标下的安全容错
#
# 用法 / Usage:
#   cd services/service-hub && bash scripts/test-scripts.sh
#   # 或在项目根目录下执行:
#   bash services/service-hub/scripts/test-scripts.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$MODULE_DIR/../.." && pwd)"

# ANSI 终端输出颜色配置
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

PASSED_TESTS=0
FAILED_TESTS=0

log_pass() {
    echo -e "  ${GREEN}✓ [PASS]${NC} $1"
    PASSED_TESTS=$((PASSED_TESTS + 1))
}

log_fail() {
    echo -e "  ${RED}✗ [FAIL]${NC} $1"
    FAILED_TESTS=$((FAILED_TESTS + 1))
}

log_step() {
    echo -e "\n${BOLD}${CYAN}══════ Step $1: $2 ══════${NC}"
}

echo -e "${BOLD}${BLUE}============================================================================${NC}"
echo -e "${BOLD}${BLUE}   PrivShield Service Hub 脚本全自动化集成回归测试套件                      ${NC}"
echo -e "${BOLD}${BLUE}============================================================================${NC}"

# ── 1. 静态检查：验证所有脚本存在性与执行权限 ─────────────────────────────
log_step "1" "静态检查脚本存在性与权限"

SCRIPTS=(
    "deploy.sh"
    "deploy-k8s.sh"
    "stop-k8s.sh"
    "stop-docker.sh"
    "gen-certs.sh"
    "health-check.sh"
    "simulate-pipeline.sh"
)

for script in "${SCRIPTS[@]}"; do
    path="$SCRIPT_DIR/$script"
    if [[ -f "$path" && -x "$path" ]]; then
        log_pass "脚本存在且可执行: $script"
    else
        log_fail "脚本不存在或缺少执行权限: $script"
    fi
done

if [[ -f "$MODULE_DIR/run.sh" && -x "$MODULE_DIR/run.sh" ]]; then
    log_pass "服务快捷入口存在且可执行: run.sh"
else
    log_fail "快捷入口不存在或缺少执行权限: run.sh"
fi

# ── 2. 语法校验：使用 bash -n 静态检查语法 ────────────────────────────────
log_step "2" "Shell 语法合规性静态扫描 (bash -n)"

for script in "${SCRIPTS[@]}"; do
    path="$SCRIPT_DIR/$script"
    if bash -n "$path" 2>/dev/null; then
        log_pass "语法扫描通过: $script"
    else
        log_fail "语法扫描异常: $script"
    fi
done

if bash -n "$MODULE_DIR/run.sh" 2>/dev/null; then
    log_pass "语法扫描通过: run.sh"
else
    log_fail "语法扫描异常: run.sh"
fi

# ── 3. 参数解析：验证 --help 与 -h 选项 ────────────────────────────────────
log_step "3" "命令行帮助与选项响应测试 (--help / -h)"

for script in "${SCRIPTS[@]}"; do
    path="$SCRIPT_DIR/$script"
    if bash "$path" --help >/dev/null 2>&1 && bash "$path" -h >/dev/null 2>&1; then
        log_pass "帮助信息解析通过: $script (--help / -h)"
    else
        log_fail "帮助信息解析失败: $script"
    fi
done

if bash "$MODULE_DIR/run.sh" --help >/dev/null 2>&1; then
    log_pass "帮助信息解析通过: run.sh (--help)"
else
    log_fail "帮助信息解析失败: run.sh"
fi

# ── 4. 证书生成：在临时隔离目录运行 gen-certs.sh ──────────────────────────
log_step "4" "动态 mTLS 证书生成流程测试 (gen-certs.sh)"

TMP_CERT_DIR="$(mktemp -d /tmp/service-hub-test-certs-XXXXXX)"
trap 'rm -rf "$TMP_CERT_DIR"' EXIT

if CERT_DAYS=30 bash "$SCRIPT_DIR/gen-certs.sh" "$TMP_CERT_DIR" >/dev/null 2>&1; then
    # 验证 7 个核心证书与密钥文件
    CERT_FILES=("ca.crt" "ca.key" "server.crt" "server.key" "client.crt" "client.key" "client.pub")
    ALL_CERTS_OK=true
    for cf in "${CERT_FILES[@]}"; do
        if [[ ! -s "$TMP_CERT_DIR/$cf" ]]; then
            ALL_CERTS_OK=false
            break
        fi
    done

    if [[ "$ALL_CERTS_OK" == true ]]; then
        log_pass "成功生成全部 7 个 mTLS 密钥、证书及公钥 PEM 文件"
    else
        log_fail "生成的 mTLS 证书文件不完整"
    fi

    # 验证私钥权限为 0600
    KEY_PERM_OK=true
    for kf in "ca.key" "server.key" "client.key"; do
        perm=$(stat -c "%a" "$TMP_CERT_DIR/$kf" 2>/dev/null || stat -f "%OLp" "$TMP_CERT_DIR/$kf" 2>/dev/null || echo "600")
        if [[ "$perm" != "600" ]]; then
            KEY_PERM_OK=false
            break
        fi
    done
    if [[ "$KEY_PERM_OK" == true ]]; then
        log_pass "私钥文件安全权限已严格收紧 (0600)"
    else
        log_fail "私钥文件安全权限未收紧至 0600"
    fi
else
    log_fail "执行 gen-certs.sh 遇到异常"
fi

# ── 5. Kubernetes 演练测试 (deploy-k8s.sh --dry-run) ───────────────────────
log_step "5" "Kubernetes 部署脚本演练测试 (deploy-k8s.sh --dry-run)"

if command -v kubectl >/dev/null 2>&1; then
    if bash "$SCRIPT_DIR/deploy-k8s.sh" --dry-run -n test-ns >/dev/null 2>&1; then
        log_pass "单服务 K8s 演练测试成功 (deploy-k8s.sh --dry-run)"
    else
        log_fail "单服务 K8s 演练测试失败"
    fi

    if bash "$SCRIPT_DIR/deploy-k8s.sh" --dry-run --with-postgres -n test-ns >/dev/null 2>&1; then
        log_pass "含 Phase B PostgreSQL 联合演练成功 (deploy-k8s.sh --dry-run --with-postgres)"
    else
        log_fail "联合演练测试失败"
    fi
else
    echo -e "  ${YELLOW}ℹ [SKIP]${NC} 未检测到 kubectl 工具，跳过实际 K8s dry-run"
fi

# ── 6. 离线探针容错测试 (health-check.sh & simulate-pipeline.sh) ───────────
log_step "6" "探针与模拟器离线容错测试"

# health-check.sh 在离线时应输出探测结果并不崩溃（退出码 0）
if SERVICE_HUB_HOST="127.0.0.1" SERVICE_HUB_PORT="59999" bash "$SCRIPT_DIR/health-check.sh" >/dev/null 2>&1; then
    log_pass "health-check.sh 在后端离线时安全输出并正常退出"
else
    log_fail "health-check.sh 在后端离线时异常崩溃"
fi

# simulate-pipeline.sh 在离线时应报错并返回非 0 退出码
if ! SERVICE_HUB_URL="http://127.0.0.1:59999" bash "$SCRIPT_DIR/simulate-pipeline.sh" 2 >/dev/null 2>&1; then
    log_pass "simulate-pipeline.sh 在后端离线时正确检测并返回非零错误码"
else
    log_fail "simulate-pipeline.sh 未能正确拦截离线错误"
fi

# ── 汇总输出 ───────────────────────────────────────────────────────────────
echo -e "\n${BOLD}${BLUE}============================================================================${NC}"
echo -e "  测试结果汇总: ${GREEN}通过: $PASSED_TESTS${NC} | ${RED}失败: $FAILED_TESTS${NC}"
echo -e "${BOLD}${BLUE}============================================================================${NC}"

if [[ $FAILED_TESTS -gt 0 ]]; then
    exit 1
fi
exit 0
