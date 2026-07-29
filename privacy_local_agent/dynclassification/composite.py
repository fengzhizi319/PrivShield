"""复合规则引擎 / Composite Rule Engine.

用于识别“单字段不敏感、多字段组合后敏感”的上下文场景。
在单条记录的字段级分类完成后执行，根据字段名组合升级敏感度等级。

典型场景：
- 单独一个 "name" 字段可能只是 L3，但如果同一条记录中同时存在
  "name" + "id_card" + "mobile"，则组合后应升级为 L5。
"""

from __future__ import annotations

# re module for compiling and matching field name patterns
import re
from typing import Any

from .models import FieldClassificationResult, SecurityTag
from .rule_schema import CompositeRuleDef
from ..observability.logging_config import get_logger

logger = get_logger(__name__)


def _normalize(name: str) -> str:
    """规范化字段名用于模式匹配。

    Normalization steps:
    1. Convert to lowercase (case-insensitive matching).
    2. Remove spaces.
    """
    return str(name).lower().replace(" ", "")


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
        # Store a defensive copy of the rules list to prevent external mutation.
        self.rules = list(rules) if rules else []
        self.domain = domain
        self.standard_id = standard_id

        # Pre-compile regex patterns for each rule to avoid repeated compilation
        # during batch evaluation (performance optimization for hot path).
        self._compiled_patterns: dict[str, list[re.Pattern]] = {}
        for rule in self.rules:
            compiled_list: list[re.Pattern] = []
            for pattern in rule.field_patterns:
                try:
                    # Compile with IGNORECASE for case-insensitive field name matching.
                    compiled_list.append(re.compile(pattern, re.IGNORECASE))
                except re.error as e:
                    logger.error(f"Invalid regex pattern in composite rule '{rule.id}': '{pattern}'. Error: {e}")
                    pass
            # Map rule ID -> its compiled patterns for O(1) lookup during evaluation.
            self._compiled_patterns[rule.id] = compiled_list

    def evaluate(
        self,
        record: dict[str, Any],
        field_results: dict[str, FieldClassificationResult] | None = None,
    ) -> list[SecurityTag]:
        """评估单条记录是否命中复合规则。

        Algorithm:
        1. Normalize all field names in the record for pattern matching.
        2. For each composite rule, count how many of its patterns match
           at least one field name in the record.
        3. If matched count >= rule.min_matches, the rule fires and produces a tag.

        Args:
            record: 原始记录字典（字段名 → 字段值）。
            field_results: 字段级分类结果（可选，预留扩展）。

        Returns:
            命中的 SecurityTag 列表。
        """
        tags: list[SecurityTag] = []
        # Build a normalized-name -> original-name mapping for all fields in the record.
        # This allows regex matching on normalized names while preserving original names for output.
        norm_fields = {_normalize(name): name for name in record}

        for rule in self.rules:
            # Counter for how many distinct patterns have matched at least one field.
            matched = 0
            # Collect original field names that matched (for potential audit use).
            matched_names: list[str] = []
            # Retrieve pre-compiled patterns for this rule (avoids re-compilation).
            compiled_patterns = self._compiled_patterns.get(rule.id, [])

            for compiled in compiled_patterns:
                # For each pattern, check if ANY field in the record matches it.
                for norm_name, original_name in norm_fields.items():
                    # Use word boundaries to avoid partial matches like 'gene' in 'general_note'
                    if re.search(r"\b" + compiled.pattern + r"\b", norm_name, re.IGNORECASE):
                        # Pattern matched a field: increment counter and record the field name.
                        matched += 1
                        matched_names.append(original_name)
                        # Break inner loop: one match per pattern is sufficient.
                        break

                # Early exit optimization: if threshold already met, skip remaining patterns.
                if matched >= rule.min_matches:
                    break

            # If the rule's threshold is satisfied, generate a SecurityTag.
            if matched >= rule.min_matches:
                tags.append(
                    SecurityTag(
                        level=rule.target_level,       # Upgraded sensitivity level
                        category=rule.category,        # Category from composite rule
                        confidence=1.0,                # Deterministic rule = full confidence
                        source_engine="COMPOSITE",     # Tag produced by composite engine
                        rule_id=rule.id,               # Which composite rule fired
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

        Logic: The final record level is the maximum of the current level
        and all composite tag levels. This ensures composite rules can only
        UPGRADE sensitivity, never downgrade it.

        Args:
            current_level: 当前记录级等级。
            composite_tags: 复合规则命中的标签。
            taxonomy: 分类体系（用于比较等级）。

        Returns:
            升级后的等级 ID。
        """
        # If no composite tags fired, the current level remains unchanged.
        if not composite_tags:
            return current_level

        # Collect all candidate levels: current + all composite tag levels.
        levels = [current_level] + [tag.level for tag in composite_tags]

        # If a taxonomy is available, use its rank-based comparison (accurate).
        if taxonomy is not None:
            return taxonomy.max_level(*levels)

        # Fallback without taxonomy: simple string max (lexicographic ordering).
        # This works for standard naming like L1 < L2 < ... < L5.
        return max(levels)