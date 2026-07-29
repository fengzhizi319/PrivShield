"""算子注册表 / Operator Registry.

提供匹配算子的统一注册、查找和管理机制。
所有算子必须实现 MatcherOperator 协议签名：(value, params) -> bool。

支持两种注册方式：
1. 装饰器注册：@OperatorRegistry.register("算子名")
2. 运行时动态注册：OperatorRegistry.register_func("算子名", func)
"""

from __future__ import annotations

# threading.Lock is used to protect write paths (register/clear) from race conditions
import threading
from typing import Any, Callable, Protocol, runtime_checkable


# @runtime_checkable enables isinstance() checks against this Protocol at runtime,
# allowing validation that a callable conforms to the operator signature.
@runtime_checkable
class MatcherOperator(Protocol):
    """匹配算子协议。

    所有算子必须实现此签名：接收待匹配值和参数字典，返回是否命中。
    算子必须是无状态纯函数，不持有实例变量，不产生副作用。

    签名: (value: Any, params: dict[str, Any]) -> bool
    """

    # Protocol method signature: any callable matching this shape is a valid operator.
    # The ellipsis (...) indicates this is a structural type, not an implementation.
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

    # Class-level lock: guards all write operations to _operators dict.
    # Using a class variable ensures all access shares the same synchronization primitive.
    _lock = threading.Lock()
    # Class-level operator storage: maps operator name (str) -> callable (MatcherOperator).
    # Shared across all usages since it is a class variable (singleton pattern).
    _operators: dict[str, MatcherOperator] = {}

    @classmethod
    def register(cls, name: str) -> Callable[[MatcherOperator], MatcherOperator]:
        """算子注册装饰器。

        Usage:
            @OperatorRegistry.register("regex")
            def regex_matcher(value, params): ...
        """
        # Return a decorator closure that captures the operator name.
        def decorator(func: MatcherOperator) -> MatcherOperator:
            # Acquire lock before mutating the shared _operators dict to prevent
            # race conditions when multiple threads register operators concurrently.
            with cls._lock:
                cls._operators[name] = func
            # Return the original function unmodified so it can still be called directly.
            return func
        return decorator

    @classmethod
    def register_func(cls, name: str, func: MatcherOperator) -> None:
        """运行时动态注册算子（支持插件热加载）。

        This method enables registering operators at runtime without decorators,
        useful for plugin systems or dynamically loaded modules.
        """
        # Acquire lock to safely insert into the shared dict.
        with cls._lock:
            cls._operators[name] = func

    @classmethod
    def get(cls, name: str) -> MatcherOperator:
        """获取已注册算子（无锁读，热路径优化）。

        Lock-free read is safe because:
        1. CPython GIL guarantees atomic dict reads.
        2. This method is on the hot evaluation path; locking would add overhead.

        Raises:
            KeyError: 算子未注册。
        """
        try:
            # Direct dict lookup - O(1) average time complexity.
            return cls._operators[name]
        except KeyError:
            # Build a helpful error message listing all available operators
            # to assist debugging misconfigured rule YAML files.
            available = list(cls._operators.keys())
            raise KeyError(
                f"未找到名为 '{name}' 的匹配算子。可用算子: {available}"
            )

    @classmethod
    def has(cls, name: str) -> bool:
        """检查算子是否已注册。

        Lock-free read: safe under GIL for single dict membership test.
        Used by the validator to check rule configs before engine execution.
        """
        return name in cls._operators

    @classmethod
    def list_operators(cls) -> list[str]:
        """列出所有已注册算子名称。

        Returns a snapshot list of current operator names.
        Used by management APIs and the validator's fuzzy-match suggestions.
        """
        return list(cls._operators.keys())

    @classmethod
    def clear(cls) -> None:
        """清除所有已注册算子（主要用于测试）。

        Acquires lock because this is a destructive write operation.
        Primarily used in unit tests to reset state between test cases.
        """
        with cls._lock:
            cls._operators.clear()

