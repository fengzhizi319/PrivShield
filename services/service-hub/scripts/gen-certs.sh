#!/usr/bin/env bash
# ============================================================================
# Generate mTLS test certificate chain for service-hub gRPC server.
# 为 service-hub gRPC 服务端生成 mTLS 测试证书链。
#
# Generated files (default output: console/service-hub/certs/) / 生成的文件：
#   ca.crt / ca.key                 Trusted root CA
#                                   受信任根 CA
#   server.crt / server.key         Server cert (SAN: localhost/127.0.0.1)
#                                   服务端证书
#   client.crt / client.key         Client cert (EKU: clientAuth)
#                                   客户端证书
#   client.pub                      Client public key (for pinning)
#                                   客户端公钥（用于固定）
#
# Usage / 用法：
#   ./scripts/gen-certs.sh [output_dir]
#
# WARNING: Generated certs are for testing/dev ONLY. Do NOT use in production.
# 注意：生成的证书仅用于测试/开发，请勿用于生产环境。
# ============================================================================

set -euo pipefail

OUT_DIR="${1:-$(dirname "$0")/../certs}"
DAYS="${CERT_DAYS:-365}"

# Convert to absolute path
# 转换为绝对路径
OUT_DIR="$(cd "$(dirname "$OUT_DIR")" && pwd)/$(basename "$OUT_DIR")"

mkdir -p "$OUT_DIR"
cd "$OUT_DIR"

echo ">> 生成 mTLS 测试证书到: $OUT_DIR"
echo "   有效期: $DAYS 天"
echo ""

# ── 1. 根 CA ─────────────────────────────────────────────────────────────
echo ">> [1/4] 生成根 CA..."
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days "$DAYS" \
    -out ca.crt -subj "/CN=service-hub-test-ca"

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
openssl req -new -key client.key -subj "/CN=service-hub-client" -out client.csr

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

# ── 清理中间文件并收紧私钥权限 ────────────────────────────────────────────
rm -f server.csr client.csr server.ext client.ext ca.srl
chmod 600 ./*.key

echo ""
echo ">> 完成，生成文件："
ls -1 "$OUT_DIR"/*.crt "$OUT_DIR"/*.key "$OUT_DIR"/*.pub 2>/dev/null || ls -1 "$OUT_DIR"
echo ""
echo ">> Service Hub gRPC 服务端启用 mTLS："
echo "   SERVICE_HUB_TLS_ENABLED=true \\"
echo "   SERVICE_HUB_TLS_CERT_FILE=$OUT_DIR/server.crt \\"
echo "   SERVICE_HUB_TLS_KEY_FILE=$OUT_DIR/server.key \\"
echo "   SERVICE_HUB_TLS_CA_FILE=$OUT_DIR/ca.crt \\"
echo "   SERVICE_HUB_TLS_CLIENT_AUTH=require"
echo ""
echo ">> 公钥固定（额外安全层）："
echo "   SERVICE_HUB_TLS_PINNED_PUBKEY_FILE=$OUT_DIR/client.pub"
echo ""
echo ">> 客户端连接配置："
echo "   使用 client.crt + client.key 作为客户端凭证"
echo "   CA 证书 ca.crt 用于验证服务端身份"
