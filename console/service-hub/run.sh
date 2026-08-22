#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) Development Startup Script
# 数据服务调度中枢开发模式启动脚本
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")"

HOST="${SERVICE_HUB_HOST:-127.0.0.1}"
PORT="${SERVICE_HUB_PORT:-8082}"

export SERVICE_HUB_HOST="$HOST"
export SERVICE_HUB_PORT="$PORT"

mkdir -p bin
go build -o bin/service-hub ./cmd/server
exec ./bin/service-hub
