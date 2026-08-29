# engine-go (PrivShield Agent) 可靠性能力说明

> 隐私脱敏引擎（engine-go / PrivShield Agent）的崩溃恢复、自动重试、完整性校验、优雅停机与容灾机制详解。

---

## 1. 能力总览

| 能力维度 | 支持状态 | 实现方式 |
|---|---|---|
| 崩溃恢复（无锁原子预算） | ✅ | CAS 循环原子扣减与原子回滚，SQLite 持久化支持 |
| 崩溃恢复（分布式预算） | ✅ | 跨实例共享 SQLite / 原子会计，支持时间窗口周期重置 |
| 自动重试 | ⚪ 不适用 | 请求级无状态纯函数处理，重试由上游网关/调用方负责 |
| 隐私预算完整性 | ✅ | HMAC-SHA256 / 国密 SM3 签名审计日志，防篡改 |
| 预算 DB 完整性校验 | ✅ | 启动时 `PRAGMA integrity_check` + WAL 模式 |
| 数据库备份 | ✅ | 通过统一备份脚本支持隐私预算 DB 备份 |
| 优雅停机 | ✅ | 标准 OS 信号拦截 (SIGINT/SIGTERM) + Gin HTTP Shutdown + gRPC GracefulStop |
| 隐私预算自动重置 | ✅ | 基于时间窗口（`PRIVACY_BUDGET_WINDOW_SECONDS`）的预算自动清零 |
| 外部 LLM 熔断保护 | ✅ | 3 状态熔断器 (Closed → Open → HalfOpen) + 超时保护 |

---

## 2. 崩溃恢复（Crash Recovery）

### 2.1 隐私预算状态持久化

隐私预算（Privacy Budget）是引擎的核心状态数据。Go 引擎支持两级存储后端：

| 后端 | 持久化 | 多协程并发安全性 | 适用场景 |
|---|---|---|---|
| 内存 (原子 CAS) | ❌ 重启丢失 | ✅ 千万级 QPS 无锁并发 | 单机开发 / 单元测试 |
| SQLite (WAL) | ✅ WAL 持久化 | ✅ 排他事务一致性 | 单机 / 边缘持久化生产 |

**崩溃恢复流程：**

```
进程崩溃 → 重启 → 从 SQLite 加载预算状态 → 恢复到崩溃前的已消耗量
```

- **原子 CAS 内存模式**：基于 `sync/atomic` 与 `math.Float64bits` 实现无锁 CAS 循环，检测到预算透支自动回滚；
- **SQLite 模式**：通过 `PRIVACY_BUDGET_DB` 环境变量配置，重启后从数据库读取 `epsilon_spent`/`delta_spent`。

### 2.2 时间窗口预算重置

引擎支持基于时间窗口的预算自动重置：

- 通过 `PRIVACY_BUDGET_WINDOW_SECONDS` 配置窗口长度；
- 窗口到期后，已消耗预算（`epsilon_spent`/`delta_spent`）自动原子清零；
- SQLite 模式下窗口信息持久化，跨重启保持一致的时间边界。

---

## 3. 隐私预算完整性保障

### 3.1 HMAC / SM3 签名审计日志

每次预算消耗都会写入不可篡改的审计日志：

**签名格式：**

```
timestamp|namespace|epsilon_total|delta_total|epsilon_spent|delta_spent → HMAC-SHA256 / SM3
```

**安全特性：**

| 特性 | 说明 |
|---|---|
| 防篡改 | 每条记录附带 HMAC-SHA256 或 SM3 签名，任何修改都会被检测 |
| 密钥管理 | 通过 `PRIVACY_AUDIT_KEY` 环境变量配置；未配置时使用进程级随机密钥 |
| 协程安全 | 原子指针与互斥锁保护审计日志并发写入 |
| 写入容错 | 审计日志写入失败不阻塞核心预算扣减，输出警告日志 |

---

## 4. 外部 LLM / 模型层熔断与保护

### 4.1 三态熔断器保护 (Circuit Breaker)

| 状态 | 行为 | 转换条件 |
|---|---|---|
| `Closed` (闭合) | 正常向外部 LLM 发送推理请求 | 连续失败达到阈值（如 5 次）→ 切换为 `Open` |
| `Open` (开启) | 立即熔断拒绝，直接降级至 Layer-1 规则结果 | 熔断冷却时间（如 30 秒）到期 → 切换为 `HalfOpen` |
| `HalfOpen` (半开) | 放行探测请求检测后端恢复情况 | 探测成功 → 恢复 `Closed`；失败 → 重新回到 `Open` |

---

## 5. 优雅停机（Graceful Shutdown）

### 5.1 停机流程

```
SIGINT/SIGTERM → 拦截信号 → http.Server.Shutdown(ctx) 停止接收新 HTTP 请求
  → 等待在途请求完成 (最大超时 10s)
  → grpcServer.GracefulStop() 排空在途 gRPC 流
  → 释放资源与连接池 → 进程安全退出
```

---

## 6. 存储与文件防护

### 6.1 压缩炸弹防护
- 流式解析 Excel (`.xlsx`) 时，通过 `io.LimitReader` 强制限制展开 XML 大小（最大 256MB），杜绝 ZIP/XML 解压炸弹。

### 6.2 目录穿越防护
- 医学影像与 DICOM 处理时，通过 `isPathAllowed` 强制校验规范化绝对路径必须位于白名单目录（`PRIVACY_IMAGE_ALLOWED_DIRS`）内。
