# 模拟数据源服务 (Mock Datasource Manager) — 运维手册

> 本文档提供 **数联天下 · 数盾 (`PrivShield`)** 模拟数据源模块（`services/datasource-mgr`）的启动、配置、mTLS 证书部署与接口联调说明。

---

## 1. 运行与启动

### 1.1 开发与调试模式

```bash
cd services/datasource-mgr
bash run.sh
```

默认同时启动：
- **HTTP REST**：`http://127.0.0.1:8083`
- **gRPC (insecure)**：`127.0.0.1:50053`

### 1.2 生产调试模式（启用 mTLS 与公钥固定）

```bash
# 编译二进制产物
cd services/datasource-mgr
make build

# 启动服务
DATASOURCE_MGR_HOST=0.0.0.0 \
DATASOURCE_MGR_PORT=8083 \
DATASOURCE_MGR_GRPC_HOST=0.0.0.0 \
DATASOURCE_MGR_GRPC_PORT=50053 \
DATASOURCE_MGR_TLS_ENABLED=true \
DATASOURCE_MGR_TLS_CERT_FILE=/certs/server.crt \
DATASOURCE_MGR_TLS_KEY_FILE=/certs/server.key \
DATASOURCE_MGR_TLS_CA_FILE=/certs/ca.crt \
DATASOURCE_MGR_TLS_CLIENT_AUTH=require \
DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE=/certs/client_pub.pem \
./bin/datasource-mgr
```

---

## 2. 环境变量速查表

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | HTTP REST 服务监听主机 |
| `DATASOURCE_MGR_PORT` | `8083` | HTTP REST 服务监听端口 |
| `DATASOURCE_MGR_GRPC_HOST` | `127.0.0.1` | gRPC 服务监听主机 |
| `DATASOURCE_MGR_GRPC_PORT` | `50053` | gRPC 服务监听端口 |
| `DATASOURCE_MGR_TLS_ENABLED` | `false` | 是否在 gRPC 服务上启用 TLS/mTLS |
| `DATASOURCE_MGR_TLS_CERT_FILE` | (空) | gRPC 服务端 X.509 证书 PEM 路径 |
| `DATASOURCE_MGR_TLS_KEY_FILE` | (空) | gRPC 服务端私钥 PEM 路径 |
| `DATASOURCE_MGR_TLS_CA_FILE` | (空) | 客户端证书校验 CA 证书 PEM 路径 |
| `DATASOURCE_MGR_TLS_CLIENT_AUTH` | (空) | 客户端认证模式: `require` \| `verify` \| `request` |
| `DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE` | (空) | 固定的客户端公钥 PEM 路径 |
| `DATASOURCE_MGR_API_KEY` | (空) | 本模块入站 API Key（空表示免密） |
| `DATASOURCE_MGR_CORS_ORIGINS` | (空) | 允许的 CORS 跨域源（逗号分隔） |
| `DATASOURCE_MGR_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `DATASOURCE_MGR_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

---

## 3. 快速联调与接口测试

### 3.1 申请医保模拟数据 (API 1)
```bash
curl -s "http://127.0.0.1:8083/api/v1/yibao?limit=5" | jq .
```

### 3.2 申请康养模拟数据 (API 2)
```bash
curl -s "http://127.0.0.1:8083/api/v1/kangyang?limit=5" | jq .
```

### 3.3 gRPC 探活与数据获取
```bash
# 探活
grpcurl -plaintext 127.0.0.1:50053 datasourcemgr.DataSourceManagerService/Health

# 获取医保数据
grpcurl -plaintext -d '{"limit": 5, "offset": 0}' 127.0.0.1:50053 datasourcemgr.DataSourceManagerService/GetYibaoData
```
