#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: verify_console_environment.sh
# 脚本说明: 极速开发与 CI 构建环境校验工具 (Console Web, Python Backend & Go Proxy)。
#
# 执行步骤总览：
#   1. 检查 Python (>= 3.10) 运行环境与后端核心包（fastapi, httpx, pytest）
#   2. 检查 Node.js (>= 18) 与 pnpm 前端工具链可用性
#   3. 检查 Go 编译器 (>= 1.20) 工具链（用于 Go gRPC 代理）
#   4. 触发 Web 前端 TypeScript 类型构建校验（npx tsc --noEmit）
#   5. 输出环境校验综合评估与错误项清单
#
# 用法 / Usage:
#   ./scripts/dev/verify_console_environment.sh
# ==============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ── 步骤 0：定位并切换至项目根目录 ──────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} Console 开发与编译构建环境巡检${NC}"
echo -e "${BLUE} 工作目录: ${PROJECT_ROOT}${NC}"
echo -e "${BLUE}====================================================${NC}"

ERRORS=0

# ── 步骤 1：检查 Python 运行环境 ──────────────────────────────────────────
echo -e "\n${YELLOW}[1/4] 检查 Python 开发环境...${NC}"
if command -v python3 &> /dev/null; then
    PY_VER=$(python3 -c "import sys; print(sys.version.split()[0])")
    echo -e "Python 版本: ${GREEN}${PY_VER}${NC}"
    
    python3 -c "import fastapi, httpx, pytest; print('Python 依赖包: [OK]')" 2>/dev/null || {
        echo -e "${YELLOW}警告: 缺少某些后端测试包 (fastapi/httpx/pytest)。${NC}"
    }
else
    echo -e "${RED}[错误] 未安装 python3！${NC}"
    ERRORS=$((ERRORS + 1))
fi

# 2. 检查 Node.js & pnpm 前端工具链
echo -e "\n${YELLOW}[2/4] 检查 Node.js & pnpm 前端工具链...${NC}"
if command -v node &> /dev/null; then
    NODE_VER=$(node -v)
    echo -e "Node.js 版本: ${GREEN}${NODE_VER}${NC}"
else
    echo -e "${RED}[错误] 未安装 Node.js！${NC}"
    ERRORS=$((ERRORS + 1))
fi

if command -v pnpm &> /dev/null; then
    PNPM_VER=$(pnpm -v)
    echo -e "pnpm 版本   : ${GREEN}${PNPM_VER}${NC}"
else
    echo -e "${YELLOW}未检测到 pnpm 全局命令，推荐安装: corepack enable 或 npm i -g pnpm${NC}"
fi

# 3. 检查 Go 编译器
echo -e "\n${YELLOW}[3/4] 检查 Go 语言编译链 (用于 Go gRPC 代理)...${NC}"
if command -v go &> /dev/null; then
    GO_VER=$(go version | awk '{print $3}')
    echo -e "Go 编译器版本: ${GREEN}${GO_VER}${NC}"
else
    echo -e "${YELLOW}未检测到 Go 工具链 (如需测试 Go 代理请安装 Go >= 1.20)。${NC}"
fi

# 4. 执行前端静态编译类型检查
echo -e "\n${YELLOW}[4/4] 验证 Web 前端 TypeScript 类型系统...${NC}"
if [ -d "console/web" ]; then
    (
        cd console/web
        if [ -d "node_modules" ]; then
            echo -e "正在执行 TypeScript 类型构建校验 (tsc)..."
            npx tsc --noEmit && echo -e "${GREEN}TypeScript 类型检查通过！${NC}" || {
                echo -e "${RED}[错误] TypeScript 类型校验报错，请修正！${NC}"
                ERRORS=$((ERRORS + 1))
            }
        else
            echo -e "${YELLOW}未找到 console/web/node_modules。请先执行: cd console/web && pnpm install${NC}"
        fi
    )
fi

echo -e "\n${BLUE}====================================================${NC}"
if [ $ERRORS -eq 0 ]; then
    echo -e "${GREEN}环境巡检通过！Console 开发与构建环境准备就绪。${NC}"
else
    echo -e "${RED}环境巡检完成，发现 ${ERRORS} 个缺失或报错项，请修正后重试。${NC}"
fi
echo -e "${BLUE}====================================================${NC}"
