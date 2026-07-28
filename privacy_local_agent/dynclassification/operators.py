"""内置匹配算子库 / Built-in Matcher Operators.

提供通用匹配算子实现，包括：
- regex: 正则表达式匹配
- keyword_contains: 关键词子串包含匹配
- prefix_match: 前缀匹配
- suffix_match: 后缀匹配
- id_card_checksum: 中国大陆身份证校验（GB 11643-1999）
- medical_card_checksum: 上海医保卡号校验
- icd10_range: ICD-10 编码区间判定
- luhn_checksum: Luhn 算法（银行卡号校验）
- length_range: 长度区间匹配
- exact_match: 准确取值匹配
- ip_address: IP 地址匹配
- mac_address: MAC 地址匹配
- chinese_name: 中文姓名匹配

所有算子均为无状态纯函数，签名: (value: Any, params: dict) -> bool。
本模块完全独立，不依赖旧分类引擎代码。
"""

from __future__ import annotations

import re
from typing import Any

from .operator_registry import OperatorRegistry


# ===========================================================================
# 内部校验工具函数 / Internal Validation Utilities
# ===========================================================================

# 中国大陆 18 位身份证号校验码权重因子（GB 11643-1999 标准）
_ID_CARD_WEIGHTS = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
# 校验码字符映射表：模 11 余数 0~10 对应的校验字符
_ID_CARD_CHARS = ["1", "0", "X", "9", "8", "7", "6", "5", "4", "3", "2"]

# 上海医保卡号 9 位数字校验权重因子（前 8 位参与计算）
_SH_MEDICAL_WEIGHTS = [7, 9, 10, 5, 8, 4, 2, 1]


def _validate_id_card(value: str) -> bool:
    """校验中国大陆 18 位身份证号（GB 11643-1999）。"""
    if len(value) != 18:
        return False
    if not re.match(
        r"^[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]$",
        value,
    ):
        return False
    try:
        total = sum(int(value[i]) * _ID_CARD_WEIGHTS[i] for i in range(17))
        expected = _ID_CARD_CHARS[total % 11]
        return value[17].upper() == expected
    except (ValueError, IndexError):
        return False


def _validate_medical_card(value: str) -> bool:
    """校验上海医保卡号 9 位数字校验码。"""
    if not re.match(r"^\d{9}$", value):
        return False
    digits = [int(c) for c in value]
    total = sum(digits[i] * _SH_MEDICAL_WEIGHTS[i] for i in range(8))
    expected = (10 - total % 10) % 10
    return digits[8] == expected


def _normalize_icd10(code: str) -> tuple[str, int] | None:
    """解析并归一化 ICD-10 编码，支持 B20.0 与 B200 格式。"""
    s = str(code).upper().strip() if code else ""
    match = re.match(r"^([A-Z])(\d{2})(?:\.?\d{0,2})?$", s)
    if not match:
        return None
    return match.group(1), int(match.group(2))


def _in_icd10_interval(code: tuple[str, int], start: str, end: str) -> bool:
    """判断 ICD-10 编码是否落在闭区间内。"""
    start_norm = _normalize_icd10(start)
    end_norm = _normalize_icd10(end)
    if not start_norm or not end_norm:
        return False
    return start_norm <= code <= end_norm


def _validate_luhn(value: str, min_length: int = 13, max_length: int = 19) -> bool:
    """Luhn 算法校验（银行卡号通用）。"""
    s = value.strip()
    if not s.isdigit() or not (min_length <= len(s) <= max_length):
        return False
    digits = [int(d) for d in s]
    odd_sum = sum(digits[-1::-2])
    even_sum = sum(sum(divmod(2 * d, 10)) for d in digits[-2::-2])
    return (odd_sum + even_sum) % 10 == 0


# ===========================================================================
# 注册内置算子 / Register Built-in Operators
# ===========================================================================


@OperatorRegistry.register("regex")
def regex_matcher(value: Any, params: dict[str, Any]) -> bool:
    """正则表达式匹配算子。

    params:
        pattern: str - 正则表达式模式
    """
    # value 不是字符串或为空？
    if not isinstance(value, str) or not value:
        return False
    # pattern 为空？
    pattern = params.get("pattern", "")
    if not pattern:
        return False
    try:
        # 执行正则搜索
        return bool(re.search(pattern, value))
    except re.error:
        return False


@OperatorRegistry.register("keyword_contains")
def keyword_contains_matcher(value: Any, params: dict[str, Any]) -> bool:
    """关键词子串包含匹配算子。

    将输入值与关键词均归一化（小写 + 去下划线/空格）后，
    检查是否包含 keywords 列表中的任一关键词。

    params:
        keywords: list[str] - 关键词列表
    """
    norm = str(value).lower().replace("_", "").replace(" ", "")
    keywords = params.get("keywords", [])
    return any(kw.lower().replace("_", "").replace(" ", "") in norm for kw in keywords if kw)


@OperatorRegistry.register("prefix_match")
def prefix_matcher(value: Any, params: dict[str, Any]) -> bool:
    """前缀匹配算子。

    params:
        prefixes: list[str] - 前缀列表
    """
    if not isinstance(value, str) or not value:
        return False
    prefixes = params.get("prefixes", [])
    return any(value.startswith(p) for p in prefixes)


@OperatorRegistry.register("suffix_match")
def suffix_matcher(value: Any, params: dict[str, Any]) -> bool:
    """后缀匹配算子。

    params:
        suffixes: list[str] - 后缀列表
    """
    if not isinstance(value, str) or not value:
        return False
    suffixes = params.get("suffixes", [])
    return any(value.endswith(s) for s in suffixes)


@OperatorRegistry.register("id_card_checksum")
def id_card_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """中国大陆 18 位身份证校验算子（GB 11643-1999）。

    params: 无额外参数
    """
    return _validate_id_card(str(value) if value else "")


@OperatorRegistry.register("medical_card_checksum")
def medical_card_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """上海医保卡号校验算子。

    params: 无额外参数
    """
    return _validate_medical_card(str(value) if value else "")


@OperatorRegistry.register("icd10_range")
def icd10_range_matcher(value: Any, params: dict[str, Any]) -> bool:
    """ICD-10 编码区间判定算子。

    判断值是否为合法 ICD-10 编码，并检查是否落在敏感区间内。
    通过 params 回写命中详情（_hit_level, _hit_category）供引擎读取。

    params:
        default_level: str - 默认等级（未命中敏感区间时）
        upgrade_level: str - 升级等级（命中敏感区间时）
        intervals: list[dict] - 敏感区间列表 [{start, end, category}]
    """
    # Step 1: Normalize the raw input into a canonical ICD-10 tuple (letter, number).
    # e.g. "A51.2" -> ("A", 51); returns None if the value is not a valid ICD-10 code.
    icd = _normalize_icd10(str(value) if value else "")
    # Early exit: not a recognizable ICD-10 code, so this operator does not apply.
    if not icd:
        return False

    # Step 2: Retrieve the list of sensitive intervals from rule params.
    # Each interval is a dict like {"start": "A50", "end": "A53", "category": "SEXUAL_DISEASE"}.
    intervals = params.get("intervals", [])
    # Step 3: Iterate over intervals to check if the code falls within any sensitive range.
    for interval in intervals:
        # Extract the closed-interval boundaries [start, end] for comparison.
        start = interval.get("start", "")
        end = interval.get("end", "")
        # _in_icd10_interval performs tuple comparison: start_norm <= icd <= end_norm.
        if _in_icd10_interval(icd, start, end):
            # Hit a sensitive interval: write back the upgraded level and matched category
            # so the downstream engine can assign a higher sensitivity grade (e.g. L4).
            params["_hit_level"] = params.get("upgrade_level", "L4")
            params["_hit_category"] = interval.get("category", "")
            return True

    # Step 4: Code is a valid ICD-10 but did NOT fall into any sensitive interval.
    # Assign the default (lower) sensitivity level and a generic medical category.
    params["_hit_level"] = params.get("default_level", "L3")
    params["_hit_category"] = "MEDICAL_ICD10_GENERAL"
    return True


@OperatorRegistry.register("luhn_checksum")
def luhn_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """Luhn 算法校验算子（银行卡号通用校验）。

    params:
        min_length: int - 最小长度（默认 13）
        max_length: int - 最大长度（默认 19）
    """
    min_len = params.get("min_length", 13)
    max_len = params.get("max_length", 19)
    return _validate_luhn(str(value) if value else "", min_len, max_len)


@OperatorRegistry.register("length_range")
def length_range_matcher(value: Any, params: dict[str, Any]) -> bool:
    """字符串长度范围匹配算子。

    params:
        min_length: int - 最小长度
        max_length: int - 最大长度
    """
    s = str(value) if value else ""
    min_len = params.get("min_length", 0)
    max_len = params.get("max_length", float("inf"))
    return min_len <= len(s) <= max_len


@OperatorRegistry.register("exact_match")
def exact_match_matcher(value: Any, params: dict[str, Any]) -> bool:
    """精确匹配算子（归一化后完全相等）。

    params:
        values: list[str] - 允许的值列表
    """
    norm = str(value).lower().replace("_", "").replace(" ", "") if value else ""
    allowed = params.get("values", [])
    return norm in [v.lower().replace("_", "").replace(" ", "") for v in allowed]


@OperatorRegistry.register("ip_address")
def ip_address_matcher(value: Any, params: dict[str, Any]) -> bool:
    """IPv4 / IPv6 地址判定算子。"""
    if not isinstance(value, str) or not value:
        return False
    ipv4_pattern = r"^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$"
    ipv6_pattern = r"^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$"
    s = value.strip()
    return bool(re.match(ipv4_pattern, s) or re.match(ipv6_pattern, s))


@OperatorRegistry.register("mac_address")
def mac_address_matcher(value: Any, params: dict[str, Any]) -> bool:
    """MAC 地址匹配算子。"""
    if not isinstance(value, str) or not value:
        return False
    mac_pattern = r"^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$"
    return bool(re.match(mac_pattern, value.strip()))


@OperatorRegistry.register("chinese_name")
def chinese_name_matcher(value: Any, params: dict[str, Any]) -> bool:
    """中文姓名匹配算子（2~4 字常见汉字姓名模式）。"""
    if not isinstance(value, str) or not value:
        return False
    name_pattern = r"^[\u4e00-\u9fa5]{2,4}$"
    return bool(re.match(name_pattern, value.strip()))
