"""医疗数据分类分级与脱敏 Pipeline 单元测试模块。
Unit Tests for Medical Privacy Pipeline.
"""

import csv
from pathlib import Path
import pytest

from privacy_local_agent.medical_pipeline.pipeline import (
    MedicalPrivacyPipeline,
    process_medical_dataset,
)
from privacy_local_agent.medical_pipeline.rules import L4_PATTERNS, L5_PATTERNS, redact_medical_text
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "scripts" / "data"))
from generate_medical_data import gen_id_card, generate_dataset


def test_generate_valid_id_card_checksum() -> None:
    """验证生成的身份证号码均符合 GB 11643-1999 校验码算法。"""
    id_card = gen_id_card()
    assert len(id_card) == 18
    
    # 校验码重新计算
    weights = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
    checksum_map = ['1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2']
    total = sum(int(id_card[i]) * weights[i] for i in range(17))
    expected_check = checksum_map[total % 11]
    assert id_card[-1].upper() == expected_check


def test_generated_dataset_fields_count() -> None:
    """验证生成的医疗数据集记录数量 (20) 与字段数量 (27 列)。"""
    records = generate_dataset(20)
    assert len(records) == 20
    assert len(records[0].keys()) == 27
    
    expected_fields = [
        "gender", "age", "diagnosis_name", "chief_complaint", "present_illness",
        "past_history", "personal_history", "is_smoking", "smoking_duration",
        "family_history", "allergic_history", "department", "height", "weight",
        "disability_category", "disability_level", "assess_type_name",
        "assess_result_name", "assess_score", "assess_time", "progress_note",
        "progress_note_time", "name", "id_card_no", "registered_address",
        "disability_cert_no", "medical_insurance_no",
    ]
    for field in expected_fields:
        assert field in records[0]


def test_medical_privacy_pipeline_dual_output() -> None:
    """验证 Pipeline 处理后生成符合结构的双重输出（分级报告 + 脱敏数据）。"""
    records = generate_dataset(5)
    pipeline = MedicalPrivacyPipeline()
    result = pipeline.process_records(records)
    
    # 验证分类分级报告
    assert len(result.classification_report) == 5
    first_report = result.classification_report[0]
    assert "record_index" in first_report
    assert "max_level" in first_report
    assert "field_details" in first_report
    
    # 验证脱敏数据
    assert len(result.sanitized_data) == 5
    first_sanitized = result.sanitized_data[0]
    assert len(first_sanitized.keys()) == 27
    
    # 验证 Summary 字段
    assert result.summary["total_records"] == 5
    assert result.summary["guarantee_no_l4_l5_raw_data"] is True


def test_medical_privacy_pipeline_no_raw_l4_l5_leak() -> None:
    """验证经 Pipeline 脱敏后的 sanitized_data 中绝对没有任何 L4/L5 级原始高危词汇。"""
    records = generate_dataset(20)
    res = process_medical_dataset(records)
    
    forbidden_terms = [
        "获得性免疫缺陷综合征", "HIV感染", "艾滋病", "重度精神分裂症",
        "幻听（命令性言语）", "被害妄想", "自伤倾向", "冲动砸物",
        "浸润性腺癌", "恶性肿瘤", "亨廷顿舞蹈病", "乙型肝炎", "丙型肝炎"
    ]
    
    for row in res.sanitized_data:
        # PII 脱敏校验
        assert "*" in row["id_card_no"]
        assert "*" in row["name"]
        
        # L4/L5 绝无泄漏校验
        for key, val in row.items():
            for term in forbidden_terms:
                assert term not in val, f"字段 {key} 泄漏了 L4/L5 敏感词汇 '{term}': {val}"


def test_data1_csv_file_pipeline_execution() -> None:
    """从本地读取真实生成的 data1.csv 并执行全流程测试。"""
    csv_path = Path(__file__).resolve().parent.parent / "privacy_local_agent" / "medical_pipeline" / "samples" / "data1.csv"
    assert csv_path.exists()
    
    records = []
    with open(csv_path, encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        for row in reader:
            records.append(row)
            
    assert len(records) >= 20
    res = process_medical_dataset(records)
    assert res.summary["total_records"] == len(records)
    assert res.summary["l5_records_count"] > 0
    assert res.summary["l4_records_count"] > 0


# === 增强测试：身份证批量校验、L4/L5 覆盖、图片病例、脱敏格式、替换标签泄露 ===


def test_batch_id_card_checksum_validation() -> None:
    """批量生成 50 个身份证号，全部必须通过 GB 11643-1999 MOD 11-2 校验。"""
    weights = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
    checksum_map = ['1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2']
    for _ in range(50):
        id_card = gen_id_card()
        assert len(id_card) == 18, f"身份证号长度不为 18 位: {id_card}"
        total = sum(int(id_card[i]) * weights[i] for i in range(17))
        expected = checksum_map[total % 11]
        assert id_card[-1].upper() == expected, f"校验码不匹配: {id_card}"


def test_generated_data_contains_l4_and_l5_content() -> None:
    """验证生成的数据中确实包含 L4 和 L5 级病史内容。
    
    使用固定种子保证可重复性，因为诊断与病史的组合是随机的。
    """
    import random
    random.seed(2026)  # 固定种子，确保可重复
    records = generate_dataset(20)
    pipeline = MedicalPrivacyPipeline()
    result = pipeline.process_records(records)
    # 分级报告应包含 L4 和 L5 级记录
    levels = {r["max_level"] for r in result.classification_report}
    assert "L5" in levels, "生成数据中缺少 L5 级记录"
    assert "L4" in levels, "生成数据中缺少 L4 级记录"


def test_generated_data_contains_image_cases() -> None:
    """验证生成数据中至少 3 条包含图片病例标记 (PACS / samples)。"""
    records = generate_dataset(20)
    image_count = sum(1 for r in records if "PACS" in r.get("present_illness", "") or "data/samples/" in r.get("present_illness", ""))
    assert image_count >= 3, f"图片病例数量不足: {image_count}"


def test_sanitize_text_strips_all_l4_l5_terms() -> None:
    """验证 sanitize_text 方法能剥离单条文本中的所有 L4/L5 术语。"""
    pipeline = MedicalPrivacyPipeline()
    # 混合 L5 (HIV) + L4 (恶性肿瘤) 的文本
    mixed_text = "患者HIV抗体阳性，同时确诊为恶性肿瘤，建议进一步检查。"
    sanitized = pipeline.sanitize_text(mixed_text)
    assert "HIV" not in sanitized
    assert "艾滋" not in sanitized
    assert "恶性肿瘤" not in sanitized
    assert "[L5-" in sanitized  # L5 替换标记存在
    assert "[L4-" in sanitized  # L4 替换标记存在


def test_replacement_tags_do_not_leak_sensitive_terms() -> None:
    """验证 L4/L5 替换标签中不包含原始敏感词汇（如 HIV、乙肝等）。"""
    leaked_terms = ["HIV", "艾滋", "精神分裂", "亨廷顿", "恶性肿瘤", "胃癌", "乙肝", "丙肝"]
    for _pat, replacement in L5_PATTERNS + L4_PATTERNS:
        for term in leaked_terms:
            assert term not in replacement, (
                f"替换标签 '{replacement}' 中泄露了敏感词 '{term}'"
            )


def test_pii_masking_format_id_card() -> None:
    """验证身份证号脱敏后保留前 6 后 4，中间为 8 个 *。"""
    records = [{"name": "张三", "id_card_no": "110101199003071234"}]
    res = process_medical_dataset(records)
    masked_id = res.sanitized_data[0]["id_card_no"]
    assert masked_id.startswith("110101"), f"身份证前 6 位应保留: {masked_id}"
    assert "********" in masked_id, f"身份证中间应为 8 个 *: {masked_id}"


def test_pii_masking_format_name() -> None:
    """验证姓名脱敏后首字保留、其余用 * 替代。"""
    records = [{"name": "张三丰", "id_card_no": "110101199003071234"}]
    res = process_medical_dataset(records)
    masked_name = res.sanitized_data[0]["name"]
    assert masked_name.startswith("张"), f"姓名首字应保留: {masked_name}"
    assert "*" in masked_name, f"姓名中间应含 *: {masked_name}"
    assert masked_name != "张三丰", "姓名不应为原始值"


def test_chinese_pii_field_names_are_classified_and_masked() -> None:
    """中文字段名应与对应英文规范字段使用相同的分类和脱敏策略。"""
    records = [{
        "姓名": "张三丰",
        "身份证号": "110101199003071234",
        "家庭住址": "北京市朝阳区幸福路100号",
        "残疾证号": "残疾证123456789",
        "医保卡号": "医保卡987654321",
    }]

    result = process_medical_dataset(records)
    masked = result.sanitized_data[0]
    report = result.classification_report[0]

    assert set(report["pii_fields_detected"]) == set(records[0])
    assert masked["姓名"] != records[0]["姓名"]
    assert masked["身份证号"].startswith("110101")
    assert "********" in masked["身份证号"]
    assert masked["家庭住址"] != records[0]["家庭住址"]
    assert masked["残疾证号"] != records[0]["残疾证号"]
    assert masked["医保卡号"] != records[0]["医保卡号"]


def test_sanitizer_rejects_unknown_mode() -> None:
    """脱敏模式拼写错误必须立即报错，不能静默进入默认分支。"""
    pipeline = MedicalPrivacyPipeline(redact_engine="rule")

    with pytest.raises(ValueError, match="Unsupported sanitization mode"):
        pipeline._medical_text_sanitizer("diagnosis_name", "乙肝", "L4", mode="redcat")


def test_pipeline_rejects_unknown_redact_engine() -> None:
    """脱敏引擎名称拼写错误必须在初始化时失败。"""
    with pytest.raises(ValueError, match="Unsupported redact_engine"):
        MedicalPrivacyPipeline(redact_engine="nerx")


def test_empty_records_handling() -> None:
    """验证空记录列表的处理不报错。"""
    res = process_medical_dataset([])
    assert res.summary["total_records"] == 0
    assert len(res.classification_report) == 0
    assert len(res.sanitized_data) == 0


def test_unified_patterns_importable_from_pipeline_masker() -> None:
    """验证 pipeline/masker.py 能正确导入统一的 L4/L5 词库。"""
    from privacy_local_agent.pipeline.masker import L4_PATTERNS as MP_L4, L5_PATTERNS as MP_L5
    assert len(MP_L4) == len(L4_PATTERNS)
    assert len(MP_L5) == len(L5_PATTERNS)


def test_unknown_field_l4_l5_text_is_removed() -> None:
    """未知字段名不能绕过医疗 L4/L5 文本门禁。"""
    raw = "扩展字段：患者 HIV 感染，同时存在恶性肿瘤。"
    result = process_medical_dataset([{"custom_note": raw}])
    sanitized = result.sanitized_data[0]["custom_note"]

    assert "HIV" not in sanitized
    assert "恶性肿瘤" not in sanitized
    assert sanitized != raw
    assert result.summary["guarantee_no_l4_l5_raw_data"] is True


def test_clean_text_fast_path_preserves_unmodified_text() -> None:
    """无敏感词文本应当通过 fast-path 原样保留，避免被语法自愈逻辑误篡改。"""
    cases = [
        "弟弟说'你好'，今天天气不错。",
        "母亲“高血压”控制良好。",
        "他长期。",
        "注意保暖。。。多休息",
        "第一段。\n\n第二段。",
    ]
    for text in cases:
        assert redact_medical_text(text) == text, f"干净文本不应被篡改: {text}"


def test_redact_paired_list_sensitive_suffix_cleanup() -> None:
    """顿号列表中敏感词在后场景不应遗留 '、。' 标点碎片。"""
    text = "一弟患'2型糖尿病'、'重度精神分裂症'。"
    assert redact_medical_text(text) == "一弟患'2型糖尿病'。"


def test_redact_medication_without_suffix() -> None:
    """无结尾治疗后缀的服药文本亦应被完整抹平。"""
    text = "服用'奥氮平片'20mg qd。"
    assert redact_medical_text(text) == ""


def test_redact_cause_of_death_with_complications() -> None:
    """带有并发症修饰的死因句法应重构为因病去世。"""
    text = "因'HIV'导致的并发症去世。"
    assert redact_medical_text(text) == "因病去世。"



def test_failed_image_redaction_never_returns_original_value() -> None:
    """不存在或损坏图片必须 fail closed，不能把原路径当作脱敏结果。"""
    raw_path = "/tmp/medical-private-case.png"
    result = process_medical_dataset([{"case_image": raw_path}])

    assert result.sanitized_data[0]["case_image"] == "[IMAGE-REDACTION-FAILED]"
    assert result.sanitized_data[0]["case_image"] != raw_path
    assert result.summary["redaction_failures"] == 1
    assert result.summary["guarantee_no_l4_l5_raw_data"] is False


def test_sanitize_false_does_not_claim_a_safety_guarantee() -> None:
    """仅分类、不脱敏时不得报告已满足无高敏原文保证。"""
    result = process_medical_dataset([{"present_illness": "HIV 感染"}], sanitize=False)

    assert result.sanitized_data[0]["present_illness"] == "HIV 感染"
    assert result.summary["guarantee_no_l4_l5_raw_data"] is False

