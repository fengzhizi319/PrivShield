# 脱敏审计日志与存证 (Audit Log) — 运维手册

> 本文档提供 **数联天下 · 数盾 (`PrivShield`)** 脱敏审计日志模块（`services/audit-log`）的部署、配置、mTLS 证书配置、监控与故障排查指南。

---

## 1. 运行与启动

### 1.1 开发模式

```bash
cd services/audit-log
bash run.sh
```

默认同时启动：
- **HTTP REST**：`127.0.0.1:8084`
- **gRPC (insecure)**：`127.0.0.1:50054`

### 1.2 生产模式（启用 mTLS 与公钥固定）

```bash
# 编译二进制产物
cd services/audit-log
make build

# 启动服务
AUDIT_LOG_HOST=0.0.0.0 \
AUDIT_LOG_PORT=8084 \
AUDIT_LOG_GRPC_HOST=0.0.0.0 \
AUDIT_LOG_GRPC_PORT=50054 \
AUDIT_LOG_TLS_ENABLED=true \
AUDIT_LOG_TLS_CERT_FILE=/etc/privshield/certs/server.crt \
AUDIT_LOG_TLS_KEY_FILE=/etc/privshield/certs/server.key \
AUDIT_LOG_TLS_CA_FILE=/etc/privshield/certs/ca.crt \
AUDIT_LOG_TLS_CLIENT_AUTH=require \
AUDIT_LOG_TLS_PINNED_PUBKEY_FILE=/etc/privshield/certs/client_pub.pem \
AUDIT_LOG_DB_PATH=/data/audit/audit.db \
./bin/audit-log
```

### 1.3 Docker 部署

```bash
docker build -t privshield-audit-log -f services/audit-log/Dockerfile .
docker run -d \
  --name audit-log \
  -p 8084:8084 \
  -p 50054:50054 \
  -v /data/audit:/app/data \
  -v /etc/privshield/certs:/certs:ro \
  -e AUDIT_LOG_HOST=0.0.0.0 \
  -e AUDIT_LOG_PORT=8084 \
  -e AUDIT_LOG_GRPC_HOST=0.0.0.0 \
  -e AUDIT_LOG_GRPC_PORT=50054 \
  -e AUDIT_LOG_TLS_ENABLED=true \
  -e AUDIT_LOG_TLS_CERT_FILE=/certs/server.crt \
  -e AUDIT_LOG_TLS_KEY_FILE=/certs/server.key \
  -e AUDIT_LOG_TLS_CA_FILE=/certs/ca.crt \
  -e AUDIT_LOG_TLS_CLIENT_AUTH=require \
  -e AUDIT_LOG_DB_PATH=/app/data/audit-log.db \
  -e PRIVACY_AGENT_REST_HOST=privshield-agent \
  -e PRIVACY_REST_PORT=8079 \
  privshield-audit-log
```

---

## 2. 环境变量速查表

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `AUDIT_LOG_HOST` | `127.0.0.1` | HTTP REST 服务监听主机 |
| `AUDIT_LOG_PORT` | `8084` | HTTP REST 服务监听端口 |
| `AUDIT_LOG_GRPC_HOST` | `127.0.0.1` | gRPC 服务监听主机 |
| `AUDIT_LOG_GRPC_PORT` | `50054` | gRPC 服务监听端口 |
| `AUDIT_LOG_TLS_ENABLED` | `false` | 是否在 gRPC 服务上启用 TLS/mTLS |
| `AUDIT_LOG_TLS_CERT_FILE` | (空) | gRPC 服务端 X.509 证书 PEM 路径 |
| `AUDIT_LOG_TLS_KEY_FILE` | (空) | gRPC 服务端私钥 PEM 路径 |
| `AUDIT_LOG_TLS_CA_FILE` | (空) | 客户端证书校验 CA 证书 PEM 路径 |
| `AUDIT_LOG_TLS_CLIENT_AUTH` | (空) | 客户端认证模式: `require` \| `verify` \| `request` |
| `AUDIT_LOG_TLS_PINNED_PUBKEY_FILE` | (空) | 固定的客户端公钥 PEM 路径（公钥固定防御） |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 主机 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 上游 Agent 认证密钥 |
| `AUDIT_LOG_API_KEY` | (空) | 本模块入站 API Key（空表示免密） |
| `AUDIT_LOG_CORS_ORIGINS` | (空) | 允许的 CORS 跨域源（逗号分隔） |
| `AUDIT_LOG_DB_PATH` | (空) | SQLite 数据库路径（空表示纯内存模式） |
| `AUDIT_LOG_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `AUDIT_LOG_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

---

## 3. 健康检查与验证

### 3.1 HTTP 健康检查
```bash
curl -s http://127.0.0.1:8084/api/health | jq .
```

### 3.2 gRPC 健康检查与探活
使用 `grpcurl` 工具：
```bash
# 明文连接模式
grpcurl -plaintext 127.0.0.1:50054 auditlog.AuditLogService/Health

# mTLS 认证模式
grpcurl -cacert /certs/ca.crt -cert /certs/client.crt -key /certs/client.key \
  127.0.0.1:50054 auditlog.AuditLogService/Health
```

### 3.3 Prometheus 监控
```bash
curl -s http://127.0.0.1:8084/metrics
```

---

## 4. 故障排查手册

| 故障现象 | 潜在原因 | 排查与修复方案 |
|---|---|---|
| **Agent unreachable** | Agent 进程未就绪或端口错误 | 检查 `curl http://127.0.0.1:8079/health`；确认 `PRIVACY_AGENT_REST_HOST` 配置 |
| **gRPC Handshake Failed** | 客户端证书不匹配或 CA 未信任 | 检查 `AUDIT_LOG_TLS_CA_FILE` 是否包含签名 CA；验证证书有效期 |
| **client public key mismatch** | 客户端公钥与固定公钥文件不符 | 检查客户端证书公钥与 `AUDIT_LOG_TLS_PINNED_PUBKEY_FILE` 的一致性 |
| **Integrity Violation** | 审计数据遭受篡改或底层 SQLite 损坏 | 调用 `/api/audit/snapshots/verify` 定位异常 snapshot_id，排查文件篡改 |
