# 前端功能 ↔ BFF 接口 ↔ 上游服务调用 全链路映射

> 本文档清晰列出 App-LZ 控制台每个前端功能模块对应的 BFF 后端接口，以及 BFF 向 4 个上游微服务发起的实际调用。
>
> **架构概览**：前端 (React :5174) → BFF Go (:8085) → 4 上游微服务 (Engine :8079 / Hub :8082 / Datasource :8083 / Audit :8084)

---

## 目录

| # | 前端组件 | 文档章节 |
|---|---------|---------|
| 1 | TopologyPanel | [1. 四微服务网格拓扑与健康矩阵](#1-四微服务网格拓扑与健康矩阵) |
| 2 | PipelineVisualizer | [2. 流水线可视化与任务调度](#2-流水线可视化与任务调度) |
| 3 | TaskLifecyclePanel | [3. 任务生命周期与租约管理](#3-任务生命周期与租约管理) |
| 4 | TestRunnerPanel | [4. 测试套件运行器](#4-测试套件运行器) |
| 5 | DatasourceExplorer | [5. 数据源浏览器](#5-数据源浏览器) |
| 6 | AuditVerifierPanel | [6. 审计验证面板](#6-审计验证面板) |
| 7 | MetricsPanel | [7. 可观测性指标面板](#7-可观测性指标面板) |
| 8 | DataApiPanel | [8. 预设数据 API（医保/康养）](#8-预设数据-api医保康养) |

---

## 1. 四微服务网格拓扑与健康矩阵

**前端组件**: `TopologyPanel.tsx`
**BFF 路由**: `GET /api/lz/topology?protocol=rest|grpc`
**BFF Handler**: `GetTopology()` → `clients.GetTopology()`

### 上游调用

BFF 并发探测 4 个微服务的 REST + gRPC 健康端点：

| 服务 | REST 探测 | gRPC 探测 |
|------|----------|----------|
| **service-hub** (:8082) | `GET :8082/api/health` | TCP `:50052` |
| **engine** (:8079) | `GET :8079/api/health` → 回退 `/health` | TCP `:50051` |
| **datasource-mgr** (:8083) | `GET :8083/api/health` | TCP `:50053` |
| **audit-log** (:8084) | `GET :8084/api/health` | TCP `:50054` |

### 前端行为

- 页面加载时自动拉取，每 15 秒自动刷新
- 支持 REST / gRPC 协议切换
- 每个服务卡片显示：状态 (ready/unhealthy/unreachable)、RTT 延迟、版本号
- 全部 ready → 集群状态 `healthy`；任一异常 → `degraded`

### 降级策略

- 上游不可达时前端使用硬编码 fallback 拓扑数据

---

## 2. 流水线可视化与任务调度

**前端组件**: `PipelineVisualizer.tsx`
**切换到此 Tab 时按需加载**

### 2.1 流水线状态

| 层 | 接口 |
|----|------|
| **前端** | `api.getPipelineStatus()` |
| **BFF** | `GET /api/lz/pipeline/status` → `GetPipelineStatus()` |
| **上游** | `GET :8082/api/hub/pipeline` (service-hub) |

返回 6 个流水线阶段的实时状态：`ingest → fetch → classify → desensitize → return → audit`

### 2.2 手动任务调度

| 层 | 接口 |
|----|------|
| **前端** | `api.dispatchTask(req)` |
| **BFF** | `POST /api/lz/pipeline/dispatch` → `DispatchTask()` |
| **上游** | `POST :8082/api/hub/dispatch` (service-hub) |

请求体：`{ source: "ds_yibao", operation: "mask", payload: {...}, priority: 50 }`

### 2.3 分类调度

| 层 | 接口 |
|----|------|
| **前端** | `api.classifyDispatch(req)` |
| **BFF** | `POST /api/lz/pipeline/classify-dispatch` → `ClassifyDispatch()` |
| **上游** | `POST :8082/api/hub/classify` (service-hub) |

请求体：`{ source: "ds_kangyang", payload: {...}, priority: 50 }`

### 降级策略

- service-hub 不可达时返回本地 `defaultStages()` 数据

---

## 3. 任务生命周期与租约管理

**前端组件**: `TaskLifecyclePanel.tsx`
**页面加载时自动拉取**

### 3.1 任务列表

| 层 | 接口 |
|----|------|
| **前端** | `api.listTasks(status?, limit?, offset?)` |
| **BFF** | `GET /api/lz/tasks?status=&limit=50&offset=0` → `ListTasks()` |
| **上游** | `GET :8082/api/hub/tasks?status=&limit=50&offset=0` (service-hub) |

### 3.2 租约状态 (G-1 改进)

| 层 | 接口 |
|----|------|
| **前端** | `api.getLeases()` |
| **BFF** | `GET /api/lz/tasks/leases` → `GetLeases()` |
| **上游** | `GET :8082/api/hub/tasks?status=running&limit=100` (service-hub) |

BFF 从 service-hub 查询 running 状态任务，按 `lease_owner` (worker) 分组，返回租约信息：
- 租约存储后端 (sqlite/postgresql)
- 各 worker 认领任务数与明细
- 孤儿任务恢复机制信息

### 降级策略

- service-hub 不可达时返回空租约结构 + 元数据

---

## 4. 测试套件运行器

**前端组件**: `TestRunnerPanel.tsx`
**页面加载时自动拉取套件定义**

### 4.1 获取可用套件

| 层 | 接口 |
|----|------|
| **前端** | `api.getSuites()` |
| **BFF** | `GET /api/lz/suites` → `GetSuites()` |
| **上游** | **无** — BFF 本地 `runner.GetAvailableSuites()` 返回预定义套件 |

预定义 7 个测试套件：TS-01 ~ TS-07

### 4.2 运行测试套件

| 层 | 接口 |
|----|------|
| **前端** | `api.runSuites(req)` |
| **BFF** | `POST /api/lz/suites/run` → `RunSuites()` |
| **上游** | 各套件独立调用（见下表） |

### 各套件上游调用明细

| 套件 | 标题 | 上游调用 |
|------|------|---------|
| **TS-01** | 隐私脱敏原语验证 | `POST :8079/v1/privacy/mask_record` (engine) |
| **TS-02** | 动态分类分级验证 | `POST :8082/api/hub/classify` (service-hub) |
| **TS-03** | 数据源切片联动 | `POST :8082/api/hub/pipeline/trigger-datasource` (service-hub) |
| **TS-04** | 审计存证完整性 | 本地 Merkle 树校验逻辑 |
| **TS-05** | 端到端流水线治理 | 组合 TS-01 ~ TS-03 |
| **TS-06** | 高并发压测 | `POST :8082/api/hub/dispatch` × N 次并发 (service-hub) |
| **TS-07** | 并发租约唯一性 | `POST :8082/api/hub/dispatch` × 5×4=20 并发 (service-hub) + `sync.Map` 零重复检测 |

---

## 5. 数据源浏览器

**前端组件**: `DatasourceExplorer.tsx`
**页面加载时自动拉取数据源列表**

### 5.1 数据源列表

| 层 | 接口 |
|----|------|
| **前端** | `api.getDatasources()` |
| **BFF** | `GET /api/lz/datasources` → `GetDatasources()` |
| **上游** | `GET :8083/api/v1/datasources` (datasource-mgr) |
| **回退** | `GET :8082/api/hub/datasources` (service-hub) |

返回 `ds_yibao` (医保 18 字段) 和 `ds_kangyang` (康养 27 字段) 的元数据。

### 5.2 数据源切片

| 层 | 接口 |
|----|------|
| **前端** | `api.getDatasourceSlice(id, limit)` |
| **BFF** | `GET /api/lz/datasources/:id/slice?limit=10` → `GetDatasourceSlice()` |
| **上游** | `GET :8083/api/v1/yibao?limit=10` 或 `GET :8083/api/v1/kangyang?limit=10` (datasource-mgr) |

datasource-mgr 从 CSV 文件加载真实数据（yibao.csv 50 行 / kangyang.csv 100 行）。

### 5.3 触发数据源流水线

| 层 | 接口 |
|----|------|
| **前端** | `api.triggerDatasource(req)` |
| **BFF** | `POST /api/lz/pipeline/trigger-datasource` → `TriggerDatasource()` |
| **上游** | `POST :8082/api/hub/pipeline/trigger-datasource` (service-hub) |

请求体：`{ datasource_id: "ds_yibao", limit: 10, operation: "mask" }`

### 降级策略

- datasource-mgr 不可达时 BFF 返回 `generateSampleSlice()` 硬编码样本数据
- 数据源列表不可达时返回 `defaultDatasources()` 静态元数据

---

## 6. 审计验证面板

**前端组件**: `AuditVerifierPanel.tsx`
**页面加载时自动拉取审计日志**

### 6.1 审计日志列表

| 层 | 接口 |
|----|------|
| **前端** | `api.getAuditLogs(limit?, offset?)` |
| **BFF** | `GET /api/lz/audit/logs?limit=50&offset=0` → `GetAuditLogs()` |
| **上游** | `GET :8084/api/v1/audit/logs?limit=50&offset=0` (audit-log) |

### 6.2 Merkle 树验证

| 层 | 接口 |
|----|------|
| **前端** | `api.verifyAudit()` |
| **BFF** | `POST /api/lz/audit/verify` → `VerifyAudit()` |
| **上游** | `POST :8084/api/v1/audit/verify` (audit-log) |

返回 Merkle 根哈希、总条目数、签名验证结果。

### 降级策略

- audit-log 不可达时返回硬编码审计日志 + 静态验证结果

---

## 7. 可观测性指标面板

**前端组件**: `MetricsPanel.tsx`
**页面加载时自动拉取**

### 7.1 原始 Prometheus 指标

| 层 | 接口 |
|----|------|
| **前端** | `api.getMetrics()` (返回纯文本) |
| **BFF** | `GET /api/lz/metrics` → `GetMetrics()` |
| **上游** | `GET :8082/metrics` (service-hub Prometheus 端点) |

### 7.2 解析后指标 (G-2/G-3 改进)

| 层 | 接口 |
|----|------|
| **前端** | `api.getParsedMetrics()` |
| **BFF** | `GET /api/lz/metrics/parsed` → `GetParsedMetrics()` |
| **上游** | `GET :8082/metrics` (service-hub) → BFF 本地解析 |

BFF 从 Prometheus 文本格式中提取：
- **stage_durations**: 6 个流水线阶段平均耗时 (ms)
- **QPS**: 从 `http_request_duration_seconds_count` 计算
- **percentiles**: P50/P90/P95/P99 从 histogram bucket 插值

### 降级策略

- service-hub 不可达时返回默认值（阶段耗时/百分位均为固定值，QPS=0）

---

## 8. 预设数据 API（医保/康养）

**前端组件**: `DataApiPanel.tsx`
**页面加载时自动拉取 API 定义**

### 8.1 API 定义列表

| 层 | 接口 |
|----|------|
| **前端** | `api.getDataApiDefinitions()` |
| **BFF** | `GET /api/lz/data-api/definitions` → `GetDataApiDefinitions()` |
| **上游** | **无** — BFF 本地 `presetDataApiDefinitions()` 返回 4 个预设定义 |

| ID | 名称 | 数据源 | 字段数 | 状态 |
|----|------|--------|--------|------|
| 1 | 医保结算数据 API | ds_yibao | 18 | active |
| 2 | 康养体征数据 API | ds_kangyang | 27 | active |
| 3 | 预留数据 API #3 | — | 0 | reserved |
| 4 | 预留数据 API #4 | — | 0 | reserved |

### 8.2 调用数据 API 会话（核心全链路）

| 层 | 接口 |
|----|------|
| **前端** | `api.invokeDataApi(apiId, limit)` |
| **BFF** | `POST /api/lz/data-api/invoke` → `InvokeDataApi()` |
| **上游** | 3 阶段编排（见下方详细链路） |

#### 全链路 3 阶段编排

```
前端 POST /api/lz/data-api/invoke { api_id: 1, limit: 5 }
  │
  ├─ Stage 1: 数据源原始数据拉取
  │   BFF → datasource-mgr
  │   GET :8083/api/v1/yibao?limit=5     (API1 医保)
  │   GET :8083/api/v1/kangyang?limit=5  (API2 康养)
  │
  ├─ Stage 2: 分类分级 + 脱敏治理 ⭐ 核心
  │   BFF → engine 医疗流水线
  │   POST :8079/v1/medical/process
  │   { records: [...原始记录...] }
  │
  │   Engine 内部执行：
  │   ├── 3-Layer 动态分类分级 (Rule → NER → LLM)
  │   ├── L4/L5 高敏文本剥离 (梅毒/HIV/癌等关键词正则抹平)
  │   ├── PII 身份字段强掩码 (姓名/身份证/地址/残疾证号/医保编号)
  │   ├── ICD-10 诊断编码按章节码脱敏
  │   ├── 日期准标识符泛化 (精确日期 → 年月)
  │   └── 诊断残留清除 (诊断名称字段整值抹平防泄露)
  │
  │   降级：engine 不可达 → BFF 本地 applyMasking() 字段级掩码
  │
  └─ Stage 3: 审计存证
      BFF → audit-log
      GET :8084/api/v1/audit/logs (验证审计服务可达)
```

#### Engine `/v1/medical/process` 响应结构

```json
{
  "classification_report": [
    {
      "record_index": 0,
      "field_details": [
        {
          "field_name": "diagnosis_name",
          "level": "L5",
          "security_tag": "HIGHLY_SENSITIVE",
          "description": "高敏诊断信息",
          "rule_matched": "MEDICAL_L5_RULE"
        }
      ]
    }
  ],
  "sanitized_data": [
    {
      "diagnosis_name": "硬*********************",
      "name": "张*丰",
      "id_card_no": "5101***********234",
      ...
    }
  ],
  "summary": {
    "total_records": 5,
    "level_distribution": { "L1": 12, "L2": 3, "L3": 1, "L4": 5, "L5": 4 },
    "guarantee_no_l4_l5_raw_data": true,
    "duration_ms": 45.2
  }
}
```

### 降级策略

- **datasource-mgr 不可达**: BFF 返回 `generateSampleSlice()` 硬编码样本（yibao 18 字段 / kangyang 27 字段）
- **engine 不可达**: BFF 降级到 `applyMasking()` 本地字段级掩码（首字保留+全星化诊断脱敏）
- **audit-log 不可达**: 标记 Stage 3 为 error，不影响整体数据返回

---

## 附录 A：BFF Client 方法 → 上游 URL 速查表

| BFF Client 方法 | 上游服务 | HTTP 端点 |
|----------------|---------|----------|
| `ProbeNode()` | 全部 4 个 | `GET /api/health` (回退 `/health`) + TCP dial |
| `GetPipelineStatus()` | service-hub | `GET /api/hub/pipeline` |
| `DispatchTask()` | service-hub | `POST /api/hub/dispatch` |
| `ClassifyDispatch()` | service-hub | `POST /api/hub/classify` |
| `TriggerDatasourcePipeline()` | service-hub | `POST /api/hub/pipeline/trigger-datasource` |
| `ListTasks()` | service-hub | `GET /api/hub/tasks` |
| `GetTask()` | service-hub | `GET /api/hub/tasks/:id` |
| `GetLeasesFromHub()` | service-hub | `GET /api/hub/tasks?status=running&limit=100` |
| `GetDatasources()` | datasource-mgr | `GET /api/v1/datasources` (回退 hub) |
| `GetDatasourceSlice()` | datasource-mgr | `GET /api/v1/{yibao\|kangyang}?limit=N` |
| `GetAuditLogs()` | audit-log | `GET /api/v1/audit/logs` |
| `VerifyAudit()` | audit-log | `POST /api/v1/audit/verify` |
| `GetHubMetrics()` | service-hub | `GET /metrics` |
| `ProcessMedicalRecords()` | **engine** | `POST /v1/medical/process` |
| `MaskRecordViaEngine()` | **engine** | `POST /v1/privacy/mask_record` |

## 附录 B：前端 API Client → BFF 路由速查表

| 前端 `api.*` 方法 | HTTP | BFF 路由 | Handler |
|------------------|------|---------|---------|
| `getTopology()` | GET | `/api/lz/topology` | `GetTopology` |
| `probeAll()` | POST | `/api/lz/probe/all` | `GetTopology` |
| `getPipelineStatus()` | GET | `/api/lz/pipeline/status` | `GetPipelineStatus` |
| `dispatchTask()` | POST | `/api/lz/pipeline/dispatch` | `DispatchTask` |
| `classifyDispatch()` | POST | `/api/lz/pipeline/classify-dispatch` | `ClassifyDispatch` |
| `triggerDatasource()` | POST | `/api/lz/pipeline/trigger-datasource` | `TriggerDatasource` |
| `listTasks()` | GET | `/api/lz/tasks` | `ListTasks` |
| `getTask()` | GET | `/api/lz/tasks/:id` | `GetTask` |
| `getLeases()` | GET | `/api/lz/tasks/leases` | `GetLeases` |
| `getSuites()` | GET | `/api/lz/suites` | `GetSuites` |
| `runSuites()` | POST | `/api/lz/suites/run` | `RunSuites` |
| `getDatasources()` | GET | `/api/lz/datasources` | `GetDatasources` |
| `getDatasourceSlice()` | GET | `/api/lz/datasources/:id/slice` | `GetDatasourceSlice` |
| `getAuditLogs()` | GET | `/api/lz/audit/logs` | `GetAuditLogs` |
| `verifyAudit()` | POST | `/api/lz/audit/verify` | `VerifyAudit` |
| `getMetrics()` | GET | `/api/lz/metrics` | `GetMetrics` |
| `getParsedMetrics()` | GET | `/api/lz/metrics/parsed` | `GetParsedMetrics` |
| `getDataApiDefinitions()` | GET | `/api/lz/data-api/definitions` | `GetDataApiDefinitions` |
| `invokeDataApi()` | POST | `/api/lz/data-api/invoke` | `InvokeDataApi` |

## 附录 C：上游服务端口与职责

| 服务 | REST 端口 | gRPC 端口 | 职责 |
|------|----------|----------|------|
| **engine** (PrivShield Agent) | :8079 | :50051 | 隐私脱敏引擎 + 动态分类分级 + 医疗流水线 |
| **service-hub** | :8082 | :50052 | 数据服务调度中枢（任务分发/租约管理/流水线编排） |
| **datasource-mgr** | :8083 | :50053 | 数据源资产管理（CSV 加载/元数据/切片查询） |
| **audit-log** | :8084 | :50054 | 脱敏审计日志（SHA-256 存证/Merkle 树验证） |

## 附录 D：数据来源三层模型

| 层级 | 占比 | 说明 |
|------|------|------|
| **L1 实时上游** | ~70% | BFF 调用真实微服务 API（拓扑探测、Prometheus 指标、engine 脱敏、审计验证、并发压测） |
| **L2 BFF 兜底** | ~20% | 上游不可达时返回硬编码数据（样本数据、默认阶段、静态审计日志） |
| **L3 前端硬编码** | ~10% | 排列顺序、角色元数据、预设样本展示、动画效果 |
