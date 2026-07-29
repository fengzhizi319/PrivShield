"""LLM 分类器适配器 / LLM Classifier Adapter.

为 dynclassification 三层漏斗提供 Layer-3 LLM 深度分类与仲裁能力。
采用 lazy-load 策略：仅在首次调用时加载底层 Qwen2-VL 模型，
避免核心路径引入重量级 ML 依赖（torch/transformers）。

执行逻辑:
┌─────────────────────────────────────────────────────────────────┐
│  LlmAdapter                                                     │
│                                                                  │
│  classify(text, upstream_level, upstream_confidence)             │
│    │                                                             │
│    ├─ 首次调用 → _lazy_init()                                   │
│    │   ├─ 尝试加载 Qwen2VLClassifier                            │
│    │   └─ 失败 → 标记不可用, 后续返回 None                      │
│    │                                                             │
│    └─ 调用底层 classify() → dict | None                         │
│       {"final_level": "L3", "confidence": 0.9, "reasoning": ""} │
│                                                                  │
│  arbitrate(field_name, value, conflict_tags, taxonomy)           │
│    │                                                             │
│    └─ 构建仲裁专用 prompt → 调用 classify → 返回裁定结果        │
│       场景: 普通规则 L3 vs 降级规则 L2 冲突时                    │
│       LLM 根据语义判断字段真实敏感度                             │
└─────────────────────────────────────────────────────────────────┘

降级策略:
- torch/transformers 未安装 → 标记不可用, 返回 None
- 模型目录不存在 → 标记不可用, 返回 None
- 推理超时 (180s) → 返回 None → 上层使用 Phase 1 置信度衰减
- JSON 解析失败 → 返回 None → 上层使用 Phase 1 置信度衰减
"""

from __future__ import annotations

from typing import Any

from ..observability.logging_config import get_logger
from .models import DomainTaxonomy, SecurityTag

logger = get_logger(__name__)


class LlmAdapter:
    """LLM 分类器适配器（Layer-3）。

    封装旧模块 privacy/classification/classification_llm.py 的 Qwen2VLClassifier，
    提供 classify() 和 arbitrate() 两个接口供 ClassificationFunnel 调用。

    Attributes:
        _classifier: 底层 LLM 分类器实例（延迟初始化）。
        _available: 分类器是否可用。
        _initialized: 是否已尝试过初始化。
    """

    def __init__(self, model_path: str | None = None):
        """初始化适配器（不加载模型）。

        Args:
            model_path: 模型本地路径（可选，默认 .models/Qwen2-VL-2B-Instruct）。
        """
        self._model_path = model_path
        self._classifier: Any = None
        self._available = True
        self._initialized = False

    def _lazy_init(self) -> None:
        """延迟初始化 LLM 分类器。

        尝试加载 Qwen2VLClassifier，失败则标记不可用。
        """
        if self._initialized:
            return
        self._initialized = True

        try:
            from ..privacy.classification.classification_llm import Qwen2VLClassifier
            self._classifier = Qwen2VLClassifier(model_path=self._model_path)
            logger.info("llm_adapter_initialized", extra={"model_path": self._model_path})
        except Exception as e:
            self._available = False
            logger.info("llm_adapter_unavailable", extra={"error": str(e)})

    @property
    def is_available(self) -> bool:
        """LLM 分类器是否可用（依赖已安装）。"""
        self._lazy_init()
        return self._available

    def classify(
        self, text: str, upstream_level: str, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """使用 LLM 对文本进行深度分类。

        这是通用的 Layer-3 分类接口，由漏斗在低置信度时触发。

        Args:
            text: 待分类文本。
            upstream_level: 上游引擎给出的等级 ID（如 "L3"）。
            upstream_confidence: 上游置信度。

        Returns:
            分类结果字典 {"final_level", "confidence", "reasoning", ...}
            或 None（不可用/降级）。
        """
        self._lazy_init()
        if not self._available or self._classifier is None:
            return None

        try:
            # 旧模块的 classify 接口接受 SensitivityLevel 枚举，
            # 这里做字符串到枚举的适配转换。
            from ..privacy.classification.classification_models import SensitivityLevel
            level_enum = SensitivityLevel(upstream_level)
            result = self._classifier.classify(text, level_enum, upstream_confidence)
            return result
        except Exception as e:
            logger.warning("llm_classify_failed", extra={"error": str(e)})
            return None

    def arbitrate(
        self,
        field_name: str,
        value: str,
        conflict_tags: list[SecurityTag],
        taxonomy: DomainTaxonomy,
    ) -> dict[str, Any] | None:
        """LLM 仲裁：解决规则冲突。

        当普通规则和降级规则同时命中（冲突）时，由 LLM 根据字段语义
        裁定最终等级和置信度。

        执行流程:
        ┌─────────────────────────────────────────────────────────────┐
        │  arbitrate(field_name="Turnover_Rate", value="0.85")        │
        │    │                                                         │
        │    ▼ 构建仲裁 prompt                                        │
        │    "字段名: Turnover_Rate, 值: 0.85                         │
        │     冲突: 规则A→L3(REPORT), 降级规则B→L2(OPERATIONAL_STAT)  │
        │     请裁定最终等级"                                          │
        │    │                                                         │
        │    ▼ 调用 LLM classify                                      │
        │    │                                                         │
        │    ▼ 返回 {"final_level": "L2", "confidence": 0.92,         │
        │            "reasoning": "营业额属于运营统计..."}             │
        └─────────────────────────────────────────────────────────────┘

        Args:
            field_name: 字段名。
            value: 字段值。
            conflict_tags: 冲突的标签列表（包含普通标签和降级标签）。
            taxonomy: 当前分类体系（用于构建等级说明）。

        Returns:
            仲裁结果字典 {"final_level", "confidence", "reasoning"}
            或 None（LLM 不可用/降级）。
        """
        self._lazy_init()
        if not self._available or self._classifier is None:
            return None

        # 构建仲裁上下文文本
        levels_desc = "\n".join(
            f"- {lid}: {lvl.name} ({lvl.description or ''})"
            for lid, lvl in sorted(taxonomy.levels.items(), key=lambda x: x[1].rank)
        )
        conflict_desc = "\n".join(
            f"- 规则 {t.rule_id} 判定为 {t.level}（{t.category}）"
            for t in conflict_tags
        )

        arbitration_text = (
            f"[仲裁请求] 以下字段的规则评估出现冲突，请裁定最终等级。\n"
            f"字段名: {field_name}\n"
            f"字段值: {value}\n"
            f"领域: {taxonomy.domain}\n"
            f"标准: {taxonomy.standard_id}\n\n"
            f"冲突信息:\n{conflict_desc}\n\n"
            f"等级定义:\n{levels_desc}\n\n"
            f"请输出 JSON: {{\"final_level\": \"等级ID\", \"confidence\": 0.0~1.0, "
            f"\"reasoning\": \"裁定理由\"}}"
        )

        try:
            from ..privacy.classification.classification_models import SensitivityLevel
            # 使用当前最高等级作为 upstream_level
            current_max = taxonomy.max_level(*(t.level for t in conflict_tags))
            level_enum = SensitivityLevel(current_max) if current_max in ("L1", "L2", "L3", "L4", "L5") else SensitivityLevel.L3
            result = self._classifier.classify(arbitration_text, level_enum, 0.5)
            return result
        except Exception as e:
            logger.warning("llm_arbitrate_failed", extra={"error": str(e)})
            return None
