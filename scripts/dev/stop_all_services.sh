#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: stop_all_services.sh
# 脚本说明: 优雅关闭 PrivShield 后台拉起的所有服务实例（Agent、BFF、微服务群）。
#
# 执行步骤总览：
#   1. 读取 .pids/agent.pid 并发送 SIGTERM 终止 Agent 主进程
#   2. 读取 .pids/console.pid 并发送 SIGTERM 终止 Console 代理后端进程
#   3. 读取 .pids/*.pid 终止各微服务进程 (service-hub, datasource-mgr, audit-log)
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

PIDS_DIR="$PROJECT_ROOT/.pids"
LEGACY_PIDS_DIR="$PROJECT_ROOT/console/.pids"

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

# ── 步骤 1：按 PID 文件停止各服务进程（新版 .pids/ + 旧版 console/.pids/）──
for dir in "$PIDS_DIR" "$LEGACY_PIDS_DIR"; do
    if [[ -d "$dir" ]]; then
        stop_by_pid "${dir}/agent.pid" "Agent 侧边栏引擎"
        stop_by_pid "${dir}/console.pid" "Console BFF 代理网关"
        stop_by_pid "${dir}/service-hub.pid" "Service Hub 调度中枢"
        stop_by_pid "${dir}/datasource-mgr.pid" "Datasource Mgr 数据源管理"
        stop_by_pid "${dir}/audit-log.pid" "Audit Log 审计日志"
    fi
done

# ── 步骤 2：按进程名兜底清理 (确保无残留孤儿进程) ─────────────────────────
pkill -f "engine.server" 2>/dev/null || true
pkill -f "engine.main" 2>/dev/null || true
pkill -f "uvicorn app.main:app" 2>/dev/null || true
pkill -f "backend-go" 2>/dev/null || true
pkill -f "bin/service-hub" 2>/dev/null || true
pkill -f "bin/datasource-mgr" 2>/dev/null || true
pkill -f "bin/audit-log" 2>/dev/null || true

# ── 步骤 3：停止可能占用开发端口的 Docker 容器 ─────────────────────────────
if command -v docker >/dev/null 2>&1; then
    for port in 8079 50051 8081 8082 8083 8084 5173; do
        cids=$(docker ps -q --filter "publish=$port" 2>/dev/null || true)
        if [[ -n "$cids" ]]; then
            for cid in $cids; do
                docker stop "$cid" >/dev/null 2>&1 || true
            done
        fi
    done
fi

echo -e "${GREEN}所有相关服务实例已成功停止！${NC}"
