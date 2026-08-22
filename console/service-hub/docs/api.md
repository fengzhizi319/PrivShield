# 数据服务调度中枢 — API 参考

## 基础信息

- 默认地址：`http://127.0.0.1:8082`
- 数据格式：JSON
- 认证：可选 Bearer Token（通过 `PRIVACY_AGENT_API_KEY` 环境变量配置）

---

## GET /api/health

健康检查，返回自身状态与上游 Agent 连通性。

**响应示例：**

```json
{
  "backend": "ok",
  "agent": {"status": "ok"},
  "agent_url": "http://127.0.0.1:8079",
  "latency_ms": 5,
  "via": "service-hub"
}
```

---

## GET /api/hub/status

返回调度中枢状态概览。

**响应示例：**

```json
{
  "status": "running",
  "uptime": "2h30m15s",
  "active_tasks": 3,
  "queued_tasks": 1,
  "completed_total": 150,
  "failed_total": 2,
  "agent_url": "http://127.0.0.1:8079"
}
```

---

## GET /api/hub/tasks

列出所有任务，可选按状态过滤。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `status` | string | 否 | 过滤状态：pending / running / completed / failed |

**响应示例：**

```json
{
  "total": 2,
  "tasks": [
    {
      "id": "task-1724300000-1",
      "status": "completed",
      "stage": "done",
      "source": "卫健数据",
      "operation": "mask",
      "created_at": "2026-08-22T10:00:00Z",
      "duration_ms": 620
    }
  ],
  "via": "service-hub"
}
```

---

## POST /api/hub/dispatch

分发新任务到调度流水线。

**请求体：**

```json
{
  "source": "卫健数据",
  "operation": "mask",
  "payload": {"field_name": "name", "value": "张三"},
  "priority": 50
}
```

**响应：**

```json
{
  "task_id": "task-1724300000-1",
  "status": "accepted",
  "via": "service-hub"
}
```

---

## GET /api/hub/pipeline

返回流水线各阶段实时状态。

**响应示例：**

```json
{
  "stages": [
    {"name": "ingest", "status": "idle", "active_count": 0},
    {"name": "fetch", "status": "processing", "active_count": 1},
    {"name": "classify", "status": "idle", "active_count": 0},
    {"name": "desensitize", "status": "idle", "active_count": 0},
    {"name": "return", "status": "idle", "active_count": 0},
    {"name": "audit", "status": "idle", "active_count": 0}
  ],
  "agent_ok": true
}
```

---

## POST /api/hub/classify

分类分级 + 自动脱敏分发（关键集成端点）。

**请求体：**

```json
{
  "source": "医保数据",
  "payload": {"records": [{"name": "李四", "id_card": "110101199001011234"}]}
}
```

**响应：**

```json
{
  "task_id": "task-1724300000-2",
  "classify_result": {"level": "L3", "fields": [...]},
  "auto_operation": "k_anon",
  "level": "L3",
  "via": "service-hub"
}
```

**等级-操作映射：**

| 等级 | 自动操作 | 说明 |
|---|---|---|
| L1 | none | 公开数据，无需脱敏 |
| L2 | mask | 字段级脱敏 |
| L3 | k_anon | K-匿名泛化 |
| L4 | dp | 差分隐私 |
| L5 | dp | 差分隐私 + 查询混淆 |
