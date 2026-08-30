# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **深度优化建议评估与定向改造（三项）**：对外部提出的 6 项优化建议逐条代码验证后实施 3 项高确定性改造：
  - **LLMClient Half-Open 并发试探限制（P1 可靠性）**：`checkCircuit` 在 Half-Open 态原为无条件放行，刚恢复的 LLM 会被瞬时并发流量二次打崩；新增 `halfOpenInflight` 原子配额（上限 3，与 gateway.CircuitBreaker 语义对齐）+ 幂等 `releaseProbe` 回调，超额请求直接拒绝走 Safety Floor 降级。
  - **RuleEngine 热路径 Stat 节流（P1 性能）**：`checkRulesReload` 原每次 Classify 调用都执行 `os.Stat` syscall；新增 5s 节流窗口（`lastCheckNano` CAS 串行化，`PRIVACY_RULES_RELOAD_CHECK_SECONDS` 可配置，0 禁用）。
  - **ArbitrateBatch 并行阈值 32→128（P3）**：单条仲裁为纯内存比较极轻量，32~128 区间 goroutine 创建开销高于并行收益，小批量改走串行单趟。
  - 未采纳项评估：fsnotify 改造（白名单热重载实际未接入请求热路径，前提错位；且违背零外部依赖约定）；LRU 读锁/S3-FIFO、流式大文件、ReverseProxy 内聚 BackendNode 因改造面大/需产品确认留待后续。新增 2 个专项单元测试，19 包 `-race` 全部通过。
- **第五轮 engine-go 深度四维架构审计优化（P1~P2，5 项）**：
  - **P1 并发安全**：balancer `selectP2C` / `NewHealthCheckHandler` 无锁读 `EWMA` 字段与 `UpdateEWMA` 写端数据竞争，加 `GetEWMA()` 安全访问器修复。
  - **P1 功能性**：`/readyz/llm` 从硬编码 `"not_loaded"` 改为通过 `PrivacyService.LLMStatus()` 真实探测 LLM 可用性（区分 not_configured / ready / unavailable）。
  - **P1 安全/内存**：分类漏斗缓存 key 包含原始 value（高基数 PII 导致内存膨胀与数据驻留），改用 SHA-256 截断 128bit 哈希化 value 部分。
  - **P2 性能**：`ProcessAgentData` 单线程循环改为 >32 记录多核 strided 并行，与 ClassifyBatch/MaskBatchContext 一致。
  - **P2 并发**：`engineCache` 随机淘汰改为两阶段策略（优先淘汰 default/低置信度条目，保留热规则命中结果）。
  - 19 个包全部通过 `go test -race -count=1 ./...`，零数据竞争。
- **第四轮 engine-go 深度四维架构审计优化（P0~P2，6 项）**：
  - **P0 正确性/安全**：kano.parseNumeric 改用 strconv.ParseFloat 消除死循环与解析错误；formatFloat 修复 `string(rune(n+'0'))` 对 >9 数值的截断 bug；constantTimeLookup 改用 sort.Strings 确定性迭代 + subtle.ConstantTimeCompare 消除时序侧信道；DICOM 文件读取加 256MB 上限（PRIVACY_DICOM_MAX_FILE_SIZE 可配置）。
  - **P1 可靠性**：agent/gateway gRPC GracefulStop 加超时回退（PRIVACY_GRPC_GRACEFUL_STOP_SECONDS），防止 RPC 不结束导致挂死；后台 goroutine（限流清理/代理缓存清理）与关闭生命周期绑定。
  - **P2 并发性能**：LDP 批量扰动（PerturbBinaryBatch/PerturbCategoricalBatch）改用 per-worker 独立 rand.Rand，消除全局 math/rand 锁竞争。
  - 19 个包全部通过 `go test -race -count=1 ./...`，零数据竞争。
- **第三轮 engine-go 深度四维架构审计优化（P0~P2，9 项）**：
  - **P0 隐私/安全/可靠性**：TypedServer Mask/MaskBatch 脱敏失败返回 `"***"` 而非原文（消除隐私泄露）；TypedServer DPHistogram/DPNoisyHistogram/DPChunkedHistogram 统一走 service 层预算核算（消除预算绕过）；grpc_proxy getOrCreateConn 修复 defer+手动 Unlock 双重解锁 panic；service 层 DPGroupBy/DPAggregate/DPAdaptiveClip 补充预算消耗检查。
  - **P1 并发安全**：RuleEngine 引入 `atomic.Pointer[ruleSnapshot]` 无锁读替换，消除 Classify 读 rules/fieldRegexps/ac 与 checkRulesReload 写端的数据竞争；WhitelistManager checkReload 加 RLock 读取 lastMtime 消除数据竞争。
  - **P2 防御性**：ProcessAgentData 归一化错误 slog.Warn 日志（替代静默忽略）；getEnvInt 全部改用 strconv.Atoi + 错误回退默认值（替代 fmt.Sscanf）。
  - 12 个 engine-go 包全部通过 `go test -race -count=1 ./...`，零数据竞争。
- **第二轮 engine-go 深度四维架构审计优化（P0~P3，24 项）**：
  - **P0 隐私安全**：dpHistogram/dpNoisyHistogram 统一走预算核算；Mask RPC 失败返回错误而非原文（消除隐私泄露）；dpAggregate/dpGroupBy 检查预算错误返回 429；PrivacyService 热重载使用 `atomic.Pointer` 消除数据竞争。
  - **P1 架构可靠性**：RuleEngine 缓存 16 分片有界化（随机半量淘汰）；热重载从文件重新加载规则；SafetyFloor Arbitrate 加 RLock；SelectNode SWRR 无锁化（atomic.Int32）；LLM 错误响应限制 1MB；strconv 替代手写解析消除溢出。
  - **P2 资源防御**：gRPC 连接池限制 256；proxyCache/rateLimiter goroutine 优雅退出；gRPC 双向流完整等待；LLM 重试可取消；冒泡排序→sort.Float64s；限流路径归一化（动态 ID→`:id`）；IsAvailable HEAD 探测。
  - **P3 性能可观测**：CacheStats atomic 计数器；ArbitrateBatch 多核并行（>32 条目）；DPChunked 使用请求 ctx；Profile 加载错误 slog.Warn 日志。
  - 12 个 engine-go 包全部通过 `go test -race -count=1 ./...`，零数据竞争。
- **全链路可靠性能力改进与文档同步**：
  - **service-hub 崩溃恢复与自动重试**：启动时区分 pending（保留队列）/ running（标记 failed）孤立任务回收，周期性后台重试失败任务（指数退避延迟 + RetryCount 结构化字段），Prometheus 指标 `orphaned_tasks_recovered_total` / `tasks_retried_total`。
  - **service-hub HTTP/gRPC 双协议 mTLS**：共享 `pkg/tlsutil` 工具库，TLS 1.3 强制最低版本，支持 require/verify/request 客户端认证模式与公钥固定（SPKI Pinning）。
  - **gateway HTTP/gRPC 故障重试**：最多 3 次重试，指数退避 + 随机抖动，幂等方法无条件重试，非幂等仅 ConnectError 重试。
  - **gateway 熔断器 Prometheus 指标**：`circuit_breaker_state{node="..."}` 实时暴露熔断器状态。
  - **gateway 动态拓扑管理**：运行时 API 注册/注销/隔离/排空/激活后端节点。
  - **engine 预算 DB 启动完整性校验**：`PRAGMA integrity_check` + WAL 模式 + BEGIN IMMEDIATE 排他事务。
  - **audit-log 独立校验脚本**：`engine/privacy/verify_audit.py` 独立验证审计数据完整性，支持 CI/CD 集成。
  - **bff-go gRPC 重试策略可配置**：环境变量配置重试次数与退避参数，默认最多 6 次，指数退避 1s→8s。
  - **备份脚本 --verify 模式**：支持备份恢复验证，自动过期清理。
  - **全量可靠性文档体系**：每个微服务/模块均具备专属 `docs/reliability.md`，架构设计文档同步添加可靠性能力矩阵与交叉引用。

## [1.8.0] - 2026-08-24

### ⚠️ Breaking Changes
- **核心包物理更名与单轨化**：核心隐私与动态分类分级引擎目录由 `PrivShield/` 物理更名为 `engine/`，Python 顶层包导入路径全面切换为 `engine.*`（如 `from engine.service import PrivacyService`、`python -m engine.server`）。
- **兼容性声明（无导入别名）**：全面移除过渡期符号链接与 `sys.modules["PrivShield"]` 动态导入别名映射，所有下游调用方必须显式导入 `engine`，实现彻底的单轨化架构。
- **构建链与镜像入口单轨化**：删除孤儿 `engine/Dockerfile`，统一采用仓库根目录多阶段 `Dockerfile`（支持 `--target core|ml`），所有 Docker 构建上下文统一收敛至仓库根目录。

### Changed
- **企业级 Monorepo 目录分层解耦**：
  - 中台微服务群解耦提至根目录 `services/{service-hub,datasource-mgr,audit-log}`。
  - Go 共享基础库提至根目录 `pkg/`，根目录 `go.work` 统一纳管全部 5 个 Go 模块（Go 1.25）。
  - 控制台职责收敛为 `console/{bff-go,web}`，统一由 Go BFF 提供 REST 入口与 gRPC 上游代理；已移除 `console/bff-py` Python REST 备用 BFF。
- **运维与启停脚本体系全面收敛**：
  - `console/scripts/` 下 20+ 个启停、Docker 编排及测试脚本全面归并至 `scripts/dev/` 与 `scripts/prod/`。
  - `console/scripts/` 中保留向后兼容转发脚本，执行时输出 `[DEPRECATED]` 迁移警告并自动转发执行新路径脚本。
  - 运行时产物路径统一，PID 统一输出至根目录 `.pids/`，运行日志统一输出至根目录 `.logs/`。
- **工具链与文档链路全面单轨化**：
  - `Makefile`、`pyproject.toml`、`.github/workflows/ci.yml`、`mkdocs.yml` 全量配置单轨化（`--cov=engine`、`engine/`）。
  - 重新编译生成 `services/service-hub/proto/servicehub.pb.go`，清除历史旧路径 rawDesc 残留。

### Security & Resilience
- **全栈多层次防 DDoS 纵深防御体系**：
  - 慢速连接与 Slowloris 防护（5s 强制请求头超时与 1MB 请求头限制）。
  - 请求体大包 DoS 防护（`MaxBodySize` 快速切断超限请求并响应 `413 Payload Too Large`）。
  - 线程安全 IP 令牌桶限流（`IPRateLimiter` 超额精准响应 `429 Too Many Requests` 与 `Retry-After: 1` 响应头，后台自动 GC 闲置桶）。
  - 并发容量硬顶保护（`MaxConcurrent` 信号量中间件，过载快速返回 `503 Service Unavailable` 保护协程池）。
- **路径穿越 (LFI) 与敏感信息泄露防护**：
  - `datasource-mgr` CSV 加载增加严格 `.csv` 后缀与文件名白名单沙箱校验。
  - 运行时 Panic 堆栈详情收敛至内部日志，HTTP 响应全局安全脱敏。
- **高可用调度与分布式一致性**：
  - 多节点 Client-Side 负载均衡（`PRIVACY_AGENT_URLS` 集群平滑轮询与容灾切换）。
  - 网关 P2C（Power of Two Choices）综合评分动态分流。
  - 分布式隐私预算 Redis Lua 原子记账与滑动窗口自动重置。

### Added
- 动态分类分级引擎基于 `OrderedDict` 的真实 LRU 评估缓存 (`ConfigurableRuleEngine._eval_cache`)，支持并发线程锁 (`_cache_lock`)、`PRIVACY_ENGINE_CACHE_MAX_SIZE` 环境变量配置及微秒级重复评估匹配
- 独立 Markdown 文档提取与规则 YAML 生成脚本 (`scripts/data/gen_yaml_from_doc.py`)，支持从 Markdown 规范直接抽取规则
- 大模型 YAML 关键词自动扩展脚本 (`scripts/data/expand_keywords_with_llm.py`)，支持在线 LLM API 与离线降级同义词表扩展
- `rule_schema.py` 核心架构与系统流转图 Docstring 文档
- 全面扩充 `general-pii` / `finance` / `medical` / `sc_health_db51` 领域规则包关键词词库
- Apache-2.0 LICENSE
- GitHub Actions CI: lint (ruff + mypy) / test (Python 3.13) / security (pip-audit) / docker build
- Ruff lint + format 配置 (pyproject.toml)
- mypy 类型检查配置
- pytest 覆盖率配置 (pytest-cov, fail_under=60)
- py.typed PEP 561 类型标记
- pre-commit hooks (ruff + ruff-format + trailing-whitespace)
- CHANGELOG.md (本文件)
- CONTRIBUTING.md 贡献指南
- SECURITY.md 安全漏洞报告流程
- Makefile 增强: lint/format/typecheck/cover/bench 目标
- pytest markers: `@pytest.mark.integration` / `@pytest.mark.slow`
- pytest-benchmark 性能基准测试

## [0.1.0] - 2024-06-01

### Added
- REST API (FastAPI, port 8079): masking / DP / K-anonymity / QoL / LocalDP / classification
- gRPC API (port 50051): 双协议统一 PrivacyService
- 差分隐私: count/sum/mean/histogram/vector_sum/vector_mean/adaptive_clip/groupby
- 本地差分隐私: binary/categorical 扰动与估计
- 数据脱敏: mobile/id_card/name/bank_card/email/address + HMAC hash
- K-匿名: 单记录/整表 Mondrian/DataFrame
- 查询混淆: 语义槽位替换 + 批量混淆
- 数据分类: Rule Engine → Small-NER → LLM 三层级联
- 隐私预算管理: 命名空间隔离 + SQLite 持久化
- 可观测性: Prometheus metrics + OTel tracing + 结构化日志
- 生产安全: API Key/mTLS 认证 + RBAC + Rate Limit + TLS
- 网关/负载均衡: REST + gRPC 反向代理
- 部署: Dockerfile 多阶段 + Helm Chart + Kustomize + docker-compose
- Arrow IPC 高效二进制端点
- 个性化隐私画像 (config/personalized-profiles.yaml)
