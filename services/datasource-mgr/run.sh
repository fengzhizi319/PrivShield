#!/usr/bin/env bash
# ============================================================================
# Datasource Manager (模拟数据源服务) 入口快捷脚本
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")"

MODE="${1:-dev}"

if [[ "$MODE" == "prod" ]]; then
    exec bash scripts/prod-run.sh
else
    exec bash scripts/dev-run.sh
fi
