# datasource-mgr 可靠性能力说明

> 模拟数据源服务（datasource-mgr）的崩溃恢复、自动重试、完整性校验与备份能力详解。

---

## 1. 能力总览

| 能力维度 | 支持状态 | 实现方式 |
|---|---|---|
| HTTP TLS/mTLS 双向认证 | ✅ | 与 gRPC 共享证书配置，TLS 1.3 强制最低版本，支持 require/verify/request 客户端认证模式 |
| gRPC TLS/mTLS 双向认证 | ✅ | TLS 1.3 + 多客户端认证模式 + 公钥固定（SPKI Pinning） |
| 崩溃恢复 | ⚪ 不适用 | 无状态服务，无持久化任务状态需要恢复 |
| 自动重试 | ⚪ 不适用 | 无状态服务，请求级处理由上游调用方负责 |
| SQLite 完整性校验 | ⚪ 不适用 | 无持久化数据库 |
| 数据库备份 | ⚪ 不适用 | 无持久化数据 |
| 优雅停机 | ✅ | SIGINT/SIGTERM → gRPC GracefulStop → HTTP Shutdown(5s) |
| Panic 恢复 | ✅ | Gin Recovery 中间件自动捕获 panic，防止进程退出 |
| 连接保活 | ✅ | HTTP Keep-Alive + gRPC HTTP/2 多路复用 |

---

## 2. HTTP/gRPC TLS/mTLS 双向认证

### 2.1 能力概述

datasource-mgr 支持 HTTP 和 gRPC 双协议的 TLS/mTLS 双向认证，与 service-hub 共享 `pkg/tlsutil` 工具库：

| 特性 | 实现 |
|---|---|
| 最低 TLS 版本 | TLS 1.3 强制最低版本 |
| HTTP mTLS | 与 gRPC 共享证书配置，支持 require/verify/request 客户端认证模式 |
| gRPC mTLS | TLS 1.3 + 多客户端认证模式 + 公钥固定（SPKI Pinning） |
| 共享工具库 | `pkg/tlsutil.BuildServerTLSConfig()` |

### 2.2 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DATASOURCE_MGR_TLS_ENABLED` | `false` | 是否开启 TLS/mTLS |
| `DATASOURCE_MGR_TLS_CERT_FILE` | — | 服务端证书文件路径 |
| `DATASOURCE_MGR_TLS_KEY_FILE` | — | 服务端私钥文件路径 |
| `DATASOURCE_MGR_TLS_CA_FILE` | — | 客户端 CA 证书文件路径 |
| `DATASOURCE_MGR_TLS_CLIENT_AUTH` | — | 客户端认证模式（require/verify/request） |
| `DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE` | — | 客户端公钥固定文件路径 |

---

## 3. 服务特性说明

### 3.1 无状态设计

datasource-mgr 是一个**纯无状态**的模拟数据源服务，其核心特征：

- **无持久化存储**：所有数据（医保 yibao、康养 kangyang 等）均为内存中实时生成的高保真模拟数据；
- **无任务队列**：不维护任何异步任务状态，每次请求独立处理；
- **无崩溃恢复需求**：服务重启后无需恢复任何中间状态，天然具备崩溃恢复能力；
- **无备份需求**：不存在需要持久化备份的业务数据。

### 3.2 可靠性保障重点

对于无状态服务，可靠性的核心在于：
1. **优雅停机**：确保在途请求不被中断；
2. **Panic 隔离**：单请求崩溃不影响整体服务；
3. **快速启动**：服务能在秒级完成启动并恢复服务能力。

---

## 4. 优雅停机（Graceful Shutdown）

### 4.1 停机流程

```
SIGINT/SIGTERM → gRPC GracefulStop → HTTP Shutdown(5s) → 进程退出
```

**详细步骤：**

1. **信号捕获**：监听 `SIGINT`（Ctrl+C）和 `SIGTERM`（K8s Pod 终止）；
2. **gRPC 优雅停机**：
   - `serviceImpl.Shutdown()`：发送内部 context 取消信号，等待后台异步任务完成；
   - `grpcServer.GracefulStop()`：停止接受新连接，等待在途 RPC 请求处理完毕（无超时上限）；
3. **HTTP 优雅停机**：
   - `httpSrv.Shutdown(ctx)`：停止接收新请求，等待现有请求完成；
   - 5 秒硬上限超时，超时后强制断开连接。

### 4.2 超时保护

| 组件 | 超时 | 行为 |
|---|---|---|
| HTTP Server | 5 秒 | 超时后强制断开连接 |
| gRPC Server | 无上限 | 等待所有 in-flight RPC 完成 |

---

## 5. Panic 恢复（Panic Recovery）

### 5.1 Gin Recovery 中间件

datasource-mgr 使用 `gin.New()` 创建纯净引擎，并手动装配 Recovery 中间件：

```go
r.Use(middleware.Recovery(s.logger, "datasource-mgr"))
```

**工作机制：**
- 捕获任何 HTTP handler 中发生的 panic；
- 记录 panic 堆栈信息到结构化日志；
- 返回 HTTP 500 错误响应给客户端；
- **进程不退出**，其他请求继续正常处理。

### 5.2 设计原则

- **单请求隔离**：一个请求的 panic 不会影响其他请求或导致进程崩溃；
- **结构化日志**：panic 信息通过 slog 结构化输出，便于日志采集系统（Loki/ELK）解析；
- **安全响应**：不向客户端暴露内部错误详情，仅返回通用 500 错误。

---

## 6. 网络安全防护

### 6.1 HTTP 超时配置

| 参数 | 值 | 说明 |
|---|---|---|
| `ReadHeaderTimeout` | 5 秒 | 防御 Slowloris 慢速连接攻击 |
| `ReadTimeout` | 30 秒 | 限制请求体读取最大时间 |
| `WriteTimeout` | 60 秒 | 限制响应写入最大时间 |
| `IdleTimeout` | 120 秒 | Keep-Alive 空闲连接保活上限 |
| `MaxHeaderBytes` | 1 MiB | 单请求 Header 最大字节限制 |

### 6.2 gRPC 消息限制

gRPC 服务端配置 64 MiB 消息收发上限（通过 `grpc.MaxRecvMsgSize` / `grpc.MaxSendMsgSize`），防止超大消息导致内存溢出。

---

## 7. 上游依赖的容错设计

由于 datasource-mgr 本身无状态，其可靠性保障主要依赖上游调用方（如 service-hub）的容错机制：

| 上游容错机制 | 说明 |
|---|---|
| service-hub 自动重试 | datasource-mgr 临时不可达时，service-hub 自动重试失败任务（最多 3 次） |
| Gateway 故障转移 | 网关层最多重试 3 次，并触发被动健康下线（5s 冷却） |
| Gateway 熔断器 | 连续失败 5 次后熔断，30 秒后半开探测恢复 |

---

## 8. 运维建议

### 8.1 部署建议

- 由于无状态特性，建议部署 **≥ 2 个副本** 以实现高可用；
- 配合 K8s 的 `readinessProbe` / `livenessProbe` 使用 `/health` 端点；
- 利用无状态特性实现零停机滚动更新（Rolling Update）。

### 8.2 故障排查

| 现象 | 可能原因 | 排查方法 |
|---|---|---|
| 频繁 panic 日志 | 请求数据格式异常 | 检查结构化日志中的 panic 堆栈 |
| HTTP 500 响应增多 | 模拟数据生成异常 | 检查请求参数合法性 |
| 连接超时 | 下游网络问题 | 检查 service-hub → datasource-mgr 网络连通性 |
