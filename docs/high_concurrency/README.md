# 高并发与多连接架构 (High Concurrency Architecture)

本目录包含 `privacy-local-agent` 支持万级并发请求 (10,000+ QPS) 和海量长连接的技术设计与实现方案。

## 目录 (Table of Contents)

- [目录结构](#目录结构)
- [已实现的优化特性](#已实现的优化特性)
- [全功能高并发组件](#全功能高并发组件分类分级脱敏通用架构)
- [环境变量速查](#环境变量速查)
- [启动方式](#启动方式)
- [测试](#测试)
- [快速导航](#快速导航)

---

## 目录结构

- [高并发架构设计与 4 种备选方案](design.md)：针对 10k QPS 场景的瓶颈分析、4 种架构方案（多进程 + SO_REUSEPORT、全异步协程化、异步 + Rust 扩展、gevent + Numba JIT）对比与详细实现指南。

## 已实现的优化特性

### 方案一：多进程 + SO_REUSEPORT 端口共享

| 模块 | 文件 | 说明 |
|------|------|------|
| 多进程启动器 | `privacy_local_agent/launcher.py` | fork N 个 worker 共享同一端口，内核级连接分发；worker 意外退出自动拉起（`launch` 与 `--warmup` 均支持） |
| fork-after-warmup | `launcher.py --warmup` | 主进程预热 NER/LLM 模型并注册到预加载表，fork 后 worker 首次使用时直接复用（COW 共享模型只读内存页，避免 N 份模型内存翻倍） |
| SQLite WAL 模式 | `privacy_local_agent/privacy/budget.py` | 读写不再互斥，支持多进程并发预算扣减 |
| gRPC 线程池调优 | `PRIVACY_GRPC_MAX_WORKERS` 环境变量 | 每 worker 默认 64 线程，支持环境变量配置 |

### 方案四：NumPy 向量化 + 请求合并（最快验证路径）

| 模块 | 文件 | 说明 |
|------|------|------|
| DP 数值核心 | `privacy_local_agent/privacy/dp_jit.py` | Laplace/Gaussian 噪声采样、值截断、L2 范数裁剪，全部 NumPy 向量化（无 Python 级循环）；检测到 Numba 时置 `HAS_NUMBA=True` 供后续 JIT 叠加 |
| 噪声预生成池 | `privacy_local_agent/privacy/high_concurrency.py` → `NoisePool` | 预生成 10K+ 噪声样本，取用 O(1)，避免实时采样 CPU 开销 |
| 批量预算扣减 | `high_concurrency.py` → `BatchedBudgetSpend` | 1ms 时间窗口合并多个 spend 为 1 次原子操作，减少锁竞争 |

### 全功能高并发组件（分类分级、脱敏、通用架构）

| 模块 | 文件 | 说明 |
|------|------|------|
| 分类分级 LRU 缓存 | `dynclassification/service.py` | 字段级分类结果高并发 LRU 缓存，相同请求重复检索提升 100x QPS |
| 动态异步批处理器 | `high_concurrency.py` → `AsyncDynamicBatcher` | 微秒/毫秒时间窗口合并多个并发单条/小批请求为一次 Batch 处理（适用 Small-NER/LLM 等） |
| 脱敏规则匹配缓存 | `privacy/masking.py` → `guess_field_type` | `functools.lru_cache` 字段推断缓存，避免海量日志/记录处理时重复正则匹配 |
| 并发信号量限流 | `high_concurrency.py` → `ConcurrencyThrottle` | CPU/GPU 密集型分类与脱敏任务信号量限流保护，防止死锁或过载 OOM |

### 通用优化

| 模块 | 文件 | 说明 |
|------|------|------|
| uvloop + httptools | `privacy_local_agent/server.py` | 自动检测并启用高性能事件循环与 HTTP 解析器 |
| GZip 响应压缩 | `privacy_local_agent/main.py` | ≥1KB 响应自动 gzip 压缩，减少网络传输 |
| 并发连接限制 | `PRIVACY_LIMIT_CONCURRENCY` 环境变量 | 默认 10000 最大并发连接，防止过载 OOM |
| 请求数限制 | `PRIVACY_LIMIT_MAX_REQUESTS` 环境变量 | 默认 100000 最大请求数，防止内存泄漏 |

## 环境变量速查

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PRIVACY_WORKERS` | `min(cpu_count, 8)` | 多进程 worker 数 |
| `PRIVACY_GRPC_MAX_WORKERS` | `64` | 每 worker gRPC 线程池大小 |
| `PRIVACY_LIMIT_CONCURRENCY` | `10000` | REST 最大并发连接数 |
| `PRIVACY_LIMIT_MAX_REQUESTS` | `100000` | REST worker 最大处理请求数 |
| `PRIVACY_TIMEOUT_KEEP_ALIVE` | `30` | keep-alive 超时（秒） |

## 启动方式

```bash
# 单进程模式（开发调试）
python -m privacy_local_agent.server

# 多进程模式（生产推荐）
python -m privacy_local_agent.launcher --workers 4

# 多进程 + ML 模型预热（节省内存）
python -m privacy_local_agent.launcher --workers 4 --warmup
```

## 测试

```bash
# Python 后端测试
PYTHONPATH=. pytest tests/test_high_concurrency.py -v

# 前端并发压测面板测试
cd console/web && pnpm test -- --run src/components/__tests__/ConcurrencyTestPanel.test.tsx
```

## 快速导航

| 文档 | 说明 | 适宜场景 |
|---|---|---|
| [设计实现文档 (design.md)](design.md) | 包含 10,000 QPS 架构设计、瓶颈分析、4 种备选方案拆解、性能对比与关键代码原型 | 高并发选型、技术架构评估、性能调优 |
