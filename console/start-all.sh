#!/usr/bin/env bash
# ============================================================================
# 一键启动「双后端」隐私测试控制台：
# One-click launch of the "dual-backend" privacy test console:
#   同时启动 privacy_local_agent（REST + gRPC）、Python REST 代理后端（8080）
#   Simultaneously starts privacy_local_agent (REST + gRPC), Python REST proxy backend (8080)
#   与 Go gRPC 代理后端（8081）。
#   and Go gRPC proxy backend (8081).
#
# 启动后，前端顶部的 Backend Selector 可在两个后端间自由切换：
# After startup, the frontend Backend Selector can freely switch between the two backends:
#   - Python REST (8080)：经 Python FastAPI 代理调用 agent REST 接口；
#     Python REST (8080): proxies to agent REST API via Python FastAPI;
#   - Go gRPC    (8081)：经 Go 代理把请求转换为 gRPC 调用 agent。
#     Go gRPC (8081): converts requests to gRPC calls to agent via Go proxy.
#
# 用法 / Usage：./console/start-all.sh [--rebuild] [--force]
#   --rebuild  强制重新编译前端、后端与 agent（即使构建产物已存在）
#              Force rebuild frontend, backend and agent (even if artifacts exist)
#   --force    端口占用时自动终止占用进程（非交互模式，适用于 CI/CD）
#              Auto-kill port occupants on conflict (non-interactive, for CI/CD)
#
# 启动流程 / Startup Flow:
#   1. 解析参数 (--rebuild, --force) / Parse args
#   2. 自动创建虚拟环境并安装依赖 / Auto-create venvs and install deps
#   3. 构建前端 (pnpm/npm) / Build frontend
#   4. 编译 Go 后端二进制 / Compile Go backend binary
#   5. 端口预检 (4 个端口) / Port pre-check (4 ports)
#   6. 后台启动 3 个服务 + 健康检查轮询 / Launch 3 services + health poll
# ============================================================================

# 启用严格模式 / Enable strict mode
set -euo pipefail

# 获取脚本目录和项目根目录 / Get script dir and project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 解析命令行参数 / Parse CLI arguments
REBUILD=false
FORCE=false
for arg in "$@"; do
    case "$arg" in
        --rebuild) REBUILD=true ;;  # 强制重建 / force rebuild
        --force) FORCE=true ;;      # 非交互模式 / non-interactive mode
    esac
done

# 虚拟环境路径 / Virtual environment paths
AGENT_VENV="$PROJECT_ROOT/.venv"          # agent 主项目 venv / agent main project venv
BACKEND_VENV="$SCRIPT_DIR/backend/.venv"  # Python 后端 venv / Python backend venv

# 服务 URL 配置 / Service URL configuration
AGENT_URL="http://127.0.0.1:8079"      # agent REST 接口 / agent REST API
PY_CONSOLE_URL="http://127.0.0.1:8080" # Python REST 代理后端 / Python REST proxy backend
GO_CONSOLE_URL="http://127.0.0.1:8081" # Go gRPC 代理后端 / Go gRPC proxy backend

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

# ── 端口占用预检（冲突时自动诊断并提供 kill 选项）────────────────────
check_port_available() {
    local port="$1"  # 目标端口 / target port
    local name="$2"  # 服务名称 / service name

    # 快速检测端口是否可用（connect 方式，比 bind 更准确）
    if ! _is_port_in_use "$port"; then
        return 0
    fi

    # 端口被占用 —— 诊断占用进程 / Port occupied — diagnose occupying process
    echo ""
    echo "⚠️  端口 $port 已被占用（$name）"
    echo "────────────────────────────────────────"

    local pids=""
    # 优先 lsof / Prefer lsof
    if command -v lsof >/dev/null 2>&1; then
        echo "诊断信息（lsof -i :$port）："
        lsof -i :"$port" 2>/dev/null || true
        echo ""
        pids=$(lsof -t -i :"$port" 2>/dev/null | sort -u | tr '\n' ' ')  # 提取去重 PID / extract deduplicated PIDs
    elif command -v ss >/dev/null 2>&1; then
        echo "诊断信息（ss -tlnp | grep $port）："
        ss -tlnp 2>/dev/null | grep -E "LISTEN.*:$port\\s" || true
        echo ""
        pids=$(ss -tlnp 2>/dev/null | grep -E "LISTEN.*:$port\\s" | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | sort -u | tr '\n' ' ')
    elif command -v fuser >/dev/null 2>&1; then  # 回退 fuser / fallback fuser
        pids=$(fuser "$port"/tcp 2>/dev/null | tr -s ' ')
        echo "占用进程 PID：$pids"
        echo ""
    fi

    # 无法定位时报错 / Error if cannot locate
    if [[ -z "$pids" ]]; then
        echo "错误：无法定位占用端口 $port 的进程，请手动排查："
        echo "  lsof -i :$port"
        echo "  或 ss -tlnp | grep $port"
        exit 1
    fi

    echo "占用端口 $port 的进程 PID：$pids"
    echo ""

    # --force 模式：非交互环境下自动终止占用进程
    # --force mode: auto-kill occupying processes in non-interactive environments
    if [[ "$FORCE" == true ]]; then
        echo "--force 模式：自动终止占用进程"  # --force mode: auto-killing occupying processes
        for pid in $pids; do
            echo "  → kill -9 $pid"
            kill -9 "$pid" 2>/dev/null || true
        done
        sleep 1
        # 再次验证端口已释放（connect 方式）
        if ! _is_port_in_use "$port"; then
            echo "✅ 端口 $port 已释放"  # Port $port freed
        else
            echo "错误：端口 $port 仍被占用，请手动排查。"
            exit 1
        fi
        return 0
    fi

    # 交互式询问 / Interactive prompt
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

# 2. Python 控制台后端虚拟环境：缺失或 --rebuild 时自动创建并安装依赖
# 2. Python console backend venv: auto-create and install deps when missing or --rebuild
if [[ ! -d "$BACKEND_VENV" ]]; then
    echo "未找到 Python 后端虚拟环境，自动创建并安装依赖：$BACKEND_VENV"  # Python backend venv not found
    python3 -m venv "$BACKEND_VENV"  # 创建 venv / create venv
    (
        source "$BACKEND_VENV/bin/activate"  # 激活 / activate
        pip install --upgrade pip >/dev/null  # 升级 pip / upgrade pip
        pip install -r "$SCRIPT_DIR/backend/requirements.txt"  # 安装依赖 / install deps
    )
    echo "Python 后端依赖安装完成。"  # Python backend deps installed.
elif [[ "$REBUILD" == true ]]; then
    echo "--rebuild：重新安装 Python 后端依赖..."  # --rebuild: reinstalling Python backend deps...
    (
        source "$BACKEND_VENV/bin/activate"
        pip install -r "$SCRIPT_DIR/backend/requirements.txt"  # 重装 / reinstall
    )
    echo "Python 后端依赖重装完成。"  # Python backend deps reinstalled.
fi

# 3. Go 后端目录与工具链检查
# 3. Go backend directory and toolchain check
if [[ ! -d "$SCRIPT_DIR/backend-go" ]]; then
    echo "错误：未找到 Go 后端目录 $SCRIPT_DIR/backend-go"  # Error: Go backend dir not found
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"  # Error: Go toolchain not found
    exit 1
fi

# 4. 前端构建产物：缺失或使用 --rebuild 时自动执行 install + build（两个后端均基于该产物提供 UI）
# 4. Frontend artifacts: auto install + build when missing or --rebuild (both backends serve UI from this)
if [[ "$REBUILD" == true && -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "--rebuild：删除旧的前端构建产物并重新构建..."  # --rebuild: removing old dist and rebuilding...
    rm -rf "$SCRIPT_DIR/web/dist"  # 删除旧产物 / remove old artifacts
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
            echo "警告：未找到 pnpm/npm，跳过前端构建，后端将以 API 模式运行。"  # Warning: no pnpm/npm, API-only mode
        fi
    )
fi

if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "警告：前端构建产物 $SCRIPT_DIR/web/dist 不存在，后端将以 API 模式运行。"  # Warning: dist missing, API-only mode
fi

# 5. 预编译 Go gRPC 代理后端二进制，编译失败时提前暴露错误
# 5. Pre-compile Go gRPC proxy backend binary; expose compile errors early
echo "编译 Go gRPC 代理后端..."  # Compiling Go gRPC proxy backend...
(cd "$SCRIPT_DIR/backend-go" && go build -o bin/backend-go ./cmd/server)  # 编译 / compile
echo "Go 后端编译完成。"  # Go backend compiled.

# PID 文件路径配置 / PID file path configuration
AGENT_PID_FILE="$SCRIPT_DIR/.pids/agent-all.pid"       # agent 主服务 / main agent
PY_CONSOLE_PID_FILE="$SCRIPT_DIR/.pids/console-all.pid"  # Python REST 后端 / Python REST backend
GO_CONSOLE_PID_FILE="$SCRIPT_DIR/.pids/console-go-all.pid" # Go gRPC 后端 / Go gRPC backend

mkdir -p "$SCRIPT_DIR/.pids"  # 确保 PID 目录存在 / ensure PID dir exists

# write_pid - 将 PID 写入文件 / Write PID to file
write_pid() {
    local file="$1"  # 目标文件 / target file
    local pid="$2"   # 进程 ID / process ID
    echo "$pid" > "$file"
}

# 清理子进程（Ctrl+C 或脚本退出时触发）/ Cleanup children (on Ctrl+C or exit)
PIDS=()  # 后台进程 PID 数组 / background process PID array
cleanup() {
    echo ""
    echo "正在停止服务..."  # Stopping services...
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true  # 终止子进程 / kill child
    done
    wait 2>/dev/null || true  # 等待退出 / wait for exit
    rm -f "$AGENT_PID_FILE" "$PY_CONSOLE_PID_FILE" "$GO_CONSOLE_PID_FILE"  # 清理 PID 文件 / clean PID files
    echo "已停止。"  # Stopped.
}
# 注册信号处理 / Register signal handlers
trap cleanup INT TERM EXIT

# 端口预检（4 个端口）/ Port pre-check (4 ports)
check_port_available 8079 "privacy_local_agent REST"   # agent REST
check_port_available 50051 "privacy_local_agent gRPC"  # agent gRPC
check_port_available 8080 "Python REST 代理后端"  # Python REST proxy
check_port_available 8081 "Go gRPC 代理后端"      # Go gRPC proxy

# 启动 privacy_local_agent（同时监听 REST 8079 与 gRPC 50051）
# Launch privacy_local_agent (listens on both REST 8079 and gRPC 50051)
echo "启动 privacy_local_agent (REST: $AGENT_URL, gRPC: 127.0.0.1:50051)..."
(
    source "$AGENT_VENV/bin/activate"  # 激活 agent venv / activate agent venv
    cd "$PROJECT_ROOT"
    exec python -m privacy_local_agent.server  # 启动 REST+gRPC / start REST+gRPC
) &
AGENT_PID=$!  # 获取 PID / get PID
PIDS+=("$AGENT_PID")
write_pid "$AGENT_PID_FILE" "$AGENT_PID"

# ── 等待 agent 就绪后再启动两个后端 / Wait for agent before launching backends ──
# Go 后端依赖 agent gRPC，Python 后端依赖 agent REST，均需 agent 先就绪。
# Go backend depends on agent gRPC, Python backend depends on agent REST; agent must be ready first.

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

# ── agent 就绪后启动两个后端 / Start both backends after agent is ready ──

# 启动 Python REST 代理后端 / Launch Python REST proxy backend
echo "启动 Python REST 代理后端 (Console: $PY_CONSOLE_URL)..."
(
    source "$BACKEND_VENV/bin/activate"  # 激活后端 venv / activate backend venv
    cd "$SCRIPT_DIR/backend"
    exec uvicorn app.main:app --host 127.0.0.1 --port 8080  # Uvicorn ASGI / Uvicorn ASGI
) &
PY_CONSOLE_PID=$!
PIDS+=("$PY_CONSOLE_PID")
write_pid "$PY_CONSOLE_PID_FILE" "$PY_CONSOLE_PID"

# 启动 Go gRPC 代理后端 / Launch Go gRPC proxy backend
echo "启动 Go gRPC 代理后端 (Console: $GO_CONSOLE_URL)..."
(
    cd "$SCRIPT_DIR/backend-go"
    exec ./bin/backend-go  # 运行预编译二进制 / run pre-compiled binary
) &
GO_CONSOLE_PID=$!
PIDS+=("$GO_CONSOLE_PID")
write_pid "$GO_CONSOLE_PID_FILE" "$GO_CONSOLE_PID"

# 3) 等待两个后端就绪 / Wait for both backends ready
wait_for_service "$PY_CONSOLE_URL/api/health" "Python REST 代理后端"
wait_for_service "$GO_CONSOLE_URL/api/health" "Go gRPC 代理后端"

# 打印启动成功信息 / Print startup success info
echo ""
echo "======================================"
echo "双后端隐私测试控制台已启动"  # Dual-backend privacy test console started
echo "Agent REST:        $AGENT_URL"
echo "Agent gRPC:        127.0.0.1:50051"
echo "Python REST 后端:  $PY_CONSOLE_URL  (Console UI + API)"
echo "Go gRPC 后端:      $GO_CONSOLE_URL  (Console UI + API)"
echo ""
echo "打开任一 Console 地址，顶部 Backend Selector 可在两后端间切换。"  # Open either Console URL, use Backend Selector to switch
if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo ""
    echo "警告：前端尚未构建，Console 仅以 API 模式运行。"  # Warning: frontend not built, API-only mode
    echo "请先构建前端：cd $SCRIPT_DIR/web && npm install && npm run build"  # Build frontend first
fi
echo "按 Ctrl+C 停止所有服务"  # Press Ctrl+C to stop all services
echo "======================================"

# 保持脚本运行（等待所有后台进程）/ Keep script running (wait for all background processes)
wait
