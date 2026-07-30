"""标准 Markdown 文档生成 YAML 规则测试 / Standard Markdown Document Generator Tests.

测试覆盖场景：
- StandardDocParser 解析 Markdown 文档并生成完整的三套 YAML 模型 (Taxonomy, Profile, StandardDef)
- 验证自动生成的 Profile 包含 full schema 字段（包含 default match_logic, force_suppress, exempt_rules 等全量参考字段）
- 验证写入临时目录后的 YAML 文件语法合法且包含预期结构
"""

from __future__ import annotations

import tempfile
from pathlib import Path

import pytest
import yaml

from privacy_local_agent.dynclassification.generator import StandardDocParser


@pytest.fixture()
def dummy_markdown_doc(tmp_path: Path) -> Path:
    """构造一个模拟分类分级标准的 Markdown 文件。"""
    md_content = """# 四川省健康医疗大数据应用指南 (DB51/T 2851-2021)
标准编号：DB51/T 2851-2021

## 一、 数据分类体系
1. 个人基本信息数据 (PERSONAL_BASIC)
2. 诊疗信息数据 (MEDICAL_TREATMENT)
3. 管理信息数据 (MANAGEMENT)

## 二、 敏感度等级定义
- 第1级 (L1): 公开数据
- 第2级 (L2): 内部数据
- 第3级 (L3): 敏感数据
- 第4级 (L4): 高敏感数据
- 第5级 (L5): 极敏感数据 (基因)

## 三、 规则词条举例
- 身份证件号码检测 (身份证, idcard)
- 手机号码检测 (手机, 电话)
- 敏感病种检测 (艾滋病, 性病, 精神病)
- 个人遗传基因数据检测 (基因, 染色体, 地中海贫血)
"""
    doc_file = tmp_path / "test_standard_db51.md"
    doc_file.write_text(md_content, encoding="utf-8")
    return doc_file


def test_standard_doc_parser(dummy_markdown_doc: Path, tmp_path: Path):
    """测试 StandardDocParser 能否成功解析文档并导出包含全量参考字段的 YAML。"""
    parser = StandardDocParser(dummy_markdown_doc)
    taxonomy, profile, standard_def = parser.parse()

    assert taxonomy.domain == "sc_health_db51"
    assert "L1" in taxonomy.levels
    assert len(profile.rules) >= 4
    assert len(profile.downgrade_rules) >= 1

    # 检查导出的 downgrade_rules 是否具有全量字段 (包含 exempt_rules, force_suppress, max_force_suppress_level)
    ops_down = profile.downgrade_rules[0]
    assert hasattr(ops_down, "exempt_rules")
    assert hasattr(ops_down, "force_suppress")
    assert hasattr(ops_down, "max_force_suppress_level")

    # 测试生成 YAML 文件落盘
    output_dir = tmp_path / "rules_output"
    generated_files = parser.generate_files(output_dir)

    assert generated_files["domain"].exists()
    assert generated_files["taxonomy"].exists()
    assert generated_files["standard"].exists()

    # 验证导出的 YAML 包含全量字段且反序列化正常
    domain_yaml_data = yaml.safe_load(generated_files["domain"].read_text(encoding="utf-8"))
    assert "downgrade_rules" in domain_yaml_data
    down_yaml = domain_yaml_data["downgrade_rules"][0]
    assert "override" in down_yaml or "force_suppress" in down_yaml
    assert "exempt_rules" in down_yaml or "exclude_rules" in down_yaml
