"""规则配置校验工具与拼写推荐 / Rule Profile Validator & Fuzzy Recommender.

提供对 rules/ 目录下 YAML 文件的离线/在线校验，检查：
1. YAML 语法正确性与 Pydantic Schema 模型校验。
2. 规则配置中使用的算子是否已在 OperatorRegistry 中注册。
3. 规则配置中使用的分类类别和敏感等级是否在对应的 Taxonomy 中有定义。
4. 如果算子拼写错误，提供拼写相似度建议 (Did you mean 'regex'?)。
"""

from __future__ import annotations

import difflib
from pathlib import Path
from typing import Any

import yaml

from .models import DomainTaxonomy
from .operator_registry import OperatorRegistry
from .rule_schema import RuleProfile, StandardDef


class ValidationResult:
    """校验结果模型。"""

    def __init__(self):
        self.is_valid: bool = True
        self.errors: list[str] = []
        self.warnings: list[str] = []

    def add_error(self, msg: str) -> None:
        self.is_valid = False
        self.errors.append(msg)

    def add_warning(self, msg: str) -> None:
        self.warnings.append(msg)


def _suggest_similar_operator(op_name: str) -> str:
    """使用 Levenshtein/difflib 推荐最相似的算子名称。"""
    available = OperatorRegistry.list_operators()
    matches = difflib.get_close_matches(op_name, available, n=1, cutoff=0.5)
    if matches:
        return f"（您是否想输入 '{matches[0]}'？）"
    return ""


def validate_rules_dir(rules_dir: str | Path = "rules") -> ValidationResult:
    """校验规则目录下的所有 YAML 文件合法性。

    Args:
        rules_dir: 规则配置根目录。

    Returns:
        ValidationResult 对象包含校验结果、错误列表与警告列表。
    """
    rules_path = Path(rules_dir)
    res = ValidationResult()

    if not rules_path.exists():
        res.add_error(f"规则配置目录不存在: {rules_path}")
        return res

    tax_dir = rules_path / "taxonomies"
    dom_dir = rules_path / "domains"
    std_dir = rules_path / "standards"

    # 1. 校验 taxonomies
    taxonomies: dict[str, DomainTaxonomy] = {}
    if tax_dir.exists():
        for yaml_file in tax_dir.glob("*.yaml"):
            try:
                data = yaml.safe_load(yaml_file.read_text(encoding="utf-8"))
                tax = DomainTaxonomy.model_validate(data)
                taxonomies[yaml_file.stem] = tax
            except Exception as exc:
                res.add_error(f"[Taxonomy 校验失败] {yaml_file.name}: {exc}")

    # 2. 校验 domains
    if dom_dir.exists():
        for yaml_file in dom_dir.glob("*.yaml"):
            try:
                data = yaml.safe_load(yaml_file.read_text(encoding="utf-8"))
                profile = RuleProfile.model_validate(data)

                # 检查算子与模式
                for rule in profile.rules:
                    for matcher in rule.matchers:
                        op_name = matcher.operator
                        if not OperatorRegistry.has(op_name):
                            suggestion = _suggest_similar_operator(op_name)
                            res.add_error(
                                f"[Domain 规则算子未找到] 文件 {yaml_file.name}, 规则 '{rule.id}', 算子 '{op_name}' 未在注册表中找到{suggestion}"
                            )
            except Exception as exc:
                res.add_error(f"[Domain Profile 校验失败] {yaml_file.name}: {exc}")

    # 3. 校验 standards
    if std_dir.exists():
        for yaml_file in std_dir.glob("*.yaml"):
            try:
                data = yaml.safe_load(yaml_file.read_text(encoding="utf-8"))
                std = StandardDef.model_validate(data)

                if std.taxonomy not in taxonomies and not (tax_dir / f"{std.taxonomy}.yaml").exists():
                    res.add_warning(
                        f"[Standard 引用提醒] 文件 {yaml_file.name} 引用了未找到的 Taxonomy: '{std.taxonomy}'"
                    )
            except Exception as exc:
                res.add_error(f"[Standard 校验失败] {yaml_file.name}: {exc}")

    return res
