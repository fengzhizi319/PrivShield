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
| Agent 客户端熔断器 | ✅ | 三态熔断（Closed→Open→HalfOpen），连续 5 次失败触发，30s 冷却后半开探测 |
| Agent 客户端指数退避重试 | ✅ | 最多 3 次重试，500ms 基础延迟 + 随机抖动，5xx/网络错误可重试、4xx 不重试 |
| 多层 Panic 恢复 | ✅ | HTTP 中间件 + gRPC 拦截器 + 异步任务协程三层 panic 安全网 |
| HTTP 服务器超时防护 | ✅ | ReadHeaderTimeout(5s)/ReadTimeout(30s)/WriteTimeout(60s)/MaxHeaderBytes(1MiB) |
| gRPC Keepalive 保活 | ✅ | MaxConnectionAge(2h)/Time(30s)/Timeout(10s) + EnforcementPolicy |
| 并发信号量限流 | ✅ | HTTP + gRPC 各 10 并发异步任务信号量 |
| Per-IP 令牌桶限流 | ✅ | 可配置 RPS/Burst，自动清理 10min 不活动 IP 桶，健康端点豁免 |
| 安全响应头 | ✅ | X-Content-Type-Options / X-Frame-Options / HSTS / Referrer-Policy / Permissions-Policy |
| 请求体大小限制 | ✅ | HTTP 中间件 32 MiB + Agent 响应体 64 MiB 双重防护 |
| 分布式链路追踪 | ✅ | X-Request-ID 中间件注入 → context 传播 → Agent 客户端自动透传 |
| Prometheus 可观测性 | ✅ | 7 项指标：HTTP/GitHub QPS+延迟、崩溃恢复、重试、熔断器状态 |
| SQLite 完整性校验 | ✅ | `PRAGMA integrity_check` 启动时阻断损坏数据库 |
| 数据库备份 | ✅ | 支持全量/增量备份、`--verify` 恢复验证模式、自动过期清理 |
| 数据保留清理 | ✅ | 每 6h 清理超过 RetentionDays 的终态任务，防止 SQLite 膨胀 |
| 优雅停机 | ✅ | SIGINT/SIGTERM → 停止后台协程 → 异步任务取消 → gRPC(30s 超时) → HTTP 顺序关闭 |
| 配置校验（Fail-Fast） | ✅ | TLS 启用时校验证书文件存在且可读，启动早期快速失败 |
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

### 2.3 核心代码

```go
// services/service-hub/cmd/server/main.go → recoverOrphanedTasks()

// 1. 扫描所有 "running" 状态的任务 → 标记为 failed（可能已部分执行）
runningTasks, _, _ := taskStore.List(store.TaskFilter{Status: "running", Limit: 10000})
for i := range runningTasks {
    runningTasks[i].Status = "failed"
    runningTasks[i].Error = "server crashed or restarted (recovered on startup)"
    now := time.Now()
    runningTasks[i].CompletedAt = &now
    runningTasks[i].DurationMs = now.Sub(runningTasks[i].CreatedAt).Milliseconds()
    _ = taskStore.Update(&runningTasks[i])
    mc.RecordOrphanedRecovery("running")  // Prometheus 指标
}

// 2. 扫描所有 "pending" 状态的任务 → 直接保留在队列中（尚未执行，无需标记失败）
pendingTasks, _, _ := taskStore.List(store.TaskFilter{Status: "pending", Limit: 10000})
for range pendingTasks {
    mc.RecordOrphanedRecovery("pending")
}
```

### 2.4 代码位置

| 文件 | 函数 | 说明 |
|---|---|---|
| `cmd/server/main.go` | `recoverOrphanedTasks()` | 崩溃恢复主逻辑 |
| `cmd/server/main.go:127` | `main()` 第 3.5 步 | 启动流程中的调用位置（在存储初始化之后、重试之前） |
| `pkg/store/store.go:16-31` | `Task` 结构体 | 任务领域模型，含 `RetryCount`/`RetryAfter` 字段 |

---

## 3. 失败任务自动重试（Automatic Task Retry）

### 3.1 重试策略

服务启动时 + **周期性后台循环**（每 60 秒），`retryFailedTasks()` 自动扫描因临时错误而失败的任务并重新排队：

重试扫描通过 `taskStore.List(store.TaskFilter{Status: "failed", Limit: 100})` 读取当前批次的失败任务。在 SQLite 模式下，这一步等价于按 `created_at DESC` 查询 `tasks` 表中 `status = 'failed'` 的记录；读取结果会还原为 `store.Task`，其中包括 `Error`、`RetryCount` 和 `RetryAfter`。函数先以小写化后的 `Error` 与可重试关键字匹配，再检查 `RetryCount < 3`，并确认 `RetryAfter` 为空或已到达。错误不可重试、重试次数耗尽，或退避时间尚未到达时，函数不会改写该任务记录，任务继续保持原有的 `failed` 状态。

当任务满足重试条件时，服务不会在 `retryFailedTasks()` 内直接重新执行六阶段流水线，而是将该任务恢复为待调度状态：`Status` 从 `failed` 改为 `pending`，`Stage` 改为 `queued`；`StartedAt` 和 `CompletedAt` 设为 `nil`，`DurationMs` 重置为 `0`；`Error` 改写为 `retrying (attempt N/3)`，以便查询接口和运维人员识别其当前处于第几次重试；`RetryCount` 加 `1`。同时根据重试前的计数计算下一次可重新扫描时间：`RetryAfter = now + 5s * 2^旧RetryCount`，因此前三次重试后的退避窗口依次为 5 秒、10 秒和 20 秒。任务在后续调度路径中被领取时，才会从 `pending` 写为 `running` 并重新执行。

每次状态改写都通过 `taskStore.Update(&task)` 落库；只有这个调用成功，代码才增加 `tasks_retried_total{result="queued"}` 指标并记录“queued for retry”日志。调用失败时仅记录错误日志，不计入已入队指标，因此内存对象中的临时修改不会被误报为持久化成功。SQLite `TaskStore.Update` 使用参数化的单条 `UPDATE tasks SET ... WHERE id = ?` 语句，按任务 ID 原子写入 `status`、`stage`、`started_at`、`completed_at`、`duration_ms`、`error`、`retry_count` 和 `retry_after`。空的开始或结束时间会以 SQL `NULL` 写入；非空时间以 RFC3339Nano 格式保存。`source`、`operation`、`priority`、`created_at` 与 `payload_json` 在本次状态更新中保持不变。

SQLite 在启动时启用 WAL、`busy_timeout=5000` 和有限连接池，以支持重试后台协程与查询请求并发访问；其 `status`、`retry_after` 索引分别支持按失败状态扫描和按退避时间检索。未配置数据库路径时，`TaskStore` 会回退为进程内存实现，状态改写接口相同，但服务重启后不会保留重试进度。

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

### 3.3 核心代码

```go
// services/service-hub/cmd/server/main.go → retryFailedTasks()

const maxRetryCount = 3

failedTasks, _, _ := taskStore.List(store.TaskFilter{Status: "failed", Limit: 100})
for i := range failedTasks {
    if !isRetryableError(failedTasks[i].Error) { continue }
    if failedTasks[i].RetryCount >= maxRetryCount {
        mc.RecordTaskRetry("exhausted")
        continue
    }
    // 检查退避延迟
    if failedTasks[i].RetryAfter != nil && time.Now().Before(*failedTasks[i].RetryAfter) {
        continue
    }

    // 指数退避延迟：5s * 2^(retryCount)
    newRetryCount := failedTasks[i].RetryCount + 1
    backoffDuration := 5 * time.Second * time.Duration(1<<uint(failedTasks[i].RetryCount))
    retryAfter := time.Now().Add(backoffDuration)

    // 重置任务状态为 pending
    failedTasks[i].Status = "pending"
    failedTasks[i].Stage = "queued"
    failedTasks[i].RetryCount = newRetryCount
    failedTasks[i].RetryAfter = &retryAfter
    _ = taskStore.Update(&failedTasks[i])
    mc.RecordTaskRetry("queued")
}
```

**可重试错误判定逻辑：**

```go
// services/service-hub/cmd/server/main.go → isRetryableError()

func isRetryableError(errMsg string) bool {
    retryablePatterns := []string{
        "timeout",
        "connection refused",
        "temporary failure",
        "network unreachable",
        "context deadline exceeded",
        "server crashed or restarted",
    }
    errMsgLower := strings.ToLower(errMsg)
    for _, pattern := range retryablePatterns {
        if strings.Contains(errMsgLower, pattern) {
            return true
        }
    }
    return false
}
```

**周期性后台重试协程：**

```go
// services/service-hub/cmd/server/main.go → periodicRetryLoop()

retryCtx, retryCancel := context.WithCancel(context.Background())
go periodicRetryLoop(retryCtx, taskStore, mc, logger, 60*time.Second)

func periodicRetryLoop(ctx context.Context, ...) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return  // 优雅停机时退出
        case <-ticker.C:
            retryFailedTasks(taskStore, mc, logger)
        }
    }
}
```

### 3.4 任务状态持久化保证与性能评估

任务状态以 `pkg/store.TaskStore` 作为唯一写入接口。生产环境必须配置非空的 `DB_PATH`，使 service-hub 初始化 SQLite `TaskStore`；未配置时会使用进程内存存储，接口调用虽保持一致，但进程重启后所有任务和重试进度都会丢失，因此不能满足“任务状态必须保存在存储中”的要求。SQLite 模式下，调度请求只有在初始 `Save()` 已成功执行 `INSERT OR REPLACE INTO tasks (...)` 后才返回 HTTP `202 Accepted` 或相应的 gRPC 接收结果；因此一个已被调用方确认受理的任务，至少已有一条 `pending/queued` 状态记录。

流水线执行期间会按阶段更新同一任务 ID 的记录：进入每个阶段时写入 `Status = running`、当前 `Stage` 与 `StartedAt`；成功结束时写入 `Status = completed`、`Stage = done`、`CompletedAt` 和 `DurationMs`；下游调用失败、停机取消或 panic 恢复时写入 `Status = failed`、`Error`、`CompletedAt` 和 `DurationMs`。自动重试则把已持久化的失败记录恢复为 `pending/queued`，并同时保存 `RetryCount` 与 `RetryAfter`。每次成功调用 `TaskStore.Update()` 时，SQLite 均以任务 ID 为条件执行参数化的单条 `UPDATE`，单条语句中同时写入状态、阶段、时间、耗时、错误和重试字段，避免这些字段在一次状态迁移中分别提交而形成部分更新。

当前实现的保证边界需要明确区分：SQLite `Save()` 的失败会被请求入口返回给调用方，重试路径的 `Update()` 失败也会记录错误并且不会把任务计为已入队。HTTP 与 gRPC 流水线的每次阶段性状态写入现在都经由 `persistTask()` 检查；进入阶段的 `running` 状态未成功写入时，协程立即停止，不会继续调用数据源或 PrivShield Agent。这样可保证任意已执行的后续阶段，都以前一状态已经成功保存为前提。

终态写入同样会被检查：分类或脱敏失败、停机取消、panic 恢复和正常完成都会调用 `persistTask()`，并在 SQLite 返回错误时输出包含 `task_id`、迁移名称、目标 `status` 和 `stage` 的结构化错误日志。存储不可用时不可能同时保证“写入成功”和“额外写入一个失败状态”，因此该协程会停止，数据库记录保留在最后一次已确认的状态；服务重启后，崩溃恢复逻辑会将遗留的 `running` 任务标记为 `failed`，使其再次进入可审计、可重试的处理路径。该机制避免在状态未知时继续执行下游操作，而不是掩盖存储故障。

该保证由 HTTP 与 gRPC 两侧的故障注入测试验证：当首个 `running/ingest` 状态更新返回错误时，`processTask()` 只尝试这一次写入后便返回，不会推进到后续流水线阶段。

从写入次数看，一条无额外取数的成功任务通常产生 **1 次初始 INSERT + 6 次阶段 UPDATE + 1 次完成 UPDATE**，即 8 次数据库写入；若任务在第 $n$ 个阶段失败，则为 1 次 INSERT、进入第 $n$ 阶段前的 $n$ 次 UPDATE 和 1 次失败 UPDATE。`fetch` 阶段从数据源补充载荷时还会触发一次额外更新调用；任务重试每次再增加 1 次 UPDATE。该成本是以较高可恢复性换取额外 I/O 的刻意取舍：任务本身的每个阶段已有约 100 ms 的处理等待，且 HTTP/gRPC 两侧均通过容量为 10 的信号量限制并发异步任务，因此状态写入不会无限制地放大为并发写洪峰。

SQLite 使用 WAL 模式，使读取任务列表与单写入者并发时通常不互相阻塞；`busy_timeout=5000` 会在短暂写锁竞争时最多等待 5 秒，连接池限制为 4 个打开连接和 2 个空闲连接，防止过多连接放大锁竞争。WAL 不会改变 SQLite 单写入者的事实：在高并发、慢磁盘或大量重试同时发生时，写入延迟仍会成为吞吐瓶颈。`idx_tasks_status` 用于失败任务和运行中任务的状态筛选，`idx_tasks_retry_after` 用于退避时间检索；但当前重试扫描按 `status` 过滤并按 `created_at DESC` 排序，数据量较大时应结合实际查询计划评估是否需要复合索引。

性能评估应在部署目标相同的磁盘、文件系统和容器资源限制下执行，不能把开发机结果直接作为生产容量承诺。建议持续采集每次 `Save`/`Update` 的成功率、延迟分位数、SQLite busy/错误次数、队列中 `pending`/`failed` 数量以及重试量；压测时分别记录单任务写入延迟、10 个并发任务下的端到端延迟，以及超过 10 个提交并发时的排队时间。验收时应确认状态写入错误率为零、P99 写入延迟低于业务允许的阶段延迟预算，并在注入 SQLite 锁竞争或磁盘错误后验证写入失败能够被显式处理。当前项目尚未提供状态写入的基准测试或生产延迟基线，以上是基于代码路径和 SQLite 配置的成本模型，而非固定吞吐量承诺。

### 3.5 代码位置

| 文件 | 函数 | 说明 |
|---|---|---|
| `cmd/server/main.go` | `retryFailedTasks()` | 自动重试主逻辑 |
| `cmd/server/main.go` | `isRetryableError()` | 可重试错误判定 |
| `cmd/server/main.go` | `periodicRetryLoop()` | 周期性后台重试协程（每 60s） |
| `cmd/server/main.go:135` | `main()` 第 3.6 步 | 启动时重试调用位置 |
| `cmd/server/main.go:143` | `main()` 第 3.7 步 | 后台重试协程启动位置 |
| `pkg/store/store.go:29-30` | `Task.RetryCount` / `Task.RetryAfter` | 结构化重试状态字段 |

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
SIGINT/SIGTERM → 停止后台协程(retry+retention) → 异步任务取消 → gRPC GracefulStop(30s超时) → HTTP Shutdown(可配置超时) → 进程退出
```

**详细步骤：**

1. **信号捕获**：监听 `SIGINT`（Ctrl+C）和 `SIGTERM`（K8s Pod 终止）；
2. **后台协程停止**：`retryCancel()` + `retentionCancel()` 取消周期性重试和数据保留清理协程；
3. **异步任务取消**：`serviceImpl.Shutdown()` + `server.Shutdown()` 发送 context 取消信号，通过 `sync.WaitGroup` 等待在途流水线任务完成；
4. **gRPC 带超时优雅停机**：`GracefulStop()` 停止接受新连接，等待当前 RPC 完成（30 秒超时后回退为 `Stop()` 强制终止）；
5. **HTTP 优雅停机**：`httpSrv.Shutdown(ctx)` 停止接收新请求，等待现有请求完成（超时时间通过 `SERVICE_HUB_SHUTDOWN_TIMEOUT` 配置，默认 5 秒）；
6. **资源释放**：关闭数据库连接、释放端口。

### 6.2 核心代码

```go
// cmd/server/main.go → 优雅停机流程

// 1. 阻塞等待退出信号
sig := <-sigChan
logger.Info("shutting down service-hub servers...", "signal", sig.String())

// 2. 停止周期性后台协程
retryCancel()
retentionCancel()

// 3. 异步任务协程取消（通过 WaitGroup 等待收敛）
serviceImpl.Shutdown()  // gRPC 侧异步任务
server.Shutdown()       // HTTP 侧异步任务

// 4. gRPC 带超时的优雅停机（30s 超时后强制停止）
grpcDone := make(chan struct{})
go func() {
    grpcServer.GracefulStop()
    close(grpcDone)
}()
select {
case <-grpcDone:
    logger.Info("gRPC server stopped")
case <-time.After(30 * time.Second):
    logger.Warn("gRPC GracefulStop timed out after 30s, forcing stop")
    grpcServer.Stop()  // 强制停止
}

// 5. HTTP 带可配置超时的优雅停机
shutdownCtx, cancel := context.WithTimeout(
    context.Background(),
    time.Duration(cfg.ShutdownTimeout)*time.Second,  // 默认 5s
)
defer cancel()
httpSrv.Shutdown(shutdownCtx)
```

**异步任务级优雅停机（HTTP/gRPC 双侧相同模式）：**

```go
// internal/handlers/handlers.go 和 internal/grpcserver/server.go

type Server struct {
    ctx    context.Context    // 优雅停机广播上下文
    cancel context.CancelFunc // 触发取消的回调
    wg     sync.WaitGroup     // 跟踪在途协程
}

func (s *Server) Shutdown() {
    s.cancel()   // 广播取消信号
    s.wg.Wait()  // 等待所有协程完成
}
```

**流水线任务中检查停机信号：**

```go
// 每个流水线阶段都检查停机信号
for _, stage := range stages {
    select {
    case <-time.After(100 * time.Millisecond):  // 正常处理
    case <-s.ctx.Done():                         // 停机信号
        task.Status = "failed"
        task.Error = "server shutting down"
        _ = s.tasks.Update(task)
        return
    }
    // ... 执行各阶段逻辑 ...
}
```

### 6.3 超时保护

| 组件 | 超时 | 行为 |
|---|---|---|
| HTTP Server | `SERVICE_HUB_SHUTDOWN_TIMEOUT`（默认 5s） | 超时后强制断开连接 |
| gRPC Server | 30s 硬编码 | 超时后回退为 `Stop()` 强制终止 |
| 异步任务 | 无上限 | 通过 context 取消信号 + WaitGroup 平滑等待 |

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

---

## 10. Phase B: PostgreSQL 原子租约与多副本 Hub

> 本节描述 Phase B 多副本 Hub 的核心能力：基于 PostgreSQL 的原子任务租约。
> 架构设计详见 [`docs/gateway_balancer/new_design.md`](../../../../docs/gateway_balancer/new_design.md)。

### 10.1 能力概述

当 service-hub 需要多副本高可用部署时，SQLite 的单写入者限制无法满足并发任务领取需求。Phase B 引入 `LeasedTaskStore` 接口，通过 PostgreSQL 的 `FOR UPDATE SKIP LOCKED` 实现无阻塞竞争领取：

| 能力 | 说明 |
|---|---|
| 原子任务领取 | `ClaimNext` 使用 `FOR UPDATE SKIP LOCKED`，多副本互不阻塞 |
| 租约保护 | 每个领取携带唯一 `lease_token`，防止过期副本覆盖结果 |
| 自动续租 | `RenewLease` 延长租约，条件为所有权有效且未过期 |
| 条件完成/失败 | `CompleteLease` / `FailLease` 仅在持有有效租约时生效 |
| 过期回收 | `RequeueExpiredLeases` 批量回收过期租约，回退为 pending |
| 可重试失败 | `FailLease` 支持自动回退 pending + 指数退避 |

### 10.2 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVICE_HUB_PG_DSN` | 空 | PostgreSQL 连接字符串（为空时回退 SQLite） |
| `SERVICE_HUB_PG_MAX_CONNS` | `10` | 连接池最大连接数 |
| `SERVICE_HUB_PG_MIN_CONNS` | `2` | 连接池最小连接数 |
| `SERVICE_HUB_LEASE_TTL` | `60` | 任务租约 TTL（秒） |

### 10.3 存储后端选择逻辑

```
SERVICE_HUB_PG_DSN 非空 → PostgreSQL LeasedTaskStore（多副本 Hub）
SERVICE_HUB_PG_DSN 为空 + SERVICE_HUB_DB_PATH 非空 → SQLite TaskStore（单副本）
两者均为空 → 内存 TaskStore（开发/测试）
```

SQLite 和内存后端的租约方法返回 `ErrLeaseNotSupported`，防止误配置。

### 10.4 时钟同步要求

租约过期时间以 PostgreSQL 服务器时间（`NOW()`）为权威。所有 Hub 副本在判断本地租约有效性时依赖本地时钟。部署必须保证所有 Hub 副本通过 NTP 与数据库服务器保持时钟同步。

### 10.5 K8s 部署

阶段 B PostgreSQL 资源位于 `deploy/k8s/service-hub/postgres/`：

```bash
# 部署 PostgreSQL（阶段 B）
kubectl apply -k deploy/k8s/service-hub/postgres/

# 更新 service-hub Deployment 中的 PG_DSN 环境变量后重新部署
kubectl apply -k deploy/k8s/service-hub/
```

### 10.6 Prometheus 租约指标

| 指标 | 用途 |
|---|---|
| `task_lease_conflicts_total` | 租约所有权冲突数 |
| `task_lease_expired_total` | 租约到期回收次数 |
| `task_claim_latency_seconds` | 任务领取延迟直方图 |
| `task_transitions_total{from,to,result}` | 状态转换计数 |
| `service_hub_ready` | 服务就绪状态 (1/0) |

### 10.7 后续工作

- 下游 Agent 接口幂等键集成
- 多副本压测与领取吞吐基准
- PostgreSQL 主从切换故障演练
- 生产环境 PITR 备份策略配置
