"""真实 NER / LLM 模型冒烟测试 / Real Model Smoke Tests.

这些测试要求 .models/ 目录下已下载 NER 与 Qwen2-VL 模型，并且当前环境安装了
[ml] 可选依赖（torch/transformers/onnxruntime/modelscope 等）。

由于加载 4.2GB 的本地大模型非常耗时，所有测试都标记为 slow + real_models，
常规 CI 运行时应使用 `-m "not real_models"` 跳过。
"""

from __future__ import annotations

import pytest

from privacy_local_agent.dynclassification.base import SensitivityLevel
from privacy_local_agent.dynclassification.llm_adapter import LlmAdapter
from privacy_local_agent.dynclassification.ner_adapter import NerAdapter

pytestmark = [pytest.mark.slow, pytest.mark.real_models]


class TestRealNerAdapter:
    """使用 .models/raner_cmeee 真实模型验证 NER 实体抽取。"""

    def test_real_ner_extracts_medical_entities(self):
        """NerAdapter 应能加载 ModelScope 后端并识别出医疗实体。"""
        adapter = NerAdapter()
        assert adapter.is_available, "NER 后端初始化失败，请检查模型文件与依赖"

        text = "患者诊断为糖尿病和高血压"
        entities = adapter.extract(text)

        labels = {ent["label"] for ent in entities}
        assert "MEDICAL_DISEASE" in labels
        texts = {ent["text"] for ent in entities}
        assert "糖尿病" in texts or "高血压" in texts


class TestRealLlmAdapter:
    """使用 .models/Qwen2-VL-2B-Instruct 真实模型验证 LLM 分类。"""

    def test_real_llm_loads_and_returns_structured_result(self):
        """LlmAdapter 应能加载本地 Qwen2-VL 并返回含 final_level 的结构化结果。"""
        adapter = LlmAdapter()
        assert adapter.is_available, "LLM 初始化失败，请检查模型文件与依赖"

        result = adapter.classify(
            text="身份证号：510101199001011234",
            upstream_level="L3",
            upstream_confidence=0.6,
        )
        assert result is not None
        assert "final_level" in result
        assert "confidence" in result
        assert 0.0 <= result["confidence"] <= 1.0

    def test_real_llm_arbitrate_returns_structured_result(self):
        """LlmAdapter.arbitrate 应能组装冲突上下文并返回裁定结果。"""
        from privacy_local_agent.dynclassification.models import (
            CategoryDef,
            DomainTaxonomy,
            SecurityTag,
            SensitivityLevelDef,
        )

        adapter = LlmAdapter()
        assert adapter.is_available, "LLM 初始化失败，请检查模型文件与依赖"

        tag1 = SecurityTag(rule_id="RULE_IDCARD", level="L3", category="PERSONAL_BASIC")
        tag2 = SecurityTag(rule_id="DOWN_OPS", level="L2", category="MANAGEMENT")
        taxonomy = DomainTaxonomy(
            domain="test",
            standard_id="test",
            levels={
                "L2": SensitivityLevelDef(id="L2", name="内部", rank=2),
                "L3": SensitivityLevelDef(id="L3", name="敏感", rank=3),
            },
            categories={
                "PERSONAL_BASIC": CategoryDef(id="PERSONAL_BASIC", name="个人基本信息"),
                "MANAGEMENT": CategoryDef(id="MANAGEMENT", name="管理信息"),
            },
            default_level="L1",
        )

        result = adapter.arbitrate("user_id", "510101199001011234", [tag1, tag2], taxonomy)
        assert result is not None
        assert "final_level" in result
        assert "confidence" in result
