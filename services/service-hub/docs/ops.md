# 数据服务调度中枢 (Service Hub) — 运维手册

> 本文档提供 **数联天下 · 数盾 (`PrivShield`)** 数据服务调度中枢模块（`services/service-hub`）的部署、配置、mTLS 证书配置、数据源联动监控与故障排查指南。

---

## 1. 运行与启动

### 1.1 开发模式

```bash
cd services/service-hub
bash run.sh
```

默认同时启动：
- **HTTP REST**：`http://127.0.0.1:8082`
- **gRPC (insecure)**：`127.0.0.1:50052`
- 上游 Agent 默认连接：`http://127.0.0.1:8079`
- 模拟数据源默认连接：`http://127.0.0.1:8083`

### 1.2 生产模式（启用 mTLS 与公钥固定）

```bash
# 编译二进制产物
cd services/service-hub
make build

# 启动服务
SERVICE_HUB_HOST=0.0.0.0 \
SERVICE_HUB_PORT=8082 \
SERVICE_HUB_GRPC_HOST=0.0.0.0 \
SERVICE_HUB_GRPC_PORT=50052 \
SERVICE_HUB_TLS_ENABLED=true \
SERVICE_HUB_TLS_CERT_FILE=/etc/privshield/certs/server.crt \
SERVICE_HUB_TLS_KEY_FILE=/etc/privshield/certs/server.key \
SERVICE_HUB_TLS_CA_FILE=/etc/privshield/certs/ca.crt \
SERVICE_HUB_TLS_CLIENT_AUTH=require \
SERVICE_HUB_TLS_PINNED_PUBKEY_FILE=/etc/privshield/certs/client_pub.pem \
DATASOURCE_MGR_HOST=127.0.0.1 \
DATASOURCE_MGR_PORT=8083 \
PRIVACY_AGENT_REST_HOST=127.0.0.1 \
PRIVACY_REST_PORT=8079 \
./bin/service-hub
```

---

## 2. 环境变量速查表

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_HOST` | `127.0.0.1` | HTTP REST 服务监听主机 |
| `SERVICE_HUB_PORT` | `8082` | HTTP REST 服务监听端口 |
| `SERVICE_HUB_GRPC_HOST` | `127.0.0.1` | gRPC 服务监听主机 |
| `SERVICE_HUB_GRPC_PORT` | `50052` | gRPC 服务监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 主机 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | 模拟数据源 HTTP 主机 |
| `DATASOURCE_MGR_PORT` | `8083` | 模拟数据源 HTTP 端口 |
| `DATASOURCE_MGR_GRPC_HOST` | `127.0.0.1` | 模拟数据源 gRPC 主机 |
| `DATASOURCE_MGR_GRPC_PORT` | `50053` | 模拟数据源 gRPC 端口 |
| `SERVICE_HUB_TLS_ENABLED` | `false` | 是否在 gRPC 服务上启用 TLS/mTLS |
| `SERVICE_HUB_TLS_CERT_FILE` | (空) | gRPC 服务端 X.509 证书 PEM 路径 |
| `SERVICE_HUB_TLS_KEY_FILE` | (空) | gRPC 服务端私钥 PEM 路径 |
| `SERVICE_HUB_TLS_CA_FILE` | (空) | 客户端证书校验 CA 证书 PEM 路径 |
| `SERVICE_HUB_TLS_CLIENT_AUTH` | (空) | 客户端认证模式: `require` \| `verify` \| `request` |
| `SERVICE_HUB_TLS_PINNED_PUBKEY_FILE` | (空) | 固定的客户端公钥 PEM 路径 |
| `SERVICE_HUB_API_KEY` | (空) | 本模块入站 API Key（空表示免密） |
| `SERVICE_HUB_CORS_ORIGINS` | (空) | 允许的 CORS 跨域源（逗号分隔） |
| `SERVICE_HUB_DB_PATH` | (空) | SQLite 数据库路径（空表示纯内存模式） |
| `SERVICE_HUB_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `SERVICE_HUB_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

---

## 3. 健康检查与运维监控

### 3.1 HTTP 综合健康检查
```bash
curl -s http://127.0.0.1:8082/api/health | jq .
```
可同时观测：
- 调度中枢自身状态 (`backend: "ok"`)
- 上游 Agent 连通性与命名空间 (`agent: {"status": "ok"}`)
- 下游模拟数据源连通性 (`datasource: "ok"`)

### 3.2 触发数据源脱敏调度流水线
```bash
curl -s -X POST http://127.0.0.1:8082/api/hub/pipeline/trigger-datasource \
  -H "Content-Type: application/json" \
  -d '{"datasource_id": "ds_yibao", "limit": 5, "operation": "mask"}' | jq .
```
