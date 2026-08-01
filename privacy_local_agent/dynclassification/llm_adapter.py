"""LLM 分类器适配器 / LLM Classifier Adapter.

为 dynclassification 三层漏斗提供 Layer-3 LLM 深度分类与仲裁能力。
采用 lazy-load 策略：仅在首次调用时加载底层 Qwen2-VL 模型，
避免核心路径引入重量级 ML 依赖（torch/transformers）。

English Description:
Provides Layer-3 LLM deep classification and arbitration capabilities for the dynclassification three-layer funnel.
Adopts a lazy-load strategy: only loads the underlying Qwen2-VL model upon the first call,
avoiding heavy ML dependencies (torch/transformers) in the core path.

执行逻辑 / Execution Logic:
┌─────────────────────────────────────────────────────────────────┐
│  LlmAdapter                                                     │
│                                                                  │
│  classify(text, upstream_level, upstream_confidence)             │
│    │                                                             │
│    ├─ 首次调用 → _lazy_init()                                     │
│    │   ├─ 尝试加载 Qwen2VLClassifier                              │
│    │   └─ 失败 → 标记不可用, 后续返回 None                           │
│    │                                                            │
│    └─ 调用底层 classify() → dict | None                           │
│       {"final_level": "L3", "confidence": 0.9, "reasoning": ""} │
│                                                                 │
│  arbitrate(field_name, value, conflict_tags, taxonomy)          │
│    │                                                            │
│    └─ 构建仲裁专用 prompt → 调用 classify → 返回裁定结果              │
│       场景: 普通规则 L3 vs 降级规则 L2 冲突时                        │
│       LLM 根据语义判断字段真实敏感度                                 │
└─────────────────────────────────────────────────────────────────┘

降级策略 / Degradation Strategy:
- torch/transformers 未安装 → 标记不可用, 返回 None / torch/transformers not installed -> mark unavailable, return None
- 模型目录不存在 → 标记不可用, 返回 None / Model dir not found -> mark unavailable, return None
- 推理超时 (180s) → 返回 None → 上层使用 Phase 1 置信度衰减 / Inference timeout (180s) -> return None -> upper layer uses Phase 1 confidence decay
- JSON 解析失败 → 返回 None → 上层使用 Phase 1 置信度衰减 / JSON parsing failed -> return None -> upper layer uses Phase 1 confidence decay
"""

from __future__ import annotations

import threading
from typing import Any

from ..observability.logging_config import get_logger
from .models import DomainTaxonomy, SecurityTag

logger = get_logger(__name__)


class LlmAdapter:
    """LLM 分类器适配器（Layer-3） / LLM Classifier Adapter (Layer-3).

    封装旧模块 privacy/classification/classification_llm.py 的 Qwen2VLClassifier，
    提供 classify() 和 arbitrate() 两个接口供 ClassificationFunnel 调用。
    Wraps Qwen2VLClassifier from the old privacy/classification/classification_llm.py module,
    providing classify() and arbitrate() interfaces for ClassificationFunnel.

    Attributes:
        _classifier: 底层 LLM 分类器实例（延迟初始化）。 / Underlying LLM classifier instance (lazy initialized).
        _available: 分类器是否可用。 / Whether the classifier is available.
        _initialized: 是否已尝试过初始化。 / Whether initialization has been attempted.
    """

    def __init__(
        self,
        model_path: str | None = None,
        classify_prompt_template: str | None = None,
        device: str | None = None,
    ):
        """初始化适配器（不加载模型） / Initialize the adapter (without loading the model).

        Args:
            model_path: 模型本地路径（可选，默认 .models/Qwen2-VL-2B-Instruct）。 / Local model path (optional).
            classify_prompt_template: LLM 分类 system prompt 模板（可选，支持占位符）。 / LLM classification system prompt template (optional).
            device: 目标计算设备（"cuda" / "cpu" / "mps" / None）。
        """
        self._model_path = model_path
        self._classify_prompt_template = classify_prompt_template
        self._device = device
        self._classifier: Any = None
        self._fallback_classifier: Any = None  # PyTorch 回退引擎（支持视觉）
        self._available = True
        self._initialized = False
        self._init_lock = threading.Lock()

    def _lazy_init(self) -> None:
        """延迟初始化 LLM 分类器 / Lazy initialize LLM classifier.

        尝试顺序 / Attempt order:
        1. MLXLlmClassifier (Apple Silicon Metal GPU, macOS 优先)
        2. Qwen2VLClassifier (PyTorch, CUDA/MPS/CPU)
        失败则标记不可用。

        线程安全：使用 Lock + double-check 防止并发请求重复初始化。
        """
        if self._initialized:
            return

        with self._init_lock:
            if self._initialized:
                return

            # 尝试 1: MLX 引擎（Apple Silicon Metal GPU）
            try:
                from .mlx_llm_engine import MLXLlmClassifier
                self._classifier = MLXLlmClassifier(
                    model_dir=self._model_path,
                    classify_prompt_template=self._classify_prompt_template,
                )
                self._classifier._lazy_init()
                logger.info("llm_adapter_initialized", extra={"backend": "mlx_metal", "model_path": self._model_path})
                self._initialized = True
                # MLX 不支持视觉，延迟初始化 PyTorch 回退引擎
                return
            except Exception as e:
                logger.debug("llm_mlx_unavailable", extra={"error": str(e)})

            # 尝试 2: PyTorch Qwen2VL 引擎
            try:
                from .llm_engines import Qwen2VLClassifier
                self._classifier = Qwen2VLClassifier(
                    model_path=self._model_path,
                    classify_prompt_template=self._classify_prompt_template,
                    device=self._device,
                )
                logger.info("llm_adapter_initialized", extra={"backend": "qwen2vl", "model_path": self._model_path})
                self._initialized = True
            except Exception as e:
                self._available = False
                self._initialized = True
                logger.info("llm_adapter_unavailable", extra={"error": str(e)})

    @property
    def is_available(self) -> bool:
        """LLM 分类器是否可用（依赖已安装）。 / Whether LLM classifier is available (dependencies installed)."""
        self._lazy_init()
        return self._available

    def classify(
        self, text: str, upstream_level: str, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """使用 LLM 对文本进行深度分类 / Perform deep classification on text using LLM.

        这是通用的 Layer-3 分类接口，由漏斗在低置信度时触发。
        This is the general Layer-3 classification interface, triggered by the funnel on low confidence.

        Args:
            text: 待分类文本。 / Text to classify.
            upstream_level: 上游引擎给出的等级 ID（如 "L3"）。 / Level ID provided by upstream engine (e.g., "L3").
            upstream_confidence: 上游置信度。 / Upstream confidence.

        Returns:
            分类结果字典 {"final_level", "confidence", "reasoning", ...}
            或 None（不可用/降级）。
            Classification result dict or None (unavailable/degraded).
        """
        self._lazy_init()
        if not self._available or self._classifier is None:
            return None

        try:
            # 旧模块的 classify 接口接受 SensitivityLevel 枚举，
            # 这里做字符串到枚举的适配转换（支持 L1~L5 和 C1~C4）。
            from .base import SensitivityLevel
            level_enum = SensitivityLevel.from_string(upstream_level)
            result = self._classifier.classify(text, level_enum, upstream_confidence)
            # MLX 引擎对图片输入返回 None，尝试回退到 PyTorch 引擎
            if result is None and self._is_image_input(text):
                result = self._classify_with_fallback(text, level_enum, upstream_confidence)
            return result
        except Exception as e:
            logger.warning("llm_classify_failed", extra={"error": str(e)})
            return None

    @staticmethod
    def _is_image_input(text: str) -> bool:
        """检测输入是否为图片（三级检测策略）。"""
        text_stripped = text.strip()
        # 第 1 级：图片扩展名
        if any(text_stripped.lower().endswith(ext) for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp")):
            return True
        # 第 2 级：Data URI 格式
        if text_stripped.startswith("data:image/"):
            return True
        # 第 3 级：纯 Base64 图片数据
        if len(text_stripped) > 100:
            import re as _re
            if _re.match(r'^[A-Za-z0-9+/\n\r]+=*$', text_stripped[:200]):
                if text_stripped.startswith(("iVBOR", "/9j/", "R0lGOD", "UklGR")):
                    return True
        return False

    def _classify_with_fallback(
        self, text: str, level_enum: Any, confidence: float
    ) -> dict[str, Any] | None:
        """使用 PyTorch Qwen2VL 回退引擎处理图片输入。"""
        if self._fallback_classifier is None:
            try:
                from .llm_engines import Qwen2VLClassifier
                self._fallback_classifier = Qwen2VLClassifier(
                    model_path=self._model_path,
                    classify_prompt_template=self._classify_prompt_template,
                    device=self._device,
                )
                logger.info("llm_fallback_initialized", extra={"backend": "qwen2vl"})
            except Exception as e:
                logger.debug("llm_fallback_unavailable", extra={"error": str(e)})
                return None
        try:
            return self._fallback_classifier.classify(text, level_enum, confidence)
        except Exception as e:
            logger.warning("llm_fallback_classify_failed", extra={"error": str(e)})
            return None

    def arbitrate(
        self,
        field_name: str,
        value: str,
        conflict_tags: list[SecurityTag],
        taxonomy: DomainTaxonomy,
    ) -> dict[str, Any] | None:
        """LLM 仲裁：解决规则冲突 / LLM Arbitration: Resolve rule conflicts.

        当普通规则和降级规则同时命中（冲突）时，由 LLM 根据字段语义
        裁定最终等级和置信度。
        When normal rules and downgrade rules hit simultaneously (conflict), LLM determines
        the final level and confidence based on field semantics.

        执行流程 / Execution flow:
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
            field_name: 字段名。 / Field name.
            value: 字段值。 / Field value.
            conflict_tags: 冲突的标签列表（包含普通标签和降级标签）。 / List of conflicting tags.
            taxonomy: 当前分类体系（用于构建等级说明）。 / Current classification taxonomy.

        Returns:
            仲裁结果字典 {"final_level", "confidence", "reasoning"}
            或 None（LLM 不可用/降级）。
            Arbitration result dict or None (LLM unavailable/degraded).
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

        # 优先使用 taxonomy 中配置的自定义 prompt 模板
        prompt_template = taxonomy.llm_arbitration_prompt_template
        if prompt_template:
            # 支持占位符: {field_name}, {value}, {domain}, {standard_id}, {conflict_desc}, {levels_desc}
            arbitration_text = prompt_template.format(
                field_name=field_name,
                value=value,
                domain=taxonomy.domain,
                standard_id=taxonomy.standard_id,
                conflict_desc=conflict_desc,
                levels_desc=levels_desc,
            )
        else:
            # 内置默认模板
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
            from .base import SensitivityLevel
            # 使用当前最高等级作为 upstream_level（支持 L1~L5 和 C1~C4）
            current_max = taxonomy.max_level(*(t.level for t in conflict_tags))
            level_enum = SensitivityLevel.from_string(current_max)
            result = self._classifier.classify(arbitration_text, level_enum, 0.5)
            return result
        except Exception as e:
            logger.warning("llm_arbitrate_failed", extra={"error": str(e)})
            return None
