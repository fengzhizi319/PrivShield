# PrivShield 生产安全运维手册

> **版本**：v16.0.0  
> **适用范围**：`PrivShield` 核心算力引擎（`engine`）、企业级中台微服务群（`service-hub` / `datasource-mgr` / `audit-log`）、控制台与双 BFF 体系（`bff-go` / `app-lz`）。  
> **定位**：生产环境部署、运维操作、证书生命周期管理、全栈防 DDoS 调优与故障排查指南。

---

## 目录

- [1. 环境变量速查表](#1-环境变量速查表)
  - [1.1 TLS / mTLS 传输安全](#11-tls--mtls-传输安全)
  - [1.2 认证鉴权与 CN 白名单](#12-认证鉴权与-cn-白名单)
  - [1.3 速率限制](#13-速率限制)
  - [1.4 健康检查豁免](#14-健康检查豁免)
  - [1.5 全栈防 DDoS 与微服务安全参数](#15-全栈防-ddos-与微服务安全参数)
- [2. 证书生成与配置（自签名开发与生产规范）](#2-证书生成与配置自签名开发与生产规范)
- [3. 本地启动示例](#3-本地启动示例)
  - [3.1 仅开启 TLS](#31-仅开启-tls)
  - [3.2 开启 TLS + mTLS + 认证 + 限速](#32-开启-tls--mtls--认证--限速)
- [4. 多语言与多协议调用示例](#4-多语言与多协议调用示例)
  - [4.1 REST (TLS + 外部 API Key)](#41-rest-tls--外部-api-key)
  - [4.2 REST (TLS + mTLS + 内部 API Key)](#42-rest-tls--mtls--内部-api-key)
  - [4.3 gRPC (Python 客户端 mTLS + 证书认证)](#43-grpc-python-客户端-mtls--证书认证)
- [5. Kubernetes 云原生探针与 Ingress 配置](#5-kubernetes-云原生探针与-ingress-配置)
- [6. 常见问题排错 (FAQ)](#6-常见问题排错-faq)

---

## 1. 环境变量速查表

### 1.1 TLS / mTLS 传输安全

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_TLS_ENABLED` | `false` | 是否启用 REST/gRPC TLS。 |
| `PRIVACY_TLS_CERT_FILE` / `PRIVACY_TLS_CERT_PATH` | — | 服务器证书 PEM 路径。 |
| `PRIVACY_TLS_KEY_FILE` / `PRIVACY_TLS_KEY_PATH` | — | 服务器私钥 PEM 路径。 |
| `PRIVACY_TLS_CA_FILE` / `PRIVACY_TLS_CA_PATH` | — | CA 证书 PEM 路径；`optional`/`require` 模式必需。 |
| `PRIVACY_TLS_CLIENT_AUTH` | `none` | 客户端认证模式：`none` / `optional` / `require`。 |
| `PRIVACY_TLS_KEY_PASSWORD` | — | 加密的私钥口令。 |

### 1.2 认证鉴权与 CN 白名单

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_AUTH_ENABLED` | `false` | 是否启用认证鉴权。 |
| `PRIVACY_AUTH_INTERNAL_KEYS_JSON` | `{}` | 内部服务 API Key 映射。 |
| `PRIVACY_AUTH_EXTERNAL_KEYS_JSON` | `{}` | 外部服务 API Key 映射。 |
| `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED` | `false` | gRPC 是否允许 mTLS 客户端证书作为内部服务（默认关闭，fail-closed）。 |
| `PRIVACY_AUTH_MTLS_WHITELIST_FILE` | — | mTLS CN 白名单 YAML 配置文件路径（推荐）。设置后启用 per-CN scope 控制与热重载。 |
| `PRIVACY_AUTH_MTLS_ALLOWED_CNS` | `[]` | mTLS 客户端证书 CN 静态白名单（JSON 数组或逗号分隔）。当 WHITELIST_FILE 未设置时使用，所有 CN 获得 `["*"]` 全权限。 |

JSON 格式示例：
```bash
PRIVACY_AUTH_INTERNAL_KEYS_JSON='{
  "sk-internal-abc": {"name": "service-hub", "scopes": ["*"]}
}'

PRIVACY_AUTH_EXTERNAL_KEYS_JSON='{
  "sk-external-xyz": {"name": "portal", "scopes": ["privacy:mask", "classification:read"]}
}'
```

### 1.3 速率限制

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_RATE_LIMIT_ENABLED` | `false` | 是否启用限速。 |
| `PRIVACY_RATE_LIMIT_DEFAULT_RPS` | `10` | 默认每秒请求数。 |
| `PRIVACY_RATE_LIMIT_DEFAULT_BURST` | `20` | 默认突发容量。 |
| `PRIVACY_RATE_LIMIT_PER_ENDPOINT_JSON` | `{}` | 按接口覆盖限速。 |
| `PRIVACY_RATE_LIMIT_REDIS_URL` | — | 多副本时共享计数器，例 `redis://redis:6379/0`。 |

覆盖示例：
```bash
PRIVACY_RATE_LIMIT_PER_ENDPOINT_JSON='{
  "/v1/privacy/dp/count": {"rps": 2, "burst": 5},
  "DPCount": {"rps": 2, "burst": 5}
}'
```

### 1.4 健康检查豁免

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_HEALTH_NO_AUTH` | `true` | `/health` 与 `Health` 是否免认证。 |
| `PRIVACY_HEALTH_NO_RATE_LIMIT` | `true` | `/health` 与 `Health` 是否免限速。 |

### 1.5 全栈防 DDoS 与微服务安全参数

| 参数 / 配置项 | 推荐生产配置 | 所在模块 / 位置 | 作用说明 |
|---|---|---|---|
| `ReadHeaderTimeout` | `5 * time.Second` | Go Server / main.go | 强制关闭缓慢发送 Header 的慢速连接 (Anti-Slowloris) |
| `ReadTimeout` | `30 * time.Second` | Go Server / main.go | 请求体完整读取超时 (Anti-Slow-POST) |
| `MaxHeaderBytes` | `1 << 20` (1 MiB) | Go Server / main.go | 限制最大 Header 字节数，防止超大头部耗尽内存 |
| `MaxBodySize` | `32 << 20` (32 MiB) / `64 MiB` | `pkg/middleware` & Python 网关 | 超过限制直接响应 413 Payload Too Large 切断连接 |
| `RateLimit(rps, burst)` | `(200, 400)` | `pkg/middleware` | 客户端 IP 令牌桶限流，超额返回 429 与 Retry-After |
| `MaxConcurrent(limit)` | `1000` | `pkg/middleware` | 全局在途请求信号量硬顶，过载快速响应 503 保护协程池 |
| `AUDIT_LOG_ENCRYPTION_KEY` | 32 字节高强度密钥 | `services/audit-log` | SM4-GCM 快照密文落盘信封加密 |
| `nginx.ingress.kubernetes.io/limit-rps` | `"100"` | Helm `values.yaml` Ingress | 云原生边缘层单 IP 速率限制 |
| `nginx.ingress.kubernetes.io/limit-connections` | `"50"` | Helm `values.yaml` Ingress | 云原生边缘层单 IP 并发连接数限制 |

---

## 2. 证书生成与配置（自签名开发与生产规范）

```bash
# 1. 生成 CA 根证书
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -sha256 -days 365 -out ca.crt \
  -subj "/CN=PrivShield-ca"

# 2. 生成服务器证书
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/CN=PrivShield"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 365 -sha256

# 3. 生成客户端证书（mTLS 用）
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr \
  -subj "/CN=internal-client"
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt -days 365 -sha256
```

---

## 3. 本地启动示例

### 3.1 仅开启 TLS

```bash
export PRIVACY_TLS_ENABLED=true
export PRIVACY_TLS_CERT_FILE=./certs/server.crt
export PRIVACY_TLS_KEY_FILE=./certs/server.key
python -m engine.server
```

REST 监听：`https://127.0.0.1:8079`；gRPC 监听：`127.0.0.1:50051`（需 gRPCs 通道）。

### 3.2 开启 TLS + mTLS + 认证 + 限速

```bash
export PRIVACY_TLS_ENABLED=true
export PRIVACY_TLS_CERT_FILE=./certs/server.crt
export PRIVACY_TLS_KEY_FILE=./certs/server.key
export PRIVACY_TLS_CA_FILE=./certs/ca.crt
export PRIVACY_TLS_CLIENT_AUTH=require

export PRIVACY_AUTH_ENABLED=true
# 启用 mTLS CN 白名单认证（默认关闭，需显式开启）
export PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true

# 方式一：YAML 白名单配置文件（推荐，支持 per-CN scope + 热重载）
export PRIVACY_AUTH_MTLS_WHITELIST_FILE=./config/mtls-whitelist.yaml

# 同时配置 API Key 作为 mTLS 的备选认证方式
export PRIVACY_AUTH_INTERNAL_KEYS_JSON='{"sk-internal":{"name":"service-hub","scopes":["*"]}}'
export PRIVACY_AUTH_EXTERNAL_KEYS_JSON='{"sk-external":{"name":"portal","scopes":["privacy:mask"]}}'

export PRIVACY_RATE_LIMIT_ENABLED=true
export PRIVACY_RATE_LIMIT_DEFAULT_RPS=10
export PRIVACY_RATE_LIMIT_DEFAULT_BURST=20
export PRIVACY_RATE_LIMIT_PER_ENDPOINT_JSON='{"/v1/privacy/dp/count":{"rps":2,"burst":5}}'

python -m engine.server
```

---

## 4. 多语言与多协议调用示例

### 4.1 REST (TLS + 外部 API Key)

```bash
curl --cacert certs/ca.crt \
  -H "Authorization: Bearer sk-external" \
  -X POST https://127.0.0.1:8079/v1/privacy/mask \
  -H "Content-Type: application/json" \
  -d '{"field_name":"mobile","value":"13812345678"}'
```

### 4.2 REST (TLS + mTLS + 内部 API Key)

```bash
curl --cacert certs/ca.crt \
  --cert certs/client.crt --key certs/client.key \
  -H "Authorization: Bearer sk-internal" \
  -X POST https://127.0.0.1:8079/v1/privacy/dp/count \
  -H "Content-Type: application/json" \
  -d '{"values":[1,0,1],"params":{"epsilon":1.0}}'
```

### 4.3 gRPC (Python 客户端 mTLS + 证书认证)

```python
import grpc
from engine import privacy_pb2, privacy_pb2_grpc

with open("certs/ca.crt", "rb") as f:
    ca = f.read()
with open("certs/client.crt", "rb") as f:
    client_cert = f.read()
with open("certs/client.key", "rb") as f:
    client_key = f.read()

creds = grpc.ssl_channel_credentials(
    root_certificates=ca,
    private_key=client_key,
    certificate_chain=client_cert,
)
with grpc.secure_channel("127.0.0.1:50051", creds) as channel:
    stub = privacy_pb2_grpc.PrivacyServiceStub(channel)
    # mTLS 证书 CN 命中白名单时自动获得内部身份
    resp = stub.Mask(
        privacy_pb2.MaskRequest(field_name="mobile", value="13812345678"),
        metadata=(("authorization", "Bearer sk-internal"),),
    )
    print(resp.result)
```

---

## 5. Kubernetes 云原生探针与 Ingress 配置

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8079
    scheme: HTTPS   # 若 TLS 开启
readinessProbe:
  httpGet:
    path: /health
    port: 8079
    scheme: HTTPS
```

保持 `PRIVACY_HEALTH_NO_AUTH=true`，探针无需携带 `Authorization` 头部。

---

## 6. 常见问题排错 (FAQ)

**Q: 开启 TLS 后本地 `curl http://...` 失败？**  
A: 使用 `https://` 并指定 `--cacert` 或在开发测试环境使用 `curl -k`。

**Q: mTLS 模式下客户端没有证书？**  
A: 服务端会直接拒绝 TLS 握手；请为客户端生成受信 CA 签发的证书，并在调用时携带。

**Q: 多副本限速不生效？**  
A: 默认使用进程内存计数器，副本间不共享。请配置 `PRIVACY_RATE_LIMIT_REDIS_URL`。

**Q: 外部服务访问了越权接口返回什么？**  
A: REST 返回 `403 Forbidden`，gRPC 返回 `PERMISSION_DENIED`。

**Q: 请求返回 413 Payload Too Large 是怎么回事？**  
A: 请求体大小超出了 `MaxBodySize` 防护上限（默认 32MB，BFF 默认 64MB）。请分批上传或检查是否有异常大包。

**Q: 请求返回 429 Too Many Requests 是怎么回事？**  
A: 触发了客户端 IP 令牌桶限流或身份限流。响应头中包含 `Retry-After: 1`，客户端应按照退避重试机制稍后再发。

**Q: 请求返回 503 Service Unavailable: Server is overloaded 是怎么回事？**  
A: 触发了 `MaxConcurrent` 并发容量硬顶熔断，表明当前系统在途处理连接数达到上限。可通过 K8s HPA 自动水平扩容增加 Pod 副本数分流。

**Q: 数据源上传 CSV 提示路径非法或行数超出上限？**  
A: 数据源探查与加载受 LFI 目录沙箱与 50,000 行安全边界保护。请确保上传文件为合法 `.csv` 文件且数据行数在限制范围内。