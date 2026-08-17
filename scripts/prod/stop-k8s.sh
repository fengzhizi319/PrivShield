#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield 原生 Kubernetes 资源卸载脚本
# Uninstall PrivShield Native Kubernetes Resources
#
# 用法 / Usage:
#   ./scripts/prod/stop-k8s.sh [选项]
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
echo "🛑 正在卸载 PrivShield Kubernetes 资源 (Namespace: $NAMESPACE)..."
echo "============================================================================"

if command -v kubectl >/dev/null 2>&1; then
    kubectl delete -k "$K8S_DIR" -n "$NAMESPACE" --ignore-not-found=true
    echo "✅ Kubernetes 资源已成功删除。"
else
    echo "❌ [错误] 未检测到 kubectl 命令。" >&2
    exit 1
fi
echo "============================================================================"
