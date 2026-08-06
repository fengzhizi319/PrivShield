"""医疗分类分级规则与 L4/L5 级词库模块 / Medical Classification Rules & L4/L5 Terminology Dictionary.
"""

from __future__ import annotations

import re
from typing import Any

# PII 身份隐私字段及其默认脱敏规则定义
PII_FIELD_RULES: dict[str, str] = {
    "name": "CHINESE_NAME",
    "id_card_no": "ID_CARD",
    "registered_address": "ADDRESS",
    "disability_cert_no": "DISABILITY_CERT",
    "medical_insurance_no": "INSURANCE_NO",
}

# 中文数据源常用字段名到规范字段名的映射。保留规范英文键作为唯一规则来源，
# 避免把别名数量误计入 PII 类型统计，同时让分类和脱敏使用完全一致的语义。
PII_FIELD_ALIASES: dict[str, str] = {
    "姓名": "name",
    "真实姓名": "name",
    "用户姓名": "name",
    "身份证": "id_card_no",
    "身份证号": "id_card_no",
    "居民身份证": "id_card_no",
    "公民身份号码": "id_card_no",
    "地址": "registered_address",
    "注册地址": "registered_address",
    "登记地址": "registered_address",
    "户籍地址": "registered_address",
    "居住地址": "registered_address",
    "居民住址": "registered_address",
    "家庭住址": "registered_address",
    "联系地址": "registered_address",
    "残疾证号": "disability_cert_no",
    "残疾人证号": "disability_cert_no",
    "医保卡号": "medical_insurance_no",
    "医保号": "medical_insurance_no",
    "医疗保险号": "medical_insurance_no",
}


def canonicalize_pii_field(field_name: str) -> str:
    """将中文或英文 PII 字段名转换为医疗 Pipeline 的规范字段名。"""
    return PII_FIELD_ALIASES.get(field_name, field_name)

# L5 极高风险病史与诊断词汇映射组（包含疾病名、缩写、临床特征）
L5_TERMS_MAP: dict[str, list[str]] = {
    "HIV_AIDS": [
        "获得性免疫缺陷综合征", "HIV感染", "HIV", "艾滋病", "艾滋", "CD4+ T淋巴细胞", "替诺福韦+拉米夫定+多替拉韦", "ART抗逆转录", "血清HIV-1"
    ],
    "PSYCHIATRIC_DISORDER": [
        "重度精神分裂症", "精神分裂症", "幻听（命令性言语）", "命令性言语", "被害妄想", "自伤倾向", "冲动砸物", "保护性约束", "奥氮平片", "精神卫生中心"
    ],
    "GENETIC_DEFECT": [
        "遗传性亨廷顿舞蹈病", "亨廷顿舞蹈病", "亨廷顿病", "Huntington Disease", "CAG重复序列", "CAG扩增", "四苯嗪", "舞蹈样动作", "舞蹈样症状", "四肢舞蹈样动作", "舞蹈病"
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
    "STD_VENEREAL": "INFECTIOUS_DISEASE",
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

# 超长文本保护阈值：超过此长度时降级为简单词库擦除，
# 防止复杂句法正则（含嵌套量词）在恶意/超长输入下触发灾难性回溯（ReDoS）
_REDACT_MAX_TEXT_LENGTH = 50_000

# 降级路径与 Fast-path 专用正则：词库级检测与擦除
_TERMS_ONLY_PATTERN = re.compile(
    "|".join([re.escape(t) for t in _ALL_L4_L5_TERMS]),
    re.IGNORECASE,
)

_MASKED_LABEL_PATTERN = r"\[(?:L4|L5)-[A-Z_]+-SENSITIVE-MASKED\]"
_MASKED_LABEL_RE = re.compile(_MASKED_LABEL_PATTERN)

_TERMS_OR = "|".join([_MASKED_LABEL_PATTERN] + [re.escape(t) for t in _ALL_L4_L5_TERMS])
_Q = r"['\"“‘'”’]?"
_DOSE = r"(?:\s*\d+(?:\.\d+)?\s*(?:mg|g|ml|u|ug|片|粒|支|%))?"
_FREQ = r"(?:\s*(?:qd|bid|tid|qid|qn|qw|im|iv|po))?"

# 1. 死因相关句法短语重构：“因/由于/死于 'L4/L5词' (去世|死于|离世|逝世...)” -> “因病去世”
# 支持“因'HIV'导致的并发症去世”、“由于'恶性肿瘤'不幸身亡”等完整句法重构
_REDACT_DEATH_ACTION = r"(?:去世|死于|离世|殁于|不幸身亡|宣告不治|逝世)"
_REDACT_CAUSE_DEATH_PATTERN = re.compile(
    rf"(?:因|由于|死于|殁于|因为|由)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:导致的并发症|引起的并发症|导致|引起)?\s*({_REDACT_DEATH_ACTION})",
    re.IGNORECASE,
)
_REDACT_SUFFER_DEATH_PATTERN = re.compile(
    rf"(?:患有?|确诊(?:为)?|诊断(?:为)?|患)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*({_REDACT_DEATH_ACTION})",
    re.IGNORECASE,
)

# 2. 完整服药/用药与处置句法擦除
# 要求：前缀(服用/口服/给予...)、剂量用法(20mg qd)或后缀(控制症状/方案)至少有其一存在，避免无修饰裸词抢先匹配
_MED_PREFIX = r"(?:长期|定期|口服|服用|给予|使用|行|实施|接受|予|给予口服|开具|遵医嘱)"
_MED_SUFFIX = r"(?:控制舞蹈样症状|控制症状|抗病毒治疗|对症治疗|治疗|对症处理|口服|方案)"
_MED_DOSE_FREQ_NONEMPTY = r"(?:\s*\d+(?:\.\d+)?\s*(?:mg|g|ml|u|ug|片|粒|支|%))(?:\s*(?:qd|bid|tid|qid|qn|qw|im|iv|po))?|\s*(?:qd|bid|tid|qid|qn|qw|im|iv|po)"

_REDACT_MEDICATION_FULL_PATTERN = re.compile(
    rf"(?:{_MED_PREFIX}\s*(?:病理提示|提示|行)?\s*{_Q}(?:{_TERMS_OR}){_Q}{_DOSE}{_FREQ}\s*(?:口服|服用)?\s*(?:及|与|和|合并)?\s*(?:{_Q}(?:{_TERMS_OR}){_Q}{_DOSE}{_FREQ})*\s*(?:{_MED_SUFFIX})?|"
    rf"{_Q}(?:{_TERMS_OR}){_Q}{_DOSE}{_FREQ}\s*(?:口服|服用)?\s*(?:及|与|和|合并)?\s*(?:{_Q}(?:{_TERMS_OR}){_Q}{_DOSE}{_FREQ})*\s*{_MED_SUFFIX}|"
    rf"{_Q}(?:{_TERMS_OR}){_Q}{_MED_DOSE_FREQ_NONEMPTY})",
    re.IGNORECASE,
)

# 3. 就诊机构与独立诊断句法整句擦除（"曾就诊于精神卫生中心"、"患者1年前查出'乙型肝炎'，"）
_REDACT_HOSPITAL_PATTERN = re.compile(
    rf"(?:曾?就诊于|就诊于|收治于|转诊至)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:，|,|。|；|;)?",
    re.IGNORECASE,
)
_REDACT_DIAGNOSIS_STANDALONE_PATTERN = re.compile(
    rf"(?:患者\s*\d+\s*(?:年|月|天)?前|既往)?\s*(?:诊断为|确诊为|检查出|查出|发现|提示为|考虑为)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:，|,|。|；|;)?",
    re.IGNORECASE,
)

# 4. 连词+敏感特征+倾向/表现整块擦除：“及保护性约束倾向” -> “”
# 要求：前缀(及|与|和|伴|伴有) 或 后缀(倾向|表现) 至少存在其一，避免裸词抢先匹配架空后续列表/亲属重构规则
_REDACT_FEATURE_TENDENCY_PATTERN = re.compile(
    rf"(?:(?:及|与|和|伴|伴有)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:倾向|表现)?|{_Q}(?:{_TERMS_OR}){_Q}\s*(?:倾向|表现))",
    re.IGNORECASE,
)

# 5. 顿号/逗号分隔的复合疾病列表中的敏感词擦除："患'重度精神分裂症'、'2型糖尿病'" -> "患'2型糖尿病'"
_REDACT_PAIRED_PATTERN = re.compile(
    rf"((?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)\s*){_Q}(?:{_TERMS_OR}){_Q}\s*[、,，]\s*",
    re.IGNORECASE,
)
# 5.1 补充：处理敏感词在列表非首位的场景（"患'2型糖尿病'、'重度精神分裂症'" -> "患'2型糖尿病'"）
_REDACT_PAIRED_SUFFIX_PATTERN = re.compile(
    rf"[、,，]\s*{_Q}(?:{_TERMS_OR}){_Q}",
    re.IGNORECASE,
)

# 6. 亲属单疾病场景整句重构为"患病"："母亲患'乙型肝炎'。" -> "母亲患病。"
_REDACT_SINGLE_SUFFER_PATTERN = re.compile(
    rf"(?:患有?|确诊(?:为)?|诊断(?:为)?|患)\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?",
    re.IGNORECASE,
)

# 7. 既往史/病史带前缀与后缀完全擦除：“慢性乙型肝炎病史4年” -> “病史4年” (消除残留的“慢性”/“慢史”)
_REDACT_HISTORY_PATTERN = re.compile(
    rf"(?:既往|慢性)?\s*{_Q}(?:{_TERMS_OR}){_Q}\s*(?:病史|史)?",
    re.IGNORECASE,
)

# 8. 亲属关系词定义与孤立主语/动词清理
_FAMILY_MEMBERS = (
    r"父亲|母亲|祖父|祖母|外公|外婆|爷爷|奶奶|伯父|叔叔|舅舅|姑姑|姨妈|大伯|大舅|大姨|二姨|小姨|"
    r"一弟|二弟|三弟|长子|次子|长女|次女|大哥|二哥|大姐|二姐|弟弟|妹妹|哥哥|姐姐|爱人|配偶|丈夫|妻子|儿子|女儿|家属|家族成员"
)
_CLEANUP_ORPHAN_SUBJECT_PATTERN = re.compile(rf"(?:^|[，,。；])\s*(?:{_FAMILY_MEMBERS})\s*([。；;])")
_CLEANUP_FAMILY_VERB_HEAL_PATTERN = re.compile(
    rf"({_FAMILY_MEMBERS})\s*(?:患有?|确诊(?:为)?|诊断(?:为)?|患|有)\s*([。；;，,])"
)

# 9. 模块级预编译规范清理正则
_CLEANUP_DEVELOP_AND_PATTERN = re.compile(r"发展为\s*与")
_CLEANUP_PATIENT_TIME_PREFIX_PATTERN = re.compile(r"(?:患者\s*\d+\s*(?:年|月|天)?前)\s*([，,])")
_CLEANUP_ORPHAN_PREP_PATTERN = re.compile(
    r"(?:同时因|由于|同时|曾?就诊于|诊断为|确诊为|检查出|查出|提示为|及倾向|及控制症状|控制症状|长期|定期|口服|服用|及|与|和)\s*([。；;，,])"
)
_CLEANUP_ORPHAN_VERB_PATTERN = re.compile(
    r"(?:^|[，,。；])\s*(?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有)\s*([。；;，,])"
)
_CLEANUP_VERB_PUNCT_PATTERN = re.compile(
    r"((?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有))\s*[、,，]"
)
_CLEANUP_NO_OBJ_VERB_PATTERN = re.compile(
    r"(?:急诊行|急诊就诊|就诊|行|实施|接受|予|给予)\s*(?:提示|检查提示|显示|示)?\s*(?:及|与|和)?\s*([。；;，,])"
)
_CLEANUP_NO_OBJ_HINT_PATTERN = re.compile(r"(?:提示|显示|检查提示|检查示|示|予)\s*([。；;，,])")
_CLEANUP_EMPTY_QUOTES_PATTERN = re.compile(r"['\"“‘]['\"”’]")
_CLEANUP_PUNCTUATION_PATTERN = re.compile(r"([，。；：,;])\1+")
_CLEANUP_EMPTY_CLAUSE_PATTERN = re.compile(r"([，,、])\s*([。;；])")
_CLEANUP_LEADING_PUNCT_PATTERN = re.compile(r"^[，,；;。]\s*")
_CLEANUP_EMPTY_PAREN_PATTERN = re.compile(r"\(\s*\)")


def _redact_terms_only(text: str) -> str:
    """超长文本降级路径：仅做词库级擦除（无句法重构），保证性能与安全。

    当输入文本超过 _REDACT_MAX_TEXT_LENGTH 时调用，避免复杂句法正则的 ReDoS 风险。
    擦除后可能残留句法碎片（如"患有去世"），由上层 pipeline 的最终门禁兜底。
    """
    return _TERMS_ONLY_PATTERN.sub("", text)


def _clean_orphan_syntax(s: str) -> str:
    """清理擦除敏感实体后残存的孤立介词、连词、无宾语动词与多余标点。"""
    if not s:
        return s

    # 1. 清理孤立无宾语动词：如“示。”、“提示。”、“急诊行提示”、“予行”、“予行及”、“予。”
    s = _CLEANUP_NO_OBJ_VERB_PATTERN.sub(r"\1", s)
    s = _CLEANUP_NO_OBJ_HINT_PATTERN.sub(r"\1", s)

    # 2. 清理孤立连词与介词碎片：如“伴及。”、“及。”、“与。”、“伴。”、“长期。”、“发展为。”
    s = _CLEANUP_ORPHAN_PREP_PATTERN.sub(r"\1", s)
    s = re.sub(r"(?:出现|发展为|表现为)\s*([。；;，,])", r"\1", s)
    s = _CLEANUP_DEVELOP_AND_PATTERN.sub("发展为", s)

    # 3. 标点与空括号自愈
    s = re.sub(r"(['\"“‘'”’])\1+", "", s)
    s = _CLEANUP_PUNCTUATION_PATTERN.sub(r"\1", s)
    s = _CLEANUP_EMPTY_CLAUSE_PATTERN.sub(r"\2", s)
    s = _CLEANUP_LEADING_PUNCT_PATTERN.sub("", s)
    s = _CLEANUP_EMPTY_PAREN_PATTERN.sub("", s)
    s = re.sub(r"([。；;,，])\1+", r"\1", s)
    return s.strip()


def redact_medical_text(text: str) -> str:
    """全场景高级无痕抹平算法 (Redaction/Purge Mode).

    全面覆盖死因、专科就诊机构、完整服药剂量句法、诊断检出、家族病史等各类复杂中文医疗句法：
    1. 把“因'恶性肿瘤'去世”自然重构为“因病去世”；
    2. 将“及保护性约束倾向”整块擦除，将“曾就诊于精神卫生中心，诊断为重度精神分裂症。”整句完全消除；
    3. 将“长期服用'奥氮平片'20mg qd及'四苯嗪'控制症状。”整句完全擦除，不残留“长期20mg qd及控制症状”；
    4. 将“一弟患'重度精神分裂症'、'2型糖尿病'”中的敏感词与顿号去除，输出“一弟患'2型糖尿病'”；
    5. 将单敏感疾病场景（如“一弟患'重度精神分裂症'”）自然重构泛化为“一弟患病”；
    6. 消除“慢性乙型肝炎病史”中的“慢性”/“慢史”拼凑残渣，将整词干净擦除；
    7. 清理“查出，”残留的动词与多余标点，做到语法自然流畅无痕。
    """
    if not text:
        return text

    # 超长文本保护：降级为简单词库擦除，防止复杂句法正则的 ReDoS 风险
    if len(text) > _REDACT_MAX_TEXT_LENGTH:
        return _redact_terms_only(text)

    # Fast-path 检查：如果不含任何 L4/L5 敏感词及脱敏标签，直接原样返回，避免自愈逻辑篡改干净文本
    if not _TERMS_ONLY_PATTERN.search(text) and not _MASKED_LABEL_RE.search(text):
        return text

    s = text

    # 1. 优先将死因句法重构为自然流畅的“因病去世/死于”
    def _death_replace(match: re.Match) -> str:
        action = match.group(1)
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
    # 6.1 补充：擦除列表非首位的敏感疾病（"患'2型糖尿病'、'重度精神分裂症'" -> "患'2型糖尿病'"）
    s = _REDACT_PAIRED_SUFFIX_PATTERN.sub("", s)

    # 7. 单敏感疾病场景：自然重构为泛化“患病”（如“一弟患'重度精神分裂症'” -> “一弟患病”）
    s = _REDACT_SINGLE_SUFFER_PATTERN.sub("患病", s)

    # 8. 擦除既往史/病史带前缀与后缀的完整词组（防止留“慢史”）
    s = _REDACT_HISTORY_PATTERN.sub("", s)

    # 9. 清理孤立残余介词、连词与标点
    s = _CLEANUP_DEVELOP_AND_PATTERN.sub("发展为", s)
    s = _CLEANUP_ORPHAN_PREP_PATTERN.sub(r"\1", s)
    s = _CLEANUP_VERB_PUNCT_PATTERN.sub("", s)

    # 9.1 孤立时间前缀自愈：将"患者1年前，"残留清除，直接输出"白细胞计数值正常。"
    s = _CLEANUP_PATIENT_TIME_PREFIX_PATTERN.sub("", s)

    # 9.2 亲属孤立动词与缺失动词自愈："一弟'重度精神分裂症'" -> "一弟患'重度精神分裂症'"，"一弟患。" -> "一弟患病。"
    s = _CLEANUP_FAMILY_VERB_HEAL_PATTERN.sub(r"\1患病\2", s)

    s = _CLEANUP_ORPHAN_VERB_PATTERN.sub(r"\1", s)
    s = _CLEANUP_ORPHAN_SUBJECT_PATTERN.sub(r"\1", s)

    # 10. 标点与空引号格式净化自愈
    s = _CLEANUP_EMPTY_QUOTES_PATTERN.sub("", s)
    s = _CLEANUP_PUNCTUATION_PATTERN.sub(r"\1", s)
    s = _CLEANUP_EMPTY_CLAUSE_PATTERN.sub(r"\2", s)

    # 11. 统一调起通用语法清理与标点自愈
    return _clean_orphan_syntax(s)


def redact_medical_text_with_ner(text: str, ner_adapter: Any = None) -> str:
    """Layer-2 Small-NER 驱动的高级命名实体识别无痕抹平引擎.

    借助 Layer-2 Small-NER (ONNXRuntime / ModelScope / TensorRT) 对文本中的
    DISEASE(疾病), DRUG(药物), TREATMENT(诊疗处置), HOSPITAL(医疗机构), SYMPTOM(症状)
    等实体进行上下文识别与精确定位抹平：
    1. 若提供了 ner_adapter 且成功识别出实体，基于实体在原文的上下文绑定（如剂量、用法、曾就诊于）进行擦除；
    2. 兼容智能语法自愈自检，确保语句自洽流畅；
    3. 支持在 NER 模型未就绪时自动平滑 fallback 降级至 redact_medical_text 规则引擎。
    """
    if not text:
        return text

    entities = []
    if ner_adapter is not None:
        try:
            raw_entities = ner_adapter.extract(text)
            if isinstance(raw_entities, list):
                entities = raw_entities
        except Exception:
            entities = []

    # 如果 NER 成功提取了实体项，执行纯 NER 神经网络驱动的实体级精准擦除 (不落入规则引擎)
    if entities:
        s = text
        sorted_entities = sorted(
            [
                e for e in entities
                if isinstance(e, dict) and e.get("text", "").strip()
            ],
            key=lambda x: len(x.get("text", "")),
            reverse=True,
        )

        for ent in sorted_entities:
            term = ent.get("text", "").strip()
            # 兼容两种实体 schema：ner_adapter 返回 "label"，其他引擎可能返回 "type"
            ent_type = str(ent.get("type") or ent.get("label") or "").upper()
            if not term or len(term) < 2:
                continue

            quoted_term = rf"['\"“‘'”’]?{re.escape(term)}['\"”’]?"

            if any(t in ent_type for t in ["DRUG", "MED", "CHEM"]):
                # NER 识别出药物/化学品：连同剂量用法及控制症状擦除
                pat = rf"(?:长期|定期|口服|服用|给予|使用|予|遵医嘱)?\s*{quoted_term}{_DOSE}{_FREQ}\s*(?:口服|服用)?\s*(?:及|与|和|合并)?\s*(?:控制舞蹈样症状|控制症状|抗病毒治疗|对症治疗|治疗|对症处理|口服|方案)?"
                s = re.sub(pat, "", s, flags=re.IGNORECASE)
            elif any(t in ent_type for t in ["HOSPITAL", "ORG", "LOC"]):
                # NER 识别出医疗机构/组织：擦除就诊短语
                pat = rf"(?:曾?就诊于|就诊于|收治于|转诊至)\s*{quoted_term}\s*(?:，|,|。|；|;)?"
                s = re.sub(pat, "", s, flags=re.IGNORECASE)
            else:
                # NER 识别出 DISEASE/TREATMENT/SYMPTOM 等：做实体级剥离
                pat_paired = rf"((?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)\s*){quoted_term}\s*[、,，]\s*"
                s = re.sub(pat_paired, r"\1", s, flags=re.IGNORECASE)
                s = re.sub(quoted_term, "", s)

        return _clean_orphan_syntax(s)

    # 仅当 NER 未识别出任何实体时，平滑降级至规则引擎
    return _clean_orphan_syntax(redact_medical_text(text))
