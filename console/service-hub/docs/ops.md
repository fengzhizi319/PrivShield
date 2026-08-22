# 数据服务调度中枢 — 运维手册

## 1. 开发模式

```bash
cd console/service-hub
bash run.sh
```

默认监听 `127.0.0.1:8082`，上游 Agent 默认连接 `127.0.0.1:8079`。

## 2. 生产模式

### 2.1 直接运行

```bash
make build
SERVICE_HUB_HOST=0.0.0.0 SERVICE_HUB_PORT=8082 ./bin/service-hub
```

### 2.2 Docker

```bash
docker build -t privshield-service-hub .
docker run -d \
  --name service-hub \
  -p 8082:8082 \
  -e SERVICE_HUB_HOST=0.0.0.0 \
  -e PRIVACY_AGENT_REST_HOST=privshield-agent \
  -e PRIVACY_REST_PORT=8079 \
  privshield-service-hub
```

### 2.3 Docker Compose

在 `deploy/docker-compose/docker-compose.yml` 中添加：

```yaml
  service-hub:
    build: ../../console/service-hub
    ports:
      - "8082:8082"
    environment:
      - SERVICE_HUB_HOST=0.0.0.0
      - PRIVACY_AGENT_REST_HOST=agent
      - PRIVACY_REST_PORT=8079
    depends_on:
      - agent
```

## 3. 环境变量速查

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `SERVICE_HUB_PORT` | `8082` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 认证密钥 |
| `SERVICE_HUB_MAX_QUEUE` | `1000` | 最大队列深度 |
| `SERVICE_HUB_SCHEDULE_TIMEOUT` | `30` | 调度超时（秒） |

## 4. 健康检查

```bash
curl http://127.0.0.1:8082/api/health
```

## 5. 日志

当前使用标准 `log` 包输出，生产环境建议接入结构化日志（如 zerolog / zap）。

## 6. 故障排查

| 现象 | 排查方向 |
|---|---|
| Agent unreachable | 检查 Agent 是否运行、端口是否正确 |
| 任务一直 pending | 检查队列深度限制、goroutine 是否泄漏 |
| 分类分级失败 | 确认 Agent 的 `/v1/dynclassification/classify` 端点可用 |
