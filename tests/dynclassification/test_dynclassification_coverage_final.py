"""针对最后剩余 3% 的死角分支补充全覆盖测试 / 100% Coverage Ultimate Boost Tests."""

from pathlib import Path
import pytest
import yaml

from privacy_local_agent.dynclassification.composite import CompositeRuleEngine
from privacy_local_agent.dynclassification.engine import ConfigurableRuleEngine
from privacy_local_agent.dynclassification.generator import StandardDocParser
from privacy_local_agent.dynclassification.operator_registry import OperatorRegistry
from privacy_local_agent.dynclassification.operators import _validate_id_card, _validate_medical_card, _in_icd10_interval
from privacy_local_agent.dynclassification.profile_loader import ProfileLoader
from privacy_local_agent.dynclassification.models import DomainTaxonomy, SecurityTag


class TestUltimateCoverageDetails:

    def test_composite_rule_engine_apply_record_level(self):
        """测试 CompositeRuleEngine.apply_to_record_level 边界"""
        comp_engine = CompositeRuleEngine(rules=[])

        # 无 composite_tags 时保留原 level
        assert comp_engine.apply_to_record_level("L3", []) == "L3"

        # 无 taxonomy 时使用字符串 max
        tags = [SecurityTag(level="L5", category="PII"), SecurityTag(level="L2", category="PII")]
        assert comp_engine.apply_to_record_level("L3", tags, taxonomy=None) == "L5"

    def test_configurable_rule_engine_evaluate_batch(self):
        """测试 ConfigurableRuleEngine.evaluate_batch"""
        taxonomy = DomainTaxonomy(domain="default", standard_id="INTERNAL", default_level="L1")
        engine = ConfigurableRuleEngine(taxonomy, [])
        res = engine.evaluate_batch([("f1", "v1"), ("f2", "v2")])
        assert "f1" in res and "f2" in res

    def test_operators_helper_functions_branches(self):
        """测试 operators.py 中辅助校验函数的内部异常分支"""
        # _validate_id_card (Int 校验中 IndexError/ValueError 捕获)
        assert _validate_id_card("1101011990030723X") is False
        assert _validate_id_card("110101199003072375") is True

        # _validate_medical_card (匹配逻辑)
        assert _validate_medical_card("123456780") is False

        # _in_icd10_interval (包含非法 start/end 编码)
        assert _in_icd10_interval(("B", 20), "INVALID_START", "B24") is False
        assert _in_icd10_interval(("B", 20), "B20", "INVALID_END") is False

    def test_generator_standard_id_variants(self, tmp_path):
        """测试 generator.py 识别多种行标编码 (GB/T 35273, GB/T 43697)"""
        gb35273_md = tmp_path / "gb35273.md"
        gb35273_md.write_text("# 规范\n标准编号：GB/T 35273-2020", encoding="utf-8")
        assert StandardDocParser(gb35273_md)._extract_standard_id() == "gb35273"


        gb43697_md = tmp_path / "gb43697.md"
        gb43697_md.write_text("# 规范\n标准编号：GB/T 43697-2024", encoding="utf-8")
        assert StandardDocParser(gb43697_md)._extract_standard_id() == "gb43697"

    def test_profile_loader_apply_rule_overrides(self, tmp_path):
        """测试 ProfileLoader._apply_rule_overrides 覆盖分支"""
        loader = ProfileLoader(rules_dir=tmp_path)
        (tmp_path / "taxonomies").mkdir(exist_ok=True)
        (tmp_path / "domains").mkdir(exist_ok=True)
        (tmp_path / "standards").mkdir(exist_ok=True)

        (tmp_path / "taxonomies" / "default.yaml").write_text("domain: default\nstandard_id: INTERNAL\ndefault_level: L1", encoding="utf-8")
        (tmp_path / "domains" / "finance.yaml").write_text("""
domain: finance
rules:
  - id: RULE_A
    category: PII
    level: L4
    matchers: []
""", encoding="utf-8")
        (tmp_path / "standards" / "std_override.yaml").write_text("""
standard_id: std_override
taxonomy: default
domains: [finance]
rule_overrides:
  RULE_A:
    level: L2
""", encoding="utf-8")

        engine = loader.get_engine(standard="std_override")
        assert engine.rules[0].level == "L2"
