"""标准文档到 YAML 配置自动生成器 / Standard Document YAML Generator.

输入一个 Markdown 格式的分类分级标准文档（如《四川省健康医疗大数据应用指南.md》），
自动解析文档中的分类树定义、等级划分矩阵与词条举例，并输出对应的：
1. taxonomies/<standard_id>.yaml - 分类分级元数据定义
2. domains/<standard_id>.yaml    - 领域匹配规则包
3. standards/<standard_id>.yaml  - 标准组合定义
"""

from __future__ import annotations

import argparse
import re
from pathlib import Path
from typing import Any, Optional

import yaml

from .models import CategoryDef, DomainTaxonomy, SensitivityLevelDef
from .rule_schema import DowngradeRuleDef, MatcherDef, RuleDef, RuleProfile, StandardDef


class StandardDocParser:
    """分类分级标准文档解析与 YAML 自动生成器。

    支持对符合国标/行标/地方标准格式的 Markdown 文档进行分析，
    抽取数据分类目录、分级定义及字段模式，自动构建三套 YAML 配置。
    """

    def __init__(self, doc_path: str | Path):
        self.doc_path = Path(doc_path)
        if not self.doc_path.exists():
            raise FileNotFoundError(f"标准文档不存在: {self.doc_path}")
        self.content = self.doc_path.read_text(encoding="utf-8")

    def parse(self) -> tuple[DomainTaxonomy, RuleProfile, StandardDef]:
        """解析文档并生成元数据体系、规则 Profile 和 Standard 组合模型。"""
        standard_id = self._extract_standard_id()
        description = self._extract_description()

        levels = self._extract_levels()
        categories = self._extract_categories()

        taxonomy = DomainTaxonomy(
            domain=standard_id,
            standard_id=standard_id,
            version="1.0.0",
            description=description,
            levels=levels,
            categories=categories,
            default_level=self._determine_default_level(levels),
        )

        rules, downgrade_rules = self._generate_rules(standard_id, levels, categories)

        profile = RuleProfile(
            domain=standard_id,
            version="1.0.0",
            description=f"从文档 {self.doc_path.name} 自动抽取的分类分级规则包",
            rules=rules,
            downgrade_rules=downgrade_rules,
        )

        standard_def = StandardDef(
            standard_id=standard_id,
            description=description,
            taxonomy=standard_id,
            domains=[standard_id],
        )

        return taxonomy, profile, standard_def

    def generate_files(self, output_dir: str | Path = "rules") -> dict[str, Path]:
        """解析文档并将自动生成的 3 个 YAML 文件写入 output_dir 对应目录。

        Args:
            output_dir: 规则输出根目录。

        Returns:
            生成的 YAML 文件路径字典 {'taxonomy': path, 'domain': path, 'standard': path}
        """
        output_dir = Path(output_dir)
        taxonomy, profile, standard_def = self.parse()

        tax_dir = output_dir / "taxonomies"
        dom_dir = output_dir / "domains"
        std_dir = output_dir / "standards"

        tax_dir.mkdir(parents=True, exist_ok=True)
        dom_dir.mkdir(parents=True, exist_ok=True)
        std_dir.mkdir(parents=True, exist_ok=True)

        tax_path = tax_dir / f"{taxonomy.standard_id}.yaml"
        dom_path = dom_dir / f"{profile.domain}.yaml"
        std_path = std_dir / f"{standard_def.standard_id}.yaml"

        # 写入 YAML
        tax_path.write_text(
            yaml.safe_dump(taxonomy.model_dump(by_alias=True, exclude_none=True), allow_unicode=True, sort_keys=False),
            encoding="utf-8",
        )
        dom_path.write_text(
            yaml.safe_dump(profile.model_dump(by_alias=True, exclude_none=True), allow_unicode=True, sort_keys=False),
            encoding="utf-8",
        )
        std_path.write_text(
            yaml.safe_dump(standard_def.model_dump(by_alias=True, exclude_none=True), allow_unicode=True, sort_keys=False),
            encoding="utf-8",
        )

        return {
            "taxonomy": tax_path,
            "domain": dom_path,
            "standard": std_path,
        }

    # ------------------------------------------------------------------
    # 内部抽词与解析辅助函数
    # ------------------------------------------------------------------

    def _extract_standard_id(self) -> str:
        """抽取标准标识符，如 DB51/T 2989 -> sc_health_db51。"""
        # 尝试正则提取标准编号
        match = re.search(r"标准编号[：:]\s*([A-Z0-9_/—\-]+)", self.content)
        if match:
            code = match.group(1).upper()
            if "DB51" in code:
                return "sc_health_db51"
            elif "JR/T" in code or "JRT" in code:
                return "jrt0197"
            elif "GB/T 35273" in code or "35273" in code:
                return "gbt35273"
            elif "GB/T 43697" in code or "43697" in code:
                return "gb43697"

        # 根据文件名回退
        name = self.doc_path.stem
        if "四川" in name or "DB51" in name:
            return "sc_health_db51"
        elif "金融" in name or "JR" in name:
            return "jrt0197"
        elif "广东" in name:
            return "gd_health_db44"

        # 默认使用文件名平滑标识
        slug = re.sub(r"[^\w\-]", "_", name).lower()
        return slug or "auto_generated_standard"

    def _extract_description(self) -> str:
        """抽取标准简短描述。"""
        lines = self.content.strip().split("\n")
        title = lines[0].replace("#", "").strip() if lines else self.doc_path.stem
        match = re.search(r"标准编号[：:]\s*([^\n]+)", self.content)
        code_str = match.group(1).strip() if match else ""
        return f"{title} ({code_str})" if code_str else title

    def _extract_levels(self) -> dict[str, SensitivityLevelDef]:
        """抽取敏感度等级定义字典。"""
        levels: dict[str, SensitivityLevelDef] = {}

        # 检查是否为 5 级划分（L1~L5 或 第1级~第5级）
        if "第1级" in self.content or "第 1 级" in self.content or "L1" in self.content:
            levels["L1"] = SensitivityLevelDef(id="L1", name="公开数据/第1级", rank=1, description="低敏感度或经脱敏的数据")
            levels["L2"] = SensitivityLevelDef(id="L2", name="内部数据/第2级", rank=2, description="机构运营生产相关数据")
            levels["L3"] = SensitivityLevelDef(id="L3", name="敏感数据/第3级", rank=3, description="个人标识与身份信息")
            levels["L4"] = SensitivityLevelDef(id="L4", name="高敏感数据/第4级", rank=4, description="敏感病种与诊疗数据")
            levels["L5"] = SensitivityLevelDef(id="L5", name="极敏感数据/第5级", rank=5, description="基因与遗传数据")
        elif "C1" in self.content or "第四级" in self.content:
            levels["C1"] = SensitivityLevelDef(id="C1", name="第一级（不敏感）", rank=1, description="公开金融数据")
            levels["C2"] = SensitivityLevelDef(id="C2", name="第二级（低敏感）", rank=2, description="内部使用金融数据")
            levels["C3"] = SensitivityLevelDef(id="C3", name="第三级（敏感）", rank=3, description="个人金融信息")
            levels["C4"] = SensitivityLevelDef(id="C4", name="第四级（高敏感）", rank=4, description="核心金融账户")
        else:
            # 默认 L1~L5 结构
            levels["L1"] = SensitivityLevelDef(id="L1", name="公开数据", rank=1)
            levels["L2"] = SensitivityLevelDef(id="L2", name="内部数据", rank=2)
            levels["L3"] = SensitivityLevelDef(id="L3", name="敏感数据", rank=3)
            levels["L4"] = SensitivityLevelDef(id="L4", name="高敏感数据", rank=4)
            levels["L5"] = SensitivityLevelDef(id="L5", name="极敏感数据", rank=5)

        return levels

    def _extract_categories(self) -> dict[str, CategoryDef]:
        """抽取分类目录树。"""
        categories: dict[str, CategoryDef] = {}

        # 匹配 5.2.1 数据分类相关条款
        if "个人基本信息" in self.content or "PERSONAL_BASIC" in self.content:
            categories["PERSONAL_BASIC"] = CategoryDef(id="PERSONAL_BASIC", name="个人基本信息数据", description="能够识别特定自然人的数据")
        if "诊疗信息" in self.content or "MEDICAL_TREATMENT" in self.content:
            categories["MEDICAL_TREATMENT"] = CategoryDef(id="MEDICAL_TREATMENT", name="诊疗信息数据", description="患者医疗服务过程产生的数据")
        if "费用信息" in self.content or "FEE_BILLING" in self.content:
            categories["FEE_BILLING"] = CategoryDef(id="FEE_BILLING", name="费用信息数据", description="医疗服务费用相关数据")
        if "公共卫生" in self.content or "PUBLIC_HEALTH" in self.content:
            categories["PUBLIC_HEALTH"] = CategoryDef(id="PUBLIC_HEALTH", name="公共卫生信息数据", description="疾病控制与公共卫生事业数据")
        if "管理信息" in self.content or "MANAGEMENT" in self.content:
            categories["MANAGEMENT"] = CategoryDef(id="MANAGEMENT", name="管理信息数据", description="反映机构运营管理状况的数据")

        # 补充特有子分类
        if "基因" in self.content or "遗传" in self.content:
            categories["GENOMIC"] = CategoryDef(id="GENOMIC", name="基因遗传数据", parent_id="MEDICAL_TREATMENT", description="个人或家族基因/多组学检测数据")
        if "金融账户" in self.content or "银行卡" in self.content:
            categories["FINANCIAL_ACCOUNT"] = CategoryDef(id="FINANCIAL_ACCOUNT", name="金融账户数据", parent_id="PERSONAL_BASIC")

        if not categories:
            # 默认分类目录
            categories["GENERAL_PII"] = CategoryDef(id="GENERAL_PII", name="通用个人信息")
            categories["BUSINESS_DATA"] = CategoryDef(id="BUSINESS_DATA", name="业务数据")

        return categories

    def _determine_default_level(self, levels: dict[str, SensitivityLevelDef]) -> str:
        if "L3" in levels:
            return "L3"
        elif "C3" in levels:
            return "C3"
        return list(levels.keys())[0] if levels else "L1"

    def _generate_rules(
        self, standard_id: str, levels: dict[str, SensitivityLevelDef], categories: dict[str, CategoryDef]
    ) -> tuple[list[RuleDef], list[DowngradeRuleDef]]:
        """从标准词条举例提取特征算子并自动构建规则。"""
        rules: list[RuleDef] = []
        downgrade_rules: list[DowngradeRuleDef] = []

        # 1. 身份证规则 (L3/C3)
        if any("身份证" in line or "身份证件" in line for line in self.content.split("\n")):
            rules.append(
                RuleDef(
                    id=f"RULE_{standard_id.upper()}_IDCARD",
                    name="身份证件号码检测",
                    category="PERSONAL_BASIC" if "PERSONAL_BASIC" in categories else list(categories.keys())[0],
                    level="L3" if "L3" in levels else "C3",
                    matchers=[
                        MatcherDef(target="field_value", operator="id_card_checksum", params={}),
                        MatcherDef(target="field_name", operator="keyword_contains", params={"keywords": ["idcard", "sfz", "identity", "id_card", "身份证"]}),
                    ],
                    match_logic="OR",
                    priority=90,
                )
            )

        # 2. 手机/联系电话规则 (L3)
        if any("电话" in line or "手机" in line for line in self.content.split("\n")):
            rules.append(
                RuleDef(
                    id=f"RULE_{standard_id.upper()}_PHONE",
                    name="手机号码检测",
                    category="PERSONAL_BASIC" if "PERSONAL_BASIC" in categories else list(categories.keys())[0],
                    level="L3" if "L3" in levels else "C3",
                    matchers=[
                        MatcherDef(target="field_value", operator="regex", params={"pattern": r"^1[3-9]\d{9}$"}),
                        MatcherDef(target="field_name", operator="keyword_contains", params={"keywords": ["mobile", "phone", "cell", "电话", "手机"]}),
                    ],
                    match_logic="OR",
                    priority=80,
                )
            )

        # 3. 银行卡/金融账户 (C4 或 L3)
        if any("金融账户" in line or "支付卡号" in line for line in self.content.split("\n")):
            target_lvl = "C4" if "C4" in levels else "L3"
            rules.append(
                RuleDef(
                    id=f"RULE_{standard_id.upper()}_BANKCARD",
                    name="支付金融账户检测",
                    category="FINANCIAL_ACCOUNT" if "FINANCIAL_ACCOUNT" in categories else "PERSONAL_BASIC",
                    level=target_lvl,
                    matchers=[
                        MatcherDef(target="field_value", operator="luhn_checksum", params={"min_length": 13, "max_length": 19}),
                        MatcherDef(target="field_name", operator="keyword_contains", params={"keywords": ["bankcard", "cardno", "card_no", "bank_card", "支付卡号"]}),
                    ],
                    match_logic="OR",
                    priority=85,
                )
            )

        # 4. 敏感病种检测 (L4)
        if any("艾滋病" in line or "性病" in line or "精神病" in line for line in self.content.split("\n")):
            rules.append(
                RuleDef(
                    id=f"RULE_{standard_id.upper()}_DISEASE",
                    name="敏感病种检测",
                    category="MEDICAL_TREATMENT" if "MEDICAL_TREATMENT" in categories else list(categories.keys())[0],
                    level="L4" if "L4" in levels else "L3",
                    matchers=[
                        MatcherDef(
                            target="field_name",
                            operator="keyword_contains",
                            params={"keywords": ["hiv", "aids", "std", "syphilis", "psychiatric", "schizophrenia", "艾滋病", "性病", "精神病"]},
                        )
                    ],
                    priority=95,
                )
            )

        # 5. 基因与遗传数据 (L5)
        if any("基因" in line or "染色体" in line or "地中海贫血" in line for line in self.content.split("\n")):
            rules.append(
                RuleDef(
                    id=f"RULE_{standard_id.upper()}_GENOMIC",
                    name="个人遗传基因数据检测",
                    category="GENOMIC" if "GENOMIC" in categories else "MEDICAL_TREATMENT",
                    level="L5" if "L5" in levels else "L4",
                    matchers=[
                        MatcherDef(
                            target="field_name",
                            operator="keyword_contains",
                            params={"keywords": ["gene", "genomic", "brca", "tp53", "snp", "cnv", "chromosome", "thalassemia", "基因", "染色体", "地中海贫血"]},
                        )
                    ],
                    priority=100,
                )
            )

        # 6. 默认降级规则 (L2 / L1)
        downgrade_rules.append(
            DowngradeRuleDef(
                id=f"DOWN_{standard_id.upper()}_OPS",
                name="运营统计指标降级",
                keywords=["turnover", "inventory", "device_usage", "开机次数", "门诊人次", "运行时间"],
                level="L2" if "L2" in levels else "C2",
                category="MANAGEMENT" if "MANAGEMENT" in categories else list(categories.keys())[0],
            )
        )

        return rules, downgrade_rules


def main():
    """CLI 工具命令行入口。"""
    parser = argparse.ArgumentParser(description="从分类分级标准 Markdown 文档生成 YAML 配置文件")
    parser.add_argument("--doc", required=True, help="标准 Markdown 文档路径，例如 docs/standard/四川省健康医疗大数据应用指南.md")
    parser.add_argument("--output", default="rules", help="YAML 输出规则根目录，默认 rules/")

    args = parser.parse_args()

    doc_parser = StandardDocParser(args.doc)
    generated = doc_parser.generate_files(args.output)

    print("=== 自动生成 YAML 配置文件成功 ===")
    for key, path in generated.items():
        print(f"[{key.upper()}] -> {path}")


if __name__ == "__main__":
    main()
