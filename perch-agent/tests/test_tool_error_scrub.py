# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the error-string scrubber in ToolErrorHandlerMiddleware.

Tool errors get fed back into the LLM context, which then surfaces them
to the user. We strip obvious secret patterns and bound length.
"""
from src.agent.middleware.tool_error_handler import _MAX_ERROR_CHARS, _scrub


def test_redacts_bearer_token():
    out = _scrub("upstream replied 401 Unauthorized: bearer eyJhbGciOiJIUzI1NiIsInR5c")
    assert "[REDACTED]" in out
    assert "eyJhbGc" not in out


def test_redacts_authorization_header_kv():
    out = _scrub("Authorization: Bearer abc.def.ghi")
    assert "[REDACTED]" in out
    assert "abc.def.ghi" not in out


def test_redacts_env_style_secret_assignments():
    for key in ("KEY", "TOKEN", "SECRET", "PASSWORD"):
        out = _scrub(f"failed: {key}=hunter2 expired")
        assert "hunter2" not in out
        assert "[REDACTED]" in out


def test_does_not_redact_unrelated_text():
    out = _scrub("ImagePullBackOff: registry returned 404")
    assert out == "ImagePullBackOff: registry returned 404"


def test_truncates_oversize_input():
    long_input = "x" * (_MAX_ERROR_CHARS + 500)
    out = _scrub(long_input)
    assert len(out) <= _MAX_ERROR_CHARS + 50  # truncation suffix overhead
    assert "truncated" in out


def test_redaction_and_truncation_compose():
    payload = "Bearer " + "y" * 2000 + " trailing context"
    out = _scrub(payload)
    assert "[REDACTED]" in out
    # The truncation marker is appended after redaction, so the final
    # length still fits the budget.
    assert len(out) <= _MAX_ERROR_CHARS + 50
