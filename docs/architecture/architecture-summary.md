# PrivShield 架构设计与工程实践总结 (Architecture & Engineering Summary)

> **版本**：v16.0.0  
> **适用范围**：`PrivShield` 核心算力引擎（`engine`）、企业级中台微服务群（`service-hub` / `datasource-mgr` / `audit-log`）、控制台与双 BFF 体系（`bff-go` / `app-lz` / `web`）及全局云原生基础设施。  
> **关联文档**：[unified_design_specifications.md](unified_design_specifications.md)（全栈统一设计规范）、[new_api_design.md](new_api_design.md)（新增数据接口扩展 SOP）、[architecture-design.md](architecture-design.md)（详细架构设计）、[production_optimization_design.md](production_optimization_design.md)（生产级优化设计）。

---

## 目录

- [一、项目定位与系统全景](#一项目定位与系统全景)
- [二、核心设计哲学与标准实践](#二核心设计哲学与标准实践)
  - [2.1 分层 Monorepo 架构](#21-分层-monorepo-架构)
  - [2.2 双栈同源协议支持](#22-双栈同源协议支持)
  - [2.3 纵深安全防御体系](#23-纵深安全防御体系)
  - [2.4 全链路可观测性三支柱](#24-全链路可观测性三支柱)
  - [2.5 分布式全局隐私预算中枢](#25-分布式全局隐私预算中枢)
  - [2.6 企业级中台微服务群实践](#26-企业级中台微服务群实践)
- [三、核心高光工程设计](#三核心高光工程设计)
  - [3.1 三层递进式动态分类分级漏斗 (3-Layer Funnel)](#31-三层递进式动态分类分级漏斗-3-layer-funnel)
  - [3.2 智能动态负载均衡 (P2C + Client-Side LB)](#32-智能动态负载均衡-p2c--client-side-lb)
  - [3.3 9 层统一中间件栈与纵深防 DDoS 体系](#33-9-层统一中间件栈与纵深防-ddos-体系)
- [四、工程注意事项与避坑指南](#四工程注意事项与避坑指南)
- [五、可复用设计模式清单](#五可复用设计模式清单)

---

## 一、项目定位与系统全景

PrivShield 是一个**企业级数据安全流通与隐私治理 Sidecar / 中台系统**，实现**「三层四柱五御六类」**安全治理体系：
- **算力面 (PrivShield Core)**：Python 3.13+（`engine/`）实现的高性能无状态隐私原语（脱敏、差分隐私、K-匿名、查询混淆）与 3 层动态分类分级漏斗（Rule → Small-NER → Local LLM）；
- **调度面 (Enterprise Services)**：Go 1.25 微服务集群负责多源数据资产管理、6 阶段流水线任务编排调度、PostgreSQL 原子租约并发与 9 要素密码学防篡改存证；
- **展现面 (Console & BFF)**：双 BFF（`console/bff-go` 与 `console/app-lz/bff-go`）与 React 18 + TypeScript 现代化测试控制台群。

---

## 二、核心设计哲学与标准实践

### 2.1 分层 Monorepo 架构

```text
PrivShield/ (Repo Root)
├── engine/               → 核心隐私算力与分类引擎 (Python 3.13+)
│   ├── privacy/          → 隐私原语与数学加噪 (dp, masking, kano, qol, budget)
│   ├── dynclassification/→ 3 层动态分类漏斗 (RuleEngine → Small-NER → Local LLM)
│   ├── security/         → 传输/认证安全 (TLS, mTLS, API Key, RateLimit, 白名单)
│   ├── observability/    → 结构化日志、OTel 追踪与 Prometheus 40+ 指标
│   └── gateway/          → P2C 智能动态负载均衡网关
├── services/             → 企业级中台微服务群 (Go 1.25 集群)
│   ├── service-hub/      → 数据服务调度中枢 (:8082 / :50052)
│   ├── datasource-mgr/   → 数据源与资产管理微服务 (:8083 / :50053)
│   └── audit-log/        → 合规存证与 9 要素哈希链微服务 (:8084 / :50054)
├── console/              → 统一管理与测试控制台群
│   ├── bff-go/           → Go BFF 代理网关 (:8081 / :50055)
│   ├── app-lz/           → 业务专有 BFF 与 E2E 测试器 (:8085)
│   └── web/ & app-lz/web/→ React 18 + TS + Vite 前端单页应用 (:5173)
├── pkg/                  → Go 全局共享基础库 (naming, middleware, store, crypto, tlsutil, metrics, validation, agent)
├── deploy/               → 云原生部署套件 (Helm, K8s, Compose, Prometheus, Grafana)
├── config/               → 运行时配置与 mtls-whitelist.yaml
└── rules/                → 分类分级领域规则库与标准体系 YAML
```

### 2.2 双栈同源协议支持

```text
REST (FastAPI, :8079)  ←→  PrivacyService (业务中枢)  ←→  隐私算法原语
gRPC (grpcio, :50051)  ←→  PrivacyService (业务中枢)  ←→  隐私算法原语
```
- Protobuf 契约定义在 `proto/privacy.proto`；
- REST 与 gRPC 共享同一底层 `PrivacyService`，保证跨协议行为 100% 一致；
- Python Agent 提供三个入口：`engine/main.py`（仅 REST，默认 `127.0.0.1:8079`）、`engine/server.py`（REST+gRPC 合一，默认 `0.0.0.0:8079 / 0.0.0.0:50051`）、`engine/grpc_server.py`（仅 gRPC，默认 `0.0.0.0:50051`）。

### 2.3 纵深安全防御体系

| 层次 | 实现机制 | 说明 |
|---|---|---|
| **传输加密** | TLS 1.3 / mTLS | 支持服务端证书与双向客户端证书校验；Go gRPC 服务器统一注册 `pkg/tlsutil` 的 `NewWhitelistInterceptor()` unary/stream 拦截器，按 `PRIVACY_AUTH_MTLS_WHITELIST_FILE` 加载 `config/mtls-whitelist.yaml`，5 秒 mtime 轮询热重载 |
| **访问认证** | API Key (Bearer Token) | 内外部 API Key 独立隔离，Fail-Closed 零信任拦截 |
| **静态加密** | SM4-GCM 信封加密 | 快照持久化数据带 `enc:v1:` 前缀加密，读取时透明还原 |
| **存证安全** | 9 要素哈希链 | 区块链式链式锚定，秒级在线核验防篡改 |
| **流量限速** | 令牌桶算法 (Token Bucket) | 支持 IP/租户级别独立限流，防单点资源耗尽 |
| **并发保护** | 并发信号量 (MaxConcurrent) | 全局在途请求上限拦截（503），保护线程池与连接池 |

### 2.4 全链路可观测性三支柱

| 支柱 | 技术选型 | 说明 |
|---|---|---|
| **Metrics** | prometheus-client / Go pkg/metrics | 统一抓取 Python 40+ 指标与 Go 15+ 指标，预置全景看板与专属调度大屏 |
| **Tracing** | OpenTelemetry (OTLP) / TraceID | `X-Request-ID` / `X-Trace-ID` 双头传递，Span 树全链路关联 |
| **Logging** | 结构化 JSON / Text 双格式 | `trace_id` 全链路自动注入，支持敏感字段上下文拦截 |

### 2.5 分布式全局隐私预算中枢

- **多后端统一抽象**：`PRIVACY_BUDGET_DB` 适配 SQLite、PostgreSQL 及内存模式；
- **原子记账与时间窗口重置**：支持多 Pod 强一致记账，配置 `PRIVACY_BUDGET_WINDOW_SECONDS` 实现自动周期重置；
- **不可篡改 HMAC 审计**：`BudgetAuditLogger` 对每笔预算消耗记录进行 HMAC-SHA256 签名存证。

### 2.6 企业级中台微服务群实践

各服务使用独立前缀的环境变量控制 HTTP/gRPC/TLS 等运行参数（`SERVICE_HUB_*`、`DATASOURCE_MGR_*`、`AUDIT_LOG_*`、`PRIVACY_CONSOLE_*`），并共享 `PRIVACY_AUTH_MTLS_WHITELIST_FILE`、`PRIVACY_AGENT_*`、`PRIVACY_REST_PORT` 等全局配置。

- **`service-hub` (:8082 / :50052)**：流水线 6 阶段调度编排（`Ingest` ➔ `Fetch` ➔ `Classify` ➔ `Desensitize` ➔ `Return` ➔ `Audit`）与 Worker Pool 异步削峰；
  - **PostgreSQL 租约并发**：`LeasedTaskStore` 基于 `FOR UPDATE SKIP LOCKED` 实现多副本并发抢占与防脑裂；
  - **崩溃恢复与自动重试**：启动时回收孤立任务，周期性后台指数退避自动重试；
  - 📖 [可靠性能力详解](../../services/service-hub/docs/reliability.md)
- **`datasource-mgr` (:8083 / :50053)**：多源异构资产纳管、内置医保与康养模拟库（`yibao.csv` & `kangyang.csv`）、动态元数据自动探查与样本切片安全提取；
  - 📖 [可靠性能力详解](../../services/datasource-mgr/docs/reliability.md)
- **`audit-log` (:8084 / :50054)**：基于 9 要素特征的不可篡改 SHA-256 存证哈希链与 SM4-GCM 快照加密；
  - **在线核验**：`POST /api/audit/chain/verify` 接口实时定位断裂节点；
  - 📖 [可靠性能力详解](../../services/audit-log/docs/reliability.md)

---

## 三、核心高光工程设计

### 3.1 三层递进式动态分类分级漏斗 (3-Layer Funnel)

```text
Layer 1: YAML 规则引擎 (10~50μs) → 正则/词典/组合条件/Safety Floor 过滤 85%+ 明确数据
  ↓ (未命中或低置信)
Layer 2: Small-NER 引擎 (1~5ms)   → ONNX 抽取中文专有实体（跳过纯数字与英文字段）
  ↓ (语义存疑/规则冲突/多模态图像)
Layer 3: Local LLM 仲裁 (100~500ms) → Qwen3.5 语义仲裁 (内存 <512MB 降级与信号量限流防 OOM)
```

### 3.2 智能动态负载均衡 (P2C + Client-Side LB)

- **Go 客户端多节点负载池 (`pkg/agent/client.go`)**：原生支持 `PRIVACY_AGENT_URLS` 集群列表，内置平滑轮询与三态熔断故障转移；
- **Python 网关 P2C 调度 (`engine/gateway/balancer.py`)**：Power of Two Choices 算法结合在途连接与响应延迟动态打分分流，消除羊群效应；
- **动态拓扑管理**：运行时 API 注册/注销/隔离/排空/激活后端节点。

### 3.3 9 层统一中间件栈与纵深防 DDoS 体系

所有 Go 微服务统一装配 9 层中间件栈：
```text
TraceMiddleware ➔ StructuredLogger ➔ Recovery ➔ SecurityHeaders ➔ MaxBodySize ➔ MaxConcurrent ➔ RateLimit ➔ CORS ➔ Auth
```
- **慢速连接防护 (Anti-Slowloris)**：配置 `ReadHeaderTimeout(5s)`、`ReadTimeout(30s)` 与 `MaxHeaderBytes(1MB)`；
- **请求体上限 (Payload Protection)**：`MaxBodySize` 限制 32MB/64MB 硬顶拦截（413）；
- **IP 令牌桶限流 (HTTP Flood)**：`RateLimit` 基于 IP 提供高精度令牌桶，超额返回 429；
- **并发容量熔断 (Concurrency Cap)**：`MaxConcurrent` 实施全局在途并发信号量拦截（503），保护协程池。

---

## 四、工程注意事项与避坑指南

1. **gRPC 延迟初始化**：`grpc_stub` 必须在当前 AsyncIO Event Loop 中延迟创建，避免在模块加载时提前绑定已关闭的事件循环；
2. **探针不设防**：`/health` 探针路由严禁挂载认证/限流中间件，防止 K8s 存活检查因无 Token 而导致容器被异常重启；
3. **大模型并发保护**：本地 LLM 推理必须由进程级信号量（`PRIVACY_LLM_MAX_CONCURRENCY`）保护，防止并发打满显存引起 CUDA OOM；
4. **单副本与多副本持久化约束**：SQLite 仅支持单副本 `Recreate` 部署；多副本 Hub 必须基于 PostgreSQL `LeasedTaskStore` 运行，禁止在共享网络卷上挂载 SQLite。

---

## 五、可复用设计模式清单

| 模式名称 | 应用场景 | 本项目代表性实现 |
|---|---|---|
| **Sidecar Pattern** | 语言无关服务化 | 独立部署提供 REST (:8079) + gRPC (:50051) |
| **Funnel Pattern** | 递进式智能分级 | Rule (10μs) ➔ NER (1ms) ➔ LLM (100ms) |
| **Graceful Degradation** | 可选重依赖解耦 | LLM/NER 缺失时回退规则层与人工审核标记 |
| **P2C (Power of Two Choices)** | 动态负载均衡 | 随机选取两节点对比在途连接与延迟打分分流 |
| **Client-Side Balancing** | 微服务高可用 | `pkg/agent/client.go` 多节点平滑轮询与三态熔断 |
| **Leased Task Pattern** | 分布式无锁抢占 | `pkg/store/postgres` `FOR UPDATE SKIP LOCKED` 原子租约 |
| **Envelope Encryption** | 敏感静态数据落盘 | `pkg/crypto` SM4-GCM `enc:v1:...` |
| **Cryptographic Hash Chain** | 存证防篡改审计 | `services/audit-log` 9 要素区块链式哈希链 |
| **Single Source of Truth** | 跨语言业务命名一致性 | `pkg/naming` 常量注册表与别名归一化 |