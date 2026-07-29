"""分类子系统公共工具函数 / Classification Utilities.

提供日志脱敏（redact）等零知识安全工具。
"""

from __future__ import annotations

from typing import Any


def redact(value: Any, max_len: int = 8, placeholder: str = "***") -> str:
    """对原始值进行脱敏，保留前 max_len 个字符，其余替换为占位符。

    示例：
        redact("13800138000")  → "13800138***"
        redact("张三丰")       → "张三丰"
        redact(None)          → ""

    Args:
        value: 待脱敏的原始值。
        max_len: 保留的最大明文长度（默认 8）。
        placeholder: 替换后缀的占位符（默认 "***"）。

    Returns:
        脱敏后的字符串。
    """
    if value is None:
        return ""
    text = str(value)
    if len(text) <= max_len:
        return text
    return text[:max_len] + placeholder
