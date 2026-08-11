"""mTLS CN whitelist management with hot-reload support.

提供基于 YAML 配置文件的 mTLS 客户端证书 CN 白名单管理，支持：
- 每个 CN 独立 scope 控制（最小权限原则）
- 基于文件 mtime 的热重载（请求驱动，被动检查）
- 线程安全的两阶段提交
- 向后兼容环境变量静态列表

Provides YAML-based mTLS client certificate CN whitelist management with:
- Per-CN scope control (least privilege principle)
- Hot-reload via file mtime detection (request-driven, passive check)
- Thread-safe two-phase commit
- Backward compatibility with environment variable static list
"""

from __future__ import annotations

import logging
import threading
import time
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class CNEntry(BaseModel):
    """A single CN whitelist entry with metadata.

    Attributes:
        cn: Common Name from the client certificate subject.
        scopes: List of permission strings granted to this CN.
                Use ["*"] for full access.
        description: Human-readable description for audit logs.
        enabled: Whether this entry is active. Disabled entries are ignored
                 during authentication.
    """

    cn: str
    scopes: list[str] = Field(default_factory=lambda: ["*"])
    description: str = ""
    enabled: bool = True


class WhitelistConfig(BaseModel):
    """Root schema for the mTLS CN whitelist YAML configuration.

    Attributes:
        version: Configuration format version for forward compatibility.
        entries: List of CN whitelist entries.
        default_scopes: Default scopes for CNs not in the whitelist but
                       passing CA verification. Empty list means reject
                       (fail-closed). Only used when ``auth_internal_mtls_enabled``
                       is True and the CN is not explicitly listed.
    """

    version: str = "1.0"
    entries: list[CNEntry] = Field(default_factory=list)
    default_scopes: list[str] = Field(default_factory=list)
    # NOTE: default_scopes is parsed from the YAML but NOT currently wired into
    # the auth decision path (auth.py always rejects unlisted CNs). It is retained
    # for forward compatibility so the schema does not need a version bump when
    # the feature is eventually implemented. Do NOT rely on it for fail-open
    # semantics — all CNs not explicitly listed are always denied.


class WhitelistManager:
    """Thread-safe mTLS CN whitelist manager with hot-reload support.

    The manager loads the whitelist from a YAML file on first access and
    reloads it when the file changes (detected via mtime). Reloading uses
    a two-phase commit: parse into a temporary buffer first, then atomically
    swap the cache on success.

    When no config file is provided, the manager falls back to a static
    list of CNs (all granted ["*"] scope) for backward compatibility with
    the ``PRIVACY_AUTH_MTLS_ALLOWED_CNS`` environment variable.

    Args:
        config_path: Path to the YAML whitelist configuration file.
                    If None, falls back to static list mode.
        static_cns: List of CNs for backward compatibility. Only used
                   when ``config_path`` is None.
    """

    def __init__(
        self,
        config_path: Path | None = None,
        static_cns: list[str] | None = None,
    ) -> None:
        self._config_path = config_path
        self._static_cns = static_cns or []
        self._lock = threading.RLock()
        self._cache: dict[str, CNEntry] = {}
        self._default_scopes: list[str] = []
        self._last_mtime: float = 0.0
        self._last_load_time: float = 0.0
        self._load_error: str | None = None

        # Initial load
        self._load()

    def _load(self) -> None:
        """Load or reload the whitelist configuration.

        Uses two-phase commit: parse into temporary buffer, then atomically
        swap on success. On failure, retains the old cache and logs the error.
        """
        if self._config_path is None:
            # Static list mode: build cache from static_cns
            with self._lock:
                self._cache = {
                    cn: CNEntry(cn=cn, scopes=["*"], description="Static env var entry")
                    for cn in self._static_cns
                }
                self._default_scopes = []
                self._last_load_time = time.time()
                self._load_error = None
            return

        path = Path(self._config_path)
        if not path.exists():
            with self._lock:
                self._load_error = f"Whitelist config file not found: {path}"
                logger.warning(self._load_error)
            return

        try:
            import yaml

            content = path.read_text(encoding="utf-8")
            raw = yaml.safe_load(content)
            if not isinstance(raw, dict):
                raise ValueError(f"Whitelist config must be a YAML object, got {type(raw).__name__}")

            # Parse into WhitelistConfig for validation
            config = WhitelistConfig.model_validate(raw)

            # Build new cache (temporary buffer)
            new_cache: dict[str, CNEntry] = {}
            for entry in config.entries:
                if entry.enabled:
                    new_cache[entry.cn] = entry
                else:
                    logger.debug(
                        "Skipping disabled CN whitelist entry",
                        extra={"cn": entry.cn},
                    )

            # Two-phase commit: atomically swap
            with self._lock:
                self._cache = new_cache
                self._default_scopes = config.default_scopes
                self._last_mtime = path.stat().st_mtime
                self._last_load_time = time.time()
                self._load_error = None

            logger.info(
                "mTLS CN whitelist loaded",
                extra={
                    "path": str(path),
                    "entry_count": len(new_cache),
                    "default_scopes": config.default_scopes,
                },
            )

        except Exception as exc:
            with self._lock:
                self._load_error = str(exc)
            logger.warning(
                "Failed to load mTLS CN whitelist, retaining previous config",
                extra={"path": str(path), "error": str(exc)},
            )

    def _check_reload(self) -> None:
        """Check if the config file has changed and trigger reload if needed.

        Called on every whitelist lookup to implement request-driven hot-reload.
        Uses mtime comparison for lightweight change detection.
        """
        if self._config_path is None:
            return

        path = Path(self._config_path)
        if not path.exists():
            return

        try:
            current_mtime = path.stat().st_mtime
        except OSError:
            return

        if current_mtime > self._last_mtime:
            logger.info(
                "mTLS CN whitelist config file changed, reloading",
                extra={"path": str(path)},
            )
            self._load()

    def get_entry(self, cn: str) -> CNEntry | None:
        """Look up a CN in the whitelist.

        Triggers a hot-reload check before lookup. Returns None if the CN
        is not in the whitelist or is disabled.

        Args:
            cn: Common Name from the client certificate.

        Returns:
            CNEntry if found and enabled, None otherwise.
        """
        self._check_reload()
        with self._lock:
            return self._cache.get(cn)

    def get_scopes(self, cn: str) -> list[str] | None:
        """Get the scopes for a CN.

        Convenience method that returns just the scopes list, or None if
        the CN is not whitelisted.

        Args:
            cn: Common Name from the client certificate.

        Returns:
            List of scope strings if CN is whitelisted, None otherwise.
        """
        entry = self.get_entry(cn)
        if entry is not None:
            return entry.scopes
        return None

    def is_allowed(self, cn: str) -> bool:
        """Check if a CN is in the whitelist and enabled.

        Args:
            cn: Common Name from the client certificate.

        Returns:
            True if CN is whitelisted and enabled, False otherwise.
        """
        return self.get_entry(cn) is not None

    @property
    def default_scopes(self) -> list[str]:
        """Default scopes for unknown CNs (empty = fail-closed)."""
        with self._lock:
            return self._default_scopes.copy()

    @property
    def all_entries(self) -> list[CNEntry]:
        """Return a snapshot of all active whitelist entries."""
        self._check_reload()
        with self._lock:
            return list(self._cache.values())

    @property
    def last_load_time(self) -> float:
        """Timestamp of the last successful load."""
        with self._lock:
            return self._last_load_time

    @property
    def last_error(self) -> str | None:
        """Error message from the last failed load, or None."""
        with self._lock:
            return self._load_error

    def reload(self) -> None:
        """Force a reload of the whitelist configuration.

        Useful for testing or when an operator wants to explicitly trigger
        a reload without waiting for the next request.
        """
        self._load()


# ---------------------------------------------------------------------------
# Module-level singleton
# ---------------------------------------------------------------------------

_manager: WhitelistManager | None = None
_manager_lock = threading.Lock()


def get_whitelist_manager() -> WhitelistManager:
    """Get or create the module-level WhitelistManager singleton.

    The manager is initialized from environment variables on first call:
    - ``PRIVACY_AUTH_MTLS_WHITELIST_FILE``: Path to YAML config file.
    - ``PRIVACY_AUTH_MTLS_ALLOWED_CNS``: Fallback static CN list (comma-separated
      or JSON array) when no config file is specified.

    Returns:
        The module-level WhitelistManager instance.
    """
    global _manager
    if _manager is not None:
        return _manager

    with _manager_lock:
        if _manager is not None:
            return _manager

        import os

        from .config import _load_str_list_env

        whitelist_file = os.environ.get("PRIVACY_AUTH_MTLS_WHITELIST_FILE", "").strip()
        if whitelist_file:
            config_path = Path(whitelist_file)
            static_cns = None
        else:
            config_path = None
            static_cns = _load_str_list_env("PRIVACY_AUTH_MTLS_ALLOWED_CNS")

        _manager = WhitelistManager(config_path=config_path, static_cns=static_cns)
        return _manager


def reset_whitelist_manager() -> None:
    """Reset the module-level singleton. For testing only."""
    global _manager
    with _manager_lock:
        _manager = None
