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

# ---------------------------------------------------------------------------
# 无痕抹平模式 (Redaction / Purge Mode) 句法重构与彻底擦除正则引擎
# ---------------------------------------------------------------------------
_ALL_L4_L5_TERMS = sorted(
    [term for terms in list(L5_TERMS_MAP.values()) + list(L4_TERMS_MAP.values()) for term in terms],
    key=len,
    reverse=True,
)

_TERMS_OR = "|".join([re.escape(t) for t in _ALL_L4_L5_TERMS])
_Q = r"['\"“‘'”’]?"

# 1. 匹配“因 'L4/L5词' (去世|死于|离世)” -> 替换为“因病\1”（如“因病去世”）
_REDACT_CAUSE_DEATH_PATTERN = re.compile(rf"因\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(去世|死于|离世)", re.IGNORECASE)

# 2. 匹配“患有?/确诊/患 'L4/L5词' (去世|死于|离世)” -> 替换为“因病\1”
_REDACT_SUFFER_DEATH_PATTERN = re.compile(rf"(?:患有?|确诊|患)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(去世|死于|离世)", re.IGNORECASE)

# 3. 匹配顿号分隔的复合疾病列表中的敏感词：“患'重度精神分裂症'、'2型糖尿病'” -> “患'2型糖尿病'”
_REDACT_PAIRED_PATTERN = re.compile(rf"((?:因|患有?|确诊|患|有)\s*){_Q}(?:{_TERMS_OR}){_Q}\s*[、,，]\s*", re.IGNORECASE)

# 4. 匹配包含介词/动词与引号的敏感短语擦除：“患'重度精神分裂症'” / “有'乙肝'病史”
_REDACT_PREFIX_PATTERN = re.compile(rf"(?:因|患有?|确诊|患|有)?\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?", re.IGNORECASE)

# 5. 消除擦除敏感词后可能残留的孤立主语短语（例如“，一弟。” / “一弟。”）
_CLEANUP_ORPHAN_SUBJECT_PATTERN = re.compile(r"(?:^|[，,。；])\s*(?:一弟|二弟|三弟|长子|次子|长女|次女|大哥|二哥|大姐|二姐|弟弟|妹妹|哥哥|姐姐|父亲|母亲|爱人|配偶|儿子|女儿)\s*([。；;])")

# 6. 标点符号与空引号清理正则
_CLEANUP_EMPTY_QUOTES_PATTERN = re.compile(r"['\"“‘]['\"”’]")
_CLEANUP_PUNCTUATION_PATTERN = re.compile(r"([，。；：,;\s])\1+")
_CLEANUP_EMPTY_CLAUSE_PATTERN = re.compile(r"([，,])\s*([。;；])")


def redact_medical_text(text: str) -> str:
    """无痕抹平算法 (Redaction/Purge Mode).

    将输入医疗病历文本中的 L4/L5 特高敏感病史词汇连同与其绑定的介词/动词前缀一并完全擦除，
    并重构句法，把“因'恶性肿瘤'去世”自然重构为“因病去世”，修复冗余标点与空引号，
    使读者无法得知原句曾提及任何敏感病史。
    """
    if not text:
        return text

    s = text
    # 1. 优先将死因句法重构为自然流畅的“因病去世/死于”
    s = _REDACT_CAUSE_DEATH_PATTERN.sub(r"因病\1", s)
    s = _REDACT_SUFFER_DEATH_PATTERN.sub(r"因病\1", s)

    # 2. 保留后接非敏感疾病时的动词修饰：“患'L4病'、'L3病'” -> “患'L3病'”
    s = _REDACT_PAIRED_PATTERN.sub(r"\1", s)

    # 3. 擦除“患有.../有...病史/因...”句法短语与单独术语
    s = _REDACT_PREFIX_PATTERN.sub("", s)

    # 4. 消除失去谓语语境的孤立主语句（如擦除病史后残存的“一弟。”）
    s = _CLEANUP_ORPHAN_SUBJECT_PATTERN.sub(r"\1", s)

    # 5. 标点与空引号格式清洗
    s = _CLEANUP_EMPTY_QUOTES_PATTERN.sub("", s)
    s = _CLEANUP_PUNCTUATION_PATTERN.sub(r"\1", s)
    s = _CLEANUP_EMPTY_CLAUSE_PATTERN.sub(r"\2", s)

    # 6. 去除可能产生的开局或结尾孤立标点
    s = re.sub(r"^[，,]\s*", "", s)

    return s

