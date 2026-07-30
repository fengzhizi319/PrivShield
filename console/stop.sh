#!/usr/bin/env bash
# ============================================================================
# 停止由 ./console/start.sh 启动的服务
# Stop services launched by ./console/start.sh
#
# 用法 / Usage：./console/stop.sh
#
# 逻辑说明 / Logic:
#   1. 读取 .pids/ 目录下由 start.sh 写入的 PID 文件
#      Read PID files written by start.sh from .pids/ directory
#   2. 对每个 PID 文件调用 stop_by_pid_file 终止对应进程及其子进程
#      Call stop_by_pid_file for each PID file to kill the process and its children
#   3. 删除 PID 文件，输出停止结果
#      Remove PID files and print stop results
# ============================================================================

# 启用严格模式：遇到错误立即退出(-e)、未定义变量报错(-u)、管道中任何命令失败则整体失败(-o pipefail)
# Enable strict mode: exit on error(-e), error on undefined vars(-u), pipe failure propagates(-o pipefail)
set -euo pipefail

# 获取脚本自身所在目录的绝对路径（无论从哪里调用都能正确定位）
# Get the absolute path of the directory containing this script (works regardless of CWD)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# PID 文件存储目录，与 start.sh 保持一致
# PID file storage directory, consistent with start.sh
PID_DIR="$SCRIPT_DIR/.pids"

# start.sh 写入的两个 PID 文件路径
# The two PID file paths written by start.sh
AGENT_PID_FILE="$PID_DIR/agent.pid"          # privacy_local_agent 主服务 / main agent service
CONSOLE_PID_FILE="$PID_DIR/console.pid"      # Python REST 代理后端 / Python REST proxy backend

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
#   3. 使用 pgrep -P 查找并强制终止所有子进程（对 go run 等启动子二进制的情况很重要）
#      Use pgrep -P to find and force-kill all child processes (important for go run which spawns child binaries)
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
        echo "未找到 $name 的 PID 文件，可能未启动或已停止"  # PID file for $name not found, may not have started or already stopped
    fi
}

# 依次停止 start.sh 启动的两个服务
# Stop the two services started by start.sh in sequence
stop_by_pid_file "$AGENT_PID_FILE" "privacy_local_agent"   # 主 agent 服务 / main agent service
stop_by_pid_file "$CONSOLE_PID_FILE" "测试控制台后端"       # Console backend / console backend

echo "所有由 start.sh 启动的服务已处理完毕。"  # All services started by start.sh have been processed.
