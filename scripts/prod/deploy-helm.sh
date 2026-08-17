#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield Kubernetes Helm Chart 生产发布与升级脚本
# Deploy and Upgrade PrivShield via Helm in Production
#
# 用法 / Usage:
#   ./scripts/prod/deploy-helm.sh [选项]
#
# 选项 / Options:
#   -n, --namespace NS       部署命名空间 (默认: privshield)
#   -r, --release RELEASE    Helm Release 名称 (默认: privshield)
#   -f, --values VALUES      生产 values 文件 (默认: deploy/helm/PrivShield/values-production.yaml)
#   --tls-secret SECRET      已有 TLS Secret 名称 (生产强制建议提供)
#   --auth-secret SECRET     已有 API Key Auth Secret 名称
#   --dry-run                执行试运行演练 (不实际修改集群)
#   -h, --help               显示帮助信息
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CHART_DIR="$PROJECT_ROOT/deploy/helm/PrivShield"

NAMESPACE="privshield"
RELEASE_NAME="privshield"
VALUES_FILE="$CHART_DIR/values-production.yaml"
TLS_SECRET=""
AUTH_SECRET=""
DRY_RUN=""

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
        -f|--values)
            VALUES_FILE="$2"
            shift 2
            ;;
        --tls-secret)
            TLS_SECRET="$2"
            shift 2
            ;;
        --auth-secret)
            AUTH_SECRET="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN="--dry-run"
            shift 1
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -n, --namespace NS       Kubernetes 命名空间 (默认: privshield)"
            echo "  -r, --release RELEASE    Helm Release 实例名称 (默认: privshield)"
            echo "  -f, --values VALUES      生产 values 配置文件路径"
            echo "  --tls-secret SECRET      Kubernetes TLS Secret 资源名称"
            echo "  --auth-secret SECRET     Kubernetes API Key Auth Secret 资源名称"
            echo "  --dry-run                执行 dry-run 演练测试"
            echo "  -h, --help               显示帮助信息并退出"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $1" >&2
            exit 1
            ;;
    esac
done

echo "============================================================================"
echo "☸️  【生产模式】PrivShield Helm 生产部署与发布"
echo "============================================================================"
echo "  • 命名空间 (Namespace) : $NAMESPACE"
echo "  • Release 名称         : $RELEASE_NAME"
echo "  • Values 文件          : $VALUES_FILE"
echo "  • Chart 目录           : $CHART_DIR"

# 1. 检查 Helm 与 Kubectl 命令
if ! command -v helm >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 helm 命令，请先安装 Helm: https://helm.sh/docs/intro/install/" >&2
    exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 kubectl 命令，请先配置 Kubernetes 客户端工具。" >&2
    exit 1
fi

# 2. 检查 values 文件
if [[ ! -f "$VALUES_FILE" ]]; then
    echo "❌ [错误] Values 文件不存在: $VALUES_FILE" >&2
    exit 1
fi

# 3. Helm Lint 预检
echo ""
echo "🔍 正在进行 Helm Chart 静态校验 (helm lint)..."
helm lint "$CHART_DIR" -f "$VALUES_FILE"

# 4. 组装 --set 参数
EXTRA_SETS=()
if [[ -n "$TLS_SECRET" ]]; then
    EXTRA_SETS+=("--set" "security.tls.existingSecret=$TLS_SECRET")
fi
if [[ -n "$AUTH_SECRET" ]]; then
    EXTRA_SETS+=("--set" "security.auth.apiKeysSecret=$AUTH_SECRET")
fi

# 5. 执行 Helm 部署/升级 (upgrade --install 平滑零停机)
echo ""
echo "🚀 正在执行 Helm 部署 (helm upgrade --install)..."
# shellcheck disable=SC2086
helm upgrade --install "$RELEASE_NAME" "$CHART_DIR" \
    --namespace "$NAMESPACE" \
    --create-namespace \
    -f "$VALUES_FILE" \
    "${EXTRA_SETS[@]}" \
    $DRY_RUN \
    --wait \
    --timeout 5m

echo ""
echo "============================================================================"
if [[ -n "$DRY_RUN" ]]; then
    echo "✅ [Dry-Run 成功] Helm 模板演练通过，未对集群做实际变更。"
else
    echo "🎉 PrivShield 生产 Helm Release [$RELEASE_NAME] 部署成功！"
    echo ""
    echo "查看 Deployment 状态:"
    echo "  kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=PrivShield"
    echo "查看 Service 状态:"
    echo "  kubectl get svc -n $NAMESPACE"
fi
echo "============================================================================"
