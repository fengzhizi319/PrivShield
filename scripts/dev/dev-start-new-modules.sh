#!/usr/bin/env bash
# ============================================================================
# Development Startup Script for Three New Microservice Modules
# 三个中台微服务模块的开发模式一键启动脚本
#
# 启动模块：
#   1. service-hub    (数据服务调度中枢)  :8082
#   2. datasource-mgr (数据源管理)        :8083
#   3. audit-log      (脱敏审计日志)      :8084
#
# 前置条件：
#   - Go 编译器已安装
#   - PrivShield Agent 已运行（REST: 8079）
#
# Usage:
#   bash scripts/dev/dev-start-new-modules.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PIDS_DIR="${PROJECT_ROOT}/.pids"
LOGS_DIR="${PROJECT_ROOT}/.logs"
DATA_DIR="${PROJECT_ROOT}/data"
GO_BIN="${GO_BIN:-go}"

mkdir -p "$PIDS_DIR" "$LOGS_DIR" "$DATA_DIR"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

if ! command -v "$GO_BIN" &>/dev/null; then
    for p in /Users/charles/go/go1.27.0/bin/go /usr/local/go/bin/go; do
        if [ -x "$p" ]; then
            GO_BIN="$p"
            break
        fi
    done
fi

if ! command -v "$GO_BIN" &>/dev/null; then
    log_error "Go compiler not found. Set GO_BIN env var or install Go."
    exit 1
fi

log_info "Using Go: $GO_BIN ($($GO_BIN version))"

# ── 模块 1: service-hub ──────────────────────────────────────────────
start_service_hub() {
    local port="${SERVICE_HUB_PORT:-8082}"
    local pid_file="${PIDS_DIR}/service-hub.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "service-hub already running (PID $(cat "$pid_file"))"
        return
    fi

    log_info "Building service-hub..."
    cd "${PROJECT_ROOT}/services/service-hub"
    SERVICE_HUB_HOST=127.0.0.1 SERVICE_HUB_PORT="$port" \
        "$GO_BIN" build -o bin/service-hub ./cmd/server

    log_info "Starting service-hub on :${port}..."
    SERVICE_HUB_HOST=127.0.0.1 SERVICE_HUB_PORT="$port" \
        SERVICE_HUB_AGENT_REST_HOST=127.0.0.1 SERVICE_HUB_AGENT_REST_PORT=8079 \
        SERVICE_HUB_DB_PATH="${DATA_DIR}/service-hub.db" \
        ./bin/service-hub >> "${LOGS_DIR}/service-hub.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "service-hub started (PID $(cat "$pid_file"))"
}

# ── 模块 2: datasource-mgr ───────────────────────────────────────────
start_datasource_mgr() {
    local port="${DATASOURCE_MGR_PORT:-8083}"
    local pid_file="${PIDS_DIR}/datasource-mgr.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "datasource-mgr already running (PID $(cat "$pid_file"))"
        return
    fi

    log_info "Building datasource-mgr..."
    cd "${PROJECT_ROOT}/services/datasource-mgr"
    DATASOURCE_MGR_HOST=127.0.0.1 DATASOURCE_MGR_PORT="$port" \
        "$GO_BIN" build -o bin/datasource-mgr ./cmd/server

    log_info "Starting datasource-mgr on :${port}..."
    DATASOURCE_MGR_HOST=127.0.0.1 DATASOURCE_MGR_PORT="$port" \
        DATASOURCE_MGR_AGENT_REST_HOST=127.0.0.1 DATASOURCE_MGR_AGENT_REST_PORT=8079 \
        DATASOURCE_MGR_DB_PATH="${DATA_DIR}/datasource-mgr.db" \
        ./bin/datasource-mgr >> "${LOGS_DIR}/datasource-mgr.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "datasource-mgr started (PID $(cat "$pid_file"))"
}

# ── 模块 3: audit-log ────────────────────────────────────────────────
start_audit_log() {
    local port="${AUDIT_LOG_PORT:-8084}"
    local pid_file="${PIDS_DIR}/audit-log.pid"

    if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
        log_warn "audit-log already running (PID $(cat "$pid_file"))"
        return
    fi

    log_info "Building audit-log..."
    cd "${PROJECT_ROOT}/services/audit-log"
    AUDIT_LOG_HOST=127.0.0.1 AUDIT_LOG_PORT="$port" \
        "$GO_BIN" build -o bin/audit-log ./cmd/server

    log_info "Starting audit-log on :${port}..."
    AUDIT_LOG_HOST=127.0.0.1 AUDIT_LOG_PORT="$port" \
        AUDIT_LOG_AGENT_REST_HOST=127.0.0.1 AUDIT_LOG_AGENT_REST_PORT=8079 \
        AUDIT_LOG_DB_PATH="${DATA_DIR}/audit-log.db" \
        ./bin/audit-log >> "${LOGS_DIR}/audit-log.log" 2>&1 &
    echo $! > "$pid_file"
    log_info "audit-log started (PID $(cat "$pid_file"))"
}

# ── 启动全部 ─────────────────────────────────────────────────────────
cd "$PROJECT_ROOT"
start_service_hub
start_datasource_mgr
start_audit_log

echo ""
log_info "=========================================="
log_info "  All 3 microservices started!"
log_info "  service-hub    : http://127.0.0.1:${SERVICE_HUB_PORT:-8082}"
log_info "  datasource-mgr : http://127.0.0.1:${DATASOURCE_MGR_PORT:-8083}"
log_info "  audit-log      : http://127.0.0.1:${AUDIT_LOG_PORT:-8084}"
log_info "=========================================="
