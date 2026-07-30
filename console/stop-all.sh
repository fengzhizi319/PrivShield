#!/usr/bin/env bash
# ============================================================================
# 停止由 ./console/start-all.sh 启动的服务
# Stop services launched by ./console/start-all.sh
#
# 用法 / Usage：./console/stop-all.sh
#
# 逻辑说明 / Logic:
#   1. 读取 .pids/ 目录下由 start-all.sh 写入的三个 PID 文件
#      Read the three PID files written by start-all.sh from .pids/ directory
#   2. 分别停止 agent、Python REST 后端、Go gRPC 后端及其子进程
#      Stop agent, Python REST backend, Go gRPC backend and their child processes
#   3. 删除 PID 文件，输出停止结果
#      Remove PID files and print stop results
# ============================================================================

# 启用严格模式：遇到错误立即退出(-e)、未定义变量报错(-u)、管道失败传播(-o pipefail)
# Enable strict mode: exit on error(-e), error on undefined vars(-u), pipe failure propagates(-o pipefail)
set -euo pipefail

# 获取脚本自身所在目录的绝对路径
# Get the absolute path of the directory containing this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# PID 文件存储目录，与 start-all.sh 保持一致
# PID file storage directory, consistent with start-all.sh
PID_DIR="$SCRIPT_DIR/.pids"

# start-all.sh 写入的三个 PID 文件路径
# The three PID file paths written by start-all.sh
AGENT_PID_FILE="$PID_DIR/agent-all.pid"          # privacy_local_agent 主服务 / main agent service
PY_CONSOLE_PID_FILE="$PID_DIR/console-all.pid"   # Python REST 代理后端 / Python REST proxy backend
GO_CONSOLE_PID_FILE="$PID_DIR/console-go-all.pid" # Go gRPC 代理后端 / Go gRPC proxy backend

# stop_by_pid_file - 根据 PID 文件停止指定服务
# Stop a service by its PID file
#
# 参数 / Parameters:
#   $1 - PID 文件路径 / PID file path
#   $2 - 服务名称（用于日志输出）/ service name (for log output)
#
# 逻辑 / Logic:
#   1. 检查 PID 文件是否存在
#      Check if the PID file exists
#   2. 读取 PID 并尝试 kill 主进程
#      Read PID and attempt to kill the main process
#   3. 使用 pgrep -P 查找并强制终止所有子进程
#      Use pgrep -P to find and force-kill all child processes
#   4. 无论成功与否都删除 PID 文件
#      Remove PID file regardless of success
stop_by_pid_file() {
    local file="$1"   # PID 文件路径 / PID file path
    local name="$2"   # 服务显示名称 / display name for logging
    if [[ -f "$file" ]]; then
        local pid
        pid="$(cat "$file")"  # 读取记录的进程号 / read the recorded process ID
        # 终止记录的主进程及其可能产生的子进程
        # Kill the recorded process and any children it may have spawned
        # 这对 `go run` 等会启动子二进制的场景很重要
        # This is important for `go run` which starts a child binary
        if kill "$pid" 2>/dev/null; then
            # 强制终止所有子进程（-9 = SIGKILL），忽略不存在的情况
            # Force-kill all child processes (-9 = SIGKILL), ignore if none exist
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

# 依次停止 start-all.sh 启动的三个服务
# Stop the three services started by start-all.sh in sequence
stop_by_pid_file "$AGENT_PID_FILE" "privacy_local_agent"     # 主 agent 服务 / main agent
stop_by_pid_file "$PY_CONSOLE_PID_FILE" "Python REST 代理后端"  # Python REST proxy backend
stop_by_pid_file "$GO_CONSOLE_PID_FILE" "Go gRPC 代理后端"    # Go gRPC proxy backend

echo "所有由 start-all.sh 启动的服务已处理完毕。"  # All services started by start-all.sh have been processed.
