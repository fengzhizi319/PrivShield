"""Shared pytest fixtures for classification tests.

Default is silent. Enable pretty-print output with the environment variable
``PLA_CLASSIFICATION_TEST_PRINT_RESULTS=1`` or the
``enable_classification_print_results`` fixture.
"""

from __future__ import annotations

import os

import pytest

from tests.classification._pretty import PRINT_RESULTS_ENV_VAR, set_print_results_enabled


@pytest.fixture(autouse=True)
def _restore_print_results_env():
    """Restore the print-results environment variable after each test."""
    original_value = os.environ.get(PRINT_RESULTS_ENV_VAR)
    yield
    if original_value is None:
        set_print_results_enabled(False)
    else:
        os.environ[PRINT_RESULTS_ENV_VAR] = original_value


@pytest.fixture
def enable_classification_print_results(monkeypatch):
    """Enable pretty-print output for a single test."""
    monkeypatch.setenv(PRINT_RESULTS_ENV_VAR, "1")
    yield

