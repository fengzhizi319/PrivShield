#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: stop_all_services.sh
# 脚本说明: 优雅关闭 PrivShield 后台拉起的所有服务实例（Agent、BFF、微服务群）。
#
# 执行步骤总览：
#   1. 读取 .logs/agent.pid 并发送 SIGTERM 终止 Agent 主进程
#   2. 读取 .logs/console.pid 并发送 SIGTERM 终止 Console 代理后端进程
#   3. 读取 .logs/*.pid 终止各微服务进程 (service-hub, datasource-mgr, audit-log)
#   4. 通过 pkill 按进程全名进行幂等兜底清理，确保无残留后台孤儿进程
#
# 用法 / Usage:
#   ./scripts/dev/stop_all_services.sh
# ==============================================================================

set -euo pipefail

# ANSI 终端颜色代码
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"

LOG_DIR="$PROJECT_ROOT/.logs"

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} 正在优雅停止 PrivShield 全栈服务集群...${NC}"
echo -e "${BLUE}====================================================${NC}"

stop_by_pid() {
    local pid_file="$1"
    local name="$2"
    if [ -f "$pid_file" ]; then
        local pid
        pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            echo -e "正在终止 ${name} (PID: ${pid})..."
            kill "$pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi
}

# ── 步骤 1：按 PID 文件停止各服务进程 ─────────────────────────────────────
stop_by_pid "${LOG_DIR}/agent.pid" "Agent 侧边栏引擎"
stop_by_pid "${LOG_DIR}/console.pid" "Console BFF 代理网关"
stop_by_pid "${LOG_DIR}/service-hub.pid" "Service Hub 调度中枢"
stop_by_pid "${LOG_DIR}/datasource-mgr.pid" "Datasource Mgr 数据源管理"
stop_by_pid "${LOG_DIR}/audit-log.pid" "Audit Log 审计日志"

# ── 步骤 2：按进程名兜底清理 (确保无残留孤儿进程) ─────────────────────────
pkill -f "engine.server" 2>/dev/null || true
pkill -f "engine.main" 2>/dev/null || true
pkill -f "uvicorn app.main:app" 2>/dev/null || true
pkill -f "backend-go" 2>/dev/null || true
pkill -f "bin/service-hub" 2>/dev/null || true
pkill -f "bin/datasource-mgr" 2>/dev/null || true
pkill -f "bin/audit-log" 2>/dev/null || true

echo -e "${GREEN}所有相关服务实例已成功停止！${NC}"
