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
# 无痕抹平模式 (Redaction / Purge Mode) 句法重构与全场景擦除正则引擎
# ---------------------------------------------------------------------------
_ALL_L4_L5_TERMS = sorted(
    [term for terms in list(L5_TERMS_MAP.values()) + list(L4_TERMS_MAP.values()) for term in terms],
    key=len,
    reverse=True,
)

_TERMS_OR = "|".join([re.escape(t) for t in _ALL_L4_L5_TERMS])
_Q = r"['\"“‘'”’]?"

# 1. 扩展死因相关句法短语重构：“因/由于/死于 'L4/L5词' (去世|死于|离世|逝世...)” -> “因病去世”
_REDACT_DEATH_ACTION = r"(?:去世|死于|离世|殁于|不幸身亡|宣告不治|逝世)"
_REDACT_CAUSE_DEATH_PATTERN = re.compile(
    rf"(?:因|由于|死于|殁于|因为|由)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:导致|引起)?\s*({_REDACT_DEATH_ACTION})?",
    re.IGNORECASE,
)
_REDACT_SUFFER_DEATH_PATTERN = re.compile(
    rf"(?:患有?|确诊(?:为)?|诊断(?:为)?|患)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*({_REDACT_DEATH_ACTION})",
    re.IGNORECASE,
)

# 2. 诊疗、手术、服药与处置短语擦除：“长期服用奥氮平片”、“行抗逆转录治疗”、“接受保护性约束”
_REDACT_TREATMENT_PATTERN = re.compile(
    rf"(?:长期|定期|行|实施|接受|进行|服用|使用|给予|配合)?\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:治疗|片|胶囊|针剂|注射|约束|手术|复查)?",
    re.IGNORECASE,
)

# 3. 诊断与检出句法短语擦除：“确诊为艾滋病”、“检查出梅毒”、“合并乙型肝炎”
_REDACT_DIAGNOSIS_PATTERN = re.compile(
    rf"(?:确诊(?:为)?|诊断(?:为)?|检查出|查出|发现|提示|存在|具有|伴有?|合并|疑似)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?",
    re.IGNORECASE,
)

# 4. 顿号分隔的复合疾病列表中的敏感词擦除：“患'重度精神分裂症'、'2型糖尿病'” -> “患'2型糖尿病'”
_REDACT_PAIRED_PATTERN = re.compile(
    rf"((?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)\s*){_Q}(?:{_TERMS_OR}){_Q}\s*[、,，]\s*",
    re.IGNORECASE,
)

# 5. 兜底匹配包含介词/动词/病史后缀的敏感短语：“患'重度精神分裂症'” / “有'乙肝'病史”
_REDACT_PREFIX_PATTERN = re.compile(
    rf"(?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)?\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?",
    re.IGNORECASE,
)

# 6. 消除擦除敏感词后可能残留的孤立亲属主语短语（例如“，一弟。” / “父亲。”）
_FAMILY_MEMBERS = (
    r"父亲|母亲|祖父|祖母|外公|外婆|爷爷|奶奶|伯父|叔叔|舅舅|姑姑|姨妈|大伯|大舅|大姨|二姨|小姨|"
    r"一弟|二弟|三弟|长子|次子|长女|次女|大哥|二哥|大姐|二姐|弟弟|妹妹|哥哥|姐姐|爱人|配偶|丈夫|妻子|儿子|女儿|家属|家族成员"
)
_CLEANUP_ORPHAN_SUBJECT_PATTERN = re.compile(rf"(?:^|[，,。；])\s*(?:{_FAMILY_MEMBERS})\s*([。；;])")

# 7. 孤立动词/介词残余清理：“，患。” / “，因。” -> “”
_CLEANUP_ORPHAN_VERB_PATTERN = re.compile(r"(?:^|[，,。；])\s*(?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有)\s*([。；;，,])")

# 8. 标点符号与空引号清理正则
_CLEANUP_EMPTY_QUOTES_PATTERN = re.compile(r"['\"“‘]['\"”’]")
_CLEANUP_PUNCTUATION_PATTERN = re.compile(r"([，。；：,;\s])\1+")
_CLEANUP_EMPTY_CLAUSE_PATTERN = re.compile(r"([，,])\s*([。;；])")


def redact_medical_text(text: str) -> str:
    """全场景无痕抹平算法 (Redaction/Purge Mode).

    全面覆盖死因、诊疗服药、诊断检出、家族病史等各类复杂中文医疗句法：
    1. 把“因'恶性肿瘤'去世”、“死于'肺腺癌'”等死因短语自然重构为“因病去世”；
    2. 将包含 L4/L5 敏感病史词汇连同与其绑定的介词/动词/服药/治疗动作完全擦除；
    3. 清理空引号、孤立动词与无谓语孤立主语句，修复多余标点，做到语法流畅、自然无痕。
    """
    if not text:
        return text

    s = text

    # 1. 优先将死因句法重构为自然流畅的“因病去世/死于”
    def _death_replace(match: re.Match) -> str:
        action = match.group(1) or "去世"
        return f"因病{action}"

    s = _REDACT_CAUSE_DEATH_PATTERN.sub(_death_replace, s)
    s = _REDACT_SUFFER_DEATH_PATTERN.sub(_death_replace, s)

    # 2. 擦除诊疗、服药、处置短语（如“长期服用奥氮平片”、“行抗逆转录治疗”）
    s = _REDACT_TREATMENT_PATTERN.sub("", s)

    # 3. 擦除诊断与检出短语（如“确诊为艾滋病”、“检查出梅毒”）
    s = _REDACT_DIAGNOSIS_PATTERN.sub("", s)

    # 4. 保留后接非敏感疾病时的动词修饰：“患'L4病'、'L3病'” -> “患'L3病'”
    s = _REDACT_PAIRED_PATTERN.sub(r"\1", s)

    # 5. 擦除“患有.../有...病史/因...”句法短语与单独术语
    s = _REDACT_PREFIX_PATTERN.sub("", s)

    # 6. 消除失去谓语语境的孤立主语句与孤立动词残余（如擦除病史后残存的“一弟。” / “，患。”）
    s = _CLEANUP_ORPHAN_VERB_PATTERN.sub(r"\1", s)
    s = _CLEANUP_ORPHAN_SUBJECT_PATTERN.sub(r"\1", s)

    # 7. 标点与空引号格式净化自愈
    s = _CLEANUP_EMPTY_QUOTES_PATTERN.sub("", s)
    s = _CLEANUP_PUNCTUATION_PATTERN.sub(r"\1", s)
    s = _CLEANUP_EMPTY_CLAUSE_PATTERN.sub(r"\2", s)

    # 8. 去除可能产生的开局或结尾孤立标点
    s = re.sub(r"^[，,]\s*", "", s)

    return s

