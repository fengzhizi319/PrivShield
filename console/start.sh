#!/usr/bin/env bash
# ============================================================================
# 一键启动隐私测试控制台：同时启动 privacy_local_agent 和前端代理后端
# One-click launch of the privacy test console: starts privacy_local_agent and the frontend proxy backend
#
# 用法 / Usage：./console/start.sh [--rebuild]
#   --rebuild  强制重新编译前端、后端与 agent（即使构建产物已存在）
#              Force rebuild frontend, backend and agent (even if build artifacts exist)
#
# 设计目标 / Design Goals:
# - 尽量“开箱即用”，减少首次启动时的手工准备步骤
#   Out-of-the-box experience, minimize manual setup steps on first launch
# - 在依赖缺失时优雅降级，而不是直接失败
#   Graceful degradation when dependencies are missing, rather than hard failure
# - 启动后先等待关键健康检查通过，再提示用户访问地址
#   Wait for critical health checks to pass before showing access URLs
#
# 启动流程 / Startup Flow:
#   1. 解析命令行参数 (--rebuild)
#      Parse CLI arguments (--rebuild)
#   2. 自动创建虚拟环境并安装依赖（agent + 控制台后端）
#      Auto-create venvs and install dependencies (agent + console backend)
#   3. 构建前端（pnpm/npm，缺失则降级为 API 模式）
#      Build frontend (pnpm/npm, degrade to API mode if missing)
#   4. 端口占用预检（冲突时提供交互式解决）
#      Port availability pre-check (interactive resolution on conflict)
#   5. 后台启动 agent + 控制台后端，写入 PID 文件
#      Launch agent + console backend in background, write PID files
#   6. 轮询健康检查接口，确认服务就绪后打印访问地址
#      Poll health endpoints, print access URLs after services are ready
# ============================================================================

# 启用严格模式：遇到错误立即退出(-e)、未定义变量报错(-u)、管道失败传播(-o pipefail)
# Enable strict mode: exit on error(-e), error on undefined vars(-u), pipe failure propagates(-o pipefail)
set -euo pipefail

# 获取脚本自身所在目录的绝对路径（无论从哪里调用都能正确定位）
# Get the absolute path of the directory containing this script (works regardless of CWD)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# 项目根目录（console 的上一级）
# Project root directory (parent of console/)
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 解析命令行参数：遍历所有参数，识别 --rebuild 标志
# Parse CLI arguments: iterate all args, recognize --rebuild flag
REBUILD=false
for arg in "$@"; do
    case "$arg" in
        --rebuild) REBUILD=true ;;  # 强制重建标志 / force rebuild flag
    esac
done

# 虚拟环境路径配置
# Virtual environment path configuration
AGENT_VENV="$PROJECT_ROOT/.venv"          # agent 主项目虚拟环境 / agent main project venv
BACKEND_VENV="$SCRIPT_DIR/backend/.venv"  # 控制台后端独立虚拟环境 / console backend isolated venv

# 服务 URL 配置
# Service URL configuration
AGENT_URL="http://127.0.0.1:8079"    # privacy_local_agent REST 接口 / agent REST API
CONSOLE_URL="http://127.0.0.1:8080"  # 控制台后端（提供 UI + API）/ console backend (serves UI + API)

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
# Port availability pre-check (auto-diagnose and offer kill option on conflict)
#
# 参数 / Parameters: $1=端口号/port, $2=服务名/service name
# 逻辑 / Logic:
#   1. TCP connect 尝试连接端口，连接成功→被占用 / Try connect; success = occupied
#   2. 占用时用 lsof/ss/fuser 诊断占用进程 / On occupied, diagnose with lsof/ss/fuser
#   3. 交互式询问是否终止 / Interactively ask whether to kill
#   4. 终止后再次验证 / Re-verify after kill
check_port_available() {
    local port="$1"  # 目标端口 / target port
    local name="$2"  # 服务名称 / service name

    # 快速检测端口是否可用（connect 方式，比 bind 更准确）
    if ! _is_port_in_use "$port"; then
        return 0  # 端口可用，直接返回 / port available, return immediately
    fi

    # 端口被占用 —— 诊断占用进程
    # Port is occupied — diagnose the occupying process
    echo ""
    echo "⚠️  端口 $port 已被占用（$name）"
    echo "────────────────────────────────────────"

    local pids=""
    # 优先使用 lsof 诊断（macOS/Linux 通用）/ Prefer lsof (macOS/Linux universal)
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
    elif command -v fuser >/dev/null 2>&1; then  # 回退到 fuser / fallback to fuser
        pids=$(fuser "$port"/tcp 2>/dev/null | tr -s ' ')
        echo "占用进程 PID：$pids"
        echo ""
    fi

    # 无法定位占用进程时报错退出 / Exit with error if cannot locate process
    if [[ -z "$pids" ]]; then
        echo "错误：无法定位占用端口 $port 的进程，请手动排查："
        echo "  lsof -i :$port"
        echo "  或 ss -tlnp | grep $port"
        exit 1
    fi

    echo "占用端口 $port 的进程 PID：$pids"  # PIDs occupying port $port
    echo ""
    # 交互式询问用户是否自动终止 / Interactively ask user whether to auto-kill
    read -rp "是否自动终止上述进程以释放端口？[y/N] " answer
    case "$answer" in
        [yY]|[yY][eE][sS])
            # 用户确认，逐个强制终止 / User confirmed, force-kill each
            for pid in $pids; do
                echo "  → kill -9 $pid"
                kill -9 "$pid" 2>/dev/null || true
            done
            sleep 1  # 等待 OS 释放端口 / wait for OS to release port
            # 再次验证端口已释放（connect 方式）
            if ! _is_port_in_use "$port"; then
                echo "✅ 端口 $port 已释放"
            else
                echo "错误：端口 $port 仍被占用，请手动排查。"
                exit 1
            fi
            ;;
        *)
            # 用户拒绝，提示手动处理 / User declined, prompt manual handling
            echo "已取消。请手动释放端口 $port 后重试："
            echo "  kill -9 $pids"
            exit 1
            ;;
    esac
}

# ── 自动补全缺失的依赖 / 构建产物 ─────────────────────────────────────

# 1. Agent 虚拟环境：缺失或 --rebuild 时自动创建并安装项目依赖
# 1. Agent venv: auto-create and install project deps when missing or --rebuild
# 这里使用项目根目录的 `.venv`，让控制台脚本和主包共享同一套运行时依赖，
# Uses project root `.venv` so console scripts and main package share runtime deps,
# 避免重复安装和版本漂移。
# avoiding duplicate installs and version drift.
if [[ ! -d "$AGENT_VENV" ]]; then
    echo "未找到 agent 虚拟环境，自动创建并安装依赖：$AGENT_VENV"  # Agent venv not found, auto-creating
    python3 -m venv "$AGENT_VENV"  # 创建虚拟环境 / create virtual environment
    (
        source "$AGENT_VENV/bin/activate"  # 激活虚拟环境 / activate venv
        cd "$PROJECT_ROOT"
        pip install --upgrade pip >/dev/null  # 升级 pip / upgrade pip
        pip install -e .  # 以可编辑模式安装主项目 / install main project in editable mode
    )
    echo "agent 依赖安装完成。"  # Agent deps installed.
elif [[ "$REBUILD" == true ]]; then
    echo "--rebuild：重新安装 agent 依赖..."  # --rebuild: reinstalling agent deps...
    (
        source "$AGENT_VENV/bin/activate"
        cd "$PROJECT_ROOT"
        pip install -e .  # 重新安装 / reinstall
    )
    echo "agent 依赖重装完成。"  # Agent deps reinstalled.
fi

# 2. 控制台后端虚拟环境：缺失或 --rebuild 时自动创建并安装依赖
# 2. Console backend venv: auto-create and install deps when missing or --rebuild
# 控制台后端保留独立虚拟环境，方便它和主 agent 分开升级、调试和回滚。
# Console backend keeps an isolated venv for independent upgrade, debug and rollback.
if [[ ! -d "$BACKEND_VENV" ]]; then
    echo "未找到后端虚拟环境，自动创建并安装依赖：$BACKEND_VENV"  # Backend venv not found, auto-creating
    python3 -m venv "$BACKEND_VENV"  # 创建虚拟环境 / create venv
    (
        source "$BACKEND_VENV/bin/activate"  # 激活 / activate
        pip install --upgrade pip >/dev/null  # 升级 pip / upgrade pip
        pip install -r "$SCRIPT_DIR/backend/requirements.txt"  # 安装后端依赖 / install backend deps
    )
    echo "后端依赖安装完成。"  # Backend deps installed.
elif [[ "$REBUILD" == true ]]; then
    echo "--rebuild：重新安装控制台后端依赖..."  # --rebuild: reinstalling console backend deps...
    (
        source "$BACKEND_VENV/bin/activate"
        pip install -r "$SCRIPT_DIR/backend/requirements.txt"  # 重新安装 / reinstall
    )
    echo "后端依赖重装完成。"  # Backend deps reinstalled.
fi

_build_frontend() {
    (
        cd "$SCRIPT_DIR/web"
        if command -v pnpm >/dev/null 2>&1; then
            if [[ -d "node_modules" ]] && pnpm build 2>/dev/null; then
                return 0
            fi
            (pnpm install --prefer-offline 2>/dev/null || pnpm install) && pnpm build
        elif command -v npm >/dev/null 2>&1; then
            if [[ -d "node_modules" ]] && npm run build 2>/dev/null; then
                return 0
            fi
            npm install && npm run build
        else
            echo "警告：未找到 pnpm/npm，跳过前端构建，控制台将以 API 模式运行。"
        fi
    )
}

_frontend_is_stale() {
    local marker="$SCRIPT_DIR/web/dist/index.html"
    [[ -f "$marker" ]] || return 0
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
    echo "--rebuild：删除旧的前端构建产物并重新构建..."  # --rebuild: removing old frontend dist and rebuilding...
    rm -rf "$SCRIPT_DIR/web/dist"  # 删除旧产物 / remove old artifacts
fi
if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "未找到前端构建产物，自动构建：$SCRIPT_DIR/web/dist"  # Frontend dist not found, auto-building
    _build_frontend
elif [[ "$REBUILD" == true ]]; then
    echo "强制重新构建前端..."
    _build_frontend
elif _frontend_is_stale; then
    echo "ℹ️  提示：检测到前端源码比静态产物新。如需重新打包 UI 请加 --rebuild 参数，或在 console/web 运行 pnpm dev。"
fi

if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo "警告：前端构建产物 $SCRIPT_DIR/web/dist 不存在，后端将以 API 模式运行。"  # Warning: frontend dist missing, backend runs in API-only mode
    echo "如需完整 UI，请执行：cd $SCRIPT_DIR/web && corepack pnpm install && corepack pnpm build"  # For full UI, run: ...
fi

# PID 文件路径配置（用于 stop.sh 停止服务）
# PID file path configuration (used by stop.sh to stop services)
AGENT_PID_FILE="$SCRIPT_DIR/.pids/agent.pid"      # agent 主服务 PID / main agent PID
CONSOLE_PID_FILE="$SCRIPT_DIR/.pids/console.pid"  # 控制台后端 PID / console backend PID

mkdir -p "$SCRIPT_DIR/.pids"  # 确保 PID 目录存在 / ensure PID directory exists

# write_pid - 将进程 ID 写入指定文件
# Write process ID to the specified file
write_pid() {
    local file="$1"  # 目标文件 / target file
    local pid="$2"   # 进程 ID / process ID
    echo "$pid" > "$file"  # 写入 PID / write PID
}

# 清理子进程（Ctrl+C 或脚本退出时自动触发）
# Cleanup child processes (auto-triggered on Ctrl+C or script exit)
PIDS=()  # 存储所有后台进程 PID / store all background process PIDs
cleanup() {
    echo ""
    echo "正在停止服务..."  # Stopping services...
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true  # 终止各子进程 / kill each child process
    done
    wait 2>/dev/null || true  # 等待所有子进程结束 / wait for all children to exit
    rm -f "$AGENT_PID_FILE" "$CONSOLE_PID_FILE"  # 清理 PID 文件 / clean up PID files
    echo "已停止。"  # Stopped.
}
# 注册信号处理：INT(Ctrl+C)、TERM、EXIT 时触发 cleanup
# Register signal handlers: trigger cleanup on INT(Ctrl+C), TERM, EXIT
trap cleanup INT TERM EXIT

# 端口预检：确保所需端口未被占用
# Port pre-check: ensure required ports are not occupied
check_port_available 8079 "privacy_local_agent REST"  # agent REST 端口 / agent REST port
check_port_available 8080 "Python REST 代理后端"  # 控制台后端端口 / console backend port

# 启动 privacy_local_agent
# Launch privacy_local_agent
# 先启动主 agent，再启动控制台后端；后者会通过 REST 访问前者，所以顺序不能反过来。
# Start agent first, then console backend; the latter accesses the former via REST, so order matters.
echo "启动 privacy_local_agent (REST: $AGENT_URL)..."  # Starting privacy_local_agent...
(
    source "$AGENT_VENV/bin/activate"  # 激活 agent 虚拟环境 / activate agent venv
    cd "$PROJECT_ROOT"
    exec python -m privacy_local_agent.server  # 启动 REST+gRPC 服务 / start REST+gRPC server
) &
AGENT_PID=$!  # 获取后台进程 PID / get background process PID
PIDS+=("$AGENT_PID")  # 加入清理列表 / add to cleanup list
write_pid "$AGENT_PID_FILE" "$AGENT_PID"  # 写入 PID 文件 / write PID file

# 启动控制台后端
# Launch console backend
# 控制台后端提供给 Web UI 和 smoke test 使用的 API，因此必须在提示页面前完成启动。
# Console backend provides APIs for Web UI and smoke test, must be ready before showing URLs.
echo "启动测试控制台后端 (Console: $CONSOLE_URL)..."  # Starting console backend...
(
    source "$BACKEND_VENV/bin/activate"  # 激活后端虚拟环境 / activate backend venv
    cd "$SCRIPT_DIR/backend"
    exec uvicorn app.main:app --host 127.0.0.1 --port 8080  # 启动 Uvicorn ASGI 服务器 / start Uvicorn ASGI server
) &
CONSOLE_PID=$!  # 获取后台进程 PID / get background process PID
PIDS+=("$CONSOLE_PID")  # 加入清理列表 / add to cleanup list
write_pid "$CONSOLE_PID_FILE" "$CONSOLE_PID"  # 写入 PID 文件 / write PID file

# 等待服务就绪
# Wait for services to be ready
# 这里轮询 health 接口，避免脚本“进程已启动但服务还没 ready”时误导用户。
# Polls health endpoints to avoid misleading users when process started but service not yet ready.
# 只有主 agent 和控制台后端都通过健康检查后，才会打印可访问地址。
# Only prints access URLs after both agent and console backend pass health checks.
#
# 参数 / Parameters: $1=健康检查 URL / health check URL, $2=服务名 / service name
# 逻辑 / Logic: 最多轮询 30 次，每次间隔 1 秒，超时则返回 1
#              Poll up to 30 times at 1s intervals, return 1 on timeout
wait_for_service() {
    local url="$1"   # 健康检查地址 / health check URL
    local name="$2"  # 服务名称 / service name
    local max_attempts=30  # 最大尝试次数 / max retry attempts
    local attempt=0        # 当前尝试计数 / current attempt counter
    echo -n "等待 $name 就绪"  # Waiting for $name to be ready
    while [[ $attempt -lt $max_attempts ]]; do
        if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q '^200$'; then  # HTTP 200 = 就绪 / ready
            echo " OK"
            return 0  # 服务就绪 / service ready
        fi
        echo -n "."  # 进度指示 / progress indicator
        sleep 1      # 等待 1 秒后重试 / wait 1s before retry
        attempt=$((attempt + 1))  # 递增计数器 / increment counter
    done
    echo " 超时"  # Timeout
    return 1
}

# 分别等待两个服务就绪 / Wait for both services to be ready
wait_for_service "$AGENT_URL/health" "privacy_local_agent"
wait_for_service "$CONSOLE_URL/api/health" "测试控制台后端"

# 打印启动成功信息和访问地址
# Print startup success info and access URLs
echo ""
echo "======================================"
echo "隐私测试控制台已启动"  # Privacy test console started
echo "Agent REST:  $AGENT_URL"
echo "Console UI:  $CONSOLE_URL"
if [[ ! -d "$SCRIPT_DIR/web/dist" ]]; then
    echo ""
    echo "注意：前端尚未构建，访问 $CONSOLE_URL 将显示 {\"detail\":\"Not Found\"}。"  # Note: frontend not built yet
    echo "请先构建前端：cd $SCRIPT_DIR/web && corepack pnpm install && corepack pnpm build"  # Build frontend first
    echo "构建完成后重新执行 ./console/start.sh 即可打开 Console UI。"  # Re-run start.sh after build
fi
echo "按 Ctrl+C 停止所有服务"  # Press Ctrl+C to stop all services
echo "======================================"

# 保持脚本运行（等待所有后台进程，直到用户 Ctrl+C）
# Keep script running (wait for all background processes until user Ctrl+C)
wait
