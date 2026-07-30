#!/usr/bin/env bash
# ============================================================================
# 停止由 ./console/start-go.sh 或 ./console/start-go-mtls.sh 启动的服务
# Stop services launched by ./console/start-go.sh or ./console/start-go-mtls.sh
#
# 用法 / Usage：./console/stop-go.sh
#
# 逻辑说明 / Logic:
#   1. 读取 .pids/ 目录下由 start-go.sh / start-go-mtls.sh 写入的 PID 文件
#      Read PID files written by start-go.sh / start-go-mtls.sh from .pids/ directory
#   2. 同时处理普通模式和 mTLS 模式的 PID 文件（共 4 个）
#      Handle both normal mode and mTLS mode PID files (4 total)
#   3. 删除 PID 文件，输出停止结果
#      Remove PID files and print stop results
# ============================================================================

# 启用严格模式：遇到错误立即退出(-e)、未定义变量报错(-u)、管道失败传播(-o pipefail)
# Enable strict mode: exit on error(-e), error on undefined vars(-u), pipe failure propagates(-o pipefail)
set -euo pipefail

# 获取脚本自身所在目录的绝对路径
# Get the absolute path of the directory containing this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# PID 文件存储目录
# PID file storage directory
PID_DIR="$SCRIPT_DIR/.pids"

# start-go.sh 写入的 PID 文件（普通模式）
# PID files written by start-go.sh (normal mode)
AGENT_PID_FILE="$PID_DIR/agent-go.pid"        # agent 主服务 / main agent service
CONSOLE_PID_FILE="$PID_DIR/console-go.pid"    # Go gRPC 代理后端 / Go gRPC proxy backend
# start-go-mtls.sh 写入的 PID 文件（mTLS 模式）
# PID files written by start-go-mtls.sh (mTLS mode)
AGENT_MTLS_PID_FILE="$PID_DIR/agent-go-mtls.pid"      # agent (mTLS) / agent with mTLS
CONSOLE_MTLS_PID_FILE="$PID_DIR/console-go-mtls.pid"  # Go 代理 (mTLS) / Go proxy with mTLS

# stop_by_pid_file - 根据 PID 文件停止指定服务
# Stop a service by its PID file
#
# 参数 / Parameters:
#   $1 - PID 文件路径 / PID file path
#   $2 - 服务名称（用于日志输出）/ service name (for log output)
#
# 逻辑 / Logic:
#   1. 检查 PID 文件是否存在，不存在则跳过
#      Check if PID file exists, skip if not
#   2. 读取 PID 并尝试 kill 主进程
#      Read PID and attempt to kill the main process
#   3. pgrep -P 查找并 SIGKILL 所有子进程
#      Use pgrep -P to find and SIGKILL all child processes
#   4. 删除 PID 文件
#      Remove PID file
stop_by_pid_file() {
    local file="$1"   # PID 文件路径 / PID file path
    local name="$2"   # 服务显示名称 / display name for logging
    if [[ -f "$file" ]]; then
        local pid
        pid="$(cat "$file")"  # 读取记录的进程号 / read the recorded process ID
        # 终止主进程及其子进程
        # Kill the recorded process and any children it may have spawned
        # 对 `go run` 启动子二进制的情况很重要
        # This is important for `go run` which starts a child binary
        if kill "$pid" 2>/dev/null; then
            # 强制终止所有子进程，忽略不存在的情况
            # Force-kill all child processes, ignore if none exist
            kill -9 $(pgrep -P "$pid" 2>/dev/null) 2>/dev/null || true
            echo "已停止 $name (PID: $pid)"  # Stopped $name (PID: $pid)
        else
            echo "$name (PID: $pid) 不存在或已停止"  # $name (PID: $pid) not found or already stopped
        fi
        rm -f "$file"  # 清理 PID 文件 / clean up PID file
    else
        echo "未找到 $name 的 PID 文件，可能未启动或已停止"  # PID file for $name not found
    fi
}

# 依次停止普通模式和 mTLS 模式的服务（不存在的 PID 文件会被安全跳过）
# Stop normal mode and mTLS mode services in sequence (missing PID files are safely skipped)
stop_by_pid_file "$AGENT_PID_FILE" "privacy_local_agent"              # 普通模式 agent / normal mode agent
stop_by_pid_file "$CONSOLE_PID_FILE" "Go gRPC 代理后端"              # 普通模式 Go 代理 / normal mode Go proxy
stop_by_pid_file "$AGENT_MTLS_PID_FILE" "privacy_local_agent (mTLS)"  # mTLS 模式 agent / mTLS mode agent
stop_by_pid_file "$CONSOLE_MTLS_PID_FILE" "Go gRPC 代理后端 (mTLS)"  # mTLS 模式 Go 代理 / mTLS mode Go proxy

echo "所有由 start-go.sh 启动的服务已处理完毕。"  # All services started by start-go.sh have been processed.
