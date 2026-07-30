"""三层漏斗引擎抽象基类与枚举 / Engine ABCs and Enums.

定义 Layer-2 NER 引擎和 Layer-3 LLM 分类器的抽象接口，
以及敏感度等级枚举，供 ner_engines.py / llm_engines.py 实现。 /
Defines abstract interfaces for Layer-2 NER engine and Layer-3 LLM classifier,
and sensitivity level enumerations for ner_engines.py / llm_engines.py to implement.
"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from enum import Enum
from typing import Any

logger = logging.getLogger(__name__)


class SensitivityLevel(str, Enum):
    """敏感度等级枚举（支持多套分级体系）。 / Sensitivity level enum (supports multiple classification systems).

    继承 str 使得枚举值可直接序列化为 JSON 字符串。 / Inherits from str so enum values can be directly serialized to JSON strings.
    支持 / Supports:
    - L1~L5: 通用/医疗行业分级 / General/Medical industry classification
    - C1~C4: 金融行业分级 (JR/T 0197) / Financial industry classification (JR/T 0197)
    """

    # 通用/医疗行业分级 / General/Medical industry classification
    L1 = "L1"  # 公开数据 / Public data
    L2 = "L2"  # 内部数据 / Internal data
    L3 = "L3"  # 敏感数据 / Sensitive data
    L4 = "L4"  # 高敏感数据 / Highly sensitive data
    L5 = "L5"  # 极敏感数据 / Extremely sensitive data

    # 金融行业分级 (JR/T 0197) / Financial industry classification (JR/T 0197)
    C1 = "C1"  # 第1级：不敏感数据 / Level 1: Non-sensitive data
    C2 = "C2"  # 第2级：低敏感数据 / Level 2: Low-sensitive data
    C3 = "C3"  # 第3级：敏感数据 / Level 3: Sensitive data
    C4 = "C4"  # 第4级：高敏感数据 / Level 4: Highly sensitive data

    @classmethod
    def from_string(cls, level: str) -> "SensitivityLevel":
        """从字符串解析等级，未知等级回退到 L3 并记录警告。 / Parse level from string, fall back to L3 for unknown levels and log a warning.

        Args:
            level: 等级字符串（如 "L3", "C4"）。 / Level string (e.g., "L3", "C4").

        Returns:
            对应的枚举值，未知等级回退到 L3。 / Corresponding enum value, falls back to L3 for unknown levels.
        """
        try:
            return cls(level)
        except ValueError:
            logger.warning(
                "Unknown sensitivity level '%s', falling back to L3. "
                "Valid values: %s",
                level,
                [e.value for e in cls],
            )
            return cls.L3


class SmallNerEngine(ABC):
    """Small-NER 引擎抽象接口（Layer 2）。 / Small-NER Engine ABC (Layer 2).

    小型命名实体识别引擎，用于从文本中提取敏感实体。 / Small named entity recognition engine for extracting sensitive entities from text.
    具体实现包括 ONNXSmallNerEngine 和 ModelScopeSmallNerEngine。 / Implementations include ONNXSmallNerEngine and ModelScopeSmallNerEngine.
    """

    @abstractmethod
    def extract(self, text: str) -> list[dict[str, Any]]:
        """从文本中提取命名实体。 / Extract named entities from text.

        Args:
            text: 待分析的文本内容。 / Text content to analyze.

        Returns:
            实体字典列表，每个字典包含 label / text / confidence。 / List of entity dicts, each containing label / text / confidence.
        """


class NoOpSmallNerEngine(SmallNerEngine):
    """默认空实现（降级用），不返回任何实体。 / Default no-op implementation (for fallback), returns no entities."""

    def extract(self, text: str) -> list[dict[str, Any]]:
        """空实现：始终返回空列表。 / No-op: always returns an empty list."""
        return []


class LlmClassifier(ABC):
    """LLM 分类器抽象接口（Layer 3）。 / LLM Classifier ABC (Layer 3).

    本地大语言模型分类器，用于处理规则引擎和 NER 无法确定的复杂场景。 / Local large language model classifier for complex scenarios that rule engines and NER cannot determine.
    """

    @abstractmethod
    def classify(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """基于上游结果对文本进行深度分类。 / Perform deep classification on text based on upstream results.

        Args:
            text: 待分类的文本内容。 / Text content to classify.
            upstream_level: 上游引擎给出的等级。 / Level given by upstream engine.
            upstream_confidence: 上游引擎的置信度。 / Confidence of the upstream engine.

        Returns:
            结构化分类结果字典，或 None 表示无需修正。 / Structured classification result dict, or None indicating no correction needed.
        """


class NoOpLlmClassifier(LlmClassifier):
    """默认空实现（降级用）：低置信度时给出保守回退结果。 / Default no-op implementation (fallback): gives conservative fallback result on low confidence."""

    def classify(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """降级分类逻辑：置信度 < 0.6 时标记需人工复核。 / Fallback classification logic: flag for human review when confidence < 0.6."""
        if upstream_confidence < 0.6:
            return {
                "final_level": upstream_level,
                "sub_category": "LLM_FALLBACK",
                "confidence": upstream_confidence,
                "reasoning": "LLM 未启用，按上游最高等级降级/保守处理 / LLM disabled, downgrading/falling back to highest upstream level",
                "suggested_action": "review",
                "needs_human_review": True,
            }
        return None


# Uppercase acronym aliases following PEP 8 conventions
SmallNEREngine = SmallNerEngine
LLMClassifier = LlmClassifier

