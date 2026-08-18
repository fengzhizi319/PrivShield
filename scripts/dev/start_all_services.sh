#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: start_all_services.sh
# 脚本说明: 一键后台启动 PrivShield 核心侧边栏服务 (REST + gRPC)
#           及 Web 测试控制台代理后端服务，并自动检测健康就绪探针。
#
# 执行步骤总览：
#   1. 初始化工作目录与端口配置（REST: 8079, gRPC: 50051, Console: 8080）
#   2. 检查端口占用并执行日志文件轮转备份（保留最近 5 份）
#   3. 使用 nohup 后台拉起 PrivShield Core REST & gRPC Agent 主进程并记录 PID
#   4. 后台拉起 Console Backend 测试控制台 API 代理服务并记录 PID
#   5. 轮询健康探针（GET /health 最长等待 15 秒）直至服务完全就绪
#
# 用法 / Usage:
#   ./scripts/dev/start_all_services.sh
# ==============================================================================

set -euo pipefail

# ANSI 终端颜色代码
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ── 步骤 1：定位项目根目录与端口初始化 ────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"

LOG_DIR="$PROJECT_ROOT/.logs"
mkdir -p "$LOG_DIR"

REST_PORT="${PRIVACY_REST_PORT:-8079}"
GRPC_PORT="${PRIVACY_GRPC_PORT:-50051}"
CONSOLE_PORT="${PRIVACY_CONSOLE_PORT:-8080}"

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} PrivShield 一键拉起服务全家桶${NC}"
echo -e "${BLUE} 工作目录     : ${PROJECT_ROOT}${NC}"
echo -e "${BLUE} 日志输出目录 : ${LOG_DIR}/${NC}"
echo -e "${BLUE} REST 端口    : ${REST_PORT}${NC}"
echo -e "${BLUE} gRPC 端口    : ${GRPC_PORT}${NC}"
echo -e "${BLUE} 控制台端口   : ${CONSOLE_PORT}${NC}"
echo -e "${BLUE}====================================================${NC}"

# 1. 检查端口占用情况
check_port() {
    local port=$1
    if command -v lsof &> /dev/null; then
        if lsof -i:"$port" &> /dev/null; then
            echo -e "${YELLOW}警告: 端口 ${port} 已被占用（本脚本不自动停止已有实例，请先运行 stop_all_services.sh）...${NC}"
        fi
    fi
}

# 日志轮转：启动时归档上一轮日志（保留最近 5 份），避免 .logs/*.log 无限增长。
# 注：仅覆盖"重启截断"场景；长时间运行持续增长的日志请使用系统 logrotate（生产环境）。
rotate_log() {
    local log_file=$1 keep=5 i
    [ -f "$log_file" ] || return 0
    for (( i=keep-1; i>=1; i-- )); do
        [ -f "${log_file}.$i" ] && mv "${log_file}.$i" "${log_file}.$((i+1))"
    done
    mv "$log_file" "${log_file}.1"
}

check_port "$REST_PORT"
check_port "$GRPC_PORT"
check_port "$CONSOLE_PORT"

# 2. 启动核心 REST + gRPC 侧边栏
echo -e "\n${YELLOW}[1/2] 启动 Core REST & gRPC Agent 侧边栏进程...${NC}"
AGENT_LOG="${LOG_DIR}/agent_server.log"
rotate_log "$AGENT_LOG"

nohup python3 -m PrivShield.server < /dev/null > "$AGENT_LOG" 2>&1 &
AGENT_PID=$!
echo $AGENT_PID > "${LOG_DIR}/agent.pid"
echo -e "Agent 进程 PID: ${GREEN}${AGENT_PID}${NC} (日志: ${AGENT_LOG})"

# 3. 启动 Console Backend 代理控制台
echo -e "\n${YELLOW}[2/2] 启动 Console API 代理控制台进程...${NC}"
CONSOLE_LOG="${LOG_DIR}/console_backend.log"
rotate_log "$CONSOLE_LOG"

if [ -f "$PROJECT_ROOT/console/backend/app/main.py" ]; then
    (
        cd "$PROJECT_ROOT/console/backend"
        if [ -f ".venv/bin/activate" ]; then
            source ".venv/bin/activate"
        fi
        nohup python3 -m uvicorn app.main:app --host 127.0.0.1 --port "$CONSOLE_PORT" < /dev/null > "$CONSOLE_LOG" 2>&1 &
        echo $! > "${LOG_DIR}/console.pid"
    )
    echo -e "Console 控制台日志: ${CONSOLE_LOG}"
else
    echo -e "${YELLOW}未检测到 console/backend/app/main.py，跳过控制台启动。${NC}"
fi

# 4. 健康轮询就绪探针 (Health Readiness Probe)
echo -e "\n${YELLOW}正在等待服务探针响应 (最长等待 15 秒)...${NC}"
MAX_RETRIES=15
RETRY_COUNT=0
HEALTH_URL="http://127.0.0.1:${REST_PORT}/health"

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if command -v curl &> /dev/null; then
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${HEALTH_URL}" || echo "000")
        if [ "$HTTP_CODE" -eq 200 ]; then
            echo -e "\n${GREEN}====================================================${NC}"
            echo -e "${GREEN} 所有服务启动成功且健康探针已就绪！${NC}"
            echo -e "${GREEN} REST 服务端点   : http://127.0.0.1:${REST_PORT}${NC}"
            echo -e "${GREEN} gRPC 服务端点   : 127.0.0.1:${GRPC_PORT}${NC}"
            echo -e "${GREEN} 控制台代理端点  : http://127.0.0.1:${CONSOLE_PORT}${NC}"
            echo -e "${GREEN} 停止服务命令    : ./scripts/dev/stop_all_services.sh${NC}"
            echo -e "${GREEN}====================================================${NC}"
            exit 0
        fi
    fi
    echo -n "."
    sleep 1
    RETRY_COUNT=$((RETRY_COUNT + 1))
done

echo -e "\n${RED}[错误] 服务启动超时，未在 15 秒内响应健康检查。${NC}"
echo -e "请检查日志文件获取详细报错: cat ${AGENT_LOG}"
exit 1
