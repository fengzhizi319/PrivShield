#!/usr/bin/env bash
# ============================================================================
# 【生产模式】单组分独立启动 PrivShield 核心 Agent 容器
# Launch Standalone PrivShield Core/ML Agent in Production Docker Container
#
# 执行步骤总览：
#   1. 解析命令行参数（core/ml 目标、端口映射、--env-file、--no-build）
#   2. 前置检查：Docker CLI 与 Docker Daemon 连通性
#   3. 创建并校验宿主机持久化目录（.data/budget、.logs）
#   4. 根据 core/ml 目标构建生产级 Docker 镜像
#   5. 组装生产环境变量与数据卷/配置/证书/密钥挂载点
#   6. 停止并清理旧有的生产同名容器（避免端口与名称冲突）
#   7. 以生产级加固参数（restart=always, no-new-privileges, 资源限制）启动容器
#   8. 轮询等待核心 Agent 就绪探针（GET /readyz 返回 200）
#
# 用法 / Usage:
#   ./scripts/prod/docker-start-agent.sh [core|ml] [选项]
#
# 参数 / Arguments:
#   core (默认)      使用轻量 Core 镜像 (纯 CPU / FastAPI / gRPC / 规则引擎)
#   ml               使用完整 ML 镜像 (含 PyTorch / Transformers / ONNX Runtime)
#
# 选项 / Options:
#   -p, --rest-port PORT   REST API 宿主机映射端口 (默认: 8079)
#   -g, --grpc-port PORT   gRPC RPC 宿主机映射端口 (默认: 50051)
#   --env-file FILE        指定环境变量文件 (默认优先检测 .env)
#   --no-build             跳过镜像构建，直接运行本地已有镜像
#   -h, --help             显示帮助信息
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

TARGET="core"
REST_PORT="8079"
GRPC_PORT="50051"
ENV_FILE=""
NO_BUILD=false

# ── 1. 参数解析 ──
POSITIONAL_ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        core|ml)
            TARGET="$1"
            shift 1
            ;;
        -p|--rest-port)
            REST_PORT="$2"
            shift 2
            ;;
        -g|--grpc-port)
            GRPC_PORT="$2"
            shift 2
            ;;
        --env-file)
            ENV_FILE="$2"
            shift 2
            ;;
        --no-build)
            NO_BUILD=true
            shift 1
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [core|ml] [选项]"
            echo ""
            echo "构建目标 / Targets:"
            echo "  core   (默认) 轻量 Core 镜像（仅含 FastAPI/gRPC 基础依赖）"
            echo "  ml     完整 ML 镜像（包含 PyTorch/Transformers/ONNX 等重量级依赖）"
            echo ""
            echo "选项 / Options:"
            echo "  -p, --rest-port PORT   REST API 宿主机监听端口 (默认: 8079)"
            echo "  -g, --grpc-port PORT   gRPC RPC 宿主机监听端口 (默认: 50051)"
            echo "  --env-file FILE        环境变量文件路径 (默认: .env 或 deploy/docker-compose/.env)"
            echo "  --no-build             跳过镜像构建，直接使用本地已有镜像"
            echo "  -h, --help             显示帮助信息并退出"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $1" >&2
            echo "   请运行 $0 --help 查看使用帮助" >&2
            exit 1
            ;;
    esac
done

# ── 2. Docker 环境与 Daemon 连通性前置检查 ──
if ! command -v docker >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 docker 命令，请先安装 Docker: https://docs.docker.com/get-docker/" >&2
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    echo "❌ [错误] 无法连接到 Docker 守护进程，请确认 Docker 正在运行！" >&2
    exit 1
fi

echo "============================================================================"
echo "🛡️  【生产模式】独立启动 PrivShield 核心 Agent 容器"
echo "   • 镜像类型 : $TARGET"
echo "   • REST 端口 : $REST_PORT"
echo "   • gRPC 端口 : $GRPC_PORT"
echo "============================================================================"

cd "$PROJECT_ROOT"

# ── 3. 准备持久化目录 ──
mkdir -p "$PROJECT_ROOT/.data/budget" "$PROJECT_ROOT/.logs"
chmod 755 "$PROJECT_ROOT/.data/budget" "$PROJECT_ROOT/.logs"

# ── 4. 镜像构建（若未指定 --no-build）──
if [[ "$TARGET" == "ml" ]]; then
    IMAGE_NAME="privshield:1.8.0-ml"
    RESOURCE_LIMITS=(--cpus="4.0" --memory="8g")
    if [[ "$NO_BUILD" != "true" ]]; then
        echo "📦 正在构建生产级 ML 镜像 ($IMAGE_NAME)..."
        docker build --target ml -t "$IMAGE_NAME" .
    fi
else
    IMAGE_NAME="privshield:1.8.0"
    RESOURCE_LIMITS=(--cpus="2.0" --memory="2g")
    if [[ "$NO_BUILD" != "true" ]]; then
        echo "📦 正在构建生产级 Core 镜像 ($IMAGE_NAME)..."
        docker build --target core -t "$IMAGE_NAME" .
    fi
fi

# ── 5. 组装环境变量文件与卷挂载 ──
ENV_FLAGS=()
if [[ -n "$ENV_FILE" && -f "$ENV_FILE" ]]; then
    ENV_FLAGS+=(--env-file "$ENV_FILE")
    echo "   • 环境变量 : $ENV_FILE"
elif [[ -f "$PROJECT_ROOT/deploy/docker-compose/.env" ]]; then
    ENV_FLAGS+=(--env-file "$PROJECT_ROOT/deploy/docker-compose/.env")
    echo "   • 环境变量 : deploy/docker-compose/.env"
elif [[ -f "$PROJECT_ROOT/.env" ]]; then
    ENV_FLAGS+=(--env-file "$PROJECT_ROOT/.env")
    echo "   • 环境变量 : .env"
fi

VOLUME_MOUNTS=(
    -v "$PROJECT_ROOT/.data/budget:/data/budget"
    -v "$PROJECT_ROOT/.logs:/var/log/privacy"
)

# 挂载 privacy-profile.yaml（优先使用 compose 目录下的 profile）
if [[ -f "$PROJECT_ROOT/deploy/docker-compose/privacy-profile.yaml" ]]; then
    VOLUME_MOUNTS+=(-v "$PROJECT_ROOT/deploy/docker-compose/privacy-profile.yaml:/etc/PrivShield/privacy-profile.yaml:ro")
elif [[ -f "$PROJECT_ROOT/config/sample-privacy-profile.yaml" ]]; then
    VOLUME_MOUNTS+=(-v "$PROJECT_ROOT/config/sample-privacy-profile.yaml:/etc/PrivShield/privacy-profile.yaml:ro")
fi

# 挂载证书目录（如果存在）
if [[ -d "$PROJECT_ROOT/deploy/docker-compose/certs" ]]; then
    VOLUME_MOUNTS+=(-v "$PROJECT_ROOT/deploy/docker-compose/certs:/certs:ro")
elif [[ -d "$PROJECT_ROOT/certs" ]]; then
    VOLUME_MOUNTS+=(-v "$PROJECT_ROOT/certs:/certs:ro")
fi

# 挂载密钥目录（如果存在）
if [[ -d "$PROJECT_ROOT/deploy/docker-compose/secrets" ]]; then
    VOLUME_MOUNTS+=(-v "$PROJECT_ROOT/deploy/docker-compose/secrets:/etc/PrivShield/secrets:ro")
elif [[ -d "$PROJECT_ROOT/secrets" ]]; then
    VOLUME_MOUNTS+=(-v "$PROJECT_ROOT/secrets:/etc/PrivShield/secrets:ro")
fi

# ── 6. 停止并清理旧容器 ──
CONTAINER_NAME="PrivShield-prod-agent"
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# ── 7. 启动生产级容器 ──
echo ""
echo "🚀 正在启动生产级 Agent 物理容器 ($CONTAINER_NAME)..."
docker run -d \
    --name "$CONTAINER_NAME" \
    --restart=always \
    --security-opt=no-new-privileges:true \
    "${RESOURCE_LIMITS[@]}" \
    -p "${REST_PORT}:8079" \
    -p "${GRPC_PORT}:50051" \
    -e PRIVACY_REST_HOST="0.0.0.0" \
    -e PRIVACY_REST_PORT="8079" \
    -e PRIVACY_GRPC_HOST="0.0.0.0" \
    -e PRIVACY_GRPC_PORT="50051" \
    -e PRIVACY_LOG_FORMAT="json" \
    -e PRIVACY_PROFILE="/etc/PrivShield/privacy-profile.yaml" \
    -e PRIVACY_BUDGET_DB="/data/budget/budget.db" \
    "${ENV_FLAGS[@]}" \
    "${VOLUME_MOUNTS[@]}" \
    "$IMAGE_NAME"

# ── 8. 等待服务就绪探针 (/readyz) ──
echo ""
echo -n "⏳ 等待 PrivShield Agent 就绪探针响应..."
MAX_ATTEMPTS=30
ATTEMPT=0
READY=false

while [[ $ATTEMPT -lt $MAX_ATTEMPTS ]]; do
    if curl -s -k -o /dev/null -w "%{http_code}" "https://127.0.0.1:${REST_PORT}/readyz" 2>/dev/null | grep -q '^200$' || \
       curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${REST_PORT}/readyz" 2>/dev/null | grep -q '^200$'; then
        READY=true
        break
    fi
    echo -n "."
    sleep 1
    ATTEMPT=$((ATTEMPT + 1))
done

if [[ "$READY" == "true" ]]; then
    echo " ✅ 已就绪"
else
    echo " ⚠️  就绪检查等待超时，请使用 'docker logs $CONTAINER_NAME' 查看容器运行状态。"
fi

echo ""
echo "============================================================================"
echo "🎉 PrivShield 生产级 Agent 已成功启动！"
echo "============================================================================"
echo "  • REST API  : http://127.0.0.1:${REST_PORT} (或 https://127.0.0.1:${REST_PORT})"
echo "  • gRPC RPC  : 127.0.0.1:${GRPC_PORT}"
echo "  • 容器名称  : $CONTAINER_NAME"
echo "  • 日志查看  : docker logs -f $CONTAINER_NAME"
echo "  • 容器停止  : ./scripts/prod/docker-stop-agent.sh"
echo "============================================================================"
