"""Pretty-print helpers for classification tests.

Default is silent. Enable output with ``PLA_CLASSIFICATION_TEST_PRINT_RESULTS=1``,
the ``enable_classification_print_results`` fixture, or
``set_print_results_enabled(True)`` in a test/debug session.
"""

from __future__ import annotations

import json
import os
from typing import Any

PRINT_RESULTS_ENV_VAR = "PLA_CLASSIFICATION_TEST_PRINT_RESULTS"


def set_print_results_enabled(enabled: bool = True) -> None:
    """Manually enable or disable pretty-print output via environment variable."""
    if enabled:
        os.environ[PRINT_RESULTS_ENV_VAR] = "1"
    else:
        os.environ.pop(PRINT_RESULTS_ENV_VAR, None)


def _is_enabled() -> bool:
    value = os.getenv(PRINT_RESULTS_ENV_VAR, "")
    return value.lower() in {"1", "true", "yes", "on"}


def _normalize(value: Any) -> Any:
    """Convert common test result objects into JSON-serializable data."""
    if hasattr(value, "model_dump"):
        return value.model_dump(mode="json")
    if isinstance(value, str):
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            return value
    return value


def pretty_json(value: Any) -> str:
    """Return a pretty-printed JSON string for test output."""
    return json.dumps(_normalize(value), ensure_ascii=False, indent=2, sort_keys=True)


def print_result(value: Any) -> None:
    """Print a pretty-printed representation of a test result."""
    if _is_enabled():
        print(pretty_json(value))

