# 三层漏斗模型 + 置信度策略设计

> 本文档与实现代码 `privacy_local_agent/dynclassification/funnel.py`、`models.py` 保持同步（最后对齐：2026-08）。

## 目录 (Table of Contents)

- [1. 背景](#1-背景)
  - [1.1 现状](#11-现状)
  - [1.2 目标](#12-目标)
- [2. 架构设计](#2-架构设计)
  - [2.1 三层漏斗执行流程](#21-三层漏斗执行流程)
  - [2.2 冲突检测逻辑](#22-冲突检测逻辑)
  - [2.3 置信度策略配置 (taxonomy YAML)](#23-置信度策略配置-taxonomy-yaml)
  - [2.4 Layer-3 Qwen 触发场景详解](#24-layer-3-qwen-触发场景详解)
  - [2.5 置信度 (Confidence Score) 计算与流转推导](#25-置信度-confidence-score-计算与流转推导)
  - [2.6 安全地板防御机制 (Safety Floor)](#26-安全地板防御机制-safety-floor)
  - [2.7 最终等级裁定优先级](#27-最终等级裁定优先级)
- [3. 新增文件](#3-新增文件)
- [4. 修改文件](#4-修改文件)
- [5. 接口设计](#5-接口设计)
  - [5.1 ClassificationFunnel](#51-classificationfunnel)
  - [5.2 FunnelResult](#52-funnelresult)
  - [5.3 NER/LLM 适配器接口](#53-nerllm-适配器接口)
- [6. 降级策略](#6-降级策略)
- [7. 默认等级配置（已有能力）](#7-默认等级配置已有能力)
- [8. 已知局限与后续优化方向](#8-已知局限与后续优化方向)

---

## 1. 背景

### 1.1 现状

`dynclassification` 模块已包含完整的三层漏斗架构（Rule → NER → LLM）。
旧模块 `privacy/classification/` 已删除（commit `ddc5b0e`），其三层漏斗逻辑已迁移至
`dynclassification`（`funnel.py`、`ner_engines.py`、`llm_engines.py` 等）。

### 1.2 目标

1. 为 `dynclassification` 增加三层漏斗模型（Rule → NER → LLM）
2. 实现置信度衰减策略（规则冲突时降低置信度）
3. 实现 LLM 仲裁能力（冲突时由 LLM 裁定）
4. 默认等级按标准独立配置（已有，无需修改）

---

## 2. 架构设计

### 2.1 三层漏斗执行流程

```
classify_field(field_name, value, sanitize=False)
  │
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Layer-1: ConfigurableRuleEngine (确定性规则, 零延迟)                │
│    Phase 1: 普通规则评估 → normal_tags                              │
│    Phase 2: 降级规则评估 → downgrade_tags                           │
│    Phase 3: Override 压制 → 移除低等级普通标签                      │
│    Phase 4: 合并去重 → rule_tags                                    │
│    补全扫描: L5 高敏医疗模式 (confidence=0.99, 强制人工复核) /       │
│              L4 高敏医疗模式 (confidence=0.95)                      │
│    confidence = max(命中规则置信度), 未命中则为 0.0                  │
│    (规则标签恒为确定性 1.0; RuleDef 无 confidence 字段)              │
└─────────────────────────────────────────────────────────────────────┘
  │
  │ 触发条件: enable_ner=true AND NER 适配器可用
  │           AND 通过智能门禁 (_should_trigger_ner)
  │           AND (无标签 OR 当前等级 rank <= ner_trigger_max_rank)
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Layer-2: Small-NER 实体识别 (毫秒级延迟)                           │
│    提取医疗实体: 疾病/药物/手术/身体部位/基因提示                   │
│    映射为 SecurityTag (source_engine="SMALL_NER")                   │
│    confidence = NER 模型原始 softmax 概率 (无截断/下限保障,          │
│                 多 token 实体取 min; 缺失时回退 0.8)                │
│    engine_layer 归属: 仅当 NER 实际影响决策时更新为 L2_SMALL_NER     │
│      (L1 无标签时 NER 提供首个分类结果，或 NER 等级高于 L1 结果)     │
└─────────────────────────────────────────────────────────────────────┘
  │
  │ 触发条件（三选一，按优先级短路）:
  │   场景A: 存在规则冲突 AND enable_llm_arbitration
  │   场景C: 检测到图像/影像输入 AND auto_llm_on_image
  │   场景B: confidence < llm_confidence_threshold AND enable_llm
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Layer-3: LLM 仲裁/深度分类 (秒级延迟, 可选)                       │
│    场景A: 规则冲突仲裁 → 裁定最终等级 + 修正置信度                  │
│    场景B: 低置信度兜底 → 深度语义理解分类                           │
│    场景C: 图像多模态识别 → 视觉深度分析分级                         │
│    confidence = LLM 输出置信度 (0.0~1.0, 非数值时回退上游置信度)     │
│    reasoning = LLM 推理过程                                         │
└─────────────────────────────────────────────────────────────────────┘
  │
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  置信度策略 (Confidence Policy)                                      │
│    无冲突: confidence = 1.0, needs_human_review = false             │
│    有冲突 + 无LLM: confidence = conflict_confidence (默认 0.5)      │
│                    needs_human_review = true                         │
│    有冲突 + 有LLM: confidence = LLM 输出, 等级 = LLM 裁定          │
│      (裁定等级必须落在冲突标签等级集合内, 否则拒绝并人工复核)        │
│    LLM 返回无合法等级: 保留上游置信度/层级归属,                      │
│      needs_human_review = true (见 2.6 安全地板第 3 条)              │
└─────────────────────────────────────────────────────────────────────┘
```

#### Layer-2 NER 智能门禁 (`_should_trigger_ner`)

遵循"简单规则先行，复杂长文本才用 NER"原则，避免 NER 对结构化短字段产生无效开销：

1. **排除空值/超短文本**：去空白后长度 < 2 不触发。
2. **排除纯数字/纯英文**：必须包含 ≥2 个连续中文汉字。
3. **排除 PII 结构化短字段**：`id_card_no`、`phone`、`name`、`patient_name`、`age`、`gender`、`sex`、`medical_insurance_no`、`social_security_no`、`disability_cert_no`、`registered_address`、`house_address`、`contact_phone`、`guardian_phone` 等直接复用 L1 规则。
4. **临床非结构化文书字段强制触发**：`chief_complaint`、`present_illness`、`past_history`、`personal_history`、`family_history`、`allergic_history`、`progress_note`、`diagnosis_name`、`diagnosis`。
5. 其余通过中文检查的文本默认触发。

### 2.2 冲突检测逻辑

```python
# 冲突定义: 普通规则标签和降级规则标签同时存在，且两者最高等级不一致。
# 若两者等级相同（如均为 L2），说明无实质矛盾，不判定为冲突。
normal_rule_tags = [t for t in tags if t.source_engine == "RULE" and not t.is_downgrade]
downgrade_tags = [t for t in tags if t.is_downgrade]

if normal_rule_tags and downgrade_tags:
    normal_max = taxonomy.max_level(*(t.level for t in normal_rule_tags))
    downgrade_max = taxonomy.max_level(*(t.level for t in downgrade_tags))
    has_conflict = normal_max != downgrade_max
else:
    has_conflict = False
```

> **注意**：降级标签通过 `SecurityTag.is_downgrade` 标志识别（降级规则产出时打标），
> 而非依赖 `is_override` 或 `source_engine` 字符串判断。

### 2.3 置信度策略配置 (taxonomy YAML)

```yaml
# rules/taxonomies/default.yaml（与实际配置文件一致，使用 snake_case 字段名）
default_level: "L3"

confidence_policy:
  conflict_confidence: 0.7        # 规则冲突且 LLM 不可用时的降级置信度（代码默认 0.5）
  conflict_needs_review: true     # 冲突时标记人工复核
  enable_llm_arbitration: false   # 是否启用 LLM 仲裁（env: PRIVACY_LLM_ENABLE_ARBITRATION，代码默认 true，需 ML 镜像）
  llm_confidence_threshold: 0.6   # LLM 触发阈值：置信度低于此值触发低置信度兜底（env: PRIVACY_LLM_CONFIDENCE_THRESHOLD，代码默认 0.75）
  enable_ner: false               # 是否启用 NER 层（env: PRIVACY_NER_ENABLE，默认 false）
  enable_llm: false               # 是否显式启用 LLM 深度分类（env: PRIVACY_LLM_ENABLE，默认 false）
  auto_llm_on_image: true         # 检测到图像/影像时自动触发多模态 LLM（env: PRIVACY_LLM_AUTO_ON_IMAGE，默认 true）
  ner_trigger_max_rank: 3         # NER 触发阈值：当前等级 rank <= 此值才触发 NER（C1~C4/G1~G4 四级体系建议设 2）
  min_tag_confidence: 0.5         # 参与最终等级裁定的最低标签置信度（低于此值仅作审计记录）
```

字段同时支持 camelCase 别名（如 `conflictConfidence`）与 snake_case 双向填充
（`populate_by_name=True`），布尔/数值类字段支持环境变量全局运维覆盖。
当前 `rules/taxonomies/` 下共四个已发布配置：default / gd_health / finance_jrt0197
均采用 `conflict_confidence: 0.7`、`llm_confidence_threshold: 0.6`、
`enable_llm_arbitration: false` 的保守配置；金融/广东医疗体系额外设置
`ner_trigger_max_rank: 2` 限制 NER 仅在低等级时触发。
第四个配置 `sc_health_db51.yaml`（四川医疗 DB51）**未定义 `confidence_policy` 节**，
运行时全部使用代码默认值——新增 taxonomy 时应显式补齐该节，避免隐式继承
`enable_llm_arbitration: true` 等代码默认行为。

### 2.4 Layer-3 Qwen 触发场景详解

第三层 Qwen（`Qwen3.5-0.8B-Privacy-Classifier-Smoother`）作为高精度语义裁决与无痕抹平重写引擎，在以下 3 种场景下被触发（按代码中的分支优先级排列）：

1. **场景 A：规则冲突仲裁 (Rule Conflict Arbitration)**
   - **触发条件**：`has_conflict = True` 且 `policy.enable_llm_arbitration = True` 且 LLM 可用。
   - **典型过程**：文本同时命中升级规则（如"肿瘤/高危病史"为 L4/L5）与降级规则（如"排除诊断/家族史"为 L2/L1），由 Qwen 从冲突等级集合中裁定最终敏感等级。
   - **约束**：裁定等级必须落在冲突标签等级集合内（见 [2.6 安全地板](#26-安全地板防御机制-safety-floor)）。
   - **短路说明**：`has_conflict = True` 时整个分支优先于场景 C/B；若仲裁未启用或 LLM 不可用，直接走置信度衰减，不会再检查图像/低置信条件。

2. **场景 C：图像/影像多模态识别 (Multimodal Image Analysis)**
   - **触发条件**：检测到图像输入且 `policy.auto_llm_on_image = True` 且 LLM 可用。图像判定（`_is_image_field_or_value`）覆盖三类信号：
     - 值以图像扩展名结尾：`.jpg .jpeg .png .bmp .webp .dcm .dicom .tiff`；
     - 值以 `data:image/` 或 `image:` 前缀开头（Base64 内联）；
     - 字段名含图像语义标识（英文 `image/photo/pic/picture/dicom/xray/ct_scan/mri/img` 词边界匹配；中文 `切片/病例图片/影像` 子串匹配），**且值长度 > 3 且非 http(s) URL**（防止把"图片链接字段说明文本"误判为图像）。
   - **典型过程**：对病例图片/DICOM 医学影像执行视觉深度分析，输出敏感分级。

3. **场景 B：低置信度兜底 (Low Confidence Fallback)**
   - **触发条件**：无冲突、非图像，且前两层累计置信度 `confidence < policy.llm_confidence_threshold`（代码默认 `0.75`）且 `policy.enable_llm = True`。
   - **典型过程**：文本未命中明确规则（如"他去拿了那个免疫靶向药"等隐晦语言），Qwen 承担深度语义理解与敏感分级。

> **关于无痕抹平 (Sanitization)**：抹平重写不是独立的 LLM 触发场景，而是贯穿场景 B/C 的
> 横切能力——调用方显式传入 `sanitize=True` 时，LLM 输出中附带 `sanitized_text`
> （如身份证号星号化 `330801********0789`、年龄 k-匿名化区间重写）；
> 图像输入则调用 `image_redaction` 模块生成打码产物，写入 `FunnelResult.sanitized_value`。

### 2.5 置信度 (Confidence Score) 计算与流转推导

置信度（取值 `0.0 ~ 1.0`）是量化评估判定确定性的核心指标，流转过程如下：

#### 1. Layer-1 规则引擎置信度
- **规则标签**（身份证号、手机号等 YAML 规则）：恒为确定性 `confidence = 1.0`
  （`RuleDef` 无 confidence 字段，`SecurityTag` 默认值）。
- **L5 高敏医疗模式**（补全扫描）：`confidence = 0.99`，且标签自带 `needs_human_review=True`。
- **L4 高敏医疗模式**（补全扫描）：`confidence = 0.95`。
- **未命中规则**：`tags` 为空，初始 `confidence = 0.0`。
- **阶段合并**：取所有命中规则的最大置信度 \(\text{confidence}_{\text{L1}} = \max(\{t.\text{confidence}\}, \text{default}=0.0)\)。

#### 2. Layer-2 Small-NER 实体识别置信度
- 提取出的实体归一化映射为 `SecurityTag`，附带 NER 模型**原始 softmax 输出概率**
  （多 token 实体取 token 概率最小值）。代码**不做任何截断/下限钳制**，
  观测值通常落在 `0.60 ~ 0.95` 区间，但这只是经验描述而非保证；
  引擎未返回置信度时回退默认 `0.8`（注意：高于 `min_tag_confidence` 默认 0.5，
  会参与最终等级计算）。
- **阶段合并（最大值覆盖策略）**：

$$\text{confidence}_{\text{L1+L2}} = \max\left(\text{confidence}_{\text{L1}}, \max(\{t.\text{confidence} \mid t \in \text{tags}_{\text{NER}}\})\right)$$

#### 3. 门限比对判定
- 若 \(\text{confidence}_{\text{L1+L2}} \ge 0.75\)（如规则直接命中 `1.0`）：说明足够确定，**直接输出，不调用 LLM**。
- 若 \(\text{confidence}_{\text{L1+L2}} < 0.75\)（如未命中规则为 `0.0`）：系统判定当前不可信，**触发场景 B，调用 Layer-3 LLM**（需 `enable_llm=true`）。

#### 4. Layer-3 LLM 置信度刷新
- Qwen 分析后导出 JSON 中的 `confidence`（如 `0.92`）。
- 经 `_safe_llm_confidence` 安全转换：LLM 可能返回 "极高" 等非数值内容（甚至经由 Prompt 注入构造），
  `float()` 失败时回退上游置信度，保证漏斗流程不崩溃。
- **前置条件**：仅当 LLM 返回了合法 `final_level`（在 taxonomy 内、场景 A 还须在冲突集合内）
  且通过降级校验后，才刷新置信度：\(\text{confidence}_{\text{final}} = \text{confidence}_{\text{LLM}}\)。
  LLM 未返回合法等级时**不刷新置信度**（见 2.6 第 3 条）。

### 2.6 安全地板防御机制 (Safety Floor)

为防止大模型幻觉或 Prompt 注入导致的危险降级放行，系统实施多重校验：

1. **场景 A 冲突集合校验**：LLM 仲裁裁定的等级必须落在冲突标签等级集合
   \(\{t.\text{level} \mid t \in \text{tags}\}\) 内，且必须是 taxonomy 合法等级。
   集合外裁定（如被注入的 LLM 返回任意低等级）一律拒绝，保留规则引擎结果，
   并打上 `needs_human_review = True` 送交人工复核工单。
2. **场景 B/C 拒绝非法降级**：若 Qwen 裁定的敏感等级 rank 低于 Layer-1/Layer-2 已确定的等级，
   系统直接拒绝该降级，保留规则引擎高等级，并打上 `needs_human_review = True`。
3. **场景 B/C 拒绝无等级结果**：LLM 返回结果但未给出合法 `final_level`
   （缺失、空值或 taxonomy 之外的伪造等级）时，视为无效裁定——**不刷新置信度、
   不归属 `L3_LLM`、不追加 LLM 审计标签**，保留上游结果并打上
   `needs_human_review = True`，同时记录 `funnel_llm_no_valid_level` 告警日志。
   该兜底防止"高置信度 + 无等级"的注入输出绕过前两条校验静默抬升整体置信度。
4. **仲裁成功后的一致性保障**：LLM 仲裁成功时，与裁定等级冲突的普通规则标签被移入
   `suppressed_tags`，确保外部对 `tags` 重算 `max_level` 的结果与 `final_level` 一致；
   且 LLM 高置信度（>= `llm_confidence_threshold`）仲裁成功时清除继承的人工复核标记，
   避免不必要的审核工单。

### 2.7 最终等级裁定优先级

```
final_level = LLM 裁定等级（若仲裁/深度分类成功裁定）
            否则 = resolve_level(有效标签)
```

`resolve_level` 的过滤规则：
1. **低置信度标签过滤**：置信度低于 `min_tag_confidence`（默认 0.5）的标签仅作审计记录，
   不参与等级计算——防止低置信度 NER 标签无条件拉高最终等级。
2. **降级标签排除**：当非降级标签存在时，降级标签（`is_downgrade=True`）不参与等级上推；
   但当 override 已压制所有普通标签、仅剩降级标签时，降级标签代表最终裁定，参与计算。
3. **无有效标签**：回退到 taxonomy 的 `default_level`。

---

## 3. 新增文件

| 文件 | 职责 |
|---|---|
| `dynclassification/funnel.py` | 三层漏斗编排器（核心） |
| `dynclassification/ner_adapter.py` | NER 引擎适配器（lazy-load 本地 NER 引擎） |
| `dynclassification/llm_adapter.py` | LLM 分类器适配器（lazy-load 本地 LLM 引擎） |

相关配套文件：`ner_engines.py`（NER 引擎实现）、`llm_engines.py`（LLM 引擎实现）、
`mlx_ner_engine.py` / `mlx_llm_engine.py`（MLX 后端）、`image_redaction.py`（图像打码）。

## 4. 修改文件

| 文件 | 变更 |
|---|---|
| `dynclassification/models.py` | 新增 `ConfidencePolicy`、`EngineLayer`；`FieldClassificationResult` 增加 `engine_layer`/`reasoning` |
| `dynclassification/service.py` | `classify_field` 改为调用 funnel；置信度计算逻辑 |
| `dynclassification/__init__.py` | 导出新符号 |
| `rules/taxonomies/*.yaml` | 增加 `confidence_policy` 配置节（`sc_health_db51.yaml` 待补齐，见 2.3） |

---

## 5. 接口设计

### 5.1 ClassificationFunnel

```python
class ClassificationFunnel:
    """三层漏斗编排器。"""

    def __init__(self, engine, taxonomy, confidence_policy=None, ner_adapter=None, llm_adapter=None):
        ...

    def classify_field(self, field_name, value, sanitize: bool = False) -> Tuple[FunnelResult, list[SecurityTag]]:
        """执行三层漏斗分类，返回 (FunnelResult, suppressed_tags)。

        sanitize: 是否计算图像打码等脱敏产物（默认 False）。
            仅当调用方显式请求脱敏时才执行图像打码，
            避免纯分类请求产生不必要的文件读写副作用。
        """
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
    sanitized_value: str       # 智能抹平/图像打码产物（仅 sanitize=True 时填充）
    has_conflict: bool
```

### 5.3 NER/LLM 适配器接口

```python
class NerAdapter:
    """NER 引擎适配器（lazy-load）。"""
    def extract(self, text: str) -> list[dict[str, Any]]: ...

class LlmAdapter:
    """LLM 分类器适配器（lazy-load）。"""
    def classify(self, text: str, upstream_level: str, upstream_confidence: float,
                 sanitize: bool = False) -> dict | None: ...
    def arbitrate(self, field_name: str, value: str, conflict_tags: list[SecurityTag],
                  taxonomy: DomainTaxonomy) -> dict | None: ...
```

---

## 6. 降级策略

```
NER 不可用（后端全部加载失败）→ extract() 返回 []，跳过 Layer-2，直接进入 Layer-3 判断
  （后端按 MLX → TensorRT → ONNX → ModelScope 顺序尝试，任一可用即生效）
NER 智能门禁拦截（PII 结构化短字段/纯数字文本）→ 跳过 Layer-2
LLM 并发过载 → 进程级信号量（PRIVACY_LLM_MAX_CONCURRENCY，默认 1）排队，
  等待超过 PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS（默认 30s）→ 返回 None → 置信度衰减
LLM 内存不足 → 可用内存低于 PRIVACY_LLM_MIN_FREE_MEM_MB（默认 512MB）跳过推理 → 返回 None
LLM 不可用（torch 未安装/模型不存在）→ 使用 Phase 1 置信度衰减输出
LLM 超时（默认 180s，env: PRIVACY_VLM_TIMEOUT）→ 返回 None → 使用 Phase 1 置信度衰减输出
LLM JSON 解析失败 → 返回 None → 使用 Phase 1 置信度衰减输出
LLM confidence 非数值 → _safe_llm_confidence 回退上游置信度，流程不崩溃
LLM 裁定等级非法/超出冲突集合/低于上游等级 → 拒绝裁定，保留规则引擎结果 + 人工复核
LLM 未返回合法 final_level → 不刷新置信度/不归属 L3，保留上游结果 + 人工复核
图像打码失败 → 记录 warning 日志，sanitized_value 留空，不影响分类结果
```

LLM 调用侧另有两处防护（`llm_adapter.py`）：仲裁请求会先经 `sanitize_for_prompt`
清洗再进 prompt，降低注入面；所有 LLM 结果 dict 在进入漏斗后仍需通过 2.6 的
安全地板校验，适配器返回值本身不被信任。

---

## 7. 默认等级配置（已有能力）

每个 taxonomy YAML 的 `default_level` 字段独立配置：

| Taxonomy | 标准体系 | `default_level` |
|---|---|---|
| `default.yaml` | L1~L5 | `L3` |
| `sc_health_db51.yaml`（四川医疗 DB51） | L1~L5 | `L3` |
| `gd_health.yaml`（广东医疗） | G1~G4 | `G2` |
| `finance_jrt0197.yaml`（金融 JR/T 0197） | C1~C4 | `C3` |
| 未来教育/政务 | — | 可设为 `L2` 或其他 |

无需额外修改。

---

## 8. 已知局限与后续优化方向

1. **层间等级冲突静默（仲裁能力不对称）**：当前冲突检测仅覆盖"普通规则 vs 降级规则"。
   若规则判 L4（0.95）而 NER 判 L2（0.90），置信度取最大值 0.95 且不触发冲突检测，
   层间等级矛盾被静默忽略——LLM 仲裁能力只对规则**内部**冲突开放，跨层不一致没有
   裁决通道。更值得注意的是反向情形：NER 标签置信度 ≥ `min_tag_confidence` 即参与
   `resolve_level`，NER 高等级会**绕过仲裁无条件抬高最终等级**（升敏方向不受 2.6
   安全地板约束，因为安全地板只校验 LLM 输出）。后续可引入层间分歧检测
   （如 |rank差| ≥ 2 时标记 `needs_human_review`）与加权融合。
2. **长尾字段 LLM 成本**：规则未命中时 `confidence = 0.0 < 0.75`，所有长尾字段都会触发
   LLM（秒级延迟）。进程级信号量（默认并发 1）+ 30s 排队超时提供了过载保护，
   但高吞吐场景 p99 延迟仍会被 LLM 拖长；建议通过 `enable_llm=false`、调低
   `llm_confidence_threshold` 或引入请求级限流/采样控制 LLM 调用比例。
3. **置信度"最大值覆盖"策略**：多引擎置信度取 max 而非加权融合，
   无法表达引擎间意见分歧的程度；且 `engine_layer` 标记的决策来源与 confidence
   的实际来源可能不是同一层，下游审计时无法回溯置信度由谁贡献。
   可作为后续概率化融合的改进方向。
4. **医疗逻辑耦合进通用漏斗**：L5/L4 补全扫描的模式表硬编码来自
   `medical_pipeline/rules.py`，通用 `ClassificationFunnel` 对所有 domain
   （包括金融 C 体系）都会执行医疗模式扫描，领域分层不干净；且 L5 命中标签固定
   `needs_human_review=True`，高频命中场景下人工复核工单量可能失控。
   后续宜将补全扫描改为按 domain/taxonomy 可配置的插件式规则源。
5. **静默放行与"确定为中敏"不可区分**：无有效标签时回退 `default_level`（如 L3），
   下游若只看 `final_level` 无法区分"什么都没检测到"与"确定是中等等级"；
   需要结合 `confidence = 0.0` 与空 `tags` 判断，接入方应在文档中明确该约定。
