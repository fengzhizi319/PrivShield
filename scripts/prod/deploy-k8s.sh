#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield 原生 Kubernetes Kustomize 生产发布脚本
# Deploy PrivShield via Native Kubernetes Kustomize
#
# 用法 / Usage:
#   ./scripts/prod/deploy-k8s.sh [选项]
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
K8S_DIR="$PROJECT_ROOT/deploy/k8s"
NAMESPACE="${K8S_NAMESPACE:-privshield}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -n, --namespace NS    Kubernetes 命名空间 (默认: privshield 或 K8S_NAMESPACE)"
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
echo "☸️  【生产模式】PrivShield 原生 Kubernetes Kustomize 部署"
echo "============================================================================"

# 1. 检查 kubectl 命令
if ! command -v kubectl >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 kubectl 命令，请先安装并配置 kubectl。" >&2
    exit 1
fi

# 2. 确保命名空间存在
echo "📦 检查或创建命名空间 [$NAMESPACE]..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# 3. 部署 K8s 资源清单
echo "🚀 应用 Kustomize 资源清单 ($K8S_DIR)..."
kubectl apply -k "$K8S_DIR" -n "$NAMESPACE"

# 4. 等待 Rollout 就绪
echo ""
echo "⏳ 等待 Deployment 滚动更新就绪..."
kubectl rollout status deployment/privshield -n "$NAMESPACE" --timeout=180s || true

echo ""
echo "============================================================================"
echo "🎉 Kubernetes 资源部署完成！"
echo "  - 查看 Pods    : kubectl get pods -n $NAMESPACE"
echo "  - 查看 Services: kubectl get svc -n $NAMESPACE"
echo "============================================================================"
