# PrivShield 生产安全模块索引

> **版本**：v16.0.0  
> **适用范围**：`PrivShield` 核心算力引擎（`engine`）、企业级中台微服务群（`service-hub` / `datasource-mgr` / `audit-log`）、控制台与双 BFF 体系（`bff-go` / `app-lz`）。  
> **定位**：系统索引全平台生产安全模块的 SDLC 文档，涵盖 TLS 1.3/mTLS 传输安全、CN 白名单动态热重载、API Key 恒定时间认证、全栈 9 层中间件纵深防 DDoS、SM4-GCM 快照信封加密与 9 要素密码学哈希链防篡改存证。

---

## 目录

- [1. 文档导航](#1-文档导航)
- [2. 全栈安全能力速查表](#2-全栈安全能力速查表)
- [3. 快速运行示例](#3-快速运行示例)

---

## 1. 文档导航

| 文档 | 描述 | 核心章节 |
|---|---|---|
| [prd.md](./prd.md) | 生产安全产品需求文档 (PRD) | 业务背景、功能与非功能需求、验收标准 |
| [design.md](./design.md) | 生产安全架构设计文档 (Design) | 架构概览、TLS/mTLS 设计、CN 白名单 5 秒热重载、9 层中间件与防 DDoS、认证鉴权、速率限制 |
| [security_requirements.md](./security_requirements.md) | 安全与编码规范 | 威胁建模、安全编码要求、CVE 防御、漏洞修复矩阵 |
| [api_reference.md](./api_reference.md) | 安全配置与 API 参考 | 环境变量矩阵、Python SDK、Go 安全库接口 |
| [ops.md](./ops.md) | 生产安全运维手册 (Ops) | 环境变量参考、证书生成、启动示例、排错指南 |
| [examples.md](./examples.md) | 生产安全加固使用示例 | 典型场景配置、REST/gRPC 客户端调用代码 |
| [testing.md](./testing.md) | 生产安全加固测试文档 | 测试策略、单测/集成测试用例、验收检查清单 |

---

## 2. 全栈安全能力速查表

| 能力分层 | 核心机制 / 开关 | 默认值 | 安全防护说明 |
|---|---|---|---|
| **传输安全 (TLS)** | `PRIVACY_TLS_ENABLED=true` | `false` | REST/gRPC 强制启用 TLS 1.3 加密传输 |
| **客户端证书 (mTLS)** | `PRIVACY_TLS_CLIENT_AUTH=require` | `none` | 强制双向 TLS 校验客户端身份证书 |
| **mTLS CN 白名单** | `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true` | `false` | gRPC 客户端证书 CN 白名单认证与 per-CN Scope 授权（支持 5 秒热重载） |
| **API Key 认证** | `PRIVACY_AUTH_ENABLED=true` / `API_KEY` | `false` / `""` | 静态 Bearer Token 恒定时间防时序攻击鉴权 |
| **静态快照加密** | `AUDIT_LOG_ENCRYPTION_KEY` | `""` (明文) | SM4-GCM 信封加密脱敏快照落盘（`enc:v1:` 前缀） |
| **不可篡改存证** | 9 要素哈希链 | 内建生效 | 基于连续 SHA-256 区块链式哈希链，支持秒级在线核验 |
| **租户/身份限流** | `PRIVACY_RATE_LIMIT_ENABLED=true` | `false` | Python 核心引擎基于调用者身份 + 接口的滑动窗口限流 |
| **IP 令牌桶防刷** | `pkg/middleware.RateLimit(200, 400)` | 自动注入 | Go 微服务基于客户端 IP 令牌桶限流，超限返回 429 与 Retry-After |
| **慢速连接防护** | `ReadHeaderTimeout: 5s` / `ReadTimeout: 30s` | 已配置 | 协议级防御 Slowloris 与慢速 POST 挂起连接攻击 |
| **大包 DoS 拦截** | `pkg/middleware.MaxBodySize(32MB/64MB)` | 自动注入 | 读取超限时切断并响应 413 Payload Too Large，保护内存 |
| **并发容量硬顶** | `pkg/middleware.MaxConcurrent(1000)` | 可选配置 | 超载时快速失败响应 503 Service Unavailable 保护协程池 |
| **路径遍历沙箱** | `datasource-mgr` CSV Loader 沙箱 | 强制启用 | 限制 .csv 扩展名、提取 BaseName、校验目录白名单，封死 LFI |
| **异常信息脱敏** | `pkg/middleware.Recovery` | 自动注入 | Panic 堆栈收敛至服务端内部日志，对外统一响应脱敏 JSON |
| **SQL 边界加固** | `pkg/store/sqlite` 分页夹紧 | 强制启用 | `Limit` 限制 1~10000，`Offset >= 0`，杜绝异常查询 |

---

## 3. 快速运行示例

```bash
cd /path/to/PrivShield
source .venv/bin/activate
PYTHONPATH=. python docs/production_security/examples/security_usage.py
```