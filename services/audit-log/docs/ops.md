# 脱敏审计日志 — 运维手册

## 1. 开发模式

```bash
cd console/audit-log
bash run.sh
```

默认监听 `127.0.0.1:8084`。

## 2. 生产模式

### 2.1 直接运行

```bash
make build
AUDIT_LOG_HOST=0.0.0.0 AUDIT_LOG_PORT=8084 ./bin/audit-log
```

### 2.2 Docker

```bash
docker build -t privshield-audit-log .
docker run -d \
  --name audit-log \
  -p 8084:8084 \
  -e AUDIT_LOG_HOST=0.0.0.0 \
  -e PRIVACY_AGENT_REST_HOST=privshield-agent \
  -e PRIVACY_REST_PORT=8079 \
  -e AUDIT_LOG_MAX_ENTRIES=100000 \
  privshield-audit-log
```

### 2.3 Docker Compose

```yaml
  audit-log:
    build: ../../console/audit-log
    ports:
      - "8084:8084"
    environment:
      - AUDIT_LOG_HOST=0.0.0.0
      - PRIVACY_AGENT_REST_HOST=agent
      - PRIVACY_REST_PORT=8079
      - AUDIT_LOG_MAX_ENTRIES=100000
    depends_on:
      - agent
```

## 3. 环境变量速查

| 变量 | 默认值 | 说明 |
|---|---|---|
| `AUDIT_LOG_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `AUDIT_LOG_PORT` | `8084` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 上游 Agent 认证密钥 |
| `AUDIT_LOG_API_KEY` | (空) | 本模块入站 API Key（空=不鉴权） |
| `AUDIT_LOG_CORS_ORIGINS` | (空) | 允许的 CORS 来源，逗号分隔（空=`*`） |
| `AUDIT_LOG_DB_PATH` | (空) | SQLite 数据库路径（空=内存模式） |
| `AUDIT_LOG_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `AUDIT_LOG_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

## 4. 健康检查

```bash
curl http://127.0.0.1:8084/api/health
```

## 5. 日志与持久化

审计日志支持 SQLite 持久化（通过 `AUDIT_LOG_DB_PATH` 配置）。空路径时回退到内存模式。

完整性哈希已增强：将 `input_hash` + `output_hash` + `parameters` + `user` + `security_level` 全部纳入 SHA-256 计算，防止数据篡改。

Prometheus 指标端点: `GET /metrics`

## 6. 故障排查

| 现象 | 排查方向 |
|---|---|
| Agent unreachable | 检查 Agent 是否运行、端口是否正确 |
| 日志丢失 | 检查 `AUDIT_LOG_MAX_ENTRIES` 是否过小 |
| 完整性校验失败 | 检查日志是否被篡改、哈希计算逻辑是否正确 |
| 合规报告数据异常 | 检查时间周期参数、确认日志时间戳正确 |

## 7. 安全建议

- 审计日志服务应部署在独立安全域，与业务系统隔离
- 启用访问控制，仅允许授权用户查询审计日志
- 定期备份审计数据，防止数据丢失
- 启用完整性校验，确保存证不可篡改
- 生产环境建议启用 TLS 加密传输
