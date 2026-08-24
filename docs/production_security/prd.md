# 生产安全加固产品设计 PRD

> Scope: P0 — REST/gRPC TLS（含 mTLS 可选）、认证鉴权、速率限制。


## 目录 (Table of Contents)

- [1. 概述](#1-概述)
- [2. 设计目标](#2-设计目标)
- [3. 用户故事](#3-用户故事)
- [4. 功能需求](#4-功能需求)
  - [4.1 TLS](#41-tls)
  - [4.2 认证与鉴权](#42-认证与鉴权)
  - [4.3 速率限制](#43-速率限制)
  - [4.4 健康检查](#44-健康检查)
- [5. 非功能需求](#5-非功能需求)
- [6. 验收标准](#6-验收标准)
- [7. 非目标](#7-非目标)

---

## 1. 概述

本文档定义 `PrivShield` 生产安全模块的产品需求与验收标准。该模块为 REST 与 gRPC 双协议提供可选的传输安全、身份认证、权限鉴权与速率限制能力，使其能够部署于多租户、跨域或半开放的生产环境。

## 2. 设计目标

- 为 REST/gRPC 提供可选的服务器端 TLS，gRPC 额外支持可选的 mTLS。
- 区分内部服务（高信任）与外部服务（低信任）两类身份，按最小权限原则控制接口访问。
- 基于调用者身份与接口路径/方法进行速率限制，防止预算爆破、模型推理资源耗尽与 DDoS。
- 所有安全能力默认关闭，通过环境变量显式开启，保证向后兼容。

## 3. 用户故事

| 角色 | 故事 |
|---|---|
| 平台运维 | 通过 TLS 加密 REST/gRPC 流量，避免隐私原语请求在链路上被窃听或篡改。 |
| SecretPad 后端（内部服务） | 使用内部 API Key 或 mTLS 调用 agent，并拥有全部隐私原语权限。 |
| 数据门户（外部服务） | 仅获得脱敏、分类等只读/低敏能力，不能调用差分隐私消耗预算或 K-匿名。 |
| SRE | `/health` 保持匿名可访问，便于 Kubernetes 探针和负载均衡健康检查。 |
| 安全团队 | 对缺失/无效凭证返回 401/`UNAUTHENTICATED`，对越权返回 403/`PERMISSION_DENIED`，对超速返回 429/`RESOURCE_EXHAUSTED`。 |

## 4. 功能需求

### 4.1 TLS

| ID | 需求 |
|---|---|
| FR-TLS-1 | 当 `PRIVACY_TLS_ENABLED=true` 时，REST 与 gRPC 均只监听 TLS 端口。 |
| FR-TLS-2 | 支持通过环境变量指定服务器证书、私钥、CA 证书、私钥口令。 |
| FR-TLS-3 | 支持 `none`/`optional`/`require` 三种客户端认证模式。 |
| FR-TLS-4 | gRPC 在 `require` 模式下通过 mTLS 提取客户端证书身份用于内部服务鉴权。 |
| FR-TLS-5 | mTLS 认证采用两层校验：传输层 CA 信任链校验 + 应用层 CN 白名单匹配，仅 CN 命中 `PRIVACY_AUTH_MTLS_ALLOWED_CNS` 的客户端才授予内部身份。 |

### 4.2 认证与鉴权

| ID | 需求 |
|---|---|
| FR-AUTH-1 | 当 `PRIVACY_AUTH_ENABLED=true` 时，除健康检查外所有接口必须携带有效凭证。 |
| FR-AUTH-2 | 支持 internal（通配权限）与 external（受限 scope）两类服务身份。 |
| FR-AUTH-3 | REST 外部服务使用 `Authorization: Bearer <token>`；REST 内部服务使用内部 API Key。 |
| FR-AUTH-4 | gRPC 外部服务通过 metadata `authorization` 携带 token；gRPC 内部服务优先使用 mTLS 身份（CN 命中白名单），也允许使用内部 API Key。 |
| FR-AUTH-5 | mTLS 认证默认关闭（fail-closed）：必须显式设置 `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true` 且配置 CN 白名单才会授予内部身份。 |
| FR-AUTH-6 | 鉴权失败返回明确的 HTTP/gRPC 状态码与错误信息，不泄露内部实现细节。 |

### 4.3 速率限制

| ID | 需求 |
|---|---|
| FR-RL-1 | 当 `PRIVACY_RATE_LIMIT_ENABLED=true` 时，按调用者身份 + 接口做限流。 |
| FR-RL-2 | 支持默认 RPS/Burst，并支持按接口单独覆盖。 |
| FR-RL-3 | REST 超速返回 `429 Too Many Requests`；gRPC 超速返回 `RESOURCE_EXHAUSTED`。 |
| FR-RL-4 | 健康检查接口默认不受限速影响。 |
| FR-RL-5 | 可选 Redis 后端，用于多副本共享限流计数器；未配置时使用进程内存。 |

### 4.4 健康检查

| ID | 需求 |
|---|---|
| FR-HEALTH-1 | `/health` 与 `Health` RPC 默认不认证、不限速。 |
| FR-HEALTH-2 | 可通过 `PRIVACY_HEALTH_NO_AUTH=false` 与 `PRIVACY_HEALTH_NO_RATE_LIMIT=false` 关闭豁免。 |

### 4.5 全栈防 DDoS 与系统容量保护 (DDoS & Capacity Protection)

| ID | 需求 |
|---|---|
| FR-DDOS-1 | **慢速连接防护**：所有 Go/Python HTTP 服务显式配置 `ReadHeaderTimeout ≤ 5s` 与 `MaxHeaderBytes ≤ 1MB`，拦截 Slowloris 与慢速 Header 挂起。 |
| FR-DDOS-2 | **大包 DoS 拦截**：全平台配置 `MaxBodySize`（32MB/64MB），超出上限时立即切断请求并返回 `413 Payload Too Large`。 |
| FR-DDOS-3 | **IP 令牌桶限流**：Go 共享基础库提供 `RateLimit(rps, burst)` 中间件，自动 GC 闲置 IP 桶，超限响应 `429 Too Many Requests` 与 `Retry-After`。 |
| FR-DDOS-4 | **并发容量硬顶**：提供 `MaxConcurrent(limit)` 信号量中间件，突发过载快速返回 `503 Service Unavailable` 保护协程池。 |
| FR-DDOS-5 | **云原生 Ingress 防护**：Helm 与生产模板预置 Nginx Ingress 连接限制（50 连接/IP）与速率限制（100 RPS/IP）。 |

### 4.6 数据源与存储沙箱安全 (Data Source & Storage Security)

| ID | 需求 |
|---|---|
| FR-DATA-1 | **路径遍历 (LFI) 沙箱防护**：`datasource-mgr` CSV 加载强制 `.csv` 白名单，提取纯文件名并在目录沙箱内加载，且限制最大读取 50,000 行。 |
| FR-DATA-2 | **异常信息脱敏**：`pkg/middleware.Recovery` 捕获 Panic 并向客户端返回安全脱敏响应，堆栈仅留存于内部结构化日志。 |
| FR-DATA-3 | **SQL 分页边界安全**：SQLite 存储层使用 `ParsePagination` 强制约束 `Limit` 在 1~10000 且 `Offset ≥ 0`。 |

## 5. 非功能需求

| 维度 | 要求 |
|---|---|
| 向后兼容 | 所有安全开关默认关闭；现有本地启动命令与测试集无需修改即可通过。 |
| 性能 | 认证与限流处理耗时 < 1ms/P99（内存模式）。 |
| 可观测 | 关键拒绝事件（认证失败、越权、超速、413/503 拦截）打印结构化日志。 |
| 可配置 | 全部行为通过环境变量配置，无需改动代码即可适配不同环境。 |
| 可测试 | 提供自签名证书生成工具/测试夹具，单元测试覆盖 TLS/mTLS/Auth/RateLimit/DDoS/LFI。 |

## 6. 验收标准

- [x] 编写 `docs/production_security/prd.md`、`design.md`、`ops.md`、`security_requirements.md`。
- [x] 新增 `engine/security/` 模块，包含 config/tls/identity/auth/ratelimit。
- [x] REST/gRPC 在开启 TLS 后仅接受 HTTPS/gRPCs 连接。
- [x] mTLS `require` 模式拒绝无客户端证书的调用。
- [x] mTLS CN 白名单：命中白名单的 CN 获得内部身份（`["*"]` scope），未命中的被拒绝。
- [x] mTLS 认证默认关闭（fail-closed）：未显式启用时即使证书合法也不授予身份。
- [x] 内部 API Key 可访问所有接口；外部 API Key 越权被拦截。
- [x] 超速调用 REST 返回 429，gRPC 返回 `RESOURCE_EXHAUSTED`。
- [x] `/health` 与 `Health` 默认保持匿名、不限速。
- [x] 全微服务配置 Slowloris 5s 超时与 1MB 请求头限制。
- [x] 引入 `MaxBodySize` 大包防御（413）与 `MaxConcurrent` 并发熔断（503）。
- [x] 实施 CSV Loader 路径穿越沙箱与 SQLite 分页上下限夹紧。
- [x] 所有现有测试在默认配置下通过；新增安全与 DDoS 测试 100% 通过。

## 7. 非目标与演进规划

- 本次不实现复杂 OAuth/OIDC 认证服务，采用静态 API Key + mTLS 客户端证书 + 静态 Scope 映射；
- 硬件安全模块 (HSM) 与 KMS 信封加密密钥自动轮换作为后续演进目标。