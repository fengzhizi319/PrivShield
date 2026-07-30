#!/usr/bin/env bash
# ============================================================================
# 一键启动 Go gRPC 代理控制台：同时启动 privacy_local_agent 和 Go gRPC 后端
# One-click launch of Go gRPC proxy console: starts privacy_local_agent and Go gRPC backend
#
# 用法 / Usage：./console/start-go.sh [--rebuild]
#   --rebuild  强制重新编译前端与 agent（Go 后端每次均重新编译）
#              Force rebuild frontend and agent (Go backend always recompiles)
#
# 启动流程 / Startup Flow:
#   1. 解析参数 (--rebuild) / Parse args
#   2. 自动创建 agent 虚拟环境 / Auto-create agent venv
#   3. 构建前端 (pnpm/npm) / Build frontend
#   4. 编译 Go 后端 / Compile Go backend
#   5. 端口预检 (3 端口) / Port pre-check (3 ports)
#   6. 启动 agent + Go 后端 + 健康检查 / Launch agent + Go backend + health check
# ============================================================================

# 启用严格模式 / Enable strict mode
set -euo pipefail

# 获取脚本目录和项目根目录 / Get script dir and project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 解析命令行参数 / Parse CLI arguments
REBUILD=false
for arg in "$@"; do
    case "$arg" in
        --rebuild) REBUILD=true ;;  # 强制重建 / force rebuild
    esac
done

# 虚拟环境路径 / Virtual environment path
AGENT_VENV="$PROJECT_ROOT/.venv"  # agent 主项目 venv / agent main project venv

# 服务 URL 配置 / Service URL configuration
AGENT_URL="http://127.0.0.1:8079"    # agent REST 接口 / agent REST API
CONSOLE_URL="http://127.0.0.1:8081"  # Go gRPC 代理后端 / Go gRPC proxy backend

# ── 端口占用预检（冲突时自动诊断并提供 kill 选项）────────────────────
check_port_available() {
    local port="$1"  # 目标端口 / target port
    local name="$2"  # 服务名称 / service name

    # 快速检测端口是否可用 / Quick check if port is available
    if python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    s.bind(('127.0.0.1', $port))
except OSError:
    sys.exit(1)
finally:
    s.close()
" 2>/dev/null; then
        return 0
    fi

    # 端口被占用 —— 诊断占用进程
    echo ""
    echo "⚠️  端口 $port 已被占用（$name）"
    echo "────────────────────────────────────────"

    local pids=""
    if command -v lsof >/dev/null 2>&1; then
        echo "诊断信息（lsof -i :$port）："
        lsof -i :"$port" 2>/dev/null || true
        echo ""
        pids=$(lsof -t -i :"$port" 2>/dev/null | sort -u | tr '\n' ' ')
    elif command -v fuser >/dev/null 2>&1; then
        pids=$(fuser "$port"/tcp 2>/dev/null | tr -s ' ')
        echo "占用进程 PID：$pids"
        echo ""
    fi

    if [[ -z "$pids" ]]; then
        echo "错误：无法定位占用端口 $port 的进程，请手动排查："
        echo "  lsof -i :$port"
        echo "  或 ss -tlnp | grep $port"
        exit 1
    fi

    echo "占用端口 $port 的进程 PID：$pids"
    echo ""
    read -rp "是否自动终止上述进程以释放端口？[y/N] " answer
    case "$answer" in
        [yY]|[yY][eE][sS])
            for pid in $pids; do
                echo "  → kill -9 $pid"
                kill -9 "$pid" 2>/dev/null || true
            done
            sleep 1
            # 再次验证端口已释放
            if python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    s.bind(('127.0.0.1', $port))
except OSError:
    sys.exit(1)
finally:
    s.close()
" 2>/dev/null; then
                echo "✅ 端口 $port 已释放"
            else
                echo "错误：端口 $port 仍被占用，请手动排查。"
                exit 1
            fi
            ;;
        *)
            echo "已取消。请手动释放端口 $port 后重试："
            echo "  kill -9 $pids"
            exit 1
            ;;
    esac
}

# ── 自动补全缺失的依赖 / 构建产物 ─────────────────────────────────────

# 1. Agent 虚拟环境：缺失或 --rebuild 时自动创建并安装项目依赖
# 1. Agent venv: auto-create and install deps when missing or --rebuild
if [[ ! -d "$AGENT_VENV" ]]; then
    echo "未找到 agent 虚拟环境，自动创建并安装依赖：$AGENT_VENV"  # Agent venv not found, auto-creating
    python3 -m venv "$AGENT_VENV"  # 创建 venv / create venv
    (
        source "$AGENT_VENV/bin/activate"  # 激活 / activate
        cd "$PROJECT_ROOT"
        pip install --upgrade pip >/dev/null  # 升级 pip / upgrade pip
        pip install -e .  # 可编辑安装 / editable install
    )
    echo "agent 依赖安装完成。"  # Agent deps installed.
elif [[ "$REBUILD" == true ]]; then
    echo "--rebuild：重新安装 agent 依赖..."  # --rebuild: reinstalling agent deps...
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        pip install -e .  # 重装 / reinstall
    )
    echo "agent 依赖重装完成。"  # Agent deps reinstalled.
fi

# Go 后端目录与工具链检查 / Go backend dir and toolchain check
if [[ ! -d "$SCRIPT_DIR/backend-go" ]]; then
    echo "错误：未找到 Go 后端目录 $SCRIPT_DIR/backend-go"  # Error: Go backend dir not found
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"  # Error: Go toolchain not found
    exit 1
fi

# 2. 前端构建产物：缺失或 --rebuild 时自动执行 install + build（Go 后端基于该产物提供 Console UI）
# 2. Frontend artifacts: auto install + build when missing or --rebuild (Go backend serves Console UI from this)
if [[ "$REBUILD" == true && -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "--rebuild：删除旧的前端构建产物并重新构建..."  # --rebuild: removing old dist...
    rm -rf "$SCRIPT_DIR/web/dist"  # 删除 / remove
fi
if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "未找到前端构建产物，自动构建：$SCRIPT_DIR/web/dist"  # Frontend dist not found, auto-building
    (
        cd "$SCRIPT_DIR/web"
        if command -v pnpm >/dev/null 2>&1; then
            pnpm install && pnpm build  # 优先 pnpm / prefer pnpm
        elif command -v npm >/dev/null 2>&1; then
            npm install && npm run build  # 回退 npm / fallback npm
        else
            echo "警告：未找到 pnpm/npm，跳过前端构建。"  # Warning: no pnpm/npm, skipping build
        fi
    )
fi

# 3. 预编译 Go gRPC 代理后端二进制，编译失败时提前暴露错误
# 3. Pre-compile Go gRPC proxy binary; expose compile errors early
echo "编译 Go gRPC 代理后端..."  # Compiling Go gRPC proxy backend...
(cd "$SCRIPT_DIR/backend-go" && go build -o bin/backend-go ./cmd/server)  # 编译 / compile
echo "Go 后端编译完成。"  # Go backend compiled.

# PID 文件配置 / PID file configuration
AGENT_PID_FILE="$SCRIPT_DIR/.pids/agent-go.pid"    # agent PID
CONSOLE_PID_FILE="$SCRIPT_DIR/.pids/console-go.pid" # Go 后端 PID / Go backend PID

mkdir -p "$SCRIPT_DIR/.pids"  # 确保目录存在 / ensure dir exists

# write_pid - 写入 PID / Write PID to file
write_pid() {
    local file="$1"  # 目标文件 / target file
    local pid="$2"   # 进程 ID / process ID
    echo "$pid" > "$file"
}

# 清理子进程 / Cleanup child processes
PIDS=()  # PID 数组 / PID array
cleanup() {
    echo ""
    echo "正在停止服务..."  # Stopping services...
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true  # 终止 / kill
    done
    wait 2>/dev/null || true  # 等待 / wait
    rm -f "$AGENT_PID_FILE" "$CONSOLE_PID_FILE"  # 清理 PID 文件 / clean PID files
    echo "已停止。"  # Stopped.
}
trap cleanup INT TERM EXIT  # 注册信号 / register signals

# 端口预检 / Port pre-check
check_port_available 8079 "privacy_local_agent REST"   # agent REST
check_port_available 50051 "privacy_local_agent gRPC"  # agent gRPC
check_port_available 8081 "Go gRPC 代理后端"  # Go gRPC proxy

# 启动 privacy_local_agent
# Launch privacy_local_agent
# 默认会同时启动 REST (8079) 和 gRPC (50051)，Go 后端通过 gRPC 调用 agent
# By default starts both REST (8079) and gRPC (50051); Go backend calls agent via gRPC
echo "启动 privacy_local_agent (REST: $AGENT_URL, gRPC: 127.0.0.1:50051)..."
(
    source "$AGENT_VENV/bin/activate"  # 激活 agent venv / activate agent venv
    cd "$PROJECT_ROOT"
    exec python -m privacy_local_agent.server  # 启动服务 / start server
) &
AGENT_PID=$!  # 获取 PID / get PID
PIDS+=("$AGENT_PID")
write_pid "$AGENT_PID_FILE" "$AGENT_PID"

# 启动 Go gRPC 代理后端 / Launch Go gRPC proxy backend
echo "启动 Go gRPC 代理后端 (Console: $CONSOLE_URL)..."
(
    cd "$SCRIPT_DIR/backend-go"
    exec ./bin/backend-go  # 运行预编译二进制 / run pre-compiled binary
) &
CONSOLE_PID=$!
PIDS+=("$CONSOLE_PID")
write_pid "$CONSOLE_PID_FILE" "$CONSOLE_PID"

# 等待服务就绪 / Wait for services to be ready
# 轮询健康检查，最多 30 次 / Poll health check, up to 30 attempts
wait_for_service() {
    local url="$1"   # 健康检查 URL / health check URL
    local name="$2"  # 服务名 / service name
    local max_attempts=30  # 最大尝试 / max attempts
    local attempt=0        # 当前计数 / current count
    echo -n "等待 $name 就绪"  # Waiting for $name
    while [[ $attempt -lt $max_attempts ]]; do
        if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q '^200$'; then  # HTTP 200 = 就绪 / ready
            echo " OK"
            return 0
        fi
        echo -n "."  # 进度 / progress
        sleep 1
        attempt=$((attempt + 1))
    done
    echo " 超时"  # Timeout
    return 1
}

# 等待两个服务就绪 / Wait for both services
wait_for_service "$AGENT_URL/health" "privacy_local_agent"
wait_for_service "$CONSOLE_URL/api/health" "Go gRPC 代理后端"

# 打印启动信息 / Print startup info
echo ""
echo "======================================"
echo "Go gRPC 代理控制台已启动"  # Go gRPC proxy console started
echo "Agent REST:  $AGENT_URL"
echo "Agent gRPC:  127.0.0.1:50051"
echo "Console UI:  $CONSOLE_URL (Go 后端直接提供 UI 与 API)"  # Go backend directly serves UI and API
if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo ""
    echo "警告：前端尚未构建，$CONSOLE_URL 仅以 API 模式运行。"  # Warning: frontend not built, API-only mode
    echo "请先构建前端：cd $SCRIPT_DIR/web && corepack pnpm install && corepack pnpm build"  # Build frontend first
    echo "构建完成后重新执行 ./console/start-go.sh 即可打开 Console UI。"  # Re-run start-go.sh after build
fi
echo "按 Ctrl+C 停止所有服务"  # Press Ctrl+C to stop all services
echo "======================================"

# 保持脚本运行 / Keep script running
wait
