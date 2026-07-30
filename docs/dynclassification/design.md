# 动态分类分级标准适配架构设计

本文档描述将 `privacy-local-agent` 中硬编码的数据分类分级逻辑改造为支持多领域、多行业标准通用适配的架构设计。核心设计思想为 **"标准配置化、规则声明化、算子插件化、执行上下文动态化"**，实现代码（引擎逻辑）与数据（行业分类标准、分级矩阵、匹配规则）的完全解耦。

## 目录

1. [概述与设计目标](#1-概述与设计目标)
2. [现状问题分析](#2-现状问题分析)
3. [设计原则](#3-设计原则)
4. [总体架构](#4-总体架构)
   - 4.4 [引擎层容错回退](#44-引擎层容错回退engine-layer-fallback)
   - 4.5 [敏感度降级规则（Downgrade Rules）](#45-敏感度降级规则downgrade-rules)
5. [数据模型设计：分类体系配置化](#5-数据模型设计分类体系配置化)
6. [声明式规则 Profile](#6-声明式规则-profile)
7. [算子插件化与注册表](#7-算子插件化与注册表)
8. [通用规则执行引擎](#8-通用规则执行引擎)
9. [Profile 管理与上下文调度](#9-profile-管理与上下文调度)
10. [向后兼容与迁移策略](#10-向后兼容与迁移策略)
11. [配置库目录结构](#11-配置库目录结构)
12. [API 接口变更](#12-api-接口变更)
13. [可观测性](#13-可观测性)
14. [测试策略](#14-测试策略)
15. [部署与运维](#15-部署与运维)
16. [扩展场景示例](#16-扩展场景示例)
17. [术语表](#17-术语表)

## 1. 概述与设计目标

### 1.1 背景

当前 `privacy-local-agent` 的数据分类分级模块已实现三层漏斗架构（规则引擎 → Small-NER → LLM），但规则定义、分类体系、合规模板均以 Python 代码硬编码方式实现。当需要接入新行业（如车联网、政务、教育）或新标准（如 GB/T 43697-2024）时，必须修改引擎源码并重新部署，无法满足"一套引擎适配多领域多标准"的产品化需求。

### 1.2 设计目标

| 目标 | 描述 |
|---|---|
| 零代码接入新标准 | 新增行业/标准仅需添加 YAML 配置文件，无需修改 Python 引擎代码 |
| 算子高度复用 | 通用算子（regex、身份证校验、Luhn 等）注册一次，跨领域复用 |
| 分类体系可配置 | 等级定义（L1~L5 / C1~C4 / 1~4级）和分类目录树均从配置加载 |
| 运行时动态切换 | 请求级参数指定 domain/standard，引擎按需加载对应规则集 |
| 热加载更新 | 规则配置支持运行时重载，无需重启服务 |
| 向后兼容 | 现有 REST/gRPC 接口契约不变，旧参数（template）自动映射 |
| 多租户支持 | 不同命名空间可绑定不同的领域/标准组合 |

## 2. 现状问题分析

### 2.1 硬编码清单

| 硬编码位置 | 文件 | 问题描述 |
|---|---|---|
| 规则引擎 evaluate() | `classification_rule_engine.py` L328-599 | 基因组/PII/ICD-10/文件格式规则以 if-else 硬编码 |
| 模板字段规则 | `classification_rule_engine.py` L601-757 | JR/T 0197、GB/T 35273、GDPR、DB51 模板规则硬编码 |
| 模板参数字典 | `classification_utils.py` L286-366 | TEMPLATES 字典写死在代码中 |
| 复合规则默认值 | `classification_composite.py` L83-120 | DEFAULT_RULES 硬编码 |
| 参数默认值 | `classification_models.py` L577-621 | ICD-10 区间、基因组关键词等写死为 Field default |
| 等级枚举 | `classification_models.py` L33-47 | SensitivityLevel 固定为 L1~L5 |
| 业务分类枚举 | `classification_models.py` L63-77 | BusinessCategory 固定为 DB51 五大类 |

### 2.2 核心矛盾

```mermaid
graph LR
    A[新行业标准] -->|当前| B[修改 Python 源码]
    B --> C[重新测试]
    C --> D[重新部署]
    A -->|目标| E[添加 YAML 配置]
    E --> F[热加载生效]
```

## 3. 设计原则

| 原则 | 说明 |
|---|---|
| 引擎与规则分离 | 引擎只做"解释执行"，不包含任何领域知识 |
| 领域包可组合 | 一个标准 = N 个领域包 + 参数覆盖 |
| 算子无状态 | 每个算子是纯函数 `(value, params) → bool`，无副作用 |
| 配置即代码 | YAML 规则文件可纳入版本控制、Code Review、CI 校验 |
| 渐进式迁移 | 旧 DefaultRuleEngine 保留为 fallback，新旧引擎可并行运行 |
| 约定优于配置 | 未指定 domain/standard 时使用默认领域包，行为与当前一致 |

## 4. 总体架构

### 4.1 架构总览

```mermaid
flowchart TD
    subgraph Client ["调用方上下文 (Context)"]
        Req["分类请求 (字段名, 字段值)"]
        Ctx["上下文参数: domain, standard"]
    end

    subgraph CoreEngine ["通用分类分级引擎 (Generic Classification Engine)"]
        Loader["1. Profile 加载器<br/>(Profile Loader)"]
        Engine["2. 通用匹配引擎<br/>(Rule Execution Pipeline)"]
        OpRegistry["3. 算子注册表<br/>(Operator Registry)"]
        Composer["4. 复合规则引擎<br/>(Composite Engine)"]
    end

    subgraph DataConfigs ["分类分级配置库 (Declarative Taxonomy & Rules)"]
        TaxonomyConf["领域分类体系定义 (Taxonomy)<br/>- 类别目录树 (Categories)<br/>- 敏感等级定义 (Levels)"]
        RuleConf["声明式规则库 (Rule Profiles)<br/>- 医疗标准规则集<br/>- 金融标准规则集<br/>- 政务/通用标准规则集"]
        StandardConf["标准组合定义 (Standard)<br/>- 领域包组合<br/>- 参数覆盖<br/>- 等级映射"]
    end

    subgraph Operators ["内置/自定义算子库 (Operators)"]
        OpRegex["regex_match"]
        OpID["id_card_checksum"]
        OpICD["icd10_range"]
        OpLuhn["luhn_checksum"]
        OpCustom["自定义算子..."]
    end

    Req --> Engine
    Ctx --> Loader
    Loader --> TaxonomyConf
    Loader --> RuleConf
    Loader --> StandardConf
    RuleConf --> Engine
    Engine --> OpRegistry
    OpRegistry --> Operators
    Operators --> Engine
    Engine --> Composer
    Composer --> Output["标准化分类结果 (ClassificationResult)"]
```
![img.png](img.png)
### 4.2 核心组件职责

| 组件 | 职责 | 对应新模块 |
|---|---|---|
| Profile Loader | 根据 domain/standard 加载并缓存规则配置 | `dynclassification/profile_loader.py` |
| Rule Execution Pipeline | 遍历声明式规则列表，调度算子执行匹配 | `dynclassification/engine.py` |
| Operator Registry | 管理所有已注册的匹配算子 | `dynclassification/operator_registry.py` |
| Composite Engine | 记录级字段组合规则后处理 | `dynclassification/composite.py` |
| Taxonomy Registry | 管理动态分类体系（等级 + 类别树） | `dynclassification/models.py` |

### 4.3 数据流

```mermaid
sequenceDiagram
    participant C as Client
    participant S as ClassificationService
    participant L as ProfileLoader
    participant E as ConfigurableRuleEngine
    participant O as OperatorRegistry

    C->>S: classify(field_name, value, {domain, standard})
    S->>L: get_profile(domain, standard)
    L-->>S: ResolvedProfile(taxonomy + rules)
    S->>E: evaluate(field_name, value, profile)
    loop 每条规则
        E->>O: get_operator(matcher.operator)
        O-->>E: operator_func
        E->>E: operator_func(target_value, matcher_params)
    end
    E-->>S: list[SecurityTag]
    S->>S: composite_engine.evaluate(record, tags)
    S-->>C: ClassificationResult
```

### 4.4 引擎层容错回退（Engine Layer Fallback）

为了保证分类服务的高可用性和鲁棒性，三层漏斗架构（规则引擎 → Small-NER → LLM）内置了自动容错回退机制。

> **注意**：此处的"降级"指的是**引擎可用性层面的回退**（高层引擎不可用时回退到低层引擎），与 4.5 节描述的**敏感度降级规则（Downgrade Rules）**是完全不同的概念。

- **定义**：当高级别的分类引擎（如 LLM 或 Small-NER）因模型未部署、文件缺失、加载失败或运行时资源不足而不可用时，系统会自动、平滑地回退（fallback）到下一级别的引擎进行处理，从而保证核心分类能力不中断。

- **回退路径**：`L3_LLM` → `L2_SMALL_NER` → `L1_RULE_ENGINE`。

- **实现方式**：系统采用"空对象模式"（No-Op Object Pattern）。在 `ClassificationService` 初始化时，会尝试加载各层引擎。如果某一层引擎加载失败（例如，`torch` 未安装或模型文件不存在），该层将被一个功能接口相同但行为为空的"假"对象（No-Op Classifier）替换。当分类请求流经该层时，这个"假"对象不会执行任何操作，而是直接将请求传递给下一层处理。

- **用户透明性**：整个回退过程对 API 调用者是透明的，服务不会因模型问题而中断或报错。但为了便于调试和监控，系统会在后台日志中明确记录回退事件，并且在最终返回的 `ClassificationResult` 的 `engine_layer` 字段中，会准确标识出**实际执行并产生结果**的引擎层级。例如，即使请求参数指定使用 LLM，如果 LLM 不可用，最终结果的 `engine_layer` 可能会是 `L1_RULE_ENGINE`。

### 4.5 敏感度降级规则（Downgrade Rules）

敏感度降级规则是 `ConfigurableRuleEngine` 中的一种**反向修正机制**，用于解决通用规则对特定字段的"过度分类"问题。当前实现已从单一的"兜底归属"演进为支持**可选的强制覆盖（override）**模式，允许 YAML 通过 `override`、`max_force_suppress_level`、`suppress_rules` 三个字段精确控制压制行为。

#### 4.5.1 设计动机

在实际业务中，部分字段虽然名称包含某些敏感关键词（如"统计"、"报告"），但其实际含义为机构运营指标或公开数据，不应被判定为高敏感等级。例如：

| 字段名 | 通用规则判定 | 实际语义 | 正确等级 |
|---|---|---|---|
| `turnover_rate`（营业额） | L3（敏感数据） | 机构运营统计指标 | L2（内部数据） |
| `outpatient_visits`（门诊人次） | L3（敏感数据） | 公开运营数据 | L2（内部数据） |
| `public_report`（公开报告） | L3（敏感数据） | 对外公开信息 | L1（公开数据） |
| `annual_summary`（年度汇总） | L3（敏感数据） | 对外公开信息 | L1（公开数据） |

降级规则通过在字段名中匹配特定关键词，将这些字段"下调"到合理的低等级。当启用 `override=true` 时，还能进一步**强制压制**误中的普通规则标签，避免宽泛规则把运营/公开字段错误地拉高等级。

> 说明：上表中的“通用规则判定”示例假设使用子串匹配语义。注意：降级规则使用纯子串匹配（`kw in norm_name`），而普通规则的 `keyword_contains` 算子默认使用单词边界匹配（`use_word_boundaries: true`），两者语义不同。

#### 4.5.2 数据模型

```python
# privacy_local_agent/dynclassification/rule_schema.py

class DowngradeRuleDef(BaseModel):
    """降级规则定义。

    当字段名匹配指定关键词时，将等级降级到目标等级。
    典型场景：公开字段降为 L1，运营统计字段降为 L2。

    强制覆盖模式（override=true）：
        默认情况下，降级规则仅作为"兜底归属"——在无普通规则命中时替代默认等级。
        当设置 override=true 后，降级规则可强制压制 rank <= max_force_suppress_level 的
        普通规则标签，解决宽泛规则误中运营/公开字段的问题。

        执行流程:
        ┌──────────────────────────────────────────────────────────────┐
        │  override=false (默认):                                       │
        │    降级标签 + 普通标签 → 取 max → 降级无效（仅兜底）          │
        │                                                              │
        │  override=true:                                              │
        │    先移除 rank <= cap 的普通标签 → 再取 max → 降级生效        │
        └──────────────────────────────────────────────────────────────┘
    """
    id: str                           # 规则唯一标识
    name: str = ""                    # 规则名称（人类可读）
    keywords: list[str]               # 触发降级的关键词列表（纯子串匹配，无单词边界限制）
    level: str                        # 降级目标等级（如 "L1"、"L2"）
    category: str                     # 降级后归属的业务类别
    match_target: str = "field_name"  # 匹配目标（目前仅支持字段名）
    override: bool = False            # 是否启用强制覆盖（压制普通规则标签）
    max_force_suppress_level: str = ""      # 覆盖等级上限（空=使用 level 字段自身）
    suppress_rules: list[str] = []     # 压制白名单：仅列出的 rule_id 可被压制（空=压制所有）
```

字段说明：

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `id` | `str` | — | 规则唯一标识，用于审计和指标。 |
| `name` | `str` | `""` | 人类可读名称。 |
| `keywords` | `list[str]` | `[]` | 触发关键词。字段名归一化（小写、去下划线/空格）后，使用**纯子串匹配**（`kw in norm_name`）。注意：降级规则不使用 `keyword_contains` 算子，无单词边界限制。 |
| `level` | `str` | — | 降级目标等级，如 `L1` / `L2`。 |
| `category` | `str` | — | 降级后归属的业务类别。 |
| `match_target` | `str` | `"field_name"` | 匹配目标，当前仅支持字段名。 |
| `override` | `bool` | `False` | 是否启用强制覆盖。`false` 为兜底模式，`true` 可压制普通规则。 |
| `max_force_suppress_level` | `str` | `""` | 覆盖等级上限（包含）。空字符串时默认使用 `level` 字段；否则使用此处指定的等级。只有 `rank <= cap_rank` 的普通标签会被移除。 |
| `suppress_rules` | `list[str]` | `[]` | 精细白名单。仅指定 `rule_id` 的普通规则允许被压制；空列表表示压制所有符合条件的普通规则。 |

#### 4.5.3 执行流程

降级规则在引擎 `evaluate()` 方法中的执行位置已扩展为四个阶段：

```mermaid
graph TD
    A["Phase 1: 遍历所有普通规则<br/>（按 priority 降序）"] --> B["Phase 2: 执行降级规则<br/>（所有降级规则均评估）"]
    B --> C["Phase 3: 强制覆盖裁定<br/>（仅 override=true 的降级规则生效）"]
    C --> D["Phase 4: 合并剩余标签与降级标签<br/>按 (level, category) 去重"]
    D --> E["返回 (final_tags, suppressed_tags)"]
```

具体执行逻辑：

1. **Phase 1 — 普通规则评估**：按优先级遍历普通规则，生成 `normal_tags`。
2. **Phase 2 — 降级规则评估**：字段名归一化后，使用纯子串匹配（`kw in norm_name`）对所有降级规则做关键词匹配（注意：降级规则不使用 `keyword_contains` 算子，无单词边界限制）；命中的生成 `downgrade_tags`，并标记 `is_downgrade=True`。若规则 `override=true`，则同时标记 `is_override=True`。
3. **Phase 3 — 强制覆盖裁定**：从 `downgrade_tags` 中筛选 `is_override=True` 的标签，计算 `cap_rank = min(rank(max_force_suppress_level))`；同时合并所有 `suppress_rules` 白名单。随后从 `normal_tags` 中移除满足以下全部条件的标签：
   - 该标签的 `match_target` 不是 `field_value`（基于字段值的命中不会被 override 压制，避免误删真实敏感内容）；
   - 该标签的 `rank <= cap_rank`；
   - 该标签的 `rule_id` 在 `suppress_rules` 白名单内（或白名单为空）。
   被移除的标签写入 `suppressed_tags`，用于审计和 Prometheus 指标。
4. **Phase 4 — 合并与去重**：将剩余普通标签与降级标签合并，按 `(level, category)` 去重，返回 `(final_tags, suppressed_tags)`。

#### 4.5.4 两种工作模式

降级规则有两种互斥的工作模式，由 `override` 字段控制：

| 模式 | `override` | 行为 | 典型场景 |
|---|---|---|---|
| **兜底模式** | `false` | 降级标签与普通标签共存，最终等级取 `max`。因此降级标签仅在无普通规则命中时才真正影响结果。 | 为未命中任何规则的运营/公开字段提供默认低等级，避免落入 `default_level`（如 L3）。 |
| **强制覆盖模式** | `true` | 先移除 `rank <= cap_rank` 的普通标签（可指定白名单），再取 `max`。降级规则可真正把被误中字段的等级拉低。 | 通用规则过于宽泛（如包含 `report` 就判定为敏感），需要把 `public_report` 等字段强制压回 L1。 |

两种模式的执行流程对比如下：

```
override=false (默认):
  normal_tags = [L4, L3]
  downgrade_tags = [L1]
  final = max(L4, L3, L1) = L4          # 降级规则仅兜底，不改变高等级结果

override=true, max_force_suppress_level="L3":
  normal_tags = [L4, L3]
  downgrade_tags = [L1(override)]
  cap_rank = rank("L3") = 3
  移除 rank <= 3 的普通标签 → normal_tags = [L4]
  final = max(L4, L1) = L4              # L4 高于 cap，不被压制

override=true, max_force_suppress_level="L4":
  normal_tags = [L4, L3]
  downgrade_tags = [L1(override)]
  cap_rank = rank("L4") = 4
  移除 rank <= 4 的普通标签 → normal_tags = []
  final = max(L1) = L1                  # 强制覆盖生效
```

#### 4.5.5 `max_force_suppress_level` 与 `suppress_rules` 精细控制

`max_force_suppress_level` 和 `suppress_rules` 共同决定强制覆盖的边界，避免一刀切：

- **`max_force_suppress_level`**：覆盖等级上限（包含）。仅 `rank <= cap_rank` 的普通标签会被移除。例如：
  - `level="L2"`、`max_force_suppress_level=""`：等价于 `cap_rank = rank("L2")`，只压制 L1/L2 的普通标签。
  - `level="L2"`、`max_force_suppress_level="L4"`：允许压制 L1~L4 的普通标签（将原本误标为 L3/L4 的误报标签强行压制并降级为 L2），但保留 L5（如果存在）。
  - **静态校验提示**：规则校验器（`validator.py`）会在检测到 `force_suppress=true` 但未指定 `max_force_suppress_level` 时主动输出 `[配置提示]` 告警，提醒配置人员如需压制更高等级误报（如将 L3/L4 强行降级为 L2），应显式指定 `max_force_suppress_level: "L3"` 或 `"L4"`。

- **`suppress_rules`**：白名单机制。当希望只压制特定宽泛规则，而不影响其他精确规则时使用：
  - `[]`（默认）：压制所有符合 rank 条件的普通标签。
  - `["RULE_PII_BROAD_KEYWORD"]`：仅压制该规则产生的标签，其他规则（如身份证校验）产生的标签即使 rank 较低也会保留。

组合逻辑（与 `force_suppress=true` 配合）：

```yaml
downgrade_rules:
  - id: "RULE_DOWN_PUBLIC"
    keywords: ["public_report", "annual_summary", "科普"]
    level: "L1"
    category: "PUBLIC_REPORT"
    force_suppress: true
    max_force_suppress_level: "L3"      # 只压制 L3 及以下普通标签
    suppress_rules: []             # 压制所有符合条件的普通规则

  - id: "RULE_DOWN_OPS"
    keywords: ["turnover_rate", "device_usage", "inventory", "门诊人次"]
    level: "L2"
    category: "OPERATIONAL_STAT"
    force_suppress: true
    max_force_suppress_level: ""          # 空=使用 level "L2" 作为 cap
    suppress_rules: ["RULE_PII_BROAD_KEYWORD"]  # 仅压制该宽泛规则
```

#### 4.5.6 与最终等级裁定的关系

最终等级裁定由 `ConfigurableRuleEngine` 或上层漏斗在 `_resolve_final_level()` 中完成：

- **兜底模式（`force_suppress=false`）**：降级标签与普通标签共同参与 `max_level`，因此降级标签**不会覆盖**高等级普通标签。核心价值是为无普通规则命中的字段提供低等级归属。
- **强制覆盖模式（`force_suppress=true`）**：`rank <= cap_rank` 的普通标签先被移除，再由剩余标签与降级标签取 `max`。因此降级规则可以**真正降低**被误中字段的最终等级。
- 无论哪种模式，被压制的标签都会作为 `suppressed_tags` 返回，可用于审计、Console 展示和 `DYNCLASSIFICATION_OVERRIDE_SUPPRESSED_TOTAL` 指标监控。
- 注意：`match_target == "field_value"` 的普通标签（即基于字段真实内容命中的规则，如身份证校验、银行卡号校验）不会被 override 压制，以保证真实敏感内容不被错误降级。

#### 4.5.7 YAML 配置示例

```yaml
# rules/domains/medical.yaml 中的降级规则部分
downgrade_rules:
  - id: "RULE_DOWN_PUBLIC"
    name: "公开报告降级"
    keywords: ["public_report", "annual_summary", "科普"]
    level: "L1"              # 降级为公开数据
    category: "PUBLIC_REPORT"
    force_suppress: true   # 启用强制覆盖
    max_force_suppress_level: "L3" # 允许压制 L3 及以下的普通标签

  - id: "RULE_DOWN_OPS"
    name: "运营统计降级"
    keywords: ["turnover_rate", "device_usage", "inventory", "门诊人次"]
    level: "L2"              # 降级为内部数据
    category: "OPERATIONAL_STAT"
    force_suppress: false  # 默认兜底模式
```

#### 4.5.8 与引擎层容错回退的区别

| 维度 | 引擎层容错回退（4.4 节） | 敏感度降级规则（本节） |
|---|---|---|
| 作用层面 | 引擎可用性 | 分类结果修正 |
| 触发条件 | 高层引擎不可用（模型缺失/加载失败） | 字段名包含特定关键词 |
| 影响对象 | 整个分类流水线 | 单个字段的敏感度等级 |
| 配置方式 | 代码内置（空对象模式） | YAML 声明式配置 |
| 对应代码 | `ClassificationService` 初始化 | `ConfigurableRuleEngine._evaluate_downgrade()` / `_apply_override_suppression()` |

## 5. 数据模型设计：分类体系配置化

### 5.1 设计动机

原有 `SensitivityLevel` 和 `BusinessCategory` 为 Python Enum 硬编码，无法适配不同行业的分级体系：

| 行业 | 分级体系 | 示例 |
|---|---|---|
| 医疗（DB51/T 2989） | L1~L5（5 级） | L4=敏感病种 |
| 金融（JR/T 0197） | C1~C4（4 级） | C4=第四级敏感 |
| 国标（GB/T 43697-2024） | 1~4 级 | 3级=敏感数据 |
| GDPR | Personal / Special Category | 二分法 |

### 5.2 动态分类体系模型

```python
# privacy_local_agent/dynclassification/models.py

from pydantic import BaseModel, Field
from typing import Optional


class SensitivityLevelDef(BaseModel):
    """动态敏感度等级定义。

    替代硬编码的 SensitivityLevel 枚举，支持任意等级体系。
    """
    id: str                    # 级别唯一标识，如 "L1", "C4", "LEVEL_3"
    name: str                  # 显示名称，如 "高敏感数据"
    rank: int                  # 排序权重（用于 max_level 比较逻辑）
    description: Optional[str] = None  # 等级说明


class CategoryDef(BaseModel):
    """动态分类类别定义。

    替代硬编码的 BusinessCategory 枚举，支持多级分类树。
    """
    id: str                    # 分类 ID，如 "PERSONAL_BASIC", "FINANCIAL_ACCOUNT"
    name: str                  # 分类名称，如 "个人基本信息"
    parent_id: Optional[str] = None  # 父分类 ID，支持多级树结构
    description: Optional[str] = None


class DomainTaxonomy(BaseModel):
    """领域分类体系完整定义。

    一个 Taxonomy 对应一个行业标准的分类分级元数据。
    """
    domain: str                # 领域标识，如 "healthcare", "finance", "gov"
    standard_id: str           # 标准编号，如 "DB51_T_2989", "JR_T_0197"
    version: str = "1.0.0"    # 体系版本号
    description: Optional[str] = None
    levels: dict[str, SensitivityLevelDef] = Field(default_factory=dict)
    categories: dict[str, CategoryDef] = Field(default_factory=dict)
    default_level: str = "L3"  # 无规则命中时的默认等级 ID

    def max_level(self, *level_ids: str) -> str:
        """返回等级集合中 rank 最高的等级 ID。"""
        if not level_ids:
            return self.default_level
        return max(level_ids, key=lambda lid: self.levels[lid].rank)

    def get_category_path(self, category_id: str) -> list[str]:
        """获取分类的完整路径（从根到叶）。"""
        path = []
        current = category_id
        while current and current in self.categories:
            path.append(current)
            current = self.categories[current].parent_id
        return list(reversed(path))
```

### 5.3 内置默认 Taxonomy（向后兼容）

```yaml
# rules/taxonomies/default.yaml
domain: "default"
standard_id: "INTERNAL"
version: "1.0.0"
description: "内置默认分类体系（兼容现有 L1~L5 + DB51 业务分类）"

levels:
  L1: {name: "公开数据", rank: 1, description: "无隐私风险"}
  L2: {name: "内部数据", rank: 2, description: "低敏感度"}
  L3: {name: "敏感数据", rank: 3, description: "中敏感度"}
  L4: {name: "高敏感数据", rank: 4, description: "需重点保护"}
  L5: {name: "极敏感数据", rank: 5, description: "最高级别保护"}

categories:
  PERSONAL_BASIC: {name: "个人基本信息"}
  MEDICAL_TREATMENT: {name: "诊疗信息"}
  FEE_BILLING: {name: "费用信息"}
  PUBLIC_HEALTH: {name: "公共卫生信息"}
  MANAGEMENT: {name: "管理信息"}
  GENOMIC: {name: "基因组信息", parent_id: "MEDICAL_TREATMENT"}
  FINANCIAL: {name: "金融信息"}

default_level: "L3"
```

```yaml
# rules/taxonomies/finance_jrt0197.yaml
domain: "finance"
standard_id: "JR_T_0197"
version: "1.0.0"
description: "JR/T 0197-2020 金融数据安全分级指南"

levels:
  C1: {name: "第一级（不敏感）", rank: 1, description: "公开可获取的金融数据"}
  C2: {name: "第二级（低敏感）", rank: 2, description: "内部使用的金融数据"}
  C3: {name: "第三级（敏感）", rank: 3, description: "涉及个人金融信息"}
  C4: {name: "第四级（高敏感）", rank: 4, description: "涉及核心金融账户"}

categories:
  FINANCIAL_ACCOUNT: {name: "金融账户信息"}
  FINANCIAL_TRANSACTION: {name: "金融交易信息"}
  PERSONAL_FINANCIAL: {name: "个人金融信息"}
  INSTITUTION_INTERNAL: {name: "机构内部信息"}

default_level: "C3"
```

## 6. 声明式规则 Profile

### 6.1 规则模型定义

```python
# privacy_local_agent/dynclassification/rule_schema.py

from pydantic import BaseModel, Field
from typing import Any, Optional


class MatcherDef(BaseModel):
    """单个匹配器定义。

    描述对字段名或字段值执行何种算子匹配。
    """
    target: str                # 匹配目标: "field_name" | "field_value"
    operator: str              # 算子名称: "regex" | "id_card_checksum" | "icd10_range" 等
    params: dict[str, Any] = Field(default_factory=dict)  # 算子参数（如 pattern、intervals）


class RuleDef(BaseModel):
    """单条声明式规则定义。"""
    id: str                    # 规则唯一标识
    name: str = ""             # 规则名称（人类可读）
    category: str              # 命中后的分类类别 ID
    level: str                 # 命中后的敏感度等级 ID
    matchers: list[MatcherDef] = Field(default_factory=list)
    match_logic: str = "AND"   # 多匹配器逻辑: "AND"(全部命中) | "OR"(任一命中)
    priority: int = 0          # 优先级（数值越大越先执行）
    enabled: bool = True       # 是否启用
    tags: dict[str, str] = Field(default_factory=dict)  # 扩展标签


class DowngradeRuleDef(BaseModel):
    """降级规则定义。"""
    id: str                    # 规则唯一标识
    name: str = ""            # 规则名称（人类可读）
    keywords: list[str]        # 触发降级的关键词列表
    level: str                 # 降级目标等级
    category: str              # 降级后归属的业务类别
    match_target: str = "field_name"  # 匹配目标
    override: bool = False     # 是否启用强制覆盖（压制普通规则标签）
    max_force_suppress_level: str = ""      # 覆盖等级上限（空=使用 level 字段）
    suppress_rules: list[str] = Field(default_factory=list)  # 压制白名单（空=压制所有）


class CompositeRuleDef(BaseModel):
    """复合规则定义（记录级）。"""
    id: str
    name: str = ""
    field_patterns: list[str]  # 字段名正则列表
    min_matches: int = 1       # 最低匹配数
    target_level: str          # 升级目标等级
    category: str


class RuleProfile(BaseModel):
    """规则 Profile 完整定义（一个领域包）。"""
    domain: str                # 所属领域
    version: str = "1.0.0"
    description: str = ""
    rules: list[RuleDef] = Field(default_factory=list)
    downgrade_rules: list[DowngradeRuleDef] = Field(default_factory=list)
    composite_rules: list[CompositeRuleDef] = Field(default_factory=list)


class StandardDef(BaseModel):
    """标准组合定义。

    一个标准 = 多个领域包组合 + 参数覆盖 + 等级映射。
    """
    standard_id: str           # 标准标识
    description: str = ""
    taxonomy: str              # 引用的 taxonomy 文件名
    domains: list[str] = Field(default_factory=list)  # 组合的领域包列表
    global_params: dict[str, Any] = Field(default_factory=dict)  # 全局参数覆盖
    rule_overrides: dict[str, dict[str, Any]] = Field(default_factory=dict)  # 规则级覆盖
    extra_rules: list[RuleDef] = Field(default_factory=list)  # 追加规则
```

### 6.2 医疗领域规则 Profile 示例

```yaml
# rules/domains/medical.yaml
domain: "medical"
version: "1.0.0"
description: "医疗健康领域分类规则（含基因组、ICD-10、敏感病种）"

rules:
  # --- 基因组字段名规则 ---
  - id: "RULE_MED_G_001"
    name: "BRCA/TP53 基因指标"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["brca1", "brca2", "tp53"]

  - id: "RULE_MED_G_002"
    name: "基因组变异指标"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["snp", "cnv", "genome", "genomic"]
      - target: "field_value"
        operator: "regex"
        params:
          pattern: "rs\\d+"
    match_logic: "OR"

  - id: "RULE_MED_G_003"
    name: "基因组文件格式"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["bam", "vcf", "fastq"]

  # --- 敏感病种字段名规则 ---
  - id: "RULE_MED_DISEASE_001"
    name: "敏感病种字段"
    category: "MEDICAL_TREATMENT"
    level: "L4"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["hiv", "aids", "std", "syphilis", "gonorrhea",
                     "psychiatric", "schizophrenia"]

  # --- ICD-10 值规则 ---
  - id: "RULE_MED_ICD10"
    name: "ICD-10 医疗编码"
    category: "MEDICAL_TREATMENT"
    level: "L3"
    matchers:
      - target: "field_value"
        operator: "icd10_range"
        params:
          default_level: "L3"
          upgrade_level: "L4"
          intervals:
            - {start: "B20", end: "B24", category: "MEDICAL_ICD10_HIV"}
            - {start: "A50", end: "A53", category: "MEDICAL_ICD10_STD"}
            - {start: "A54", end: "A64", category: "MEDICAL_ICD10_STD"}
            - {start: "F20", end: "F29", category: "MEDICAL_ICD10_PSYCHIATRIC"}
            - {start: "C00", end: "C97", category: "MEDICAL_ICD10_CANCER"}

  # --- 基因组文件内容检测 ---
  - id: "RULE_MED_FILE_BAM"
    name: "BAM 文件头检测"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_value"
        operator: "prefix_match"
        params:
          prefixes: ["BAM\u0001", "@SQ"]

  - id: "RULE_MED_FILE_VCF"
    name: "VCF 文件头检测"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_value"
        operator: "prefix_match"
        params:
          prefixes: ["##fileformat=VCF"]

  - id: "RULE_MED_SEQ"
    name: "碱基序列检测"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_value"
        operator: "regex"
        params:
          pattern: "[ATCGNatcgn]{50,}"

downgrade_rules:
  - id: "RULE_DOWN_PUBLIC"
    keywords: ["public_report", "annual_summary", "科普"]
    level: "L1"
    category: "PUBLIC_REPORT"
  - id: "RULE_DOWN_OPS"
    keywords: ["turnover_rate", "device_usage", "inventory"]
    level: "L2"
    category: "OPERATIONAL_STAT"

composite_rules:
  - id: "COMP_MED_001"
    name: "医疗基因组合"
    field_patterns: ["diagnosis|disease|illness", "gene|genomic|mutation|brca|tp53"]
    min_matches: 2
    target_level: "L5"
    category: "COMPOSITE_MEDICAL_GENOMIC"
```

### 6.3 金融领域规则 Profile 示例

```yaml
# rules/domains/finance.yaml
domain: "finance"
version: "1.0.0"
description: "金融行业分类规则 (JR/T 0197-2020)"

rules:
  - id: "RULE_FIN_ACCOUNT"
    name: "金融账户字段"
    category: "FINANCIAL_ACCOUNT"
    level: "C4"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["bankcard", "cardno", "credit", "transaction",
                     "asset", "balance", "account", "bank_card"]

  - id: "RULE_FIN_CARD_VALUE"
    name: "银行卡号值校验"
    category: "FINANCIAL_ACCOUNT"
    level: "C4"
    matchers:
      - target: "field_value"
        operator: "luhn_checksum"
        params:
          min_length: 16
          max_length: 19

composite_rules:
  - id: "COMP_FIN_001"
    name: "金融账户组合"
    field_patterns: ["bank_card|bankcard|card_no|account|credit|transaction"]
    min_matches: 1
    target_level: "C4"
    category: "COMPOSITE_FINANCE_COMBO"
```

### 6.4 通用 PII 领域规则 Profile 示例

```yaml
# rules/domains/general-pii.yaml
domain: "general-pii"
version: "1.0.0"
description: "通用个人信息规则 (GB/T 35273)"

rules:
  - id: "RULE_PII_IDCARD"
    name: "中国大陆身份证"
    category: "PERSONAL_BASIC"
    level: "L3"
    matchers:
      - target: "field_value"
        operator: "id_card_checksum"
        params: {}

  - id: "RULE_PII_PHONE"
    name: "中国大陆手机号"
    category: "PERSONAL_BASIC"
    level: "L3"
    matchers:
      - target: "field_value"
        operator: "regex"
        params:
          pattern: "^1[3-9]\\d{9}$"

  - id: "RULE_PII_MEDICAL_CARD"
    name: "医保卡号"
    category: "PERSONAL_BASIC"
    level: "L3"
    matchers:
      - target: "field_value"
        operator: "medical_card_checksum"
        params: {}

  - id: "RULE_PII_CONTACT"
    name: "联系方式/位置字段"
    category: "PERSONAL_BASIC"
    level: "L3"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["email", "address", "location", "轨迹"]

  - id: "RULE_PII_BIOMETRIC"
    name: "生物识别信息"
    category: "PERSONAL_BASIC"
    level: "L3"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["fingerprint", "voiceprint", "palmprint",
                     "iris", "face", "biometric"]

composite_rules:
  - id: "COMP_PII_001"
    name: "高敏感个人信息组合"
    field_patterns: ["^name$", "id_card|idcard|identity", "mobile|phone|cell"]
    min_matches: 3
    target_level: "L5"
    category: "COMPOSITE_PII_COMBO"
```

### 6.5 标准组合定义示例

```yaml
# rules/standards/sc_health_db51.yaml
standard_id: "sc_health_db51"
description: "DB51/T 2989—2023 四川省健康医疗大数据应用指南"
taxonomy: "default"  # 使用 L1~L5 体系
domains:
  - "general-pii"
  - "medical"
global_params:
  default_level: "L3"
rule_overrides:
  # 四川省指南将金融账户定为 L3（而非通用金融标准的 L4/C4）
  RULE_FIN_ACCOUNT:
    level: "L3"
extra_rules:
  - id: "RULE_DB51_MINOR"
    name: "未成年人信息"
    category: "PERSONAL_BASIC"
    level: "L3"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["minor", "child", "未成年", "儿童"]
  - id: "RULE_DB51_GENETIC_EXT"
    name: "四川指南遗传信息扩展"
    category: "GENOMIC"
    level: "L5"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["genetic", "chromosome", "embryo", "thalassemia",
                     "proteomics", "metabolomics", "omics"]
```

```yaml
# rules/standards/jrt0197.yaml
standard_id: "jrt0197"
description: "JR/T 0197-2020 金融数据安全分级指南"
taxonomy: "finance_jrt0197"  # 使用 C1~C4 体系
domains:
  - "general-pii"
  - "finance"
global_params:
  default_level: "C3"
```

## 7. 算子插件化与注册表

### 7.1 算子签名与注册表

```python
# privacy_local_agent/dynclassification/operator_registry.py

from typing import Any, Callable, Protocol


class MatcherOperator(Protocol):
    """匹配算子协议。

    所有算子必须实现此签名：接收待匹配值和参数字典，返回是否命中。
    算子必须是无状态纯函数，不持有实例变量。
    """
    def __call__(self, value: Any, params: dict[str, Any]) -> bool: ...


class OperatorRegistry:
    """算子注册表（单例）。

    管理所有已注册的匹配算子，支持装饰器注册和运行时动态注册。
    """

    _operators: dict[str, MatcherOperator] = {}

    @classmethod
    def register(cls, name: str):
        """算子注册装饰器。"""
        def decorator(func: MatcherOperator) -> MatcherOperator:
            cls._operators[name] = func
            return func
        return decorator

    @classmethod
    def register_func(cls, name: str, func: MatcherOperator) -> None:
        """运行时动态注册算子（支持插件热加载）。"""
        cls._operators[name] = func

    @classmethod
    def get(cls, name: str) -> MatcherOperator:
        """获取已注册算子。"""
        if name not in cls._operators:
            raise KeyError(f"未找到名为 '{name}' 的匹配算子，"
                          f"可用算子: {list(cls._operators.keys())}")
        return cls._operators[name]

    @classmethod
    def list_operators(cls) -> list[str]:
        """列出所有已注册算子名称。"""
        return list(cls._operators.keys())
```

### 7.2 内置算子实现

```python
# privacy_local_agent/dynclassification/operators.py

import re
from typing import Any
from .operator_registry import OperatorRegistry


@OperatorRegistry.register("regex")
def regex_matcher(value: Any, params: dict[str, Any]) -> bool:
    """正则表达式匹配算子。"""
    if not isinstance(value, str) or not value:
        return False
    pattern = params.get("pattern", "")
    return bool(re.search(pattern, value))


@OperatorRegistry.register("keyword_contains")
def keyword_contains_matcher(value: Any, params: dict[str, Any]) -> bool:
    """关键词匹配算子。

    将输入值与关键词均归一化（小写 + 去下划线/空格）后匹配。
    默认使用单词边界（\\b）进行匹配，避免 "report" 误中 "reported"；
    需要纯子串匹配时可配置 params.use_word_boundaries=false。

    params:
        keywords: list[str] - 关键词列表
        use_word_boundaries: bool - 是否使用单词边界（默认 True）
    """
    norm = str(value).lower().replace("_", "").replace(" ", "")
    keywords = params.get("keywords", [])
    use_word_boundaries = params.get("use_word_boundaries", True)

    if use_word_boundaries:
        for kw in keywords:
            if kw:
                pattern = r"\b" + re.escape(kw.lower().replace("_", "").replace(" ", "")) + r"\b"
                if re.search(pattern, norm):
                    return True
        return False
    else:
        return any(kw.lower().replace("_", "").replace(" ", "") in norm for kw in keywords if kw)


@OperatorRegistry.register("prefix_match")
def prefix_matcher(value: Any, params: dict[str, Any]) -> bool:
    """前缀匹配算子。"""
    if not isinstance(value, str):
        return False
    prefixes = params.get("prefixes", [])
    return any(value.startswith(p) for p in prefixes)


@OperatorRegistry.register("id_card_checksum")
def id_card_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """中国大陆 18 位身份证校验算子（GB 11643-1999）。"""
    # 复用现有 classification_rule_engine.py 中的校验逻辑
    from .classification_rule_engine import _id_card_checksum
    return _id_card_checksum(str(value) if value else "")


@OperatorRegistry.register("medical_card_checksum")
def medical_card_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """上海医保卡号校验算子。"""
    from .classification_rule_engine import _shanghai_medical_card_checksum
    return _shanghai_medical_card_checksum(str(value) if value else "")


@OperatorRegistry.register("icd10_range")
def icd10_range_matcher(value: Any, params: dict[str, Any]) -> bool:
    """ICD-10 编码格式校验算子（仅判断是否为合法 ICD-10 编码）。"""
    from .classification_rule_engine import _normalize_icd10
    return _normalize_icd10(str(value) if value else "") is not None


@OperatorRegistry.register("luhn_checksum")
def luhn_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """Luhn 算法校验算子（银行卡号通用校验）。"""
    s = str(value).strip() if value else ""
    min_len = params.get("min_length", 13)
    max_len = params.get("max_length", 19)
    if not s.isdigit() or not (min_len <= len(s) <= max_len):
        return False
    digits = [int(d) for d in s]
    odd_sum = sum(digits[-1::-2])
    even_sum = sum(sum(divmod(2 * d, 10)) for d in digits[-2::-2])
    return (odd_sum + even_sum) % 10 == 0
```

### 7.3 算子扩展机制

新增算子只需两步：

1. 实现符合 `MatcherOperator` 签名的函数
2. 使用 `@OperatorRegistry.register("算子名")` 注册

```python
# 示例：自定义车牌号算子
@OperatorRegistry.register("plate_number")
def plate_number_matcher(value: Any, params: dict[str, Any]) -> bool:
    """中国车牌号匹配算子。"""
    import re
    pattern = r"^[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤川青藏琼宁][A-Z][A-Z0-9]{5,6}$"
    return bool(re.match(pattern, str(value))) if value else False
```

## 8. 通用规则执行引擎

### 8.1 ConfigurableRuleEngine

```python
# privacy_local_agent/dynclassification/engine.py

from typing import Any, Tuple
from .models import DomainTaxonomy, SecurityTag
from .rule_schema import RuleProfile, RuleDef, MatcherDef, DowngradeRuleDef
from .operator_registry import OperatorRegistry


class ConfigurableRuleEngine:
    """通用可配置规则引擎。

    替代 DefaultRuleEngine 的硬编码逻辑，根据声明式 RuleProfile
    动态执行规则匹配。引擎本身不包含任何领域知识。
    """

    def __init__(
        self,
        taxonomy: DomainTaxonomy,
        profiles: list[RuleProfile],
        domain: str = "",
        standard_id: str = "",
    ):
        self.taxonomy = taxonomy
        self.domain = domain
        self.standard_id = standard_id
        # 合并所有领域包的规则，按 priority 降序排列
        self.rules = self._merge_rules(profiles)
        self.downgrade_rules = self._merge_downgrade_rules(profiles)

    def _merge_rules(self, profiles: list[RuleProfile]) -> list[RuleDef]:
        """合并多个领域包的规则列表。"""
        all_rules = []
        for profile in profiles:
            all_rules.extend(r for r in profile.rules if r.enabled)
        return sorted(all_rules, key=lambda r: r.priority, reverse=True)

    def _merge_downgrade_rules(
        self, profiles: list[RuleProfile]
    ) -> list[DowngradeRuleDef]:
        """合并多个领域包的降级规则列表。降级规则不按优先级排序。"""
        all_rules = []
        for profile in profiles:
            all_rules.extend(profile.downgrade_rules)
        return all_rules

    def evaluate(
        self, field_name: str, value: Any, context: dict[str, Any] | None = None
    ) -> Tuple[list[SecurityTag], list[SecurityTag]]:
        """评估单个字段，返回 (最终标签, 被压制标签)。

        执行流程：
        1. 遍历所有普通规则，生成 normal_tags
        2. 执行降级规则，生成 downgrade_tags（标记 is_override / is_downgrade）
        3. 对 override=true 的降级规则执行强制覆盖压制
        4. 合并剩余标签 + 降级标签，按 (level, category) 去重
        """
        str_value = str(value) if value is not None else ""

        # Phase 1: 普通规则评估
        normal_tags: list[SecurityTag] = []
        for rule in self.rules:
            tag = self._evaluate_rule(rule, field_name, str_value)
            if tag is not None:
                normal_tags.append(tag)

        # Phase 2: 降级规则评估
        downgrade_tags = self._evaluate_downgrade(field_name)

        # Phase 3: 强制覆盖压制（仅 override=true 的降级规则生效）
        surviving_tags, suppressed_tags = self._apply_override_suppression(
            normal_tags, downgrade_tags
        )

        # Phase 4: 合并 + 去重
        all_tags = surviving_tags + downgrade_tags
        return self._unique_tags(all_tags), suppressed_tags

    def _evaluate_rule(
        self, rule: RuleDef, field_name: str, str_value: str
    ) -> SecurityTag | None:
        """评估单条规则，命中则返回 SecurityTag，否则返回 None。"""
        if not rule.matchers:
            return None

        results = []
        for matcher in rule.matchers:
            hit = self._execute_matcher(matcher, field_name, str_value)
            results.append(hit)

        if rule.match_logic.upper() == "OR":
            matched = any(results)
        else:
            matched = all(results)

        if not matched:
            return None

        return SecurityTag(
            level=rule.level,
            category=rule.category,
            source_engine="RULE",
            rule_id=rule.id,
            domain=self.domain,
            standard_id=self.standard_id,
            match_target="field_value" if any(
                m.target == "field_value" for m in rule.matchers
            ) else "field_name",
        )

    def _execute_matcher(
        self, matcher: MatcherDef, field_name: str, str_value: str
    ) -> bool:
        """执行单个匹配器。"""
        op_func = OperatorRegistry.get(matcher.operator)
        target_value = field_name if matcher.target == "field_name" else str_value
        if target_value is None or target_value == "":
            return False
        return op_func(target_value, matcher.params)

    def _evaluate_downgrade(self, field_name: str) -> list[SecurityTag]:
        """执行敏感度降级规则（详见 4.5 节）。"""
        tags = []
        norm_name = field_name.lower().replace("_", "").replace(" ", "")
        for rule in self.downgrade_rules:
            keywords = [kw.lower().replace("_", "").replace(" ", "") for kw in rule.keywords]
            if any(kw in norm_name for kw in keywords):
                tags.append(SecurityTag(
                    level=rule.level,
                    category=rule.category,
                    source_engine="RULE",
                    rule_id=rule.id,
                    domain=self.domain,
                    standard_id=self.standard_id,
                    is_override=rule.override,
                    is_downgrade=True,
                ))
        return tags

    def _apply_override_suppression(
        self,
        normal_tags: list[SecurityTag],
        downgrade_tags: list[SecurityTag],
    ) -> Tuple[list[SecurityTag], list[SecurityTag]]:
        """对普通规则标签执行强制覆盖压制。

        返回 (surviving_tags, suppressed_tags)。
        """
        override_tags = [t for t in downgrade_tags if t.is_override]
        if not override_tags:
            return normal_tags, []

        # 计算所有 override 规则中最低的 cap_rank
        cap_ranks = []
        for tag in override_tags:
            cap_level = self._get_override_cap_level(tag.rule_id, tag.level)
            cap_rank = self.taxonomy.get_level_rank(cap_level)
            if cap_rank > 0:
                cap_ranks.append(cap_rank)
        if not cap_ranks:
            return normal_tags, []
        min_cap_rank = min(cap_ranks)

        # 合并 suppress_rules 白名单
        suppress_whitelist: set[str] = set()
        has_whitelist = False
        for tag in override_tags:
            rule_def = self._find_downgrade_rule(tag.rule_id)
            if rule_def and rule_def.suppress_rules:
                has_whitelist = True
                suppress_whitelist.update(rule_def.suppress_rules)
            elif rule_def and not rule_def.suppress_rules:
                has_whitelist = False
                suppress_whitelist.clear()
                break

        # 移除 rank <= cap_rank 的普通标签（field_value 命中除外）
        surviving_tags = []
        suppressed_tags = []
        for tag in normal_tags:
            if tag.match_target == "field_value":
                surviving_tags.append(tag)
                continue
            tag_rank = self.taxonomy.get_level_rank(tag.level)
            if tag_rank <= min_cap_rank:
                if has_whitelist and tag.rule_id not in suppress_whitelist:
                    surviving_tags.append(tag)
                    continue
                suppressed_tags.append(tag)
            else:
                surviving_tags.append(tag)

        return surviving_tags, suppressed_tags

    def _get_override_cap_level(self, rule_id: str, fallback_level: str) -> str:
        """获取降级规则的覆盖等级上限。"""
        rule = self._find_downgrade_rule(rule_id)
        if rule:
            return rule.max_force_suppress_level if rule.max_force_suppress_level else rule.level
        return fallback_level

    def _find_downgrade_rule(self, rule_id: str) -> DowngradeRuleDef | None:
        """根据 rule_id 查找降级规则定义。"""
        for rule in self.downgrade_rules:
            if rule.id == rule_id:
                return rule
        return None

    def _unique_tags(self, tags: list[SecurityTag]) -> list[SecurityTag]:
        """按 (level, category) 去重。"""
        seen = set()
        result = []
        for tag in tags:
            key = (str(tag.level), tag.category)
            if key not in seen:
                seen.add(key)
                result.append(tag)
        return result
```

### 8.2 ICD-10 特殊处理

ICD-10 规则需要动态返回等级（一般编码 L3，敏感区间 L4），通过 `icd10_range` 算子的扩展实现：

```python
@OperatorRegistry.register("icd10_range")
def icd10_range_matcher(value: Any, params: dict[str, Any]) -> Tuple[bool, str, str]:
    """ICD-10 编码区间判定算子。

    返回一个元组 (is_hit, level, category)：
    - is_hit: 是否为合法 ICD-10 编码
    - level: 动态等级（敏感区间返回 upgrade_level，否则返回 default_level）
    - category: 动态类别（敏感区间返回配置的 category，否则返回 MEDICAL_ICD10_GENERAL）
    """
    icd = _normalize_icd10(str(value) if value else "")
    if not icd:
        return False, "", ""

    intervals = params.get("intervals", [])
    for interval in intervals:
        if _in_icd10_interval(icd, interval["start"], interval["end"]):
            # 命中敏感区间：返回升级等级和类别
            level = params.get("upgrade_level", "L4")
            category = interval.get("category", "")
            return True, level, category

    # 未命中敏感区间：使用默认等级
    level = params.get("default_level", "L3")
    return True, level, "MEDICAL_ICD10_GENERAL"
```

## 9. Profile 管理与上下文调度

### 9.1 ProfileLoader

```python
# privacy_local_agent/dynclassification/profile_loader.py

import os
import threading
import time
from pathlib import Path
from typing import Optional
import yaml

from .models import DomainTaxonomy
from .rule_schema import RuleProfile, StandardDef, CompositeRuleDef
from .engine import ConfigurableRuleEngine
from .composite import CompositeRuleEngine


class ProfileLoader:
    """Profile 加载器与缓存管理器。

    负责从 YAML 文件加载 Taxonomy、RuleProfile、StandardDef，
    并根据 domain/standard 组合构建 ConfigurableRuleEngine 及 CompositeRuleEngine 实例。
    支持基于文件 mtime 的热重载（两阶段提交）与线程安全缓存。
    """

    def __init__(self, rules_dir: str | Path | None = None):
        # 解析规则目录：显式参数 > 环境变量 > 默认 "rules"
        env_rules_dir = os.environ.get("PRIVACY_DYNCLASSIFICATION_RULES_DIR", "rules")
        self.rules_dir = Path(rules_dir if rules_dir is not None else env_rules_dir)

        # 热重载配置
        self.hot_reload_enabled = (
            os.environ.get("PRIVACY_DYNCLASSIFICATION_HOT_RELOAD", "true").lower() == "true"
        )
        self.reload_interval_seconds = float(
            os.environ.get("PRIVACY_DYNCLASSIFICATION_RELOAD_INTERVAL", "0")
        )
        self._last_check_time = 0.0

        # 可重入锁：保护所有缓存变更和热重载扫描
        self._lock = threading.RLock()
        self._taxonomy_cache: dict[str, DomainTaxonomy] = {}
        self._profile_cache: dict[str, RuleProfile] = {}
        self._standard_cache: dict[str, StandardDef] = {}
        self._engine_cache: dict[str, ConfigurableRuleEngine] = {}
        self._composite_cache: dict[str, CompositeRuleEngine] = {}
        self._file_mtimes: dict[Path, float] = {}

    def check_and_reload(self, force: bool = False) -> bool:
        """检查文件 mtime 变动，触发两阶段提交热重载。线程安全。"""
        ...

    def load_taxonomy(self, name: str) -> DomainTaxonomy:
        """加载分类体系定义（带缓存 + RLock）。"""
        with self._lock:
            if name not in self._taxonomy_cache:
                path = self.rules_dir / "taxonomies" / f"{name}.yaml"
                data = yaml.safe_load(path.read_text(encoding="utf-8"))
                self._taxonomy_cache[name] = DomainTaxonomy.model_validate(data)
            return self._taxonomy_cache[name]

    def get_engine(self, domain=None, standard=None) -> ConfigurableRuleEngine:
        """获取或构建规则引擎实例（带 Prometheus 耗时指标）。"""
        cache_key = f"{domain or 'default'}:{standard or 'default'}"
        with self._lock:
            if cache_key not in self._engine_cache:
                engine = self._build_engine(domain, standard)
                self._engine_cache[cache_key] = engine
            return self._engine_cache[cache_key]

    def get_composite_engine(self, domain=None, standard=None) -> CompositeRuleEngine:
        """获取或构建复合规则引擎实例。"""
        ...

    def invalidate_cache(self) -> None:
        """清空所有缓存（热重载时调用）。"""
        with self._lock:
            self._taxonomy_cache.clear()
            self._profile_cache.clear()
            self._standard_cache.clear()
            self._engine_cache.clear()
            self._composite_cache.clear()
```

> 完整实现包含：两阶段提交热重载（`_perform_two_phase_reload`）、规则级属性覆盖（`_apply_rule_overrides`）、Taxonomy 一致性校验（`_validate_profile_taxonomy`）、Prometheus 指标集成、目录发现方法（`list_taxonomies`/`list_domains`/`list_standards`）。详见源码 `profile_loader.py`。

### 9.2 上下文调度集成

```python
# ClassificationService 改造（向后兼容）

class ClassificationService:
    """数据分类统一服务类（增强版）。"""

    def __init__(self, profile_path=None, rules_dir="rules"):
        self.profile_loader = ProfileLoader(rules_dir=rules_dir)
        # 保留旧 ClassificationAPI 作为 fallback
        self._legacy_api = ClassificationAPI(profile_path=profile_path)

    def classify_field(self, field_name, value, params=None):
        params = params or {}
        domain = params.pop("domain", None)
        standard = params.pop("standard", None)

        # 新路径：使用声明式引擎
        if domain or standard:
            engine = self.profile_loader.get_engine(domain, standard)
            tags = engine.evaluate(field_name, value)
            return self._build_result(tags)

        # 旧路径：兼容现有 template 参数
        template = params.get("template")
        if template:
            # 将旧 template 名映射到新 standard
            standard_mapping = {
                "gbt35273": "gbt35273",
                "gdpr": "gdpr",
                "jrt0197": "jrt0197",
                "sc_health_db51": "sc_health_db51",
            }
            mapped = standard_mapping.get(template)
            if mapped:
                engine = self.profile_loader.get_engine(standard=mapped)
                tags = engine.evaluate(field_name, value)
                return self._build_result(tags)

        # Fallback：使用旧引擎
        return self._legacy_api.classify_field(field_name, value, params)
```

## 10. 向后兼容与迁移策略

### 10.1 兼容映射表

| 旧参数 | 新参数 | 映射方式 |
|---|---|---|
| `params.template = "sc_health_db51"` | `params.standard = "sc_health_db51"` | 自动映射 |
| `params.template = "jrt0197"` | `params.standard = "jrt0197"` | 自动映射 |
| `params.icd10_l4_intervals` | 规则 YAML 中 `icd10_range.params.intervals` | 配置迁移 |
| `params.genomic_keywords` | 规则 YAML 中 `keyword_contains.params.keywords` | 配置迁移 |
| `params.composite_rules` | 规则 YAML 中 `composite_rules` 或请求级传入 | 双通道 |
| `SensitivityLevel.L3` | `taxonomy.levels["L3"]` | 枚举保留 + 动态扩展 |

### 10.2 渐进式迁移阶段

```mermaid
graph LR
    P1["Phase 1<br/>基础框架"] --> P2["Phase 2<br/>规则外迁"]
    P2 --> P3["Phase 3<br/>动态注入"]
    P3 --> P4["Phase 4<br/>旧引擎退役"]
```

| 阶段 | 内容 | 兼容性保证 |
|---|---|---|
| Phase 1 | 实现 `OperatorRegistry`、`ProfileLoader`、`ConfigurableRuleEngine`、`taxonomy.py`、`rule_schema.py` | 新模块独立，不影响现有代码 |
| Phase 2 | 将现有硬编码规则导出为 YAML 文件（medical/general-pii/finance），旧 template 映射到 standard | `DefaultRuleEngine` 保留为 fallback |
| Phase 3 | 支持请求级 `domain`/`standard`/`extra_rules` 动态注入；支持热加载 | 旧接口行为不变 |
| Phase 4 | 删除 `DefaultRuleEngine` 中的硬编码分支，全面切换到声明式引擎 | 大版本升级（v2.0） |

### 10.3 并行运行与影子对比

迁移期间支持新旧引擎并行，通过影子模式对比结果：

```python
# 影子模式：新旧引擎结果对比
if params.get("shadow_mode"):
    new_result = configurable_engine.evaluate(field_name, value)
    old_result = legacy_engine.evaluate(field_name, value, params)
    diff = compare_results(new_result, old_result)
    if diff:
        logger.warning("engine_shadow_diff", extra={"diff": diff})
```

## 11. 配置库目录结构

```text
rules/                              # 规则配置根目录
├── taxonomies/                     # 分类体系定义
│   ├── default.yaml                # 内置 L1~L5 + DB51 业务分类
│   ├── finance_jrt0197.yaml        # 金融 C1~C4 体系
│   └── gov_gb43697.yaml            # 国标 1~4 级体系
├── domains/                        # 领域规则包
│   ├── general-pii.yaml            # 通用 PII（身份证/手机号/地址）
│   ├── medical.yaml                # 医疗健康（基因组/ICD-10/敏感病种）
│   ├── finance.yaml                # 金融（银行卡/交易/资产）
│   ├── gov.yaml                    # 政务（公文/编制/统计）
│   └── iot-vehicle.yaml            # 车联网（轨迹/驾驶行为）
├── standards/                      # 标准组合定义
│   ├── sc_health_db51.yaml         # DB51/T 2989 四川健康医疗
│   ├── gbt35273.yaml               # GB/T 35273 个人信息安全
│   ├── gdpr.yaml                   # EU GDPR
│   ├── jrt0197.yaml                # JR/T 0197 金融数据
│   └── gb43697.yaml                # GB/T 43697-2024 数据安全技术
└── README.md                       # 配置编写指南
```

## 12. API 接口变更

### 12.1 请求参数扩展

现有接口契约不变，`params` 字典新增可选字段：

```json
{
  "field_name": "patient_brca1_status",
  "value": "阳性",
  "params": {
    "domain": "medical",
    "standard": "sc_health_db51",
    "extra_rules": [
      {
        "id": "CUSTOM_001",
        "name": "自定义规则",
        "category": "CUSTOM",
        "level": "L5",
        "matchers": [
          {
            "target": "field_name",
            "operator": "keyword_contains",
            "params": {"keywords": ["custom_marker"]}
          }
        ]
      }
    ]
  }
}
```

### 12.2 新增管理接口

| 接口 | 方法 | 说明 |
|---|---|---|
| `/v1/dynclassification/standards` | GET | 列出所有可用标准 |
| `/v1/dynclassification/domains` | GET | 列出所有可用领域包 |
| `/v1/dynclassification/operators` | GET | 列出所有已注册算子 |
| `/v1/dynclassification/profiles/reload` | POST | 热加载规则配置 |
| `/v1/dynclassification/validate` | POST | 校验规则 YAML 合法性 |

### 12.3 响应格式扩展

`SecurityTag` 新增可选字段：

```json
{
  "level": "L5",
  "category": "GENOMIC",
  "domain": "medical",
  "standard_id": "sc_health_db51",
  "rule_id": "RULE_MED_G_001",
  "source_engine": "RULE",
  "confidence": 1.0
}
```

## 13. 可观测性

### 13.1 Prometheus 指标

| 指标名 | 类型 | 标签 | 说明 |
|---|---|---|---|
| `classification_rule_hits_total` | Counter | `rule_id`, `domain`, `standard` | 规则命中计数 |
| `classification_operator_calls_total` | Counter | `operator`, `result` | 算子调用计数 |
| `classification_engine_load_duration_seconds` | Histogram | `domain`, `standard` | 引擎加载耗时 |
| `classification_profile_cache_size` | Gauge | — | 缓存的引擎实例数 |
| `classification_operator_errors_total` | Counter | `operator`, `rule_id` | 算子执行错误 |

### 13.2 结构化日志

```json
{
  "event": "rule_evaluation",
  "field_name": "brca1_status",
  "domain": "medical",
  "standard": "sc_health_db51",
  "rules_evaluated": 12,
  "rules_hit": 1,
  "hit_rule_ids": ["RULE_MED_G_001"],
  "duration_ms": 0.3
}
```

## 14. 测试策略

### 14.1 单元测试

| 测试对象 | 测试内容 |
|---|---|
| `OperatorRegistry` | 注册/获取/未注册异常/动态注册 |
| 各内置算子 | 正例/反例/边界值/None 输入 |
| `ConfigurableRuleEngine` | AND/OR 逻辑/空规则/降级规则/override 强制覆盖/去重 |
| `ProfileLoader` | YAML 加载/缓存命中/文件不存在/热加载 |
| `DomainTaxonomy` | max_level/category_path/空输入 |

### 14.2 集成测试

| 场景 | 验证点 |
|---|---|
| 旧接口兼容 | `template="sc_health_db51"` 结果与新引擎一致 |
| 新标准接入 | 仅添加 YAML 后 `standard="jrt0197"` 正常工作 |
| 请求级规则注入 | `extra_rules` 生效且不影响缓存 |
| 影子模式 | 新旧引擎差异正确记录 |
| 热加载 | 修改 YAML 后 reload 接口生效 |

### 14.3 规则 YAML Schema 校验

CI 中使用 `pydantic` 模型校验所有 YAML 文件：

```bash
PYTHONPATH=. python -m privacy_local_agent.dynclassification.validate_rules rules/
```

## 15. 部署与运维

### 15.1 配置挂载

```yaml
# Helm values 新增
classification:
  rulesDir: "/etc/privacy-agent/rules"
  hotReload: true
  reloadIntervalSeconds: 60

# ConfigMap 或 PVC 挂载规则目录
volumes:
  - name: classification-rules
    configMap:
      name: pla-classification-rules
```

### 15.2 热加载机制

`privacy-local-agent` 实现了基于文件系统修改时间（`mtime`）比对与延迟缓存失效（Lazy Invalidation）的平滑热加载机制。

#### 15.2.1 配置控制参数

热加载行为由以下环境变量和构造参数控制：

| 参数 / 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_DYNCLASSIFICATION_HOT_RELOAD` | `true` | 是否启用热重载文件变动监测 |
| `PRIVACY_DYNCLASSIFICATION_RELOAD_INTERVAL` | `0` (秒) | 两次文件变动检测之间的最小时间间隔（秒），用于高并发场景的 IO 节流 |
| `PRIVACY_DYNCLASSIFICATION_RULES_DIR` | `"rules"` | 规则配置根目录路径 |

#### 15.2.2 核心执行流程 (`ProfileLoader.check_and_reload`)

在 REST 路由与 gRPC 服务的请求入口阶段，系统会自动触发 `svc.loader.check_and_reload()`：

```mermaid
graph TD
    A[收到分类请求 / 触发 check_and_reload] --> B{hot_reload_enabled 为 True & rules_dir 存在?}
    B -- 否 --> C[跳过检测，返回 False]
    B -- 是 --> D{获取 RLock & 校验检查时间间隔}
    D -- "(now - last_check) < reload_interval" --> C
    D -- 达到检测间隔时间 --> E[递归扫描 rules_dir/**/*.yaml]
    E --> F{比对 yaml_file.stat().st_mtime 与 _file_mtimes}
    F -- 发现新文件或 mtime 改变 --> G[更新 _file_mtimes，设置 changed = True]
    F -- 文件无变化 --> H{changed == True?}
    G --> H
    H -- 是 --> I[调用 invalidate_cache 清空全部缓存]
    H -- 否 --> J[结束检测, 返回 False]
    I --> K[更新 Prometheus 监控指标，返回 True]
```

#### 15.2.3 关键逻辑说明

1. **并发安全锁（`threading.RLock`）**：采用可重入锁保护，避免在并发请求场景下多个线程同时扫描目录或清空缓存导致数据不一致。
2. **IO 节流与冷却期（`reload_interval_seconds`）**：在极高并发场景下，通过配置检查间隔，避免每个请求均触发 `stat()` 系统调用，降低 OS IO 损耗。
3. **`mtime` 戳比对**：
   - 扫描 `rules_dir` 下全部 `taxonomies/*.yaml`、`domains/*.yaml` 与 `standards/*.yaml`；
   - 记录 `yaml_file.stat().st_mtime` 到内部字典 `_file_mtimes`；
   - 一旦发现新增文件或已有文件时间戳发生改变，记录变动标记 `changed = True`。
4. **全量缓存清空与延迟按需构建（Lazy Invalidation & Re-building）**：
   - 当 `changed == True` 时，执行 `invalidate_cache()` 清空 `_taxonomy_cache`、`_profile_cache`、`_standard_cache`、`_engine_cache` 与 `_composite_cache`；
   - 同时将 Prometheus 指标 `DYNCLASSIFICATION_PROFILE_CACHE_SIZE` 重置为 `0`；
   - 清空缓存后，后续的分类请求在调用 `get_engine()` 或 `load_profile()` 时，会重新读取最新 YAML 配置，反序列化 Pydantic 模型并构建新的引擎实例，从而在无需重启 Python 进程的前提下完成零停机平滑更新。

### 15.3 规则版本管理

- 规则 YAML 文件纳入 Git 版本控制
- 每次变更通过 CI 校验（Schema + 单元测试）
- 支持 Git tag 标记规则集版本
- `AuditInfo.rule_set_version` 记录当前使用的规则版本

## 16. 扩展场景示例

### 16.1 接入车联网数据标准

无需修改任何 Python 代码，仅新增两个文件：

```yaml
# rules/domains/iot-vehicle.yaml
domain: "iot-vehicle"
version: "1.0.0"
description: "车联网数据分类规则"

rules:
  - id: "RULE_VIN"
    name: "车辆识别码 (VIN)"
    category: "VEHICLE_IDENTITY"
    level: "L3"
    matchers:
      - target: "field_value"
        operator: "regex"
        params:
          pattern: "^[A-HJ-NPR-Z0-9]{17}$"

  - id: "RULE_DRIVING_BEHAVIOR"
    name: "驾驶行为数据"
    category: "VEHICLE_BEHAVIOR"
    level: "L4"
    matchers:
      - target: "field_name"
        operator: "keyword_contains"
        params:
          keywords: ["speed", "acceleration", "brake", "steering", "trajectory"]

  - id: "RULE_PLATE"
    name: "车牌号"
    category: "VEHICLE_IDENTITY"
    level: "L3"
    matchers:
      - target: "field_value"
        operator: "plate_number"
        params: {}
```

```yaml
# rules/standards/iot_vehicle.yaml
standard_id: "iot_vehicle"
description: "车联网数据安全标准"
taxonomy: "default"
domains:
  - "general-pii"
  - "iot-vehicle"
global_params:
  default_level: "L3"
```

调用方式：

```json
{
  "field_name": "vehicle_trajectory",
  "value": "...",
  "params": {"standard": "iot_vehicle"}
}
```

### 16.2 多租户场景

不同命名空间绑定不同标准：

```yaml
# privacy-profile.yaml
namespaces:
  hospital-a:
    classification:
      standard: "sc_health_db51"
  bank-b:
    classification:
      standard: "jrt0197"
  default:
    classification:
      domains: ["general-pii", "medical"]
```

### 16.3 规则 GUI 管理（远期）

规则配置可持久化到数据库，通过 Console 管理界面供安全管理员配置：

```mermaid
graph LR
    A[Console GUI] -->|CRUD| B[规则 API]
    B -->|写入| C[MySQL/MongoDB]
    B -->|热加载通知| D[Agent Sidecar]
    D -->|重新加载| E[ProfileLoader]
```

## 17. 术语表

| 术语 | 说明 |
|---|---|
| Taxonomy | 分类体系，定义等级和类别的元数据结构 |
| Rule Profile | 规则配置文件，声明式描述一组匹配规则 |
| Domain Pack | 领域规则包，一个行业领域的规则集合 |
| Standard | 标准组合，由多个 Domain Pack + 参数覆盖构成 |
| Operator | 匹配算子，执行具体匹配算法的无状态纯函数 |
| Operator Registry | 算子注册表，管理所有可用算子的单例 |
| Profile Loader | 配置加载器，负责 YAML 解析、缓存和热加载 |
| ConfigurableRuleEngine | 通用规则引擎，解释执行声明式规则 |
| Downgrade Rules | 敏感度降级规则，通过字段名关键词匹配将过度分类的字段下调到合理低等级；支持 `force_suppress` 强制覆盖、`max_force_suppress_level` 与 `suppress_rules` 精细控制 |
| Engine Fallback | 引擎层容错回退，高层引擎不可用时自动回退到低层引擎 |
| Hot Reload | 热加载，运行时重新加载配置无需重启 |
| Shadow Mode | 影子模式，新旧引擎并行对比结果 |

## 相关文档

| 文档 | 路径 | 说明 |
|---|---|---|
| 现有分类设计 | [三层漏斗架构](../classification/design.md) | 三层漏斗架构详细设计 |
| 分类 PRD | [分类 PRD](../classification/prd.md) | 产品需求 |
| 分类运维 | [分类运维](../classification/ops.md) | 部署与配置 |
| 合规模板说明 | [合规模板说明](../classification/design.md) | 现有模板机制 |