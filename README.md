# 数盾 PrivShield (Data & Privacy Shield)

> **数联天下 · 数盾 (PrivShield)** —— 企业级数据隐私计算、多原语脱敏与三层动态分类分级治理中台 (Data Privacy & Security Governance Sidecar & Platform)，全面落地 **「三层四柱五御六类」数据安全与隐私治理架构**，提供 REST + gRPC 双协议高可用服务与政务级全链路流通调度中台。
>
> 🌐 **GitHub Repository**: [https://github.com/fengzhizi319/PrivShield](https://github.com/fengzhizi319/PrivShield)

---

## 一、 平台架构与多语言分层

PrivShield 采用现代**多语言分层云原生 Monorepo 架构**，清晰解耦底层算力、业务编排中台与表现层接入：

```text
PrivShield/ (Repo Root)
├── engine/                       # 【Python 隐私算力引擎】REST(:8079) + gRPC(:50051)
│   ├── main.py / grpc_server.py / server.py
│   ├── privacy/                  # 脱敏 (Masking)、差分隐私 (DP/LDP)、K-匿名、查询混淆 (QOL)、预算记账
│   ├── dynclassification/        # 三层分类分级漏斗 (Rule -> NER -> LLM/VLM)
│   ├── security/                 # TLS 1.3 / mTLS 白名单 / API Key 鉴权 / 滑动窗口限流
│   ├── observability/            # 结构化日志、Prometheus /metrics、OpenTelemetry Tracing
│   └── gateway/                  # P2C 负载均衡与反向代理
├── services/                     # 【Go 企业级中台微服务群】
│   ├── service-hub/              # 数据服务调度中枢 (:8082) - 流水线编排 (Ingest→Classify→Mask→Audit)
│   ├── datasource-mgr/           # 数据源与资产管理微服务 (:8083) - CSV/DB 连接池、元数据探查与抽样
│   └── audit-log/                # 脱敏审计与存证微服务 (:8084) - 审计快照、SHA-256 存证哈希链
├── console/                      # 【统一控制台与接入层】
│   ├── web/                      # React 18 + TS + Vite + TailwindCSS 交互控制台 (:5173)
│   ├── bff-go/                   # 主力 Go gRPC API Gateway / BFF (:8081)
│   └── bff-py/                   # 备用 Python FastAPI 代理网关 (:8080)
├── pkg/                          # 【Go 全局共享基础库】连接池、中间件、安全防御、SQLite/Memory 存储
├── proto/                        # 【Protobuf 契约定义】privacy.proto / servicehub.proto
├── deploy/                       # 【云原生运维套件】Docker Compose / Helm / K8s / Prometheus / Grafana
├── config/                       # 环境变量模板、Profile YAML、mTLS 白名单
├── rules/                        # 分类分级标准 (GB/T 37988, 医疗, 医保, 金融) 与规则体系
└── scripts/                      # 开发、测试、压测与生产自动化运维工具链
```

---

## 二、 核心能力概览

### 1. 隐私保护计算原语 (Processing Primitives)

| 隐私原语 | REST 端点 | gRPC 接口 | 本地 SDK 方法 | 算法特性 |
|---|---|---|---|---|
| **数据脱敏** | `POST /v1/privacy/mask` | `Mask` | `PrivacyService.mask` | 字段语义识别、掩码掩盖、FPE 格式保留加密 |
| **整记录脱敏** | `POST /v1/privacy/mask_record` | `MaskRecord` | `PrivacyService.mask_record` | 批量字段并行处理、个性化 Profile 策略 |
| **HMAC 哈希** | `POST /v1/privacy/hash` | `Hash` | `PrivacyService.hash` | 盐值混淆、SHA-256 不可逆单向变换 |
| **差分隐私计数** | `POST /v1/privacy/dp/count` | `DPCount` | `PrivacyService.dp_count` | Laplace / Gaussian 机制、预算实时消耗 |
| **差分隐私求和** | `POST /v1/privacy/dp/sum` | `DPSum` | `PrivacyService.dp_sum` | 灵敏度截断、解析高斯极值保护 |
| **差分隐私均值** | `POST /v1/privacy/dp/mean` | `DPMean` | `PrivacyService.dp_mean` | 边界夹紧、噪声校准 |
| **本地差分隐私 LDP** | `POST /v1/privacy/ldp` | `LDP` | `PrivacyService.ldp` | 本地化随机响应、频数/直方图估计 |
| **K-匿名泛化** | `POST /v1/privacy/k_anonymize/record` | `KAnonymizeRecord` | `PrivacyService.k_anonymize_record` | Mondrian 多维区间划分、准标识符泛化 |
| **文件级隐私处理** | `POST /v1/file/process` | — | `PrivacyService.process_file` | CSV/Excel/JSON 自动识别、字段级脱敏 |
| **医疗数据流水线** | `POST /v1/medical/process` | — | `PrivacyService.process_medical` | DICOM/HL7/FHIR 解析、影像脱敏 |
| **参数推荐** | `POST /v1/profile/recommend` | — | `PrivacyService.recommend_profile` | 基于数据特征推荐脱敏策略 |
| **运维诊断** | `GET /v1/ops/diagnostics` | — | — | 运行时健康、依赖与配置快照 |
| **查询混淆注入** | `POST /v1/privacy/qol/obfuscate` | `ObfuscateQuery` | `PrivacyService.obfuscate_query` | 假查询注入 (Dummy Injection)、KL 散度混淆 |
| **隐私预算记账** | `GET /v1/privacy/budget` | `Health` | `PrivacyService.budget_remaining` | 内存/SQLite/Redis Lua 原子记账、滑动窗口重置 |

### 2. 动态数据分类分级三层漏斗 (Dynamic Classification Funnel)

引擎创新性地构建了 **「规则引擎 ➔ 实体识别 ➔ 认知仲裁」** 阶梯式识别架构：

1. **Layer-1: 高性能声明式规则引擎 (Rule Engine)**：毫秒级 YAML 规则匹配、正则表达式、关键词字典、校验和算子（如身份证校验码算子）；
2. **Layer-2: 轻量命名实体识别 (Small-NER)**：采用轻量 ONNX / ModelScope NER 模型，针对无规则显式特征的文本段落提取上下文实体；
3. **Layer-3: 大语言模型/多模态仲裁 (Local LLM / VLM)**：集成 Qwen3.5 等本地模型，对低置信度、歧义场景或医学影像 (DICOM) 执行语义仲裁；
4. **安全底座兜底 (Safety Floor)**：高敏安全红线机制，确保任何降级与仲裁均不低于法定最低保护等级。

### 3. 企业级数据流通中台微服务群 (Go Microservices)

- **调度中枢 ([services/service-hub](file:///home/charles/code/sfwork/PrivShield/services/service-hub))**：串联国密 VPN 专线网关、任务流转、分类分级打标、动态脱敏处理、存证上链回传 6 阶段流水线；
  - **崩溃恢复与自动重试**：启动时自动回收孤立任务，周期性后台重试失败任务（指数退避 + RetryCount）；
  - **HTTP/gRPC 双协议 mTLS**：共享 `pkg/tlsutil` 工具库，TLS 1.3 + 公钥固定；
  - 📖 [可靠性能力详解](services/service-hub/docs/reliability.md)
- **数据源资产管理 ([services/datasource-mgr](file:///home/charles/code/sfwork/PrivShield/services/datasource-mgr))**：提供 4 个模拟数据源接口（医保 `yibao`、康养 `kangyang` 及 2 个预留接口），支持 HTTPS REST + gRPC mTLS 双协议与公钥固定，内置数据抽样与资产目录；
  - 📖 [可靠性能力详解](services/datasource-mgr/docs/reliability.md)
- **脱敏审计与存证 ([services/audit-log](file:///home/charles/code/sfwork/PrivShield/services/audit-log))**：实时落盘脱敏快照，构建基于 SHA-256 的不可篡改哈希存证链与合规只读看板；
  - **完整性校验**：启动时 `PRAGMA integrity_check` + HMAC-SHA256 签名审计日志 + 独立校验脚本；
  - 📖 [可靠性能力详解](services/audit-log/docs/reliability.md)
- **控制台 BFF ([console/bff-go](file:///home/charles/code/sfwork/PrivShield/console/bff-go))**：基于 gRPC 连接池实现请求聚合、多节点 Client-Side 轮询与故障转移；
  - **gRPC 自动重试**：可配置重试策略（默认最多 6 次，指数退避 1s→8s）；
  - 📖 [可靠性能力详解](console/bff-go/docs/reliability.md)

### 4. 全栈多层次纵深防 DDoS 与安全基底 (Anti-DDoS & Security Shield)

- **协议级慢速攻击防护 (Anti-Slowloris)**：强制设置 `ReadHeaderTimeout: 5s`、`ReadTimeout: 30s` 与 `MaxHeaderBytes: 1MB`；
- **大包 DoS 拦截 (MaxBodySize)**：全微服务配置 32MB/64MB 请求体上限，超限使用 `http.MaxBytesReader` 快速返回 `413 Payload Too Large`；
- **IP 令牌桶防刷 (RateLimit)**：提供并发安全 `IPRateLimiter`（自动后台 GC 10 分钟闲置 IP 桶），超额响应 `429 Too Many Requests` 与 `Retry-After: 1`；
- **并发容量硬顶 (MaxConcurrent)**：信号量并发熔断保护协程池，过载快速响应 `503 Service Unavailable`；
- **数据源沙箱防护 (LFI Prevention)**：CSV 上传校验 `.csv` 白名单、提取 `BaseName` 并在指定目录沙箱内加载，硬性限制 50,000 行。

---

## 三、 快速开始 (Quick Start)

### 1. 本地快速启动（算力引擎）

```bash
# 1. 激活虚拟环境并安装依赖
python -m venv .venv
source .venv/bin/activate
pip install -e .

# 2. 启动 REST + gRPC 联合服务
python -m engine.server
```

### 2. Docker Compose 全栈一键运行

```bash
# 启动核心服务（Agent + Go BFF + Web UI）
bash ./scripts/dev/docker-start-go.sh

# 启动全栈微服务群（Agent + 3 Go 中台微服务 + 双 BFF + Web UI）
bash ./scripts/dev/docker-start-all.sh

# 启动全栈 + vLLM 本地大模型推理
bash ./scripts/dev/docker-start-all.sh --with-llm

# 启动 Prometheus + Grafana 监控大屏
docker compose --profile monitoring up -d

# 一键停止全栈容器
bash ./scripts/dev/docker-stop.sh
```

### 3. 全服务端口与职责速查表

| 服务模块 | 默认端口 | 运行形态 | 职责说明 |
|---|---|---|---|
| **Privacy Engine (REST)** | `8079` | Python / FastAPI | 核心隐私算法与分类分级 REST 接口 |
| **Privacy Engine (gRPC)** | `50051` | Python / gRPC | 核心隐私算法高性能 RPC 通信 |
| **Console Web UI** | `5173` | React 18 + Vite | 控制台可视化大盘与调试页面 |
| **Console BFF (Go)** | `8081` | Go / Gin + gRPC | 主力 BFF 聚合网关，连接池与多节点分流 |
| **Console BFF (Python)** | `8080` | Python / FastAPI | 备用 BFF 代理网关，支持流式解析 |
| **Service Hub** | `8082` | Go / Gin + gRPC | 数据流通流水线调度中枢微服务 |
| **Datasource Mgr** | `8083` | Go / Gin | 数据源管理与敏感特征自动探查微服务 |
| **Audit Log** | `8084` | Go / Gin | 脱敏审计快照与不可篡改存证微服务 |
| **vLLM (可选)** | `8000` | Python / vLLM | GPU 大模型/VLM 本地推理加速服务 |
| **Prometheus (可选)** | `9090` | Prometheus 容器 | 全微服务指标抓取与告警评估 |
| **Grafana (可选)** | `3000` | Grafana 容器 | 预置中台调度大盘与集群全景大屏 |

---

## 四、 自动化构建与测试

### 1. 运行多语言全量测试

```bash
# 运行 Go 基础库与全部 4 个 Go 微服务单测
make test-go

# 运行 Python 核心算力引擎单测（423 个用例）
PYTHONPATH=. pytest tests/ -q

# 运行 Python BFF 网关单测（37 个用例）
cd console/bff-py && pytest tests/ -v

# 运行前端控制台 Vitest 单测（77 个用例）
cd console/web && corepack pnpm test -- --run

# 运行真实跨服务 E2E 全链路流水线测试
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./services/service-hub/internal/handlers/
```

### 2. 容器镜像构建

```bash
# 构建 core 镜像（推荐，轻量算力镜像，不含 ML 大依赖）
make docker-core

# 构建 ml 镜像（含 Torch/Transformers/ModelScope 依赖）
make docker-ml

# 校验 Helm 语法与模板渲染
make helm-lint && make helm-template
```

### 3. 本地可编辑安装

```bash
pip install -e .
# 或安装完整开发依赖
pip install -e ".[dev,observability,docs]"
```

---

## 五、 生产安全与可观测性

### 1. 生产安全防护 (TLS/mTLS/Auth/RateLimit/DDoS)

所有安全特性默认开启平滑兼容，生产环境建议开启：

```bash
PRIVACY_TLS_ENABLED=true \
PRIVACY_TLS_CERT_FILE=deploy/tls/server.crt \
PRIVACY_TLS_KEY_FILE=deploy/tls/server.key \
PRIVACY_TLS_CA_FILE=deploy/tls/ca.crt \
PRIVACY_TLS_CLIENT_AUTH=require \
PRIVACY_AUTH_ENABLED=true \
PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true \
PRIVACY_AUTH_MTLS_WHITELIST_FILE=config/mtls-whitelist.yaml \
PRIVACY_RATE_LIMIT_ENABLED=true \
python -m engine.server
```

### 2. 生产可观测性 (Prometheus/Grafana/Tracing)

- **Prometheus 端点**：所有服务均暴露 `/metrics` 指标接口；
- **Grafana 预置大屏**：
  - [deploy/grafana/dashboard.json](file:///home/charles/code/sfwork/PrivShield/deploy/grafana/dashboard.json)：PrivShield 集群全景与算力监控大屏；
  - [deploy/grafana/service-hub-dashboard.json](file:///home/charles/code/sfwork/PrivShield/deploy/grafana/service-hub-dashboard.json)：数联数据服务调度中枢专属大屏。

### 3. 云原生 K8s 与 Helm 部署

```bash
# 生产 Helm 安装
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=your-tls-secret \
  --set security.auth.apiKeysSecret=your-apikeys-secret
```

---

## 六、 完整文档导航 (Documentation Hub)

项目提供基于 **MkDocs + Material** 的离线与在线文档书：

```bash
# 本地热重载预览 (http://127.0.0.1:8000)
make docs-serve

# 静态站点全量构建 (site/)
make docs-build
```

### 核心架构与中台微服务文档
- **[系统架构与全景设计 (Architecture Design)](file:///home/charles/code/sfwork/PrivShield/docs/architecture/architecture-design.md)**
- **[全平台目录架构重构方案 (Migration Design)](file:///home/charles/code/sfwork/PrivShield/docs/archive/migration-design.md)**
- **[企业级中台微服务总览 (Services Overview)](file:///home/charles/code/sfwork/PrivShield/services/README.md)**
- **[数据服务调度中枢文档 (Service Hub Docs)](file:///home/charles/code/sfwork/PrivShield/services/service-hub/docs/design.md)**
- **[数据源与资产管理文档 (Datasource Manager Docs)](file:///home/charles/code/sfwork/PrivShield/services/datasource-mgr/docs/design.md)**
- **[脱敏审计与不可篡改存证文档 (Audit Log Docs)](file:///home/charles/code/sfwork/PrivShield/services/audit-log/docs/design.md)**
- **[Go 全局共享基础库文档 (Pkg README)](file:///home/charles/code/sfwork/PrivShield/pkg/README.md)**
- **[统一控制台与接入层手册 (Console README)](file:///home/charles/code/sfwork/PrivShield/console/README.md)**

### 隐私原语与分类算法文档
- **[数据脱敏设计 (Masking Design)](file:///home/charles/code/sfwork/PrivShield/docs/masking/design.md)**
- **[差分隐私机制 (Differential Privacy Design)](file:///home/charles/code/sfwork/PrivShield/docs/dp/design.md)**
- **[K-匿名算法 (K-Anonymity Design)](file:///home/charles/code/sfwork/PrivShield/docs/k_anonymity/design.md)**
- **[查询混淆注入 (Query Obfuscation Design)](file:///home/charles/code/sfwork/PrivShield/docs/qol/design.md)**
- **[三层动态分类分级漏斗 (3-Layer Funnel Design)](file:///home/charles/code/sfwork/PrivShield/docs/dynclassification/three_layer_funnel_design.md)**

### 生产治理、安全与部署文档
- **[生产安全规范与设计 (Production Security Design)](file:///home/charles/code/sfwork/PrivShield/docs/production_security/design.md)**
- **[安全合规要求与审计修复表 (Security Requirements)](file:///home/charles/code/sfwork/PrivShield/docs/production_security/security_requirements.md)**
- **[生产可观测性设计 (Observability Design)](file:///home/charles/code/sfwork/PrivShield/docs/production_observability/design.md)**
- **[云原生多环境部署全景指南 (Deployment Guide)](file:///home/charles/code/sfwork/PrivShield/deploy/README.md)**
- **[网关负载均衡与 P2C 调度 (Gateway Balancer)](file:///home/charles/code/sfwork/PrivShield/docs/gateway_balancer/design.md)**

---

## 开源许可证 (License)

本项目采用 Apache 2.0 开源许可证。