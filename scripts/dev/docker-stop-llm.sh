#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】停止并清理 vLLM Layer-3 LLM 推理容器
# Stop and remove vLLM Layer-3 LLM inference container
#
# 用法 / Usage: ./scripts/dev/docker-stop-llm.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================================================"
echo "🛑 [Docker Mode] 正在停止 vLLM 大模型推理容器..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"
docker compose --profile llm stop vllm 2>/dev/null || true
docker rm -f privacy-local-agent-vllm 2>/dev/null || true

echo "✅ vLLM 大模型推理容器已成功停止与清理！"
echo "============================================================================"
