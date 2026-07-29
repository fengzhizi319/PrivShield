"""NER 引擎适配器 / NER Engine Adapter.

为 dynclassification 三层漏斗提供 Layer-2 NER 实体识别能力。
采用 lazy-load 策略：仅在首次调用时加载底层 NER 引擎（ONNX/ModelScope），
避免核心路径引入重量级 ML 依赖。

执行逻辑:
┌─────────────────────────────────────────────────────────────────┐
│  NerAdapter.extract(text)                                        │
│    │                                                             │
│    ├─ 首次调用 → _lazy_init()                                   │
│    │   ├─ 尝试加载 ONNXSmallNerEngine (轻量, 推荐)              │
│    │   ├─ 失败 → 尝试 ModelScopeSmallNerEngine                  │
│    │   └─ 均失败 → 标记不可用, 后续直接返回 []                   │
│    │                                                             │
│    └─ 调用底层引擎 extract(text) → 实体列表                     │
│       [{"label": "MEDICAL_DISEASE", "text": "...", "confidence": 0.9}] │
└─────────────────────────────────────────────────────────────────┘

降级策略:
- onnxruntime 未安装 → 尝试 ModelScope
- modelscope 未安装 → 标记不可用, 返回空列表
- 模型文件不存在 → 标记不可用, 返回空列表
- 任何异常 → 返回空列表 (fail-safe)
"""

from __future__ import annotations

from typing import Any

from ..observability.logging_config import get_logger

logger = get_logger(__name__)


class NerAdapter:
    """NER 引擎适配器（Layer-2）。

    封装旧模块 privacy/classification/classification_ner.py 的 NER 引擎，
    提供统一的 extract() 接口供 ClassificationFunnel 调用。

    Attributes:
        _engine: 底层 NER 引擎实例（延迟初始化）。
        _available: 引擎是否可用（初始化失败后标记为 False）。
        _initialized: 是否已尝试过初始化。
    """

    def __init__(self, model_path: str | None = None, vocab_path: str | None = None, label_mapping: dict[str, str] | None = None):
        """初始化适配器（不加载模型）。

        Args:
            model_path: ONNX 模型文件路径（可选，默认自动检测）。
            vocab_path: 词表文件路径（可选，默认自动检测）。
            label_mapping: 原始标签→标准标签映射（可选，默认使用内置医疗映射）。
        """
        self._model_path = model_path
        self._vocab_path = vocab_path
        self._label_mapping = label_mapping
        self._engine: Any = None
        self._available = True  # 乐观假设可用，初始化失败后改为 False
        self._initialized = False

    def _lazy_init(self) -> None:
        """延迟初始化 NER 引擎。

        尝试顺序:
        1. ONNXSmallNerEngine (轻量, 无需 PyTorch)
        2. ModelScopeSmallNerEngine (需 PyTorch + modelscope)
        3. 均失败 → 标记不可用
        """
        if self._initialized:
            return
        self._initialized = True

        # 尝试 1: ONNX Runtime 引擎
        try:
            from .ner_engines import ONNXSmallNerEngine
            engine = ONNXSmallNerEngine(
                model_path=self._model_path,
                vocab_path=self._vocab_path,
                label_mapping=self._label_mapping,
            )
            # 触发模型加载验证（如果文件不存在会抛出异常）
            engine._lazy_init()
            self._engine = engine
            logger.info("ner_adapter_initialized", extra={"backend": "onnx"})
            return
        except Exception as e:
            logger.debug("ner_onnx_unavailable", extra={"error": str(e)})
        
        # 尝试 2: ModelScope 引擎
        try:
            from .ner_engines import ModelScopeSmallNerEngine
            engine = ModelScopeSmallNerEngine(label_mapping=self._label_mapping)
            self._engine = engine
            logger.info("ner_adapter_initialized", extra={"backend": "modelscope"})
            return
        except Exception as e:
            logger.debug("ner_modelscope_unavailable", extra={"error": str(e)})

        # 均不可用
        self._available = False
        logger.info("ner_adapter_unavailable", extra={"reason": "no backend available"})

    @property
    def is_available(self) -> bool:
        """NER 引擎是否可用。"""
        self._lazy_init()
        return self._available

    def extract(self, text: str) -> list[dict[str, Any]]:
        """从文本中提取命名实体。

        Args:
            text: 待分析的文本内容。

        Returns:
            实体字典列表，每个字典包含:
            - label: 实体标签 (如 "MEDICAL_DISEASE", "MEDICATION")
            - text: 实体文本
            - confidence: 识别置信度

            引擎不可用或异常时返回空列表。
        """
        self._lazy_init()
        if not self._available or self._engine is None:
            return []

        try:
            return self._engine.extract(text)
        except Exception as e:
            logger.warning("ner_extract_failed", extra={"error": str(e)})
            return []
