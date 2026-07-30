"""标准文档 YAML 生成器转接模块 / Standard Document Generator Alias Module.

向后兼容提示 / Backward-compatibility Note:
原 `generator.py` 已重命名为语义更明确的 `standard_doc_generator.py`。
本文件仅保留 `StandardDocParser` 导出和 CLI 转接，建议新开发代码直接使用 `standard_doc_generator`。
"""

from __future__ import annotations

from .standard_doc_generator import StandardDocParser, main

__all__ = ["StandardDocParser", "main"]

if __name__ == "__main__":
    main()
