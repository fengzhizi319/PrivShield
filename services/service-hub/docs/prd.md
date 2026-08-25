# 数据服务调度中枢 — 产品需求文档 (PRD)

## 1. 产品概述

**数据服务调度中枢**（Service Hub）是 PrivShield 平台的企业级数据流通中枢微服务，负责统一接入上游调用（React Web 控制台、Go BFF、外部业务系统），并将数据治理请求编排为 **6 阶段安全流水线**（`ingest` ➔ `fetch` ➔ `classify` ➔ `desensitize` ➔ `return` ➔ `audit`），协同数据源管理（`datasource-mgr`）、隐私计算引擎（`PrivShield Agent`）与不可篡改审计存证（`audit-log`）。

| 属性 | 值 | 说明 |
|---|---|---|
| 模块名称 | `service-hub` | 数据流通调度中枢 |
| HTTP REST 端口 | `8082` | 默认监听地址 `127.0.0.1:8082`（面向 Web 控制台与 BFF） |
| gRPC 端口 | `50052` | 默认监听地址 `127.0.0.1:50052`（支持 TLS 1.3 / mTLS / SPKI Pinning） |
| 开发语言与框架 | Go 1.24+ / Gin / gRPC | 原生协程并发、强类型、高吞吐 |
| 上游依赖 | PrivShield Agent (`:8079` REST) | 3 层动态分类漏斗与脱敏隐私原语 |
| 下游数据源依赖 | datasource-mgr (`:8083` REST / `:50053` gRPC) | 医保/康养等仿真模拟数据源 |
| 下游审计依赖 | audit-log (`:8084` REST / `:50054` gRPC) | SHA-256 不可篡改存证与审计快照 |
| 存储引擎 | SQLite (WAL 模式) / 内存模式 | 本地任务持久化、状态流转与崩溃恢复 |

---

## 2. 核心业务需求

### 2.1 六阶段安全调度流水线

每个进入调度中枢的数据处理任务必须按严格顺序经过 6 个阶段：

```text
① ingest (接入) ──▶ ② fetch (取数) ──▶ ③ classify (分类) ──▶ ④ desensitize (脱敏) ──▶ ⑤ return (返回) ──▶ ⑥ audit (存证) ──▶ done
```

| 阶段 | 标识 | 说明 | 协同模块与动作 |
|---|---|---|---|
| ① | `ingest` | 接收请求，参数校验，生成唯一 `task_id`，落库 `pending` 状态 | 快速校验与入队，立即响应 `202 Accepted` |
| ② | `fetch` | 申请并抽取原始数据 | 若请求未显式携带 Payload，自动调用 `datasource-mgr` 采样 |
| ③ | `classify` | 敏感度动态探查与分级 | 调用 Agent `/v1/dynclassification/classify` 评估（L1~L5） |
| ④ | `desensitize` | 隐私原语执行 | 根据等级或显式指令调用 Agent 执行 mask/k_anon/dp/qol |
| ⑤ | `return` | 结果封装与格式校验 | 组装脱敏输出与耗时元数据 |
| ⑥ | `audit` | 存证审计写盘 | 触发异步审计日志记录与 SHA-256 存证 |

### 2.2 敏感度等级到脱敏策略自动映射

在自适应调度模式（`ClassifyAndDispatch`）下，根据 Agent 三层漏斗裁定的敏感度等级自动匹配脱敏算子：

| 安全等级 | 业务敏感定义 | 自动分发脱敏算子 | 执行优先级 | 策略说明 |
|---|---|---|---|---|
| **L1 (公开)** | 公开数据 / 机构代码 | `none` | low (0) | 无需脱敏，直接放行流通 |
| **L2 (内部)** | 姓名 / 电话 / 身份证号 | `mask` | normal (20) | 字段级动态掩码与哈希打码 |
| **L3 (敏感)** | 年龄 / 邮编 / 准标识符集合 | `k_anon` | high (50) | K-匿名化区间泛化与微聚合 |
| **L4 (机密)** | 诊疗金额 / 报销数值 / 频次统计 | `dp` | critical (80) | 差分隐私（Laplace / Gaussian 加噪） |
| **L5 (绝密)** | 传染病 / HIV / 绝密特种诊断 | `dp` + `qol` | critical (100) | 差分隐私加噪或查询混淆阻断 |

### 2.3 任务生命周期与状态机

- **状态集合**：`pending`（等待调度）➔ `running`（流水线执行中）➔ `completed`（处理成功）/ `failed`（处理失败）。
- **异步处理**：任务提交（`POST /api/hub/dispatch` 或 `POST /api/hub/classify`）后立即返回 `202 Accepted` + `task_id`。
- **并发控制**：内部通过容量为 10 的 Goroutine 信号量（`taskSem`）限制同时并发执行的流水线任务数，最大排队深度由 `SERVICE_HUB_MAX_QUEUE` 控制。
- **状态查询与过滤**：支持按 `task_id` 单查详情，或在列表接口按 `status`（`pending`/`running`/`completed`/`failed`）分页过滤。

---

## 3. 接口需求

### 3.1 HTTP REST 接口清单

| 方法 | 路径 | 鉴权要求 | 说明 |
|---|---|---|---|
| GET | `/health` | 免密 | 存活探针（Liveness Probe，进程存活即返回 200） |
| GET | `/readyz` | 免密 | 就绪探针（Readiness Probe，检查 Agent+Datasource 依赖，失败返回 503） |
| GET | `/api/health` | 免密 | 综合健康检查（兼容别名，返回自身及上下游依赖延迟） |
| GET | `/api/hub/status` | 可选 API Key | 调度中枢运行状态（Uptime、排队数、活跃任务数、成功/失败总量） |
| GET | `/api/hub/tasks` | 可选 API Key | 分页查询任务列表（支持 `?status=` 过滤与 `limit`/`offset` 参数） |
| GET | `/api/hub/tasks/:id` | 可选 API Key | 查询单个任务详情（包含流水线阶段、耗时与错误信息） |
| POST | `/api/hub/dispatch` | 可选 API Key | 手动提交指定算子的隐私调度任务（返回 202 Accepted） |
| GET | `/api/hub/pipeline` | 可选 API Key | 获取 6 阶段流水线活跃状态与 Agent 连通性 |
| POST | `/api/hub/classify` | 可选 API Key | 智能分类分级并根据等级自动策略下发脱敏流水线 |
| POST | `/api/hub/pipeline/trigger-datasource` | 可选 API Key | 联动 `datasource-mgr` 采样并全自动触发脱敏流水线 |
| GET | `/api/hub/datasources` | 可选 API Key | 代理列出 `datasource-mgr` 当前已注册的数据源清单 |
| GET | `/metrics` | 免密 | Prometheus 格式指标导出端点 |

### 3.2 gRPC 服务接口清单 (`servicehub.ServiceHubService`)

| RPC 方法 | 入参 Request | 出参 Response | 说明 |
|---|---|---|---|
| `Health` | `HealthRequest` | `HealthResponse` | gRPC 探针：自检 + 上游 Agent 连通性 |
| `HubStatus` | `HubStatusRequest` | `HubStatusResponse` | 调度中枢状态与队列深度 |
| `Dispatch` | `DispatchRequest` | `DispatchResponse` | 高性能提交任务到流水线 |
| `ClassifyAndDispatch` | `ClassifyAndDispatchRequest` | `ClassifyAndDispatchResponse` | 分类分级并自动分发策略 |
| `GetTask` | `GetTaskRequest` | `TaskProto` | 任务详情查询 |
| `ListTasks` | `ListTasksRequest` | `ListTasksResponse` | 任务列表查询（支持状态过滤） |
| `PipelineStatus` | `PipelineStatusRequest` | `PipelineStatusResponse` | 流水线阶段活跃监控 |

---

## 4. 运行配置与环境变量需求

| 环境变量 | 默认值 | 类型 | 说明 |
|---|---|---|---|
| `SERVICE_HUB_HOST` | `127.0.0.1` | string | HTTP REST 监听主机地址（生产通常设为 `0.0.0.0`） |
| `SERVICE_HUB_PORT` | `8082` | int | HTTP REST 服务端口 |
| `SERVICE_HUB_GRPC_HOST` | `127.0.0.1` | string | gRPC 服务监听主机地址 |
| `SERVICE_HUB_GRPC_PORT` | `50052` | int | gRPC 服务端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | string | 上游 PrivShield Agent REST 主机地址 |
| `PRIVACY_REST_PORT` | `8079` | int | 上游 PrivShield Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | `""` | string | 请求上游 Agent 所需的 API Key |
| `PRIVACY_AGENT_URLS` | `""` | string | 多 Agent 负载均衡/故障转移地址列表（逗号分隔） |
| `SERVICE_HUB_MAX_QUEUE` | `1000` | int | 最大任务等待排队深度 |
| `SERVICE_HUB_SCHEDULE_TIMEOUT` | `30` | int | 任务单步调度与执行超时（秒） |
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | string | datasource-mgr HTTP 地址 |
| `DATASOURCE_MGR_PORT` | `8083` | int | datasource-mgr HTTP 端口 |
| `DATASOURCE_MGR_GRPC_HOST` | `127.0.0.1` | string | datasource-mgr gRPC 地址 |
| `DATASOURCE_MGR_GRPC_PORT` | `50053` | int | datasource-mgr gRPC 端口 |
| `SERVICE_HUB_TLS_ENABLED` | `false` | bool | 是否启用 HTTP/gRPC TLS 强加密 |
| `SERVICE_HUB_TLS_CERT_FILE` | `""` | string | 服务端 X.509 证书路径 |
| `SERVICE_HUB_TLS_KEY_FILE` | `""` | string | 服务端私钥路径 |
| `SERVICE_HUB_TLS_CA_FILE` | `""` | string | 验证客户端身份的受信任根 CA 路径 |
| `SERVICE_HUB_TLS_CLIENT_AUTH` | `""` | string | 客户端双向认证模式：`require` \| `verify` \| `request` |
| `SERVICE_HUB_TLS_PINNED_PUBKEY_FILE` | `""` | string | 客户端公钥指纹固定文件路径 (SPKI Pinning) |
| `SERVICE_HUB_API_KEY` | `""` | string | 入站 HTTP Bearer 认证密钥（为空表示免密） |
| `SERVICE_HUB_CORS_ORIGINS` | `""` | string | 允许跨域的 Origin 列表（逗号分隔） |
| `SERVICE_HUB_DB_PATH` | `""` | string | SQLite 数据库物理路径（为空表示进程内内存模式） |
| `SERVICE_HUB_RETENTION_DAYS` | `30` | int | 终态任务数据保留天数（0 表示禁用清理） |
| `SERVICE_HUB_SHUTDOWN_TIMEOUT` | `5` | int | 优雅停机等待超时（秒） |
| `SERVICE_HUB_LOG_FORMAT` | `json` | string | 结构化日志格式：`json`（生产推荐）或 `text` |
| `SERVICE_HUB_LOG_LEVEL` | `info` | string | 日志级别：`debug` \| `info` \| `warn` \| `error` |

---

## 5. 可靠性与非功能需求

1. **零信任双向传输安全**：
   - HTTP 与 gRPC 均支持 TLS 1.3 强加密；
   - 支持 `mTLS` 客户端证书认证与 SPKI 公钥哈希固定，杜绝中间人劫持与伪造证书。
2. **崩溃恢复 (Crash Recovery)**：
   - 服务启动时自动扫描孤立任务：将中断前处于 `running` 状态的任务标记为 `failed`，将未执行的 `pending` 任务保留在队列中继续处理。
3. **失败自动重试 (Automatic Task Retry)**：
   - 针对因临时网络超时或连接重置而失败的任务，支持在启动时与后台协程（每 60 秒）自动重试（最多重试 3 次，结合指数退避与随机抖动延迟）。
4. **存储完整性校验与自动数据保留**：
   - 启动时执行 `PRAGMA integrity_check` 校验数据库文件完整性，阻断带病运行；
   - 每 6 小时自动扫描并清理超过 `SERVICE_HUB_RETENTION_DAYS`（默认 30 天）的历史终态任务。
5. **防御性网络与并发保护**：
   - HTTP 配置 5s `ReadHeaderTimeout` 防御 Slowloris 慢速连接拒绝服务攻击；
   - 配置 `MaxBodySize(32MB)` 防范超大请求体内存溢出；
   - gRPC 配置 Keepalive 周期保活与空闲连接清理；
   - 多层 Panic 安全网（HTTP 中间件 + gRPC 拦截器 + 异步协程 Recover）。
6. **全链路可观测性**：
   - 自动注入并透传 `X-Request-ID`；
   - 暴露 Prometheus `/metrics` 端点，监控请求 QPS、耗时分布、各流水线阶段吞吐与恢复/重试计数器。
