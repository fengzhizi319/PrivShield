#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键启动 PrivShield 调度之眼控制台 (App-LZ BFF + Vite HMR)
# Launch PrivShield App-LZ Console in DEV mode (:8085 BFF + :5174 Web)
#
# 用法 / Usage:
#   ./scripts/dev/dev-app-lz.sh [--force]
# ============================================================================

set -euo pipefail

FORCE=false
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo "  --force   端口被占用时自动终止占用进程（非交互模式）"
            echo "  -h, --help 显示此帮助信息"
            exit 0
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
APP_LZ_DIR="$PROJECT_ROOT/console/app-lz"
PIDS_DIR="$PROJECT_ROOT/.pids"
LOGS_DIR="$PROJECT_ROOT/.logs"

mkdir -p "$PIDS_DIR" "$LOGS_DIR"

BFF_PORT=8085
VITE_PORT=5174

_is_port_in_use() {
    local port="$1"
    python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(0.5)
res = s.connect_ex(('127.0.0.1', int('$port')))
s.close()
sys.exit(0 if res == 0 else 1)
" 2>/dev/null
}

_kill_port() {
    local port="$1"
    if _is_port_in_use "$port"; then
        echo "终止端口 $port 上的占用进程..."
        fuser -k -9 "${port}/tcp" 2>/dev/null || true
        sleep 0.5
    fi
}

check_and_free_port() {
    local port="$1"
    local desc="$2"
    if _is_port_in_use "$port"; then
        if [[ "$FORCE" == "true" ]]; then
            echo "⚠️  端口 $port ($desc) 被占用，--force 模式下自动清理..."
            _kill_port "$port"
        else
            echo "⚠️  端口 $port ($desc) 已被占用！"
            echo "使用 --force 参数自动清理，或手动释放端口后重试。"
            exit 1
        fi
    fi
}

check_and_free_port "$BFF_PORT" "App-LZ Go BFF"
check_and_free_port "$VITE_PORT" "App-LZ Vite Web"

echo "=================================================================="
echo " 🚀 启动 PrivShield App-LZ 调度全景控制台 [开发模式 (HMR)]"
echo "=================================================================="
echo "  BFF 后端端口:   http://127.0.0.1:$BFF_PORT"
echo "  Web 前端端口:   http://localhost:$VITE_PORT"
echo "  调度中枢 (Hub): http://127.0.0.1:8082"
echo "=================================================================="

# 1. 编译并启动 Go BFF
echo "编译 App-LZ Go BFF..."
(cd "$APP_LZ_DIR/bff-go" && go build -o bin/server ./cmd/server)

echo "启动 App-LZ Go BFF..."
APP_LZ_PORT="$BFF_PORT" "$APP_LZ_DIR/bff-go/bin/server" > "$LOGS_DIR/app-lz-bff.log" 2>&1 &
BFF_PID=$!
echo "$BFF_PID" > "$PIDS_DIR/app-lz-bff.pid"

# 等待 BFF 就绪
for i in {1..30}; do
    if curl -s "http://127.0.0.1:$BFF_PORT/api/health" >/dev/null 2>&1; then
        echo "✅ App-LZ Go BFF 已就绪 (PID: $BFF_PID)"
        break
    fi
    sleep 0.2
done

# 2. 启动 Vite 前端开发服务器
echo "启动 App-LZ Vite 前端开发服务器 (HMR: :$VITE_PORT)..."
(cd "$APP_LZ_DIR/web" && npx vite --port "$VITE_PORT" --host 0.0.0.0) > "$LOGS_DIR/app-lz-web.log" 2>&1 &
WEB_PID=$!
echo "$WEB_PID" > "$PIDS_DIR/app-lz-web.pid"

cleanup() {
    echo ""
    echo "正在停止 App-LZ 控制台服务..."
    kill "$BFF_PID" 2>/dev/null || true
    kill "$WEB_PID" 2>/dev/null || true
    rm -f "$PIDS_DIR/app-lz-bff.pid" "$PIDS_DIR/app-lz-web.pid"
    echo "已停止。"
    exit 0
}

trap cleanup INT TERM

echo "=================================================================="
echo " ✨ App-LZ 控制台已启动完成！"
echo " 🌐 前端访问地址: http://localhost:$VITE_PORT"
echo " 🔌 BFF 接口地址: http://127.0.0.1:$BFF_PORT/api/lz/topology"
echo " 按 Ctrl+C 停止服务"
echo "=================================================================="

wait "$BFF_PID" "$WEB_PID"
