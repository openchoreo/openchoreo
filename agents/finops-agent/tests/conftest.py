# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Shared pytest configuration.

Environment variables are set *before* any ``src`` module is imported so that
``src.config.Settings()`` (evaluated at import time) and the module-level
``FINOPS_AGENT`` / ``get_model()`` construction succeed without a real LLM
backend or network access.

These are assigned directly (not ``setdefault``) so the test environment is
hermetic: a stray ``REPORT_BACKEND=postgresql`` or ``SQL_BACKEND_URI`` in the
developer/CI shell can't leak in and break import-time config validation.

``openai:gpt-4o-mini`` is used because ``init_chat_model`` constructs the
``ChatOpenAI`` client lazily — no network call happens at construction time, so
importing the agent module is safe even with a dummy API key.
"""

import os

os.environ["LLM_NAME"] = "openai:gpt-4o-mini"
os.environ["LLM_API_KEY"] = "test-key"
os.environ["REPORT_BACKEND"] = "sqlite"
# Force-clear the URI so the sqlite default is applied and no stray postgres URI
# from the ambient env trips the scheme/backend cross-check in Settings.
os.environ["SQL_BACKEND_URI"] = ""
