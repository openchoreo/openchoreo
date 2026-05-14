# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ChatRequest validation — particularly the new total-content cap."""
import pytest
from pydantic import ValidationError

from src.api.agent_routes import (
    ChatMessage,
    ChatRequest,
    ChatScope,
    PrefetchedLogEntry,
    _TOTAL_CONTENT_LIMIT,
)


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


def test_prefetched_logs_accepts_camel_case_alias():
    """The frontend serializes ``prefetchedLogs``; the agent stores it
    as ``prefetched_logs``. Round-trip through the alias is the
    contract — break this and chats from Backstage stop seeing the
    pre-loaded rows even though pydantic happily ignores the field."""
    scope = ChatScope.model_validate(
        {
            "prefetchedLogs": [
                {
                    "timestamp": "2026-05-14T10:00:00Z",
                    "level": "ERROR",
                    "message": "db connection refused",
                    "componentName": "order-api",
                },
            ],
        }
    )
    assert scope.prefetched_logs is not None
    assert len(scope.prefetched_logs) == 1
    assert scope.prefetched_logs[0].message == "db connection refused"
    assert scope.prefetched_logs[0].component_name == "order-api"


def test_prefetched_logs_message_required():
    """A row with no message is meaningless — the agent has nothing to
    show the user from it. Forcing ``message`` to be present means
    the frontend trimmer's empty-message bug surfaces here instead
    of producing a useless prompt."""
    with pytest.raises(ValidationError):
        PrefetchedLogEntry(timestamp="2026-05-14T10:00:00Z", level="ERROR")


def test_prefetched_logs_row_count_capped():
    """Per the ChatScope.prefetched_logs Field(max_length=50). Without
    this, a malicious / buggy client could flood the prompt context
    without tripping the per-row size cap."""
    rows = [{"message": "x"} for _ in range(51)]
    with pytest.raises(ValidationError):
        ChatScope.model_validate({"prefetchedLogs": rows})


def test_total_content_counter_includes_prefetched_logs():
    """The total-content validator must count strings nested inside
    list-of-models. Earlier the validator only walked list-of-string
    items; a 50-row prefetched_logs list at the per-row cap would
    have been invisible to the budget check."""
    # Each row.message=2000 chars × 50 rows = 100k chars from
    # prefetched_logs alone → above the 60k limit when added to even
    # one short ChatMessage.
    huge_rows = [{"message": "x" * 2000} for _ in range(50)]
    msgs = [ChatMessage(role="user", content="hi")]
    with pytest.raises(ValidationError) as excinfo:
        ChatRequest.model_validate(
            {
                "messages": [m.model_dump() for m in msgs],
                "scope": {"prefetchedLogs": huge_rows},
            }
        )
    assert "exceeds limit" in str(excinfo.value)
