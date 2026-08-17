#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield Kubernetes Helm Release 卸载脚本
# Uninstall PrivShield Helm Release
#
# 用法 / Usage:
#   ./scripts/prod/uninstall-helm.sh [选项]
# ============================================================================

set -euo pipefail

NAMESPACE="privshield"
RELEASE_NAME="privshield"

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
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -n, --namespace NS       命名空间 (默认: privshield)"
            echo "  -r, --release RELEASE    Release 名称 (默认: privshield)"
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

if helm status "$RELEASE_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
    helm uninstall "$RELEASE_NAME" -n "$NAMESPACE"
    echo "✅ Release [$RELEASE_NAME] 已成功卸载。"
else
    echo "ℹ️  Release [$RELEASE_NAME] 不存在于命名空间 [$NAMESPACE]，无需卸载。"
fi
echo "============================================================================"
