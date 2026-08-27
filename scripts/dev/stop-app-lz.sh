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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
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

# 清理端口兜底
fuser -k -9 8085/tcp 2>/dev/null || true
fuser -k -9 5174/tcp 2>/dev/null || true

echo "App-LZ 控制台服务已全部停止。"
