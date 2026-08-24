#!/usr/bin/env bash
# ============================================================================
# [DEPRECATED] This script has been consolidated into scripts/prod/
# Please update your references to:
#   bash ./scripts/prod/prod-start-go-mtls.sh "$@"
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "⚠️  [DEPRECATED] console/scripts/prod-start-go-mtls.sh has moved to scripts/prod/prod-start-go-mtls.sh" >&2
echo "⚠️  Please update your shortcuts or CI scripts accordingly." >&2

exec bash "$PROJECT_ROOT/scripts/prod/prod-start-go-mtls.sh" "$@"
