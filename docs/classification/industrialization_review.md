# 数据分类分级模块 — 工业化评审文档

> 本文档用于对 `privacy_local_agent/privacy/classification*` 及其 REST/gRPC 接入层进行工业化评审，统一说明评审范围、发布门禁、风险权重、证据要求、评分标准和评审记录模板。

## 目录

1. [适用范围](#1-适用范围)
2. [评审依据与原理](#2-评审依据与原理)
3. [评审流程](#3-评审流程)
4. [发布门禁](#4-发布门禁)
5. [风险分级与评分权重](#5-风险分级与评分权重)
6. [证据包与时效](#6-证据包与时效)
7. [详细审核标准](#7-详细审核标准)
8. [测量合同与运行处置](#8-测量合同与运行处置)
9. [文件级审核清单](#9-文件级审核清单)
10. [签署矩阵（RACI）](#10-签署矩阵raci)
11. [评审记录模板](#11-评审记录模板)
12. [最小行动清单](#12-最小行动清单)
13. [机器可读评分卡示例](#13-机器可读评分卡示例)
14. [变更记录](#14-变更记录)

## 1. 适用范围

本文档适用于以下对象和场景：

- 对 `privacy_local_agent/privacy/classification*` 的新版本发布准入评审。
- 对 `classification_routes.py`、`classification_service.py`、`classification_grpc.py` 等接入层的契约评审。
- 规则集（`rule_set_version`）或合规模板变更后的复审。
- 生产事故复盘后的补充评审。
- 定期成熟度复评，用于跟踪证据是否过期、技术债是否收敛。

单元测试期间的临时自查直接使用 `make test` / `make lint`，不必执行完整评审流程。

## 2. 评审依据与原理

| 依据 | 评审中的用途 |
|---|---|
| ISO/IEC 25010 | 功能、性能、可靠性、安全性、可维护性和兼容性维度 |
| Google SRE | SLI、SLO、错误预算和运行处置 |
| OWASP ASVS / Top 10 | 输入校验、日志脱敏、鉴权和依赖安全 |
| SonarQube 质量门禁 | 覆盖率、复杂度、重复率等量化指标 |
| CMMI | 流程标准化、证据追溯和持续改进 |
| 等保 2.0 / GDPR / JR/T 0197 / GB/T 35273 | 合规模板和数据分级要求 |
| 本仓库既有实现与设计文档 | 三层漏斗、复合规则、异步任务、复核闭环和可观测性 |

评审遵循以下原则：

1. **证据优先于陈述**：没有可复现证据的描述不计分。
2. **门禁优先于均分**：先判断是否允许发布，再计算成熟度分数。
3. **版本绑定**：评审必须绑定 commit SHA 或镜像摘要。
4. **风险即权重**：处理的数据越敏感，安全和可靠性权重越高。
5. **时效即衰减**：证据会过期，过期后必须重跑或降级。
6. **责任到人**：每个维度必须有明确的 Accountable 角色。
7. **零知识优先**：评审制品只能使用合成、脱敏或不可逆摘要数据。

## 3. 评审流程

1. 固定一个不可变的发布候选版本，并记录 `release_id`。
2. 确定被评审模块可接触到的最高敏感度等级。
3. 完成第 4 节发布门禁；任一失败即禁止发布。
4. 按第 5 节选择风险对应的权重。
5. 按第 7 节逐项评分，并为每个分数附上证据。
6. 按第 10 节完成责任人签署。
7. 将评审记录归档到 `artifacts/classification/<release_id>/review.md`。

## 4. 发布门禁

任一门禁未通过即禁止发布。例外必须记录风险说明、影响范围、补偿控制、审批人和失效日期；到期后自动恢复为阻断项。

| 门禁 | 通过条件 | 最低证据 | 失败处置 |
|---|---|---|---|
| 敏感数据泄露 | 日志、指标、trace、异常和导出制品均无原始敏感值 | canary PII 端到端扫描报告 | 修复后重新全量扫描 |
| 规则集可追溯 | 结果可关联规则版本、参数来源和时间戳 | API / REST / gRPC 契约测试与审计样本 | 阻断发布 |
| Golden set 核心正确性 | L5 不漏报，其他标签达到约定阈值 | 版本化 golden set 报告 | 阻断或书面风险接受 |
| 依赖与代码安全 | 无未豁免的高危/严重漏洞，密钥扫描通过 | SCA / SAST / secret scan 报告 | 阻断发布 |
| 可恢复运行 | ML 依赖、模型或推理失败时仍能启动并降级 | 启动、降级和恢复测试 | 阻断发布 |
| 接口兼容 | REST、gRPC、Pydantic 契约与发布说明一致 | 契约测试和 proto 兼容检查 | 阻断发布 |

## 5. 风险分级与评分权重

按模块正常路径可能接触到的最高 `SensitivityLevel` 判定风险等级。

| 风险等级 | 判定依据 | 典型模块 |
|---|---|---|
| 高（L5） | 基因组、生物特征或多字段组合升级结果 | `classification_llm.py`、`classification_composite.py`、`classification_rule_engine.py` |
| 中（L4） | 身份证、病史等高风险字段，不直接处理基因组 | `classification_ner.py`、`classification_async.py` |
| 低（≤ L3） | 配置、模板或聚合结果 | `classification_utils.py`、`classification_models.py` |

门禁全部通过后，按下表计算加权总分。各维度得分范围为 0~10，缺少证据的子项记 0 分，不得标记为“未评估”后跳过。

| 维度 | 低风险 | 中风险 | 高风险 |
|---|---:|---:|---:|
| 功能质量 F | 25% | 20% | 15% |
| 性能 P | 15% | 15% | 10% |
| 可靠性 R | 15% | 20% | 25% |
| 安全运营 Sec | 10% | 15% | 25% |
| 可维护性 M | 20% | 15% | 12.5% |
| 工程化 E | 15% | 15% | 12.5% |

计算公式：

$$
S = w_F \cdot F + w_P \cdot P + w_R \cdot R + w_{Sec} \cdot Sec + w_M \cdot M + w_E \cdot E
$$

| 总分 | 结论 |
|---|---|
| 9.0~10.0 | 标杆（Benchmark） |
| 7.0~8.9 | 通过（Pass） |
| 5.0~6.9 | 有条件通过（Conditional） |
| 1.0~4.9 | 不通过（Fail） |

## 6. 证据包与时效

一次评审至少记录以下元数据：`release_id`、`rule_set_version`、`profile_digest`、运行环境、`dataset_id` 和带时区的 `executed_at`。

建议制品目录：

```text
artifacts/classification/<release_id>/
  manifest.json
  unit-test.xml
  coverage.xml
  benchmark.json
  security-scan.json
  observability-check.json
  review-sample.jsonl
```

| 证据类型 | 新鲜度窗口 | 过期处理 |
|---|---|---|
| Golden set 正确性报告 | 90 天；规则版本变更立即失效 | 重跑，评分暂降 |
| 性能基准 | 90 天；依赖、模型或硬件变更立即失效 | 重跑，评分暂降 |
| 安全扫描 | 30 天；CVE 库更新建议重扫 | 超期视为门禁未通过 |
| 泄露 canary | 日志、指标或导出逻辑变更即重跑 | 未重跑不得合并 |
| SLO / 告警演练 | 180 天 | 重新演练，否则工程化维度降级 |

制品不得包含生产样本或真实敏感值。`manifest.json` 应记录命令、退出码、工具版本、输入摘要和产物摘要。

## 7. 详细审核标准

评分口径：1~2 分表示缺失或存在反向实现；3~4 分表示基础实现但关键路径有缺口；5~6 分表示主流程可用但证据不足；7~8 分表示覆盖完整且可追溯；9~10 分表示标准化并可作为标杆。

### 7.1 功能质量 F

| 检查项 | 0 分 | 5 分 | 9~10 分 |
|---|---|---|---|
| 三层漏斗正确性 | 无法验证触发条件或已知漏判 L5 | 主路径有测试覆盖 | golden set 覆盖字段 / 记录 / 表三级，L5 漏报为 0 |
| 规则准确性 | 无测试或误报明显 | 校验和逻辑有单测 | 覆盖正例、边界值和已知误报 |
| 复合规则协同 | 未测试或存在字段组合误判 | 默认规则有测试 | 默认、自定义规则及层间协同均有回归测试 |
| 输入格式与降级 | 仅验证 JSON | 主要格式有冒烟测试 | JSON / DataFrame / Arrow / SecretFlow、空值和异常输入均有专项测试 |

### 7.2 性能 P

| 检查项 | 0 分 | 5 分 | 9~10 分 |
|---|---|---|---|
| 延迟测量 | 无基准测试 | 有脚本但环境不固定 | 预热后固定环境，报告 p50 / p95 / p99 和资源峰值 |
| 规则路径 | 未测量 | 有粗略数据 | p95 ≤ 5ms、p99 ≤ 10ms，连续 3 次达标 |
| 向量化吞吐 | 未验证一致性 | 单次测量 | ≥ 10000 行/s 且与标量结果一致 |
| LLM / NER | 无超时保护 | 有保护但未测试极端输入 | 按输入类型统计成功、超时、降级和错误 |

### 7.3 可靠性 R

| 检查项 | 0 分 | 5 分 | 9~10 分 |
|---|---|---|---|
| 降级容错 | 依赖缺失导致崩溃 | 有 NoOp 降级 | 所有降级路径均有故障注入测试 |
| 并发安全 | 无锁保护 | 有 `threading.Lock` | 双重检查锁定并通过并发压测 |
| 异步状态机 | 状态可能丢失或卡死 | 正常路径有测试 | PENDING→RUNNING→DONE/FAILED 全状态覆盖 |
| 持久化一致性 | 未测试并发写入 | 基本读写有测试 | 并发、异常中断和重启后数据均一致 |

### 7.4 安全运营 Sec

| 检查项 | 0 分（门禁失败） | 5 分 | 9~10 分 |
|---|---|---|---|
| Zero-Knowledge | 制品出现原始敏感值 | 有 `redact` 但未回归 | canary 端到端扫描无泄露并纳入 CI |
| 输入校验 | 公共入口缺少 Pydantic 校验 | 主要入口有校验 | REST / gRPC 参数均经 `model_validate` |
| 传输与鉴权 | 生产未启用且无风险接受 | 可配置但未验证 | TLS 1.2+、API Key / mTLS 均验证 |
| 依赖与代码安全 | 存在未豁免高危漏洞 | 有扫描但未设门禁 | SCA / SAST / secret scan 均纳入 CI |
| 合规模板 | 与声明标准不符 | 存在但未回归 | JR/T 0197、GB/T 35273、GDPR 均有测试 |

### 7.5 可维护性 M

| 检查项 | 0 分 | 5 分 | 9~10 分 |
|---|---|---|---|
| 静态检查 | 未接入或长期失败 | 本地可通过 | CI 强制执行 lint 和 typecheck |
| 测试覆盖率 | 无统计 | 有统计但低于基线 | 达到团队基线并跟踪关键模块 |
| 代码规范 | 类型标注、docstring 缺失 | 核心类型标注齐全 | type hints、双语 docstring、流程说明齐备 |
| 变更追溯 | 无法关联需求 / 缺陷 | 提交基本可追溯 | PR、测试映射和评分记录互相可追溯 |

### 7.6 工程化 E

| 检查项 | 0 分 | 5 分 | 9~10 分 |
|---|---|---|---|
| 可观测性 | 关键路径无指标 | 主路径有埋点 | Counter / Histogram / Gauge 覆盖关键路径且无敏感标签 |
| SLO 与告警 | 未定义或未演练 | 已定义但未验证 | 每项 SLI 均有阈值、动作并完成演练 |
| CI/CD | 手工发布、制品不可追踪 | 有 CI 但未关联证据 | CI 自动产出评审所需制品 |
| 版本与回滚 | 无版本号或无法回滚 | 有 `rule_set_version` | 影子模式、版本化参数和快速回滚均可用 |

## 8. 测量合同与运行处置

| 指标 | 测量合同 | 建议阈值 / 处置 |
|---|---|---|
| 规则字段延迟 | 预热后固定字段分布，至少 10,000 次，报告 p50 / p95 / p99 | p95 ≤ 5ms，p99 ≤ 10ms |
| 向量化吞吐 | 固定列数、行数和空值比例，至少运行 3 次取最差值 | ≥ 10,000 行/s，且与标量结果一致 |
| NER 延迟 | 固定模型、输入长度分桶，冷热启动分开 | 超阈值则降级或排队 |
| LLM 超时率 | 分组统计成功、超时、降级和错误 | 超时不得阻塞请求线程 |
| 异步任务可靠性 | 满载、重启、TTL 清理后核对终态 | 不丢失已受理任务，超限返回确定性错误 |
| 分类准确性 | 仅使用版本化 golden set | L5 漏报为 0 |

关键 SLO 应同时定义告警条件和首要动作，例如规则分类错误率超过阈值时检查 profile 并回滚规则版本；LLM 降级率升高时检查模型资源并转人工复核。错误预算耗尽后应暂停扩大流量或切换新规则集。

## 9. 文件级审核清单

| 文件 | 审核重点 |
|---|---|
| `classification.py` | 主编排链路、参数解析、审计信息 |
| `classification_models.py` | 数据模型、字段约束、序列化 |
| `classification_rule_engine.py` | 规则准确性、去重、规则追溯 |
| `classification_vectorized.py` | 向量化一致性和回退路径 |
| `classification_composite.py` | 复合规则、上下文升级 |
| `classification_async.py` | 状态机、TTL、并发控制 |
| `classification_review.py` | 复核闭环、导出、脱敏 |
| `classification_utils.py` | 模板、参数合并和校验 |
| `classification_ner.py` | NER 降级、实体映射、初始化 |
| `classification_llm.py` | 多模态推理、超时、线程安全、回退 |

## 10. 签署矩阵（RACI）

| 维度 | Responsible | Accountable | Consulted | Informed |
|---|---|---|---|---|
| 功能质量 F | 模块开发者 | 分类模块 Tech Lead | 测试负责人 | 产品 / 合规 |
| 性能 P | 模块开发者 | SRE Owner | 容量规划负责人 | Tech Lead |
| 可靠性 R | 模块开发者 | SRE Owner | Tech Lead | 值班团队 |
| 安全运营 Sec | 安全测试执行人 | 安全负责人 | 隐私合规负责人 | Tech Lead、产品 |
| 可维护性 M | 模块开发者 | Tech Lead | Reviewer | 团队全员 |
| 工程化 E | 平台 / DevOps 负责人 | SRE Owner | Tech Lead | 值班团队 |
| 一票否决门禁 | 安全测试执行人 | 安全负责人 | 法务 / 合规 | 全体干系人 |

未指定 Accountable 角色签字的评分不得作为发布依据。

## 11. 评审记录模板

```text
发布候选：<release_id, 例如 git:abc1234>
模块/文件：<被评审的文件或子系统>
风险等级：<低/中/高>
规则集版本：<rule_set_version>
评测集：<dataset_id，仅合成/脱敏数据>
环境：<CPU/GPU、内存、Python 与依赖版本>
执行时间：<executed_at, 带时区>

门禁：
  [PASS/FAIL] 敏感数据泄露 canary
  [PASS/FAIL] 审计可追溯性
  [PASS/FAIL] Golden set 核心正确性
  [PASS/FAIL] 安全扫描
  [PASS/FAIL] 降级与恢复
  [PASS/FAIL] 接口契约

加权评分（风险等级=<低/中/高>）：
  功能质量 F：__/10，证据：<artifact>
  性能 P：__/10，证据：<artifact>
  可靠性 R：__/10，证据：<artifact>
  安全运营 Sec：__/10，证据：<artifact>
  可维护性 M：__/10，证据：<artifact>
  工程化 E：__/10，证据：<artifact>
  加权总分 S：__/10

结论：可发布 / 不可发布 / 限制发布
亮点：
问题：
限制与风险接受：<issue 或 无>
复验负责人及日期：<owner, due date>
```

## 12. 最小行动清单

1. 建立规则、模板和复合规则的版本化 golden set，并接入 CI。
2. 将性能目标固化为可重复 benchmark 命令和 JSON 制品。
3. 增加日志、指标、trace 和导出制品的 canary 泄露回归测试。
4. 增加 ML 不可用、推理超时、任务池满载和持久化异常的故障注入测试。
5. 为每个 Prometheus 指标补齐 SLI / SLO、告警阈值、负责人和 runbook。
6. 将制品摘要、评分、门禁结果和风险接受记录绑定到 `release_id`。
7. 补齐 SCA / SAST / secret scan，并在 CI 中设置安全门禁。

## 13. 机器可读评分卡示例

评分卡应直接绑定现有工具链，减少人工抄录和主观偏差：

| 评分子项 | 命令或来源 | 制品 |
|---|---|---|
| 功能质量 | `make test` 或分类测试 | JUnit XML |
| 性能 | `python tests/benchmark_classification.py` | `benchmark.json` |
| 可维护性 | `make lint`、`make typecheck`、`make test-cov` | lint、mypy、coverage 报告 |
| 安全运营 | 分类安全测试与 SCA / SAST / secret scan | `security-scan.json` |
| 工程化 | CI 运行记录、指标和告警演练 | `observability-check.json` |

```yaml
release_id: git:<commit_sha>
module: privacy_local_agent/privacy/classification_llm.py
risk_tier: high
rule_set_version: "1.2.0"
evidence:
  functional: artifacts/classification/<release_id>/unit-test.xml
  performance: artifacts/classification/<release_id>/benchmark.json
  security: artifacts/classification/<release_id>/security-scan.json
  maintainability: artifacts/classification/<release_id>/coverage.xml
  engineering: artifacts/classification/<release_id>/observability-check.json
evidence_generated_at: <timestamp>
expires_at: <timestamp>
scores:
  functional: null
  performance: null
  reliability: null
  security: null
  maintainability: null
  engineering: null
```

## 14. 变更记录

| 日期 | 版本 | 变更说明 |
|---|---|---|
| 2026-07-25 | 1.0 | 初始版本，基于 design.md 第 13、14、15 章整理 |
| 2026-07-25 | 1.1 | 补充评审依据、原理和六维度审核标准 |
| 2026-07-27 | 1.2 | 合并重复章节，统一目录和章节编号 |
| 2026-07-27 | 1.3 | 将评分卡、证据时效和落地示例合并到统一评审流程 |
