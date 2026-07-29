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

# --- Import composite rule engine for record-level multi-field combination evaluation ---
from .composite import CompositeRuleEngine
# --- Import the core configurable rule engine that interprets declarative RuleProfile ---
from .engine import ConfigurableRuleEngine
# --- Import the 3-layer funnel orchestrator (Rule → NER → LLM) ---
from .funnel import ClassificationFunnel, FunnelResult
# --- Import the standard document parser for auto-generating YAML configs from Markdown ---
from .generator import StandardDocParser
# --- Import LLM adapter for Layer-3 deep classification and arbitration ---
from .llm_adapter import LlmAdapter
# --- Import all Pydantic data models used across the module ---
from .models import (
    AuditInfo,                     # Audit metadata for classification requests
    CategoryDef,                   # Dynamic category definition (replaces hardcoded enum)
    ClassificationResponse,        # Top-level response wrapper for all classification APIs
    ConfidencePolicy,              # Confidence decay and LLM arbitration policy
    DomainTaxonomy,                # Full taxonomy definition (levels + categories)
    EngineLayer,                   # Engine layer constants (L1_RULE/L2_SMALL_NER/L3_LLM)
    FieldClassificationResult,     # Single field classification output
    RecordClassificationResult,    # Multi-field record classification output
    SecurityTag,                   # Atomic security tag produced by each rule hit
    SensitivityLevelDef,           # Dynamic sensitivity level definition
    TableClassificationResult,     # Table/batch-level classification output
)
# --- Import NER adapter for Layer-2 entity extraction ---
from .ner_adapter import NerAdapter
# --- Import operator registry and protocol for plugin-style matcher management ---
from .operator_registry import MatcherOperator, OperatorRegistry
# --- Import profile loader responsible for YAML loading, caching and hot-reload ---
from .profile_loader import ProfileLoader
# --- Import declarative rule schema models (matchers, rules, profiles, standards) ---
from .rule_schema import (
    CompositeRuleDef,              # Record-level composite rule definition
    DowngradeRuleDef,              # Downgrade rule definition (lower sensitivity)
    MatcherDef,                    # Single matcher definition (operator + params)
    RuleDef,                       # Single classification rule definition
    RuleProfile,                   # Domain rule pack (collection of rules)
    StandardDef,                   # Standard combination definition (multi-domain)
)
# --- Import the high-level service entry point ---
from .service import DynClassificationService

# --- Public API surface: controls what is exported via `from dynclassification import *` ---
__all__ = [
    # Service entry point
    "DynClassificationService",
    # Document-to-YAML generator
    "StandardDocParser",
    # Rule engines
    "ConfigurableRuleEngine",
    "CompositeRuleEngine",
    # 3-layer funnel
    "ClassificationFunnel",
    "FunnelResult",
    # ML adapters (Layer-2 NER, Layer-3 LLM)
    "NerAdapter",
    "LlmAdapter",
    # Configuration loader
    "ProfileLoader",
    # Operator plugin registry
    "OperatorRegistry",
    "MatcherOperator",
    # Data models
    "DomainTaxonomy",
    "SensitivityLevelDef",
    "CategoryDef",
    "SecurityTag",
    "FieldClassificationResult",
    "RecordClassificationResult",
    "TableClassificationResult",
    "AuditInfo",
    "ClassificationResponse",
    "ConfidencePolicy",
    "EngineLayer",
    # Rule schema models
    "MatcherDef",
    "RuleDef",
    "DowngradeRuleDef",
    "CompositeRuleDef",
    "RuleProfile",
    "StandardDef",
]
