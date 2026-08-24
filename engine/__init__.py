"""隐私计算本地代理（PrivShield）根包。

本包对外提供 REST/gRPC 两种接入方式，封装了差分隐私、K-匿名、
数据脱敏、查询混淆等隐私保护原语，可用于 SecretFlow 生态中的本地隐私增强场景。

Root package of the PrivShield, exposing REST/gRPC entrypoints
and privacy primitives such as DP, K-anonymity, masking and query obfuscation.
"""

import sys

# 向后兼容别名：允许既有代码通过 import PrivShield 继续访问
if "PrivShield" not in sys.modules:
    sys.modules["PrivShield"] = sys.modules[__name__]

