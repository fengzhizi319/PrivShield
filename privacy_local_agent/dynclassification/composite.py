"""复合规则引擎 / Composite Rule Engine.

用于识别"单字段不敏感、多字段组合后敏感"的上下文场景。
在单条记录的字段级分类完成后执行，根据字段名组合升级敏感度等级。

典型场景：
- 单独一个 "name" 字段可能只是 L3，但如果同一条记录中同时存在
  "name" + "id_card" + "mobile"，则组合后应升级为 L5。
"""

from __future__ import annotations

import re
from typing import Any

from .models import FieldClassificationResult, SecurityTag
from .rule_schema import CompositeRuleDef


def _normalize(name: str) -> str:
    """规范化字段名用于模式匹配。"""
    return str(name).lower().replace("_", "").replace(" ", "")


class CompositeRuleEngine:
    """复合规则引擎。

    维护一组复合规则，对单条记录及其字段分类结果进行后处理。
    当记录中的字段名组合满足某条复合规则的 min_matches 阈值时，
    生成对应的 SecurityTag 并升级记录的最终敏感度等级。

    Attributes:
        rules: 当前生效的复合规则列表。
        domain: 领域标识。
        standard_id: 标准标识。
    """

    def __init__(
        self,
        rules: list[CompositeRuleDef] | None = None,
        domain: str = "",
        standard_id: str = "",
    ):
        """初始化复合规则引擎。

        Args:
            rules: 复合规则列表；None 时使用空列表。
            domain: 领域标识。
            standard_id: 标准标识。
        """
        self.rules = list(rules) if rules else []
        self.domain = domain
        self.standard_id = standard_id

        # 预编译字段匹配模式正则，避免批量评估时重复编译
        self._compiled_patterns: dict[str, list[re.Pattern]] = {}
        for rule in self.rules:
            compiled_list: list[re.Pattern] = []
            for pattern in rule.field_patterns:
                try:
                    compiled_list.append(re.compile(pattern, re.IGNORECASE))
                except re.error:
                    pass
            self._compiled_patterns[rule.id] = compiled_list

    def evaluate(
        self,
        record: dict[str, Any],
        field_results: dict[str, FieldClassificationResult] | None = None,
    ) -> list[SecurityTag]:
        """评估单条记录是否命中复合规则。

        Args:
            record: 原始记录字典（字段名 → 字段值）。
            field_results: 字段级分类结果（可选，预留扩展）。

        Returns:
            命中的 SecurityTag 列表。
        """
        tags: list[SecurityTag] = []
        norm_fields = {_normalize(name): name for name in record}

        for rule in self.rules:
            matched = 0
            matched_names: list[str] = []
            compiled_patterns = self._compiled_patterns.get(rule.id, [])

            for compiled in compiled_patterns:
                for norm_name, original_name in norm_fields.items():
                    if compiled.search(norm_name):
                        matched += 1
                        matched_names.append(original_name)
                        break

                if matched >= rule.min_matches:
                    break

            if matched >= rule.min_matches:
                tags.append(
                    SecurityTag(
                        level=rule.target_level,
                        category=rule.category,
                        confidence=1.0,
                        source_engine="COMPOSITE",
                        rule_id=rule.id,
                        domain=self.domain,
                        standard_id=self.standard_id,
                    )
                )

        return tags


    def apply_to_record_level(
        self,
        current_level: str,
        composite_tags: list[SecurityTag],
        taxonomy: Any = None,
    ) -> str:
        """将复合规则标签应用到记录级等级（取最高）。

        Args:
            current_level: 当前记录级等级。
            composite_tags: 复合规则命中的标签。
            taxonomy: 分类体系（用于比较等级）。

        Returns:
            升级后的等级 ID。
        """
        if not composite_tags:
            return current_level

        levels = [current_level] + [tag.level for tag in composite_tags]

        if taxonomy is not None:
            return taxonomy.max_level(*levels)

        # 无 taxonomy 时简单取最大值（按字符串排序）
        return max(levels)
