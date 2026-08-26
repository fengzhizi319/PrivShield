# 调度之眼 · 调度中枢全景测试与治理控制台 (Console App-LZ) — 架构与系统设计文档

> **文档版本**：v1.1.0  
> **编写时间**：2026-08-26  
> **适用模块**：`console/app-lz` (Frontend Web + Go BFF)  
> **联动服务**：`services/service-hub` (:8082/:50052)、`services/datasource-mgr` (:8083/:50053)、`services/audit-log` (:8084/:50054)、`engine` (:8079/:50051)

---

## 1. 系统定位与设计背景

### 1.1 系统定位 (Positioning)
**`console/app-lz`**（简称 **App-LZ**，取意"联中 / 调度之眼 / 流水线全景治理平台"）是专为 **数联数据服务调度中枢 (`services/service-hub`)** 打造的**全链路集成测试、实时动态观测与微服务网格治理控制台**。

### 1.2 为什么需要独立开发 App-LZ？
- **原有控制台 (`console/web` + `console/bff-go`) 的重心**：主要面向底层 **隐私计算原语 (`engine/privacy`)**（如脱敏、差分隐私、K-匿名、查询混淆）与 **动态分类分级漏斗 (`engine/dynclassification`)** 的单点算法验证。
- **调度中枢 (`services/service-hub`) 的核心价值**：在于**企业级跨微服务流水线编排**，它向上承接任务分发，横向调用 `datasource-mgr` 抓取数据、调用 `engine` 动态分类与脱敏，向下联动 `audit-log` 进行不可篡改 SHA-256 / Merkle 树存证，并在底层支持 Phase B PostgreSQL 原子租约调度。
- **App-LZ 的核心使命**：
  1. **直观打通 4 大核心服务**：彻底串联 `service-hub`、`datasource-mgr`、`audit-log` 与 `engine`，实现从数据源抓取 ➔ 分类分级 ➔ 隐私脱敏 ➔ 审计存证的全链路可视化。
  2. **全面覆盖 `service-hub` 的所有测试场景**：提供从单接口测试、自适应路由、数据源切片联动、Phase B 租约并发争抢、高并发 QPS 压测到故障注入/熔断恢复的**一站式图形化测试工作台**。
  3. **实时全景可观测性**：提供 6 阶段流水线动效图谱、任务生命周期甘特图、微服务网格健康矩阵与不可篡改审计验真大屏。

### 1.3 与 `console/bff-go` 的独立关系与代码共享策略

App-LZ 作为**独立项目**与 `console/bff-go` 平行共存，两者定位不同、服务不同、发布节奏独立：

| 维度 | `console/bff-go` (现有) | `console/app-lz/bff-go` (本模块) |
|---|---|---|
| **定位** | 隐私原语与分类分级测试代理 | 调度中枢全链路测试与治理 |
| **上游服务** | 仅 `engine` Agent | 4 大服务 (Hub + Datasource + Audit + Agent) |
| **端口** | HTTP `:8081` / gRPC `:50051` | HTTP `:8085` / gRPC `:50055` |
| **前端** | `console/web` (:5173) | `console/app-lz/web` (:5174) |
| **独有功能** | 文件上传/解析、负载均衡测试、医疗流水线 | 拓扑聚合、6 阶段大屏、租约看板、E2E 测试执行器 |

**代码共享策略**：App-LZ 通过以下三个层次实现与现有代码的复用，避免从零开发：

1. **`pkg/` 共享基础库（直接引用）**：通过 `go.mod` 的 `replace` 指令引用 `pkg/`，复用 `pkg/middleware`（CORS / Auth / RequestID / Recovery / SecurityHeaders / MaxBodySize）、`pkg/metrics`（Prometheus Collector）、`pkg/config`（SetupLogger / GetEnvBool）、`pkg/validation`（输入校验 / ID 生成）、`pkg/agent`（带熔断器的 HTTP Client）、`pkg/tlsutil`（TLS 配置构建）。
2. **`console/bff-go` 代码参考复制**：将 `console/bff-go` 中成熟的模式（Gin 路由注册、静态文件托管 SPA 回退、优雅停机、安全中间件链、gRPC 网关启动）复制到 App-LZ BFF，按 4 上游服务场景进行适配改造。
3. **App-LZ 独有代码**：拓扑聚合器、测试执行引擎（runner）、租约看板数据聚合等为本模块专有实现。

**`go.work` 管理**：App-LZ BFF 作为独立 Go module 注册到根目录 `go.work`：

```text
go.work（仓库根目录）
├── ./pkg
├── ./console/bff-go              # 现有隐私原语 BFF
├── ./console/app-lz/bff-go       # 新增：调度之眼 BFF
├── ./services/service-hub
├── ./services/datasource-mgr
└── ./services/audit-log
```

---

## 2. 总体架构设计

### 2.1 架构拓扑全景 (Architecture Topology)

```mermaid
flowchart TB
    subgraph Frontend ["App-LZ 前端展现层 (console/app-lz/web :5174)"]
        UI_Home["集群拓扑与健康矩阵\n(Topology & Mesh Health)"]
        UI_Pipeline["6阶段流水线实时大屏\n(Pipeline Visualizer)"]
        UI_Tasks["任务生命周期与租约看板\n(Task & Lease Inspector)"]
        UI_Runner["一键全场景自动化测试\n(E2E Suite Runner)"]
        UI_Datasource["数据源资产探查与切片\n(Datasource Explorer)"]
        UI_Audit["不可篡改审计与Merkle验真\n(Audit & Merkle Verifier)"]
        UI_Metrics["实时监控与阶段耗时分析\n(Metrics & Performance)"]
    end

    subgraph BFF ["App-LZ Go BFF 聚合代理层 (console/app-lz/bff-go :8085 / :50055)"]
        BFF_Router["Gin REST API 路由网关"]
        BFF_Aggregator["多服务状态聚合与健康探针管理器"]
        BFF_E2E["自动化测试执行引擎 (Test Runner Engine)"]
        BFF_Clients["统一 gRPC / HTTP 客户端连接池 (带重试/保活/mTLS)"]
    end

    subgraph UpstreamServices ["PrivShield 核心服务集群"]
        subgraph ServiceHub ["1. 调度中枢 (service-hub :8082 / :50052)"]
            SH_API["REST / gRPC 调度接口"]
            SH_Pipe["6-Stage Pipeline 调度引擎\n(Ingest➔Fetch➔Classify➔Desensitize➔Return➔Audit)"]
            SH_Store["TaskStore 存储引擎\n(PostgreSQL Leased / SQLite / Memory)"]
        end

        subgraph DatasourceMgr ["2. 数据源管理 (datasource-mgr :8083 / :50053)"]
            DS_Yibao["医保结算数据源 (ds_yibao)"]
            DS_Kangyang["智慧康养数据源 (ds_kangyang)"]
            DS_Probe["特征探查与切片采样接口"]
        end

        subgraph AuditLog ["3. 审计日志 (audit-log :8084 / :50054)"]
            AL_Store["不可篡改审计存证"]
            AL_Merkle["Merkle Tree / SHA-256 防篡改校验"]
        end

        subgraph AgentEngine ["4. 隐私与分类引擎 (engine :8079 / :50051)"]
            AE_Funnel["三层分类漏斗 (Rule ➔ NER ➔ LLM)"]
            AE_Primitives["隐私原语 (Mask / DP / LDP / Kano / QoL)"]
        end
    end

    subgraph Storage ["数据持久层"]
        PG_DB[("PostgreSQL 16\nPhase B 原子租约库")]
        SQLITE_DB[("SQLite\n本地轻量数据库")]
    end

    %% 连接关系
    Frontend <-->|HTTP / JSON / SSE| BFF
    BFF -->|REST / gRPC mTLS| ServiceHub
    BFF -->|REST / gRPC mTLS| DatasourceMgr
    BFF -->|REST / gRPC mTLS| AuditLog
    BFF -->|REST / gRPC mTLS| AgentEngine

    ServiceHub -->|1. 抓取原始切片| DatasourceMgr
    ServiceHub -->|2. 分类分级与脱敏| AgentEngine
    ServiceHub -->|3. 异步存证写入| AuditLog
    ServiceHub -->|4. 任务租约与持久化| Storage
```

### 2.2 技术栈选型

| 分层 | 技术选型 | 版本/规范 | 选型理由 |
|---|---|---|---|
| **前端框架** | React + TypeScript + Vite | React 18 / TS 5.x / Vite 6.x | 毫秒级 HMR 开发体验、强类型安全契约，与 `console/web` 保持技术同构 |
| **前端样式与图标** | Tailwind CSS + Lucide React | Tailwind v3.4+ / Lucide | 极简现代化 UI、深浅色模式支持、统一的设计系统原子类 |
| **图表与动画** | ECharts (echarts-for-react) + SVG 流向动效 | ECharts 5.x | 流水线拓扑图、甘特图、延时直方图、QPS 仪表盘的高性能渲染 |
| **BFF 后端** | Go + Gin + gRPC-Go | Go 1.25+ / Gin v1.10+ | 极低内存占用、高并发协程调度、原生双协议（HTTP/gRPC）高效聚合 |
| **通信协议** | REST (HTTP/1.1 JSON) + gRPC (HTTP/2 mTLS) | Protobuf v3 | 外部交互简洁通用，内部服务聚合高速可靠 |

### 2.3 gRPC Proto 依赖与 Stub 生成策略

App-LZ BFF 需要通过 gRPC 连接 4 个上游服务，需引用以下 proto 文件并生成 Go stub：

| 上游服务 | Proto 文件 | 生成目标 | 用途 |
|---|---|---|---|
| `service-hub` | `services/service-hub/proto/servicehub.proto` | `bff-go/proto/servicehub/` | 调度任务管理、流水线状态、租约查询 |
| `datasource-mgr` | `services/datasource-mgr/proto/datasourcemgr.proto` | `bff-go/proto/datasourcemgr/` | 数据源元数据查询、切片采样 |
| `audit-log` | `services/audit-log/proto/auditlog.proto` | `bff-go/proto/auditlog/` | 审计日志查询、Merkle 验真 |
| `engine` | `proto/privacy.proto` | `bff-go/proto/privacy/` | 分类分级、隐私原语调用（可选，HTTP 已覆盖主要场景） |

**Stub 生成命令**（在 `console/app-lz/bff-go/` 目录下执行）：

```bash
# 生成 service-hub proto stub
python -m grpc_tools.protoc \
  -I ../../../services/service-hub/proto \
  --go_out=proto/servicehub --go-grpc_out=proto/servicehub \
  ../../../services/service-hub/proto/servicehub.proto

# 生成 datasource-mgr proto stub
python -m grpc_tools.protoc \
  -I ../../../services/datasource-mgr/proto \
  --go_out=proto/datasourcemgr --go-grpc_out=proto/datasourcemgr \
  ../../../services/datasource-mgr/proto/datasourcemgr.proto

# 生成 audit-log proto stub
python -m grpc_tools.protoc \
  -I ../../../services/audit-log/proto \
  --go_out=proto/auditlog --go-grpc_out=proto/auditlog \
  ../../../services/audit-log/proto/auditlog.proto

# 生成 engine proto stub（可选）
python -m grpc_tools.protoc \
  -I ../../../proto \
  --go_out=proto/privacy --go-grpc_out=proto/privacy \
  ../../../proto/privacy.proto
```

> **注意**：proto stub 应在上游 proto 文件变更后重新生成，并在 Makefile 中提供 `make proto-gen` 目标一键更新。

---

## 3. 核心功能模块设计

App-LZ 共划分为 **7 大核心功能工作台**，全方位覆盖 `service-hub` 的所有测试场景与观测需求：

### 3.1 模块一：四微服务固定网格拓扑与双协议健康矩阵 (Fixed Mesh Topology & Dual-Protocol Health Matrix)
- **业务目标**：实时感知 4 大微服务集群（Hub、Agent、Datasource、Audit）的物理连接状态与健康度，排查链路单点故障，并提供 REST 与 gRPC 通信协议的无缝切换观测。
- **固定四微服务显示顺序 (Fixed Layout Ordering)**：
  为了提供统一、直观的拓扑视图，前端面板与 BFF 聚合层严格固定四微服务的物理展示位置：
  1. **`#1` 调度中枢 (Service Hub)**: `service-hub` (:8082 / :50052) — 核心流程调度中枢。
  2. **`#2` 隐私与分类引擎 (PrivShield Agent)**: `engine` (:8079 / :50051) — 3层分类漏斗与4大隐私原语引擎。
  3. **`#3` 数据源管理 (Datasource Mgr)**: `datasource-mgr` (:8083 / :50053) — 医保/康养数据源资产探查与切片抽取。
  4. **`#4` 脱敏审计日志 (Audit Log)**: `audit-log` (:8084 / :50054) — SHA-256 审计存证与 Merkle 链验真。
- **REST / gRPC 双协议切换机制 (Protocol Channel Switcher)**：
  - **`⚡ REST (HTTP/1.1 JSON)`**：展示各服务 HTTP 访问端点（如 `http://127.0.0.1:8082`）、REST 往返延时与健康度，主要用于 Web 控制台与常规业务对接。
  - **`🛡️ gRPC (HTTP/2.0 mTLS / Protobuf)`**：展示各服务 gRPC 监听端口（如 `127.0.0.1:50052`）、gRPC 往返延时与连通性，主要用于微服务间内部高性能低延时通信与 mTLS 双向鉴权。
- **聚合策略**：BFF 并发执行 HTTP 探针与 TCP/gRPC 端口探测，分别记录 `rest_rtt_ms` 与 `grpc_rtt_ms`，支持 `/api/lz/topology?protocol=rest|grpc` 动态过滤。单个服务不可达时**不阻塞**整体响应，该节点标记为 `unreachable`。

```text
┌────────────────────────────────────────────────────────────────────────┐
│  🌐 PrivShield 4-Service Live Mesh Health Matrix (固定四节点与双协议)   │
├───────────────────┬───────────────────┬──────────────────┬─────────────┤
│ #1 调度中枢 (Hub)  │ #2 隐私引擎(Agent)│ #3 数据源 (DS)   │ #4 审计日志 │
│ :8082 / :50052    │ :8079 / :50051    │ :8083 / :50053   │ :8084/:50054│
│ ● 状态: Ready     │ ● 状态: Ready     │ ● 状态: Ready    │ ● 状态: Ready│
│ ⏱ REST: 1.8ms     │ ⏱ REST: 3.2ms     │ ⏱ REST: 2.1ms    │ ⏱ REST: 1.5ms│
│ ⏱ gRPC: 1.2ms     │ ⏱ gRPC: 2.4ms     │ ⏱ gRPC: 1.5ms    │ ⏱ gRPC: 1.1ms│
│ 📦 存储: Postgres │ 🧠 漏斗: L1-L3    │ 📊 医保/康养: 1.8k│ 🔒 Merkle: 有效 │
└───────────────────┴───────────────────┴──────────────────┴─────────────┘
```

---

### 3.2 模块二：6 阶段流水线动态调度大屏 (6-Stage Pipeline Visualizer)
- **业务目标**：可视化展示 `service-hub` 核心 6 阶段流水线在处理数据时的流转全貌。
- **6 大阶段全流程**：
  1. **`Ingest` (任务接收与校验)**：校验请求体格式、分配全局唯一 `task_id`、写入存储并置为 `pending`。
  2. **`Fetch` (数据源拉取)**：联动 `datasource-mgr` 根据 `datasource_id` 提取原始数据切片。
  3. **`Classify` (动态分类分级)**：调用 `engine` 规则引擎/NER/LLM，评估字段与记录的敏感等级 (L1~L5)。
  4. **`Desensitize` (自适应隐私脱敏)**：根据分类结果自动匹配并执行脱敏原语（掩码/差分隐私/K-匿名/混淆）。
  5. **`Return` (结果装配与返回)**：装配安全治理后的合规数据包。
  6. **`Audit` (不可篡改存证)**：向 `audit-log` 异步提交 SHA-256 存证记录与任务元数据。
- **交互特性**：
  - 各阶段状态灯（空闲 `idle` / 处理中 `processing` / 失败 `error`）与活跃任务计数卡片。
  - **数据穿透比对面板（Payload Inspector）**：左侧展示原始输入数据（如明文身份证、病历），右侧展示脱敏后数据，中间高亮展示分类标签（如 `[L3-PERSONAL_BASIC] ➔ 掩码脱敏`）。
- **上游映射**：流水线状态数据来源于 `service-hub` 的 `GET /api/hub/pipeline` 接口，BFF 额外聚合 `service-hub` 的 `GET /api/hub/status` 获取全局队列深度。

---

### 3.3 模块三：任务全生命周期与 Phase B 租约看板 (Task Lifecycle & Lease Inspector)
- **业务目标**：直观管理和查看所有调度任务的运行轨迹，并对 Phase B PostgreSQL 原子租约调度进行深度观测。
- **功能特性**：
  1. **任务多维检索与过滤**：支持按状态（`pending` / `running` / `completed` / `failed`）、操作类型（`mask` / `k_anon` / `dp` / `classify`）、数据源（`ds_yibao` / `ds_kangyang`）与优先级即时搜索。
  2. **任务详情与时间线追溯**：展示任务从创建、认领、执行到存证的耗时阶段分析（精确到毫秒）。
  3. **Phase B PostgreSQL 原子租约监控（专有视图）**：
     - **租约持有者 (`lease_owner`)**：显示认领该任务的 Hub 节点 ID。
     - **租约倒计时 (`lease_expires_at`)**：动态计算租约过期剩余时间。
     - **原子争抢状态**：展示多副本环境下基于 `FOR UPDATE SKIP LOCKED` 的任务认领状态。
     - **孤儿任务回收监控**：展示调度器是否自动回收并重新分配超时任务。
- **存储后端自适应**：BFF 在启动时探测 `service-hub` 的存储后端类型（通过 `GET /api/hub/status` 返回的 `store_type` 字段）。当后端为 `sqlite` 或 `memory` 时，租约看板自动切换为**简化模式**——隐藏 PostgreSQL 专有指标（行锁状态、`SKIP LOCKED` 争抢），仅展示任务队列深度与孤儿任务回收计数，并在 UI 上以提示条告知用户"租约调度需 PostgreSQL 后端"。

---

### 3.4 模块四：一键全场景自动化测试执行器 (One-Click E2E Test Suite Runner)
- **业务目标**：将 `services/service-hub` 的所有单元测试、集成测试、端到端测试与压力测试沉淀为前端可随时触发的**图形化测试矩阵**。
- **预设测试套件（Test Suites）**：

| 用例编号 | 测试场景名称 | 测试目的与链路 | 验证断言 (Assertions) | 前置条件 |
|:---|:---|:---|:---|:---|
| **TS-01** | **基础脱敏任务分发** | 测试 `POST /api/hub/dispatch` 手动分发 Mask 任务 | 任务返回 `202 Accepted`，流水线状态变为 `completed`，敏感字段完成打码 | 4 服务全部在线 |
| **TS-02** | **自适应分类与自动策略路由** | 测试 `POST /api/hub/classify` 智能路由 | Agent 返回准确等级 (如 `L3`)，Hub 自动选取 `mask` 原语并成功执行 | 4 服务全部在线 |
| **TS-03** | **数据源切片联动调度** | 测试 `POST /api/hub/pipeline/trigger-datasource` | 联动 `datasource-mgr` 提取 10 条医保切片，批量执行隐私治理并返回统计结果 | 4 服务全部在线 |
| **TS-04** | **全链路审计存证与 Merkle 验真** | 验证流水线处理完数据后，存证写入 `audit-log` | `audit-log` 存在该 `task_id` 的 SHA-256 存证记录，且 Merkle 链校验通过 | 4 服务全部在线 |
| **TS-05** | **Agent 宕机熔断与降级测试** | 模拟上游 Agent 响应超时或异常 | Hub 熔断器状态变为 `Open`，快速失败避免雪崩；恢复后探针探测进入 `Half-Open` ➔ `Closed` | 需配置 Agent 故障注入（见下方说明） |
| **TS-06** | **高并发吞吐量与延迟压测** | 模拟 50~200 并发突发请求调度 | 统计 QPS、成功率，输出 **P50 / P90 / P95 / P99 / Avg / Min / Max** 延迟分布直方图 | 4 服务全部在线，建议关闭 LLM 层避免瓶颈 |
| **TS-07** | **Phase B 租约多副本并发争抢** | 模拟 5 个并发 Hub Worker 争抢 50 个待处理任务 | 验证 `FOR UPDATE SKIP LOCKED` 严格保证**零任务重复执行**、**零死锁** | **需 PostgreSQL 后端** + 多 Hub 副本部署 |

- **测试执行引擎架构**：
  - **声明式测试定义**：每个测试用例以 Go 结构体定义（`runner/cases/`），包含请求模板、断言规则、轮询策略与超时配置。
  - **断言引擎**（`runner/assert.go`）：支持 HTTP 状态码断言、JSON Path 值匹配、字段存在性校验、轮询等待（poll-until）等断言类型。
  - **报告生成器**（`runner/report.go`）：测试完成后生成结构化报告（JSON / Markdown），包含每个用例的耗时、断言详情（预期值 vs 实际值）、通过/失败统计。
  - **SSE 日志流**：测试执行过程中通过 SSE (`GET /api/lz/suites/stream/:run_id`) 实时推送日志到前端。

- **TS-05 故障注入说明**：通过环境变量 `PRIVACY_TEST_FAULT_INJECT=1` 启动 BFF 的故障注入模式，此时 Agent 客户端可被配置为返回超时或 500 错误。测试执行器在 TS-05 开始前通过 BFF 内部 API 激活故障注入，测试完成后自动恢复。

- **执行面板特性**：
  - 支持"一键全量运行 (Run All)"与"单项调试运行 (Run Selected)"。
  - 实时输出测试日志流与进度条（Pass / Fail / Skip）。
  - 测试完成后可直接一键导出 JSON / Markdown 测试报告。

---

### 3.5 模块五：模拟数据源资产探查器 (Datasource Asset Explorer)
- **业务目标**：直连 `services/datasource-mgr`，实时探索底层数据源资产与特征画像。
- **功能特性**：
  1. **数据源卡片展示**：展示 `ds_yibao` (城镇职工基本医疗保险结算数据源) 与 `ds_kangyang` (智慧养老健康监护数据源) 的总记录数、字段列表、主键定义。
  2. **在线切片采样 (Slice Sampler)**：支持自定义选择拉取 1~100 条切片样本，预览原始表格数据。
  3. **一键派发至流水线**：选定切片后直接点击"派发至调度中枢"，无缝跳转至流水线跟踪其脱敏与存证全过程。
- **上游映射**：BFF 直通转发至 `datasource-mgr` 的 `GET /api/datasources` 和 `GET /api/datasources/:id/slice`。

---

### 3.6 模块六：不可篡改审计存证与哈希链验真 (Audit Log & Merkle Verifier)
- **业务目标**：直连 `services/audit-log`，校验流水线产生的脱敏存证记录与 Merkle 树防篡改完整性。
- **功能特性**：
  1. **审计存证日志流**：按时间倒序展示由 `service-hub` 触发的脱敏审计流水（含任务ID、数据指纹、操作人、加密算法）。
  2. **Merkle 链完整性一键验真**：前端调用 `POST /api/lz/audit/verify` 接口，展示 Merkle Tree 校验结论、根哈希（Root Hash）与防篡改签名。
  3. **数据一致性比对器**：输入原始任务数据和存证哈希，验证数据在流转过程中是否被篡改。
- **上游映射**：BFF 直通转发至 `audit-log` 的 `GET /api/audit/logs` 和 `POST /api/audit/snapshots/verify`。

---

### 3.7 模块七：性能监控与耗时直方图 (Metrics & Performance Analyzer)
- **业务目标**：直观展示调度中枢的 Prometheus 指标与流水线各阶段性能瓶颈。
- **功能特性**：
  1. **QPS 与吞吐量实时仪表盘**：按秒级/分钟级统计任务接收速率与处理速率。
  2. **6 阶段耗时占比瀑布图**：精准量化 `Ingest`、`Fetch`、`Classify`、`Desensitize`、`Return`、`Audit` 的平均耗时分布（例如识别出瓶颈是在 Agent 分类还是在数据源提取）。
  3. **系统资源与重试率监控**：监控失败重试计数、孤立任务回收次数、错误率趋势。
- **数据来源**：BFF 通过 HTTP 调用各上游服务的 `GET /metrics` 端点获取 Prometheus 原始指标，聚合后以 JSON 格式返回前端。前端使用 ECharts 渲染图表。

---

## 4. 前后端接口契约规范 (BFF API Specification)

> 完整的接口定义、请求/响应示例与错误码详见 [API 接口与数据契约规范 (`api.md`)](api.md)。

`console/app-lz/bff-go` 提供统一的聚合 REST API，监听端口 **`:8085`**（gRPC 端口 **`:50055`**）：

### 4.1 集群拓扑与健康聚合
- `GET /api/lz/topology`：**[聚合]** 并发探测 4 大服务健康状态，返回统一拓扑矩阵。
- `POST /api/lz/probe/all`：**[聚合]** 并发执行全集群深度自检探针（`/readyz`）。

### 4.2 调度流水线交互
- `GET /api/lz/pipeline/status`：**[聚合]** 合并 `service-hub` 的 `/api/hub/pipeline` + `/api/hub/status`。
- `POST /api/lz/pipeline/dispatch`：**[转发]** → `service-hub` `POST /api/hub/dispatch`。
- `POST /api/lz/pipeline/classify-dispatch`：**[转发]** → `service-hub` `POST /api/hub/classify`。
- `POST /api/lz/pipeline/trigger-datasource`：**[转发]** → `service-hub` `POST /api/hub/pipeline/trigger-datasource`。

### 4.3 任务生命周期与租约
- `GET /api/lz/tasks`：**[转发]** → `service-hub` `GET /api/hub/tasks`（支持 `status`, `operation`, `limit`, `offset` 过滤）。
- `GET /api/lz/tasks/:id`：**[转发]** → `service-hub` `GET /api/hub/tasks/:id`。
- `GET /api/lz/tasks/leases`：**[聚合]** 查询 `service-hub` 存储后端类型与租约状态（SQLite/内存模式返回简化信息）。

### 4.4 自动化测试套件
- `GET /api/lz/suites`：获取所有内置 E2E 测试用例列表与历史运行记录。
- `POST /api/lz/suites/run`：执行指定测试用例或全量测试套件（支持并发压测参数）。
- `GET /api/lz/suites/stream/:run_id`：通过 SSE (Server-Sent Events) 流式推送测试执行日志。

### 4.5 数据源与审计直通
- `GET /api/lz/datasources`：**[转发]** → `datasource-mgr` `GET /api/datasources`。
- `GET /api/lz/datasources/:id/slice`：**[转发]** → `datasource-mgr` `GET /api/datasources/:id/slice`。
- `GET /api/lz/audit/logs`：**[转发]** → `audit-log` `GET /api/audit/logs`。
- `POST /api/lz/audit/verify`：**[转发]** → `audit-log` `POST /api/audit/snapshots/verify`。

> **[聚合]** = BFF 并发调用多个上游并合并结果；**[转发]** = BFF 透传请求到单一上游，附加认证头与 RequestID。

---

## 5. 安全设计 (Security Design)

### 5.1 入站安全（前端 ➔ BFF）

| 安全层 | 实现方式 | 配置项 |
|---|---|---|
| **API Key 认证** | 复用 `pkg/middleware.Auth`，前端请求须携带 `Authorization: Bearer <key>` | `LZ_CONSOLE_API_KEY` |
| **速率限制** | 复用 `pkg/middleware` 令牌桶限流 | `LZ_CONSOLE_RATE_LIMIT` (默认 100 req/s) |
| **请求体限制** | 复用 `pkg/middleware.MaxBodySize`，限制 32 MiB | 硬编码 |
| **安全响应头** | 复用 `pkg/middleware.SecurityHeaders`（CSP / HSTS / X-Content-Type-Options 等） | 自动启用 |
| **TLS (可选)** | 复用 `pkg/tlsutil` 构建服务端 TLS 配置，支持 mTLS 客户端证书验证 | `LZ_CONSOLE_TLS_ENABLED` 等 |

### 5.2 出站安全（BFF ➔ 上游服务）

| 上游服务 | 认证方式 | 配置项 |
|---|---|---|
| `service-hub` | Bearer Token (`Authorization: Bearer <key>`) | `LZ_HUB_API_KEY` |
| `datasource-mgr` | Bearer Token | `LZ_DATASOURCE_API_KEY` |
| `audit-log` | Bearer Token | `LZ_AUDIT_API_KEY` |
| `engine` Agent | Bearer Token + 可选 mTLS | `LZ_AGENT_API_KEY` / `LZ_AGENT_TLS_*` |

### 5.3 SSE 流认证

SSE 端点 (`GET /api/lz/suites/stream/:run_id`) 通过 URL 查询参数 `?token=<run_token>` 进行认证，`run_token` 在 `POST /api/lz/suites/run` 响应中返回，一次性有效。

### 5.4 中间件链装配顺序

```
RequestID() → StructuredLogger() → Recovery() → SecurityHeaders() → MaxBodySize(32MiB) → CORS() → Auth()
```

与 `console/bff-go` 和 3 个 Go 微服务保持完全一致的中间件装配模式。

---

## 6. 错误处理与降级策略 (Error Handling & Degradation)

### 6.1 统一错误响应格式

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

### 6.2 错误码定义

| 错误码 | HTTP 状态码 | 触发条件 |
|---|---|---|
| `UPSTREAM_UNAVAILABLE` | 502 | 上游服务连接失败或超时 |
| `UPSTREAM_TIMEOUT` | 504 | 上游服务响应超时（默认 30s） |
| `UPSTREAM_ERROR` | 502 | 上游返回 5xx |
| `INVALID_REQUEST` | 400 | 请求参数校验失败 |
| `UNAUTHORIZED` | 401 | API Key 缺失或无效 |
| `RATE_LIMITED` | 429 | 请求频率超过限制 |
| `PARTIAL_DEGRADED` | 200 | 聚合查询中部分服务不可达（拓扑/探针场景） |

### 6.3 降级策略

| 场景 | 降级行为 |
|---|---|
| 拓扑查询中单个服务不可达 | 返回 200 + `PARTIAL_DEGRADED`，不可达节点标记 `unreachable`，其余正常返回 |
| 流水线状态查询时 Agent 不可达 | `agent_ok: false`，其余阶段数据正常返回 |
| 测试执行中上游不可用 | 该用例标记为 `FAIL`，附带错误详情，继续执行后续用例 |
| SSE 流中断 | 前端自动重连（指数退避，最大 3 次），BFF 保留最近 100 条日志缓冲 |

---

## 7. 前端视觉与交互规范 (UI/UX Design)

App-LZ 遵循与 `console/web` 高度一致的企业级现代设计语言：

1. **色彩系统**：
   - 主色调（Primary）：科技靛蓝（Indigo-600 `#4F46E5`）与 极光橙（Orange-500 `#F97316`）。
   - 状态色彩：就绪绿（Emerald-500）、警告黄（Amber-500）、危险红（Rose-500）、待机蓝（Sky-500）。
   - 背景色：浅灰白（`bg-gray-50`）与 纯白卡片（`bg-white`），边框采用柔和细线（`border-gray-200`）。
2. **布局结构**：
   - **左侧全局侧边栏 (Sidebar)**：包含应用标题、4 服务在线状态指示灯、7 大工作台导航菜单、中英文切换器。
   - **顶部操作栏 (Header)**：当前环境状态标签（开发/生产）、一键集群刷新、快速测试入口。
   - **右侧主工作区 (Main Workspace)**：采用响应式网格与平滑卡片，内部包含富文本代码高亮器与数据 Diff 比对器。
3. **交互与可访问性**：
   - 按钮具备加载态旋转动效与禁用样式。
   - 分位数指标（P50/P90/P95/P99）配备悬停气泡解释。
   - 完整支持中英文无缝切换（`zh-CN` / `en-US`）。

---

## 8. 工程目录结构规划 (Repository Layout)

```text
console/app-lz/
├── docs/                                  # App-LZ 文档目录
│   ├── design.md                          # 本设计文档（系统架构与全景规划）
│   ├── api.md                             # REST & gRPC API 契约文档
│   ├── prd.md                             # 产品需求与测试用例文档
│   └── testing.md                         # 测试与运维验证手册
├── bff-go/                                # App-LZ Go 聚合代理后端
│   ├── cmd/server/main.go                 # BFF 启动入口 (:8085)
│   ├── internal/
│   │   ├── config/config.go               # 环境变量与上游服务地址配置
│   │   ├── handlers/                      # Gin HTTP 路由处理器（按功能域分组）
│   │   │   ├── topology/
│   │   │   │   └── handler.go             # 集群拓扑与健康探测（聚合 4 服务）
│   │   │   ├── pipeline/
│   │   │   │   └── handler.go             # 流水线与任务调度（转发 + 聚合）
│   │   │   ├── suites/
│   │   │   │   └── handler.go             # E2E 自动化测试执行入口 + SSE 流
│   │   │   ├── datasource/
│   │   │   │   └── handler.go             # 数据源探查（直通转发）
│   │   │   └── audit/
│   │   │       └── handler.go             # 审计存证与验真（直通转发）
│   │   ├── clients/                       # 4 大上游服务客户端池（REST + gRPC）
│   │   │   ├── hub_client.go              # service-hub 客户端
│   │   │   ├── datasource_client.go       # datasource-mgr 客户端
│   │   │   ├── audit_client.go            # audit-log 客户端
│   │   │   └── agent_client.go            # engine Agent 客户端
│   │   ├── models/models.go               # BFF 层数据模型（聚合响应、前端契约）
│   │   └── runner/                        # E2E 测试执行引擎
│   │       ├── engine.go                  # 执行引擎核心（调度、并发控制、生命周期）
│   │       ├── cases/                     # TS-01~TS-07 测试用例定义
│   │       │   ├── ts01_basic_dispatch.go
│   │       │   ├── ts02_classify_dispatch.go
│   │       │   ├── ts03_datasource_pipeline.go
│   │       │   ├── ts04_audit_verify.go
│   │       │   ├── ts05_circuit_breaker.go
│   │       │   ├── ts06_high_concurrency.go
│   │       │   └── ts07_lease_contention.go
│   │       ├── assert.go                  # 断言引擎（状态码/JSONPath/poll-until）
│   │       ├── report.go                  # 报告生成器（JSON / Markdown）
│   │       └── stream.go                  # SSE 日志流管理器
│   ├── proto/                             # gRPC proto 生成代码
│   │   ├── servicehub/                    # service-hub proto stub
│   │   ├── datasourcemgr/                 # datasource-mgr proto stub
│   │   └── auditlog/                      # audit-log proto stub
│   ├── go.mod                             # Go module（引用 pkg/ + proto 依赖）
│   └── Makefile                           # BFF 构建、测试、proto 生成
├── web/                                   # App-LZ React 前端项目
│   ├── src/
│   │   ├── api/client.ts                  # BFF API 请求客户端（Axios/fetch 封装）
│   │   ├── features/                      # 按功能域组织（非平铺 components/）
│   │   │   ├── topology/
│   │   │   │   └── TopologyPanel.tsx      # 模块 1: 集群拓扑与健康矩阵
│   │   │   ├── pipeline/
│   │   │   │   └── PipelineVisualizer.tsx # 模块 2: 6 阶段流水线大屏
│   │   │   ├── tasks/
│   │   │   │   └── TaskLifecyclePanel.tsx # 模块 3: 任务管理与租约看板
│   │   │   ├── suites/
│   │   │   │   └── TestRunnerPanel.tsx    # 模块 4: 一键自动化测试执行器
│   │   │   ├── datasource/
│   │   │   │   └── DatasourceExplorer.tsx # 模块 5: 数据源资产探查器
│   │   │   ├── audit/
│   │   │   │   └── AuditVerifierPanel.tsx # 模块 6: 不可篡改审计验真
│   │   │   └── metrics/
│   │   │       └── MetricsPanel.tsx       # 模块 7: 性能监控与耗时直方图
│   │   ├── shared/                        # 跨功能域共享组件
│   │   │   ├── Sidebar.tsx                # 左侧导航栏
│   │   │   ├── Header.tsx                 # 顶部操作栏
│   │   │   └── common/                    # 公共 UI 组件 (Card, Badge, Tooltip)
│   │   ├── hooks/                         # 自定义 React Hooks
│   │   │   ├── useTopology.ts             # 拓扑数据轮询与缓存
│   │   │   ├── useSSE.ts                  # SSE 流连接管理
│   │   │   └── useHealthProbe.ts          # 健康探针定时探测
│   │   ├── i18n/index.tsx                 # 中英文国际化字典
│   │   ├── types/api.ts                   # TypeScript 类型定义（与 BFF API 契约对齐）
│   │   ├── App.tsx                        # 顶层应用路由与布局
│   │   └── main.tsx                       # 入口渲染文件
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   └── tsconfig.json
├── scripts/                               # 自动化启停与验证脚本
│   ├── dev-app-lz.sh                      # 开发模式一键拉起 (BFF + Vite HMR)
│   ├── prod-app-lz.sh                     # 生产模式一键拉起 (BFF + 静态托管)
│   ├── stop-app-lz.sh                     # 一键停止 App-LZ
│   └── run-e2e-suite.sh                   # 命令行静默执行 E2E 测试套件
├── Makefile                               # 统一构建与测试 Makefile
└── README.md                              # 项目使用指南与快速入门
```

---

## 9. 脚本集成与运维协调 (Script Integration)

App-LZ 的脚本与现有脚本体系**并行共存、互不干扰**：

| 场景 | 现有脚本 | App-LZ 脚本 | 说明 |
|---|---|---|---|
| 开发模式启动 | `scripts/dev/dev-bff-agent.sh` | `scripts/dev/dev-app-lz.sh` | 独立启动，不替代现有脚本 |
| 全服务 E2E | `scripts/dev/e2e-start-all-services.sh` | — | 现有脚本已拉起 3 个 Go 服务 + Agent，App-LZ 直接连接 |
| 停止开发服务 | `scripts/dev/dev-stop.sh` | `scripts/dev/stop-app-lz.sh` | 独立停止 |
| 集成测试 | `scripts/dev/integration-test-new-modules.sh` | — | App-LZ 的 E2E 测试执行器替代 curl 脚本 |

**开发模式启动流程** (`dev-app-lz.sh`)：

```bash
#!/bin/bash
# 1. 检查 4 大上游服务是否运行（提示用户先执行 e2e-start-all-services.sh）
# 2. 编译并启动 bff-go (:8085)
# 3. 启动 Vite dev server (:5174)
# 4. 等待 BFF 健康检查通过
# 5. 输出访问地址
```

---

## 10. 实施路线图与交付计划 (Implementation Roadmap)

| 阶段 (Phase) | 核心任务 | 交付物 | 预估工时 |
|---|---|---|---|
| **Phase 1: 架构与规范** | 编写系统设计文档、API 规范文档与 PRD 需求列表 | `console/app-lz/docs/` 规范文档集 | 2 天 |
| **Phase 2: BFF 基础骨架** | 从 `console/bff-go` 复制成熟模式（Gin 路由 / 中间件链 / 优雅停机 / 静态托管 / TLS），适配 4 上游服务客户端，注册到 `go.work` | `bff-go/` 骨架代码、配置、单元测试 | 3 天 |
| **Phase 3: BFF 聚合与测试引擎** | 实现拓扑聚合器、流水线状态聚合、测试执行引擎（runner + cases + assert + report + SSE 流） | `handlers/` + `runner/` 代码与测试 | 5 天 |
| **Phase 4: Web 前端工作台开发** | 基于 React 18 + Vite + Tailwind + ECharts 实现 7 大核心面板（按 features/ 组织） | `web/` 组件库、页面与 Vitest 单元测试 | 7 天 |
| **Phase 5: 全链路联调与自动化验证** | 编写全套启停脚本，打通真实 4 服务集群进行 E2E 回归测试，修复联调问题 | 自动化运维脚本、E2E 测试全部通过 | 3 天 |

**关键里程碑**：
- **M1 (Phase 2 完成)**：BFF 可启动，`/api/health` 返回 200，4 上游客户端连通测试通过。
- **M2 (Phase 3 完成)**：所有 BFF API 端点可用，TS-01 测试用例可在命令行执行通过。
- **M3 (Phase 4 完成)**：7 大前端面板渲染正常，与 BFF API 联调通过。
- **M4 (Phase 5 完成)**：TS-01~TS-07 全量通过，启停脚本一键可用。

---

## 11. 核心代码实现与关键接口剖析 (Core Code Implementations & Interfaces)

### 11.1 Go BFF 核心模块代码设计

#### 1. 微服务客户端池与双协议探针 (`internal/clients/clients.go`)
- **`ProbeNode` 函数实现**：
  同时执行 REST HTTP 端点（`/api/health`）与 gRPC 端口（`net.DialTimeout`）的双向健康探测，精准计算微秒级 RTT 延迟并换算为毫秒：
  ```go
  func (c *ClientPool) ProbeNode(ctx context.Context, id, name, httpURL, grpcAddr, protocol string) models.ServiceNode {
      // 1. 探测 REST 端点 (/api/health)
      startREST := time.Now()
      resp, errREST := c.httpClient.Do(req)
      node.RESTRTTMs = float64(time.Since(startREST).Microseconds()) / 1000.0
      
      // 2. 探测 gRPC 端口 (TCP 握手探活)
      startGRPC := time.Now()
      conn, errGRPC := net.DialTimeout("tcp", grpcAddr, 800*time.Millisecond)
      node.GRPCRTTMs = float64(time.Since(startGRPC).Microseconds()) / 1000.0
      
      // 3. 根据激活协议设定主视图状态与延迟
      if protocol == "grpc" {
          node.Status = node.GRPCStatus
          node.RTTMs = node.GRPCRTTMs
      } else {
          node.Status = node.RESTStatus
          node.RTTMs = node.RESTRTTMs
      }
      return node
  }
  ```
- **`GetTopology` 严格索引保序机制**：
  采用固定下标切片分配 `nodes[idx] = c.ProbeNode(...)`，杜绝并发 Goroutine 异步完成时因 `append` 顺序随机而导致的节点颠倒，确保拓扑矩阵始终以 `1.调度中枢 ➔ 2.隐私引擎 ➔ 3.数据源管理 ➔ 4.脱敏审计日志` 的顺序返回。

#### 2. E2E 自动化测试执行引擎 (`internal/runner/runner.go`)
- 内置 TS-01 ~ TS-07 自动化测试套件执行器。
- 支持并发 Worker 池（`concurrency`）、高并发压测（`benchmark_requests`）、精确分位数统计（`calculatePercentiles` 计算 P50/P90/P95/P99），以及多维度断言判定（Assertion Engine）。
- 支持一键导出标准 Markdown 测试验收报告。

#### 3. Gin HTTP 网关路由与静态托管 (`internal/handlers/handlers.go`)
- **路由矩阵**：
  - `/api/health`: BFF 自身健康探针
  - `/api/lz/topology`: 四微服务拓扑与双协议健康矩阵查询
  - `/api/lz/probe/all`: 强制全集群主动并发重探测
  - `/api/lz/pipeline/status`: 6 阶段流水线与队列深度状态
  - `/api/lz/dispatch`: 手动任务分发
  - `/api/lz/dispatch/classify`: 三层智能分类分级联动分发
  - `/api/lz/tasks` & `/api/lz/leases`: 任务全生命周期与 Phase B 租约
  - `/api/lz/suites` & `/api/lz/suites/run`: TS-01~TS-07 测试用例运行
  - `/api/lz/datasources` & `/api/lz/datasources/:id/slice`: 数据源资产与采样
  - `/api/lz/audit/logs` & `/api/lz/audit/verify`: 审计日志流与 Merkle 验真
  - `/api/lz/metrics`: Prometheus 监控指标与阶段耗时

---

### 11.2 前端 React 7 大工作台组件架构

| 组件文件 | 核心技术点 | 业务职责 |
|---|---|---|
| [`TopologyPanel.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/TopologyPanel.tsx) | `FIXED_ORDER` 排序锁、REST/gRPC 工具栏、实时 RTT 徽标 | 展示四微服务固定网格拓扑、通信协议切换与探针明细 |
| [`PipelineVisualizer.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/PipelineVisualizer.tsx) | 6 阶段状态机动效、医保/康养预设、双栏 JSON Diff | 实时渲染 Ingest➔Fetch➔Classify➔Desensitize➔Return➔Audit 流转 |
| [`TaskLifecyclePanel.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/TaskLifecyclePanel.tsx) | 任务多维过滤、Phase B 租约表、TTL 倒计时 | 观测任务执行阶段、Worker 分布与 `FOR UPDATE SKIP LOCKED` |
| [`TestRunnerPanel.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/TestRunnerPanel.tsx) | 多用例勾选、并发压测滑块、暗黑终端流、MD 导出 | TS-01~TS-07 一键执行、实时断言判定与测试报告生成 |
| [`DatasourceExplorer.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/DatasourceExplorer.tsx) | 动态 Schema 解析、切片采样分页、一键流水线联动 | 医保与康养数据源探查，实时提取切片并直接打通脱敏流水线 |
| [`AuditVerifierPanel.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/AuditVerifierPanel.tsx) | SHA-256 存证流、Merkle 根哈希展示、数字签名校验 | 脱敏存证审计，在线执行 Merkle Tree 防篡改链式验真 |
| [`MetricsPanel.tsx`](file:///home/charles/code/PrivShield/console/app-lz/web/src/components/MetricsPanel.tsx) | 6 阶段耗时瀑布图、P50/P90/P95/P99 统计释义卡片 | 实时 QPS 吞吐分析与 Prometheus 指标流监控 |
