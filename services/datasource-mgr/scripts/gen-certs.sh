#!/usr/bin/env bash
# ============================================================================
# Generate mTLS test certificate chain for datasource-mgr gRPC server.
# 为 datasource-mgr 模拟数据源 gRPC 服务端生成 mTLS 测试证书链与公钥固定文件。
#
# Generated files (default output: services/datasource-mgr/certs/) / 生成的文件：
#   ca.crt / ca.key                 Trusted root CA
#                                   受信任根 CA
#   server.crt / server.key         Server cert (SAN: localhost/127.0.0.1)
#                                   服务端证书
#   client.crt / client.key         Client cert (EKU: clientAuth)
#                                   客户端证书
#   client.pub                      Client public key (for public key pinning)
#                                   客户端公钥（用于固定）
#
# Usage / 用法：
#   ./scripts/gen-certs.sh [output_dir]
#
# NOTE: These certificates and public keys are committed to Git for reproducible
#       testing and static public key pinning during development and verification.
# 注意：此证书与公钥已加入版本管理，用于测试环境可复现的公钥固定校验。
# ============================================================================

set -euo pipefail

OUT_DIR="${1:-$(dirname "$0")/../certs}"
DAYS="${CERT_DAYS:-3650}"

# Convert to absolute path
OUT_DIR="$(cd "$(dirname "$OUT_DIR")" && pwd)/$(basename "$OUT_DIR")"

mkdir -p "$OUT_DIR"
cd "$OUT_DIR"

echo ">> 生成 datasource-mgr mTLS 测试证书到: $OUT_DIR"
echo "   有效期: $DAYS 天"
echo ""

# ── 1. 根 CA ─────────────────────────────────────────────────────────────
echo ">> [1/4] 生成根 CA..."
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days "$DAYS" \
    -out ca.crt -subj "/CN=datasource-mgr-test-ca"

# ── 2. 服务端证书 ─────────────────────────────────────────────────────────
echo ">> [2/4] 生成服务端证书（SAN: localhost/127.0.0.1）..."
openssl genrsa -out server.key 2048
openssl req -new -key server.key -subj "/CN=localhost" -out server.csr

cat > server.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=@alt_names

[alt_names]
DNS.1=localhost
IP.1=127.0.0.1
EOF

openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days "$DAYS" -sha256 -extfile server.ext

# ── 3. 客户端证书 ─────────────────────────────────────────────────────────
echo ">> [3/4] 生成客户端证书（EKU: clientAuth）..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -subj "/CN=datasource-mgr-client" -out client.csr

cat > client.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
EOF

openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out client.crt -days "$DAYS" -sha256 -extfile client.ext

# ── 4. 提取客户端公钥（用于公钥固定）──────────────────────────────────────
echo ">> [4/4] 提取客户端公钥（用于公钥固定）..."
openssl rsa -in client.key -pubout -out client.pub

# ── 清理中间临时文件 ──────────────────────────────────────────────────────
rm -f server.csr client.csr server.ext client.ext ca.srl

chmod 644 ./*.crt ./*.pub 2>/dev/null || true
chmod 600 ./*.key 2>/dev/null || true

echo ""
echo ">> 完成，生成文件："
ls -la "$OUT_DIR"
echo ""
echo ">> datasource-mgr 生产运行 (prod-run.sh) 环境变量配置："
echo "   DATASOURCE_MGR_TLS_ENABLED=true \\"
echo "   DATASOURCE_MGR_TLS_CERT_FILE=$OUT_DIR/server.crt \\"
echo "   DATASOURCE_MGR_TLS_KEY_FILE=$OUT_DIR/server.key \\"
echo "   DATASOURCE_MGR_TLS_CA_FILE=$OUT_DIR/ca.crt \\"
echo "   DATASOURCE_MGR_TLS_CLIENT_AUTH=require \\"
echo "   DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE=$OUT_DIR/client.pub"
