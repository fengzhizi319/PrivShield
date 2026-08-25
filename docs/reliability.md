# engine (PrivShield Agent) 可靠性能力说明

> 隐私脱敏引擎（engine / PrivShield Agent）的崩溃恢复、自动重试、完整性校验与备份能力详解。

---

## 1. 能力总览

| 能力维度 | 支持状态 | 实现方式 |
|---|---|---|
| 崩溃恢复（预算状态恢复） | ✅ | SQLite 持久化隐私预算，重启后自动恢复 |
| 崩溃恢复（Redis 分布式预算） | ✅ | Redis 持久化预算，支持 Lua 原子操作 |
| 自动重试 | ⚪ 不适用 | 请求级处理，重试由上游网关/调用方负责 |
| 隐私预算完整性 | ✅ | HMAC-SHA256 签名审计日志，防篡改 |
| 预算 DB 完整性校验 | ✅ | 启动时 `PRAGMA integrity_check` + WAL 模式 + BEGIN IMMEDIATE 排他事务 |
| 数据库备份 | ✅ | 通过统一备份脚本支持隐私预算 DB 备份 |
| 优雅停机 | ✅ | Uvicorn 信号处理 + FastAPI Lifespan |
| 隐私预算自动重置 | ✅ | 基于时间窗口的预算自动清零 |
| LLM 并发保护 | ✅ | 信号量限流 + 内存阈值检测，防止 OOM |

---

## 2. 崩溃恢复（Crash Recovery）

### 2.1 隐私预算状态持久化

隐私预算（Privacy Budget）是引擎的核心状态数据。引擎支持三级存储后端：

| 后端 | 持久化 | 多实例一致性 | 适用场景 |
|---|---|---|---|
| 内存 | ❌ 重启丢失 | ❌ 不一致 | 测试/单实例开发 |
| SQLite | ✅ WAL 持久化 | ✅ 共享 DB 文件 | 单实例生产 |
| Redis | ✅ 持久化 | ✅ 分布式共享 | 多实例/分布式生产 |

**崩溃恢复流程：**

```
进程崩溃 → 重启 → 从 SQLite/Redis 加载预算状态 → 恢复到崩溃前的已消耗量
```

- **SQLite 模式**：通过 `PRIVACY_BUDGET_DB` 环境变量配置，重启后从数据库读取 `epsilon_spent`/`delta_spent`；
- **Redis 模式**：通过 `PRIVACY_BUDGET_REDIS_URL` 或 `PRIVACY_BUDGET_BACKEND=redis` 配置，Lua 脚本保证原子性读写；
- **Redis 降级**：Redis 不可用时自动降级到 SQLite 或内存模式。

### 2.2 时间窗口预算重置

引擎支持基于时间窗口的预算自动重置：

- 通过 `PRIVACY_BUDGET_WINDOW_SECONDS` 配置窗口长度；
- 窗口到期后，已消耗预算（`epsilon_spent`/`delta_spent`）自动清零；
- SQLite/Redis 模式下窗口信息持久化，多实例共享一致的时间边界。

---

## 3. 隐私预算完整性保障

### 3.1 HMAC 签名审计日志

每次预算消耗都会写入不可篡改的审计日志：

**签名格式：**

```
timestamp|namespace|epsilon_total|delta_total|epsilon_spent|delta_spent → HMAC-SHA256
```

**安全特性：**

| 特性 | 说明 |
|---|---|
| 防篡改 | 每条记录附带 HMAC-SHA256 签名，任何修改都会被检测 |
| 密钥管理 | 通过 `PRIVACY_AUDIT_KEY` 环境变量配置；未配置时使用进程级随机密钥 |
| 线程安全 | `_lock` 互斥锁保护审计日志写入 |
| 写入失败容错 | 审计日志写入失败不阻塞预算扣减，仅输出警告 |

### 3.2 并发安全

预算扣减使用多层并发保护：

1. **实例级互斥锁**（`_mu`）：保护 `spend()`/`remaining()` 的原子性；
2. **SQLite BEGIN IMMEDIATE**：排他性事务，保证多进程写入一致性；
3. **Redis Lua 脚本**：原子性判断预算充足并扣减，避免竞态条件；
4. **BudgetRegistry 注册表锁**：保证同一命名空间只存在一份预算状态。

---

## 4. 预算 DB 启动完整性校验（Integrity Check）

### 4.1 校验时机

引擎启动时，`BudgetAccountant._init_db()` 方法在初始化 SQLite 连接后、执行任何业务操作前，对数据库文件进行完整性校验：

```
启动 → _init_db() → 打开 SQLite 连接 → PRAGMA integrity_check
  → 结果 "ok" → 继续正常初始化
  → 结果非 "ok" → 输出 ERROR 日志（budget_db_integrity_check_failed）
  → 异常 → 输出 ERROR 日志（budget_db_integrity_check_error）
```

### 4.2 校验实现

```python
# engine/privacy/budget.py — BudgetAccountant._init_db()
if os.path.exists(db_path):
    check_conn = sqlite3.connect(db_path, timeout=10.0)
    result = check_conn.execute("PRAGMA integrity_check").fetchone()
    if result and result[0] != "ok":
        logger.error("budget_db_integrity_check_failed", detail=result[0])
```

### 4.3 设计原则

- **非阻断告警**：与 Go 微服务的 `log.Fatalf()` 不同，Python 引擎采用 ERROR 日志告警策略，允许运维介入但不强制阻断启动（避免单点故障扩散）；
- **独立连接校验**：使用独立的 SQLite 连接执行校验，不影响后续业务连接；
- **存在性检查**：仅在数据库文件已存在时执行校验（首次初始化跳过）。

---

## 5. LLM 推理保护

### 4.1 并发限流

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_LLM_MAX_CONCURRENCY` | `1` | 进程级 LLM 推理并发上限（信号量控制） |
| `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | `30` | 等待推理槽位的最大时间 |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | `512` | 可用内存低于此值时跳过 LLM 层 |

**工作机制：**
- 使用 `asyncio.Semaphore` 控制进程级 LLM 并发，防止 OOM；
- 等待超时后自动降级（跳过 LLM 层，使用规则/NER 结果）；
- 内存检测在 LLM 推理前执行，内存不足时直接跳过。

---

## 6. 数据库备份（Backup）

### 5.1 隐私预算 DB 备份

通过 `scripts/prod/backup-sqlite-databases.sh` 或 `scripts/prod/backup_privacy_budget.sh` 备份隐私预算数据库。

### 5.2 备份特性

- **在线备份**：`sqlite3 .backup` 命令不锁库；
- **WAL 模式安全**：WAL 模式下 `.backup` 保证一致性快照；
- **压缩 + 过期清理**：默认 gzip 压缩，自动清理超过保留天数的旧备份。

---

## 7. 优雅停机（Graceful Shutdown）

### 6.1 停机流程

```
SIGINT/SIGTERM → Uvicorn 停止接受新连接 → 等待在途请求完成 → FastAPI Lifespan 清理 → 进程退出
```

**关键步骤：**

1. **信号捕获**：Uvicorn 监听 SIGINT/SIGTERM；
2. **请求排空**：停止接受新连接，等待在途请求处理完毕；
3. **资源清理**：FastAPI Lifespan 上下文管理器执行清理（关闭 HTTP 客户端连接池等）；
4. **gRPC 停机**：`grpcServer.stop(0)` 或等待 gRPC 线程结束。

---

## 8. 存储可靠性

### 7.1 SQLite 配置（隐私预算 DB）

| PRAGMA | 值 | 说明 |
|---|---|---|
| `journal_mode` | `WAL` | 崩溃安全，读写不互斥 |
| `synchronous` | `NORMAL` | WAL 模式下安全且高性能 |
| `busy_timeout` | `10000` | 遇锁等待 10 秒（比 Go 微服务更长，适应 Python GIL 竞争） |

### 7.2 连接管理

- **按操作连接**：每次 `spend()`/`remaining()` 独立打开/关闭 SQLite 连接，避免线程池连接泄漏；
- **WAL 格式保证**：初始化阶段即开启 WAL 模式，确保数据库文件始终为 WAL 格式；
- **事务回滚保护**：异常时执行 `conn.rollback()`，防止残留事务影响后续操作。

---

## 9. 运维建议

### 9.1 生产部署检查清单

- [ ] 设置 `PRIVACY_BUDGET_DB` 启用预算持久化（避免内存模式重启丢失）；
- [ ] 多实例部署时设置 `PRIVACY_BUDGET_BACKEND=redis` 或共享 SQLite 路径；
- [ ] 设置 `PRIVACY_AUDIT_KEY`（高强度随机密钥），确保审计签名跨重启可校验；
- [ ] 配置 `PRIVACY_LLM_MAX_CONCURRENCY` 限制 LLM 并发，防止 OOM；
- [ ] 配置 `PRIVACY_LLM_MIN_FREE_MEM_MB` 设置内存安全阈值；
- [ ] 定期备份隐私预算数据库；
- [ ] 定期使用 `python -m engine.privacy.verify_audit` 校验审计日志签名完整性。

### 9.2 故障排查

| 现象 | 可能原因 | 排查方法 |
|---|---|---|
| 重启后预算被重置 | 未配置持久化后端 | 检查 `PRIVACY_BUDGET_DB` |
| 预算扣减报 "exhausted" | 窗口未重置或预算确实耗尽 | 检查 `PRIVACY_BUDGET_WINDOW_SECONDS` |
| LLM 推理 OOM | 并发过高或内存不足 | 降低 `PRIVACY_LLM_MAX_CONCURRENCY` |
| 审计签名校验失败 | 密钥变更 | 检查 `PRIVACY_AUDIT_KEY` 一致性 |
| 启动日志出现 `budget_db_integrity_check_failed` | 预算 DB 文件损坏 | 从备份恢复或检查磁盘健康 |
