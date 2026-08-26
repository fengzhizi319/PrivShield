# 动态数据分类分级与三层漏斗仲裁系统技术指南 / Dynamic Classification & 3-Layer Funnel Technical Guide

## 1. 技术简介 / Introduction

在现代化数据要素流通与安全治理中，静态、硬编码的敏感字段匹配已无法应对海量多模态数据、复杂跨字段关联以及各行业监管合规标准差异（如 **GB/T 35273 个人信息规范**、**GB/T 43697 数据分类分级通用规则**、**JR/T 0197 金融数据安全规范**、**健康医疗大数据规范**）。

`PrivShield` 设计并实现了 **「三层四柱五御六类」** 治理架构中的核心大脑 —— **三层流式动态分类分级漏斗系统（Three-Layer Classification Funnel）**。

```text
                    待探查数据 (Field Name + Data Value)
                                     │
                                     ▼
      ┌─────────────────────────────────────────────────────────────┐
      │  ★ Layer 1: 确定性 AST 规则引擎 (L1_RULE)                     │
      │  - 纯 YAML 声明式配置 (rules/domains/*.yaml)                 │
      │  - 正则表达式 / 关键词树 / 范围算子 / Luhn 校验 / 熵值计算     │
      │  - 吞吐率: > 100,000 QPS, 延迟: < 0.1ms                      │
      └──────────────────────────────┬──────────────────────────────┘
                                     │ [未命中 / 置信度不足 / 存在降级冲突]
                                     ▼
      ┌─────────────────────────────────────────────────────────────┐
      │  ★ Layer 2: 轻量级命名实体识别 (L2_SMALL_NER)                 │
      │  - 基于 StructBERT / RoBERTa / ONNX Runtime Token 抽取      │
      │  - 无字段名提示下的非结构化文本敏感实体发现                   │
      │  - 吞吐率: ~2,000 QPS, 延迟: ~2ms                           │
      └──────────────────────────────┬──────────────────────────────┘
                                     │ [疑难冲突 / 语义多义 / 长尾小样本]
                                     ▼
      ┌─────────────────────────────────────────────────────────────┐
      │  ★ Layer 3: 本地大模型智能仲裁 (L3_LLM / VLM)                 │
      │  - 基于本地部署的 Qwen-3.5-0.8B/7B 结合 Chain-of-Thought 裁决  │
      │  - 结构化 JSON Schema 约束输出，严格防范幻觉                 │
      │  - 带有 Fail-Closed 安全底线 (Safety Floor) 拦截降级越权      │
      └──────────────────────────────┬──────────────────────────────┘
                                     │
                                     ▼
                      最终裁决安全标签 (SecurityTag)
                      [Category, Level, Confidence, Reasoning]
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 声明式规则模型与算子系统 / Rule Schema & Operator Registry

文件 / File：[`engine/dynclassification/rule_schema.py`](engine/dynclassification/rule_schema.py) & [`engine/dynclassification/operators.py`](engine/dynclassification/operators.py)

规则包完全与代码解耦，以 YAML 文件维护在 `rules/domains/`（领域规则）与 `rules/taxonomies/`（标准体系）中：

```yaml
# 示例：rules/domains/general-pii.yaml 中的身份证与手机号规则
domain: general-pii
rules:
  - id: pii_id_card_cn
    category: 个人身份信息
    level: S3
    confidence: 0.98
    match_logic: ALL
    matchers:
      - target: field_value
        operator: id_card_checksum
      - target: field_name
        operator: regex
        params:
          pattern: '(?i)(id_?card|identity|id_?no|sfz|idcard|身份证)'

downgrade_rules:
  - keywords: ["脱敏", "mask", "anon", "已隐藏", "测试"]
    level: S1
    force_suppress: false
```

#### 算子注册中心架构 (Operator Registry)

```python
class OperatorRegistry:
    """算子注册表：支持对字段名与字段值执行灵活的匹配与校验。"""
    _registry: dict[str, BaseOperator] = {}

    @classmethod
    def register(cls, name: str):
        def decorator(op_cls):
            cls._registry[name] = op_cls()
            return op_cls
        return decorator

    @classmethod
    def evaluate(cls, op_name: str, value: Any, params: dict[str, Any]) -> bool:
        op = cls._registry.get(op_name)
        if op is None:
            raise KeyError(f"Operator '{op_name}' not found")
        return op.match(value, params)
```

内置算子清单：
- `regex`：高性能正则预编译匹配；
- `keyword_contains` / `exact_match`：关键词与集合匹配；
- `id_card_checksum`：中国居民二代身份证 ISO 7064:1983.MOD 11-2 加权校验码校验；
- `luhn_checksum`：银行卡/信用卡 Luhn 校验；
- `icd10_range`：国际疾病分类第十次修订版 ICD-10 编码区间匹配；
- `entropy_threshold`：香农熵（Shannon Entropy）密码/密钥特征识别。

---

### 2.2 跨字段复合规则引擎 / Composite Rule Engine

文件 / File：[`engine/dynclassification/composite.py`](engine/dynclassification/composite.py)

单字段可能仅属于低敏感级别（例如单独的“姓名”为 S1），但当多个字段在同一张数据表中联合出现时，重识别风险将发生质变。复合规则引擎负责实现**多字段敏感度联合提级**：

```python
class CompositeRuleEngine:
    """评估记录级多字段组合升级规则。"""
    def evaluate(self, record_tags: dict[str, list[SecurityTag]]) -> list[SecurityTag]:
        """
        例如：[姓名 (S1)] + [手机号 (S2)] + [地理定位 (S2)] 
             联合触发复合规则 -> 提级判定为 [个人高敏轨迹档案 (S4)]
        """
        ...
```

---

### 2.3 三层漏斗编排器与仲裁算法 / Classification Funnel & Adjudication

文件 / File：[`engine/dynclassification/funnel.py`](engine/dynclassification/funnel.py)

#### 漏斗仲裁核心流程与 Fail-Closed 安全底线

```python
class ClassificationFunnel:
    """三层漏斗编排器：规则 -> NER -> LLM。"""

    def classify_field(self, field_name: str, value: Any) -> FunnelResult:
        # Step 1: Layer-1 规则引擎评估
        tags, suppressed_tags = self.engine.evaluate(field_name, value)
        confidence = max((t.confidence for t in tags), default=0.0)
        layer = EngineLayer.L1_RULE

        # Step 2: 冲突检测（如常规高敏规则与降级规则同时命中）
        has_conflict = self._detect_conflict(tags, suppressed_tags)

        # Step 3: Layer-2 NER (未命中或置信度低于阈值时触发)
        if self.policy.enable_ner and (not tags or confidence < self.policy.ner_trigger_threshold):
            ner_entities = self.ner_adapter.extract(str(value))
            if ner_entities:
                ner_tags = self._map_ner_to_tags(ner_entities)
                tags.extend(ner_tags)
                confidence = max((t.confidence for t in tags), default=0.0)
                layer = EngineLayer.L2_SMALL_NER

        # Step 4: Layer-3 LLM 仲裁或深度语义理解
        if has_conflict or confidence < self.policy.llm_confidence_threshold:
            if self.policy.enable_llm_arbitration and self.llm_adapter.is_available:
                llm_res = self.llm_adapter.arbitrate(field_name, str(value), candidates=tags)
                
                # ★ 安全底线约束 (Safety Floor / Fail-Closed):
                # LLM 仲裁等级必须属于候选标签集合之内；若 LLM 给出越权低等级，
                # 强制拒绝 LLM 降级并标记需要人工复核 (needs_human_review = True)
                if self._is_valid_arbitration(llm_res.level, tags):
                    return FunnelResult(
                        tags=llm_res.tags,
                        final_level=llm_res.level,
                        confidence=llm_res.confidence,
                        engine_layer=EngineLayer.L3_LLM,
                        reasoning=llm_res.reasoning,
                    )
                else:
                    logger.warning("LLM arbitration rejected by Safety Floor — fallback to highest rule level")
                    return FunnelResult(
                        tags=tags,
                        final_level=self._resolve_max_level(tags),
                        confidence=0.5,
                        engine_layer=layer,
                        needs_human_review=True,
                        reasoning="Safety Floor: LLM invalid downgrade rejected",
                    )

        # Step 5: 计算最终等级
        final_level = self._resolve_max_level(tags)
        return FunnelResult(
            tags=tags,
            final_level=final_level,
            confidence=confidence,
            engine_layer=layer,
        )
```

---

## 3. 多合规标准体系无缝切换 / Standards Switching

文件 / File：[`rules/taxonomies/`](rules/taxonomies/) & [`engine/dynclassification/models.py`](engine/dynclassification/models.py)

`PrivShield` 将“敏感分类/实体类型”与“安全等级（Rank/Level）”解耦：

| 标准体系 | 等级结构 (升序) | 默认等级 | 典型适用场景 |
|---|---|---|---|
| **GB/T 35273** | `C1 < C2 < C3 < C4` | `C1` | 个人信息保护、通用移动互联网 App 合规 |
| **GB/T 43697** | `L1 (一般) < L2 (重要) < L3 (核心)` | `L1` | 国家级数据资产分类分级与安全评估 |
| **JR/T 0197** | `J1 < J2 < J3 < J4` | `J1` | 银行、证券、保险金融机构高敏资产定级 |
| **四川健康医疗 (DB51)** | `S1 < S2 < S3 < S4 < S5` | `S1` | 区域医疗健康平台、医院 HIS/PACS/EMR 系统 |

客户端只需在请求中传入 `standard: "jrt0197"` 或 `standard: "gbt43697"`，漏斗系统将动态加载对应的 Taxonomy 与置信度策略，无需修改底层任何代码。

---

## 4. ConfigurableRuleEngine 四阶段评估管线 / 4-Phase Evaluation Pipeline

文件 / File：[`engine/dynclassification/engine.py`](engine/dynclassification/engine.py)

`ConfigurableRuleEngine` 是 Layer-1 的核心执行引擎，本身不包含任何领域知识，仅负责**解释执行**声明式规则配置。每次 `evaluate(field_name, value)` 调用经历四个阶段：

```text
  evaluate(field_name, value)
       │
       ▼
  ┌────────────────────────────────────────────────────────────────────┐
  │  Phase 0: LRU 缓存查找                                             │
  │    key = (field_name, str_value[:200] or (len, hash))              │
  │    命中 → 深拷贝返回（线程安全）                                      │
  │    未命中 → 继续 Phase 1                                            │
  └──────────────────────────────┬─────────────────────────────────────┘
                                 ▼
  ┌────────────────────────────────────────────────────────────────────┐
  │  Phase 1: 普通规则评估（按 priority 降序遍历）                        │
  │    for rule in sorted_rules:                                       │
  │      tag = _evaluate_rule(rule, field_name, str_value)             │
  │      → 短路优化: OR 命中即断 / AND 未命中即断                          │
  │      → 动态等级: ICD-10 算子可返回 OperatorResult(level=...)         │
  │    结果: normal_tags = [L5, L4, L3, ...]                           │
  └──────────────────────────────┬─────────────────────────────────────┘
                                 ▼
  ┌────────────────────────────────────────────────────────────────────┐
  │  Phase 2: 降级规则评估                                               │
  │    归一化字段名 → 遍历所有降级规则关键词子串匹配                         │
  │    结果: downgrade_tags = [L2(override), L1, ...]                   │
  └──────────────────────────────┬─────────────────────────────────────┘
                                 ▼
  ┌────────────────────────────────────────────────────────────────────┐
  │  Phase 3: Override 强制覆盖压制                                      │
  │    4 重判定条件:                                                     │
  │      ① 非降级标签                                                    │
  │      ② 非值级匹配标签 (match_target != 'field_value')                │
  │      ③ 等级 rank <= cap_rank                                        │
  │      ④ 未命中 exempt_rules 豁免模式 (fnmatch 通配符)                  │
  │    结果: (surviving_tags, suppressed_tags)                          │
  └──────────────────────────────┬─────────────────────────────────────┘
                                 ▼
  ┌────────────────────────────────────────────────────────────────────┐
  │  Phase 4: 合并 + 去重 (level, category) → 写入 LRU 缓存 → 返回       │
  └────────────────────────────────────────────────────────────────────┘
```

### 4.1 LRU 评估缓存 / Evaluation Cache

引擎使用 `OrderedDict` 实现真正的 LRU 缓存（非 TTL），容量由 `PRIVACY_ENGINE_CACHE_MAX_SIZE` 环境变量控制（默认 4096）：

```python
class ConfigurableRuleEngine:
    def __init__(self, ..., cache_max_size: int | None = None):
        # 线程锁保护缓存并发读写
        self._cache_lock = threading.Lock()
        # 缓存键: (field_name, str_value[:200]) 或 (field_name, (len, hash)) 用于长文本
        self._eval_cache: OrderedDict[...] = OrderedDict()

    def evaluate(self, field_name, value, context=None):
        cache_key = (
            field_name,
            str_value if len(str_value) <= 200 else (len(str_value), hash(str_value)),
        )
        # 缓存命中: move_to_end 标记为最近使用
        if cache_key in self._eval_cache:
            self._eval_cache.move_to_end(cache_key)
            return list(cached_final), list(cached_suppressed)
        # 缓存未命中: 执行完整 4 阶段评估
        ...
        # 写入缓存: 超容量时 popitem(last=False) 淘汰最久未使用
        if len(self._eval_cache) >= self._eval_cache_max_size:
            self._eval_cache.popitem(last=False)
        self._eval_cache[cache_key] = (final_tags, suppressed_tags)
```

**关键设计决策**：
- **长文本哈希键**：超过 200 字符的值使用 `(len, hash)` 作为键，避免前缀相同但后缀不同的值碰撞；
- **深拷贝返回**：缓存命中时返回 `list(cached_final)` 浅拷贝，防止调用方修改标签列表污染缓存；
- **context 旁路**：传入自定义 `context` 时跳过缓存（预留扩展场景）。

### 4.2 短路优化 / Short-Circuit Optimization

规则评估中的 AND/OR 逻辑均实现了短路优化，避免冗余算子调用：

```python
def _evaluate_rule(self, rule, field_name, str_value):
    is_or_logic = rule.match_logic.upper() == "OR"
    for matcher in rule.matchers:
        op_result = self._execute_matcher(matcher, field_name, str_value)
        results.append(op_result.hit)
        # OR 逻辑: 一个命中即可短路
        if is_or_logic and op_result.hit:
            break
        # AND 逻辑: 一个未命中即可短路
        elif not is_or_logic and not op_result.hit:
            break
```

---

## 5. 算子系统完整实现 / Complete Operator Implementation

文件 / File：[`engine/dynclassification/operators.py`](engine/dynclassification/operators.py)

### 5.1 算子注册与调用流程

所有算子通过 `@OperatorRegistry.register("name")` 装饰器在模块导入时自动注册。引擎通过 `OperatorRegistry.get(name)` 查找并调用。算子签名统一为 `(value: Any, params: dict) -> bool | OperatorResult`。

算子返回值经 `normalize_result()` 统一转换为 `OperatorResult`：

```python
@dataclass
class OperatorResult:
    hit: bool                        # 是否命中
    level: str | None = None         # 动态等级（如 ICD-10 区间升级）
    category: str | None = None      # 动态类别

def normalize_result(raw) -> OperatorResult:
    """统一转换 bool / tuple / OperatorResult 为标准 OperatorResult。"""
    if isinstance(raw, OperatorResult):
        return raw
    if isinstance(raw, bool):
        return OperatorResult(hit=raw)
    if isinstance(raw, tuple):  # 向后兼容 (hit, level, category)
        return OperatorResult(hit=raw[0], level=raw[1] if len(raw) > 1 else None, ...)
    return OperatorResult(hit=False)
```

### 5.2 内置算子完整清单

| 算子名称 | 功能 | 参数 | 安全特性 |
|---|---|---|---|
| `regex` | 正则表达式匹配 | `pattern` | ReDoS 缓解: 输入截断 256KB |
| `keyword_contains` | 关键词子串包含 | `keywords`, `use_word_boundaries` | 归一化匹配（去下划线/空格） |
| `prefix_match` | 前缀匹配 | `prefixes`, `case_insensitive` | 默认大小写不敏感 |
| `suffix_match` | 后缀匹配 | `suffixes`, `case_insensitive` | 默认大小写不敏感 |
| `id_card_checksum` | 身份证 GB 11643-1999 校验 | 无 | 18 位结构+加权校验码 |
| `medical_card_checksum` | 上海医保卡 9 位校验 | 无 | 加权取模校验 |
| `icd10_range` | ICD-10 编码区间判定 | `intervals`, `upgrade_level`, `default_level` | 返回动态等级 OperatorResult |
| `luhn_checksum` | 银行卡 Luhn 校验 | `min_length`, `max_length` | ISO/IEC 7812-1 |
| `length_range` | 字符串长度范围 | `min_length`, `max_length` | 辅助过滤器 |
| `exact_match` | 精确取值匹配 | `values` | 归一化后完全相等 |
| `ip_address` | IPv4/IPv6 地址判定 | 无 | RFC 格式校验 |
| `mac_address` | MAC 地址判定 | 无 | 6 组 2 位十六进制 |
| `chinese_name` | 中文姓名判定 | 无 | 2~4 字 CJK 表意文字 |
| `email` | 电子邮箱判定 | 无 | RFC 5322 简化版 |

### 5.3 ReDoS 缓解策略

正则算子对输入施加长度假硬限制，防止灾难性回溯：

```python
_REGEX_MAX_INPUT_LEN = 256 * 1024  # 256KB

@OperatorRegistry.register("regex")
def regex_matcher(value, params):
    if len(value) > _REGEX_MAX_INPUT_LEN:
        value = value[:_REGEX_MAX_INPUT_LEN]  # 截断缓解 ReDoS 放大
    try:
        return bool(re.search(pattern, value))
    except re.error:
        return False  # fail-safe: 非法正则视为未命中
```

### 5.4 ICD-10 动态等级算子

`icd10_range` 是唯一的**动态等级算子**——它不返回简单的 `bool`，而是返回携带 `level` 和 `category` 的 `OperatorResult`，使规则引擎能根据疾病编码区间自动调整敏感度等级：

```python
@OperatorRegistry.register("icd10_range")
def icd10_range_matcher(value, params):
    icd = _normalize_icd10(str(value))  # 解析 "B20.0" → ("B", 20)
    if not icd:
        return OperatorResult(hit=False)
    # 检查是否落入敏感区间（如传染病 A00-B99）
    for interval in params.get("intervals", []):
        if _in_icd10_interval(icd, interval["start"], interval["end"]):
            return OperatorResult(
                hit=True,
                level=params.get("upgrade_level", "L4"),  # 动态升级
                category=interval.get("category", ""),
            )
    # 非敏感区间: 普通等级
    return OperatorResult(hit=True, level=params.get("default_level", "L3"), ...)
```

---

## 6. ConfidencePolicy 置信度策略体系 / Confidence Policy System

文件 / File：[`engine/dynclassification/models.py`](engine/dynclassification/models.py#L80-L148)

`ConfidencePolicy` 控制三层漏斗之间的流转逻辑，所有字段均支持环境变量覆盖：

```python
class ConfidencePolicy(BaseModel):
    # 冲突时的衰减置信度（普通规则与降级规则同时命中时）
    conflict_confidence: float = 0.5

    # 冲突时是否标记人工复核
    conflict_needs_review: bool = True

    # 是否启用 LLM 仲裁（场景 A: 规则冲突时触发）
    enable_llm_arbitration: bool = True  # PRIVACY_LLM_ENABLE_ARBITRATION

    # LLM 触发阈值（场景 B: 置信度低于此值时触发 LLM 深度分类）
    llm_confidence_threshold: float = 0.75  # PRIVACY_LLM_CONFIDENCE_THRESHOLD

    # 是否启用 Layer-2 NER
    enable_ner: bool = False  # PRIVACY_NER_ENABLE

    # 是否显式启用 Layer-3 LLM（无论置信度）
    enable_llm: bool = False  # PRIVACY_LLM_ENABLE

    # 检测到图像输入时是否自动触发多模态 LLM（场景 C）
    auto_llm_on_image: bool = True  # PRIVACY_LLM_AUTO_ON_IMAGE

    # NER 触发阈值: 当前等级 rank <= 此值时触发 NER
    ner_trigger_max_rank: int = 3

    # 参与最终等级裁定的最低标签置信度
    min_tag_confidence: float = 0.5
```

### 6.1 三种 LLM 触发场景

```text
  场景 A: 规则冲突 (has_conflict=True)
    普通规则标签 + 降级标签共存且等级不一致
    → LLM 仲裁: 从冲突等级集合中选择裁定等级
    → Safety Floor: 裁定等级必须 ≥ 值级证据最大 rank
    → 失败回退: conflict_confidence=0.5 + needs_human_review=True

  场景 B: 低置信度 (confidence < llm_confidence_threshold)
    规则命中但置信度不足（如仅字段名弱匹配）
    → LLM 深度分类: 基于语义理解重新裁定等级
    → Safety Floor: LLM 等级不得低于上游规则等级（防降级逃逸）

  场景 C: 图像输入 (is_image_field_or_value=True)
    检测到 .jpg/.png/.dcm 等图像文件或 Base64 Data URI
    → 多模态 LLM 视觉识别（如 Qwen-VL）
    → LLM 不可用时: 按高敏医学影像保护 (L4/C4, confidence=0.95)
```

---

## 7. Safety Floor 安全底线机制 / Safety Floor Mechanism

文件 / File：[`engine/dynclassification/funnel.py`](engine/dynclassification/funnel.py)

Safety Floor 是漏斗系统中**最关键的安全约束**，确保 LLM 的输出永远不会绕过数据保护底线。它由四重防护组成：

### 7.1 值级证据安全地基 (Value-Level Evidence Foundation)

```python
# 若存在 field_value 证据标签（如身份证校验码通过），
# LLM 裁定的等级绝不可低于值级证据中的最大 rank
val_evidence_tags = [t for t in tags if t.match_target == "field_value" and not t.is_downgrade]
val_evidence_max_rank = max(
    (self.taxonomy.get_level_rank(t.level) for t in val_evidence_tags),
    default=0,
)
# 校验: llm_level_rank >= val_evidence_max_rank
```

**原理**：当数据值本身通过了校验（如 Luhn 校验通过 → 确认是银行卡号），这种证据比 LLM 的语义猜测更可靠，LLM 无权降级。

### 7.2 冲突集合约束

```python
# LLM 仲裁等级必须属于冲突标签等级集合
conflict_levels = {t.level for t in tags}
if llm_level not in conflict_levels:
    # 拒绝: LLM 给出了不在候选集合中的等级
    needs_human_review = True
```

**原理**：防止 LLM 幻觉出一个完全不相关的等级（如规则冲突在 L3 和 L5 之间，LLM 却给出 L1）。

### 7.3 LLM 置信度安全转换

```python
@staticmethod
def _safe_llm_confidence(raw: Any, fallback: float) -> float:
    """LLM 输出不可信：可能返回 "极高"、NaN、Inf 或 95.0。"""
    try:
        val = float(raw)
        if math.isnan(val) or math.isinf(val):
            return fallback
        if val > 100.0 or val < 0.0:
            return fallback  # 异常范围 → 回退，不 clamp
        if val > 1.0 and val <= 100.0:
            val = val / 100.0  # 容错 95.0 → 0.95
        return max(0.0, min(1.0, val))
    except (TypeError, ValueError):
        return fallback  # 非数值 → 回退
```

### 7.4 一致性压制与复核保护

LLM 仲裁成功后，对非 LLM 来源的标签执行一致性压制，但**绝不擦除值级证据标签**：

```python
surviving_tags = []
for t in tags:
    if (t.source_engine != "LLM" and not t.is_downgrade
        and t.level != llm_level and t.match_target != "field_value"):
        suppressed_tags.append(t)  # 压制不一致标签
    else:
        surviving_tags.append(t)   # 保留: LLM/降级/值级证据
```

低置信度仲裁成功时**不得覆盖既有复核标记**：

```python
has_surviving_review = any(t.needs_human_review for t in tags if t.source_engine != "LLM")
if confidence >= threshold and not has_surviving_review:
    needs_human_review = False
else:
    needs_human_review = needs_human_review or has_surviving_review
```

---

## 8. 降级规则与 Override 压制系统 / Downgrade & Override Suppression

文件 / File：[`engine/dynclassification/engine.py`](engine/dynclassification/engine.py#L414-L549)

### 8.1 降级规则执行

降级规则是一种"反向修正"机制。典型场景：字段名含 `turnover`（营业额）被通用规则误判为敏感数据，实际属于运营统计指标，应降级为 L2。

```python
def _evaluate_downgrade(self, field_name: str) -> list[SecurityTag]:
    norm_name = field_name.lower().replace("_", "").replace(" ", "")
    tags = []
    for rule in self.downgrade_rules:
        keywords = [kw.lower().replace("_", "").replace(" ", "") for kw in rule.keywords]
        if any(kw in norm_name for kw in keywords):
            tags.append(SecurityTag(
                level=rule.level,
                is_override=rule.force_suppress,  # 是否具有强制压制能力
                is_downgrade=True,
            ))
    return tags
```

### 8.2 Override 强制覆盖压制

当降级规则设置了 `force_suppress: true` 时，它会**强制压制**低于指定等级上限的普通规则标签。压制遵循 4 重判定条件：

```text
  压制 4 重判定条件:
  ┌─────────────────────────────────────────────────────────────┐
  │  ① 非降级标签 (is_override=False)                             │
  │     降级标签自身不会互相压制                                    │
  │                                                              │
  │  ② 非值级匹配标签 (match_target != 'field_value')              │
  │     数据值扫描结果默认豁免保底（如身份证校验通过 ≠ 可被压制）      │
  │                                                              │
  │  ③ 标签敏感等级 ≤ 覆盖上限 (tag_rank ≤ min_cap_rank)           │
  │     多条 override 命中时取最小 cap_rank（最弱压制能力）           │
  │     安全保守原则：例外应从严解释                                 │
  │                                                              │
  │  ④ 未命中 exempt_rules 豁免模式                                │
  │     支持精确匹配及 fnmatch 通配符（如 '*_EXACT'）               │
  │     命中豁免名单的规则受保护不被压制                              │
  └─────────────────────────────────────────────────────────────┘
```

---

## 9. DynClassificationService 服务层架构 / Service Layer Architecture

文件 / File：[`engine/dynclassification/service.py`](engine/dynclassification/service.py)

### 9.1 三级分类粒度

`DynClassificationService` 提供字段级、记录级和表级三种分类粒度：

```python
service = DynClassificationService(rules_dir="rules")

# 字段级: 单个字段名+值 → FieldClassificationResult
result = service.classify_field("phone_number", "13800138000")

# 记录级: 多字段字典 → RecordClassificationResult（含复合规则提级）
record = {"name": "张三", "id_card": "110101199001011237", "phone": "13800138000"}
result = service.classify_record(record)

# 表级: 多行记录 → TableClassificationResult
result = service.classify_table(schema=["name", "id_card"], rows=[...])
```

### 9.2 fork-after-warmup COW 优化

高并发场景下，Uvicorn 多 Worker 模式会导致每个 Worker 独立加载模型，内存翻倍。`PrivShield` 通过 fork-after-warmup 模式解决：

```python
# launcher.py --warmup 模式: 主进程 fork 前预加载模型
_preloaded_adapters: dict[str, Any] = {}

def register_preloaded_adapter(kind: str, adapter: Any) -> None:
    """注册预加载适配器（fork 前调用）。"""
    _preloaded_adapters[kind] = adapter

# 子进程 (Worker) 首次需要适配器时优先复用
funnel = self._build_funnel(engine)
# _build_funnel 内部:
preloaded = consume_preloaded_adapter("ner")
if preloaded is not None:
    self._ner_adapter = preloaded  # COW 共享只读模型页
    logger.info("ner_adapter_reused_preloaded")
```

**原理**：`fork()` 后子进程继承父进程的所有内存页（Copy-on-Write），模型权重页为只读，N 个 Worker 共享同一份物理内存，避免 N 倍内存膨胀。

### 9.3 高并发 LRU 分类结果缓存

服务层使用 `HighConcurrencyLRUCache`（线程安全的 LRU 缓存），容量由 `PRIVACY_CLASSIFICATION_CACHE_SIZE` 控制（默认 10000）：

```python
# 缓存键: (domain, standard, field_name, value_key, sanitize)
val_key = val_str if len(val_str) <= 200 else (len(val_str), val_str[:100], hash(val_str))
cache_key = (domain, standard, field_name, val_key, sanitize)
cached_resp = self._classification_cache.get(cache_key)
if cached_resp is not None:
    # 深拷贝共享可变对象，避免污染缓存
    field_result = cached_resp.field_result.model_copy(deep=True)
    return ClassificationResponse(field_result=field_result, audit_info=audit)
```

### 9.4 可插拔文本脱敏回调

服务层通过回调函数解耦领域特定脱敏逻辑：

```python
class DynClassificationService:
    def __init__(self, rules_dir, text_sanitizer=None):
        # text_sanitizer: (field_name, text, final_level) -> sanitized_text
        self._text_sanitizer = text_sanitizer

    def _fallback_text_sanitizer(self, field_name, text, final_level):
        """向后兼容: 医疗领域 L5/L4 模式替换 + PII 掩码 + 最终门禁检查"""
        try:
            s_text = text
            for pat, rep in L5_PATTERNS:  # L5 高敏替换
                s_text = pat.sub(rep, s_text)
            for pat, rep in L4_PATTERNS:  # L4 中敏替换
                s_text = pat.sub(rep, s_text)
            if final_level in ["L3", "L4", "L5"]:
                s_text = mask_value(field_name, s_text)  # PII 掩码
            if contains_high_risk_text(s_text):
                return "[L4-L5-DATA-REMOVED]"  # 最终门禁
            return s_text
        except Exception:
            return "[REDACTION-FAILED]"  # fail-closed: 脱敏失败绝不泄露原文
```

---

## 10. DomainTaxonomy 分类体系模型 / Taxonomy Model

文件 / File：[`engine/dynclassification/models.py`](engine/dynclassification/models.py#L197-L354)

`DomainTaxonomy` 是一个行业标准的分类分级元数据容器，支持多标准体系无缝切换：

```python
class DomainTaxonomy(BaseModel):
    domain: str               # 领域标识: 'healthcare', 'finance', 'gov'
    standard_id: str          # 标准编号: 'DB51_T_2989', 'JR_T_0197'
    version: str              # 体系版本号
    levels: dict[str, SensitivityLevelDef]   # 等级 ID → 等级定义
    categories: dict[str, CategoryDef]       # 分类 ID → 分类定义（支持多级树）
    default_level: str        # 无规则命中时的默认等级
    confidence_policy: ConfidencePolicy | None  # 置信度策略
    ner_entity_mapping: dict[str, str] | None   # NER 实体→等级映射
    ner_sensitive_keywords: list[str] | None    # NER 敏感关键词
    llm_arbitration_prompt_template: str | None  # LLM 仲裁 prompt 模板
```

### 10.1 等级比较与 max_level

`max_level()` 方法基于 `rank` 字段进行等级比较，rank 越大越敏感：

```python
taxonomy.max_level("L3", "L5", "L2")  # → "L5" (rank 最高)
```

未知等级 ID 会被**静默过滤**（fail-open）并记录警告日志，不会导致整个流程崩溃。

### 10.2 分类路径遍历

`get_category_path()` 通过 `parent_id` 链向上遍历，获取从根到叶的完整分类路径：

```python
# 例: "PERSONAL_IDENTITY" → ["PERSONAL", "PERSONAL_IDENTITY"]
path = taxonomy.get_category_path("PERSONAL_IDENTITY")
```

使用 `visited` 集合防止循环 `parent_id` 引用导致无限循环。

---

## 11. 复合规则引擎 / Composite Rule Engine

文件 / File：[`engine/dynclassification/composite.py`](engine/dynclassification/composite.py)

复合规则引擎在**记录级**执行，用于识别「单字段不敏感、多字段组合后敏感」的场景：

```python
class CompositeRuleEngine:
    def evaluate(self, record, field_results=None):
        norm_fields = {_normalize(name): name for name in record}
        for rule in self.rules:
            matched = 0
            for compiled in self._compiled_patterns[rule.id]:
                for norm_name, original_name in norm_fields.items():
                    if compiled.search(norm_name) or compiled.search(original_name):
                        matched += 1
                        break  # 该模式已匹配，检查下一个模式
                if matched >= rule.min_matches:
                    break  # 短路: 已满足阈值
            if matched >= rule.min_matches:
                tags.append(SecurityTag(
                    level=rule.target_level,
                    source_engine="COMPOSITE",
                    rule_id=rule.id,
                ))
        return tags
```

**关键设计**：
- **只升不降**：`apply_to_record_level()` 确保复合规则只能提级，不能降级
- **双端匹配**：同时匹配规范化名称和原始名称，兼顾 `id_card` 和 `ID-Card` 风格
- **原子分组包覆**：pattern 编译时使用 `(?:pattern)` 防止 `|` 交替符破坏词边界

---

## 12. REST API 与调用示例 / REST API Examples

文件 / File：[`engine/routers/dynclassification.py`](engine/routers/dynclassification.py)

### 12.1 字段级分类请求

```bash
curl -X POST http://127.0.0.1:8079/dynclassification/field \
  -H "Content-Type: application/json" \
  -d '{
    "field_name": "bank_card_no",
    "value": "6222021234567890123",
    "standard": "jrt0197",
    "sanitize": true
  }'
```

响应示例：
```json
{
  "fieldResult": {
    "fieldName": "bank_card_no",
    "fieldValue": "6222021234567890123",
    "tags": [
      {"level": "J3", "category": "FINANCIAL_ACCOUNT", "confidence": 1.0,
       "sourceEngine": "RULE", "ruleId": "fin_bank_card_luhn"}
    ],
    "finalLevel": "J3",
    "confidence": 1.0,
    "engineLayer": "L1_RULE",
    "reasoning": "命中规则: fin_bank_card_luhn",
    "sanitizedValue": "6222**** **** *123"
  },
  "auditInfo": {
    "domain": "finance",
    "standardId": "jrt0197",
    "rulesEvaluated": 42,
    "rulesHit": 1,
    "durationMs": 0.15
  }
}
```

### 12.2 Dry-Run 规则预演

```bash
curl -X POST http://127.0.0.1:8079/dynclassification/dry-run \
  -H "Content-Type: application/json" \
  -d '{
    "sample_data": [
      {"name": "张三", "phone": "13800138000", "diagnosis": "J06.9"},
      {"name": "李四", "id_card": "110101199001011237"}
    ],
    "domain": "medical"
  }'
```

返回命中分布统计：
```json
{
  "summary": {
    "total_records": 2,
    "total_fields": 5,
    "total_hits": 4,
    "hit_rate": 0.8,
    "rules_evaluated": 38
  },
  "level_distribution": {"L3": 2, "L4": 1, "L5": 1},
  "category_distribution": {"PII_NAME": 2, "PII_PHONE": 1, "MEDICAL_ICD10": 1}
}
```

### 12.3 管理接口

```bash
# 列出所有可用标准
curl http://127.0.0.1:8079/dynclassification/standards

# 列出标准详情（含等级体系）
curl http://127.0.0.1:8079/dynclassification/standards/detail

# 列出所有已注册算子
curl http://127.0.0.1:8079/dynclassification/operators

# 强制重载配置
curl -X POST http://127.0.0.1:8079/dynclassification/reload
```

---

## 13. 测试策略与最佳实践 / Testing Strategy

### 13.1 漏斗测试分层

```text
  tests/dynclassification/
  ├── test_funnel.py              # 三层漏斗端到端测试
  ├── test_engine.py              # 规则引擎 4 阶段测试
  ├── test_operators.py           # 算子单元测试
  ├── test_composite.py           # 复合规则测试
  ├── test_downgrade_override.py  # Override 压制测试
  ├── test_standards_switching.py # 多标准体系切换测试
  ├── test_ner_adapter.py         # NER 适配器测试 (Mock ML)
  ├── test_llm_adapter.py         # LLM 适配器测试 (Mock ML)
  └── test_safety_floor.py        # Safety Floor 安全底线测试
```

### 13.2 ML Mock 策略

NER/LLM 测试通过 `unittest.mock.patch` 模拟适配器返回值，避免加载真实模型：

```python
@patch("engine.dynclassification.ner_adapter.NerAdapter.extract")
def test_funnel_ner_layer(mock_extract):
    mock_extract.return_value = [
        {"text": "HIV", "label": "MEDICAL_DISEASE", "confidence": 0.95}
    ]
    funnel = ClassificationFunnel(engine, taxonomy, policy, ner_adapter=mock_ner)
    result, _ = funnel.classify_field("diagnosis", "患者HIV阳性")
    assert result.engine_layer == "L2_SMALL_NER"
    assert result.final_level in ("L5", "L4")  # 高敏疾病升级
```

### 13.3 Safety Floor 测试要点

- **LLM 降级拒绝**：LLM 返回低于规则等级的结果 → `needs_human_review=True`
- **值级证据保护**：`match_target="field_value"` 的标签不被 override 压制
- **置信度异常处理**：LLM 返回 `"极高"` / `NaN` / `Inf` / `95.0` → 安全转换
- **冲突集合约束**：LLM 返回不在冲突集合中的等级 → 拒绝并人工复核

---

## 14. 运维配置速查 / Operations Quick Reference

| 环境变量 | 默认值 | 用途 |
|---|---|---|
| `PRIVACY_ENGINE_CACHE_MAX_SIZE` | `4096` | 规则引擎 LRU 缓存容量 |
| `PRIVACY_CLASSIFICATION_CACHE_SIZE` | `10000` | 服务层分类结果 LRU 缓存容量 |
| `PRIVACY_NER_ENABLE` | `false` | 启用 Layer-2 NER 实体识别 |
| `PRIVACY_LLM_ENABLE` | `false` | 显式启用 Layer-3 LLM（无论置信度） |
| `PRIVACY_LLM_ENABLE_ARBITRATION` | `true` | 启用 LLM 冲突仲裁 |
| `PRIVACY_LLM_CONFIDENCE_THRESHOLD` | `0.75` | LLM 触发置信度阈值 |
| `PRIVACY_LLM_AUTO_ON_IMAGE` | `true` | 图像输入自动触发多模态 LLM |
| `PRIVACY_LLM_MAX_CONCURRENCY` | `1` (本地) / `16` (远程) | 进程级 LLM 推理并发上限 |
| `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | `30` | LLM 信号量排队超时 |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | `512` | 可用内存低于此值跳过 LLM |
