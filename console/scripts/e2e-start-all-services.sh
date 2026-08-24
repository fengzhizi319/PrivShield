#!/usr/bin/env bash
# ============================================================================
# [DEPRECATED] This script has been consolidated into scripts/dev/
# Please update your references to:
#   bash ./scripts/dev/e2e-start-all-services.sh "$@"
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "⚠️  [DEPRECATED] console/scripts/e2e-start-all-services.sh has moved to scripts/dev/e2e-start-all-services.sh" >&2
echo "⚠️  Please update your shortcuts or CI scripts accordingly." >&2

exec bash "$PROJECT_ROOT/scripts/dev/e2e-start-all-services.sh" "$@"
