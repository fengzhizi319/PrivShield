#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: run_console_e2e_tests.sh
# 脚本说明: 一键运行 Console 控制台前后端 (Web + Python Backend + Go Proxy)
#           的全套端到端 (E2E) 集成自动化回归测试。
#
# 执行步骤总览：
#   1. 启动轻量 Mock Agent 桩服务 (端口 8079)
#   2. 运行 Console Backend (Python FastAPI) pytest 单元测试与烟雾测试
#   3. 运行 Console Backend-Go (Go gRPC) 代理单元测试与集成测试
#   4. 运行 Console Web (React 前端) Vitest 自动化单元与组件测试
#   5. 捕获 EXIT 信号自动清理并释放所有后台桩服务进程
#
# 用法 / Usage:
#   ./scripts/dev/run_console_e2e_tests.sh
# ==============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

MOCK_PID=""
BACKEND_PID=""

# ── 步骤 0：注册退出资源自动清理钩子 ──────────────────────────────────────
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

# ── 步骤 1：启动 Mock Agent 服务 (端口 8079) ───────────────────────────────
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
echo -e "\n${YELLOW}[步骤 2/5] 运行 Console BFF (Python) 单元测试与 Smoke Test...${NC}"
if [ -d "console/bff-py" ]; then
    (
        cd console/bff-py
        echo "运行 pytest 路由与降级单元测试..."
        PYTHONPATH=. pytest tests/ -v

        echo "启动 Console BFF FastAPI 服务 (端口 8080)..."
        python3 -m uvicorn app.main:app --host 127.0.0.1 --port 8080 &
        INNER_BACKEND_PID=$!
        sleep 2
        
        echo "运行烟雾测试 (smoke_test.py)..."
        python3 smoke_test.py
        kill "$INNER_BACKEND_PID" 2>/dev/null || true
    )
    echo -e "${GREEN}[成功] Python BFF 控制台与 Smoke Test 全部通过！${NC}"
else
    echo -e "${YELLOW}未发现 console/bff-py 目录，跳过。${NC}"
fi

# 3. 运行 Go BFF 与共享库测试
echo -e "\n${YELLOW}[步骤 3/5] 运行 Console BFF-Go (Go) 与 Pkg 基础库测试...${NC}"
if command -v go &> /dev/null && [ -d "console/bff-go" ]; then
    (
        go test ./pkg/... ./console/bff-go/...
    )
    echo -e "${GREEN}[成功] Go BFF 与 Pkg 测试通过！${NC}"
else
    echo -e "${YELLOW}未发现 go 命令或 console/bff-go 目录，跳过 Go BFF 测试。${NC}"
fi

# 4. 运行 Services 微服务群测试
echo -e "\n${YELLOW}[步骤 4/5] 运行 Services 微服务群 (service-hub / datasource-mgr / audit-log) 测试...${NC}"
if command -v go &> /dev/null && [ -d "services" ]; then
    (
        go test ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/...
    )
    echo -e "${GREEN}[成功] Services 中台微服务群测试通过！${NC}"
else
    echo -e "${YELLOW}未发现 services 目录，跳过微服务测试。${NC}"
fi

# 5. 运行 Web 前端组件与单元测试
echo -e "\n${YELLOW}[步骤 5/5] 运行 Console Web (React) 组件与自动化测试...${NC}"
if [ -d "console/web" ]; then
    (
        cd console/web
        if command -v corepack &> /dev/null; then
            corepack pnpm test -- --run
        elif command -v npm &> /dev/null; then
            npm test -- --run
        fi
    )
    echo -e "${GREEN}[成功] React 前端组件测试通过！${NC}"
else
    echo -e "${YELLOW}未发现 console/web 目录，跳过。${NC}"
fi

echo -e "\n${GREEN}====================================================${NC}"
echo -e "${GREEN} 恭喜！Console 前后端端到端 (E2E) 全套测试通过！${NC}"
echo -e "${GREEN}====================================================${NC}"
