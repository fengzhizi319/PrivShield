#!/usr/bin/env bash
# ============================================================================
# Stop All Real Services (E2E Integration Testing Cleanup)
# 停止全部真实服务
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")/../.."

CONSOLE_DIR="$(pwd)/console"
PIDS_DIR="${CONSOLE_DIR}/.pids"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }

stop_module() {
    local name="$1"
    local pid_file="${PIDS_DIR}/${name}.pid"

    if [ ! -f "$pid_file" ]; then
        log_warn "${name}: no PID file"
        return
    fi

    local pid
    pid=$(cat "$pid_file")

    if kill -0 "$pid" 2>/dev/null; then
        log_info "Stopping ${name} (PID ${pid})..."
        kill "$pid"
        local count=0
        while kill -0 "$pid" 2>/dev/null && [ $count -lt 20 ]; do
            sleep 0.5
            count=$((count + 1))
        done
        if kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid" 2>/dev/null || true
        fi
        log_info "${name} stopped"
    else
        log_warn "${name} (PID ${pid}) not running"
    fi
    rm -f "$pid_file"
}

# 停止顺序：先停 Go 模块，再停 Agent
stop_module "service-hub"
stop_module "datasource-mgr"
stop_module "audit-log"
stop_module "agent"

echo ""
log_info "All services stopped."
