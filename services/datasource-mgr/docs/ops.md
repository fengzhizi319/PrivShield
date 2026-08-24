# 模拟数据源服务 (Mock Datasource Manager) — 运维手册

> 本文档提供 **数联天下 · 数盾 (`PrivShield`)** 模拟数据源模块（`services/datasource-mgr`）的启动、配置、mTLS 证书部署与接口联调说明。

---

## 1. 运行与启动脚本

### 1.1 开发模式 (Insecure / No-mTLS)

```bash
cd services/datasource-mgr
bash scripts/dev-run.sh
# 或
make dev
```

默认同时启动：
- **HTTP REST**：`http://127.0.0.1:8083`
- **gRPC (insecure)**：`127.0.0.1:50053`

### 1.2 生产加固模式 (mTLS + 公钥固定)

```bash
cd services/datasource-mgr
bash scripts/prod-run.sh
# 或
make prod
```

自动加载 `services/datasource-mgr/certs/` 目录中的测试证书与客户端固定公钥 `client.pub`。

### 1.3 重新生成测试证书链

```bash
cd services/datasource-mgr
bash scripts/gen-certs.sh
# 或
make gen-certs
```

---

## 2. 环境变量速查表

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | HTTP/HTTPS REST 服务监听主机 |
| `DATASOURCE_MGR_PORT` | `8083` | HTTP/HTTPS REST 服务监听端口 |
| `DATASOURCE_MGR_GRPC_HOST` | `127.0.0.1` | gRPC 服务监听主机 |
| `DATASOURCE_MGR_GRPC_PORT` | `50053` | gRPC 服务监听端口 |
| `DATASOURCE_MGR_TLS_ENABLED` | `false` | 是否在 HTTP REST 与 gRPC 服务上启用 TLS 1.3 / mTLS |
| `DATASOURCE_MGR_TLS_CERT_FILE` | (空) | 服务端 X.509 证书 PEM 路径 |
| `DATASOURCE_MGR_TLS_KEY_FILE` | (空) | 服务端私钥 PEM 路径 |
| `DATASOURCE_MGR_TLS_CA_FILE` | (空) | 客户端证书校验 CA 证书 PEM 路径 |
| `DATASOURCE_MGR_TLS_CLIENT_AUTH` | (空) | 客户端认证模式: `require` \| `verify` \| `request` |
| `DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE` | (空) | 固定的客户端公钥 PEM 路径 (SPKI Pinning) |
| `DATASOURCE_MGR_API_KEY` | (空) | 本模块入站 API Key（空表示免密） |
| `DATASOURCE_MGR_CORS_ORIGINS` | (空) | 允许的 CORS 跨域源（逗号分隔） |
| `DATASOURCE_MGR_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `DATASOURCE_MGR_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

---

## 3. 接口快速验证与联调

### 3.1 HTTP 综合健康检查（开发模式）
```bash
curl -s http://127.0.0.1:8083/api/health | jq .
```

### 3.2 HTTPS 双向认证 (mTLS) 调取示例（生产加固模式）
```bash
# 携带 CA 根证书与已固定公钥的客户端证书访问 HTTPS REST API
curl -s --cacert certs/ca.crt \
  --cert certs/client.crt \
  --key certs/client.key \
  https://127.0.0.1:8083/api/v1/yibao?limit=5 | jq .
```

### 3.3 申请 API 2 康养数据
```bash
curl -s "http://127.0.0.1:8083/api/v1/kangyang?limit=5" | jq .
```
