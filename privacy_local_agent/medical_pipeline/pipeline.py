"""医疗数据分类分级与脱敏 Pipeline 核心实现模块。
Core implementation of Medical Data Classification & Desensitization Pipeline.
"""

from __future__ import annotations

import threading
import time
from dataclasses import asdict, dataclass
from typing import Any

from privacy_local_agent.dynclassification import DynClassificationService
from privacy_local_agent.dynclassification.image_redaction import IMAGE_REDACTION_FAILURE
from privacy_local_agent.privacy.masking import mask_value

from .rules import L4_PATTERNS, L5_PATTERNS, PII_FIELD_RULES, redact_medical_text

IMAGE_FAILURE = IMAGE_REDACTION_FAILURE


def _mask_string(field_name: str, value: str) -> str:
    """调用 masking 并稳定返回字符串，屏蔽其可选详情返回类型。"""
    result = mask_value(field_name, value, return_details=False)
    if isinstance(result, str):
        return result
    return result.value


@dataclass
class FieldClassification:
    field_name: str
    level: str  # L1, L2, L3, L4, L5
    security_tag: str
    description: str
    rule_matched: str
    raw_value: str = ""
    sanitized_value: str = ""


@dataclass
class RecordClassificationReport:
    record_index: int
    max_level: str
    pii_fields_detected: list[str]
    high_sensitivity_detected: list[str]  # L4/L5 级风险
    field_details: list[FieldClassification]
    raw_record: dict[str, str] = None


@dataclass
class MedicalPipelineResult:
    classification_report: list[dict[str, Any]]
    sanitized_data: list[dict[str, str]]
    raw_data: list[dict[str, str]]
    summary: dict[str, Any]


try:
    from ..dynclassification.ner_adapter import NerAdapter
except ImportError:
    NerAdapter = None


class MedicalPrivacyPipeline:
    """医疗敏感数据全流程治理 Pipeline。
    
    1. 动态分类分级：集成 dynclassification 3层漏斗 (Rule -> NER -> LLM) 识别 27 个字段及临床文本中的 L1~L5 风险标识；
    2. 身份与特高风险脱敏：抹平 PII 信息，强剥离 L4/L5 高风险病史词汇；支持 "ner" (Layer-2 Small-NER) 与 "rule" 双引擎抹平模式；
    3. 输出双结构数据：(1) 分级报告 (2) 合规脱敏数据。
    """

    def __init__(
        self,
        dyn_service: DynClassificationService | None = None,
        redact_engine: str = "ner",
    ):
        """初始化 Pipeline 引擎，挂载 DynClassificationService 统一分类能力与 Small-NER 抹平引擎。"""
        if dyn_service is None:
            # 创建 DynClassificationService 并注入医疗领域文本脱敏回调
            # 解耦 dynclassification 对 medical_pipeline 的反向依赖
            dyn_service = DynClassificationService(text_sanitizer=self._medical_text_sanitizer)
        self.dyn_service = dyn_service
        self.redact_engine = redact_engine
        self.ner_adapter = NerAdapter() if NerAdapter is not None else None
        self._lock = threading.Lock()
        # 缓存 _classify_field 中 dyn_service 计算的 sanitized_value，供 _sanitize_field 复用，
        # 避免同一字段被三层漏斗分类两次（性能优化）。
        self._sanitized_cache: dict[tuple[str, str], str] = {}

    def _medical_text_sanitizer(self, field_name: str, text: str, final_level: str, mode: str = "redact") -> str:
        """医疗领域文本脱敏回调（注入到 DynClassificationService）。

        支持双引擎抹平模式：
        - redact_engine=="ner": 启用 Layer-2 Small-NER (ONNXRuntime/ModelScope/TensorRT) 实体识别无痕抹平
        - redact_engine=="rule": 采用高级规则引擎抹平模式

        默认彻底擦除 L4/L5 敏感病史与关联句法介词，不留任何形如 [L4-xxx] 的提示性标志。
        若 mode=="mask" 则回退为显式标签掩码模式。
        """
        if mode == "mask":
            sanitized_text: str = text
            for pat, replacement in L5_PATTERNS:
                sanitized_text = pat.sub(replacement, sanitized_text)
            for pat, replacement in L4_PATTERNS:
                sanitized_text = pat.sub(replacement, sanitized_text)
        else:
            # 优先采用 Layer-2 Small-NER 抹平模式
            if self.redact_engine == "ner":
                sanitized_text = redact_medical_text_with_ner(text, ner_adapter=self.ner_adapter)
            else:
                sanitized_text = redact_medical_text(text)

            if field_name in ["diagnosis_name", "diagnosis"]:
                if sanitized_text.strip() in ["慢性", "既往", "既往慢性"]:
                    sanitized_text = ""

        # 仅对明确的个人信息字段应用 PII 掩码
        if field_name in PII_FIELD_RULES:
            return _mask_string(field_name, sanitized_text)

        return sanitized_text

    def _classify_field(self, key: str, val: str) -> FieldClassification:
        """单字段分类分级评估算法（优先调度 dynclassification 动态分类引擎）。
        
        算法流程：
        1. 类型安全转换：在 None 时置为空串，保留 0 和 False 等合法数据；
        2. 调度 dynclassification 引擎评估该字段；
        3. PII 身份规则拦截：若列名命中 PII 词库，根据 GB 11643 标准设定 ID Card 为 L4，其余为 L3；
        4. 病史文本深度匹配：扫描 L5 (极高敏: HIV/重度精神障碍/遗传缺陷) 与 L4 (高敏: 肿瘤/性病/乙肝/衰竭) 词库；
        5. 普通临床与评估字段映射：根据医疗标准规范赋予 L3 (主诉/病史) 与 L2 (健康评估/个人史)；
        6. 综合 dynclassification 与医疗规则取最高敏等级。
        """
        val_str = "" if val is None else str(val)
        
        # 先调度 dynclassification 动态引擎获取通用/领域分类结果
        # 优化: 同时请求 sanitize=True，将 sanitized_value 缓存供 _sanitize_field 复用
        dyn_level: str | None = None
        try:
            dyn_resp = self.dyn_service.classify_field(key, val_str, sanitize=True)
            if dyn_resp and dyn_resp.field_result:
                dyn_level = dyn_resp.field_result.final_level
                # 缓存 dyn_service 智能抹平结果，避免 _sanitize_field 重复调用
                sanitized_value = dyn_resp.field_result.sanitized_value
                if isinstance(sanitized_value, str) and sanitized_value != val_str:
                    with self._lock:
                        self._sanitized_cache[(key, val_str)] = sanitized_value
        except Exception:
            dyn_level = None

        # 步骤 1: PII 身份字段检测与分级
        if key in PII_FIELD_RULES:
            level = "L4" if key == "id_card_no" else "L3"
            return FieldClassification(
                field_name=key,
                level=level,
                security_tag="PII_IDENTITY",
                description=f"个人身份标识信息 ({key})",
                rule_matched=f"PII_RULE_{PII_FIELD_RULES[key]}",
            )

        # 步骤 2: 病史文本中扫描所有 L5/L4 术语，取最高命中等级
        # 特别优化: 先扫描 L5，命中立即中断 (L5 已是最高级)；未命中再扫描 L4
        detected_level: str | None = None
        detected_category: str | None = None

        for pat, _replacement in L5_PATTERNS:
            if pat.search(val_str):
                detected_level = "L5"
                # 从替换标签中提取类别代码 (如 [L5-IMMUNODEFICIENCY-SENSITIVE-MASKED] → IMMUNODEFICIENCY)
                tag = _replacement.strip("[]")
                parts = tag.split("-")
                if len(parts) >= 2:
                    detected_category = parts[1]
                break  # L5 已是最高级，中断循环

        if detected_level is None:
            for pat, _replacement in L4_PATTERNS:
                if pat.search(val_str):
                    detected_level = "L4"
                    tag = _replacement.strip("[]")
                    parts = tag.split("-")
                    if len(parts) >= 2:
                        detected_category = parts[1]
                    break  # 已找到 L4

        # 融合 dynclassification 动态分类引擎的定级结果 (如 3-Layer 漏斗识别的高敏词汇)
        if detected_level is None and dyn_level in ["L4", "L5"]:
            detected_level = dyn_level

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
                description="高风险病史/诊断信息 (L4: 恶性肿瘤/性病传染病/重度衰竭)",
                rule_matched="MEDICAL_L4_STRICT_RULE",
            )

        # 步骤 3: 其他普通临床与评估字段映射
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

        # 步骤 4: 通用 L1 级兜底
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

    @staticmethod
    def _contains_high_risk_text(text: str) -> bool:
        """判断文本是否仍包含未抹平的 L4/L5 术语。"""
        return any(pattern.search(text) for pattern, _replacement in L4_PATTERNS + L5_PATTERNS)

    def _mask_pii_value(self, key: str, val_str: str) -> str:
        """PII 身份字段统一脱敏（提取公共逻辑，避免重复代码）。"""
        if key == "id_card_no":
            return _mask_string("id_card_no", val_str)
        if key == "name":
            return _mask_string("name", val_str)
        if key == "registered_address":
            return _mask_string("address", val_str)
        if key in ["disability_cert_no", "medical_insurance_no"]:
            if len(val_str) > 6:
                return val_str[:4] + "*" * (len(val_str) - 6) + val_str[-2:]
            return "****"
        return val_str

    def sanitize_field(self, key: str, val: str) -> str:
        """字段智能抹平脱敏（公开 API，向后兼容）。

        注意：在 process_records 循环中使用 _sanitize_field 代替，
        因为 _sanitize_field 可复用 _classify_field 的缓存，避免重复调用 dyn_service。
        """
        # 公开 API 需要先执行分类以填充缓存
        self._classify_field(key, val)
        return self._sanitize_field(key, val)

    def _sanitize_field(self, key: str, val: str) -> str:
        """字段智能抹平脱敏（供 process_records 内部使用）。

        优先复用 _classify_field 中 dyn_service 已计算的 sanitized_value（避免二次调用），
        对 PII 字段保持强掩码规则，对图像病例调用图像打码模块。
        """
        val_str = "" if val is None else str(val)

        # 0. 图像病例检测：文件路径或 Base64 Data URI → 调用图像打码
        val_stripped = val_str.strip()
        is_image = (
            len(val_stripped) < 512
            and any(
                val_stripped.lower().endswith(ext)
                for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp", ".tif", ".tiff", ".dcm", ".dicom")
            )
        ) or val_stripped.lower().startswith(("data:image/", "image:"))
        if is_image:
            try:
                from privacy_local_agent.dynclassification.image_redaction import sanitize_image_input
                return sanitize_image_input(val_str)
            except Exception:
                return IMAGE_FAILURE

        # 1. 优先复用 _classify_field 中 dyn_service 已计算的 sanitized_value
        cache_key = (key, val_str)
        cached: str | None = None
        with self._lock:
            if cache_key in self._sanitized_cache:
                cached = self._sanitized_cache.pop(cache_key)

        if cached is not None:
            # PII 字段保持强掩码规则（dyn_service 的 sanitize 可能不够强）
            if key in PII_FIELD_RULES:
                return self._mask_pii_value(key, val_str)
            return cached

        # 2. PII 字段始终使用强掩码
        if key in PII_FIELD_RULES:
            return self._mask_pii_value(key, val_str)

        # 3. 备用降级：文本强剥离 L4/L5 术语
        clinical_keys = {
            "diagnosis_name", "chief_complaint", "present_illness",
            "past_history", "personal_history", "family_history",
            "allergic_history", "progress_note",
        }
        # 不依赖字段名：未知字段中的高敏医疗文本同样必须被抹平。
        if (
            key in clinical_keys
            or self._contains_high_risk_text(val_str)
            or self._classify_field(key, val_str).level in ["L4", "L5"]
        ):
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
        redaction_failures = 0

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
                    
                # 使用 _sanitize_field 复用 _classify_field 的 dyn_service 结果（单次调用优化）
                if sanitize:
                    sanitized_rec[key] = self._sanitize_field(key, val_str)
                    if sanitized_rec[key] == "[IMAGE-REDACTION-FAILED]":
                        redaction_failures += 1
                    elif self._contains_high_risk_text(sanitized_rec[key]):
                        # 最终门禁：任何漏网的高敏文本整体删除，不能返回部分原文。
                        sanitized_rec[key] = "[L4-L5-DATA-REMOVED]"
                else:
                    sanitized_rec[key] = val_str

                fc.raw_value = val_str
                fc.sanitized_value = sanitized_rec[key]

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
                raw_record=rec,
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
            "redaction_failures": redaction_failures,
            "guarantee_no_l4_l5_raw_data": bool(sanitize and redaction_failures == 0),
            "duration_ms": round(elapsed_ms, 2),
        }

        return MedicalPipelineResult(
            classification_report=reports,
            sanitized_data=sanitized_records,
            raw_data=records,
            summary=summary,
        )


def process_medical_dataset(
    records: list[dict[str, str]], sanitize: bool = True
) -> MedicalPipelineResult:
    """高层入口：处理医疗数据集并返回分类分级报告与脱敏清洗数据。"""
    return MedicalPrivacyPipeline().process_records(records, sanitize=sanitize)
