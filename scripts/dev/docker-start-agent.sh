#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】单组分启动 Privacy Local Agent
# Launch Privacy Local Agent in Docker container
#
# 用法 / Usage: ./scripts/dev/docker-start-agent.sh [core|ml]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET="${1:-core}"

echo "============================================================================"
echo "🚀 [Docker Mode] 正在构建并启动 Privacy Local Agent (Target: $TARGET)..."
echo "============================================================================"

cd "$PROJECT_ROOT"

if [[ "$TARGET" == "ml" ]]; then
    echo "📦 构建含有 PyTorch / Transformers / ONNX 的 ML 镜像..."
    docker build --target ml -t privacy-local-agent:0.1.0-ml .
    IMAGE_NAME="privacy-local-agent:0.1.0-ml"
else
    echo "📦 构建轻量 Core 镜像..."
    docker build --target core -t privacy-local-agent:0.1.0 .
    IMAGE_NAME="privacy-local-agent:0.1.0"
fi

# 停止旧容器
docker rm -f privacy-local-agent 2>/dev/null || true

# 启动容器
docker run -d \
  --name privacy-local-agent \
  -p 8079:8079 \
  -p 50051:50051 \
  -e PRIVACY_REST_HOST="0.0.0.0" \
  -e PRIVACY_GRPC_HOST="0.0.0.0" \
  -e PRIVACY_LOG_LEVEL="INFO" \
  "$IMAGE_NAME"

echo ""
echo "✅ Privacy Local Agent (Docker) 已成功启动！"
echo "   - REST API : http://127.0.0.1:8079"
echo "   - gRPC RPC : 127.0.0.1:50051"
echo "   - 查看日志 : docker logs -f privacy-local-agent"
echo "============================================================================"
