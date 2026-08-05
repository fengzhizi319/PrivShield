#!/usr/bin/env bash
# ============================================================================
# 【正式部署/生产模式】一键启动 mTLS 模式 Go gRPC 控制台 (静态托管)
# Launch Go gRPC proxy console with mTLS in PROD mode with static dist hosting
#
# 用法 / Usage: ./console/scripts/prod-start-go-mtls.sh [--rebuild]
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
CERT_DIR="$CONSOLE_DIR/backend-go/certs"
GEN_CERTS="$CONSOLE_DIR/backend-go/scripts/gen-certs.sh"

CONSOLE_URL="http://127.0.0.1:8081"
AGENT_GRPC_ADDR="127.0.0.1:50051"

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

# 1. 证书检测
if [[ ! -f "$CERT_DIR/ca.crt" || ! -f "$CERT_DIR/server.crt" || ! -f "$CERT_DIR/client.crt" ]]; then
    echo "未找到 mTLS 证书，自动生成测试证书链..."
    bash "$GEN_CERTS" "$CERT_DIR"
fi

# 2. Agent 依赖
if [[ ! -d "$AGENT_VENV" ]]; then
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

# 3. Go 工具链
if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"
    exit 1
fi

# 4. 前端打包
_build_frontend() {
    (
        cd "$CONSOLE_DIR/web"
        if command -v pnpm >/dev/null 2>&1; then
            pnpm install && pnpm build
        elif command -v npm >/dev/null 2>&1; then
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
    echo "未找到静态前端构建产物，自动打包：$CONSOLE_DIR/web/dist"
    _build_frontend
fi

# 5. 编译 Go 代理
echo "编译 Go gRPC 代理后端..."
(cd "$CONSOLE_DIR/backend-go" && go build -o bin/backend-go ./cmd/server)

AGENT_PID_FILE="$CONSOLE_DIR/.pids/agent-go-mtls.pid"
CONSOLE_PID_FILE="$CONSOLE_DIR/.pids/console-go-mtls.pid"
mkdir -p "$CONSOLE_DIR/.pids"

write_pid() {
    echo "$2" > "$1"
}

PIDS=()
cleanup() {
    echo ""
    echo "正在停止【生产模式 mTLS】所有服务..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -f "$AGENT_PID_FILE" "$CONSOLE_PID_FILE"
    echo "已停止。"
}
trap cleanup INT TERM EXIT

check_port_available 8079 "privacy_local_agent REST"
check_port_available 50051 "privacy_local_agent gRPC (mTLS)"
check_port_available 8081 "Go gRPC 代理后端"

echo "启动 privacy_local_agent (gRPC mTLS: $AGENT_GRPC_ADDR, client_auth=require)..."
(
    source "$AGENT_VENV/bin/activate"
    cd "$PROJECT_ROOT"
    export PRIVACY_TLS_ENABLED=true
    export PRIVACY_TLS_CERT_FILE="$CERT_DIR/server.crt"
    export PRIVACY_TLS_KEY_FILE="$CERT_DIR/server.key"
    export PRIVACY_TLS_CA_FILE="$CERT_DIR/ca.crt"
    export PRIVACY_TLS_CLIENT_AUTH=require
    exec python -m privacy_local_agent.server
) &
AGENT_PID=$!
PIDS+=("$AGENT_PID")
write_pid "$AGENT_PID_FILE" "$AGENT_PID"

echo -n "等待 agent gRPC mTLS (127.0.0.1:50051) 就绪"
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

echo "启动 Go gRPC 代理后端 (mTLS -> $AGENT_GRPC_ADDR, Console: $CONSOLE_URL)..."
(
    cd "$CONSOLE_DIR/backend-go"
    export PRIVACY_AGENT_TLS_ENABLED=true
    export PRIVACY_AGENT_TLS_CERT_FILE="$CERT_DIR/client.crt"
    export PRIVACY_AGENT_TLS_KEY_FILE="$CERT_DIR/client.key"
    export PRIVACY_AGENT_TLS_CA_FILE="$CERT_DIR/ca.crt"
    export PRIVACY_AGENT_TLS_SERVER_NAME=localhost
    exec ./bin/backend-go
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
wait_for_service "$CONSOLE_URL/api/health" "Go gRPC 代理后端"

echo ""
echo "======================================================================"
echo "🔒📦【正式部署/生产模式 mTLS】 Go gRPC 代理控制台已成功启动！"
echo "======================================================================"
echo "  Agent gRPC (mTLS):  $AGENT_GRPC_ADDR (双向认证已开启)"
echo "  Console UI & API:   $CONSOLE_URL (Go 后端托管静态 dist/)"
echo "──────────────────────────────────────────────────────────────────────"
echo "  按 Ctrl+C 停止所有服务"
echo "======================================================================"

wait
