#!/usr/bin/env bash
# 便捷入口：自动转发到 console/scripts/dev-start-go.sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$SCRIPT_DIR/scripts/dev-start-go.sh" "$@"
