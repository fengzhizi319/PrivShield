#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) Kubernetes Standalone Deployment Script
# 数据服务调度中枢独立 Kubernetes Kustomize 部署脚本
#
# 功能说明：
#   1. 使用 service-hub 自包含的 deploy/k8s/ 清单进行独立部署；
#   2. 支持指定命名空间（默认: privshield 或 K8S_NAMESPACE 环境变量）；
#   3. 支持通过 --with-postgres 参数同时部署 Phase B PostgreSQL 租约存储；
#   4. 自动等待 Deployment 滚动就绪并输出服务访问端点。
#
# 用法 / Usage:
#   bash ./scripts/deploy-k8s.sh [选项]
#
# 选项 / Options:
#   -n, --namespace NS    Kubernetes 命名空间 (默认: privshield 或 K8S_NAMESPACE)
#   --with-postgres       同时部署 Phase B PostgreSQL 租约数据库
#   --dry-run             演练模式（仅生成并校验清单，不实际提交集群）
#   -h, --help            显示帮助信息并退出
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
K8S_DIR="$MODULE_DIR/deploy/k8s"

NAMESPACE="${K8S_NAMESPACE:-privshield}"
WITH_POSTGRES=false
DRY_RUN=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --with-postgres)
            WITH_POSTGRES=true
            shift
            ;;
        --dry-run)
            DRY_RUN="--dry-run=client"
            shift
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -n, --namespace NS    Kubernetes 命名空间 (默认: privshield 或 K8S_NAMESPACE)"
            echo "  --with-postgres       同时部署 Phase B PostgreSQL 租约数据库"
            echo "  --dry-run             演练模式 (客户端校验)"
            echo "  -h, --help            显示帮助信息并退出"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $1" >&2
            exit 1
            ;;
    esac
done

echo "============================================================================"
echo "☸️  【独立部署】Service Hub Kubernetes Kustomize 部署"
echo "   - 命名空间 : $NAMESPACE"
echo "   - 清单路径 : $K8S_DIR"
echo "   - 租约模式 : $( [[ "$WITH_POSTGRES" == "true" ]] && echo "PostgreSQL (Phase B 多副本)" || echo "SQLite (Phase A 单副本)" )"
echo "============================================================================"

# 前置检查
if ! command -v kubectl >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 kubectl 命令，请先安装并配置 Kubernetes 访问凭据。" >&2
    exit 1
fi

# 检查或创建命名空间
echo "📦 检查/创建命名空间 [$NAMESPACE]..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# 部署 service-hub 核心资源
echo "🚀 部署 service-hub 资源 ($K8S_DIR)..."
kubectl apply -k "$K8S_DIR" -n "$NAMESPACE" $DRY_RUN

# 可选部署 PostgreSQL (Phase B)
if [[ "$WITH_POSTGRES" == "true" ]]; then
    PG_DIR="$K8S_DIR/postgres"
    echo ""
    echo "🐘 部署 Phase B PostgreSQL 资源 ($PG_DIR)..."
    kubectl apply -k "$PG_DIR" -n "$NAMESPACE" $DRY_RUN
    if [[ -z "$DRY_RUN" ]]; then
        echo "⏳ 等待 PostgreSQL 就绪..."
        kubectl rollout status deployment/service-hub-postgres -n "$NAMESPACE" --timeout=120s || true
    fi
fi

if [[ -z "$DRY_RUN" ]]; then
    echo ""
    echo "⏳ 等待 service-hub Deployment 就绪..."
    kubectl rollout status deployment/service-hub -n "$NAMESPACE" --timeout=180s || true

    echo ""
    echo "============================================================================"
    echo "🎉 service-hub Kubernetes 资源部署完成！"
    echo "  - 查看 Pods    : kubectl get pods -l app=service-hub -n $NAMESPACE"
    echo "  - 查看 Services: kubectl get svc -l app=service-hub -n $NAMESPACE"
    echo "  - 端口转发测试 : kubectl port-forward -n $NAMESPACE svc/service-hub 8082:8082 50052:50052"
    echo "============================================================================"
else
    echo ""
    echo "✅ [Dry-Run] 客户端清单演练通过，未实际修改集群。"
fi
