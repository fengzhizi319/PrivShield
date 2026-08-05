"""医疗数据分类分级与脱敏 Pipeline 核心实现模块。
Core implementation of Medical Data Classification & Desensitization Pipeline.
"""

from __future__ import annotations

import time
from dataclasses import asdict, dataclass
from typing import Any

from privacy_local_agent.privacy.masking import mask_value

from .rules import L4_PATTERNS, L5_PATTERNS, PII_FIELD_RULES


@dataclass
class FieldClassification:
    field_name: str
    level: str  # L1, L2, L3, L4, L5
    security_tag: str
    description: str
    rule_matched: str


@dataclass
class RecordClassificationReport:
    record_index: int
    max_level: str
    pii_fields_detected: list[str]
    high_sensitivity_detected: list[str]  # L4/L5 级风险
    field_details: list[FieldClassification]


@dataclass
class MedicalPipelineResult:
    classification_report: list[dict[str, Any]]
    sanitized_data: list[dict[str, str]]
    summary: dict[str, Any]


class MedicalPrivacyPipeline:
    """医疗敏感数据全流程治理 Pipeline。
    
    1. 动态分类分级：识别 27 个字段及临床文本中的 L1~L5 风险标识；
    2. 身份与特高风险脱敏：抹平 PII 信息，强剥离 L4/L5 高风险病史词汇；
    3. 输出双结构数据：(1) 分级报告 (2) 合规脱敏数据。
    """

    def _classify_field(self, key: str, val: str) -> FieldClassification:
        val_str = "" if val is None else str(val)
        
        # 1. PII 身份字段检测
        if key in PII_FIELD_RULES:
            level = "L4" if key == "id_card_no" else "L3"
            return FieldClassification(
                field_name=key,
                level=level,
                security_tag="PII_IDENTITY",
                description=f"个人身份标识信息 ({key})",
                rule_matched=f"PII_RULE_{PII_FIELD_RULES[key]}",
            )

        # 2. 病史文本中扫描所有 L5/L4 术语，取最高命中等级
        #    修复: 原先仅返回首个匹配，现遍历全部规则以确保最高等级被捕获
        detected_level: str | None = None
        detected_category: str | None = None

        for pat, _replacement in L5_PATTERNS:
            if pat.search(val_str):
                detected_level = "L5"
                # 从替换标签中提取类别名 (如 [L5-HIV_AIDS-SENSITIVE-MASKED] → HIV_AIDS)
                tag = _replacement.strip("[]")
                parts = tag.split("-")
                if len(parts) >= 2:
                    detected_category = parts[1]
                break  # L5 已是最高级，无需继续

        if detected_level is None:
            for pat, _replacement in L4_PATTERNS:
                if pat.search(val_str):
                    detected_level = "L4"
                    tag = _replacement.strip("[]")
                    parts = tag.split("-")
                    if len(parts) >= 2:
                        detected_category = parts[1]
                    break  # 已找到 L4

        if detected_level == "L5":
            return FieldClassification(
                field_name=key,
                level="L5",
                security_tag="HIGH_RISK_MEDICAL_L5",
                description="极高风险病史/诊断信息 (L5: 重度精神障碍/HIV/重大遗传缺陷)",
                rule_matched="MEDICAL_L5_STRICT_RULE",
            )
        if detected_level == "L4":
            return FieldClassification(
                field_name=key,
                level="L4",
                security_tag="HIGH_RISK_MEDICAL_L4",
                description="高风险病史/诊断信息 (L4: 恶性肿瘤/烈性传染病/重度衰竭)",
                rule_matched="MEDICAL_L4_STRICT_RULE",
            )

        # 3. 其他普通临床与评估字段
        if key in ["chief_complaint", "past_history", "family_history", "allergic_history"]:
            return FieldClassification(
                field_name=key,
                level="L3",
                security_tag="CLINICAL_HISTORY",
                description="临床病史与问诊主诉",
                rule_matched="CLINICAL_TEXT_RULE",
            )
            
        if key in ["disability_category", "disability_level", "assess_result_name", "personal_history"]:
            return FieldClassification(
                field_name=key,
                level="L2",
                security_tag="HEALTH_ASSESSMENT",
                description="健康与残疾评估信息",
                rule_matched="ASSESSMENT_RULE",
            )

        return FieldClassification(
            field_name=key,
            level="L1",
            security_tag="GENERAL_INFO",
            description="普通健康/人口学统计信息",
            rule_matched="DEFAULT_L1_RULE",
        )

    def sanitize_text(self, text: str) -> str:
        """剥离与替换文本中的所有 L4/L5 敏感病史术语，保障无 L4/L5 原始词汇泄露。"""
        if not text:
            return text
        sanitized = text
        for pat, replacement in L5_PATTERNS:
            sanitized = pat.sub(replacement, sanitized)
        for pat, replacement in L4_PATTERNS:
            sanitized = pat.sub(replacement, sanitized)
        return sanitized

    def sanitize_field(self, key: str, val: str) -> str:
        """根据字段敏感类型执行脱敏与剥离。"""
        val_str = "" if val is None else str(val)
        
        # 身份 PII 字段脱敏：使用实际字段名调用 mask_value，确保 guess_field_type 正确推断
        if key in PII_FIELD_RULES:
            if key == "id_card_no":
                return mask_value("id_card_no", val_str)
            elif key == "name":
                return mask_value("name", val_str)
            elif key == "registered_address":
                return mask_value("address", val_str)
            elif key in ["disability_cert_no", "medical_insurance_no"]:
                if len(val_str) > 6:
                    return val_str[:4] + "*" * (len(val_str) - 6) + val_str[-2:]
                return "****"

        # 临床文本字段或任何判定包含 L4/L5 敏感词汇的字段，强制执行 L4/L5 术语剥离
        clinical_keys = {
            "diagnosis_name", "chief_complaint", "present_illness",
            "past_history", "personal_history", "family_history",
            "allergic_history", "progress_note",
        }
        if key in clinical_keys or self._classify_field(key, val_str).level in ["L4", "L5"]:
            return self.sanitize_text(val_str)

        return val_str

    def process_records(
        self, records: list[dict[str, str]], sanitize: bool = True
    ) -> MedicalPipelineResult:
        """处理医疗数据集记录并生成双输出。
        
        Args:
            records: 输入医疗记录列表。
            sanitize: 是否进行高敏与 PII 脱敏（默认 True）。若为 True，在单次推断/循环中同时完成分级与脱敏。
        """
        start_time = time.perf_counter()
        
        reports: list[dict[str, Any]] = []
        sanitized_records: list[dict[str, str]] = []
        
        l5_count = 0
        l4_count = 0
        l3_count = 0
        
        for idx, rec in enumerate(records, start=1):
            field_classifications: list[FieldClassification] = []
            sanitized_rec: dict[str, str] = {}
            
            rec_pii: list[str] = []
            rec_high_risk: list[str] = []
            max_level = "L1"
            
            # 兼容 L1~L5 与 C1~C5 双重等级体系防 crash
            level_rank = {
                "L1": 1, "L2": 2, "L3": 3, "L4": 4, "L5": 5,
                "C1": 1, "C2": 2, "C3": 3, "C4": 4, "C5": 5,
            }
            
            for key, val in rec.items():
                val_str = "" if val is None else str(val)
                fc = self._classify_field(key, val_str)
                field_classifications.append(fc)
                
                if fc.security_tag == "PII_IDENTITY":
                    rec_pii.append(key)
                if fc.level in ["L4", "L5", "C4", "C5"]:
                    rec_high_risk.append(f"{key}:{fc.level}")
                    
                fc_rank = level_rank.get(fc.level, 1)
                max_rank = level_rank.get(max_level, 1)
                if fc_rank > max_rank:
                    max_level = fc.level
                    
                # 单次联合推断/脱敏处理：当 sanitize=True 时进行脱敏，否则保留原值
                if sanitize:
                    sanitized_rec[key] = self.sanitize_field(key, val_str)
                else:
                    sanitized_rec[key] = val_str
                
            if max_level == "L5":
                l5_count += 1
            elif max_level == "L4":
                l4_count += 1
            elif max_level == "L3":
                l3_count += 1
                
            rep = RecordClassificationReport(
                record_index=idx,
                max_level=max_level,
                pii_fields_detected=rec_pii,
                high_sensitivity_detected=rec_high_risk,
                field_details=field_classifications,
            )
            reports.append(asdict(rep))
            sanitized_records.append(sanitized_rec)
            
        elapsed_ms = (time.perf_counter() - start_time) * 1000.0
        
        summary = {
            "total_records": len(records),
            "l5_records_count": l5_count,
            "l4_records_count": l4_count,
            "l3_records_count": l3_count,
            "l1_l2_records_count": len(records) - l5_count - l4_count - l3_count,
            "sanitized_pii_fields_per_record": len(PII_FIELD_RULES) if sanitize else 0,
            "guarantee_no_l4_l5_raw_data": sanitize,
            "duration_ms": round(elapsed_ms, 2),
        }
        
        return MedicalPipelineResult(
            classification_report=reports,
            sanitized_data=sanitized_records,
            summary=summary,
        )


def process_medical_dataset(
    records: list[dict[str, str]], sanitize: bool = True
) -> MedicalPipelineResult:
    """高层入口：处理医疗数据集并返回分类分级报告与脱敏清洗数据。"""
    return MedicalPrivacyPipeline().process_records(records, sanitize=sanitize)
