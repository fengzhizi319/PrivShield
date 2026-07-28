# 动态分类分级（多标准适配）产品需求文档 (PRD)

## 1. 概述与背景

在原有的 `privacy-local-agent` 分类分级实现中，分类目录（`BusinessCategory`）、敏感等级（`SensitivityLevel`）以及具体的字段模式匹配逻辑均采用 Python 代码硬编码。随着部署环境的多样化，不同行业领域（如医疗健康 DB51/T 2989、金融行业 JR/T 0197、国家标准 GB/T 43697-2024、欧盟 GDPR 等）对分类维度与敏感分级存在截然不同的标准与合规要求。

本需求旨在构建**动态分类分级标准适配引擎**，实现引擎代码与行业分类标准、分级矩阵以及匹配规则的彻底解耦。新行业接入或标准变更时，只需更新 YAML 规则配置文件，无需重构或重新发布 sidecar 引擎服务。

---

## 2. 产品设计目标

- **零代码接入新标准**：新增行业/标准仅需添加 YAML 配置文件，无需修改 Python 引擎代码。
- **分类体系配置化**：等级定义（L1~L5 / C1~C4 / 1~4级）和分类目录树均由元数据 YAML 配置驱动。
- **匹配算子插件化**：通用算子（`regex`、身份证校验、Luhn 校验等）一次注册，多领域规则共享复用，并支持运行时自定义扩展。
- **运行时动态上下文**：请求支持通过 `domain` 或 `standard` 上下文参数动态切换规则集。
- **配置热重载**：支持在不重启 Sidecar 服务的前提下进行配置热加载与规则重载。
- **无缝向后兼容**：兼容现有分类接口契约，旧合规模板参数（`template`）可自动平滑映射到新标准体系。

---

## 3. 功能需求矩阵

| 需求 ID | 功能模块 | 需求描述 | 优先级 |
|---|---|---|---|
| DYN-TAX-1 | 分类体系配置 | 支持通过 YAML 定义行业领域的 Taxonomy，包含敏感等级集合（id, name, rank）、分类树目录结构及默认等级。 | P0 |
| DYN-TAX-2 | 多等级体系适配 | 支持医疗（L1~L5）、金融（C1~C4）、国标（1~4级）以及二分法等任意等级体系，并能基于 rank 自动计算最大敏感等级（`max_level`）。 | P0 |
| DYN-RULE-1 | 声明式规则包 | 支持以 `Domain Profile` 为单位组织领域规则库，每条规则指定匹配目标（字段名/字段值）、匹配算子（operator）及参数。 | P0 |
| DYN-RULE-2 | 逻辑组合与优先级 | 规则支持指定 `AND` / `OR` 多匹配算子逻辑，以及 `priority` 优先级控制执行次序。 | P0 |
| DYN-RULE-3 | 标准组合定义 | 支持通过 `Standard Profile` 将多个领域包（如 `general-pii` + `medical`）进行组合，并支持覆盖默认等级或追加特有规则。 | P0 |
| DYN-RULE-4 | 降级规则支持 | 支持针对公开数据、运营指标等关键字的标签降级规则机制。 | P1 |
| DYN-RULE-5 | 复合升级规则 | 支持结合多字段共存上下文（如“疾病”+“基因”）进行记录级分类升级的复合规则判定。 | P1 |
| DYN-OP-1 | 内置匹配算子库 | 内置提供 `regex`、`keyword_contains`、`prefix_match`、`id_card_checksum`、`medical_card_checksum`、`luhn_checksum`、`icd10_range` 等常用算子。 | P0 |
| DYN-OP-2 | 算子注册表 | 提供 `OperatorRegistry` 单例，支持使用 Python 装饰器（`@OperatorRegistry.register`）注册自定义匹配算子。 | P0 |
| DYN-ENG-1 | 通用规则引擎 | 提供 `ConfigurableRuleEngine`，根据转入的 `DomainTaxonomy` 和 `RuleProfile` 动态求值，不包含任何硬编码领域逻辑。 | P0 |
| DYN-LOAD-1 | Profile 加载与缓存 | 提供 `ProfileLoader` 支持配置文件加载、对象解析校验与 LRU 缓存调度。 | P0 |
| DYN-LOAD-2 | 规则热重载 | 提供 REST API（`POST /v1/dynclassification/profiles/reload`）及定时轮询触发规则缓存失效与重载。 | P1 |
| DYN-COMPAT-1 | 模板向下兼容 | 旧参数 `params.template` 能够自动转换映射至 `params.standard`，确保现有客户端不破坏。 | P0 |
| DYN-COMPAT-2 | 影子模式对比 | 提供影子模式（Shadow Mode），支持新旧引擎并行执行并对输出 Tag 差异进行审计警告，方便平滑迁移。 | P1 |
| DYN-METRICS-1 | 可观测性监控 | 暴露规则命中计数、算子调用频率、引擎加载耗时等 Prometheus 指标及结构化日志。 | P1 |

---

## 4. 接口契约与验收标准

### 4.1 REST / HTTP 接口验收标准
1. **POST `/v1/dynclassification/eval`**：
   - 当请求体传入 `{"domain": "finance", "standard": "jrt0197"}` 时，引擎应成功调用金融标准规则集并返回对应 `C1~C4` 级别的 `SecurityTag`。
   - 当请求传入旧参数 `"template": "sc_health_db51"` 时，引擎应自动映射为 `standard="sc_health_db51"` 并正常返回结果。

2. **POST `/v1/dynclassification/profiles/reload`**：
   - 触发热重载后，响应应返回成功与重新载入的 Profile 数量，后续请求应立即使用更新后的 YAML 配置。

3. **GET `/v1/dynclassification/standards` 与 `/v1/dynclassification/operators`**：
   - 应能列出当前环境中已注册的所有有效标准清单及匹配算子列表。

### 4.2 性能与稳定性验收标准
- **延迟指标**：单个字段的动态匹配延迟较旧版硬编码引擎增加不超过 5%（百微秒级）。
- **内存开销**：多 Standard Profile 缓存占用增加不超过 10 MB。
- **异常隔离**：若某个自定义算子执行抛出异常，引擎应有 try-catch 拦截防护，记录错误指标并安全跳过该规则，不得引发整个服务进程崩溃。
