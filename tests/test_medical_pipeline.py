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
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "scripts"))
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
    with open(csv_path, encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            records.append(row)
            
    assert len(records) == 20
    res = process_medical_dataset(records)
    assert res.summary["total_records"] == 20
    assert res.summary["l5_records_count"] > 0
    assert res.summary["l4_records_count"] > 0
