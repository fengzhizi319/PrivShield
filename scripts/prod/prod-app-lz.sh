#!/usr/bin/env bash
# ============================================================================
# 【生产模式】一键启动 PrivShield 调度之眼控制台 (App-LZ BFF + 静态打包托管)
# Launch PrivShield App-LZ Console in PROD mode (:8085 BFF Hosting Static SPA)
#
# 用法 / Usage:
#   ./scripts/prod/prod-app-lz.sh [--force]
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

if _is_port_in_use "$BFF_PORT"; then
    if [[ "$FORCE" == "true" ]]; then
        echo "⚠️  端口 $BFF_PORT 被占用，--force 模式下自动清理..."
        _kill_port "$BFF_PORT"
    else
        echo "⚠️  端口 $BFF_PORT 已被占用！"
        echo "使用 --force 参数自动清理，或手动释放端口后重试。"
        exit 1
    fi
fi

echo "=================================================================="
echo " 🚀 构建并启动 PrivShield App-LZ [生产模式 (Static SPA Embedded)]"
echo "=================================================================="

# 1. 构建 Web 前端生产包
echo "构建 Web 前端生产静态资源..."
(cd "$APP_LZ_DIR/web" && npx vite build)

# 2. 编译 Go BFF
echo "编译 App-LZ Go BFF..."
(cd "$APP_LZ_DIR/bff-go" && go build -o bin/server ./cmd/server)

# 3. 启动 Go BFF 托管静态前端与聚合接口
echo "启动 App-LZ Go 独立托管服务 (:8085)..."
APP_LZ_PORT="$BFF_PORT" APP_LZ_STATIC_DIR="$APP_LZ_DIR/web/dist" "$APP_LZ_DIR/bff-go/bin/server" > "$LOGS_DIR/app-lz-prod.log" 2>&1 &
BFF_PID=$!
echo "$BFF_PID" > "$PIDS_DIR/app-lz-prod.pid"

cleanup() {
    echo ""
    echo "正在停止 App-LZ 生产服务..."
    kill "$BFF_PID" 2>/dev/null || true
    rm -f "$PIDS_DIR/app-lz-prod.pid"
    echo "已停止。"
    exit 0
}

trap cleanup INT TERM

echo "=================================================================="
echo " ✨ App-LZ 生产模式已就绪！"
echo " 🌐 访问地址: http://localhost:$BFF_PORT"
echo " 按 Ctrl+C 停止服务"
echo "=================================================================="

wait "$BFF_PID"
