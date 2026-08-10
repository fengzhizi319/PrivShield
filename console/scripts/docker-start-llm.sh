#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动 Layer-3 LLM 推理服务 (vLLM / GPU 加速)
# Launch vLLM Layer-3 LLM inference container
#
# 用法 / Usage: ./console/scripts/docker-start-llm.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================================================"
echo "🤖 [Docker Mode] 正在使用 Docker Compose 启动 vLLM 本地大模型服务..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"

# 使用 compose profile 'llm' 启动 vLLM 服务
docker compose --profile llm up -d vllm

echo ""
echo "✅ vLLM 大模型推理容器已启动！"
echo "   - OpenAI 兼容接口 : http://127.0.0.1:8000/v1"
echo "   - 查看日志        : docker logs -f privacy-local-agent-vllm"
echo "============================================================================"
