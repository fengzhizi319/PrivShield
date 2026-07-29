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

# re module for regex-based pattern matching operators
import re
from typing import Any

# Import the registry to register all built-in operators via decorators
from .operator_registry import OperatorRegistry


# ===========================================================================
# 内部校验工具函数 / Internal Validation Utilities
# ===========================================================================

# Weight factors for China mainland 18-digit ID card checksum (GB 11643-1999).
# Formula: sum(digit[i] * weight[i] for i in 0..16) mod 11 -> check character.
_ID_CARD_WEIGHTS = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
# Check character mapping: remainder (0~10) -> expected check character.
_ID_CARD_CHARS = ["1", "0", "X", "9", "8", "7", "6", "5", "4", "3", "2"]

# Weight factors for Shanghai medical card 9-digit checksum (first 8 digits participate).
_SH_MEDICAL_WEIGHTS = [7, 9, 10, 5, 8, 4, 2, 1]


def _validate_id_card(value: str) -> bool:
    """校验中国大陆 18 位身份证号（GB 11643-1999）。

    Validation steps:
    1. Check length is exactly 18 characters.
    2. Validate format via regex (area code + birthdate + sequence + check digit).
    3. Compute weighted checksum of first 17 digits, mod 11, map to check character.
    4. Compare computed check character with the 18th character.
    """
    # Step 1: Length must be exactly 18 characters.
    if len(value) != 18:
        return False
    # Step 2: Regex validates structure: 6-digit area code + 8-digit birthdate (19xx/20xx)
    # + valid month (01-12) + valid day (01-31) + 3-digit sequence + check char (digit or X).
    if not re.match(
        r"^[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]$",
        value,
    ):
        return False
    try:
        # Step 3: Compute weighted sum of first 17 digits.
        # Formula: total = sum(int(value[i]) * weight[i] for i in range(17))
        total = sum(int(value[i]) * _ID_CARD_WEIGHTS[i] for i in range(17))
        # Map remainder (mod 11) to expected check character.
        expected = _ID_CARD_CHARS[total % 11]
        # Step 4: Compare with actual 18th character (case-insensitive for 'X').
        return value[17].upper() == expected
    except (ValueError, IndexError):
        # Non-numeric characters in first 17 positions or index out of range.
        return False


def _validate_medical_card(value: str) -> bool:
    """校验上海医保卡号 9 位数字校验码。

    Algorithm:
    1. Verify input is exactly 9 digits.
    2. Compute weighted sum of first 8 digits using _SH_MEDICAL_WEIGHTS.
    3. Check digit = (10 - total % 10) % 10 (mod-10 complement).
    4. Compare computed check digit with the 9th digit.
    """
    # Step 1: Must be exactly 9 numeric digits.
    if not re.match(r"^\d{9}$", value):
        return False
    # Convert each character to integer for arithmetic.
    digits = [int(c) for c in value]
    # Step 2: Weighted sum of first 8 digits.
    total = sum(digits[i] * _SH_MEDICAL_WEIGHTS[i] for i in range(8))
    # Step 3: Compute expected check digit using mod-10 complement formula.
    expected = (10 - total % 10) % 10
    # Step 4: Compare with actual 9th digit (index 8).
    return digits[8] == expected


def _normalize_icd10(code: str) -> tuple[str, int] | None:
    """解析并归一化 ICD-10 编码，支持 B20.0 与 B200 格式。

    Parses an ICD-10 code string into a canonical (letter, number) tuple
    for interval comparison. E.g. "A51.2" -> ("A", 51).

    Returns None if the input is not a recognizable ICD-10 code.
    """
    # Normalize: uppercase, strip whitespace.
    s = str(code).upper().strip() if code else ""
    # Match pattern: 1 uppercase letter + 2 digits + optional dot + 0-2 sub-digits.
    # Examples: "A50", "B20.0", "C341" all match.
    match = re.match(r"^([A-Z])(\d{2})(?:\.?\d{0,2})?$", s)
    if not match:
        return None
    # Return (category_letter, category_number) tuple for lexicographic comparison.
    return match.group(1), int(match.group(2))


def _in_icd10_interval(code: tuple[str, int], start: str, end: str) -> bool:
    """判断 ICD-10 编码是否落在闭区间内。

    Performs tuple comparison: start_norm <= code <= end_norm.
    Tuple comparison is lexicographic: first by letter, then by number.
    """
    # Normalize interval boundaries to the same tuple format.
    start_norm = _normalize_icd10(start)
    end_norm = _normalize_icd10(end)
    # If either boundary is invalid, the interval is undefined -> no match.
    if not start_norm or not end_norm:
        return False
    # Closed interval check using Python tuple comparison (letter first, then number).
    return start_norm <= code <= end_norm


def _validate_luhn(value: str, min_length: int = 13, max_length: int = 19) -> bool:
    """Luhn 算法校验（银行卡号通用）。

    Luhn algorithm (ISO/IEC 7812-1):
    1. From rightmost digit (check digit), double every second digit moving left.
    2. If doubling results in > 9, subtract 9 (equivalent to sum of digits).
    3. Sum all digits (doubled and undoubled).
    4. Valid if total mod 10 == 0.
    """
    # Strip whitespace and validate basic format.
    s = value.strip()
    # Must be all digits and within acceptable card number length range.
    if not s.isdigit() or not (min_length <= len(s) <= max_length):
        return False
    digits = [int(d) for d in s]
    # Sum of odd-position digits from right (positions 1, 3, 5, ... from right).
    odd_sum = sum(digits[-1::-2])
    # Sum of even-position digits from right (positions 2, 4, 6, ...):
    # double each, then sum digits of result via divmod(2*d, 10) = (quotient + remainder).
    even_sum = sum(sum(divmod(2 * d, 10)) for d in digits[-2::-2])
    # Valid card number if total is divisible by 10.
    return (odd_sum + even_sum) % 10 == 0


# ===========================================================================
# 注册内置算子 / Register Built-in Operators
# ===========================================================================


@OperatorRegistry.register("regex")
def regex_matcher(value: Any, params: dict[str, Any]) -> bool:
    """正则表达式匹配算子。

    Performs re.search (not fullmatch) so the pattern can match anywhere in the string.

    params:
        pattern: str - 正则表达式模式
    """
    # Guard: value must be a non-empty string to perform regex matching.
    if not isinstance(value, str) or not value:
        return False
    # Guard: pattern must be provided and non-empty.
    pattern = params.get("pattern", "")
    if not pattern:
        return False
    try:
        # Execute regex search: returns True if pattern matches anywhere in value.
        return bool(re.search(pattern, value))
    except re.error:
        # Invalid regex pattern in configuration: treat as no match (fail-safe).
        return False


@OperatorRegistry.register("keyword_contains")
def keyword_contains_matcher(value: Any, params: dict[str, Any]) -> bool:
    """关键词子串包含匹配算子。

    将输入值与关键词均归一化（小写 + 去下划线/空格）后，
    检查是否包含 keywords 列表中的任一关键词。

    Normalization ensures 'ID_Card' matches keyword 'idcard'.

    params:
        keywords: list[str] - 关键词列表
    """
    # Normalize the input value: lowercase + remove underscores and spaces.
    norm = str(value).lower().replace("_", "").replace(" ", "")
    # Retrieve keyword list from params.
    keywords = params.get("keywords", [])
    # Check if ANY normalized keyword is a substring of the normalized value.
    # Each keyword is also normalized for consistent comparison.
    return any(kw.lower().replace("_", "").replace(" ", "") in norm for kw in keywords if kw)


@OperatorRegistry.register("prefix_match")
def prefix_matcher(value: Any, params: dict[str, Any]) -> bool:
    """前缀匹配算子。

    Checks if the value starts with any of the configured prefixes.
    Useful for matching ICD codes (e.g. prefixes=["A50", "A51"]).

    params:
        prefixes: list[str] - 前缀列表
    """
    # Guard: must be a non-empty string.
    if not isinstance(value, str) or not value:
        return False
    prefixes = params.get("prefixes", [])
    # Return True if value starts with any configured prefix.
    return any(value.startswith(p) for p in prefixes)


@OperatorRegistry.register("suffix_match")
def suffix_matcher(value: Any, params: dict[str, Any]) -> bool:
    """后缀匹配算子。

    Checks if the value ends with any of the configured suffixes.

    params:
        suffixes: list[str] - 后缀列表
    """
    # Guard: must be a non-empty string.
    if not isinstance(value, str) or not value:
        return False
    suffixes = params.get("suffixes", [])
    # Return True if value ends with any configured suffix.
    return any(value.endswith(s) for s in suffixes)


@OperatorRegistry.register("id_card_checksum")
def id_card_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """中国大陆 18 位身份证校验算子（GB 11643-1999）。

    Delegates to _validate_id_card which performs full structural + checksum validation.

    params: 无额外参数
    """
    # Convert value to string (handle None) and delegate to internal validator.
    return _validate_id_card(str(value) if value else "")


@OperatorRegistry.register("medical_card_checksum")
def medical_card_checksum_matcher(value: Any, params: dict[str, Any]) -> bool:
    """上海医保卡号校验算子。

    Delegates to _validate_medical_card for 9-digit checksum verification.

    params: 无额外参数
    """
    # Convert value to string (handle None) and delegate to internal validator.
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

    Validates card numbers using the Luhn algorithm (ISO/IEC 7812-1).
    Supports configurable length bounds for different card types.

    params:
        min_length: int - 最小长度（默认 13）
        max_length: int - 最大长度（默认 19）
    """
    # Extract configurable length bounds from params with sensible defaults.
    min_len = params.get("min_length", 13)
    max_len = params.get("max_length", 19)
    # Delegate to internal Luhn validator with length constraints.
    return _validate_luhn(str(value) if value else "", min_len, max_len)


@OperatorRegistry.register("length_range")
def length_range_matcher(value: Any, params: dict[str, Any]) -> bool:
    """字符串长度范围匹配算子。

    Checks if the string representation of value falls within [min_length, max_length].
    Useful as a secondary filter (e.g. phone numbers are 11 digits).

    params:
        min_length: int - 最小长度
        max_length: int - 最大长度
    """
    # Convert to string for length measurement.
    s = str(value) if value else ""
    # Extract bounds; default min=0, max=infinity (accept any length).
    min_len = params.get("min_length", 0)
    max_len = params.get("max_length", float("inf"))
    # Closed interval check on string length.
    return min_len <= len(s) <= max_len


@OperatorRegistry.register("exact_match")
def exact_match_matcher(value: Any, params: dict[str, Any]) -> bool:
    """精确匹配算子（归一化后完全相等）。

    Normalizes both the value and allowed values (lowercase + strip underscores/spaces)
    then checks for exact equality. Useful for enum-like fields (e.g. gender='M'/'F').

    params:
        values: list[str] - 允许的值列表
    """
    # Normalize input value for comparison.
    norm = str(value).lower().replace("_", "").replace(" ", "") if value else ""
    # Retrieve the list of allowed values.
    allowed = params.get("values", [])
    # Check if normalized value matches any normalized allowed value.
    return norm in [v.lower().replace("_", "").replace(" ", "") for v in allowed]


@OperatorRegistry.register("ip_address")
def ip_address_matcher(value: Any, params: dict[str, Any]) -> bool:
    """IPv4 / IPv6 地址判定算子。

    Validates whether the value is a well-formed IPv4 or IPv6 address.
    IPv4: 4 octets (0-255) separated by dots.
    IPv6: 8 groups of 4 hex digits separated by colons (full form only).
    """
    # Guard: must be a non-empty string.
    if not isinstance(value, str) or not value:
        return False
    # IPv4 pattern: each octet is 0-255 (25[0-5] | 2[0-4]x | [01]?xx).
    ipv4_pattern = r"^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$"
    # IPv6 pattern: full 8-group notation (does not handle :: abbreviation).
    ipv6_pattern = r"^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$"
    s = value.strip()
    # Return True if either IPv4 or IPv6 pattern matches.
    return bool(re.match(ipv4_pattern, s) or re.match(ipv6_pattern, s))


@OperatorRegistry.register("mac_address")
def mac_address_matcher(value: Any, params: dict[str, Any]) -> bool:
    """MAC 地址匹配算子。

    Validates standard MAC address format: 6 groups of 2 hex digits
    separated by colons or hyphens (e.g. "AA:BB:CC:DD:EE:FF").
    """
    # Guard: must be a non-empty string.
    if not isinstance(value, str) or not value:
        return False
    # MAC pattern: 6 pairs of hex digits separated by ':' or '-'.
    mac_pattern = r"^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$"
    return bool(re.match(mac_pattern, value.strip()))


@OperatorRegistry.register("chinese_name")
def chinese_name_matcher(value: Any, params: dict[str, Any]) -> bool:
    """中文姓名匹配算子（2~4 字常见汉字姓名模式）。

    Matches strings consisting of 2-4 CJK Unified Ideographs (U+4E00~U+9FA5).
    This covers the vast majority of Chinese personal names.
    """
    # Guard: must be a non-empty string.
    if not isinstance(value, str) or not value:
        return False
    # Pattern: exactly 2-4 characters in the CJK Unified Ideographs block.
    name_pattern = r"^[\u4e00-\u9fa5]{2,4}$"
    return bool(re.match(name_pattern, value.strip()))
