#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动全栈服务（Agent + 双后端 + Web UI + 可选 vLLM）
# Launch Full Stack Container Suite in Docker Compose
#
# 用法 / Usage: ./console/scripts/docker-start-all.sh [--with-llm]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WITH_LLM="${1:-}"

echo "============================================================================"
echo "🌟 [Docker Mode] 正在启动 PrivShield 全栈容器套件..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"

if [[ "$WITH_LLM" == "--with-llm" ]]; then
    echo "🤖 同时启动 vLLM 大模型推理容器 (GPU)..."
    docker compose --profile llm up -d
else
    docker compose up -d
fi

echo ""
echo "✅ 全栈 Docker 容器服务已成功启动！"
echo "   - React 控制台 Web UI     : http://localhost:5173"
echo "   - Python 代理后端 REST API : http://localhost:8080"
echo "   - Go 代理后端 REST API     : http://localhost:8081"
echo "   - Privacy Agent REST      : http://localhost:8079"
echo "   - Privacy Agent gRPC      : localhost:50051"
if [[ "$WITH_LLM" == "--with-llm" ]]; then
    echo "   - vLLM 本地大模型推理     : http://localhost:8000/v1"
fi
echo "============================================================================"
