#!/usr/bin/env bash
# ============================================================================
# 【正式部署/生产模式】一键停止控制台全部服务
# Stop all prod mode console services
#
# ⚠️ 注意 / WARNING:
#   本脚本除按 console/.pids/ 中的 PID 文件精确停止外，还会对固定端口
#   (8080/8081/8079/50051) 上残留的任何进程执行 kill 清理。
#   若你在这些端口上手动启动了自己的进程（非本脚本拉起的），也会被终止。
#   清理策略为先 SIGTERM 优雅退出、1 秒后仍存活再 SIGKILL 强杀。
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
        # 两段式终止：先 SIGTERM 允许进程优雅退出，1 秒后仍存活再 SIGKILL
        for pid in $pids; do
            kill -15 "$pid" 2>/dev/null || true
        done
        sleep 1
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        done
    fi
}

echo "正在停止【生产模式】控制台所有服务..."

kill_by_pid_file "$CONSOLE_GO_MTLS_PID_FILE" "Go gRPC 代理后端 (mTLS)"
kill_by_pid_file "$CONSOLE_GO_ALL_PID_FILE" "Go gRPC 代理后端 (all)"
kill_by_pid_file "$CONSOLE_GO_PID_FILE" "Go gRPC 代理后端"
kill_by_pid_file "$CONSOLE_ALL_PID_FILE" "Python REST 代理后端 (all)"
kill_by_pid_file "$CONSOLE_PID_FILE" "Python REST 代理后端"
kill_by_pid_file "$AGENT_GO_MTLS_PID_FILE" "privacy_local_agent (mTLS)"
kill_by_pid_file "$AGENT_ALL_PID_FILE" "privacy_local_agent (all)"
kill_by_pid_file "$AGENT_GO_PID_FILE" "privacy_local_agent (gRPC)"
kill_by_pid_file "$AGENT_PID_FILE" "privacy_local_agent (REST)"

kill_by_port 8081 "Go gRPC 代理后端"
kill_by_port 8080 "Python REST 代理后端"
kill_by_port 50051 "privacy_local_agent gRPC"
kill_by_port 8079 "privacy_local_agent REST"

echo "✅ 生产模式所有服务已安全停止。"
