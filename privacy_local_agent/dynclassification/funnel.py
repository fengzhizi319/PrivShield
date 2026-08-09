"""三层漏斗编排器 / Three-Layer Classification Funnel.

编排 Layer-1 规则引擎、Layer-2 NER、Layer-3 LLM 的执行顺序和降级逻辑， / Orchestrates the execution order and fallback logic of Layer-1 Rule Engine, Layer-2 NER, and Layer-3 LLM,
并实现置信度衰减策略（Phase 1）和 LLM 仲裁（Phase 2）。 / and implements confidence decay policy (Phase 1) and LLM arbitration (Phase 2).

执行流程:
┌─────────────────────────────────────────────────────────────────────────┐
│  ClassificationFunnel.classify_field(field_name, value)                 │
│                                                                         │
│  Step 1: Layer-1 规则引擎评估                                             │
│    tags, suppressed_tags = engine.evaluate(field_name, value)           │
│    confidence = max(tag.confidence for tag in tags) or 0.0              │
│    engine_layer = "L1_RULE"                                             │
│                                                                         │
│  Step 2: 冲突检测（精细化）                                                 │
│    normal_rule_tags = 普通规则标签（非降级）                              │
│    downgrade_tags = 降级标签                                            │
│    has_conflict = 两者共存 AND max_level(normal) != max_level(downgrade) │
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
│                                                                          │
│  安全约束（fail-closed）:                                                  │
│    - 场景 A 仲裁: LLM 裁定等级必须落在冲突标签等级集合内，否则拒绝并强制人工复核 │
│    - 场景 B/C: LLM 裁定等级低于规则/上游等级时拒绝降级，保留原等级并人工复核     │
│    - LLM 返回的 confidence 非数值时回退上游置信度，不崩溃                     │
└─────────────────────────────────────────────────────────────────────────┘
"""

from __future__ import annotations

import re
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
    # Smart sanitized/masked value.
    sanitized_value: str = ""
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

    @staticmethod
    def _should_trigger_ner(field_name: str, value: str, l1_tags: list[SecurityTag]) -> bool:
        """评估是否需要触发 Layer-2 Small-NER 深度神经实体识别。
        
        遵循“简单规则先行，复杂长文本才用 NER”原则：
        1. 排除 PII 身份/结构化短字段（如 id_card_no, phone, name, age, gender 等）；
        2. 排除纯数字、纯英文、短标记（去空白长度 < 2 或无连续中文）；
        3. 仅对临床非结构化文书字段（present_illness, family_history 等）或 L1 未命中的复杂长文本触发。
        """
        if not value or len(value.strip()) < 2:
            return False

        # 检查是否包含连续中文汉字（简单数字/代码/英文不触发 NER）
        if not re.search(r"[\u4e00-\u9fa5]{2,}", value):
            return False

        # 结构化字段/明确的 PII 与评估属性字段直接复用 L1 规则，不走 NER
        simple_structured_fields = {
            "id_card_no", "phone", "name", "patient_name", "age", "gender", "sex",
            "medical_insurance_no", "social_security_no", "disability_cert_no",
            "disability_category", "disability_level", "registered_address",
            "house_address", "contact_phone", "guardian_phone", "assess_result_name"
        }
        if field_name.lower() in simple_structured_fields:
            return False

        # 复杂的非结构化临床长文书字段必定触发 NER
        complex_clinical_fields = {
            "chief_complaint", "present_illness", "past_history",
            "personal_history", "family_history", "allergic_history",
            "progress_note", "diagnosis_name", "diagnosis"
        }
        if field_name.lower() in complex_clinical_fields:
            return True

        return True

    def classify_field(self, field_name: str, value: Any, sanitize: bool = False) -> Tuple[FunnelResult, list[SecurityTag]]:
        """对单个字段执行三层漏斗分类。

        Args:
            field_name: 字段名。
            value: 字段值。
            sanitize: 是否计算图像打码等脱敏产物（默认 False）。
                仅当调用方显式请求脱敏时才执行 Step 6 的图像打码，
                避免纯分类请求产生不必要的文件读写副作用。

        Returns:
            一个元组 (FunnelResult, suppressed_tags)，包含完整的分类决策信息和被压制的标签列表。
        """
        str_value = str(value) if value is not None else ""

        # ===== Step 1: Layer-1 规则引擎评估 =====
        tags, suppressed_tags = self.engine.evaluate(field_name, value)

        # 补全高敏病史扫描：当文本命中 L5/L4 医疗模式时，确保生成对应的 L5/L4 SecurityTag
        try:
            from ..medical_pipeline.rules import L4_PATTERNS, L5_PATTERNS, normalize_fullwidth_alphanumeric
            norm_val = normalize_fullwidth_alphanumeric(str_value)
            stripped_val = re.sub(r"(?<=[a-zA-Z0-9\u4e00-\u9fa5])[\s\.\-_]+(?=[a-zA-Z0-9\u4e00-\u9fa5])", "", norm_val)
            scan_targets = {str_value, norm_val, stripped_val}

            is_l5 = False
            for pat, _rep in L5_PATTERNS:
                if any(pat.search(t) for t in scan_targets):
                    tags.append(SecurityTag(
                        level="L5", category="HIGH_RISK_MEDICAL_L5", confidence=0.99,
                        source_engine="RULE", rule_id="MEDICAL_L5_STRICT_RULE",
                        domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                        needs_human_review=True,
                    ))
                    is_l5 = True
                    break

            if not is_l5:
                for pat, _rep in L4_PATTERNS:
                    if any(pat.search(t) for t in scan_targets):
                        tags.append(SecurityTag(
                            level="L4", category="HIGH_RISK_MEDICAL_L4", confidence=0.95,
                            source_engine="RULE", rule_id="MEDICAL_L4_STRICT_RULE",
                            domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                        ))
                        break
        except Exception:
            pass

        # 取所有命中标签的最大置信度（规则标签恒为确定性 1.0，L5/L4 补全扫描为 0.99/0.95）
        # Use max confidence from matched tags (rule tags are deterministic 1.0)
        confidence = max((t.confidence for t in tags), default=0.0)
        engine_layer = EngineLayer.L1_RULE
        reasoning = ""
        # 记录 L1 是否产出了标签（用于 Step 3 判断 engine_layer 归属）
        l1_has_tags = bool(tags)

        if tags:
            rule_ids = [t.rule_id for t in tags if t.rule_id]
            reasoning = "命中规则: " + ", ".join(rule_ids)

        # ===== Step 2: 冲突检测（精细化） =====
        # 冲突定义: 普通规则标签和降级标签同时存在，且等级不一致。
        # 若两者等级相同（如均为 L2），说明无实质矛盾，不判定为冲突。
        # Conflict = normal tags and downgrade tags coexist WITH different levels.
        # Same-level coexistence is not a real conflict.
        normal_rule_tags = [
            t for t in tags if t.source_engine == "RULE" and not t.is_downgrade
        ]
        downgrade_tags = [t for t in tags if t.is_downgrade]
        has_normal = bool(normal_rule_tags)
        has_downgrade = bool(downgrade_tags)
        if has_normal and has_downgrade:
            normal_max = self.taxonomy.max_level(*(t.level for t in normal_rule_tags))
            downgrade_max = self.taxonomy.max_level(*(t.level for t in downgrade_tags))
            has_conflict = normal_max != downgrade_max
        else:
            has_conflict = False

        # ===== Step 3: Layer-2 NER 实体识别（智能门禁: 仅复杂非结构化长文本触发） =====
        if (
            self.policy.enable_ner
            and self.ner is not None
            and self._should_trigger_ner(field_name, str_value, tags)
        ):
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
                # 冲突标签等级集合：LLM 仲裁只允许在该集合内选择，
                # 集合外裁定（如被注入的 LLM 返回任意低等级）一律拒绝。
                conflict_levels = {t.level for t in tags}
                llm_result = self.llm.arbitrate(
                    field_name=field_name,
                    value=str_value,
                    conflict_tags=tags,
                    taxonomy=self.taxonomy,
                )
                if llm_result:
                    llm_confidence = self._safe_llm_confidence(
                        llm_result.get("confidence"), confidence
                    )
                    llm_level = llm_result.get("final_level", "")
                    if llm_level and llm_level in self.taxonomy.levels and llm_level in conflict_levels:
                        # LLM 仲裁成功（等级合法且在冲突集合内）: 使用 LLM 裁定的等级和置信度
                        confidence = llm_confidence
                        reasoning = str(llm_result.get("reasoning", reasoning))
                        engine_layer = EngineLayer.L3_LLM
                        # LLM 裁定等级：直接作为最终等级，不被其他标签的 max_level 覆盖
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
                        # 一致性保障: 将与 LLM 裁定等级冲突的普通规则标签
                        # 移入 suppressed_tags，确保外部对 tags 重算 max_level
                        # 的结果与 final_level 一致。
                        # Consistency: suppress normal rule tags whose level conflicts
                        # with LLM verdict so external re-computation stays consistent.
                        surviving_tags = []
                        for t in tags:
                            if (
                                t.source_engine == "RULE"
                                and not t.is_downgrade
                                and t.level != llm_level
                            ):
                                suppressed_tags.append(t)
                            else:
                                surviving_tags.append(t)
                        tags[:] = surviving_tags
                        # 复核标记刷新: LLM 高置信度仲裁成功时，清除历史复核标记，
                        # 避免不必要的审核工单。
                        # Refresh review flag: clear inherited needs_human_review when LLM
                        # arbitrates with high confidence (>= llm_confidence_threshold).
                        if confidence >= self.policy.llm_confidence_threshold:
                            needs_human_review = False
                        logger.info(
                            "funnel_llm_arbitration",
                            extra={"field_name": field_name, "llm_level": llm_level},
                        )
                    else:
                        # 安全地板校验：LLM 裁定等级非法或超出冲突标签等级集合
                        # （可能来自 Prompt 注入/模型幻觉），拒绝采用其裁定，
                        # 保留规则引擎结果并强制人工复核。
                        needs_human_review = True
                        reasoning += " | LLM仲裁等级超出冲突集合(已拒绝,待人工复核)"
                        logger.warning(
                            "funnel_llm_arbitration_rejected",
                            extra={
                                "field_name": field_name,
                                "llm_level": llm_level,
                                "conflict_levels": sorted(conflict_levels),
                            },
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

        elif self._is_image_field_or_value(field_name, value) and self.policy.auto_llm_on_image and self.llm is not None and self.llm.is_available:
            # 场景 C: 运维优化动态识别：包含图像/图片病例时，自动强制触发 Layer-3 多模态 LLM 视觉深度分析
            current_level = self._resolve_level(tags)
            llm_result = self.llm.classify(str_value, current_level, confidence, sanitize=sanitize)
            if llm_result:
                llm_confidence = self._safe_llm_confidence(
                    llm_result.get("confidence"), confidence
                )
                llm_level = llm_result.get("final_level", "")
                if llm_level and llm_level in self.taxonomy.levels:
                    # 安全地板校验：LLM 无权将等级降到规则/上游已判定等级之下
                    # （防止 Prompt 注入或模型幻觉导致的降级放行）。
                    if self.taxonomy.get_level_rank(llm_level) < self.taxonomy.get_level_rank(current_level):
                        needs_human_review = True
                        reasoning += " | 多模态LLM降级裁定被拒绝(低于规则/上游等级,待人工复核)"
                        logger.warning(
                            "funnel_llm_downgrade_rejected",
                            extra={
                                "field_name": field_name,
                                "llm_level": llm_level,
                                "upstream_level": current_level,
                            },
                        )
                    else:
                        confidence = llm_confidence
                        reasoning = "【多模态视觉识别】" + str(llm_result.get("reasoning", reasoning))
                        engine_layer = EngineLayer.L3_LLM
                        llm_adjudicated_level = llm_level
                        tags.append(SecurityTag(
                            level=llm_level,
                            category="MULTIMODAL_IMAGE_ANALYSIS",
                            confidence=confidence,
                            source_engine="LLM",
                            rule_id="LLM_MULTIMODAL_IMAGE",
                            domain=self.taxonomy.domain,
                            standard_id=self.taxonomy.standard_id,
                        ))
                else:
                    # 安全地板兜底：LLM 未返回合法等级（可能来自 Prompt 注入/模型幻觉），
                    # 视为无效裁定——不刷新置信度、不归属 L3，保留上游结果并强制人工复核，
                    # 防止"高置信度 + 无等级"输出静默抬高整体置信度。
                    needs_human_review = True
                    reasoning += " | 多模态LLM未返回有效等级(保留上游结果,待人工复核)"
                    logger.warning(
                        "funnel_llm_no_valid_level",
                        extra={"field_name": field_name, "scenario": "C"},
                    )

        elif confidence < self.policy.llm_confidence_threshold:
            # 场景 B: 低置信度兜底（无冲突但置信度不足）
            if self.policy.enable_llm and self.llm is not None and self.llm.is_available:
                current_level = self._resolve_level(tags)
                llm_result = self.llm.classify(str_value, current_level, confidence, sanitize=sanitize)
                if llm_result:
                    llm_confidence = self._safe_llm_confidence(
                        llm_result.get("confidence"), confidence
                    )
                    llm_level = llm_result.get("final_level", "")
                    if llm_level and llm_level in self.taxonomy.levels:
                        # 安全地板校验：LLM 无权将等级降到规则/上游已判定等级之下
                        # （防止 Prompt 注入或模型幻觉导致的降级放行）。
                        if self.taxonomy.get_level_rank(llm_level) < self.taxonomy.get_level_rank(current_level):
                            needs_human_review = True
                            reasoning += " | LLM降级裁定被拒绝(低于规则/上游等级,待人工复核)"
                            logger.warning(
                                "funnel_llm_downgrade_rejected",
                                extra={
                                    "field_name": field_name,
                                    "llm_level": llm_level,
                                    "upstream_level": current_level,
                                },
                            )
                        else:
                            confidence = llm_confidence
                            reasoning = str(llm_result.get("reasoning", reasoning))
                            engine_layer = EngineLayer.L3_LLM
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
                    else:
                        # 安全地板兜底：LLM 未返回合法等级（可能来自 Prompt 注入/模型幻觉），
                        # 视为无效裁定——不刷新置信度、不归属 L3，保留上游结果并强制人工复核，
                        # 防止"高置信度 + 无等级"输出静默抬高整体置信度。
                        needs_human_review = True
                        reasoning += " | LLM未返回有效等级(保留上游结果,待人工复核)"
                        logger.warning(
                            "funnel_llm_no_valid_level",
                            extra={"field_name": field_name, "scenario": "B"},
                        )

        # ===== Step 5: 计算最终等级 =====
        # 优先级: LLM 裁定等级 > 有效标签 max_level
        # Priority: LLM adjudicated level > max_level of effective tags
        if llm_adjudicated_level:
            # LLM 仲裁/深度分类成功裁定了等级，直接使用，不被其他标签覆盖
            final_level = llm_adjudicated_level
        else:
            final_level = self._resolve_level(tags)

        # ===== Step 6: 计算智能抹平 sanitized_value =====
        # 仅当调用方显式请求脱敏（sanitize=True）且输入为图像时，
        # 才调用图像打码模块生成脱敏后的图像——纯分类请求不产生文件读写副作用。
        sanitized_value = ""
        if sanitize and self._is_image_field_or_value(field_name, value):
            try:
                from .image_redaction import sanitize_image_input
                sanitized_value = sanitize_image_input(str_value)
            except Exception as e:
                logger.warning(f"图像打码失败: {e}")

        funnel_result = FunnelResult(
            tags=tags,
            final_level=final_level,
            confidence=confidence,
            engine_layer=engine_layer,
            needs_human_review=needs_human_review,
            reasoning=reasoning,
            has_conflict=has_conflict,
            sanitized_value=sanitized_value,
        )
        return funnel_result, suppressed_tags

    def _is_image_field_or_value(self, field_name: str, value: Any) -> bool:
        """运维智能识别算法：判断字段是否包含图像/图片病例/DICOM医学影像。"""
        if value is None:
            return False
        val_str = str(value).strip()
        val_lower = val_str.lower()
        # 1. 常见图像文件扩展名检测
        if any(val_lower.endswith(ext) for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp", ".dcm", ".dicom", ".tiff")):
            return True
        # 2. Base64 图像编码
        if val_lower.startswith("data:image/") or val_lower.startswith("image:"):
            return True
        # 3. 字段名称包含图像/影像语义标识
        # 拉丁关键词使用词边界匹配（前后不得紧邻其他拉丁字母），
        # 避免 "topic" 命中 "pic"、"imaging" 命中 "img" 之类的子串误报；
        # 中文关键词无词边界概念，保持子串匹配。
        field_lower = field_name.lower()
        latin_keywords = ("image", "photo", "pic", "picture", "dicom", "xray", "ct_scan", "mri", "img")
        cjk_keywords = ("切片", "病例图片", "影像")
        keyword_hit = any(
            re.search(r"(^|[^a-z])" + re.escape(k) + r"([^a-z]|$)", field_lower)
            for k in latin_keywords
        ) or any(k in field_lower for k in cjk_keywords)
        if keyword_hit:
            if len(val_str) > 3 and not val_str.startswith("http://") and not val_str.startswith("https://"):
                return True
        return False

    @staticmethod
    def _safe_llm_confidence(raw: Any, fallback: float) -> float:
        """将 LLM 返回的 confidence 安全转换为 float。

        LLM 输出不可信：可能返回 "极高" 等非数值内容（甚至经由 Prompt 注入
        构造），直接 float() 会抛 ValueError 导致请求 500。转换失败时回退到
        上游置信度，保证漏斗流程不崩溃。
        """
        try:
            return float(raw)
        except (TypeError, ValueError):
            logger.warning(
                "funnel_llm_confidence_invalid",
                extra={"raw_confidence": str(raw)[:64], "fallback": fallback},
            )
            return fallback

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
                l5_kws = ["hiv", "aids", "艾滋", "精神分裂", "基因", "遗传缺陷"]
                if any(kw in ent_text for kw in l5_kws):
                    tags.append(SecurityTag(
                        level=highest_level, category="HIGH_RISK_MEDICAL_L5", confidence=conf,
                        source_engine="SMALL_NER", rule_id="NER_DIS_L5_STRICT",
                        domain=self.taxonomy.domain, standard_id=self.taxonomy.standard_id,
                        needs_human_review=True,
                    ))
                elif any(kw in ent_text for kw in sensitive_keywords):
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