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

生产环境推荐配置 `AUDIT_LOG_PG_DSN`（PostgreSQL Phase B）或非空的 `AUDIT_LOG_DB_PATH`（SQLite WAL 模式）；内存回退仅适合测试或临时开发。

1. **HTTP 与 gRPC 双协议原子快照联动**：
   - HTTP `POST /api/audit/logs` 与 gRPC `RecordAudit` 均通过 `SaveLogWithSnapshot()` 实现主记录与关联快照的单事务原子提交；任一写入失败均会完整回滚，绝不产生悬挂孤立记录。
2. **前后防篡改哈希链 (Hash Chain)**：
   - 每条审计记录在落盘时包含 `prev_hash`，指向上一条记录的综合密码学哈希；
   - 提供 `VerifyChain` 端点支持全量/区间连续性核验，杜绝物理删行或记录插入攻击。
3. **敏感样本应用层信封加密 (Envelope Encryption)**：
   - 快照表中的 `input_sample` 和 `output_sample` 由 `pkg/crypto` 采用 SM4-GCM 密文落盘，防止数据库文件被拖库导致隐私外泄。
4. **PostgreSQL Phase B 高并发与多副本扩容**：
   - 在高吞吐集群环境下，通过配置 PostgreSQL DSN 消除 SQLite 单写锁瓶颈，支持多副本并发存证与 `SaveLogsBatch` 批量管道刷盘。

WAL 模式允许 SQLite 读写并发，`busy_timeout=5000` 在短暂写锁竞争时等待最多 5 秒；而在 PostgreSQL 模式下支持行级并发事务与连接池自动弹性伸缩。接口将 `parameters` 序列化后的大小限制为 1 MiB，有效防范超大报文对存储的非预期占用。

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

- [ ] 配置 `AUDIT_LOG_DB_PATH` 启用 SQLite 持久化，并定期执行备份脚本；
- [ ] 设置 `PRIVACY_AUDIT_KEY` 环境变量（高强度随机密钥），确保审计签名跨重启可校验；
- [ ] 定期使用 `python -m engine.privacy.verify_audit` 校验审计日志签名完整性；
- [ ] 定期执行备份脚本的 `--verify` 模式验证备份可恢复性；
- [ ] 监控启动日志中的 `integrity check` 状态。

### 7.2 故障排查

| 现象 | 可能原因 | 排查方法 |
|---|---|---|
| 启动时 integrity check failed | SQLite 文件损坏 | 从备份恢复数据库 |
| 审计签名校验失败 | 密钥变更或未配置 | 检查 `PRIVACY_AUDIT_KEY` 是否一致 |
| 审计数据丢失 | 使用了内存模式 | 确认 `AUDIT_LOG_DB_PATH` 已配置 |
