"""Environment variable loader for privacy-local-agent.

Auto-detects and loads .env from project root or custom path without requiring
external dependencies (uses standard python parsing, falls back to dotenv if installed).
"""

from __future__ import annotations

import os
from pathlib import Path

_ENV_LOADED = False


def load_env_file(dotenv_path: str | Path | None = None, override: bool = False) -> bool:
    """自动加载项目根目录下的 .env 文件至 os.environ。

    Args:
        dotenv_path: .env 文件路径（默认自动从项目根目录寻找）。
        override: 是否用 .env 中的变量覆盖已有的 os.environ 环境变量。

    Returns:
        True 表示成功找到并加载了 .env，False 表示未找到或加载失败。
    """
    global _ENV_LOADED

    if _ENV_LOADED and not override and not dotenv_path:
        return True

    # 尝试寻找 .env 路径
    target_path: Path | None = None
    if dotenv_path:
        target_path = Path(dotenv_path)
    else:
        pkg_root = Path(__file__).resolve().parent
        project_root = pkg_root.parent
        possible_paths = [
            project_root / ".env",
            Path.cwd() / ".env",
        ]
        for p in possible_paths:
            if p.is_file():
                target_path = p
                break

    if not target_path or not target_path.is_file():
        return False

    # 优先使用 python-dotenv
    try:
        import dotenv

        dotenv.load_dotenv(dotenv_path=target_path, override=override)
        _ENV_LOADED = True
        return True
    except ImportError:
        pass

    # 无 python-dotenv 时，使用原生轻量解析
    try:
        with open(target_path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" not in line:
                    continue
                key, _, value = line.partition("=")
                key = key.strip()
                if key.startswith("export "):
                    key = key[7:].strip()
                value = value.strip()
                # 剥离单双引号
                if (value.startswith('"') and value.endswith('"')) or (
                    value.startswith("'") and value.endswith("'")
                ):
                    value = value[1:-1]

                if override or key not in os.environ:
                    os.environ[key] = value

        _ENV_LOADED = True
        return True
    except Exception:
        return False
