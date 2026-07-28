"""声明式规则 Profile 数据模型 / Declarative Rule Profile Schema.

定义规则匹配器（Matcher）、规则（Rule）、降级规则、复合规则、
领域规则包（RuleProfile）和标准组合（StandardDef）的 Pydantic 模型。

所有规则均通过 YAML/JSON 声明式定义，引擎仅负责解释执行。
"""

from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, ConfigDict, Field


# ===========================================================================
# 匹配器定义 / Matcher Definition
# ===========================================================================


class MatcherDef(BaseModel):
    """单个匹配器定义。

    描述对字段名或字段值执行何种算子匹配。
    一个规则可包含多个匹配器，通过 match_logic 决定组合逻辑。
    """

    model_config = ConfigDict(populate_by_name=True)

    target: str = Field(description="匹配目标: 'field_name' | 'field_value'")
    operator: str = Field(description="算子名称: 'regex' | 'keyword_contains' | 'id_card_checksum' 等")
    params: dict[str, Any] = Field(default_factory=dict, description="算子参数（如 pattern、keywords、intervals）")


# ===========================================================================
# 规则定义 / Rule Definitions
# ===========================================================================


class RuleDef(BaseModel):
    """单条声明式规则定义。

    规则是分类引擎的最小执行单元，包含一组匹配器和命中后的标签信息。
    """

    model_config = ConfigDict(populate_by_name=True)

    id: str = Field(description="规则唯一标识，如 'RULE_PII_IDCARD'")
    name: str = Field(default="", description="规则名称（人类可读）")
    category: str = Field(description="命中后的分类类别 ID")
    level: str = Field(description="命中后的敏感度等级 ID")
    matchers: list[MatcherDef] = Field(default_factory=list, description="匹配器列表")
    match_logic: str = Field(default="AND", description="多匹配器逻辑: 'AND'(全部命中) | 'OR'(任一命中)")
    priority: int = Field(default=0, description="优先级（数值越大越先执行）")
    enabled: bool = Field(default=True, description="是否启用")
    tags: dict[str, str] = Field(default_factory=dict, description="扩展标签（自定义元数据）")


class DowngradeRuleDef(BaseModel):
    """降级规则定义。

    当字段名匹配指定关键词时，将等级降级到目标等级。
    典型场景：公开字段降为 L1，运营统计字段降为 L2。
    """

    model_config = ConfigDict(populate_by_name=True)

    id: str = Field(description="规则唯一标识")
    name: str = Field(default="", description="规则名称")
    keywords: list[str] = Field(default_factory=list, description="关键词列表")
    level: str = Field(description="降级目标等级")
    category: str = Field(description="分类类别")
    match_target: str = Field(default="field_name", description="匹配目标")


class CompositeRuleDef(BaseModel):
    """复合规则定义（记录级）。

    当一条记录中同时有 >= min_matches 个字段匹配指定的字段模式时，
    将整条记录的敏感度升级为 target_level。
    """

    model_config = ConfigDict(populate_by_name=True)

    id: str = Field(description="规则唯一标识")
    name: str = Field(default="", description="规则名称")
    field_patterns: list[str] = Field(default_factory=list, description="字段名正则列表")
    min_matches: int = Field(default=1, description="最低匹配数")
    target_level: str = Field(description="升级目标等级")
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

    domain: str = Field(description="所属领域，如 'medical', 'finance', 'general-pii'")
    version: str = Field(default="1.0.0", description="版本号")
    description: str = Field(default="", description="领域包说明")
    rules: list[RuleDef] = Field(default_factory=list, description="普通规则列表")
    downgrade_rules: list[DowngradeRuleDef] = Field(default_factory=list, description="降级规则列表")
    composite_rules: list[CompositeRuleDef] = Field(default_factory=list, description="复合规则列表")


class StandardDef(BaseModel):
    """标准组合定义。

    一个标准 = 多个领域包组合 + 参数覆盖 + 规则级覆盖 + 追加规则。
    例如 sc_health_db51 = general-pii + medical + 四川指南特有规则。
    """

    model_config = ConfigDict(populate_by_name=True)

    standard_id: str = Field(description="标准标识，如 'sc_health_db51', 'jrt0197'")
    description: str = Field(default="", description="标准说明")
    taxonomy: str = Field(default="default", description="引用的 taxonomy 文件名")
    domains: list[str] = Field(default_factory=list, description="组合的领域包列表")
    overrides: dict[str, Any] = Field(default_factory=dict, description="全局参数覆盖")
    rule_overrides: dict[str, dict[str, Any]] = Field(
        default_factory=dict, description="规则级覆盖: {rule_id: {field: value}}"
    )
    extra_rules: list[RuleDef] = Field(default_factory=list, description="追加规则")
    extra_downgrade_rules: list[DowngradeRuleDef] = Field(
        default_factory=list, description="追加降级规则"
    )
    extra_composite_rules: list[CompositeRuleDef] = Field(
        default_factory=list, description="追加复合规则"
    )
