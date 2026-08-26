# 调度之眼 (Console App-LZ) — API 接口与数据契约规范

> **文档版本**：v1.1.0  
> **服务端口**：HTTP REST `:8085` / gRPC `:50055`  
> **聚合上游**：
> - `service-hub`: `http://127.0.0.1:8082` (gRPC `127.0.0.1:50052`)
> - `datasource-mgr`: `http://127.0.0.1:8083` (gRPC `127.0.0.1:50053`)
> - `audit-log`: `http://127.0.0.1:8084` (gRPC `127.0.0.1:50054`)
> - `engine Agent`: `http://127.0.0.1:8079` (gRPC `127.0.0.1:50051`)

---

## 1. 接口概览

| 模块 | HTTP 方法 | 端点路径 | 调用模式 | 上游映射 |
|---|---|---|---|---|
| **健康与拓扑** | `GET` | `/api/health` | 本地 | BFF 自身存活探针 |
| | `GET` | `/api/lz/topology` | **[聚合]** | 并发调用 4 服务 `/health` + `/readyz` |
| | `POST` | `/api/lz/probe/all` | **[聚合]** | 并发调用 4 服务 `/readyz` 深度探测 |
| **流水线调度** | `GET` | `/api/lz/pipeline/status` | **[聚合]** | `service-hub` `/api/hub/pipeline` + `/api/hub/status` |
| | `POST` | `/api/lz/pipeline/dispatch` | **[转发]** | → `service-hub` `POST /api/hub/dispatch` |
| | `POST` | `/api/lz/pipeline/classify-dispatch` | **[转发]** | → `service-hub` `POST /api/hub/classify` |
| | `POST` | `/api/lz/pipeline/trigger-datasource` | **[转发]** | → `service-hub` `POST /api/hub/pipeline/trigger-datasource` |
| **任务与租约** | `GET` | `/api/lz/tasks` | **[转发]** | → `service-hub` `GET /api/hub/tasks` |
| | `GET` | `/api/lz/tasks/:id` | **[转发]** | → `service-hub` `GET /api/hub/tasks/:id` |
| | `GET` | `/api/lz/tasks/leases` | **[聚合]** | `service-hub` 存储后端检测 + 租约状态 |
| **自动化测试** | `GET` | `/api/lz/suites` | 本地 | BFF 内置测试用例定义 |
| | `POST` | `/api/lz/suites/run` | 本地 | BFF 测试执行引擎（调用上游服务执行断言） |
| | `GET` | `/api/lz/suites/stream/:run_id` | 本地 (SSE) | BFF 测试日志流推送 |
| **数据源直通** | `GET` | `/api/lz/datasources` | **[转发]** | → `datasource-mgr` `GET /api/datasources` |
| | `GET` | `/api/lz/datasources/:id/slice` | **[转发]** | → `datasource-mgr` `GET /api/datasources/:id/slice` |
| **审计验真** | `GET` | `/api/lz/audit/logs` | **[转发]** | → `audit-log` `GET /api/audit/logs` |
| | `POST` | `/api/lz/audit/verify` | **[转发]** | → `audit-log` `POST /api/audit/snapshots/verify` |
| **监控指标** | `GET` | `/metrics` | 本地 | BFF 自身 Prometheus 指标 |

> **[聚合]** = BFF 并发调用多个上游服务并合并结果；**[转发]** = BFF 透传请求到单一上游，附加认证头与 `X-Request-ID`。

---

## 2. 认证与安全

### 2.1 入站认证

所有 `/api/lz/*` 端点（除 `/api/health` 和 `/metrics`）要求携带 API Key：

```
Authorization: Bearer <LZ_CONSOLE_API_KEY>
```

未携带或 Key 无效时返回：

```json
{ "error": { "code": "UNAUTHORIZED", "message": "missing or invalid API key" }, "via": "app-lz-bff" }
```

### 2.2 出站认证

BFF 向各上游服务转发请求时，自动注入对应的 Bearer Token：

| 上游服务 | 认证头 | 环境变量 |
|---|---|---|
| `service-hub` | `Authorization: Bearer <key>` | `LZ_HUB_API_KEY` |
| `datasource-mgr` | `Authorization: Bearer <key>` | `LZ_DATASOURCE_API_KEY` |
| `audit-log` | `Authorization: Bearer <key>` | `LZ_AUDIT_API_KEY` |
| `engine` Agent | `Authorization: Bearer <key>` | `LZ_AGENT_API_KEY` |

### 2.3 SSE 流认证

SSE 端点通过 URL 查询参数认证：`GET /api/lz/suites/stream/:run_id?token=<run_token>`。`run_token` 在 `POST /api/lz/suites/run` 响应中返回。

---

## 3. 统一错误响应格式

### 3.1 错误响应结构

所有错误响应遵循统一格式：

```json
{
  "error": {
    "code": "UPSTREAM_UNAVAILABLE",
    "message": "service-hub is unreachable",
    "details": {
      "service": "service-hub",
      "url": "http://127.0.0.1:8082",
      "timeout_ms": 5000
    }
  },
  "via": "app-lz-bff"
}
```

### 3.2 错误码定义

| 错误码 | HTTP 状态码 | 触发条件 | `details` 字段 |
|---|---|---|---|
| `UNAUTHORIZED` | 401 | API Key 缺失或无效 | — |
| `RATE_LIMITED` | 429 | 请求频率超过限制 | `limit`, `retry_after_seconds` |
| `INVALID_REQUEST` | 400 | 请求参数校验失败 | `field`, `reason` |
| `UPSTREAM_UNAVAILABLE` | 502 | 上游服务连接失败或超时 | `service`, `url`, `timeout_ms` |
| `UPSTREAM_TIMEOUT` | 504 | 上游服务响应超时 | `service`, `url`, `timeout_ms` |
| `UPSTREAM_ERROR` | 502 | 上游返回 5xx 错误 | `service`, `upstream_status`, `upstream_body` |
| `PARTIAL_DEGRADED` | 200 | 聚合查询中部分服务不可达 | `degraded_services[]`, `healthy_services[]` |

### 3.3 部分降级响应示例

拓扑查询中 `audit-log` 不可达时：

```json
{
  "status": "partial_degraded",
  "timestamp": "2026-08-26T10:45:00Z",
  "services": [
    { "id": "service-hub", "status": "ready", "rtt_ms": 1.8 },
    { "id": "datasource-mgr", "status": "ready", "rtt_ms": 2.1 },
    {
      "id": "audit-log",
      "status": "unreachable",
      "rtt_ms": null,
      "error": { "code": "UPSTREAM_UNAVAILABLE", "message": "connection refused" }
    },
    { "id": "engine", "status": "ready", "rtt_ms": 3.2 }
  ],
  "degraded_services": ["audit-log"],
  "healthy_services": ["service-hub", "datasource-mgr", "engine"],
  "via": "app-lz-bff"
}
```

---

## 4. 核心接口详细定义

### 4.1 4 服务实时拓扑与双协议健康探针 (`GET /api/lz/topology`)

- **调用模式**：[聚合] 并发调用 4 个上游服务的 REST `/health` 与 gRPC 端口连通性
- **查询参数**：
  - `protocol` (可选，字符串): 激活协议视角，可选值 `rest`（默认）或 `grpc`
- **超时策略**：单个服务探测超时 1.5 秒，整体响应超时 5 秒
- **返回顺序**：严格保证固定四节点顺序：`1. service-hub ➔ 2. engine ➔ 3. datasource-mgr ➔ 4. audit-log`
- **降级行为**：单个服务不可达时返回 200 + `status: degraded`，不阻塞整体响应
- **响应格式**：`application/json`
- **响应示例**：
```json
{
  "status": "healthy",
  "active_protocol": "rest",
  "timestamp": "2026-08-26T10:45:00Z",
  "services": [
    {
      "id": "service-hub",
      "name": "调度中枢 (Service Hub)",
      "http_url": "http://127.0.0.1:8082",
      "grpc_addr": "127.0.0.1:50052",
      "status": "ready",
      "rtt_ms": 1.8,
      "rest_status": "ready",
      "rest_rtt_ms": 1.8,
      "grpc_status": "ready",
      "grpc_rtt_ms": 1.2,
      "protocol": "rest",
      "version": "1.8.0",
      "details": {
        "store_type": "postgres_leased",
        "active_tasks": 2,
        "completed_total": 128,
        "failed_total": 3,
        "uptime": "12h34m56s"
      }
    },
    {
      "id": "engine",
      "name": "隐私与分类引擎 (PrivShield Agent)",
      "http_url": "http://127.0.0.1:8079",
      "grpc_addr": "127.0.0.1:50051",
      "status": "ready",
      "rtt_ms": 3.2,
      "rest_status": "ready",
      "rest_rtt_ms": 3.2,
      "grpc_status": "ready",
      "grpc_rtt_ms": 2.4,
      "protocol": "rest",
      "version": "1.8.0",
      "details": {
        "funnel_layers": ["rule", "ner", "llm"],
        "primitives": ["mask", "dp", "ldp", "kano", "qol"]
      }
    },
    {
      "id": "datasource-mgr",
      "name": "数据源管理 (Datasource Mgr)",
      "http_url": "http://127.0.0.1:8083",
      "grpc_addr": "127.0.0.1:50053",
      "status": "ready",
      "rtt_ms": 2.1,
      "rest_status": "ready",
      "rest_rtt_ms": 2.1,
      "grpc_status": "ready",
      "grpc_rtt_ms": 1.5,
      "protocol": "rest",
      "version": "1.8.0",
      "details": {
        "datasources_count": 2,
        "total_records": 1800
      }
    },
    {
      "id": "audit-log",
      "name": "脱敏审计日志 (Audit Log)",
      "http_url": "http://127.0.0.1:8084",
      "grpc_addr": "127.0.0.1:50054",
      "status": "ready",
      "rtt_ms": 1.5,
      "rest_status": "ready",
      "rest_rtt_ms": 1.5,
      "grpc_status": "ready",
      "grpc_rtt_ms": 1.1,
      "protocol": "rest",
      "version": "1.8.0",
      "details": {
        "merkle_valid": true,
        "total_audit_logs": 256
      }
    }
  ]
}
```

---

### 4.2 6 阶段流水线大屏状态 (`GET /api/lz/pipeline/status`)

- **调用模式**：[聚合] 合并 `service-hub` 的 `/api/hub/pipeline` + `/api/hub/status`
- **字段映射**：BFF 字段与上游 `service-hub` 字段的对应关系：

| BFF 响应字段 | 上游 `service-hub` 字段 | 说明 |
|---|---|---|
| `stages[].avg_latency_ms` | `PipelineStage.avg_latency_ms` | 直接映射，保持命名一致 |
| `stages[].throughput` | `PipelineStage.throughput` | 每分钟处理任务数 |
| `total_rps` | `PipelineStatus.total_rps` | 聚合每秒请求数 |
| `agent_ok` | `PipelineStatus.agent_ok` | Agent 连通状态 |
| `hub_status.*` | `HubStatus.*` | 来自 `/api/hub/status` 的队列深度 |

- **响应示例**：
```json
{
  "stages": [
    {"name": "ingest", "title": "任务接收与解析", "status": "idle", "active_count": 0, "avg_latency_ms": 1.2, "throughput": 450},
    {"name": "fetch", "title": "数据源切片拉取", "status": "idle", "active_count": 0, "avg_latency_ms": 8.5, "throughput": 320},
    {"name": "classify", "title": "动态分类分级评估", "status": "processing", "active_count": 1, "avg_latency_ms": 15.2, "throughput": 180},
    {"name": "desensitize", "title": "自适应隐私脱敏治理", "status": "processing", "active_count": 1, "avg_latency_ms": 6.8, "throughput": 260},
    {"name": "return", "title": "结果封装与回传", "status": "idle", "active_count": 0, "avg_latency_ms": 0.8, "throughput": 480},
    {"name": "audit", "title": "不可篡改审计存证", "status": "idle", "active_count": 0, "avg_latency_ms": 3.4, "throughput": 400}
  ],
  "total_rps": 45.2,
  "agent_ok": true,
  "hub_status": {
    "active_tasks": 2,
    "queued_tasks": 5,
    "completed_total": 128,
    "failed_total": 3,
    "uptime": "12h34m56s"
  },
  "via": "app-lz-bff"
}
```

---

### 4.3 任务分发 (`POST /api/lz/pipeline/dispatch`)

- **调用模式**：[转发] → `service-hub` `POST /api/hub/dispatch`
- **请求体**：
```json
{
  "source": "ds_yibao",
  "operation": "mask",
  "payload": {
    "name": "张三",
    "id_card": "110101199001011234",
    "phone": "13800138000"
  },
  "priority": 50
}
```
- **响应**（HTTP 202）：
```json
{
  "task_id": "task-1787554500-eabf3934",
  "status": "accepted",
  "via": "service-hub"
}
```

---

### 4.4 自适应分类分级调度 (`POST /api/lz/pipeline/classify-dispatch`)

- **调用模式**：[转发] → `service-hub` `POST /api/hub/classify`
- **请求体**：
```json
{
  "source": "ds_kangyang",
  "payload": {
    "patient_name": "李四",
    "diagnosis": "高血压",
    "blood_pressure": "160/100"
  },
  "priority": 80
}
```
- **响应**（HTTP 202）：
```json
{
  "task_id": "task-1787554501-89bcdef1",
  "status": "accepted",
  "classified_level": "L3",
  "selected_operation": "k_anon",
  "via": "service-hub"
}
```

---

### 4.5 任务列表查询 (`GET /api/lz/tasks`)

- **调用模式**：[转发] → `service-hub` `GET /api/hub/tasks`
- **查询参数**：

| 参数 | 类型 | 默认值 | 上限 | 说明 |
|---|---|---|---|---|
| `status` | string | — | `pending` / `running` / `completed` / `failed` | 按状态过滤 |
| `operation` | string | — | `mask` / `k_anon` / `dp` / `none` | 按操作类型过滤 |
| `limit` | int | 100 | 1000 | 分页大小 |
| `offset` | int | 0 | — | 分页偏移 |

- **响应示例**：
```json
{
  "total": 128,
  "tasks": [
    {
      "id": "task-1787554500-eabf3934",
      "status": "completed",
      "stage": "done",
      "source": "ds_yibao",
      "operation": "mask",
      "created_at": "2026-08-26T10:30:00Z",
      "started_at": "2026-08-26T10:30:00Z",
      "completed_at": "2026-08-26T10:30:01Z",
      "duration_ms": 35,
      "error": ""
    }
  ],
  "via": "service-hub"
}
```

---

### 4.6 Phase B 租约看板 (`GET /api/lz/tasks/leases`)

- **调用模式**：[聚合] 查询 `service-hub` 存储后端类型，PostgreSQL 模式下返回租约详情
- **存储后端自适应**：
  - **PostgreSQL 模式**：返回完整租约信息（Worker 分布、租约倒计时、孤儿回收）
  - **SQLite / Memory 模式**：返回 `store_backend` 标识 + 提示信息，`workers` 为空数组

- **PostgreSQL 模式响应示例**：
```json
{
  "store_backend": "postgres_leased",
  "total_leased_tasks": 3,
  "workers": [
    {
      "worker_id": "hub-worker-replica-1",
      "claimed_tasks_count": 2,
      "tasks": [
        {
          "task_id": "task-1787554500-eabf3934",
          "stage": "desensitize",
          "lease_expires_in_seconds": 28.5,
          "priority": 50
        }
      ]
    },
    {
      "worker_id": "hub-worker-replica-2",
      "claimed_tasks_count": 1,
      "tasks": [
        {
          "task_id": "task-1787554501-89bcdef1",
          "stage": "classify",
          "lease_expires_in_seconds": 29.1,
          "priority": 80
        }
      ]
    }
  ],
  "orphan_recovery": {
    "enabled": true,
    "scan_interval_seconds": 5,
    "recovered_total": 0
  },
  "via": "app-lz-bff"
}
```

- **SQLite / Memory 模式响应示例**：
```json
{
  "store_backend": "sqlite",
  "total_leased_tasks": 0,
  "workers": [],
  "orphan_recovery": {
    "enabled": false,
    "scan_interval_seconds": 0,
    "recovered_total": 0
  },
  "notice": "Lease-based scheduling requires PostgreSQL backend. Current backend is SQLite.",
  "via": "app-lz-bff"
}
```

---

### 4.7 自动化测试套件执行 (`POST /api/lz/suites/run`)

- **调用模式**：本地执行（BFF 测试引擎调用上游服务执行断言）
- **请求体**：
```json
{
  "suite_ids": ["TS-01", "TS-02", "TS-03"],
  "concurrency": 20,
  "benchmark_requests": 100
}
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `suite_ids` | string[] | 否 | 全部 TS-01~TS-03 | 指定执行的用例 ID 列表 |
| `concurrency` | int | 否 | 10 | TS-02 压测并发数 |
| `benchmark_requests` | int | 否 | 100 | TS-02 压测总请求数 |

- **响应示例**：
```json
{
  "run_id": "run-20260826-104500-a1b2",
  "status": "running",
  "total_cases": 3,
  "run_token": "rt-xxxx-yyyy-zzzz",
  "stream_url": "/api/lz/suites/stream/run-20260826-104500-a1b2?token=rt-xxxx-yyyy-zzzz",
  "via": "app-lz-bff"
}
```

---

### 4.8 SSE 测试日志流 (`GET /api/lz/suites/stream/:run_id`)

- **调用模式**：本地 SSE 流
- **认证**：URL 参数 `?token=<run_token>`
- **事件类型**：

| 事件类型 | 数据格式 | 说明 |
|---|---|---|
| `log` | `{"timestamp": "...", "level": "info", "message": "...", "suite_id": "TS-01"}` | 测试执行日志行 |
| `assertion` | `{"suite_id": "TS-01", "name": "status_code", "expected": 202, "actual": 202, "passed": true}` | 断言结果 |
| `suite_complete` | `{"suite_id": "TS-01", "status": "pass", "duration_ms": 1234, "assertions_total": 5, "assertions_passed": 5}` | 单个用例完成 |
| `run_complete` | `{"status": "completed", "total": 7, "passed": 6, "failed": 1, "duration_ms": 45678}` | 全部执行完成 |
| `error` | `{"code": "UPSTREAM_UNAVAILABLE", "message": "..."}` | 执行过程错误 |

---

### 4.9 数据源与审计直通

- `GET /api/lz/datasources`：直通转发至 `datasource-mgr` `GET /api/datasources`，响应格式与上游一致。
- `GET /api/lz/datasources/:id/slice`：直通转发至 `datasource-mgr` `GET /api/datasources/:id/slice`，支持 `limit` 参数（1~100）。
- `GET /api/lz/audit/logs`：直通转发至 `audit-log` `GET /api/audit/logs`，支持 `limit`/`offset` 分页。
- `POST /api/lz/audit/verify`：直通转发至 `audit-log` `POST /api/audit/snapshots/verify`。

> 直通转发接口的响应格式完全由上游服务定义，BFF 仅透传响应体和状态码。若上游不可达，BFF 返回统一错误格式（见第 3 节）。
