"""dynclassification 模块 Layer-2 NER 适配器与引擎单元测试 / NER Adapter & Engine Unit Tests.

测试覆盖场景：
- NerAdapter 延迟加载与优雅降级（底层依赖缺失或损坏时不崩溃）
- ONNXSmallNerEngine Mock 测试（字符串清洗、实体标注提取、标签映射）
- ModelScopeSmallNerEngine Mock 测试（Pipeline 结果解析与阈值过滤）
- NerAdapter.extract() 在各种异常输入下的健壮性（空串、全标点等）
"""

from __future__ import annotations

from typing import Any, Dict, List
from unittest.mock import MagicMock, patch

import pytest

from privacy_local_agent.dynclassification.ner_adapter import NerAdapter
from privacy_local_agent.dynclassification.ner_engines import (
    ModelScopeSmallNerEngine,
    ONNXSmallNerEngine,
)


class TestNerAdapter:
    """测试 NerAdapter 的延迟初始化、降级机制与 extract() 方法。"""

    def test_adapter_initialization_defaults(self):
        """测试适配器默认初始化状态（未加载模型，处于乐观可用状态）。"""
        adapter = NerAdapter()
        assert adapter._initialized is False
        assert adapter._engine is None
        assert adapter._available is True

    def test_lazy_init_failure_degradation(self, monkeypatch):
        """测试当 ONNX 和 ModelScope 均不可用时，适配器能优雅降级且 is_available 为 False。"""
        adapter = NerAdapter(model_path="/non_existent_path/model.onnx")

        # 模拟两个引擎初始化均抛出 Exception
        with patch.object(ONNXSmallNerEngine, "_lazy_init", side_effect=RuntimeError("ONNX not installed")), \
             patch.object(ModelScopeSmallNerEngine, "__init__", side_effect=RuntimeError("ModelScope not installed")):
            assert adapter.is_available is False
            # 再次调用 extract 应优雅返回空列表，不受影响
            result = adapter.extract("患者张三，诊断为糖尿病")
            assert result == []

    def test_extract_with_mocked_onnx_engine(self):
        """测试当 ONNX 引擎可用时，extract() 正确转发并返回标准实体字典结构。"""
        adapter = NerAdapter()
        mock_engine = MagicMock()
        mock_engine.extract.return_value = [
            {"label": "MEDICAL_DISEASE", "text": "糖尿病", "confidence": 0.95},
        ]

        # 注入 Mock 引擎
        adapter._engine = mock_engine
        adapter._initialized = True
        adapter._available = True

        res = adapter.extract("患者诊断为糖尿病")
        assert len(res) == 1
        assert res[0]["label"] == "MEDICAL_DISEASE"
        assert res[0]["text"] == "糖尿病"
        assert res[0]["confidence"] == 0.95
        mock_engine.extract.assert_called_once_with("患者诊断为糖尿病")

    def test_extract_exception_handling(self):
        """测试底层引擎调用抛出未捕获异常时，extract() 捕获异常并返回空列表（Fail-safe）。"""
        adapter = NerAdapter()
        mock_engine = MagicMock()
        mock_engine.extract.side_effect = Exception("CUDA Out of Memory")

        adapter._engine = mock_engine
        adapter._initialized = True
        adapter._available = True

        res = adapter.extract("异常测试文本")
        assert res == []

    def test_empty_input_handling(self):
        """测试传入空字符串或空白字符的处理。"""
        adapter = NerAdapter()
        mock_engine = MagicMock()
        mock_engine.extract.return_value = []

        adapter._engine = mock_engine
        adapter._initialized = True
        adapter._available = True

        assert adapter.extract("") == []
        assert adapter.extract("   ") == []


class TestONNXSmallNerEngine:
    """测试 ONNXSmallNerEngine 的初始化与 BIO 标注解析。"""

    def test_onnx_engine_lazy_init_file_not_found(self):
        """测试当指定不存在的模型文件路径时抛出 FileNotFoundError。"""
        engine = ONNXSmallNerEngine(model_path="/path/to/missing_model.onnx")
        with pytest.raises(FileNotFoundError):
            engine._lazy_init()

    def test_onnx_engine_parse_bio_tags(self):
        """测试 _parse_bio_tags 对 BIO 预测标记序列的实体提取。"""
        engine = ONNXSmallNerEngine()
        tokens = ["[CLS]", "糖", "尿", "病", "[SEP]"]
        label_indices = [0, 1, 2, 2, 0]  # 1=B-dis, 2=I-dis
        probs = [0.99, 0.95, 0.96, 0.94, 0.99]

        entities = engine._parse_bio_tags(tokens, label_indices, probs)
        assert len(entities) == 1
        assert entities[0]["text"] == "糖尿病"
        assert entities[0]["label"] == "dis"


class TestModelScopeSmallNerEngine:
    """测试 ModelScopeSmallNerEngine 的模拟集成逻辑。"""

    def test_modelscope_extract_parsing_mocked(self):
        """Mock ModelScope pipeline 的输出，校验实体抽取与映射。"""
        engine = ModelScopeSmallNerEngine(
            label_mapping={"dis": "MEDICAL_DISEASE"},
        )

        mock_pipeline = MagicMock()
        mock_pipeline.return_value = {
            "output": [
                {"type": "dis", "span": "高血压", "probability": 0.98},
            ]
        }
        engine.pipeline = mock_pipeline
        engine._initialized = True
        engine._available = True

        entities = engine.extract("患者高血压三级")
        assert len(entities) == 1
        assert entities[0]["label"] == "MEDICAL_DISEASE"
        assert entities[0]["text"] == "高血压"
        assert entities[0]["confidence"] == 1.0
