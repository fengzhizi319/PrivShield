#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: stop_all_services.sh
# 脚本说明: 优雅关闭 PrivShield 后台拉起的所有服务实例。
#
# 执行步骤总览：
#   1. 读取 .logs/agent.pid 并发送 SIGTERM 终止 Agent 主进程
#   2. 读取 .logs/console.pid 并发送 SIGTERM 终止 Console 代理后端进程
#   3. 通过 pkill 按进程全名进行幂等兜底清理，确保无残留后台孤儿进程
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
echo -e "${BLUE} 正在优雅停止 PrivShield 侧边栏服务...${NC}"
echo -e "${BLUE}====================================================${NC}"

# ── 步骤 1：停止 Agent 侧边栏主进程 ───────────────────────────────────────
if [ -f "${LOG_DIR}/agent.pid" ]; then
    PID=$(cat "${LOG_DIR}/agent.pid")
    echo -e "正在终止 Agent 进程 (PID: ${PID})..."
    kill "$PID" 2>/dev/null || true
    rm -f "${LOG_DIR}/agent.pid"
fi

# ── 步骤 2：停止 Console 代理控制台进程 ───────────────────────────────────
if [ -f "${LOG_DIR}/console.pid" ]; then
    PID=$(cat "${LOG_DIR}/console.pid")
    echo -e "正在终止 Console 控制台进程 (PID: ${PID})..."
    kill "$PID" 2>/dev/null || true
    rm -f "${LOG_DIR}/console.pid"
fi

# ── 步骤 3：按进程全名兜底清理 (确保无残留孤儿进程) ───────────────────────
pkill -f "PrivShield.server" 2>/dev/null || true
pkill -f "PrivShield.main" 2>/dev/null || true
pkill -f "uvicorn app.main:app" 2>/dev/null || true

echo -e "${GREEN}所有相关服务实例已成功停止！${NC}"
