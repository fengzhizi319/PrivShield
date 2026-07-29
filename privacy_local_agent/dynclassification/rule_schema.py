"""声明式规则 Profile 数据模型 / Declarative Rule Profile Schema.

定义规则匹配器（Matcher）、规则（Rule）、降级规则、复合规则、
领域规则包（RuleProfile）和标准组合（StandardDef）的 Pydantic 模型。

所有规则均通过 YAML/JSON 声明式定义，引擎仅负责解释执行。
"""

from __future__ import annotations

from typing import Any, Optional

# Pydantic v2 imports: BaseModel for schema definition, ConfigDict for model config,
# Field for metadata annotations (description, alias, default).
from pydantic import BaseModel, ConfigDict, Field


# ===========================================================================
# 匹配器定义 / Matcher Definition
# ===========================================================================


class MatcherDef(BaseModel):
    """单个匹配器定义。

    描述对字段名或字段值执行何种算子匹配。
    一个规则可包含多个匹配器，通过 match_logic 决定组合逻辑。
    """

    # Allow instantiation using both Python field names and JSON/YAML alias names.
    model_config = ConfigDict(populate_by_name=True)

    # Matching target: either "field_name" (match against column name) or
    # "field_value" (match against the actual data value).
    target: str = Field(description="匹配目标: 'field_name' | 'field_value'")
    # Operator name registered in OperatorRegistry, e.g. 'regex', 'keyword_contains',
    # 'id_card_checksum', 'icd10_range', 'luhn_checksum', etc.
    operator: str = Field(description="算子名称: 'regex' | 'keyword_contains' | 'id_card_checksum' 等")
    # Operator-specific parameters dict (e.g. {"pattern": "..."} for regex,
    # {"keywords": [...]} for keyword_contains, {"intervals": [...]} for icd10_range).
    params: dict[str, Any] = Field(default_factory=dict, description="算子参数（如 pattern、keywords、intervals）")


# ===========================================================================
# 规则定义 / Rule Definitions
# ===========================================================================


class RuleDef(BaseModel):
    """单条声明式规则定义。

    规则是分类引擎的最小执行单元，包含一组匹配器和命中后的标签信息。
    """

    # Allow both snake_case and camelCase field access.
    model_config = ConfigDict(populate_by_name=True)

    # Unique rule identifier, used for metrics labeling and audit trail.
    id: str = Field(description="规则唯一标识，如 'RULE_PII_IDCARD'")
    # Human-readable rule name for display in management UIs.
    name: str = Field(default="", description="规则名称（人类可读）")
    # Category ID assigned to the field when this rule hits (e.g. 'PII_ID_CARD').
    category: str = Field(description="命中后的分类类别 ID")
    # Sensitivity level ID assigned when this rule hits (e.g. 'L3', 'C4').
    level: str = Field(description="命中后的敏感度等级 ID")
    # List of matchers to evaluate; combined via match_logic (AND/OR).
    matchers: list[MatcherDef] = Field(default_factory=list, description="匹配器列表")
    # Logic for combining multiple matchers: "AND" = all must hit, "OR" = any one hit suffices.
    match_logic: str = Field(default="AND", description="多匹配器逻辑: 'AND'(全部命中) | 'OR'(任一命中)")
    # Execution priority: higher values execute first. Used for sorting after merge.
    priority: int = Field(default=0, description="优先级（数值越大越先执行）")
    # Enable/disable toggle: disabled rules are skipped during profile merge.
    enabled: bool = Field(default=True, description="是否启用")
    # Extension tags for custom metadata (key-value pairs, not interpreted by engine).
    tags: dict[str, str] = Field(default_factory=dict, description="扩展标签（自定义元数据）")


class DowngradeRuleDef(BaseModel):
    """降级规则定义。

    当字段名匹配指定关键词时，将等级降级到目标等级。
    典型场景：公开字段降为 L1，运营统计字段降为 L2。

    强制覆盖模式（override=true）：
        默认情况下，降级规则仅作为"兜底归属"——在无普通规则命中时替代默认等级。
        当设置 override=true 后，降级规则可强制压制 rank <= max_override_level 的
        普通规则标签，解决宽泛规则误中运营/公开字段的问题。

        执行流程:
        ┌──────────────────────────────────────────────────────────────┐
        │  override=false (默认):                                       │
        │    降级标签 + 普通标签 → 取 max → 降级无效（仅兜底）          │
        │                                                              │
        │  override=true:                                              │
        │    先移除 rank <= cap 的普通标签 → 再取 max → 降级生效        │
        └──────────────────────────────────────────────────────────────┘
    """

    model_config = ConfigDict(populate_by_name=True)

    # Unique identifier for this downgrade rule.
    id: str = Field(description="规则唯一标识")
    # Human-readable name for display purposes.
    name: str = Field(default="", description="规则名称")
    # Keywords to match against normalized field names (case-insensitive, underscore-stripped).
    keywords: list[str] = Field(default_factory=list, description="关键词列表")
    # Target level to downgrade to (e.g. 'L1' for public fields, 'L2' for operational stats).
    level: str = Field(description="降级目标等级")
    # Category assigned when downgrade rule fires.
    category: str = Field(description="分类类别")
    # What to match against (currently only 'field_name' is supported).
    match_target: str = Field(default="field_name", description="匹配目标")
    # Whether this downgrade rule can forcibly suppress normal rule tags.
    # When true, normal tags with rank <= max_override_level's rank are removed.
    # Default false: backward-compatible, downgrade tag only serves as fallback.
    override: bool = Field(default=False, description="是否启用强制覆盖（压制低等级普通规则标签）")
    # Maximum level this override can suppress (inclusive).
    # E.g. 'L3' means only normal tags with rank <= rank('L3') can be suppressed.
    # Empty string defaults to using this rule's own 'level' field as the cap.
    max_override_level: str = Field(default="", description="覆盖等级上限（空=使用 level 字段）")
    # Fine-grained suppression whitelist: only normal rules whose ID appears here
    # can be suppressed by this override rule.
    # Empty list (default) = suppress ALL eligible normal tags (backward-compatible).
    # Non-empty = only tags from listed rule IDs are candidates for suppression.
    suppress_rules: list[str] = Field(
        default_factory=list,
        description="压制白名单: 仅列出的规则 ID 可被压制（空=压制所有符合条件的规则）",
    )


class CompositeRuleDef(BaseModel):
    """复合规则定义（记录级）。

    当一条记录中同时有 >= min_matches 个字段匹配指定的字段模式时，
    将整条记录的敏感度升级为 target_level。
    """

    model_config = ConfigDict(populate_by_name=True)

    # Unique identifier for this composite rule.
    id: str = Field(description="规则唯一标识")
    # Human-readable name.
    name: str = Field(default="", description="规则名称")
    # List of regex patterns to match against field names in a record.
    # Each pattern that matches at least one field counts as one "hit".
    field_patterns: list[str] = Field(default_factory=list, description="字段名正则列表")
    # Minimum number of pattern hits required to trigger this composite rule.
    # E.g. min_matches=3 means at least 3 different patterns must match fields.
    min_matches: int = Field(default=1, description="最低匹配数")
    # The upgraded sensitivity level when this composite rule fires (e.g. 'L5').
    target_level: str = Field(description="升级目标等级")
    # Category assigned when composite rule fires.
    category: str = Field(description="分类类别")


# ===========================================================================
# 规则 Profile 与标准组合 / Rule Profile & Standard Definition
# ===========================================================================


class RuleProfile(BaseModel):
    """规则 Profile 完整定义（一个领域包）。

    一个 RuleProfile 对应一个行业领域的全部规则集合，
    包含普通规则、降级规则和复合规则三类。
    """

    model_config = ConfigDict(populate_by_name=True)

    # Domain identifier this profile belongs to (e.g. 'medical', 'finance', 'general-pii').
    domain: str = Field(description="所属领域，如 'medical', 'finance', 'general-pii'")
    # Semantic version of this rule pack for audit trail.
    version: str = Field(default="1.0.0", description="版本号")
    # Human-readable description of this domain pack.
    description: str = Field(default="", description="领域包说明")
    # Primary classification rules evaluated per-field.
    rules: list[RuleDef] = Field(default_factory=list, description="普通规则列表")
    # Downgrade rules that lower sensitivity for specific field patterns.
    downgrade_rules: list[DowngradeRuleDef] = Field(default_factory=list, description="降级规则列表")
    # Composite rules evaluated at record level (multi-field combination).
    composite_rules: list[CompositeRuleDef] = Field(default_factory=list, description="复合规则列表")


class StandardDef(BaseModel):
    """标准组合定义。

    一个标准 = 多个领域包组合 + 参数覆盖 + 规则级覆盖 + 追加规则。
    例如 sc_health_db51 = general-pii + medical + 四川指南特有规则。
    """

    model_config = ConfigDict(populate_by_name=True)

    # Standard identifier (e.g. 'sc_health_db51', 'jrt0197', 'gbt35273').
    standard_id: str = Field(description="标准标识，如 'sc_health_db51', 'jrt0197'")
    # Human-readable description of this standard combination.
    description: str = Field(default="", description="标准说明")
    # Reference to the taxonomy YAML file name (without .yaml extension).
    taxonomy: str = Field(default="default", description="引用的 taxonomy 文件名")
    # List of domain profile names to combine (loaded from domains/ directory).
    domains: list[str] = Field(default_factory=list, description="组合的领域包列表")
    # Global parameter overrides applied to all rules in this standard.
    overrides: dict[str, Any] = Field(default_factory=dict, description="全局参数覆盖")
    # Per-rule attribute overrides: {rule_id: {field_name: new_value}}.
    # Allows a standard to adjust specific rules' level/category/priority.
    rule_overrides: dict[str, dict[str, Any]] = Field(
        default_factory=dict, description="规则级覆盖: {rule_id: {field: value}}"
    )
    # Additional rules appended to the combined profile (standard-specific).
    extra_rules: list[RuleDef] = Field(default_factory=list, description="追加规则")
    # Additional downgrade rules appended to the combined profile.
    extra_downgrade_rules: list[DowngradeRuleDef] = Field(
        default_factory=list, description="追加降级规则"
    )
    # Additional composite rules appended to the combined profile.
    extra_composite_rules: list[CompositeRuleDef] = Field(
        default_factory=list, description="追加复合规则"
    )
