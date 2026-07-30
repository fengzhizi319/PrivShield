"""测试类与字段命名规范及别名兼容性。

确保重构后的异常类、首字母缩写别名（LDP, QOL, KAnonymity, NER, LLM）及增强属性在运行时工作正常。
"""

from privacy_local_agent.privacy.budget import PrivacyBudgetExhausted, PrivacyBudgetExhaustedError
from privacy_local_agent.schemas import (
    KAnonRequest,
    KAnonymityRequest,
    LdpPerturbBinaryRequest,
    LDPPerturbBinaryRequest,
    QolRequest,
    QOLRequest,
    QueryObfuscationRequest,
)
from privacy_local_agent.dynclassification.base import SmallNerEngine, SmallNEREngine, LlmClassifier, LLMClassifier
from privacy_local_agent.dynclassification.service import DynClassificationService, DynamicClassificationService
from privacy_local_agent.dynclassification.models import TableClassificationResult


def test_exception_naming_convention():
    """测试异常类命名及其兼容别名。"""
    assert issubclass(PrivacyBudgetExhaustedError, Exception)
    assert PrivacyBudgetExhausted is PrivacyBudgetExhaustedError

    err = PrivacyBudgetExhaustedError("Budget exhausted")
    assert isinstance(err, Exception)


def test_acronym_schema_aliases():
    """测试首字母缩写词 REST Schema 别名。"""
    assert LDPPerturbBinaryRequest is LdpPerturbBinaryRequest
    assert QOLRequest is QolRequest
    assert QueryObfuscationRequest is QolRequest
    assert KAnonymityRequest is KAnonRequest

    req = LDPPerturbBinaryRequest(values=[1, 0, 1], epsilon=0.5)
    assert req.values == [1, 0, 1]
    assert req.epsilon == 0.5


def test_engine_acronym_aliases():
    """测试引擎接口缩写别名。"""
    assert SmallNEREngine is SmallNerEngine
    assert LLMClassifier is LlmClassifier
    assert DynamicClassificationService is DynClassificationService


def test_table_classification_result_columns_property():
    """测试 TableClassificationResult 的 columns 属性别名。"""
    res = TableClassificationResult(
        schema=["id", "name", "age"],
        record_results=[],
        aggregated_tags=[],
        final_level="L1",
        confidence=1.0,
    )
    assert res.schema_ == ["id", "name", "age"]
    assert res.columns == ["id", "name", "age"]
