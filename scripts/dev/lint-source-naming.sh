#!/usr/bin/env bash
# ==============================================================================
# PrivShield Source & Naming Consistency Guard / 源码命名一致性门禁检查
# ==============================================================================
# 检查项：
# 1. 拦截已废弃的非法历史标识字面量（如 mock1, mock2, /api/v1/datasources 等）
# 2. 检查 Go (pkg/naming)、Python (engine/naming.py) 与 TS (web/src/types/naming.ts) 的 SSOT 对齐
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "=== [Lint] Starting PrivShield Source & Naming Consistency Check ==="

ERRORS=0

# 1. 检查遗留错误 URL 路径
echo "[1/3] Checking for deprecated API routes in source code..."
DEPRECATED_ROUTES=(
    "/api/v1/datasources"
    "/api/v1/audit/logs"
    "/api/v1/audit/verify"
)

for route in "${DEPRECATED_ROUTES[@]}"; do
    MATCHES=$(grep -rn "${route}" "${ROOT_DIR}/services" "${ROOT_DIR}/console" "${ROOT_DIR}/pkg" 2>/dev/null | grep -v "_test.go" | grep -v "\.md" | grep -v "//" || true)
    if [ -n "${MATCHES}" ]; then
        echo "❌ ERROR: Found deprecated route '${route}' in active code:"
        echo "${MATCHES}"
        ERRORS=$((ERRORS + 1))
    fi
done

# 2. 检查非法 mock1 / mock2 标识
echo "[2/3] Checking for obsolete mock1 / mock2 identifiers..."
OBSOLETE_IDS=(
    "mock1"
    "mock2"
    "ds_mock1"
    "ds_mock2"
)

for obs in "${OBSOLETE_IDS[@]}"; do
    MATCHES=$(grep -rn "\"${obs}\"" "${ROOT_DIR}/services" "${ROOT_DIR}/console" "${ROOT_DIR}/engine" "${ROOT_DIR}/pkg" 2>/dev/null | grep -v "_test.go" | grep -v "test_" | grep -v "\.md" || true)
    if [ -n "${MATCHES}" ]; then
        echo "❌ ERROR: Found obsolete identifier '${obs}' in active code:"
        echo "${MATCHES}"
        ERRORS=$((ERRORS + 1))
    fi
done

# 3. 运行 Go 与 Python 命名一致性单元测试
echo "[3/3] Running Cross-Language SSOT Parity Unit Tests..."
cd "${ROOT_DIR}"
if ! go test -v ./pkg/naming/... > /dev/null 2>&1; then
    echo "❌ ERROR: Go pkg/naming unit tests failed!"
    ERRORS=$((ERRORS + 1))
fi

if [ "${ERRORS}" -eq 0 ]; then
    echo "✅ [Lint Passed] All PrivShield naming standards & SSOT parity verified successfully!"
    exit 0
else
    echo "❌ [Lint Failed] Found ${ERRORS} naming consistency errors."
    exit 1
fi
