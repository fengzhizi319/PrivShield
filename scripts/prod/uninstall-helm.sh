#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield Kubernetes Helm Release 卸载脚本
# Uninstall PrivShield Helm Release
#
# 执行步骤总览：
#   1. 解析命令行参数（命名空间、Release 名称）
#   2. 检查 Helm Release 存在性与状态
#   3. 执行 helm uninstall 卸载 Release 并释放相关 Pod 与 Service
#   4. 可选：删除 Phase B PostgreSQL 资源（--with-postgres）
#
# 用法 / Usage:
#   ./scripts/prod/uninstall-helm.sh [选项]
#
# 选项 / Options:
#   -n, --namespace NS       命名空间 (默认: privshield)
#   -r, --release RELEASE    Release 名称 (默认: privshield)
#   --with-postgres          同时删除 Phase B PostgreSQL 资源
#   -h, --help               显示帮助信息
# ============================================================================

set -euo pipefail

# ── 步骤 1：设置参数默认值 ────────────────────────────────────────────────
NAMESPACE="privshield"
RELEASE_NAME="privshield"
WITH_POSTGRES=false

# ── 步骤 2：解析命令行参数 ────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -r|--release)
            RELEASE_NAME="$2"
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
            echo "  -n, --namespace NS       命名空间 (默认: privshield)"
            echo "  -r, --release RELEASE    Release 名称 (默认: privshield)"
            echo "  --with-postgres          同时删除 Phase B PostgreSQL 资源"
            echo "  -h, --help               显示帮助信息"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $1" >&2
            exit 1
            ;;
    esac
done

echo "============================================================================"
echo "🛑 正在卸载 PrivShield Helm Release [$RELEASE_NAME] (Namespace: $NAMESPACE)..."
echo "============================================================================"

# ── 步骤 3：检查并卸载 Release ────────────────────────────────────────────
if helm status "$RELEASE_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
    helm uninstall "$RELEASE_NAME" -n "$NAMESPACE"
    echo "✅ Release [$RELEASE_NAME] 已成功卸载。"
else
    echo "ℹ️  Release [$RELEASE_NAME] 不存在于命名空间 [$NAMESPACE]，无需卸载。"
fi

# 可选：删除 Phase B PostgreSQL 资源
if [[ "$WITH_POSTGRES" == "true" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
    PG_DIR="$PROJECT_ROOT/deploy/k8s/service-hub/postgres"
    echo "🐘 删除 Phase B PostgreSQL 资源..."
    if command -v kubectl >/dev/null 2>&1; then
        kubectl delete -k "$PG_DIR" -n "$NAMESPACE" --ignore-not-found=true
        echo "✅ PostgreSQL 资源已删除。"
    else
        echo "❌ [错误] 未检测到 kubectl 命令，无法删除 PostgreSQL 资源。" >&2
    fi
fi
echo "============================================================================"
