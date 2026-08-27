#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) Kubernetes Standalone Cleanup Script
# 数据服务调度中枢独立 Kubernetes 资源卸载与停止脚本
#
# 用法 / Usage:
#   bash ./scripts/stop-k8s.sh [选项]
#
# 选项 / Options:
#   -n, --namespace NS    Kubernetes 命名空间 (默认: privshield 或 K8S_NAMESPACE)
#   --with-postgres       同时删除 Phase B PostgreSQL 租约数据库
#   -h, --help            显示帮助信息并退出
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
K8S_DIR="$MODULE_DIR/deploy/k8s"

NAMESPACE="${K8S_NAMESPACE:-privshield}"
WITH_POSTGRES=false

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
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -n, --namespace NS    Kubernetes 命名空间 (默认: privshield 或 K8S_NAMESPACE)"
            echo "  --with-postgres       同时删除 Phase B PostgreSQL 租约数据库"
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
echo "🛑 正在卸载 service-hub Kubernetes 资源 (Namespace: $NAMESPACE)..."
echo "============================================================================"

if ! command -v kubectl >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 kubectl 命令。" >&2
    exit 1
fi

kubectl delete -k "$K8S_DIR" -n "$NAMESPACE" --ignore-not-found=true

if [[ "$WITH_POSTGRES" == "true" ]]; then
    PG_DIR="$K8S_DIR/postgres"
    echo "🐘 删除 Phase B PostgreSQL 资源..."
    kubectl delete -k "$PG_DIR" -n "$NAMESPACE" --ignore-not-found=true
fi

echo "✅ service-hub Kubernetes 资源已成功删除。"
echo "============================================================================"
