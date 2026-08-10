#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键启动「双后端」隐私测试控制台 (Vite 热更新)
# Launch dual-backend privacy test console in DEV mode with Vite HMR
#
# 用法 / Usage: ./console/scripts/dev-start-all.sh [--force]
#   --force: 非交互模式，端口被占用时自动终止占用进程（CI/脚本化场景）
# ============================================================================

set -euo pipefail

# --force：非交互模式（CI/无 TTY 环境），端口冲突时自动终止占用进程
FORCE=false
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONSOLE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

AGENT_VENV="$PROJECT_ROOT/.venv"
BACKEND_VENV="$CONSOLE_DIR/backend/.venv"

AGENT_URL="http://127.0.0.1:8079"
PY_CONSOLE_URL="http://127.0.0.1:8080"
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

# 2. Python 后端虚拟环境
if [[ ! -d "$BACKEND_VENV" ]]; then
    echo "未找到 Python 后端虚拟环境，自动创建并安装依赖：$BACKEND_VENV"
    python3 -m venv "$BACKEND_VENV"
    (
        source "$BACKEND_VENV/bin/activate"
        pip install --upgrade pip >/dev/null
        pip install -r "$CONSOLE_DIR/backend/requirements.txt"
    )
fi

# 3. Go 工具链检查
if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"
    exit 1
fi

# 4. 确保前端 node_modules 存在
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

# 5. 编译 Go 后端
echo "编译 Go gRPC 代理后端..."
(cd "$CONSOLE_DIR/backend-go" && go build -o bin/backend-go ./cmd/server)

AGENT_PID_FILE="$CONSOLE_DIR/.pids/agent-all.pid"
PY_CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console-all.pid"
GO_CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console-go-all.pid"
VITE_PID_FILE="$CONSOLE_DIR/.pids/vite-dev.pid"
mkdir -p "$CONSOLE_DIR/.pids"

write_pid() {
    echo "$2" > "$1"
}

PIDS=()
STOPPING=false
cleanup() {
    STOPPING=true
    echo ""
    echo "正在停止【开发模式】双后端全量服务..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -f "$AGENT_PID_FILE" "$PY_CONSOLE_PID_FILE" "$GO_CONSOLE_PID_FILE" "$VITE_PID_FILE"
    echo "已停止。"
}
trap cleanup INT TERM EXIT

check_port_available 8079 "privacy_local_agent REST"
check_port_available 50051 "privacy_local_agent gRPC"
check_port_available 8080 "Python REST 代理后端"
check_port_available 8081 "Go gRPC 代理后端"
check_port_available 5173 "Vite 前端开发服务器"

launch_agent() {
    local agent_log="$PROJECT_ROOT/.logs/agent_all.log"
    mkdir -p "$PROJECT_ROOT/.logs"
    echo "启动 privacy_local_agent (REST: $AGENT_URL, gRPC: 127.0.0.1:50051)，日志: $agent_log..."
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        # 日志持久化到 .logs/agent_all.log，agent 崩溃/重启后可回溯根因
        exec python -m privacy_local_agent.server >> "$agent_log" 2>&1
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

wait_for_service "$AGENT_URL/health" "privacy_local_agent"

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

echo "启动 Python REST 代理后端 (API: $PY_CONSOLE_URL)..."
(
    source "$BACKEND_VENV/bin/activate"
    cd "$CONSOLE_DIR/backend"
    exec uvicorn app.main:app --host 127.0.0.1 --port 8080
) &
PY_CONSOLE_PID=$!
PIDS+=("$PY_CONSOLE_PID")
write_pid "$PY_CONSOLE_PID_FILE" "$PY_CONSOLE_PID"

echo "启动 Go gRPC 代理后端 (API: $GO_CONSOLE_URL)..."
(
    cd "$CONSOLE_DIR/backend-go"
    exec ./bin/backend-go
) &
GO_CONSOLE_PID=$!
PIDS+=("$GO_CONSOLE_PID")
write_pid "$GO_CONSOLE_PID_FILE" "$GO_CONSOLE_PID"

wait_for_service "$PY_CONSOLE_URL/api/health" "Python REST 代理后端"
wait_for_service "$GO_CONSOLE_URL/api/health" "Go gRPC 代理后端"

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
echo "🚀【开发模式】 双后端隐私测试控制台已成功启动！"
echo "======================================================================"
echo "  前端 UI (Vite HMR):  $VITE_URL  <-- 页面顶部 Selector 可切换两后端"
echo "  Python REST 后端:    $PY_CONSOLE_URL"
echo "  Go gRPC 后端:        $GO_CONSOLE_URL"
echo "  Agent REST / gRPC:   $AGENT_URL / 127.0.0.1:50051"
echo "──────────────────────────────────────────────────────────────────────"
echo "  按 Ctrl+C 停止所有开发服务"
echo "======================================================================"

# Watchdog 守护 agent
set +e
wait "$AGENT_PID" 2>/dev/null
wait_rc=$?
set -e

while [[ "$STOPPING" != "true" ]]; do
    echo "[watchdog] agent 已退出 (PID $AGENT_PID, exit code $wait_rc)，1 秒后自动重启..."
    sleep 1
    if [[ "$STOPPING" == "true" ]]; then
        break
    fi
    launch_agent
    if ! wait_for_service "$AGENT_URL/health" "重启后的 privacy_local_agent"; then
        echo "[watchdog] 警告：agent 重启后未在 30 秒内就绪（REST）。"
    fi
    # 等待 gRPC 端口就绪
    echo -n "等待重启后的 agent gRPC (127.0.0.1:50051) 就绪"
    for i in $(seq 1 30); do
        if _is_port_in_use 50051; then
            echo " OK"
            break
        fi
        echo -n "."
        sleep 1
        if [[ $i -eq 30 ]]; then
            echo " 超时"
        fi
    done
    set +e
    wait "$AGENT_PID" 2>/dev/null
    wait_rc=$?
    set -e
done
