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
