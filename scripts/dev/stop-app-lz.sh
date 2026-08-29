#!/usr/bin/env bash
# ============================================================================
# 一键停止 PrivShield 调度之眼控制台 (App-LZ)
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
PIDS_DIR="$PROJECT_ROOT/.pids"

echo "正在停止 PrivShield App-LZ 控制台所有服务..."

_kill_pid_file() {
    local file="$1"
    if [[ -f "$file" ]]; then
        local pid
        pid=$(cat "$file" 2>/dev/null || true)
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            sleep 0.2
            kill -9 "$pid" 2>/dev/null || true
        fi
        rm -f "$file"
    fi
}

_kill_pid_file "$PIDS_DIR/app-lz-bff.pid"
_kill_pid_file "$PIDS_DIR/app-lz-web.pid"
_kill_pid_file "$PIDS_DIR/app-lz-prod.pid"
_kill_pid_file "$PIDS_DIR/agent.pid"
_kill_pid_file "$PIDS_DIR/service-hub.pid"
_kill_pid_file "$PIDS_DIR/datasource-mgr.pid"
_kill_pid_file "$PIDS_DIR/audit-log.pid"

# 清理端口兜底 (跨平台 Linux/macOS)
for port in 8085 5174 8079 8082 8083 8084 50051 50052 50053 50054; do
    if command -v lsof >/dev/null 2>&1; then
        lsof -ti ":$port" | xargs kill -9 2>/dev/null || true
    elif command -v fuser >/dev/null 2>&1; then
        fuser -k -9 "$port/tcp" 2>/dev/null || true
    fi
done

echo "✅ App-LZ 与关联微服务已全部停止并释放端口。"
