#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键启动全套服务 (Agent + Go BFF + Vite 前端)
# Launch all services in DEV mode with Vite HMR
#
# 用法 / Usage: ./scripts/dev/dev-start-all.sh [--force]
#   --force: 非交互模式，端口被占用时自动终止占用进程（CI/脚本化场景）
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
GO_CONSOLE_URL="http://127.0.0.1:8081"
VITE_URL="http://localhost:5173"

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
    echo "⚠️  端口 $port 已被占用（$name）"
    echo "────────────────────────────────────────"

    local pids=""
    if command -v lsof >/dev/null 2>&1; then
        lsof -i :"$port" 2>/dev/null || true
        pids=$(lsof -t -i :"$port" 2>/dev/null | sort -u | tr '\n' ' ')
    elif command -v ss >/dev/null 2>&1; then
        ss -tlnp 2>/dev/null | grep -E "LISTEN.*:$port\\s" || true
        pids=$(ss -tlnp 2>/dev/null | grep -E "LISTEN.*:$port\\s" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | sort -u | tr '\n' ' ')
    elif command -v fuser >/dev/null 2>&1; then
        pids=$(fuser "$port"/tcp 2>/dev/null | tr -s ' ')
    fi

    if [[ -z "$pids" ]]; then
        echo "错误：无法定位占用端口 $port 的进程，请手动排查。"
        exit 1
    fi

    if [[ "$FORCE" == "true" ]]; then
        echo "（--force 非交互模式：自动终止占用端口 $port 的进程）"
        answer="y"
    elif [[ ! -t 0 ]]; then
        echo "错误：端口 $port 被占用且当前为非交互环境（无 TTY）。请手动释放端口，或使用 --force 自动处理。"
        exit 1
    else
        read -rp "是否自动终止上述进程以释放端口？[y/N] " answer
    fi
    case "$answer" in
        [yY]|[yY][eE][sS])
            for pid in $pids; do
                kill -9 "$pid" 2>/dev/null || true
            done
            sleep 1
            if ! _is_port_in_use "$port"; then
                echo "✅ 端口 $port 已释放"
            else
                echo "错误：端口 $port 仍被占用，请手动排查。"
                exit 1
            fi
            ;;
        *)
            echo "已取消。请手动释放端口 $port 后重试。"
            exit 1
            ;;
    esac
}

# 1. Agent 虚拟环境
if [[ ! -d "$AGENT_VENV" ]]; then
    echo "未找到 agent 虚拟环境，自动创建并安装依赖：$AGENT_VENV"
    python3 -m venv "$AGENT_VENV"
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        pip install --upgrade pip >/dev/null
        pip install -e .
    )
fi

# 2. Go 工具链检查
if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"
    exit 1
fi

# 3. 确保前端 node_modules 存在
if [[ ! -d "$CONSOLE_DIR/web/node_modules" ]]; then
    echo "未找到前端 node_modules，自动安装依赖..."
    (
        cd "$CONSOLE_DIR/web"
        if command -v corepack >/dev/null 2>&1; then
            corepack pnpm install
        elif command -v pnpm >/dev/null 2>&1; then
            pnpm install
        elif command -v npm >/dev/null 2>&1; then
            npm install
        fi
    )
fi

# 4. 编译 Go 后端
echo "编译 Go gRPC 代理后端..."
(cd "$CONSOLE_DIR/bff-go" && go build -o bin/backend-go ./cmd/server)

AGENT_PID_FILE="$PIDS_DIR/agent-all.pid"
GO_CONSOLE_PID_FILE="$PIDS_DIR/console-go-all.pid"
VITE_PID_FILE="$PIDS_DIR/vite-dev.pid"

write_pid() {
    echo "$2" > "$1"
}

PIDS=()
STOPPING=false
cleanup() {
    STOPPING=true
    echo ""
    echo "正在停止【开发模式】全量服务..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -f "$AGENT_PID_FILE" "$GO_CONSOLE_PID_FILE" "$VITE_PID_FILE"
    echo "已停止。"
}
trap cleanup INT TERM EXIT

check_port_available 8079 "PrivShield REST"
check_port_available 50051 "PrivShield gRPC"
check_port_available 8081 "Go gRPC 代理后端"
check_port_available 5173 "Vite 前端开发服务器"

launch_agent() {
    local agent_log="$LOGS_DIR/agent_all.log"
    echo "启动 PrivShield (REST: $AGENT_URL, gRPC: 127.0.0.1:50051)，日志: $agent_log..."
    (
        if [ -f "$AGENT_VENV/bin/activate" ]; then
            source "$AGENT_VENV/bin/activate"
        fi
        cd "$PROJECT_ROOT"
        exec python -m engine.server >> "$agent_log" 2>&1
    ) &
    AGENT_PID=$!
    PIDS[0]="$AGENT_PID"
    write_pid "$AGENT_PID_FILE" "$AGENT_PID"
}
launch_agent

wait_for_service() {
    local url="$1"
    local name="$2"
    local max_attempts=30
    local attempt=0
    echo -n "等待 $name 就绪"
    while [[ $attempt -lt $max_attempts ]]; do
        if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q '^200$'; then
            echo " OK"
            return 0
        fi
        echo -n "."
        sleep 1
        attempt=$((attempt + 1))
    done
    echo " 超时"
    return 1
}

wait_for_service "$AGENT_URL/health" "PrivShield"

echo -n "等待 agent gRPC (127.0.0.1:50051) 就绪"
for i in $(seq 1 30); do
    if _is_port_in_use 50051; then
        echo " OK"
        break
    fi
    echo -n "."
    sleep 1
    if [[ $i -eq 30 ]]; then
        echo " 超时"
        exit 1
    fi
done

echo "启动 Go gRPC 代理后端 (API: $GO_CONSOLE_URL)..."
(
    cd "$CONSOLE_DIR/bff-go"
    exec ./bin/backend-go
) > "$LOGS_DIR/backend-go-all.log" 2>&1 &
GO_CONSOLE_PID=$!
PIDS+=("$GO_CONSOLE_PID")
write_pid "$GO_CONSOLE_PID_FILE" "$GO_CONSOLE_PID"

wait_for_service "$GO_CONSOLE_URL/api/health" "Go gRPC 代理后端"

echo "启动 Vite 前端开发服务器 (UI: $VITE_URL)..."
(
    cd "$CONSOLE_DIR/web"
    export VITE_PROXY_TARGET="$GO_CONSOLE_URL"
    if command -v corepack >/dev/null 2>&1; then
        corepack pnpm dev
    elif command -v pnpm >/dev/null 2>&1; then
        pnpm dev
    else
        npm run dev
    fi
) > "$LOGS_DIR/vite-all.log" 2>&1 &
VITE_PID=$!
PIDS+=("$VITE_PID")
write_pid "$VITE_PID_FILE" "$VITE_PID"

wait_for_service "$VITE_URL" "Vite 前端"

echo ""
echo "================================================================="
echo "🎉 PrivShield 全量服务 (Agent + Go BFF + Vite UI) 已全部启动！"
echo "  🌐 前端界面 (UI):    $VITE_URL"
echo "  🔌 代理后端 (Go):    $GO_CONSOLE_URL"
echo "  🛡️  隐私引擎 (Agent): $AGENT_URL (REST) / 127.0.0.1:50051 (gRPC)"
echo "  🛑 停止所有服务: 按 Ctrl+C 或运行 ./scripts/dev/dev-stop.sh"
echo "================================================================="

wait
