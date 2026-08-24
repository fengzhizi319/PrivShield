#!/usr/bin/env bash
# ============================================================================
# Datasource Manager (模拟数据源服务) — 生产启动脚本 (Production Run with mTLS)
#
# 特性：
#   - 强制启用 mTLS 双向认证 (TLS_ENABLED=true)
#   - 校验客户端证书 (CLIENT_AUTH=require)
#   - 启用客户端公钥固定 (PINNED_PUBKEY_FILE)
#   - 默认绑定 0.0.0.0 (可通过环境变量调整)
#
# 端口监听：
#   - HTTP REST: http://0.0.0.0:8083
#   - gRPC (mTLS): 0.0.0.0:50053
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CERTS_DIR="${DATASOURCE_MGR_CERTS_DIR:-$MODULE_DIR/certs}"

cd "$MODULE_DIR"

# 检查证书文件是否存在，若不存在则自动生成
if [[ ! -f "$CERTS_DIR/server.crt" || ! -f "$CERTS_DIR/server.key" || ! -f "$CERTS_DIR/ca.crt" || ! -f "$CERTS_DIR/client.pub" ]]; then
    echo ">> ⚠️ 未在 $CERTS_DIR 找到完整证书链，正在自动生成测试证书..."
    bash "$SCRIPT_DIR/gen-certs.sh" "$CERTS_DIR"
fi

export DATASOURCE_MGR_HOST="${DATASOURCE_MGR_HOST:-0.0.0.0}"
export DATASOURCE_MGR_PORT="${DATASOURCE_MGR_PORT:-8083}"
export DATASOURCE_MGR_GRPC_HOST="${DATASOURCE_MGR_GRPC_HOST:-0.0.0.0}"
export DATASOURCE_MGR_GRPC_PORT="${DATASOURCE_MGR_GRPC_PORT:-50053}"

# 强制开启 mTLS 双向认证与公钥固定
export DATASOURCE_MGR_TLS_ENABLED="true"
export DATASOURCE_MGR_TLS_CERT_FILE="${DATASOURCE_MGR_TLS_CERT_FILE:-$CERTS_DIR/server.crt}"
export DATASOURCE_MGR_TLS_KEY_FILE="${DATASOURCE_MGR_TLS_KEY_FILE:-$CERTS_DIR/server.key}"
export DATASOURCE_MGR_TLS_CA_FILE="${DATASOURCE_MGR_TLS_CA_FILE:-$CERTS_DIR/ca.crt}"
export DATASOURCE_MGR_TLS_CLIENT_AUTH="${DATASOURCE_MGR_TLS_CLIENT_AUTH:-require}"
export DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE="${DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE:-$CERTS_DIR/client.pub}"

export DATASOURCE_MGR_LOG_FORMAT="${DATASOURCE_MGR_LOG_FORMAT:-json}"
export DATASOURCE_MGR_LOG_LEVEL="${DATASOURCE_MGR_LOG_LEVEL:-info}"

echo "============================================================"
echo " 🔒 启动 datasource-mgr [生产加固模式 (双协议 mTLS + 公钥固定)]"
echo "============================================================"
echo "  HTTPS REST (mTLS): https://$DATASOURCE_MGR_HOST:$DATASOURCE_MGR_PORT"
echo "  gRPC (mTLS):       $DATASOURCE_MGR_GRPC_HOST:$DATASOURCE_MGR_GRPC_PORT"
echo "  Server Cert: $DATASOURCE_MGR_TLS_CERT_FILE"
echo "  CA File:     $DATASOURCE_MGR_TLS_CA_FILE"
echo "  Client Auth: $DATASOURCE_MGR_TLS_CLIENT_AUTH"
echo "  Pinned Key:  $DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE"
echo "  Log:         $DATASOURCE_MGR_LOG_FORMAT / $DATASOURCE_MGR_LOG_LEVEL"
echo "============================================================"

mkdir -p bin
go build -o bin/datasource-mgr ./cmd/server

exec ./bin/datasource-mgr
