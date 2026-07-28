"""动态分类分级模块 / Dynamic Classification Module.

提供声明式、可配置的数据分类分级引擎，支持多领域、多行业标准。
核心设计思想：标准配置化、规则声明化、算子插件化、执行上下文动态化。

本模块完全独立于旧分类引擎（privacy/classification/），无交叉依赖，
可与旧模块并行使用。

Usage:
    from privacy_local_agent.dynclassification import DynClassificationService

    service = DynClassificationService(rules_dir="rules")
    result = service.classify_field("phone_number", "13800138000")
"""

from .composite import CompositeRuleEngine
from .engine import ConfigurableRuleEngine
from .generator import StandardDocParser
from .models import (
    AuditInfo,
    CategoryDef,
    ClassificationResponse,
    DomainTaxonomy,
    FieldClassificationResult,
    RecordClassificationResult,
    SecurityTag,
    SensitivityLevelDef,
    TableClassificationResult,
)
from .operator_registry import MatcherOperator, OperatorRegistry
from .profile_loader import ProfileLoader
from .rule_schema import (
    CompositeRuleDef,
    DowngradeRuleDef,
    MatcherDef,
    RuleDef,
    RuleProfile,
    StandardDef,
)
from .service import DynClassificationService

__all__ = [
    # 服务入口
    "DynClassificationService",
    # 生成器
    "StandardDocParser",
    # 引擎

    "ConfigurableRuleEngine",
    "CompositeRuleEngine",
    # 加载器
    "ProfileLoader",
    # 算子注册
    "OperatorRegistry",
    "MatcherOperator",
    # 数据模型
    "DomainTaxonomy",
    "SensitivityLevelDef",
    "CategoryDef",
    "SecurityTag",
    "FieldClassificationResult",
    "RecordClassificationResult",
    "TableClassificationResult",
    "AuditInfo",
    "ClassificationResponse",
    # 规则模型
    "MatcherDef",
    "RuleDef",
    "DowngradeRuleDef",
    "CompositeRuleDef",
    "RuleProfile",
    "StandardDef",
]
