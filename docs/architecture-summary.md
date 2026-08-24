# PrivShield 架构设计与工程实践总结 (Architecture & Engineering Summary)

> **版本**：v2.0.0  
> **适用范围**：`PrivShield` 核心算力引擎、企业级中台微服务群（`service-hub` / `datasource-mgr` / `audit-log`）、控制台 BFF 体系（`bff-go` / `bff-py` / `web`）及全局云原生基础设施。  
> **关联文档**：[architecture-design.md](architecture-design.md)（详细架构设计）、[production_optimization_design.md](production_optimization_design.md)（生产级优化设计）。

---

## 目录 (Table of Contents)

- [一、项目定位与架构演进](#一项目定位与架构演进)
- [二、核心设计哲学与标准实践](#二核心设计哲学与标准实践)
  - [2.1 分层 Monorepo 架构](#21-分层-monorepo-架构)
  - [2.2 双栈同源协议支持](#22-双栈同源协议支持)
  - [2.3 纵深安全防御体系](#23-纵深安全防御体系)
  - [2.4 全链路可观测性三支柱](#24-全链路可观测性三支柱)
  - [2.5 分布式全局隐私预算中枢](#25-分布式全局隐私预算中枢)
  - [2.6 企业级中台微服务群实践](#26-企业级中台微服务群实践)
  - [2.7 测试与自动化验证矩阵](#27-测试与自动化验证矩阵)
  - [2.8 多环境云原生部署与弹性扩缩](#28-多环境云原生部署与弹性扩缩)
- [三、核心高光工程设计](#三核心高光工程设计)
  - [3.1 三层递进式动态分类分级漏斗 (3-Layer Funnel)](#31-三层递进式动态分类分级漏斗-3-layer-funnel)
  - [3.2 智能动态负载均衡 (P2C + Client-Side LB)](#32-智能动态负载均衡-p2c--client-side-lb)
  - [3.3 分布式无噪累加器 (Accumulator)](#33-分布式无噪累加器-accumulator)
  - [3.4 向量化加速与 PyArrow 零拷贝](#34-向量化加速与-pyarrow-零拷贝)
  - [3.5 可选依赖优雅降级 (NoOp Pattern)](#35-可选依赖优雅降级-noop-pattern)
  - [3.6 模拟数据源引擎与自动种子注入](#36-模拟数据源引擎与自动种子注入)
- [四、工程注意事项与避坑指南](#四工程注意事项与避坑指南)
- [五、技术栈速查总表](#五技术栈速查总表)
- [六、可复用设计模式清单](#六可复用设计模式清单)

---

## 一、项目定位与架构演进

PrivShield 是一个**企业级数据安全流通与隐私治理 Sidecar / 中台系统**，实现**「三层四柱五御六类」**安全治理体系：
- **算力面 (PrivShield Core)**：Python 3.13+ 实现的高性能无状态隐私原语（脱敏、差分隐私、K-匿名、查询混淆）与 3 层动态分类分级漏斗；
- **调度面 (Enterprise Services)**：Go 1.25 微服务集群负责多源数据资产管理、流水线任务编排调度与不可篡改存证；
- **展现面 (Console & BFF)**：双 BFF（Go gRPC / Python REST）网关与 React 18 现代化测试控制台。

---

## 二、核心设计哲学与标准实践

### 2.1 分层 Monorepo 架构

```text
PrivShield/ (Repo Root)
├── PrivShield/           → 核心隐私算力引擎 (Python 3.13+)
│   ├── privacy/          → 隐私原语与数学加噪 (dp, masking, kano, qol, budget)
│   ├── dynclassification/→ 3 层分类漏斗 (RuleEngine → Small-NER → Local LLM)
│   ├── security/         → 传输/认证安全 (TLS, mTLS, API Key, RateLimit)
│   ├── observability/    → 结构化日志、OTel 追踪与 Prometheus 指标
│   └── gateway/          → P2C 智能动态负载均衡网关
├── services/             → 企业级中台微服务群 (Go 1.25 集群)
│   ├── service-hub/      → 数据服务调度中枢 (:8082)
│   ├── datasource-mgr/   → 数据源与资产管理微服务 (:8083)
│   └── audit-log/        → 合规存证与审计日志微服务 (:8084)
├── console/              → 统一管理控制台
│   ├── bff-go/           → Go gRPC 聚合网关 / 主力 BFF (:8081)
│   ├── bff-py/           → Python REST 代理网关 / 备用 BFF (:8080)
│   └── web/              → React 18 + TS + Vite 前端单页应用 (:5173)
├── pkg/                  → Go 全局共享基础库 (Client-Side LB, Store, Metrics)
├── deploy/               → 云原生部署套件 (Helm, K8s, Compose, Prometheus, Grafana)
└── rules/                → 分类分级领域规则库与标准体系 YAML
```

### 2.2 双栈同源协议支持

```text
REST (FastAPI, :8079)  ←→  PrivacyService (业务中枢)  ←→  隐私算法原语
gRPC (grpcio, :50051)  ←→  PrivacyService (业务中枢)  ←→  隐私算法原语
```
- Protobuf 契约定义在 `proto/privacy.proto`；
- REST 与 gRPC 共享同一底层 `PrivacyService`，保证跨协议行为 100% 一致。

### 2.3 纵深安全防御体系

| 层次 | 实现机制 | 说明 |
|---|---|---|
| **传输加密** | TLS 1.3 / mTLS | 支持服务端证书与双向客户端证书校验，支持 CN 白名单热重载 |
| **访问认证** | API Key (Bearer Token) | 内外部 API Key 独立隔离 |
| **细粒度授权** | RBAC (Scope-based) | `require_permission("dp:query")` 权限域管控 |
| **流量限速** | 令牌桶算法 (Token Bucket) | 支持 IP/租户级别独立限流，防单点资源耗尽 |

### 2.4 全链路可观测性三支柱

| 支柱 | 技术选型 | 说明 |
|---|---|---|
| **Metrics** | prometheus-client / Go pkg/metrics | 统一抓取 5 大组件，预置全景看板与 Service Hub 专属调度大屏 |
| **Tracing** | OpenTelemetry (OTLP) | 分布式链路追踪（可选），未启用时 zero-overhead no-op |
| **Logging** | 结构化 JSON / Text 双格式 | `request_id` 全链路透传，支持敏感字段上下文拦截 |

### 2.5 分布式全局隐私预算中枢

- **多后端统一抽象**：`PRIVACY_BUDGET_BACKEND=redis|sqlite|memory`；
- **Redis 分布式原子记账**：采用 Redis Lua 脚本在集群多 Pod 间执行原子性 $(\epsilon, \delta)$ 扣减与滑动时间窗口（`window_seconds`）重置，防并发预算穿透；
- **不可篡改 HMAC 审计**：`BudgetAuditLogger` 对每笔预算消耗记录进行 HMAC-SHA256 签名存证。

### 2.6 企业级中台微服务群实践

- **`service-hub` (:8082)**：流水线 6 阶段调度编排（`Ingest` ➔ `Fetch` ➔ `Classify` ➔ `Desensitize` ➔ `Return` ➔ `Audit`）与 Worker Pool 异步削峰；
- **`datasource-mgr` (:8083)**：多源异构资产纳管、内置医保与康养模拟库（`yibao.csv` & `kangyang.csv`）、动态元数据自动探查与样本抽样；
- **`audit-log` (:8084)**：基于 8 维度特征的不可篡改 SHA-256 存证哈希链。

### 2.7 测试与自动化验证矩阵

- **单元测试**：40+ Python 单测 + 5 大 Go 模块单测（`make test-go`）；
- **属性与分布测试**：`hypothesis` 属性测试与 `scipy` KS 噪声分布假设检验；
- **端到端 E2E 回归**：`scripts/dev/run_console_e2e_tests.sh` 跨服务 5 阶段流水线测试；
- **极限性能压测**：`scripts/test/stress_test_suite.py` 输出并发 QPS 与 P50/P90/P95/P99 延迟 SLA 报告。

### 2.8 多环境云原生部署与弹性扩缩

- **多模式支持**：Docker Compose（含 5 大服务）、原生 K8s 清单与生产级 Helm Chart；
- **多维自动扩缩容**：
  - 基于 Prometheus QPS 速率与 LLM 排队深度的自定义 HPA；
  - KEDA 事件驱动 `ScaledObject` 毫秒级自动弹性伸缩；
  - CronHPA 潮汐预测扩缩容（工作日早高峰 08:15 自动扩容至 10 副本）。

---

## 三、核心高光工程设计

### 3.1 三层递进式动态分类分级漏斗 (3-Layer Funnel)

```text
Layer 1: YAML 规则引擎 (10~50μs) → 正则/词典/组合条件过滤 85%+ 明确数据
  ↓ (未命中或低置信)
Layer 2: Small-NER 引擎 (1~5ms)   → ONNX / ModelScope 抽取专有实体
  ↓ (复杂上下文/长文本)
Layer 3: Local LLM 仲裁 (100~500ms) → Qwen3.5 语义判定与无痕平滑 (信号量限流防 OOM)
```

### 3.2 智能动态负载均衡 (P2C + Client-Side LB)

- **Go 客户端多节点负载池 (`pkg/agent/client.go`)**：原生支持 `PRIVACY_AGENT_URLS` 集群列表，内置平滑轮询与节点宕机自动剔除与容灾切换；
- **Python 网关 P2C 调度 (`PrivShield/gateway/balancer.py`)**：Power of Two Choices 算法结合在途连接与响应延迟动态打分分流，消除羊群效应。

### 3.3 分布式无噪累加器 (Accumulator)

```python
@dataclass
class Accumulator:
    """MapReduce / 联邦学习场景：Worker 本地无噪累加 → Master 合并 → 统一注入一次噪声"""
    count: int = 0
    sum: float = 0.0
    sum_squares: float = 0.0

    def __add__(self, other): ...
    def finalize_dp(self, epsilon, delta, mechanism): ...
```
**优势**：避免分布式多节点各自加噪导致的噪声累积放大问题。

### 3.4 向量化加速与 PyArrow 零拷贝

- 统一采用 C-contiguous 内存布局（`np.ascontiguousarray`）；
- 核心批处理支持 **PyArrow Table** 零拷贝数据传递，大数据量下比纯 Python 循环快 10~100 倍。

### 3.5 可选依赖优雅降级 (NoOp Pattern)

- 核心算力（规则分类 + 隐私原语）仅需标准轻量依赖；
- 重型 AI 依赖（`torch`, `transformers`, `onnxruntime`, `redis`）在初始化时按优先级自动探测，缺失时注入 NoOp 实现并输出明确 Warning，不影响服务启动。

### 3.6 模拟数据源引擎与自动种子注入

- `services/datasource-mgr` 内置 `yibao.csv`（医保结算）与 `kangyang.csv`（康养慢病）真实模拟数据库；
- 启动自动执行 `SeedMockDataSources` 注入存储，支持真实字段元数据动态解析与 `/api/datasources/:id/records` 样本抽样。

### 3.7 全栈纵深防 DDoS 体系 (Multi-Tier Anti-DDoS)

- **慢速连接防护 (Anti-Slowloris)**：全微服务配置 `ReadHeaderTimeout(5s)`、`ReadTimeout(30s)` 与 `MaxHeaderBytes(1MB)`；
- **请求体上限 (Payload Protection)**：`pkg/middleware.MaxBodySize` 与网关 `Content-Length` 预检实施 32MB/64MB 硬顶拦截（413）；
- **IP 令牌桶限流 (HTTP Flood)**：`pkg/middleware.RateLimit` 基于 IP 提供高精度令牌桶，自动 GC 闲置 IP；
- **并发容量熔断 (Concurrency Cap)**：`pkg/middleware.MaxConcurrent` 实施全局在途并发信号量拦截（503），保护协程池。

---

## 四、工程注意事项与避坑指南

1. **gRPC 延迟初始化**：`grpc_stub` 必须在当前 AsyncIO Event Loop 中延迟创建，避免在模块加载时提前绑定已关闭的事件循环；
2. **探针不设防**：`/health` 探针路由严禁挂载认证/限流中间件，防止 K8s 存活检查因无 Token 而导致容器被异常重启；
3. **大模型并发保护**：本地 LLM 推理必须由进程级信号量（`PRIVACY_LLM_MAX_CONCURRENCY`）保护，防止并发打满显存引起 CUDA OOM；
4. **长连接负载倾斜治理**：微服务调用 Agent 时，应使用 `pkg/agent/client.go` 的 Client-Side LB 机制或 K8s Headless Service。

---

## 五、技术栈速查总表

| 领域 | 组件 / 框架 | 核心用途与选型考量 |
|---|---|---|
| **核心算力** | Python 3.13+ / FastAPI / Pydantic v2 | 异步高性能 REST API + Rust 核心强类型输入校验 |
| **RPC 通信** | gRPC / Protocol Buffers | 强类型二进制低延迟 RPC 通信 |
| **AI / 分类** | YAML 规则 / ONNX / Qwen3.5 | 3 层漏斗智能定级与语义平滑 |
| **中台微服务** | Go 1.25 / Gin / ByteDance Sonic | 超轻量并发流水线调度，JIT+SIMD 极速序列化 |
| **存储持久化** | Redis / SQLite (Pure Go WAL) | 分布式原子预算记账与轻量无 CGO 嵌入式存储 |
| **表现层** | React 18 / TypeScript / Vite / Tailwind | 纯函数式单页控制台，毫秒级 HMR 与极小 CSS 产物 |
| **云原生部署** | Helm / KEDA / CronHPA / Docker Compose | 企业级声明式编排、业务指标弹性扩缩容 |
| **可观测性** | Prometheus / Grafana / OTel | 5 大服务指标采集、专属调度看板与微服务告警组 |

---

## 六、可复用设计模式清单

| 模式名称 | 应用场景 | 本项目代表性实现 |
|---|---|---|
| **Sidecar Pattern** | 语言无关服务化 | 独立部署提供 REST (:8079) + gRPC (:50051) |
| **Funnel Pattern** | 递进式智能分级 | Rule (10μs) ➔ NER (1ms) ➔ LLM (100ms) |
| **Graceful Degradation** | 可选重依赖解耦 | Redis/LLM/NER 缺失时回退基础可用子集 |
| **P2C (Power of Two Choices)** | 动态负载均衡 | 随机选取两节点对比在途连接与延迟打分分流 |
| **Client-Side Balancing** | 微服务高可用 | `pkg/agent/client.go` 多节点平滑轮询与故障转移 |
| **Accumulator Pattern** | 分布式聚合加噪 | `Accumulator.__add__` + `finalize_dp` 单次注噪 |
| **Token Bucket & DDoS Shield** | 租户级流量整形与防刷 | `security/ratelimit.py` + `pkg/middleware.RateLimit` |
| **Concurrency Semaphore** | 系统过载容量保护 | `pkg/middleware.MaxConcurrent` + LLM 信号量限流 |
| **HMAC & Hash Chain** | 防篡改存证审计 | `BudgetAuditLogger` + `audit-log` SHA-256 存证链 |