"""三层漏斗引擎抽象基类与枚举 / Engine ABCs and Enums.

定义 Layer-2 NER 引擎和 Layer-3 LLM 分类器的抽象接口，
以及敏感度等级枚举，供 ner_engines.py / llm_engines.py 实现。
"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from enum import Enum
from typing import Any

logger = logging.getLogger(__name__)


class SensitivityLevel(str, Enum):
    """敏感度等级枚举（支持多套分级体系）。

    继承 str 使得枚举值可直接序列化为 JSON 字符串。
    支持:
    - L1~L5: 通用/医疗行业分级
    - C1~C4: 金融行业分级 (JR/T 0197)
    """

    # 通用/医疗行业分级
    L1 = "L1"  # 公开数据
    L2 = "L2"  # 内部数据
    L3 = "L3"  # 敏感数据
    L4 = "L4"  # 高敏感数据
    L5 = "L5"  # 极敏感数据

    # 金融行业分级 (JR/T 0197)
    C1 = "C1"  # 第1级：不敏感数据
    C2 = "C2"  # 第2级：低敏感数据
    C3 = "C3"  # 第3级：敏感数据
    C4 = "C4"  # 第4级：高敏感数据

    @classmethod
    def from_string(cls, level: str) -> "SensitivityLevel":
        """从字符串解析等级，未知等级回退到 L3 并记录警告。

        Args:
            level: 等级字符串（如 "L3", "C4"）。

        Returns:
            对应的枚举值，未知等级回退到 L3。
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
    """Small-NER 引擎抽象接口（Layer 2）。

    小型命名实体识别引擎，用于从文本中提取敏感实体。
    具体实现包括 ONNXSmallNerEngine 和 ModelScopeSmallNerEngine。
    """

    @abstractmethod
    def extract(self, text: str) -> list[dict[str, Any]]:
        """从文本中提取命名实体。

        Args:
            text: 待分析的文本内容。

        Returns:
            实体字典列表，每个字典包含 label / text / confidence。
        """


class NoOpSmallNerEngine(SmallNerEngine):
    """默认空实现（降级用），不返回任何实体。"""

    def extract(self, text: str) -> list[dict[str, Any]]:
        """空实现：始终返回空列表。"""
        return []


class LlmClassifier(ABC):
    """LLM 分类器抽象接口（Layer 3）。

    本地大语言模型分类器，用于处理规则引擎和 NER 无法确定的复杂场景。
    """

    @abstractmethod
    def classify(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """基于上游结果对文本进行深度分类。

        Args:
            text: 待分类的文本内容。
            upstream_level: 上游引擎给出的等级。
            upstream_confidence: 上游引擎的置信度。

        Returns:
            结构化分类结果字典，或 None 表示无需修正。
        """


class NoOpLlmClassifier(LlmClassifier):
    """默认空实现（降级用）：低置信度时给出保守回退结果。"""

    def classify(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """降级分类逻辑：置信度 < 0.6 时标记需人工复核。"""
        if upstream_confidence < 0.6:
            return {
                "final_level": upstream_level,
                "sub_category": "LLM_FALLBACK",
                "confidence": upstream_confidence,
                "reasoning": "LLM 未启用，按上游最高等级降级/保守处理",
                "suggested_action": "review",
                "needs_human_review": True,
            }
        return None
