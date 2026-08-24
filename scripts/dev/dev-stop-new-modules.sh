#!/usr/bin/env bash
# ============================================================================
# Development Stop Script for Three New Microservice Modules
# 三个中台微服务模块的停止脚本
#
# 停止模块：
#   1. service-hub    (数据服务调度中枢)
#   2. datasource-mgr (数据源管理)
#   3. audit-log      (脱敏审计日志)
#
# Usage:
#   bash scripts/dev/dev-stop-new-modules.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PIDS_DIR="${PROJECT_ROOT}/.pids"
LEGACY_PIDS_DIR="${PROJECT_ROOT}/console/.pids"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

stop_module() {
    local name="$1"
    local pid_file=""

    if [ -f "${PIDS_DIR}/${name}.pid" ]; then
        pid_file="${PIDS_DIR}/${name}.pid"
    elif [ -f "${LEGACY_PIDS_DIR}/${name}.pid" ]; then
        pid_file="${LEGACY_PIDS_DIR}/${name}.pid"
    fi

    if [ -z "$pid_file" ] || [ ! -f "$pid_file" ]; then
        log_warn "${name}: no PID file found, may not be running"
        return
    fi

    local pid
    pid=$(cat "$pid_file")

    if kill -0 "$pid" 2>/dev/null; then
        log_info "Stopping ${name} (PID ${pid})..."
        kill "$pid" 2>/dev/null || true
        local count=0
        while kill -0 "$pid" 2>/dev/null && [ $count -lt 20 ]; do
            sleep 0.5
            count=$((count + 1))
        done

        if kill -0 "$pid" 2>/dev/null; then
            log_warn "${name} (PID ${pid}) did not exit, sending SIGKILL..."
            kill -9 "$pid" 2>/dev/null || true
        fi
        log_info "${name} stopped"
    else
        log_warn "${name} (PID ${pid}) is not running"
    fi

    rm -f "$pid_file"
}

stop_module "service-hub"
stop_module "datasource-mgr"
stop_module "audit-log"

echo ""
log_info "=========================================="
log_info "  All 3 microservices stopped."
log_info "=========================================="
