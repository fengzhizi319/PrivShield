#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键启动 Go gRPC 控制台 (Vite 热更新)
# Launch Go gRPC proxy console in DEV mode with Vite HMR dev server
#
# 用法 / Usage: ./scripts/dev/dev-start.sh [--force]
#   --force: 非交互模式，端口被占用时自动终止占用进程（CI/脚本化场景）
#
# 启动组件 / Launched Components:
#   1. PrivShield Engine (REST: 8079, gRPC: 50051)
#   2. Go gRPC 代理后端 (API: 8081)
#   3. Vite 前端开发服务器 (UI: 5173, 支持 <50ms HMR 热重载)
# ============================================================================

set -euo pipefail

# ── 解析命令行参数：仅支持 --force ─────────────────────────────────
FORCE=false
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
    esac
done

# ── 解析脚本目录，初始化全局变量 ──────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONSOLE_DIR="$PROJECT_ROOT/console"
PIDS_DIR="$PROJECT_ROOT/.pids"
LOGS_DIR="$PROJECT_ROOT/.logs"

mkdir -p "$PIDS_DIR" "$LOGS_DIR"

AGENT_VENV="$PROJECT_ROOT/.venv"
AGENT_URL="http://127.0.0.1:8079"
CONSOLE_URL="http://127.0.0.1:8081"
VITE_URL="http://localhost:5173"

# ── TCP connect 端口探测 ──────────────────────────────────────────────
_is_port_in_use() {
    local port="$1"
    python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(0.5)
try:
    s.connect(('127.0.0.1', $port))
    s.close()
    sys.exit(0)
except (ConnectionRefusedError, socket.timeout, OSError):
    sys.exit(1)
" 2>/dev/null
}

check_port_available() {
    local port="$1"
    local name="$2"

    if ! _is_port_in_use "$port"; then
        return 0
    fi

    echo ""
    echo "⚠️  [端口占用] $name 目标端口 $port 已被占用"

    if [ "$FORCE" = true ]; then
        echo "   --force 模式：自动查找并终止占用进程..."
        local pids
        pids=$(lsof -ti :"$port" 2>/dev/null || true)
        if [ -n "$pids" ]; then
            echo "$pids" | xargs kill -9 2>/dev/null || true
            sleep 1
            echo "   ✅ 已终止占用端口 $port 的进程: $pids"
        else
            echo "   ❌ 未能自动获取占用端口 $port 的 PID，请手动释放后重试"
            exit 1
        fi
    else
        read -r -p "   是否自动终止占用端口 $port 的进程？[y/N] " answer
        case "$answer" in
            [yY][eE][sS]|[yY])
                local pids
                pids=$(lsof -ti :"$port" 2>/dev/null || true)
                if [ -n "$pids" ]; then
                    echo "$pids" | xargs kill -9 2>/dev/null || true
                    sleep 1
                    echo "   ✅ 已终止进程: $pids"
                else
                    echo "   ❌ 未能获取 PID，请手动释放端口 $port"
                    exit 1
                fi
                ;;
            *)
                echo "   已取消启动。请手动释放端口 $port 后重新运行本脚本。"
                exit 1
                ;;
        esac
    fi
}

# ── 启动前端口检查 ──────────────────────────────────────────────────
check_port_available 8079 "PrivShield Engine REST"
check_port_available 50051 "PrivShield Engine gRPC"
check_port_available 8081 "Go gRPC 代理后端"
check_port_available 5173 "Vite 前端开发服务器"

# ── 工具函数：轮询等待 HTTP 端点就绪 ───────────────────────────────────
wait_for_url() {
    local url="$1"
    local name="$2"
    local max_retries="${3:-30}"
    local count=0

    printf "  ⏳ 等待 %s 就绪 (%s)..." "$name" "$url"
    while [ $count -lt "$max_retries" ]; do
        if curl -sf -o /dev/null --max-time 1 "$url" 2>/dev/null; then
            printf " \033[32m[就绪]\033[0m\n"
            return 0
        fi
        sleep 1
        count=$((count + 1))
        printf "."
    done
    printf " \033[31m[超时]\033[0m\n"
    return 1
}

# ── 启动组件 1：PrivShield Engine (REST: 8079, gRPC: 50051) ──────────
echo "▶ 启动 PrivShield Engine (REST: 8079, gRPC: 50051)..."
if [ -f "$AGENT_VENV/bin/activate" ]; then
    # shellcheck disable=SC1091
    source "$AGENT_VENV/bin/activate"
fi

if [ -f "$PROJECT_ROOT/engine/server.py" ]; then
    (cd "$PROJECT_ROOT" && python3 -m engine.server) \
        > "$LOGS_DIR/agent.log" 2>&1 &
    AGENT_PID=$!
else
    (cd "$PROJECT_ROOT" && python3 -m engine.main) \
        > "$LOGS_DIR/agent.log" 2>&1 &
    AGENT_PID=$!
fi
echo "$AGENT_PID" > "$PIDS_DIR/agent.pid"
echo "  PID: $AGENT_PID | 日志: $LOGS_DIR/agent.log"

if ! wait_for_url "$AGENT_URL/health" "PrivShield Engine"; then
    echo "❌ PrivShield Engine 启动失败，请检查日志: $LOGS_DIR/agent.log"
    exit 1
fi

# ── 启动组件 2：Go gRPC 代理后端 (API: 8081) ───────────────────────
echo "▶ 启动 Go gRPC 代理后端 (API: 8081)..."
BACKEND_GO_DIR="$CONSOLE_DIR/bff-go"
mkdir -p "$BACKEND_GO_DIR/bin"

if ! command -v go &>/dev/null; then
    echo "❌ 未找到 go 命令，请先安装 Go 1.22+ 编译环境"
    exit 1
fi

echo "  正在编译 Go gRPC 代理后端..."
(cd "$BACKEND_GO_DIR" && go build -o bin/backend-go ./cmd/server)
if [ ! -f "$BACKEND_GO_DIR/bin/backend-go" ]; then
    echo "❌ Go 代理后端编译失败！"
    exit 1
fi

(
    cd "$BACKEND_GO_DIR"
    export PRIVACY_AGENT_GRPC_HOST=127.0.0.1
    export PRIVACY_AGENT_GRPC_PORT=50051
    export PRIVACY_CONSOLE_HOST=127.0.0.1
    export PRIVACY_CONSOLE_PORT=8081
    export PRIVACY_CONSOLE_STATIC_DIR=""
    ./bin/backend-go
) > "$LOGS_DIR/backend-go.log" 2>&1 &
BACKEND_GO_PID=$!
echo "$BACKEND_GO_PID" > "$PIDS_DIR/backend-go.pid"
echo "  PID: $BACKEND_GO_PID | 日志: $LOGS_DIR/backend-go.log"

if ! wait_for_url "$CONSOLE_URL/api/health" "Go gRPC 代理后端"; then
    echo "❌ Go gRPC 代理后端启动失败，请检查日志: $LOGS_DIR/backend-go.log"
    exit 1
fi

# ── 启动组件 3：Vite 前端开发服务器 (UI: 5173, HMR) ─────────────────
echo "▶ 启动 Vite 前端开发服务器 (UI: 5173, 支持 <50ms 热更新)..."
WEB_DIR="$CONSOLE_DIR/web"
if [ ! -d "$WEB_DIR/node_modules" ]; then
    echo "  首次运行，正在安装前端依赖 (corepack pnpm install)..."
    (cd "$WEB_DIR" && corepack pnpm install)
fi

(
    cd "$WEB_DIR"
    export VITE_PROXY_TARGET="http://127.0.0.1:8081"
    corepack pnpm dev
) > "$LOGS_DIR/vite.log" 2>&1 &
VITE_PID=$!
echo "$VITE_PID" > "$PIDS_DIR/vite.pid"
echo "  PID: $VITE_PID | 日志: $LOGS_DIR/vite.log"

if ! wait_for_url "$VITE_URL" "Vite 前端开发服务器"; then
    echo "❌ Vite 前端服务器启动失败，请检查日志: $LOGS_DIR/vite.log"
    exit 1
fi

# ── 启动完成提示 ────────────────────────────────────────────────────
echo ""
echo "================================================================="
echo "🎉 PrivShield 控制台 (Go gRPC 代理 + Vite HMR) 启动成功！"
echo ""
echo "  🌐 前端界面 (UI):    $VITE_URL"
echo "  🔌 代理后端 (API):   $CONSOLE_URL"
echo "  🛡️  隐私引擎 (Agent): $AGENT_URL (REST) / 127.0.0.1:50051 (gRPC)"
echo ""
echo "  📁 日志文件:"
echo "     - Engine:     $LOGS_DIR/agent.log"
echo "     - Backend-Go: $LOGS_DIR/backend-go.log"
echo "     - Vite:       $LOGS_DIR/vite.log"
echo ""
echo "  🛑 停止服务: ./scripts/dev/dev-stop.sh"
echo "================================================================="
