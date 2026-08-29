#!/usr/bin/env bash
# ============================================================================
# 【开发模式】单组分启动 PrivShield (支持 Linux / macOS / Windows WSL2)
# Launch PrivShield in Docker container (Supports Linux / macOS / Windows WSL2)
#
# 执行步骤总览：
#   1. 解析构建目标参数（core 或 ml 镜像）
#   2. 自动识别操作系统与 CPU 架构平台（Linux / macOS / Windows WSL2）
#   3. 前置检查：Docker CLI 与 Docker Daemon 连通性
#   4. 使用 Dockerfile 构建指定目标（core/ml）的镜像
#   5. 停止并清理旧有的开发同名容器（避免端口与名称冲突）
#   6. 启动开发容器并映射 REST (8079) 与 gRPC (50051) 端口
#
# 用法 / Usage: ./scripts/dev/docker-start-agent.sh [core|ml]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET="${1:-core}"

# ── 步骤 1：帮助信息与参数合法性白名单校验 ────────────────────────────────
if [[ "$TARGET" == "-h" || "$TARGET" == "--help" ]]; then
    echo "用法 / Usage: $0 [core|ml|go]"
    echo ""
    echo "构建目标 / Targets:"
    echo "  core   (默认) 构建并启动轻量 Core 镜像（仅含 FastAPI/gRPC 基础依赖）"
    echo "  ml     构建并启动完整 ML 镜像（包含 PyTorch/Transformers/ONNX 等重量级依赖）"
    echo "  go     构建并启动 Go 原生高性能引擎镜像（极轻量 ~15MB）"
    echo ""
    echo "跨平台支持 / Cross-Platform:"
    echo "  - Linux: x86_64 / aarch64 原生 Docker Engine"
    echo "  - macOS: Apple Silicon (M1/M2/M3/M4) / Intel (Docker Desktop)"
    echo "  - Windows 11: WSL2 / Git Bash (或直接运行 scripts/dev/docker-start-agent.ps1)"
    exit 0
fi

if [[ "$TARGET" != "core" && "$TARGET" != "ml" && "$TARGET" != "go" ]]; then
    echo "❌ [错误] 无效的构建目标: '$TARGET'" >&2
    echo "   用法: $0 [core|ml|go]" >&2
    exit 1
fi

# ── 步骤 2：操作系统与平台环境自动识别 ────────────────────────────────────
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

# ── 步骤 3：Docker 环境与 Daemon 连通性前置检查 ───────────────────────────
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

# ── 步骤 4：镜像构建 ──────────────────────────────────────────────────────
echo "📦 构建 PrivShield Go 原生高性能引擎镜像..."
docker build -t privshield:latest .
IMAGE_NAME="privshield:latest"

# ── 步骤 5：停止并清理旧容器（防止名称冲突）──────────────────────────────
docker rm -f PrivShield 2>/dev/null || true

HOST_REST_PORT="${PRIVACY_REST_PORT:-8079}"
HOST_GRPC_PORT="${PRIVACY_GRPC_PORT:-50051}"

# ── 步骤 6：启动物理容器 ──────────────────────────────────────────────────
docker run -d \
  --name PrivShield \
  -p "${HOST_REST_PORT}:8079" \
  -p "${HOST_GRPC_PORT}:50051" \
  -e PRIVACY_REST_HOST="0.0.0.0" \
  -e PRIVACY_GRPC_HOST="0.0.0.0" \
  -e PRIVACY_LOG_LEVEL="INFO" \
  "$IMAGE_NAME"

echo ""
echo "✅ PrivShield (Docker) 已成功启动！"
echo "   - REST API : http://127.0.0.1:${HOST_REST_PORT}"
echo "   - gRPC RPC : 127.0.0.1:${HOST_GRPC_PORT}"
echo "   - 查看日志 : docker logs -f PrivShield"
echo "============================================================================"


