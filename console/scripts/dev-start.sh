#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键启动 Python REST 控制台 (Vite 热更新)
# Launch Python REST proxy console in DEV mode with Vite HMR dev server
#
# 用法 / Usage: ./console/scripts/dev-start.sh
#
# 启动组件 / Launched Components:
#   1. privacy_local_agent (REST: 8079)
#   2. Python REST 代理后端 (API: 8080)
#   3. Vite 前端开发服务器 (UI: 5173, 支持 <50ms HMR 热重载)
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONSOLE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

AGENT_VENV="$PROJECT_ROOT/.venv"
BACKEND_VENV="$CONSOLE_DIR/backend/.venv"

AGENT_URL="http://127.0.0.1:8079"
CONSOLE_URL="http://127.0.0.1:8080"
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

    read -rp "是否自动终止上述进程以释放端口？[y/N] " answer
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

# 2. 控制台后端虚拟环境
if [[ ! -d "$BACKEND_VENV" ]]; then
    echo "未找到后端虚拟环境，自动创建并安装依赖：$BACKEND_VENV"
    python3 -m venv "$BACKEND_VENV"
    (
        source "$BACKEND_VENV/bin/activate"
        pip install --upgrade pip >/dev/null
        pip install -r "$CONSOLE_DIR/backend/requirements.txt"
    )
fi

# 3. 确保前端 node_modules 存在
if [[ ! -d "$CONSOLE_DIR/web/node_modules" ]]; then
    echo "未找到前端 node_modules，自动安装依赖..."
    (
        cd "$CONSOLE_DIR/web"
        if command -v pnpm >/dev/null 2>&1; then
            pnpm install
        elif command -v npm >/dev/null 2>&1; then
            npm install
        fi
    )
fi

AGENT_PID_FILE="$CONSOLE_DIR/.pids/agent.pid"
CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console.pid"
VITE_PID_FILE="$CONSOLE_DIR/.pids/vite-dev.pid"
mkdir -p "$CONSOLE_DIR/.pids"

write_pid() {
    echo "$2" > "$1"
}

PIDS=()
cleanup() {
    echo ""
    echo "正在停止【开发模式】所有服务..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -f "$AGENT_PID_FILE" "$CONSOLE_PID_FILE" "$VITE_PID_FILE"
    echo "已停止。"
}
trap cleanup INT TERM EXIT

check_port_available 8079 "privacy_local_agent REST"
check_port_available 8080 "Python REST 代理后端"
check_port_available 5173 "Vite 前端开发服务器"

# 启动 privacy_local_agent
echo "启动 privacy_local_agent (REST: $AGENT_URL)..."
(
    source "$AGENT_VENV/bin/activate"
    cd "$PROJECT_ROOT"
    exec python -m privacy_local_agent.server
) &
AGENT_PID=$!
PIDS+=("$AGENT_PID")
write_pid "$AGENT_PID_FILE" "$AGENT_PID"

# 启动控制台后端
echo "启动测试控制台后端 (Console: $CONSOLE_URL)..."
(
    source "$BACKEND_VENV/bin/activate"
    cd "$CONSOLE_DIR/backend"
    exec uvicorn app.main:app --host 127.0.0.1 --port 8080
) &
CONSOLE_PID=$!
PIDS+=("$CONSOLE_PID")
write_pid "$CONSOLE_PID_FILE" "$CONSOLE_PID"

# 启动 Vite 开发服务器
echo "启动 Vite 前端开发服务器 (HMR 模式)..."
(
    cd "$CONSOLE_DIR/web"
    if command -v pnpm >/dev/null 2>&1; then
        exec pnpm dev
    else
        exec npm run dev
    fi
) &
VITE_PID=$!
PIDS+=("$VITE_PID")
write_pid "$VITE_PID_FILE" "$VITE_PID"

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

wait_for_service "$AGENT_URL/health" "privacy_local_agent"
wait_for_service "$CONSOLE_URL/api/health" "测试控制台后端"

echo -n "等待 Vite 开发服务器就绪"
for i in $(seq 1 30); do
    if _is_port_in_use 5173; then
        echo " OK"
        break
    fi
    echo -n "."
    sleep 1
done

echo ""
echo "======================================================================"
echo "🚀【开发模式】 Python REST 代理控制台已成功启动！"
echo "======================================================================"
echo "  前端 UI (Vite HMR):  $VITE_URL  <-- 请访问此地址获得毫秒级热更新！"
echo "  Python 代理后端 (API): $CONSOLE_URL"
echo "  Agent REST 接口:     $AGENT_URL"
echo "──────────────────────────────────────────────────────────────────────"
echo "  按 Ctrl+C 停止所有开发服务"
echo "======================================================================"

wait
