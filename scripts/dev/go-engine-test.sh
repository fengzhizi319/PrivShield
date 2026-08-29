#!/usr/bin/env bash
# 运行 Go 引擎全量测试
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== PrivShield Go Engine Tests ==="
echo ""

# privacy-go-sdk 测试
echo "--- privacy-go-sdk tests ---"
cd "$PROJECT_ROOT/privacy-go-sdk"
CGO_ENABLED=0 go test -v -count=1 ./...
echo ""

# engine-go 测试
echo "--- engine-go tests ---"
cd "$PROJECT_ROOT/engine-go"
CGO_ENABLED=0 go test -v -count=1 ./...
echo ""

# pkg 共享库测试
echo "--- pkg tests ---"
cd "$PROJECT_ROOT/pkg"
CGO_ENABLED=0 go test -v -count=1 ./...
echo ""

# services 微服务测试
echo "--- services tests ---"
cd "$PROJECT_ROOT"
CGO_ENABLED=0 go test -v -count=1 ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/...
echo ""

# console/bff-go 测试
echo "--- console/bff-go tests ---"
cd "$PROJECT_ROOT/console/bff-go"
CGO_ENABLED=0 go test -v -count=1 ./...
echo ""

echo "=== All Go engine tests passed 100% ==="
