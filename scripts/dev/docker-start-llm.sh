#!/usr/bin/env bash
# ============================================================================
# 【开发模式】启动 Layer-3 LLM 推理服务 (vLLM / GPU 加速 / 支持 Linux/macOS/Windows)
# Launch vLLM Layer-3 LLM inference container (Supports Linux / macOS / Windows WSL2)
#
# 执行步骤总览：
#   1. 解析命令行参数（帮助信息）
#   2. 自动识别操作系统与 CPU 架构平台（Linux / macOS / Windows WSL2）
#   3. 前置检查：Docker CLI 与 Docker Daemon 连通性
#   4. 检查本地大模型权重目录存在性并给出下载指引
#   5. 使用 Docker Compose profile 'llm' 启动 vLLM 独立容器（端口 8000）
#
# 用法 / Usage: ./scripts/dev/docker-start-llm.sh
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ── 步骤 1：帮助信息处理 ──────────────────────────────────────────────────
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    echo "用法 / Usage: $0"
    echo ""
    echo "功能说明 / Description:"
    echo "  使用 Docker Compose 在独立容器中拉起 vLLM GPU 推理服务（服务名: vllm，端口: 8000）。"
    echo "  默认模型: Qwen3.5-0.8B-Privacy-Classifier-Smoother"
    echo ""
    echo "跨平台支持 / Cross-Platform:"
    echo "  - Linux: NVIDIA GPU (需 nvidia-container-toolkit)"
    echo "  - macOS: Docker Desktop (CPU / 容器化推理)"
    echo "  - Windows 11: WSL2 (NVIDIA GPU 直通) 或直接运行 scripts/dev/docker-start-llm.ps1"
    exit 0
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
echo "🤖 [Docker Mode] 正在使用 Docker Compose 启动 vLLM 本地大模型服务"
echo "   • 平台环境 : $PLATFORM_NAME"
echo "============================================================================"

# ── 步骤 4：检查本地大模型权重目录 ────────────────────────────────────────
MODEL_DIR="$PROJECT_ROOT/.models/Qwen3.5-0.8B-Privacy-Classifier-Smoother"
if [ ! -d "$MODEL_DIR" ]; then
    echo "⚠️  [提示] 本地大模型权重目录不存在: $MODEL_DIR"
    echo "   建议先执行模型下载命令以获取微调权重:"
    echo "   python scripts/models/download_model.py"
    echo ""
fi

cd "$PROJECT_ROOT/deploy/docker-compose"

# ── 步骤 5：使用 compose profile 'llm' 启动 vLLM 服务 ──────────────────────
docker compose --profile llm up -d vllm

echo ""
echo "✅ vLLM 大模型推理容器已启动！"
echo "   - OpenAI 兼容接口 : http://127.0.0.1:8000/v1"
echo "   - 查看日志        : docker logs -f PrivShield-vllm"
echo "============================================================================"

