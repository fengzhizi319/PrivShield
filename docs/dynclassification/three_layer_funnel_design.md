# 三层漏斗模型 + 置信度策略设计

## 1. 背景

### 1.1 现状

`dynclassification` 模块当前仅有 Layer-1 规则引擎（`ConfigurableRuleEngine`），
缺乏 NER 实体识别和 LLM 深度推理能力。旧模块 `privacy/classification/` 将被删除，
其三层漏斗逻辑需迁移至 `dynclassification`。

### 1.2 目标

1. 为 `dynclassification` 增加三层漏斗模型（Rule → NER → LLM）
2. 实现置信度衰减策略（规则冲突时降低置信度）
3. 实现 LLM 仲裁能力（冲突时由 LLM 裁定）
4. 默认等级按标准独立配置（已有，无需修改）

---

## 2. 架构设计

### 2.1 三层漏斗执行流程

```
classify_field(field_name, value, domain, standard)
  │
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Layer-1: ConfigurableRuleEngine (确定性规则, 零延迟)                │
│    Phase 1: 普通规则评估 → normal_tags                              │
│    Phase 2: 降级规则评估 → downgrade_tags                           │
│    Phase 3: Override 压制 → 移除低等级普通标签                      │
│    Phase 4: 合并去重 → rule_tags                                    │
│    confidence = 1.0 (确定性)                                        │
└─────────────────────────────────────────────────────────────────────┘
  │
  │ 触发条件: enable_ner=true AND (无标签 OR 等级 <= 阈值)
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Layer-2: Small-NER 实体识别 (毫秒级延迟)                           │
│    提取医疗实体: 疾病/药物/手术/身体部位/基因提示                   │
│    映射为 SecurityTag (source_engine="SMALL_NER")                   │
│    confidence = NER 模型输出置信度 (0.6~0.95)                       │
└─────────────────────────────────────────────────────────────────────┘
  │
  │ 触发条件: 存在规则冲突 AND confidence_policy.enable_llm_arbitration
  │           OR enable_llm=true
  │           OR confidence < llm_confidence_threshold
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Layer-3: LLM 仲裁/深度分类 (秒级延迟, 可选)                       │
│    场景A: 规则冲突仲裁 → 裁定最终等级 + 修正置信度                  │
│    场景B: 低置信度兜底 → 深度语义理解分类                           │
│    confidence = LLM 输出置信度 (0.0~1.0)                            │
│    reasoning = LLM 推理过程                                         │
└─────────────────────────────────────────────────────────────────────┘
  │
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  置信度策略 (Confidence Policy)                                      │
│    无冲突: confidence = 1.0, needs_human_review = false             │
│    有冲突 + 无LLM: confidence = conflict_confidence (0.7)           │
│                    needs_human_review = true                         │
│    有冲突 + 有LLM: confidence = LLM 输出, 等级 = LLM 裁定          │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 冲突检测逻辑

```python
# 冲突定义: 普通规则标签和降级规则标签同时存活（未被 override 完全压制）
has_normal = any(t.source_engine == "RULE" and not t.is_override for t in tags)
has_downgrade = any(t.is_override or t.source_engine == "DOWNGRADE" for t in tags)
has_conflict = has_normal and has_downgrade
```

### 2.3 置信度策略配置 (taxonomy YAML)

```yaml
# rules/taxonomies/default.yaml
default_level: "L3"

confidence_policy:
  conflict_confidence: 0.7        # 规则冲突时的置信度
  conflict_needs_review: true     # 冲突时标记人工复核
  enable_llm_arbitration: false   # 是否启用 LLM 仲裁
  llm_confidence_threshold: 0.6   # LLM 触发阈值
  enable_ner: false               # 是否启用 NER 层
```

---

## 3. 新增文件

| 文件 | 职责 |
|---|---|
| `dynclassification/funnel.py` | 三层漏斗编排器（核心） |
| `dynclassification/ner_adapter.py` | NER 引擎适配器（lazy-load 旧模块） |
| `dynclassification/llm_adapter.py` | LLM 分类器适配器（lazy-load 旧模块） |

## 4. 修改文件

| 文件 | 变更 |
|---|---|
| `dynclassification/models.py` | 新增 `ConfidencePolicy`、`EngineLayer`；`FieldClassificationResult` 增加 `engine_layer`/`reasoning` |
| `dynclassification/service.py` | `classify_field` 改为调用 funnel；置信度计算逻辑 |
| `dynclassification/__init__.py` | 导出新符号 |
| `rules/taxonomies/*.yaml` | 增加 `confidence_policy` 配置节 |

---

## 5. 接口设计

### 5.1 ClassificationFunnel

```python
class ClassificationFunnel:
    """三层漏斗编排器。"""

    def __init__(self, engine, taxonomy, confidence_policy, ner_engine=None, llm_classifier=None):
        ...

    def classify_field(self, field_name, value) -> FunnelResult:
        """执行三层漏斗分类。"""
        ...
```

### 5.2 FunnelResult

```python
@dataclass
class FunnelResult:
    tags: list[SecurityTag]
    final_level: str
    confidence: float
    engine_layer: str          # "L1_RULE" | "L2_SMALL_NER" | "L3_LLM"
    needs_human_review: bool
    reasoning: str
    has_conflict: bool
```

### 5.3 NER/LLM 适配器接口

```python
class NerAdapter:
    """NER 引擎适配器（lazy-load）。"""
    def extract(self, text: str) -> list[dict[str, Any]]: ...

class LlmAdapter:
    """LLM 分类器适配器（lazy-load）。"""
    def classify(self, text: str, upstream_level: str, upstream_confidence: float) -> dict | None: ...
    def arbitrate(self, field_name: str, value: str, conflict_tags: list[SecurityTag], taxonomy: DomainTaxonomy) -> dict | None: ...
```

---

## 6. 降级策略

```
NER 不可用（onnxruntime 未安装/模型不存在）→ 跳过 Layer-2，直接进入 Layer-3 判断
LLM 不可用（torch 未安装/模型不存在）→ 使用 Phase 1 置信度衰减输出
LLM 超时（180s）→ 返回 None → 使用 Phase 1 置信度衰减输出
LLM JSON 解析失败 → 返回 None → 使用 Phase 1 置信度衰减输出
```

---

## 7. 默认等级配置（已有能力）

每个 taxonomy YAML 的 `default_level` 字段独立配置：
- 医疗: `default_level: "L3"`
- 金融: `default_level: "C3"`
- 未来教育/政务: 可设为 `"L2"` 或其他

无需额外修改。
