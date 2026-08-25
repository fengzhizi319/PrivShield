#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】启动全栈服务（Agent + Go BFF + Web UI + 可选 vLLM）
# Launch Full Stack Container Suite in Docker Compose
#
# 用法 / Usage: ./scripts/dev/docker-start-all.sh [--with-llm] [--no-build]
# ============================================================================

set -euo pipefail

# ── 解析脚本所在目录，定位项目根目录 ────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WITH_LLM=false
BUILD_FLAG="--build"   # 默认启动前重新构建镜像

# ── 解析命令行参数 ─────────────────────────────────────────────────────
#   --with-llm  : 同时启动 vLLM 大模型推理容器（需 GPU 支持）
#   --no-build  : 跳过镜像构建，直接使用本地已有镜像
#   --build     : 启动前重新构建本地镜像（默认行为）
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

# ── 前置准备：确保前端与 Go 微服务二进制已就绪 ───────────────────────────
if [[ ! -d "$PROJECT_ROOT/console/web/dist" || "$BUILD_FLAG" == "--build" ]]; then
    echo "📦 准备前端静态资源 (Vite build)..."
    (
        cd "$PROJECT_ROOT/console/web"
        if command -v corepack >/dev/null 2>&1; then
            corepack pnpm build 2>/dev/null || npm run build
        elif command -v pnpm >/dev/null 2>&1; then
            pnpm build 2>/dev/null || npm run build
        elif command -v npm >/dev/null 2>&1; then
            npm run build
        fi
    )
fi

if [[ "$BUILD_FLAG" == "--build" ]]; then
    echo "🔨 准备 Go 微服务二进制构建产物 (加速 Docker 本地构建)..."
    export GOPROXY="${GOPROXY:-https://goproxy.cn,https://goproxy.io,https://mirrors.aliyun.com/goproxy/,direct}"
    (cd "$PROJECT_ROOT/console/bff-go" && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bin/server ./cmd/server 2>/dev/null || true)
    (cd "$PROJECT_ROOT/services/service-hub" && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bin/server ./cmd/server 2>/dev/null || true)
    (cd "$PROJECT_ROOT/services/datasource-mgr" && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bin/server ./cmd/server 2>/dev/null || true)
    (cd "$PROJECT_ROOT/services/audit-log" && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bin/server ./cmd/server 2>/dev/null || true)
fi

# ── 清理可能残留的同名独立单容器 ─────────────────────────────────────────
docker rm -f PrivShield PrivShield-vllm privacy-console-backend-go privacy-console-web privshield-service-hub privshield-datasource-mgr privshield-audit-log 2>/dev/null || true

# ── 进入 docker-compose 目录，启动容器 ──────────────────────────────────
# 默认仅启动核心服务（Agent + Go BFF + Web UI + 3大中台微服务）
# 传入 --with-llm 时激活 llm profile，额外启动 vLLM 推理容器
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
echo "   - Service Hub 调度中枢    : http://localhost:8082"
echo "   - Datasource Mgr 数据源   : http://localhost:8083"
echo "   - Audit Log 脱敏审计日志  : http://localhost:8084"
echo "   - Privacy Agent REST      : http://localhost:8079"
echo "   - Privacy Agent gRPC      : localhost:50051"
if [[ "$WITH_LLM" == "true" ]]; then
    echo "   - vLLM 本地大模型推理     : http://localhost:8000/v1"
fi
echo "============================================================================"
