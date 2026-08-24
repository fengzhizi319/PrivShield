# 数据服务调度中枢 (Service Hub) — API 规范

`service-hub` 是 PrivShield 平台的流水线调度中枢，负责串联 **模拟数据源 (datasource-mgr)**、**隐私与分类引擎 (PrivShield Agent)** 与 **审计存证 (audit-log)**，支持 **REST (HTTP/JSON :8082) + gRPC (mTLS :50052)** 双协议接入。

---

## 1. 通信协议与端口规划

| 协议 | 默认地址 | 认证方式 | 说明 |
|---|---|---|---|
| **HTTP REST** | `http://127.0.0.1:8082` | Bearer Token / API Key | 供 React 前端与 BFF 交互 |
| **gRPC (mTLS)** | `127.0.0.1:50052` | 双向 TLS (mTLS) + 公钥固定 | 供调度客户端高性能提交与编排任务 |

---

## 2. HTTP REST 接口规范

### 2.1 健康检查与状态概览

#### `GET /api/health`
- **说明**：检查自身、上游 PrivShield Agent 以及下游模拟数据源 `datasource-mgr` 的健康状态与连通性。
- **响应示例**：
```json
{
  "backend": "ok",
  "agent": {"status": "ok", "namespace": "default"},
  "agent_url": "http://127.0.0.1:8079",
  "datasource": "ok",
  "datasource_url": "http://127.0.0.1:8083",
  "latency_ms": 3,
  "via": "service-hub"
}
```

#### `GET /api/hub/status`
- **说明**：返回调度中枢状态概览（运行时长、排队/运行/完成任务数、Agent 与 Datasource 连接地址）。

---

### 2.2 流水线调度与数据源联动

#### `POST /api/hub/pipeline/trigger-datasource`
- **说明**：一键联动 `datasource-mgr` 申请数据并执行脱敏流水线调度。
- **请求体**：
```json
{
  "datasource_id": "ds_yibao",
  "limit": 10,
  "operation": "mask"
}
```
- **响应示例**：
```json
{
  "task_id": "task-1787554500-eabf3934",
  "datasource_id": "ds_yibao",
  "records_count": 10,
  "operation": "mask",
  "status": "accepted",
  "via": "service-hub"
}
```

#### `GET /api/hub/datasources`
- **说明**：代理获取 `datasource-mgr` 当前已注册的所有模拟数据源（医保 `ds_yibao`、康养 `ds_kangyang` 及预留数据源 3/4）。

#### `POST /api/hub/dispatch`
- **说明**：手动分发任务到调度流水线。
- **请求体**：
```json
{
  "source": "ds_yibao",
  "operation": "mask",
  "payload": {"name": "张三", "id_card": "510101199001011234"},
  "priority": 50
}
```

#### `POST /api/hub/classify`
- **说明**：先调用 Agent 动态分类漏斗评定敏感度等级（L1~L5），并根据策略自动选择脱敏原语分发执行。

#### `GET /api/hub/pipeline`
- **说明**：返回 6 大流水线阶段（`ingest` ➔ `fetch` ➔ `classify` ➔ `desensitize` ➔ `return` ➔ `audit`）的实时活跃任务数。
