#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】单组分启动 PrivShield (支持 Linux / macOS / Windows WSL2)
# Launch PrivShield in Docker container (Supports Linux / macOS / Windows WSL2)
#
# 用法 / Usage: ./scripts/dev/docker-start-agent.sh [core|ml]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET="${1:-core}"

# ── 1. 帮助信息与参数合法性白名单校验 ──
if [[ "$TARGET" == "-h" || "$TARGET" == "--help" ]]; then
    echo "用法 / Usage: $0 [core|ml]"
    echo ""
    echo "构建目标 / Targets:"
    echo "  core   (默认) 构建并启动轻量 Core 镜像（仅含 FastAPI/gRPC 基础依赖）"
    echo "  ml     构建并启动完整 ML 镜像（包含 PyTorch/Transformers/ONNX 等重量级依赖）"
    echo ""
    echo "跨平台支持 / Cross-Platform:"
    echo "  - Linux: x86_64 / aarch64 原生 Docker Engine"
    echo "  - macOS: Apple Silicon (M1/M2/M3/M4) / Intel (Docker Desktop)"
    echo "  - Windows 11: WSL2 / Git Bash (或直接运行 scripts/dev/docker-start-agent.ps1)"
    exit 0
fi

if [[ "$TARGET" != "core" && "$TARGET" != "ml" ]]; then
    echo "❌ [错误] 无效的构建目标: '$TARGET'" >&2
    echo "   用法: $0 [core|ml]" >&2
    exit 1
fi

# ── 2. 操作系统与平台环境自动识别 ──
OS_TYPE="$(uname -s 2>/dev/null || echo "Unknown")"
ARCH_TYPE="$(uname -m 2>/dev/null || echo "Unknown")"
case "$OS_TYPE" in
    Darwin*)
        PLATFORM_NAME="macOS ($ARCH_TYPE, Docker Desktop)"
        ;;
    Linux*)
        if grep -qi "microsoft" /proc/version 2>/dev/null; then
            PLATFORM_NAME="Windows 11 (WSL2 Linux, $ARCH_TYPE)"
        else
            PLATFORM_NAME="Linux ($ARCH_TYPE, Docker Engine)"
        fi
        ;;
    MINGW*|MSYS*|CYGWIN*)
        PLATFORM_NAME="Windows 11 ($OS_TYPE, $ARCH_TYPE)"
        ;;
    *)
        PLATFORM_NAME="$OS_TYPE ($ARCH_TYPE)"
        ;;
esac

# ── 3. Docker 环境与 Daemon 连通性前置检查 ──
if ! command -v docker >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 docker 命令，请先安装 Docker: https://docs.docker.com/get-docker/" >&2
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    echo "❌ [错误] 无法连接到 Docker 守护进程，请确认 Docker Engine 或 Docker Desktop 正在运行！" >&2
    exit 1
fi

echo "============================================================================"
echo "🚀 [Docker Mode] 正在构建并启动 PrivShield"
echo "   • 平台环境 : $PLATFORM_NAME"
echo "   • 构建目标 : $TARGET"
echo "============================================================================"

cd "$PROJECT_ROOT"

# ── 4. 镜像构建 ──
if [[ "$TARGET" == "ml" ]]; then
    echo "📦 构建含有 PyTorch / Transformers / ONNX 的 ML 镜像..."
    docker build --target ml -t privshield:0.1.0-ml .
    IMAGE_NAME="privshield:0.1.0-ml"
else
    echo "📦 构建轻量 Core 镜像..."
    docker build --target core -t privshield:0.1.0 .
    IMAGE_NAME="privshield:0.1.0"
fi

# ── 5. 停止并清理旧容器（防止名称冲突）──
docker rm -f PrivShield 2>/dev/null || true

# ── 6. 启动物理容器 ──
docker run -d \
  --name PrivShield \
  -p 8079:8079 \
  -p 50051:50051 \
  -e PRIVACY_REST_HOST="0.0.0.0" \
  -e PRIVACY_GRPC_HOST="0.0.0.0" \
  -e PRIVACY_LOG_LEVEL="INFO" \
  "$IMAGE_NAME"

echo ""
echo "✅ PrivShield (Docker) 已成功启动！"
echo "   - REST API : http://127.0.0.1:8079"
echo "   - gRPC RPC : 127.0.0.1:50051"
echo "   - 查看日志 : docker logs -f PrivShield"
echo "============================================================================"


