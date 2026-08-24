#!/usr/bin/env bash
# ============================================================================
# Datasource Manager (数据源管理) Development Startup Script
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")"

HOST="${DATASOURCE_MGR_HOST:-127.0.0.1}"
PORT="${DATASOURCE_MGR_PORT:-8083}"

export DATASOURCE_MGR_HOST="$HOST"
export DATASOURCE_MGR_PORT="$PORT"

mkdir -p bin
go build -o bin/datasource-mgr ./cmd/server
exec ./bin/datasource-mgr
