"""dynclassification 模块 Layer-2 NER 适配器与引擎单元测试 / NER Adapter & Engine Unit Tests.

测试覆盖场景：
- NerAdapter 延迟加载与优雅降级（底层依赖缺失或损坏时不崩溃）
- ONNXSmallNerEngine Mock 测试（字符串清洗、实体标注提取、标签映射）
- ModelScopeSmallNerEngine Mock 测试（Pipeline 结果解析与阈值过滤）
- NerAdapter.extract() 在各种异常输入下的健壮性（空串、全标点等）
"""

from __future__ import annotations

import os
from typing import Any, Dict, List
from unittest.mock import MagicMock, patch

import pytest

from privacy_local_agent.dynclassification.ner_adapter import NerAdapter
from privacy_local_agent.dynclassification.ner_engines import (
    ModelScopeSmallNerEngine,
    ONNXSmallNerEngine,
    TensorRTSmallNerEngine,
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
        """测试当 TensorRT, ONNX 和 ModelScope 均不可用时，适配器能优雅降级且 is_available 为 False。"""
        adapter = NerAdapter(model_path="/non_existent_path/model.onnx")

        # 模拟三个引擎初始化均抛出 Exception
        with patch.object(TensorRTSmallNerEngine, "_lazy_init", side_effect=RuntimeError("TensorRT not installed")), \
             patch.object(ONNXSmallNerEngine, "_lazy_init", side_effect=RuntimeError("ONNX not installed")), \
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


class TestTensorRTSmallNerEngine:
    """测试 TensorRTSmallNerEngine 的初始化与 C++ TensorRT 挂载逻辑。"""

    def test_tensorrt_engine_lazy_init_file_not_found(self):
        """测试当指定不存在的模型文件路径时抛出 FileNotFoundError。"""
        engine = TensorRTSmallNerEngine(model_path="/path/to/missing_model.onnx")
        with pytest.raises(FileNotFoundError):
            engine._lazy_init()

    def test_tensorrt_engine_lazy_init_providers_mock(self):
        """Mock onnxruntime 验证 TensorRT 选项与引擎加载。"""
        engine = TensorRTSmallNerEngine(
            model_path=__file__,  # 使用真实存在的测试文件模拟模型路径
            vocab_path=__file__,  # 使用真实存在的测试文件模拟词表路径
        )
        mock_ort = MagicMock()
        mock_ort.get_available_providers.return_value = [
            "TensorrtExecutionProvider",
            "CUDAExecutionProvider",
            "CPUExecutionProvider",
        ]
        mock_session = MagicMock()
        mock_session.get_providers.return_value = [
            "TensorrtExecutionProvider",
            "CUDAExecutionProvider",
            "CPUExecutionProvider",
        ]
        mock_ort.InferenceSession.return_value = mock_session

        with patch.dict("sys.modules", {"onnxruntime": mock_ort}):
            engine._lazy_init()
            assert engine._initialized is True
            assert engine.session is mock_session
            mock_ort.InferenceSession.assert_called_once()
            # 校验是否使用了 TensorrtExecutionProvider 且配置了 trt_fp16_enable
            providers_arg = mock_ort.InferenceSession.call_args[1].get("providers")
            assert isinstance(providers_arg[0], tuple)
            assert providers_arg[0][0] == "TensorrtExecutionProvider"
            assert providers_arg[0][1]["trt_fp16_enable"] is True

    def test_tensorrt_engine_parse_bio_tags(self):
        """测试 TensorRTSmallNerEngine 的 BIO 预测标记序列实体提取（单实体测试）。"""
        engine = TensorRTSmallNerEngine()
        tokens = ["[CLS]", "阿", "司", "匹", "林", "[SEP]"]
        label_indices = [0, 3, 4, 4, 4, 0]  # 3=B-dru, 4=I-dru
        probs = [0.99, 0.98, 0.97, 0.96, 0.95]

        entities = engine._parse_bio_tags(tokens, label_indices, probs)
        assert len(entities) == 1
        assert entities[0]["text"] == "阿司匹林"
        assert entities[0]["label"] == "dru"
        assert entities[0]["confidence"] == 0.95

    def test_tensorrt_engine_multi_entity_vectors(self):
        """测试 TensorRTSmallNerEngine 提取多类型实体测试向量 (疾病+症状+药物+检查项目+身体部位)。"""
        engine = TensorRTSmallNerEngine()

        # 文本向量: [CLS] 糖 尿 病 (1,2,2) 头 晕 (7,8) 阿 莫 西 林 (3,4,4,4) 心 电 图 (9,10,10) [SEP]
        tokens = ["[CLS]", "糖", "尿", "病", "头", "晕", "阿", "莫", "西", "林", "心", "电", "图", "[SEP]"]
        label_indices = [0, 1, 2, 2, 7, 8, 3, 4, 4, 4, 9, 10, 10, 0]
        probs = [0.99, 0.96, 0.95, 0.97, 0.91, 0.92, 0.98, 0.97, 0.96, 0.95, 0.94, 0.93, 0.92, 0.99]

        entities = engine._parse_bio_tags(tokens, label_indices, probs)
        assert len(entities) == 4

        # 1. 疾病: 糖尿病
        assert entities[0]["text"] == "糖尿病"
        assert entities[0]["label"] == "dis"
        assert round(entities[0]["confidence"], 2) == 0.95

        # 2. 症状: 头晕
        assert entities[1]["text"] == "头晕"
        assert entities[1]["label"] == "sym"
        assert round(entities[1]["confidence"], 2) == 0.91

        # 3. 药物: 阿莫西林
        assert entities[2]["text"] == "阿莫西林"
        assert entities[2]["label"] == "dru"
        assert round(entities[2]["confidence"], 2) == 0.95

        # 4. 检查项目: 心电图
        assert entities[3]["text"] == "心电图"
        assert entities[3]["label"] == "ite"
        assert round(entities[3]["confidence"], 2) == 0.92

    def test_tensorrt_engine_custom_label_mapping(self):
        """测试 TensorRTSmallNerEngine 的自定义类别映射标签转换向量。"""
        custom_mapping = {
            "dis": "SEC_DISEASE_LVL4",
            "dru": "SEC_MEDICATION_LVL3",
            "sym": "SEC_SYMPTOM_LVL2",
        }
        engine = TensorRTSmallNerEngine(label_mapping=custom_mapping)
        tokens = ["[CLS]", "高", "血", "压", "[SEP]"]
        label_indices = [0, 1, 2, 2, 0]  # 1=B-dis, 2=I-dis
        probs = [0.99, 0.98, 0.97, 0.96, 0.99]

        entities = engine._parse_bio_tags(tokens, label_indices, probs)
        # 执行映射转换
        for ent in entities:
            if ent["label"] in engine.label_mapping:
                ent["label"] = engine.label_mapping[ent["label"]]

        assert len(entities) == 1
        assert entities[0]["text"] == "高血压"
        assert entities[0]["label"] == "SEC_DISEASE_LVL4"

    def test_tensorrt_engine_edge_case_inputs(self):
        """测试无实体文本、标点符号文本与边界向量。"""
        engine = TensorRTSmallNerEngine()
        tokens = ["[CLS]", "，", "。", "！", "[SEP]"]
        label_indices = [0, 0, 0, 0, 0]  # 全部为 O
        probs = [0.99, 0.99, 0.99, 0.99, 0.99]

        entities = engine._parse_bio_tags(tokens, label_indices, probs)
        assert entities == []

    def test_tensorrt_engine_real_model_inference_vector(self):
        """若本地存在 .models/raner_cmeee.onnx 则运行真实的端到端推理测试向量。"""
        current_dir = os.path.dirname(os.path.abspath(__file__))
        project_root = os.path.dirname(os.path.dirname(current_dir))
        model_path = os.path.join(project_root, ".models", "raner_cmeee.onnx")
        vocab_path = os.path.join(project_root, ".models", "vocab.txt")

        if not (os.path.exists(model_path) and os.path.exists(vocab_path)):
            pytest.skip("未在 .models/ 找到 raner_cmeee.onnx 和 vocab.txt，跳过真实模型端到端测试")

        engine = TensorRTSmallNerEngine(model_path=model_path, vocab_path=vocab_path)
        test_text = "患者张三，主诉高血压三级，合并2型糖尿病，口服阿司匹林。"
        results = engine.extract(test_text)

        # 校验返回结构为字典列表，且字段完整
        assert isinstance(results, list)
        for ent in results:
            assert "text" in ent
            assert "label" in ent
            assert "confidence" in ent
            assert ent["confidence"] > 0.0
