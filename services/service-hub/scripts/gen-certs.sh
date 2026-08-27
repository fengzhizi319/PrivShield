#!/usr/bin/env bash
# ============================================================================
# Generate mTLS test certificate chain for service-hub gRPC server.
# 为 service-hub gRPC 服务端生成 mTLS 双向认证测试证书链。
#
# 生成的证书与密钥文件（默认输出目录：services/service-hub/certs/）：
#   ca.crt / ca.key                 受信任根证书授权机构（Root CA，4096-bit RSA）
#   server.crt / server.key         服务端证书（SAN 扩展支持 localhost 与 127.0.0.1）
#   client.crt / client.key         客户端证书（包含 clientAuth 扩展密钥用法）
#   client.pub                      客户端公钥 PEM 文件（用于服务端公钥固定比对校验）
#
# 执行流程：
#   1. 生成自签名 4096-bit RSA 根 CA 证书及私钥；
#   2. 生成服务端私钥与 CSR，并通过 CA 签发包含 SAN 扩展的服务端证书；
#   3. 生成客户端私钥与 CSR，并通过 CA 签发包含 clientAuth 扩展的客户端证书；
#   4. 从客户端私钥中导出 SubjectPublicKeyInfo 格式公钥文件（client.pub）；
#   5. 清理中间 CSR 与配置文件，将所有私钥权限收紧至 600（仅属主可读写）。
#
# 使用方法：
#   ./scripts/gen-certs.sh [output_dir]
#
# 环境变量：
#   CERT_DAYS: 证书有效天数（默认 365 天）
#
# 注意：生成的自签名证书仅用于本地开发与单测验证，请勿直接部署于生产环境。
# ============================================================================

set -euo pipefail

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    echo "用法 / Usage: $0 [输出目录 / output_dir]"
    echo ""
    echo "选项 / Options:"
    echo "  -h, --help    显示帮助信息并退出"
    echo ""
    echo "环境变量 / Env vars:"
    echo "  CERT_DAYS     证书有效天数 (默认: 365)"
    exit 0
fi

OUT_DIR="${1:-$(dirname "$0")/../certs}"
DAYS="${CERT_DAYS:-365}"

# 转换为标准化绝对路径
OUT_DIR="$(cd "$(dirname "$OUT_DIR")" && pwd)/$(basename "$OUT_DIR")"

mkdir -p "$OUT_DIR"
cd "$OUT_DIR"

echo ">> 生成 mTLS 测试证书到: $OUT_DIR"
echo "   有效期: $DAYS 天"
echo ""

# ── 1. 生成自签名根 CA ────────────────────────────────────────────────────────
echo ">> [1/4] 生成根 CA (4096-bit RSA)..."
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days "$DAYS" \
    -out ca.crt -subj "/CN=service-hub-test-ca"

# ── 2. 生成服务端证书与私钥 ────────────────────────────────────────────────────
echo ">> [2/4] 生成服务端证书（SAN: localhost / 127.0.0.1）..."
openssl genrsa -out server.key 2048
openssl req -new -key server.key -subj "/CN=localhost" -out server.csr

# 写入服务端 X.509 V3 扩展（包含 SAN 与 serverAuth）
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

# ── 3. 生成客户端证书与私钥 ────────────────────────────────────────────────────
echo ">> [3/4] 生成客户端证书（EKU: clientAuth）..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -subj "/CN=service-hub-client" -out client.csr

# 写入客户端 X.509 V3 扩展（包含 clientAuth）
cat > client.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
EOF

openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out client.crt -days "$DAYS" -sha256 -extfile client.ext

# ── 4. 导出客户端公钥（用于服务端 Public Key Pinning）───────────────────────────
echo ">> [4/4] 提取客户端公钥（用于公钥固定）..."
openssl rsa -in client.key -pubout -out client.pub

# ── 5. 清理中间临时文件并收紧私钥文件系统权限 ─────────────────────────────────
rm -f server.csr client.csr server.ext client.ext ca.srl
chmod 600 ./*.key

echo ""
echo ">> 证书生成完成，生成清单："
ls -1 "$OUT_DIR"/*.crt "$OUT_DIR"/*.key "$OUT_DIR"/*.pub 2>/dev/null || ls -1 "$OUT_DIR"
echo ""
echo ">> Service Hub gRPC 服务端启用 mTLS 配置参考："
echo "   SERVICE_HUB_TLS_ENABLED=true \\"
echo "   SERVICE_HUB_TLS_CERT_FILE=$OUT_DIR/server.crt \\"
echo "   SERVICE_HUB_TLS_KEY_FILE=$OUT_DIR/server.key \\"
echo "   SERVICE_HUB_TLS_CA_FILE=$OUT_DIR/ca.crt \\"
echo "   SERVICE_HUB_TLS_CLIENT_AUTH=require"
echo ""
echo ">> 公钥固定配置参考（额外安全加固）："
echo "   SERVICE_HUB_TLS_PINNED_PUBKEY_FILE=$OUT_DIR/client.pub"
echo ""
echo ">> 客户端连接配置参考："
echo "   使用 client.crt + client.key 作为客户端双向认证凭证"
echo "   CA 证书 ca.crt 用于验证服务端证书有效性"

