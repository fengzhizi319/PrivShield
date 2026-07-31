"""三层漏斗编排器 / Three-Layer Classification Funnel.

编排 Layer-1 规则引擎、Layer-2 NER、Layer-3 LLM 的执行顺序和降级逻辑， / Orchestrates the execution order and fallback logic of Layer-1 Rule Engine, Layer-2 NER, and Layer-3 LLM,
并实现置信度衰减策略（Phase 1）和 LLM 仲裁（Phase 2）。 / and implements confidence decay policy (Phase 1) and LLM arbitration (Phase 2).

执行流程:
┌─────────────────────────────────────────────────────────────────────────┐
│  ClassificationFunnel.classify_field(field_name, value)                 │
│                                                                         │
│  Step 1: Layer-1 规则引擎评估                                             │
│    tags, suppressed_tags = engine.evaluate(field_name, value)           │
│    confidence = 1.0 if tags else 0.0                                    │
│    engine_layer = "L1_RULE"                                             │
│                                                                         │
│  Step 2: 冲突检测                                                         │
│    has_normal = 存在 source_engine=="RULE" 且 is_override==False 的标签    │
│    has_downgrade = 存在 is_override==True 的标签                          │
│    has_conflict = has_normal AND has_downgrade                          │
│                                                                         │
│  Step 3: Layer-2 NER (可选)                                              │
│    触发: policy.enable_ner AND (无标签 OR 等级 <= 阈值)                     │
│    ner_tags = ner_adapter.extract(value)                                │
│    → 映射为 SecurityTag, 追加到 tags                                      │
│    → engine_layer = "L2_SMALL_NER"                                      │
│                                                                         │
│  Step 4: 置信度策略 + Layer-3 LLM (可选)                                   │
│    ┌────────────────────────────────────────────────────────────────┐    │
│    │  if has_conflict:                                              │    │
│    │    if policy.enable_llm_arbitration AND llm.is_available:      │    │
│    │      → LLM 仲裁: 裁定等级 + 修正置信度                              │    │
│    │      → engine_layer = "L3_LLM"                                 │    │
│    │    else:                                                       │    │
│    │      → Phase 1 衰减: confidence = policy.conflict_confidence    │    │
│    │      → needs_human_review = policy.conflict_needs_review       │    │
│    │  elif confidence < policy.llm_confidence_threshold:            │    │
│    │    if policy.enable_llm AND llm.is_available:                  │    │
│    │      → LLM 深度分类                                              │    │
│    │      → engine_layer = "L3_LLM"                                 │    │
│    └────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  Step 5: 计算最终等级 + 构造 FunnelResult                                   │
│    - LLM 仲裁成功时直接使用 LLM 裁定等级（不再走 max_level）                  │
│    - 否则: 排除降级标签 + 过滤低置信度标签后取 max_level                      │
│    final_level = llm_level or resolve_level(effective_tags)             │
└─────────────────────────────────────────────────────────────────────────┘
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Tuple

from ..observability.logging_config import get_logger
from .engine import ConfigurableRuleEngine
from .llm_adapter import LlmAdapter
from .models import (
    ConfidencePolicy,
    DomainTaxonomy,
    EngineLayer,
    SecurityTag,
)
from .ner_adapter import NerAdapter

logger = get_logger(__name__)


# ===========================================================================
# 漏斗输出结果 / Funnel Output Result
# ===========================================================================


@dataclass
class FunnelResult:
    """三层漏斗分类结果。

    包含最终标签列表、裁定等级、置信度、决策层级、推理说明等完整信息。
    由 ClassificationFunnel.classify_field() 产出，供 service 层包装为
    FieldClassificationResult。
    """

    # All security tags produced across all layers.
    tags: list[SecurityTag] = field(default_factory=list)
    # Final adjudicated sensitivity level ID (e.g. "L3", "C4").
    final_level: str = ""
    # Aggregate confidence score [0.0, 1.0].
    confidence: float = 0.0
    # Which engine layer produced the final decision.
    engine_layer: str = EngineLayer.L1_RULE
    # Whether this result needs human review.
    needs_human_review: bool = False
    # Human-readable reasoning explanation.
    reasoning: str = ""
    # Whether a rule conflict was detected (normal + downgrade coexist).
    has_conflict: bool = False


# ===========================================================================
# 三层漏斗编排器 / Three-Layer Funnel Orchestrator
# ===========================================================================


class ClassificationFunnel:
    """三层漏斗编排器。

    编排 ConfigurableRuleEngine (Layer-1)、NerAdapter (Layer-2)、
    LlmAdapter (Layer-3) 的执行顺序，并实现置信度策略。

    设计原则:
    - Layer-1 始终执行（确定性规则，零延迟）
    - Layer-2 按需执行（enable_ner=true 且置信度不足时）
    - Layer-3 按需执行（冲突仲裁 或 低置信度兜底）
    - 任何层失败均不影响整体流程（graceful degradation）
    """

    def __init__(
        self,
        engine: ConfigurableRuleEngine,
        taxonomy: DomainTaxonomy,
        confidence_policy: ConfidencePolicy | None = None,
        ner_adapter: NerAdapter | None = None,
        llm_adapter: LlmAdapter | None = None,
    ):
        """初始化漏斗编排器。

        Args:
            engine: Layer-1 可配置规则引擎实例。
            taxonomy: 当前分类体系（等级/类别定义）。
            confidence_policy: 置信度策略配置（None 时使用默认策略）。
            ner_adapter: Layer-2 NER 适配器（None 时跳过 NER 层）。
            llm_adapter: Layer-3 LLM 适配器（None 时跳过 LLM 层）。
        """
        self.engine = engine
        self.taxonomy = taxonomy
        self.policy = confidence_policy or ConfidencePolicy()
        self.ner = ner_adapter
        self.llm = llm_adapter

    def classify_field(self, field_name: str, value: Any) -> Tuple[FunnelResult, list[SecurityTag]]:
        """对单个字段执行三层漏斗分类。

        Args:
            field_name: 字段名。
            value: 字段值。

        Returns:
            一个元组 (FunnelResult, suppressed_tags)，包含完整的分类决策信息和被压制的标签列表。
        """
        str_value = str(value) if value is not None else ""

        # ===== Step 1: Layer-1 规则引擎评估 =====
        tags, suppressed_tags = self.engine.evaluate(field_name, value)
        confidence = 1.0 if tags else 0.0
        engine_layer = EngineLayer.L1_RULE
        reasoning = ""
        # 记录 L1 是否产出了标签（用于 Step 3 判断 engine_layer 归属）
        l1_has_tags = bool(tags)

        if tags:
            rule_ids = [t.rule_id for t in tags if t.rule_id]
            reasoning = "命中规则: " + ", ".join(rule_ids)

        # ===== Step 2: 冲突检测 =====
        # 冲突定义: 普通规则标签（非降级）和降级规则标签（is_downgrade）同时存活
        has_normal = any(
            t.source_engine == "RULE" and not t.is_downgrade for t in tags
        )
        has_downgrade = any(t.is_downgrade for t in tags)
        has_conflict = has_normal and has_downgrade

        # ===== Step 3: Layer-2 NER 实体识别（可选） =====
        if self.policy.enable_ner and self.ner is not None:
            # 触发条件: 无标签 或 当前最高等级 rank <= 配置阈值（中低敏感度时才需 NER 辅助）
            current_level = self._resolve_level(tags)
            current_rank = self.taxonomy.get_level_rank(current_level)
            ner_trigger_rank = self.policy.ner_trigger_max_rank
            if not tags or current_rank <= ner_trigger_rank:
                ner_tags = self._run_ner(str_value)
                if ner_tags:
                    tags.extend(ner_tags)
                    confidence = max(confidence, max(t.confidence for t in ner_tags))
                    # 仅当 NER 实际影响了最终决策时才更新 engine_layer 归属:
                    # - L1 无标签时 NER 提供了首个分类结果 → 归属 L2
                    # - NER 等级高于 L1 结果 → 归属 L2
                    # Update engine_layer only when NER actually influences the outcome:
                    # - L1 produced no tags and NER provides the first classification
                    # - NER level rank exceeds what L1 determined
                    ner_level = self._resolve_level(ner_tags)
                    if not l1_has_tags or self.taxonomy.get_level_rank(ner_level) > current_rank:
                        engine_layer = EngineLayer.L2_SMALL_NER
                    reasoning += " | NER 实体识别命中"

        # ===== Step 4: 置信度策略 + Layer-3 LLM =====
        needs_human_review = any(t.needs_human_review for t in tags)
        # LLM 裁定等级：非空时 Step 5 直接使用此等级，不再走 max_level
        llm_adjudicated_level: str = ""

        if has_conflict:
            # 场景 A: 规则冲突
            if self.policy.enable_llm_arbitration and self.llm is not None and self.llm.is_available:
                # Phase 2: LLM 仲裁
                llm_result = self.llm.arbitrate(
                    field_name=field_name,
                    value=str_value,
                    conflict_tags=tags,
                    taxonomy=self.taxonomy,
                )
                if llm_result:
                    # LLM 仲裁成功: 使用 LLM 裁定的等级和置信度
                    confidence = float(llm_result.get("confidence", confidence))
                    reasoning = str(llm_result.get("reasoning", reasoning))
                    engine_layer = EngineLayer.L3_LLM
                    # LLM 裁定等级：直接作为最终等级，不被其他标签的 max_level 覆盖
                    llm_level = llm_result.get("final_level", "")
                    if llm_level and llm_level in self.taxonomy.levels:
                        llm_adjudicated_level = llm_level
                        # 追加一个 LLM 裁定标签（用于审计追踪）
                        tags.append(SecurityTag(
                            level=llm_level,
                            category="LLM_ARBITRATION",
                            confidence=confidence,
                            source_engine="LLM",
                            rule_id="LLM_ARBITRATE",
                            domain=self.taxonomy.domain,
                            standard_id=self.taxonomy.standard_id,
                        ))
                    logger.info(
                        "funnel_llm_arbitration",
                        extra={"field_name": field_name, "llm_level": llm_level},
                    )
                else:
                    # LLM 仲裁失败: 回退到 Phase 1 置信度衰减
                    confidence = self.policy.conflict_confidence
                    needs_human_review = self.policy.conflict_needs_review
                    reasoning += " | 规则冲突(LLM仲裁失败,置信度衰减)"
            else:
                # Phase 1: 置信度衰减（无 LLM 或 LLM 不可用）
                confidence = self.policy.conflict_confidence
                needs_human_review = self.policy.conflict_needs_review
                reasoning += " | 规则冲突(置信度衰减)"

        elif confidence < self.policy.llm_confidence_threshold:
            # 场景 B: 低置信度兜底（无冲突但置信度不足）
            if self.policy.enable_llm and self.llm is not None and self.llm.is_available:
                current_level = self._resolve_level(tags)
                llm_result = self.llm.classify(str_value, current_level, confidence)
                if llm_result:
                    confidence = float(llm_result.get("confidence", confidence))
                    reasoning = str(llm_result.get("reasoning", reasoning))
                    engine_layer = EngineLayer.L3_LLM
                    llm_level = llm_result.get("final_level", "")
                    if llm_level and llm_level in self.taxonomy.levels:
                        llm_adjudicated_level = llm_level
                        tags.append(SecurityTag(
                            level=llm_level,
                            category="LLM_CLASSIFICATION",
                            confidence=confidence,
                            source_engine="LLM",
                            rule_id="LLM_DEEP",
                            domain=self.taxonomy.domain,
                            standard_id=self.taxonomy.standard_id,
                        ))

        # ===== Step 5: 计算最终等级 =====
        # 优先级: LLM 裁定等级 > 有效标签 max_level
        # Priority: LLM adjudicated level > max_level of effective tags
        if llm_adjudicated_level:
            # LLM 仲裁/深度分类成功裁定了等级，直接使用，不被其他标签覆盖
            final_level = llm_adjudicated_level
        else:
            final_level = self._resolve_level(tags)

        funnel_result = FunnelResult(
            tags=tags,
            final_level=final_level,
            confidence=confidence,
            engine_layer=engine_layer,
            needs_human_review=needs_human_review,
            reasoning=reasoning,
            has_conflict=has_conflict,
        )
        return funnel_result, suppressed_tags

    # ------------------------------------------------------------------
    # 内部方法 / Internal Methods
    # ------------------------------------------------------------------

    def _resolve_level(self, tags: list[SecurityTag]) -> str:
        """从标签列表中解析最终等级（取最高 rank）。

        Resolve the final sensitivity level from a list of security tags.

        过滤规则 / Filtering rules:
        1. 排除置信度低于 min_tag_confidence 的标签：防止低置信度 NER 标签
           无条件拉高最终等级。/ Tags below min_tag_confidence are excluded to
           prevent low-confidence NER tags from unconditionally raising the level.
        2. 当非降级标签存在时，排除降级标签（is_downgrade=True）：
           降级标签不应上推等级，其压制效果已通过 suppressed_tags 体现。
           但当 override 已压制所有普通标签、仅剩降级标签时，
           降级标签代表最终裁定，应参与计算。
           / Downgrade tags are excluded when normal tags survive (they shouldn't
           raise the level). But when override has suppressed all normal tags and
           only downgrade tags remain, they represent the final verdict.

        无有效标签时返回 taxonomy 的 default_level。
        Returns taxonomy.default_level when no effective tags remain.
        """
        if not tags:
            return self.taxonomy.default_level
        # Step 1: 过滤低置信度标签 / Filter low-confidence tags
        min_conf = self.policy.min_tag_confidence
        confident_tags = [t for t in tags if t.confidence >= min_conf]
        if not confident_tags:
            # 所有标签均低于置信度阈值，回退到默认等级
            return self.taxonomy.default_level
        # Step 2: 当非降级标签存在时，排除降级标签
        # Exclude downgrade tags only when normal (non-downgrade) tags survive
        normal_tags = [t for t in confident_tags if not t.is_downgrade]
        effective = normal_tags if normal_tags else confident_tags
        levels = [t.level for t in effective]
        return self.taxonomy.max_level(*levels)

    def _run_ner(self, text: str) -> list[SecurityTag]:
        """执行 NER 实体提取并映射为 SecurityTag。

        实体类型到等级的映射优先从 taxonomy.ner_entity_mapping 配置读取，
        若未配置则使用默认映射（L3/L4/L5）。

        默认映射:
        - GENOMIC_HINT → L5 (极敏感, 需人工复核)
        - MEDICAL_DISEASE + 敏感关键词 → L4
        - MEDICAL_DISEASE (普通) → L3
        - MEDICATION / SURGERY / BODY_PART → L3
        """
        if self.ner is None:
            return []

        entities = self.ner.extract(text)
        if not entities:
            return []

        # 从 taxonomy 获取可配置的实体→等级映射
        entity_mapping = self.taxonomy.ner_entity_mapping or {}
        # 默认等级回退值
        default_level = self.taxonomy.default_level
        # 从 taxonomy levels 中推断高/中/低等级
        sorted_levels = sorted(self.taxonomy.levels.values(), key=lambda l: l.rank)
        level_ids = [l.id for l in sorted_levels]
        # 动态推断: 最高等级、次高等级、中间等级
        highest_level = level_ids[-1] if level_ids else "L5"
        second_highest = level_ids[-2] if len(level_ids) >= 2 else highest_level
        mid_level = level_ids[len(level_ids) // 2] if level_ids else default_level

        tags: list[SecurityTag] = []
        # 敏感疾病关键词: 命中时升级为次高等级
        # 优先从 taxonomy 配置读取，否则使用内置默认值
        sensitive_keywords = self.taxonomy.ner_sensitive_keywords or [
            "hiv", "精神分裂", "艾滋", "梅毒", "肿瘤", "癌症", "白血病", "抑郁症"
        ]

        for ent in entities:
            label = ent.get("label", "")
            ent_text = str(ent.get("text", "")).lower()
            conf = float(ent.get("confidence", 0.8))

            # 优先使用配置化映射
            if label in entity_mapping:
                mapped_level = entity_mapping[label]
                tags.append(SecurityTag(
                    level=mapped_level, category=label, confidence=conf,
                    source_engine="SMALL_NER", rule_id=f"NER_{label}",
                    domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                    needs_human_review=(mapped_level == highest_level),
                ))
            elif label == "GENOMIC_HINT":
                tags.append(SecurityTag(
                    level=highest_level, category="GENOMIC_HINT", confidence=conf,
                    source_engine="SMALL_NER", rule_id="NER_GENE_001",
                    domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                    needs_human_review=True,
                ))
            elif label == "MEDICAL_DISEASE":
                if any(kw in ent_text for kw in sensitive_keywords):
                    tags.append(SecurityTag(
                        level=second_highest, category="MEDICAL_SENSITIVE_DISEASE", confidence=conf,
                        source_engine="SMALL_NER", rule_id="NER_DIS_SENSITIVE",
                        domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                    ))
                else:
                    tags.append(SecurityTag(
                        level=mid_level, category="MEDICAL_DISEASE", confidence=conf,
                        source_engine="SMALL_NER", rule_id="NER_DIS_NORMAL",
                        domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                    ))
            elif label in ("MEDICATION", "SURGERY", "BODY_PART"):
                tags.append(SecurityTag(
                    level=mid_level, category=label, confidence=conf,
                    source_engine="SMALL_NER", rule_id=f"NER_{label}",
                    domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                ))
            else:
                # 未知实体标签回退：使用中间等级，避免静默丢弃自定义 NER 标签
                tags.append(SecurityTag(
                    level=mid_level, category=label or "UNKNOWN_NER", confidence=conf,
                    source_engine="SMALL_NER", rule_id=f"NER_{label or 'UNKNOWN'}",
                    domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                ))

        return tags