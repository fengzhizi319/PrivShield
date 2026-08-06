#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: run_console_e2e_tests.sh
# 脚本说明: 一键运行 Console 控制台前后端 (Web + Python Backend + Go Proxy)
#           的全套端到端 (E2E) 集成自动化回归测试。
#
# 测试链路:
#   1. 自动开启 Mock Agent 服务 (端口 8079)
#   2. 启动 Console Backend (端口 8080)，运行 console/backend Python 代理后端单元测试与 smoke_test
#   3. 运行 console/backend-go Go 代理后端单元测试与集成测试
#   4. 运行 console/web React 前端组件 Vitest 自动化测试
#   5. 退出时清理所有后台进程并输出测试套件报告
# ==============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

MOCK_PID=""
BACKEND_PID=""

cleanup() {
    echo -e "\n${YELLOW}[清理] 正在释放测试模拟服务与临时资源...${NC}"
    if [ -n "$BACKEND_PID" ]; then
        kill "$BACKEND_PID" 2>/dev/null || true
    fi
    if [ -n "$MOCK_PID" ]; then
        kill "$MOCK_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} Console 端到端 (E2E) 全套自动化测试套件${NC}"
echo -e "${BLUE}====================================================${NC}"

# 1. 启动 Mock Agent 服务 (端口 8079)
echo -e "\n${YELLOW}[步骤 1/4] 启动 Mock Agent 桩服务 (端口 8079)...${NC}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "$SCRIPT_DIR/mock_agent_server.py" 8079 &
MOCK_PID=$!
sleep 1
if ! kill -0 "$MOCK_PID" 2>/dev/null; then
    echo -e "${RED}[错误] Mock Agent 启动失败！${NC}"
    exit 1
fi
echo -e "${GREEN}Mock Agent 已启动 (PID: ${MOCK_PID})${NC}"

# 2. 启动 Console Python 后端 (端口 8080) 并运行单元测试与烟雾测试
echo -e "\n${YELLOW}[步骤 2/4] 运行 Console Backend (Python) 单元测试与 Smoke Test...${NC}"
if [ -d "console/backend" ]; then
    (
        cd console/backend
        echo "运行 pytest 路由与降级单元测试..."
        PYTHONPATH=. pytest tests/ -v

        echo "启动 Console Backend FastAPI 服务 (端口 8080)..."
        python3 -m uvicorn app.main:app --host 127.0.0.1 --port 8080 &
        INNER_BACKEND_PID=$!
        sleep 2
        
        echo "运行烟雾测试 (smoke_test.py)..."
        python3 smoke_test.py
        kill "$INNER_BACKEND_PID" 2>/dev/null || true
    )
    echo -e "${GREEN}[成功] Python 控制台后端与 Smoke Test 全部通过！${NC}"
else
    echo -e "${YELLOW}未发现 console/backend 目录，跳过。${NC}"
fi

# 3. 运行 Go 代理后端测试
echo -e "\n${YELLOW}[步骤 3/4] 运行 Console Backend-Go (Go) 测试...${NC}"
if command -v go &> /dev/null && [ -d "console/backend-go" ]; then
    (
        cd console/backend-go
        go test -short ./...
        go test ./tests -v
    )
    echo -e "${GREEN}[成功] Go 代理后端测试通过！${NC}"
else
    echo -e "${YELLOW}未发现 go 命令或 console/backend-go 目录，跳过 Go 测试。${NC}"
fi

# 4. 运行 Web 前端组件与单元测试
echo -e "\n${YELLOW}[步骤 4/4] 运行 Console Web (React) 组件与自动化测试...${NC}"
if [ -d "console/web" ]; then
    (
        cd console/web
        corepack pnpm test -- --run
    )
    echo -e "${GREEN}[成功] React 前端组件测试通过！${NC}"
else
    echo -e "${YELLOW}未发现 console/web 目录，跳过。${NC}"
fi

echo -e "\n${GREEN}====================================================${NC}"
echo -e "${GREEN} 恭喜！Console 前后端端到端 (E2E) 全套测试通过！${NC}"
echo -e "${GREEN}====================================================${NC}"
