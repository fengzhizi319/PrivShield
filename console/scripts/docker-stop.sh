#!/usr/bin/env bash
# ============================================================================
# [DEPRECATED] This script has been consolidated into scripts/dev/
# Please update your references to:
#   bash ./scripts/dev/docker-stop.sh "$@"
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "⚠️  [DEPRECATED] console/scripts/docker-stop.sh has moved to scripts/dev/docker-stop.sh" >&2
echo "⚠️  Please update your shortcuts or CI scripts accordingly." >&2

exec bash "$PROJECT_ROOT/scripts/dev/docker-stop.sh" "$@"
