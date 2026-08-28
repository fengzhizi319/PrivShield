# PrivShield 全栈统一架构设计再评估与全系统平滑迁移实施方案

> **文档定位**：本文档为 `PrivShield` 体系提供全栈统一架构设计的**深度再评估报告**与**系统级细节迁移落地实施方案（Migration Playbook）**。  
> **版本**：v15.0.0  
> **状态**：🎯 Target Blueprint + ✅ Phase 1 Implemented + 📋 Phase 2 Plan  
> **最后更新**：2026-08-28
> **覆盖范围**：`engine`（Python 核心隐私引擎）、`services/service-hub`（调度中枢）、`services/datasource-mgr`（数据源管理）、`services/audit-log`（审计存证）、`console/bff-go` & `console/app-lz`（BFF网关与测试执行器）、`console/web` & `console/app-lz/web`（前端控制台群）、`pkg/`（共享基础库）及云原生部署基础设施。

---

## 0. 设计落地状态总览

| 设计能力 | 当前状态 | Phase 1 已实现要点 | Phase 2 待改造项 |
|:---|:---|:---|:---|
| 统一错误信封 | ✅ Phase 1 完成 | Python `engine/observability/envelope.py` + Go `pkg/middleware/envelope.go`；FastAPI/Starlette 全局捕获；MaxBodySize 走信封 | 无 |
| 全链路分布式追踪 | 🟡 Phase 1 主体完成，部分 outbound 待补齐 | HTTP/gRPC 入口注入 `X-Request-ID`/`X-Trace-ID`；engine gRPC 提取 metadata；service-hub task 持久化 TraceID | BFF-Go REST 向上游透传（已做需文档确认）；service-hub→datasource-mgr gRPC 双头（已做需文档确认）；app-lz/bff-go outbound 透传 |
| SSOT 数据源命名 | ✅ Phase 1 完成 | `pkg/naming/` SSOT；Go/Python/TS 常量对齐；`make lint-naming` | 无 |
| SQLite → PostgreSQL 迁移 | 🟡 Phase 1 工具可用，验真待增强 | `pkg/store/cmd/migrate/main.go` + `scripts/prod/migrate_sqlite_to_postgres.sh`；哈希链迁移后校验 | 增加 snapshot 密文 AES-GCM 验真；只读锁定/幂等重跑优化 |
| mTLS CN 白名单 | 🟡 库完成，服务端注册待完成 | `pkg/tlsutil/whitelist.go` + `grpc_interceptor.go`；热重载（mtime 轮询）；Python 端消费 | Go service-hub/datasource-mgr/audit-log/bff-go gRPC server 注册拦截器；统一 `config/mtls-whitelist.yaml` scope 语义 |
| 前端双控制台 | ✅ Phase 1 完成 | `console/web` 与 `console/app-lz/web` 统一错误解析、状态指示器、动态 API 渲染 | 无 |
| 可观测性指标 | 🟡 定义完成，部分未埋点 | Python/Go metric 定义；中间件计数；部分 primitive 埋点 | 补齐 `privacy_classification_*`、`privacy_*_duration_seconds` 埋点；Go `service_hub_ready` / `circuit_breaker_state` 更新 |
| BFF 微服务直连 | 📋 Phase 2 | 当前 `console/bff-go` 只代理到 Python Agent | `console/bff-go` 增加直连 service-hub/datasource-mgr/audit-log 的客户端、路由与错误映射 |
| 零信任 outbound 认证 | 📋 Phase 2 | service-hub→datasource-mgr HTTP 未发 API Key；app-lz outbound 未透传 trace/auth | 统一 outbound API Key + trace header 注入 |

## 1. 统一设计顶层再评估与技术代差审计

### 1.1 演进背景与协同现状评估

随着 PrivShield 从最初的**单体 Python 隐私 Sidecar** 演进为**企业级分布式数据安全流通治理中台**，各模块在快速迭代中形成了多语言、多协议、多介质的异构格局。为了实现高内聚、低耦合、零语义漂移的企业级标准，对当前各子系统进行协同度量化评估：

| 子系统 / 模块 | 命名一致性 | 错误信封 | 分布式追踪 | 存储抽象 | 零信任安全 | 综合成熟度评级 |
|:---|:---:|:---:|:---:|:---:|:---:|:---|
| **pkg/ 基础共享库** | ★★★★★ | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | **Level 5 (准生产)** |
| **services/audit-log** | ★★★★★ | ★★★★☆ | ★★★★☆ | ★★★★★ | ★★★★★ | **Level 5 (准生产)** |
| **services/service-hub** | ★★★★★ | ★★★★☆ | ★★★★☆ | ★★★★★ | ★★★★☆ | **Level 5 (准生产)** |
| **services/datasource-mgr** | ★★★★★ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | **Level 5 (准生产)** |
| **console/app-lz** | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★☆ | ★★★★★ | **Level 5 (准生产)** |
| **console/bff-go** | ★★★★★ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★★ | **Level 5 (准生产)** |
| **engine (Python)** | ★★★★★ | ★★★★☆ | ★★★★☆ | ★★★☆☆ | ★★★★☆ | **Level 5 (准生产)** |
| **console 前端群** | ★★★★☆ | ★★★★☆ | ★★★☆☆ | N/A | N/A | **Level 4 (就绪)** |


### 1.2 已收敛的核心协同短板（Phase 1）

> 以下 4 项历史短板在 Phase 1 中已得到收敛，但部分 outbound 追踪/认证、验真与埋点仍需 Phase 2 继续完善。

1. **错误响应信封格式差异** ✅ — Python/Go 双端已统一，`MaxBodySize` 也已接入信封；
2. **追踪上下文断链风险** 🟡 — HTTP/gRPC 入口已贯通；Task `TraceID` 已持久化；剩余 outbound 补齐见 [§10](#10-第二阶段改造计划phase-2)；
3. **数据源命名硬编码** ✅ — SSOT + `make lint-naming` 已落地；
4. **SQLite → PostgreSQL 割接** 🟡 — 迁移工具已可用，支持 `dry-run`/`verify`；snapshot 密文原样迁移，AES-GCM 验真待 Phase 2。

---

## 2. 统一设计全景技术架构蓝图

```mermaid
flowchart TD
    subgraph LayerPresentation ["1. 统一表现与接入层 (Presentation & Gateway)"]
        WebFull["console/web<br/>(4大隐私原语 + 分类漏斗)"]
        WebAppLZ["console/app-lz/web<br/>(医保/康养政务流水线)"]
        BFFGo["console/bff-go (:8081)<br/>REST/gRPC 聚合网关"]
        BFFLZ["app-lz/bff-go (:8085)<br/>会话调度与 E2E 测试器"]
        PyGW["engine/gateway<br/>Python 负载均衡网关<br/>(6算法/熔断/重试/动态拓扑)"]
    end

    subgraph LayerMiddleware ["2. 统一中间件与上下文透传层 (Cross-Cutting Middleware)"]
        TraceMW["TraceID 自动注入与 Header 传递"]
        AuthMW["API Key 鉴权与 Leaky Bucket 限流"]
        EnvelopeMW["统一 JSON 响应信封包裹器"]
        DDoSMW["DDoS 防护<br/>(MaxBodySize/MaxConcurrent/RateLimit)"]
        SecHeaders["安全响应头<br/>(CSP/HSTS/X-Frame-Options)"]
    end

    subgraph LayerGovernance ["3. 企业级数据流通调度与存证层 (Services Cluster)"]
        Hub["services/service-hub (:8082)<br/>6 阶段流水线 / Phase B 租约 Worker"]
        DSMgr["services/datasource-mgr (:8083)<br/>多源数据纳管 / 样本切片提取"]
        Audit["services/audit-log (:8084)<br/>9要素防篡改哈希链 / 快照信封加密"]
    end

    subgraph LayerCoreCompute ["4. 核心隐私计算与动态分类引擎 (Core Engine)"]
        Funnel["3-Layer 动态分类漏斗<br/>(Rule → Small-NER → Local LLM)"]
        Primitives["四大隐私原语<br/>(Masking / DP / K-Anon / QoL)"]
        Budget["隐私预算会计<br/>(Epsilon/Delta + 时间窗口重置)"]
        EngineMW["FastAPI 全局异常信封拦截器"]
    end

    subgraph LayerStorageSecurity ["5. 统一存储与密码学基座 (Storage & Crypto Foundations)"]
        SSOT["pkg/naming<br/>(全局唯一事实源)"]
        StoreFacade["pkg/store<br/>(Memory / SQLite / PostgreSQL)"]
        EnvelopeCrypto["pkg/crypto<br/>(AES-256-GCM enc:v1:...)"]
        mTLSAuth["pkg/tlsutil<br/>(TLS 1.3 mTLS + CN 白名单)"]
    end

    subgraph LayerObservability ["6. 全栈可观测性体系 (Observability)"]
        Metrics["Prometheus Metrics<br/>(Python prometheus_client + Go client_golang)"]
        StructLog["结构化日志<br/>(JSON/Text 双格式)"]
        Tracing["OpenTelemetry Tracing<br/>(可选 OTLP 导出)"]
        Grafana["Grafana Dashboard + ServiceMonitor"]
    end

    WebFull --> BFFGo
    WebAppLZ --> BFFLZ
    BFFGo & BFFLZ --> LayerMiddleware
    LayerMiddleware --> Hub & DSMgr & Audit & LayerCoreCompute
    LayerMiddleware --> PyGW
    PyGW --> LayerCoreCompute
    Hub -->|HTTP| DSMgr
    Hub -->|gRPC| LayerCoreCompute
    LayerGovernance --> LayerStorageSecurity
    LayerCoreCompute --> LayerStorageSecurity
    LayerCoreCompute --> LayerObservability
    LayerGovernance --> LayerObservability
    LayerMiddleware --> LayerObservability
```

### 2.1 服务通信拓扑矩阵

| 调用方 → 被调方 | 协议 | 端口 | 认证方式 | 追踪透传 |
|---|---|---|---|---|
| console/web → console/bff-go | HTTPS | :8081 | API Key (可选) | X-Request-ID |
| console/app-lz/web → app-lz/bff-go | HTTPS | :8085 | API Key (可选) | X-Request-ID |
| console/bff-go → service-hub (未实现；app-lz 独占) | — | — | — | — |
| console/bff-go → datasource-mgr (未实现；app-lz 独占) | — | — | — | — |
| console/bff-go → audit-log (未实现；app-lz 独占) | — | — | — | — |
| console/bff-go → engine (REST) | HTTP | :8079 | API Key | X-Request-ID + X-Trace-ID |
| console/bff-go → engine (gRPC) | gRPC | :50051 | mTLS (可选) | x-request-id metadata |
| console/bff-go (gRPC server) | gRPC | :50055 | mTLS (可选) | — (可选组件，`PRIVACY_CONSOLE_GRPC_ENABLED`) |
| app-lz/bff-go → service-hub | HTTP | :8082 | API Key（代码层待接入） | X-Request-ID（代码层待完整接入 X-Trace-ID） |
| app-lz/bff-go → datasource-mgr | HTTP | :8083 | API Key（代码层待接入） | X-Request-ID（代码层待完整接入 X-Trace-ID） |
| app-lz/bff-go → audit-log | HTTP | :8084 | API Key（代码层待接入） | X-Request-ID（代码层待完整接入 X-Trace-ID） |
| app-lz/bff-go → engine (REST) | HTTP | :8079 | API Key | X-Request-ID |
| service-hub → engine (REST) | HTTP | :8079 | API Key | X-Request-ID |
| service-hub → datasource-mgr | HTTP | :8083 | — | X-Request-ID |
| engine/gateway → engine worker | HTTP | :8079 | 无（内部） | X-Request-ID |

> **注意**：console/bff-go 当前仅代理到 Python Agent，未直接调用 Go 微服务。

### 2.2 全栈环境变量速查

<details>
<summary>点击展开完整环境变量参考表</summary>

#### Python 引擎 (`engine/`)

| 变量 | 默认值 | 用途 |
|---|---|---|
| `PRIVACY_REST_HOST` / `PRIVACY_REST_PORT` | `0.0.0.0` / `8079` | REST 监听地址 |
| `PRIVACY_GRPC_HOST` / `PRIVACY_GRPC_PORT` | `0.0.0.0` / `50051` | gRPC 监听地址 |
| `PRIVACY_LOG_FORMAT` / `PRIVACY_LOG_LEVEL` | `text` / `INFO` | 日志格式与级别 |
| `PRIVACY_TLS_ENABLED` | `false` | 启用 TLS |
| `PRIVACY_AUTH_ENABLED` | `false` | 启用 API Key 鉴权 |
| `PRIVACY_RATE_LIMIT_ENABLED` | `false` | 启用限流 |
| `PRIVACY_LLM_MAX_CONCURRENCY` | `1` | LLM 推理并发上限 |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | `512` | 内存阈值降级 |
| `PRIVACY_BUDGET_DB` | — | 分布式预算 DB 路径 |
| `PRIVACY_BUDGET_WINDOW_SECONDS` | — | 预算自动重置周期 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OpenTelemetry OTLP 端点 |

> 注意：`engine/main.py` 单独入口默认监听 127.0.0.1，而 `engine.server` / `launcher` 默认监听 0.0.0.0。

#### Go 微服务 (`services/`, `console/`, `pkg/`)

| 服务 | 关键环境变量 | 默认值 |
|---|---|---|
| service-hub | `SERVICE_HUB_HOST` / `_PORT` | `127.0.0.1:8082` |
| | `SERVICE_HUB_GRPC_HOST` / `_PORT` | `127.0.0.1:50052` |
| | `SERVICE_HUB_PG_DSN` / `_PG_MAX_CONNS` | `` / `10` |
| | `SERVICE_HUB_RATE_LIMIT_RPS` / `_BURST` | `100` / `200` |
| | `SERVICE_HUB_LOG_FORMAT` / `_LOG_LEVEL` | `json` / `info` |
| | `SERVICE_HUB_SHUTDOWN_TIMEOUT` | `5` (秒) |
| | `SERVICE_HUB_LEASE_TTL` | `60` (秒) |
| datasource-mgr | `DATASOURCE_MGR_HOST` / `_PORT` | `127.0.0.1:8083` |
| | `DATASOURCE_MGR_GRPC_HOST` / `_PORT` | `127.0.0.1:50053` |
| | `DATASOURCE_MGR_RATE_LIMIT_RPS` / `_BURST` | `100` / `200` |
| audit-log | `AUDIT_LOG_HOST` / `_PORT` | `127.0.0.1:8084` |
| | `AUDIT_LOG_GRPC_HOST` / `_PORT` | `127.0.0.1:50054` |
| | `AUDIT_LOG_PG_DSN` (回退: `PG_DSN`) | `""` |
| | `AUDIT_LOG_RETENTION_DAYS` | `90` |
| | `AUDIT_LOG_ENCRYPTION_KEY` (回退: `PRIVACY_AUDIT_KEY`) | `""` |
| console/bff-go | `PRIVACY_CONSOLE_HOST` / `_PORT` | `127.0.0.1:8081` |
| | `CONSOLE_API_KEY` / `CONSOLE_RATE_LIMIT` | `""` / `600` (req/min) |
| | `PRIVACY_CONSOLE_GRPC_ENABLED` / `_PORT` | `false` / `50055` |
| app-lz/bff-go | `APP_LZ_HOST` / `_PORT` | `0.0.0.0:8085` |
| | `APP_LZ_HUB_URL` / `_DATASOURCE_URL` / `_AUDIT_URL` | 各上游服务地址 |
| | `APP_LZ_RATE_LIMIT_RPS` / `_BURST` | `100` / `200` |
| 通用 | `PRIVACY_AUTH_MTLS_WHITELIST_FILE` | — / None |
| | `PRIVACY_GRPC_MAX_WORKERS` | `64` |

</details>

### 2.3 Python 引擎 REST API 端点速查

<details>
<summary>点击展开 Python 引擎 API 端点参考表</summary>

#### 健康检查与运维

| 方法 | 端点 | 功能 |
|---|---|---|
| GET/POST | `/health`, `/livez`, `/readyz` | 健康检查 / 存活 / 就绪探针 |
| GET | `/readyz/llm` | LLM 层就绪状态探针 |
| GET | `/v1/privacy/health` | 引擎运行状态摘要 |
| GET | `/v1/ops/diagnostics` | 运行时诊断快照 |
| GET | `/metrics` | Prometheus 指标 |

#### 四大隐私原语

| 方法 | 端点 | 功能 |
|---|---|---|
| POST | `/v1/privacy/mask` | 字段级脱敏 |
| POST | `/v1/privacy/mask_record` | 单条记录脱敏 |
| POST | `/v1/privacy/mask/batch`, `/v1/privacy/mask/dataframe` | 批量/DataFrame 脱敏 |
| POST | `/v1/privacy/hash` | HMAC 哈希 |
| POST | `/v1/privacy/dp/count`, `/sum`, `/mean`, `/histogram` | 差分隐私聚合（4 种） |
| POST | `/v1/privacy/dp/noisy_count`, `/noisy_sum`, `/noisy_mean`, `/noisy_histogram` | 噪声聚合变体 |
| POST | `/v1/privacy/dp/aggregate`, `/vector_sum`, `/vector_mean` | 向量聚合 |
| POST | `/v1/privacy/dp/adaptive_clip`, `/groupby`, `/chunked_count`, `/chunked_sum`, `/chunked_mean`, `/chunked_histogram` | 高级 DP |
| POST | `/v1/privacy/ldp/perturb/binary`, `/perturb/categorical` | 本地 DP 扰动 |
| POST | `/v1/privacy/ldp/estimate/binary`, `/estimate/categorical` | 本地 DP 估计 |
| POST | `/v1/privacy/k_anonymize/record`, `/table`, `/dataframe` | K-匿名（3 粒度） |
| POST | `/v1/privacy/qol/obfuscate`, `/obfuscate/batch` | 查询混淆 |
| POST | `/v1/agent/process` | Agent 统一处理入口 |
| POST | `/v1/medical/process` | 医疗数据处理（Deprecated） |
| POST | `/v1/pipeline/process_records` | 记录流水线处理 |
| POST | `/v1/pipeline/process_csv` | CSV 流水线处理 |

#### 分类漏斗与配置

| 方法 | 端点 | 功能 |
|---|---|---|
| POST | `/v1/dynclassification/eval` | 单字段分类评估 |
| POST | `/v1/dynclassification/eval_record` | 单记录多字段分类 |
| POST | `/v1/dynclassification/eval_table` | 表级批量分类 |
| POST | `/v1/dynclassification/dry_run` | 分类干运行（不生效） |
| POST | `/v1/dynclassification/profiles/reload` | 热重载分类配置 |
| POST | `/v1/dynclassification/generate_profile` | 生成分类 Profile |
| POST | `/v1/dynclassification/validate` | 校验分类规则 |
| GET | `/v1/dynclassification/standards`, `/domains`, `/operators` | 查询已注册体系/领域/算子 |

#### 预算、配置与文件

| 方法 | 端点 | 功能 |
|---|---|---|
| GET | `/v1/privacy/budget` | 隐私预算查询 |
| POST | `/v1/privacy/budget/reset` | 重置隐私预算 |
| DELETE | `/v1/privacy/budget` | 删除隐私预算命名空间 |
| POST | `/v1/privacy/profile/recommend` | 隐私配置推荐 |
| POST | `/v1/privacy/process_file` | 文件级隐私处理（CSV/JSON（Excel 待实现）） |

</details>

### 2.4 Go 微服务 REST API 端点速查

<details>
<summary>点击展开完整 API 端点参考表</summary>

#### Service Hub (:8082)

| 方法 | 端点 | 功能 |
|---|---|---|
| GET | `/health`, `/api/health`, `/readyz` | 健康检查 / 就绪探针 |
| GET | `/api/hub/status` | 调度中枢运行状态快照 |
| GET | `/api/hub/tasks` | 任务列表（支持分页） |
| GET | `/api/hub/tasks/:id` | 任务详情 |
| POST | `/api/hub/dispatch` | 分发新任务 |
| GET | `/api/hub/pipeline` | 6 阶段流水线状态 |
| GET | `/metrics` | Prometheus 指标 |

#### Datasource Mgr (:8083)

| 方法 | 端点 | 功能 |
|---|---|---|
| GET | `/health`, `/api/health`, `/readyz` | 健康检查 / 就绪探针 |
| GET | `/api/v1/yibao` | 医保数据（`ds_yibao`） |
| GET | `/api/v1/kangyang` | 康养数据（`ds_kangyang`） |
| GET | `/api/v1/mock3` | Mock 数据源 3 |
| GET | `/api/v1/mock4` | Mock 数据源 4 |
| GET | `/api/datasources` | 数据源列表 |
| GET | `/api/datasources/:id` | 数据源详情 |
| GET | `/api/datasources/:id/records` | 数据源记录查询 |
| GET | `/api/datasources/:id/sample` | 样本切片提取 |
| POST | `/api/datasources/:id/test` | 连接测试 |
| GET | `/api/datasources/:id/metadata` | 元数据查询 |
| GET | `/api/datasources/:id/audit` | 访问审计日志 |
| POST | `/api/datasources/seed` | 初始化种子数据 |
| GET | `/metrics` | Prometheus 指标 |

#### Audit Log (:8084)

| 方法 | 端点 | 功能 |
|---|---|---|
| GET | `/health`, `/api/health`, `/readyz` | 健康检查 / 就绪探针 |
| GET | `/api/audit/logs` | 审计日志列表 |
| POST | `/api/audit/logs` | 创建审计日志条目 |
| GET | `/api/audit/logs/:id` | 审计日志详情 |
| GET | `/api/audit/stats` | 审计统计概览 |
| GET | `/api/audit/snapshots` | 快照列表 |
| POST | `/api/audit/snapshots/verify` | 快照完整性验真 |
| POST | `/api/audit/chain/verify` | 9 要素哈希链验真 |
| POST | `/api/audit/report` | 生成审计报告 |
| GET | `/metrics` | Prometheus 指标 |

</details>

---

## 3. 六大专项技术迁移实施方案 (Migration Playbooks — Reference Summary)

> **实施状态总览**：六大专项已全部完成核心实现（✅ = 已完成）。以下为各专项的目标、关键实现文件与要点摘要，详细代码实现请直接参考对应源文件。

---

### 专项方案 1：跨语言统一 API 错误信封与状态码平滑迁移 ✅

#### 1. 迁移目标
消除各微服务（Python + Go）在错误响应上的格式差异，统一输出遵循以下规范的 JSON 响应信封：

```json
{
  "code": "INVALID_DATASOURCE_ID",
  "message": "指定的业务数据源不存在或未激活",
  "detail": "datasource 'ds_unknown' is not registered in canonical naming",
  "trace_id": "req-1787554500-abc12345",
  "timestamp": "2026-08-27T09:30:00.123Z"
}
```

#### 2. 实现要点
- **Python 端** (`engine/observability/envelope.py`)：FastAPI 全局异常处理器统一捕获 `RequestValidationError` / `HTTPException` / 未捕获异常，输出标准信封；
- **Go 端** (`pkg/middleware/envelope.go`)：`AbortWithError(c, httpStatus, code, message, detail)` 标准响应函数，所有 5 个 Go 服务统一调用；
- **前端双控制台** (`console/web/src/api/client.ts`, `console/app-lz/web/src/api/client.ts`)：统一解析 `{code, message, detail, trace_id}` 信封，向后兼容旧格式；
- **双轨兼容**：响应头强制下发 `X-Request-ID` + `X-Trace-ID`，过渡期保留 `detail` 字段兼容旧客户端。

**实现状态**：Python/Go 双端已统一错误信封；`MaxBodySize` 拦截也返回标准信封；前端双控制台已按新格式解析。

**Phase 2 待办**：
- 无（本专项已收敛，仅随新服务接入复用既有信封函数）。

---

### 专项方案 2：全链路分布式追踪 (Trace Context) 贯穿迁移 ✅

#### 1. 迁移目标
确保由前端生成的 `X-Request-ID`，在跨越 HTTP REST、Go 内部调度流水线、gRPC 跨机调用、异步 Goroutine 消费以及 Audit Log 存证数据库落盘的全生命周期中**保持绝对单调且不丢失**。

```text
┌────────────────┐  HTTP: X-Request-ID  ┌────────────────┐  gRPC Metadata  ┌────────────────┐
│  前端 React UI  │ ───────────────────▶ │   BFF / Hub    │ ───────────────▶ │ Engine / Audit │
│ (生成 TraceID)  │                      │ (Context 注入) │  (x-request-id) │ (日志结构化输出)│
└────────────────┘                      └────────────────┘                 └────────────────┘
```

#### 2. 实现要点
- **HTTP 层** (`pkg/middleware/trace.go`)：`TraceMiddleware()` 自动注入/传播 `X-Request-ID` + `X-Trace-ID` 双头下发；
- **gRPC 客户端** (`console/bff-go/internal/agent/client.go`)：外发拦截器将 trace ID 写入 `metadata.AppendToOutgoingContext`；
- **gRPC 服务端** (`engine/grpc_server.py`)：元数据提取器从 `invocation_metadata()` 读取 `x-request-id` / `x-trace-id`；
- **异步任务** (`services/service-hub/internal/handlers/handlers.go`)：`Dispatch` 时将 `TraceID` 持久化至 `models.Task.TraceID`，Worker 消费时还原为 `context.Context`。

---

### 专项方案 3：业务标识统一与别名归一化迁移 (SSOT Naming) ✅

#### 1. 迁移目标
彻底消除全栈代码中对数据源名称、API 编号的硬编码，将所有识别、校验与展示逻辑统一收敛至 [`pkg/naming`](../../pkg/naming/)。

#### 2. 平滑迁移矩阵与废弃端点兼容

| 历史/别名标识 | Canonical 数据源 ID | 对应 API 编码 | 兼容处理策略 |
|---|---|---|---|
| `"yibao"`, `"yibao.csv"`, `"医保"` | `ds_yibao` (常量: `naming.DSYibao`) | `api1_yibao` | 边界自动归一化，返回 `Deprecation`、`Sunset`、`Link` 及 `X-PrivShield-Canonical-Path` 头 |
| `"kangyang"`, `"kangyang.csv"`, `"康养"` | `ds_kangyang` (常量: `naming.DSKangyang`) | `api2_kangyang` | 边界自动归一化，返回 `Deprecation`、`Sunset`、`Link` 及 `X-PrivShield-Canonical-Path` 头 |
| 任意未知标识 (如 `"custom_test"`) | 拦截拒绝 | N/A | **Fail-Closed 阻断**，返回 `400 INVALID_DATASOURCE_ID` |

#### 3. 实现要点
- **核心库** (`pkg/naming/`)：常量定义 + 别名归一化函数 + Observer 模式上报 Prometheus 指标；
- **自动化检查**：`Makefile lint-naming` 目标扫描业务代码中的裸字符串硬编码。

---

### 专项方案 4：存储底座 Phase A (SQLite) 到 Phase B (PostgreSQL) 生产平滑迁移 ✅

#### 1. 迁移目标与挑战
在单机环境下，PrivShield 使用 SQLite WAL 模式（`service-hub.db` 与 `audit-log.db`）。当升级到多节点企业级高并发集群时，需切换至 PostgreSQL Phase B 存储底座。  
**核心挑战**：必须保证存量审计日志在割接过程中 **9 要素连续哈希链不断链**，且 **snapshot 密文原样迁移、哈希链完整性可校验**。

```text
┌───────────────────────┐                               ┌──────────────────────────┐
│  Phase A (SQLite WAL) │                               │ Phase B (PostgreSQL)     │
├───────────────────────┤                               ├──────────────────────────┤
│ • service-hub.db      │  ─── 迁移工具平滑割接 ───▶    │ • 表: tasks (行级锁租约) │
│ • audit-log.db        │      (哈希链完整性校验)        │ • 表: audit_logs (连续链)│
│ • snapshots.db        │                               │ • 表: snapshots (信封密文)│
└───────────────────────┘                               └──────────────────────────┘
```

#### 2. 数据迁移与割接实施流程

##### Step 4.1：PostgreSQL 生产表结构
任务表结构由 `pkg/store/postgres/schema.go` 维护；审计日志与快照表结构由 `pkg/store/postgres/audit.go` 维护。均支持 `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS` 增量演进。

##### Step 4.2：迁移工具 (`pkg/store/cmd/migrate/main.go`，包装器 `scripts/prod/migrate_sqlite_to_postgres.sh`)
原子执行以下关键步骤：
1. **按哈希链顺序从 SQLite WAL 流式抽取** `tasks`、`audit_logs`、`snapshots` → 2. **snapshot 密文原样迁移**（不重新加解密） → 3. **使用 `pgx.Batch` 批量写入 PostgreSQL** → 4. **迁移后校验 9 要素哈希链完整性**

支持 `--dry-run` 模式预检验证与 `--verify` 迁移后校验。

##### Step 4.3：生产环境变量切换
设置 `*_PG_DSN` 环境变量后重启服务，自动激活 Phase B 存储后端（`AUDIT_LOG_PG_DSN`、`SERVICE_HUB_PG_DSN`）。

---

### 专项方案 5：零信任通信与 mTLS CN 白名单动态热重载迁移 ✅

#### 1. 迁移目标
将静态编译在代码或单机环境变量中的证书 CN 列表，迁移为基于动态配置文件的 **微服务访问控制白名单 (`mtls-whitelist.yaml`)**，支持在不停机的情况下基于文件 mtime 轮询（默认 5 秒）实现授权热生效。

#### 2. 白名单配置文件标准结构 (`config/mtls-whitelist.yaml`)

```yaml
# 每个客户端条目包含: cn (证书 CommonName), role, description, scopes
# scopes 支持通配符 "*" 和精确权限匹配 (如 "privacy:mask")
entries:
  - cn: "bff-go.privshield.internal"        # 主控制台 BFF → 全 Scope 访问
    role: "bff"
    scopes: ["*"]
  - cn: "app-lz-bff.privshield.internal"    # 调度之眼 BFF → 限定服务
    role: "app-lz-bff"
    scopes: ["servicehub:*", "audit:*", "datasource:*"]
  - cn: "service-hub.privshield.internal"   # 调度中枢 → 限定方法
    role: "service-hub"
    scopes: ["privacy:process", "audit:record", "datasource:fetch"]
  - cn: "external-hospital-client"          # 外部调用方 → 仅脱敏接口
    role: "external"
    scopes: ["privacy:mask"]
```

> 完整配置参见 [`config/mtls-whitelist.yaml`](../../config/mtls-whitelist.yaml)（83 行，含注释与版本信息）。

#### 3. 实现要点
- **白名单配置** (`config/mtls-whitelist.yaml`)：YAML 格式定义 CN → role → scopes 映射，支持通配符 `"*"` 和精确权限匹配；
- **动态热重载** (`pkg/tlsutil/whitelist.go` / `engine/security/whitelist.py`)：基于文件 mtime 轮询（默认 5 秒）自动重载授权列表；
- **gRPC 拦截器**：`pkg/tlsutil/grpc_interceptor.go` 从 TLS peer 证书提取 CN，与白名单比对并校验 Scope，未授权 CN 返回 `PermissionDenied`；
- **Fail-Closed 策略**：无证书、未验证证书或 CN 不在白名单中的请求一律拒绝。

> **注意**：当前 `config/mtls-whitelist.yaml` 主要由 Python Agent (`engine/security/whitelist.py`) 消费；Go 服务端 gRPC 拦截器已提供 (`pkg/tlsutil/grpc_interceptor.go`)，尚未在 service-hub/datasource-mgr/audit-log/bff-go 中显式注册，生产使用需通过 TLS client auth 与静态 CN 列表做兜底。

---

### 专项方案 6：前端双控制台（Web & App-LZ）组件与规范收敛迁移 ✅

#### 1. 迁移目标与职责边界划分
- **`console/web`（全量隐私控制台）**：面向数据安全工程师，提供 4 大通用隐私原语、三层漏斗策略调优与算子性能基准测试；
- **`console/app-lz/web`（数联调度之眼）**：面向数据要素流通与业务运营，聚焦 `ds_yibao`（医保）与 `ds_kangyang`（康养）真实数据流水线、租约状态监控、字段手风琴对比与自动化测试大屏。

#### 2. UI 规范收敛实施
1. **状态指示器统一标准**：
   - `completed`: 翠绿色背景（`bg-emerald-500/10 text-emerald-400 border-emerald-500/20`）+ `IconCheckCircle`；
   - `running`: 靛蓝色背景（`bg-indigo-500/10 text-indigo-400 border-indigo-500/20`）+ 呼吸光晕圆点；
   - `failed`: 玫瑰红背景（`bg-rose-500/10 text-rose-400 border-rose-500/20`）+ `IconXCircle`；
   - `pending`: 蓝灰色背景（`bg-slate-800 text-slate-400 border-slate-700`）。
2. **预设数据 API 动态渲染**：
   - 彻底废除前端写死 API 列表的逻辑，统一通过 `GET /api/lz/data-api/definitions` 动态拉取卡片列表，自动适配未来新增的 `ds_xx1` 等新数据源。

---

## 4. 迁移风险矩阵、回滚预案与应急响应 (Rollback Playbook)

| 潜在故障风险 | 严重等级 | 触发指征 | 应急处置与一键回滚操作 |
|:---|:---:|:---|:---|
| **1. 错误信封解析导致老客户端报错** | High | 前端抛出 Unhandled Exception 或无法渲染 | 信封设计已向后兼容：响应体保留 `detail` 字段兼容旧格式；紧急时可回滚前端至上一版本 |
| **2. PostgreSQL 数据库连接池耗尽** | Critical | PG 写入超时、租约争抢报错连接拒绝 | 清除 `PG_DSN` 环境变量，自动回滚至本地 SQLite WAL 模式，保障核心流通不中断 |
| **3. 审计哈希链断裂 (Broken Chain)** | Critical | VerifyChain 响应返回 broken_at_id 异常 | 运行 `repair_hash_chain` 工具重新锚定断点记录，恢复区块链式连续性 |
| **4. mTLS 热重载配置文件格式损坏** | High | 正常客户端报 403 / PermissionDenied | 恢复 `mtls-whitelist.yaml.bak` 备份文件，轮询（默认 5 秒）自动热更新重载恢复 |
| **5. 隐私预算耗尽导致服务不可用** | High | DP 查询返回 429 / BudgetExhausted 异常 | 配置 `PRIVACY_BUDGET_WINDOW_SECONDS` 自动重置；多实例部署启用 `PRIVACY_BUDGET_DB` |
| **6. 审计日志膨胀超出存储配额** | Medium | 磁盘空间告警 / 查询延迟上升 | 配置 `AUDIT_LOG_RETENTION_DAYS`（默认 90）自动清理超期记录；PG 启用分区表 |
| **7. LLM 推理 OOM 或推理超时** | High | 进程崩溃 / OOM Killer 杀进程 | `PRIVACY_LLM_MAX_CONCURRENCY` 信号量限流；`PRIVACY_LLM_MIN_FREE_MEM_MB` 内存阈值降级 |
| **8. 网关后端全部熔断** | High | 所有请求返回 503 | 主动健康检查持续探测，半开状态单请求恢复；全部节点故障时检查后端进程与网络连通性 |



---

## 5. 全栈迁移验证与验收测试套件 (Verification DoD)

迁移完成后，需依次执行以下验收测试套件，满足 **100% 通过（Definition of Done）** 准则：

### 5.1 自动化测试命令清单

```bash
# 1. 运行所有 Go 共享库与核心微服务测试（-count=1 禁用缓存）
go test -count=1 ./pkg/... ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/... ./console/bff-go/... ./console/app-lz/bff-go/...

# 2. 运行 Python 核心隐私引擎测试
PYTHONPATH=. pytest tests/ -q

# 3. 执行端到端全链路集成测试（基于 curl 的 HTTP 端到端集成测试，暂未覆盖 gRPC）
bash ./scripts/dev/integration-test-new-modules.sh

# 4. 执行 App-LZ 自动化测试套件（TS-01 ~ TS-04）
# 注意：PRIVSHIELD_E2E=1 当前未被 mock-based runner 测试消费；以下命令运行的是 mock E2E suites。
go test -v -run TestRunSuites ./console/app-lz/bff-go/internal/runner/

# 5. 前端编译与类型检查
cd console/app-lz/web && pnpm build
cd ../../web && pnpm build
```

### 5.2 核心业务功能验收标准
- [x] **SSOT 唯一事实源**：`ds_yibao` 与 `ds_kangyang` 全链路无字面量硬编码，新增数据源 5 步即可上线；
- [x] **9 要素哈希链与验真**：`POST /api/audit/chain/verify` 返回 `valid: true`，哈希链无任何断裂；
- [x] **信封加密**：数据库中快照样本全部携带 `enc:v1:` 密文前缀，读取时透明还原；
- [x] **Phase B 租约并发**：20 个并发任务无死锁、无重复执行（TS-03 100% 通过）；
- [x] **全链路追踪**：各服务日志中均输出一致的 `X-Request-ID`；
- [x] **Prometheus 指标暴露**：Python `/metrics` 与 Go `/metrics` 均可抓取，包含请求计数、延迟直方图、隐私原语操作计数；
- [x] **DDoS 防护中间件**：所有 Go 服务启用 `MaxBodySize` + `MaxConcurrent` + `RateLimit`（RPS=0 可关闭），Python 启用 `limit_concurrency` + `limit_max_requests`；
- [x] **优雅停机**：所有 Go 服务使用 `signal.NotifyContext`，Python 使用 `timeout_graceful_shutdown`，在途请求排空完成后再退出；
- [x] **熔断器保护**：Agent 客户端与 Gateway 负载均衡器均具备三态熔断器（Closed/Open/Half-Open）；
- [x] **数据保留策略**：审计日志超期自动清理（`AUDIT_LOG_RETENTION_DAYS`，默认 90 天）。

### 5.3 测试覆盖统计摘要

| 测试域 | 命令 | 覆盖范围 |
|---|---|---|
| Go 共享库 | `go test ./pkg/...` | 中间件 / 指标 / 命名 / 存储 / TLS / 校验 |
| Go 微服务群 | `go test ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/...` | service-hub / datasource-mgr / audit-log |
| Go 控制台 | `go test ./console/bff-go/... ./console/app-lz/bff-go/...` | bff-go / app-lz/bff-go（含 E2E runner） |
| Python 引擎 | `pytest tests/ -q` | REST API / 隐私原语 / 分类漏斗 / 网关 / 安全 / 可观测性 |
| 前端 | `pnpm build` + vitest | console/web + console/app-lz/web 类型检查与单元测试 |

---

## 6. 全栈可观测性体系设计 (Observability Architecture)

> 原设计仅覆盖了分布式追踪（专项 2），缺失了 Prometheus 指标体系、结构化日志规范与 OpenTelemetry 集成设计。本节补齐。

### 6.1 Prometheus 指标体系

#### Python 引擎端 (`engine/observability/metrics.py`)

<details>
<summary>点击展开完整 Python 指标参考表（40 个指标）</summary>

##### 核心隐私原语

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_requests_total` | Counter | `method`, `path`, `status` | REST/gRPC 请求计数 |
| `privacy_request_duration_seconds` | Histogram | `method`, `path` | 请求延迟分布（P50/P95/P99） |
| `privacy_dp_queries_total` | Counter | `mechanism`, `aggregation` | 差分隐私查询计数 |
| `privacy_dp_duration_seconds` | Histogram | `mechanism` | DP 操作延迟 |
| `privacy_masking_operations_total` | Counter | `operation` | 脱敏操作计数 |
| `privacy_masking_duration_seconds` | Histogram | `operation` | 脱敏操作延迟 |
| `privacy_kano_operations_total` | Counter | `operation` | K-匿名操作计数 |
| `privacy_kano_duration_seconds` | Histogram | `operation` | K-匿名操作延迟 |
| `privacy_qol_operations_total` | Counter | `domain` | 查询混淆操作计数 |
| `privacy_qol_duration_seconds` | Histogram | `domain` | 查询混淆操作延迟 |
| `privacy_auth_denials_total` | Counter | `reason` | 认证拒绝计数 |
| `privacy_auth_duration_seconds` | Histogram | — | 认证操作延迟 |
| `privacy_traffic_bytes_total` | Counter | `method`, `path`, `direction` | 网络流量字节数 |
| `privacy_profile_resolve_total` | Counter | `profile` | 隐私配置解析计数 |
| `privacy_data_extraction_total` | Counter | `source` | 数据提取操作计数 |

##### 隐私预算

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_budget_remaining` | Gauge | `namespace`, `budget_type` | 剩余隐私预算（epsilon/delta） |

##### 动态分类漏斗

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_classification_total` | Counter | `final_level`, `layer` | 分类漏斗各层结果计数 |
| `privacy_classification_jobs_total` | Counter | `status` | 分类任务触发计数 |
| `privacy_classification_jobs_duration_seconds` | Histogram | `status` | 分类任务延迟 |
| `privacy_classification_rule_hits_total` | Counter | `rule_id` | 规则引擎命中计数 |
| `privacy_classification_ner_total` | Counter | `status` | NER 实体识别计数 |
| `privacy_classification_llm_total` | Counter | `status` | LLM 仲裁结果计数 |
| `privacy_classification_composite_hits_total` | Counter | `rule_id` | 复合规则命中计数 |
| `privacy_classification_ner_duration_seconds` | Histogram | — | NER 推理延迟 |
| `privacy_classification_llm_duration_seconds` | Histogram | — | LLM 推理延迟 |
| `privacy_classification_llm_tokens_total` | Counter | `direction` | LLM Token 消耗量 |
| `privacy_classification_duration_seconds` | Histogram | `source` | 分类端到端延迟 |

##### 分类引擎内部（同 `engine/observability/metrics.py`）

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `classification_rule_hits_total` | Counter | `rule_id`, `domain`, `standard` | 规则命中计数（含领域/体系） |
| `classification_operator_calls_total` | Counter | `operator`, `result` | 算子调用计数（含结果） |
| `classification_operator_errors_total` | Counter | `operator`, `rule_id` | 算子错误计数 |
| `classification_engine_load_duration_seconds` | Histogram | `domain`, `standard` | 规则引擎加载延迟 |
| `classification_profile_cache_size` | Gauge | — | 配置 Profile 缓存大小 |
| `classification_override_suppressed_total` | Counter | `domain`, `suppressed_rule_id` | 安全覆盖抑制计数 |

##### 网关

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_gateway_healthy_nodes` | Gauge | — | 网关健康后端节点数 |
| `privacy_gateway_retries_total` | Counter | `protocol`, `reason` | 网关重试计数 |
| `privacy_gateway_circuit_breaker_state` | Gauge | `node` | 熔断器状态（0=closed, 1=open, 2=half_open） |
| `privacy_gateway_requests_total` | Counter | `method`, `status` | 网关请求计数 |
| `privacy_gateway_latency_seconds` | Histogram | `method` | 网关请求延迟 |
| `privacy_gateway_node_admin_state` | Gauge | `node` | 网关节点管理状态 |

</details>

> **注意**：部分指标（如 `privacy_classification_*`、`privacy_*_duration_seconds`）已在 `engine/observability/metrics.py` 中定义，但尚未在全部代码路径中埋点，实际暴露以代码调用为准。

#### Go 微服务端 (`pkg/metrics/metrics.go`)

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `http_requests_total` | Counter | `method`, `path`, `status` | HTTP 请求计数 |
| `http_request_duration_seconds` | Histogram | `method`, `path` | HTTP 请求延迟 |
| `agent_requests_total` | Counter | `method`, `status` | Agent gRPC 调用计数 |
| `agent_request_duration_seconds` | Histogram | `method` | Agent gRPC 调用延迟 |
| `orphaned_tasks_recovered_total` | Counter | — | 崩溃恢复时回收的孤儿任务数 |
| `tasks_retried_total` | Counter | — | 自动重试的任务数 |
| `circuit_breaker_state` | Gauge | `node` | Agent 客户端熔断器状态 |
| `task_lease_conflicts_total` | Counter | — | 租约争抢冲突计数 |
| `task_lease_expired_total` | Counter | — | 超期失效的租约计数 |
| `task_claim_latency_seconds` | Histogram | — | 任务领取延迟 |
| `task_transitions_total` | Counter | `from`, `to`, `result` | 任务状态转换计数 |
| `service_hub_ready` | Gauge | — | Service-Hub 就绪状态（1=就绪） |
| `privshield_api_alias_requests_total` | Counter | `alias`, `canonical`, `target` | 别名 API 请求计数 |
| `privshield_datasource_normalize_errors_total` | Counter | `reason` | 数据源归一化失败计数 |
| `privshield_datasource_requests_total` | Counter | `datasource_id`, `api_code`, `status` | 数据源请求计数 |

每个 Go 服务使用独立的 `prometheus.Registry`，避免全局注册冲突。暴露 `/metrics` 端点供 Prometheus 或 ServiceMonitor 抓取。

> **注意**：`service_hub_ready` 与 `circuit_breaker_state` 已在 `pkg/metrics/metrics.go` 中定义，但目前生产代码尚未更新这两个 gauge，状态以日志与探针为准。

### 6.2 结构化日志规范

#### Python 引擎
- 通过 `PRIVACY_LOG_FORMAT` 环境变量切换 `text`（开发）或 `json`（生产）格式。
- JSON 格式使用 `python-json-logger`，每条日志自动注入 `service`、`trace_id`、`timestamp` 字段。
- 所有隐私操作日志强制携带 `extra={"trace_id": ...}` 上下文。

#### Go 微服务
- 使用标准 `log/slog` 结构化日志，JSON 格式输出。
- 每条日志自动注入 `trace_id`、`service`、`component` 字段。
- 审计日志额外携带 `integrity_hash` 与 `prev_hash` 用于哈希链验真。

### 6.3 OpenTelemetry 分布式追踪

Python 引擎可选启用 OpenTelemetry（`engine/observability/tracing.py`）：

```text
┌────────────────┐     OTLP/gRPC      ┌──────────────────┐
│  Python Engine  │ ─────────────────▶ │ Jaeger / Tempo   │
│  (SpanExporter) │                    │ (Trace Backend)  │
└────────────────┘                    └──────────────────┘
```

- 通过 `OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量激活，未设置时为 no-op。
- 支持 `BatchSpanProcessor` 批量导出，减少网络开销。
- Span 自动关联 `X-Request-ID`，与 Go 端 TraceMiddleware 形成完整调用链。

### 6.4 Grafana 仪表盘与告警

- 预置仪表盘模板：`deploy/grafana/dashboard.json` 与 `deploy/grafana/service-hub-dashboard.json`。
- K8s 部署通过 `ServiceMonitor` CRD 自动注册 Prometheus 抓取目标（`deploy/helm/PrivShield/templates/servicemonitor.yaml`）。
- 推荐告警规则：
  - `privacy_requests_total{status=~"5.."}` 5 分钟速率突增 → P1 告警
  - `privacy_gateway_healthy_nodes == 0` → P0 告警（全后端不可用）
  - `circuit_breaker_state > 0` 持续 5 分钟 → P2 告警（后端异常）

---

## 7. 韧性与安全加固设计 (Resilience & Security Hardening)

> 原设计缺失跨服务韧性模式（熔断/重试/降级）、DDoS 防护中间件层、优雅停机协议与隐私预算会计模型的设计说明。本节补齐。

### 7.1 跨服务韧性模式

#### 7.1.1 熔断器（Circuit Breaker）

系统中有两处关键熔断器实现：

| 位置 | 保护目标 | 参数 |
|---|---|---|
| `pkg/agent/client.go` | Agent gRPC 客户端 → Engine | 连续失败 5 次触发，30 秒冷却 |
| `engine/gateway/balancer.py` | Gateway → 多后端 Engine 节点 | 连续失败 5 次触发，30 秒冷却，半开单探测许可证 |

三态模型：`Closed`（正常）→ `Open`（熔断）→ `Half-Open`（单请求探测恢复）。

#### 7.1.2 重试策略

| 组件 | 重试条件 | 最大次数 | 退避策略 |
|---|---|---|---|
| Gateway HTTP 代理 | 幂等方法或 ConnectError | 3 | 指数退避 + 随机抖动 |
| Gateway gRPC 代理 | UNAVAILABLE 或未知异常 | 3 | 指数退避 + 随机抖动 |
| BFF-Go → Agent gRPC | gRPC 服务配置 `retryPolicy` | 按配置 | 指数退避 |
| Service-Hub → Datasource | HTTP 连接失败 | 按配置 | 指数退避 |

#### 7.1.3 分类漏斗降级链

- **Layer-1 规则引擎**：始终运行，并对医疗/默认等领域应用 Safety Floor 高敏感度标签；存在正常标签与降级标签时会进行冲突检测。
- **Layer-2 Small-NER**：仅在 `_should_trigger_ner` 通过时触发；会跳过短结构化字段、纯数字字段、英文字段，且仅当字段 rank ≤ `ner_trigger_max_rank` 时才进入 NER。
- **Layer-3 Local LLM**：包含三个独立触发条件——仲裁模式（`PRIVACY_LLM_ENABLE_ARBITRATION`）、图片自动触发（`PRIVACY_LLM_AUTO_ON_IMAGE`）以及低置信度启用（`PRIVACY_LLM_ENABLE` + 置信度低于阈值）。
- **Fallback（回退）**：当 NER/LLM 不可用或内存不足时，不采用独立“保守回退层”，而是通过置信度衰减与 `needs_human_review` 标记完成降级。

### 7.2 DDoS 防护与安全中间件层

#### Go 微服务中间件栈 (`pkg/middleware/`)

4 个核心 Go 服务（service-hub, audit-log, datasource-mgr, app-lz/bff-go）统一启用以下 9 层中间件链（顺序严格一致）：

```text
TraceMiddleware → StructuredLogger → Recovery → SecurityHeaders → MaxBodySize → MaxConcurrent → [RateLimit] → CORS → Auth
```

| 中间件 | 功能 | 配置参数 |
|---|---|---|
| `TraceMiddleware()` | 自动注入/传播 X-Request-ID，双头下发 | — |
| `StructuredLogger(logger, module)` | 每请求结构化日志（method/path/status/latency） | `*_LOG_FORMAT` |
| `Recovery(logger, module)` | 全局 panic 恢复，返回 500 而非崩溃 | — |
| `SecurityHeaders()` | 注入 CSP/HSTS/X-Frame-Options/X-Content-Type-Options | 固定值 |
| `MaxBodySize(maxBytes)` | 限制请求体大小，防止大包 OOM | 32 MB（默认）；bff-go 为 64 MB（支持大文件上传） |
| `MaxConcurrent(limit)` | 限制在途请求总数，防止并发耗尽资源 | 1000（默认） |
| `RateLimit(rps, burst)` | 每客户端 IP 令牌桶限流（RPS=0 时跳过） | 100 rps / 200 burst（默认） |
| `CORS(origins)` | 可配置跨域来源 | 环境变量 |
| `Auth(apiKey)` | API Key 鉴权（为空时跳过） | 环境变量 |

> **特殊说明**：`console/bff-go` 使用本地 `securityMiddleware()` 替代独立的 `RateLimit` + `Auth`，该函数将 API Key 鉴权与滑动窗口限流（`CONSOLE_RATE_LIMIT`，默认 600 req/min）合并为单一中间件，并配备后台 goroutine 定期清理过期 IP 条目防止内存泄漏。

#### Python 引擎防护

| 参数 | 默认值 | 用途 |
|---|---|---|
| `PRIVACY_LIMIT_CONCURRENCY` | 10000 | Uvicorn 最大并发连接数 |
| `PRIVACY_LIMIT_MAX_REQUESTS` | 100000 | 单连接最大请求数（防内存泄漏） |
| `PRIVACY_TIMEOUT_KEEP_ALIVE` | 30 | 空闲连接超时（秒） |
| `PRIVACY_MAX_BODY_SIZE` | 32 MiB | 最大请求体大小 |
| `PRIVACY_MAX_UPLOAD_BYTES` | 10 MiB | 最大上传文件大小 |
| `PRIVACY_TIMEOUT_GRACEFUL_SHUTDOWN` | 10 s | 优雅停机超时 |
| REST / gRPC 限流 | identity + path / method | REST 限流通过 `Depends(rate_limit_dependency)` 按 identity + path 控制；gRPC 限流通过 `RateLimitInterceptor` 按 identity + method 控制 |

### 7.3 优雅停机协议

#### Go 服务

```text
SIGTERM/SIGINT 到达
  → 停止接收新连接
  → 排空在途请求（最长 shutdown_timeout 秒）
  → 持久化未完成任务状态到 SQLite/PG
  → 关闭数据库连接池
  → 退出（exit 0）
```

所有 5 个 Go 服务（service-hub, audit-log, datasource-mgr, console/bff-go, console/app-lz/bff-go）统一使用 `signal.NotifyContext`（Go 1.16+）监听 SIGINT/SIGTERM，信号到达时自动取消 context，通过 `http.Server.Shutdown(ctx)` 或 `grpcServer.GracefulStop()` 实现排空。

#### Python 服务

所有 4 个 Python 入口（`main.py`, `server.py`, `launcher.py`, `gateway/server.py`）统一使用 uvicorn 的 `timeout_graceful_shutdown` 参数（默认 10 秒）。Python gRPC 独立模式使用 `server.stop(grace=5)` 排空在途 RPC。

### 7.4 隐私预算会计模型

`BudgetAccountant`（`engine/privacy/budget.py`）提供严格的差分隐私预算管理：

| 能力 | 实现 |
|---|---|
| 命名空间隔离 | 每个 `namespace` 独立追踪 epsilon/delta 消耗 |
| 预算耗尽保护 | 累计消耗超过上限时抛出 `PrivacyBudgetExhaustedError` |
| 时间窗口自动重置 | `PRIVACY_BUDGET_WINDOW_SECONDS` 配置周期重置 |
| 多实例一致性 | `PRIVACY_BUDGET_DB` (SQLite/PG) 支持跨实例预算同步 |
| 审计日志 | `BudgetAuditLogger` 记录每次 epsilon/delta 支出到防篡改日志 |

### 7.5 数据生命周期管理

| 数据类型 | 保留策略 | 清理机制 |
|---|---|---|
| 审计日志 (`audit_logs`) | `AUDIT_LOG_RETENTION_DAYS`（默认 90 天） | 超期自动清理，保留哈希链完整性 |
| 任务记录 (`tasks`) | 按服务配置 | 已完成任务定期归档 |
| 隐私预算日志 | 永久保留 | 仅追加，不删除 |
| 快照加密数据 | 跟随审计日志 | AES-256-GCM 密文随日志一同清理 |

---

## 8. 生产部署基础设施设计 (Production Deployment Infrastructure)

> 原设计仅提及 Docker/Helm/K8s 的基本安装命令，缺失 K8s 生产级基础设施的架构设计。本节补齐。

### 8.1 K8s 生产级能力矩阵

| 能力 | Helm 模板 | 生产启用条件 |
|---|---|---|
| 水平自动扩缩 (HPA) | `templates/hpa.yaml` | `autoscaling.enabled=true`，CPU 80% / 内存 80% 阈值，2~10 副本 |
| 潮汐预测扩缩 (CronHPA) | `templates/cron-hpa.yaml` | 业务高峰期定时扩容 |
| Pod 中断预算 (PDB) | `templates/poddisruptionbudget.yaml` | `podDisruptionBudget.enabled=true`，保障滚动更新时最小可用副本数 |
| 网络策略 (NetworkPolicy) | `templates/networkpolicy.yaml` | `networkPolicy.enabled=true`，同命名空间隔离 |
| Prometheus 集成 (ServiceMonitor) | `templates/servicemonitor.yaml` | `serviceMonitor.enabled=true`，自动注册抓取目标 |
| 启动探针 (startupProbe) | `templates/deployment.yaml` | 保护慢启动应用（ML 模型加载），最长 150 秒 |
| 存活探针 (livenessProbe) | `templates/deployment.yaml` | `/health` 端点，周期性检查 |
| 就绪探针 (readinessProbe) | `templates/deployment.yaml` | `/readyz` 端点，检查上游连通性 |

### 8.2 数据库 Schema 迁移策略

当前采用**增量 ALTER TABLE** 模式（`pkg/store/sqlite/init.go` 与 `pkg/store/postgres/schema.go`）：

- **Phase A (SQLite)**：服务启动时自动执行 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`，幂等安全。
- **Phase B (PostgreSQL)**：使用 `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 增量演进。
- **迁移工具**：`pkg/store/cmd/migrate/main.go`（包装器 `scripts/prod/migrate_sqlite_to_postgres.sh`）提供 SQLite → PostgreSQL 的原子割接，带哈希链完整性校验。

> **设计改进方向**：当 Schema 变更频率增加时，应引入正式的迁移框架（如 `golang-migrate` 或 `goose`），
> 支持版本号追踪、回滚和 CI 集成。当前增量 ALTER 模式适用于低频变更阶段。

### 8.3 API 版本控制策略

当前代码库使用 `/v1/` 路径前缀（如 `/v1/privacy/mask`、`/v1/dynclassification/eval`），但尚未制定正式的 API 版本演进策略。

**推荐策略**：
- URL 路径版本控制：`/v1/...` → `/v2/...`
- 旧版本至少维护 2 个发布周期后标记 Deprecated
- BFF 层负责版本路由与协议转换
- gRPC 通过 `.proto` 文件的 `package` 版本实现向后兼容

---

## 9. 开发者快速参考卡 (Developer Quick Reference)

<details>
<summary>点击展开常用命令速查</summary>

### 本地开发启动

```bash
# Python 引擎 (REST + gRPC)
python -m engine.server                    # http://127.0.0.1:8079 + grpc://127.0.0.1:50051

# Go 微服务群 (需分别启动)
cd services/service-hub && go run cmd/server/main.go        # :8082
cd services/datasource-mgr && go run cmd/server/main.go     # :8083
cd services/audit-log && go run cmd/server/main.go          # :8084

# 控制台 (Agent + Go BFF + Vite HMR)
bash ./scripts/dev/dev-bff-agent.sh        # :8079 + :8081 + :5173

# App-LZ 测试控制台
bash ./scripts/dev/dev-app-lz.sh           # :8085
```

### 测试

```bash
# Go 全量测试（禁用缓存）
go test -count=1 ./pkg/... ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/... ./console/bff-go/... ./console/app-lz/bff-go/...

# Python 全量测试
PYTHONPATH=. pytest tests/ -q

# 集成测试（基于 curl 的 HTTP 端到端测试，暂未覆盖 gRPC）
bash ./scripts/dev/integration-test-new-modules.sh

# App-LZ E2E 测试（mock-based runner；PRIVSHIELD_E2E=1 当前未被其消费）
go test -v -run TestRunSuites ./console/app-lz/bff-go/internal/runner/
```

### 构建与部署

```bash
# Docker 镜像
docker build --target core -t privshield:1.8.0 .    # 轻量核心镜像
docker build --target ml -t privshield:1.8.0-ml .   # 含 ML 依赖镜像

# Helm 部署
helm install privshield ./deploy/helm/PrivShield

# Docker Compose 全栈
bash ./scripts/prod/deploy-docker-compose.sh --with-postgres
```

### 运维工具

```bash
# 隐私预算备份
bash ./scripts/prod/backup_privacy_budget.sh

# SQLite → PostgreSQL 迁移（干运行）
bash scripts/prod/migrate_sqlite_to_postgres.sh --dry-run

# 审计哈希链验真
curl -X POST http://localhost:8084/api/audit/chain/verify

# 生产健康检查
bash ./scripts/prod/prod_health_check.sh
```

</details>

---

## 附录 A. 文档修订历史

| 版本 | 日期 | 核心变更 |
|---|---|---|
| v1.0 | 2026-08 | 初始版本：全栈协同度评估 + 六大专项迁移方案 |
| v2.0 | 2026-08-27 | 新增 §6 可观测性 + §7 韧性安全 + §8 部署基础设施 |
| v3.0 | 2026-08-28 | 设计文档优化 + app-lz RateLimit 补齐 |
| v4.0 | 2026-08-28 | §1.2 短板标记已消除、§3 代码块精简（-350 行）、§2.1 服务通信拓扑、§2.2 环境变量速查 |
| v5.0 | 2026-08-28 | 指标名拼写修正、中间件链差异化说明、Makefile `-count=1` |
| v6.0 | 2026-08-28 | §2.3 REST API 端点速查、Go 环境变量扩展、移除不存在的 `FEATURE_FLAG` |
| v7.0 | 2026-08-28 | 服务拓扑补齐 gRPC :50055、mTLS YAML 精简、测试统计摘要 |
| v8.0 | 2026-08-28 | Python 指标 18→40 精确对齐、Go 指标 7 处修正、Helm 文件名修正 |
| v9.0 | 2026-08-28 | 移除拓扑矩阵虚假链路、§1.2/§8.3 细节修正 |
| v10.0 | 2026-08-28 | datasource-mgr 升级至 Level 5、新增 Python 引擎 50+ API 端点速查 |
| v11.0 | 2026-08-28 | 架构图补齐服务间链路、新增 §9 快速参考卡 + 附录 A 修订历史 |
| v12.0 | 2026-08-28 | 修复测试构建失败 + Go workspace 测试命令修正 |
| v13.0 | 2026-08-28 | 修正分类引擎指标文件路径（不存在 dynclassification/metrics.py）+ 5 处标签对齐代码 |
| **v14.0** | **2026-08-28** | **修正 gRPC 客户端文件路径引用（pkg/agent/grpc_client.go → console/bff-go/internal/agent/client.go）** |

## 附录 B. 专业名词详解 (Glossary)

> **定位**：本文档为 PrivShield 全栈架构设计中涉及的专业术语、缩写及技术概念提供标准化释义，供架构师、开发者及运维人员快速参考。
> **版本**：v14.0.0 配套
> **最后更新**：2026-08-28

---

### B.1 架构与系统模式

| 术语                  | 英文全称               | 释义                                                         |
| :-------------------- | :--------------------- | :----------------------------------------------------------- |
| **BFF**               | Backend for Frontend   | **后端为前端**模式。针对特定前端（Web/移动端）定制后端聚合层，避免前端直接调用多个微服务，减少网络往返与适配复杂度。PrivShield 中 `console/bff-go` 与 `app-lz/bff-go` 即为两类前端的专属 BFF。 |
| **Sidecar**           | —                      | **边车模式**。将辅助功能（如日志、监控、安全）从主业务容器中剥离为独立进程/容器，随主容器同生命周期部署。PrivShield 早期为 Python 隐私引擎 Sidecar，后演进为独立服务集群。 |
| **SSOT**              | Single Source of Truth | **唯一事实源**。全栈唯一、权威的数据或配置来源，消除多副本不一致。PrivShield 中 `pkg/naming` 为数据源标识的 SSOT，禁止任何硬编码别名。 |
| **Zero Trust**        | —                      | **零信任安全模型**。默认不信任任何内外部请求，每次访问均需验证身份、设备与权限，最小权限原则。PrivShield 通过 mTLS + CN 白名单 + API Key 三层实现零信任。 |
| **Fail-Closed**       | —                      | **故障关闭策略**。当安全校验异常（如证书无效、白名单未命中）时，默认拒绝访问而非放行。与 Fail-Open（故障打开）相对，是安全系统的首选策略。 |
| **E2E**               | End-to-End             | **端到端**。指从用户发起请求到最终数据返回的完整链路测试，覆盖所有中间环节，验证全系统集成正确性。 |
| **Graceful Shutdown** | —                      | **优雅停机**。服务在收到终止信号（SIGTERM）后，停止接收新请求，等待在途请求处理完毕再退出，避免强制中断导致的数据不一致。 |

---

### B.2 隐私计算与数据安全

| 术语                             | 英文全称                                                     | 释义                                                         |
| :------------------------------- | :----------------------------------------------------------- | :----------------------------------------------------------- |
| **DP**                           | Differential Privacy                                         | **差分隐私**。一种严格的数学隐私保护框架，通过在查询结果中添加可控噪声，确保单条记录的 presence/absence 不会显著改变输出，从而防止成员推断攻击。PrivShield 支持全局 DP（Central DP）与本地 DP（Local DP, LDP）。 |
| **ε (Epsilon)**                  | Privacy Budget                                               | **隐私预算**。差分隐私的核心参数，表示单条记录对查询结果的最大影响程度（对数尺度）。ε 越小隐私保护越强，但数据可用性越低。每次查询消耗预算，耗尽后拒绝服务。 |
| **δ (Delta)**                    | —                                                            | **隐私预算松弛项**。(ε, δ)-DP 中的 δ 表示隐私保护失败的概率上限（通常取极小值如 10⁻⁶），允许极大概率下的严格隐私保证。 |
| **K-Anonymity**                  | —                                                            | **K-匿名**。一种数据脱敏模型，要求发布的数据中每条记录在准标识符（Quasi-Identifier）上至少与 K-1 条其他记录不可区分，降低重识别风险。 |
| **QoL**                          | Query Obfuscation Layer                                      | **查询混淆层**。PrivShield 四大隐私原语之一，通过改写查询条件、添加伪条件或扰动查询范围，防止攻击者通过精确查询反推敏感个体。 |
| **Masking**                      | Data Masking                                                 | **数据脱敏**。对敏感字段进行部分遮蔽、替换或泛化（如手机号 `138****8888`），保留数据格式但隐藏真实内容。 |
| **Privacy Primitive**            | —                                                            | **隐私原语**。构成隐私计算体系的基础原子操作。PrivShield 定义四大原语：Masking（脱敏）、DP（差分隐私）、K-Anon（K-匿名）、QoL（查询混淆）。 |
| **Privacy Budget Accountant**    | —                                                            | **隐私预算会计**。追踪并管理各命名空间（namespace）下 ε/δ 的消耗与剩余，防止超支，支持时间窗口自动重置与跨实例同步。 |
| **Hash Chain**                   | —                                                            | **哈希链**。将审计日志条目按时间顺序串联，每条记录包含前一条记录的哈希值，形成区块链式防篡改结构。PrivShield 的 9 要素哈希链确保审计日志的完整性与可追溯性。 |
| **HMAC**                         | Hash-based Message Authentication Code                       | **基于哈希的消息认证码**。使用密钥与哈希函数生成的认证标签，用于验证数据完整性与真实性。PrivShield 用于敏感字段的确定性哈希。 |
| **AES-256-GCM**                  | Advanced Encryption Standard - 256 bit - Galois/Counter Mode | **高级加密标准-256位-GCM模式**。对称加密算法，提供机密性（AES-256）与认证加密（GCM），防止密文篡改。PrivShield 用于快照信封加密，密文前缀标记为 `enc:v1:`。 |
| **Snapshot Envelope Encryption** | —                                                            | **快照信封加密**。对审计快照数据先进行 AES-256-GCM 加密，再将密文存入数据库，读取时透明解密。即使数据库被拖库，敏感内容仍为密文。 |
| **Conservative Fallback**        | —                                                            | **保守回退**。当高级隐私机制（如 LLM 仲裁）不可用时，系统不降格安全等级，而是采用更保守的处理策略（如提升分类敏感度、增强脱敏强度），确保安全底线不被突破。 |

---

### B.3 通信协议与网络安全

| 术语              | 英文全称                              | 释义                                                         |
| :---------------- | :------------------------------------ | :----------------------------------------------------------- |
| **gRPC**          | Google Remote Procedure Call          | **Google 远程过程调用**。基于 HTTP/2 与 Protocol Buffers 的高性能 RPC 框架，支持双向流、头部压缩与强类型接口。PrivShield 用于 service-hub → engine 及 BFF → engine 的内部高效通信。 |
| **mTLS**          | Mutual TLS                            | **双向传输层安全**。客户端与服务端互相验证 X.509 证书，确保通信双方身份可信。PrivShield 通过 `pkg/tlsutil` 实现 mTLS + CN 白名单动态授权。 |
| **CN**            | Common Name                           | **通用名称**。X.509 证书中的主体标识字段，通常用于标识服务身份（如 `bff-go.privshield.internal`）。PrivShield 白名单基于 CN 进行访问控制。 |
| **CSP**           | Content Security Policy               | **内容安全策略**。HTTP 响应头，限制浏览器可加载的资源类型与来源，防止 XSS 与数据注入攻击。 |
| **HSTS**          | HTTP Strict Transport Security        | **HTTP 严格传输安全**。强制浏览器仅通过 HTTPS 访问站点，防止 SSL 剥离攻击。 |
| **CORS**          | Cross-Origin Resource Sharing         | **跨源资源共享**。浏览器安全机制，通过 HTTP 头控制哪些外部域可访问本域资源。PrivShield BFF 层统一配置 CORS 策略。 |
| **DDoS**          | Distributed Denial of Service         | **分布式拒绝服务**。通过大量分布式请求耗尽目标资源，导致服务不可用。PrivShield 通过 MaxBodySize、MaxConcurrent、RateLimit 三层中间件防护。 |
| **Rate Limiting** | —                                     | **速率限制**。控制单位时间内允许的请求数量，防止单客户端过度消耗资源。PrivShield 采用令牌桶（Token Bucket）与漏桶（Leaky Bucket）算法。 |
| **Leaky Bucket**  | —                                     | **漏桶算法**。流量整形算法，请求以固定速率流出处理，突发流量被缓存或丢弃，平滑请求曲线。 |
| **API Key**       | Application Programming Interface Key | **API 密钥**。用于标识与认证调用方身份的密钥字符串，通常通过 HTTP Header（如 `X-API-Key`）传递。 |

---

### B.4 可观测性与监控

| 术语                   | 英文全称               | 释义                                                         |
| :--------------------- | :--------------------- | :----------------------------------------------------------- |
| **Prometheus**         | —                      | **开源监控与告警系统**。通过拉取（Pull）方式采集时序指标数据，支持多维数据模型与 PromQL 查询语言。PrivShield 所有服务均暴露 `/metrics` 端点。 |
| **Grafana**            | —                      | **开源可视化仪表盘平台**。与 Prometheus 集成，提供图表、告警与仪表盘展示。 |
| **OpenTelemetry**      | —                      | **开源可观测性框架**。统一标准用于采集分布式追踪（Tracing）、指标（Metrics）与日志（Logs），支持 OTLP 协议导出至 Jaeger/Tempo 等后端。 |
| **OTLP**               | OpenTelemetry Protocol | **OpenTelemetry 协议**。OpenTelemetry 的标准传输协议，基于 gRPC/HTTP 将遥测数据发送至后端分析系统。 |
| **Trace ID**           | —                      | **追踪标识符**。分布式追踪中的全局唯一 ID，贯穿请求全生命周期，用于串联跨服务的调用链。PrivShield 通过 `X-Request-ID` / `X-Trace-ID` 双头传递。 |
| **Span**               | —                      | **跨度**。分布式追踪中的基本工作单元，表示一次具体的操作（如 HTTP 请求、数据库查询），包含起始时间、耗时与标签。 |
| **P50 / P95 / P99**    | Percentile             | **百分位延迟**。P50 表示 50% 请求低于该延迟（中位数），P95/P99 表示 95%/99% 请求低于该延迟，用于衡量尾部延迟与服务质量。 |
| **ServiceMonitor**     | —                      | **K8s 监控资源**。Prometheus Operator 的 CRD，自动发现带有特定标签的 Service 并配置抓取目标，无需手动修改 Prometheus 配置。 |
| **Structured Logging** | —                      | **结构化日志**。以 JSON 等结构化格式输出日志，便于机器解析与检索，每条日志包含固定字段（如 `trace_id`、`service`、`timestamp`）。 |

---

### B.5 云原生与容器编排

| 术语                | 英文全称                       | 释义                                                         |
| :------------------ | :----------------------------- | :----------------------------------------------------------- |
| **K8s**             | Kubernetes                     | **容器编排平台**。自动化部署、扩展与管理容器化应用的开源系统。 |
| **HPA**             | Horizontal Pod Autoscaler      | **水平 Pod 自动扩缩**。根据 CPU/内存/自定义指标自动调整 Pod 副本数量，应对负载变化。 |
| **CronHPA**         | Cron Horizontal Pod Autoscaler | **定时水平扩缩**。基于 Cron 表达式在预定时间自动扩缩容，适用于可预测的业务高峰（如医保结算日）。 |
| **PDB**             | Pod Disruption Budget          | **Pod 中断预算**。限制同一时刻可自愿中断（如滚动更新、节点维护）的 Pod 数量，保证最小可用副本。 |
| **NetworkPolicy**   | —                              | **网络策略**。K8s 资源，定义 Pod 间的网络访问规则，实现命名空间级别的微隔离。 |
| **Startup Probe**   | —                              | **启动探针**。检测容器应用是否已启动完成，在启动阶段替代 liveness 探针，避免慢启动应用被误杀。 |
| **Liveness Probe**  | —                              | **存活探针**。周期性检测容器是否正常运行，失败则重启容器。   |
| **Readiness Probe** | —                              | **就绪探针**。检测容器是否准备好接收流量，失败则从 Service 端点列表中移除，避免向异常实例转发请求。 |
| **Helm**            | —                              | **K8s 包管理工具**。通过 Chart（模板化配置包）简化复杂应用的部署与管理。 |
| **WAL**             | Write-Ahead Logging            | **预写式日志**。数据库事务机制，先写日志再写数据，保证崩溃恢复时的数据一致性。SQLite WAL 模式支持高并发读。 |
| **DSN**             | Data Source Name               | **数据源名称**。包含数据库连接信息的字符串（如 `postgres://user:pass@host/db`），用于建立数据库连接。 |

---

### B.6 韧性设计与高可用

| 术语                     | 英文全称 | 释义                                                         |
| :----------------------- | :------- | :----------------------------------------------------------- |
| **Circuit Breaker**      | —        | **熔断器**。韧性设计模式，当后端连续失败超过阈值时，快速失败（Open 状态）而非持续等待，防止级联故障；后端恢复后通过半开（Half-Open）状态探测逐步恢复流量。 |
| **Retry with Backoff**   | —        | **退避重试**。请求失败时按指数退避（Exponential Backoff）+ 随机抖动（Jitter）策略延迟重试，避免惊群效应。 |
| **Bulkhead**             | —        | **舱壁隔离**。将资源（线程池、连接池）按服务或优先级隔离，防止单服务故障耗尽全局资源。 |
| **Semaphore**            | —        | **信号量**。并发控制原语，限制同时访问某资源的线程/协程数量。PrivShield 用信号量限制 LLM 推理并发度。 |
| **Lease**                | —        | **租约**。分布式系统中的限时锁机制，任务执行前获取租约，超期未释放则自动过期，防止任务重复执行或孤儿任务。PrivShield Phase B 使用租约实现任务去重。 |
| **Idempotency**          | —        | **幂等性**。同一操作多次执行与一次执行结果相同。重试机制要求被重试的接口具备幂等性，避免重复副作用。 |
| **Graceful Degradation** | —        | **优雅降级**。当部分功能不可用时，系统降低服务质量（如关闭 LLM 层、简化分类策略）而非完全崩溃，保障核心功能可用。 |

---

### B.7 机器学习与 NLP

| 术语             | 英文全称                             | 释义                                                         |
| :--------------- | :----------------------------------- | :----------------------------------------------------------- |
| **LLM**          | Large Language Model                 | **大语言模型**。基于 Transformer 架构的大规模预训练语言模型，具备文本理解、生成与推理能力。PrivShield 第三层分类漏斗使用本地部署的 LLM 进行高复杂度隐私分类仲裁。 |
| **NER**          | Named Entity Recognition             | **命名实体识别**。NLP 任务，识别文本中具有特定意义的实体（如人名、身份证号、银行卡号）。PrivShield 第二层使用轻量 ONNX NER 模型识别敏感实体。 |
| **ONNX Runtime** | Open Neural Network Exchange Runtime | **开放神经网络交换运行时**。跨平台高性能推理引擎，支持多种深度学习框架导出的 ONNX 模型。PrivShield 用于本地轻量 NER 模型推理。 |
| **Token**        | —                                    | **令牌**。LLM 处理文本的最小单位（可为字、词或子词）。API 计费与上下文长度限制通常以 Token 数计量。 |
| **Inference**    | —                                    | **推理**。已训练模型对新输入数据进行预测的过程，区别于训练（Training）。 |
| **OOM**          | Out of Memory                        | **内存溢出**。进程申请的内存超过系统可用上限，被操作系统终止（OOM Killer）。PrivShield 通过内存阈值检测防止 LLM 推理触发 OOM。 |

---

### B.8 数据存储与数据库

| 术语                 | 英文全称                                      | 释义                                                         |
| :------------------- | :-------------------------------------------- | :----------------------------------------------------------- |
| **PostgreSQL**       | —                                             | **开源对象-关系型数据库**。支持 ACID 事务、复杂查询与高并发，PrivShield Phase B 生产环境首选。 |
| **SQLite**           | —                                             | **嵌入式关系型数据库**。零配置、单文件存储，适合单机或测试环境。PrivShield Phase A 使用 SQLite WAL 模式。 |
| **Schema Migration** | —                                             | **Schema 迁移**。数据库表结构的版本化管理，包括创建表、添加字段、建立索引等变更的追踪与回滚。 |
| **Connection Pool**  | —                                             | **连接池**。预先建立并缓存数据库连接，避免频繁创建/销毁连接的开销，限制最大并发连接数防止资源耗尽。 |
| **Partitioning**     | —                                             | **分区表**。将大表按规则（如时间范围）拆分为多个物理子表，提升查询性能与数据管理效率。PrivShield 建议审计日志表按时间分区。 |
| **ACID**             | Atomicity, Consistency, Isolation, Durability | **原子性、一致性、隔离性、持久性**。关系型数据库事务的四大特性，保证数据可靠性。 |

---

### B.9 前端与 UI 工程

| 术语             | 英文全称               | 释义                                                         |
| :--------------- | :--------------------- | :----------------------------------------------------------- |
| **HMR**          | Hot Module Replacement | **热模块替换**。开发环境下，修改代码后无需刷新页面即可实时更新对应模块，提升开发效率。Vite 提供此能力。 |
| **Vite**         | —                      | **下一代前端构建工具**。基于原生 ES 模块与 Rollup，提供极速冷启动与 HMR。 |
| **Tailwind CSS** | —                      | **实用优先的 CSS 框架**。通过原子化类名（如 `bg-emerald-500`）快速构建界面，无需编写自定义 CSS。 |
| **Accordion**    | —                      | **手风琴组件**。可展开/折叠的 UI 面板，用于在有限空间内展示分层信息。PrivShield App-LZ 用于字段对比展示。 |

---

### B.10 缩写速查表

| 缩写        | 全称                                                 | 中文                   | 所在章节     |
| :---------- | :--------------------------------------------------- | :--------------------- | :----------- |
| BFF         | Backend for Frontend                                 | 后端为前端             | §2, §3.6     |
| SSOT        | Single Source of Truth                               | 唯一事实源             | §3.3         |
| DP          | Differential Privacy                                 | 差分隐私               | §2.3, §7.4   |
| LDP         | Local Differential Privacy                           | 本地差分隐私           | §2.3         |
| K-Anon      | K-Anonymity                                          | K-匿名                 | §2.3         |
| QoL         | Query Obfuscation Layer                              | 查询混淆层             | §2.3         |
| NER         | Named Entity Recognition                             | 命名实体识别           | §2, §7.1.3   |
| LLM         | Large Language Model                                 | 大语言模型             | §2, §7.1.3   |
| gRPC        | Google Remote Procedure Call                         | Google 远程过程调用    | §2.1, §2.2   |
| mTLS        | Mutual Transport Layer Security                      | 双向传输层安全         | §2, §3.5     |
| CN          | Common Name                                          | 通用名称（证书）       | §3.5         |
| DDoS        | Distributed Denial of Service                        | 分布式拒绝服务         | §2, §7.2     |
| HMAC        | Hash-based Message Authentication Code               | 基于哈希的消息认证码   | §2.3         |
| AES-256-GCM | Advanced Encryption Standard 256 Galois/Counter Mode | 高级加密标准-256-GCM   | §3.4         |
| WAL         | Write-Ahead Logging                                  | 预写式日志             | §3.4         |
| DSN         | Data Source Name                                     | 数据源名称             | §2.2         |
| HPA         | Horizontal Pod Autoscaler                            | 水平 Pod 自动扩缩      | §8.1         |
| PDB         | Pod Disruption Budget                                | Pod 中断预算           | §8.1         |
| OTLP        | OpenTelemetry Protocol                               | OpenTelemetry 协议     | §6.3         |
| OOM         | Out of Memory                                        | 内存溢出               | §2.2, §7.1.3 |
| E2E         | End-to-End                                           | 端到端                 | §5.1         |
| K8s         | Kubernetes                                           | Kubernetes（容器编排） | §8           |
| CRD         | Custom Resource Definition                           | 自定义资源定义         | §8.1         |
| REST        | Representational State Transfer                      | 表述性状态传递         | §2.3         |
| JSON        | JavaScript Object Notation                           | JavaScript 对象表示法  | §3.1         |
| YAML        | YAML Ain't Markup Language                           | YAML 标记语言          | §3.5         |
| PG          | PostgreSQL                                           | PostgreSQL 数据库      | §3.4         |
| CI          | Continuous Integration                               | 持续集成               | §8.2         |
| P0/P1/P2    | Priority 0/1/2                                       | 优先级 0/1/2（告警）   | §6.4         |
| RPS         | Requests Per Second                                  | 每秒请求数             | §2.2         |
| TTL         | Time To Live                                         | 生存时间               | §2.2         |
| CORS        | Cross-Origin Resource Sharing                        | 跨源资源共享           | §7.2         |
| CSP         | Content Security Policy                              | 内容安全策略           | §7.2         |
| HSTS        | HTTP Strict Transport Security                       | HTTP 严格传输安全      | §7.2         |
| P50/P95/P99 | 50th/95th/99th Percentile                            | 第 50/95/99 百分位     | §6.1         |
| ONNX        | Open Neural Network Exchange                         | 开放神经网络交换       | §7.1.3       |
| GPU         | Graphics Processing Unit                             | 图形处理器             | §7.1.3       |
| CPU         | Central Processing Unit                              | 中央处理器             | §8.1         |
| MB          | Megabyte                                             | 兆字节                 | §2.2         |
| GB          | Gigabyte                                             | 吉字节                 | §7.1.3       |
| TS          | Test Suite                                           | 测试套件               | §5.1         |
| DoD         | Definition of Done                                   | 完成的定义             | §5           |
| SLA         | Service Level Agreement                              | 服务等级协议           | §7           |
| SRE         | Site Reliability Engineering                         | 站点可靠性工程         | §6           |

---

> **使用建议**：本文档建议与主架构设计文档配套维护。当新增技术术语时，应按领域归类并同步更新缩写速查表，确保全栈术语一致性。