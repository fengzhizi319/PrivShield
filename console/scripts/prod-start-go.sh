#!/usr/bin/env bash
# ============================================================================
# 【正式部署/生产预览模式】一键启动 Go gRPC 控制台 (静态托管)
# Launch Go gRPC proxy console in PROD mode with static dist hosting
#
# 用法 / Usage: ./console/scripts/prod-start-go.sh [--rebuild]
#   --rebuild  强制重新打包前端与 agent 依赖
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
AGENT_URL="http://127.0.0.1:8079"
CONSOLE_URL="http://127.0.0.1:8081"

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

# 1. Agent 依赖
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
    echo "--rebuild：重新安装 agent 依赖..."
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        pip install -e .
    )
fi

# Go 工具链检查
if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"
    exit 1
fi

# 2. 前端构建产物检查与打包
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
    echo "--rebuild：删除旧的前端构建产物并重新打包..."
    rm -rf "$CONSOLE_DIR/web/dist"
fi

if [[ ! -d "$CONSOLE_DIR/web/dist" ]]; then
    echo "未找到静态前端构建产物，自动打包：$CONSOLE_DIR/web/dist"
    _build_frontend
fi

# 3. 编译 Go gRPC 代理后端
echo "编译 Go gRPC 代理后端..."
(cd "$CONSOLE_DIR/bff-go" && go build -o bin/backend-go ./cmd/server)

AGENT_PID_FILE="$CONSOLE_DIR/.pids/agent-go.pid"
CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console-go.pid"
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
check_port_available 50051 "PrivShield gRPC"
check_port_available 8081 "Go gRPC 代理后端"

launch_agent() {
    local agent_log="$PROJECT_ROOT/.logs/agent_go.log"
    mkdir -p "$PROJECT_ROOT/.logs"
    echo "启动 PrivShield (REST: $AGENT_URL, gRPC: 127.0.0.1:50051)，日志: $agent_log..."
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        # 日志持久化到 .logs/agent_go.log（后台运行时终端不显示 agent 输出），
        # agent 崩溃/重启后可回溯根因；注意：不能用 exec ... | tee 管道，
        # 否则 $! 会指向 tee 而非 python，kill 将无法终止 agent。
        exec python -m PrivShield.server >> "$agent_log" 2>&1
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

echo "启动 Go gRPC 代理后端 (Console UI + API: $CONSOLE_URL)..."
(
    cd "$CONSOLE_DIR/bff-go"
    exec ./bin/backend-go
) &
CONSOLE_PID=$!
PIDS+=("$CONSOLE_PID")
write_pid "$CONSOLE_PID_FILE" "$CONSOLE_PID"

wait_for_service "$CONSOLE_URL/api/health" "Go gRPC 代理后端"

echo ""
echo "======================================================================"
echo "📦【正式部署/生产模式】 Go gRPC 代理控制台已成功启动！"
echo "======================================================================"
echo "  Console UI & API:    $CONSOLE_URL (Go 后端直接提供 UI 与 API)"
echo "  Agent REST 接口:     $AGENT_URL"
echo "  Agent gRPC 接口:     127.0.0.1:50051"
echo "──────────────────────────────────────────────────────────────────────"
echo "  按 Ctrl+C 停止所有服务"
echo "======================================================================"

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
    # 等待 gRPC 端口就绪：避免 agent 进程已起但 gRPC 未监听时健康检查连接失败
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
