#!/usr/bin/env bash
# ============================================================================
# Datasource Manager (模拟数据源服务) — 开发启动脚本 (Development Run)
#
# 特性：
#   - 无需 mTLS 双向认证 (TLS_ENABLED=false)
#   - 默认绑定 127.0.0.1
#   - 快速本地联调与测试
#
# 端口监听：
#   - HTTP REST: http://127.0.0.1:8083
#   - gRPC (insecure): 127.0.0.1:50053
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$MODULE_DIR"

export DATASOURCE_MGR_HOST="${DATASOURCE_MGR_HOST:-127.0.0.1}"
export DATASOURCE_MGR_PORT="${DATASOURCE_MGR_PORT:-8083}"
export DATASOURCE_MGR_GRPC_HOST="${DATASOURCE_MGR_GRPC_HOST:-127.0.0.1}"
export DATASOURCE_MGR_GRPC_PORT="${DATASOURCE_MGR_GRPC_PORT:-50053}"

# 禁用 mTLS
export DATASOURCE_MGR_TLS_ENABLED="false"
export DATASOURCE_MGR_LOG_FORMAT="${DATASOURCE_MGR_LOG_FORMAT:-text}"
export DATASOURCE_MGR_LOG_LEVEL="${DATASOURCE_MGR_LOG_LEVEL:-debug}"

echo "============================================================"
echo " 🚀 启动 datasource-mgr [开发调试模式 (Insecure / No-mTLS)]"
echo "============================================================"
echo "  HTTP REST: http://$DATASOURCE_MGR_HOST:$DATASOURCE_MGR_PORT"
echo "  gRPC:      $DATASOURCE_MGR_GRPC_HOST:$DATASOURCE_MGR_GRPC_PORT"
echo "  mTLS:      Disabled"
echo "  Log:       $DATASOURCE_MGR_LOG_FORMAT / $DATASOURCE_MGR_LOG_LEVEL"
echo "============================================================"

mkdir -p bin
go build -o bin/datasource-mgr ./cmd/server

exec ./bin/datasource-mgr
