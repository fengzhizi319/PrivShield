#!/bin/bash
# 运行 Go 引擎全量测试
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== PrivShield Go Engine Tests ==="
echo ""

# privacy-go-sdk 测试
echo "--- privacy-go-sdk tests ---"
cd "$PROJECT_ROOT/privacy-go-sdk"
go test -v -race -count=1 ./...
echo ""

# engine-go 测试
echo "--- engine-go tests ---"
cd "$PROJECT_ROOT/engine-go"
go test -v -race -count=1 ./...
echo ""

echo "=== All Go engine tests passed ==="
