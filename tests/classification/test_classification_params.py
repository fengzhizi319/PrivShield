"""ClassificationAPI 重要参数覆盖测试。

针对现有测试仅使用默认参数的不足，本文件对以下重要参数逐一验证：
- enable_rule_engine：禁用 Layer-1 规则引擎
- enable_small_ner：启用 Layer-2 NER 引擎（注入 Mock）
- enable_llm：启用 Layer-3 LLM 分类器（注入 Mock）
- llm_confidence_threshold：LLM 触发阈值
- return_field_values：是否在结果中返回字段原始值
- enable_review：是否收集人工复核条目
- icd10_l4_intervals：自定义 ICD-10 L4 敏感区间
- public_field_whitelist：自定义公开字段白名单
- operational_field_patterns：自定义运营字段模式
- default_level：无规则命中时的默认等级

Tests for important ClassificationAPI parameters not covered by default-param tests.
"""

from typing import Any
from unittest.mock import MagicMock

import pytest

from privacy_local_agent.dynclassification.classification import ClassificationAPI
from privacy_local_agent.dynclassification.classification_models import (
    EngineLayer,
    LlmClassifier,
    NoOpLlmClassifier,
    NoOpSmallNerEngine,
    SensitivityLevel,
    SmallNerEngine,
)

from ._pretty import print_result


# ---------------------------------------------------------------------------
# Mock 引擎 / Mock Engines
# ---------------------------------------------------------------------------


class FakeNerEngine(SmallNerEngine):
    """返回固定实体列表的 Mock NER 引擎，用于测试 enable_small_ner 参数。"""

    def __init__(self, entities: list[dict[str, Any]]):
        self._entities = entities

    def extract(self, text: str) -> list[dict[str, Any]]:
        return self._entities


class FakeLlmClassifier(LlmClassifier):
    """返回固定结果的 Mock LLM 分类器，用于测试 enable_llm 参数。"""

    def __init__(self, result: dict[str, Any] | None):
        self._result = result
        self.called = False

    def classify(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        self.called = True
        return self._result


# ---------------------------------------------------------------------------
# enable_rule_engine 参数 / enable_rule_engine parameter
# ---------------------------------------------------------------------------


def test_enable_rule_engine_false_skips_rules():
    """禁用规则引擎后，身份证号不应命中 PII_ID_CARD。

    With enable_rule_engine=False, id_card value should NOT trigger PII_ID_CARD.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "id_card",
        "110101199001011237",
        params={"enableRuleEngine": False, "enableLlm": False},
    )
    print_result(result)
    categories = [t.category for t in result.tags]
    assert "PII_ID_CARD" not in categories
    # 无规则命中时置信度应为 0.0
    assert result.confidence == 0.0


def test_enable_rule_engine_true_by_default():
    """默认启用规则引擎，身份证号应命中 PII_ID_CARD。

    With default params (enable_rule_engine=True), id_card should trigger PII_ID_CARD.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field("id_card", "110101199001011237")
    print_result(result)
    assert any(t.category == "PII_ID_CARD" for t in result.tags)
    assert result.final_level == SensitivityLevel.L3


# ---------------------------------------------------------------------------
# enable_small_ner 参数 / enable_small_ner parameter
# ---------------------------------------------------------------------------


def test_enable_small_ner_true_triggers_ner():
    """启用 NER 且注入 Mock 引擎后，应产生 NER 标签并标记决策层为 L2。

    With enable_small_ner=True and a mock NER engine returning a MEDICAL_DISEASE entity,
    the result should contain NER tags and engine_layer should be L2_SMALL_NER.
    """
    fake_ner = FakeNerEngine([
        {"label": "MEDICAL_DISEASE", "text": "感冒", "confidence": 0.92},
    ])
    api = ClassificationAPI(small_ner=fake_ner, llm=NoOpLlmClassifier())
    result = api.classify_field(
        "note",
        "患者主诉感冒三天",
        params={"enableSmallNer": True},
    )
    print_result(result)
    assert any(t.category == "MEDICAL_DISEASE" for t in result.tags)
    assert result.engine_layer == EngineLayer.L2_SMALL_NER
    assert result.final_level == SensitivityLevel.L3


def test_enable_small_ner_false_by_default():
    """默认不启用 NER，即使注入了 Mock NER 引擎也不应产生 NER 标签。

    With default params (enable_small_ner=False), NER tags should NOT appear
    even if a mock NER engine is injected.
    """
    fake_ner = FakeNerEngine([
        {"label": "MEDICAL_DISEASE", "text": "感冒", "confidence": 0.92},
    ])
    api = ClassificationAPI(small_ner=fake_ner, llm=NoOpLlmClassifier())
    result = api.classify_field("note", "患者主诉感冒三天")
    print_result(result)
    assert not any(t.category == "MEDICAL_DISEASE" for t in result.tags)


def test_enable_small_ner_sensitive_disease_l4():
    """启用 NER 后，含敏感关键字的疾病实体应升级为 L4。

    With enable_small_ner=True, a disease entity containing sensitive keywords
    (e.g. HIV) should be escalated to L4.
    """
    fake_ner = FakeNerEngine([
        {"label": "MEDICAL_DISEASE", "text": "HIV感染", "confidence": 0.95},
    ])
    api = ClassificationAPI(small_ner=fake_ner, llm=NoOpLlmClassifier())
    result = api.classify_field(
        "diagnosis_note",
        "患者确诊HIV感染",
        params={"enableSmallNer": True},
    )
    print_result(result)
    assert any(t.category == "MEDICAL_SENSITIVE_DISEASE" for t in result.tags)
    assert result.final_level == SensitivityLevel.L4


def test_enable_small_ner_genomic_hint_l5():
    """启用 NER 后，基因组提示实体应标记为 L5 且需人工复核。

    With enable_small_ner=True, a GENOMIC_HINT entity should be marked L5
    and flagged for human review.
    """
    fake_ner = FakeNerEngine([
        {"label": "GENOMIC_HINT", "text": "BRCA1突变", "confidence": 0.88},
    ])
    api = ClassificationAPI(small_ner=fake_ner, llm=NoOpLlmClassifier())
    # 使用不含基因组关键词的字段名，避免规则引擎抢先命中 GENOMIC_HINT
    result = api.classify_field(
        "clinical_note",
        "检测到BRCA1突变",
        params={"enableSmallNer": True},
    )
    print_result(result)
    genomic_tags = [t for t in result.tags if t.category == "GENOMIC_HINT"]
    assert genomic_tags
    assert genomic_tags[0].level == SensitivityLevel.L5
    assert genomic_tags[0].needs_human_review is True
    assert result.final_level == SensitivityLevel.L5


# ---------------------------------------------------------------------------
# enable_llm 参数 / enable_llm parameter
# ---------------------------------------------------------------------------


def test_enable_llm_true_uses_llm_result():
    """显式启用 LLM 后，应使用 LLM 返回的等级和推理说明。

    With enable_llm=True, the LLM result should override the upstream level
    and engine_layer should be L3_LLM.
    """
    fake_llm = FakeLlmClassifier({
        "final_level": "L4",
        "confidence": 0.97,
        "reasoning": "LLM 判定为高敏感医疗数据",
    })
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    result = api.classify_field(
        "note",
        "患者确诊艾滋病",
        params={"enableLlm": True},
    )
    print_result(result)
    assert fake_llm.called
    assert result.final_level == SensitivityLevel.L4
    assert result.engine_layer == EngineLayer.L3_LLM
    assert result.confidence == 0.97
    assert "LLM" in result.reasoning


def test_enable_llm_false_by_default():
    """默认不启用 LLM，Mock LLM 不应被调用（置信度足够高时）。

    With default params (enable_llm=False) and high upstream confidence,
    the mock LLM should NOT be called.
    """
    fake_llm = FakeLlmClassifier({"final_level": "L5", "confidence": 1.0, "reasoning": "x"})
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    # id_card 命中规则后置信度为 1.0，默认阈值 0.6，不触发 LLM
    result = api.classify_field("id_card", "110101199001011237")
    print_result(result)
    assert not fake_llm.called
    assert result.engine_layer == EngineLayer.L1_RULE


def test_enable_llm_returns_none_keeps_upstream():
    """LLM 返回 None 时，应保持上游引擎的等级和决策层。

    When LLM returns None, the upstream level and engine_layer should be preserved.
    """
    fake_llm = FakeLlmClassifier(None)
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    result = api.classify_field(
        "id_card",
        "110101199001011237",
        params={"enableLlm": True},
    )
    print_result(result)
    assert fake_llm.called
    # LLM 返回 None，保持 L1_RULE 决策层和 L3 等级
    assert result.final_level == SensitivityLevel.L3
    assert result.engine_layer == EngineLayer.L1_RULE


# ---------------------------------------------------------------------------
# llm_confidence_threshold 参数 / llm_confidence_threshold parameter
# ---------------------------------------------------------------------------


def test_llm_triggered_when_confidence_below_threshold():
    """上游置信度低于阈值时应触发 LLM（即使 enable_llm=False）。

    When upstream confidence is below llm_confidence_threshold,
    LLM should be triggered even with enable_llm=False.
    """
    fake_llm = FakeLlmClassifier({
        "final_level": "L2",
        "confidence": 0.80,
        "reasoning": "LLM 复核",
    })
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    # 未知字段无规则命中，置信度 0.0 < 阈值 0.9 → 触发 LLM
    result = api.classify_field(
        "unknown_field",
        "some value",
        params={"llmConfidenceThreshold": 0.9},
    )
    print_result(result)
    assert fake_llm.called
    assert result.engine_layer == EngineLayer.L3_LLM


def test_llm_not_triggered_when_confidence_above_threshold():
    """上游置信度高于阈值时不应触发 LLM（enable_llm=False）。

    When upstream confidence is above llm_confidence_threshold,
    LLM should NOT be triggered with enable_llm=False.
    """
    fake_llm = FakeLlmClassifier({"final_level": "L5", "confidence": 1.0, "reasoning": "x"})
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    # id_card 命中规则，置信度 1.0 > 阈值 0.5 → 不触发 LLM
    result = api.classify_field(
        "id_card",
        "110101199001011237",
        params={"llmConfidenceThreshold": 0.5},
    )
    print_result(result)
    assert not fake_llm.called
    assert result.engine_layer == EngineLayer.L1_RULE


def test_llm_threshold_zero_never_triggers():
    """阈值为 0 时，置信度 0.0 也不触发 LLM（0.0 < 0 为假）。

    With llm_confidence_threshold=0.0, LLM should never be triggered
    by low confidence (0.0 < 0.0 is False).
    """
    fake_llm = FakeLlmClassifier({"final_level": "L5", "confidence": 1.0, "reasoning": "x"})
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    result = api.classify_field(
        "unknown_field",
        "some value",
        params={"llmConfidenceThreshold": 0.0, "enableLlm": False},
    )
    print_result(result)
    assert not fake_llm.called


# ---------------------------------------------------------------------------
# return_field_values 参数 / return_field_values parameter
# ---------------------------------------------------------------------------


def test_return_field_values_true_default():
    """默认 return_field_values=True，结果中应包含字段原始值。

    With default return_field_values=True, field_value should be present in result.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field("mobile", "13800138000")
    print_result(result)
    assert result.field_value == "13800138000"


def test_return_field_values_false():
    """return_field_values=False 时，field_value 应为 None。

    With return_field_values=False, field_value should be None.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "mobile",
        "13800138000",
        params={"returnFieldValues": False},
    )
    print_result(result)
    assert result.field_value is None


def test_return_field_values_false_record_level():
    """return_field_values=False 时，记录级各字段的 field_value 均应为 None。

    With return_field_values=False, all field_value in record results should be None.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_record(
        {"id_card": "110101199001011237", "mobile": "13800138000"},
        params={"returnFieldValues": False},
    )
    print_result(result)
    for field_result in result.field_results.values():
        assert field_result.field_value is None


# ---------------------------------------------------------------------------
# enable_review 参数 / enable_review parameter
# ---------------------------------------------------------------------------


def test_enable_review_true_collects_entries():
    """默认 enable_review=True，L5 复合规则应产生复核条目。

    With default enable_review=True, L5 composite rule should produce review entries.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_table(
        schema=["name", "id_card", "mobile"],
        rows=[{"name": "张三", "id_card": "110101199001011237", "mobile": "13800138000"}],
    )
    print_result(result)
    assert result.needs_human_review
    assert result.review_entries  # 非空


def test_enable_review_false_no_entries():
    """enable_review=False 时，即使存在需复核字段也不收集复核条目。

    With enable_review=False, review_entries should be empty even when
    needs_human_review is True.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_table(
        schema=["name", "id_card", "mobile"],
        rows=[{"name": "张三", "id_card": "110101199001011237", "mobile": "13800138000"}],
        params={"enableReview": False},
    )
    print_result(result)
    assert result.review_entries == []


# ---------------------------------------------------------------------------
# icd10_l4_intervals 参数 / icd10_l4_intervals parameter
# ---------------------------------------------------------------------------


def test_custom_icd10_l4_intervals():
    """自定义 ICD-10 L4 区间：将糖尿病 E10-E14 加入敏感区间后应升级为 L4。

    Custom icd10_l4_intervals: adding E10-E14 (diabetes) should escalate to L4.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    custom_intervals = [
        {"start": "E10", "end": "E14"},  # 糖尿病区间
    ]
    result = api.classify_field(
        "diagnosis",
        "E11.9",  # 2型糖尿病，未特指
        params={"icd10L4Intervals": custom_intervals},
    )
    print_result(result)
    assert result.final_level == SensitivityLevel.L4
    assert any(t.category == "MEDICAL_ICD10_GENERAL" for t in result.tags)


def test_default_icd10_intervals_not_affected_by_custom():
    """自定义区间不包含默认 HIV 区间时，B21.1 应回退为 L3 一般医疗编码。

    When custom intervals do not include the default HIV interval,
    B21.1 should fall back to L3 (general medical code).
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    custom_intervals = [
        {"start": "E10", "end": "E14"},  # 仅糖尿病，不含 HIV
    ]
    result = api.classify_field(
        "diagnosis",
        "B21.1",
        params={"icd10L4Intervals": custom_intervals},
    )
    print_result(result)
    # B21.1 不在自定义区间内，应为 L3
    assert result.final_level == SensitivityLevel.L3
    assert any(t.category == "MEDICAL_ICD10_GENERAL" for t in result.tags)


# ---------------------------------------------------------------------------
# public_field_whitelist 参数 / public_field_whitelist parameter
# ---------------------------------------------------------------------------


def test_custom_public_field_whitelist():
    """自定义公开字段白名单：新增 open_data 后该字段应标记为 L1。

    Custom public_field_whitelist: adding 'open_data' should mark that field as L1.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "open_data",
        "2023年公开年报",
        params={"publicFieldWhitelist": ["open_data"]},
    )
    print_result(result)
    assert any(t.category == "PUBLIC_REPORT" for t in result.tags)
    assert result.final_level == SensitivityLevel.L1


def test_default_public_whitelist_not_overridden():
    """自定义白名单替换默认值后，原默认字段 public_report 不再命中。

    When custom whitelist replaces defaults, the original default field
    'public_report' should no longer trigger PUBLIC_REPORT.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "public_report",
        "2023 annual summary",
        params={"publicFieldWhitelist": ["open_data"]},  # 替换默认白名单
    )
    print_result(result)
    # public_report 不在新白名单中，不应命中 PUBLIC_REPORT
    assert not any(t.category == "PUBLIC_REPORT" for t in result.tags)


# ---------------------------------------------------------------------------
# operational_field_patterns 参数 / operational_field_patterns parameter
# ---------------------------------------------------------------------------


def test_custom_operational_field_patterns():
    """自定义运营字段模式：新增 cpu_usage 后该字段应标记为 L2。

    Custom operational_field_patterns: adding 'cpu_usage' should mark that field as L2.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "cpu_usage",
        "78.5%",
        params={"operationalFieldPatterns": ["cpu_usage"]},
    )
    print_result(result)
    assert any(t.category == "OPERATIONAL_STAT" for t in result.tags)
    assert result.final_level == SensitivityLevel.L2


def test_default_operational_pattern_not_overridden():
    """自定义运营字段模式替换默认值后，原默认字段 turnover_rate 不再命中。

    When custom patterns replace defaults, the original default field
    'turnover_rate' should no longer trigger OPERATIONAL_STAT.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "turnover_rate",
        "0.85",
        params={"operationalFieldPatterns": ["cpu_usage"]},  # 替换默认模式
    )
    print_result(result)
    assert not any(t.category == "OPERATIONAL_STAT" for t in result.tags)


# ---------------------------------------------------------------------------
# default_level 参数 / default_level parameter
# ---------------------------------------------------------------------------


def test_default_level_l1():
    """default_level=L1 时，无规则命中的未知字段应返回 L1。

    With default_level=L1, an unknown field with no rule hits should return L1.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "some_unknown_field",
        "random value",
        params={"defaultLevel": "L1", "llmConfidenceThreshold": 0.0},
    )
    print_result(result)
    assert result.final_level == SensitivityLevel.L1


def test_default_level_l4():
    """default_level=L4 时，无规则命中的未知字段应返回 L4。

    With default_level=L4, an unknown field with no rule hits should return L4.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    result = api.classify_field(
        "some_unknown_field",
        "random value",
        params={"defaultLevel": "L4", "llmConfidenceThreshold": 0.0},
    )
    print_result(result)
    assert result.final_level == SensitivityLevel.L4


def test_default_level_does_not_override_rule_hit():
    """default_level 不影响已有规则命中的字段等级。

    default_level should NOT override the level of fields that already have rule hits.
    """
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=NoOpLlmClassifier())
    # id_card 命中 PII_ID_CARD (L3)，default_level=L1 不应降低它
    result = api.classify_field(
        "id_card",
        "110101199001011237",
        params={"defaultLevel": "L1"},
    )
    print_result(result)
    assert result.final_level == SensitivityLevel.L3


# ---------------------------------------------------------------------------
# 参数组合 / Parameter combinations
# ---------------------------------------------------------------------------


def test_disable_rule_engine_and_enable_llm():
    """禁用规则引擎同时启用 LLM：LLM 应接管分类决策。

    With enable_rule_engine=False and enable_llm=True, LLM should take over
    the classification decision.
    """
    fake_llm = FakeLlmClassifier({
        "final_level": "L3",
        "confidence": 0.85,
        "reasoning": "LLM 直接分类",
    })
    api = ClassificationAPI(small_ner=NoOpSmallNerEngine(), llm=fake_llm)
    result = api.classify_field(
        "id_card",
        "110101199001011237",
        params={"enableRuleEngine": False, "enableLlm": True},
    )
    print_result(result)
    assert fake_llm.called
    assert result.engine_layer == EngineLayer.L3_LLM
    # 规则引擎被禁用，不应有 PII_ID_CARD 标签
    assert not any(t.category == "PII_ID_CARD" for t in result.tags)


def test_ner_and_llm_combined():
    """同时启用 NER 和 LLM：NER 命中后置信度足够高时 LLM 不应覆盖。

    With both enable_small_ner=True and enable_llm=False, NER hits with high
    confidence should not trigger LLM.
    """
    fake_ner = FakeNerEngine([
        {"label": "MEDICATION", "text": "阿司匹林", "confidence": 0.99},
    ])
    fake_llm = FakeLlmClassifier({"final_level": "L5", "confidence": 1.0, "reasoning": "x"})
    api = ClassificationAPI(small_ner=fake_ner, llm=fake_llm)
    result = api.classify_field(
        "medication_note",
        "服用阿司匹林",
        params={"enableSmallNer": True, "llmConfidenceThreshold": 0.6},
    )
    print_result(result)
    # NER 置信度 0.99 > 阈值 0.6，LLM 不应被触发
    assert not fake_llm.called
    assert result.engine_layer == EngineLayer.L2_SMALL_NER
    assert result.final_level == SensitivityLevel.L3
