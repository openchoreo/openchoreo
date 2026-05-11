# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ChatRequest validation — particularly the new total-content cap."""
import pytest
from pydantic import ValidationError

from src.api.agent_routes import ChatMessage, ChatRequest, _TOTAL_CONTENT_LIMIT


def test_short_request_passes():
    req = ChatRequest(
        messages=[ChatMessage(role="user", content="hi")],
    )
    assert len(req.messages) == 1


def test_per_message_cap_enforced():
    with pytest.raises(ValidationError):
        ChatMessage(role="user", content="x" * 10_001)


def test_total_content_cap_rejects_overage():
    # Each message at 10k → 7 messages = 70k > 60k limit → reject.
    msgs = [ChatMessage(role="user", content="x" * 10_000) for _ in range(7)]
    with pytest.raises(ValidationError) as excinfo:
        ChatRequest(messages=msgs)
    assert "exceeds limit" in str(excinfo.value)


def test_total_content_cap_at_boundary():
    # Right at the limit must pass.
    msg = ChatMessage(role="user", content="x" * _TOTAL_CONTENT_LIMIT)
    req = ChatRequest(messages=[msg])
    assert sum(len(m.content) for m in req.messages) == _TOTAL_CONTENT_LIMIT


def test_extra_keys_rejected_by_chat_message():
    # ChatMessage has model_config={"extra": "forbid"}.
    with pytest.raises(ValidationError):
        ChatMessage(role="user", content="hi", extra_field="bad")


def test_message_count_cap_enforced():
    msgs = [ChatMessage(role="user", content="x") for _ in range(51)]
    with pytest.raises(ValidationError):
        ChatRequest(messages=msgs)
