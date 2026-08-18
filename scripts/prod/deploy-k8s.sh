#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield 原生 Kubernetes Kustomize 生产发布脚本
# Deploy PrivShield via Native Kubernetes Kustomize
#
# 与 Helm 部署的区别：
#   - Helm（deploy-helm.sh）：Go 模板 + values 参数驱动，功能全面（HPA/Ingress/PDB 等）
#   - 本脚本（deploy-k8s.sh）：直接 apply 静态 YAML 清单，适合无 Helm 环境或轻量部署
#
# 执行步骤总览：
#   1. 解析命令行参数（命名空间）
#   2. 前置检查：kubectl 命令是否可用
#   3. 确保 K8s 命名空间存在（幂等创建）
#   4. 通过 Kustomize 应用全部资源清单（Namespace/ConfigMap/Deployment/Service）
#   5. 等待 Deployment 滚动更新就绪
#   6. 输出部署结果与后续验证命令
#
# 用法 / Usage:
#   ./scripts/prod/deploy-k8s.sh [选项]
#
# 选项 / Options:
#   -n, --namespace NS    Kubernetes 命名空间 (默认: privshield 或环境变量 K8S_NAMESPACE)
#   -h, --help            显示帮助信息并退出
# ============================================================================

# set -e: 任何命令返回非零状态码立即退出（防止错误级联）
# set -u: 引用未定义变量时报错（防止拼写错误导致静默失败）
# set -o pipefail: 管道中任一命令失败则整体返回非零（防止 | 后掩盖错误）
set -euo pipefail

# ── 步骤 0：定位路径 ──────────────────────────────────────────────────────
# 通过 $0（脚本自身路径）反推项目根目录，确保无论从哪里调用都能正确定位 K8s 清单
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"          # 脚本所在目录：scripts/prod/
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"       # 上溯两级：项目根目录
K8S_DIR="$PROJECT_ROOT/deploy/k8s"                     # 原生 K8s 清单目录（含 kustomization.yaml）

# ── 步骤 1：设置参数默认值并解析命令行参数 ────────────────────────────────
# 命名空间优先级：命令行 -n > 环境变量 K8S_NAMESPACE > 默认值 privshield
NAMESPACE="${K8S_NAMESPACE:-privshield}"

# 遍历所有位置参数，按 --key value 配对消费（shift 2 跳过已处理的两个参数）
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

# ── 步骤 2：打印部署摘要 ──────────────────────────────────────────────────
# 展示本次部署的关键信息，便于运维确认
echo "============================================================================"
echo "☸️  【生产模式】PrivShield 原生 Kubernetes Kustomize 部署"
echo "============================================================================"

# ── 步骤 3：前置检查 — kubectl 可用性 ────────────────────────────────────
# command -v 检查命令是否存在于 PATH 中；缺失则提示安装并退出
if ! command -v kubectl >/dev/null 2>&1; then
    echo "❌ [错误] 未检测到 kubectl 命令，请先安装并配置 kubectl。" >&2
    exit 1
fi

# ── 步骤 4：幂等创建命名空间 ──────────────────────────────────────────────
# 技巧：kubectl create --dry-run=client -o yaml | kubectl apply -f -
#   1. --dry-run=client  仅在客户端生成 YAML，不实际请求 API Server
#   2. -o yaml           输出生成的 Namespace YAML
#   3. kubectl apply -f - 将 YAML 提交给 API Server（已存在则更新，不存在则创建）
# 效果：命名空间不存在则创建，已存在则跳过（幂等，可重复执行不报错）
echo "📦 检查或创建命名空间 [$NAMESPACE]..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# ── 步骤 5：通过 Kustomize 应用全部资源清单 ───────────────────────────────
# kubectl apply -k 指向含 kustomization.yaml 的目录，Kustomize 会：
#   1. 读取 kustomization.yaml 中声明的 resources 列表
#   2. 按顺序合并所有 YAML 文件（namespace.yaml / configmap.yaml / deployment.yaml / service.yaml）
#   3. 应用 kustomization.yaml 中的 namespace 字段覆盖（统一注入命名空间）
#   4. 将合并后的完整 YAML 一次性提交给 K8s API Server
# 等效于手动 kubectl apply 每个文件，但更简洁且支持覆盖
# 注意：kustomization.yaml 中默认仅启用核心 4 个资源；
#       TLS Secret / LLM 独立服务需手动取消注释后才会被应用
echo "🚀 应用 Kustomize 资源清单 ($K8S_DIR)..."
kubectl apply -k "$K8S_DIR" -n "$NAMESPACE"

# ── 步骤 6：等待 Deployment 滚动更新就绪 ────────────────────────────────
# kubectl rollout status 阻塞等待直至 Deployment 所有副本更新完毕（新 Pod Ready + 旧 Pod 终止）
# --timeout=180s  最大等待 3 分钟，超时则返回非零
# || true         即使超时也不中断脚本（set -e 下防止脚本意外退出）
#   原因：rollout 超时不代表部署失败，可能只是镜像拉取较慢，
#         运维可后续手动 kubectl rollout status 继续观察
echo ""
echo "⏳ 等待 Deployment 滚动更新就绪..."
kubectl rollout status deployment/privshield -n "$NAMESPACE" --timeout=180s || true

# ── 步骤 7：输出部署结果与后续验证命令 ────────────────────────────────────
# 提供常用 kubectl 命令帮助运维快速确认部署状态
echo ""
echo "============================================================================"
echo "🎉 Kubernetes 资源部署完成！"
echo "  - 查看 Pods    : kubectl get pods -n $NAMESPACE"
echo "  - 查看 Services: kubectl get svc -n $NAMESPACE"
echo "============================================================================"
