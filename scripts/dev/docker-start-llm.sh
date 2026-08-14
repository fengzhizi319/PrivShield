#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动 Layer-3 LLM 推理服务 (vLLM / GPU 加速)
# Launch vLLM Layer-3 LLM inference container
#
# 用法 / Usage: ./scripts/dev/docker-start-llm.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================================================"
echo "🤖 [Docker Mode] 正在使用 Docker Compose 启动 vLLM 本地大模型服务..."
echo "============================================================================"

MODEL_DIR="$PROJECT_ROOT/.models/Qwen3.5-0.8B-Privacy-Classifier-Smoother"
if [ ! -d "$MODEL_DIR" ]; then
    echo "⚠️  [提示] 本地大模型权重目录不存在: $MODEL_DIR"
    echo "   建议先执行模型下载命令以获取微调权重:"
    echo "   python -m privacy_local_agent.privacy.download_model"
    echo ""
fi

cd "$PROJECT_ROOT/deploy/docker-compose"

# 使用 compose profile 'llm' 启动 vLLM 服务
docker compose --profile llm up -d vllm

echo ""
echo "✅ vLLM 大模型推理容器已启动！"
echo "   - OpenAI 兼容接口 : http://127.0.0.1:8000/v1"
echo "   - 查看日志        : docker logs -f privacy-local-agent-vllm"
echo "============================================================================"
