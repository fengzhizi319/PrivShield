#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield 生产级 Docker Compose 一键部署脚本
# Launch PrivShield in Production Mode with Docker Compose Orchestration
#
# 用法 / Usage:
#   ./scripts/prod/deploy-docker-compose.sh [选项]
#
# 选项 / Options:
#   -f, --file FILE      指定 Compose 配置文件 (默认: docker-compose.prod.yml)
#   --with-llm           启用 vLLM 大模型推理容器 (需具备 NVIDIA GPU / CUDA 环境)
#   --with-monitoring    启用生产监控栈 (Prometheus + Grafana)
#   --build              强制重新构建容器镜像 (默认使用已有镜像)
#   --pull               拉取最新基础镜像
#   -h, --help           显示帮助信息
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_DIR="$PROJECT_ROOT/deploy/docker-compose"

COMPOSE_FILE="docker-compose.prod.yml"
WITH_LLM=false
WITH_MONITORING=false
BUILD_FLAG=""
PULL_FLAG=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -f|--file)
            COMPOSE_FILE="$2"
            shift 2
            ;;
        --with-llm)
            WITH_LLM=true
            shift 1
            ;;
        --with-monitoring)
            WITH_MONITORING=true
            shift 1
            ;;
        --build)
            BUILD_FLAG="--build"
            shift 1
            ;;
        --pull)
            PULL_FLAG="--pull always"
            shift 1
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -f, --file FILE      指定 Compose 配置文件 (默认: docker-compose.prod.yml)"
            echo "  --with-llm           启用 vLLM 大模型 GPU 推理容器"
            echo "  --with-monitoring    启用 Prometheus + Grafana 生产监控套件"
            echo "  --build              在启动前重新构建应用镜像"
            echo "  --pull               拉取最新的依赖基础镜像"
            echo "  -h, --help           显示帮助信息并退出"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $1" >&2
            echo "   请运行 $0 --help 查看帮助" >&2
            exit 1
            ;;
    esac
done

echo "============================================================================"
echo "🛡️  【生产模式】PrivShield 生产级 Docker Compose 部署"
echo "============================================================================"

# 1. 检查 Docker 与 Compose 插件
if ! command -v docker >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 docker 命令，请先安装 Docker Engine: https://docs.docker.com/engine/install/" >&2
    exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 Docker Compose v2 插件，请安装 docker-compose-plugin。" >&2
    exit 1
fi

# 2. 检查持久化数据目录
mkdir -p "$PROJECT_ROOT/.data" "$PROJECT_ROOT/.logs"
chmod 755 "$PROJECT_ROOT/.data" "$PROJECT_ROOT/.logs"

# 3. 构建 Profile 参数
PROFILES=()
if [[ "$WITH_LLM" == "true" ]]; then
    PROFILES+=("--profile" "llm")
    echo "   • 大模型推理 : 已启用 vLLM (Profile: llm)"
else
    echo "   • 大模型推理 : 未启用 (轻量 Core 规则+NER 模式)"
fi

if [[ "$WITH_MONITORING" == "true" ]]; then
    PROFILES+=("--profile" "monitoring")
    echo "   • 生产监控栈 : 已启用 (Prometheus + Grafana)"
else
    echo "   • 生产监控栈 : 未启用"
fi

cd "$COMPOSE_DIR"

if [[ ! -f "$COMPOSE_FILE" ]]; then
    echo "⚠️  指定的 Compose 文件不存在: $COMPOSE_FILE，回退为 docker-compose.yml"
    COMPOSE_FILE="docker-compose.yml"
fi

echo "   • 编排配置文件: $COMPOSE_FILE"

# 4. 执行 Docker Compose 启动
echo ""
echo "🚀 正在启动生产服务容器群..."
# shellcheck disable=SC2086
docker compose -f "$COMPOSE_FILE" "${PROFILES[@]}" up -d $BUILD_FLAG $PULL_FLAG

# 5. 等待服务就绪探针 (GET /readyz)
echo ""
echo -n "⏳ 等待 PrivShield 核心 Agent 服务就绪探针响应..."
MAX_ATTEMPTS=30
ATTEMPT=0
READY=false

while [[ $ATTEMPT -lt $MAX_ATTEMPTS ]]; do
    if curl -s -k -o /dev/null -w "%{http_code}" "https://127.0.0.1:8079/readyz" 2>/dev/null | grep -q '^200$' || \
       curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:8079/readyz" 2>/dev/null | grep -q '^200$'; then
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
    echo " ⚠️  就绪检查等待超时，请使用 'docker compose -f $COMPOSE_FILE logs' 查看容器运行状态。"
fi

echo ""
echo "============================================================================"
echo "🎉 PrivShield 生产级服务已启动完毕！"
echo "============================================================================"
echo "  • 核心 Agent REST API : http://127.0.0.1:8079 (或 https://127.0.0.1:8079)"
echo "  • 核心 Agent gRPC RPC : 127.0.0.1:50051"
echo "  • Web 控制台 UI       : http://127.0.0.1:5173"
echo "  • Go 代理后端 REST    : http://127.0.0.1:8081"
if [[ "$WITH_LLM" == "true" ]]; then
    echo "  • vLLM 大模型推理 API : http://127.0.0.1:8000/v1"
fi
if [[ "$WITH_MONITORING" == "true" ]]; then
    echo "  • Prometheus 监控指标 : http://127.0.0.1:9090"
    echo "  • Grafana 可视化大屏  : http://127.0.0.1:3000 (admin / privshield_admin)"
fi
echo "────────────────────────────────────────────────────────────────────────────"
echo "  常用维护命令:"
echo "    - 查看容器运行状态 : cd $COMPOSE_DIR && docker compose -f $COMPOSE_FILE ps"
echo "    - 实时查看服务日志 : cd $COMPOSE_DIR && docker compose -f $COMPOSE_FILE logs -f"
echo "    - 停止生产服务集群 : ./scripts/prod/stop-docker-compose.sh"
echo "    - 生产健康全面巡检 : ./scripts/prod/prod_health_check.sh"
echo "============================================================================"
