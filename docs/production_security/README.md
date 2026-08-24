# 生产安全加固与防御体系文档索引

本目录包含 `PrivShield` 全平台（Python 核心隐私算力引擎、Go 中台微服务群与控制台 BFF）生产安全模块的全套 SDLC 文档，覆盖 REST/gRPC 的 TLS/mTLS 传输加密、认证鉴权、速率限制、全栈纵深防 DDoS（Slowloris/Payload/Flood/Concurrency）、路径穿越沙箱防御、错误脱敏与存储安全。

## 目录 (Table of Contents)

- [文档清单](#文档清单)
- [快速开始](#快速开始)
- [运行示例](#运行示例)
- [安全开关速查](#安全开关速查)

---

## 文档清单

| 文档 | 说明 | 目标读者 |
|---|---|---|
| [security_requirements.md](./security_requirements.md) | 技术栈常见漏洞总结、安全编码规范与全平台审计修复清单 | 全体开发人员、安全审计员 |
| [prd.md](./prd.md) | 生产安全与纵深防 DDoS 产品需求文档与验收标准 | 产品经理、项目经理 |
| [design.md](./design.md) | 技术架构、威胁模型、mTLS 白名单认证、全栈防 DDoS 与实现细节 | 安全架构师、后端开发 |
| [api_reference.md](./api_reference.md) | 环境变量、配置项与 TLS/Auth/RateLimit/DDoS 接口参考 | 接入开发者、SRE |
| [examples.md](./examples.md) | TLS、API Key、速率限制与安全中间件的配置示例 | 接入开发者 |
| [examples/security_usage.py](./examples/security_usage.py) | 可运行的完整示例脚本 | 接入开发者 |
| [testing.md](./testing.md) | 安全测试策略、DDoS 压测与测试代码示例 | QA、测试开发 |
| [ops.md](./ops.md) | 运维手册、生产安全加固参数建议与故障排查 | SRE、运维 |

## 快速开始

1. 阅读 [prd.md](./prd.md) 了解全栈安全能力范围与验收标准。
2. 阅读 [design.md](./design.md) 掌握 TLS/mTLS、认证鉴权、防 DDoS 架构与 Go 共享安全栈。
3. 查看 [examples.md](./examples.md) 或运行 [examples/security_usage.py](./examples/security_usage.py) 快速上手。
4. 开发/部署时参考 [api_reference.md](./api_reference.md) 与 [ops.md](./ops.md)。
5. 编写安全测试参考 [testing.md](./testing.md)。

## 运行示例

```bash
cd /path/to/PrivShield
source .venv/bin/activate
PYTHONPATH=. python docs/production_security/examples/security_usage.py
```

## 安全能力与开关速查

| 能力分层 | 核心机制 / 开关 | 默认值 | 安全防护说明 |
|---|---|---|---|
| **传输安全 (TLS)** | `PRIVACY_TLS_ENABLED=true` | `false` | REST/gRPC 强制启用 TLS 1.3 加密传输 |
| **客户端证书 (mTLS)** | `PRIVACY_TLS_CLIENT_AUTH=require` | `none` | 强制双向 TLS 校验客户端身份证书 |
| **mTLS CN 白名单** | `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true` | `false` | gRPC 客户端证书 CN 白名单认证与 per-CN Scope 授权 |
| **API Key 认证** | `PRIVACY_AUTH_ENABLED=true` / `API_KEY` | `false` / `""` | 静态 Bearer Token 恒定时间防时序攻击鉴权 |
| **租户/身份限流** | `PRIVACY_RATE_LIMIT_ENABLED=true` | `false` | Python 核心引擎基于调用者身份 + 接口的滑动窗口限流 |
| **IP 令牌桶防刷** | `pkg/middleware.RateLimit(200, 400)` | 自动注入 | Go 微服务基于客户端 IP 令牌桶限流，超限返回 429 与 Retry-After |
| **慢速连接防护** | `ReadHeaderTimeout: 5s` / `ReadTimeout: 30s` | 已配置 | 协议级防御 Slowloris 与慢速 POST 挂起连接攻击 |
| **大包 DoS 拦截** | `pkg/middleware.MaxBodySize(32MB/64MB)` | 自动注入 | 读取超限时切断并响应 413 Payload Too Large，保护内存 |
| **并发容量硬顶** | `pkg/middleware.MaxConcurrent(1000)` | 可选配置 | 超载时快速失败响应 503 Service Unavailable 保护协程池 |
| **路径遍历沙箱** | `datasource-mgr` CSV Loader 沙箱 | 强制启用 | 限制 .csv 扩展名、提取 BaseName、校验目录白名单，封死 LFI |
| **异常信息脱敏** | `pkg/middleware.Recovery` | 自动注入 | Panic 堆栈收敛至服务端内部日志，对外统一响应脱敏 JSON |
| **SQL 边界加固** | `pkg/store/sqlite` 分页夹紧 | 强制启用 | `Limit` 限制 1~10000，`Offset >= 0`，杜绝异常查询 |

> 核心开关默认保持平滑兼容（可通过环境变量按需开启）；关键输入沙箱与防护中间件均已默认内建生效。