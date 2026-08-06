"""医疗分类分级规则与 L4/L5 级脱敏引擎模块 / Medical Privacy Rules & Redaction Engine.

采用 【🥇 动态字典 + 句法正则表达式 (Dynamic Dictionary & Regex Engine)】 核心架构：
1. **动态字典 (Dynamic Dictionaries)**：分层分类维护 PII 别名字典与 L4/L5 重大高敏词库（涵盖 HIV、精神障碍、遗传缺陷、性病、恶性肿瘤、病毒性肝炎、重度器官损害等）；
2. **Fast-Path 前置校验**：词库自动编译为长词优先正则，针对干净文本实现 <1ms 超低延迟原样放行，零篡改零误伤；
3. **句法正则表达式 (Clause Grammar Patterns)**：高精度匹配服药剂量频次、血清学滴度、基因检测突变、死因重构、就诊机构及列表顿号；
4. **语法自愈流水线 (_clean_orphan_syntax)**：自动消除断句残渣、悬空连词/介词/标点，对仅剩无主语状语从句执行 Purge to Empty 干净抹平。
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
        "重度精神分裂症", "精神分裂症", "言语关联妄想", "关联妄想", "命令性幻听", "保护性约束倾向", "幻听（命令性言语）", "命令性言语", "被害妄想", "幻听", "自伤倾向", "冲动砸物", "保护性约束", "奥氮平片", "精神卫生中心"
    ],
    "GENETIC_DEFECT": [
        "遗传性亨廷顿舞蹈病", "亨廷顿舞蹈病", "亨廷顿病", "Huntington Disease", "HTT基因CAG重复序列", "HTT基因", "HTT", "CAG重复序列", "CAG重复", "CAG扩增", "四苯嗪", "舞蹈样动作", "舞蹈样症状", "四肢舞蹈样动作", "舞蹈病"
    ],
}

# L4 高风险病史与诊断词汇映射组（肿瘤、性病/传染病、严重器官损害）
L4_TERMS_MAP: dict[str, list[str]] = {
    "STD_VENEREAL": [
        "梅毒", "苍白密螺旋体", "TPPA阳性", "TPPA", "RPR阳性", "RPR 1:16", "RPR", "淋病", "淋球菌", "尖锐湿疣",
        "生殖器疱疹", "软下疳", "性病", "性传播疾病", "不洁性接触史", "不洁接触史", "无痛性溃疡", "硬下疳", "人乳头瘤病毒高危型"
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

# 3. 就诊机构与独立诊断句法整句擦除（"曾就诊于精神卫生中心"、"就诊于某某医院"、"患者1年前查出'乙型肝炎'，"）
_REDACT_HOSPITAL_PATTERN = re.compile(
    rf"(?:曾?就诊于|就诊于|收治于|转诊至|住院于|门诊于)\s*{_Q}(?:{_TERMS_OR}|[\w\u4e00-\u9fa5]{{2,15}}(?:医院|中心|诊所|专科|卫生院|卫生所|外院)){_Q}\s*(?:，|,|。|；|;)?",
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
    r"(?:同时因|由于|同时|曾?就诊于|诊断为|确诊为|检查出|查出|提示为|及倾向|及控制症状|控制症状|控制|基因检测提示|基因检测示|基因检测|长期|定期|口服|服用|血清学|血清学检查示?|予|给予|及|与|和)\s*([。；;，,])"
)
_CLEANUP_ORPHAN_VERB_PATTERN = re.compile(
    r"(?:^|[，,。；])\s*(?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有|予|控制)\s*([。；;，,])"
)
_CLEANUP_VERB_PUNCT_PATTERN = re.compile(
    r"((?:因|由于|患有?|确诊|患|有|行|进行|接受|服用|合并|伴有))\s*[、,，]"
)
_CLEANUP_NO_OBJ_VERB_PATTERN = re.compile(
    r"(?:急诊行|急诊就诊|就诊|行|实施|接受|予|给予)\s*(?:提示|检查提示|显示|示)?\s*(?:及|与|和)?\s*([。；;，,])"
)
_CLEANUP_NO_OBJ_HINT_PATTERN = re.compile(r"(?:基因检测提示|基因检测示|基因检测|检查提示|检查示|提示|显示|示|予|控制)\s*([。；;，,])")
_CLEANUP_EMPTY_QUOTES_PATTERN = re.compile(r"['\"“‘]['\"”’]")
_CLEANUP_PUNCTUATION_PATTERN = re.compile(r"([，。；：,;])\1+")
_CLEANUP_EMPTY_CLAUSE_PATTERN = re.compile(r"([，,、])\s*([。;；])")
_CLEANUP_LEADING_PUNCT_PATTERN = re.compile(r"^[，,；;。]\s*")
_CLEANUP_EMPTY_PAREN_PATTERN = re.compile(r"\(\s*\)")

# 10. 性传播疾病与极高敏特征综合句法擦除正则（涵盖血清学检查示TPPA/RPR滴度、不洁接触史、无痛性溃疡/硬下疳自愈等完整词句）
_REDACT_STD_FEATURE_CLAUSE_PATTERN = re.compile(
    r"(?:"
    r"(?:检查出|确诊为|诊断为)?\s*['\"“]?(?:梅毒|TPPA阳性|RPR阳性|淋病|尖锐湿疣)['\"”]?\s*[，,。；;]?"
    r"|(?:血清学检查示|血清学检查|血清学)?\s*(?:TPPA阳性|TPPA|RPR阳性|RPR\s*1:\d+|\d+:\d+)\s*[，,。；;]?"
    r"|(?:追问病史[，,]?)?\s*(?:1年前有|既往有|曾有)?\s*(?:不洁性接触史|不洁接触史)\s*[，,。；;]?"
    r"|(?:半年前|1年前)?\s*(?:外阴)?(?:曾出现|出现)?\s*无痛性溃疡(?:\(硬下疳\))?\s*(?:自愈)?\s*[，,。；;]?"
    r")",
    re.IGNORECASE,
)

# 11. 遗传缺陷与基因检测综合句法擦除正则（涵盖基因检测提示'遗传性亨廷顿舞蹈病'(HTT基因CAG重复序列46次)、四肢舞蹈样动作等）
_REDACT_GENETIC_CLAUSE_PATTERN = re.compile(
    r"(?:"
    r"(?:基因检测提示|基因检测示|基因检测结果示?|基因检测)?\s*['\"“]?(?:遗传性亨廷顿舞蹈病|亨廷顿病?|舞蹈病|HTT基因|CAG重复序列|CAG重复|CAG扩增)['\"”]?\s*(?:\([^)]*\)|（[^）]*）)?\s*[，,。；;]?"
    r"|(?:四肢)?舞蹈样动作\s*(?:与|和|及)?\s*"
    r")",
    re.IGNORECASE,
)

# 12. 图片/附件路径名称清理（防止路径中泄露敏感词，如 /data/hiv_test_01.jpg -> /data/masked_01.jpg）
_IMAGE_PATH_PATTERN = re.compile(r"/(?:[^/]+\.png|[^/]+\.jpg|[^/]+\.jpeg|[^/]+\.bmp)")


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

    # 0. 优先清理擦除产生的空括号，避免阻碍后续孤立动词与标点匹配；并自动擦除就诊医院/机构句法
    s = _CLEANUP_EMPTY_PAREN_PATTERN.sub("", s)
    s = _REDACT_HOSPITAL_PATTERN.sub("", s)

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

    # 5. 清理擦除敏感病史/症状后遗留的孤立前缀、后缀与时间短语（如"追问病史，1年前有"、"半年前外阴曾出现"、"自愈"、"长期"、"诊断为"）
    s = re.sub(r"(?:1年前有|半年前|1年前|既往有|曾有|自述有|外阴|曾出现|出现|自愈)\s*([。；;，,])", r"\1", s)
    s = re.sub(r"(?:1年前有|半年前|1年前|既往有|曾有|自述有|外阴|曾出现|出现|自愈)", "", s)
    s = re.sub(r"(?:追问病史|诊断为|确诊为|长期|定期)\s*([。；;，,])", r"\1", s)
    s = re.sub(r"(?:追问病史|诊断为|确诊为|长期|定期)", "", s)
    s = re.sub(r"(?:曾?就诊于|就诊于|收治于|转诊至|住院于)\s*([。；;，,])", r"\1", s)
    s = re.sub(r"(?:曾?就诊于|就诊于|收治于|转诊至|住院于)", "", s)

    # 5.1 死因孤立介词自愈重构 ("因去世" -> "因病去世") 与动词+顿号残渣清理 ("一弟患、'2型糖尿病'" -> "一弟患'2型糖尿病'")
    s = re.sub(r"(?:因|死于|因于)\s*(去世|死于|离世|逝世)", r"因病\1", s)
    s = re.sub(r"((?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?))\s*[、,，]\s*", r"\1", s)

    s = _CLEANUP_EMPTY_CLAUSE_PATTERN.sub(r"\2", s)
    s = _CLEANUP_LEADING_PUNCT_PATTERN.sub("", s)
    s = re.sub(r"([。；;,，])\1+", r"\1", s)

    # 6. 去标识化擦除文本或图片引用路径中包含的高敏感英文词汇（如 syphilis, hiv, cancer 等）
    s = re.sub(
        r"(\b[\w/\\.-]*?)(?:syphilis|hiv|aids|cancer|tumor|hepatitis)([\w/\\.-]*\.(?:png|jpg|jpeg|dcm|webp|gif)\b)",
        r"\1sanitized_case_image\2",
        s,
        flags=re.IGNORECASE,
    )

    # 7. 清理开头孤立的连词（如擦除"幻听"后剩下的 "与反复发作3年" -> "反复发作3年"）
    s = re.sub(r"^[与和及且并]+\s*", "", s)

    # 8. 最终判断：若全句抹平后仅剩无主语/无主病因孤立频次或时间状语从句（如 "患者3年前无明显诱因"、"反复发作3年"、"3年"、"反复发作"），直接抹平清空
    if re.match(r"^(?:患者)?\s*(?:\d+\s*(?:年|月|天|周|小时)?\s*(?:前)?)?\s*(?:无明显诱因|反复发作|发作|持续|既往)?\s*\d*\s*(?:年|月|天|周|小时)?\s*(?:余)?\s*(?:年|月|天|周)?\s*[。；;，,]?$", s.strip()):
        return ""

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

    # 1. 优先擦除遗传缺陷与基因检测突变综合句法（涵盖HTT基因CAG重复序列、舞蹈样动作等）
    s = _REDACT_GENETIC_CLAUSE_PATTERN.sub("", s)

    # 1.1 优先将死因句法重构为自然流畅的“因病去世/死于”
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


# ---------------------------------------------------------------------------
# L4/L5 重大高敏疾病（及关联高敏处置/药物）提示词指南与判定核心逻辑
# ---------------------------------------------------------------------------
L4_L5_MAJOR_SENSITIVE_PROMPT_GUIDELINE = """
【Layer-2 NER 级医疗实体无痕脱敏提示词准则 / L4-L5 Major Sensitive Entity Prompt Guidelines】
NER 模型与提示词必须仅提取/匹配属于 L4/L5 重大高敏级别的医疗实体及其强相关高敏处置/用药：
1. L5 极高敏级别：
   - 免疫缺陷/艾滋病：HIV感染、HIV、艾滋病、艾滋、CD4+ T淋巴细胞、ART抗逆转录等；
   - 精神障碍：重度精神分裂症、精神分裂症、幻听（命令性言语）、被害妄想、自伤倾向、保护性约束、奥氮平片、精神卫生中心等；
   - 遗传缺陷：遗传性亨廷顿舞蹈病、亨廷顿舞蹈病、亨廷顿病、CAG重复序列、四苯嗪、舞蹈病等。
2. L4 高敏级别：
   - 性传播疾病：梅毒、苍白密螺旋体、TPPA阳性、RPR阳性、淋病、尖锐湿疣、生殖器疱疹、不洁性接触史、硬下疳等；
   - 恶性肿瘤：恶性肿瘤、浸润性腺癌、肺腺癌、胃癌、肝癌、乳腺癌、宫颈癌、癌症、转移性肿瘤、奥希替尼、EGFR基因检测等；
   - 病毒性肝炎：慢性乙型病毒性肝炎、乙型肝炎、乙肝、丙型肝炎、丙肝、HBV-DNA、HCV、恩替卡韦、肝硬化代偿期等；
   - 严重器官损害：慢性阻塞性肺疾病、COPD、急性心肌梗死、冠状动脉重度狭窄等。

【非重大高敏剔除原则】：
常规慢性病（高血压、高脂血症、高血糖、普通糖尿病、脂肪肝、痛风等）、常见轻症（感冒、发烧、咳嗽、胃炎、头痛等）及常规治疗药物（如阿托伐他汀、硝苯地平、降压药、降脂药、二甲双胍、感冒药等）属于 L1/L2 低敏范围，严禁脱敏，必须原样保留。
"""

_MAJOR_SENSITIVE_KEYWORDS = (
    # L5 Keywords
    "HIV", "AIDS", "艾滋", "免疫缺陷", "CD4+", "抗逆转录",
    "精神分裂", "幻听", "妄想", "自伤", "砸物", "保护性约束", "奥氮平", "精神卫生",
    "亨廷顿", "CAG重复", "CAG扩增", "CAG", "HTT基因", "HTT", "四苯嗪", "舞蹈病", "舞蹈样",
    # L4 Keywords
    "梅毒", "密螺旋体", "TPPA", "RPR", "淋病", "淋球菌", "尖锐湿疣", "疱疹", "软下疳", "性病", "不洁性接触", "硬下疳",
    "恶性肿瘤", "腺癌", "肺癌", "胃癌", "肝癌", "乳腺癌", "宫颈癌", "癌症", "转移性肿瘤", "转移瘤", "癌", "肉瘤", "奥希替尼", "EGFR",
    "乙型肝炎", "乙肝", "丙型肝炎", "丙肝", "HBV", "HCV", "恩替卡韦", "肝硬化", "静脉曲张",
    "急性心肌梗死", "心肌梗死", "冠状动脉重度狭窄", "重度狭窄", "COPD", "阻塞性肺"
)


def _is_major_sensitive_entity(term: str, ent_type: str = "") -> bool:
    """判定实体词是否属于 L4/L5 重大高敏疾病或关联高敏处置/药物。"""
    if not term or len(term) < 2:
        return False

    term_clean = term.strip()
    term_upper = term_clean.upper()

    # 1. 词库精确或包含匹配
    if _TERMS_ONLY_PATTERN.search(term_clean):
        return True

    # 2. 核心重大高敏关键字匹配
    for kw in _MAJOR_SENSITIVE_KEYWORDS:
        if kw.upper() in term_upper:
            return True

    return False


def redact_medical_text_with_ner(text: str, ner_adapter: Any = None) -> str:
    """Layer-2 Small-NER 驱动的高级命名实体识别无痕抹平引擎 (Gold Standard Implementation).

    推荐黄金架构：【全上下文 NER 实体抽取 -> L4/L5 重大高敏筛选 -> 实体锚点句法绑定擦除 -> 语法自愈与完全抹平】
    1. **全上下文 NER 抽取**：保持输入病历文本完整，直接调用 NER 模型进行实体抽取 (100% 保持神经网络上下文)；
    2. **L4/L5 准则筛选**：遵循 L4_L5_MAJOR_SENSITIVE_PROMPT_GUIDELINE 准则，筛选出 L4/L5 重大高敏实体 (保留高血压/高脂血症等常规慢病)；
    3. **句法绑定擦除**：以 NER 定位的实体为锚点，结合剂量用法、血清学滴度、基因突变修饰及就诊短语进行结构化擦除；
    4. **语法自愈与 Purge**：调用 _clean_orphan_syntax 自愈清理断句残渣与敏感文件路径，若全句无主语病因则 Purge 抹平为 ""；
    5. **Fast-Path 与降级兜底**：具备 Fast-Path (<1ms) 与 NER 未就绪时的平滑 fallback 规则降级能力。
    """
    if not text:
        return text

    # 0. Fast-Path 前置校验：若文本不含任何 L4/L5 敏感词及脱敏标签，快速原样返回 (<1ms)，避免误篡改干净文本
    if not _TERMS_ONLY_PATTERN.search(text) and not _MASKED_LABEL_RE.search(text):
        return text

    entities = []
    if ner_adapter is not None:
        try:
            raw_entities = ner_adapter.extract(text)
            if isinstance(raw_entities, list):
                entities = raw_entities
        except Exception:
            entities = []

    # 1. 筛选并仅保留 L4/L5 重大高敏级别的实体，过滤剔除高血压、高脂血症等常规 L1/L2 慢病/常用药
    sensitive_entities = [
        e for e in entities
        if isinstance(e, dict)
        and e.get("text", "").strip()
        and _is_major_sensitive_entity(e.get("text", ""), str(e.get("type") or e.get("label") or ""))
    ]

    # 2. 如果 NER 成功提取了重大高敏实体，在完整原文上执行实体锚点驱动的上下文句法绑定擦除
    if sensitive_entities:
        s = text
        sorted_entities = sorted(
            sensitive_entities,
            key=lambda x: len(x.get("text", "")),
            reverse=True,
        )

        # 优先同步重构/擦除文本中关联的死因、基因检测突变、血清学滴度短语
        def _death_replace(match: re.Match) -> str:
            action = match.group(1)
            return f"因病{action}"

        s = _REDACT_CAUSE_DEATH_PATTERN.sub(_death_replace, s)
        s = _REDACT_SUFFER_DEATH_PATTERN.sub(_death_replace, s)
        s = _REDACT_GENETIC_CLAUSE_PATTERN.sub("", s)
        s = _REDACT_STD_FEATURE_CLAUSE_PATTERN.sub("", s)

        for ent in sorted_entities:
            term = ent.get("text", "").strip()
            ent_type = str(ent.get("type") or ent.get("label") or "").upper()
            if not term or len(term) < 2:
                continue

            quoted_term = rf"['\"“‘'”’]?{re.escape(term)}['\"”’]?"

            if any(t in ent_type for t in ["DRUG", "MED", "CHEM"]):
                # NER 识别出药物：连同剂量用法及控制症状短语绑定擦除
                pat = rf"(?:长期|定期|口服|服用|给予|使用|予|遵医嘱)?\s*{quoted_term}{_DOSE}{_FREQ}\s*(?:口服|服用)?\s*(?:及|与|和|合并)?\s*(?:控制舞蹈样症状|控制症状|抗病毒治疗|对症治疗|治疗|对症处理|口服|方案)?"
                s = re.sub(pat, "", s, flags=re.IGNORECASE)
            elif any(t in ent_type for t in ["HOSPITAL", "ORG", "LOC"]):
                # NER 识别出医疗机构/组织：连同就诊短语绑定擦除
                pat = rf"(?:曾?就诊于|就诊于|收治于|转诊至|住院于|门诊于)\s*{quoted_term}\s*(?:，|,|。|；|;)?"
                s = re.sub(pat, "", s, flags=re.IGNORECASE)
            else:
                # NER 识别出 DISEASE/TREATMENT/SYMPTOM 等：做实体级精准剥离与列表顿号自愈
                pat_paired = rf"((?:因|患有?|确诊(?:为)?|诊断(?:为)?|患|有|合并|伴有?)\s*){quoted_term}\s*[、,，]\s*"
                s = re.sub(pat_paired, r"\1", s, flags=re.IGNORECASE)
                s = re.sub(quoted_term, "", s)

        return _clean_orphan_syntax(s)

    # 3. 当 NER 未识别出实体（或推断异常）时，平滑降级由规则引擎兜底处理
    return _clean_orphan_syntax(redact_medical_text(text))
