"""医疗分类分级规则与 L4/L5 级词库模块 / Medical Classification Rules & L4/L5 Terminology Dictionary.
"""

from __future__ import annotations

import re

# PII 身份隐私字段及其默认脱敏规则定义
PII_FIELD_RULES: dict[str, str] = {
    "name": "CHINESE_NAME",
    "id_card_no": "ID_CARD",
    "registered_address": "ADDRESS",
    "disability_cert_no": "DISABILITY_CERT",
    "medical_insurance_no": "INSURANCE_NO",
}

# L5 极高风险病史与诊断词汇映射组（包含疾病名、缩写、临床特征）
L5_TERMS_MAP: dict[str, list[str]] = {
    "HIV_AIDS": [
        "获得性免疫缺陷综合征", "HIV感染", "HIV", "艾滋病", "艾滋", "CD4+ T淋巴细胞", "替诺福韦+拉米夫定+多替拉韦", "ART抗逆转录", "血清HIV-1"
    ],
    "PSYCHIATRIC_DISORDER": [
        "重度精神分裂症", "精神分裂症", "幻听（命令性言语）", "命令性言语", "被害妄想", "自伤倾向", "冲动砸物", "保护性约束", "奥氮平片", "精神卫生中心"
    ],
    "GENETIC_DEFECT": [
        "遗传性亨廷顿舞蹈病", "亨廷顿舞蹈病", "Huntington Disease", "CAG重复序列", "CAG扩增", "四苯嗪", "舞蹈样动作"
    ],
}

# L4 高风险病史与诊断词汇映射组（肿瘤、性病/传染病、严重器官损害）
L4_TERMS_MAP: dict[str, list[str]] = {
    "STD_VENEREAL": [
        "梅毒", "苍白密螺旋体", "TPPA阳性", "TPPA", "RPR阳性", "RPR", "淋病", "淋球菌", "尖锐湿疣",
        "生殖器疱疹", "软下疳", "性病", "性传播疾病", "不洁性接触史", "硬下疳", "人乳头瘤病毒高危型"
    ],
    "MALIGNANT_NEOPLASM": [
        "恶性肿瘤", "浸润性腺癌", "肺腺癌", "胃癌", "肝癌", "乳腺癌", "宫颈癌", "癌症", "转移性肿瘤", "奥希替尼", "EGFR基因检测", "EGFR突变"
    ],
    "HEPATITIS_VIRUS": [
        "慢性乙型病毒性肝炎", "乙型肝炎", "乙肝", "丙型肝炎", "丙肝", "HBV-DNA", "HCV", "恩替卡韦", "肝硬化代偿期", "食管静脉曲张"
    ],
    "SEVERE_ORGAN_DAMAGE": [
        "慢性阻塞性肺疾病", "COPD", "急性心肌梗死", "冠状动脉重度狭窄"
    ],
}

# 替换标签映射：使用抽象类别代码，避免替换文本中泄露原始敏感词
# 例如不使用 [L5-HIV_AIDS-...] 而用 [L5-IMMUNODEFICIENCY-...]，防止替换后仍含 "HIV"
_L5_REPLACEMENT_MAP: dict[str, str] = {
    "HIV_AIDS": "IMMUNODEFICIENCY",
    "PSYCHIATRIC_DISORDER": "PSYCHIATRIC_DISORDER",
    "GENETIC_DEFECT": "GENETIC_DEFECT",
}

_L4_REPLACEMENT_MAP: dict[str, str] = {
    "STD_VENEREAL": "STD_VENEREAL",
    "MALIGNANT_NEOPLASM": "MALIGNANT_NEOPLASM",
    "HEPATITIS_VIRUS": "HEPATITIS_VIRUS",
    "SEVERE_ORGAN_DAMAGE": "SEVERE_ORGAN_DAMAGE",
}

# 文本脱敏正则表达式：按类别编译 L5/L4 术语为正则，长词优先匹配
L5_PATTERNS: list[tuple[re.Pattern, str]] = [
    (re.compile("|".join([re.escape(t) for t in sorted(terms, key=len, reverse=True)]), re.IGNORECASE),
     f"[L5-{_L5_REPLACEMENT_MAP.get(cat, cat)}-SENSITIVE-MASKED]")
    for cat, terms in L5_TERMS_MAP.items()
]

L4_PATTERNS: list[tuple[re.Pattern, str]] = [
    (re.compile("|".join([re.escape(t) for t in sorted(terms, key=len, reverse=True)]), re.IGNORECASE),
     f"[L4-{_L4_REPLACEMENT_MAP.get(cat, cat)}-SENSITIVE-MASKED]")
    for cat, terms in L4_TERMS_MAP.items()
]
