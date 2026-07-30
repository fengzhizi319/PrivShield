"""规则配置校验工具与拼写推荐 / Rule Profile Validator & Fuzzy Recommender.

提供对 rules/ 目录下 YAML 文件的离线/在线校验，检查：
1. YAML 语法正确性与 Pydantic Schema 模型校验。
2. 规则配置中使用的算子是否已在 OperatorRegistry 中注册。
3. 规则配置中使用的分类类别和敏感等级是否在对应的 Taxonomy 中有定义。
4. 如果算子拼写错误，提供拼写相似度建议 (Did you mean 'regex'?)。
"""

from __future__ import annotations

# difflib provides fuzzy string matching for "did you mean?" suggestions
import difflib
from pathlib import Path
from typing import Any

# PyYAML for parsing rule configuration files
import yaml

from .models import DomainTaxonomy
from .operator_registry import OperatorRegistry
from .rule_schema import RuleProfile, StandardDef


class ValidationResult:
    """校验结果模型。

    Accumulates errors and warnings during validation.
    is_valid is set to False as soon as any error is added.
    """

    def __init__(self):
        # Overall validity flag: True until first error is encountered.
        self.is_valid: bool = True
        # Critical issues that prevent correct engine operation.
        self.errors: list[str] = []
        # Non-critical issues that may indicate misconfiguration.
        self.warnings: list[str] = []

    def add_error(self, msg: str) -> None:
        """Record a validation error and mark result as invalid."""
        self.is_valid = False
        self.errors.append(msg)

    def add_warning(self, msg: str) -> None:
        """Record a non-critical warning (does not affect is_valid)."""
        self.warnings.append(msg)


def _suggest_similar_operator(op_name: str) -> str:
    """使用 Levenshtein/difflib 推荐最相似的算子名称。

    Uses difflib.get_close_matches with a 0.5 similarity cutoff to find
    the closest registered operator name. Returns a formatted suggestion
    string or empty string if no close match exists.
    """
    # Get all registered operator names as the candidate pool.
    available = OperatorRegistry.list_operators()
    # Find the single closest match with at least 50% similarity.
    matches = difflib.get_close_matches(op_name, available, n=1, cutoff=0.5)
    if matches:
        return f"（您是否想输入 '{matches[0]}'？）"
    return ""


def validate_rules_dir(rules_dir: str | Path = "rules") -> ValidationResult:
    """校验规则目录下的所有 YAML 文件合法性。

    Validation pipeline:
    1. Validate all taxonomy YAML files against DomainTaxonomy schema.
    2. Validate all domain profile YAML files against RuleProfile schema,
       and check that every operator referenced in matchers is registered.
    3. Validate all standard YAML files against StandardDef schema,
       and check that referenced taxonomy files exist.

    Args:
        rules_dir: 规则配置根目录。

    Returns:
        ValidationResult 对象包含校验结果、错误列表与警告列表。
    """
    rules_path = Path(rules_dir)
    res = ValidationResult()

    # Early exit if the rules directory does not exist at all.
    if not rules_path.exists():
        res.add_error(f"规则配置目录不存在: {rules_path}")
        return res

    # Resolve the three expected subdirectories.
    tax_dir = rules_path / "taxonomies"
    dom_dir = rules_path / "domains"
    std_dir = rules_path / "standards"

    # --- Phase 1: Validate taxonomy definitions ---
    taxonomies: dict[str, DomainTaxonomy] = {}
    if tax_dir.exists():
        for yaml_file in tax_dir.glob("*.yaml"):
            try:
                # Parse YAML content and validate against DomainTaxonomy Pydantic schema.
                data = yaml.safe_load(yaml_file.read_text(encoding="utf-8"))
                tax = DomainTaxonomy.model_validate(data)
                # Cache valid taxonomies for cross-referencing in Phase 3.
                taxonomies[yaml_file.stem] = tax
            except Exception as exc:
                # Any parse/validation failure is a critical error.
                res.add_error(f"[Taxonomy 校验失败] {yaml_file.name}: {exc}")

    # --- Phase 2: Validate domain profiles and their operator references ---
    if dom_dir.exists():
        for yaml_file in dom_dir.glob("*.yaml"):
            try:
                # Parse and validate against RuleProfile schema.
                data = yaml.safe_load(yaml_file.read_text(encoding="utf-8"))
                profile = RuleProfile.model_validate(data)

                # Check each rule's matchers reference valid registered operators.
                for rule in profile.rules:
                    for matcher in rule.matchers:
                        op_name = matcher.operator
                        # Verify the operator exists in the global registry.
                        if not OperatorRegistry.has(op_name):
                            # Provide a fuzzy suggestion for common typos.
                            suggestion = _suggest_similar_operator(op_name)
                            res.add_error(
                                f"[Domain 规则算子未找到] 文件 {yaml_file.name}, 规则 '{rule.id}', 算子 '{op_name}' 未在注册表中找到{suggestion}"
                            )

                # --- 新增校验: 降级规则字段合法性 ---
                _validate_downgrade_rules(
                    profile, yaml_file.name, taxonomies, res
                )

                # --- 新增校验: 规则 ID 唯一性 ---
                _validate_rule_id_uniqueness(profile, yaml_file.name, res)

            except Exception as exc:
                res.add_error(f"[Domain Profile 校验失败] {yaml_file.name}: {exc}")

    # --- Phase 3: Validate standard definitions and taxonomy references ---
    if std_dir.exists():
        for yaml_file in std_dir.glob("*.yaml"):
            try:
                # Parse and validate against StandardDef schema.
                data = yaml.safe_load(yaml_file.read_text(encoding="utf-8"))
                std = StandardDef.model_validate(data)

                # Check that the referenced taxonomy file exists (either already loaded
                # in Phase 1 or physically present on disk).
                if std.taxonomy not in taxonomies and not (tax_dir / f"{std.taxonomy}.yaml").exists():
                    res.add_warning(
                        f"[Standard 引用提醒] 文件 {yaml_file.name} 引用了未找到的 Taxonomy: '{std.taxonomy}'"
                    )
            except Exception as exc:
                res.add_error(f"[Standard 校验失败] {yaml_file.name}: {exc}")

    return res


# ===========================================================================
# 内部校验工具函数 / Internal Validation Helpers
# ===========================================================================


def _validate_downgrade_rules(
    profile: RuleProfile,
    file_name: str,
    taxonomies: dict[str, DomainTaxonomy],
    res: ValidationResult,
) -> None:
    """校验降级规则的新字段合法性。

    检查项:
    1. max_force_suppress_level 在 taxonomy 中存在（拼错如 "L33" 会导致 force_suppress 静默失效）
    2. force_suppress=false 却配置了 max_force_suppress_level 属于死配置（告警）
    3. 降级规则的 level 在 taxonomy 中存在
    4. 普通规则的 level 在 taxonomy 中存在（rank=0 会被任何 force_suppress 规则压制）
    """
    # 尝试获取关联的 taxonomy（可能不存在，此时跳过等级存在性检查）
    taxonomy: DomainTaxonomy | None = None
    if taxonomies:
        # 使用第一个可用的 taxonomy 作为参考（多数项目只有一个 default）
        taxonomy = next(iter(taxonomies.values()))

    # 校验降级规则
    for rule in profile.downgrade_rules:
        # 检查 1: max_force_suppress_level 存在性
        if rule.max_force_suppress_level and taxonomy:
            if rule.max_force_suppress_level not in taxonomy.levels:
                res.add_error(
                    f"[降级规则等级未找到] 文件 {file_name}, 规则 '{rule.id}': "
                    f"max_force_suppress_level='{rule.max_force_suppress_level}' 在 taxonomy 中不存在"
                    f"（可用: {list(taxonomy.levels.keys())}）"
                )

        # 检查 2: 死配置告警
        if not rule.force_suppress and rule.max_force_suppress_level:
            res.add_warning(
                f"[死配置] 文件 {file_name}, 规则 '{rule.id}': "
                f"force_suppress=false 但配置了 max_force_suppress_level='{rule.max_force_suppress_level}'，该配置不会生效"
            )

        # 检查 3: 降级规则 level 存在性
        if taxonomy and rule.level not in taxonomy.levels:
            res.add_error(
                f"[降级规则等级未找到] 文件 {file_name}, 规则 '{rule.id}': "
                f"level='{rule.level}' 在 taxonomy 中不存在"
            )

        # 检查 5: suppress_rules 白名单引用的规则 ID 存在性
        if rule.suppress_rules:
            if not rule.force_suppress:
                res.add_warning(
                    f"[死配置] 文件 {file_name}, 规则 '{rule.id}': "
                    f"force_suppress=false 但配置了 suppress_rules，该配置不会生效"
                )
            normal_rule_ids = {r.id for r in profile.rules}
            for ref_id in rule.suppress_rules:
                if ref_id not in normal_rule_ids:
                    res.add_warning(
                        f"[压制白名单引用未找到] 文件 {file_name}, 规则 '{rule.id}': "
                        f"suppress_rules 中引用的 '{ref_id}' 在普通规则中不存在"
                    )

    # 检查 4: 普通规则 level 存在性
    if taxonomy:
        for rule in profile.rules:
            if rule.level not in taxonomy.levels:
                res.add_error(
                    f"[规则等级未找到] 文件 {file_name}, 规则 '{rule.id}': "
                    f"level='{rule.level}' 在 taxonomy 中不存在"
                    f"（rank=0 会被任何 force_suppress 规则压制）"
                )


def _validate_rule_id_uniqueness(
    profile: RuleProfile,
    file_name: str,
    res: ValidationResult,
) -> None:
    """校验规则 ID 唯一性。

    多领域包合并后若 id 重复，_get_override_cap_level 会取第一个匹配项，
    可能导致非预期行为。
    """
    seen_ids: dict[str, str] = {}  # rule_id -> rule type
    all_rules = (
        [(r.id, "普通规则") for r in profile.rules]
        + [(r.id, "降级规则") for r in profile.downgrade_rules]
        + [(r.id, "复合规则") for r in profile.composite_rules]
    )
    for rule_id, rule_type in all_rules:
        if rule_id in seen_ids:
            res.add_warning(
                f"[规则 ID 重复] 文件 {file_name}: "
                f"'{rule_id}' 同时用于{seen_ids[rule_id]}和{rule_type}"
            )
        else:
            seen_ids[rule_id] = rule_type
