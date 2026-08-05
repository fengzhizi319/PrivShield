#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键停止控制台全部开发服务
# Stop all dev mode console services (Agent, Backends, Vite Dev Server)
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONSOLE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

AGENT_PID_FILE="$CONSOLE_DIR/.pids/agent.pid"
AGENT_GO_PID_FILE="$CONSOLE_DIR/.pids/agent-go.pid"
AGENT_ALL_PID_FILE="$CONSOLE_DIR/.pids/agent-all.pid"
AGENT_GO_MTLS_PID_FILE="$CONSOLE_DIR/.pids/agent-go-mtls.pid"
CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console.pid"
CONSOLE_GO_PID_FILE="$CONSOLE_DIR/.pids/console-go.pid"
CONSOLE_ALL_PID_FILE="$CONSOLE_DIR/.pids/console-all.pid"
CONSOLE_GO_ALL_PID_FILE="$CONSOLE_DIR/.pids/console-go-all.pid"
CONSOLE_GO_MTLS_PID_FILE="$CONSOLE_DIR/.pids/console-go-mtls.pid"
VITE_PID_FILE="$CONSOLE_DIR/.pids/vite-dev.pid"

kill_by_pid_file() {
    local file="$1"
    local name="$2"
    if [[ -f "$file" ]]; then
        local pid
        pid=$(cat "$file")
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            echo "停止 $name (PID $pid)..."
            kill "$pid" 2>/dev/null || true
            sleep 0.5
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
        rm -f "$file"
    fi
}

kill_by_port() {
    local port="$1"
    local name="$2"
    local pids=""
    if command -v lsof >/dev/null 2>&1; then
        pids=$(lsof -t -i :"$port" 2>/dev/null | sort -u | tr '\n' ' ')
    elif command -v fuser >/dev/null 2>&1; then
        pids=$(fuser "$port"/tcp 2>/dev/null | tr -s ' ')
    fi

    if [[ -n "$pids" ]]; then
        echo "清理端口 $port 上的残余进程 ($name: $pids)..."
        for pid in $pids; do
            kill -9 "$pid" 2>/dev/null || true
        done
    fi
}

echo "正在停止【开发模式】控制台所有服务..."

kill_by_pid_file "$VITE_PID_FILE" "Vite 开发服务器"
kill_by_pid_file "$CONSOLE_GO_MTLS_PID_FILE" "Go gRPC 代理后端 (mTLS)"
kill_by_pid_file "$CONSOLE_GO_ALL_PID_FILE" "Go gRPC 代理后端 (all)"
kill_by_pid_file "$CONSOLE_GO_PID_FILE" "Go gRPC 代理后端"
kill_by_pid_file "$CONSOLE_ALL_PID_FILE" "Python REST 代理后端 (all)"
kill_by_pid_file "$CONSOLE_PID_FILE" "Python REST 代理后端"
kill_by_pid_file "$AGENT_GO_MTLS_PID_FILE" "privacy_local_agent (mTLS)"
kill_by_pid_file "$AGENT_ALL_PID_FILE" "privacy_local_agent (all)"
kill_by_pid_file "$AGENT_GO_PID_FILE" "privacy_local_agent (gRPC)"
kill_by_pid_file "$AGENT_PID_FILE" "privacy_local_agent (REST)"

# 端口清理
kill_by_port 5173 "Vite 前端开发服务器"
kill_by_port 8081 "Go gRPC 代理后端"
kill_by_port 8080 "Python REST 代理后端"
kill_by_port 50051 "privacy_local_agent gRPC"
kill_by_port 8079 "privacy_local_agent REST"

echo "✅ 开发模式所有服务已安全停止。"
