#!/usr/bin/env bash
# ============================================================================
# 一键启动 mTLS 模式的 Go gRPC 代理控制台。
# One-click launch of Go gRPC proxy console in mTLS mode.
#
# 与 start-go.sh 的区别 / Differences from start-go.sh:
#   - agent 的 gRPC 服务端启用 mTLS（PRIVACY_TLS_CLIENT_AUTH=require，要求客户端证书）
#     Agent gRPC server enables mTLS (PRIVACY_TLS_CLIENT_AUTH=require, requires client cert)
#   - Go 代理的 gRPC 客户端启用 mTLS（出示客户端证书并校验服务端证书）
#     Go proxy gRPC client enables mTLS (presents client cert and verifies server cert)
#   - 若证书缺失，自动调用 backend-go/scripts/gen-certs.sh 生成一套自签名测试证书
#     If certs are missing, auto-generates self-signed test certs via gen-certs.sh
#
# 用法 / Usage:
#   ./console/start-go-mtls.sh [--rebuild]
#
# 说明 / Notes:
#   本脚本面向本地测试/联调，使用自签名证书。生产环境请使用正式 CA 签发的证书，
#   This script is for local testing/debugging with self-signed certs. For production, use CA-issued certs
#   并通过环境变量显式指定各证书路径（参见 backend-go/docs/ops.md）。
#   and explicitly specify cert paths via env vars (see backend-go/docs/ops.md).
# ============================================================================

# 启用严格模式 / Enable strict mode
set -euo pipefail

# 获取脚本目录和项目根目录 / Get script dir and project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 解析参数 / Parse arguments
REBUILD=false
for arg in "$@"; do
    case "$arg" in
        --rebuild) REBUILD=true ;;  # 强制重建 / force rebuild
    esac
done

# 路径配置 / Path configuration
AGENT_VENV="$PROJECT_ROOT/.venv"                    # agent venv
CERT_DIR="$SCRIPT_DIR/backend-go/certs"             # 证书目录 / certificate directory
GEN_CERTS="$SCRIPT_DIR/backend-go/scripts/gen-certs.sh"  # 证书生成脚本 / cert generation script

# mTLS 模式下 Go 代理控制台仍为 HTTP（仅代理到 agent 的 gRPC 链路为 mTLS）
# In mTLS mode, Go proxy console is still HTTP (only the gRPC link to agent uses mTLS)
CONSOLE_URL="http://127.0.0.1:8081"      # Go 代理 HTTP 地址 / Go proxy HTTP address
AGENT_GRPC_ADDR="127.0.0.1:50051"        # agent gRPC 地址 / agent gRPC address

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

# ── 1. 准备证书 / Prepare certificates ───────────────────────────────────
# 检查 CA、服务端、客户端证书是否存在，缺失则自动生成
# Check if CA, server, client certs exist; auto-generate if missing
if [[ ! -f "$CERT_DIR/ca.crt" || ! -f "$CERT_DIR/server.crt" || ! -f "$CERT_DIR/client.crt" ]]; then
    echo "未找到 mTLS 证书，自动生成测试证书链..."  # mTLS certs not found, auto-generating...
    bash "$GEN_CERTS" "$CERT_DIR"  # 调用证书生成脚本 / invoke cert generation script
else
    echo "复用已有 mTLS 证书：$CERT_DIR"  # Reusing existing mTLS certs
fi

# ── 2. 准备 agent 虚拟环境 / Prepare agent venv ────────────────────────────
if [[ ! -d "$AGENT_VENV" ]]; then
    echo "未找到 agent 虚拟环境，自动创建并安装依赖：$AGENT_VENV"  # Agent venv not found, creating
    python3 -m venv "$AGENT_VENV"  # 创建 venv / create venv
    (
        source "$AGENT_VENV/bin/activate"  # 激活 / activate
        cd "$PROJECT_ROOT"
        pip install --upgrade pip >/dev/null  # 升级 pip / upgrade pip
        pip install -e .  # 可编辑安装 / editable install
    )
fi

# Go 工具链检查 / Go toolchain check
if ! command -v go >/dev/null 2>&1; then
    echo "错误：未找到 Go 工具链，请先安装 Go。"  # Error: Go toolchain not found
    exit 1
fi

# ── 3. 编译 Go 代理 / Compile Go proxy ───────────────────────────────────
echo "编译 Go gRPC 代理后端..."  # Compiling Go gRPC proxy backend...
(cd "$SCRIPT_DIR/backend-go" && go build -o bin/backend-go ./cmd/server)  # 编译 / compile

# ── 4. 启动服务 / Start services ───────────────────────────────────────────
mkdir -p "$SCRIPT_DIR/.pids"  # 确保 PID 目录存在 / ensure PID dir exists
AGENT_PID_FILE="$SCRIPT_DIR/.pids/agent-go-mtls.pid"    # agent PID 文件 / agent PID file
CONSOLE_PID_FILE="$SCRIPT_DIR/.pids/console-go-mtls.pid" # Go 代理 PID 文件 / Go proxy PID file

# 清理子进程 / Cleanup child processes
PIDS=()
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
check_port_available 8079 "privacy_local_agent REST"         # agent REST
check_port_available 50051 "privacy_local_agent gRPC (mTLS)"  # agent gRPC mTLS
check_port_available 8081 "Go gRPC 代理后端"  # Go gRPC proxy

# 4.1 启动 agent（gRPC 服务端启用 mTLS，要求客户端证书）
# 4.1 Launch agent (gRPC server enables mTLS, requires client cert)
echo "启动 privacy_local_agent (gRPC mTLS: $AGENT_GRPC_ADDR, client_auth=require)..."
(
    source "$AGENT_VENV/bin/activate"  # 激活 venv / activate venv
    cd "$PROJECT_ROOT"
    export PRIVACY_TLS_ENABLED=true              # 启用 TLS / enable TLS
    export PRIVACY_TLS_CERT_FILE="$CERT_DIR/server.crt"  # 服务端证书 / server cert
    export PRIVACY_TLS_KEY_FILE="$CERT_DIR/server.key"   # 服务端私钥 / server key
    export PRIVACY_TLS_CA_FILE="$CERT_DIR/ca.crt"        # CA 证书 / CA cert
    export PRIVACY_TLS_CLIENT_AUTH=require               # 要求客户端证书 / require client cert
    exec python -m privacy_local_agent.server  # 启动服务 / start server
) &
AGENT_PID=$!  # 获取 PID / get PID
PIDS+=("$AGENT_PID")
echo "$AGENT_PID" > "$AGENT_PID_FILE"  # 写入 PID 文件 / write PID file

# 4.2 启动 Go 代理（gRPC 客户端启用 mTLS，出示客户端证书）
# 4.2 Launch Go proxy (gRPC client enables mTLS, presents client cert)
echo "启动 Go gRPC 代理后端 (mTLS -> $AGENT_GRPC_ADDR, Console: $CONSOLE_URL)..."
(
    cd "$SCRIPT_DIR/backend-go"
    export PRIVACY_AGENT_TLS_ENABLED=true              # 启用 TLS / enable TLS
    export PRIVACY_AGENT_TLS_CERT_FILE="$CERT_DIR/client.crt"  # 客户端证书 / client cert
    export PRIVACY_AGENT_TLS_KEY_FILE="$CERT_DIR/client.key"   # 客户端私钥 / client key
    export PRIVACY_AGENT_TLS_CA_FILE="$CERT_DIR/ca.crt"        # CA 证书 / CA cert
    # 连接目标为 127.0.0.1，但证书 SAN 含 localhost，覆盖校验主机名
    # Target is 127.0.0.1 but cert SAN contains localhost, override hostname verification
    export PRIVACY_AGENT_TLS_SERVER_NAME=localhost
    exec ./bin/backend-go  # 运行二进制 / run binary
) &
CONSOLE_PID=$!
PIDS+=("$CONSOLE_PID")
echo "$CONSOLE_PID" > "$CONSOLE_PID_FILE"  # 写入 PID 文件 / write PID file

# ── 5. 等待就绪 / Wait for ready ───────────────────────────────────────────
echo -n "等待 Go 代理就绪"  # Waiting for Go proxy to be ready
for _ in $(seq 1 30); do  # 最多轮询 30 次 / poll up to 30 times
    if curl -s -o /dev/null -w "%{http_code}" "$CONSOLE_URL/api/health" | grep -q '^200$'; then
        echo " OK"
        break
    fi
    echo -n "."  # 进度 / progress
    sleep 1
done

# 打印启动信息 / Print startup info
echo ""
echo "======================================"
echo "Go gRPC 代理控制台已启动（mTLS 模式）"  # Go gRPC proxy console started (mTLS mode)
echo "Agent gRPC:  $AGENT_GRPC_ADDR (mTLS, 要求客户端证书)"  # requires client cert
echo "Console UI:  $CONSOLE_URL (Go 后端 -> agent 全程 mTLS)"  # full mTLS link
echo "证书目录:    $CERT_DIR"  # Cert directory
echo "按 Ctrl+C 停止所有服务"  # Press Ctrl+C to stop all services
echo "======================================"

# 保持脚本运行 / Keep script running
wait
