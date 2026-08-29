#!/usr/bin/env bash
# ============================================================================
# Stop All Real Services (E2E Integration Testing Cleanup)
# 停止全部真实服务
#
# Usage:
#   bash scripts/dev/e2e-stop-all-services.sh
# ============================================================================

set -euo pipefail

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -h, --help    显示帮助信息并退出"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
PROJECT_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || (cd "$SCRIPT_DIR/../.." && pwd -P))"
PIDS_DIR="${PROJECT_ROOT}/.pids"
LEGACY_PIDS_DIR="${PROJECT_ROOT}/console/.pids"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }

# ── stop_module: 通过 PID 文件停止单个模块 ────────────────────────────
# 1. 在 .pids/ 和 console/.pids/ 中查找 PID 文件（优先新版目录）
# 2. 发送 SIGTERM 优雅退出，每 0.5s 检查一次，最多等待 10s
# 3. 超时后发送 SIGKILL 强杀
# 4. 删除 PID 文件
stop_module() {
    local name="$1"
    local pid_file=""

    if [ -f "${PIDS_DIR}/${name}.pid" ]; then
        pid_file="${PIDS_DIR}/${name}.pid"
    elif [ -f "${LEGACY_PIDS_DIR}/${name}.pid" ]; then
        pid_file="${LEGACY_PIDS_DIR}/${name}.pid"
    fi

    if [ -z "$pid_file" ] || [ ! -f "$pid_file" ]; then
        log_warn "${name}: no PID file"
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
            kill -9 "$pid" 2>/dev/null || true
        fi
        log_info "${name} stopped"
    else
        log_warn "${name} (PID ${pid}) not running"
    fi
    rm -f "$pid_file"
}

# ── 停止顺序：先停 Go 微服务群，再停 Agent（避免服务依赖导致僵尸进程） ──
stop_module "service-hub"
stop_module "datasource-mgr"
stop_module "audit-log"
stop_module "privshield-gateway"
stop_module "privshield-agent"
stop_module "agent"

echo ""
log_info "All E2E services stopped."
