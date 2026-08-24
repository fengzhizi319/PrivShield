# 数据服务调度中枢 — 运维手册

## 1. 开发模式

```bash
cd services/service-hub
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
docker build -f services/service-hub/Dockerfile -t privshield-service-hub .
docker run -d \
  --name service-hub \
  -p 8082:8082 \
  -e SERVICE_HUB_HOST=0.0.0.0 \
  -e PRIVACY_AGENT_REST_HOST=privshield-agent \
  -e PRIVACY_REST_PORT=8079 \
  privshield-service-hub
```

### 2.3 Docker Compose

在 `deploy/docker-compose/docker-compose.prod.yml` 中已内置该微服务：

```bash
docker compose -f deploy/docker-compose/docker-compose.prod.yml up -d service-hub
```

## 3. Prometheus 指标与 Grafana 监控大屏

### 3.1 Prometheus 指标采集端点
Service Hub 暴露标准 Prometheus 指标：`GET /metrics`：
* `http_requests_total{module="service-hub"}`：HTTP 调度请求数（按 path/status）
* `http_request_duration_seconds{module="service-hub"}`：调度延迟直方图
* `agent_requests_total{module="service-hub"}`：上游 Agent 算力调用总数（按 endpoint/status）
* `agent_request_duration_seconds{module="service-hub"}`：上游 Agent 算力调用延迟直方图

### 3.2 专属 Grafana 监控大屏
预置监控大屏模板位于：
* **[deploy/grafana/service-hub-dashboard.json](../../../deploy/grafana/service-hub-dashboard.json)**（Service Hub 专属流水线调度大屏）
* **[deploy/grafana/dashboard.json](../../../deploy/grafana/dashboard.json)**（全平台联合监控总览）

一键启动监控栈：
```bash
docker compose -f deploy/docker-compose/docker-compose.prod.yml --profile monitoring up -d
# 访问 Grafana: http://localhost:3000
```

## 4. 环境变量速查

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `SERVICE_HUB_PORT` | `8082` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 上游 Agent 认证密钥 |
| `SERVICE_HUB_MAX_QUEUE` | `1000` | 最大队列深度 |
| `SERVICE_HUB_SCHEDULE_TIMEOUT` | `30` | 调度超时（秒） |
| `SERVICE_HUB_API_KEY` | (空) | 本模块入站 API Key（空=不鉴权） |
| `SERVICE_HUB_CORS_ORIGINS` | (空) | 允许的 CORS 来源，逗号分隔（空=`*`） |
| `SERVICE_HUB_DB_PATH` | (空) | SQLite 数据库路径（空=内存模式） |
| `SERVICE_HUB_LOG_FORMAT` | `json` | 日志格式: `json` \| `text` |
| `SERVICE_HUB_LOG_LEVEL` | `info` | 日志级别: `debug` \| `info` \| `warn` \| `error` |

## 4. 健康检查

```bash
curl http://127.0.0.1:8082/api/health
```

## 5. 日志

使用 Go 标准 `log/slog` 结构化日志，默认 JSON 格式输出。可通过 `SERVICE_HUB_LOG_FORMAT=text` 切换为文本格式。

Prometheus 指标端点: `GET /metrics`

## 6. 故障排查

| 现象 | 排查方向 |
|---|---|
| Agent unreachable | 检查 Agent 是否运行、端口是否正确 |
| 任务一直 pending | 检查队列深度限制、goroutine 是否泄漏 |
| 分类分级失败 | 确认 Agent 的 `/v1/dynclassification/classify` 端点可用 |
