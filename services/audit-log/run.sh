#!/usr/bin/env bash
# ============================================================================
# Audit Log (脱敏审计日志) Development Startup Script
# ============================================================================

set -euo pipefail

cd "$(dirname "$0")"

HOST="${AUDIT_LOG_HOST:-127.0.0.1}"
PORT="${AUDIT_LOG_PORT:-8084}"

export AUDIT_LOG_HOST="$HOST"
export AUDIT_LOG_PORT="$PORT"

mkdir -p bin
go build -o bin/audit-log ./cmd/server
exec ./bin/audit-log
