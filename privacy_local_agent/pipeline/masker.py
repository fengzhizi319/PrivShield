"""脱敏处理封装器 / Masking & Sanitization Wrapper.

封装 privacy/masking 原语与 L4/L5 降级逻辑，对字段进行脱敏处理。
"""

from __future__ import annotations

import re
from typing import Any

from ..privacy.masking import FieldType, mask_record, mask_value
from .models import MaskingDetail, RecordClassificationDetail


# 字段名到 FieldType 自动推断映射
FIELD_TYPE_MAP: dict[str, FieldType] = {
    "name": FieldType.NAME,
    "id_card_no": FieldType.ID_CARD,
    "registered_address": FieldType.ADDRESS,
    "disability_cert_no": FieldType.ID_CARD,
    "medical_insurance_no": FieldType.ID_CARD,
}

# L4/L5 级高敏感术语匹配正则与替换文案
L4_L5_PATTERNS: list[tuple[re.Pattern[str], str]] = [
    (re.compile(r"HIV|艾滋|获得性免疫缺陷", re.IGNORECASE), "[L5-IMMUNODEFICIENCY-SENSITIVE-MASKED]"),
    (re.compile(r"精神分裂症|双相情感障碍|重度抑郁症", re.IGNORECASE), "[L5-PSYCHIATRIC_DISORDER-SENSITIVE-MASKED]"),
    (re.compile(r"亨廷顿舞蹈病|渐冻症|肌萎缩侧索硬化|白血病", re.IGNORECASE), "[L5-GENETIC_SEVERE-SENSITIVE-MASKED]"),
    (re.compile(r"恶性肿瘤|肺癌|胃癌|肝癌|结肠癌|乳腺癌", re.IGNORECASE), "[L4-MALIGNANT_NEOPLASM-SENSITIVE-MASKED]"),
    (re.compile(r"乙型病毒性肝炎|乙肝|丙型肝炎|丙肝", re.IGNORECASE), "[L4-HEPATITIS-SENSITIVE-MASKED]"),
    (re.compile(r"冠状动脉粥样硬化性心脏病|冠心病|心肌梗死", re.IGNORECASE), "[L4-CORONARY_HEART-SENSITIVE-MASKED]"),
]


def mask_records(
    records: list[dict[str, Any]],
    record_details: list[RecordClassificationDetail],
    mask_l4: bool = True,
    mask_l5: bool = True,
) -> tuple[list[dict[str, Any]], list[MaskingDetail]]:
    """根据分级明细与策略对多条记录进行脱敏处理。

    Args:
        records: 原始记录列表。
        record_details: 分级明细列表。
        mask_l4: 是否对 L4 级数据进行脱敏/剥离。
        mask_l5: 是否对 L5 级数据进行脱敏/剥离。

    Returns:
        (masked_records, masking_details) 脱敏后记录列表与脱敏明细列表。
    """
    masked_records: list[dict[str, Any]] = []
    masking_details: list[MaskingDetail] = []

    # 建立 record_index -> RecordClassificationDetail 映射
    detail_map = {d.record_index: d for d in record_details}

    for idx, record in enumerate(records):
        rec_detail = detail_map.get(idx)
        masked_rec = dict(record)

        # 1. 对常规 PII 字段使用 privacy.masking 进行字段感知脱敏
        for fname, val in record.items():
            str_val = str(val) if val is not None else ""
            if not str_val:
                continue

            # 查寻找分类明细中对应的等级
            orig_level = "L1"
            if rec_detail:
                for fd in rec_detail.field_details:
                    if fd.field_name == fname:
                        orig_level = fd.sensitivity_level
                        break

            # PII 自动推断脱敏
            ftype = FIELD_TYPE_MAP.get(fname)
            if ftype:
                mval = mask_value(field_name=fname, value=str_val)
                if mval != str_val:
                    masked_rec[fname] = mval
                    masking_details.append(
                        MaskingDetail(
                            record_index=idx,
                            field_name=fname,
                            original_level=orig_level,
                            masking_type=ftype.value if hasattr(ftype, "value") else str(ftype),
                            original_value=str_val,
                            masked_value=mval,
                        )
                    )
                    str_val = mval  # 更新用于后续处理

            # 2. 对 L4/L5 级敏感数据进行高敏词汇强剥离
            if (orig_level == "L5" and mask_l5) or (orig_level == "L4" and mask_l4) or fname in (
                "diagnosis_name", "present_illness", "past_history", "progress_note", "family_history"
            ):
                cur_val = masked_rec[fname]
                new_val = cur_val
                for pattern, replacement in L4_L5_PATTERNS:
                    if pattern.search(new_val):
                        new_val = pattern.sub(replacement, new_val)

                if new_val != cur_val:
                    masked_rec[fname] = new_val
                    masking_details.append(
                        MaskingDetail(
                            record_index=idx,
                            field_name=fname,
                            original_level=orig_level,
                            masking_type="L4_L5_SANITY_STRIP",
                            original_value=cur_val,
                            masked_value=new_val,
                        )
                    )

        masked_records.append(masked_rec)

    return masked_records, masking_details
