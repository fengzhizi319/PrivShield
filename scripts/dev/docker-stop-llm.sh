#!/usr/bin/env bash
# ============================================================================
# 【开发模式】停止并清理 vLLM Layer-3 LLM 推理容器
# Stop and remove vLLM Layer-3 LLM inference container
#
# 执行步骤总览：
#   1. 切换到 Docker Compose 编排目录
#   2. 执行 docker compose stop vllm 优雅停止容器
#   3. 强制清理 PrivShield-vllm 容器残留并输出结果
#
# 用法 / Usage: ./scripts/dev/docker-stop-llm.sh
# ============================================================================

set -euo pipefail

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -h, --help    显示帮助信息并退出"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================================================"
echo "🛑 [Docker Mode] 正在停止 vLLM 大模型推理容器..."
echo "============================================================================"

# ── 步骤 1：定位编排目录并停止清理容器 ────────────────────────────────────
cd "$PROJECT_ROOT/deploy/docker-compose"
docker compose --profile llm stop vllm 2>/dev/null || true
docker rm -f PrivShield-vllm 2>/dev/null || true

# ── 步骤 2：输出清理完成提示 ──────────────────────────────────────────────
echo "✅ vLLM 大模型推理容器已成功停止与清理！"
echo "============================================================================"
