# 数据源管理 — 运维手册

## 1. 开发模式

```bash
cd console/datasource-mgr
bash run.sh
```

默认监听 `127.0.0.1:8083`。

## 2. 生产模式

### 2.1 直接运行

```bash
make build
DATASOURCE_MGR_HOST=0.0.0.0 DATASOURCE_MGR_PORT=8083 ./bin/datasource-mgr
```

### 2.2 Docker

```bash
docker build -t privshield-datasource-mgr .
docker run -d \
  --name datasource-mgr \
  -p 8083:8083 \
  -e DATASOURCE_MGR_HOST=0.0.0.0 \
  -e PRIVACY_AGENT_REST_HOST=privshield-agent \
  -e PRIVACY_REST_PORT=8079 \
  privshield-datasource-mgr
```

### 2.3 Docker Compose

```yaml
  datasource-mgr:
    build: ../../console/datasource-mgr
    ports:
      - "8083:8083"
    environment:
      - DATASOURCE_MGR_HOST=0.0.0.0
      - PRIVACY_AGENT_REST_HOST=agent
      - PRIVACY_REST_PORT=8079
    depends_on:
      - agent
```

## 3. 环境变量速查

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `DATASOURCE_MGR_PORT` | `8083` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 认证密钥 |

## 4. 健康检查

```bash
curl http://127.0.0.1:8083/api/health
```

## 5. 故障排查

| 现象 | 排查方向 |
|---|---|
| Agent unreachable | 检查 Agent 是否运行、端口是否正确 |
| 数据源连接失败 | 检查数据源 host/port 是否可达、防火墙规则 |
| 元数据查询失败 | 确认 Agent 的 `/v1/dynclassification/classify` 端点可用 |
