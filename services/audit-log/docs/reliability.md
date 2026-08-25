# audit-log 可靠性能力说明

> 脱敏审计日志与存证服务（audit-log）的崩溃恢复、自动重试、完整性校验与备份能力详解。

---

## 1. 能力总览

| 能力维度 | 支持状态 | 实现方式 |
|---|---|---|
| 崩溃恢复 | ⚪ 不适用 | 无异步任务队列，无需恢复孤立任务 |
| 自动重试 | ⚪ 不适用 | 审计写入为同步操作，由上游调用方负责重试 |
| SQLite 完整性校验 | ✅ | `PRAGMA integrity_check` 启动时阻断损坏数据库 |
| 数据库备份 | ✅ | 通过统一备份脚本支持全量/增量备份 |
| 优雅停机 | ✅ | SIGINT/SIGTERM → gRPC GracefulStop → HTTP Shutdown(5s) |
| 审计数据完整性 | ✅ | HMAC-SHA256 签名审计日志 + 快照完整性哈希 + `verify_audit.py` 独立校验脚本 |
| 存储持久化 | ✅ | SQLite WAL 模式，支持内存回退 |

---

## 2. SQLite 完整性校验（Integrity Check）

### 2.1 校验时机

服务启动早期（审计存储初始化之前），对 SQLite 数据库文件执行完整性校验：

```
启动 → ValidateIntegrity(dbPath) → 通过 → 继续初始化审计存储
                                   → 失败 → log.Fatalf() 阻止启动
```

### 2.2 校验实现

使用共享库 `pkg/store/sqlite/init.go` 中的 `ValidateIntegrity()` 函数：

```go
// 1. 打开数据库连接
// 2. 执行 PRAGMA integrity_check
// 3. 检查结果是否为 "ok"
// 4. 损坏时返回 "database corruption detected: ..." 错误
```

### 2.3 设计原则

- **Fail-Fast**：数据库损坏时立即终止启动，防止审计数据进一步损坏或丢失；
- **统一实现**：通过 `sqlite.ValidateIntegrity()` 共享函数，与 service-hub 保持一致；
- **内存模式豁免**：`dbPath` 为空时跳过校验。

---

## 3. 审计数据完整性保障

### 3.1 HMAC 签名审计日志

audit-log 服务关联的 `BudgetAuditLogger`（位于 `engine/privacy/budget.py`）提供不可篡改的审计日志：

**签名机制：**

```
timestamp|namespace|epsilon_total|delta_total|epsilon_spent|delta_spent → HMAC-SHA256 → 签名
```

- 每条审计记录附带 HMAC-SHA256 签名，防止事后篡改；
- 签名密钥通过 `PRIVACY_AUDIT_KEY` 环境变量配置（生产环境必须设置）；
- 未配置密钥时使用进程级随机密钥（重启后旧记录不可校验，输出警告）。

### 3.2 快照完整性哈希

`audit_logs` 表与 `snapshots` 表通过外键关联，快照记录包含 `integrity_hash` 字段：

```sql
audit_logs (id, timestamp, operation, input_hash, output_hash, ...)
    ↓ FOREIGN KEY
snapshots (id, audit_log_id, input_sample, output_sample, integrity_hash, ...)
```

- `input_hash` / `output_hash`：输入/输出数据的哈希值，用于检测数据篡改；
- `integrity_hash`：快照完整性哈希，用于验证快照数据的完整性。

### 3.3 审计签名校验脚本

`engine/privacy/verify_audit.py` 提供独立的 HMAC-SHA256 审计签名校验能力（#9）：

**使用方式：**

```bash
# 基本用法
python -m engine.privacy.verify_audit --log-file /path/to/audit.log --key "your-secret-key"

# 通过文件提供密钥
python -m engine.privacy.verify_audit --log-file /path/to/audit.log --key-file /path/to/key

# 使用环境变量 PRIVACY_AUDIT_KEY
python -m engine.privacy.verify_audit --log-file /path/to/audit.log
```

**功能特性：**

| 特性 | 说明 |
|---|---|
| 逐行校验 | 每行独立解析并验证 HMAC-SHA256 签名 |
| 失败汇总 | 输出失败行号、期望签名与实际签名 |
| 退出码 | 0=全部通过，1=存在失败，2=参数错误 |
| CI 集成 | 可集成到 CI/CD 流自动化校验 |

### 3.4 数据库表结构

| 表名 | 用途 | 关键字段 |
|---|---|---|
| `audit_logs` | 脱敏操作审计记录 | `input_hash`, `output_hash`, `algorithm`, `security_level` |
| `snapshots` | 审计快照（含输入/输出样本） | `integrity_hash`, `input_sample`, `output_sample` |

### 3.5 审计写入持久化保证与性能评估

生产环境必须配置非空的 `DB_PATH`，使 audit-log 使用 SQLite `AuditStore`；内存回退仅适合测试或临时开发，重启后数据会丢失，并且内存实现会在达到容量上限后丢弃最早的记录，不能作为合规存证的持久化方案。SQLite 模式下，`SaveLog()` 以参数化单条 `INSERT INTO audit_logs (...)` 写入完整主审计记录。HTTP `POST /api/audit/logs` 仅在该调用成功后才返回 `201 Created`，写入失败则返回 `500`；gRPC `RecordAudit` 同样仅在 `SaveLog()` 成功后返回 `success = true`，失败时返回 `Internal`。因此，调用方收到成功响应至少可确认主审计记录已由存储层接受，而不是仅保存在服务进程内存中。

主审计记录与快照的原子性边界必须明确：HTTP 创建接口先写入 `audit_logs`，再单独调用 `SaveSnapshot()` 写入 `snapshots`。两次写入目前不是同一个 SQLite 事务；快照写入失败时服务记录错误日志，但 HTTP 响应仍为 `201`，所以该响应不保证关联快照已经存在。gRPC `RecordAudit` 当前只创建主审计记录，不自动创建快照。外键约束确保已写入的快照必须引用已有主记录，但不能使两个独立 `INSERT` 自动共同提交。对“日志和快照必须同时存在”的强合规要求，应将二者封装为同一 `sql.Tx` 中的写入操作，并在任一语句失败时回滚；在此改动完成前，运维应将主记录和快照数量差异、快照写入失败日志及完整性校验失败作为告警信号。

每条仅含主记录的审计请求产生 1 次 SQLite 写入；通过 HTTP 同时创建快照的请求通常产生 2 次顺序写入。WAL 允许审计查询与写入并发进行，`busy_timeout=5000` 在短暂写锁竞争时等待最多 5 秒，连接池限制为 4 个打开连接、2 个空闲连接以限制竞争；但 SQLite 仍只有一个写入者，高并发审计提交、慢磁盘或大尺寸 `parameters_json`/快照样本都会提升写入尾延迟。接口将 `parameters` 序列化后的大小限制为 1 MiB，可避免单条参数无限制放大数据库和 I/O 成本，但输入/输出样本的大小仍应通过网关或调用方约束并纳入容量规划。

性能验收必须在实际存储卷、文件系统和容器限额下执行，至少采集 `SaveLog` 与 `SaveSnapshot` 的成功率和 P50/P95/P99 延迟、SQLite busy/错误次数、WAL 文件大小、每秒写入数，以及主记录与快照的数量差异。基准应分别覆盖单并发主记录写入、多并发主记录写入和 HTTP 双写路径；不要将开发机吞吐量作为生产承诺。可靠性验收还应注入锁竞争和磁盘错误，确认主记录失败会向调用方暴露，而快照失败会被日志与监控及时发现并补偿。

---

## 4. 数据库备份（Backup）

### 4.1 备份脚本

通过 `scripts/prod/backup-sqlite-databases.sh` 统一备份 audit-log 数据库。

### 4.2 备份模式

| 模式 | 参数 | 说明 |
|---|---|---|
| 全量备份 | `--full`（默认） | 完整备份审计日志数据库 |
| 增量备份 | `--incremental` | 基于 SHA-256 哈希比对，仅备份变化的数据库 |
| 恢复验证 | `--verify` | 解压最新备份并执行 `PRAGMA integrity_check`，确保备份可恢复 |

### 4.3 备份特性

- **在线备份**：使用 `sqlite3 .backup` 命令，不锁库；
- **压缩存储**：默认 gzip 压缩；
- **过期清理**：自动删除超过 `RETENTION_DAYS`（默认 7 天）的旧备份；
- **Cron 集成**：`--install-cron` 安装定时任务。

### 4.4 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `BACKUP_DIR` | `/var/backups/privshield` | 备份存储目录 |
| `AUDIT_LOG_DB_PATH` | — | audit-log 数据库文件路径 |
| `RETENTION_DAYS` | `7` | 备份保留天数 |

---

## 5. 优雅停机（Graceful Shutdown）

### 5.1 停机流程

```
SIGINT/SIGTERM → gRPC GracefulStop → HTTP Shutdown(5s) → 进程退出
```

**详细步骤：**

1. **信号捕获**：监听 `SIGINT` 和 `SIGTERM`；
2. **gRPC 优雅停机**：
   - `serviceImpl.Shutdown()`：发送 context 取消信号；
   - `grpcServer.GracefulStop()`：等待在途 RPC 完成；
3. **HTTP 优雅停机**：`httpSrv.Shutdown(ctx)` 等待现有请求完成（5 秒硬上限）。

### 5.2 数据安全保证

- 优雅停机确保正在写入的审计记录完整落盘；
- SQLite WAL 模式保证崩溃安全（即使非优雅停机也不会损坏数据库）；
- 在途的审计写入请求在停机信号到达后会继续完成。

---

## 6. 存储可靠性

### 6.1 SQLite 配置

| PRAGMA | 值 | 说明 |
|---|---|---|
| `journal_mode` | `WAL` | Write-Ahead Logging，崩溃安全 |
| `synchronous` | `NORMAL` | 性能与安全平衡 |
| `busy_timeout` | `5000` | 遇锁等待 5 秒 |
| `foreign_keys` | `ON` | 强制外键约束（audit_logs ↔ snapshots） |

### 6.2 连接池配置

| 参数 | 值 | 说明 |
|---|---|---|
| `MaxOpenConns` | 4 | 限制并发连接数 |
| `MaxIdleConns` | 2 | 保持适量空闲连接 |
| `ConnMaxLifetime` | 5 分钟 | 防止长连接泄漏 |

### 6.3 存储回退

- **SQLite 模式**（`DBPath` 非空）：持久化存储，审计数据重启后可查；
- **内存模式**（`DBPath` 为空）：`memory.NewAuditStore()`，适用于测试场景，**重启后审计数据丢失**。

---

## 7. 运维建议

### 7.1 生产部署检查清单

- [ ] 配置 `DB_PATH` 启用 SQLite 持久化；
- [ ] 配置 `AUDIT_LOG_DB_PATH` 并定期执行备份脚本；
- [ ] 设置 `PRIVACY_AUDIT_KEY` 环境变量（高强度随机密钥），确保审计签名跨重启可校验；
- [ ] 定期使用 `python -m engine.privacy.verify_audit` 校验审计日志签名完整性；
- [ ] 定期执行备份脚本的 `--verify` 模式验证备份可恢复性；
- [ ] 监控启动日志中的 `integrity check` 状态。

### 7.2 故障排查

| 现象 | 可能原因 | 排查方法 |
|---|---|---|
| 启动时 integrity check failed | SQLite 文件损坏 | 从备份恢复数据库 |
| 审计签名校验失败 | 密钥变更或未配置 | 检查 `PRIVACY_AUDIT_KEY` 是否一致 |
| 审计数据丢失 | 使用了内存模式 | 确认 `DB_PATH` 已配置 |
