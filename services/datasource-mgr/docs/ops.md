# 数据源管理 (Datasource Manager) — 运维手册

> 本文档提供 **数联天下 · 数盾 (`PrivShield`)** 数据源管理模块（`services/datasource-mgr`）的部署、配置、mTLS 证书配置、监控与故障排查指南。

---

## 1. 运行与启动

### 1.1 开发模式

```bash
cd services/datasource-mgr
bash run.sh
```

默认同时启动：
- **HTTP REST**：`127.0.0.1:8083`
- **gRPC (insecure)**：`127.0.0.1:50053`

### 1.2 生产模式（启用 mTLS 与公钥固定）

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
DATASOURCE_MGR_TLS_CERT_FILE=/etc/privshield/certs/server.crt \
DATASOURCE_MGR_TLS_KEY_FILE=/etc/privshield/certs/server.key \
DATASOURCE_MGR_TLS_CA_FILE=/etc/privshield/certs/ca.crt \
DATASOURCE_MGR_TLS_CLIENT_AUTH=require \
DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE=/etc/privshield/certs/client_pub.pem \
DATASOURCE_MGR_DB_PATH=/data/datasources/datasources.db \
./bin/datasource-mgr
```

### 1.3 Docker 部署

```bash
docker build -t privshield-datasource-mgr -f services/datasource-mgr/Dockerfile .
docker run -d \
  --name datasource-mgr \
  -p 8083:8083 \
  -p 50053:50053 \
  -v /data/datasources:/data \
  -v /etc/privshield/certs:/certs:ro \
  -e DATASOURCE_MGR_HOST=0.0.0.0 \
  -e DATASOURCE_MGR_PORT=8083 \
  -e DATASOURCE_MGR_GRPC_HOST=0.0.0.0 \
  -e DATASOURCE_MGR_GRPC_PORT=50053 \
  -e DATASOURCE_MGR_TLS_ENABLED=true \
  -e DATASOURCE_MGR_TLS_CERT_FILE=/certs/server.crt \
  -e DATASOURCE_MGR_TLS_KEY_FILE=/certs/server.key \
  -e DATASOURCE_MGR_TLS_CA_FILE=/certs/ca.crt \
  -e DATASOURCE_MGR_TLS_CLIENT_AUTH=require \
  -e DATASOURCE_MGR_DB_PATH=/data/datasources.db \
  -e PRIVACY_AGENT_REST_HOST=privshield-agent \
  -e PRIVACY_REST_PORT=8079 \
  privshield-datasource-mgr
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
| `DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE` | (空) | 固定的客户端公钥 PEM 路径（公钥固定防御） |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 主机 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 上游 Agent 认证密钥 |
| `DATASOURCE_MGR_API_KEY` | (空) | 本模块入站 API Key（空表示免密） |
| `DATASOURCE_MGR_CORS_ORIGINS` | (空) | 允许的 CORS 跨域源（逗号分隔） |
| `DATASOURCE_MGR_DB_PATH` | (空) | SQLite 数据库路径（空表示纯内存模式） |
| `DATASOURCE_MGR_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `DATASOURCE_MGR_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

---

## 3. 健康检查与监控

### 3.1 HTTP 健康检查
```bash
curl -s http://127.0.0.1:8083/api/health | jq .
```

### 3.2 gRPC 健康检查
使用 `grpcurl` 工具：
```bash
# 明文连接模式
grpcurl -plaintext 127.0.0.1:50053 datasourcemgr.DataSourceManagerService/Health

# mTLS 认证模式
grpcurl -cacert /certs/ca.crt -cert /certs/client.crt -key /certs/client.key \
  127.0.0.1:50053 datasourcemgr.DataSourceManagerService/Health
```

### 3.3 Prometheus 监控
```bash
curl -s http://127.0.0.1:8083/metrics
```

---

## 4. 故障排查手册

| 故障现象 | 潜在原因 | 排查与修复方案 |
|---|---|---|
| **Agent unreachable** | Agent 进程未就绪或端口错误 | 检查 `curl http://127.0.0.1:8079/health` 是否正常；确认 `PRIVACY_AGENT_REST_HOST` 配置 |
| **gRPC Handshake Failed** | 客户端证书不匹配或 CA 未信任 | 检查 `DATASOURCE_MGR_TLS_CA_FILE` 是否包含签名 CA；验证证书有效期与域名/IP SAN |
| **client public key mismatch** | 客户端公钥与固定公钥文件不符 | 检查客户端证书公钥与 `DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE` 的一致性 |
| **file not found in allowed dir** | CSV 文件名包含非法路径或不在白名单中 | 确认样本文件位于 `samples/`、`data/` 等白名单目录中，防止路径穿越 |
