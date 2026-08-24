#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动全栈服务（Agent + 双后端 + Web UI + 可选 vLLM）
# Launch Full Stack Container Suite in Docker Compose
#
# 用法 / Usage: ./scripts/dev/docker-start-all.sh [--with-llm] [--no-build]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WITH_LLM=false
BUILD_FLAG="--build"

for arg in "$@"; do
    case "$arg" in
        --with-llm)
            WITH_LLM=true
            ;;
        --no-build)
            BUILD_FLAG=""
            ;;
        --build)
            BUILD_FLAG="--build"
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [--with-llm] [--build] [--no-build]"
            echo ""
            echo "选项 / Options:"
            echo "  --with-llm   同时启动 vLLM 大模型推理容器 (需 GPU)"
            echo "  --no-build   跳过镜像构建，使用本地已有镜像"
            echo "  --build      启动前重新构建本地镜像 (默认)"
            echo "  -h, --help   显示帮助信息"
            exit 0
            ;;
    esac
done

echo "============================================================================"
echo "🌟 [Docker Mode] 正在启动 PrivShield 全栈容器套件..."
echo "============================================================================"

cd "$PROJECT_ROOT/deploy/docker-compose"

if [[ "$WITH_LLM" == "true" ]]; then
    echo "🤖 同时启动 vLLM 大模型推理容器 (GPU)..."
    # shellcheck disable=SC2086
    docker compose --profile llm up -d $BUILD_FLAG
else
    # shellcheck disable=SC2086
    docker compose up -d $BUILD_FLAG
fi

echo ""
echo "✅ 全栈 Docker 容器服务已成功启动！"
echo "   - React 控制台 Web UI     : http://localhost:5173"
echo "   - Go BFF 代理网关 REST     : http://localhost:8081"
echo "   - Python BFF 代理后端 REST : http://localhost:8080"
echo "   - Service Hub 调度中枢    : http://localhost:8082"
echo "   - Datasource Mgr 数据源   : http://localhost:8083"
echo "   - Audit Log 脱敏审计日志  : http://localhost:8084"
echo "   - Privacy Agent REST      : http://localhost:8079"
echo "   - Privacy Agent gRPC      : localhost:50051"
if [[ "$WITH_LLM" == "true" ]]; then
    echo "   - vLLM 本地大模型推理     : http://localhost:8000/v1"
fi
echo "============================================================================"
