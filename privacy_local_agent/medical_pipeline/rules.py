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
_DOSE = r"(?:\s*\d+(?:\.\d+)?\s*(?:mg|g|ml|u|ug|片|粒|支))?"
_FREQ = r"(?:\s*(?:qd|bid|tid|qid|qn|qw|im|iv|po))?"

# 1. 死因相关句法短语重构：“因/由于/死于 'L4/L5词' (去世|死于|离世|逝世...)” -> “因病去世”
_REDACT_DEATH_ACTION = r"(?:去世|死于|离世|殁于|不幸身亡|宣告不治|逝世)"
_REDACT_CAUSE_DEATH_PATTERN = re.compile(
    rf"(?:因|由于|死于|殁于|因为|由)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:导致|引起)?\s*({_REDACT_DEATH_ACTION})?",
    re.IGNORECASE,
)
_REDACT_SUFFER_DEATH_PATTERN = re.compile(
    rf"(?:患有?|确诊(?:为)?|诊断(?:为)?|患)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*({_REDACT_DEATH_ACTION})",
    re.IGNORECASE,
)

# 2. 完整服药/用药句法整句擦除（包含药名、剂量、频次、连词及“控制症状/治疗”）
_REDACT_MEDICATION_FULL_PATTERN = re.compile(
    rf"(?:长期|定期|口服|服用|给予|使用|行|实施|接受)?\s*{_Q}(?:{_TERMS_OR}){_Q}{_DOSE}{_FREQ}\s*(?:及|与|和|合并)?\s*(?:{_Q}(?:{_TERMS_OR}){_Q}{_DOSE}{_FREQ})*\s*(?:控制症状|抗病毒治疗|对症治疗|治疗|对症处理)?",
    re.IGNORECASE,
)

# 3. 就诊机构与就诊短语整句擦除：“曾就诊于精神卫生中心” -> “”
_REDACT_HOSPITAL_PATTERN = re.compile(
    rf"(?:曾?就诊于|就诊于|收治于|转诊至)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:，|,|。|；|;)?",
    re.IGNORECASE,
)

# 4. 专科独立诊断短语整句擦除：“诊断为重度精神分裂症。” -> “”
_REDACT_DIAGNOSIS_STANDALONE_PATTERN = re.compile(
    rf"(?:诊断为|确诊为|检查出|提示为|考虑为)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:，|,|。|；|;)?",
    re.IGNORECASE,
)

# 5. 连词+敏感特征+倾向/表现整块擦除：“及保护性约束倾向” -> “”
_REDACT_FEATURE_TENDENCY_PATTERN = re.compile(
    rf"(?:及|与|和|伴|伴有)?\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:倾向|表现)?",
    re.IGNORECASE,
)

# 6. 顿号/逗号分隔的复合疾病列表中的敏感词擦除：“患'重度精神分裂症'、'2型糖尿病'” -> “患'2型糖尿病'”
_REDACT_PAIRED_PATTERN = re.compile(
    rf"((?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)\s*){_Q}(?:{_TERMS_OR}){_Q}\s*[、,，]\s*",
    re.IGNORECASE,
)

# 7. 单疾病场景泛化为“患病”：“一弟患'重度精神分裂症'” -> “一弟患病”
_REDACT_SINGLE_SUFFER_PATTERN = re.compile(
    rf"(?:患有?|确诊(?:为)?|诊断(?:为)?|患)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?",
    re.IGNORECASE,
)

# 8. 兜底匹配包含介词/动词/病史后缀的敏感短语
_REDACT_PREFIX_PATTERN = re.compile(
    rf"(?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)?\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?",
    re.IGNORECASE,
)

# 9. 消除擦除敏感词后可能残留的孤立亲属主语短语（例如“，一弟。” / “父亲。”）
_FAMILY_MEMBERS = (
    r"父亲|母亲|祖父|祖母|外公|外婆|爷爷|奶奶|伯父|叔叔|舅舅|姑姑|姨妈|大伯|大舅|大姨|二姨|小姨|"
    r"一弟|二弟|三弟|长子|次子|长女|次女|大哥|二哥|大姐|二姐|弟弟|妹妹|哥哥|姐姐|爱人|配偶|丈夫|妻子|儿子|女儿|家属|家族成员"
)
_CLEANUP_ORPHAN_SUBJECT_PATTERN = re.compile(rf"(?:^|[，,。；])\s*(?:{_FAMILY_MEMBERS})\s*([。；;])")

# 10. 孤立介词、无用连词与残余修饰清理：“曾就诊于”、“诊断为”、“及倾向” -> “”
_CLEANUP_ORPHAN_CLEANUP_PATTERN = re.compile(
    r"(?:曾?就诊于|诊断为|确诊为|检查出|提示为|及倾向|及控制症状|控制症状)\s*([。；;，,])"
)
_CLEANUP_ORPHAN_VERB_PATTERN = re.compile(
    r"(?:^|[，,。；])\s*(?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有)\s*([。；;，,])"
)
_CLEANUP_VERB_PUNCT_PATTERN = re.compile(
    r"((?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有))\s*[、,，]"
)

# 11. 标点符号与空引号清理正则
_CLEANUP_EMPTY_QUOTES_PATTERN = re.compile(r"['\"“‘]['\"”’]")
_CLEANUP_PUNCTUATION_PATTERN = re.compile(r"([，。；：,;\s])\1+")
_CLEANUP_EMPTY_CLAUSE_PATTERN = re.compile(r"([，,])\s*([。;；])")


def redact_medical_text(text: str) -> str:
    """全场景高级无痕抹平算法 (Redaction/Purge Mode).

    全面覆盖死因、专科就诊机构、完整服药剂量句法、诊断检出、家族病史等各类复杂中文医疗句法：
    1. 把“因'恶性肿瘤'去世”自然重构为“因病去世”；
    2. 将“及保护性约束倾向”整块擦除，将“曾就诊于精神卫生中心，诊断为重度精神分裂症。”整句完全消除；
    3. 将“长期服用'奥氮平片'20mg qd及'四苯嗪'控制症状。”整句完全擦除，不残留“长期20mg qd及控制症状”；
    4. 将“一弟患'重度精神分裂症'、'2型糖尿病'”中的敏感词与顿号去除，输出“一弟患'2型糖尿病'”；
    5. 将单敏感疾病场景（如“一弟患'重度精神分裂症'”）自然重构泛化为“一弟患病”；
    6. 清理残余介词、空引号与多余标点，做到语法自然流畅无痕。
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

    # 2. 优先擦除完整服药用药句法（包含剂量、用法、连词及控制症状等修饰）
    s = _REDACT_MEDICATION_FULL_PATTERN.sub("", s)

    # 3. 擦除就诊机构句法短语（如“曾就诊于精神卫生中心，”）
    s = _REDACT_HOSPITAL_PATTERN.sub("", s)

    # 4. 擦除独立诊断句法短语（如“诊断为重度精神分裂症。”）
    s = _REDACT_DIAGNOSIS_STANDALONE_PATTERN.sub("", s)

    # 5. 擦除敏感特征倾向短语（如“及保护性约束倾向”）
    s = _REDACT_FEATURE_TENDENCY_PATTERN.sub("", s)

    # 6. 复合疾病场景：仅擦除敏感疾病与紧随的顿号，保留动词与后续非敏感疾病
    s = _REDACT_PAIRED_PATTERN.sub(r"\1", s)

    # 7. 单敏感疾病场景：自然重构为泛化“患病”（如“一弟患'重度精神分裂症'” -> “一弟患病”）
    s = _REDACT_SINGLE_SUFFER_PATTERN.sub("患病", s)

    # 8. 擦除剩余“有...病史/因...”通用前缀短语
    s = _REDACT_PREFIX_PATTERN.sub("", s)

    # 9. 清理孤立残余介词、主语与标点
    s = _CLEANUP_ORPHAN_CLEANUP_PATTERN.sub(r"\1", s)
    s = _CLEANUP_VERB_PUNCT_PATTERN.sub("", s)
    s = _CLEANUP_ORPHAN_VERB_PATTERN.sub(r"\1", s)
    s = _CLEANUP_ORPHAN_SUBJECT_PATTERN.sub(r"\1", s)

    # 10. 标点与空引号格式净化自愈
    s = _CLEANUP_EMPTY_QUOTES_PATTERN.sub("", s)
    s = _CLEANUP_PUNCTUATION_PATTERN.sub(r"\1", s)
    s = _CLEANUP_EMPTY_CLAUSE_PATTERN.sub(r"\2", s)

    # 11. 去除可能产生的开局或结尾孤立标点
    s = re.sub(r"^[，,]\s*", "", s)

    return s

