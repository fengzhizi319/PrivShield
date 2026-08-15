#!/usr/bin/env bash
# ============================================================================
# 【正式部署/生产预览模式】一键启动 Python REST 控制台 (静态托管)
# Launch Python REST proxy console in PROD mode with static dist hosting
#
# 用法 / Usage: ./console/scripts/prod-start.sh [--rebuild]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONSOLE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

REBUILD=false
for arg in "$@"; do
    case "$arg" in
        --rebuild) REBUILD=true ;;
    esac
done

AGENT_VENV="$PROJECT_ROOT/.venv"
BACKEND_VENV="$CONSOLE_DIR/backend/.venv"

AGENT_URL="http://127.0.0.1:8079"
CONSOLE_URL="http://127.0.0.1:8080"

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
elif [[ "$REBUILD" == true ]]; then
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
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
elif [[ "$REBUILD" == true ]]; then
    (
        source "$BACKEND_VENV/bin/activate"
        pip install -r "$CONSOLE_DIR/backend/requirements.txt"
    )
fi

# 3. 前端打包
_build_frontend() {
    (
        cd "$CONSOLE_DIR/web"
        if command -v pnpm >/dev/null 2>&1; then
            # 若已存在 node_modules，优先直接 build，避免网络波动/无网环境下连线 npm 仓库失败
            if [[ -d "node_modules" ]] && pnpm build 2>/dev/null; then
                return 0
            fi
            # 否则执行 install（优先从离线缓存读取）并构建
            (pnpm install --prefer-offline 2>/dev/null || pnpm install) && pnpm build
        elif command -v npm >/dev/null 2>&1; then
            # 若已存在 node_modules，优先直接 build，避免网络波动/无网环境下连线 npm 仓库失败
            if [[ -d "node_modules" ]] && npm run build 2>/dev/null; then
                return 0
            fi
            npm install && npm run build
        else
            echo "警告：未找到 pnpm/npm，跳过前端打包。"
        fi
    )
}

if [[ "$REBUILD" == true && -d "$CONSOLE_DIR/web/dist" ]]; then
    rm -rf "$CONSOLE_DIR/web/dist"
fi

if [[ ! -d "$CONSOLE_DIR/web/dist" ]]; then
    echo "未找到前端打包产物，自动打包：$CONSOLE_DIR/web/dist"
    _build_frontend
fi

AGENT_PID_FILE="$CONSOLE_DIR/.pids/agent.pid"
CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console.pid"
mkdir -p "$CONSOLE_DIR/.pids"

write_pid() {
    echo "$2" > "$1"
}

PIDS=()
STOPPING=false
cleanup() {
    STOPPING=true
    echo ""
    echo "正在停止【生产模式】所有服务..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -f "$AGENT_PID_FILE" "$CONSOLE_PID_FILE"
    echo "已停止。"
}
trap cleanup INT TERM EXIT

check_port_available 8079 "PrivShield REST"
check_port_available 8080 "Python REST 代理后端"

launch_agent() {
    local agent_log="$PROJECT_ROOT/.logs/agent_py.log"
    mkdir -p "$PROJECT_ROOT/.logs"
    echo "启动 PrivShield (REST: $AGENT_URL)，日志: $agent_log..."
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        # 日志持久化到 .logs/agent_py.log，agent 崩溃/重启后可回溯根因
        exec python -m PrivShield.server >> "$agent_log" 2>&1
    ) &
    AGENT_PID=$!
    PIDS[0]="$AGENT_PID"
    write_pid "$AGENT_PID_FILE" "$AGENT_PID"
}
launch_agent

echo "启动测试控制台后端 (Console UI + API: $CONSOLE_URL)..."
(
    source "$BACKEND_VENV/bin/activate"
    cd "$CONSOLE_DIR/backend"
    exec uvicorn app.main:app --host 127.0.0.1 --port 8080
) &
CONSOLE_PID=$!
PIDS+=("$CONSOLE_PID")
write_pid "$CONSOLE_PID_FILE" "$CONSOLE_PID"

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
wait_for_service "$CONSOLE_URL/api/health" "测试控制台后端"

echo ""
echo "======================================================================"
echo "📦【正式部署/生产模式】 Python REST 代理控制台已成功启动！"
echo "======================================================================"
echo "  Console UI & API:    $CONSOLE_URL (Python 后端直接提供 UI 与 API)"
echo "  Agent REST 接口:     $AGENT_URL"
echo "──────────────────────────────────────────────────────────────────────"
echo "  按 Ctrl+C 停止所有服务"
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
    if ! wait_for_service "$AGENT_URL/health" "重启后的 PrivShield"; then
        echo "[watchdog] 警告：agent 重启后未在 30 秒内就绪（REST）。"
    fi
    set +e
    wait "$AGENT_PID" 2>/dev/null
    wait_rc=$?
    set -e
done
