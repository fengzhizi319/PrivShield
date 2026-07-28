"""算子注册表 / Operator Registry.

提供匹配算子的统一注册、查找和管理机制。
所有算子必须实现 MatcherOperator 协议签名：(value, params) -> bool。

支持两种注册方式：
1. 装饰器注册：@OperatorRegistry.register("算子名")
2. 运行时动态注册：OperatorRegistry.register_func("算子名", func)
"""

from __future__ import annotations

import threading
from typing import Any, Callable, Protocol, runtime_checkable


@runtime_checkable
class MatcherOperator(Protocol):
    """匹配算子协议。

    所有算子必须实现此签名：接收待匹配值和参数字典，返回是否命中。
    算子必须是无状态纯函数，不持有实例变量，不产生副作用。

    签名: (value: Any, params: dict[str, Any]) -> bool
    """

    def __call__(self, value: Any, params: dict[str, Any]) -> bool: ...


class OperatorRegistry:
    """算子注册表（类级单例）。

    管理所有已注册的匹配算子，支持装饰器注册和运行时动态注册。
    注册表使用类变量存储，所有实例共享同一份算子映射。

    线程安全策略：
    - 写路径（register / register_func / clear）使用 Lock 保护，
      防止并发注册或热加载时的竞态条件。
    - 读路径（get / has / list_operators）无锁，
      因 CPython GIL 已保证 dict 单次读操作的原子性，
      且读操作处于规则求值热路径，加锁会引入不必要的性能开销。
    """

    _lock = threading.Lock()
    _operators: dict[str, MatcherOperator] = {}

    @classmethod
    def register(cls, name: str) -> Callable[[MatcherOperator], MatcherOperator]:
        """算子注册装饰器。"""
        def decorator(func: MatcherOperator) -> MatcherOperator:
            with cls._lock:
                cls._operators[name] = func
            return func
        return decorator

    @classmethod
    def register_func(cls, name: str, func: MatcherOperator) -> None:
        """运行时动态注册算子（支持插件热加载）。"""
        with cls._lock:
            cls._operators[name] = func

    @classmethod
    def get(cls, name: str) -> MatcherOperator:
        """获取已注册算子（无锁读，热路径优化）。

        Raises:
            KeyError: 算子未注册。
        """
        try:
            return cls._operators[name]
        except KeyError:
            available = list(cls._operators.keys())
            raise KeyError(
                f"未找到名为 '{name}' 的匹配算子。可用算子: {available}"
            )

    @classmethod
    def has(cls, name: str) -> bool:
        """检查算子是否已注册。"""
        return name in cls._operators

    @classmethod
    def list_operators(cls) -> list[str]:
        """列出所有已注册算子名称。"""
        return list(cls._operators.keys())

    @classmethod
    def clear(cls) -> None:
        """清除所有已注册算子（主要用于测试）。"""
        with cls._lock:
            cls._operators.clear()

