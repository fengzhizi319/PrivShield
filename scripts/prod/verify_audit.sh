#!/usr/bin/env bash
# ============================================================================
# 审计日志 HMAC-SHA256 签名完整性校验脚本
# Verify Audit Log HMAC-SHA256 Signature Integrity
#
# 本脚本是 engine/privacy/verify_audit.py 的运维便捷包装，
# 用于校验 BudgetAuditLogger 写入的审计日志签名是否完整、未被篡改。
#
# 用法 / Usage:
#   bash scripts/prod/verify_audit.sh --key <HMAC_KEY> [--log-file <PATH>]
#   PRIVACY_AUDIT_KEY=<key> bash scripts/prod/verify_audit.sh
#
# 退出码 / Exit codes:
#   0 - 所有记录签名校验通过
#   1 - 存在签名不匹配或格式错误的记录
#   2 - 参数错误或文件不存在
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ── 自动探测 Python 解释器：优先 venv，回退到系统 python3 ──────────────
if [ -x "${PROJECT_ROOT}/.venv/bin/python" ]; then
    PYTHON="${PYTHON:-${PROJECT_ROOT}/.venv/bin/python}"
else
    PYTHON="${PYTHON:-python3}"
fi

# ── 帮助信息 ──────────────────────────────────────────────────────────
usage() {
    cat <<EOF
用法: $0 [选项]

选项:
  --key KEY           HMAC-SHA256 签名密钥（也可通过 PRIVACY_AUDIT_KEY 环境变量提供）
  --key-file PATH     从文件读取 HMAC 密钥
  --log-file PATH     审计日志文件路径（默认: \$PRIVACY_BUDGET_AUDIT_LOG 或 /tmp/budget_audit.log）
  -h, --help          显示帮助信息

示例:
  # 使用环境变量
  PRIVACY_AUDIT_KEY=my-secret bash scripts/prod/verify_audit.sh

  # 显式指定密钥与日志文件
  bash scripts/prod/verify_audit.sh --key my-secret --log-file /var/log/privshield/budget_audit.log
EOF
    exit 0
}

# ── 参数透传给 Python 模块 ────────────────────────────────────────────
ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            ;;
        *)
            ARGS+=("$1")
            shift
            ;;
    esac
done

cd "$PROJECT_ROOT"
exec "$PYTHON" -m engine.privacy.verify_audit "${ARGS[@]}"
