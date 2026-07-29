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

# Structured logger for this module (JSON/text output based on config)
from ..observability.logging_config import get_logger
# Side-effect import: importing operators module triggers @OperatorRegistry.register
# decorators, ensuring all built-in operators are available before engine evaluation.
from . import operators  # noqa: F401 - Ensure built-in operators are registered
from .models import DomainTaxonomy, SecurityTag
from .operator_registry import OperatorRegistry
from .rule_schema import DowngradeRuleDef, MatcherDef, RuleDef, RuleProfile

logger = get_logger(__name__)


# Prometheus metrics for monitoring engine behavior in production:
# - OPERATOR_CALLS_TOTAL: counts every operator invocation (hit/miss)
# - OPERATOR_ERRORS_TOTAL: counts operator execution failures
# - RULE_HITS_TOTAL: counts successful rule matches
from ..observability.metrics import (
    DYNCLASSIFICATION_OPERATOR_CALLS_TOTAL,
    DYNCLASSIFICATION_OPERATOR_ERRORS_TOTAL,
    DYNCLASSIFICATION_OVERRIDE_SUPPRESSED_TOTAL,
    DYNCLASSIFICATION_RULE_HITS_TOTAL,
)


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
        # Store taxonomy for level comparison and default level resolution.
        self.taxonomy = taxonomy
        # Domain and standard identifiers are embedded in every produced SecurityTag.
        self.domain = domain
        self.standard_id = standard_id
        # Merge all profiles' rules into a single priority-sorted list.
        self.rules = self._merge_rules(profiles)
        # Merge all profiles' downgrade rules into a flat list.
        self.downgrade_rules = self._merge_downgrade_rules(profiles)
        # Audit trail: suppressed tags from the most recent evaluate() call.
        # Populated by _apply_override_suppression; read by service layer.
        self.last_suppressed_tags: list[SecurityTag] = []

    def _merge_rules(self, profiles: list[RuleProfile]) -> list[RuleDef]:
        """合并多个领域包的规则列表，按 priority 降序排列。

        Only enabled rules are included. Higher priority rules execute first,
        ensuring more specific rules take precedence over general ones.
        """
        all_rules: list[RuleDef] = []
        for profile in profiles:
            # Filter: only include rules with enabled=True.
            all_rules.extend(r for r in profile.rules if r.enabled)
        # Sort by priority descending: highest priority rules are evaluated first.
        return sorted(all_rules, key=lambda r: r.priority, reverse=True)

    def _merge_downgrade_rules(self, profiles: list[RuleProfile]) -> list[DowngradeRuleDef]:
        """合并多个领域包的降级规则列表。

        降级策略（Downgrade Strategy）说明：
            降级规则是一种"反向修正"机制。当普通规则将某字段判定为较高敏感等级后，
            降级规则会根据字段名中的特定关键词，将其敏感度"下调"到更低的等级。

            典型场景：
            - 字段名包含 "turnover"（营业额）、"门诊人次" 等运营统计关键词时，
              虽然可能被通用规则误判为敏感数据，但实际属于机构运营指标，
              应降级为 L2（内部数据）而非 L3（敏感数据）。
            - 字段名包含 "public"、"公开" 等关键词时，应降级为 L1（公开数据）。

            执行时机：降级规则在所有普通规则评估完毕后统一执行，
            产生的低等级标签会与普通规则标签一同参与最终等级裁定（取最高），
            因此降级标签本身不会"覆盖"高等级标签，而是为无其他规则命中的字段
            提供一个合理的低等级归属。

        注意：降级规则不按优先级排序，所有降级规则对每个字段均会被评估。
        """
        # 初始化空列表，用于汇聚所有领域包中的降级规则
        all_rules: list[DowngradeRuleDef] = []
        for profile in profiles:
            # 将当前领域包的降级规则追加到汇总列表中
            all_rules.extend(profile.downgrade_rules)
        return all_rules

    def evaluate(
        self,
        field_name: str,
        value: Any,
        context: dict[str, Any] | None = None,
    ) -> list[SecurityTag]:
        """评估单个字段，返回命中的安全标签列表。
    
        Execution flow:
        1. Convert value to string for uniform operator processing.
        2. Phase 1: Iterate all rules (priority order) and evaluate each.
        3. Phase 2: Evaluate downgrade rules.
        4. Phase 3: Apply override suppression (override=true downgrade rules
           forcibly remove normal tags with rank <= cap).
        5. Phase 4: Merge + deduplicate by (level, category) and return.
    
        Args:
            field_name: 字段名。
            value: 字段值。
            context: 可选的执行上下文（预留扩展）。
    
        Returns:
            命中的 SecurityTag 列表（已去重）。
    
        字段评估流程（含强制覆盖）:
        ┌─────────────────────────────────────────────────────────────────┐
        │  evaluate(field_name, value)                                     │
        │                                                                  │
        │  Phase 1: 普通规则评估 → normal_tags = [L5, L4, L3, ...]        │
        │  Phase 2: 降级规则评估 → downgrade_tags = [L2, L1, ...]         │
        │                                                                  │
        │  Phase 3: 强制覆盖裁定 (override=true 的降级规则)               │
        │    ┌─────────────────────────────────────────────────────────┐  │
        │    │ 对每条 override 降级标签:                                 │  │
        │    │   cap_rank = rank(max_override_level)                    │  │
        │    │   从 normal_tags 中移除 rank <= cap_rank 的标签          │  │
        │    │   (被移除标签记入日志用于审计)                            │  │
        │    └─────────────────────────────────────────────────────────┘  │
        │                                                                  │
        │  Phase 4: 合并 normal_tags + downgrade_tags → 去重 → return     │
        └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌─────────────────────────────────────────────────────────────────┐
        │  _resolve_final_level(tags, engine)                              │
        │    if not tags → return default_level ("L3")                     │
        │    else → return max_level(*levels)  ← 取所有标签中最高          │
        └─────────────────────────────────────────────────────────────────┘
        """
        # Convert value to string once; all operators work on string representation.
        str_value = str(value) if value is not None else ""
    
        # Phase 1: Evaluate all normal rules in priority order.
        normal_tags: list[SecurityTag] = []
        for rule in self.rules:
            tag = self._evaluate_rule(rule, field_name, str_value)
            if tag is not None:
                normal_tags.append(tag)
    
        # Phase 2: Evaluate downgrade rules (produces downgrade tags + override info).
        downgrade_tags = self._evaluate_downgrade(field_name)
    
        # Phase 3: Apply override suppression.
        # For each override-enabled downgrade tag that fired, remove normal tags
        # whose rank <= the override cap rank. This is the core enhancement that
        # allows downgrade rules to forcibly correct over-classification.
        normal_tags = self._apply_override_suppression(normal_tags, downgrade_tags)
    
        # Phase 4: Merge all surviving tags and deduplicate.
        all_tags = normal_tags + downgrade_tags
        return self._unique_tags(all_tags)

    def evaluate_batch(
        self,
        fields: list[tuple[str, Any]],
        context: dict[str, Any] | None = None,
    ) -> dict[str, list[SecurityTag]]:
        """批量评估多个字段。

        Convenience wrapper that calls evaluate() for each field sequentially.
        Future optimization: could add vectorized/batch operator execution.

        Args:
            fields: (字段名, 字段值) 元组列表。
            context: 可选的执行上下文。

        Returns:
            字段名 → 标签列表的映射。
        """
        results: dict[str, list[SecurityTag]] = {}
        # Iterate each (field_name, value) pair and evaluate independently.
        for field_name, value in fields:
            results[field_name] = self.evaluate(field_name, value, context)
        return results

    def _evaluate_rule(self, rule: RuleDef, field_name: str, str_value: str) -> SecurityTag | None:
        """评估单条规则，命中则返回 SecurityTag，否则返回 None。

        Algorithm:
        1. Execute each matcher in the rule and collect boolean results.
        2. Apply match_logic (AND/OR) to determine if the rule fires.
        3. If fired, resolve dynamic level/category (ICD-10 support) and build tag.
        """
        # A rule with no matchers can never fire.
        if not rule.matchers:
            return None

        # Collect boolean results from each matcher execution.
        results: list[bool] = []
        # Store params from ICD-10 operator if it hits (for dynamic level resolution).
        hit_params: dict[str, Any] = {}

        for matcher in rule.matchers:
            # Execute the matcher and record whether it hit.
            hit = self._execute_matcher(matcher, field_name, str_value)
            results.append(hit)
            # Special handling: if icd10_range operator hits, capture its params
            # because they contain _hit_level/_hit_category written back by the operator.
            if hit and matcher.operator == "icd10_range":
                hit_params = matcher.params

        # Apply match_logic to combine multiple matcher results.
        if rule.match_logic.upper() == "OR":
            # OR logic: rule fires if ANY matcher hit.
            matched = any(results)
        else:  # AND (default)
            # AND logic: rule fires only if ALL matchers hit.
            matched = all(results)

        # If the rule did not fire, return None (no tag produced).
        if not matched:
            return None

        # Increment Prometheus counter for rule hits (monitoring/alerting).
        DYNCLASSIFICATION_RULE_HITS_TOTAL.labels(
            rule_id=rule.id,
            domain=self.domain or "default",
            standard=self.standard_id or "default",
        ).inc()

        # Resolve final level and category:
        # Default to rule's static level/category, but allow ICD-10 operator
        # to dynamically override them via _hit_level/_hit_category in params.
        level = rule.level
        category = rule.category
        if "_hit_level" in hit_params:
            # ICD-10 operator determined a dynamic level (e.g. L4 for sensitive disease).
            level = hit_params["_hit_level"]
        if "_hit_category" in hit_params and hit_params["_hit_category"]:
            # ICD-10 operator determined a specific category (e.g. SEXUAL_DISEASE).
            category = hit_params["_hit_category"]

        # Determine match_target: if ANY matcher targets field_value, the tag is
        # considered a value-level hit (higher confidence, exempt from override suppression).
        match_target = "field_value" if any(
            m.target == "field_value" for m in rule.matchers
        ) else "field_name"

        # Construct and return the security tag with full audit metadata.
        return SecurityTag(
            level=level,
            category=category,
            source_engine="RULE",
            rule_id=rule.id,
            domain=self.domain,
            standard_id=self.standard_id,
            match_target=match_target,
        )

    def _execute_matcher(self, matcher: MatcherDef, field_name: str, str_value: str) -> bool:
        """执行单个匹配器。

        Steps:
        1. Look up the operator function from the registry.
        2. Determine target value (field_name or str_value based on matcher.target).
        3. Execute the operator and record metrics.
        4. Handle exceptions gracefully (fail-safe: return False on error).
        """
        # Step 1: Resolve operator function from the global registry.
        try:
            op_func = OperatorRegistry.get(matcher.operator)
        except KeyError:
            # Operator not registered: record miss metric and return False.
            DYNCLASSIFICATION_OPERATOR_CALLS_TOTAL.labels(
                operator=matcher.operator, result="miss"
            ).inc()
            return False

        # Step 2: Select target value based on matcher configuration.
        # "field_name" -> match against the column/field name.
        # "field_value" (default) -> match against the actual data value.
        target_value = field_name if matcher.target == "field_name" else str_value
        # Guard: empty target cannot match anything.
        if target_value is None or target_value == "":
            DYNCLASSIFICATION_OPERATOR_CALLS_TOTAL.labels(
                operator=matcher.operator, result="miss"
            ).inc()
            return False

        # Step 3: Execute the operator function with target and params.
        try:
            res = bool(op_func(target_value, matcher.params))
            # Record hit/miss metric for observability dashboards.
            DYNCLASSIFICATION_OPERATOR_CALLS_TOTAL.labels(
                operator=matcher.operator, result="hit" if res else "miss"
            ).inc()
            return res
        except Exception as exc:
            # Step 4: Operator raised an exception (e.g. bad params, runtime error).
            # Record error metric and log warning for debugging.
            DYNCLASSIFICATION_OPERATOR_ERRORS_TOTAL.labels(
                operator=matcher.operator, rule_id=""
            ).inc()
            logger.warning(
                "operator_execution_failed",
                extra={
                    "operator": matcher.operator,
                    "field_name": field_name,
                    "error": str(exc),
                },
            )
            # Fail-safe: treat operator errors as non-match.
            return False

    def _evaluate_downgrade(self, field_name: str) -> list[SecurityTag]:
        """执行降级规则。

        Downgrade rules check if the normalized field name contains any of the
        rule's keywords. If so, a lower-sensitivity tag is produced.
        Typical use: operational/public fields downgraded to L1/L2.

        输入: field_name = "Turnover_Rate"
          │
          ▼ 归一化
        norm_name = "turnoverrate"
          │
          ▼ 遍历每条降级规则
          ├─ RULE_DOWN_PUBLIC: keywords=["publicreport","annualsummary","科普"] → 不包含 → 跳过
          └─ RULE_DOWN_OPS:    keywords=["turnoverrate","deviceusage","inventory"] → 包含! → 生成 L2 标签
          │
          ▼
        输出: [SecurityTag(level="L2", category="OPERATIONAL_STAT", is_override=True, ...)]
        """
        tags: list[SecurityTag] = []
        # Normalize field name: lowercase + remove underscores/spaces for matching.
        norm_name = field_name.lower().replace("_", "").replace(" ", "")

        for rule in self.downgrade_rules:
            # Normalize each keyword the same way for consistent comparison.
            keywords = [kw.lower().replace("_", "").replace(" ", "") for kw in rule.keywords]
            # Check if any keyword is a substring of the normalized field name.
            if any(kw in norm_name for kw in keywords):
                # Keyword matched: produce a downgrade tag with the rule's target level.
                # Mark is_override=True if this rule has override capability enabled.
                # Mark is_downgrade=True for all downgrade tags (used by funnel conflict detection).
                tags.append(
                    SecurityTag(
                        level=rule.level,
                        category=rule.category,
                        source_engine="RULE",
                        rule_id=rule.id,
                        domain=self.domain,
                        standard_id=self.standard_id,
                        is_override=rule.override,
                        is_downgrade=True,
                    )
                )
        return tags

    def _apply_override_suppression(
        self,
        normal_tags: list[SecurityTag],
        downgrade_tags: list[SecurityTag],
    ) -> list[SecurityTag]:
        """对普通规则标签执行强制覆盖压制。

        执行流程:
        ┌─────────────────────────────────────────────────────────────────┐
        │  _apply_override_suppression(normal_tags, downgrade_tags)        │
        │                                                                  │
        │  Step 1: 从 downgrade_tags 中筛选 is_override=True 的标签       │
        │  Step 2: 对每条 override 标签，计算覆盖等级上限 (cap_rank)      │
        │          多条 override 取 min_cap_rank（安全保守原则）          │
        │  Step 3: 从 normal_tags 中移除 rank <= cap_rank 的标签          │
        │          ★ 豁免: match_target="field_value" 的标签不被压制    │
        │          ★ 白名单: suppress_rules 非空时仅压制列出的规则      │
        │  Step 4: 被移除标签记入日志 + Prometheus 指标                 │
        │  Step 5: 返回存活的 normal_tags                                  │
        └─────────────────────────────────────────────────────────────────┘

        安全保障:
        - 仅 override=True 的降级标签才有压制能力
        - cap_rank 由 max_override_level 决定，L4/L5 级规则永远不会被压制
        - 值级命中（field_value）永远不被压制（如手机号 regex、身份证 checksum）
        - suppress_rules 白名单: 非空时仅列出的规则 ID 可被压制（精细控制）
        - 无 override 标签时，此方法为 no-op（完全向后兼容）

        Args:
            normal_tags: 普通规则产出的标签列表。
            downgrade_tags: 降级规则产出的标签列表。

        Returns:
            压制后存活的普通规则标签列表。
        """
        # Step 1: Filter to only override-enabled downgrade tags.
        override_tags = [t for t in downgrade_tags if t.is_override]
        # Early exit: no override tags means no suppression needed (backward-compatible).
        if not override_tags:
            return normal_tags

        # Step 2: Determine the suppression cap rank across all override tags.
        # 安全保守原则: 多条 override 规则同时命中时，取最小 cap_rank（压制范围最小化）。
        # 理由: override 是对“安全优先”原则的例外豁免，例外应从严解释。
        cap_ranks: list[int] = []
        for tag in override_tags:
            cap_level = self._get_override_cap_level(tag.rule_id, tag.level)
            cap_rank = self.taxonomy.get_level_rank(cap_level)
            if cap_rank > 0:  # 忽略无效等级（rank=0 表示配置错误）
                cap_ranks.append(cap_rank)
        # 无有效 cap 时不执行压制（配置容错）
        if not cap_ranks:
            return normal_tags
        # 取最小值: 保守方获胜，压制范围最小化
        min_cap_rank = min(cap_ranks)

        # Step 2.5: 合并所有 override 规则的 suppress_rules 白名单。
        # 空列表 = 无白名单限制（压制所有符合条件的标签，向后兼容）。
        # 非空 = 仅白名单中的 rule_id 可被压制（精细控制）。
        # 混合场景: 若任一 override 规则无白名单（空），则视为全局压制（不限制）。
        suppress_whitelist: set[str] = set()
        has_whitelist = False
        for tag in override_tags:
            rule_def = self._find_downgrade_rule(tag.rule_id)
            if rule_def and rule_def.suppress_rules:
                has_whitelist = True
                suppress_whitelist.update(rule_def.suppress_rules)
            elif rule_def and not rule_def.suppress_rules:
                # 该 override 规则无白名单限制 → 取消全局白名单约束
                has_whitelist = False
                suppress_whitelist.clear()
                break

        # Step 3: Filter out normal tags whose rank <= cap_rank.
        # ★ 值级豁免: match_target="field_value" 的标签代表高置信度证据
        #   （如身份证 checksum、手机号 regex），不应被字段名降级规则压制。
        # ★ 白名单豁免: suppress_rules 非空时，未列入白名单的规则不被压制。
        surviving_tags: list[SecurityTag] = []
        suppressed_tags: list[SecurityTag] = []
        for tag in normal_tags:
            # 值级命中豁免: 基于字段值精确校验的标签永远不被压制
            if tag.match_target == "field_value":
                surviving_tags.append(tag)
                continue
            tag_rank = self.taxonomy.get_level_rank(tag.level)
            if tag_rank <= min_cap_rank:
                # 白名单检查: 若配置了 suppress_rules，仅压制白名单内的规则
                if has_whitelist and tag.rule_id not in suppress_whitelist:
                    # 该规则不在白名单中，豁免压制
                    surviving_tags.append(tag)
                    continue
                # This normal tag is suppressed by the override downgrade rule.
                suppressed_tags.append(tag)
            else:
                # This normal tag's rank exceeds the cap: it survives.
                surviving_tags.append(tag)

        # Step 4: Log suppressed tags + Prometheus metric (only if any were suppressed).
        if suppressed_tags:
            logger.info(
                "downgrade_override_suppression",
                extra={
                    "suppressed_count": len(suppressed_tags),
                    "suppressed_tags": [str(t) for t in suppressed_tags],
                    "suppressed_rule_ids": [t.rule_id for t in suppressed_tags],
                    "override_rules": [t.rule_id for t in override_tags],
                    "cap_rank": min_cap_rank,
                    "has_whitelist": has_whitelist,
                },
            )
            # Prometheus: 监控压制事件，关键词配置过宽导致大面积压制时可告警
            for tag in suppressed_tags:
                DYNCLASSIFICATION_OVERRIDE_SUPPRESSED_TOTAL.labels(
                    domain=self.domain or "default",
                    suppressed_rule_id=tag.rule_id,
                ).inc()

        # Step 5: Return only the surviving normal tags.
        # Store suppressed tags on instance for service-layer audit trail.
        self.last_suppressed_tags = suppressed_tags
        return surviving_tags

    def _get_override_cap_level(self, rule_id: str, fallback_level: str) -> str:
        """获取降级规则的覆盖等级上限。

        优先使用 max_override_level 字段，若为空则回退到规则自身的 level 字段。

        Args:
            rule_id: 降级规则 ID。
            fallback_level: 回退等级（规则的 level 字段）。

        Returns:
            覆盖等级上限的等级 ID。
        """
        # Search for the matching DowngradeRuleDef by rule_id.
        rule = self._find_downgrade_rule(rule_id)
        if rule:
            return rule.max_override_level if rule.max_override_level else rule.level
        # Fallback if rule not found (should not happen in normal flow).
        return fallback_level

    def _find_downgrade_rule(self, rule_id: str) -> DowngradeRuleDef | None:
        """根据 rule_id 查找降级规则定义。

        Args:
            rule_id: 降级规则唯一标识。

        Returns:
            匹配的 DowngradeRuleDef，未找到时返回 None。
        """
        for rule in self.downgrade_rules:
            if rule.id == rule_id:
                return rule
        return None

    def _unique_tags(self, tags: list[SecurityTag]) -> list[SecurityTag]:
        """按 (level, category) 去重，保留首次出现。

        Deduplication ensures that if multiple rules produce the same
        (level, category) combination, only the first one is kept.
        This prevents redundant tags in the output.
        """
        # Set of (level, category) tuples already seen.
        seen: set[tuple[str, str]] = set()
        result: list[SecurityTag] = []
        for tag in tags:
            # Build dedup key from level and category.
            key = (tag.level, tag.category)
            if key not in seen:
                # First occurrence: keep this tag.
                seen.add(key)
                result.append(tag)
        return result

    @property
    def rule_count(self) -> int:
        """当前引擎的规则总数（仅包含已启用的普通规则）。"""
        return len(self.rules)

    @property
    def downgrade_rule_count(self) -> int:
        """当前引擎的降级规则总数。"""
        return len(self.downgrade_rules)
