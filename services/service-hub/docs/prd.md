# 数据服务调度中枢 — 产品需求文档 (PRD)

## 1. 产品概述

**数据服务调度中枢**（Service Hub）是 PrivShield 控制台的核心调度模块，负责将用户的数据处理请求编排为 6 阶段流水线，自动完成分类分级与脱敏处理。

| 属性 | 值 |
|---|---|
| 模块名称 | service-hub |
| 默认端口 | 8082 |
| 开发语言 | Go + Gin |
| 上游依赖 | PrivShield Agent REST API (:8079) |

## 2. 核心需求

### 2.1 六阶段调度流水线

每个数据处理任务必须经过以下 6 个阶段：

| 阶段 | 名称 | 说明 |
|---|---|---|
| ① | ingest | 接收用户请求，生成任务 ID |
| ② | fetch | 从数据源获取原始数据 |
| ③ | classify | 调用 Agent `/v1/dynclassification/classify` 进行分类分级 |
| ④ | desensitize | 根据分级结果选择脱敏策略（mask/k_anon/dp/qol） |
| ⑤ | return | 返回脱敏后结果 |
| ⑥ | audit | 记录审计日志（异步写入 audit-log 模块） |

### 2.2 分级-脱敏策略映射

| 安全等级 | 脱敏操作 | 优先级 |
|---|---|---|
| L1 (公开) | none | low |
| L2 (内部) | mask | normal |
| L3 (敏感) | k_anon | high |
| L4 (机密) | dp | critical |
| L5 (绝密) | dp + qol | critical |

### 2.3 任务管理

- 异步处理：任务提交后立即返回 `202 Accepted` + `task_id`
- 状态查询：通过 `task_id` 查询任务实时状态
- 列表过滤：按状态（pending/processing/completed/failed）筛选
- 并发控制：通过 `MaxQueueDepth` 限制队列深度

## 3. 功能需求

### 3.1 API 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查 |
| GET | `/api/hub/status` | 调度中枢状态 |
| GET | `/api/tasks` | 任务列表（支持状态过滤） |
| GET | `/api/tasks/:id` | 任务详情 |
| POST | `/api/tasks/dispatch` | 提交调度任务 |
| POST | `/api/tasks/pipeline` | 手动执行流水线 |

### 3.2 与 Agent 联动

- 调用 Agent `/v1/dynclassification/classify` 进行三层分类（规则→NER→LLM）
- 调用 Agent `/v1/privacy/mask` 执行字段级脱敏
- Agent 不可达时任务标记为 `failed`，不影响其他任务

### 3.3 配置项

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_HOST` | `0.0.0.0` | 监听地址 |
| `SERVICE_HUB_PORT` | `8082` | 监听端口 |
| `SERVICE_HUB_AGENT_REST_HOST` | `127.0.0.1` | Agent REST 主机 |
| `SERVICE_HUB_AGENT_REST_PORT` | `8079` | Agent REST 端口 |
| `SERVICE_HUB_AGENT_API_KEY` | — | Agent API Key |
| `SERVICE_HUB_MAX_QUEUE_DEPTH` | `100` | 最大排队任务数 |
| `SERVICE_HUB_SCHEDULE_TIMEOUT` | `30` | 调度超时（秒） |

## 4. 非功能需求

- **性能**: 任务提交响应 < 10ms，流水线处理 < 30s
- **可靠性**: 单任务失败不影响其他任务
- **可观测**: 结构化 JSON 日志，含 task_id 全链路追踪
- **安全**: 非 root 用户运行，支持 API Key 鉴权传递

## 5. 集成关系

```
用户请求 → service-hub → PrivShield Agent (分类+脱敏)
                        → audit-log (审计记录)
```

- **上游**: PrivShield Agent（分类分级 + 脱敏原语）
- **下游**: audit-log 模块（审计日志写入）
- **协同**: datasource-mgr 提供数据源元信息
