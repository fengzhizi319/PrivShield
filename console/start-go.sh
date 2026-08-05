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
# ── TCP connect 端口探测（比 bind 更可靠，不会被 SO_REUSEADDR 误导）────
# 原理：尝试 connect() 到 127.0.0.1:port，连接成功→端口已被占用，连接拒绝→端口空闲
_is_port_in_use() {
    local port="$1"
    python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(0.5)
try:
    s.connect(('127.0.0.1', $port))
    s.close()
    sys.exit(0)  # 连接成功 → 端口已被占用
except (ConnectionRefusedError, socket.timeout, OSError):
    sys.exit(1)  # 连接失败 → 端口空闲
" 2>/dev/null
}

check_port_available() {
    local port="$1"  # 目标端口 / target port
    local name="$2"  # 服务名称 / service name

    # 快速检测端口是否可用（connect 方式，比 bind 更准确）
    if ! _is_port_in_use "$port"; then
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
    elif command -v ss >/dev/null 2>&1; then
        echo "诊断信息（ss -tlnp | grep $port）："
        ss -tlnp 2>/dev/null | grep -E "LISTEN.*:$port\\s" || true
        echo ""
        pids=$(ss -tlnp 2>/dev/null | grep -E "LISTEN.*:$port\\s" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | sort -u | tr '\n' ' ')
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
            # 再次验证端口已释放（connect 方式）
            if ! _is_port_in_use "$port"; then
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

# macOS Apple Silicon 专属提示：mlx 未安装时 NER 无法启用 Metal GPU 加速。
# macOS Apple Silicon hint: without mlx the NER layer cannot use Metal GPU acceleration.
# 注意：agent 的引擎探测结果缓存于进程生命周期内，运行中补装 mlx 后
# 需在控制台“运维诊断”页点击“刷新诊断”重新探测，或重启服务。
# Note: engine probe results are cached per process; after installing mlx at runtime,
# click "Refresh" on the Ops Diagnostics page to re-probe, or restart the service.
if [[ "$(uname -s)" == "Darwin" && "$(uname -m)" == "arm64" ]]; then
    if ! "$AGENT_VENV/bin/python" -c "import importlib.util, sys; sys.exit(0 if importlib.util.find_spec('mlx') else 1)" 2>/dev/null; then
        echo ""
        echo "ℹ️  检测到 macOS Apple Silicon，但 agent 虚拟环境未安装 mlx："
        echo "   NER 将无法使用 Metal GPU 加速（会自动降级到 ONNX/ModelScope）。"
        echo "   启用方式：source $AGENT_VENV/bin/activate && pip install mlx"
        echo ""
    fi
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

# 2. 前端构建产物：缺失、过期（源码比产物新）或 --rebuild 时自动执行 install + build
# 2. Frontend artifacts: auto install + build when missing, stale (sources newer than dist) or --rebuild

# 执行前端构建（pnpm 优先，npm 回退）/ Run frontend build (prefer pnpm, fallback npm)
_build_frontend() {
    (
        cd "$SCRIPT_DIR/web"
        if command -v pnpm >/dev/null 2>&1; then
            # 若已存在 node_modules，优先直接 build，避免网络波动/无网环境下连线 npm 仓库失败
            if [[ -d "node_modules" ]] && pnpm build 2>/dev/null; then
                return 0
            fi
            # 否则执行 install（优先从离线缓存读取）并构建
            (pnpm install --prefer-offline 2>/dev/null || pnpm install) && pnpm build
        elif command -v npm >/dev/null 2>&1; then
            if [[ -d "node_modules" ]] && npm run build 2>/dev/null; then
                return 0
            fi
            npm install && npm run build
        else
            echo "警告：未找到 pnpm/npm，跳过前端构建。"  # Warning: no pnpm/npm, skipping build
        fi
    )
}

# 判断前端构建产物是否过期：任一源码文件比 dist/index.html 新则视为过期。
# Detect stale frontend artifacts: any source file newer than dist/index.html means stale.
# 覆盖 git pull 后源码更新但 dist 未重编译的场景（find -newer 兼容 BSD/GNU find）。
# Covers the case where sources changed after git pull but dist was not rebuilt.
_frontend_is_stale() {
    local marker="$SCRIPT_DIR/web/dist/index.html"
    # 构建标记不存在（如上次构建中断）视为过期 / Missing build marker (e.g. interrupted build) → stale
    [[ -f "$marker" ]] || return 0
    # 列出比构建标记更新的源码文件，命中任意一个即短路返回（-print -quit）
    # List source files newer than the marker; short-circuit on first hit
    local newer
    newer=$(find \
        "$SCRIPT_DIR/web/src" \
        "$SCRIPT_DIR/web/index.html" \
        "$SCRIPT_DIR/web/package.json" \
        "$SCRIPT_DIR/web/vite.config.ts" \
        "$SCRIPT_DIR/web/tailwind.config.js" \
        "$SCRIPT_DIR/web/postcss.config.js" \
        "$SCRIPT_DIR/web/tsconfig.json" \
        "$SCRIPT_DIR/web/tsconfig.node.json" \
        -newer "$marker" -print -quit 2>/dev/null || true)
    [[ -n "$newer" ]]
}

if [[ "$REBUILD" == true && -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "--rebuild：删除旧的前端构建产物并重新构建..."  # --rebuild: removing old dist...
    rm -rf "$SCRIPT_DIR/web/dist"  # 删除 / remove
fi
if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "未找到前端构建产物，自动构建：$SCRIPT_DIR/web/dist"  # Frontend dist not found, auto-building
    _build_frontend
elif _frontend_is_stale; then
    echo "检测到前端源码比构建产物更新（可能刚执行过 git pull），自动重新构建前端..."
    echo "Frontend sources are newer than dist (possibly after git pull), rebuilding..."
    _build_frontend
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
# 停止标志：为 true 时 watchdog 不再自动拉起 agent（区分用户主动停止 vs 崩溃）
# Stop flag: when true the watchdog stops auto-restarting the agent
# (distinguishes intentional shutdown from crash).
STOPPING=false
cleanup() {
    STOPPING=true  # 标记正在停止，禁止 watchdog 重启 / mark stopping to block watchdog restart
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

# 启动 privacy_local_agent（watchdog 崩溃重启时复用）
# Launch privacy_local_agent (reused by watchdog for crash restart)
# 默认会同时启动 REST (8079) 和 gRPC (50051)，Go 后端通过 gRPC 调用 agent
# By default starts both REST (8079) and gRPC (50051); Go backend calls agent via gRPC
launch_agent() {
    echo "启动 privacy_local_agent (REST: $AGENT_URL, gRPC: 127.0.0.1:50051)..."
    (
        source "$AGENT_VENV/bin/activate"  # 激活 agent venv / activate agent venv
        cd "$PROJECT_ROOT"
        exec python -m privacy_local_agent.server  # 启动服务 / start server
    ) &
    AGENT_PID=$!  # 获取 PID / get PID
    PIDS[0]="$AGENT_PID"  # 更新 PID 数组首位，cleanup 时能停止最新 agent
    write_pid "$AGENT_PID_FILE" "$AGENT_PID"
}

launch_agent

# ── 等待 agent REST 与 gRPC 就绪后再启动 Go 后端 ────────────────────────
# Go 后端依赖 agent gRPC 连接，必须在 agent 完全就绪后启动，
# 否则 grpc.NewClient 首次 RPC 会得到 "connection refused"。
# Go backend depends on agent gRPC; must wait for agent to be fully ready,
# otherwise grpc.NewClient's first RPC gets "connection refused".

# 轮询健康检查通用函数 / Generic health-check polling function
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

# 1) 等待 agent REST 就绪 / Wait for agent REST ready
wait_for_service "$AGENT_URL/health" "privacy_local_agent"

# 2) 等待 agent gRPC 端口接受连接 / Wait for agent gRPC port accepting connections
#    虽然 REST 可能先就绪，gRPC 端口绑定可能稍慢（server.add_insecure_port 晚于 uvicorn 启动）。
#    Although REST may be ready first, gRPC port binding may lag slightly
#    (server.add_insecure_port happens after uvicorn starts).
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
        echo "错误：agent gRPC 未在 30 秒内就绪，请检查 agent 日志。"
        exit 1
    fi
done

# ── agent 完全就绪后启动 Go 后端 / Start Go backend only after agent is fully ready ──
echo "启动 Go gRPC 代理后端 (Console: $CONSOLE_URL)..."
(
    cd "$SCRIPT_DIR/backend-go"
    exec ./bin/backend-go  # 运行预编译二进制 / run pre-compiled binary
) &
CONSOLE_PID=$!
PIDS+=("$CONSOLE_PID")
write_pid "$CONSOLE_PID_FILE" "$CONSOLE_PID"

# 3) 等待 Go 后端就绪 / Wait for Go backend ready
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

# ── Agent 崩溃自动拉起 watchdog / Watchdog: auto-restart agent on crash ──────────
# 当 agent 因 OOM/崩溃退出时，Go 后端 gRPC 连接会被重置（connection reset by peer），
# 客户端 keepalive + 重试策略可透明恢复；watchdog 负责在数秒内重新拉起 agent，
# 缩短故障窗口，避免需要人工重启。
# When the agent exits due to OOM/crash, the Go backend's gRPC connection gets reset;
# client keepalive + retry policy recover transparently, while this watchdog relaunches
# the agent within seconds to minimize the failure window without manual intervention.

# 等待 agent 退出（阻塞），返回退出码 / Block until agent exits, capture exit code
set +e  # 临时关闭 set -e：agent 崩溃退出码非零不应终止脚本
wait "$AGENT_PID" 2>/dev/null
wait_rc=$?
set -e

echo "[watchdog] agent 已退出 (PID $AGENT_PID, exit code $wait_rc)，开始自动拉起监督..."
while [[ "$STOPPING" != "true" ]]; do
    # 重新拉起 agent / Relaunch the agent
    echo "[watchdog] 1 秒后自动重启 agent..."
    sleep 1
    if [[ "$STOPPING" == "true" ]]; then
        break
    fi
    launch_agent
    # 等待重启后的 agent 就绪（REST 健康检查通过即视为就绪）
    if ! wait_for_service "$AGENT_URL/health" "重启后的 privacy_local_agent"; then
        echo "[watchdog] 警告：agent 重启后未在 30 秒内就绪，继续监督。"
    fi
    # 阻塞等待 agent 再次退出（wait 同时回收僵尸进程）
    set +e
    wait "$AGENT_PID" 2>/dev/null
    wait_rc=$?
    set -e
    if [[ "$STOPPING" == "true" ]]; then
        break
    fi
    echo "[watchdog] agent 再次退出 (PID $AGENT_PID, exit code $wait_rc)"
done
echo "[watchdog] 监督循环已退出。"
