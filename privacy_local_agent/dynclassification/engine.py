"""通用可配置规则引擎 / Configurable Rule Engine.

根据声明式 RuleProfile 动态执行规则匹配。
引擎本身不包含任何领域知识，仅负责解释执行规则配置。

执行流程：
1. 合并多个领域包的规则列表（按 priority 降序）
2. 遍历规则，对每条规则的 matchers 执行算子匹配
3. 根据 match_logic (AND/OR) 判断是否命中
4. 命中则生成 SecurityTag（支持 ICD-10 动态等级）
5. 执行降级规则
6. 去重返回
"""

from __future__ import annotations

from typing import Any

from ..observability.logging_config import get_logger
from . import operators  # noqa: F401 - Ensure built-in operators are registered
from .models import DomainTaxonomy, SecurityTag
from .operator_registry import OperatorRegistry
from .rule_schema import DowngradeRuleDef, MatcherDef, RuleDef, RuleProfile

logger = get_logger(__name__)


class ConfigurableRuleEngine:
    """通用可配置规则引擎。

    替代 DefaultRuleEngine 的硬编码逻辑，根据声明式 RuleProfile
    动态执行规则匹配。引擎本身不包含任何领域知识。

    Attributes:
        taxonomy: 分类体系定义（等级 + 类别）。
        rules: 合并后的规则列表（按 priority 降序）。
        downgrade_rules: 合并后的降级规则列表。
        domain: 当前引擎绑定的领域标识。
        standard_id: 当前引擎绑定的标准标识。
    """

    def __init__(
        self,
        taxonomy: DomainTaxonomy,
        profiles: list[RuleProfile],
        domain: str = "",
        standard_id: str = "",
    ):
        """初始化规则引擎。

        Args:
            taxonomy: 分类体系定义。
            profiles: 领域规则包列表（将被合并）。
            domain: 领域标识（写入标签）。
            standard_id: 标准标识（写入标签）。
        """
        self.taxonomy = taxonomy
        self.domain = domain
        self.standard_id = standard_id
        self.rules = self._merge_rules(profiles)
        self.downgrade_rules = self._merge_downgrade_rules(profiles)

    def _merge_rules(self, profiles: list[RuleProfile]) -> list[RuleDef]:
        """合并多个领域包的规则列表，按 priority 降序排列。"""
        all_rules: list[RuleDef] = []
        for profile in profiles:
            all_rules.extend(r for r in profile.rules if r.enabled)
        return sorted(all_rules, key=lambda r: r.priority, reverse=True)

    def _merge_downgrade_rules(self, profiles: list[RuleProfile]) -> list[DowngradeRuleDef]:
        """合并多个领域包的降级规则列表。"""
        all_rules: list[DowngradeRuleDef] = []
        for profile in profiles:
            all_rules.extend(profile.downgrade_rules)
        return all_rules

    def evaluate(
        self,
        field_name: str,
        value: Any,
        context: dict[str, Any] | None = None,
    ) -> list[SecurityTag]:
        """评估单个字段，返回命中的安全标签列表。

        Args:
            field_name: 字段名。
            value: 字段值。
            context: 可选的执行上下文（预留扩展）。

        Returns:
            命中的 SecurityTag 列表（已去重）。
        """
        tags: list[SecurityTag] = []
        str_value = str(value) if value is not None else ""

        # 执行普通规则
        for rule in self.rules:
            tag = self._evaluate_rule(rule, field_name, str_value)
            if tag is not None:
                tags.append(tag)

        # 执行降级规则
        tags.extend(self._evaluate_downgrade(field_name))

        return self._unique_tags(tags)

    def evaluate_batch(
        self,
        fields: list[tuple[str, Any]],
        context: dict[str, Any] | None = None,
    ) -> dict[str, list[SecurityTag]]:
        """批量评估多个字段。

        Args:
            fields: (字段名, 字段值) 元组列表。
            context: 可选的执行上下文。

        Returns:
            字段名 → 标签列表的映射。
        """
        results: dict[str, list[SecurityTag]] = {}
        for field_name, value in fields:
            results[field_name] = self.evaluate(field_name, value, context)
        return results

    def _evaluate_rule(self, rule: RuleDef, field_name: str, str_value: str) -> SecurityTag | None:
        """评估单条规则，命中则返回 SecurityTag，否则返回 None。"""
        if not rule.matchers:
            return None

        results: list[bool] = []
        hit_params: dict[str, Any] = {}  # 保存命中算子的 params（用于 ICD-10 动态等级）

        for matcher in rule.matchers:
            hit = self._execute_matcher(matcher, field_name, str_value)
            results.append(hit)
            if hit and matcher.operator == "icd10_range":
                hit_params = matcher.params

        # 根据 match_logic 判断是否命中
        if rule.match_logic.upper() == "OR":
            matched = any(results)
        else:  # AND
            matched = all(results)

        if not matched:
            return None

        # 确定最终等级和类别（支持 ICD-10 动态等级）
        level = rule.level
        category = rule.category
        if "_hit_level" in hit_params:
            level = hit_params["_hit_level"]
        if "_hit_category" in hit_params and hit_params["_hit_category"]:
            category = hit_params["_hit_category"]

        return SecurityTag(
            level=level,
            category=category,
            source_engine="RULE",
            rule_id=rule.id,
            domain=self.domain,
            standard_id=self.standard_id,
        )

    def _execute_matcher(self, matcher: MatcherDef, field_name: str, str_value: str) -> bool:
        """执行单个匹配器。"""
        try:
            op_func = OperatorRegistry.get(matcher.operator)
        except KeyError:
            return False

        target_value = field_name if matcher.target == "field_name" else str_value
        if target_value is None or target_value == "":
            return False

        try:
            return op_func(target_value, matcher.params)
        except Exception as exc:
            logger.warning(
                "operator_execution_failed",
                extra={
                    "operator": matcher.operator,
                    "field_name": field_name,
                    "error": str(exc),
                },
            )
            return False

    def _evaluate_downgrade(self, field_name: str) -> list[SecurityTag]:
        """执行降级规则。"""
        tags: list[SecurityTag] = []
        norm_name = field_name.lower().replace("_", "").replace(" ", "")

        for rule in self.downgrade_rules:
            keywords = [kw.lower().replace("_", "").replace(" ", "") for kw in rule.keywords]
            if any(kw in norm_name for kw in keywords):
                tags.append(
                    SecurityTag(
                        level=rule.level,
                        category=rule.category,
                        source_engine="RULE",
                        rule_id=rule.id,
                        domain=self.domain,
                        standard_id=self.standard_id,
                    )
                )
        return tags

    def _unique_tags(self, tags: list[SecurityTag]) -> list[SecurityTag]:
        """按 (level, category) 去重，保留首次出现。"""
        seen: set[tuple[str, str]] = set()
        result: list[SecurityTag] = []
        for tag in tags:
            key = (tag.level, tag.category)
            if key not in seen:
                seen.add(key)
                result.append(tag)
        return result

    @property
    def rule_count(self) -> int:
        """当前引擎的规则总数。"""
        return len(self.rules)

    @property
    def downgrade_rule_count(self) -> int:
        """当前引擎的降级规则总数。"""
        return len(self.downgrade_rules)
