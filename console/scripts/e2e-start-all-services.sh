#!/usr/bin/env bash
# ============================================================================
# Start All Real Services for E2E Integration Testing
# 启动全部真实服务（用于全流程集成测试）
#
# 启动服务：
#   1. PrivShield Agent  (Python REST)  :8079  — 分级脱敏核心引擎
#   2. service-hub       (Go)           :8082  — 数据服务调度中枢
#   3. datasource-mgr    (Go)           :8083  — 数据源管理
#   4. audit-log         (Go)           :8084  — 脱敏审计日志
#
# Usage:
#   bash console/scripts/e2e-start-all-services.sh
#
# 停止：
#   bash console/scripts/e2e-stop-all-services.sh
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")/../.."

REPO_ROOT="$(pwd)"
CONSOLE_DIR="${REPO_ROOT}/console"
PIDS_DIR="${CONSOLE_DIR}/.pids"
GO_BIN="${GO_BIN:-go}"
# 优先使用项目 venv 中的 Python（需要 Python 3.13+）
if [ -x "${REPO_ROOT}/.venv/bin/python" ]; then
    PYTHON="${PYTHON:-${REPO_ROOT}/.venv/bin/python}"
else
    PYTHON="${PYTHON:-python3}"
fi

mkdir -p "$PIDS_DIR"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_step()  { echo -e "${CYAN}[STEP]${NC}  $*"; }

# ── 检查依赖 ─────────────────────────────────────────────────────────
check_go() {
    if ! command -v "$GO_BIN" &>/dev/null; then
        for p in /Users/charles/go/go1.27.0/bin/go /usr/local/go/bin/go; do
            if [ -x "$p" ]; then
                GO_BIN="$p"
                break
            fi
        done
    fi
    if ! command -v "$GO_BIN" &>/dev/null; then
        log_error "Go compiler not found. Set GO_BIN env var."
        exit 1
    fi
    log_info "Go: $GO_BIN ($($GO_BIN version))"
}

check_python() {
    if ! command -v "$PYTHON" &>/dev/null; then
        # 尝试 venv
        if [ -x "${REPO_ROOT}/.venv/bin/python" ]; then
            PYTHON="${REPO_ROOT}/.venv/bin/python"
        fi
    fi
    if ! command -v "$PYTHON" &>/dev/null; then
        log_error "Python not found. Set PYTHON env var."
        exit 1
    fi
    log_info "Python: $PYTHON ($($PYTHON --version))"
}

wait_for_service() {
    local name="$1"
    local url="$2"
    local max_wait="${3:-30}"
    local count=0

    while [ $count -lt $max_wait ]; do
        if curl -sf -o /dev/null "$url" 2>/dev/null; then
            log_info "$name is ready at $url"
            return 0
        fi
        sleep 1
        count=$((count + 1))
    done

    log_error "$name failed to start within ${max_wait}s at $url"
    return 1
}

# ── 1. PrivShield Agent (Python REST) ────────────────────────────────
start_agent() {
    local port="${PRIVACY_REST_PORT:-8079}"
    local pid_file="${PIDS_DIR}/agent.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "PrivShield Agent already running (PID $(cat "$pid_file"))"
        return
    fi

    log_step "Starting PrivShield Agent on :${port}..."
    cd "$REPO_ROOT"
    PRIVACY_REST_HOST=127.0.0.1 PRIVACY_REST_PORT="$port" \
        $PYTHON -m engine.main --host 127.0.0.1 --port "$port" \
        > "${CONSOLE_DIR}/.pids/agent.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "Agent started (PID $(cat "$pid_file"))"

    if ! wait_for_service "PrivShield Agent" "http://127.0.0.1:${port}/health" 30; then
        log_error "Agent startup failed. Check ${CONSOLE_DIR}/.pids/agent.log"
        exit 1
    fi
}

# ── 2. service-hub (Go) ──────────────────────────────────────────────
start_service_hub() {
    local port="${SERVICE_HUB_PORT:-8082}"
    local pid_file="${PIDS_DIR}/service-hub.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "service-hub already running (PID $(cat "$pid_file"))"
        return
    fi

    log_step "Building & starting service-hub on :${port}..."
    cd "${PROJECT_ROOT}/services/service-hub"
    "$GO_BIN" build -o bin/service-hub ./cmd/server

    SERVICE_HUB_HOST=127.0.0.1 SERVICE_HUB_PORT="$port" \
        SERVICE_HUB_AGENT_REST_HOST=127.0.0.1 SERVICE_HUB_AGENT_REST_PORT=8079 \
        ./bin/service-hub > "${CONSOLE_DIR}/.pids/service-hub.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "service-hub started (PID $(cat "$pid_file"))"

    wait_for_service "service-hub" "http://127.0.0.1:${port}/api/health" 10
}

# ── 3. datasource-mgr (Go) ───────────────────────────────────────────
start_datasource_mgr() {
    local port="${DATASOURCE_MGR_PORT:-8083}"
    local pid_file="${PIDS_DIR}/datasource-mgr.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "datasource-mgr already running (PID $(cat "$pid_file"))"
        return
    fi

    log_step "Building & starting datasource-mgr on :${port}..."
    cd "${PROJECT_ROOT}/services/datasource-mgr"
    "$GO_BIN" build -o bin/datasource-mgr ./cmd/server

    DATASOURCE_MGR_HOST=127.0.0.1 DATASOURCE_MGR_PORT="$port" \
        DATASOURCE_MGR_AGENT_REST_HOST=127.0.0.1 DATASOURCE_MGR_AGENT_REST_PORT=8079 \
        ./bin/datasource-mgr > "${CONSOLE_DIR}/.pids/datasource-mgr.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "datasource-mgr started (PID $(cat "$pid_file"))"

    wait_for_service "datasource-mgr" "http://127.0.0.1:${port}/api/health" 10
}

# ── 4. audit-log (Go) ────────────────────────────────────────────────
start_audit_log() {
    local port="${AUDIT_LOG_PORT:-8084}"
    local pid_file="${PIDS_DIR}/audit-log.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "audit-log already running (PID $(cat "$pid_file"))"
        return
    fi

    log_step "Building & starting audit-log on :${port}..."
    cd "${PROJECT_ROOT}/services/audit-log"
    "$GO_BIN" build -o bin/audit-log ./cmd/server

    AUDIT_LOG_HOST=127.0.0.1 AUDIT_LOG_PORT="$port" \
        AUDIT_LOG_AGENT_REST_HOST=127.0.0.1 AUDIT_LOG_AGENT_REST_PORT=8079 \
        ./bin/audit-log > "${CONSOLE_DIR}/.pids/audit-log.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "audit-log started (PID $(cat "$pid_file"))"

    wait_for_service "audit-log" "http://127.0.0.1:${port}/api/health" 10
}

# ── 启动所有服务 ─────────────────────────────────────────────────────
cd "$REPO_ROOT"

log_step "Checking dependencies..."
check_go
check_python

echo ""
log_step "Starting all services..."
echo ""

start_agent
start_service_hub
start_datasource_mgr
start_audit_log

echo ""
log_info "╔══════════════════════════════════════════════════════════════╗"
log_info "║          All 4 services started successfully!                ║"
log_info "╠══════════════════════════════════════════════════════════════╣"
log_info "║  PrivShield Agent  → http://127.0.0.1:8079  (分级脱敏引擎)   ║"
log_info "║  service-hub       → http://127.0.0.1:8082  (调度中枢)       ║"
log_info "║  datasource-mgr    → http://127.0.0.1:8083  (数据源管理)     ║"
log_info "║  audit-log         → http://127.0.0.1:8084  (审计日志)       ║"
log_info "╠══════════════════════════════════════════════════════════════╣"
log_info "║  Run E2E tests:                                              ║"
log_info "║    cd services/service-hub && go test -v -run TestRealE2E     ║"
log_info "║  Stop all:                                                   ║"
log_info "║    bash console/scripts/e2e-stop-all-services.sh             ║"
log_info "╚══════════════════════════════════════════════════════════════╝"
