# service-hub 可靠性能力说明

> 数据服务调度中枢（service-hub）的崩溃恢复、自动重试、完整性校验与备份能力详解。

---

## 1. 能力总览

| 能力维度 | 支持状态 | 实现方式 |
|---|---|---|
| HTTP TLS/mTLS 双向认证 | ✅ | 与 gRPC 共享证书配置，TLS 1.3 强制最低版本，支持 require/verify/request 客户端认证模式 |
| gRPC TLS/mTLS 双向认证 | ✅ | TLS 1.3 + 多客户端认证模式 + 公钥固定（SPKI Pinning） |
| 崩溃恢复（孤立任务回收） | ✅ | 启动时区分 pending（保留队列）/ running（标记 failed）任务 |
| 失败任务自动重试 | ✅ | 启动时 + 周期性后台重试，结构化 RetryCount 字段，指数退避延迟 |
| Prometheus 可观测性 | ✅ | `orphaned_tasks_recovered_total`、`tasks_retried_total` 指标 |
| SQLite 完整性校验 | ✅ | `PRAGMA integrity_check` 启动时阻断损坏数据库 |
| 数据库备份 | ✅ | 支持全量/增量备份、`--verify` 恢复验证模式、自动过期清理 |
| 优雅停机 | ✅ | SIGINT/SIGTERM → 停止后台重试 → 异步任务取消 → gRPC/HTTP 顺序关闭 |
| 存储持久化 | ✅ | SQLite WAL 模式，支持内存回退 |

---

## 2. 崩溃恢复（Crash Recovery）

### 2.1 问题场景

当服务突然崩溃（`kill -9`、OOM Kill、断电）时，优雅停机代码不会执行，导致：
- **running 状态任务**：正在执行的任务卡在 "running" 状态，永远不会完成；
- **pending 状态任务**：已接收但未执行的任务永远停留在 "pending" 队列。

### 2.2 恢复机制

服务启动时，`recoverOrphanedTasks()` 函数自动执行以下操作：

```
启动 → 初始化 SQLite → 扫描 running 任务 → 标记为 failed
                                 → 扫描 pending 任务 → 保留在队列中 → 记录日志 + Prometheus 指标
```

**处理流程：**

1. **扫描 running 任务**：调用 `taskStore.List(TaskFilter{Status: "running"})` 获取所有运行中任务（上限 10000 条）；
2. **标记为 failed**：设置 `Status = "failed"`，`Error = "server crashed or restarted (recovered on startup)"`，记录 `CompletedAt` 和 `DurationMs`；
3. **扫描 pending 任务**：获取所有 pending 任务，**直接保留在队列中**（它们尚未执行，无需标记失败）；
4. **日志输出**：恢复任务数量 > 0 时输出 WARN 级别日志，包含 running/pending 分类计数；
5. **Prometheus 指标**：通过 `orphaned_tasks_recovered_total{type="running|pending"}` 记录恢复数量。

### 2.3 代码位置

| 文件 | 函数 | 说明 |
|---|---|---|
| `services/service-hub/cmd/server/main.go` | `recoverOrphanedTasks()` | 崩溃恢复主逻辑 |
| `services/service-hub/cmd/server/main.go:110` | `main()` 第 3.5 步 | 启动流程中的调用位置 |

---

## 3. 失败任务自动重试（Automatic Task Retry）

### 3.1 重试策略

服务启动时 + **周期性后台循环**（每 60 秒），`retryFailedTasks()` 自动扫描因临时错误而失败的任务并重新排队：

**可重试的错误类型：**

| 错误模式 | 匹配关键字 | 说明 |
|---|---|---|
| 网络超时 | `timeout` | 下游服务响应超时 |
| 连接拒绝 | `connection refused` | 下游服务未启动或端口未监听 |
| 临时故障 | `temporary failure` | DNS 解析失败等临时错误 |
| 网络不可达 | `network unreachable` | 路由不可达 |
| 上下文超时 | `context deadline exceeded` | gRPC 上下文超时 |
| 崩溃恢复任务 | `server crashed or restarted` | 崩溃恢复标记的任务 |

### 3.2 重试限制

- **最大重试次数**：3 次（通过结构化 `RetryCount` 字段精确跟踪，替代脆弱的字符串匹配）；
- **指数退避延迟**：`5s × 2^RetryCount`（即 5s → 10s → 20s），通过 `RetryAfter` 字段控制最早重试时间；
- **超限处理**：超过最大重试次数的任务保持 `failed` 状态，输出 WARN 日志，Prometheus 指标 `tasks_retried_total{result="exhausted"}` 递增；
- **状态重置**：重试时重置 `Status = "pending"`、`Stage = "queued"`，清空 `StartedAt`/`CompletedAt`/`DurationMs`；
- **周期性后台重试**：`periodicRetryLoop()` 协程每 60 秒扫描一次，解决“运行时失败的任务必须等到下次重启”的问题；
- **停机清理**：优雅停机时通过 `retryCancel()` 取消后台重试协程。

### 3.3 代码位置

| 文件 | 函数 | 说明 |
|---|---|---|
| `services/service-hub/cmd/server/main.go` | `retryFailedTasks()` | 自动重试主逻辑 |
| `services/service-hub/cmd/server/main.go` | `isRetryableError()` | 可重试错误判定 |
| `services/service-hub/cmd/server/main.go` | `periodicRetryLoop()` | 周期性后台重试协程（每 60s） |
| `pkg/store/store.go` | `Task.RetryCount` / `Task.RetryAfter` | 结构化重试状态字段 |

---

## 4. SQLite 完整性校验（Integrity Check）

### 4.1 校验时机

服务启动早期（存储初始化之前），对 SQLite 数据库文件执行完整性校验：

```
启动 → ValidateIntegrity(dbPath) → 通过 → 继续初始化
                                   → 失败 → log.Fatalf() 阻止启动
```

### 4.2 校验实现

使用共享库 `pkg/store/sqlite/init.go` 中的 `ValidateIntegrity()` 函数：

```go
// 1. 打开数据库连接
// 2. 执行 PRAGMA integrity_check
// 3. 检查结果是否为 "ok"
// 4. 损坏时返回包含详细错误信息的 error
```

### 4.3 设计原则

- **Fail-Fast**：数据库损坏时立即终止启动，防止带病运行导致数据进一步损坏；
- **统一实现**：通过 `sqlite.ValidateIntegrity()` 共享函数避免各模块重复代码；
- **内存模式豁免**：`dbPath` 为空时跳过校验（内存存储无需持久化校验）。

---

## 5. 数据库备份（Backup）

### 5.1 备份脚本

通过 `scripts/prod/backup-sqlite-databases.sh` 统一备份，支持 service-hub、audit-log、datasource-mgr 三个服务的数据库。

### 5.2 备份模式

| 模式 | 参数 | 说明 |
|---|---|---|
| 全量备份 | `--full`（默认） | 每次完整备份所有配置的数据库 |
| 增量备份 | `--incremental` | 基于 SHA-256 哈希比对，仅备份发生变化的数据库 |
| 恢复验证 | `--verify` | 解压最新备份并执行 `PRAGMA integrity_check` 校验，确保备份可恢复 |

### 5.3 备份特性

- **在线备份**：使用 `sqlite3 .backup` 命令，不锁库、不影响在线服务；
- **压缩存储**：默认启用 gzip 压缩（`COMPRESS_ENABLED=true`）；
- **过期清理**：自动删除超过 `RETENTION_DAYS`（默认 7 天）的旧备份；
- **Cron 集成**：`--install-cron` 安装每天凌晨 2 点的定时任务；
- **哈希追踪**：增量备份通过 `.db_hashes` 文件记录上次备份的文件哈希。

### 5.4 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `BACKUP_DIR` | `/var/backups/privshield` | 备份存储目录 |
| `SERVICE_HUB_DB_PATH` | — | service-hub 数据库文件路径 |
| `RETENTION_DAYS` | `7` | 备份保留天数 |
| `COMPRESS_ENABLED` | `true` | 是否 gzip 压缩 |

---

## 6. 优雅停机（Graceful Shutdown）

### 6.1 停机流程

```
SIGINT/SIGTERM → 取消异步任务 → 停止 gRPC → 停止 HTTP（5s 超时）→ 进程退出
```

**详细步骤：**

1. **信号捕获**：监听 `SIGINT`（Ctrl+C）和 `SIGTERM`（K8s Pod 终止）；
2. **异步任务取消**：`serviceImpl.Shutdown()` 发送 context 取消信号，等待在途流水线任务完成；
3. **gRPC 优雅停机**：`grpcServer.GracefulStop()` 停止接受新连接，等待当前 RPC 完成；
4. **HTTP 优雅停机**：`httpSrv.Shutdown(ctx)` 停止接收新请求，等待现有请求完成（5 秒硬上限）；
5. **资源释放**：关闭数据库连接、释放端口。

### 6.2 超时保护

| 组件 | 超时 | 行为 |
|---|---|---|
| HTTP Server | 5 秒 | 超时后强制断开连接 |
| gRPC Server | 无上限 | 等待所有 in-flight RPC 完成 |
| 异步任务 | 无上限 | 通过 context 取消信号平滑等待 |

---

## 7. 存储可靠性

### 7.1 SQLite 配置

| PRAGMA | 值 | 说明 |
|---|---|---|
| `journal_mode` | `WAL` | Write-Ahead Logging，读写不互斥，崩溃安全 |
| `synchronous` | `NORMAL` | WAL 模式下的安全同步级别，性能与安全的平衡 |
| `busy_timeout` | `5000` | 遇锁等待 5 秒，避免短暂锁竞争报错 |
| `foreign_keys` | `ON` | 强制外键约束 |

### 7.2 连接池配置

| 参数 | 值 | 说明 |
|---|---|---|
| `MaxOpenConns` | 4 | 限制并发连接数，防止过度锁竞争 |
| `MaxIdleConns` | 2 | 保持适量空闲连接 |
| `ConnMaxLifetime` | 5 分钟 | 连接最大存活时间，防止长连接泄漏 |

### 7.3 存储回退

- **SQLite 模式**（`DBPath` 非空）：持久化存储，支持崩溃恢复与重启数据连续性；
- **内存模式**（`DBPath` 为空）：`memory.NewTaskStore()`，轻量无依赖，适用于测试与轻量场景，**重启后数据丢失**。

---

## 8. TLS/mTLS 双向认证（Transport Layer Security）

### 8.1 概述

service-hub 的 HTTP REST 和 gRPC 双协议均支持 TLS/mTLS 双向认证，共享同一套证书配置。TLS 核心实现已抽取到共享库 `pkg/tlsutil`，避免代码重复。

### 8.2 支持的能力

| 能力 | HTTP REST | gRPC |
|---|---|---|
| 服务端 TLS 加密 | ✅ | ✅ |
| TLS 1.3 强制最低版本 | ✅ | ✅ |
| mTLS 双向认证 | ✅ | ✅ |
| 客户端认证模式 | require / verify / request | require / verify / request |
| 公钥固定（SPKI Pinning） | ✅ | ✅ |

### 8.3 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_TLS_ENABLED` | `false` | 是否启用 TLS/mTLS（同时影响 HTTP 和 gRPC） |
| `SERVICE_HUB_TLS_CERT_FILE` | `""` | 服务端 X.509 证书 PEM 文件路径 |
| `SERVICE_HUB_TLS_KEY_FILE` | `""` | 服务端私钥 PEM 文件路径 |
| `SERVICE_HUB_TLS_CA_FILE` | `""` | 验证客户端身份的根 CA 证书路径（mTLS 必需） |
| `SERVICE_HUB_TLS_CLIENT_AUTH` | `""` | 客户端认证模式：`require`（强制双向）/ `verify`（可选）/ `request`（请求） |
| `SERVICE_HUB_TLS_PINNED_PUBKEY_FILE` | `""` | 固定的客户端公钥 PEM 路径（可选，防御 CA 劫持） |

### 8.4 客户端认证模式详解

| 模式 | 行为 | 适用场景 |
|---|---|---|
| `require` | 强制要求客户端提供有效证书，否则拒绝连接 | 零信任内网、服务间互信 |
| `verify` | 客户端提供证书时校验，不提供也放行 | 混合模式（部分客户端有证书） |
| `request` | 请求客户端证书但不强制 | 渐进式迁移 |

### 8.5 共享 TLS 工具库

`pkg/tlsutil` 提供统一的 TLS 配置构建函数，供 HTTP 和 gRPC 复用：

```go
// pkg/tlsutil.BuildServerTLSConfig 构建 *tls.Config
// 支持 TLS 1.3、mTLS 客户端认证、公钥固定
tlsConfig, err := tlsutil.BuildServerTLSConfig(&tlsutil.ServerTLSConfig{
    Enabled:    true,
    CertFile:   "/path/to/server.crt",
    KeyFile:    "/path/to/server.key",
    CAFile:     "/path/to/ca.crt",
    ClientAuth: "require",
})
```

### 8.6 测试覆盖

HTTP TLS/mTLS 集成测试位于 `internal/handlers/httptls_test.go`，覆盖：

| 测试用例 | 验证内容 |
|---|---|
| `TestHTTPServer_TLSDisabled` | 未启用 TLS 时 HTTP 正常工作 |
| `TestHTTPServer_TLSOnly` | 启用 TLS 但未要求客户端证书时 HTTPS 正常 |
| `TestHTTPServer_MTLSRequire/WithoutClientCert` | mTLS require 模式下无客户端证书被拒绝 |
| `TestHTTPServer_MTLSRequire/WithValidClientCert` | mTLS require 模式下有效客户端证书通过 |
| `TestHTTPServer_MTLSWithPinnedKey/WithMatchingClientCert` | 公钥固定匹配时通过 |
| `TestHTTPServer_MTLSWithPinnedKey/WithDifferentClientCert` | 公钥固定不匹配时拒绝 |
| `TestHTTPServer_MTLSVerifyMode` | verify 模式下客户端证书可选 |

---

## 9. 运维建议

### 9.1 生产部署检查清单

- [ ] 配置 `SERVICE_HUB_TLS_ENABLED=true` 启用双协议 TLS/mTLS；
- [ ] 配置 `SERVICE_HUB_TLS_CLIENT_AUTH=require` 启用双向认证；
- [ ] 配置 `DB_PATH` 启用 SQLite 持久化（避免内存模式数据丢失）；
- [ ] 配置 `SERVICE_HUB_DB_PATH` 环境变量并定期执行备份脚本；
- [ ] 使用 `--install-cron` 安装自动备份定时任务；
- [ ] 定期执行 `--verify` 验证备份可恢复性；
- [ ] 监控 Prometheus 指标 `orphaned_tasks_recovered_total` 和 `tasks_retried_total`；
- [ ] 监控启动日志中的 `recovered orphaned tasks` 告警，频繁出现说明服务不稳定；
- [ ] 监控 `retryable failed tasks` 日志，排查下游服务连通性问题。

### 9.2 故障排查

| 现象 | 可能原因 | 排查方法 |
|---|---|---|
| HTTPS 连接被拒绝 | mTLS 客户端未提供证书 | 检查客户端证书配置 |
| 公钥固定校验失败 | 客户端公钥与配置不匹配 | 检查 `SERVICE_HUB_TLS_PINNED_PUBKEY_FILE` |
| 启动时 integrity check failed | SQLite 文件损坏（断电/磁盘故障） | 检查磁盘健康，从备份恢复 |
| 大量 orphaned tasks | 频繁 OOM Kill 或 kill -9 | 检查内存使用，调整资源限制 |
| 重试任务持续失败 | 下游 Agent/datasource 不可达 | 检查网络连通性和服务健康状态 |
